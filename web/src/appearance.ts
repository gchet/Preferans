import {t} from './i18n';
import {interfaceConfig} from './interface-config';
import type {TableRules} from './rules';
export interface Appearance {
  roomServiceURL?: string;
	language?: 'ru' | 'uk' | 'en';
	localOnly?: boolean;
  debug?: boolean;
  debugAuction?: boolean;
  debugPlay?: boolean;
  passPriceHint?: boolean;
	playerCount?: number;
	poolTarget?: number;
  defaultRoom?: string;
  tableRules?: TableRules;
  name?: string;
  deck: string;
  cardFrames?: Record<string, boolean>;
  size: string;
  largeIndices: boolean;
  back?: string;
  logDisabled?: boolean;
  logPath?: string;
  updateURL?: string;
}
export const backs = [["plaid", t("web.appearance.text007")], ["diamonds", t("web.appearance.text006")], ["ornament", t("web.appearance.text005")], ["waves", t("web.appearance.text004")], ["classic", t("web.appearance.text003")]] as const;
export const decks = [
  ["atlas", t("web.appearance.text002")],
  ["english", t("web.appearance.text001")],
  ["woodcut", "Woodcut"],
] as const;
export let appearance: Appearance = {
  deck: "atlas",
  size: "normal",
  largeIndices: true,
};
export function applyAppearance(value: Appearance) {
  appearance = value;
  document.body.dataset.deck = value.deck;
  document.body.dataset.back = value.back || 'plaid';
  document.body.dataset.cardSize = 'normal';
  document.body.classList.toggle(
    "large-indices",
    value.largeIndices || value.deck === "english",
  );
}
export function cardImage(card: number, deck = appearance.deck, frame = appearance.cardFrames?.[deck] !== false): string {
  return `<img class="card-image" data-card-frame="${frame ? 'on' : 'off'}" src="${cardSource(card, deck)}" alt="" draggable="false" aria-hidden="true" />`;
}
function cardSource(card: number, deck: string): string {
  const selected = decks.some(([id]) => id === deck) ? deck : 'atlas';
  const rank = ['7', '8', '9', '10', 'jack', 'queen', 'king', 'ace'][card % 8];
  const suit = ['spades', 'clubs', 'diamonds', 'hearts'][card >> 3];
  // One outer contour is drawn by the card element, matching its rounded edge.
  return `/cards/${selected}/${rank}-${suit}.svg#no-frame`;
}

// Decode the complete selected deck before replacing the hand's DOM. This also
// warms future deals and exposed hands, rather than loading each SVG on a move.
const preparedDecks = new Map<string, Promise<HTMLImageElement[]>>();
export function prepareCards(value: Appearance): Promise<void> {
  const deck = decks.some(([id]) => id === value.deck) ? value.deck : 'atlas';
  const key = deck;
  let pending = preparedDecks.get(key);
  if (!pending) {
    pending = Promise.all(Array.from({length:32}, (_, card) => {
      const img = new Image();
      img.src = cardSource(card, deck);
      return new Promise<HTMLImageElement>(resolve => {
        const timer = setTimeout(() => resolve(img), interfaceConfig.cardDecodeTimeoutMs);
        img.decode().catch(() => {}).finally(() => { clearTimeout(timer); resolve(img); });
      });
    }));
    preparedDecks.set(key, pending);
  }
  return pending.then(() => {});
}
