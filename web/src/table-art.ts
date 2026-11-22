import {t, locale} from './i18n';
import { cardImage, appearance } from "./appearance";
// Display order is independent of the engine's bidding order.
export function ordered(cards: number[]): number[] {
  const order = [0, 1, 2, 3]; // spades, clubs, diamonds, hearts
  return [...cards].sort(
    (a, b) =>
      order.indexOf(a >> 3) - order.indexOf(b >> 3) || (a % 8) - (b % 8),
  );
}

export function face(c: number, deck?: string, frame?: boolean): string { return cardImage(c, deck, frame); }

import { wholeWhists } from "./settlement";

interface ScoreView {
  rules?: {endCondition: string; minutes: number};
  deadline?: number;
  pausedAt?: number;
  stage?: string;
  seat: number;
  target: number;
  players: { name: string }[];
  pool: number[];
  mountain: number[];
  whists: number[][];
  results: number[];
  history?: { pool?: number[]; mountain?: number[] }[];
  teeth?: number[][];
}
// Semi-transparent serrated "shark teeth" drawn over a whist column. Teeth aimed at the left or right player lean
// that way and sit in that part of the column; teeth aimed across the table stand upright in the middle.
// Tooth drawings. "shark" is the serrated tooth with barbs pointing up; "molar" is a back tooth (crown with two cusps and two roots) drawn after
// the hand-made sample зуб.jpg. Both are about 20 units tall and are filled with the same colour.
const sharkTooth = "M0-12L0.26-7.5 1.66-8.3 2.3-1.1 3.7-1.9 4.35 5.3 5.75 4.5 7 10H-7L-5.75 4.5-4.35 5.3-3.7-1.9-2.3-1.1-1.66-8.3-0.26-7.5Z"; // the barbs point up, towards the tip
// Right half of the molar's outline in the sample's own 512x512 pixels, from the notch between the cusps down the
// right side, round the right root, up to the fork between the roots. The left half is its mirror image.
const molarHalf: [number, number][] = [
  [255, 45], [270, 25], [300, 5], [340, -3], [380, 3], [410, 15], [430, 30], [452, 52], [466, 75], [478, 100], [486, 125],
  [489, 150], [487, 175], [481, 200], [470, 225], [455, 250], [440, 275], [428, 300], [414, 350], [407, 400], [394, 450],
  [382, 480], [367, 500], [341, 514], [317, 500], [304, 480], [292, 450], [283, 410], [277, 370], [269, 340], [262, 330], [255, 322],
];
const molarTooth = (() => {
  const k = 0.04, point = ([x, y]: [number, number]) => `${((x - 255.5) * k).toFixed(2)} ${((y - 255) * k).toFixed(2)}`;
  const left = [...molarHalf].reverse().map(([x, y]): [number, number] => [511 - x, y]);
  return `M${[...molarHalf, ...left].map(point).join("L")}Z`;
})();
export function toothPath(style: string | undefined = appearance.toothStyle): string {
  return style === "molar" ? molarTooth : sharkTooth;
}
// Does a tooth standing at (x, 442) and leaning by `lean` degrees cover any part of the whist number in the column middle?
function toothCoversNumber(x: number, lean: number, label: string, centre: number): boolean {
  const rad = lean * Math.PI / 180, cos = Math.cos(rad), sin = Math.sin(rad);
  const half = label.length * 5.7; // 19px monospace digits, centred on the column
  for (let py = -12; py <= 10; py++) {
    const reach = appearance.toothStyle === "molar" ? 9 : 7 * (py + 12) / 22; // the shark tooth is a triangle: apex up, wide base down; the molar is about as wide everywhere
    for (let px = -reach; px <= reach; px += 1) {
      const wx = x + px * cos - py * sin, wy = 442 + px * sin + py * cos;
      if (wx > centre - half && wx < centre + half && wy > 434 && wy < 450) return true;
    }
  }
  return false;
}
function teethSvg(count: number, left: number, columnWidth: number, direction: "left" | "up" | "right", label: string): string {
  if (!(count > 0)) return "";
  const lean = direction === "left" ? -32 : direction === "right" ? 32 : 0;
  const molar = appearance.toothStyle === "molar";
  const step = Math.min(molar ? (direction === "up" ? 17 : 15) : direction === "up" ? 14 : 12, (columnWidth - 28) / count);
  const first = direction === "left" ? left + 14 : direction === "right" ? left + columnWidth - 14 - step * (count - 1) : left + columnWidth / 2 - step * (count - 1) / 2;
  const centre = left + columnWidth / 2;
  return Array.from({length: count}, (_, i) =>
    `<path class="pool-tooth${toothCoversNumber(first + i * step, lean, label, centre) ? " over-number" : ""}" transform="translate(${(first + i * step).toFixed(1)} 442) rotate(${lean})" d="${toothPath()}"/>`).join("");
}// Running total after every entry, as players write the pool by hand: 2.10.20.
// A long record keeps its beginning, then "..", then the last two entries: 2.4.10.12..22.24.
export function runningTally(v: ScoreView, key: "pool" | "mountain", seat: number, maxChars: number): string {
  const total = v[key][seat] || 0;
  const entries: number[] = [];
  let sum = 0;
  for (const ledger of v.history || []) {
    const x = ledger[key]?.[seat] || 0;
    if (x !== 0) { sum += x; entries.push(sum); }
  }
  // Without a matching history (old saves) the plain total is the only honest value.
  if (entries.length < 2 || sum !== total) return String(total);
  const full = entries.join(".");
  if (full.length <= maxChars || entries.length <= 3) return full;
  const tail = entries.slice(-2).join(".");
  let head = 1;
  for (let i = 2; i <= entries.length - 2; i++) {
    if (`${entries.slice(0, i).join(".")}..${tail}`.length > maxChars) break;
    head = i;
  }
  return `${entries.slice(0, head).join(".")}..${tail}`;
}
export function remainingMinutes(v: ScoreView): number {
  if (v.stage === 'finished') return 0;
  return v.deadline ? Math.max(0, Math.ceil((v.deadline-(v.pausedAt || Date.now()/1000))/60)) : v.rules?.minutes || 0;
}
export function poolDrawing(v: ScoreView, esc: (s: unknown) => string, options: { teeth?: boolean; dots?: boolean } = {}): string {
  const n = v.players.length;
  const totals = wholeWhists(v.results);
  const seats = Array.from({ length: n }, (_, i) => (v.seat + i) % n);
  const angles = n === 3 ? [0, 90, 270] : [0, 90, 180, 270];
  const sectors = seats
    .map((seat, i) => {
      const a = angles[i];
      // Rotate the sector as a whole. Its whist columns belong to opponents
      // in the owner's seat order, independent of the viewer's bottom seat.
      const opponents = Array.from({length: n - 1}, (_, offset) => (seat + offset + 1) % n);
      const whists = opponents.map((s, j) => {
        const x = 57 + 366 * (j + 0.5) / opponents.length;
        const divider = j ? `<path d="M${57 + 366 * j / opponents.length} 423V462"/>` : "";
        const direction = opponents.length === 2 ? (["left", "right"] as const)[j] : (["left", "up", "right"] as const)[j];
        const teeth = v.teeth?.[seat]?.[s] || 0;
        const drawn = teeth ? `<g class="pool-teeth" pointer-events="none"><title>${t("web.teeth.on", {p0: esc(v.players[seat].name), p1: esc(v.players[s].name), p2: teeth})}</title>${teethSvg(teeth, 57 + 366 * j / opponents.length, 366 / opponents.length, direction, String(v.whists[seat]?.[s] || 0))}</g>` : "";
        // My own whist area carries a tooth button for every opponent.
        const reach = Math.min(366 / opponents.length / 2 - 18, 52);
        const bx = direction === "right" ? x - reach : x + reach;
        const button = options.teeth && seat === v.seat ? `<g class="tooth-button" data-tooth-aim="${s}" role="button" tabindex="0" aria-label="${t("web.teeth.button")}"><title>${t("web.teeth.title", {p0: esc(v.players[s].name)})}</title><rect class="tooth-hit" x="${bx - 16}" y="425" width="32" height="34" rx="8"/><path class="tooth-icon" transform="translate(${bx.toFixed(1)} 442) scale(0.85)" d="${toothPath()}"/></g>` : "";
        return `${divider}<text x="${x}" y="449" class="pool-whists"><title>${t("web.table_art.text006", {p2: esc(v.players[seat].name), p3: esc(v.players[s].name)})}</title>${v.whists[seat]?.[s] || 0}</text>${drawn}${button}`;
      }).join("");
      const result = (totals[seat] || 0).toLocaleString(locale);
      return `<g transform="rotate(${a} 240 240)"><text x="240" y="307" class="pool-balance"><title>${t("web.table_art.text005", {p1: esc(v.players[seat].name)})}</title>${result}</text><text x="240" y="338" class="pool-name">${esc(v.players[seat].name)}</text><text x="240" y="369" class="pool-mountain"><title>${t("web.main.text049")}</title>${options.dots === false ? v.mountain[seat] || 0 : runningTally(v, "mountain", seat, 15)}</text><text x="240" y="410" class="pool-points"><title>${t("web.main.text006")}</title>${options.dots === false ? v.pool[seat] || 0 : runningTally(v, "pool", seat, 21)}</text>${whists}</g>`;
    })
    .join("");
  return `<button class="pool-drawing" data-panel="score" aria-label="${t("web.table_art.text004")}"><svg viewBox="0 0 480 480" role="img" aria-label="${v.rules?.endCondition === 'time' ? t("web.table_art.text001") : `${t("web.rules.text001", {p0: v.target})}`}"><rect x="18" y="18" width="444" height="444" rx="22"/><rect x="57" y="57" width="366" height="366" rx="18"/><rect x="99" y="99" width="282" height="282" rx="12"/><path d="M240 240L18 462M240 240L462 462${n === 3 ? "M240 240V18" : "M240 240L18 18M240 240L462 18"}"/>${sectors}<circle cx="240" cy="240" r="43" class="pool-target"/><text x="240" y="233" class="pool-total">${v.rules?.endCondition === 'time' ? remainingMinutes(v) : v.target}</text><text x="240" y="253" class="pool-name">${v.rules?.endCondition === 'time' ? t("web.table_art.text003") : t("web.table_art.text002")}</text></svg></button>`;
}
