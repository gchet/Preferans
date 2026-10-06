import ru from '../../locales/ru.json' with {type: 'json'};
import uk from '../../locales/uk.json' with {type: 'json'};
import en from '../../locales/en.json' with {type: 'json'};

export type Language = 'ru' | 'uk' | 'en';
type Parameters = Record<string, unknown>;
const catalogs: Record<Language, Record<string, string>> = {ru, uk, en};
export function normalizeLanguage(value: unknown): Language {
  return value === 'ru' || value === 'en' ? value : 'uk';
}
export const language: Language = normalizeLanguage(typeof document === 'undefined' ? 'uk' : document.documentElement.dataset.language);
export const locale = {ru: 'ru-RU', uk: 'uk-UA', en: 'en-US'}[language];

// Values are inserted once. They must already be escaped when used in HTML,
// exactly as ordinary template-literal interpolations in the rendering code.
export function translate(lang: Language, key: string, values: Parameters = {}): string {
  const message = catalogs[lang]?.[key] || catalogs.ru[key] || key;
  return message.replace(/\{(p\d+)\}/g, (token, name: string) =>
    Object.prototype.hasOwnProperty.call(values, name) ? String(values[name]) : token);
}
export function t(key: string, values: Parameters = {}): string {
  return translate(language, key, values);
}

// Shared states and server errors remain language-neutral across participants.
// Translate display messages only, never names, room names, IDs or model output.
const messageMatchers = Object.entries(ru).filter(([key, value]) => key.startsWith('go.') && value.length > 3)
  .map(([key, value]) => {
    const slots: string[] = [];
    const pattern = value.split(/(\{p\d+\}|%(?:\[\d+\])?[-+# 0]*(?:\d+|\*)?(?:\.\d+)?[a-zA-Z%])/g)
      .map(part => {
        if (/^(?:\{p\d+\}|%[^%]*[a-zA-Z])$/.test(part)) { slots.push(part); return '([\\s\\S]*?)'; }
        return part.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      }).join('');
    return {key, slots, regex:new RegExp(`^${pattern}$`), weight:value.replace(/\{p\d+\}|%[^ ]+/g,'').length};
  }).sort((a,b)=>b.weight-a.weight);
export function localizeMessage(message: string, depth = 0, lang:Language=language): string {
  if(lang==='ru' || depth>3)return message;
  for(const {key, slots, regex} of messageMatchers) {
    const match=message.match(regex);
    if(!match)continue;
    let index=0;
    return (catalogs[lang][key] || catalogs.ru[key]).replace(/\{p\d+\}|%(?:\[\d+\])?[-+# 0]*(?:\d+|\*)?(?:\.\d+)?[a-zA-Z%]/g,token=>{
      if(token==='%%')return '%';
      const slot=token.startsWith('{') ? slots.indexOf(token) : index++;
      const value=match[slot+1] ?? token;
      return /%[wv]$/.test(token) ? localizeMessage(value,depth+1,lang) : value;
    });
  }
  return message;
}
export function localizeResponse<T>(value:T,lang:Language=language):T {
  function walk(item:unknown,field=''):unknown {
    if(typeof item==='string') {
      if(field==='label')return localizeMessage(item,0,lang)
        .replace(/^Мизер/,translate(lang,'web.main.text098'))
        .replace(/^Распасы/,translate(lang,'web.main.text099'))
        .replace(/ · Без трёх$/,translate(lang,'go.internal.game.engine.text019'))
        .replace(/ без прикупа/,translate(lang,'go.contract.no_talon_suffix'));
      if(['error','roomError','summary'].includes(field))return localizeMessage(item,0,lang);
      if(field==='logs')return item.replace(/^(.*?\s{2})(.*)$/s,(_,stamp,message)=>stamp+localizeMessage(message,0,lang));
      return item;
    }
    if(Array.isArray(item))return item.map(child=>walk(child,field));
    if(item && typeof item==='object') {
      const object=item as Record<string,unknown>;
      const result=Object.fromEntries(Object.entries(object).map(([key,child])=>[key,walk(child,key)]));
      if(object.id==='local-bots' && object.name==='Локальная')result.name=translate(lang,'web.local.room');
      if(object.bot===true && typeof object.name==='string') {
        result.name=object.name.replace(/^Бот (\d+)/,(_,number)=>translate(lang,'web.bot.name',{p0:number}));
      }
      return result;
    }
    return item;
  }
  return walk(value) as T;
}

if (typeof document !== 'undefined') {
  document.documentElement.lang = language;
	for(const selector of document.querySelectorAll<HTMLSelectElement>('[data-language-picker]')) {
    selector.value=language;
    selector.title={ru:'Русский',uk:'Українська',en:'English'}[language];
  }
  for (const element of document.querySelectorAll<HTMLElement>('[data-i18n]')) {
    element.textContent = t(element.dataset.i18n!);
  }
  // Translate the brand's own text node without adding a flex item or replacing
  // its suit icon and subtitle.
  for (const element of document.querySelectorAll<HTMLElement>('[data-i18n-own-text]')) {
    const node = [...element.childNodes].find(child => child.nodeType === Node.TEXT_NODE);
    if (node) node.textContent = ` ${t(element.dataset.i18nOwnText!)} `;
  }
  for (const attribute of ['title', 'aria-label']) {
    for (const element of document.querySelectorAll(`[data-i18n-${attribute}]`)) {
      element.setAttribute(attribute, t(element.getAttribute(`data-i18n-${attribute}`)!));
    }
  }
}
