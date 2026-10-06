import {t,locale} from './i18n';
import {interfaceConfig} from './interface-config';
import type {Room} from './rooms';
import {confirmDialog} from './confirm-dialog';

type RPC=<T=unknown>(action:string,params?:Record<string,unknown>)=>Promise<T>;
interface Party {
  id:string;host:string;players:{id:string;name:string;bot:boolean}[];
  stage:string;round:number;startedAt:number;finishedAt:number;updatedAt:number;
}
interface AdminRoom extends Room {parties?:Party[]}
const esc=(v:string)=>v.replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));
const date=(value:number)=>value?new Date(value*1000).toLocaleString(locale):'—';

export class AdminUI {
  private root:HTMLElement|null=null;
  private timer:ReturnType<typeof setTimeout>|undefined;
  private unlocked=false;
  private loading=false;
  private deleting=false;
  private canDelete=false;
  private collapsedRooms=new Set<string>();
  private partyTexts=new Map<string,string>();

  constructor(private rpc:RPC) {}
  mount(root:HTMLElement) {
    clearTimeout(this.timer);
    this.root=root;this.unlocked=false;this.loading=false;
    this.unlock(root);
  }
  private unlock(root:HTMLElement) {
    if(this.root!==root || this.unlocked)return;
    this.unlocked=true;
    root.innerHTML=`<div class="admin-toolbar"><h3>${t('web.admin.title')}</h3><button type="button" id="admin-refresh">${t('web.admin.refresh')}</button></div><p id="admin-status" role="status"></p><div id="admin-rooms"></div>`;
    root.querySelector('#admin-refresh')!.addEventListener('click',()=>void this.refresh());
    root.querySelector('#admin-rooms')!.addEventListener('click',event=>{
      const toggle=(event.target as Element).closest<HTMLButtonElement>('[data-admin-toggle]');
      if(toggle){this.toggleRoom(toggle);return;}
      const copy=(event.target as Element).closest<HTMLButtonElement>('[data-admin-copy]');
      if(copy){void this.copyParty(copy);return;}
      const button=(event.target as Element).closest<HTMLButtonElement>('[data-admin-delete]');
      if(button)void this.remove(button);
    });
    void this.refresh();
  }
  private async refresh() {
    const root=this.root;
    if(!root?.isConnected || !this.unlocked || this.loading || this.deleting)return;
    clearTimeout(this.timer);
    this.loading=true;
    const button=root.querySelector<HTMLButtonElement>('#admin-refresh')!;
    button.disabled=true;
    try {
      const out=await this.rpc<{rooms?:AdminRoom[];adminCatalog?:boolean;adminDelete?:boolean}>('admin-rooms');
      if(this.root!==root || !root.isConnected)return;
      this.canDelete=!!out.adminDelete;
      root.querySelector('#admin-status')!.textContent=!out.adminCatalog || !out.adminDelete?t('web.admin.server_update'):t('web.admin.updated',{p0:new Date().toLocaleTimeString(locale)});
      const rooms=out.rooms || [];
      this.partyTexts.clear();
      root.querySelector('#admin-rooms')!.innerHTML=rooms.length?`<div class="admin-table-scroll"><table class="admin-table"><thead><tr><th>${t('web.admin.room')}</th><th>${t('web.admin.players')}</th><th>${t('web.admin.parties')}</th></tr></thead><tbody>${rooms.map(room=>this.roomHTML(room)).join('')}</tbody></table></div>`:`<p>${t('web.admin.no_rooms')}</p>`;
    }catch(error){
      if(this.root===root && root.isConnected)root.querySelector('#admin-status')!.textContent=t('web.admin.failed',{p0:String(error)});
    }finally{
      button.disabled=false;
      if(this.root===root)this.loading=false;
      if(this.root===root && root.isConnected)this.timer=setTimeout(()=>{
        if(root.isConnected && !root.hidden)void this.refresh();
      },interfaceConfig.adminRefreshIntervalMs);
    }
  }
  activate() {if(this.unlocked)void this.refresh();}
  private toggleRoom(button:HTMLButtonElement) {
    const id=button.dataset.adminToggle!;
    if(this.collapsedRooms.has(id))this.collapsedRooms.delete(id);else this.collapsedRooms.add(id);
    const collapsed=this.collapsedRooms.has(id);
    button.setAttribute('aria-expanded',String(!collapsed));
    button.title=t(collapsed?'web.admin.expand':'web.admin.collapse');
    button.querySelector('.admin-room-arrow')!.textContent=collapsed?'▸':'▾';
    const row=button.closest('tr')!;
    row.querySelectorAll<HTMLElement>('[data-admin-detail]').forEach(el=>el.hidden=collapsed);
    row.querySelectorAll<HTMLElement>('[data-admin-summary]').forEach(el=>el.hidden=!collapsed);
  }
  private async copyParty(button:HTMLButtonElement) {
    const root=this.root;
    const text=this.partyTexts.get(`${button.dataset.room}\n${button.dataset.adminCopy}`);
    if(!root || text==null)return;
    try {
      try {
        if(!navigator.clipboard)throw Error('Clipboard API unavailable');
        await navigator.clipboard.writeText(text);
      }catch{
        // Android WebViews may lack Clipboard API support; keep focus in the dialog.
        const input=document.createElement('textarea');
        input.value=text;input.className='admin-clipboard';input.readOnly=true;
        root.append(input);
        try {input.select();if(!document.execCommand('copy'))throw Error(t('web.admin.copy_failed'));}
        finally {input.remove();button.focus();}
      }
      if(root===this.root && root.isConnected)root.querySelector('#admin-status')!.textContent=t('web.admin.copied');
    }catch(error){
      if(root===this.root && root.isConnected)root.querySelector('#admin-status')!.textContent=t('web.admin.failed',{p0:String(error)});
    }
  }
  private deleteButton(kind:string,room:AdminRoom,id:string,name:string):string {
    return `<button type="button" class="admin-delete danger" data-admin-delete="${kind}" data-room="${esc(room.id)}" data-element-id="${esc(id)}" data-name="${esc(name)}" title="${esc(t('web.admin.delete_named',{p0:name}))}" aria-label="${esc(t('web.admin.delete_named',{p0:name}))}" ${this.canDelete?'':'disabled'}>×</button>`;
  }
  private async remove(button:HTMLButtonElement) {
    if(this.deleting || !this.canDelete)return;
    this.deleting=true;clearTimeout(this.timer);
    const root=this.root;
    let failure='';
    try {
      const kind=button.dataset.adminDelete!;
      const name=button.dataset.name!;
      const message=t(`web.admin.confirm_${kind}`,{p0:name});
      if(!await confirmDialog(message,t('web.admin.delete')))return;
      button.disabled=true;
      await this.rpc(`admin-delete-${kind}`,{id:button.dataset.room,elementID:button.dataset.elementId});
    }catch(error){
      failure=t('web.admin.failed',{p0:String(error)});
    }finally{
      this.deleting=false;
      if(root===this.root && root?.isConnected)await this.refresh();
      if(failure && root===this.root && root?.isConnected)root.querySelector('#admin-status')!.textContent=failure;
    }
  }
  private roomHTML(room:AdminRoom):string {
    const parties=[...(room.parties || [])];
    const table=room.table;
    if(table && !parties.some(p=>p.id===table.id))parties.unshift({id:table.id,host:table.host,players:table.players.map((id,i)=>({id,name:room.members.find(m=>m.id===id)?.name || (i===0?table.name:'—'),bot:table.bots[i]})),stage:'unknown',round:0,startedAt:0,finishedAt:0,updatedAt:0});
    parties.sort((a,b)=>Number(b.id===table?.id)-Number(a.id===table?.id) || b.updatedAt-a.updatedAt);
    const collapsed=this.collapsedRooms.has(room.id);
    return `<tr class="admin-room"><td><div class="admin-item-heading"><button type="button" class="admin-room-toggle" data-admin-toggle="${esc(room.id)}" aria-expanded="${!collapsed}" title="${t(collapsed?'web.admin.expand':'web.admin.collapse')}"><span class="admin-room-arrow" aria-hidden="true">${collapsed?'▸':'▾'}</span><strong>${esc(room.name)}</strong></button>${this.deleteButton('room',room,room.id,room.name)}</div></td><td><span class="muted" data-admin-summary ${collapsed?'':'hidden'}>${room.members.length}</span><div data-admin-detail ${collapsed?'hidden':''}>${room.members.length?`<ul class="admin-members">${room.members.map(m=>`<li><div><strong>${esc(m.name)}</strong><small class="admin-device muted">${esc(m.id.slice(0,8))}</small><span class="${m.online?'':'muted'}">${t(m.online?'web.admin.online':'web.admin.offline')}</span></div>${this.deleteButton('player',room,m.id,`${m.name} (${m.id.slice(0,8)})`)}</li>`).join('')}</ul>`:`<p class="muted">${t('web.admin.no_players')}</p>`}</div></td><td><span class="muted" data-admin-summary ${collapsed?'':'hidden'}>${parties.length}</span><div data-admin-detail ${collapsed?'hidden':''}>${parties.length?parties.map(p=>{
      const active=p.id===table?.id;
      const state=p.stage==='finished'?'web.admin.finished':active?'web.admin.active':'web.admin.saved';
      const host=p.players.find(player=>player.id===p.host)?.name || '—';
      const players=p.players.filter(player=>!player.bot);
      this.partyTexts.set(`${room.id}\n${p.id}`,[
        `${t('web.admin.room')}: ${room.name}`,
        t('web.admin.room_id',{p0:room.id}),t('web.admin.id',{p0:p.id}),
        t('web.admin.state',{p0:`${t(state)} (${p.stage})`}),
        t('web.admin.round',{p0:p.round}),t('web.admin.host',{p0:host}),
        `${t('web.admin.players')}:`,...players.map(player=>`  ${player.name} (${player.id || '—'})`),
        t('web.admin.dates',{p0:date(p.startedAt),p1:date(p.finishedAt)}),
        t('web.admin.last_updated',{p0:date(p.updatedAt)}),
      ].join('\n'));
      return `<div class="admin-party"><div class="admin-item-heading"><div><strong>${t(state)}</strong> · ${t('web.admin.round',{p0:p.round})}</div>${this.deleteButton('party',room,p.id,p.id)}</div><div>${t('web.admin.host',{p0:esc(host)})}</div><div>${t('web.admin.player_names',{p0:esc(players.map(player=>player.name).join(' · '))})}</div><div class="muted">${t('web.admin.dates',{p0:date(p.startedAt),p1:date(p.finishedAt)})}</div><small class="muted">${t('web.admin.id',{p0:esc(p.id)})}</small><button type="button" class="admin-copy" data-admin-copy="${esc(p.id)}" data-room="${esc(room.id)}">${t('web.admin.copy')}</button></div>`;
    }).join(''):`<p class="muted">${t('web.admin.no_parties')}</p>`}</div></td></tr>`;
  }
}
