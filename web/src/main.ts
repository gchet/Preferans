import {AIBotsUI} from "./ai-bots";
import {AIReviewUI,type AIReview} from "./ai-review";
import {DebugBotUI, installDebugBotMenu} from './debug-bot';
import {t, locale, language, normalizeLanguage, localizeMessage, localizeResponse} from './i18n';
import {interfaceConfig} from './interface-config';
import {networkConfig} from './network-config';
import {
  type Appearance,
  appearance,
  applyAppearance,
  prepareCards,
  decks,
  backs,
} from "./appearance";
import "./style.css";
import './table-layout.css';
import './table-layout-horizontal-hand.css';
import './misere-tracker.css';
import './start-layout.css';
import './voice.css';
import { fitCardFans } from './card-fan-layout';
window.addEventListener('resize', () => requestAnimationFrame(fitCardFans));
import { wholeWhists } from "./settlement";
import {historyTotals, type HistoryEntry} from './history';
import {defaultRules, rulesForm, readRules, rulesSummary, type TableRules} from './rules';
import { cardDrag } from "./card-drag";
import { ordered, face, poolDrawing, remainingMinutes } from "./table-art";
import { VoiceChat, voiceIcon } from './voice';
import {RoomUI,roomVoice,type RoomStatus} from './rooms';
import './rooms.css';
import './exit-app';
import {buttonPressActive} from './button-feedback';
import {setupScreen, setupServers, enhanceConnectionSettings, settingsServers} from './connection-setup';
document.addEventListener('click', e=>{
  const button=(e.target as Element).closest<HTMLButtonElement>('#verify-turn');
  if(!button)return;
  void run(async()=>{
    const result=$('#turn-check-result');
    result.textContent=t("web.main.text283");button.disabled=true;
    try {
      await rpc('connection-setup',{stun:settingsServers($('#settings-form'))});
      result.textContent=t("web.main.text282");
      const input=document.querySelector<HTMLInputElement>('[name="turn-password"]')!;
      const servers=settingsServers($('#settings-form'));
      document.querySelector<HTMLTextAreaElement>('[name="stun"]')!.value=servers.filter(s=>s.startsWith('turn:')||s.startsWith('turns:')).join('\n');
      input.defaultValue=input.value;
    } catch(error) {result.textContent=error instanceof Error?error.message:String(error);throw error;}
    finally{button.disabled=false;}
  });
});

interface Contract {
  noTalon?: boolean;
  level: number;
  suit: number;
  misere: boolean;
}
interface Player {
  botModel?: string;
  name: string;
  bot: boolean;
  ready: boolean;
  count: number;
  cards?: number[];
}
interface Ledger {
  amnestyTricks?: number;
  amnesty?: number;
  round: number;
  label: string;
  pool: number[];
  mountain: number[];
  whists: number[][];
}
interface View {
  aiReviewSeat?: number;
  debugBotSeat?: number;
  debugWaitingSeat?: number;
  botThinking?: {seat:number; startedAt:number};
  misereCards?: number[];
  miserePlayed?: number[];
  startedAt?: number;
  finishedAt?: number;
  manualPaused?: boolean;
  pausedBy?: number;
  pausedAt?: number;
  pauseSeconds?: number;
	debugBots?: string;
  talonShown?: boolean;
  claim?: number;
  rules?: TableRules;
  deadline?: number;
  id: string;
  version: string;
  revision: number;
  seat: number;
  players: Player[];
  stage: string;
  target: number;
  round: number;
  dealer: number;
  turn: number;
  actor: number;
  hand: number[];
  talon?: number[];
  trick: { seat: number; card: number }[];
  lastTrick?: { seat: number; card: number }[];
  trickNo: number;
  taken: number[];
  winner: number;
  bid: Contract | null;
  contract: Contract | null;
  declarer: number;
  defence: number[];
  halfSeat?: number;
  dealerLook?: number;
  dealerWhist?: boolean;
  passed: boolean[];
  open: boolean;
  allPass: boolean;
  passPrice: number;
  passStreak: number;
  actions: string[];
  contracts: Contract[];
  legal: number[];
  playHand: number[];
  pool: number[];
  mountain: number[];
  whists: number[][];
  results: number[];
  history: Ledger[];
}
interface Status extends RoomStatus {
  aiReview?: AIReview;
  aiEnabled?: boolean;
  connectionSetup?: boolean;
  logPath?: string;
  logSize?: number;
  logPathOverride?: boolean;
  freeSeats: number;
  current: boolean;
  appearance: Appearance;
  view: View | null;
  connected: boolean[];
  paused: boolean;
  error: string;
  host: boolean;
  stun: string[];
  links: Record<string, string>;
  connectionReasons?: Record<string,string>;
  logs?: string[];
}
interface SaveInfo {
  id: string;
  round: number;
  stage: string;
  host: boolean;
  names: string[];
}
interface UpdateManifest {
  versionCode?: number;
  versionName?: string;
  apk?: string;
}
const $ = <T extends HTMLElement = HTMLElement>(s: string) =>
  document.querySelector<T>(s)!;
const esc = (v: unknown) =>
  String(v ?? "").replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ]!,
  );
const token =
  location.hash.slice(1) || sessionStorage.getItem("preferans-token") || "";
sessionStorage.setItem("preferans-token", token);
history.replaceState(null, "", location.pathname);
let status: Status = {
  freeSeats: 0,
  appearance,
  view: null,
  connected: [],
  paused: false,
  error: "",
  host: false,
  current: false,
  stun: [],
  links: {},
};
let selected: number[] = [];
let contractContext: {revision: number; action: "bid" | "declare" | "declare-no-talon"} | null = null;
let previous = "";
let startWaitingTable = '';
let busy = false;
let lastError = "";
let pendingConnection: {table: string; seat: number} | null = null;
let signalingBusy = false;
let autoJoining = false;
let signalingTrace: string[] = [];
let lastSignalingTrace = "";
let submittedOfferCode = "";
let submittedOfferAt = 0;
let pendingUpdateURL = "";
let autoUpdateChecked = false;
type NativeUpdateCallback = (ok: boolean, payload: string) => void;
const nativeUpdateCallbacks: Record<number, NativeUpdateCallback> = {};
let nativeUpdateRequestID = 0;
// Android performs the manifest request natively because some WebView builds
// block localhost-to-LAN requests even when the server sends CORS headers.
(window as any).__preferansUpdateResult = (id: number, ok: boolean, payload: string) => {
  const callback = nativeUpdateCallbacks[id];
  if (!callback) return;
  delete nativeUpdateCallbacks[id];
  callback(ok, payload);
};
function requestNativeUpdateManifest(base: URL): Promise<string> {
  return new Promise((resolve, reject) => {
    const id = ++nativeUpdateRequestID;
    const timer = window.setTimeout(() => {
      delete nativeUpdateCallbacks[id];
      reject(Error(t("web.main.text281")));
    }, 15000);
    nativeUpdateCallbacks[id] = (ok, payload) => {
      window.clearTimeout(timer);
      if (ok) resolve(payload);
      else reject(Error(payload || t("web.main.text280")));
    };
    try {
      window.PreferansAndroid!.checkUpdate(base.toString(), id);
    } catch (e) {
      window.clearTimeout(timer);
      delete nativeUpdateCallbacks[id];
      reject(e);
    }
  });
}
const signalingURL = networkConfig.legacySignalingURL;
function traceSignaling(message: string) {
  const line = `${new Date().toLocaleTimeString(locale)}  signaling: ${message}`;
  if (message === lastSignalingTrace) return;
  lastSignalingTrace = message;
  signalingTrace.push(line);
  if (signalingTrace.length > 80) signalingTrace = signalingTrace.slice(-80);
  renderConnectionLog();
}
async function signalingRequest(method: "GET"|"POST", body?: unknown) {
  if (!signalingURL) throw Error(t('web.connection.server_required'));
  const target = method === "GET" ? `${signalingURL}?t=${Date.now()}` : signalingURL;
  traceSignaling(`${method} ${signalingURL}`);
  try {
    const r = await fetch(target, {method, cache: "no-store", headers: body ? {"Content-Type":"application/json"} : undefined, body: body ? JSON.stringify(body) : undefined});
    if (!r.ok) throw Error(`HTTP ${r.status}`);
    const result = r.status === 204 ? null : await r.json();
    traceSignaling(`${t("web.main.text279", {p0: method})}`);
    return result;
  } catch (e) {
    traceSignaling(`${t("web.main.text278", {p0: method, p1: e instanceof Error ? e.message : String(e)})}`);
    throw e;
  }
}
function normalizeSignaling(raw: any) {
  return { offer: raw?.offer || raw?.Offer, answer: raw?.answer || raw?.Answer } as {offer?: {code?: string; seat?: number; name?: string}; answer?: {code?: string; seat?: number}};
}
async function autoConnect() {
  if(status.roomMode)return;
  if (signalingBusy || (!status.host && !autoJoining)) return;
  signalingBusy = true;
  const v = status.view;
  try {
    const current = normalizeSignaling(await signalingRequest("GET"));
    if (!status.host && !autoJoining) return;
    if (status.host && v) {
      traceSignaling(`${t("web.main.text277", {p0: current.offer?.code ? t("web.main.text276") : t("web.main.text275"), p1: current.answer?.code ? t("web.main.text276") : t("web.main.text275")})}`);
      if (current.offer?.code && typeof current.offer.seat === "number" && current.offer.seat > 0 && !status.connected?.[current.offer.seat] && current.answer?.code && current.answer.seat === current.offer.seat) {
        try {
          await rpc("answer", {code: current.answer.code});
        } catch (e) {
          const message = e instanceof Error ? e.message : String(e);
          if (!message.includes(t("web.main.text274")) && !message.includes(t("web.main.text273"))) throw e;
          traceSignaling(t("web.main.text272"));
        } finally {
          await signalingRequest("POST",{kind:"clear"});
        }
      } else if (!current.offer?.code && v.players.length > 1) {
        const seat = v.players.findIndex((p,i) => i > 0 && !p.bot && !status.connected?.[i]);
        const linkState = seat > 0 ? status.links[String(seat)] : "";
        const inProgress = ["gathering", "waiting-answer", "connecting", "authenticating", "connected"].includes(linkState);
        if (seat > 0 && !inProgress) { const code = await rpc("invite", {seat}); await signalingRequest("POST",{kind:"offer",code,seat,name:v.players[0].name}); }
      }
    } else if (autoJoining && !v && current.offer?.code && current.offer.seat && (current.offer.code !== submittedOfferCode || Date.now() - submittedOfferAt > 15000)) {
      if (["gathering", "connecting", "authenticating", "connected"].includes(status.links['0']) && Date.now()-submittedOfferAt < 45000) return;
      traceSignaling(`${t("web.main.text271", {p0: current.offer.seat + 1})}`);
      submittedOfferCode = current.offer.code; submittedOfferAt = Date.now(); const answer = await rpc<string>("join",{code:current.offer.code}); await signalingRequest("POST",{kind:"answer",code:answer,seat:current.offer.seat});
    } else if (!v && !current.offer?.code) {
      if (!["gathering", "connecting", "authenticating", "connected"].includes(status.links['0'])) submittedOfferCode = "";
    } else if (!v) {
      traceSignaling(t("web.main.text270"));
    }
  } catch (e) { traceSignaling(`${t("web.main.text269", {p0: e instanceof Error ? e.message : String(e)})}`); }
  finally { signalingBusy = false; }
}
let currentPanel = "";
const safeJoin = (value: string[] | null | undefined, separator = "\n") =>
  (value || []).join(separator);
const suits = ["♠\uFE0E", "♣\uFE0E", "♦\uFE0E", "♥\uFE0E", t("web.main.text268")],
  ranks = ["7", "8", "9", "10", t("web.main.text267"), t("web.main.text266"), t("web.main.text265"), t("web.main.text264")];
const contract = (c: Contract | null): string =>
  c?.noTalon && c.suit === -1 ? t("go.contract.nine_no_talon") : c?.noTalon ? contract({...c,noTalon:false}) + t("go.contract.no_talon_suffix") : !c ? t("web.main.text247") : c.misere ? t("web.main.text098") : `${["", "", "", "", "", "", t("web.main.text263"), t("web.main.text262"), t("web.main.text261"), t("web.main.text260"), t("web.main.text259")][c.level]} ${[t("web.main.text211"), t("web.main.text210"), t("web.main.text209"), t("web.main.text208"), t("web.main.text207")][c.suit]}`;
const stages: Record<string, string> = {
  lobby: t("web.main.text258"),
  auction: t("web.main.text247"),
  discard: t("web.main.text257"),
  contract: t("web.main.text212"),
  defend: t("web.main.text255"),
  catch: t("web.main.text256"),
  option: t("web.main.text255"),
  return: t("web.main.text254"),
  "dealer-choice": t("web.dealer_whist.stage"),
  "dealer-whist": t("web.dealer_whist.stage"),
  mode: t("web.main.text253"),
  play: t("web.main.text252"),
  trick: t("web.main.text251"),
  round: t("web.main.text250"),
  finished: t("web.main.text184"),
};
function tableVoiceIcon(seat:number,own:boolean) {
  if (soloWithBots(status.view)) return "";
  const id=status.room?.table?.players[seat];
  const slot=status.room?.members.find(m=>m.id===id)?.slot;
  return voiceIcon(slot ?? seat,own);
}
function soloWithBots(view: View | null) {
  return !!view && view.players.filter(p => !p.bot).length === 1;
}
const debugAuction = () => appearance.debugAuction ?? !!appearance.debug;
const debugPlay = () => appearance.debugPlay ?? !!appearance.debug;
const debugEnabled = () => debugAuction() || debugPlay();
function passLabel(v: View): string {
  return appearance.passPriceHint !== false
    ? t("web.pass_price.label", {p0: v.passPrice})
    : t("web.main.text099");
}
function role(v: View, i: number): string {
  if (v.stage === "lobby") return v.players[i].ready ? t("web.main.text243") : t("web.main.text249");
  if (v.players.length === 4 && v.dealer === i) return v.dealerWhist ? t("web.main.text237") : t("web.main.text248");
  if (v.allPass) return passLabel(v);
  if (v.stage === "discard" && v.passed?.[i]) return t("web.main.text206");
  if (v.stage === "auction")
    return v.passed?.[i]
      ? t("web.main.text206")
      : v.declarer === i
        ? contract(v.bid)
        : t("web.main.text247");
  if (v.declarer === i) return contract(v.contract || v.bid);
  if (v.contract?.misere) return v.defence?.[i] === 2 ? t("web.main.text236") : v.defence?.[i] === 1 ? t("web.main.text235") : t("web.main.text246");
  if(v.halfSeat===i)return halfLabel(v);
  return (
    ({ 1: t("web.main.text206"), 2: t("web.main.text237"), 3: halfLabel(v) } as Record<number, string>)[
      v.defence?.[i]
    ] ||
    (v.contract?.misere || (v.contract?.level === 10 && v.rules?.tenCheck !== false) ? t("web.main.text245") : t("web.main.text244"))
  );
}
const actions: Record<string, string> = {
  "dealer-skip": t("web.dealer_whist.no"),
  "dealer-first": t("web.dealer_whist.first"),
  "dealer-second": t("web.dealer_whist.second"),
  "fill-bots": t("web.main.text161"),
  ready: t("web.main.text243"),
  start: t("web.main.text242"),
  next: t("web.main.text241"),
  collect: t("web.main.text240"),
  pass: t("web.main.text206"),
  bid: t("web.main.text239"),
  declare: t("web.main.text238"),
  "declare-no-talon": t("web.main.text144"),
  whist: t("web.main.text237"),
  catch: t("web.main.text236"),
  trust: t("web.main.text235"),
  half: t("web.main.text234"),
  open: t("web.main.text233"),
  closed: t("web.main.text232"),
  play: t("web.main.text231"),
};
async function rpc<T = unknown>(
  action: string,
  params: Record<string, unknown> = {},
): Promise<T> {
  const res = await fetch("/api", {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Preferans-Token": token },
    body: JSON.stringify({ action, ...params }),
  });
  if (!res.ok) throw Error(`${t("web.main.text230", {p0: res.status})}`);
  const body = await res.json();
  if (body.error) throw Error(localizeMessage(body.error));
  return localizeResponse(body.data as T);
}
let noticeTimer: ReturnType<typeof setTimeout> | undefined;
function updateLogFileSize(s: Status) {
  const el=document.querySelector('#log-file-size');
  if(!el)return;
  const bytes=s.logSize;
  if(bytes===undefined || bytes<0){el.textContent=t('web.log.size_unknown');return;}
  const unit=Math.min(3,bytes ? Math.floor(Math.log(bytes)/Math.log(1024)) : 0);
  const size=(bytes/1024**unit).toLocaleString(locale,{maximumFractionDigits:unit ? 1 : 0});
  el.textContent=t('web.log.size',{p0:size,p1:t(['web.log.bytes','web.log.kb','web.log.mb','web.log.gb'][unit])});
  el.setAttribute('title',t('web.log.size',{p0:bytes.toLocaleString(locale),p1:t('web.log.bytes')}));
}
function amnestyText(l: Ledger): string {
  return l.amnestyTricks ? t('web.score.amnesty_tricks',{p0:l.amnestyTricks,p1:(l.amnesty || 0)/l.amnestyTricks}) : t('web.score.amnesty',{p0:l.amnesty || 0});
}
function syncAISettingsTab() {
  const form=document.querySelector('#settings-form');
  if(!form)return;
  const tab=document.querySelector<HTMLButtonElement>('#settings-tab-ai');
  const section=document.querySelector<HTMLElement>('#settings-ai');
  if(status.aiEnabled===false) {
    if(tab?.getAttribute('aria-selected')==='true')document.querySelector<HTMLButtonElement>('#settings-tab-game')?.click();
    tab?.remove();section?.remove();
  } else if(!section) {
    document.querySelector('.settings-tabs')!.insertAdjacentHTML('beforeend',`<button type="button" role="tab" id="settings-tab-ai" data-settings-tab="ai" aria-controls="settings-ai" aria-selected="false">${t('web.ai.tab')}</button>`);
    form.querySelector(':scope > button.primary')!.insertAdjacentHTML('beforebegin','<section id="settings-ai" role="tabpanel" aria-labelledby="settings-tab-ai" hidden></section>');
    void aiUI.mount($('#settings-ai'),appearance.updateURL || '',!!appearance.debug);
  }
}
function notice(s: string) {
  clearTimeout(noticeTimer);
  pendingConnection = null;
  const panel = $<HTMLDialogElement>('#panel');
  (panel.open ? panel : document.body).append($('#notice'));
  $("#notice").textContent = s;
  $("#notice").classList.toggle("visible", !!s);
  if (s) noticeTimer = setTimeout(() => notice(''), interfaceConfig.noticeDurationMs);
}
const voice = new VoiceChat(rpc, notice);
$('#notice').addEventListener('click', () => notice(''));
$('#panel').addEventListener('close', () => {
  document.body.append($('#notice'));
  if (currentPanel === "contract") { $('#panel-body').innerHTML = ""; contractContext = null;  }
});
document.addEventListener('dblclick', (event) => {
  const target = event.target as HTMLElement;
  if (target.closest('button, input, select, textarea, a, label')) return;
  const text = target.textContent?.trim();
  if (!text || text.length > 20000) return;
  void navigator.clipboard?.writeText(text).then(() => notice(t("web.main.text229")));
});
function card(c: number, selectable = false, disabled = false) {
  const suit = Math.floor(c / 8);
  return `<button class="card ${suit >= 2 ? "red" : ""} ${selected.includes(c) ? "selected" : ""}" ${selectable ? `data-card="${c}"` : 'tabindex="-1"'} ${disabled ? "disabled" : ""} aria-label="${ranks[c % 8]} ${[t("web.main.text228"), t("web.main.text227"), t("web.main.text226"), t("web.main.text225")][suit]}" ${selectable ? `aria-pressed="${selected.includes(c)}"` : ""}>${face(c)}</button>`;
}
function groupedCards(cards: number[], draw: (c: number) => string) {
  return [0, 1, 2, 3].map(suit => {
    const group = ordered(cards).filter(c => (c >> 3) === suit);
    return group.length ? `<div class="suit-group" data-suit="${suit}" style="--suit-count:${group.length}">${group.map(draw).join("")}</div>` : "";
  }).join("");
}
function halfLabel(v: View) {
  return v.contract?.level === 7 ? t("web.main.text224") : t("web.main.text223");
}
function connectionDetail(seat: number): string {
  const reason=status.connectionReasons?.[seat] || 'waiting';
  return t(`web.connection.${reason}`);
}
function talonThrown(v: View) { return v.players.length === 4 && v.talonShown; }
function miniCard(c: number, classes = '', suffix = ''): string {
  return `<div class="mini-card ${(c >> 3) >= 2 ? 'red' : 'black'} ${classes}" role="img" aria-label="${esc(ranks[c % 8] + suits[c >> 3] + suffix)}"><span class="mini-face" aria-hidden="true"><b>${ranks[c % 8]}</b><span>${suits[c >> 3]}</span></span>${classes.includes('was-played') ? '<span class="played-mark" aria-hidden="true">✓</span>' : ''}</div>`;
}
function lastTrick(v: View): string {
  const cards = (v.lastTrick || []).filter(p => p.seat >= 0);
  return cards.length && ['play','trick','round','finished'].includes(v.stage) ? `<section class="last-trick" aria-label="${t('web.table.last_trick')}"><small>${t('web.table.last_trick')}</small><div>${cards.map(p => {
    const relative = (p.seat - v.seat + v.players.length) % v.players.length;
    const position = relative === 0 ? 'self' : relative === 1 ? 'left' : v.players.length === 4 && relative === 2 ? 'top' : 'right';
    return `<div class="mini-seat-${position}" title="${esc(v.players[p.seat]?.name)}">${miniCard(p.card)}</div>`;
  }).join('')}</div></section>` : '';
}
function misereTracker(v: View): string {
  return `<section class="misere-tracker" aria-label="${t("web.main.text221")}"><small>${t("web.main.text222", {p0: v.misereCards!.length})}</small><div class="misere-tracker-grid">${ordered(v.misereCards!).map(c => {
    const played = v.miserePlayed?.includes(c);
    return miniCard(c, `misere-tracker-card ${played ? 'was-played' : ''}`, played ? t("web.main.text220") : '');
  }).join('')}</div></section>`;
}
function trickCount(n: number) { return `${n} ${n === 1 ? t("web.main.text219") : n >= 2 && n <= 4 ? t("web.main.text218") : t("web.main.text217")}`; }
function openClaim() {
  const v = status.view!;
  if (!v.actions.includes("claim")) return;
  showPanel("claim");
  $("#panel-title").textContent = t("web.main.text216");
  $("#panel-body").innerHTML = `<label>${t("web.main.text215")}<select id="claim-tricks">${Array.from({length:11-v.trickNo},(_,n)=>`<option value="${n}">${t("web.main.text214", {p1: trickCount(n), p2: (v.taken[v.seat] || 0)+n})}</option>`).join("")}</select></label><button data-action="submit-claim" class="primary">${t("web.main.text154")}</button>`;
}
function openContract(action: "bid" | "declare" | "declare-no-talon") {
  const v = status.view!;
  if (!(v.actions || []).includes(action) || (action === "declare" && !talonThrown(v) && selected.length !== 2)) return;
  contractContext = {revision: v.revision, action};
  
  showPanel("contract");
  $("#panel-title").textContent = action === "bid" ? t("web.main.text213") : t("web.main.text212");
  const noTalonDeclaration = action === "declare-no-talon";
  const grid = (noTalonDeclaration ? [9] : [6,7,8,9,10]).flatMap(level => [0,1,2,3,4].map(suit => {
    const index = v.contracts.findIndex(c => !!c.noTalon === noTalonDeclaration && !c.misere && c.level === level && c.suit === suit);
    return `<button class="contract-cell ${suit === 2 || suit === 3 ? "red" : ""}" data-contract-index="${index}" ${index < 0 ? "disabled" : ""} aria-label="${level} ${[t("web.main.text211"),t("web.main.text210"),t("web.main.text209"),t("web.main.text208"),t("web.main.text207")][suit]}">${level}<span>${suits[suit]}</span></button>`;
  })).join("");
  const misere = v.contracts.findIndex(c => c.misere && !c.noTalon);
  const noTalonMisere = v.contracts.findIndex(c => c.misere && c.noTalon);
  const noTalonNines = v.contracts.map((c,index)=>({c,index})).filter(({c})=>c.noTalon && !c.misere && c.suit === -1).map(({c,index})=>`<button data-contract-index="${index}">${contract(c)}</button>`).join("");
  const noTalonBids = `${noTalonMisere >= 0 ? `<button data-contract-index="${noTalonMisere}">${contract(v.contracts[noTalonMisere])}</button>` : ""}${noTalonNines}`;
  // The engine includes the current bid only for the senior of two remaining bidders.
  const here = action === "bid" && v.bid ? v.contracts.findIndex(c => c.level === v.bid!.level && c.suit === v.bid!.suit && !!c.misere === !!v.bid!.misere && !!c.noTalon === !!v.bid!.noTalon) : -1;
  $("#panel-body").innerHTML = `<p class="muted">${action === "bid" ? t("web.main.text204") : noTalonDeclaration ? t("web.contract.no_talon_declare") : talonThrown(v) ? t("web.main.text203") : t("web.main.text202")}</p><div class="contract-grid">${grid}</div>${noTalonBids ? `<div class="contract-no-talon">${noTalonBids}</div>` : ""}<div class="contract-footer">${here >= 0 ? `<button data-contract-index="${here}">${t("web.main.text205")}</button>` : ""}${misere >= 0 ? `<button data-contract-index="${misere}">${t("web.main.text098")}</button>` : ""}${action === "bid" && (v.actions || []).includes("pass") ? `<button data-action="pass">${t("web.main.text206")}</button>` : ""}</div>`;
}
async function submitContract(c: Contract) {
  const v = status.view!;
  const context = contractContext;
  if (!context || context.revision !== v.revision || !(v.actions || []).includes(context.action)) throw Error(t("web.main.text201"));
  await rpc("command", {command: {id: crypto.randomUUID(), revision:v.revision, seat:v.seat, action:context.action, contract:c, cards:context.action === "declare" && !talonThrown(v) ? selected : []}});
  selected = []; 
  $<HTMLDialogElement>("#panel").close();
}
let summaryKey = "";
let summaryHidden = false;
let summaryTimer: ReturnType<typeof setTimeout> | undefined;
function dismissSummary() {
  summaryHidden = true;
  clearTimeout(summaryTimer);
  document.querySelector('[data-dismiss-summary]')?.remove();
}
function syncSummary(v: View | null) {
  const key = v && ["round", "finished"].includes(v.stage) ? `${v.id}/${v.round}/${v.stage}` : "";
  if (key === summaryKey) return;
  clearTimeout(summaryTimer);
  summaryKey = key;
  summaryHidden = false;
  if (key && v?.stage === "round") summaryTimer = setTimeout(dismissSummary, interfaceConfig.roundSummaryDurationMs);
}
document.addEventListener("click", e => {
  const target = e.target as HTMLElement;
  if (target.closest("[data-dismiss-summary]") && (!target.closest("button") || target.closest("[data-close-summary]"))) dismissSummary();
});
document.addEventListener("keydown", e => {
  if ((e.key === "Enter" || e.key === " ") && (e.target as HTMLElement).matches("[data-dismiss-summary]")) {
    e.preventDefault();
    dismissSummary();
  }
});
function clockText(seconds: number): string { return `${Math.floor(seconds/60)}:${String(seconds%60).padStart(2,'0')}`; }
function partyDates(v: {startedAt?: number; finishedAt?: number}): string {
  const format = (time?: number) => time ? new Date(time*1000).toLocaleString(locale, {day:'2-digit',month:'2-digit',year:'numeric',hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false}) : t("web.main.text200");
  return `<p class="party-dates">${t("web.main.text198", {p0: format(v.startedAt)})}<br>${t("web.main.text199", {p1: format(v.finishedAt)})}</p>`;
}
function tableProgress(v: View): string {
  if (v.pausedAt) return `${t("web.main.text194")} <span>${clockText(Math.max(0,Math.floor(Date.now()/1000)-v.pausedAt))}</span><small class="pause-owner">${esc(v.aiReviewSeat!==undefined ? t("web.ai.review-pause",{p0:v.players[v.aiReviewSeat]?.name||""}) : v.manualPaused ? v.players[v.pausedBy || 0]?.name : t("web.main.text197"))}</small>`;
  if (v.rules?.endCondition !== 'time') return `${t("web.main.text006")} <span>${v.pool.reduce((a,b)=>a+b,0)} / ${v.target*v.players.length}</span>`;
  const seconds = v.stage === 'finished' ? 0 : v.deadline ? Math.max(0,v.deadline-Math.floor(Date.now()/1000)) : v.rules.minutes*60;
  return `${t("web.main.text196")} <span>${Math.floor(seconds/60)}:${String(seconds%60).padStart(2,'0')}</span>`;
}
function updateBotThinking() {
  const thinking=status.paused ? undefined : status.view?.botThinking;
  for (const icon of document.querySelectorAll<HTMLElement>('.bot-model-icon')) {
    let indicator=icon.parentElement!.querySelector<HTMLElement>('.bot-thinking');
    if (!thinking || Number(icon.dataset.botSeat)!==thinking.seat) { indicator?.remove(); continue; }
    if (!indicator) {
      indicator=document.createElement('span');
      indicator.className='bot-thinking';
      indicator.title=t('web.ai.thinking');
      indicator.innerHTML='<svg viewBox="0 0 16 20" aria-hidden="true"><path d="M2 1h12v4l-5 5 5 5v4H2v-4l5-5-5-5Z" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M4 17h8l-4-4Z" fill="currentColor"/></svg><span></span>';
      icon.parentElement!.append(indicator);
    }
    const elapsed=Math.max(0,Math.floor((Date.now()-thinking.startedAt)/1000));
    indicator.querySelector('span')!.textContent=t('web.ai.elapsed',{p0:elapsed});
    indicator.setAttribute('aria-label',`${t('web.ai.thinking')}: ${t('web.ai.elapsed',{p0:elapsed})}`);
  }
}
function botModelIcon(v:View,seat:number) {
  if(status.aiEnabled === false && !debugEnabled()) return "";
  if(status.host && v.debugWaitingSeat===seat) return `<button type="button" class="bot-model-icon" data-bot-seat="${seat}" title="${esc(t('web.debug.wait',{p0:v.players[seat].name}))}" aria-label="${esc(t('web.debug.menu',{p0:v.players[seat].name}))}">⏳</button>`;
  const reviewing=status.aiEnabled !== false && status.aiReview?.seat===seat;
  const inspect=!reviewing && status.host && (v.stage==='auction'?debugAuction():debugPlay()) && ["auction","play","trick"].includes(v.stage);
  const editable=status.host && (reviewing || inspect || (status.aiEnabled !== false && (v.stage==="lobby" || v.manualPaused)));
  const label=esc(reviewing ? t("web.ai.review-reopen",{p0:v.players[seat].name}) : inspect ? t(v.debugBotSeat===seat ? "web.debug.hide" : "web.debug.show",{p0:v.players[seat].name}) : `${t("web.ai.tab")}: ${v.players[seat].name}`);
  return `<button type="button" class="bot-model-icon" ${reviewing ? `data-panel="ai-review"` : inspect ? `data-debug-inspect="${seat}" aria-pressed="${v.debugBotSeat===seat}"` : `data-panel="bot-models"`} data-bot-seat="${seat}" aria-label="${label}" title="${label}" ${editable ? "" : "disabled"}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v3m-7 4H3v7h2m14-7h2v7h-2M7 6h10a2 2 0 0 1 2 2v11H5V8a2 2 0 0 1 2-2Z" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linejoin="round"/><circle cx="9" cy="11" r="1.3" fill="currentColor"/><circle cx="15" cy="11" r="1.3" fill="currentColor"/><path d="M9 15h6" stroke="currentColor" stroke-width="1.7"/></svg></button>`;
}
function render() {
  requestAnimationFrame(fitCardFans);
  cancelCardDrag();
  const v = status.view;
  syncSummary(v);
  if (!v || v.id !== startWaitingTable || v.stage !== 'lobby' || v.players.every(p=>p.ready)) startWaitingTable = '';
  const roomLabel = document.querySelector<HTMLElement>('#header-room')!;
  roomLabel.hidden = !v || !status.room;
  if (v && status.room) {
    const roomText = t('web.table.room', {p0: status.room.name});
    const hostText = t('web.table.host', {p0: v.players[0]?.name || status.room.table?.name || ''});
    const hostName = v.players[0]?.name || status.room.table?.name || '';
    roomLabel.innerHTML = `<span>${t('web.table.room', {p0: `<strong><em>${esc(status.room.name)}</em></strong>`})}</span><span>${t('web.table.host', {p0: `<strong><em>${esc(hostName)}</em></strong>`})}</span>`;
    roomLabel.title = `${roomText}\n${hostText}`;
  } else {
    roomLabel.textContent = '';
    roomLabel.title = '';
  }
  document.querySelector('#pause-game')?.remove();
  if (v && !['lobby','finished'].includes(v.stage) && (!v.manualPaused || v.pausedBy === v.seat)) {
    document.querySelector('.topbar nav')!.insertAdjacentHTML('afterbegin', `<button id="pause-game" class="quiet" data-action="${v.manualPaused ? 'resume-play' : 'pause'}" aria-pressed="${!!v.manualPaused}">${v.manualPaused ? t("web.main.text195") : t("web.main.text194")}</button>`);
  }
  if (!v) {
    document.body.classList.remove("at-table");
    renderStart();
    return;
  }
  document.body.classList.add("at-table");
  document.body.dataset.stage = v.stage;
  const mine = v.actor === v.seat;
  const actingSeat = !status.paused && !["lobby", "trick", "round", "finished"].includes(v.stage) ? (v.stage === "play" && v.claim == null ? v.turn : v.actor) : -1;
  const turnText = status.paused ? t("web.main.text194") : actingSeat < 0 ? stages[v.stage] : actingSeat === v.seat ? t("web.main.text193") : `${t("web.main.text192", {p0: v.players[actingSeat]?.name || ""})}`;
  const title = v.allPass
    ? passLabel(v)
    : contract(v.contract || v.bid);
  const other = v.players
    .map((p, i) => ({ p, i }))
    .filter((x) => x.i !== v.seat)
    .sort(
      (a, b) =>
        ((a.i - v.seat + v.players.length) % v.players.length) -
        ((b.i - v.seat + v.players.length) % v.players.length),
    );
  const hand = ordered((v.hand || []).filter(c => !(v.stage === "discard" && talonThrown(v) && v.talon?.includes(c))));
  const playedTalon = !v.allPass && ["play", "trick", "round", "finished"].includes(v.stage) && !!v.contract;
  const dealerCollected = Math.max(0, (v.taken?.[v.dealer] || 0) - (v.stage === "trick" && v.winner === v.dealer ? 1 : 0));
  const dealerPassHand = v.trickNo === 0
    ? `<span class="card-back" style="visibility:hidden" aria-hidden="true"></span><div class="card-back" aria-label="${t("web.main.text186")}"></div>`
    : dealerCollected > 0
      ? `<div class="collected-tricks" role="img" aria-label="${t("web.main.text189", {p0: dealerCollected})}" title="${t("web.main.text190", {p1: dealerCollected})}">${Array.from({length: Math.min(dealerCollected * 4, 6)}, (_, i) => `<div class="card-back" style="--stack-index:${i}" aria-hidden="true"></div>`).join("")}</div>`
      : "";
  const dealerHand = v.misereCards?.length ? misereTracker(v) : v.allPass && v.players.length === 4 ? (dealerPassHand ? `<div class="revealed-talon dealer-hand" aria-label="${t("web.main.text188")}">${dealerPassHand}</div>` : "") : v.allPass && v.players.length === 3 ? (v.trickNo < 2 ? `<div class="revealed-talon dealer-hand" aria-label="${t("web.main.text187")}">${[0,1].map(i=>i<v.trickNo ? '<span class="card-back" style="visibility:hidden" aria-hidden="true"></span>' : i<(v.talon?.length || 0) ? card(v.talon![i]) : `<div class="card-back" aria-label="${t("web.main.text186")}"></div>`).join("")}</div>` : "") : v.stage === "auction" || (v.talon?.length && ["discard", "defend", "option", "return", "dealer-choice", "dealer-whist", "mode"].includes(v.stage)) || playedTalon ? `<div class="revealed-talon dealer-hand" aria-label="${t("web.main.text187")}">${v.stage === "auction" || (playedTalon && !v.talonShown) ? `<div class="card-back" aria-label="${t("web.main.text186")}"></div><div class="card-back" aria-label="${t("web.main.text186")}"></div>` : (v.talon || []).map(c => card(c)).join("")}</div>` : "";
  const canSelect =
    mine && v.claim == null && !status.paused && ((v.stage === "play" && v.turn === v.seat) || (v.stage === "discard" && !talonThrown(v)));
  $("#app").innerHTML =
    `<section class="table-heading"><div><h1>${v.stage === "lobby" ? t("web.main.text163") : esc(title)}</h1><button data-panel="score">${tableProgress(v)}</button></div></section>
 <div class="playing-field ${v.players.length === 3 ? "three-player-table" : ""}"><section class="opponents">${other
   .map(
     ({ p, i }) =>
       `<article class="player ${(i-v.seat+v.players.length)%v.players.length === 1 ? 'player-left' : 'player-right'} ${v.players.length === 4 && (i - v.seat + 4) % 4 === 2 ? "player-top" : ""} ${actingSeat === i ? "active" : ""} ${!p.bot && !status.connected[i] ? "connection-lost" : ""}"><div class="player-info"><div class="speech">${esc(role(v, i))}</div>${!p.bot && !status.connected[i] ? `<small class="reconnecting-label">${t("web.main.text164")}<span class="connection-detail">${esc(connectionDetail(i))}</span></small>` : ""}${actingSeat === i ? `<span class="turn-badge">${t("web.main.text165")}</span>` : ""}${p.bot ? "" : `<div class="avatar">${esc(p.name.slice(0, 1))}</div>`}<div><strong>${p.bot ? botModelIcon(v,i) : ""}${esc(p.name)}</strong>${p.bot ? "" : tableVoiceIcon(i, false)}<div class="seat-marks">${v.stage !== "lobby" && v.round > 0 ? `<span data-taken title="${t("web.main.text166")}">${v.taken?.[i] || 0}</span>${i === (v.dealer + 1) % v.players.length ? `<span class="first-hand">${t("web.main.text162")}</span>` : ""}` : ""}</div></div></div><div class="player-cards">${(v.players.length === 4 && i === v.dealer ? dealerHand : "") + (p.cards?.length
           ? `<div class="exposed">${groupedCards(p.cards, c => card(c, mine && v.claim == null && !status.paused && v.stage === "play" && v.turn === i, mine && v.stage === "play" && v.turn === i && !(v.legal || []).includes(c)))}</div>`
           : p.count
             ? v.players.length === 4 && (i - v.seat + 4) % 4 === 2
               ? `<div class="closed-hand" aria-label="${t("web.main.text167", {p0: p.count})}">${Array.from({length: p.count}, () => `<div class="card-back"></div>`).join("")}</div>`
               : `<div class="card-back" aria-label="${t("web.main.text167", {p0: p.count})}"><span>${p.count}</span></div>`
             : "")}</div><div class="top-card-stats">${v.stage !== "lobby" && v.round > 0 ? `<span data-taken title="${t("web.main.text166")}">${v.taken?.[i] || 0}</span>` : ""}</div></article>`,
   )
   .join("")}</section>

 ${v.stage === "lobby"
     ? `<section class="center lobby"><div class="table-symbol">♠</div><h2>${t("web.main.text173", {p0: v.players.length})}</h2><p>${rulesSummary(v.rules || defaultRules, v.target)}</p><button data-panel="settings" class="quiet">${t("web.main.text174")}</button>${appearance.debug && status.host && !v.players[0].ready ? `<button data-panel="debug-bots">${t("web.main.text172", {p0: debugBotsLabel(v.debugBots)})}</button>` : appearance.debug && v.debugBots ? `<p>${t("web.main.text172", {p0: debugBotsLabel(v.debugBots)})}</p>` : ""}</section>`
     : `<section class="center"><div class="table-announcement"><div class="turn-message">${esc(title)}<br>${esc(turnText)}</div>${v.players.length === 3 ? dealerHand : ""}</div>${poolDrawing(v, esc)}<div class="trick-label">${v.stage === "trick" ? `${t("web.main.text170", {p0: esc(v.players[v.winner]?.name)})}` : v.stage === "round" || v.stage === "finished" ? t("web.main.text169") : `${t("web.main.text168", {p0: Math.min(v.trickNo + 1, 10)})}`}</div><div class="trick-cards">${(["play", "trick"].includes(v.stage) ? v.trick || [] : []).filter(p => p.seat >= 0).map((p, n) => {
      const rel = p.seat < 0 ? -1 : (p.seat - v.seat + v.players.length) % v.players.length;
      const pos = rel === 0 ? "self" : rel === 1 ? "left" : v.players.length === 4 && rel === 2 ? "top" : "right";
      return `<div class="trick-seat-${pos}" style="--trick-order:${n}">${card(p.card)}<small>${p.seat < 0 ? t("web.main.text171") : esc(v.players[p.seat].name)}</small></div>`;
    }).join("") || ""}</div>${v.stage === "round" || v.stage === "finished" ? roundSummary(v) : ""}</section>`}
 </div>${v.stage === "discard" && mine && !talonThrown(v) ? `<section class="discard-tray" aria-label="${t("web.main.text175")}"><span>${t("web.main.text176", {p0: selected.length})}</span>${selected.map(c => card(c, true)).join("")}</section>` : ""}<section class="hand-area ${actingSeat === v.seat ? "active-hand" : ""}"><div class="hand-label"><strong>${esc(v.players[v.seat].name)} ${v.players[v.seat].bot ? "" : tableVoiceIcon(v.seat, true)} <span class="own-taken" data-taken title="${t("web.main.text166")}">${v.taken?.[v.seat] || 0}</span> <small>${t("web.main.text185", {p10: esc(role(v, v.seat)), p11: v.dealer === v.seat ? t("web.main.text177") : ""})}</small></strong><span>${v.stage === "finished" ? t("web.main.text184") : v.stage === "round" ? t("web.main.text183") : mine && v.claim == null && v.stage === "play" && v.turn !== v.seat ? `${t("web.main.text182", {p0: esc(v.players[v.turn].name)})}` : mine ? t("web.main.text181") : v.stage === "lobby" ? t("web.main.text180") : `${t("web.main.text179", {p0: esc(v.players[v.actor]?.name || t("web.main.text178"))})}`}</span></div>${v.players.length === 4 && v.seat === v.dealer ? dealerHand : ""}<div class="hand" style="--count:${Math.max(hand.length, 1)}">${groupedCards(hand.filter(c => v.stage !== "discard" || talonThrown(v) || !selected.includes(c)), c => card(c, canSelect , canSelect && v.stage === "play" && !(v.legal || []).includes(c)))}</div><div class="action-area">${actionControls(v)}</div></section>`;
  document.querySelector('.playing-field')?.insertAdjacentHTML('beforeend', lastTrick(v));
  if(status.host && v.debugWaitingSeat!=null) {
    document.querySelector<HTMLElement>(`.bot-model-icon[data-bot-seat="${v.debugWaitingSeat}"]`)?.closest<HTMLElement>('.player')?.setAttribute('data-debug-waiting',String(v.debugWaitingSeat));
  }
  if (v.players.length === 3 && v.stage !== 'lobby' && v.round > 0 && v.seat === (v.dealer + 1) % v.players.length) {
    document.querySelector('.hand-label > strong')!.insertAdjacentHTML('beforeend', `<span class="first-hand own-first-hand">${t("web.main.text162")}</span>`);
  }
}
function debugBotsLabel(mode?: string) { return mode === "pass" ? t("web.main.text099") : mode === "misere" ? t("web.main.text098") : t("web.main.text100"); }
function actionControls(v: View) {
  if(status.host && v.debugWaitingSeat!=null) return `<span class="drag-hint">${t('web.debug.wait',{p0:esc(v.players[v.debugWaitingSeat].name)})}</span>`;
  const fillBots = status.host && v.stage === "lobby" && status.freeSeats > 0
    ? `<button type="button" data-action="fill-bots" class="fill-bots-icon" aria-label="${t("web.main.text161")}" title="${t("web.main.text161")}"><svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><rect x="4" y="7" width="16" height="14" rx="3"/><path d="M12 3v4M2 12v5M22 12v5M9 17h6"/><circle cx="8" cy="12" r="1"/><circle cx="16" cy="12" r="1"/></svg></button>` : '';
  const acts = [...(v.actions || [])].filter(a => a !== "play" && a !== "collect" && a !== "next");
  if(acts.includes('bid') && acts.includes('pass')){acts.splice(acts.indexOf('bid'),1);acts.unshift('bid');}
  if (v.claim != null && !status.paused) return `<span class="drag-hint claim-text">${t("web.main.text160", {p0: esc(v.players[v.declarer].name), p1: trickCount(v.claim), p2: (v.taken[v.declarer] || 0)+v.claim})}</span>${acts.includes("accept-claim") ? `<button data-action="accept-claim" class="primary">${t("web.main.text158")}</button><button data-action="reject-claim">${t("web.main.text159")}</button>` : `<span class="muted">${t("web.main.text157")}</span>`}`;
  if (v.stage === "trick") return `<span class="muted">${t("web.main.text156")}</span>`;
  if ((v.actions || []).includes("play") && !status.paused) return `<span class="drag-hint">${t("web.main.text155")}</span>${acts.includes("claim") ? `<button data-action="claim">${t("web.main.text154")}</button>` : ""}`;
  if (status.paused) return fillBots + `<p class="muted">${v.manualPaused ? `${t(v.pausedBy === v.seat ? "web.main.text153" : "web.pause.wait_owner", {p0: esc(v.players[v.pausedBy || 0]?.name)})}` : t("web.main.text152")}${status.roomMode && status.room && !status.roomOnline ? ` <span>${t("web.connection.server")}</span>` : ""}</p>`;
  if (v.stage === "dealer-choice" && acts.includes("dealer-skip")) {
    const defenders = [1, 2].map(offset => {
      let seat = v.declarer;
      for (let i = 0; i < offset; i++) { do { seat = (seat + 1) % 4; } while (seat === v.dealer); }
      return seat;
    });
    return `<span class="drag-hint">${t("web.dealer_whist.choose")}</span><button data-action="dealer-skip">${t("web.dealer_whist.no")}</button>${defenders.map((seat,i)=>`<button data-action="dealer-${i===0?'first':'second'}">${t(i===0?'web.dealer_whist.first':'web.dealer_whist.second')} · ${esc(v.players[seat].name)}</button>`).join('')}`;
  }
  if (v.stage === "dealer-whist" && acts.includes("whist")) return `<span class="drag-hint">${t("web.dealer_whist.inspect", {p0: esc(v.players[v.dealerLook!].name)})}</span><button data-action="whist" class="primary">${actions.whist}</button><button data-action="pass">${actions.pass}</button>`;
  if (acts.includes("declare")) {
    return `<span class="drag-hint">${talonThrown(v) ? t("web.main.text146") : t("web.main.text145")}</span><button data-action="declare" class="primary" ${!talonThrown(v) && selected.length !== 2 ? "disabled" : ""}>${t("web.main.text144")}</button>${acts.includes("without-three") ? `<button data-action="without-three" title="${t("web.main.text147")}">${t("web.main.text148")}</button>` : ""}${acts.includes("show-talon") ? `<button data-action="show-talon" aria-pressed="${!!v.talonShown}" title="${t("web.main.text151")}">${v.players.length === 4 ? t("web.main.text150") : t("web.main.text149")}</button>` : ""}`;
  }
  const pulseReady = startWaitingTable===v.id && status.host && !v.players[v.seat].ready && v.players.every((p,i)=>i===v.seat || p.ready);
  return `${acts.includes("ready") ? `<label class="contract-picker">${t("web.connection_setup.text008")}<input id="player-name" maxlength="24" value="${esc(v.players[v.seat].name)}"></label>` : ""}${fillBots}${acts.map(a => {
    if (a==='start' && startWaitingTable===v.id) return `<button data-action="start" class="primary" disabled aria-busy="true" title="${esc(t('web.start.not_ready',{p0:v.players.filter(p=>!p.ready).map(p=>p.name).join(', ')}))}"><span class="connection-spinner" aria-hidden="true"></span>${t('web.start.waiting')}</button>`;
    return `<button data-action="${a}" class="${["play","start","next","ready","bid"].includes(a) ? "primary" : ""}${a==='ready' && pulseReady ? ' ready-attention' : ''}" ${a === "play" && selected.length !== 1 ? "disabled" : ""}>${a === "bid" ? t("web.main.text144") : a === "ready" && v.players[v.seat].ready ? t("web.main.text143") : a === "half" ? halfLabel(v) : actions[a]}</button>`;
  }).join("")}`;
}
function roundSummary(v: View) {
  if (summaryHidden) return "";
  const l = v.history?.at(-1);
  return `<div class="round-summary" data-dismiss-summary tabindex="0" aria-label="${t("web.main.text140")}"><button class="summary-close" data-close-summary aria-label="${t("web.main.text141")}">×</button><h2>${v.stage === "finished" ? t("web.main.text137") : t("web.main.text136")}</h2>${v.players.map((p, i) => `<div><span>${esc(p.name)}</span><strong>${v.stage === "finished" ? `${t("web.main.text139", {p0: wholeWhists(v.results)[i].toLocaleString(locale)})}` : `${t("web.main.text138", {p0: l?.pool[i] || 0, p1: l?.mountain[i] || 0})}`}</strong></div>`).join("")}${l?.amnesty ? `<p class="muted">${amnestyText(l)}</p>` : ""}<button data-panel="score">${t("web.main.text142")}</button></div>`;
}
function updateCreateLimit(r = appearance.tableRules || defaultRules, target = appearance.poolTarget || 30) {
  const form = document.querySelector<HTMLFormElement>('#create-form');
  if (!form) return;
  const timed = r.endCondition === 'time';
  form.querySelector<HTMLElement>('[data-pool-limit]')!.hidden = timed;
  form.querySelector<HTMLElement>('[data-time-limit]')!.hidden = !timed;
  form.querySelector<HTMLInputElement>('[name="target"]')!.value = String(target);
  form.querySelector<HTMLInputElement>('[name="minutes"]')!.value = String(r.minutes);
}
  function renderStart() {
  if(status.roomMode && status.connectionSetup && !status.room){
    if(!document.querySelector('#connection-setup'))$('#app').innerHTML=setupScreen(appearance.name || t("web.main.text001"), appearance.roomServiceURL || interfaceConfig.roomServiceURL);
    if(!document.querySelector('#connection-setup [data-room-action="local"]'))$('#connection-setup').insertAdjacentHTML('beforeend',`<button type="button" data-room-action="local">${t('web.local.play')}</button>`);
    return;
  }
  if(status.roomMode&&!status.room){roomUI.picker($("#app"));return;}
  if ($("#create-form")) return;
    const savedName = appearance.name || t("web.main.text001");
    $("#app").innerHTML =
    `<section class="welcome"><p class="eyebrow">${t("web.main.text123")}</p><h1>${t("web.main.text124")}</h1><p>${t("web.main.text125")}<br>${t("web.main.text126")}</p></section><section class="start-grid"><form id="create-form" class="surface"><h2>${t("web.main.text127")}</h2><label>${t("web.connection_setup.text008")}<input name="name" maxlength="24" value="${t("web.main.text001")}" required></label><div class="form-row"><label>${t("web.main.text128")}<select name="players"><option>3</option><option>4</option></select></label><label>${t("web.main.text129")}<input name="target" type="number" min="1" max="1000" value="${appearance.poolTarget || 30}" required></label></div><label class="check"><input name="bots" type="checkbox"> ${t("web.main.text130")}</label><div class="form-row start-actions"><button class="primary" type="submit">${t("web.main.text131")}</button><button class="secondary" type="button" data-auto-join>${t("web.main.text132")}</button></div></form><div class="surface"><p>${t("web.main.text133")}</p><div id="saves"><p class="muted">${t("web.main.text134")}</p></div><button type="button" id="party-history">${t("web.main.text024")}</button><button id="finish-party" class="danger" type="button">${t("web.main.text135")}</button></div></section>`;
    const targetLabel = document.querySelector<HTMLInputElement>('#create-form [name="target"]')!.parentElement!;
    targetLabel.dataset.poolLimit = '';
    targetLabel.insertAdjacentHTML('afterend', `<label data-time-limit hidden>${t("web.main.text007")}<input name="minutes" type="number" min="1" max="1440" value="60" required></label>`);
    updateCreateLimit();
    document.querySelector<HTMLSelectElement>('#create-form [name="players"]')!.value = String(appearance.playerCount || 3);
    const nameInput = document.querySelector<HTMLInputElement>('#create-form input[name="name"]');
    if(status.roomMode){
      document.querySelector('.welcome')!.insertAdjacentHTML('beforebegin','<section id="room-roster"></section>');
      document.querySelector('[data-auto-join]')?.remove();
      const bots=document.querySelector<HTMLInputElement>('[name="bots"]')!;bots.checked=true;
      bots.parentElement!.lastChild!.textContent=t("web.main.text122");
      if(status.roomLocal) {
        bots.disabled=true;
        document.querySelector('.surface > p')!.textContent=t('web.local.hint');
      }
      const players=document.querySelector<HTMLSelectElement>('[name="players"]')!;players.value=String(appearance.playerCount || (status.room!.members.filter(m=>m.online).length===4?4:3));
      roomUI.paint();
    }
    if (nameInput) nameInput.value = savedName;
    void loadSaves();
}
async function loadSaves() {
  try {
    const saves = await rpc<SaveInfo[]>("list");
    for (const save of saves) save.names = save.names || [];
    if (!$("#saves")) return;
    $("#saves").innerHTML =
      `<h3>${t("web.main.text121", {p0: saves.length})}</h3>${saves.length ? `<select id="saved-party" aria-label="${t("web.main.text118")}" style="width:100%;min-width:0;max-width:100%">${saves.map((s) => `<option value="${esc(s.id)}">${t("web.main.text117", {p1: esc(s.names.join(" · ") || t("web.main.text114")), p2: s.host ? t("web.main.text116") : t("web.main.text115"), p3: s.round, p4: esc(stages[s.stage] || t("web.main.text076")), p5: esc(s.id.slice(0, 8))})}</option>`).join("")}</select><div class="toolbar"><button id="resume-save" class="primary">${t("web.main.text119")}</button><button id="delete-save">${t("web.main.text120")}</button></div>` : `<p class="muted">${t("web.main.text113")}</p>`}`;
  } catch (e) {
    notice(String(e));
  }
}
const rulesHTML = `<p>${t("web.main.text108")}</p><p>${t("web.main.text109")}</p><p>${t("web.main.text110")}</p><p>${t("web.dealer_whist.rule")}</p><p><strong>${t("web.main.text111")}</strong> ${t("web.main.text112")}</p>`;
function previewDeck() {
  const form = document.querySelector<HTMLFormElement>("#settings-form");
  if (!form) return;
  const d = new FormData(form);
  const deck = String(d.get("deck"));
  const frame = d.has(`frame-${deck}`);
  for (const [id] of decks) {
    $(`[data-deck-thumb="${id}"]`).innerHTML = face(6, id, d.has(`frame-${id}`));
  }
  const preview = $("#deck-preview");
  preview.dataset.cardSize = "normal";
  preview.classList.toggle(
    "preview-large",
    d.has("large-indices") || deck === "english",
  );
  preview.innerHTML = [6, 29, 12, 18]
    .map((c) => {
      const suit = Math.floor(c / 8);
      return `<div class="card ${suit >= 2 ? "red" : ""}">${face(c, deck, frame)}</div>`;
    })
    .join("");
}
document.addEventListener("click", e => {
  const tab = (e.target as HTMLElement).closest<HTMLElement>("[data-settings-tab]");
  if (!tab) return;
  document.querySelectorAll<HTMLElement>("[data-settings-tab]").forEach(t => {
    const active = t === tab;
    t.setAttribute("aria-selected", String(active));
    document.getElementById(t.getAttribute("aria-controls")!)!.hidden = !active;
  });
});
const aiUI=new AIBotsUI(rpc,()=>void refresh());
const aiReviewUI=new AIReviewUI(rpc,()=>void refresh());
const debugBotUI=new DebugBotUI(rpc,c=>card(c).replace('<button','<div').replace('</button>','</div>'),option=>{
  if(option.action==='claim')return t('web.debug.claim',{p0:option.tricks ?? 0});
  if(option.action==='half' && status.view)return halfLabel(status.view);
  const label=option.contract?contract(option.contract):({'without-three':t('web.main.text148'),'show-talon':t('web.main.text150'),'accept-claim':t('web.main.text158'),'reject-claim':t('web.main.text159')} as Record<string,string>)[option.action] || actions[option.action] || option.label;
  return option.cards?.length?`${option.cards.map(c=>ranks[c%8]+suits[c>>3]).join(' + ')} · ${label}`:label;
},()=>{$<HTMLDialogElement>('#panel').close();void refresh();},(options,view)=>{
  const grid=[6,7,8,9,10].flatMap(level=>[0,1,2,3,4].map(suit=>{
    const index=options.findIndex(o=>o.contract && !o.contract.noTalon && !o.contract.misere && o.contract.level===level && o.contract.suit===suit);
    return `<button type="button" class="contract-cell ${suit===2 || suit===3?'red':''}" data-debug-option="${index}" ${index<0?'disabled':''} aria-label="${level} ${[t('web.main.text211'),t('web.main.text210'),t('web.main.text209'),t('web.main.text208'),t('web.main.text207')][suit]}">${level}<span>${suits[suit]}</span></button>`;
  })).join('');
  const auction=view.stage==='auction';
  const misere=options.findIndex(o=>o.contract?.misere && !o.contract.noTalon);
  const noTalon=options.map((o,i)=>o.contract?.noTalon?`<button type="button" data-debug-option="${i}">${contract(o.contract)}</button>`:'').join('');
  const here=auction && view.bid?options.findIndex(o=>o.contract && o.contract.level===view.bid!.level && o.contract.suit===view.bid!.suit && !!o.contract.misere===!!view.bid!.misere && !!o.contract.noTalon===!!view.bid!.noTalon):-1;
  const pass=auction?options.findIndex(o=>o.action==='pass'):-1;
  return `<p>${t(auction?'web.main.text204':'web.debug.contract')}</p><div class="contract-grid">${grid}</div>${noTalon?`<div class="contract-no-talon">${noTalon}</div>`:''}<div class="contract-footer">${here>=0?`<button type="button" data-debug-option="${here}">${t('web.main.text205')}</button>`:''}${misere>=0?`<button type="button" data-debug-option="${misere}">${t('web.main.text098')}</button>`:''}${pass>=0?`<button type="button" data-debug-option="${pass}">${t('web.main.text206')}</button>`:''}</div>`;
});
installDebugBotMenu(seat=>{
  if(status.host && status.view?.debugWaitingSeat===seat && !$<HTMLDialogElement>('#panel').open)showPanel('debug-turn',undefined,seat);
});
let shownAIReview="";
function showPanel(kind: string, historyView?: View, botSeat?: number) {
 if(status.aiEnabled === false && ["ai-review","bot-models"].includes(kind)) return;
  status.stun = status.stun || [];
  currentPanel = kind;
  const panel = $<HTMLDialogElement>("#panel");
  panel.dataset.kind = kind;
  $("#panel-title").textContent =
    (
      {
        rules: t("web.main.text107"),
        settings: t("web.main.text106"),
        network: t("web.main.text105"),
        score: t("web.main.text104"),
      } as Record<string, string>
    )[kind] || kind;
  const v = historyView || status.view;
  if(kind==='debug-turn' && v && status.host && botSeat===v.debugWaitingSeat) {
    $('#panel-title').textContent=t('web.debug.menu',{p0:v.players[botSeat!].name});
    $('#panel-body').innerHTML='<section id="debug-turn"></section>';
    void debugBotUI.mount($('#debug-turn'),{seat:botSeat!,revision:v.revision,name:v.players[botSeat!].name});
  }
  if(kind==="ai-review" && status.aiReview){
    $("#panel-title").textContent=t("web.ai.review-title");
    $("#panel-body").innerHTML='<section id="ai-review"></section>';
    aiReviewUI.mount($("#ai-review"),status.aiReview);
  }
  if (kind === "debug-bots" && appearance.debug && v && status.host && v.stage === "lobby" && !v.players[0].ready) {
    $("#panel-title").textContent = t("web.main.text103");
    $("#panel-body").innerHTML = `<p>${t("web.main.text101")}</p><div class="toolbar">${[["",t("web.main.text100")],["pass",t("web.main.text099")],["misere",t("web.main.text098")]].map(([mode,label])=>`<button data-debug-bots="${mode}" aria-pressed="${(v.debugBots || "")===mode}">${label}</button>`).join("")}</div><p>${t("web.main.text102")}</p>`;
  }
  if (kind === "bot-models" && v && status.host && (v.stage === "lobby" || v.manualPaused)) {
    $("#panel-title").textContent=t("web.ai.tab");
    $("#panel-body").innerHTML=`<section id="bot-models"></section>`;
    void aiUI.assignments($("#bot-models"),v.players,botSeat);
  }
  if (kind === "rules") $("#panel-body").innerHTML = rulesForm(v ? v.rules || defaultRules : appearance.tableRules || defaultRules, v?.target || Number(document.querySelector<HTMLInputElement>('#create-form [name="target"]')?.value || appearance.poolTarget || 30), false, true) + rulesHTML;
  if (kind === "settings")
    $("#panel-body").innerHTML =
      `<div class="settings-tabs" role="tablist" aria-label="${t("web.main.text080")}">${[["game",t("web.main.text078")],["cards",t("web.main.text077")],["connection",t("web.main.text076")],["ai",t("web.ai.tab")]].map(([id,label]) => `<button type="button" role="tab" id="settings-tab-${id}" data-settings-tab="${id}" aria-controls="settings-${id}" aria-selected="${id === "game"}">${label}</button>`).join("")}</div><form id="settings-form"><section id="settings-game" role="tabpanel" aria-labelledby="settings-tab-game"></section><section id="settings-cards" role="tabpanel" aria-labelledby="settings-tab-cards" hidden><h3>${t("web.main.text077")}</h3><div class="deck-options">${decks.map(([id, name]) => `<div class="deck-choice"><label class="deck-option"><input type="radio" name="deck" value="${id}" ${appearance.deck === id ? "checked" : ""}><span class="deck-thumb card" data-deck-thumb="${id}">${face(6, id)}</span><strong>${name}</strong></label><label class="check"><input type="checkbox" name="frame-${id}" ${appearance.cardFrames?.[id] !== false ? "checked" : ""}>${t("web.main.text079")}</label></div>`).join("")}</div><div id="deck-preview" class="deck-preview" aria-label="${t("web.main.text081")}"></div><h3>${t("web.main.text082")}</h3><div class="back-options">${backs.map(([id,name]) => `<label class="deck-option"><input type="radio" name="back" value="${id}" ${(appearance.back || "plaid") === id ? "checked" : ""}><span class="card-back back-preview" data-back="${id}"></span><strong>${name}</strong></label>`).join("")}</div></section><section id="settings-connection" role="tabpanel" aria-labelledby="settings-tab-connection" hidden><label>${t("web.main.text083")}<textarea name="stun" rows="4" placeholder="${t("web.main.text084")}">${esc(status.stun.filter(u => u.startsWith("turn:") || u.startsWith("turns:")).join("\n"))}</textarea></label><p>${t("web.main.text085")}</p><details><summary>${t("web.main.text086")}</summary><textarea name="stun-fallback" rows="3">${esc(status.stun.filter(u => u.startsWith("stun:")).join("\n"))}</textarea><p>${t("web.main.text087")}</p></details><label class="check"><input type="checkbox" name="log-enabled" ${!appearance.logDisabled ? "checked" : ""}>${t("web.main.text088")}</label><p class="muted">${t("web.main.text089")} <code>-c</code> ${t("web.main.text090")} <code>--config</code>.</p><h3>${t("web.main.text091")}</h3><label>${t("web.main.text092")}<input name="update-url" type="url" value="${esc(appearance.updateURL || "")}" placeholder="${esc(networkConfig.updateAddressHint)}"></label><p class="muted">${t("web.main.text093")}</p><button type="button" id="check-update">${t("web.main.text094")}</button><div id="update-result" class="muted" role="status"></div></section><section id="settings-ai" role="tabpanel" aria-labelledby="settings-tab-ai" hidden></section><button class="primary">${t("web.main.text095")}</button></form><hr><button id="leave">${t("web.main.text096")}</button><p class="muted">${t("web.main.text097")}</p>`;
  if (kind === "settings") {
    setupUpdateAddressHint();
    if(status.aiEnabled !== false) void aiUI.mount($("#settings-ai"), appearance.updateURL || "",!!appearance.debug);
    else { document.querySelector("#settings-tab-ai")?.remove(); document.querySelector("#settings-ai")?.remove(); }
    enhanceConnectionSettings(status.stun);
    document.querySelector('#settings-connection')!.insertAdjacentHTML('afterbegin', `<label>${t("web.connection.room_service")}<input name="room-service-url" type="url" value="${esc(appearance.roomServiceURL || interfaceConfig.roomServiceURL)}" placeholder="https://signaling.example.org/v2/rooms"></label>`);
    const logLabel = document.querySelector<HTMLInputElement>('[name="log-enabled"]')!.closest('label')!;
    const logHelp = logLabel.nextElementSibling!;
    if (window.PreferansAndroid) {
      logHelp.textContent = t("web.main.text075");
      logHelp.insertAdjacentHTML('afterend',`<div class="toolbar"><span id="log-file-size" role="status"></span><button type="button" id="clear-log-file">${t("web.log.clear")}</button>${window.PreferansAndroid.saveFullLogToDownloads ? `<button type="button" id="save-full-log">${t('web.log.downloads')}</button>` : ''}</div>`);
      updateLogFileSize(status);
    }
    else {
      logLabel.insertAdjacentHTML('afterend', `<label>${t("web.main.text074")}<input name="log-path" type="text" value="${esc(appearance.logPath || '')}" placeholder="C:\\Logs\\preferans.log" spellcheck="false"></label>`);
      logHelp.textContent = status.logPathOverride
        ? `${t("web.main.text073", {p0: status.logPath})}`
        : t("web.main.text072");
    }
    if(status.roomMode && !status.roomLocal){$('#settings-connection').insertAdjacentHTML('afterbegin',`<label>${t("web.main.text070")}<select name="default-room"><option value="">${t("web.main.text071")}</option></select></label>`);void roomUI.settings(appearance.defaultRoom || '');}
    $("#settings-game").insertAdjacentHTML("afterbegin", `<section class="conventions"><div class="variant-tabs" role="tablist" aria-label="${t("web.main.text068")}"><button id="tab-leningrad" role="tab" aria-selected="true" aria-controls="leningrad-settings">${t("web.main.text069")}</button>${[t("web.main.text067"),t("web.main.text066"),t("web.main.text065")].map(name => `<button role="tab" aria-selected="false" disabled title="${t("web.main.text063")}">${name}<small>${t("web.main.text064")}</small></button>`).join("")}</div><div id="leningrad-settings" role="tabpanel" aria-labelledby="tab-leningrad">${rulesForm(v ? v.rules || defaultRules : appearance.tableRules || defaultRules, v?.target || Number(document.querySelector<HTMLInputElement>('#create-form [name="target"]')?.value || appearance.poolTarget || 30), !v || (status.host && v.stage==='lobby'))}</div></section>`);
    $("#leave").setAttribute("type", "button");
    $("#settings-game").append($("#leave"), $("#leave").nextElementSibling!);
    $("#tab-leningrad").setAttribute("type", "button");
    $("#settings-game").insertAdjacentHTML("afterbegin",`<div class="debug-settings"><strong>${t("web.debug.setting")}:</strong><label class="check"><input type="checkbox" name="debug-auction" ${debugAuction() ? "checked" : ""}>${t("web.debug.auction")}</label><label class="check"><input type="checkbox" name="debug-play" ${debugPlay() ? "checked" : ""}>${t("web.debug.play")}</label></div>`);
    $("#settings-game").insertAdjacentHTML("afterbegin", `<label class="check"><input type="checkbox" name="pass-price-hint" ${appearance.passPriceHint !== false ? "checked" : ""}>${t("web.pass_price.setting")}</label>`);
    $('#settings-game').insertAdjacentHTML('afterbegin',`<label class="check"><input type="checkbox" name="local-only" ${appearance.localOnly?'checked':''}>${t('web.local.setting')}</label>`);
    previewDeck();
  }
  if (kind === "network")
    $("#panel-body").innerHTML =
      `<p>${autoJoining ? t("web.main.text055") : t("web.main.text054")}</p>${status.host && v ? `<div class="connection-list">${v.players.map((p, i) => (i === 0 || p.bot ? "" : `<div><strong>${esc(p.name)}</strong><span>${status.connected?.[i] ? t("web.main.text058") : esc(status.links[i] || t("web.main.text057"))}</span></div>`)).join("")}</div>` : `<p class="connection-waiting">${t("web.main.text056")}</p>`}<details open class="connection-log"><summary>${t("web.main.text059")}</summary><button type="button" id="copy-connection-log">${t("web.main.text060")}</button><button type="button" id="save-connection-log">${t("web.main.text061")}</button><pre id="connection-log-text"></pre></details><p class="muted">${t("web.main.text062")}</p>`;
  if (kind === "network") renderConnectionLog();
  if (kind === "score") {
    $("#panel-body").innerHTML = v
      ? `<div class="score-scroll"><table><thead><tr><th>${t("web.main.text001")}</th><th>${t("web.main.text006")}</th><th>${t("web.main.text049")}</th><th>${t("web.main.text050")}</th></tr></thead><tbody>${v.players.map((p, i) => `<tr><td>${esc(p.name)}</td><td>${v.pool[i]}</td><td>${v.mountain[i]}</td><td>${wholeWhists(v.results)[i].toLocaleString(locale)}</td></tr>`).join("")}</tbody></table><h3>${t("web.main.text051")}</h3><table><thead><tr><th></th>${v.players.map((p) => `<th>${esc(p.name)}</th>`).join("")}</tr></thead><tbody>${v.players.map((p, i) => `<tr><th>${esc(p.name)}</th>${v.whists[i].map((w, j) => `<td>${i === j ? "—" : w}</td>`).join("")}</tr>`).join("")}</tbody></table></div><p class="muted">${t("web.main.text052", {p3: v.stage === "finished" ? t("web.main.text047") : t("web.main.text046")})}</p><h3>${t("web.main.text053")}</h3>${[
          ...(v.history || []),
        ]
          .reverse()
          .map(
            (l) =>
              `<details><summary>№ ${l.round} · ${esc(l.label)}</summary>${l.amnesty ? `<p>${amnestyText(l)}</p>` : ""}${v.players.map((p, i) => `<p>${t("web.main.text048", {p0: esc(p.name), p1: l.pool[i], p2: l.mountain[i], p3: l.whists[i].join(" / ")})}</p>`).join("")}</details>`,
          )
          .join("")}`
      : `<p>${t("web.main.text045")}</p>`;
  }
  updateExitButtons();
  if (kind === 'score' && v?.stage === 'finished') $('#panel-body').insertAdjacentHTML('afterbegin', partyDates(v));
  if (!panel.open) panel.showModal();
  $('#panel-body').scrollTop = 0;
  panel.append($('#notice'));
}
function renderConnectionLog() {
  const el = document.querySelector<HTMLElement>("#connection-log-text");
  if (!el) return;
  el.textContent = [...(status.logs || []), ...signalingTrace].join("\n") || t("web.main.text044");
  el.scrollTop = el.scrollHeight;
}
function setupUpdateAddressHint() {
  const input = document.querySelector<HTMLInputElement>('[name="update-url"]');
  if (!input) return;
  input.title = t('web.update.address_hint');
  input.insertAdjacentHTML('afterend', `<small class="muted">${t('web.update.address_hint')}</small>`);
  const fill = () => {
    if (input.value.trim() || !input.placeholder) return;
    input.value = input.placeholder;
    input.dispatchEvent(new Event('input', {bubbles: true}));
    input.dispatchEvent(new Event('change', {bubbles: true}));
  };
  input.addEventListener('dblclick', fill);
  let down: {x: number; y: number; time: number} | null = null;
  let previousTap = 0;
  input.addEventListener('pointerdown', event => {
    if (event.pointerType !== 'touch') return;
    down = {x: event.clientX, y: event.clientY, time: performance.now()};
  });
  input.addEventListener('pointercancel', () => {down = null; previousTap = 0;});
  input.addEventListener('pointerup', event => {
    if (event.pointerType !== 'touch' || !down) return;
    const now = performance.now();
    const tap = now - down.time <= interfaceConfig.updateAddressDoubleTapMs &&
      Math.hypot(event.clientX - down.x, event.clientY - down.y) <= interfaceConfig.buttonCancelDistancePx;
    down = null;
    if (!tap) {previousTap = 0; return;}
    if (previousTap && now - previousTap <= interfaceConfig.updateAddressDoubleTapMs) {
      fill();
      previousTap = 0;
    } else previousTap = now;
  });
}
async function checkForUpdate(silent = false) {
  const input = document.querySelector<HTMLInputElement>('[name="update-url"]');
  const result = document.querySelector<HTMLElement>("#update-result");
  const rawBase = input?.value.trim().replace(/\/+$/, "") || "";
  if (!rawBase) {
    if (silent) return;
    throw Error(t("web.main.text043"));
  }
  let base: URL;
  try {
    base = new URL(rawBase + "/");
    if (!["http:", "https:"].includes(base.protocol)) throw Error();
  } catch {
    if (silent) return;
    throw Error(t("web.main.text042"));
  }
  if (result) result.textContent = t("web.main.text041");
  let manifestText: string;
  try {
    if (window.PreferansAndroid?.checkUpdate) {
      manifestText = await requestNativeUpdateManifest(base);
    } else {
      const response = await fetch(new URL(`version.json?t=${Date.now()}`, base), { cache: "no-store" });
      if (!response.ok) throw Error(`${t("web.main.text040", {p0: response.status})}`);
      manifestText = await response.text();
    }
  } catch (e) {
    if (silent) return;
    throw e;
  }
  let info: UpdateManifest;
  try {
    info = JSON.parse(manifestText) as UpdateManifest;
  } catch {
    if (silent) return;
    throw Error(t("web.main.text039"));
  }
  const code = Number(info.versionCode || 0);
  const name = String(info.versionName || code || t("web.main.text038"));
  const installed = window.PreferansAndroid?.getVersionCode?.() || 0;
  if (!code || !info.apk) {
    if (silent) return;
    throw Error(t("web.main.text037"));
  }
  try {
    pendingUpdateURL = new URL(info.apk, base).toString();
  } catch {
    if (silent) return;
    throw Error(t("web.main.text036"));
  }
  if (code <= installed) {
    if (result) result.textContent = `${t("web.main.text035", {p0: window.PreferansAndroid?.getVersionName?.() || installed})}`;
    notice(t("web.main.text034"));
    return;
  }
  if (!result) {
    showPanel("settings");
  }
  const updateResult = document.querySelector<HTMLElement>("#update-result");
  if (updateResult) {
    updateResult.innerHTML = `<span>${t("web.main.text032", {p0: esc(name)})}</span> <button type="button" id="install-update" class="primary">${t("web.main.text033")}</button>`;
  }
  notice(`${t("web.main.text031", {p0: name})}`);
}
async function run(fn: () => Promise<void>) {
  if (busy) return;
  busy = true;
  document.body.classList.add("busy");
  try {
    notice("");
    await fn();
    await refresh();
  } catch (e) {
    notice(e instanceof Error ? e.message : String(e));
  } finally {
    busy = false;
    document.body.classList.remove("busy");
  }
}
function output(code: string) {
  $("#output-wrap").hidden = false;
  $<HTMLTextAreaElement>("#output-code").value = code;
  notice(t("web.main.text030"));
}
async function finishParty() {
  await rpc("leave");
  const panel = document.querySelector<HTMLDialogElement>("#panel");
  if (panel?.open) panel.close();
  currentPanel = "";
  autoJoining = false;
  selected = [];
  status.view = null;
  status.current = false;
  previous = "";
  $("#app").innerHTML = "";
  renderStart();
}
document.addEventListener("click", (e) => {
  const b = (e.target as HTMLElement).closest<HTMLElement>("button,a");
  if (!b) return;
  if (b instanceof HTMLButtonElement && b.disabled) return;
  if (b.dataset.contractIndex !== undefined) {
    const v = status.view!;
    if (!contractContext || v.revision !== contractContext.revision) return;
    const c = v.contracts[Number(b.dataset.contractIndex)];
    if (c) void run(() => submitContract(c));
    return;
  }
  if (b.dataset.debugBots !== undefined) {
    void run(async () => {
      const v=status.view!;
      await rpc("command",{command:{id:crypto.randomUUID(),seat:v.seat,revision:v.revision,action:"debug-bots",debugBots:b.dataset.debugBots}});
      $<HTMLDialogElement>("#panel").close();
    });
    return;
  }
  if (b.dataset.debugInspect!==undefined) {
    void run(async()=>{await rpc("debug-inspect",{seat:Number(b.dataset.debugInspect)});});
    return;
  }
  if (b.dataset.panel) {
    showPanel(b.dataset.panel,undefined,b.dataset.botSeat===undefined ? undefined : Number(b.dataset.botSeat));
    return;
  }
  if (b.dataset.autoJoin !== undefined) {
    void run(async () => {
      const name = document.querySelector<HTMLInputElement>('#create-form [name="name"]')?.value.trim() || appearance.name || t("web.main.text001");
      await rpc("profile", {name});
      appearance.name = name;
      autoJoining = true;
      notice(t("web.main.text029"));
      showPanel("network");
      await autoConnect();
    });
    return;
  }
  if (b.id === "copy-connection-log") {
    const text = [...(status.logs || []), ...signalingTrace].join("\n");
    void navigator.clipboard?.writeText(text).then(() => notice(t("web.main.text028")));
    return;
  }
  if (b.id === "clear-log-file") {
    void run(async()=>{await rpc('log-clear');notice(t('web.log.cleared'));});
    return;
  }
  if (b.id === "save-full-log") {
    window.PreferansAndroid?.saveFullLogToDownloads?.();
    return;
  }
  if (b.id === "save-connection-log") {
    const log = [...(status.logs || []), ...signalingTrace].join("\n");
    if (window.PreferansAndroid) {
      window.PreferansAndroid.saveLog(log);
    } else {
      const url = URL.createObjectURL(new Blob([log], {type: "text/plain;charset=utf-8"}));
      const link = document.createElement("a");
      link.href = url;
      link.download = "preferans.log";
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    }
    notice(t("web.main.text027"));
    return;
  }
  if (b.id === "check-update") {
    void run(checkForUpdate);
    return;
  }
  if (b.id === "install-update") {
    if (!pendingUpdateURL) return;
    if (window.PreferansAndroid?.installUpdate) {
      window.PreferansAndroid.installUpdate(pendingUpdateURL);
      notice(t("web.main.text026"));
    } else {
      window.open(pendingUpdateURL, "_blank");
      notice(t("web.main.text025"));
    }
    return;
  }
  if (b.id === "close-panel") {
    $<HTMLDialogElement>("#panel").close();
    return;
  }
  if (b.id === "party-history" || b.id === "history-back") void run(async () => {
    const entries = await rpc<HistoryEntry[]>("history");
    showPanel(t("web.main.text024"));
    if (status.room) $('#panel-title').textContent = `${t("web.main.text023", {p0: status.room.name})}`;
    const totals = historyTotals(entries);
    const summary = totals.length ? `<section class="history-totals"><h3>${t('web.history.totals')}</h3><p class="muted">${t('web.history.totals_scope')}</p><table><thead><tr><th>${t('web.history.player')}</th><th>${t('web.history.parties')}</th><th>${t('web.history.whists')}</th></tr></thead><tbody>${totals.map(player => `<tr><td>${esc(player.name)}</td><td>${player.parties}</td><td><strong>${player.whists.toLocaleString(locale)}</strong></td></tr>`).join('')}</tbody></table></section>` : '';
    $("#panel-body").innerHTML = summary + `<p class="muted">${t("web.main.text022")}</p>` + (entries.length ? entries.map(item => {
      const results = wholeWhists(item.results || []);
      return `<article class="history-entry"><div class="history-summary"><div class="history-players">${item.names.map((name, i) => `<div><strong>${esc(name)}</strong><span class="history-whists" title="${t("web.main.text050")}">${results[i] == null ? "—" : results[i].toLocaleString(locale)}</span></div>`).join("")}</div>${partyDates(item)}</div><button data-history-id="${esc(item.id)}">${t("web.main.text021")}</button></article>`;
    }).join("") : `<p>${t("web.main.text019")}</p>`);
  });
  if (b.dataset.historyId) void run(async () => {
    const result = await rpc<View>("history-result", {id:b.dataset.historyId});
    showPanel("history-result", result);
    $('#panel-body').innerHTML = `<button type="button" id="history-back" aria-label="${t("web.history.back")}">← ${t("web.history.back")}</button>${partyDates(result)}<div class="history-pool">${poolDrawing(result, esc)}</div>`;
    const drawing = $('#panel-body .pool-drawing');
    const staticDrawing = document.createElement('div');
    staticDrawing.className = drawing.className;
    staticDrawing.innerHTML = drawing.innerHTML;
    drawing.replaceWith(staticDrawing);
    $("#panel-title").textContent = t("web.main.text018");
  });
  if (b.id === "brand") {
    e.preventDefault();
    return;
  }
  if (b.dataset.action)
    void run(async () => {
      const v = status.view!;
      const action = b.dataset.action!;
      if (action==='start' && v.stage==='lobby' && v.players.some(p=>!p.ready)) {
        startWaitingTable=v.id;
        render();
        return;
      }
      if (action === "claim") { openClaim(); return; }
      if (action === "submit-claim") {
        if (!v.actions.includes("claim")) throw Error(t("web.main.text017"));
        await rpc("command", {command:{id:crypto.randomUUID(),seat:v.seat,revision:v.revision,action:"claim",tricks:Number($<HTMLSelectElement>("#claim-tricks").value)}});
        $<HTMLDialogElement>("#panel").close();
        return;
      }
      if (action === "bid" || action === "declare" || action === "declare-no-talon") { openContract(action); return; }
      if (action === "fill-bots") {
        await rpc("fill-bots");
        return;
      }
      const idx = Number($<HTMLSelectElement>("#contract")?.value || 0);
      const playerName = action === "ready"
        ? ($<HTMLInputElement>("#player-name")?.value || v.players[v.seat].name).trim()
        : undefined;
      await rpc("command", {
        command: {
          id: crypto.randomUUID(),
          seat: v.seat,
          revision: v.revision,
          round: v.round,
          action,
          cards: selected,
          name: playerName,
          contract: ["bid", "declare"].includes(action)
            ? v.contracts[idx]
            : undefined,
        },
      });
      if (action === "ready" && playerName) {
        await rpc("profile", { name: playerName });
        appearance.name = playerName;
      }
      selected = [];
      if (currentPanel === "contract") $<HTMLDialogElement>("#panel").close();
    });
  if (b.id === "delete-save")
    void run(async () => {
      const id = $<HTMLSelectElement>("#saved-party")?.value;
      if (!id) return;
      await rpc("delete-save", { id });
      await loadSaves();
      notice(t("web.main.text016"));
    });
  if (b.dataset.resume || b.id === "resume-save")
    void run(async () => {
      await rpc("resume", { id: b.dataset.resume || $<HTMLSelectElement>("#saved-party").value });
      previous = "";
      $("#app").innerHTML = "";
    });
  if (b.dataset.invite)
    void run(async () => {
      notice(t("web.main.text015"));
      output(await rpc<string>("invite", { seat: Number(b.dataset.invite) }));
    });
  if (b.id === "join")
    void run(async () => {
      notice(t("web.main.text015"));
      output(
        await rpc<string>("join", {
          code: $<HTMLTextAreaElement>("#input-code").value,
        }),
      );
    });
  if (b.id === "accept-answer")
    void run(async () => {
      const result = await rpc<{seat:number}>("answer", {
        code: $<HTMLTextAreaElement>("#input-code").value,
      });
      notice(t("web.main.text014"));
      pendingConnection = {table: status.view!.id, seat: result.seat};
    });
  if (b.id === "copy-code")
    void run(async () => {
      const el = $<HTMLTextAreaElement>("#output-code");
      try {
        await navigator.clipboard.writeText(el.value);
      } catch {
        el.select();
        document.execCommand("copy");
      }
      notice(t("web.main.text013"));
    });
  if (b.id === "download-code") {
    const code = $<HTMLTextAreaElement>("#output-code").value;
    if (window.PreferansAndroid) {
      window.PreferansAndroid.saveCode(code);
    } else {
      const blob = new Blob([code], { type: "text/plain" });
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = "preferans-code.txt";
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    }
  }
  if (b.id === "leave" || b.id === "finish-party" || b.id === "finish-party-topbar")
    void run(finishParty);
});
document.addEventListener("submit", (e) => {
  e.preventDefault();
  const f = e.target as HTMLFormElement;
  const d = new FormData(f);
  if(f.id==='connection-setup-form') void run(async()=>{
    const result=$('#setup-result');
    result.textContent=t("web.main.text012");
    f.querySelector<HTMLButtonElement>('button[type="submit"]')!.disabled=true;
    try {
      await rpc('room-service',{url:String(d.get('room-service-url'))});
      await rpc('connection-setup',{name:String(d.get('name')),stun:setupServers(d)});
      previous='';
      notice(t("web.main.text011"));
    } catch(error) {result.textContent=error instanceof Error?error.message:String(error);throw error;}
    finally {f.querySelector<HTMLButtonElement>('button[type="submit"]')!.disabled=false;}
  });
  if (f.id === "create-form")
    void run(async () => {
      if(!status.roomMode) try {
        const active = normalizeSignaling(await signalingRequest("GET"));
        if (active.offer?.code) {
          const requestedName = String(d.get("name") || t("web.main.text001")).trim();
          if (active.offer.name && active.offer.name === requestedName) {
            await signalingRequest("POST", {kind: "clear"});
          } else {
            throw Error(`${t("web.main.text010", {p0: active.offer.name || t("web.main.text009")})}`);
          }
        }
      } catch (e) {
        if (e instanceof Error && e.message.startsWith(t("web.main.text008"))) throw e;
      }
      await rpc("profile", { name: String(d.get("name") || t("web.main.text001")) });
      if (appearance.tableRules?.endCondition === 'time') {
        const minutes = Number(d.get('minutes'));
        if (minutes !== appearance.tableRules.minutes) {
          await rpc('appearance', {appearance: {...appearance, name: String(d.get('name') || t("web.main.text001")), tableRules: {...appearance.tableRules, minutes}}});
        }
      }
      await rpc("create", {
        name: d.get("name"),
        players: Number(d.get("players")),
        target: Number(d.get("target")),
        bots: d.has("bots"),
      });
      previous = "";
    });
  if (f.id === "settings-form")
    void run(async () => {
      if(status.aiEnabled !== false) await aiUI.save();
      const roomURL=String(d.get('room-service-url') || '').trim();
      if(roomURL!==(appearance.roomServiceURL || interfaceConfig.roomServiceURL))await rpc('room-service',{url:roomURL});
      const servers=settingsServers(f);
      if(JSON.stringify(servers)!==JSON.stringify(status.stun))await rpc('connection-setup',{stun:servers});
      const editable = !status.view || (status.host && status.view.stage === 'lobby');
      const tableRules = editable ? readRules(d) : appearance.tableRules;
      const target = Number(d.get('rule-target'));
      if (editable && status.view && (JSON.stringify(tableRules)!==JSON.stringify(status.view.rules || defaultRules) || target!==status.view.target)) {
        await rpc('command', {command:{id:crypto.randomUUID(),revision:status.view.revision,action:'rules',rules:tableRules,target}});
      }
      await rpc("appearance", {
        appearance: {
          language,
          poolTarget: editable ? target : appearance.poolTarget,
          playerCount: appearance.playerCount,
          debug: d.has("debug-auction") || d.has("debug-play"),
          debugAuction: d.has("debug-auction"),
          debugPlay: d.has("debug-play"),
          passPriceHint: d.has("pass-price-hint"),
          localOnly: d.has('local-only'),
          tableRules,
          deck: String(d.get("deck")),
          cardFrames: Object.fromEntries(decks.map(([id]) => [id, d.has(`frame-${id}`)])),
          size: "normal",
          largeIndices: String(d.get("deck")) === "english",
          back: String(d.get("back") || "plaid"),
          logDisabled: !d.has("log-enabled"),
          logPath: String(d.get('log-path') ?? appearance.logPath ?? '').trim(),
          updateURL: String(d.get("update-url") || "").trim(),
          defaultRoom: String(d.get('default-room') ?? appearance.defaultRoom ?? ''),
        },
      });
      if (editable && !status.view) {
        updateCreateLimit(tableRules, target);
      }
      $<HTMLDialogElement>("#panel").close();
    });
});
document.addEventListener("change", (e) => {
  const el = e.target as HTMLInputElement;
  if(el.matches('[data-language-picker]')) {
    const next=normalizeLanguage(el.value);
    if(next===language)return;
    void run(async()=>{
      // Fetch fresh preferences to preserve concurrent edits and table state.
      const latest=await rpc<Status>('status');
      await rpc('appearance',{appearance:{...latest.appearance,language:next}});
      window.PreferansAndroid?.setLanguage?.(next);
      await window.preferansLanguage?.(next);
      location.reload();
    });
    return;
  }
  if(el.name.startsWith('debug-') && status.aiEnabled !== false)aiUI.setDebug(!!document.querySelector<HTMLInputElement>('[name="debug-auction"]')?.checked || !!document.querySelector<HTMLInputElement>('[name="debug-play"]')?.checked);
  if (el.name === 'players' && el.closest('#create-form')) {
    const playerCount = Number(el.value);
    void run(async () => {
      await rpc('appearance', {appearance: {...appearance, playerCount}});
    });
  }
  if (el.name === 'rule-end') {
    const form = el.closest('fieldset')!;
    const timed = el.value === 'time';
    form.querySelector('[data-rule-limit-label]')!.textContent = timed ? t("web.main.text007") : t("web.main.text006");
    form.querySelector<HTMLSelectElement>('[name="rule-target"]')!.hidden = timed;
    form.querySelector<HTMLSelectElement>('[name="rule-minutes"]')!.hidden = !timed;
  }
  if (el.name === "deck" || el.name.startsWith("frame-")) previewDeck();
  if (el.id === "code-file" && el.files?.[0])
    void run(async () => {
      if (el.files![0].size > 131072) throw Error(t("web.main.text005"));
      $<HTMLTextAreaElement>("#input-code").value = await el.files![0].text();
    });
});
function updateExitButtons() {
  for (const button of document.querySelectorAll<HTMLButtonElement>('#finish-party, #finish-party-topbar, #leave')) {
    button.textContent = status.host ? t("web.main.text004") : t("web.main.text003");
    if (button.id === 'leave') button.hidden = !status.current;
  }
}
let refreshing = false;
async function refresh() {
  if (refreshing) return;
  refreshing = true;
  try { await refreshState(); } finally { refreshing = false; }
}
async function refreshState() {
  const s = await rpc<Status>("status");
  window.PreferansAndroid?.setLanguage?.(language);
  if (s.view) await prepareCards(s.appearance);
  else void prepareCards(s.appearance);
  // Keep the actual button under the finger until the native click is dispatched.
  if(buttonPressActive())return;
  if (pendingConnection && (s.view?.id !== pendingConnection.table || s.connected[pendingConnection.seat])) notice('');
  const key = JSON.stringify([
    s.view?.id,
    s.view?.revision,
    s.view?.debugBotSeat,
    s.view?.debugWaitingSeat,
    s.aiReview?.id,
    s.view?.aiReviewSeat,
    s.current,
    s.paused,
    s.connected,
    s.connectionReasons,
    s.roomOnline,
    s.freeSeats,
    s.appearance,
    s.room?.id,
    s.room?.name,
    s.roomMode,
    s.roomLocal,
    s.aiEnabled,
    s.connectionSetup,
  ]);
  if (s.error && s.error !== lastError) notice(s.error);
  lastError = s.error;
  if (key !== previous) {
    if (s.view?.id !== status.view?.id || s.view?.revision !== status.view?.revision) {
      selected = []; 
      if (currentPanel === "contract") $<HTMLDialogElement>("#panel").close();
      contractContext = null; 
    }
    status = s;
    applyAppearance(s.appearance);
    previous = key;
    render();
  } else status = s;
  if(s.aiReview){
    if(shownAIReview!==s.aiReview.id){shownAIReview=s.aiReview.id;showPanel("ai-review");}
    aiReviewUI.update(s.aiReview);
  }else{
    shownAIReview="";
    if(currentPanel==="ai-review"){ $<HTMLDialogElement>("#panel").close();currentPanel=""; }
  }
  updateBotThinking();
  syncAISettingsTab();
  roomUI.paint();
  updateLogFileSize(s);
  voice.sync(s.roomLocal || soloWithBots(s.view) ? {...s, view:null} : {...s,...(roomVoice(s)||{})});
  renderConnectionLog();
  updateExitButtons();
  if (document.querySelector('#create-form')) {
    const finish = document.querySelector<HTMLButtonElement>('#finish-party');
    if (finish) finish.hidden = !s.current;
  }
  const progress = document.querySelector<HTMLButtonElement>('.table-heading [data-panel="score"]');
  if (progress && s.view) {
    progress.id = 'table-progress';
    progress.innerHTML = tableProgress(s.view);
    if (s.view.rules?.endCondition === 'time') {
      const total = document.querySelector('.pool-total');
      if (total) total.textContent = String(remainingMinutes(s.view));
    }
  }
  if (!s.roomLocal && !autoUpdateChecked && window.PreferansAndroid && s.appearance.updateURL) {
    autoUpdateChecked = true;
    void checkForUpdate(true);
  }
  if (autoJoining && !s.host && s.view && s.connected?.[s.view.seat]) {
    autoJoining = false;
    const connectionPanel = $<HTMLDialogElement>("#panel");
    if (connectionPanel.open && currentPanel === "network") {
      connectionPanel.close();
      currentPanel = "";
    }
    notice("");
  }
  void autoConnect();
}

const cancelCardDrag = cardDrag(() => {
  const v=status.view;
  return {key:`${v?.id}/${v?.revision}`,mode:v?.stage || "",enabled:!!v && !busy && !status.paused && !$<HTMLDialogElement>("#panel").open && ((v.actions || []).includes("play") || ((v.actions || []).includes("declare") && !talonThrown(v)))};
}, async (c,from,to) => {
  const v=status.view;
  if (!v || busy || status.paused) return false;
  if (v.stage==="play" && from==="hand" && to==="table" && (v.actions || []).includes("play") && (v.legal || []).includes(c)) {
    let accepted=false;
    await run(async()=>{await rpc("command",{command:{id:crypto.randomUUID(),seat:v.seat,revision:v.revision,action:"play",cards:[c]}}); accepted=true;});
    return accepted;
  } else if(v.stage==="discard" && !talonThrown(v) && (v.actions || []).includes("declare")) {
    if(from==="hand" && to==="discard" && v.hand.includes(c) && !selected.includes(c) && selected.length<2) selected.push(c);
    else if(from==="discard" && to==="hand") selected=selected.filter(x=>x!==c);
    render();
  }
  return false;
});
async function poll() {
  try {
    await refresh();
  } catch (e) {
    notice(`${t("web.main.text002", {p0: String(e)})}`);
  }
  setTimeout(poll, interfaceConfig.uiPollIntervalMs);
}
const roomUI=new RoomUI(rpc,run,()=>status,()=>appearance.name || t("web.main.text001"));
void poll();

