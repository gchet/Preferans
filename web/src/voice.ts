import {t} from './i18n';
import {RelayAudio, relaySupported} from './voice-relay';
type RPC = <T = unknown>(action: string, params?: Record<string, unknown>) => Promise<T>;
interface Room { id: string; seat: number; players: {bot: boolean; id?:string}[] }
interface Signal {
  table: string; from: number; to: number; session: string; target?: string;
  call?: string; kind: string; payload?: string; mic: boolean;
}
interface Remote {
  session: string; mic: boolean; seen: number; pc?: RTCPeerConnection;
  sender?: RTCRtpSender; audio?: HTMLAudioElement; call?: string;
  started: number; attempts: number; sent: boolean; ice: RTCIceCandidateInit[];
  // Диагностика для журнала: счётчики кандидатов по типам (адреса не сохраняются) и признак потока аудио.
  lc: Record<string, number>; rc: Record<string, number>; checked: number; rx: number; flow: boolean;
  // Запасной путь: звук этого игрока идёт по игровому каналу через стол, пока WebRTC не поднялся.
  viaRelay: boolean; noConn: number;
}
export type VoicePath = 'auto' | 'webrtc' | 'table';
const candType = (c?: string) => / typ (\w+)/.exec(c || '')?.[1] || '?';
const fmtCount = (c: Record<string, number>) => Object.entries(c).map(([k, v]) => `${k}:${v}`).join(',') || '-';
const micSVG = '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="9" y="2" width="6" height="12" rx="3"/><path d="M5 10v2a7 7 0 0 0 14 0v-2M12 19v3M8 22h8"/><path class="mic-slash" d="M3 3l18 18"/></svg>';
export function voiceIcon(seat: number, own: boolean): string {
  const mic = own ? `<button type="button" class="voice-mic muted" data-microphone="${seat}" aria-label="${t("web.voice.text005")}" aria-pressed="false">${micSVG}</button>`
    : `<span class="voice-mic muted offline" data-voice-seat="${seat}" role="img" aria-label="${t("web.voice.text016")}">${micSVG}</span>`;
  return mic + `<button type="button" class="voice-speaker" data-speaker="${own?'all':seat}" aria-label="${own?t("web.voice.text012"):t("web.voice.text010")}" aria-pressed="true"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 9h4l5-4v14l-5-4H4z"/><path class="speaker-waves" d="M16 8a6 6 0 0 1 0 8M19 5a10 10 0 0 1 0 14"/><path class="speaker-slash" d="m16 9 6 6m0-6-6 6"/></svg></button>`;
}

// A small audio mesh (at most four humans). Signalling uses the authenticated
// game's data channels; audio never traverses the game's command queue.
export class VoiceChat {
  private room: Room | null = null;
  private session = '';
  private after = 0;
  private peers = new Map<number, Remote>();
  private connected: boolean[] = [];
  // Голос идёт браузерным WebRTC: только публичные STUN-серверы, без TURN.
  private readonly servers: RTCIceServer[] = [{urls: 'stun:stun.l.google.com:19302'}, {urls: 'stun:stun.cloudflare.com:3478'}];
  private stream?: MediaStream;
  private busy = false;
  private micBusy = false;
  private heartbeat = 0;
  private soundEnabled = true;
  private mutedPlayers = new Set<string>();
  private resumeMic?: {roomId: string; seat: number};
  private restoreMicrophone = false;
  private backgrounded = false;
  private outbound: Promise<unknown> = Promise.resolve();
  private timer: ReturnType<typeof setInterval>;
  private relayTimer: ReturnType<typeof setInterval>;
  private relay: RelayAudio;
  private pathMode: VoicePath = 'auto';
  private relayAfter = 0;
  private relayRunning = false;
  private relayLogAt = 0;
  private logBuf: string[] = [];
  private logTimer = 0;
  private logErrAt = 0;
  // События голоса уходят в журнал приложения (пачками): по ним видно, на каком шаге рвётся связь между игроками.
  private log(message: string) {
    this.logBuf.push(message);
    if (!this.logTimer) this.logTimer = window.setTimeout(() => this.flushLog(), 1500);
  }
  private flushLog() {
    this.logTimer = 0;
    const lines = this.logBuf.splice(0);
    if (lines.length) void this.rpc('voice-log', {message: lines.join('\n')}).catch(() => {});
  }
  private async pairInfo(pc: RTCPeerConnection): Promise<{text: string; rx: number}> {
    try {
      const stats = await pc.getStats();
      const byId = new Map<string, any>();
      stats.forEach(s => byId.set(s.id, s));
      let pair: any;
      stats.forEach(s => { if (s.type === 'transport' && s.selectedCandidatePairId) pair = byId.get(s.selectedCandidatePairId); });
      if (!pair) stats.forEach(s => { if (s.type === 'candidate-pair' && s.nominated && s.state === 'succeeded') pair = s; });
      const states: Record<string, number> = {};
      let rx = 0, tx = 0;
      stats.forEach(s => {
        if (s.type === 'candidate-pair') states[s.state] = (states[s.state] || 0) + 1;
        if (s.type === 'inbound-rtp' && s.kind === 'audio') rx += s.bytesReceived || 0;
        if (s.type === 'outbound-rtp' && s.kind === 'audio') tx += s.bytesSent || 0;
      });
      const l = pair && byId.get(pair.localCandidateId), rm = pair && byId.get(pair.remoteCandidateId);
      const chosen = pair ? `${l?.candidateType}/${l?.protocol}${l?.networkType ? '/' + l.networkType : ''} ↔ ${rm?.candidateType}/${rm?.protocol}, rtt ${pair.currentRoundTripTime ?? '?'} с` : 'нет';
      return {text: `пары ${fmtCount(states)}; выбрана ${chosen}; принято ${rx} Б, отправлено ${tx} Б`, rx};
    } catch (e) { return {text: 'статистика недоступна', rx: 0}; }
  }
  private async checkFlow(seat: number, peer: Remote) {
    const pc = peer.pc;
    if (!pc) return;
    const info = await this.pairInfo(pc);
    if (peer.pc !== pc) return;
    const flow = peer.rx >= 0 && info.rx > peer.rx;
    if (peer.rx < 0) this.log(`место ${seat + 1}: проверка связи: ${info.text}`);
    else if (flow !== peer.flow) this.log(`место ${seat + 1}: аудио ${flow ? 'идёт' : 'не принимается'} (микрофон там ${peer.mic ? 'включён' : 'выключен'}); ${info.text}`);
    peer.flow = flow; peer.rx = info.rx; peer.checked = Date.now();
  }
  constructor(private rpc: RPC, private notify: (message: string) => void) {
    try {
      const saved = sessionStorage.getItem('preferans-voice-resume');
      if (saved) {
        sessionStorage.removeItem('preferans-voice-resume');
        const value = JSON.parse(saved) as {roomId?: unknown; seat?: unknown};
        if (typeof value.roomId === 'string' && typeof value.seat === 'number')
          this.resumeMic = {roomId:value.roomId, seat:value.seat};
      }
    } catch { /* Session storage may be unavailable in restricted WebViews. */ }
    this.relay = new RelayAudio(rpc, message => this.log(message));
    try {
      const mode = localStorage.getItem('preferans-voice-path');
      if (mode === 'webrtc' || mode === 'table') this.pathMode = mode;
    } catch { /* Хранилище может быть недоступно: остаётся автоматический выбор. */ }
    this.timer = setInterval(() => void this.tick(), 100);
    this.relayTimer = setInterval(() => this.relayTick(), 250);
    window.addEventListener('pagehide', () => this.flushLog());
    document.addEventListener('click', e => {
      const speaker=(e.target as Element).closest<HTMLButtonElement>('[data-speaker]');
      if(speaker && this.room){
        if(speaker.dataset.speaker==='all')this.soundEnabled=!this.soundEnabled;
        else {
          const key=this.speakerKey(Number(speaker.dataset.speaker));
          if(this.mutedPlayers.has(key))this.mutedPlayers.delete(key);else this.mutedPlayers.add(key);
        }
        this.paint();this.playAudio();return;
      }
      if ((e.target as Element).closest('[data-microphone]')) void this.toggle();
    });
    document.addEventListener('pointerdown', () => this.playAudio());
    window.addEventListener('preferans-voice-background', () => {
      this.backgrounded = true;
      this.restoreMicrophone = !!this.stream;
    });
    window.addEventListener('preferans-voice-resume', () => {
      this.backgrounded = false;
      const restore = this.restoreMicrophone;
      this.restoreMicrophone = false;
      if (restore && !this.stream) void this.toggle();
    });
    // Android sends this only when the device is locked (screen turned off).
    window.addEventListener('preferans-voice-lock', () => {
      this.backgrounded = false;
      this.restoreMicrophone = false;
      void this.mute();
    });
    window.addEventListener('pagehide', () => { this.stop(); clearInterval(this.timer); clearInterval(this.relayTimer); });
  }
  get path(): VoicePath { return this.pathMode; }
  // Как идёт голос: автоматически (WebRTC, а для не соединившейся пары — через стол), только WebRTC или только через стол.
  setPath(mode: VoicePath) {
    if (mode === this.pathMode) return;
    this.pathMode = mode;
    try { localStorage.setItem('preferans-voice-path', mode); } catch { /* значение действует до перезагрузки */ }
    this.log(`способ передачи голоса: ${mode === 'auto' ? 'автоматически' : mode === 'webrtc' ? 'только WebRTC' : 'только через стол'}${mode === 'table' && !relaySupported() ? ' (WebCodecs недоступен, остаётся WebRTC)' : ''}`);
    for (const peer of this.peers.values()) {
      peer.noConn = 0;
      if (this.tableOnly) this.close(peer);
    }
    this.paint();
  }
  private get tableOnly() { return this.pathMode === 'table' && relaySupported(); }
  private relayTick() {
    const via = [...this.peers].filter(([, p]) => p.viaRelay).map(([seat]) => seat);
    if (!this.room || !via.length) {
      this.relay.stopCapture();
      this.relay.prune(new Set());
      return;
    }
    this.relay.ensure();
    this.relay.prune(new Set(via));
    if (this.stream && !this.relay.capturing) this.relay.startCapture(this.stream);
    else if (!this.stream && this.relay.capturing) this.relay.stopCapture();
    if (!this.relayRunning) void this.relayLoop(this.session);
    if (Date.now() - this.relayLogAt > 20000) {
      if (this.relayLogAt) { const s = this.relay.stats; this.log(`звук через стол: пачек отправлено ${s.sent}, принято ${s.received}, отброшено ${s.dropped}; звук ${this.relay.running ? 'разрешён' : 'ждёт нажатия на экран'}`); s.sent = s.received = s.dropped = 0; }
      this.relayLogAt = Date.now();
    }
  }
  private async relayLoop(session: string) {
    this.relayRunning = true;
    try {
      while (this.room && session === this.session && [...this.peers.values()].some(p => p.viaRelay)) {
        let res: {events: {seq: number; batch: {from: number; frames: string[]}}[]; last: number};
        try { res = await this.rpc('audio-poll', {after: this.relayAfter}); }
        catch { await new Promise(r => setTimeout(r, 500)); continue; }
        if (session !== this.session) return;
        for (const ev of res.events) {
          const peer = this.peers.get(ev.batch.from);
          if (peer?.viaRelay) this.relay.play(ev.batch.from, ev.batch.frames);
        }
        this.relayAfter = res.last;
      }
    } finally { this.relayRunning = false; }
  }
  sync(status: {view: Room | null; connected: boolean[]}) {
    const room = status.view;
    if (room?.id !== this.room?.id || room?.seat !== this.room?.seat) {
      this.stop();
      if (room) this.log(`голос: стол ${room.id.slice(0, 6)}, место ${room.seat + 1}`);
      this.room = room;
      this.session = crypto.randomUUID();
      this.after = 0;
      this.relayAfter = 0;
      this.heartbeat = 0;
    }
    this.room = room;
    this.connected = status.connected || [];
    if (this.resumeMic && room && (room.id !== this.resumeMic.roomId || room.seat !== this.resumeMic.seat)) this.resumeMic = undefined;
    if (this.resumeMic && room?.id === this.resumeMic.roomId && room.seat === this.resumeMic.seat) {
      this.resumeMic = undefined;
      queueMicrotask(() => void this.toggle());
    }
    for (const [seat, peer] of this.peers) {
      if (!room || room.players[seat]?.bot !== false || !this.connected[seat]) {
        this.log(`место ${seat + 1}: голос закрыт (${!room ? 'нет стола' : room.players[seat]?.bot !== false ? 'место занято ботом или пусто' : 'игровой канал с местом не подключён'})`);
        this.close(peer); this.peers.delete(seat);
      }
    }
    this.paint();
  }
  preserveMicrophoneOnReload() {
    if (!this.stream || !this.room) return;
    try {
      sessionStorage.setItem('preferans-voice-resume', JSON.stringify({roomId:this.room.id, seat:this.room.seat}));
    } catch { /* A reload without storage will require the user to re-enable the mic. */ }
  }
  private send(kind: string, to = -1, payload?: unknown, peer?: Remote) {
    if (!this.room) return Promise.resolve();
    const voice: Signal = {table:this.room.id, from:this.room.seat, to, session:this.session, kind,
      mic:!!this.stream, ...(payload === undefined ? {} : {payload:JSON.stringify(payload)}), target:peer?.session, call:peer?.call};
    const session = this.session;
    const pending = this.outbound.catch(() => {}).then(() => {
      if (session === this.session) return this.rpc('voice-send', {voice});
    });
    this.outbound = pending;
    return pending;
  }
  private async tick() {
    if (this.busy || !this.room || typeof RTCPeerConnection === 'undefined') return;
    this.busy = true;
    const session = this.session;
    try {
      if (Date.now()-this.heartbeat > 3000) { await this.send('hello'); this.heartbeat = Date.now(); }
      const events = await this.rpc<{seq:number; message:Signal}[]>('voice-poll', {id:this.room.id, after:this.after});
      if (session !== this.session) return;
      for (const event of events) {
        if (session !== this.session) return;
        try { await this.receive(event.message); } catch { /* A new offer retries a failed negotiation. */ }
        this.after = event.seq;
      }
      for (const [seat, peer] of this.peers) {
        if (Date.now()-peer.seen > 12000) { this.log(`место ${seat + 1}: нет приветствий 12 с, голос закрыт`); this.close(peer); this.peers.delete(seat); continue; }
        if (peer.pc?.connectionState === 'connected' && Date.now() - peer.checked > 20000) { peer.checked = Date.now(); void this.checkFlow(seat, peer); }
        const up = peer.pc?.connectionState === 'connected';
        if (up) peer.noConn = 0; else if (!peer.noConn) peer.noConn = Date.now();
        const via = this.tableOnly || this.pathMode === 'auto' && peer.noConn > 0 && Date.now() - peer.noConn > 25000 && relaySupported();
        if (via !== peer.viaRelay) {
          peer.viaRelay = via;
          this.log(via ? `место ${seat + 1}: голос через стол (${this.tableOnly ? 'выбрано в настройках' : `WebRTC не поднялся за ${Math.round((Date.now() - peer.noConn) / 1000)} с`})` : `место ${seat + 1}: голос снова по WebRTC`);
        }
        if (!this.tableOnly && this.room && this.room.seat < seat && (!peer.pc || (peer.pc.connectionState !== 'connected' && Date.now()-peer.started > 18000))) {
          await this.offer(seat, peer);
        }
      }
    } catch (e) {
      /* Game reconnects independently; keep mic state and retry. */
      if (Date.now() - this.logErrAt > 30000) { this.logErrAt = Date.now(); this.log(`сигналы голоса: ошибка ${e instanceof Error ? e.message : String(e)}`); }
    }
    finally { this.busy = false; this.paint(); }
  }
  private async receive(m: Signal) {
    if (!this.room || m.table !== this.room.id || m.from === this.room.seat || this.room.players[m.from]?.bot !== false || !this.connected[m.from]) return;
    if (m.kind !== 'hello' && m.kind !== 'bye' && m.target !== this.session) return;
    let peer = this.peers.get(m.from);
    if (m.kind === 'bye') { if (peer?.session === m.session) { this.close(peer); this.peers.delete(m.from); } return; }
    if (m.kind === 'hello' || m.kind === 'offer') {
      if (!peer || peer.session !== m.session) {
        if (peer) this.close(peer);
        peer = {session:m.session, mic:m.mic, seen:Date.now(), started:0, attempts:0, sent:false, ice:[], lc:{}, rc:{}, checked:0, rx:-1, flow:false, viaRelay:false, noConn:0};
        this.peers.set(m.from, peer);
        this.log(`место ${m.from + 1}: игрок в голосе (микрофон ${m.mic ? 'включён' : 'выключен'})`);
      }
      peer.seen = Date.now(); peer.mic = m.mic;
    }
    if (!peer || peer.session !== m.session) return;
    if (m.kind === 'offer' && this.room.seat > m.from && !this.tableOnly) {
      this.close(peer);
      peer.attempts++;
      peer.call = m.call;
      if (peer.attempts <= 3 || peer.attempts % 10 === 0) this.log(`место ${m.from + 1}: получен offer, отвечаем (попытка ${peer.attempts})`);
      const pc = this.create(m.from, peer);
      await pc.setRemoteDescription(JSON.parse(m.payload!));
      const transceiver = pc.getTransceivers()[0];
      transceiver.direction = 'sendrecv';
      peer.sender = transceiver.sender;
      await peer.sender.replaceTrack(this.stream?.getAudioTracks()[0] || null);
      await pc.setLocalDescription(await pc.createAnswer());
      await this.send('answer', m.from, pc.localDescription, peer);
      await this.flushICE(m.from, peer);
    } else if (m.call && m.call === peer.call && peer.pc) {
      if (m.kind === 'answer' && peer.pc.signalingState === 'have-local-offer') await peer.pc.setRemoteDescription(JSON.parse(m.payload!));
      if (m.kind === 'ice') {
        const cand = JSON.parse(m.payload!) as RTCIceCandidateInit;
        const type = candType(cand.candidate);
        peer.rc[type] = (peer.rc[type] || 0) + 1;
        try { await peer.pc.addIceCandidate(cand); } catch (e) { this.log(`место ${m.from + 1}: addIceCandidate: ${e instanceof Error ? e.message : String(e)}`); }
      }
    }
  }
  private create(seat: number, peer: Remote) {
    const pc = new RTCPeerConnection({iceServers:this.servers});
    peer.pc = pc; peer.started = Date.now(); peer.sent = false; peer.ice = []; peer.lc = {}; peer.rc = {}; peer.rx = -1; peer.flow = false; peer.checked = Date.now();
    const verbose = () => peer.attempts <= 3 || peer.attempts % 10 === 0;
    let wasConnected = false;
    const state = (what: string) => {
      if (peer.pc !== pc || !verbose() && !wasConnected) return;
      this.log(`место ${seat + 1}: ${what}: соединение=${pc.connectionState} ice=${pc.iceConnectionState} сбор=${pc.iceGatheringState} (${((Date.now() - peer.started) / 1000).toFixed(1)} с, попытка ${peer.attempts}); кандидаты свои ${fmtCount(peer.lc)}, чужие ${fmtCount(peer.rc)}`);
    };
    pc.oniceconnectionstatechange = () => state('ICE');
    pc.onicegatheringstatechange = () => {
      state('сбор кандидатов');
      if (pc.iceGatheringState === 'complete' && peer.pc === pc && verbose() && !peer.lc.srflx && !peer.lc.relay) this.log(`место ${seat + 1}: нет кандидатов srflx — STUN не ответил или заблокирован`);
    };
    pc.onicecandidateerror = e => {
      if (peer.pc === pc && verbose()) { const x = e as RTCPeerConnectionIceErrorEvent; this.log(`место ${seat + 1}: ошибка ICE ${x.errorCode} ${x.errorText || ''} ${x.url || ''}`); }
    };
    pc.onicecandidate = e => {
      if (e.candidate && peer.pc === pc) { const type = e.candidate.type || candType(e.candidate.candidate); peer.lc[type] = (peer.lc[type] || 0) + 1; }
      if (!e.candidate || peer.pc !== pc) return;
      if (peer.sent) void this.send('ice', seat, e.candidate.toJSON(), peer).catch(() => {});
      else peer.ice.push(e.candidate.toJSON());
    };
    pc.ontrack = e => {
      if (peer.pc !== pc) return;
      const audio = document.createElement('audio');
      audio.autoplay = true;
      audio.muted = !this.soundEnabled || this.mutedPlayers.has(this.speakerKey(seat));
      audio.srcObject = new MediaStream([e.track]);
      audio.dataset.voiceAudio = String(seat);
      peer.audio?.remove(); peer.audio = audio;
      document.body.append(audio);
      if (verbose()) this.log(`место ${seat + 1}: получен аудиотрек`);
      void audio.play().catch(() => { audio.dataset.blocked = 'true'; this.log(`место ${seat + 1}: воспроизведение заблокировано системой`); this.paint(); });
    };
    pc.onconnectionstatechange = () => {
      this.paint();
      const st = pc.connectionState;
      if (st === 'connected') wasConnected = true;
      if (peer.pc !== pc || !verbose() && !wasConnected) return;
      state('связь');
      if (st === 'connected' || st === 'failed' || st === 'disconnected') void this.pairInfo(pc).then(info => { if (peer.pc === pc) this.log(`место ${seat + 1}: ${st}: ${info.text}`); });
    };
    return pc;
  }
  private async offer(seat: number, peer: Remote) {
    this.close(peer); peer.attempts++; peer.call = crypto.randomUUID();
    const pc = this.create(seat, peer);
    if (peer.attempts <= 3 || peer.attempts % 10 === 0) this.log(`место ${seat + 1}: отправляем offer (попытка ${peer.attempts})`);
    peer.sender = pc.addTransceiver('audio', {direction:'sendrecv'}).sender;
    await peer.sender.replaceTrack(this.stream?.getAudioTracks()[0] || null);
    await pc.setLocalDescription(await pc.createOffer());
    await this.send('offer', seat, pc.localDescription, peer);
    await this.flushICE(seat, peer);
  }
  private async flushICE(seat: number, peer: Remote) {
    peer.sent = true;
    for (const ice of peer.ice.splice(0)) await this.send('ice', seat, ice, peer);
  }
  private playAudio() {
    this.relay.resume();
    for (const peer of this.peers.values()) if (peer.audio) void peer.audio.play().then(() => { delete peer.audio!.dataset.blocked; }).catch(() => {});
  }
  private async toggle() {
    if (!this.room || this.micBusy) return;
    this.playAudio();
    if (this.stream) { await this.mute(); return; }
    const session = this.session;
    this.micBusy = true; this.paint();
    try {
      if (!navigator.mediaDevices?.getUserMedia) throw Error(t("web.voice.text015"));
      const stream = await navigator.mediaDevices.getUserMedia({audio:{echoCancellation:true, noiseSuppression:true, autoGainControl:true}, video:false});
      if (session !== this.session || !this.room || document.hidden) { stream.getTracks().forEach(t => t.stop()); return; }
      this.stream = stream;
      this.log('микрофон включён');
      const track = stream.getAudioTracks()[0];
      track.onended = () => {
        if (this.stream?.getAudioTracks().includes(track)) {
          if (this.backgrounded) this.restoreMicrophone = true;
          this.log('микрофон отключён системой');
          this.notify(t('web.voice.text017'));
          void this.mute();
        }
      };
      await Promise.all([...this.peers.values()].map(p => p.sender?.replaceTrack(track).catch(() => {})));
      await this.send('hello');
      this.heartbeat = Date.now();
      void this.tick();
    } catch (e) {
      this.log(`микрофон: ошибка ${e instanceof Error ? e.name + ' ' + e.message : String(e)}`);
      await this.mute();
      this.notify(e instanceof DOMException && e.name === 'NotAllowedError' ? t("web.voice.text014") : `${t("web.voice.text013", {p0: e instanceof Error ? e.message : e})}`);
    } finally { this.micBusy = false; this.paint(); }
  }
  private async mute() {
    const stream = this.stream; this.stream = undefined;
    if (stream) this.log('микрофон выключен');
    stream?.getTracks().forEach(t => t.stop());
    this.relay.stopCapture();
    await Promise.all([...this.peers.values()].map(p => p.sender?.replaceTrack(null).catch(() => {})));
    this.paint();
    if (stream && this.room) await this.send('hello').catch(() => {});
  }
  private close(peer: Remote) {
    peer.pc?.close(); peer.pc = undefined; peer.sender = undefined;
    if (peer.audio) { peer.audio.pause(); peer.audio.srcObject = null; peer.audio.remove(); peer.audio = undefined; }
  }
  private stop() {
    this.stream?.getTracks().forEach(t => t.stop()); this.stream = undefined;
    this.restoreMicrophone = false; this.backgrounded = false;
    for (const peer of this.peers.values()) this.close(peer);
    this.relay.stopAll();
    this.peers.clear(); this.room = null;
  }
  private paint() {
    for(const [seat,peer] of this.peers){
      const audible=this.soundEnabled && !this.mutedPlayers.has(this.speakerKey(seat));
      if(peer.audio)peer.audio.muted=!audible;
      this.relay.setAudible(seat,audible);
    }
    for(const el of document.querySelectorAll<HTMLButtonElement>('[data-speaker]')){
      const own=el.dataset.speaker==='all';
      const on=own?this.soundEnabled:!this.mutedPlayers.has(this.speakerKey(Number(el.dataset.speaker)));
      const label=own?(on?t("web.voice.text012"):t("web.voice.text011")):(on?t("web.voice.text010"):t("web.voice.text009"));
      el.classList.toggle('muted',!on);
      el.classList.toggle('master-muted',!own&&!this.soundEnabled);
      el.setAttribute('aria-pressed',String(on));el.disabled=!this.room;
      el.title=label+(!own&&!this.soundEnabled?t("web.voice.text008"):'');el.setAttribute('aria-label',el.title);
    }
    for (const el of document.querySelectorAll<HTMLElement>('[data-microphone], [data-voice-seat]')) {
      const own = el.hasAttribute('data-microphone');
      const peer = this.peers.get(Number(el.dataset.voiceSeat));
      const on = own ? !!this.stream : !!peer?.mic;
      const ready = own || peer?.pc?.connectionState === 'connected' || !!peer?.viaRelay;
      const blocked = !own && (!!peer?.audio?.dataset.blocked || !!peer?.viaRelay && this.relay.exists && !this.relay.running);
      const label = own ? (this.micBusy ? t("web.voice.text007") : on ? t("web.voice.text006") : t("web.voice.text005"))
        : blocked ? t("web.voice.text004") : !ready ? t("web.voice.text003") : on ? t("web.voice.text002") : t("web.voice.text001");
      el.classList.toggle('muted', !on); el.classList.toggle('offline', !ready); el.classList.toggle('audio-blocked', blocked);
      el.title = label; el.setAttribute('aria-label', label);
      if (own) { el.setAttribute('aria-pressed', String(on)); (el as HTMLButtonElement).disabled = this.micBusy; }
    }
  }
  private speakerKey(seat:number){return `${this.room?.id}/${this.room?.players[seat]?.id || seat}`;}
}
