const $=id=>document.getElementById(id);
const token=location.hash.slice(1)||sessionStorage.getItem("netcheck-token")||"";
sessionStorage.setItem("netcheck-token",token);history.replaceState(null,"",location.pathname);
let status={}, probes=[], busy=false, initialized=false, last="", pollCount=0;
const events=[], started=new Date().toISOString();
const esc=s=>String(s??"").replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
const event=(message)=>{events.push({time:new Date().toISOString(),message});if(events.length>240)events.shift()};
async function rpc(action,params={}) {
 const response=await fetch("/api",{method:"POST",headers:{"Content-Type":"application/json","X-Preferans-Token":token},body:JSON.stringify({action,...params})});
 if(!response.ok)throw Error("HTTP "+response.status);
 const result=await response.json();if(result.error)throw Error(result.error);return result.data;
}
async function run(task) {
 if(busy)return;busy=true;buttons();$("notice").textContent="Выполняется…";
 try {await task()} catch(e){$("notice").textContent=String(e);event(String(e))}
 finally {busy=false;buttons()}
}
function buttons(){
 for(const b of document.querySelectorAll("button"))b.disabled=busy;
 $("bot").disabled=busy||!status.host||!status.freeSeats;
 $("ready").disabled=busy||!status.view||status.view.stage!=="lobby";
 $("reinvite").disabled=busy||!status.host||Boolean(status.connected?.[1]);
 $("answer").disabled=busy||!status.host;
}
function output(code){$("output").value=code;$("output-wrap").hidden=false}
function save(text,name){
 if(window.PreferansAndroid){
  if(name.endsWith(".json")&&window.PreferansAndroid.saveReport){window.PreferansAndroid.saveReport(text);return}
  if(name.endsWith(".txt")){window.PreferansAndroid.saveCode(text);return}
 }
 const url=URL.createObjectURL(new Blob([text],{type:"text/plain;charset=utf-8"}));
 const a=document.createElement("a");a.href=url;a.download=name;a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
}
async function copy(text,area){try{await navigator.clipboard.writeText(text);$("notice").textContent="Скопировано"}catch{area.focus();area.select();$("notice").textContent="Выделено: скопируйте текст вручную"}}
$("configure").onclick=()=>run(async()=>{await rpc("configure",{stun:$("stun").value.split(/\r?\n/).map(s=>s.trim()).filter(Boolean)});$("notice").textContent="STUN сохранены. Применяются к новому соединению.";event("Изменены STUN")});
$("create").onclick=()=>run(async()=>{await rpc("create",{players:3,target:30,name:$("name").value||"Ведущий",bots:false});output(await rpc("invite",{seat:1}));$("notice").textContent="Передайте приглашение второму устройству.";event("Создан тест и приглашение")});
$("reinvite").onclick=()=>run(async()=>{output(await rpc("invite",{seat:1}));$("notice").textContent="Передайте новое приглашение прежнему устройству.";event("Повторное приглашение")});
$("join").onclick=()=>run(async()=>{output(await rpc("join",{code:$("input").value}));$("notice").textContent="Передайте ответ ведущему.";event("Сформирован ответ")});
$("answer").onclick=()=>run(async()=>{await rpc("answer",{code:$("input").value});$("notice").textContent="Ответ принят. Следите за живыми счётчиками ниже.";event("Принят ответ")});
$("file").onchange=()=>run(async()=>{const f=$("file").files[0];if(f){if(f.size>131072)throw Error("Файл слишком большой");$("input").value=await f.text();$("notice").textContent="Код загружен"}});
$("copy").onclick=()=>copy($("output").value,$("output"));
$("save-code").onclick=()=>save($("output").value,"preferans-code.txt");
$("ready").onclick=()=>run(async()=>{const s=await rpc("status"),v=s.view;if(!v)throw Error("Нет стола");await rpc("command",{command:{id:crypto.randomUUID(),revision:v.revision,seat:v.seat,action:"ready"}});$("notice").textContent="Готовность изменена — проверьте второе устройство.";event("Переключена готовность")});
$("bot").onclick=()=>run(async()=>{const n=await rpc("fill-bots");$("notice").textContent="Добавлено ботов: "+n;event("Добавлено ботов: "+n)});
$("leave").onclick=()=>run(async()=>{await rpc("leave");$("notice").textContent="Тест завершён";event("Тест завершён")});
function report(){
 const v=status.view;
 const data={started,generated:new Date().toISOString(),stun:status.stun,host:status.host,stage:v?.stage,revision:v?.revision,
 players:v?.players.map((p,i)=>({seat:i,name:p.name,bot:p.bot,ready:p.ready,connected:status.connected?.[i]})),probes,events};
 const text=JSON.stringify(data,null,2);$("report-text").value=text;return text;
}
$("report").onclick=()=>save(report(),"preferans-network-report.json");
$("copy-report").onclick=()=>copy(report(),$("report-text"));
const states={"connected":"Соединено","connecting":"Соединяемся","gathering":"Сбор адресов","waiting-answer":"Ожидается ответ","disconnected":"Связь прервана","failed":"Ошибка соединения","closed":"Закрыто"};
async function poll(){
 try{
  status=await rpc("status");probes=await rpc("diagnostics");
  if(!initialized){$("stun").value=(status.stun||[]).join("\n");initialized=true}
  const good=probes.length>0&&probes.every(p=>p.Healthy);
  $("verdict").textContent=good?"Обмен в обе стороны подтверждён":"Свежий обмен в обе стороны НЕ подтверждён";
  $("verdict").classList.toggle("good",good);
  $("links").innerHTML=probes.map(p=>`<div class="link"><b>Место ${p.seat+1}: ${esc(states[p.State]||p.State)}</b><p>Канал: ${esc(p.Channel)} · ICE: ${esc(p.LocalType||"—")} ↔ ${esc(p.RemoteType||"—")}</p><p>${esc(p.Route||"Маршрут ещё не выбран")}</p><div class="metrics">${[["Отправлено",p.Sent],["Подтверждено",p.Acknowledged],["Получено от партнёра",p.Received],["Таймауты >5 с",p.Timeouts],["Ошибки",p.Errors],["Задержка туда-обратно",p.Acknowledged?p.RTT.toFixed(1)+" мс":"—"],["Последний ответ",p.ReplyAge<0?"нет":p.ReplyAge.toFixed(1)+" с назад"]].map(([label,value])=>`<p>${label}<strong>${value}</strong></p>`).join("")}</div><small>${esc(p.LastError)}</small></div>`).join("");
  $("protocol-log").textContent=(status.logs||[]).join("\n")||"Ожидание событий соединения…";
  $("protocol-log").scrollTop=$("protocol-log").scrollHeight;
  const v=status.view;
  $("players").innerHTML=v?`<p>Роль: ${status.host?"ведущий":"участник"} · ревизия: ${v.revision}</p>`+v.players.map((p,i)=>`<p>${esc(p.name)}: ${p.bot?"бот (без сети)":p.ready?"Готов":"Не готов"} ${i===v.seat?"· это устройство":""}</p>`).join(""):"<p>Тестовый стол ещё не открыт</p>";
  const key=JSON.stringify([good,probes.map(p=>[p.State,p.Channel,p.Route,p.Timeouts,p.Errors]),v?.revision]);
  if(key!==last||++pollCount%10===0){event({links:probes,revision:v?.revision});last=key}
  if(good&&$("notice").textContent.startsWith("Ответ принят."))$("notice").textContent="Связь проверена реальным обменом.";
  buttons();
 }catch(e){$("verdict").textContent="Нет связи с локальным приложением: "+e;$("verdict").classList.remove("good")}
 setTimeout(poll,1000);
}
poll();
