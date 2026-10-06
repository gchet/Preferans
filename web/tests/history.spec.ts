import {test, expect} from '@playwright/test';
import {historyTotals, type HistoryEntry} from '../src/history';
import {defaultRules, readRules, rulesForm} from '../src/rules';

test('pass-exit finish rule defaults on and reads an explicit disabled setting', () => {
  const form = new FormData();
  form.set('rule-prices','2 4 8');
  expect(defaultRules.finishAfterPassExit).toBe(true);
  form.set('rule-finish-after-pass-exit','on');
  expect(readRules(form).finishAfterPassExit).toBe(true);
  form.delete('rule-finish-after-pass-exit');
  expect(readRules(form).finishAfterPassExit).toBe(false);
  expect(rulesForm(defaultRules,30,true)).toContain('name="rule-finish-after-pass-exit"');
  expect(rulesForm(defaultRules,30,false,true)).toContain('Завершувати пулю лише після виходу з розпасів');
});

test('room totals round each party and retain identities across names and seat order', () => {
  const entries: HistoryEntry[] = [
    {id:'old', names:['Old name','B','C'], playerIDs:['a','b','c'], results:[4,-2,-2], round:1, startedAt:1, finishedAt:2},
    {id:'new', names:['B','New name','C','D'], playerIDs:['b','a','c','d'], results:[-4,12,-4,-4], round:2, startedAt:3, finishedAt:4},
  ];
  expect(historyTotals(entries)).toEqual([
    {name:'New name',whists:5,parties:2,bot:false},
    {name:'D',whists:-1,parties:1,bot:false},
    {name:'B',whists:-2,parties:2,bot:false},
    {name:'C',whists:-2,parties:2,bot:false},
  ].sort((a,b)=>b.whists-a.whists || a.name.localeCompare(b.name)));
  expect(historyTotals(entries).reduce((sum,p)=>sum+p.whists,0)).toBe(0);
});

test('legacy history uses names and separates bots from human names', () => {
  const base = {id:'x',round:1,startedAt:1,finishedAt:2,names:['A','B','C'],results:[30,-15,-15]};
  const totals = historyTotals([base,{...base,id:'y',bots:[true,false,false]}]);
  expect(totals).toHaveLength(4);
  expect(totals.filter(p=>p.name==='A')).toHaveLength(2);
  expect(totals.find(p=>p.name==='B')?.whists).toBe(-10);
});

test('room totals combine Lena from different tablets and legacy saves', () => {
  const base = {id:'first',round:1,startedAt:1,finishedAt:2,names:['Лена','Б','В'],playerIDs:['tablet-1','b','c'],results:[30,-15,-15]};
  const totals = historyTotals([
    base,
    {...base,id:'second',finishedAt:4,names:[' Лена ','Б','В'],playerIDs:['tablet-2','b','c'],results:[-6,3,3]},
    {...base,id:'legacy',finishedAt:3,playerIDs:undefined,results:[3,0,-3]},
  ]);
  expect(totals).toHaveLength(3);
  expect(totals.find(p=>p.name==='Лена')).toEqual({name:'Лена',whists:9,parties:3,bot:false});
  expect(totals.reduce((sum,p)=>sum+p.whists,0)).toBe(0);
});
