import {interfaceConfig} from './interface-config';
type Zone = "table" | "discard" | "hand";
interface Context { key: string; enabled: boolean; mode: string }

// Pointer events cover mouse, touch and pen without native HTML drag-and-drop.
export function cardDrag(context: () => Context, drop: (card: number, from: Zone, to: Zone) => Promise<boolean>) {
  let drag: {id:number; card:number; from:Zone; key:string; x:number; y:number; source:HTMLElement; ghost:HTMLElement|null; width:number; height:number} | null = null;
  let suppressClickUntil = 0;
  let pending = false;
  const zoneAt = (x:number,y:number): Zone|null => {
    // Layout overlays and child cards must not punch holes in a drop zone.
    for (const [selector,zone] of [['.discard-tray','discard'],['.hand','hand'],['.playing-field .center','table']] as const) {
      const el=document.querySelector<HTMLElement>(selector);
      if (!el) continue;
      const r=el.getBoundingClientRect();
      if(r.width>0 && r.height>0 && x>=r.left && x<=r.right && y>=r.top && y<=r.bottom) return zone;
    }
    return null;
  };
  const clearTargets = () => document.querySelectorAll('.drop-active').forEach(el=>el.classList.remove('drop-active'));
  const cancel = () => {
    if (!drag) return;
    const d=drag; drag=null;
    d.ghost?.remove(); d.source.classList.remove('drag-source');
    try { if(d.source.hasPointerCapture(d.id)) d.source.releasePointerCapture(d.id); } catch {}
    clearTargets(); document.body.classList.remove('dragging-card');
  };
  document.addEventListener('pointerdown', e => {
    const source=(e.target as HTMLElement).closest<HTMLElement>('[data-card]');
    const c=context();
    if (drag || pending || !e.isPrimary || e.button !== 0 || !source || source.matches(':disabled') || !c.enabled) return;
    const r=source.getBoundingClientRect();
    drag={id:e.pointerId,card:Number(source.dataset.card),from:source.closest('.discard-tray')?'discard':'hand',key:c.key,x:e.clientX,y:e.clientY,source,ghost:null,width:r.width,height:r.height};
    try {source.setPointerCapture(e.pointerId);} catch {}
    e.preventDefault();
  });
  document.addEventListener('pointermove', e => {
    if (!drag || drag.id!==e.pointerId) return;
    const c=context();
    if (!c.enabled || c.key!==drag.key) {cancel();return;}
    if (!drag.ghost && Math.hypot(e.clientX-drag.x,e.clientY-drag.y)<interfaceConfig.dragStartDistancePx) return;
    e.preventDefault();
    if (!drag.ghost) {
      const ghost=drag.source.cloneNode(true) as HTMLElement;
      ghost.removeAttribute('data-card'); ghost.removeAttribute('id'); ghost.setAttribute('aria-hidden','true');
      ghost.classList.add('drag-ghost');
      ghost.style.width=`${drag.width}px`; ghost.style.height=`${drag.height}px`;
      drag.ghost=ghost; document.body.append(ghost);
      drag.source.classList.add('drag-source'); document.body.classList.add('dragging-card');
    }
    drag.ghost.style.left=`${e.clientX-drag.width/2}px`;
    drag.ghost.style.top=`${e.clientY-drag.height/3}px`;
    clearTargets();
    const zone=zoneAt(e.clientX,e.clientY);
    const allowed=c.mode==='play' ? zone==='table' : (drag.from==='hand' && zone==='discard') || (drag.from==='discard' && zone==='hand');
    if(allowed) document.querySelector(zone==='table'?'.playing-field .center':zone==='discard'?'.discard-tray':'.hand')?.classList.add('drop-active');
  },{passive:false});
  document.addEventListener('pointerup', async e => {
    if (!drag || drag.id!==e.pointerId) return;
    const d=drag, c=context(), zone=zoneAt(e.clientX,e.clientY);
    suppressClickUntil=Date.now()+interfaceConfig.dragClickSuppressionMs;
    const allowed = c.mode==='play' ? zone==='table' : (d.from==='hand' && zone==='discard') || (d.from==='discard' && zone==='hand');
    if (!d.ghost || !c.enabled || c.key!==d.key || !zone || !allowed) {cancel();return;}
    // Keep the released card on the table while the command travels to the host.
    // Releasing pointer capture must not restore the card in the old hand.
    drag=null;
    pending=true;
    d.source.style.visibility='hidden';
    try { if(d.source.hasPointerCapture(d.id)) d.source.releasePointerCapture(d.id); } catch {}
    clearTargets(); document.body.classList.remove('dragging-card');
    let timer: ReturnType<typeof setTimeout>;
    let settling=false;
    const selector=zone==='table'?'.trick-cards':zone==='discard'?'.discard-tray':'.hand';
    const destination=()=>Array.from(document.querySelectorAll<HTMLElement>(`${selector} .card`)).find(el=>el.getAttribute('aria-label')===d.source.getAttribute('aria-label'));
    const finish = (animate=false) => {
      if(settling) return;
      settling=true;
      clearTimeout(timer);
      const target=destination();
      const cleanup=()=>{
        observer.disconnect();
        destination()?.style.removeProperty('visibility');
        target?.style.removeProperty('visibility');
        d.ghost?.remove(); d.source.style.removeProperty('visibility');
        d.source.classList.remove('drag-source'); pending=false;
      };
      if(animate && target && d.ghost) {
        target.style.visibility='hidden';
        // fitCardFans runs on the next frame; measure its final layout afterwards.
        requestAnimationFrame(()=>{
          const r=(destination() || target).getBoundingClientRect();
          const x=parseFloat(d.ghost!.style.left), y=parseFloat(d.ghost!.style.top);
          d.ghost!.style.transformOrigin='top left';
          const motion=d.ghost!.animate([
            {transform:'translate(0, 0) rotate(-3deg) scale(1)'},
            {transform:`translate(${r.left-x}px, ${r.top-y}px) rotate(0deg) scale(${r.width/d.width}, ${r.height/d.height})`}
          ],{duration:interfaceConfig.cardFlightDurationMs,easing:'cubic-bezier(.22,.61,.36,1)',fill:'forwards'});
          motion.finished.then(cleanup,cleanup);
        });
      } else cleanup();
    };
    const observer = new MutationObserver(() => {
      if(settling) {
        // A bot/network update may redraw the destination during the flight.
        const target=destination();
        if(target) target.style.visibility='hidden';
        return;
      }
      if(context().key!==d.key || !d.source.isConnected) finish(true);
    });
    observer.observe(document.querySelector('#app') || document.body, {childList:true,subtree:true});
    timer=setTimeout(finish,interfaceConfig.cardDropTimeoutMs);
    try { if(!await drop(d.card,d.from,zone)) finish(); } catch {finish();}
  });
  document.addEventListener('pointercancel', e=>{if(drag?.id===e.pointerId)cancel();});
  document.addEventListener('lostpointercapture', e=>{if(drag?.id===e.pointerId)cancel();});
  document.addEventListener('dragstart', e=>{if((e.target as HTMLElement).closest('.card'))e.preventDefault();});
  document.addEventListener('click', e=>{
    const target=e.target as HTMLElement;
    if((Date.now()<suppressClickUntil && target.closest('.playing-field,.hand,.discard-tray')) || target.closest('[data-card]')) {e.preventDefault();e.stopImmediatePropagation();}
  },true);
  window.addEventListener('blur',cancel);
  document.addEventListener('visibilitychange',()=>{if(document.hidden)cancel();});
  return cancel;
}
