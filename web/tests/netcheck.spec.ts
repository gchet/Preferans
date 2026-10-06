import {test,expect} from "@playwright/test";
import {spawn,ChildProcess} from "node:child_process";
import {resolve} from "node:path";
const children: ChildProcess[]=[];
async function launch(label:string) {
 const root=resolve("..");
 const child=spawn(resolve(root,".build/preferans-test.exe"),["-headless","-netcheck","-update-port","-1","-data",resolve(root,`.build/netcheck-${Date.now()}-${label}`)],{cwd:root,windowsHide:true});
 children.push(child);
 return new Promise<string>((res,rej)=>{child.stdout!.once("data",b=>res(String(b).trim()));child.once("error",rej);child.once("exit",c=>rej(Error(String(c))))});
}
test.afterEach(()=>{children.splice(0).forEach(p=>p.kill())});
test("network checker confirms real exchange before and after adding bot",async({browser})=>{
 const host=await browser.newPage(),client=await browser.newPage({viewport:{width:390,height:844}});
 await host.goto(await launch("host"));await client.goto(await launch("client"));
 for(const page of [host,client]){
  await expect(page.locator("#stun")).toContainText("",{timeout:1000});
  await expect(page.locator("#stun")).toHaveValue(/stun:/);
  await page.locator("#stun").fill("");
  await page.locator("#configure").click();
  await expect(page.locator("#notice")).toContainText("STUN сохранены");
 }
 await host.locator("#create").click();
 await expect(host.locator("#output")).toHaveValue(/^PREF1/);
 await client.locator("#input").fill(await host.locator("#output").inputValue());
 await client.locator("#join").click();
 await expect(client.locator("#output")).toHaveValue(/^PREF1/);
 await host.locator("#input").fill(await client.locator("#output").inputValue());
 await host.locator("#answer").click();
 for(const page of [host,client])await expect(page.locator("#verdict")).toHaveClass("good",{timeout:15000});
 await client.locator("#ready").click();
 await expect(host.locator("#players")).toContainText("Готов");
 await host.locator("#bot").click();
 for(const page of [host,client]){
  await expect(page.locator("#players")).toContainText("бот (без сети)");
  await expect(page.locator("#verdict")).toHaveClass("good");
 }
 await host.locator("#copy-report").click();
 const report=JSON.parse(await host.locator("#report-text").inputValue());
 expect(report.probes[0].Acknowledged).toBeGreaterThan(0);
 expect(report.probes[0].Received).toBeGreaterThan(0);
 expect(JSON.stringify(report)).not.toContain("PREF1.");
 expect(await client.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
 await host.screenshot({path:"../.build/netcheck-windows.png",fullPage:true});
 await client.screenshot({path:"../.build/netcheck-mobile.png",fullPage:true});
 await client.locator("#leave").click();
 await expect(client.locator("#verdict")).not.toHaveClass("good");
 await host.close();await client.close();
});
