type RPC = <T = unknown>(action: string, params?: Record<string, unknown>) => Promise<T>;

// Запасной путь голоса: если прямое WebRTC-соединение двух игроков не поднялось, кадры Opus (WebCodecs, 20 мс, 24 кбит/с)
// идут по игровому каналу через стол: гость -> ведущий -> остальные. Звук не сохраняется и не попадает в журнал.
const RATE = 48000;
const FRAME = 960; // 20 мс при 48 кГц
const FRAME_US = 20000;
const PER_BATCH = 3; // кадров в пачке (60 мс)

const b64 = (u: Uint8Array) => { let s = ''; for (let i = 0; i < u.length; i++) s += String.fromCharCode(u[i]); return btoa(s); };
const unb64 = (s: string) => { const bin = atob(s), u = new Uint8Array(bin.length); for (let i = 0; i < bin.length; i++) u[i] = bin.charCodeAt(i); return u; };

export const relaySupported = () => {
  const w = window as any;
  return typeof w.AudioEncoder === 'function' && typeof w.AudioDecoder === 'function' && typeof w.AudioData === 'function' && typeof w.EncodedAudioChunk === 'function' && typeof AudioContext !== 'undefined';
};

interface Sink { dec: any; gain: GainNode; next: number; ts: number }

// Ограничитель после усиления: без него усиленная речь клипуется и хрипит. Ниже порога почти не меняет звук.
export function makeLimiter(ctx: AudioContext): DynamicsCompressorNode {
  const c = ctx.createDynamicsCompressor();
  c.threshold.value = -3; c.knee.value = 0; c.ratio.value = 20; c.attack.value = 0.003; c.release.value = 0.15;
  return c;
}

export class RelayAudio {
  private boost = 1;
  private ctx?: AudioContext;
  private src?: MediaStreamAudioSourceNode;
  private proc?: ScriptProcessorNode;
  private enc?: any;
  private acc = new Float32Array(FRAME * 4);
  private accN = 0;
  private ts = 0;
  private batch: Uint8Array[] = [];
  private seq = 0;
  private inflight = 0;
  private sinks = new Map<number, Sink>();
  private audible = new Map<number, boolean>();
  stats = {sent: 0, received: 0, dropped: 0};
  constructor(private rpc: RPC, private log: (message: string) => void) {}

  get capturing() { return !!this.proc; }
  get exists() { return !!this.ctx; }
  ensure() { if (relaySupported()) this.context(); }
  get running() { return this.ctx?.state === 'running'; }
  private context() {
    if (!this.ctx) {
      this.ctx = new AudioContext({sampleRate: RATE, latencyHint: 'interactive'});
      this.log(`звук через стол: AudioContext ${this.ctx.sampleRate} Гц, состояние ${this.ctx.state}`);
    }
    return this.ctx;
  }
  resume() { if (this.ctx && this.ctx.state !== 'running') void this.ctx.resume().catch(() => {}); }

  startCapture(stream: MediaStream) {
    if (this.proc || !relaySupported()) return;
    const ctx = this.context();
    this.resume();
    const w = window as any;
    this.enc = new w.AudioEncoder({
      output: (chunk: any) => { const u = new Uint8Array(chunk.byteLength); chunk.copyTo(u); this.push(u); },
      error: (e: any) => this.log(`звук через стол: ошибка кодера Opus: ${e?.message || e}`),
    });
    this.enc.configure({codec: 'opus', sampleRate: RATE, numberOfChannels: 1, bitrate: 24000, opus: {frameDuration: FRAME_US, application: 'voip', signal: 'voice'}});
    this.src = ctx.createMediaStreamSource(stream);
    this.proc = ctx.createScriptProcessor(1024, 1, 1);
    this.proc.onaudioprocess = e => this.capture(e.inputBuffer.getChannelData(0));
    this.src.connect(this.proc);
    this.proc.connect(ctx.destination); // без подключения к выходу обработчик не вызывается; выход остаётся тишиной
    this.accN = 0; this.ts = 0; this.batch = [];
    this.log('звук через стол: передача включена');
  }
  stopCapture() {
    if (!this.proc) return;
    this.flush();
    this.proc.onaudioprocess = null;
    try { this.src?.disconnect(); this.proc.disconnect(); } catch { /* уже отключено */ }
    this.proc = undefined; this.src = undefined;
    try { this.enc?.close(); } catch { /* уже закрыт */ }
    this.enc = undefined;
    this.log('звук через стол: передача выключена');
  }
  private capture(input: Float32Array) {
    if (!this.enc || this.enc.state === 'closed') return;
    if (this.accN + input.length > this.acc.length) this.accN = 0; // защита от переполнения
    this.acc.set(input, this.accN);
    this.accN += input.length;
    const w = window as any;
    while (this.accN >= FRAME) {
      if (this.enc.encodeQueueSize > 12) this.stats.dropped++;
      else {
        const data = new w.AudioData({format: 'f32-planar', sampleRate: RATE, numberOfFrames: FRAME, numberOfChannels: 1, timestamp: this.ts, data: this.acc.slice(0, FRAME)});
        this.enc.encode(data);
        data.close();
      }
      this.ts += FRAME_US;
      this.acc.copyWithin(0, FRAME, this.accN);
      this.accN -= FRAME;
    }
  }
  private push(frame: Uint8Array) {
    this.batch.push(frame);
    if (this.batch.length >= PER_BATCH) this.flush();
  }
  private flush() {
    const frames = this.batch.splice(0);
    if (!frames.length) return;
    if (this.inflight >= 3) { this.stats.dropped += frames.length; return; } // канал не успевает: звук не копим
    this.inflight++;
    const seq = this.seq;
    this.seq += frames.length;
    this.stats.sent++;
    void this.rpc('audio-send', {audio: {seq, ts: Date.now(), frames: frames.map(b64)}}).catch(() => { this.stats.dropped += frames.length; }).finally(() => { this.inflight--; });
  }

  setAudible(seat: number, on: boolean) {
    this.audible.set(seat, on);
    const sink = this.sinks.get(seat);
    if (sink) sink.gain.gain.value = on ? this.boost : 0;
  }
  // Усиление звука, пришедшего через стол (1 — без усиления).
  setBoost(value: number) {
    this.boost = value;
    for (const [seat, sink] of this.sinks) sink.gain.gain.value = this.audible.get(seat) === false ? 0 : value;
  }
  private sink(seat: number): Sink | undefined {
    let s = this.sinks.get(seat);
    if (s) return s;
    if (!relaySupported()) return undefined;
    const ctx = this.context();
    const w = window as any;
    const gain = ctx.createGain();
    gain.gain.value = this.audible.get(seat) === false ? 0 : this.boost;
    const limiter = makeLimiter(ctx);
    gain.connect(limiter); limiter.connect(ctx.destination);
    const dec = new w.AudioDecoder({
      output: (data: any) => this.out(seat, data),
      error: (e: any) => this.log(`звук через стол: ошибка декодера Opus (место ${seat + 1}): ${e?.message || e}`),
    });
    dec.configure({codec: 'opus', sampleRate: RATE, numberOfChannels: 1});
    s = {dec, gain, next: 0, ts: 0};
    this.sinks.set(seat, s);
    return s;
  }
  play(seat: number, frames: string[]) {
    const s = this.sink(seat);
    if (!s || s.dec.state === 'closed') return;
    this.stats.received++;
    const w = window as any;
    for (const f of frames) {
      try { s.dec.decode(new w.EncodedAudioChunk({type: 'key', timestamp: s.ts, data: unb64(f)})); } catch { this.stats.dropped++; }
      s.ts += FRAME_US;
    }
  }
  private out(seat: number, data: any) {
    const s = this.sinks.get(seat), ctx = this.ctx;
    const n = data.numberOfFrames as number;
    if (!s || !ctx || ctx.state !== 'running') { data.close(); this.stats.dropped++; return; }
    const arr = new Float32Array(n);
    data.copyTo(arr, {planeIndex: 0, format: 'f32-planar'});
    data.close();
    const buf = ctx.createBuffer(1, n, RATE);
    buf.copyToChannel(arr, 0);
    const node = ctx.createBufferSource();
    node.buffer = buf;
    node.connect(s.gain);
    const now = ctx.currentTime;
    if (s.next < now + 0.02) s.next = now + 0.1; // небольшой буфер против дрожания задержки
    else if (s.next > now + 0.6) { this.stats.dropped++; return; } // накопилась задержка: старый звук отбрасываем
    node.start(s.next);
    s.next += n / RATE;
  }
  prune(seats: Set<number>) {
    for (const [seat, s] of this.sinks) {
      if (seats.has(seat)) continue;
      try { s.dec.close(); } catch { /* уже закрыт */ }
      s.gain.disconnect();
      this.sinks.delete(seat);
    }
  }
  stopAll() {
    this.stopCapture();
    this.prune(new Set());
  }
}
