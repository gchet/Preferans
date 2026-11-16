import {test, expect, chromium, type Page, type BrowserContext} from '@playwright/test';
import {spawn, type ChildProcess} from 'node:child_process';
import {resolve} from 'node:path';
import {readFile} from 'node:fs/promises';
import {createServer} from 'node:net';

for(const roomsMode of [false]) test('voice falls back to the game channel when WebRTC cannot connect', async () => {
  test.setTimeout(240000);
  const browser = await chromium.launch({executablePath:process.env.CHROME_PATH || (process.platform==='win32'?'C:/Program Files/Google/Chrome/Application/chrome.exe':chromium.executablePath()), args:['--use-fake-ui-for-media-stream','--use-fake-device-for-media-stream','--autoplay-policy=no-user-gesture-required']});
  const processes: ChildProcess[] = [], contexts: BrowserContext[] = [];
  const clients: {page:Page; token:string; url:string}[] = [];
  const rpc = async (i:number, action:string, params:Record<string,unknown>={}) => {
    const c=clients[i];
    const r=await c.page.request.post(new URL('/api',c.url).href,{headers:{'X-Preferans-Token':c.token},data:{action,...params}});
    const body=await r.json();if(body.error)throw Error(body.error);return body.data;
  };
  try {
    let roomsURL='';
    if(roomsMode){
      const port=await new Promise<number>(resolve=>{const s=createServer();s.listen(0,'127.0.0.1',()=>{const p=(s.address() as any).port;s.close(()=>resolve(p));});});
      roomsURL=`http://127.0.0.1:${port}/v2/rooms`;
      const proc=spawn(resolve('../.build/signaling-test.exe'),['-listen',`127.0.0.1:${port}`,'-data',resolve(`../.build/rooms-voice-${Date.now()}`)],{windowsHide:true});processes.push(proc);
      await new Promise<void>((resolve,reject)=>{proc.stderr!.once('data',()=>resolve());proc.once('error',reject);});
    }
    for(let i=0;i<3;i++) {
      const proc=spawn(resolve('../.build/preferans-test.exe'),['-headless','-update-port','-1','-data',resolve(`../.build/voice-${Date.now()}-${i}`)],{windowsHide:true,env:{...process.env,PREFERANS_P2P_LOOPBACK:'1',...(roomsMode?{PREFERANS_ROOMS_URL:roomsURL}:{})}});
      processes.push(proc);
      const url=await new Promise<string>((resolve,reject)=>{proc.stdout!.once('data',b=>resolve(String(b).trim()));proc.once('error',reject);proc.once('exit',code=>reject(Error(`backend exited ${code}`)));});
      const context=await browser.newContext();contexts.push(context);
      await context.addInitScript(() => {
        const original=window.RTCPeerConnection;
        (window as any).__pcs=[];(window as any).__tracks=[];
        window.RTCPeerConnection=class extends original {constructor(config?:RTCConfiguration){super({...config,iceTransportPolicy:'relay'});(window as any).__pcs.push(this);}};
        const capture=navigator.mediaDevices.getUserMedia.bind(navigator.mediaDevices);
        navigator.mediaDevices.getUserMedia=async constraints=>{const s=await capture(constraints);(window as any).__tracks.push(...s.getTracks());return s;};
      });
      const page=await context.newPage();
      await page.route('**/*',async route=>{
        const u=new URL(route.request().url());
        if (u.origin===new URL(url).origin && u.pathname==='/api' && route.request().postDataJSON().action==='status') {
          const response=await route.fetch(), body=await response.json();
          body.data.roomMode=roomsMode;
          await route.fulfill({response,json:body});return;
        }
        if(u.origin===new URL(url).origin && (u.pathname==='/' || u.pathname.startsWith('/assets/') || u.pathname.startsWith('/cards/'))) {
          const file=u.pathname==='/'?'index.html':u.pathname.slice(1);
          let body=await readFile(resolve('dist',file));
          if(file==='index.html')body=Buffer.from(body.toString().replace('<html','<html data-language="ru"'));
          await route.fulfill({body,contentType:file.endsWith('.js')?'text/javascript':file.endsWith('.css')?'text/css':file.endsWith('.svg')?'image/svg+xml':'text/html'});
        } else await route.continue();
      });
      clients.push({page,token:new URL(url).hash.slice(1),url});
      const state=await rpc(i,'status');
      await rpc(i,'appearance',{appearance:{...state.appearance,language:'ru'}});
      await page.goto(url);
    }
    if(roomsMode){
      const {room}=await rpc(0,'rooms-create',{name:'Голосовая комната'});
      for(let i=0;i<3;i++)await rpc(i,'room-enter',{id:room.id});
    }else{
    await rpc(0,'create',{players:4,target:30,name:'Voice host',bots:false});
    for(let i=1;i<3;i++) {
      const code=await rpc(0,'invite',{seat:i});
      await rpc(i,'join',{code});
      await expect.poll(async()=>!!(await rpc(0,'status')).connected[i],{timeout:15000}).toBeTruthy();
    }
    await rpc(0,'fill-bots');
    }
    for(const {page} of clients) {
      await expect(page.locator('[data-microphone]')).toHaveCount(1);
      await expect(page.locator('[data-voice-seat]')).toHaveCount(2);
      await page.locator('[data-microphone]').click();
      await expect(page.locator('[data-microphone]')).toHaveAttribute('aria-pressed','true');
    }
    // пока WebRTC не поднялся, значки собеседников «нет соединения»; через ~25 с голос переходит на игровой канал
    for(const {page} of clients) await expect(page.locator('[data-voice-seat].offline')).toHaveCount(0,{timeout:70000});
    for(const {page} of clients) {
      expect(await page.evaluate(()=>((window as any).__pcs as RTCPeerConnection[]).some(pc=>pc.connectionState==='connected'))).toBeFalsy();
    }
    const lines=async(i:number)=>((await rpc(i,'status')).logs as string[]).filter(l=>l.includes('звук через стол')||l.includes('голос через стол')).join('\n');
    for(let i=0;i<3;i++) {
      await expect.poll(()=>lines(i),{timeout:70000}).toMatch(/голос через стол \(WebRTC не поднялся/);
      await expect.poll(()=>lines(i),{timeout:70000}).toMatch(/передача включена/);
      await expect.poll(()=>lines(i),{timeout:70000}).toMatch(/пачек отправлено [1-9]\d*, принято [1-9]\d*/);
      expect(await lines(i)).not.toMatch(/\d+\.\d+\.\d+\.\d+/);
      expect(await lines(i)).not.toMatch(/ошибка (кодера|декодера)/);
    }
  } finally {
    for(const c of contexts) await c.unrouteAll({behavior:'wait'});
    await browser.close();
    for(const proc of processes)proc.kill();
  }
});
