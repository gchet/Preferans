import {test, expect, chromium, type Page, type BrowserContext} from '@playwright/test';
import {spawn, type ChildProcess} from 'node:child_process';
import {resolve} from 'node:path';
import {readFile} from 'node:fs/promises';
import {createServer} from 'node:net';

for(const roomsMode of [false,true]) test(`three human players exchange audio, mute, reconnect and leave; rooms=${roomsMode}`, async () => {
  test.setTimeout(120000);
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
      const proc=spawn(resolve('../.build/preferans-test.exe'),['-headless','-update-port','-1','-data',resolve(`../.build/voice-${Date.now()}-${i}`)],{windowsHide:true,env:{...process.env,...(roomsMode?{PREFERANS_ROOMS_URL:roomsURL}:{})}});
      processes.push(proc);
      const url=await new Promise<string>((resolve,reject)=>{proc.stdout!.once('data',b=>resolve(String(b).trim()));proc.once('error',reject);proc.once('exit',code=>reject(Error(`backend exited ${code}`)));});
      const context=await browser.newContext();contexts.push(context);
      await context.addInitScript(() => {
        const original=window.RTCPeerConnection;
        (window as any).__pcs=[];(window as any).__tracks=[];
        window.RTCPeerConnection=class extends original {constructor(config?:RTCConfiguration){super(config);(window as any).__pcs.push(this);}};
        const capture=navigator.mediaDevices.getUserMedia.bind(navigator.mediaDevices);
        navigator.mediaDevices.getUserMedia=async constraints=>{const s=await capture(constraints);(window as any).__tracks.push(...s.getTracks());return s;};
      });
      const page=await context.newPage();
      await page.route('**/*',async route=>{
        const u=new URL(route.request().url());
        if (u.origin===new URL(url).origin && u.pathname==='/api' && route.request().postDataJSON().action==='status') {
          const response=await route.fetch(), body=await response.json();
          // The game's P2P link stays local. Only audio first tries an absent
          // TURN endpoint, exercising automatic fallback to direct ICE.
          body.data.stun=['turn:127.0.0.1:3479?transport=udp|test|test'];
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
      await rpc(i,'configure',{stun:[]});
      await page.goto(url);
    }
    if(roomsMode){
      const {room}=await rpc(0,'rooms-create',{name:'Голосовая комната'});
      for(let i=0;i<3;i++)await rpc(i,'room-enter',{id:room.id});
    }else{
    await rpc(0,'create',{players:4,target:30,name:'Voice host',bots:false});
    for(let i=1;i<3;i++) {
      const offer=await rpc(0,'invite',{seat:i});
      const answer=await rpc(i,'join',{code:offer});
      await rpc(0,'answer',{code:answer});
      await expect.poll(async()=>!!(await rpc(0,'status')).connected[i],{timeout:15000}).toBeTruthy();
    }
    await rpc(0,'fill-bots');
    }
    for(const {page} of clients) {
      await expect(page.locator('[data-microphone]')).toHaveCount(1);
      await expect(page.locator('[data-voice-seat]')).toHaveCount(2);
      await expect(page.locator('[data-voice-seat].offline')).toHaveCount(0,{timeout:30000});
      await page.locator('[data-microphone]').click();
      await expect(page.locator('[data-microphone]')).toHaveAttribute('aria-pressed','true');
      expect(await page.evaluate(()=>(window as any).__pcs.some((pc:RTCPeerConnection)=>pc.getConfiguration().iceTransportPolicy==='relay'))).toBeTruthy();
    }
    for(const {page} of clients) {
      await expect.poll(()=>page.evaluate(async()=>{
        let receiving=0;
        for(const pc of (window as any).__pcs as RTCPeerConnection[]) {
          if(pc.connectionState!=='connected')continue;
          const stats=await pc.getStats();
          if([...stats.values()].some(s=>s.type==='inbound-rtp' && s.kind==='audio' && s.packetsReceived>10))receiving++;
        }
        return receiving;
      }),{timeout:20000}).toBe(2);
    }
    const languagePicker=clients[0].page.locator('.topbar [data-language-picker]');
    await Promise.all([clients[0].page.waitForNavigation(), languagePicker.selectOption('en')]);
    await expect(clients[0].page.locator('[data-microphone]')).toHaveAttribute('aria-pressed','true',{timeout:15000});
    await expect(clients[0].page.locator('[data-voice-seat].offline')).toHaveCount(0,{timeout:30000});
    const host=clients[0].page;
    const mutedAudio=()=>host.locator('[data-voice-audio]').evaluateAll(nodes=>nodes.map(n=>({seat:(n as HTMLElement).dataset.voiceAudio,muted:(n as HTMLAudioElement).muted})).sort((a,b)=>Number(a.seat)-Number(b.seat)));
    await expect(host.locator('[data-speaker]')).toHaveCount(3);
    await expect.poll(mutedAudio).toEqual([{seat:'1',muted:false},{seat:'2',muted:false}]);
    await host.locator('[data-speaker="1"]').click();
    await expect.poll(mutedAudio).toEqual([{seat:'1',muted:true},{seat:'2',muted:false}]);
    await host.locator('[data-speaker="all"]').click();
    await expect.poll(mutedAudio).toEqual([{seat:'1',muted:true},{seat:'2',muted:true}]);
    await host.locator('[data-speaker="all"]').click();
    await expect.poll(mutedAudio).toEqual([{seat:'1',muted:true},{seat:'2',muted:false}]);
    await expect(host.locator('[data-microphone]')).toHaveAttribute('aria-pressed','true');
    expect(await clients[1].page.locator('[data-voice-audio]').evaluateAll(nodes=>nodes.every(n=>!(n as HTMLAudioElement).muted))).toBeTruthy();
    if(roomsMode){
      await rpc(0,'create',{players:4,target:30,name:'Voice host',bots:true});
      for(const {page} of clients){
        await expect(page.locator('.center.lobby')).toBeVisible({timeout:30000});
        await expect(page.locator('[data-microphone]')).toHaveAttribute('aria-pressed','true');
        await expect(page.locator('[data-voice-seat]')).toHaveCount(2);
      }
    }
    await expect.poll(mutedAudio).toEqual([{seat:'1',muted:true},{seat:'2',muted:false}]);
    await clients[1].page.locator('[data-microphone]').click();
    await expect(clients[0].page.locator('[data-voice-seat="1"]')).toHaveClass(/muted/);
    expect(await clients[1].page.evaluate(()=>(window as any).__tracks.every((t:MediaStreamTrack)=>t.readyState==='ended'))).toBeTruthy();
    await clients[1].page.reload();
    await expect(clients[1].page.locator('[data-microphone]')).toHaveAttribute('aria-pressed','false');
    await expect(clients[1].page.locator('[data-voice-seat].offline')).toHaveCount(0,{timeout:30000});
    await expect.poll(mutedAudio).toEqual([{seat:'1',muted:true},{seat:'2',muted:false}]);
    await host.locator('[data-speaker="1"]').click();
    await expect.poll(mutedAudio).toEqual([{seat:'1',muted:false},{seat:'2',muted:false}]);
    await clients[1].page.evaluate(()=>{navigator.mediaDevices.getUserMedia=()=>Promise.reject(new DOMException('Denied','NotAllowedError'));});
    await clients[1].page.locator('[data-microphone]').click();
    await expect(clients[1].page.locator('#notice')).toContainText('Разрешите доступ к микрофону');
    await expect(clients[1].page.locator('[data-microphone]')).toHaveAttribute('aria-pressed','false');
    await rpc(2,'leave');
    if(roomsMode)await rpc(2,'room-exit');
    await expect.poll(()=>clients[2].page.evaluate(()=>(window as any).__tracks.every((t:MediaStreamTrack)=>t.readyState==='ended'))).toBeTruthy();
    await expect(clients[2].page.locator('[data-voice-audio]')).toHaveCount(0);
    await host.evaluate(() => {
      const track=(window as any).__tracks.find((candidate:MediaStreamTrack)=>candidate.readyState==='live');
      track?.dispatchEvent(new Event('ended'));
    });
    await expect(host.locator('#notice')).toContainText('Микрофон отключён системой');
    await expect(host.locator('[data-microphone]')).toHaveAttribute('aria-pressed','false');
  } finally {
    for(const c of contexts) await c.unrouteAll({behavior:'wait'});
    await browser.close();
    for(const proc of processes)proc.kill();
  }
});
