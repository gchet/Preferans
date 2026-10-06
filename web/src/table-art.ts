import {t, locale} from './i18n';
import { cardImage } from "./appearance";
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
}
export function remainingMinutes(v: ScoreView): number {
  if (v.stage === 'finished') return 0;
  return v.deadline ? Math.max(0, Math.ceil((v.deadline-(v.pausedAt || Date.now()/1000))/60)) : v.rules?.minutes || 0;
}
export function poolDrawing(v: ScoreView, esc: (s: unknown) => string): string {
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
        return `${divider}<text x="${x}" y="449" class="pool-whists"><title>${t("web.table_art.text006", {p2: esc(v.players[seat].name), p3: esc(v.players[s].name)})}</title>${v.whists[seat]?.[s] || 0}</text>`;
      }).join("");
      const result = (totals[seat] || 0).toLocaleString(locale);
      return `<g transform="rotate(${a} 240 240)"><text x="240" y="307" class="pool-balance"><title>${t("web.table_art.text005", {p1: esc(v.players[seat].name)})}</title>${result}</text><text x="240" y="338" class="pool-name">${esc(v.players[seat].name)}</text><text x="240" y="369" class="pool-mountain"><title>${t("web.main.text049")}</title>${v.mountain[seat] || 0}</text><text x="240" y="410" class="pool-points"><title>${t("web.main.text006")}</title>${v.pool[seat] || 0}</text>${whists}</g>`;
    })
    .join("");
  return `<button class="pool-drawing" data-panel="score" aria-label="${t("web.table_art.text004")}"><svg viewBox="0 0 480 480" role="img" aria-label="${v.rules?.endCondition === 'time' ? t("web.table_art.text001") : `${t("web.rules.text001", {p0: v.target})}`}"><rect x="18" y="18" width="444" height="444" rx="22"/><rect x="57" y="57" width="366" height="366" rx="18"/><rect x="99" y="99" width="282" height="282" rx="12"/><path d="M240 240L18 462M240 240L462 462${n === 3 ? "M240 240V18" : "M240 240L18 18M240 240L462 18"}"/>${sectors}<circle cx="240" cy="240" r="43" class="pool-target"/><text x="240" y="233" class="pool-total">${v.rules?.endCondition === 'time' ? remainingMinutes(v) : v.target}</text><text x="240" y="253" class="pool-name">${v.rules?.endCondition === 'time' ? t("web.table_art.text003") : t("web.table_art.text002")}</text></svg></button>`;
}
