import {t} from './i18n';
import {passwordToggle} from "./password-field";
import {AdminUI} from './admin';
import './admin-page.css';

const app=document.querySelector<HTMLElement>('#admin-app')!;
let csrf='';
const esc=(text:string)=>text.replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));
const errorText=(code:string)=>{const key=`web.admin.auth_${code}`,value=t(key);return value===key?code:value;};
async function api<T>(path:string,body?:unknown):Promise<T> {
  const response=await fetch(`/admin/api/${path}`,{method:body===undefined?'GET':'POST',credentials:'same-origin',headers:body===undefined?{}:{'Content-Type':'application/json','X-Admin-CSRF':csrf},body:body===undefined?undefined:JSON.stringify(body)});
  const value=await response.json();
  if(!response.ok){
    if(response.status===401 && path!=='login')showLogin(t('web.admin.auth_login_required'));
    throw Error(errorText(value.error || String(response.status)));
  }
  return value as T;
}
const ui=new AdminUI(async<T>(action:string,params?:Record<string,unknown>)=>api<T>('rooms',{op:action==='admin-rooms'?'list':action,room:params?.id,elementID:params?.elementID}));

function addPasswordToggles(form: HTMLFormElement) {
  form.querySelectorAll<HTMLInputElement>("input[type=password]").forEach(input=>{
    input.id=`${form.id}-${input.name}`;
    const wrapper=document.createElement("span");wrapper.className="password-input";
    input.before(wrapper);wrapper.append(input);wrapper.insertAdjacentHTML("beforeend",passwordToggle(input.id));
  });
}
function showLogin(message='') {
  csrf='';
  app.innerHTML=`<section class="admin-login"><h1>${t('web.admin.tab')}</h1><form id="admin-login-form"><label>${t('web.admin.password')}<input name="password" type="password" autocomplete="current-password" required></label><button type="submit">${t('web.admin.login')}</button></form><p id="auth-status" role="status">${esc(message)}</p></section>`;
  const form=app.querySelector<HTMLFormElement>('form')!;
  addPasswordToggles(form);
  form.addEventListener('submit',async event=>{
    event.preventDefault();const button=form.querySelector<HTMLButtonElement>('button[type=submit]')!;button.disabled=true;
    try {
      const data=new FormData(form);
      const session=await api<{csrf:string;initial:boolean}>('login',{password:data.get('password')});
      form.reset();showAdmin(session);
    }catch(error){app.querySelector('#auth-status')!.textContent=String(error instanceof Error?error.message:error);}
    finally{button.disabled=false;}
  });
}
function showAdmin(session:{csrf:string;initial:boolean}) {
  csrf=session.csrf;
  app.innerHTML=`<header class="admin-page-header"><h1>${t('web.admin.tab')}</h1><button type="button" id="admin-logout">${t('web.admin.logout')}</button></header>${session.initial?`<p class="admin-warning">${t('web.admin.initial_password')}</p>`:''}<details class="admin-password-change" ${session.initial?'open':''}><summary>${t('web.admin.change_password')}</summary><form id="admin-password-form"><label>${t('web.admin.current_password')}<input name="current" type="password" autocomplete="current-password" required></label><label>${t('web.admin.new_password')}<input name="password" type="password" autocomplete="new-password" minlength="8" required></label><label>${t('web.admin.confirm_password')}<input name="confirm" type="password" autocomplete="new-password" minlength="8" required></label><button type="submit">${t('web.admin.change_password')}</button><p id="password-status" role="status"></p></form></details><section id="admin-directory"></section>`;
  ui.mount(app.querySelector('#admin-directory')!);
  app.querySelector('#admin-logout')!.addEventListener('click',async()=>{
    try{await api('logout',{});showLogin();}catch(error){showLogin(String(error));}
  });
  const form=app.querySelector<HTMLFormElement>('#admin-password-form')!;
  addPasswordToggles(form);
  form.addEventListener('submit',async event=>{
    event.preventDefault();const data=new FormData(form),button=form.querySelector<HTMLButtonElement>('button[type=submit]')!;
    if(data.get('password')!==data.get('confirm')){form.querySelector('#password-status')!.textContent=t('web.admin.password_mismatch');return;}
    button.disabled=true;
    try{await api('password',{current:data.get('current'),password:data.get('password')});form.reset();showLogin(t('web.admin.password_changed'));}
    catch(error){const status=form.querySelector('#password-status');if(status)status.textContent=String(error instanceof Error?error.message:error);}
    finally{button.disabled=false;}
  });
}
void api<{csrf:string;initial:boolean}>('session').then(showAdmin).catch(error=>{
  if(error instanceof Error && error.message===errorText('https_required'))app.innerHTML=`<section class="admin-login"><h1>${t('web.admin.tab')}</h1><p>${t('web.admin.auth_https_required')}</p></section>`;
  else showLogin();
});
