// Рисует значки приложения из native/windows/icon.svg: Windows (icon.png, bullet.ico) и Android (mipmap-*).
// Запуск: node scripts/render-icons.mjs  (нужен Playwright из web/node_modules; затем scripts/Rebuild-WindowsIcon.ps1).
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const { chromium } = await import(pathToFileURL(path.join(root, 'web/node_modules/playwright-core/index.mjs')).href);
const svg = fs.readFileSync(path.join(root, 'native/windows/icon.svg'), 'utf8');

// ICON_BROWSER — путь к chrome.exe, если браузер Playwright не установлен (npx playwright install chromium).
const browser = await chromium.launch(process.env.ICON_BROWSER ? { executablePath: process.env.ICON_BROWSER } : {});
const page = await browser.newPage();
async function render(size) {
  const sized = svg.replace('<svg ', `<svg width="${size}" height="${size}" `);
  await page.setViewportSize({ width: size, height: size });
  await page.setContent(`<!doctype html><html><body style="margin:0;background:transparent">${sized}</body></html>`);
  return page.screenshot({ clip: { x: 0, y: 0, width: size, height: size }, omitBackground: true, type: 'png' });
}

fs.writeFileSync(path.join(root, 'native/windows/icon.png'), await render(2048));
const android = { mdpi: 48, hdpi: 72, xhdpi: 96, xxhdpi: 144, xxxhdpi: 192 };
for (const [density, size] of Object.entries(android)) {
  fs.writeFileSync(path.join(root, `android/app/src/main/res/mipmap-${density}/ic_launcher.png`), await render(size));
}

// ICO из PNG-кадров (Windows Vista и новее читают PNG внутри ICO).
const sizes = [16, 24, 32, 48, 64, 128, 256];
const frames = [];
for (const size of sizes) frames.push({ size, data: await render(size) });
const header = Buffer.alloc(6 + 16 * frames.length);
header.writeUInt16LE(0, 0); header.writeUInt16LE(1, 2); header.writeUInt16LE(frames.length, 4);
let offset = header.length;
frames.forEach((f, i) => {
  const o = 6 + 16 * i;
  header[o] = f.size === 256 ? 0 : f.size; header[o + 1] = f.size === 256 ? 0 : f.size;
  header.writeUInt16LE(1, o + 4); header.writeUInt16LE(32, o + 6);
  header.writeUInt32LE(f.data.length, o + 8); header.writeUInt32LE(offset, o + 12);
  offset += f.data.length;
});
fs.writeFileSync(path.join(root, 'native/windows/bullet.ico'), Buffer.concat([header, ...frames.map((f) => f.data)]));
await browser.close();
console.log('Значки обновлены: icon.png, bullet.ico, mipmap-*');