import {t} from './i18n';
import './ai-review.css';

type RPC=<T=unknown>(action:string,params?:Record<string,unknown>)=>Promise<T>;
interface Exchange {phase:string;systemRU:string;messages:{role:string;content:string}[];response:string;error?:string}
export interface AIReview {
  id:string;seat:number;bot:string;phase:string;language:string;summary:string;why:string;error?:string;
  model:{alias:string;model:string;maxTokens:number;temperature:number};
  elapsedMs:number;promptTokens:number;completionTokens:number;exchanges:Exchange[];
  chat?:{role:string;text:string}[];questionBusy:boolean;
}
const esc=(s:unknown)=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));

export class AIReviewUI {
  private root:HTMLElement|null=null;
  private review:AIReview|null=null;
  private asking=false;
  private chatKey='';
  constructor(private rpc:RPC,private changed:()=>void){}
  mount(root:HTMLElement,review:AIReview) {
    this.root=root;this.review=review;this.chatKey='';this.asking=false;
    root.innerHTML=`<p><strong>${esc(review.bot)} · ${esc(review.model.alias)}</strong></p><p>${t('web.ai.review-params',{p0:esc(review.model.model),p1:review.model.maxTokens,p2:review.model.temperature})}</p><p>${t('web.ai.review-counts',{p0:(review.elapsedMs/1000).toFixed(1),p1:review.promptTokens,p2:review.completionTokens})}</p>${review.error?`<p>${t('web.ai.review-fallback',{p0:esc(review.error)})}</p>`:''}<p class="ai-review-decision"><strong>${esc(review.summary)}</strong></p><p><strong>${t('web.ai.review-why')}:</strong> ${esc(review.why)||'—'}</p>${review.exchanges.map((x,i)=>`<section class="ai-exchange"><h3>${t('web.ai.review-attempt',{p0:i+1,p1:esc(x.phase)})}</h3><details><summary>${t('web.ai.review-prompt')}</summary><pre>${esc(JSON.stringify(x.messages,null,2))}</pre></details>${review.language==='en'?`<details><summary>${t('web.ai.review-russian')}</summary><pre>${esc(x.systemRU)}</pre></details>`:''}<details open><summary>${t('web.ai.review-response')}</summary><pre>${esc(x.response||x.error||t('web.ai.review-none'))}</pre></details></section>`).join('')}<section class="ai-review-chat"><h3>${t('web.ai.review-dialogue')}</h3><p class="muted">${t('web.ai.review-discussion-help')}</p><div data-review-chat></div><label>${t('web.ai.review-question')}<textarea data-review-question rows="3" maxlength="2000"></textarea></label><button type="button" data-review-ask>${t('web.ai.review-ask')}</button><p data-review-error role="status"></p></section><div class="ai-review-actions"><button type="button" class="primary" data-review-continue>${t('web.ai.review-continue')}</button></div>`;
    root.onclick=event=>{
      const button=(event.target as Element).closest<HTMLButtonElement>('button');
      if(!button)return;
      if(button.hasAttribute('data-review-continue')){
        button.disabled=true;
        void this.rpc('ai-review-continue',{id:review.id}).then(()=>this.changed()).catch(e=>this.error(root,e)).finally(()=>button.disabled=false);
      }
      if(button.hasAttribute('data-review-ask')){
        if(this.asking||this.review?.questionBusy)return;
        const input=root.querySelector<HTMLTextAreaElement>('[data-review-question]')!;
        if(!input.value.trim())return;
        this.asking=true;button.disabled=true;
        void this.rpc('ai-review-ask',{id:review.id,name:input.value}).then(()=>{input.value='';this.changed();}).catch(e=>this.error(root,e)).finally(()=>{this.asking=false;if(root.isConnected)button.disabled=false;});
      }
    };
    this.update(review);
  }
  private error(root:HTMLElement,error:unknown){if(root.isConnected)root.querySelector('[data-review-error]')!.textContent=String(error);}
  update(review:AIReview) {
    if(!this.root?.isConnected||this.review?.id!==review.id)return;
    this.review=review;
    const chatKey=JSON.stringify([review.chat,review.questionBusy]);
    if(this.chatKey!==chatKey){
      this.chatKey=chatKey;
      this.root.querySelector('[data-review-chat]')!.innerHTML=(review.chat||[]).map(m=>`<div><strong>${t(m.role==='user'?'web.ai.review-user':'web.ai.review-assistant')}</strong><p>${esc(m.text)}</p></div>`).join('')+(review.questionBusy?`<p>${t('web.ai.review-wait')}</p>`:'');
    }
    this.root.querySelector<HTMLButtonElement>('[data-review-ask]')!.disabled=this.asking||review.questionBusy;
  }
}
