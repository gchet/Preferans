import './button-feedback.css';
import {interfaceConfig} from './interface-config';
let held: {button:HTMLButtonElement;id:number;x:number;y:number}|null=null;
export const buttonPressActive=()=>held!==null;
function release(){held?.button.classList.remove('button-contact');held=null;}
document.addEventListener('pointerdown',e=>{
  const button=(e.target as Element).closest<HTMLButtonElement>('button');
  if(!button || button.disabled || e.button!==0 || button.matches('.card'))return;
  release();held={button,id:e.pointerId,x:e.clientX,y:e.clientY};button.classList.add('button-contact');
},true);
document.addEventListener('pointermove',e=>{
  if(held?.id===e.pointerId && Math.hypot(e.clientX-held.x,e.clientY-held.y)>interfaceConfig.buttonCancelDistancePx)release();
},true);
document.addEventListener('pointerup',release,true);
document.addEventListener('pointercancel',release,true);
window.addEventListener('blur',release);
document.addEventListener('click',e=>{
  const button=(e.target as Element).closest<HTMLButtonElement>('button');
  if(!button || button.disabled || button.matches('.card'))return;
  const r=button.getBoundingClientRect(),pulse=document.createElement('span');
  pulse.className='button-click-feedback';pulse.setAttribute('aria-hidden','true');
  pulse.style.animationDuration=`${interfaceConfig.buttonFeedbackDurationMs}ms`;
  Object.assign(pulse.style,{left:`${r.left}px`,top:`${r.top}px`,width:`${r.width}px`,height:`${r.height}px`,borderRadius:getComputedStyle(button).borderRadius});
  // A popover stays above modal dialogs and survives replacement of the button.
  pulse.setAttribute('popover','manual');document.body.append(pulse);
  if(pulse.showPopover)pulse.showPopover();
  setTimeout(()=>pulse.remove(),interfaceConfig.buttonFeedbackDurationMs+20);
},true);
