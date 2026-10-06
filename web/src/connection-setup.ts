import {t} from './i18n';
import {passwordField} from './password-field';
import {networkConfig} from './network-config';
const defaultServer = networkConfig.turnServer;
const esc = (s:string) => s.replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));

export function setupScreen(name:string, roomServiceURL:string) {
  return `<section id="connection-setup" class="surface" style="max-width:600px;margin:16px auto"><h1>${t("web.connection_setup.text006")}</h1><p>${t("web.connection_setup.text007")}</p><form id="connection-setup-form"><label>${t("web.connection_setup.text008")}<input name="name" maxlength="24" value="${esc(name)}" required></label>${passwordField("setup-turn-password", "required")}<label>${t("web.connection.room_service")}<input name="room-service-url" type="url" value="${esc(roomServiceURL)}" placeholder="https://signaling.example.org/v2/rooms" required></label><details ${defaultServer ? '' : 'open'}><summary>${t("web.connection_setup.text009")}</summary><label>${t("web.connection_setup.text010")}<input name="turn-server" value="${esc(defaultServer)}" placeholder="turn:turn.example.org:3478?transport=udp" required></label><label>${t("web.connection_setup.text011")}<input name="turn-user" value="${esc(networkConfig.turnUser)}" required></label></details><p id="setup-result" role="status"></p><button type="submit" class="primary">${t("web.connection_setup.text012")}</button></form></section>`;
}

export function setupServers(data:FormData):string[] {
  const password=String(data.get('turn-password') || '');
  if(!password || /^[*•]+$/.test(password))throw Error(t("web.connection_setup.text002"));
  if(/[|\r\n]/.test(password))throw Error(t("web.connection_setup.text001"));
  return [`${String(data.get('turn-server')).trim()}|${String(data.get('turn-user')).trim()}|${password}`,'stun:stun.l.google.com:19302','stun:stun.cloudflare.com:3478'];
}

export function enhanceConnectionSettings(servers:string[]) {
  const textarea=document.querySelector<HTMLTextAreaElement>('[name="stun"]')!;
  const label=textarea.closest('label')!, help=label.nextElementSibling!;
  const first=servers.find(s=>s.startsWith('turn:')||s.startsWith('turns:'))?.split('|');
  const section=label.parentElement!;
  const advanced=document.createElement('details');
  advanced.innerHTML=`<summary>${t("web.connection_setup.text005")}</summary>`;
  section.insertBefore(advanced,label);advanced.append(label,help);
  advanced.insertAdjacentHTML('beforeend',`<p class="muted">${t("web.connection_setup.text004")}</p>`);
  advanced.insertAdjacentHTML('beforebegin',`${passwordField("settings-turn-password", `value="${esc(first?.[2] || '')}"`)}<button type="button" id="verify-turn">${t("web.connection_setup.text003")}</button><p id="turn-check-result" role="status"></p>`);
}

export function settingsServers(form:HTMLFormElement):string[] {
  const d=new FormData(form);
  const list=String(d.get('stun') || '').split(/\r?\n/).map(s=>s.trim()).filter(Boolean);
  const password=form.querySelector<HTMLInputElement>('[name="turn-password"]')!;
  if(password.value!==password.defaultValue || (!list.length && password.value)) {
    if(!password.value || /^[*•]+$/.test(password.value))throw Error(t("web.connection_setup.text002"));
    if(/[|\r\n]/.test(password.value))throw Error(t("web.connection_setup.text001"));
    const first=list[0]?.split('|') || [defaultServer,'preferans'];
    list[0]=`${first[0]}|${first[1] || 'preferans'}|${password.value}`;
  }
  return [...list,...String(d.get('stun-fallback') || '').split(/\s+/).filter(Boolean)];
}
