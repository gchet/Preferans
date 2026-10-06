import {test, expect} from '@playwright/test';
import {defaultRules, readRules, rulesForm} from '../src/rules';

test('Here convention and ten whisting round-trip through rule settings',()=>{
  expect(defaultRules.hereConvention).toBe('through');
  const html=rulesForm(defaultRules,30,true);
  expect(html).toContain('Конвенція «Тут»');
  expect(html).toContain('Через пасувальника (або здавача)');
  expect(html).toContain('За старшинством від здавача');
  expect(html).toContain('<option value="through" selected>');
  expect(html).toContain('Перевіряється розіграшем');
  expect(html).toContain('Вістується');
  expect(html).not.toContain('Зараховується без розіграшу');
  for(const here of ['through','seniority']) {
    const data=new FormData();
    for(const [key,value] of Object.entries({here,ten:'false',prices:'2 4 8',end:'pool',minutes:'60',whist:'gentleman',responsibility:'half',exit:'7'})) data.set('rule-'+key,value);
    const rules=readRules(data);
    expect(rules.hereConvention).toBe(here);
    expect(rules.tenCheck).toBe(false);
    const summary=rulesForm(rules,30,false,true);
    expect(summary).not.toContain('<select');
    expect(summary).toContain(here==='through'?'Через пасувальника (або здавача)':'За старшинством від здавача');
    expect(summary).toContain('Вістується');
  }
});
