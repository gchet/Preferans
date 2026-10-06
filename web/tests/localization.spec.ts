import {test, expect} from '@playwright/test';
import {translate} from '../src/i18n';
import ru from '../../locales/ru.json' with {type: 'json'};

test('translated languages differ and parameters are inserted only once',()=>{
  for(const language of ['ru','uk','en'] as const) {
    expect(translate(language,'web.main.text004')).toBe({ru:'Завершить партию',uk:'Завершити партію',en:'End game'}[language]);
    expect(translate(language,'web.main.text010',{p0:'Игрок {p1} $&'})).toBe({ru:'Стол уже создан Игрок {p1} $&',uk:'Стіл уже створено: Игрок {p1} $&',en:'Table already created by Игрок {p1} $&'}[language]);
    expect(translate(language,'missing.key')).toBe('missing.key');
  }
});

test('all Russian web messages resolve without leaking catalog keys or parameters',()=>{
  for(const [key,message] of Object.entries(ru)) {
    if(!key.startsWith('web.'))continue;
    const values=Object.fromEntries([...message.matchAll(/\{(p\d+)\}/g)].map(match=>[match[1],'VALUE']));
    expect(translate('ru',key,values)).toBe(message.replace(/\{p\d+\}/g,'VALUE'));
  }
});
