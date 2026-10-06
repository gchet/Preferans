import {t} from './i18n';
import './password-field.css';

export function passwordToggle(id: string): string {
  return `<button type="button" class="password-toggle" data-password-toggle="${id}" aria-controls="${id}" aria-label="${t("web.password_field.text001")}" title="${t("web.password_field.text001")}" aria-pressed="false"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/><path class="password-slash" d="m3 3 18 18"/></svg></button>`;
}

export function passwordField(id: string, inputAttributes: string): string {
  return `<div class="password-field"><label for="${id}">${t("web.password_field.text003")}</label><div class="password-input"><input id="${id}" name="turn-password" type="password" placeholder="${t("web.password_field.text004")}" autocomplete="new-password" spellcheck="false" ${inputAttributes}>${passwordToggle(id)}</div></div>`;
}

document.addEventListener('click', event => {
  const button=(event.target as Element).closest<HTMLButtonElement>('[data-password-toggle]');
  if(!button)return;
  const input=document.getElementById(button.dataset.passwordToggle!) as HTMLInputElement | null;
  if(!input)return;
  const visible=input.type==='password';
  const start=input.selectionStart, end=input.selectionEnd;
  input.type=visible?'text':'password';
  if(start!==null && end!==null)input.setSelectionRange(start,end);
  const label=visible?t("web.password_field.text002"):t("web.password_field.text001");
  button.setAttribute('aria-pressed',String(visible));
  button.setAttribute('aria-label',label);
  button.title=label;
});
