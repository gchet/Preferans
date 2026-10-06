import {t} from './i18n';
import {voiceIcon} from './voice';
import {confirmDialog} from './confirm-dialog';
export interface Room {id:string;name:string;members:{id:string;name:string;slot:number;online:boolean}[];table?:{id:string;host:string;name:string;players:string[];bots:boolean[]}}
export interface RoomStatus {roomMode?:boolean;roomLocal?:boolean;appearance?:{localOnly?:boolean};room?:Room;roomSelf?:string;roomError?:string;roomOnline?:boolean;current?:boolean;connected?:boolean[];view?:{id:string;seat:number}|null}
type RPC=<T=unknown>(action:string,params?:Record<string,unknown>)=>Promise<T>;
const esc=(v:string)=>v.replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));
export class RoomUI {
  private rooms:Room[]=[];
  private loading=false;
  private lastLoad=0;
  private rosterKey='';
  private rejoining='';
  constructor(private rpc:RPC,private run:(fn:()=>Promise<void>)=>Promise<void>,private status:()=>RoomStatus,private name:()=>string){
    document.addEventListener('click',e=>{
      const b=(e.target as HTMLElement).closest<HTMLElement>('[data-room-action]');if(!b)return;
      void this.run(async()=>{
        const op=b.dataset.roomAction!,id=(document.querySelector('#room-select') as HTMLSelectElement)?.value || '';
        const name=(document.querySelector('#room-name') as HTMLInputElement)?.value.trim() || '';
        if(op==='local'){
          const player=document.querySelector<HTMLInputElement>('#room-player-name, #connection-setup-form [name="name"], #create-form [name="name"]')?.value || this.name();
          await this.rpc('profile',{name:player});await this.rpc('room-local');
        }else if(op==='enter'){
          if(!id)throw Error(t("web.rooms.text011"));
          const player=(document.querySelector('#room-player-name') as HTMLInputElement).value;
          await this.rpc('profile',{name:player});await this.rpc('room-enter',{id});
        }else if(op==='exit'){await this.rpc('room-exit');}
        else if(op==='rejoin'){
          if(this.rejoining)return;
          this.rejoining=this.status().room?.table?.id || '';this.paint();
          try{await this.rpc('room-rejoin');}
          catch(error){this.rejoining='';this.paint();throw error;}
        }
        else if(op==='close-table'){
          if(!await confirmDialog(t("web.rooms.close_confirm"), t("web.rooms.close_table")))return;
          await this.rpc('leave');
        }
        else if(op==='refresh'){await this.load();}
        else {
          if((op==='create'||op==='rename')&&!name)throw Error(t("web.rooms.text022"));
          if(op==='delete'&&!await confirmDialog(t("web.rooms.text021"), t("web.rooms.text020")))return;
          const out=await this.rpc<{room?:Room}>(`rooms-${op}`,{id,name});
          await this.load(out.room?.id);
        }
      });
    });
    document.addEventListener('change',e=>{if((e.target as HTMLElement).id==='room-select'){const r=this.rooms.find(r=>r.id===(e.target as HTMLSelectElement).value);const input=document.querySelector<HTMLInputElement>('#room-name');if(r&&input)input.value=r.name}});
  }
  picker(container:HTMLElement){
    if(!document.querySelector('#room-picker'))container.innerHTML=`<section id="room-picker" class="surface"><h1>${t("web.rooms.text011")}</h1><label>${t("web.connection_setup.text008")}<input id="room-player-name" maxlength="24" value="${esc(this.name())}"></label><label>${t("web.rooms.text012")}<select id="room-select"><option value="">${t("web.main.text071")}</option></select></label><div class="toolbar"><button type="button" class="primary" data-room-action="enter">${t("web.rooms.text013")}</button><button type="button" data-room-action="refresh">${t("web.rooms.text014")}</button></div><details><summary>${t("web.rooms.text015")}</summary><label>${t("web.rooms.text016")}<input id="room-name" maxlength="40" placeholder="${t("web.rooms.text017")}"></label><div class="toolbar"><button type="button" data-room-action="create">${t("web.rooms.text018")}</button><button type="button" data-room-action="rename">${t("web.rooms.text019")}</button><button type="button" data-room-action="delete">${t("web.rooms.text020")}</button></div></details><p id="room-directory-error" role="status"></p></section>`;
    if(!document.querySelector('#room-connection-error'))document.querySelector('#room-picker')!.insertAdjacentHTML('beforeend','<p id="room-connection-error" role="status"></p>');
    if(!document.querySelector('#room-picker [data-room-action="local"]'))document.querySelector('#room-picker')!.insertAdjacentHTML('beforeend',`<button type="button" data-room-action="local">${t('web.local.play')}</button>`);
    if(Date.now()-this.lastLoad>5000)void this.load();
  }
  async load(selected?:string){
    if(this.loading)return;this.loading=true;this.lastLoad=Date.now();
    try{
      const out=await this.rpc<{rooms:Room[]}>('rooms-list');this.rooms=out.rooms || [];
      const select=document.querySelector<HTMLSelectElement>('#room-select');
      if(select){const old=selected || select.value;select.innerHTML=this.rooms.length?this.rooms.map(r=>`<option value="${esc(r.id)}">${esc(r.name)} · ${r.members.filter(m=>m.online).length}/4${r.table?t("web.rooms.text010"):''}</option>`).join(''):`<option value="">${t("web.rooms.text009")}</option>`;if(this.rooms.some(r=>r.id===old))select.value=old;}
      const error=document.querySelector('#room-directory-error');if(error)error.textContent='';
    }catch(e){const error=document.querySelector('#room-directory-error');if(error)error.textContent=String(e);}
    finally{this.loading=false;}
  }
  async settings(selected:string){
    await this.load();const el=document.querySelector<HTMLSelectElement>('[name="default-room"]');if(!el)return;
    el.innerHTML=`<option value="">${t("web.rooms.text008")}</option>`+this.rooms.map(r=>`<option value="${esc(r.id)}">${esc(r.name)}</option>`).join('');
    if(selected&&!this.rooms.some(r=>r.id===selected))el.insertAdjacentHTML('beforeend',`<option value="${esc(selected)}">${t("web.rooms.text007")}</option>`);
    el.value=selected;
  }
  paint(){
    const s=this.status(),r=s.room;
    if(this.rejoining && (r?.table?.id!==this.rejoining || (s.view?.id===this.rejoining && !!s.connected?.[s.view.seat] && !!s.connected?.[0])))this.rejoining='';
    if(!r){if(document.querySelector('#room-picker')){if(Date.now()-this.lastLoad>5000)void this.load();const el=document.querySelector('#room-connection-error');if(el)el.textContent=s.roomError || '';}return;}
    const box=document.querySelector('#room-roster');if(!box)return;
    const key=JSON.stringify([r.id,r.name,s.roomSelf,r.table,r.members.map(m=>[m.id,m.name,m.slot,m.online]),s.roomOnline,s.roomError,s.current,s.view?.id,this.rejoining]);
    if(this.rosterKey===key&&box.childElementCount)return;this.rosterKey=key;
    const members=[...r.members].sort((a,b)=>Number(b.id===s.roomSelf)-Number(a.id===s.roomSelf));
    box.innerHTML=`<div class="room-heading"><button type="button" data-room-action="exit">${t("web.rooms.text006")}</button></div><div class="room-members">${members.map(m=>`<div class="room-member ${m.online?'':'connection-lost'}"><span class="room-member-label"><strong>${esc(m.name)}</strong>${m.id===s.roomSelf?` ${t("web.rooms.text001")} <em>${esc(r.name)}</em>`:''}</span>${s.roomLocal?'':voiceIcon(m.slot,m.id===s.roomSelf)}${m.online?'':`<small>${t("web.main.text197")}</small>`}</div>`).join('')}</div><p role="status">${s.roomLocal?t('web.local.hint'):s.roomError&&!this.rejoining?esc(s.roomError):!s.roomOnline&&!this.rejoining?t("web.rooms.text005"):r.table?`${this.rejoining && (s.roomError || !s.roomOnline) ? esc(s.roomError || t("web.rooms.text005")) + " " : ""}${t("web.rooms.text003", {p0: esc(r.table.name)})} <button type="button" data-room-action="rejoin" ${this.rejoining ? "disabled aria-busy=\"true\"" : ""}>${this.rejoining ? `<span class="connection-spinner" aria-hidden="true"></span>${t("web.connection.rejoining")}` : t("web.rooms.text004")}</button>`:t("web.rooms.text002")}</p>`;
    const exit=box.querySelector<HTMLButtonElement>('[data-room-action="exit"]');
    if(exit)exit.hidden=!!s.appearance?.localOnly;
    for(const el of document.querySelectorAll<HTMLButtonElement>('#create-form button[type="submit"],#resume-save'))el.disabled=!!r.table;
    if(r.table && r.table.host===s.roomSelf && (!s.current || s.view?.id!==r.table.id)){
      const message=box.querySelector('[role="status"]')!;
      message.innerHTML=`${esc(s.roomError || (!s.roomOnline?t("web.rooms.text005"):t("web.rooms.restore_missing")))} <button type="button" class="danger" data-room-action="close-table">${t("web.rooms.close_table")}</button>`;
    }
  }
}

export function roomVoice(status:RoomStatus){
	if(status.roomLocal)return null;
  const r=status.room;if(!r)return null;
  const own=r.members.find(m=>m.id===status.roomSelf);if(!own)return null;
  return {view:{id:r.id,seat:own.slot,players:Array.from({length:4},(_,i)=>({id:r.members.find(m=>m.slot===i)?.id,bot:!r.members.some(m=>m.slot===i)}))},connected:Array.from({length:4},(_,i)=>!!r.members.find(m=>m.slot===i)?.online)};
}
