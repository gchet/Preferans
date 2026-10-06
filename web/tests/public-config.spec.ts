import {test, expect} from '@playwright/test';
import {spawn} from 'node:child_process';
import {resolve} from 'node:path';

test('public build starts without personal servers and can play offline', async ({page}) => {
  const root=resolve('..');
  const executable=process.platform==='win32' ? '.build/public-build/Preferans.exe' : '.build/preferans-public-test.exe';
  const environment={...process.env};
  delete environment.PREFERANS_ROOMS_URL;
  const child=spawn(resolve(root,executable), ['-headless','-update-port','-1','-data',resolve(root,`.build/public-ui-${Date.now()}`)], {cwd:root,windowsHide:true,env:environment});
  try {
    const url=await new Promise<string>((done,fail)=>{
      child.stdout!.once('data',b=>done(String(b).trim()));
      child.once('error',fail);
      child.once('exit',code=>fail(Error(`Public backend exited ${code}`)));
    });
    const endpoint=new URL('/api',url).href;
    const headers={'X-Preferans-Token':new URL(url).hash.slice(1)};
    const status=(await (await page.request.post(endpoint,{headers,data:{action:'status'}})).json()).data;
    expect(status.connectionSetup).toBe(true);
    await page.request.post(endpoint,{headers,data:{action:'appearance',appearance:{...status.appearance,language:'ru'}}});
    await page.goto(url);
    await expect(page.locator('[name="room-service-url"]')).toHaveValue('');
    await expect(page.locator('[name="turn-server"]')).toBeVisible();
    await expect(page.locator('[name="turn-server"]')).toHaveValue('');
    await page.getByRole('button',{name:'Играть локально с ботами',exact:true}).click();
    await expect(page.locator('#create-form')).toBeVisible();
    await page.getByRole('button',{name:'Создать стол →'}).click();
    await expect(page.locator('.center.lobby')).toBeVisible();
    await page.getByRole('button',{name:'Готов',exact:true}).click();
    await page.getByRole('button',{name:'Начать партию',exact:true}).click();
    await expect(page.locator('.hand .card')).toHaveCount(10);
    await page.locator('#finish-party-topbar').click();
    await expect(page.locator('#create-form')).toBeVisible();
  } finally {
    await page.close();
    child.kill();
  }
});
