import {interfaceConfig as c} from './interface-config';
// Reveal as much of each card as the current hand's width permits.
export function fitCardFans() {
  if (innerWidth <= 700) return;
  // Measure the heading instead of assuming its height: Android's shorter
  // viewport uses a different heading size from the desktop layout.
  const field = document.querySelector<HTMLElement>('.playing-field');
  const heading = document.querySelector<HTMLElement>('.table-heading');
  const handArea = document.querySelector<HTMLElement>('.hand-area');
  const sideCards = field?.querySelector<HTMLElement>('.player:not(.player-top) .player-cards');
  if (field && handArea && sideCards && innerHeight >= c.wideLayoutMinHeightPx && document.body.dataset.stage !== 'lobby') {
    const four = !!field.querySelector('.player-top');
    const sideHeight = four ? c.sideHeightScaleFour : c.sideHeightScaleThree;
    const step = Math.max(c.cardStairMinPx, Math.min(innerWidth * c.cardStairViewportRatio, c.cardStairMaxPx));
    const tray = document.querySelector<HTMLElement>('.discard-tray');
    const reserved = (tray?.getBoundingClientRect().height || 0) + (tray ? c.discardGapPx : 0);
    // Reserve four suit rows, six stair steps and three gaps for every deal.
    // The bottom hand needs 1.12h. Measure chrome instead of assuming its size.
    // The whole side seat is lowered, including its labels and turn frame;
    // its measured top already includes that offset.
    const available = handArea.getBoundingClientRect().bottom - sideCards.getBoundingClientRect().top - reserved;
    const infoHeight = document.querySelector<HTMLElement>('.hand-label > strong')?.getBoundingClientRect().height || 0;
    const height = Math.max(c.cardMinHeightPx, Math.min(innerWidth * c.cardViewportWidthRatio, c.cardMaxHeightPx, (available - c.layoutVerticalReservePx - 6 * step) / (sideHeight + c.ownCardScale), (available - c.layoutVerticalReservePx - 6 * step - infoHeight) / sideHeight));
    document.body.style.setProperty('--fitted-card-h', `${height}px`);
  } else {
    document.body.style.removeProperty('--fitted-card-h');
  }
  if (field && heading) {
    field.style.setProperty('--upper-seat-top', `${heading.getBoundingClientRect().top - field.getBoundingClientRect().top}px`);
  }
  for (const container of document.querySelectorAll<HTMLElement>('.hand, .exposed, .closed-hand')) {
    const groups = [...container.querySelectorAll<HTMLElement>(':scope > .suit-group')];
    const vertical = container.matches('.exposed') && !container.closest('.player-top');
    const rows = vertical ? groups : [container];
    for (const row of rows) {
      const cards = [...row.querySelectorAll<HTMLElement>('.card, .card-back')];
      if (!cards.length) continue;
      const width = cards[0].getBoundingClientRect().width;
      const count = vertical ? 1 : Math.max(groups.length, 1);
      const gaps = Math.max(0, count - 1) * (parseFloat(getComputedStyle(container).columnGap) || 0);
      const available = container.clientWidth - gaps - c.cardFanEdgePaddingPx;
      const overlap = cards.length > count ? Math.min(0, (available - count * width) / (cards.length - count) - width) : 0;
      row.style.setProperty('--card-overlap', `${overlap}px`);
    }
  }
}
