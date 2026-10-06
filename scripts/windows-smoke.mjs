import {spawn} from 'node:child_process';
import {resolve} from 'node:path';
import {writeFileSync} from 'node:fs';
const delay=ms=>new Promise(r=>setTimeout(r,ms));
const child=spawn(resolve('dist/Preferans.exe'),['-data',resolve(`.build/windows-smoke-${Date.now()}`)],{windowsHide:true,env:{...process.env,WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS:'--remote-debugging-port=9224'}});
let ws;
try {
 let target;
 for(let i=0;i<120;i++){try{target=(await(await fetch('http://127.0.0.1:9224/json/list')).json()).find(p=>p.type==='page'&&p.url.startsWith('http://127.0.0.1:'));if(target)break}catch{};if(child.exitCode!==null)throw Error(`Native app exited ${child.exitCode}`);await delay(250)}
 if(!target)throw Error('WebView2 did not start');
 ws=new WebSocket(target.webSocketDebuggerUrl);await new Promise((res,rej)=>{ws.onopen=res;ws.onerror=rej});let id=0;const pending=new Map();
 ws.onmessage=e=>{const msg=JSON.parse(e.data);if(!msg.id)return;const cb=pending.get(msg.id);if(cb){pending.delete(msg.id);if(msg.error)cb.reject(Error(msg.error.message));else cb.resolve(msg.result)}};
 const send=(method,params={})=>new Promise((resolve,reject)=>{const key=++id;pending.set(key,{resolve,reject});ws.send(JSON.stringify({id:key,method,params}))});
 const evaluate=async expression=>{const r=await send('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value};
 for(let i=0;i<120;i++){if(await evaluate('Boolean(document.querySelector("#create-form"))'))break;await delay(250)}
 if(!await evaluate('Boolean(document.querySelector("#create-form"))'))throw Error('Native UI failed to load');
 await evaluate(`document.querySelector('[name="bots"]').checked=true;document.querySelector('#create-form').requestSubmit()`);
 for(let i=0;i<120;i++){if(await evaluate('Boolean(document.querySelector("[data-action=ready]"))'))break;await delay(100)}
 await evaluate(`document.querySelector('[data-action="ready"]').click()`);await delay(1000);
 await evaluate(`document.querySelector('[data-action="start"]').click()`);await delay(1000);
 if(!await evaluate('Boolean(document.querySelector(".hand .card"))'))throw Error('Native table failed to start');
 const screenshot=await send('Page.captureScreenshot',{format:'png'});writeFileSync('.build/windows-native.png',Buffer.from(screenshot.data,'base64'));
 console.log('Windows native WebView2: startup, create table and deal OK');
} finally {ws?.close();child.kill()}
