import {networkConfig} from './network-config';
import {t} from './i18n';
import {passwordToggle} from './password-field';
import {confirmDialog} from './confirm-dialog';
import './ai-bots.css';

type RPC=<T=unknown>(action:string,params?:Record<string,unknown>)=>Promise<T>;
interface Model {id:string;alias:string;provider:string;url:string;model:string;keyID:string;maxTokens:number;temperature:number}
interface Key {id:string;name:string;hasValue:boolean}
interface Settings {reviewResponses?:boolean;promptLanguage?:string;models:Model[];keys:Key[];timeoutSeconds:number}
interface Bot {name:string;bot:boolean;botModel?:string}
const esc=(value:unknown)=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));
export class AIBotsUI {
  private root:HTMLElement|null=null;
  private settings:Settings={models:[],keys:[],timeoutSeconds:10};
  private selected='';
  private working=false;
  private debugEnabled=false;
  constructor(private rpc:RPC,private changed:()=>void){}
  async mount(root:HTMLElement,updateURL:string,debugEnabled=false) {
    this.debugEnabled=debugEnabled;this.root=root;root.textContent=t('web.ai.loading');
    try {this.settings=await this.rpc<Settings>('ai-settings');if(this.root!==root||!root.isConnected)return;this.selected=this.settings.models[0]?.id||'';this.paint(updateURL);}catch(error){root.textContent=String(error);}
  }
  setDebug(enabled:boolean){this.debugEnabled=enabled;const input=this.root?.querySelector<HTMLInputElement>("[data-ai-review]");if(input)input.disabled=!enabled;}
  private input(name:string,label:string,value:unknown,type='text',attrs='') {return `<label>${t(`web.ai.${label}`)}<input data-ai-field="${name}" type="${type}" value="${esc(value)}" ${attrs}></label>`;}
  private paint(updateURL:string) {
    const root=this.root!; const model=this.settings.models.find(m=>m.id===this.selected);
    root.innerHTML=`<label>${t("web.ai.prompt-language")}<select data-ai-language><option value="ru" ${this.settings.promptLanguage!=="en"?"selected":""}>${t("web.ai.language-ru")}</option><option value="en" ${this.settings.promptLanguage==="en"?"selected":""}>${t("web.ai.language-en")}</option></select></label><label class="check"><input type="checkbox" data-ai-review ${this.settings.reviewResponses?"checked":""} ${this.debugEnabled?"":"disabled"}>${t("web.ai.review-setting")}</label><p class="muted">${t("web.ai.review-help")}</p><h3>${t('web.ai.title')}</h3><p>${t('web.ai.help')}</p>${this.input('timeout','timeout',this.settings.timeoutSeconds,'number','min="1" max="120" step="1"')}<div class="ai-model-layout"><div class="ai-model-list"><select size="5" data-ai-model-list aria-label="${t('web.ai.models')}">${this.settings.models.map(m=>`<option value="${esc(m.id)}" ${m.id===this.selected?'selected':''}>${esc(m.alias)}</option>`).join('')}</select><div class="toolbar"><button type="button" data-ai="add">${t('web.ai.add')}</button><button type="button" data-ai="delete" ${!model?'disabled':''}>${t('web.ai.delete')}</button></div></div><div class="ai-model-editor">${model?`${this.input('alias','alias',model.alias,'text','maxlength="40"')}${this.input('provider','provider',model.provider)}${this.input('url','url',model.url,'url')}${this.input('model','model',model.model)}<label>${t('web.ai.key')}<select data-ai-field="keyID"><option value="">${t('web.ai.no-key')}</option>${this.settings.keys.map(k=>`<option value="${esc(k.id)}" ${k.id===model.keyID?'selected':''}>${esc(k.name)}</option>`).join('')}</select></label><div class="ai-parameters">${this.input('maxTokens','tokens',model.maxTokens,'number','min="1" max="131072" step="1"')}${this.input('temperature','temperature',model.temperature,'number','min="0" max="2" step="0.05"')}</div>`:`<p>${t('web.ai.empty')}</p>`}</div></div><details class="ai-keys"><summary>${t('web.ai.keys')}</summary><p>${t('web.ai.key-help')}</p><div class="ai-key-list">${this.settings.keys.map(k=>`<div><span>${esc(k.name)}</span><button type="button" data-ai-key-edit="${esc(k.id)}">${t('web.ai.edit')}</button><button type="button" data-ai-key-delete="${esc(k.id)}">${t('web.ai.delete')}</button></div>`).join('')}</div><input type="hidden" data-ai-key-id><label>${t('web.ai.key-name')}<input data-ai-key-name maxlength="40"></label><label for="ai-key-value">${t('web.ai.key-value')}</label><div class="password-input"><input id="ai-key-value" type="password" autocomplete="new-password" spellcheck="false" placeholder="${t('web.ai.key-placeholder')}">${passwordToggle('ai-key-value')}</div><div class="toolbar"><button type="button" data-ai="key-save">${t('web.ai.key-save')}</button><button type="button" data-ai="key-new">${t('web.ai.key-new')}</button></div></details><details class="ai-transfer"><summary>${t('web.ai.transfer')}</summary><p>${t('web.ai.transfer-help')}</p><button type="button" data-ai="export">${t('web.ai.transfer-create')}</button><p data-ai-code></p><label>${t('web.ai.transfer-address')}<input data-ai-transfer-url value="${esc(updateURL)}" type="url" placeholder="${esc(networkConfig.updateAddressHint)}"></label><label>${t('web.ai.transfer-code')}<input data-ai-transfer-code type="text" inputmode="numeric" pattern="[0-9]{6}" maxlength="6" autocomplete="off" spellcheck="false"></label><button type="button" data-ai="import">${t('web.ai.transfer-import')}</button></details><p data-ai-status role="status"></p>`;
    // Replace the delegated listener when repainting this section.
    root.removeEventListener('click',this.click);root.addEventListener('click',this.click);
    root.onchange=event=>{const el=event.target as HTMLSelectElement;if(el.matches('[data-ai-model-list]')){this.capture();this.selected=el.value;this.paint(updateURL);}};
    root.dataset.updateUrl=updateURL;
  }
  private capture() {
    const root=this.root;if(!root)return;
    const field=(name:string)=>root.querySelector<HTMLInputElement|HTMLSelectElement>(`[data-ai-field="${name}"]`)?.value;
    this.settings.reviewResponses=!!root.querySelector<HTMLInputElement>("[data-ai-review]")?.checked;
    this.settings.promptLanguage=root.querySelector<HTMLSelectElement>("[data-ai-language]")?.value||"ru";
    this.settings.timeoutSeconds=Number(field('timeout'));
    const model=this.settings.models.find(m=>m.id===this.selected);if(!model)return;
    model.alias=field('alias')?.trim()||'';model.provider=field('provider')?.trim()||'';model.url=field('url')?.trim()||'';model.model=field('model')?.trim()||'';model.keyID=field('keyID')||'';model.maxTokens=Number(field('maxTokens'));model.temperature=Number(field('temperature'));
  }
  async save(){if(!this.root?.isConnected)return;if(this.working||!this.root.querySelector('[data-ai-field="timeout"]'))throw Error(t('web.ai.loading'));this.capture();await this.rpc('ai-save',{ai:this.settings});}
  private click=(event:Event)=>{
    const button=(event.target as Element).closest<HTMLButtonElement>('button');if(!button||!this.root?.contains(button)||!button.matches('[data-ai],[data-ai-key-edit],[data-ai-key-delete]'))return;
    void this.perform(async()=>{
      const root=this.root!;this.capture();const action=button.dataset.ai;
      if(action==='add'){const id=crypto.randomUUID();this.settings.models.push({id,alias:'',provider:'Groq',url:'https://api.groq.com/openai/v1/chat/completions',model:'',keyID:'',maxTokens:500,temperature:0});this.selected=id;this.paint(root.dataset.updateUrl||'');}
      if(action==='delete'&&await confirmDialog(t('web.ai.delete-model-warning'),t('web.ai.delete'))){this.settings.models=this.settings.models.filter(m=>m.id!==this.selected);this.selected=this.settings.models[0]?.id||'';this.paint(root.dataset.updateUrl||'');}
      if(action==='key-new'){root.querySelector<HTMLInputElement>('[data-ai-key-id]')!.value='';root.querySelector<HTMLInputElement>('[data-ai-key-name]')!.value='';root.querySelector<HTMLInputElement>('#ai-key-value')!.value='';}
      if(button.dataset.aiKeyEdit){const key=this.settings.keys.find(k=>k.id===button.dataset.aiKeyEdit)!;root.querySelector<HTMLInputElement>('[data-ai-key-id]')!.value=key.id;root.querySelector<HTMLInputElement>('[data-ai-key-name]')!.value=key.name;root.querySelector<HTMLInputElement>('#ai-key-value')!.value='';}
      if(button.dataset.aiKeyDelete&&await confirmDialog(t('web.ai.delete-key-warning'),t('web.ai.delete'))){await this.rpc('ai-key-delete',{id:button.dataset.aiKeyDelete});this.settings.keys=(await this.rpc<Settings>('ai-settings')).keys;this.paint(root.dataset.updateUrl||'');}
      if(action==='key-save'){
        const value=root.querySelector<HTMLInputElement>('#ai-key-value')!;
        const id=await this.rpc<string>('ai-key-save',{id:root.querySelector<HTMLInputElement>('[data-ai-key-id]')!.value,name:root.querySelector<HTMLInputElement>('[data-ai-key-name]')!.value,secret:value.value});value.value='';
        this.settings.keys=(await this.rpc<Settings>('ai-settings')).keys;const m=this.settings.models.find(m=>m.id===this.selected);if(m&&!m.keyID)m.keyID=id;this.paint(root.dataset.updateUrl||'');
      }
      if(action==='export'){await this.rpc('ai-save',{ai:this.settings});const code=await this.rpc<string>('ai-transfer-create');root.querySelector('[data-ai-code]')!.textContent=t('web.ai.code-ready',{p0:code});}
      if(action==='import'&&await confirmDialog(t('web.ai.import-warning'),t('web.ai.transfer-import'))){await this.rpc('ai-transfer-import',{name:root.querySelector<HTMLInputElement>('[data-ai-transfer-url]')!.value,code:root.querySelector<HTMLInputElement>('[data-ai-transfer-code]')!.value});this.settings=await this.rpc<Settings>('ai-settings');this.selected=this.settings.models[0]?.id||'';this.paint(root.dataset.updateUrl||'');this.root!.querySelector('[data-ai-status]')!.textContent=t('web.ai.imported');}
    });
  };
  private async perform(action:()=>Promise<void>) {
    if(this.working)return;this.working=true;
    try{await action();this.changed();}catch(error){this.root?.querySelector('[data-ai-status]')?.replaceChildren(document.createTextNode(error instanceof Error?error.message:String(error)));}
    finally{this.working=false;}
  }
  async assignments(root:HTMLElement,players:Bot[],selectedSeat?:number) {
    const settings=await this.rpc<Settings>('ai-settings');if(!root.isConnected)return;
    root.innerHTML=`<p>${t('web.ai.assignment-help')}</p>${players.map((p,seat)=>p.bot && (selectedSeat===undefined || seat===selectedSeat)?`<label>${esc(p.name)}<select data-bot-seat="${seat}"><option value="">${t('web.ai.local')}</option>${settings.models.map(m=>`<option value="${esc(m.id)}" ${m.id===p.botModel?'selected':''}>${esc(m.alias)}</option>`).join('')}</select></label>`:'').join('')}<p data-bot-status role="status"></p>`;
    root.onchange=event=>{const select=event.target as HTMLSelectElement;if(select.dataset.botSeat===undefined)return;select.disabled=true;void this.rpc('ai-assign',{seat:Number(select.dataset.botSeat),modelID:select.value}).then(()=>this.changed()).catch(error=>root.querySelector('[data-bot-status]')!.textContent=String(error)).finally(()=>select.disabled=false);};
  }
}
