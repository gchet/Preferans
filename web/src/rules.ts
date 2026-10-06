import {t} from './i18n';
export interface TableRules {
  finishAfterPassExit?: boolean;
  hereConvention?: string;
  passExitWhistedOnly?: boolean;
  passExitSpecial?: boolean;
  talonPenalty?: string;
  whist: string; responsibility: string; passPrices: number[]; passExit: number;
  stalingrad: boolean; tenCheck: boolean; misereTalon: boolean; allowNoTalon?: boolean; dealerBonus: boolean;
  endCondition: string; minutes: number;
}
export const defaultRules: TableRules = {
  finishAfterPassExit: true,
  hereConvention: 'through',
  passExitWhistedOnly: true,
  passExitSpecial: true,
  talonPenalty: 'whists',
  whist: 'gentleman', responsibility: 'half', passPrices: [2,4,8], passExit: 7,
  stalingrad: true, tenCheck: true, misereTalon: true, dealerBonus: true,
  endCondition: 'pool', minutes: 60,
};
export function rulesForm(r: TableRules, target: number, editable: boolean, plain = false): string {
  const select = (name: string, value: string, choices: string[][]) => plain ? `<strong>${choices.find(([id])=>id===value)?.[1] || value}</strong>` : `<select name="rule-${name}">${choices.map(([id,label])=>`<option value="${id}" ${id===value?'selected':''}>${label}</option>`).join('')}</select>`;
  const yesNo = (name: string, value: boolean, yes=t("web.rules.text035"), no=t("web.rules.text034")) => plain || yes !== t("web.rules.text035") || no !== t("web.rules.text034")
    ? select(name,String(value),[['true',yes],['false',no]])
    : `<input type="checkbox" name="rule-${name}" ${value?'checked':''}>`;
  const row = (label: string, field: string) => plain ? `<div class="rule-row"><span>${label}</span>${field}</div>` : `<label><span>${label}</span>${field}</label>`;
  const timed = r.endCondition==='time';
  const limitSelect = (name: string, value: number, values: number[], hidden: boolean) => `<select name="rule-${name}" ${hidden?'hidden':''}>${(!editable && !values.includes(value) ? [value,...values] : values).map(n=>`<option value="${n}" ${n===value?'selected':''}>${n}</option>`).join('')}</select>`;
  return `<fieldset class="table-rules ${plain?'rules-text':''}" ${editable?'':'disabled'}><legend>${t("web.main.text174")}</legend>
  ${row(t("web.rules.text007"),select('end',r.endCondition,[['pool',t("web.rules.text006")],['time',t("web.rules.text005")]]))}
  ${row(`<span data-rule-limit-label>${timed?t("web.main.text007"):t("web.main.text006")}</span>`,plain ? `<strong>${timed?r.minutes:target}</strong>` : limitSelect('target',target,[10,20,30,40,50],timed)+limitSelect('minutes',r.minutes,[30,45,60,90,120],!timed))}
  ${row(t("web.main.text237"),select('whist',r.whist,[['gentleman',t("web.rules.text009")],['greedy',t("web.rules.text008")]]))}
  ${row(t("web.rules.text012"),select('responsibility',r.responsibility,[['half',t("web.rules.text011")],['full',t("web.rules.text010")]]))}
  ${row(t('web.rules.here'),select('here',r.hereConvention || 'through',[['through',t('web.rules.here_through')],['seniority',t('web.rules.here_seniority')]]))}
  ${row(t("web.rules.text014"),plain ? `<strong>${r.passPrices.join('–')}–…</strong>` : `<input name="rule-prices" value="${r.passPrices.join(' ')}" required><small>${t("web.rules.text013")}</small>`)}
  ${row(t("web.rules.text018"),select('exit',String(r.passExit),[['6',t("web.rules.text017")],['7',t("web.rules.text016")],['8',t("web.rules.text015")]]))}
  ${row(t("web.rules.text019"),yesNo('exit-whisted',r.passExitWhistedOnly ?? true))}
  ${row(t("web.rules.text020"),yesNo('exit-special',r.passExitSpecial ?? true))}
  ${row(t('web.rules.finish_after_pass_exit'),yesNo('finish-after-pass-exit',r.finishAfterPassExit ?? true))}
  ${row(t("web.rules.text021"),yesNo('stalingrad',r.stalingrad))}
  ${row(t("web.rules.text024"),yesNo('ten',r.tenCheck,t("web.rules.text023"),t("web.rules.text022")))}
  ${row(t("web.rules.no_talon_bids"),yesNo('no-talon',r.allowNoTalon ?? !r.misereTalon))}
  ${row(t("web.rules.text027"),yesNo('bonus',r.dealerBonus))}
  ${row(t("web.rules.text030"),select('talon-penalty',r.talonPenalty || 'whists',[['whists',t("web.rules.text029")],['mountain',t("web.rules.text028")]]))}
  </fieldset><p class="muted">${t("web.rules.text033", {p15: editable?t("web.rules.text032"):t("web.rules.text031")})}</p>`;
}
export function readRules(d: FormData): TableRules {
  const prices=String(d.get('rule-prices')||'').trim().split(/[\s,;→-]+/).map(Number);
  if(!prices.length || prices.length>10 || prices.some((v,i)=>!Number.isInteger(v)||v<1||v>100||(i>0&&v<prices[i-1]))) throw Error(t("web.rules.text004"));
  return {finishAfterPassExit:d.has('rule-finish-after-pass-exit'),hereConvention:String(d.get("rule-here") || "through"),passExitWhistedOnly:d.has('rule-exit-whisted'),passExitSpecial:d.has('rule-exit-special'),talonPenalty:String(d.get('rule-talon-penalty') || 'whists'),whist:String(d.get('rule-whist')),responsibility:String(d.get('rule-responsibility')),passPrices:prices,passExit:Number(d.get('rule-exit')),stalingrad:d.has('rule-stalingrad'),tenCheck:d.get('rule-ten')==='true',misereTalon:true,allowNoTalon:d.has('rule-no-talon'),dealerBonus:d.has('rule-bonus'),endCondition:String(d.get('rule-end')),minutes:Number(d.get('rule-minutes'))};
}
export function rulesSummary(r: TableRules, target: number): string {
  return `${t("web.rules.text003", {p0: r.endCondition==='time'?`${t("web.rules.text002", {p0: r.minutes})}`:`${t("web.rules.text001", {p0: target})}`, p1: r.passPrices.join('–'), p2: r.passExit})}`;
}
