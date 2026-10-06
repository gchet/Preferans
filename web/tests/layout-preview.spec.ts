import { test, expect, type Page, type Locator } from "@playwright/test";
import { spawn, ChildProcess } from "node:child_process";
import { resolve } from "node:path";
import { readFile } from 'node:fs/promises';
let processHandle: ChildProcess;
let url: string;
async function dragCard(page: Page, source: Locator, target: Locator) {
  const from = await source.boundingBox(), to = await target.boundingBox();
  if (!from || !to) throw Error('Missing drag source or destination');
  await page.mouse.move(from.x+8, from.y+20);
  await page.mouse.down();
  await page.mouse.move(to.x+to.width/2,to.y+to.height/2,{steps:8});
  await page.mouse.up();
  await expect(page.locator('.drag-ghost')).toHaveCount(0);
}
test.beforeEach(async ({page}) => {
  const root = resolve("..");
  processHandle = spawn(
    resolve(root, ".build/preferans-test.exe"),
    ["-headless", "-update-port", "-1", "-data", resolve(root, `.build/ui-${Date.now()}`)],
    { cwd: root, windowsHide: true },
  );
  url = await new Promise<string>((resolve, reject) => {
    processHandle.stdout!.once("data", (b) => resolve(String(b).trim()));
    processHandle.once("error", reject);
    processHandle.once("exit", (c) => reject(Error(`App exited ${c}`)));
  });
  const endpoint=new URL('/api',url).href;
  const headers={'X-Preferans-Token':new URL(url).hash.slice(1)};
  const state=(await (await page.request.post(endpoint,{headers,data:{action:'status'}})).json()).data;
  await page.request.post(endpoint,{headers,data:{action:'appearance',appearance:{...state.appearance,language:'ru'}}});
  await page.route('**/api',async route=>{
    const response=await route.fetch(),body=await response.json();
    if(route.request().postDataJSON().action==='status')body.data.roomMode=false;
    await route.fulfill({response,json:body});
  });
  // Serve the current built UI with the isolated backend, without relinking Go for CSS edits.
  await page.route('**/*', async route => {
    const requested = new URL(route.request().url());
    if(requested.origin === new URL(url).origin && (requested.pathname==='/' || /^\/assets\/[^/]+\.(js|css)$/.test(requested.pathname))) {
      const file = requested.pathname==='/' ? 'index.html' : requested.pathname.slice(1);
      let body=await readFile(resolve('dist',file));
      if(file==='index.html')body=Buffer.from(body.toString().replace('<html','<html data-language="ru"'));
      return route.fulfill({body,contentType:file.endsWith('.js')?'text/javascript':file.endsWith('.css')?'text/css':'text/html'});
    }
    await route.fallback();
  });
});
test.afterEach(() => {
  processHandle?.kill();
});

for(const players of [3,4]) test(`preview separated suits ${players} players`, async ({page})=>{
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
      body.data.roomMode=false;
      const v=body.data.view;
      Object.assign(v,{stage,revision,round:3,actor:0,turn:0,open:stage==='play',declarer:0,dealer:players-1,actions:stage==='play'?['play']:['pass','bid'],hand:[0,1,7,8,9,16,18,24,26,31],playHand:[0,1,7,8,9,16,18,24,26,31],legal:[0],trick:[],defence:players===3?[0,2,1]:[0,2,1,0]});
      v.players.forEach((p:any,i:number)=>{p.count=i===players-1&&players===4?0:10;p.cards=stage==='play'&&i>0&&p.count?[2,3,6,10,11,17,19,25,27,30]:[];});
    }
    await route.fulfill({response,json:body});
  });
  for(const mode of ['play']){
    stage=mode;revision++;
    await expect(page.locator('body')).toHaveAttribute('data-stage',mode);
    for(const [width,height] of [[1440,1000]]){
      await page.setViewportSize({width,height});
      await page.screenshot({path:`../.build/preview-separated-suits-${players}.png`});
      const sizes=await page.locator('.hand .card,.exposed .card,.player-cards .card-back').evaluateAll(es=>es.map(e=>{const r=e.getBoundingClientRect();return {w:r.width,h:r.height,b:r.bottom,l:r.left,r:r.right};}));
      expect(sizes.length).toBeGreaterThan(10);
      expect(sizes.every(r=>r.w>0 && r.h>0)).toBeTruthy();
      expect(sizes.every(r=>r.b<=height)).toBeTruthy();
      expect(sizes.every(r=>r.l>=0&&r.r<=width),JSON.stringify({width,height,sizes})).toBeTruthy();
      await expect.poll(()=>page.locator('.card-image').evaluateAll(es=>es.every(e=>(e as HTMLImageElement).complete && (e as HTMLImageElement).naturalWidth>0))).toBeTruthy();
    }
  }
});
