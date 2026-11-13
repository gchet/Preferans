import {t,locale} from './i18n';
import type {Room,RoomParty} from './rooms';

const esc=(v:string)=>v.replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));
const date=(value?:number)=>value?new Date(value*1000).toLocaleString(locale):'—';

/** Переименовать и удалить комнату может её создатель; у комнат без записанного создателя (старые версии) — любой. */
export const ownsRoom=(room:Room,self:string)=>!room.creator||room.creator===self;

function partyList(room:Room):RoomParty[]{
  const parties=[...(room.parties||[])];
  const table=room.table;
  if(table&&!parties.some(p=>p.id===table.id))parties.unshift({
    id:table.id,host:table.host,
    players:table.players.map((id,i)=>({id,name:room.members.find(m=>m.id===id)?.name||(i===0?table.name:'—'),bot:!!table.bots[i]})),
    stage:'unknown',round:0,startedAt:0,finishedAt:0,updatedAt:0,
  });
  return parties.sort((a,b)=>Number(b.id===table?.id)-Number(a.id===table?.id)||b.updatedAt-a.updatedAt);
}

function partyHTML(room:Room,p:RoomParty):string{
  const active=p.id===room.table?.id;
  const state=p.stage==='finished'?'web.admin.finished':active?'web.admin.active':'web.admin.saved';
  const host=p.players.find(x=>x.id===p.host)?.name||'—';
  const players=p.players.filter(x=>!x.bot).map(x=>x.name).join(' · ');
  return `<div class="rooms-party"><div><strong>${t(state)}</strong>${p.round?` · ${t('web.admin.round',{p0:p.round})}`:''}</div><div>${t('web.admin.host',{p0:esc(host)})}</div><div>${t('web.admin.player_names',{p0:esc(players)})}</div><div class="muted">${t('web.admin.dates',{p0:date(p.startedAt),p1:date(p.finishedAt)})}</div><div class="muted">${t('web.admin.last_updated',{p0:date(p.updatedAt)})}</div><small class="muted">${t('web.admin.id',{p0:esc(p.id)})}</small></div>`;
}

export function creatorHTML(room:Room,self:string):string{
  if(!room.creator&&!room.creatorName)return `<span class="muted">${t('web.rooms.creator_unknown')}</span>`;
  const mine=!!room.creator&&room.creator===self;
  return `<strong>${esc(room.creatorName||'—')}</strong>${mine?` <em>${t('web.rooms.mine')}</em>`:''}${room.creator?`<small class="muted rooms-device">${esc(room.creator.slice(0,8))}</small>`:''}${room.created?`<small class="muted">${t('web.rooms.created',{p0:date(room.created)})}</small>`:''}`;
}

/** Таблица комнат в духе админки: создатель, участники, история партий; действия — только над своими комнатами. */
export function roomsTable(rooms:Room[],self:string,open:ReadonlySet<string>):string{
  if(!rooms.length)return `<p class="muted">${t('web.rooms.text009')}</p>`;
  const rows=rooms.map(room=>{
    const parties=partyList(room);
    const finished=parties.filter(p=>p.stage==='finished').length;
    const online=room.members.filter(m=>m.online).length;
    const name=`<button type="button" class="rooms-pick" data-room-pick="${esc(room.id)}" title="${esc(t('web.rooms.pick'))}"><strong>${esc(room.name)}</strong></button><small class="muted">${online}/4${room.table?t('web.rooms.text010'):''}</small>`;
    const members=room.members.length
      ?`<ul class="rooms-members">${room.members.map(m=>`<li class="${m.online?'':'muted'}"><strong>${esc(m.name)}</strong> <small>${t(m.online?'web.admin.online':'web.admin.offline')}</small></li>`).join('')}</ul>`
      :`<span class="muted">${t('web.admin.no_players')}</span>`;
    const history=parties.length
      ?`<details data-room-history="${esc(room.id)}" ${open.has(room.id)?'open':''}><summary>${t('web.rooms.history_summary',{p0:parties.length,p1:finished})}</summary>${parties.map(p=>partyHTML(room,p)).join('')}</details>`
      :`<span class="muted">${t('web.rooms.no_history')}</span>`;
    const actions=ownsRoom(room,self)
      ?`<div class="rooms-actions"><button type="button" data-room-action="rename" data-room-id="${esc(room.id)}" aria-label="${esc(t('web.rooms.rename_named',{p0:room.name}))}">${t('web.rooms.text019')}</button><button type="button" class="danger" data-room-action="delete" data-room-id="${esc(room.id)}" aria-label="${esc(t('web.rooms.delete_named',{p0:room.name}))}">${t('web.rooms.text020')}</button></div>`
      :`<span class="muted" title="${esc(t('web.rooms.only_creator'))}">—</span>`;
    return `<tr><td>${name}</td><td>${creatorHTML(room,self)}</td><td>${members}</td><td>${history}</td><td>${actions}</td></tr>`;
  }).join('');
  return `<div class="rooms-table-scroll"><table class="rooms-table"><thead><tr><th>${t('web.rooms.col_room')}</th><th>${t('web.rooms.col_creator')}</th><th>${t('web.rooms.col_players')}</th><th>${t('web.rooms.col_history')}</th><th>${t('web.rooms.col_actions')}</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}
