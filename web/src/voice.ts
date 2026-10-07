import {t} from './i18n';
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
}
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
  private servers: RTCIceServer[] = [];
  private stream?: MediaStream;
  private busy = false;
  private micBusy = false;
  private heartbeat = 0;
  private soundEnabled = true;
  private mutedPlayers = new Set<string>();
  private resumeMic?: {roomId: string; seat: number};
  private outbound: Promise<unknown> = Promise.resolve();
  private timer: ReturnType<typeof setInterval>;
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
    this.timer = setInterval(() => void this.tick(), 500);
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
    document.addEventListener('visibilitychange', () => { if (document.hidden) void this.mute(); });
    window.addEventListener('preferans-voice-suspend', () => { void this.mute(); });
    window.addEventListener('pagehide', () => { this.stop(); clearInterval(this.timer); });
  }
  sync(status: {view: Room | null; connected: boolean[]; stun: string[]}) {
    const room = status.view;
    if (room?.id !== this.room?.id || room?.seat !== this.room?.seat) {
      this.stop();
      this.room = room;
      this.session = crypto.randomUUID();
      this.after = 0;
      this.heartbeat = 0;
    }
    this.room = room;
    this.connected = status.connected || [];
    this.servers = (status.stun || []).map(entry => {
      const [urls, username, credential] = entry.split('|');
      return {urls, ...(username ? {username, credential} : {})};
    });
    if (this.resumeMic && room && (room.id !== this.resumeMic.roomId || room.seat !== this.resumeMic.seat)) this.resumeMic = undefined;
    if (this.resumeMic && room?.id === this.resumeMic.roomId && room.seat === this.resumeMic.seat) {
      this.resumeMic = undefined;
      queueMicrotask(() => void this.toggle());
    }
    for (const [seat, peer] of this.peers) {
      if (!room || room.players[seat]?.bot !== false || !this.connected[seat]) { this.close(peer); this.peers.delete(seat); }
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
        if (Date.now()-peer.seen > 12000) { this.close(peer); this.peers.delete(seat); continue; }
        if (this.room && this.room.seat < seat && (!peer.pc || (peer.pc.connectionState !== 'connected' && Date.now()-peer.started > 18000))) {
          await this.offer(seat, peer);
        }
      }
    } catch { /* Game reconnects independently; keep mic state and retry. */ }
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
        peer = {session:m.session, mic:m.mic, seen:Date.now(), started:0, attempts:0, sent:false, ice:[]};
        this.peers.set(m.from, peer);
      }
      peer.seen = Date.now(); peer.mic = m.mic;
    }
    if (!peer || peer.session !== m.session) return;
    if (m.kind === 'offer' && this.room.seat > m.from) {
      this.close(peer);
      peer.attempts++;
      peer.call = m.call;
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
      if (m.kind === 'ice') await peer.pc.addIceCandidate(JSON.parse(m.payload!));
    }
  }
  private create(seat: number, peer: Remote) {
    const turns = this.servers.filter(s => String(s.urls).startsWith('turn'));
    // TURN first; the next attempt includes STUN/direct candidates.
    const relay = turns.length > 0 && peer.attempts < 2;
    const pc = new RTCPeerConnection({iceServers:relay ? turns : this.servers, iceTransportPolicy:relay ? 'relay' : 'all'});
    peer.pc = pc; peer.started = Date.now(); peer.sent = false; peer.ice = [];
    pc.onicecandidate = e => {
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
      void audio.play().catch(() => { audio.dataset.blocked = 'true'; this.paint(); });
    };
    pc.onconnectionstatechange = () => this.paint();
    return pc;
  }
  private async offer(seat: number, peer: Remote) {
    this.close(peer); peer.attempts++; peer.call = crypto.randomUUID();
    const pc = this.create(seat, peer);
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
      const track = stream.getAudioTracks()[0];
      track.onended = () => {
        if (this.stream?.getAudioTracks().includes(track)) {
          this.notify(t('web.voice.text017'));
          void this.mute();
        }
      };
      await Promise.all([...this.peers.values()].map(p => p.sender?.replaceTrack(track).catch(() => {})));
      await this.send('hello');
    } catch (e) {
      await this.mute();
      this.notify(e instanceof DOMException && e.name === 'NotAllowedError' ? t("web.voice.text014") : `${t("web.voice.text013", {p0: e instanceof Error ? e.message : e})}`);
    } finally { this.micBusy = false; this.paint(); }
  }
  private async mute() {
    const stream = this.stream; this.stream = undefined;
    stream?.getTracks().forEach(t => t.stop());
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
    for (const peer of this.peers.values()) this.close(peer);
    this.peers.clear(); this.room = null;
  }
  private paint() {
    for(const [seat,peer] of this.peers)if(peer.audio)peer.audio.muted=!this.soundEnabled || this.mutedPlayers.has(this.speakerKey(seat));
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
      const ready = own || peer?.pc?.connectionState === 'connected';
      const blocked = !own && !!peer?.audio?.dataset.blocked;
      const label = own ? (this.micBusy ? t("web.voice.text007") : on ? t("web.voice.text006") : t("web.voice.text005"))
        : blocked ? t("web.voice.text004") : !ready ? t("web.voice.text003") : on ? t("web.voice.text002") : t("web.voice.text001");
      el.classList.toggle('muted', !on); el.classList.toggle('offline', !ready); el.classList.toggle('audio-blocked', blocked);
      el.title = label; el.setAttribute('aria-label', label);
      if (own) { el.setAttribute('aria-pressed', String(on)); (el as HTMLButtonElement).disabled = this.micBusy; }
    }
  }
  private speakerKey(seat:number){return `${this.room?.id}/${this.room?.players[seat]?.id || seat}`;}
}
