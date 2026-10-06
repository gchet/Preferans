// Run after installing and launching the emulator APK. Uses only test app data.
import { spawn, execFileSync } from "node:child_process";
import { resolve } from "node:path";
import { mkdirSync, writeFileSync } from "node:fs";
const adb = process.env.ADB || "adb";
const serial = process.env.ANDROID_SERIAL || "emulator-5554";
const args = ["-s", serial];
const children = [];
let browser;
const delay = (ms) => new Promise((r) => setTimeout(r, ms));
async function wait(fn, timeout = 45000) {
  const start = Date.now();
  while (Date.now() - start < timeout) {
    if (await fn()) return;
    await delay(150);
  }
  throw Error("Timed out waiting for connection/state");
}
async function desktop(label) {
  const dir = resolve(`.build/android-smoke-${Date.now()}-${label}`);
  mkdirSync(dir, { recursive: true });
  const p = spawn(
    resolve(".build/preferans-test.exe"),
    ["-headless", "-update-port", "-1", "-data", dir],
    { windowsHide: true },
  );
  children.push(p);
  const url = await new Promise((res, rej) => {
    p.stdout.once("data", (b) => res(String(b).trim()));
    p.once("error", rej);
  });
  const u = new URL(url);
  return async (action, extra = {}) => {
    const r = await fetch(u.origin + "/api", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Preferans-Token": u.hash.slice(1),
      },
      body: JSON.stringify({ action, ...extra }),
    });
    const b = await r.json();
    if (b.error) throw Error(b.error);
    return b.data;
  };
}
async function mobilePage() {
  let pid;
  await wait(async () => {
    try {
      pid = execFileSync(
        adb,
        [...args, "shell", "pidof", "com.preferans.game"],
        { encoding: "utf8" },
      ).trim();
      return !!pid;
    } catch {
      return false;
    }
  });
  execFileSync(adb, [
    ...args,
    "forward",
    "tcp:9223",
    `localabstract:webview_devtools_remote_${pid}`,
  ]);
  let target;
  await wait(async () => {
    try {
      target = (
        await (await fetch("http://127.0.0.1:9223/json/list")).json()
      ).find((p) => p.type === "page");
      return !!target;
    } catch {
      return false;
    }
  });
  const ws = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((res, rej) => {
    ws.onopen = res;
    ws.onerror = rej;
  });
  let id = 0;
  const pending = new Map();
  ws.onmessage = (e) => {
    const msg = JSON.parse(e.data);
    if (!msg.id) return;
    const cb = pending.get(msg.id);
    if (cb) {
      pending.delete(msg.id);
      if (msg.error) cb.reject(Error(msg.error.message));
      else cb.resolve(msg.result);
    }
  };
  const send = (method, params = {}) =>
    new Promise((resolve, reject) => {
      const key = ++id;
      pending.set(key, { resolve, reject });
      ws.send(JSON.stringify({ id: key, method, params }));
    });
  browser = { close: async () => ws.close() };
  const page = {
    evaluate: async (fn, arg) => {
      const r = await send("Runtime.evaluate", {
        expression: `(${fn.toString()})(${JSON.stringify(arg) ?? ""})`,
        awaitPromise: true,
        returnByValue: true,
      });
      if (r.exceptionDetails)
        throw Error(
          r.exceptionDetails.exception?.description || r.exceptionDetails.text,
        );
      return r.result.value;
    },
    screenshot: async ({ path }) => {
      const r = await send("Page.captureScreenshot", { format: "png" });
      writeFileSync(path, Buffer.from(r.data, "base64"));
    },
  };
  await wait(() =>
    page.evaluate(
      () =>
        !!document.querySelector("#app")?.children.length &&
        !!sessionStorage.getItem("preferans-token"),
    ),
  );
  return page;
}
function mobileRPC(page) {
  return async (action, extra = {}) =>
    page.evaluate(
      async ({ action, extra }) => {
        const r = await fetch("/api", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-Preferans-Token": sessionStorage.getItem("preferans-token"),
          },
          body: JSON.stringify({ action, ...extra }),
        });
        const b = await r.json();
        if (b.error) throw Error(b.error);
        return b.data;
      },
      { action, extra },
    );
}
try {
  let page = await mobilePage();
  let mobile = mobileRPC(page);
  const host = await desktop("host");
  const guest = await desktop("guest");
  let peers = [host, mobile, guest];
  for (const rpc of peers) await rpc("configure", { stun: [] });
  await host("create", {
    players: 3,
    target: 30,
    name: "Windows",
    bots: false,
  });
  async function connect(seat) {
    const offer = await host("invite", { seat });
    const answer = await peers[seat]("join", { code: offer });
    await host("answer", { code: answer });
    await wait(async () => {
      const s = await peers[seat]("status");
      return s.view && s.connected.every(Boolean);
    });
  }
  // First client arrives while seat 2 is still empty.
  const offer = await host("invite", { seat: 1 });
  const answer = await mobile("join", { code: offer });
  await host("answer", { code: answer });
  await wait(async () => {
    const s = await mobile("status");
    return s.view && s.connected[1];
  });
  await connect(2);
  async function command(seat, action, extra = {}) {
    const v = (await peers[seat]("status")).view;
    await peers[seat]("command", {
      command: {
        id: crypto.randomUUID(),
        seat,
        revision: v.revision,
        action,
        ...extra,
      },
    });
    await wait(async () => {
      for (const rpc of peers) {
        if ((await rpc("status")).view.revision <= v.revision) return false;
      }
      return true;
    });
  }
  for (let seat = 0; seat < 3; seat++)
    await command(seat, "ready", {
      name: ["Windows", "Android", "Партнёр"][seat],
    });
  await command(0, "start");
  for (let i = 0; i < 3; i++) {
    const v = (await host("status")).view;
    await command(v.actor, "pass");
  }
  for (let step = 0; step < 45; step++) {
    const v = (await host("status")).view;
    if (v.stage === "round" || v.stage === "finished") break;
    if (v.stage === "trick") {
      await command(0, "collect");
    } else {
      const local = (await peers[v.actor]("status")).view;
      await command(v.actor, "play", { cards: [local.legal[0]] });
    }
  }
  const before = (await host("status")).view;
  if (before.history.length !== 1) throw Error("Round did not finish");
  for (const rpc of peers) {
    const v = (await rpc("status")).view;
    if (JSON.stringify(v.history) !== JSON.stringify(before.history))
      throw Error("Score mismatch");
  }
  await delay(1000);
  await page.screenshot({ path: ".build/android-round.png", fullPage: true });
  console.log(
    "Android ↔ Windows: direct connection, complete round and matching scores OK",
  );
  await browser.close();
  browser = null;
  execFileSync(adb, [
    ...args,
    "shell",
    "am",
    "force-stop",
    "com.preferans.game",
  ]);
  await wait(async () => !(await host("status")).connected[1]);
  execFileSync(adb, [
    ...args,
    "shell",
    "am",
    "start",
    "-n",
    "com.preferans.game/.MainActivity",
  ]);
  page = await mobilePage();
  mobile = mobileRPC(page);
  peers = [host, mobile, guest];
  await connect(1);
  const after = (await mobile("status")).view;
  if (
    after.revision !== before.revision ||
    JSON.stringify(after.history) !== JSON.stringify(before.history)
  )
    throw Error("Android restore mismatch");
  await delay(1000);
  await page.screenshot({
    path: ".build/android-restored.png",
    fullPage: true,
  });
  console.log("Android process restart and seat recovery OK");
} finally {
  if (browser) await browser.close();
  for (const p of children) p.kill();
  try {
    execFileSync(adb, [...args, "forward", "--remove", "tcp:9223"]);
  } catch {}
}
