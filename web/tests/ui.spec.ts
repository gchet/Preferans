import { test, expect, type Page, type Locator } from "@playwright/test";
import { spawn, ChildProcess } from "node:child_process";
import { resolve } from "node:path";
import { readFile } from 'node:fs/promises';
import { readFileSync } from 'node:fs';
let processHandle: ChildProcess;
let url: string;

test('rooms: local language selection persists across screens without changing the game',async({page})=>{
  await page.setViewportSize({width:1024,height:600});
  await page.goto(url);
  const picker=page.locator('.topbar [data-language-picker]');
  await expect(picker).toHaveValue('uk');
  await expect(picker.locator('option[value="uk"]')).toHaveText('UK');
  await expect(page.locator('html')).toHaveAttribute('lang','uk');
  await expect(picker).toHaveValue('uk');
  await page.getByRole('button',{name:'Грати локально з ботами',exact:true}).click();
  await expect(page.locator('#room-roster')).toContainText('Локальна');
  await page.locator('#create-form [name="name"]').fill('Гена');
  await page.getByRole('button',{name:'Налаштування',exact:true}).click();
  await expect(page.getByRole('tab',{name:'Карти',exact:true})).toBeVisible();
  await expect(page.getByRole('tab',{name:'Підключення',exact:true})).toBeVisible();
  await page.locator('#panel [data-language-picker]').selectOption('en');
  await expect(page.locator('html')).toHaveAttribute('lang','en');
  await expect(page.getByRole('button',{name:'Create table →',exact:true})).toBeVisible();
  await page.getByRole('button',{name:'Settings',exact:true}).click();
  await expect(page.getByRole('tab',{name:'Cards',exact:true})).toBeVisible();
  await page.locator('#settings-form > button.primary').click();
  await page.reload();
  await expect(picker).toHaveValue('en');
  await page.locator('#create-form [name="name"]').fill('Гена');
  await page.locator('#create-form [name="players"]').selectOption('4');
  await page.getByRole('button',{name:'Create table →',exact:true}).click();
  await expect(page.locator('.playing-field')).toContainText('Bot 2');
  await page.getByRole('button',{name:'Ready',exact:true}).click();
  await page.getByRole('button',{name:'Start game',exact:true}).click();
  await expect(page.locator('.hand .card')).toHaveCount(10);
  const response=await page.request.post(new URL('/api',url).href,{headers:{'X-Preferans-Token':new URL(url).hash.slice(1)},data:{action:'status'}});
  const id=(await response.json()).data.view.id;
  await picker.selectOption('uk');
  await expect(page.locator('.hand .card')).toHaveCount(10);
  await expect(page.locator('.playing-field')).toContainText('Гена');
  const after=await page.request.post(new URL('/api',url).href,{headers:{'X-Preferans-Token':new URL(url).hash.slice(1)},data:{action:'status'}});
  expect((await after.json()).data.view.id).toBe(id);
  for(const size of [{width:1024,height:600},{width:848,height:360}]) {
    await page.setViewportSize(size);
    for(const lang of ['en','uk','ru']) {
      await picker.selectOption(lang);
      await expect(page.locator('html')).toHaveAttribute('lang',lang);
      await expect(page.locator('.hand .card')).toHaveCount(10);
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
      const visible=await picker.evaluate(el=>{const r=el.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth&&r.bottom<=innerHeight;});
      expect(visible).toBeTruthy();
      await page.screenshot({path:`../.build/language-${lang}-${size.width}.png`});
    }
  }
  await picker.selectOption('en');
  await expect(page.locator('html')).toHaveAttribute('lang','en');
  await page.locator('#finish-party-topbar').click();
  await expect(page.locator('#create-form')).toBeVisible();
  await page.locator('#create-form [name="players"]').selectOption('3');
  await page.getByRole('button',{name:'Create table →',exact:true}).click();
  await page.getByRole('button',{name:'Ready',exact:true}).click();
  await page.getByRole('button',{name:'Start game',exact:true}).click();
  await expect(page.locator('.hand .card')).toHaveCount(10);
  await page.getByRole('button',{name:'Rules',exact:true}).click();
  await expect(page.locator('#panel')).toContainText('Table agreements');
  await expect(page.locator('#panel')).toContainText('“Here” convention');
  await expect(page.locator('#panel [data-language-picker]')).toHaveValue('en');
});

test.describe('local offline interface',()=>{
  test.use({hasTouch:true,isMobile:true});
  test('rooms: local bots work without TURN and leave by tapping in both orientations',async({page})=>{
    await page.goto(url);
    await page.getByRole('button',{name:'Играть локально с ботами',exact:true}).tap();
    await expect(page.locator('#room-roster')).toContainText('Локальная');
    await expect(page.locator('#room-roster .voice-mic')).toHaveCount(0);
    const bots=page.locator('#create-form [name="bots"]');
    await expect(bots).toBeChecked();await expect(bots).toBeDisabled();
    await page.getByRole('button',{name:'Настройки',exact:true}).tap();
    await page.getByLabel('Только игра с ботами (без интернета)',{exact:true}).check();
    await page.locator('#settings-form > button.primary').tap();
    await page.reload();
    await expect(page.locator('#create-form')).toBeVisible();
    const externalActions:string[]=[];
    page.on('request',request=>{
      if(!request.url().endsWith('/api'))return;
      const action=request.postDataJSON().action;
      if(action.startsWith('rooms-') || action.startsWith('ai-') || action==='voice-send')externalActions.push(action);
    });
    for(const size of [{width:412,height:915},{width:915,height:412}]){
      await page.setViewportSize(size);
      // Wait for both layout and the mobile visual viewport after rotation.
      await expect.poll(()=>page.evaluate(()=>({width:visualViewport?.width,height:visualViewport?.height}))).toEqual(size);
      await page.evaluate(()=>new Promise<void>(done=>requestAnimationFrame(()=>requestAnimationFrame(()=>done()))));
      await page.getByRole('button',{name:'Создать стол →'}).tap();
      await expect(page.locator('.center.lobby')).toBeVisible();
      await page.getByRole('button',{name:'Готов',exact:true}).tap();
      await page.getByRole('button',{name:'Начать партию',exact:true}).tap();
      await expect(page.locator('.hand .card')).toHaveCount(10);
      await page.locator('#finish-party-topbar').tap();
      await expect(page.locator('#create-form')).toBeVisible();
      await expect(page.locator('#room-roster')).toContainText('Локальная');
    }
    expect(externalActions).toEqual([]);
    await page.getByRole('button',{name:'Настройки',exact:true}).tap();
    await expect(page.getByLabel('Только игра с ботами (без интернета)',{exact:true})).toBeChecked();
    await page.getByLabel('Только игра с ботами (без интернета)',{exact:true}).uncheck();
    await page.locator('#settings-form > button.primary').tap();
    await expect(page.getByRole('button',{name:'Играть локально с ботами',exact:true})).toBeVisible();
  });
});

test('debug discard marks two hand cards then opens the normal contract grid',async({page})=>{
  await page.goto(url);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.locator('[name="debug-auction"]').check();
  await page.locator('#settings-form > button.primary').click();
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await page.getByRole('button',{name:'Готов',exact:true}).click();
  await page.getByRole('button',{name:'Начать партию',exact:true}).click();
  await page.locator('[data-action="pass"]').click();
  await expect(page.locator('.player[data-debug-waiting="1"]')).toBeVisible();
  const rpc=async(action:string,params:Record<string,unknown>={})=>{
    const response=await page.request.post(new URL('/api',url).href,{headers:{'X-Preferans-Token':new URL(url).hash.slice(1)},data:{action,...params}});
    const body=await response.json();expect(body.error).toBeFalsy();return body.data;
  };
  let v=(await rpc('status')).view;
  await rpc('debug-bot-command',{command:{seat:1,revision:v.revision,action:'bid',contract:{level:6,suit:2,misere:false}}});
  v=(await rpc('status')).view;
  await rpc('debug-bot-command',{command:{seat:2,revision:v.revision,action:'pass'}});
  await expect(page.locator('.player[data-debug-waiting="1"] [aria-label="12 закрытых карт"]')).toBeVisible();
  await expect(page.locator('.player[data-debug-waiting="1"]')).toBeVisible();
  await page.locator('.player[data-debug-waiting="1"]').click({button:'right'});
  await expect(page.locator('.debug-turn-cards .card')).toHaveCount(12);
  await page.getByRole('button',{name:'Выбрать за бота',exact:true}).click();
  const cards=page.locator('[data-debug-card]');
  await expect(cards).toHaveCount(12);
  const bet=page.locator('[data-debug-bet]');
  await expect(bet).toBeDisabled();
  const selected=[Number(await cards.nth(0).getAttribute('data-debug-card')),Number(await cards.nth(1).getAttribute('data-debug-card'))];
  await cards.nth(0).locator('.card').click();
  await expect(bet).toBeDisabled();
  await cards.nth(1).locator('.card').click();
  await expect(page.locator('[data-debug-card][aria-pressed="true"]')).toHaveCount(2);
  await cards.nth(2).click();
  await expect(page.locator('[data-debug-card][aria-pressed="true"]')).toHaveCount(2);
  await cards.nth(1).click();
  await expect(bet).toBeDisabled();
  await cards.nth(1).click();
  await bet.click();
  await expect(page.locator('#debug-turn .contract-grid')).toBeVisible();
  await expect(page.locator('#debug-turn .contract-cell')).toHaveCount(25);
  await page.locator('#debug-turn').getByRole('button',{name:'6 бубен',exact:true}).click();
  await expect(page.locator('#panel')).not.toBeVisible();
  v=(await rpc('status')).view;
  expect(v.contract).toMatchObject({level:6,suit:2});
  expect(v.players[1].count).toBe(10);
  const after=await rpc('debug-bot-options',{seat:v.actor,command:{revision:v.revision}});
  expect(after.view.stage).toBe('defend');
  expect(selected).toHaveLength(2);
});

for(const gesture of ['right-click','long-press'])test(`debug bot turn accepts ${gesture} and host choice`,async({page})=>{
  await page.goto(url);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.locator('[name="debug-auction"]').check();
  await expect(page.locator('[name="debug-play"]')).not.toBeChecked();
  await page.locator('#settings-form > button.primary').click();
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await page.getByRole('button',{name:'Готов',exact:true}).click();
  await page.getByRole('button',{name:'Начать партию',exact:true}).click();
  await page.locator('[data-action="pass"]').click();
  const waiting=page.locator('.player[data-debug-waiting="1"]');
  await expect(waiting).toBeVisible();
  await page.waitForTimeout(600);
  await expect(waiting).toBeVisible();
  if(gesture==='right-click')await waiting.click({button:'right'});
  else {
    await waiting.dispatchEvent('pointerdown',{pointerType:'touch',pointerId:7,clientX:100,clientY:200});
    await expect(page.locator('#debug-turn')).toBeVisible();
    await waiting.dispatchEvent('pointerup',{pointerType:'touch',pointerId:7,clientX:100,clientY:200});
  }
  await expect(page.locator('#panel-title')).toContainText('Отладочный ход: Бот 1');
  await expect(page.locator('.debug-turn-cards .card')).toHaveCount(10);
  await page.getByRole('button',{name:'Выбрать за бота',exact:true}).click();
  if(gesture==='right-click') {
    await page.locator('#debug-turn').getByRole('button',{name:'Ставка',exact:true}).click();
    await expect(page.locator('#debug-turn .contract-cell')).toHaveCount(25);
    await expect(page.locator('#debug-turn').getByRole('button',{name:'Мизер',exact:true})).toBeVisible();
    await expect(page.locator('#debug-turn').getByRole('button',{name:'Пас',exact:true})).toBeVisible();
    await expect(page.locator('#debug-turn .contract-no-talon')).toHaveCount(0);
    await page.locator('#debug-turn').getByRole('button',{name:'6 бубен',exact:true}).click();
  } else await page.locator('#debug-turn').getByRole('button',{name:'Пас',exact:true}).click();
  await expect(page.locator('#panel')).not.toBeVisible();
  const next=page.locator('.player[data-debug-waiting="2"]');
  await expect(next).toBeVisible();
  await next.click({button:'right'});
  await expect(page.locator('#debug-turn')).toBeVisible();
  if(gesture==='right-click') {
    await page.getByRole('button',{name:'Выбрать за бота',exact:true}).click();
    await page.locator('#debug-turn').getByRole('button',{name:'Ставка',exact:true}).click();
    await expect(page.locator('#debug-turn').getByRole('button',{name:'6 пик',exact:true})).toBeDisabled();
  }
  await page.getByRole('button',{name:'Продолжить ход бота',exact:true}).click();
  await expect(page.locator('#panel')).not.toBeVisible();
});

test('all-pass price hint updates labels and persists when disabled',async({page})=>{
  await page.request.post(new URL('/api',url).href,{headers:{'X-Preferans-Token':new URL(url).hash.slice(1)},data:{action:'create',players:3,target:30,name:'Игрок',bots:true}});
  let price=2;
  await page.route('**/api',async route=>{
    if(route.request().postDataJSON().action!=='status')return route.fallback();
    const response=await route.fetch(),body=await response.json(),v=body.data.view;
    Object.assign(v,{stage:'play',revision:100+price,round:1,allPass:true,passPrice:price,actor:0,turn:0,hand:[],actions:[],taken:[0,0,0],trick:[],trickNo:0});
    await route.fulfill({response,json:body});
  });
  await page.goto(url);
  await expect(page.locator('.player-left .speech')).toHaveText('Распасы (по 2)');
  price=4;
  await expect(page.locator('.player-left .speech')).toHaveText('Распасы (по 4)');
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.getByLabel('Подсказка цены распасов',{exact:true}).uncheck();
  await page.locator('#settings-form > button.primary').click();
  await expect(page.locator('.player-left .speech')).toHaveText('Распасы');
  await page.reload();
  await expect(page.locator('.player-left .speech')).toHaveText('Распасы');
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await expect(page.getByLabel('Подсказка цены распасов',{exact:true})).not.toBeChecked();
});

test('dealer can inspect one defender and then only whist or pass',async({page})=>{
  await page.request.post(new URL('/api',url).href,{headers:{'X-Preferans-Token':new URL(url).hash.slice(1)},data:{action:'create',players:4,target:30,name:'Сдающий',bots:true}});
  let phase='dealer-choice';
  const commands:string[]=[];
  await page.route('**/api',async route=>{
    const query=route.request().postDataJSON();
    if(query.action==='command') {
      commands.push(query.command.action);
      phase=query.command.action==='dealer-first'?'dealer-whist':'play';
      return route.fulfill({json:{data:{}}});
    }
    if(query.action!=='status')return route.fallback();
    const response=await route.fetch(),body=await response.json(),v=body.data.view;
    Object.assign(v,{stage:phase,round:1,revision:100+commands.length,seat:0,actor:0,turn:phase==='play'?1:0,dealer:0,declarer:3,contract:{level:7,suit:1},hand:[],taken:[0,0,0,0],defence:[phase==='play'?2:0,1,1,0],trick:[],trickNo:1,open:phase==='play',dealerWhist:phase==='play',dealerLook:phase==='dealer-choice'?undefined:1,actions:phase==='dealer-choice'?['dealer-skip','dealer-first','dealer-second']:phase==='dealer-whist'?['whist','pass']:['play'],legal:[0],playHand:[0,1,2,3,4,5,6,7,8,9]});
    v.players.forEach((p:any,i:number)=>{p.name=['Сдающий','Первый','Второй','Играющий'][i];p.count=i===0?0:10;p.cards=i===1&&phase!=='dealer-choice'||i===2&&phase==='play'?[0,1,2,3,4,5,6,7,8,9]:[]});
    await route.fulfill({response,json:body});
  });
  await page.goto(url);
  await expect(page.getByRole('button',{name:'Нет',exact:true})).toBeVisible();
  await expect(page.getByRole('button',{name:'2-й вистующий · Второй',exact:true})).toBeVisible();
  await page.getByRole('button',{name:'1-й вистующий · Первый',exact:true}).click();
  await expect(page.locator('.player-left .exposed .card')).toHaveCount(10);
  await expect(page.locator('.player-top .exposed .card')).toHaveCount(0);
  await expect(page.getByRole('button',{name:'Вист',exact:true})).toBeVisible();
  await expect(page.getByRole('button',{name:'Пас',exact:true})).toBeVisible();
  await expect(page.locator('[data-action="half"], [data-action="dealer-second"]')).toHaveCount(0);
  await page.getByRole('button',{name:'Вист',exact:true}).click();
  await expect(page.locator('.player-left .exposed .card')).toHaveCount(10);
  await expect(page.locator('.player-top .exposed .card')).toHaveCount(10);
  await expect(page.locator('.player-left .exposed .card[data-card]')).toHaveCount(10);
  expect(commands).toEqual(['dealer-first','whist']);
});

for (const count of [3,4]) {
  test(`pool whist columns keep their owners for every viewer with ${count} seats`,async({page})=>{
    await page.request.post(new URL('/api',url).href,{headers:{'X-Preferans-Token':new URL(url).hash.slice(1)},data:{action:'create',players:count,target:30,name:'Player 0',bots:true}});
    let viewer=0;
    const whists=Array.from({length:count},(_,owner)=>Array.from({length:count},(_,opponent)=>owner===opponent?0:100*(owner+1)+opponent+1));
    await page.route('**/api',async route=>{
      if(route.request().postDataJSON().action!=='status')return route.fallback();
      const response=await route.fetch(),body=await response.json(),v=body.data.view;
      Object.assign(v,{stage:'play',round:1,revision:100+viewer,seat:viewer,actor:-1,turn:0,contract:{level:7,suit:3},hand:[],actions:[],taken:Array(count).fill(0),whists,results:Array(count).fill(0)});
      v.players.forEach((p:any,i:number)=>{p.name=`Player ${i}`});
      await route.fulfill({response,json:body});
    });
    await page.goto(url);
    for(viewer=0;viewer<count;viewer++) {
      await expect(page.locator('.pool-drawing > svg > g > .pool-name').first()).toHaveText(`Player ${viewer}`);
      const sectors=await page.locator('.pool-drawing > svg > g').evaluateAll(groups=>groups.map(g=>({
        owner:Number(g.querySelector('.pool-name')!.textContent!.replace('Player ','')),
        columns:[...g.querySelectorAll('.pool-whists')].map(text=>({value:Number(text.lastChild!.textContent),title:text.querySelector('title')!.textContent}))
      })));
      for(const sector of sectors) {
        const opponents=Array.from({length:count-1},(_,i)=>(sector.owner+i+1)%count);
        expect(sector.columns.map(column=>column.value),`viewer ${viewer}, owner ${sector.owner}`).toEqual(opponents.map(opponent=>whists[sector.owner][opponent]));
        sector.columns.forEach((column,index)=>{
          expect(column.title).toContain(`Player ${sector.owner}`);
          expect(column.title).toContain(`Player ${opponents[index]}`);
        });
      }
    }
  });
}

test('start button waits for unready players and becomes available when ready',async({page})=>{
  await page.request.post(new URL('/api',url).href,{headers:{'X-Preferans-Token':new URL(url).hash.slice(1)},data:{action:'create',players:3,target:30,name:'Гена',bots:true}});
  let ready=false,hostReady=true,starts=0;
  await page.route('**/api',async route=>{
    const query=route.request().postDataJSON();
    if(query.action==='command' && query.command.action==='start')starts++;
    if(query.action==='command' && query.command.action==='ready')hostReady=true;
    if(query.action!=='status')return route.fallback();
    const response=await route.fetch(),body=await response.json();
    body.data.view.revision=!hostReady?102:ready?101:100;
    body.data.view.players[0].ready=hostReady;
    body.data.view.players[1].ready=ready;
    body.data.view.players[1].name='Лена';
    await route.fulfill({response,json:body});
  });
  await page.goto(url);
  await page.getByRole('button',{name:'Начать партию',exact:true}).click();
  const waiting=page.getByRole('button',{name:'Ожидание готовности…',exact:true});
  await expect(waiting).toBeDisabled();
  await expect(waiting).toHaveAttribute('aria-busy','true');
  await expect(waiting).toHaveAttribute('title','Ещё не готовы: Лена');
  await expect(waiting.locator('.connection-spinner')).toBeVisible();
  await expect(page.locator('[data-action="ready"]')).not.toHaveClass(/ready-attention/);
  expect(starts).toBe(0);
  ready=true;
  await expect(page.getByRole('button',{name:'Начать партию',exact:true})).toBeEnabled();
  expect(starts).toBe(0);
  hostReady=false;
  await expect(page.getByRole('button',{name:'Готов',exact:true})).toBeVisible();
  await page.getByRole('button',{name:'Начать партию',exact:true}).click();
  const hostButton=page.getByRole('button',{name:'Готов',exact:true});
  await expect(hostButton).toHaveClass(/ready-attention/);
  expect(await hostButton.evaluate(el=>getComputedStyle(el).animationName)).toBe('ready-attention');
  await hostButton.click();
  await expect(page.locator('[data-action="ready"]')).not.toHaveClass(/ready-attention/);
  await expect(page.getByRole('button',{name:'Начать партию',exact:true})).toBeEnabled();
  expect(starts).toBe(0);
});

for (const seats of [3,4]) {
  test(`phone landscape keeps ${seats} player seats inside viewport`, async ({page}) => {
    await page.request.post(new URL('/api',url).href,{headers:{'X-Preferans-Token':new URL(url).hash.slice(1)},data:{action:'create',players:seats,target:30,name:'Гена',bots:true}});
    await page.route('**/api',async route=>{
      if(route.request().postDataJSON().action!=='status')return route.fallback();
      const response=await route.fetch(),body=await response.json(),v=body.data.view;
      Object.assign(v,{stage:'play',round:1,revision:90,actor:0,turn:0,dealer:2,declarer:0,contract:{level:7,suit:3},taken:Array(seats).fill(0),hand:[0,1,2,8,9,16,17,24,25,26],legal:[0,1,2],actions:['play'],open:true,trick:[],trickNo:1});
      v.players.forEach((p:any,i:number)=>{p.bot=false;p.name=i===seats-1?'Правый игрок':'Игрок';p.count=seats===4&&i===2?0:10;p.cards=i===0||seats===4&&i===2?[]:[3,4,5,10,11,18,19,27,28,29]});
      body.data.connected=Array(seats).fill(true);
      await route.fulfill({response,json:body});
    });
    await page.setViewportSize({width:844,height:390});
    await page.goto(url);
    await expect(page.locator('.player-right:not(.player-top)')).toBeVisible();
    for(const size of [{width:844,height:390},{width:780,height:360},{width:667,height:375}]) {
      await page.setViewportSize(size);
      await page.waitForTimeout(150);
      await page.screenshot({path:`../.build/phone-${seats}-${size.width}.png`});
      const bounds=await page.locator('.player:not(.player-top), .player:not(.player-top) .player-info, .player:not(.player-top) .card, .player:not(.player-top) .voice-control').evaluateAll(es=>es.map(e=>{const r=e.getBoundingClientRect();return {element:e.className,left:r.left,right:r.right,top:r.top,bottom:r.bottom}}));
      expect(bounds.every(r=>r.left>=-1 && r.right<=size.width+1 && r.bottom<=size.height+1),JSON.stringify({size,bounds})).toBeTruthy();
    }
  });
}

test("no-AI build hides model settings and blocks AI operations",async({page})=>{
  const aiRequests:string[]=[];
  page.on("request",request=>{
    if(!request.url().endsWith("/api")) return;
    const action=request.postDataJSON()?.action;
    if(action?.startsWith("ai-")) aiRequests.push(action);
  });
  await page.goto(url);
  await page.getByRole("button",{name:"Настройки",exact:true}).click();
  await expect(page.getByRole("tab",{name:"ИИ боты",exact:true})).toHaveCount(0);
  await page.locator("#settings-form > button.primary").click();
  await expect(page.locator("#panel")).not.toBeVisible();
  await page.locator("#create-form [name=bots]").check();
  await page.getByRole("button",{name:"Создать стол →"}).click();
  await expect(page.locator(".center.lobby")).toBeVisible();
  await expect(page.locator(".bot-model-icon")).toHaveCount(0);
  expect(aiRequests).toEqual([]);
  const status=await page.request.post(new URL("/api",url).href,{headers:{"X-Preferans-Token":new URL(url).hash.slice(1)},data:{action:"status"}});
  expect((await status.json()).data.aiEnabled).toBe(false);
  const response=await page.request.post(new URL("/api",url).href,{headers:{"X-Preferans-Token":new URL(url).hash.slice(1)},data:{action:"ai-settings"}});
  expect((await response.json()).error).toContain("ИИ отключён");
});

test("no-talon continuation has one generic nine and a separate suit declaration",async({page})=>{
  await page.setViewportSize({width:1024,height:700});
  let sent:any=null,phase=0,observedPhase=0;
  await page.route("**/api",async route=>{
    const q=route.request().postDataJSON();
    if(q.action==="command") {
      sent=q.command;
      return route.fulfill({json:{data:null}});
    }
    const response=await route.fetch(),body=await response.json();
    if(q.action==="status") {
      body.data.roomMode=false;body.data.connectionSetup=false;
      if(phase && body.data.view) Object.assign(body.data.view,{
        stage:phase===4?"contract":"auction",round:1,revision:99+phase,seat:0,actor:0,turn:0,
        actions:phase===4?["declare-no-talon"]:["bid","pass"],
        bid:phase===4?{level:9,suit:-1,noTalon:true}:null,
        contracts:phase===1?[{misere:true,level:0,suit:4}]:
          phase===2?[{misere:true,level:0,suit:4,noTalon:true}]:
          phase===3?[{level:9,suit:-1,misere:false,noTalon:true}]:
          [3,4].map(suit=>({level:9,suit,misere:false,noTalon:true}))
      });
    }
    await route.fulfill({response,json:body});
    if(q.action==="status") observedPhase=phase;
  });
  await page.goto(url);
  await page.locator("#create-form [name= bots]").check();
  await page.getByRole("button",{name:"Создать стол →"}).click();
  await expect(page.locator(".center.lobby")).toBeVisible();
  phase=1;
  await page.getByRole("button",{name:"Ставка",exact:true}).click();
  await expect(page.getByRole("button",{name:"Мизер",exact:true})).toBeVisible();
  await expect(page.locator(".contract-no-talon button")).toHaveCount(0);
  await page.keyboard.press("Escape");phase=2;
  await expect(page.locator(".hand-area [data-action=bid]")).toBeVisible();
  await expect.poll(()=>observedPhase).toBe(phase);
  await page.locator(".hand-area [data-action=bid]").click();
  await expect(page.getByRole("button",{name:"Мизер",exact:true})).toHaveCount(0);
  await expect(page.getByRole("button",{name:"Мизер без прикупа",exact:true})).toBeVisible();
  await page.keyboard.press("Escape");phase=3;
  await expect.poll(()=>observedPhase).toBe(phase);
  await page.locator(".hand-area [data-action=bid]").click();
  await expect(page.locator(".contract-no-talon button")).toHaveCount(1);
  await page.getByRole("button",{name:"9 без прикупа",exact:true}).click();
  expect(sent.contract).toEqual({level:9,suit:-1,misere:false,noTalon:true});
  phase=4;
  await page.locator("[data-action=declare-no-talon]").click();
  await expect(page.locator(".contract-grid button")).toHaveCount(5);
  await expect(page.getByRole("button",{name:"9 пик",exact:true})).toBeDisabled();
  await page.getByRole("button",{name:"9 черв",exact:true}).click();
  expect(sent.action).toBe("declare-no-talon");
  expect(sent.contract).toEqual({level:9,suit:3,misere:false,noTalon:true});
  expect(sent.cards).toEqual([]);
});

test('AI review settings persist and dialogue holds the decision until Continue',async({page})=>{
  await page.setViewportSize({width:1024,height:600});
  await page.goto(url);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.getByRole('tab',{name:'ИИ боты',exact:true}).click();
  await expect(page.locator('[data-ai-review]')).toBeDisabled();
  await expect(page.locator('[data-ai-language]')).toHaveValue('ru');
  await page.getByRole('tab',{name:'Игра',exact:true}).click();
  await page.locator('[name="debug-auction"]').check();
  await page.locator('[name="debug-play"]').check();
  await page.getByRole('tab',{name:'ИИ боты',exact:true}).click();
  await page.locator('[data-ai-review]').check();
  await page.locator('[data-ai-language]').selectOption('en');
  await page.locator('#settings-form > button.primary').click();
  await expect(page.locator('#panel')).not.toBeVisible();
  await page.reload();
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.getByRole('tab',{name:'ИИ боты',exact:true}).click();
  await expect(page.locator('[data-ai-review]')).toBeChecked();
  await expect(page.locator('[data-ai-language]')).toHaveValue('en');
  await page.keyboard.press('Escape');
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  let review:any={id:'review-test',seat:1,bot:'Бот 1 qwen',phase:'play',language:'en',summary:'Ход: 7♠',why:'Сохраняю старшие карты',model:{alias:'qwen',model:'test',maxTokens:500,temperature:0},elapsedMs:1500,promptTokens:200,completionTokens:30,chat:[],questionBusy:false,exchanges:[{phase:'play',systemRU:'Тестовая русская версия промпта',messages:[{role:'system',content:'Test system English'},{role:'user',content:'{"legal_actions":[{"move":0}]}'}],response:'{"choices":[{"message":{"content":"{\"move\":0,\"why\":\"Сохраняю старшие карты\"}"}}]}'}]};
  let continued=0;
  await page.route('**/api',async route=>{
    const q=route.request().postDataJSON();
    if(q.action==='ai-review-ask'){
      review.chat.push({role:'user',text:q.name},{role:'assistant',text:'Младшая карта сохраняет старшие для последующих взяток.'});
      return route.fulfill({json:{data:'Младшая карта сохраняет старшие для последующих взяток.'}});
    }
    if(q.action==='ai-review-continue'){continued++;review=null;return route.fulfill({json:{data:null}});}
    const response=await route.fetch(),body=await response.json();
    if(q.action==='status'){
      body.data.roomMode=false;body.data.paused=!!review;body.data.aiReview=review;
      Object.assign(body.data.view,{stage:'auction',round:1,revision:90+continued,aiReviewSeat:review?1:undefined});
    }
    await route.fulfill({response,json:body});
  });
  await expect(page.locator('#panel-title')).toHaveText('Разбор решения ИИ');
  await expect(page.locator('.ai-review-decision')).toHaveText('Ход: 7♠');
  await page.getByText('Отправленный промпт и данные',{exact:true}).click();
  await expect(page.locator('.ai-exchange')).toContainText('Test system English');
  await page.getByText('Промпт на русском (справочная версия)',{exact:true}).click();
  await expect(page.locator('.ai-exchange')).toContainText('Тестовая русская версия');
  await page.getByRole('textbox',{name:'Вопрос о решении'}).fill('Почему выбрана младшая карта?');
  await page.getByRole('button',{name:'Спросить модель',exact:true}).click();
  await expect(page.locator('[data-review-chat]')).toContainText('Младшая карта сохраняет');
  expect(continued).toBe(0);
  await page.locator('#panel').screenshot({path:'../.build/ai-review-tablet.png'});
  await page.getByRole('button',{name:'Продолжить',exact:true}).click();
  await expect(page.locator('#panel')).not.toBeVisible();
  expect(continued).toBe(1);
});

test('debug preference gates bot tools and inspection toggles a shared pause',async({page})=>{
  await page.goto(url);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await expect(page.locator('[name="debug-auction"]')).not.toBeChecked();
  await expect(page.locator('[name="debug-play"]')).not.toBeChecked();
  await page.keyboard.press('Escape');
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  await expect(page.locator('[data-panel="debug-bots"]')).toHaveCount(0);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.locator('[name="debug-auction"]').check();
  await page.locator('[name="debug-play"]').check();
  await expect(page.locator('[data-ai-field="timeout"]')).toHaveValue('10');
  await page.locator('#settings-form > button.primary').click();
  await expect(page.locator('#panel')).not.toBeVisible();
  await expect(page.locator('[data-panel="debug-bots"]')).toBeVisible();
  await page.reload();
  await expect(page.locator('[data-panel="debug-bots"]')).toBeVisible();
  await page.getByRole('button',{name:'Готов',exact:true}).click();
  await page.getByRole('button',{name:'Начать партию',exact:true}).click();
  const bot=page.locator('.bot-model-icon[data-debug-inspect="1"]');
  await bot.click();
  await expect(bot).toHaveAttribute('aria-pressed','true');
  await expect(page.locator('.player-left .exposed .card')).toHaveCount(10);
  await expect(page.locator('.player-left .exposed [data-card]')).toHaveCount(0);
  await page.waitForTimeout(400);
  await expect(bot).toHaveAttribute('aria-pressed','true');
  await bot.click();
  await expect(bot).toHaveAttribute('aria-pressed','false');
  await expect(page.locator('.player-left .exposed')).toHaveCount(0);
});

test('AI key saving reports unavailable native vault without storing a secret', async ({page}) => {
  test.skip(process.platform === 'win32', 'Windows provides a native credential vault.');
  await page.goto(url);
  await page.getByRole('button', {name: 'Настройки', exact: true}).click();
  await page.getByRole('tab', {name: 'ИИ боты', exact: true}).click();
  await page.locator('[data-ai="add"]').click();
  await page.locator('.ai-keys summary').click();
  await page.locator('[data-ai-key-name]').fill('Test key');
  await page.locator('#ai-key-value').fill('fake-key-for-test');
  await page.locator('[data-ai="key-save"]').click();
  await expect(page.locator('#notice')).toContainText('Защищённое хранилище ключей на этом устройстве недоступно');
  await expect(page.locator('[data-ai-field="keyID"] option')).toHaveCount(1);
});

test('AI models and encrypted keys persist and can be assigned to individual bots',async({page})=>{
  test.skip(process.platform !== 'win32', 'Native credential storage uses Windows DPAPI; Android supplies its own Keystore.');
  await page.setViewportSize({width:1100,height:800});
  await page.goto(url);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.getByRole('tab',{name:'ИИ боты',exact:true}).click();
  await expect(page.locator('[data-ai-field="timeout"]')).toHaveValue('10');
  await page.locator('[data-ai="add"]').click();
  await page.locator('[data-ai-field="alias"]').fill('qwen');
  await page.locator('[data-ai-field="model"]').fill('test-model');
  await page.locator('.ai-keys summary').click();
  await page.locator('[data-ai-key-name]').fill('Groq test');
  await page.locator('#ai-key-value').fill('fake-key-for-test');
  await page.locator('[data-password-toggle="ai-key-value"]').click();
  await expect(page.locator('#ai-key-value')).toHaveAttribute('type','text');
  await page.locator('[data-ai="key-save"]').click();
  await expect(page.locator('[data-ai-field="keyID"] option')).toHaveCount(2);
  await page.locator('[data-ai-field="timeout"]').fill('8');
  await page.locator('[data-ai-field="maxTokens"]').fill('700');
  await page.locator('[data-ai-field="temperature"]').fill('0.2');
  await page.screenshot({path:'../.build/ai-model-settings.png'});
  await page.locator('#settings-form > button.primary').click();
  await expect(page.locator('#panel')).not.toBeVisible();
  await page.reload();
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.getByRole('tab',{name:'ИИ боты',exact:true}).click();
  await expect(page.locator('[data-ai-field="alias"]')).toHaveValue('qwen');
  await expect(page.locator('[data-ai-field="timeout"]')).toHaveValue('8');
  await expect(page.locator('[data-ai-field="maxTokens"]')).toHaveValue('700');
  await expect(page.locator('[data-ai-field="temperature"]')).toHaveValue('0.2');
  await expect(page.locator('#ai-key-value')).toHaveValue('');
  await page.locator('#settings-form > button.primary').click();
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.table-heading [data-panel="bot-models"]')).toHaveCount(0);
  await page.locator('.bot-model-icon[data-bot-seat="1"]').click();
  await expect(page.locator('#bot-models select')).toHaveCount(1);
  await page.locator('#bot-models select[data-bot-seat="1"]').selectOption({label:'qwen'});
  await expect(page.locator('.player strong').filter({hasText:'Бот 1 qwen'})).toHaveCount(1);
  await page.keyboard.press('Escape');
  await page.setViewportSize({width:1024,height:600});
  await page.locator('.bot-model-icon[data-bot-seat="2"]').click();
  await expect(page.locator('#bot-models select[data-bot-seat="2"]')).toHaveValue('');
  await page.screenshot({path:'../.build/ai-bot-assignments.png'});
  await page.keyboard.press('Escape');
  let thinking=true;
  const startedAt=Date.now()-3000;
  await page.route('**/api',async route=>{
    const response=await route.fetch(),body=await response.json();
    if(route.request().postDataJSON().action==='status') {
      body.data.roomMode=false;
      body.data.view.botThinking=thinking ? {seat:1,startedAt} : undefined;
    }
    await route.fulfill({response,json:body});
  });
  await expect(page.locator('.bot-thinking')).toHaveCount(1);
  const timer=page.locator('.bot-thinking > span');
  const first=await timer.textContent();
  await expect(timer).not.toHaveText(first!);
  await page.screenshot({path:'../.build/ai-thinking-tablet.png'});
  thinking=false;
  await expect(page.locator('.bot-thinking')).toHaveCount(0);
});
for (const [n,dealer] of [[3,2],[4,2],[4,1],[4,3]]) test(`misere tracker retains twelve cards and marks only played: ${n}/${dealer}`,async({page})=>{
  await page.setViewportSize({width:1100,height:800});
  await page.goto(url);
  await page.locator('[name="players"]').selectOption(String(n));
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  let played:number[]=[];
  await page.route('**/api',async route=>{
    const response=await route.fetch(),body=await response.json();
    if(route.request().postDataJSON().action==='status'){
      body.data.roomMode=false;
      const v=body.data.view, declarer=dealer===1?2:1;
      Object.assign(v,{stage:'play',round:1,revision:100+played.length,dealer,declarer,actor:0,turn:0,contract:{misere:true,suit:4,level:0},hand:Array.from({length:10},(_,i)=>i+12),actions:['play'],legal:[12],taken:Array(n).fill(0),trick:[],lastTrick:Array.from({length:n},(_,i)=>({seat:i,card:i*8})),trickNo:0,misereCards:Array.from({length:12},(_,i)=>i),miserePlayed:played});
      v.players.forEach((p:any,i:number)=>{p.count=n===4&&i===dealer?0:10;p.cards=i!==declarer&&i!==dealer?Array.from({length:10},(_,j)=>j+12):[]});
    }
    await route.fulfill({response,json:body});
  });
  await expect(page.locator('.misere-tracker-card')).toHaveCount(12);
  await expect(page.locator('.misere-tracker .was-played')).toHaveCount(0);
  await expect(page.locator('.misere-tracker [data-card]')).toHaveCount(0);
  await expect(page.locator('.misere-tracker img')).toHaveCount(0);
  await expect(page.locator('.last-trick .mini-card')).toHaveCount(n);
  const local=(await page.locator('.last-trick .mini-seat-self').boundingBox())!;
  const left=(await page.locator('.last-trick .mini-seat-left').boundingBox())!;
  const right=(await page.locator('.last-trick .mini-seat-right').boundingBox())!;
  expect(left.x).toBeLessThan(local.x);
  expect(right.x).toBeGreaterThan(local.x);
  expect(local.y).toBeGreaterThan(left.y);
  if(n===4) expect((await page.locator('.last-trick .mini-seat-top').boundingBox())!.y).toBeLessThan(left.y);
  const preview=(await page.locator('.last-trick').boundingBox())!;
  const pool=(await page.locator('.pool-drawing').boundingBox())!;
  const own=(await page.locator('.hand-area').boundingBox())!;
  expect(preview.x+preview.width).toBeLessThanOrEqual(pool.x);
  expect(preview.y+preview.height).toBeLessThanOrEqual(own.y);
  played=[2,5];
  await expect(page.locator('.misere-tracker .was-played')).toHaveCount(2);
  await expect(page.locator('.misere-tracker-card')).toHaveCount(12);
  const box=(await page.locator('.misere-tracker').boundingBox())!;
  expect(box.x).toBeGreaterThanOrEqual(0);expect(box.y).toBeGreaterThanOrEqual(0);
  expect(box.x+box.width).toBeLessThanOrEqual(1100);expect(box.y+box.height).toBeLessThanOrEqual(800);
  await page.screenshot({path:`../.build/misere-tracker-${n}-${dealer}.png`});
  await page.setViewportSize({width:1024,height:600});
  await page.waitForTimeout(150);
  for (const card of await page.locator('.misere-tracker .mini-card').all()) {
    const square=(await card.boundingBox())!, face=(await card.locator('.mini-face').boundingBox())!;
    expect(Math.abs(square.width-square.height)).toBeLessThan(1);
    expect(face.height).toBeLessThanOrEqual(square.height);
  }
  const compactPreview=(await page.locator('.last-trick').boundingBox())!;
  const compactPool=(await page.locator('.pool-drawing').boundingBox())!;
  expect(compactPreview.x+compactPreview.width).toBeLessThanOrEqual(compactPool.x);
  await page.screenshot({path:`../.build/mini-cards-compact-${n}-${dealer}.png`});
});
test('Here button repeats the offered bid and hides when repetition is unavailable',async({page})=>{
  await page.goto(url);
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  let allowed=true,submitted:any=null;
  const bid={level:7,suit:2,misere:false};
  await page.route('**/api',async route=>{
    const q=route.request().postDataJSON();
    if(q.action==='command' && q.command.action==='bid'){
      submitted=q.command.contract;return route.fulfill({json:{data:null}});
    }
    const response=await route.fetch(),body=await response.json();
    if(q.action==='status'){
      body.data.roomMode=false;
      Object.assign(body.data.view,{stage:'auction',round:1,revision:allowed?100:101,turn:0,actor:0,dealer:2,declarer:1,bid,passed:[false,false,true],actions:['bid','pass'],contracts:allowed?[bid,{level:7,suit:3,misere:false}]:[{level:7,suit:3,misere:false}]});
    }
    await route.fulfill({response,json:body});
  });
  await page.locator('[data-action="bid"]').click();
  await expect(page.getByRole('button',{name:'Здесь',exact:true})).toBeVisible();
  await page.getByRole('button',{name:'Здесь',exact:true}).click();
  await expect.poll(()=>submitted).toEqual(bid);
  await expect(page.locator('#panel')).not.toBeVisible();
  allowed=false;
  await expect.poll(async()=>{await page.waitForTimeout(300);return page.locator('[data-action="bid"]').count()}).toBe(1);
  await page.waitForTimeout(700);
  await page.locator('[data-action="bid"]').click();
  await expect(page.locator('#panel')).toBeVisible();
  await expect(page.getByRole('button',{name:'Здесь',exact:true})).toHaveCount(0);
});
test('all-pass history shows net penalties after cancelling current tricks',async({page})=>{
  await page.goto(url);
  await page.locator('#create-form [name=bots]').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  await page.route('**/api',async route=>{
    const q=route.request().postDataJSON();
    const response=await route.fetch(),body=await response.json();
    if(q.action==='status')Object.assign(body.data.view,{stage:'round',round:6,revision:999,actions:[],allPass:true,taken:[5,1,4],history:[{round:6,label:'Распасы · 8',amnesty:8,amnestyTricks:1,pool:[0,0,0],mountain:[32,0,24],whists:[[0,0,0],[0,0,0],[0,0,0]]}]});
    await route.fulfill({response,json:body});
  });
  await expect(page.locator('.round-summary')).toContainText('число взяток каждого уменьшено на 1');
  await page.locator('.summary-close').click();
  await page.locator('.table-heading [data-panel=score]').click();
  await page.locator('#panel-body details summary').click();
  await expect(page.locator('#panel-body')).toContainText('гора +32');
  await expect(page.locator('#panel-body')).toContainText('гора +0');
  await expect(page.locator('#panel-body')).toContainText('гора +24');
  await expect(page.locator('#panel-body')).not.toContainText('с горы каждого игрока');
});

test('host AI flag hides an already open tab and restores it after leaving',async({page})=>{
  let enabled=true;
  await page.route('**/api',async route=>{
    if(route.request().postDataJSON().action!=='status')return route.fallback();
    const response=await route.fetch(),body=await response.json();
    body.data.aiEnabled=enabled;
    await route.fulfill({response,json:body});
  });
  await page.goto(url);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.getByRole('tab',{name:'ИИ боты',exact:true}).click();
  await expect(page.locator('#settings-ai')).toBeVisible();
  enabled=false;
  await expect(page.getByRole('tab',{name:'ИИ боты',exact:true})).toHaveCount(0);
  await expect(page.locator('#settings-game')).toBeVisible();
  await expect(page.locator('#settings-ai')).toHaveCount(0);
  enabled=true;
  await expect(page.getByRole('tab',{name:'ИИ боты',exact:true})).toBeVisible();
  await page.getByRole('tab',{name:'ИИ боты',exact:true}).click();
  await expect(page.locator('#settings-ai')).toBeVisible();
});

test('Android log settings show disk size, export full file and clear it',async({page})=>{
  await page.addInitScript(()=>{
    (window as any).logExports=0;
    (window as any).PreferansAndroid={exitApp(){},saveLog(){},saveFullLogToDownloads(){(window as any).logExports++;}};
  });
  let size=1048576,clears=0;
  await page.route('**/api',async route=>{
    const q=route.request().postDataJSON();
    if(q.action==='log-clear'){size=0;clears++;return route.fulfill({json:{data:null}});}
    const response=await route.fetch(),body=await response.json();
    if(q.action==='status')body.data.logSize=size;
    await route.fulfill({response,json:body});
  });
  await page.goto(url);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.getByRole('tab',{name:'Подключение',exact:true}).click();
  await expect(page.locator('#log-file-size')).toContainText('1 МБ');
  await page.getByRole('button',{name:'Сохранить в Загрузки',exact:true}).click();
  expect(await page.evaluate(()=>(window as any).logExports)).toBe(1);
  expect(clears).toBe(0);
  await page.getByRole('button',{name:'Очистить',exact:true}).click();
  await expect(page.locator('#log-file-size')).toContainText('0 Б');
  expect(clears).toBe(1);
});

test('pause resume button is visible only to its owner', async ({page})=>{
  await page.goto(url);
  await page.locator('#create-form [name=bots]').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await page.locator('[data-action=ready]').click();
  await page.locator('[data-action=start]').click();
  await page.locator('#pause-game').click();
  let owner=1;
  await page.route('**/api',async route=>{
    if(route.request().postDataJSON().action!=='status')return route.fallback();
    const response=await route.fetch(),body=await response.json();
    body.data.view.pausedBy=owner;
    body.data.view.revision+=100+owner;
    body.data.view.players[1].name='Друг';
    await route.fulfill({response,json:body});
  });
  await expect(page.locator('#pause-game')).toHaveCount(0);
  await expect(page.locator('.hand-area')).toContainText('Ждём, когда этот игрок продолжит игру');
  await expect(page.locator('#table-progress')).toContainText('Друг');
  owner=0;
  await expect(page.locator('#pause-game')).toHaveText('Продолжить игру');
});

for (const n of [3,4]) test('table preferences and shared pause clock survive refresh: '+n, async ({page}) => {
  await page.setViewportSize({width:1100,height:800});
  await page.goto(url);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.locator('[name="rule-target"]').selectOption('50');
  await page.locator('[name="rule-end"]').selectOption('time');
  await page.locator('[name="rule-minutes"]').selectOption('45');
  await page.locator('#settings-form button.primary').click();
  await expect(page.locator('#panel')).not.toBeVisible();
  await expect(page.getByLabel('Время, минут',{exact:true})).toHaveValue('45');
  await expect(page.getByLabel('Пуля до',{exact:true})).toBeHidden();
  await page.reload();
  await expect(page.locator('[name="target"]')).toHaveValue('50');
  await expect(page.getByLabel('Время, минут',{exact:true})).toHaveValue('45');
  await expect(page.getByLabel('Пуля до',{exact:true})).toBeHidden();
  await page.locator('[name="players"]').selectOption(String(n));
  await page.locator('[name="name"]').fill('Пауза-тест');
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await page.locator('[data-action="ready"]').click();
  await page.locator('[data-action="start"]').click();
  await page.locator('#pause-game').click();
  await expect(page.locator('#pause-game')).toHaveText('Продолжить игру');
  await expect(page.locator('#table-progress')).toContainText('Пауза-тест');
  const timer=await page.locator('#table-progress span').innerText();
  await expect.poll(()=>page.locator('#table-progress span').innerText()).not.toBe(timer);
  await page.reload();
  await expect(page.locator('#pause-game')).toHaveText('Продолжить игру');
  await expect(page.locator('.pool-total')).toHaveText('45');
  await page.screenshot({path:`../.build/pause-table-${n}.png`});
  await page.locator('#pause-game').click();
  await expect(page.locator('#pause-game')).toHaveText('Пауза');
  await expect(page.locator('#table-progress')).toContainText('Время');
});

test('finished result stays visible and history opens without joining',async({page})=>{
  await page.goto(url);
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  let finalView:any=null, exited=false;
  await page.route('**/api',async route=>{
    const q=route.request().postDataJSON();
    if(q.action==='history')return route.fulfill({json:{data:[{id:finalView.id,names:finalView.players.map((p:any)=>p.name),results:finalView.results,round:3,startedAt:1700000000-2700,finishedAt:1700000000}]}});
    if(q.action==='history-result')return route.fulfill({json:{data:finalView}});
    if(q.action==='leave')exited=true;
    const response=await route.fetch(),body=await response.json();
    if(q.action==='status')body.data.roomMode=false;
    if(q.action==='status' && !exited && body.data.view){
      Object.assign(body.data.view,{stage:'finished',round:3,revision:999,results:[30,-15,-15],taken:[0,0,0],actions:[],startedAt:1700000000-2700,finishedAt:1700000000});
      finalView=body.data.view;
    }
    await route.fulfill({response,json:body});
  });
  await expect(page.getByRole('heading',{name:'Итог партии',exact:true})).toBeVisible();
  await page.waitForTimeout(2500);
  await expect(page.getByRole('heading',{name:'Итог партии',exact:true})).toBeVisible();
  await page.locator('#finish-party-topbar').click();
  await page.locator('#party-history').click();
  await expect(page.locator('.party-dates')).toContainText('Начало:');
  await expect(page.locator('.party-dates')).toContainText('Окончание:');
  await expect(page.locator('.party-dates')).toContainText('2023');
  await expect(page.locator('.party-dates')).not.toContainText('не записано');
  await expect(page.locator('.history-whists')).toHaveText(['10','-5','-5']);
  await expect(page.locator('.history-totals')).toContainText('Итоги игроков комнаты');
  await expect(page.locator('.history-totals tbody tr')).toHaveCount(3);
  await expect(page.locator('.history-totals tbody strong')).toHaveText(['10','-5','-5']);
  await page.screenshot({path:'../.build/history-totals.png'});
  await page.locator('[data-history-id]').click();
  await expect(page.locator('#panel-title')).toHaveText('Результат завершённой партии');
  await expect(page.locator('.history-pool svg')).toBeVisible();
  await expect(page.locator('.history-pool .pool-balance')).toHaveText([/10$/,/-5$/,/-5$/]);
  await expect(page.locator('.history-pool [data-panel]')).toHaveCount(0);
  await page.waitForTimeout(500);
  const drawing = (await page.locator('.history-pool svg').boundingBox())!;
  const panel = (await page.locator('#panel').boundingBox())!;
  expect(drawing.y + drawing.height).toBeLessThan(panel.y + panel.height);
  await page.screenshot({path:'../.build/history-pool.png'});
  await expect(page.locator('.party-dates')).toContainText('2023');
  await expect(page.locator('#create-form')).toBeAttached();
  await page.locator('#history-back').click();
  await expect(page.locator('.history-entry')).toHaveCount(1);
  await expect(page.locator('.history-pool')).toHaveCount(0);
});

test('finish-after-passes agreement persists through settings and reload', async ({page}) => {
  await page.goto(url);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await expect(page.getByRole('tab',{name:'Администратор',exact:true})).toHaveCount(0);
  await expect(page.locator('[name="rule-finish-after-pass-exit"]')).toBeChecked();
  await page.locator('[name="rule-finish-after-pass-exit"]').uncheck();
  await page.locator('#settings-form button.primary').click();
  await expect(page.locator('#panel')).not.toBeVisible();
  await page.reload();
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await expect(page.locator('[name="rule-finish-after-pass-exit"]')).not.toBeChecked();
});

test('controlled upper hand accepts dragging across the card face',async({page})=>{
  await page.setViewportSize({width:1100,height:820});
  await page.goto(url);
  await page.locator('[name="players"]').selectOption('4');
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  await page.route('**/api',async route=>{
    const response=await route.fetch(),body=await response.json();
    if(route.request().postDataJSON().action==='status'){
      body.data.roomMode=false;
      Object.assign(body.data.view,{stage:'play',revision:100,round:1,actor:0,turn:2,dealer:3,declarer:1,contract:{misere:true,suit:4},open:true,actions:['play'],legal:[7],hand:[0],trick:[{seat:1,card:1},{seat:0,card:2}],trickNo:0});
      body.data.view.players[2].cards=[7,8,10,15,19,22,23,26,27,28];
    }
    await route.fulfill({response,json:body});
  });
  const card=page.locator('.player-top [data-card="7"]');
  await expect(card).toBeVisible();
  const misses=await card.evaluate(el=>{
    const r=el.getBoundingClientRect(),misses:string[]=[];
    for(const x of [.15,.5,.85])for(const y of [.15,.5,.85]){
      const hit=document.elementFromPoint(r.x+r.width*x,r.y+r.height*y);
      if(hit?.closest('[data-card]')!==el)misses.push(`${x}/${y}: ${hit?.tagName}.${hit?.className}`);
    }
    return misses;
  });
  expect(misses).toEqual([]);
  for(const y of [.15,.5,.85]) {
    const r=(await card.boundingBox())!;
    await page.mouse.move(r.x+r.width*.5,r.y+r.height*y);
    await page.mouse.down();
    await page.mouse.move(r.x+r.width*.5+15,r.y+r.height*y+15,{steps:3});
    await expect(page.locator('.drag-ghost')).toHaveCount(1);
    await page.mouse.move(5,5);
    await page.mouse.up();
    await expect(page.locator('.drag-ghost')).toHaveCount(0);
  }
});

test('player count survives reopening and notices expire',async({page})=>{
  let playerCount=3;
  let deleted=false;
  await page.route('**/api',async route=>{
    const q=route.request().postDataJSON();
    if(q.action==='list')return route.fulfill({json:{data:deleted?[]:[{id:'test-save',names:['Player'],host:true,round:1,stage:'lobby'}]}});
    if(q.action==='delete-save'){deleted=true;return route.fulfill({json:{data:null}});}
    if(q.action==='appearance'){
      playerCount=q.appearance.playerCount;
      return route.fulfill({json:{data:null}});
    }
    const response=await route.fetch(),body=await response.json();
    if(q.action==='status'){
      body.data.roomMode=false;
      body.data.connectionSetup=false;
      body.data.appearance.playerCount=playerCount;
    }
    await route.fulfill({response,json:body});
  });
  await page.goto(url);
  await page.locator('[name="players"]').selectOption('4');
  await expect.poll(()=>playerCount).toBe(4);
  await page.reload();
  await expect(page.locator('[name="players"]')).toHaveValue('4');
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.locator('#settings-form button.primary').click();
  await expect(page.locator('#panel')).not.toBeVisible();
  await page.locator('#delete-save').click();
  await expect(page.locator('#notice')).toBeVisible();
  await expect(page.locator('#notice')).not.toBeVisible({timeout:4500});
  await page.reload();
  await expect(page.locator('[name="players"]')).toHaveValue('4');
});

test('room owner can close a table without a local game',async({page})=>{
  let closed=false;
  await page.route('**/api',async route=>{
    const q=route.request().postDataJSON();
    if(q.action==='leave'){closed=true;return route.fulfill({json:{data:null}});}
    const response=await route.fetch(),body=await response.json();
    if(q.action==='status')Object.assign(body.data,{current:false,view:null,host:false,connectionSetup:false,roomMode:true,roomSelf:'me',roomOnline:true,roomError:'',room:{id:'room',name:'Test',members:[{id:'me',name:'Host',slot:0,online:true}],table:closed?undefined:{id:'lost',host:'me',name:'Host',players:['me','',''],bots:[false,true,true]}}});
    await route.fulfill({response,json:body});
  });
  await page.goto(url);
  await expect(page.locator('#room-roster')).toContainText('партия не восстановлена');
  await expect(page.locator('[data-room-action="rejoin"]')).toHaveCount(0);
  await expect(page.locator('#create-form button[type="submit"]')).toBeDisabled();
  await page.getByRole('button',{name:'Закрыть стол',exact:true}).click();
  await expect(page.locator('.app-confirm')).toBeVisible();
  await page.locator('.app-confirm').getByRole('button',{name:'Отмена',exact:true}).click();
  expect(closed).toBe(false);
  await expect(page.locator('.app-confirm')).toHaveCount(0);
  await page.getByRole('button',{name:'Закрыть стол',exact:true}).click();
  await page.locator('.app-confirm').getByRole('button',{name:'Закрыть стол',exact:true}).click();
  await expect(page.locator('#create-form button[type="submit"]')).toBeEnabled();
  await expect(page.locator('[data-room-action="close-table"]')).toHaveCount(0);
});

test('rooms: first connection asks only name and password and retries errors',async({page})=>{
  let configured=false, attempts=0;
  await page.route('**/api',async route=>{
    const q=route.request().postDataJSON();
    if(q.action==='room-service')return route.fulfill({json:{data:null}});
    if(q.action==='connection-setup'){
      attempts++;
      expect(q.name).toBe('Партнёр');
      expect(q.stun[0]).toMatch(/^turn:signaling.example.org:3478\?transport=udp\|preferans\|/);
      if(q.stun[0].endsWith('|bad'))return route.fulfill({json:{error:'TURN отклонил пользователя или пароль'}});
      configured=true;return route.fulfill({json:{data:null}});
    }
    if(q.action==='rooms-list')return route.fulfill({json:{data:{rooms:[]}}});
    const response=await route.fetch(),body=await response.json();
    if(q.action==='status')Object.assign(body.data,{connectionSetup:!configured,roomMode:true,room:null});
    await route.fulfill({response,json:body});
  });
  await page.goto(url);
  await expect(page.locator('#connection-setup')).toBeVisible();
  await expect(page.locator('[name="turn-server"]')).not.toBeVisible();
  await expect(page.getByLabel('Пароль подключения',{exact:true})).toHaveAttribute('type','password');
  await page.getByLabel('Ваше имя',{exact:true}).fill('Партнёр');
  await page.getByLabel('Пароль подключения',{exact:true}).fill('bad');
  await page.getByRole('button',{name:'Проверить и сохранить',exact:true}).click();
  await expect(page.locator('#setup-result')).toContainText('пароль');
  await page.getByLabel('Пароль подключения',{exact:true}).fill('good');
  await page.getByRole('button',{name:'Проверить и сохранить',exact:true}).click();
  await expect(page.locator('#room-picker')).toBeVisible();expect(attempts).toBe(2);
  await page.reload();await expect(page.locator('#room-picker')).toBeVisible();
});

test('button contact and click have separate feedback without duplicate actions',async({page})=>{
  await page.goto(url);
  const button=page.getByRole('button',{name:'Правила',exact:true});
  const r=(await button.boundingBox())!;
  await page.mouse.move(r.x+r.width/2,r.y+r.height/2);await page.mouse.down();
  await expect(button).toHaveClass(/button-contact/);
  await expect(page.locator('#panel')).not.toBeVisible();
  await page.mouse.up();
  await expect(page.locator('#panel')).toBeVisible();
  await expect(page.locator('.button-click-feedback')).toHaveCount(1);
  await expect(page.locator('.button-contact')).toHaveCount(0);
  await expect(page.locator('.button-click-feedback')).toHaveCount(0);
  await page.locator('#close-panel').click();
  const cdp=await page.context().newCDPSession(page);
  await cdp.send('Emulation.setTouchEmulationEnabled',{enabled:true});
  const touch={x:r.x+r.width/2,y:r.y+r.height/2,id:1};
  await cdp.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[touch]});
  await expect(button).toHaveClass(/button-contact/);
  await cdp.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});
  await expect(page.locator('#panel')).toBeVisible();
  await expect(page.locator('.button-contact')).toHaveCount(0);
  await cdp.detach();
});
test('Windows log path can be saved in connection settings', async ({page}) => {
  test.skip(process.platform !== 'win32', 'The configurable desktop log path is a Windows feature.');
  await page.goto(url);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.getByRole('tab',{name:'Подключение',exact:true}).click();
  const path=resolve('..', '.build', `settings-log-${Date.now()}.log`);
  await page.getByLabel('Путь к журналу Windows',{exact:true}).fill(path);
  await page.locator('[name="log-enabled"]').check();
  await page.locator('#settings-form button.primary').click();
  await expect(page.locator('#panel')).not.toBeVisible();
  await expect.poll(async () => { try { return await readFile(path,'utf8'); } catch { return ''; } }).toContain('Путь журнала применён');
  await page.reload();
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.getByRole('tab',{name:'Подключение',exact:true}).click();
  await expect(page.getByLabel('Путь к журналу Windows',{exact:true})).toHaveValue(path);
});
test('dealer backs match settings and solo bots have no microphone', async ({page}) => {
  await page.goto(url);
  await expect(page.locator('.topbar [data-exit-app] svg')).toBeVisible();
  await page.locator('[name="players"]').selectOption('4');
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button', {name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  await expect(page.locator('#app .voice-mic')).toHaveCount(0);
  let back = 'plaid', humanGuest = false;
  await page.route('**/api', async route => {
    const response = await route.fetch(), body = await response.json();
    if (route.request().postDataJSON().action === 'status') {
      body.data.appearance.back = back;
      const v = body.data.view;
      Object.assign(v, {stage:'auction', revision:900 + ['plaid','diamonds','ornament','waves','classic'].indexOf(back) + Number(humanGuest)*10,
        round:1, dealer:0, actor:1, turn:1, hand:[], actions:[]});
      v.players[0].count = 0;
      v.players[1].bot = !humanGuest;
      v.players.slice(1).forEach((p:any) => {p.count=10;p.cards=[];});
    }
    await route.fulfill({response,json:body});
  });
  for (const chosen of ['plaid','diamonds','ornament','waves','classic']) {
    back = chosen;
    await expect(page.locator('body')).toHaveAttribute('data-back',chosen);
    await expect(page.locator('.hand-area .dealer-hand .card-back')).toHaveCount(2);
    const styles = await page.locator('#app .card-back').evaluateAll(cards => cards.map(card => {
      const s=getComputedStyle(card); return [s.backgroundImage,s.backgroundColor,s.backgroundSize];
    }));
    expect(styles.length).toBeGreaterThan(2);
    for (const style of styles) expect(style).toEqual(styles[0]);
    await expect(page.locator('#app .voice-mic')).toHaveCount(0);
  }
  humanGuest = true;
  await expect(page.locator('#app .voice-mic')).toHaveCount(2);
});
for (const shell of ['windows', 'android']) {
  test(`application exit: ${shell} bridge from home, dialog and table`, async ({page}) => {
    await page.addInitScript((shell) => {
      (window as any).exitCalls = 0;
      const exit = () => { (window as any).exitCalls++; };
      if (shell === 'android') (window as any).PreferansAndroid = {exitApp: exit};
      else (window as any).preferansExit = async () => exit();
    }, shell);
    await page.goto(url);
    const exit = page.getByRole('button', {name:'Выход из приложения', exact:true});
    await exit.click();
    await expect.poll(() => page.evaluate(() => (window as any).exitCalls)).toBe(1);
    await page.getByRole('button', {name:'Правила', exact:true}).click();
    await page.locator('#panel [data-exit-app]').click();
    await expect.poll(() => page.evaluate(() => (window as any).exitCalls)).toBe(2);
    await page.locator('#close-panel').click();
    await page.locator('[name="bots"]').check();
    await page.getByRole('button', {name:'Создать стол →'}).click();
    await expect(page.locator('body')).toHaveClass(/at-table/);
    await exit.click();
    await expect.poll(() => page.evaluate(() => (window as any).exitCalls)).toBe(3);
    // Exit must not call "leave" and remove the current saved game.
    await expect(page.locator('body')).toHaveClass(/at-table/);
  });
}
async function dragCard(page: Page, source: Locator, target: Locator) {
  const from = await source.boundingBox(), to = await target.boundingBox();
  if (!from || !to) throw Error('Missing drag source or destination');
  await page.mouse.move(from.x+8, from.y+20);
  await page.mouse.down();
  await page.mouse.move(to.x+to.width/2,to.y+to.height/2,{steps:8});
  await page.mouse.up();
  await expect(page.locator('.drag-ghost')).toHaveCount(0);
}
test.beforeEach(async ({page},info) => {
  const root = resolve("..");
  processHandle = spawn(
    resolve(root, info.title.startsWith("no-AI build") ? "dist/Preferans-NoAI.exe" : ".build/preferans-test.exe"),
    ["-headless", "-update-port", "-1", "-data", resolve(root, `.build/ui-${Date.now()}`)],
    { cwd: root, windowsHide: true, env:info.title.startsWith('rooms: local')?{...process.env,PREFERANS_ROOMS_URL:'http://127.0.0.1:1'}:process.env },
  );
  url = await new Promise<string>((resolve, reject) => {
    processHandle.stdout!.once("data", (b) => resolve(String(b).trim()));
    processHandle.once("error", reject);
    processHandle.once("exit", (c) => reject(Error(`App exited ${c}`)));
  });
  // Existing gameplay cases intentionally run in Russian; the language case
  // exercises the unconfigured device's Ukrainian default.
  if(!info.title.startsWith('rooms: local language')) {
    const headers={'X-Preferans-Token':new URL(url).hash.slice(1)};
    const endpoint=new URL('/api',url).href;
    const state=await (await page.request.post(endpoint,{headers,data:{action:'status'}})).json();
    await page.request.post(endpoint,{headers,data:{action:'appearance',appearance:{...state.data.appearance,language:'ru'}}});
  }
  // Serve the current built UI with the isolated backend, without relinking Go for CSS edits.
  await page.route('**/*', async route => {
    const requested = new URL(route.request().url());
    if (requested.origin === new URL(url).origin && /^\/cards\/[^/]+\/[^/]+\.svg$/.test(requested.pathname)) {
      return route.fulfill({body: readFileSync(resolve('dist', requested.pathname.slice(1))), contentType:'image/svg+xml'});
    }
    if(requested.origin === new URL(url).origin && (requested.pathname==='/' || /^\/assets\/[^/]+\.(js|css)$/.test(requested.pathname))) {
      const file = requested.pathname==='/' ? 'index.html' : requested.pathname.slice(1);
      let body=readFileSync(resolve('dist',file));
      if(file==='index.html') {
        const bootstrap=await route.fetch();
        const language=(await bootstrap.text()).match(/data-language="(ru|uk|en)"/)?.[1] || 'ru';
        body=Buffer.from(body.toString().replace('<html',`<html data-language="${language}"`));
      }
      return route.fulfill({body,contentType:file.endsWith('.js')?'text/javascript':file.endsWith('.css')?'text/css':'text/html'});
    }
    await route.continue();
  });
});
test.beforeEach(async ({page},info)=>{
  if(info.title.startsWith('rooms:'))return;
  // Legacy table tests exercise gameplay directly; room entry is covered separately.
  await page.route('**/api',async route=>{
    if(route.request().postDataJSON().action!=='status')return route.fallback();
    const response=await route.fetch(),body=await response.json();body.data.roomMode=false;
    await route.fulfill({response,json:body});
  });
});

test('rooms: rejoin indicator persists until authenticated connection',async({page})=>{
  await page.request.post(new URL('/api',url).href,{headers:{'X-Preferans-Token':new URL(url).hash.slice(1)},data:{action:'create',players:3,target:30,name:'Host',bots:true}});
  let connected=false,offline=false,requests=0;
  await page.route('**/api',async route=>{
    const q=route.request().postDataJSON();
    if(q.action==='room-rejoin'){requests++;return route.fulfill({json:{data:null}});}
    const response=await route.fetch(),body=await response.json();
    if(q.action==='status'){
      const view=connected?body.data.view:null;
      if(view){view.id='table';view.seat=1;view.players[0].bot=false;view.players[1].bot=false;}
      Object.assign(body.data,{current:connected,view,host:false,connected:connected?[true,true,true]:[],connectionSetup:false,roomMode:true,roomSelf:'me',roomOnline:!offline,roomError:'',room:{id:'room',name:'Test',members:[{id:'me',name:'Guest',slot:1,online:true}],table:{id:'table',host:'host',name:'Host',players:['host','me',''],bots:[false,false,true]}}});
    }
    await route.fulfill({response,json:body});
  });
  await page.goto(url);
  await page.getByRole('button',{name:'Вернуться к столу',exact:true}).click();
  const button=page.locator('[data-room-action=rejoin]');
  await expect(button).toContainText('Подключаемся');
  await expect(button).toBeDisabled();
  await expect(button.locator('.connection-spinner')).toBeVisible();
  offline=true;
  await expect(page.locator('#room-roster')).toContainText('Нет связи');
  await expect(button).toBeDisabled();
  expect(requests).toBe(1);
  offline=false;connected=true;
  await expect(page.locator('.center.lobby')).toBeVisible();
  await expect(page.locator('.connection-spinner')).toHaveCount(0);
});

test('rooms: directory, lobby, voice controls and default room',async({page})=>{
  await page.setViewportSize({width:1100,height:800});
  let room:any=null,defaultRoom='',catalog:any[]=[];
  await page.route('**/api',async route=>{
    const req=route.request().postDataJSON();
    if(req.action==='rooms-list')return route.fulfill({json:{data:{rooms:catalog}}});
    if(req.action==='rooms-create'){const r={id:'room-a',name:req.name,members:[]};catalog.push(r);return route.fulfill({json:{data:{room:r}}});}
    if(req.action==='rooms-rename'){catalog[0].name=req.name;return route.fulfill({json:{data:{room:catalog[0]}}});}
    if(req.action==='rooms-delete'){catalog=[];return route.fulfill({json:{data:{}}});}
    if(req.action==='room-enter'){room={...catalog[0],members:[{id:'me',name:'Гена',slot:0,online:true},{id:'friend',name:'Друг',slot:1,online:true}]};return route.fulfill({json:{data:null}});}
    if(req.action==='room-exit'){room=null;return route.fulfill({json:{data:null}});}
    if(req.action==='appearance')defaultRoom=req.appearance.defaultRoom;
    if(req.action.startsWith('voice-'))return route.fulfill({json:{data:req.action==='voice-poll'?[]:null}});
    const response=await route.fetch(),body=await response.json();
    if(req.action==='status')Object.assign(body.data,{roomMode:true,connectionSetup:false,room,roomSelf:'me',roomOnline:true});
    await route.fulfill({response,json:body});
  });
  await page.goto(url);
  await expect(page.locator('#room-picker')).toBeVisible();
  await page.getByText('Управление комнатами',{exact:true}).click();
  await page.locator('#room-name').fill('   ');
  await page.locator('[data-room-action="create"]').click();
  await expect(page.locator('#notice')).toContainText('Введите название комнаты');
  expect(catalog).toHaveLength(0);
  await page.locator('#room-name').fill('Друзья');
  await page.locator('[data-room-action="create"]').click();
  await expect(page.locator('#room-select')).toContainText('Друзья');
  await page.locator('#room-name').fill('Вечерний стол');
  await page.locator('[data-room-action="rename"]').click();
  await expect(page.locator('#room-select')).toContainText('Вечерний стол');
  await page.locator('[data-room-action="enter"]').click();
  await expect(page.locator('#room-roster')).toContainText('Друг');
  await expect(page.locator('.room-heading h2')).toHaveCount(0);
  await expect(page.locator('.room-member-label').first()).toContainText('в комнате Вечерний стол');
  await expect(page.locator('.room-member-label em').first()).toHaveText('Вечерний стол');
  await expect(page.locator('#room-roster [data-microphone]')).toHaveCount(1);
  await expect(page.locator('#room-roster [data-voice-seat]')).toHaveCount(1);
  await expect(page.locator('[data-auto-join]')).toHaveCount(0);
  await page.screenshot({path:'../.build/room-lobby.png'});
  const bounds=await page.locator('#create-form').boundingBox();
  expect(bounds!.y+bounds!.height).toBeLessThanOrEqual(800);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.getByRole('tab',{name:'Подключение',exact:true}).click();
  await page.locator('[name="default-room"]').selectOption('room-a');
  await page.locator('#settings-form button.primary').click();
  await expect.poll(()=>defaultRoom).toBe('room-a');
  await expect(page.locator('body')).not.toHaveClass(/busy/);
  await page.locator('[data-room-action="exit"]').click();
  await expect(page.locator('#room-picker')).toBeVisible();
});
test.afterEach(async ({page}) => {
  await page.unrouteAll({behavior:'wait'});
  processHandle?.kill();
});

test('finish party is available before the first deal', async ({page}) => {
  await page.route('http://signaling.example.org:8080/**', route => route.fulfill({json:{}}));
  await page.goto(url);
  await expect(page.locator('#finish-party-topbar')).toBeHidden();
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button', {name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  await expect(page.locator('#finish-party-topbar')).toBeVisible();
  await expect(page.locator('#finish-party-topbar')).toHaveText('Завершить партию');
  await page.locator('#finish-party-topbar').click();
  await expect(page.getByRole('button', {name:'Создать стол →'})).toBeVisible();
  await expect(page.locator('#finish-party-topbar')).toBeHidden();
});

test('guest sees Exit on the table and in settings', async ({page}) => {
  await page.route('http://signaling.example.org:8080/**', route=>route.fulfill({json:{}}));
  await page.goto(url);
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  await page.route('**/api',async route=>{
    const response=await route.fetch(),body=await response.json();
    if(route.request().postDataJSON().action==='status') {
      body.data.host=false;
      body.data.view.revision+=100;
    }
    await route.fulfill({response,json:body});
  });
  await expect(page.locator('#finish-party-topbar')).toHaveText('Выйти');
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await expect(page.locator('#leave')).toHaveText('Выйти');
});

test('joining saves the name from the first screen before discovery', async ({page}) => {
  let searched = false;
  await page.route('http://signaling.example.org:8080/**', async route => {
    const response = await page.request.post(new URL('/api',url).toString(), {
      headers:{'X-Preferans-Token':new URL(url).hash.slice(1)}, data:{action:'status'}
    });
    expect((await response.json()).data.appearance.name).toBe('Игорь');
    searched = true;
    await route.fulfill({json:{}});
  });
  await page.goto(url);
  await page.locator('#create-form [name="name"]').fill('Игорь');
  await page.locator('[data-auto-join]').click();
  await expect.poll(()=>searched).toBe(true);
  await page.reload();
  await expect(page.locator('#create-form [name="name"]')).toHaveValue('Игорь');
});

for(const players of [3,4]) test(`edited SVG layout ${players} players uses equal cards`, async ({page})=>{
  await page.route('http://signaling.example.org:8080/**', route=>route.fulfill({json:{}}));
  await page.goto(url);
  await page.locator('[name="players"]').selectOption(String(players));
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  let stage='auction', revision=980;
  await page.route('**/api',async route=>{
    const response=await route.fetch(),body=await response.json();
    if(route.request().postDataJSON().action==='status'){
      const v=body.data.view;
      Object.assign(v,{stage,revision,round:3,actor:0,turn:0,open:stage==='play',declarer:0,dealer:players-1,actions:stage==='play'?['play']:['pass','bid'],hand:[0,1,7,8,9,16,18,24,26,31],playHand:[0,1,7,8,9,16,18,24,26,31],legal:[0],trick:[],defence:players===3?[0,2,1]:[0,2,1,0]});
      v.players.forEach((p:any,i:number)=>{p.count=i===players-1&&players===4&&stage!=='play'?0:10;p.cards=stage==='play'&&i>0&&p.count?[2,3,6,10,11,17,19,25,27,30]:[];});
      if (players===4 && stage==='play') v.dealer=0;
    }
    await route.fulfill({response,json:body});
  });
  for(const mode of ['auction','play']){
    stage=mode;revision++;
    await expect(page.locator('body')).toHaveAttribute('data-stage',mode);
    for(const [width,height] of [[1440,900],[1100,800],[1000,600],[844,390],[390,844]]){
      await page.setViewportSize({width,height});
      await page.screenshot({path:`../.build/layout-${players}-${mode}-${width}.png`});
      const sizes=await page.locator('.hand .card,.exposed .card,.player-cards .card-back').evaluateAll(es=>es.map(e=>{const r=e.getBoundingClientRect();return {own:!!e.closest('.hand'),w:r.width,h:r.height,b:r.bottom,l:r.left,r:r.right};}));
      const base=sizes.find(r=>!r.own)!;
      expect(sizes.every(r=>Math.abs(r.h-base.h*(width>700&&r.own?1.12:1))<1)).toBeTruthy();
      expect(sizes.every(r=>r.b<=height)).toBeTruthy();
      expect(sizes.every(r=>r.l>=0&&r.r<=width),JSON.stringify({width,height,sizes})).toBeTruthy();
      if (width > 700 && (height > 650 || players === 4)) {
        const layout = await page.evaluate(() => {
          const area = document.querySelector('.hand-area')!.getBoundingClientRect();
          const info = document.querySelector('.hand-label > strong')!.getBoundingClientRect();
          const actions = document.querySelector('.action-area')!.getBoundingClientRect();
          const hand = [...document.querySelectorAll('.hand .card')].map(e => e.getBoundingClientRect());
          const side = [...document.querySelectorAll('.player:not(.player-top) .exposed .card')].map(e => e.getBoundingClientRect());
          return {
            separate: side.every(r => r.bottom < area.top),
            horizontal: hand.every(r => r.left >= info.right && r.right <= actions.left),
            contained: hand.every(r => r.top >= area.top && r.bottom <= area.bottom),
          };
        });
        expect(layout, `horizontal player area at ${width}x${height}`).toEqual({separate:true,horizontal:true,contained:true});
      }
      const images = page.locator('.hand .card-image, .exposed .card-image');
      expect(await images.count()).toBeGreaterThan(0);
      await expect.poll(() => images.evaluateAll(es => es.every(e => (e as HTMLImageElement).complete && (e as HTMLImageElement).naturalWidth > 0))).toBeTruthy();
      if (players === 4 && width > 700) {
        if (mode==='play' && height>=500) {
          const overlaps = await page.evaluate(() => {
            const upper=[...document.querySelectorAll('.player-top .exposed .card')].map(e=>e.getBoundingClientRect());
            return [...document.querySelectorAll('.player:not(.player-top) .exposed .card')].some(e=>{
              const r=e.getBoundingClientRect();
              return upper.some(u=>r.left<u.right && r.right>u.left && r.top<u.bottom && r.bottom>u.top);
            });
          });
          expect(overlaps, `upper/side cards at ${width}x${height}`).toBe(false);
        }
        const gap = await page.evaluate(() => document.querySelector('.player-top')!.getBoundingClientRect().top - document.querySelector('.table-heading')!.getBoundingClientRect().top);
        expect(Math.abs(gap), `upper seat heading gap at ${width}x${height}`).toBeLessThan(2);
        await expect(page.locator('.player-info p, .player-info small, .top-card-stats p, .top-card-stats small')).toHaveCount(0);
        const centers = await page.evaluate(() => {
          const cards = document.querySelector('.player-top .player-cards')!.getBoundingClientRect();
          const counter = document.querySelector('.player-top .top-card-stats > span')!.getBoundingClientRect();
          return Math.abs(cards.top+cards.height/2-counter.top-counter.height/2);
        });
        expect(centers).toBeLessThan(3);
        const counters = await page.locator('.seat-marks > [title="Взяток"]:visible, .own-taken:visible, .top-card-stats > [title="Взяток"]:visible').evaluateAll(es => es.map(e => { const r=e.getBoundingClientRect(); return {w:r.width,h:r.height,font:getComputedStyle(e).fontSize}; }));
        expect(counters).toHaveLength(4);
        expect(counters.every(c => Math.abs(c.w-counters[0].w)<1 && Math.abs(c.h-counters[0].h)<1 && c.font===counters[0].font)).toBeTruthy();
      }
    }
  }
});

test('single whister drags cards from the passed hand at its seat', async ({page}) => {
  await page.route('http://signaling.example.org:8080/**', route => route.fulfill({json:{}}));
  await page.goto(url);
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  let played: number[] = [];
  await page.route('**/api', async route => {
    const req = route.request().postDataJSON();
    if(req.action === 'command' && req.command.action === 'play') {
      played = req.command.cards;
      return route.fulfill({json:{data:null}});
    }
    const response = await route.fetch(), body = await response.json();
    if(req.action === 'status') {
      Object.assign(body.data.view,{stage:'play',revision:900+played.length,round:1,actor:0,turn:1,open:true,declarer:2,actions:['play'],hand:[24,25],playHand:played.length?[1]:[0,1],legal:[0],trick:played.length?[{seat:1,card:0}]:[],defence:[2,1,0]});
      body.data.view.players[1].cards=played.length?[1]:[0,1];
    }
    await route.fulfill({response,json:body});
  });
  await expect(page.locator('.player.active .exposed [data-card="0"]')).toBeVisible();
  await expect(page.locator('.hand [data-card]')).toHaveCount(0);
  await expect(page.locator('.player.active .exposed [data-card="1"]')).toBeDisabled();
  await dragCard(page,page.locator('.player.active .exposed [data-card="0"]'),page.locator('.playing-field .center'));
  await expect.poll(()=>played).toEqual([0]);
});

test('startup waits for an explicit join even when a table exists', async ({page}) => {
  let searches = 0;
  await page.route('http://signaling.example.org:8080/**', route => {
    searches++;
    return route.fulfill({json:{Offer:{kind:'offer',code:'test-offer',seat:1,name:'Host'}}});
  });
  await page.route('**/api', async route => {
    if (route.request().postDataJSON().action === 'join') return route.fulfill({json:{error:'Test connection stopped'}});
    await route.fallback();
  });
  await page.goto(url);
  await expect(page.locator('#create-form')).toBeVisible();
  await page.waitForTimeout(1700);
  expect(searches).toBe(0);
  await expect(page.locator('#finish-party')).toBeHidden();
  await page.getByRole('button',{name:'Присоединиться →'}).click();
  await expect.poll(()=>searches).toBeGreaterThan(0);
});

test('table agreements save before creation and freeze on start', async ({page}) => {
  await page.route('http://signaling.example.org:8080/**', route => route.fulfill({json:{Offer:{kind:''},Answer:{kind:''}}}));
  await page.goto(url);
  await expect(page.locator('#finish-party')).toBeHidden();
  await page.locator('[data-panel="settings"]').first().click();
  await page.locator('[name="rule-end"]').selectOption('time');
  await expect(page.locator('[name="rule-target"]')).toBeHidden();
  await expect(page.locator('[name="rule-minutes"] option')).toHaveText(['30','45','60','90','120']);
  await expect(page.locator('[name="rule-target"] option')).toHaveText(['10','20','30','40','50']);
  await page.locator('[name="rule-minutes"]').selectOption('45');
  await page.locator('[name="rule-end"]').selectOption('pool');
  await expect(page.locator('[name="rule-minutes"]')).toBeHidden();
  await expect(page.locator('[name="rule-target"]')).toBeVisible();
  await page.locator('[name="rule-end"]').selectOption('time');
  await expect(page.locator('[name="rule-minutes"]')).toHaveValue('45');
  await page.locator('[name="rule-prices"]').fill('1 2 4');
  await page.locator('[name="rule-exit"]').selectOption('8');
  await page.locator('[name="rule-stalingrad"]').uncheck();
  const exit = await page.locator('[name="rule-exit"]').boundingBox();
  const here = await page.locator('[name="rule-here"]').boundingBox();
  // These controls share a grid column; intervening agreements can change rows.
  expect(Math.abs(exit!.x-here!.x)).toBeLessThan(1);
  expect(Math.abs(exit!.width-here!.width)).toBeLessThan(1);
  await page.locator('#panel').screenshot({path:'../.build/rules-settings.png'});
  await page.getByRole('button',{name:'Сохранить',exact:true}).click();
  await expect(page.locator('#panel')).not.toBeVisible();
  await page.locator('[data-panel="rules"]').first().click();
  await expect(page.locator('#panel-body select, #panel-body input')).toHaveCount(0);
  await expect(page.locator('#panel-body')).toContainText('Время, минут');
  await page.locator('#panel').screenshot({path:'../.build/rules-text.png'});
  await page.keyboard.press('Escape');
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toContainText('Время 45 мин');
  await expect(page.locator('.center.lobby')).toContainText('распасы 1–2–4');
  await page.getByRole('button',{name:'Готов',exact:true}).click();
  await page.locator('[data-panel="settings"]').first().click();
  await page.locator('[name="rule-whist"]').selectOption('greedy');
  await page.getByRole('button',{name:'Сохранить',exact:true}).click();
  await expect(page.getByRole('button',{name:'Готов',exact:true})).toBeVisible();
  await page.getByRole('button',{name:'Готов',exact:true}).click();
  await page.getByRole('button',{name:'Начать партию',exact:true}).click();
  await expect(page.locator('#table-progress')).toContainText('Время');
  await expect(page.locator('.pool-total')).toHaveText('45');
  await expect(page.locator('.table-announcement .dealer-hand .card-back')).toHaveCount(2);
  await expect(page.locator('.player .dealer-hand')).toHaveCount(0);
  await page.screenshot({path:'../.build/table-announcement.png'});
  const initial = await page.locator('#table-progress').textContent();
  await expect(page.locator('#table-progress')).not.toHaveText(initial!);
  await page.locator('[data-panel="settings"]').first().click();
  await expect(page.locator('[name="rule-whist"]')).toHaveValue('greedy');
  await expect(page.locator('[name="rule-whist"]')).toBeDisabled();
  await expect(page.locator('[name="rule-stalingrad"]')).not.toBeChecked();
});

test('round summary closes by timer, tap and cross; turn follows the played hand', async ({page}) => {
  await page.goto(url);
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  let stage = 'round', round = 1, revision = 1000;
  await page.route('**/api', async route => {
    const response = await route.fetch(), body = await response.json();
    if (route.request().postDataJSON().action === 'status') {
      body.data.roomMode=false;
      Object.assign(body.data.view, {stage, round, revision, actor:0, turn:1, actions:[], trick:[{seat:0,card:0},{seat:1,card:1},{seat:2,card:2}], hand:[3,4,5], pool:[0,0,0], mountain:[0,0,0], results:[0,0,0]});
    }
    await route.fulfill({response,json:body});
  });
  await page.reload();
  await expect(page.locator('[data-dismiss-summary]')).toBeVisible();
  await expect(page.locator('[data-dismiss-summary]')).toHaveCount(0, {timeout:3000});
  await expect(page.locator('.trick-cards .card')).toHaveCount(0);
  revision++;
  await page.waitForTimeout(700);
  await expect(page.locator('[data-dismiss-summary]')).toHaveCount(0);
  round++; revision++;
  await expect(page.locator('[data-close-summary]')).toBeVisible();
  await page.locator('[data-close-summary]').click();
  await expect(page.locator('[data-dismiss-summary]')).toHaveCount(0);
  round++; revision++;
  await expect(page.locator('.round-summary h2')).toBeVisible();
  await page.locator('.round-summary h2').click();
  await expect(page.locator('[data-dismiss-summary]')).toHaveCount(0);
  stage = 'play'; revision++;
  await expect(page.locator('.player.active')).toContainText('Бот 1');
  await expect(page.locator('.turn-message')).toContainText('Ход: Бот 1');
  await expect(page.locator('.active-hand')).toHaveCount(0);
  await expect(page.locator('.trick-cards .card')).toHaveCount(3);
  stage = 'auction'; revision++;
  await expect(page.locator('body')).toHaveAttribute('data-stage', 'auction');
  const first = page.locator('.hand .card').first();
  const before = await first.boundingBox();
  const background = await first.evaluate(e => getComputedStyle(e).backgroundColor);
  await first.hover({position:{x:8,y:20}});
  await page.waitForTimeout(250);
  const after = await first.boundingBox();
  expect(after!.y).toBe(before!.y);
  expect(await first.evaluate(e => getComputedStyle(e).backgroundColor)).toBe(background);
  await expect(page.locator('.hand [data-card]')).toHaveCount(0);
});

test('reference open hands form vertical columns and score stays in sectors', async ({page}) => {
  await page.route('http://signaling.example.org:8080/**', route => route.fulfill({json:{}}));
  await page.goto(url);
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await page.getByRole('button',{name:'Готов',exact:true}).click();
  await page.getByRole('button',{name:'Начать партию',exact:true}).click();
  await expect(page.locator('.hand .card')).toHaveCount(10);
  // A fixed view fixture tests rendering independently of random deals and bot choices.
  await page.route('**/api', async route => {
    const response = await route.fetch();
    const body = await response.json();
    if (route.request().postDataJSON().action === 'status') {
      const v = body.data.view;
      Object.assign(v,{revision:1000,stage:'play',actor:0,turn:0,open:true,contract:{level:6,suit:3,misere:false},declarer:0,actions:['play'],trick:[],pool:[2,0,0],mountain:[0,0,0],whists:[[0,0,0],[16,0,0],[0,0,0]],results:[30,9,-39],defence:[0,2,1]});
      v.players[1].cards = [0,1,7,8,9,16,18,24,26,31];
      v.players[2].cards = [2,3,6,10,11,17,19,25,27,30];
      v.playHand = v.hand; v.legal = v.hand;
    }
    await route.fulfill({response,json:body});
  });
  await page.reload();
  await expect(page.locator('.pool-balance')).toHaveText([/10$/,/3$/,/-13$/]);
  await expect(page.locator('.pool-points')).toHaveText([/2$/, /0$/, /0$/]);
  const pool = page.locator('.pool-drawing');
  const poolBackground = await pool.evaluate(e => getComputedStyle(e).backgroundColor);
  await pool.hover();
  await page.waitForTimeout(200);
  expect(await pool.evaluate(e => getComputedStyle(e).backgroundColor)).toBe(poolBackground);
  await pool.click();
  await expect(page.locator('#panel')).toContainText('Пуля и расчёт');
  await page.getByRole('button', {name:'Закрыть', exact:true}).click();
  for (const [width,height] of [[1440,900],[1000,600],[844,390],[390,844]]) {
    await page.setViewportSize({width,height});
    const columns = await page.locator('.exposed').evaluateAll(es=>es.map(e=>Array.from(e.children).map(g=>{
      const r=g.getBoundingClientRect(); return {x:r.x,y:r.y,bottom:r.bottom};
    })));
    for (const groups of columns) {
      expect(groups.length).toBe(4);
      expect(groups.every((g,i)=>!i || g.y>groups[i-1].y)).toBeTruthy();
      expect(groups.every(g=>g.bottom<=height)).toBeTruthy();
    }
    await page.screenshot({path:`../.build/reference-open-${width}.png`});
  }
});

for (const [mode,label] of [['pass','Распасы'],['misere','Мизер']]) test(`debug bots ${mode} selected before readiness`, async ({page}) => {
  await page.route('http://signaling.example.org:8080/**', route=>route.fulfill({json:{}}));
  await page.goto(url);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.locator('[name="debug-auction"]').check();
  await page.locator('[name="debug-play"]').check();
  await page.locator('#settings-form > button.primary').click();
  await expect(page.locator('#panel')).not.toBeVisible();
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await page.locator('[data-panel="debug-bots"]').click();
  await page.locator(`[data-debug-bots="${mode}"]`).click();
  await expect(page.locator('[data-panel="debug-bots"]')).toContainText(label);
  await page.getByRole('button',{name:'Готов',exact:true}).click();
  await expect(page.locator('[data-panel="debug-bots"]')).toHaveCount(0);
  await page.getByRole('button',{name:'Начать партию',exact:true}).click();
  await page.locator('[data-action="pass"]').click();
  if (mode==='misere') {
    // Step the debug preset through bot bidding, discard and catcher selection.
    for(let i=0;i<30;i++) {
      const response=await page.request.post(new URL('/api',url).href,{headers:{'X-Preferans-Token':new URL(url).hash.slice(1)},data:{action:'status'}});
      const view=(await response.json()).data.view;
      if(view.debugWaitingSeat==null && view.actions?.includes('catch'))break;
      if(view.debugWaitingSeat!=null) await page.request.post(new URL('/api',url).href,{headers:{'X-Preferans-Token':new URL(url).hash.slice(1)},data:{action:'debug-bot-continue',seat:view.debugWaitingSeat,command:{revision:view.revision}}});
      await page.waitForTimeout(150);
    }
    await expect(page.locator('[data-action="catch"]')).toHaveText('Ловлю');
    await expect(page.locator('[data-action="trust"]')).toHaveText('Доверяю');
    await page.locator('[data-action="catch"]').click();
  }
  for(let i=0;i<30;i++) {
    const response=await page.request.post(new URL('/api',url).href,{headers:{'X-Preferans-Token':new URL(url).hash.slice(1)},data:{action:'status'}});
    const view=(await response.json()).data.view;
    if(view.stage==='play')break;
    if(view.debugWaitingSeat!=null) await page.request.post(new URL('/api',url).href,{headers:{'X-Preferans-Token':new URL(url).hash.slice(1)},data:{action:'debug-bot-continue',seat:view.debugWaitingSeat,command:{revision:view.revision}}});
    await page.waitForTimeout(150);
  }
  await expect(page.locator('body')).toHaveAttribute('data-stage','play');
  await expect(page.locator('.turn-message')).toContainText(label);
});

test('three player pass talon stays separate from player trick cards', async ({page}) => {
  await page.route('http://signaling.example.org:8080/**', route=>route.fulfill({json:{}}));
  await page.goto(url);
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  let trickNo=0;
  await page.route('**/api',async route=>{
    const response=await route.fetch(),body=await response.json();
    if(route.request().postDataJSON().action==='status') Object.assign(body.data.view,{
      revision:700+trickNo,stage:'play',round:1,allPass:true,passPrice:2,trickNo,
      talon:trickNo===0?[7]:[7,15],trick:[{seat:-1,card:trickNo===0?7:15},{seat:0,card:0},{seat:1,card:1}],
      hand:[2,3,4],legal:[],actions:[],turn:2,actor:2
    });
    await route.fulfill({response,json:body});
  });
  const talon=page.locator('.table-announcement .dealer-hand');
  await expect(talon.locator('.card')).toHaveCount(1);
  await expect(talon.locator('.card-back')).toHaveCount(1);
  await expect(page.locator('.trick-cards .card')).toHaveCount(2);
  const first=await talon.boundingBox();
  trickNo=1;
  await expect(talon.locator('.card')).toHaveCount(1);
  await expect(talon.locator('.card-back')).toBeHidden();
  await expect(page.locator('.trick-cards .card')).toHaveCount(2);
  const second=await talon.boundingBox();
  expect(Math.abs(first!.x-second!.x)).toBeLessThan(1);
  expect(Math.abs(first!.y-second!.y)).toBeLessThan(1);
  trickNo=2;
  await expect(talon).toHaveCount(0);
});

test('four player pass talon keeps the second card at the dealer until its turn', async ({page}) => {
  await page.route('http://signaling.example.org:8080/**', route=>route.fulfill({json:{}}));
  await page.goto(url);
  await page.locator('[name="players"]').selectOption('4');
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  let trickNo=0, taken=0, collecting=false;
  await page.route('**/api',async route=>{
    const response=await route.fetch(),body=await response.json();
    if(route.request().postDataJSON().action==='status') Object.assign(body.data.view,{
      revision:800+trickNo+taken*10+Number(collecting)*100,stage:collecting?'trick':'play',round:1,dealer:2,allPass:true,passPrice:2,trickNo,
      taken:[0,0,taken,0],winner:2,
      talon:trickNo===0?[7]:[7,15],trick:[{seat:2,card:trickNo===0?7:15},{seat:0,card:0}],
      hand:[2,3,4],legal:[],actions:[],turn:1,actor:1
    });
    await route.fulfill({response,json:body});
  });
  const talon=page.locator('.player-top .dealer-hand');
  await expect(talon.locator('[aria-label="Закрытая карта прикупа"]')).toBeVisible();
  await expect(talon.locator('.card')).toHaveCount(0);
  await expect(page.locator('.trick-cards .card')).toHaveCount(2);
  taken=1;
  collecting=true;
  await expect(page.locator('body')).toHaveAttribute('data-stage','trick');
  await expect(talon.locator('.collected-tricks')).toHaveCount(0);
  trickNo=1;
  collecting=false;
  await expect(talon.locator('.collected-tricks')).toHaveAttribute('aria-label','Взятки сдающего: 1');
  await expect(talon.locator('.collected-tricks .card-back')).toHaveCount(4);
  await expect(talon.locator('.card')).toHaveCount(0);
  trickNo=2;
  taken=2;
  await expect(talon.locator('.collected-tricks')).toHaveAttribute('aria-label','Взятки сдающего: 2');
  await expect(talon.locator('.collected-tricks .card-back')).toHaveCount(6);
  taken=0;
  await expect(talon).toHaveCount(0);
  await expect(page.locator('.trick-cards .card')).toHaveCount(2);
});

for (const rejected of [false,true]) test(`touch drop keeps card out of hand while command is pending: rejected=${rejected}`, async ({page}) => {
  await page.emulateMedia({reducedMotion:'reduce'});
  await page.route('http://signaling.example.org:8080/**',route=>route.fulfill({json:{}}));
  await page.goto(url);
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  let played=false, received=false;
  let release!:()=>void;
  const gate=new Promise<void>(resolve=>release=resolve);
  await page.route('**/api',async route=>{
    const req=route.request().postDataJSON();
    if(req.action==='command' && req.command.action==='play') {
      received=true; await gate; played=!rejected;
      return route.fulfill({json:rejected?{ok:false,error:'Ход отклонён'}:{ok:true}});
    }
    const response=await route.fetch(),body=await response.json();
    if(req.action==='status') Object.assign(body.data.view,{revision:900+Number(played),stage:'play',round:1,seat:0,actor:0,turn:0,
      hand:played?[1,2]:[0,1,2],playHand:played?[1,2]:[0,1,2],legal:[0],actions:['play'],trick:played?[{seat:0,card:0}]:[]});
    await route.fulfill({response,json:body});
  });
  const source=page.locator('.hand [data-card="0"]');
  await expect(source).toBeVisible();
  const from=await source.boundingBox(),to=await page.locator('.center').boundingBox();
  const start={pointerId:7,pointerType:'touch',isPrimary:true,button:0,clientX:from!.x+8,clientY:from!.y+20};
  await source.dispatchEvent('pointerdown',start);
  await source.dispatchEvent('pointermove',{...start,clientX:to!.x+to!.width/2,clientY:to!.y+to!.height/2});
  await source.dispatchEvent('pointerup',{...start,clientX:to!.x+to!.width/2,clientY:to!.y+to!.height/2});
  await expect.poll(()=>received).toBe(true);
  await expect(page.locator('.drag-ghost')).toBeVisible();
  await expect(source).toBeHidden();
  release();
  if(!rejected) {
    await expect.poll(()=>page.locator('.drag-ghost').evaluateAll(els=>els.some(el=>el.getAnimations().length>0))).toBe(true);
    const travel=await page.locator('.drag-ghost').evaluate(el=>{
      const animation=el.getAnimations()[0];
      animation.pause(); animation.currentTime=0;
      const start=el.getBoundingClientRect();
      animation.currentTime=210;
      const middle=el.getBoundingClientRect();
      animation.play();
      return {distance:Math.hypot(middle.x-start.x,middle.y-start.y),duration:animation.effect!.getTiming().duration};
    });
    expect(travel.duration).toBe(420);
    expect(travel.distance).toBeGreaterThan(5);
    await expect(page.locator('.trick-cards .card')).toBeHidden();
  }
  await expect(page.locator('.drag-ghost')).toHaveCount(0);
  if(rejected) await expect(source).toBeVisible();
  else {
    await expect(source).toHaveCount(0);
    await expect(page.locator('.trick-cards .card')).toHaveCount(1);
  }
});

for (const level of [7, 10]) test(`claim controls show all remaining results and defender responses: ${level}`, async ({page}) => {
  await page.route('http://signaling.example.org:8080/**', route=>route.fulfill({json:{}}));
  await page.goto(url);
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await expect(page.locator('.center.lobby')).toBeVisible();
  let responder=false, pending=false, submitted=-1;
  await page.route('**/api', async route=>{
    const request=route.request().postDataJSON();
    if(request.action==='command' && ['claim','reject-claim'].includes(request.command.action)) {
      pending=request.command.action==='claim';submitted=request.command.tricks ?? submitted;
      return route.fulfill({json:{ok:true}});
    }
    const response=await route.fetch(),body=await response.json();
    if(request.action==='status') {
      Object.assign(body.data.view,{stage:'play',revision:950+Number(pending)+Number(responder)*2,seat:0,actor:pending?1:0,turn:0,declarer:responder?1:0,contract:{level,suit:1},trickNo:6,taken:responder?[2,4,0]:[4,2,0],hand:[0,1,2,3],playHand:[0,1,2,3],legal:[0],trick:[],claim:pending?3:null,actions:pending?(responder?['accept-claim','reject-claim']:[]):['play','claim']});
    }
    await route.fulfill({response,json:body});
  });
  await page.getByRole('button',{name:'Предложить',exact:true}).click();
  await expect(page.locator('#claim-tricks option')).toHaveCount(5);
  await expect(page.locator('#claim-tricks option[value="3"]')).toHaveText('Беру ещё 3 взятки (всего 7)');
  await page.locator('#claim-tricks').selectOption('3');
  await page.locator('[data-action="submit-claim"]').click();
  await expect.poll(()=>submitted).toBe(3);
  await expect(page.locator('.action-area')).toContainText('Ожидаем согласия');
  responder=true;
  await expect(page.getByRole('button',{name:'Принять',exact:true})).toBeVisible();
  await expect(page.getByRole('button',{name:'Отказаться',exact:true})).toBeVisible();
  expect(await page.locator('.claim-text').evaluate(e=>parseFloat(getComputedStyle(e).fontSize))).toBeGreaterThanOrEqual(16);
  await page.getByRole('button',{name:'Отказаться',exact:true}).click();
  await expect(page.getByRole('button',{name:'Предложить',exact:true})).toBeVisible();
});

test('four player talon toggle skips manual discard without scrolling', async ({page}) => {
  await page.route('http://signaling.example.org:8080/**', route => route.fulfill({json:{}}));
  await page.goto(url);
  await page.locator('[name="players"]').selectOption('4');
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await page.getByRole('button',{name:'Готов',exact:true}).click();
  await page.getByRole('button',{name:'Начать партию',exact:true}).click();
  await page.locator('[data-action="bid"]').click();
  await page.getByRole('button',{name:'8 без козыря',exact:true}).click();
  const toggle = page.getByRole('button',{name:'Прикуп в морду',exact:true});
  await expect(toggle).toBeVisible();
  for (const [width,height] of [[1440,900],[1100,800],[1000,600]]) {
    await page.setViewportSize({width,height});
    await expect(page.locator('[data-action="declare"]')).toBeDisabled();
    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-pressed','true');
    await expect(page.locator('.discard-tray')).toHaveCount(0);
    await expect(page.locator('.hand .card')).toHaveCount(10);
    await expect(page.locator('[data-action="declare"]')).toBeEnabled();
    const layout = await page.locator('.action-area').evaluate(e => ({overflow:getComputedStyle(e).overflowY,bottom:e.getBoundingClientRect().bottom}));
    expect(layout.overflow).toBe('visible');
    expect(layout.bottom).toBeLessThanOrEqual(height);
    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-pressed','false');
    await expect(page.locator('.hand .card')).toHaveCount(12);
    await expect(page.locator('.discard-tray')).toBeVisible();
  }
  await toggle.click();
  await page.locator('[data-action="declare"]').click();
  await page.getByRole('button',{name:'8 без козыря',exact:true}).click();
  await expect(page.locator('body')).not.toHaveAttribute('data-stage','discard');
});

for (const players of [3,4]) test(`reference settings, contract grid and reversible discard: ${players}`, async ({page}) => {
  await page.goto(url);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await expect(page.getByRole('tab',{name:'Ленинград'})).toHaveAttribute('aria-selected','true');
  for (const name of ['Сочи','Классика','Ростов']) await expect(page.getByRole('tab',{name:new RegExp(name)})).toBeDisabled();
  await expect(page.locator('[name="rule-prices"]')).toHaveValue('2 4 8');
  await page.screenshot({path:'../.build/reference-settings.png'});
  await page.getByRole('button',{name:'Закрыть',exact:true}).click();
  await page.locator('[name="players"]').selectOption(String(players));
  await page.getByLabel('Боты для отладки').check();
  await page.getByRole('button',{name:'Создать стол →'}).click();
  await page.getByRole('button',{name:'Готов',exact:true}).click();
  await page.getByRole('button',{name:'Начать партию',exact:true}).click();
  await page.locator('[data-action="bid"]').click();
  await expect(page.locator('.contract-cell')).toHaveCount(25);
  await page.screenshot({path:'../.build/reference-contract.png'});
  await page.getByRole('button',{name:'8 без козыря',exact:true}).click();
  await expect(page.locator('.discard-tray')).toBeVisible();
  await expect(page.locator('.revealed-talon .card')).toHaveCount(2);
  await expect(page.locator('.hand .card')).toHaveCount(12);
  const readRevision = () => page.evaluate(async () => {
    const r = await fetch('/api',{method:'POST',headers:{'Content-Type':'application/json','X-Preferans-Token':sessionStorage.getItem('preferans-token')!},body:JSON.stringify({action:'status'})});
    return (await r.json()).data.view.revision;
  });
  const revision = await readRevision();
  // The complete tray, including its corners and caption, accepts drops.
  for(const [x,y] of [[.05,.1],[.95,.1],[.05,.9],[.95,.9],[.5,.5]]) {
    const a=(await page.locator('.hand [data-card]').first().boundingBox())!;
    const b=(await page.locator('.discard-tray').boundingBox())!;
    await page.mouse.move(a.x+8,a.y+20);
    await page.mouse.down();
    await page.mouse.move(b.x+b.width*x,b.y+b.height*y,{steps:8});
    await page.mouse.up();
    await expect(page.locator('.discard-tray .card')).toHaveCount(1);
    await expect(page.locator('.drag-ghost')).toHaveCount(0);
    await dragCard(page,page.locator('.discard-tray [data-card]').first(),page.locator('.hand'));
    await expect(page.locator('.discard-tray .card')).toHaveCount(0);
    await expect(page.locator('.drag-ghost')).toHaveCount(0);
  }
  await page.locator('.hand [data-card]').first().click({position:{x:8,y:20}});
  await expect(page.locator('.discard-tray .card')).toHaveCount(0);
  await dragCard(page,page.locator('.hand [data-card]').first(),page.locator('.topbar'));
  await expect(page.locator('.hand .card')).toHaveCount(12);
  await dragCard(page,page.locator('.hand [data-card]').first(),page.locator('.discard-tray'));
  await expect(page.locator('.discard-tray .card')).toHaveCount(1);
  await expect(page.locator('[data-action="declare"]')).toBeDisabled();
  await dragCard(page,page.locator('.discard-tray [data-card]').first(),page.locator('.hand'));
  await expect(page.locator('.hand .card')).toHaveCount(12);
  // Exercise the same path with touch PointerEvents (Android WebView input).
  await page.evaluate(() => {
    const source=document.querySelector('.hand [data-card]')!, target=document.querySelector('.discard-tray')!;
    const a=source.getBoundingClientRect(), b=target.getBoundingClientRect();
    const init={pointerId:77,pointerType:'touch',isPrimary:true,button:0,buttons:1,bubbles:true,cancelable:true};
    source.dispatchEvent(new PointerEvent('pointerdown',{...init,clientX:a.x+8,clientY:a.y+20}));
    document.dispatchEvent(new PointerEvent('pointermove',{...init,clientX:b.x+b.width/2,clientY:b.y+b.height/2}));
    document.dispatchEvent(new PointerEvent('pointerup',{...init,buttons:0,clientX:b.x+b.width/2,clientY:b.y+b.height/2}));
  });
  await expect(page.locator('.discard-tray .card')).toHaveCount(1);
  await expect(page.locator('.drag-ghost')).toHaveCount(0);
  await dragCard(page,page.locator('.hand [data-card]').first(),page.locator('.discard-tray'));
  await expect(page.locator('.hand .card')).toHaveCount(10);
  await expect(page.locator('.discard-tray .card')).toHaveCount(2);
  await dragCard(page,page.locator('.hand [data-card]').first(),page.locator('.discard-tray'));
  await expect(page.locator('.discard-tray .card')).toHaveCount(2);
  await page.screenshot({path:'../.build/reference-discard.png'});
  await expect(page.locator('.discard-tray .card')).toHaveCount(2);
  expect(await readRevision()).toBe(revision);
  await page.locator('[data-action="declare"]').click();
  expect(await readRevision()).toBe(revision);
  await page.locator('.contract-cell:enabled').first().click();
  await expect(page.locator('#panel')).not.toBeVisible();
  await expect.poll(readRevision).toBeGreaterThan(revision);
});

test("fill empty seats and manage saves from compact selector", async ({ page }) => {
  await page.goto(url);
  await page.getByRole("button", { name: "Создать стол →" }).click();
  await expect(page.locator('[data-action="fill-bots"] svg')).toBeVisible();
  await expect(page.locator('[data-action="fill-bots"]')).toHaveText('');
  await page.getByRole("button", { name: "Заполнить свободные места ботами" }).click();
  await expect(page.getByRole("button", { name: "Заполнить свободные места ботами" })).toHaveCount(0);
  await expect(page.locator(".opponents")).toContainText("Бот 1");
  await page.evaluate(async () => {
    const rpc = async (action: string, params = {}) => fetch('/api', {
      method: 'POST', headers: {'Content-Type':'application/json', 'X-Preferans-Token': sessionStorage.getItem('preferans-token')!},
      body: JSON.stringify({action,...params}),
    });
    await rpc('leave');
    for (let i = 0; i < 12; i++) {
      await rpc('create', {players:3,target:30,name:`Партия ${i}`});
      await rpc('leave');
    }
  });
  await page.reload();
  await expect(page.locator('#saved-party option')).toHaveCount(13);
  await expect(page.locator('#saved-party')).toBeVisible();
  await page.getByRole('button', {name:'Удалить выбранную'}).click();
  await expect(page.locator('#saved-party option')).toHaveCount(12);
  await page.reload();
  await expect(page.locator('#saved-party option')).toHaveCount(12);
  await page.getByRole('button', {name:'Продолжить', exact:true}).click();
  await expect(page.getByRole('heading', {name:'Ваш стол'})).toBeVisible();
});
test("create, trade, play, score and responsive layout", async ({ page }) => {
  test.setTimeout(150000);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto(url);
  await expect(
    page.getByRole("heading", { name: "Создать стол" }),
  ).toBeVisible();
  await page.getByLabel("Ваше имя").fill("Тестовый игрок");
  await page.getByLabel("Боты для отладки").check();
  await page.getByRole("button", { name: "Создать стол →" }).click();
  await expect(page.getByRole("heading", { name: "Ваш стол" })).toBeVisible();
  await page.getByRole("button", { name: "Готов", exact: true }).click();
  await page
    .getByRole("button", { name: "Начать партию", exact: true })
    .click();
  await expect(page.locator('[data-action="bid"]')).toBeVisible();
  await expect(page.locator(".pool-drawing")).toBeVisible();
  await page.locator(".pool-drawing").click();
  await expect(page.locator("#panel")).toBeVisible();
  await page.getByRole("button", { name: "Закрыть", exact: true }).click();
  const displayedSuits = await page
    .locator(".hand .card")
    .evaluateAll((es) =>
      es.map((e) => e.getAttribute("aria-label")!.split(" ").at(-1)),
    );
  const suitOrder = ["пики", "трефы", "бубны", "червы"];
  expect(displayedSuits.map((s) => suitOrder.indexOf(s!))).toEqual(
    displayedSuits.map((s) => suitOrder.indexOf(s!)).sort((a, b) => a - b),
  );
  await page.locator('[data-action="bid"]').click();
  await page.getByRole('button',{name:'8 без козыря',exact:true}).click();
  await expect(page.locator('.discard-tray')).toBeVisible();
  // Search-based bots can take longer on shared CI runners. Observe the
  // persistent ledger, rather than racing the automatically dismissed summary.
  const endpoint = new URL('/api', url).href;
  const headers = {'X-Preferans-Token': new URL(url).hash.slice(1)};
  const roundHistory = async () => {
    const response = await page.request.post(endpoint, {headers, data: {action: 'status'}});
    return (await response.json()).data.view.history ?? [];
  };
  const deadline = Date.now() + 110000;
  while (Date.now() < deadline) {
    if ((await roundHistory()).length) break;
    if (await page.locator('.discard-tray').count()) {
      await dragCard(page,page.locator(".hand [data-card]").first(),page.locator('.discard-tray'));
      await dragCard(page,page.locator(".hand [data-card]").first(),page.locator('.discard-tray'));
      await expect(page.locator('.discard-tray .card')).toHaveCount(2);
      await page.locator('[data-action="declare"]').click();
      await page.locator('.contract-cell:enabled').first().click();
      await expect(page.locator('#panel')).not.toBeVisible();
      await expect(page.locator('[data-action="declare"]')).toHaveCount(0);
    } else if (await page.locator('[data-action="bid"]').count()) {
      await page.locator('.action-area [data-action="pass"]').click();
    } else if (await page.locator('[data-action="open"]').count()) {
      await page.locator('[data-action="open"]').click();
    } else if (await page.locator('[data-action="whist"]').count()) {
      await page.locator('[data-action="whist"]').click();
    } else if (await page.locator('.hand [data-card]:enabled').count()) {
      await dragCard(page,page.locator(".hand [data-card]:enabled").first(),page.locator('.playing-field .center'));
    }
    await page.waitForTimeout(250);
  }
  await expect.poll(async () => (await roundHistory()).length).toBeGreaterThan(0);
  // Recorded results remain accessible after the temporary summary disappears.
  await page.locator('.table-heading [data-panel="score"]').click();
  await expect(page.locator("#panel")).toContainText("История раздач");
  await page.getByRole("button", { name: "Закрыть", exact: true }).click();
    await expect(page.locator('[data-dismiss-summary]')).toHaveCount(0, {timeout: 3000});
  // The next deal starts automatically after the summary delay.
  await expect(page.locator('[data-action="bid"]')).toBeVisible();
  for (const [width, height] of [
    [1440, 900],
    [1200, 600],
    [1000, 480],
    [844, 390],
  ]) {
    await page.setViewportSize({ width, height });
    await page.waitForTimeout(150);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBeTruthy();
    const cards = await page.locator(".hand .card").evaluateAll((es) =>
      es.map((el) => {
        const r = el.getBoundingClientRect();
        return { left: r.left, right: r.right };
      }),
    );
    expect(cards.every((r) => r.left >= 0 && r.right <= width)).toBeTruthy();
    expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight)).toBeTruthy();
    const controls = await page.locator('.action-area button').evaluateAll(es => es.map(el => {
      const r = el.getBoundingClientRect();
      const hit = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2);
      return r.top >= 0 && r.bottom <= innerHeight && (hit === el || el.contains(hit));
    }));
    expect(controls.length).toBeGreaterThan(0);
    expect(controls.every(Boolean)).toBeTruthy();
    await page.screenshot({
      path: `../.build/ui-${width}.png`,
      fullPage: true,
    });
  }
  await page.reload();
  await expect(page.locator(".table-heading")).toBeVisible();
  expect(errors).toEqual([]);
});
test("connection settings and invalid invitation API", async ({ page }) => {
  const response=await page.request.post(new URL('/api',url).href,{
    headers:{'X-Preferans-Token':new URL(url).hash.slice(1)},
    data:{action:'join',code:'bad-code'},
  });
  expect((await response.json()).error).toContain('Неверный код');
  await page.goto(url);
  await page.getByRole("button", {name:"Настройки",exact:true}).click();
  await page.getByRole("tab", {name:"Подключение",exact:true}).click();
  await page.locator('[name="stun"]').locator('xpath=ancestor::details').locator('summary').click();
  await page.locator('[name="stun"]').fill("");
  await page.getByRole("button", {name:"Сохранить",exact:true}).click();
  await expect(page.locator("#panel")).not.toBeVisible();
});

test('all 96 SVG cards load with distinct suit-specific figures', async ({page}) => {
  await page.goto(url);
  const result = await page.evaluate(async () => {
    const decks = ['atlas','english','woodcut'];
    const ranks = ['7','8','9','10','jack','queen','king','ace'];
    const suits = ['spades','clubs','diamonds','hearts'];
    const cards = [];
    for (const deck of decks) for (const rank of ranks) for (const suit of suits) {
      const src = `/cards/${deck}/${rank}-${suit}.svg`;
      const response = await fetch(src);
      const svg = await response.text();
      const doc = new DOMParser().parseFromString(svg,'image/svg+xml');
      const img = new Image(); img.src = src; await img.decode();
      const canvas = document.createElement('canvas'); canvas.width=360; canvas.height=540;
      const ctx=canvas.getContext('2d')!; ctx.drawImage(img,0,0,360,540);
      const framed=canvas.toDataURL();
      const plain=new Image(); plain.src=src+'#no-frame'; await plain.decode();
      ctx.clearRect(0,0,360,540); ctx.drawImage(plain,0,0,360,540);
      cards.push({src, frameChanges:framed!==canvas.toDataURL(), ok:response.ok && img.naturalWidth > 0 && !doc.querySelector('parsererror'), raster:!!doc.querySelector('image'), svg});
    }
    return cards;
  });
  expect(result).toHaveLength(96);
  expect(result.every(c => c.ok && !c.raster)).toBeTruthy();
  expect(new Set(result.map(c => c.svg)).size).toBe(96);
  expect(result.filter(c=>!c.frameChanges).map(c=>c.src)).toEqual([]);
});

test('three SVG deck choices preview and persist', async ({page}) => {
  await page.goto(url);
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.getByRole('tab',{name:'Карты',exact:true}).click();
  await expect(page.locator('[name="deck"]')).toHaveCount(3);
  await expect(page.locator('[name="card-size"]')).toHaveCount(0);
  await page.locator('[name="frame-atlas"]').uncheck();
  await page.locator('[name="frame-woodcut"]').uncheck();
  for(const deck of ['atlas','english','woodcut']) {
    await page.locator(`[name="deck"][value="${deck}"]`).check();
    await expect(page.locator('#deck-preview .card-image')).toHaveCount(4);
    await expect.poll(() => page.locator('#deck-preview .card-image').evaluateAll(es => es.every(e => (e as HTMLImageElement).complete && (e as HTMLImageElement).naturalWidth > 0))).toBeTruthy();
    await page.locator('#deck-preview').screenshot({path:`../.build/deck-${deck}.png`});
  }
  await page.getByRole('button',{name:'Сохранить',exact:true}).click();
  await expect(page.locator('#panel')).not.toBeVisible();
  await page.reload();
  await expect(page.locator('body')).toHaveAttribute('data-deck','woodcut');
  await page.getByRole('button',{name:'Настройки',exact:true}).click();
  await page.getByRole('tab',{name:'Карты',exact:true}).click();
  await expect(page.locator('[name="frame-atlas"]')).not.toBeChecked();
  await expect(page.locator('[name="frame-english"]')).toBeChecked();
  await expect(page.locator('[name="frame-woodcut"]')).not.toBeChecked();
  await expect(page.locator('#deck-preview .card-image').first()).toHaveAttribute('data-card-frame','off');
});

test("four seats around the score sheet", async ({ page }) => {
  await page.goto(url);
  await page.locator('select[name="players"]').selectOption("4");
  await page.getByLabel("Боты для отладки").check();
  await page.getByRole("button", { name: "Создать стол →" }).click();
  await page.getByRole("button", { name: "Готов", exact: true }).click();
  await page
    .getByRole("button", { name: "Начать партию", exact: true })
    .click();
  await expect(page.locator(".pool-drawing")).toBeVisible();
  await expect(page.locator(".pool-drawing .pool-name")).toHaveCount(5);
  for (const width of [1440, 1024]) {
    await page.setViewportSize({ width, height: 900 });
    const boxes = await page
      .locator(".player .card-back, .pool-drawing")
      .evaluateAll((es) =>
        es.map((e) => {
          const r = e.getBoundingClientRect();
          return { l: r.left, r: r.right, t: r.top, b: r.bottom };
        }),
      );
    const pool = boxes.at(-1)!;
    expect(
      boxes
        .slice(0, -1)
        .every(
          (b) =>
            b.r <= pool.l || b.l >= pool.r || b.b <= pool.t || b.t >= pool.b,
        ),
      JSON.stringify({width, boxes})).toBeTruthy();
    await page.screenshot({
      path: `../.build/ui-four-${width}.png`,
      fullPage: true,
    });
  }
});
