import {t} from './i18n';
import {interfaceConfig} from './interface-config';
import './debug-bot.css';

type RPC = <T=unknown>(action:string,params?:Record<string,unknown>)=>Promise<T>;
export interface DebugTurn {seat:number;revision:number;name:string}
export interface DebugOption {action:string;cards?:number[];contract?:{level:number;suit:number;misere:boolean;noTalon?:boolean};tricks?:number;label:string}
interface DebugView {stage:string;hand:number[];playHand?:number[];talonShown?:boolean;bid?:DebugOption['contract']}
interface Situation {view:DebugView;legal_actions:DebugOption[]}
const esc=(value:unknown)=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));

export class DebugBotUI {
  constructor(private rpc:RPC, private card:(card:number)=>string, private label:(option:DebugOption)=>string, private done:()=>void, private contracts:(options:DebugOption[],view:DebugView)=>string) {}

  async mount(root:HTMLElement,turn:DebugTurn) {
    let situation:Situation;
    let selected:number[]=[];
    let choosing=false;
    let declaring=false;
    let busy=false;
    const options=async()=>this.rpc<Situation>('debug-bot-options',{seat:turn.seat,command:{revision:turn.revision,cards:selected}});
    const paint=()=>{
      const hand=situation.view.playHand?.length?situation.view.playHand:situation.view.hand;
      const discard=situation.view.stage==='discard';
      const auction=situation.view.stage==='auction';
      const marking=discard && choosing && !declaring && !situation.view.talonShown;
      const cards=hand.map(c=>marking?`<button type="button" class="debug-discard-card" data-debug-card="${c}" aria-pressed="${selected.includes(c)}" aria-label="${esc(t('web.debug.discard_card',{p0:c}))}">${this.card(c)}</button>`:this.card(c)).join('');
      const choices=discard || auction?declaring?this.contracts(situation.legal_actions,situation.view):`${discard?`<p>${t('web.debug.discard')}</p>`:''}<div class="toolbar"><button type="button" data-debug-bet class="primary" ${discard && !situation.view.talonShown && selected.length!==2?'disabled':''}>${t('web.main.text144')}</button>${situation.legal_actions.map((option,i)=>option.action!==(auction?'bid':'declare')?`<button type="button" data-debug-option="${i}">${esc(this.label(option))}</button>`:'').join('')}</div>`:`<div class="debug-turn-options">${situation.legal_actions.map((option,i)=>`<button type="button" data-debug-option="${i}" aria-label="${esc(this.label(option))}">${option.cards?.length && option.action==='play'?option.cards.map(this.card).join(''):esc(this.label(option))}</button>`).join('')}</div>`;
      root.innerHTML=`<div class="debug-turn-cards">${cards}</div><div class="toolbar"><button type="button" data-debug-continue class="primary">${t('web.debug.continue')}</button>${!choosing?`<button type="button" data-debug-choose>${t('web.debug.choose')}</button>`:`<button type="button" data-debug-back>${t('web.debug.back')}</button>`}</div>${choosing?choices:''}<p data-debug-error role="status"></p>`;
    };
    try {
      situation=await options();
      if(!root.isConnected)return;
      paint();
    } catch(error) { if(root.isConnected)root.textContent=String(error);return; }
    root.onclick=event=>{
      const button=(event.target as Element).closest<HTMLButtonElement>('button');
      if(!button || button.disabled || busy)return;
      if(button.dataset.debugCard!==undefined) {
        const card=Number(button.dataset.debugCard);
        if(selected.includes(card))selected=selected.filter(c=>c!==card);
        else if(selected.length<2)selected.push(card);
        paint();return;
      }
      void (async()=>{
        busy=true;
        root.querySelectorAll<HTMLButtonElement>('button').forEach(b=>b.disabled=true);
        try {
          if(button.hasAttribute('data-debug-continue')) {
            await this.rpc('debug-bot-continue',{seat:turn.seat,command:{revision:turn.revision}});
            this.done();return;
          }
          if(button.hasAttribute('data-debug-choose')) { choosing=true;paint();return; }
          if(button.hasAttribute('data-debug-back')) { selected=[];declaring=false;choosing=false;situation=await options();if(root.isConnected)paint();return; }
          if(button.hasAttribute('data-debug-bet')) { situation=await options();declaring=true;if(root.isConnected)paint();return; }
          if(button.dataset.debugOption!==undefined) {
            const option=situation.legal_actions[Number(button.dataset.debugOption)];
            if(!option)return;
            await this.rpc('debug-bot-command',{command:{id:crypto.randomUUID(),seat:turn.seat,revision:turn.revision,action:option.action,cards:option.cards,contract:option.contract,tricks:option.tricks}});
            this.done();
          }
        } catch(error) {if(root.isConnected)root.querySelector('[data-debug-error]')!.textContent=String(error);}
        finally {busy=false;if(root.isConnected)root.querySelectorAll<HTMLButtonElement>('button').forEach(b=>b.disabled=b.dataset.debugOption==='-1' || b.hasAttribute('data-debug-bet') && situation.view.stage==='discard' && !situation.view.talonShown && selected.length!==2);}
      })();
    };
  }
}

// Native context menus and the click synthesized after a long press are suppressed.
export function installDebugBotMenu(open:(seat:number)=>void) {
  let press:{id:number;x:number;y:number;seat:number;timer:number}|null=null;
  let suppressUntil=0;
  const cancel=()=>{if(press)clearTimeout(press.timer);press=null;};
  const seatAt=(target:EventTarget|null)=>{
    const element=(target as Element|null)?.closest<HTMLElement>('[data-debug-waiting]');
    return element?Number(element.dataset.debugWaiting):null;
  };
  document.addEventListener('contextmenu',event=>{
    const seat=seatAt(event.target);
    if(seat==null)return;
    event.preventDefault();cancel();
    open(seat);
  });
  document.addEventListener('pointerdown',event=>{
    cancel();const seat=seatAt(event.target);
    if(seat==null || event.pointerType!=='touch')return;
    press={id:event.pointerId,x:event.clientX,y:event.clientY,seat,timer:window.setTimeout(()=>{
      const active=press;press=null;if(!active)return;
      suppressUntil=Date.now()+interfaceConfig.dragClickSuppressionMs;
      open(active.seat);
    },interfaceConfig.botDebugLongPressMs)};
  });
  document.addEventListener('pointermove',event=>{
    if(press && (event.pointerId!==press.id || Math.hypot(event.clientX-press.x,event.clientY-press.y)>interfaceConfig.buttonCancelDistancePx))cancel();
  });
  document.addEventListener('pointerup',cancel);
  document.addEventListener('pointercancel',cancel);
  document.addEventListener('click',event=>{if(Date.now()<suppressUntil && seatAt(event.target)!=null){event.preventDefault();event.stopImmediatePropagation();}},true);
}
