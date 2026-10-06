import {test,expect} from '@playwright/test';
import {translate,localizeMessage,localizeResponse} from '../src/i18n';
import ru from '../../locales/ru.json' with {type:'json'};
import uk from '../../locales/uk.json' with {type:'json'};
import en from '../../locales/en.json' with {type:'json'};

test('localization: complete catalogs preserve placeholders',()=>{
  for(const catalog of [uk,en]) {
    expect(Object.keys(catalog).sort()).toEqual(Object.keys(ru).sort());
    for(const [key,message] of Object.entries(ru)) {
      const target=(catalog as Record<string,string>)[key];
      expect(target.trim(),key).not.toBe('');
      expect((target.match(/\{p\d+\}/g)||[]).sort(),key).toEqual((message.match(/\{p\d+\}/g)||[]).sort());
    }
  }
  expect(translate('uk','web.table.room',{p0:'Север'})).toBe('у кімнаті Север');
  expect(translate('en','web.table.room',{p0:'Север'})).toBe('in room Север');
});

test('localization: server messages do not translate names, secrets or model output',()=>{
  expect(localizeMessage('Стол уже создан Мизер',0,'en')).toBe('Table already created by Мизер');
  expect(localizeMessage('Распасы · 8',0,'en')).toBe('All-pass · 8');
  expect(localizeMessage('Unknown server error for user Мизер',0,'en')).toBe('Unknown server error for user Мизер');
  expect(localizeResponse({logs:['12:34:56.000  место 2: участник подтверждён, канал готов']},'en').logs[0]).toBe('12:34:56.000  seat 2: participant confirmed, channel ready');
  expect(localizeMessage('Не удалось сохранить ход: Партия на паузе',0,'en')).toBe('Could not save the move: Game is paused');
  const source={room:{id:'custom',name:'Распасы'},players:[{bot:true,name:'Бот 1 qwen'},{bot:false,name:'Бот 2'}],error:'Сейчас ход другого игрока',model:{alias:'Мизер',why:'Партия на паузе'},stun:['turn:server|user|пароль'],label:'Мизер · Без трёх'};
  const result=localizeResponse(source,'en');
  expect(result.room.name).toBe('Распасы');
  expect(result.players.map(p=>p.name)).toEqual(['Bot 1 qwen','Бот 2']);
  expect(result.error).toBe("It is another player's turn");
  expect(result.model).toEqual(source.model);
  expect(result.stun).toEqual(source.stun);
  expect(source.players[0].name).toBe('Бот 1 qwen');
  expect(result.label).toBe('Misère · Three undertricks');
});
