import {t} from './i18n';
// Native shells own process shutdown; closing a table is a separate operation.
// SVG works even when Android's system font has no power-button glyph.
for (const button of document.querySelectorAll('[data-exit-app]')) {
  button.innerHTML = '<svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M12 3v9M6.3 5.9a9 9 0 1 0 11.4 0"/></svg>';
}
document.addEventListener('click', async event => {
  if (!(event.target instanceof Element) || !event.target.closest('[data-exit-app]')) return;
  event.preventDefault();
  event.stopImmediatePropagation();
  try {
    if (window.PreferansAndroid?.exitApp) window.PreferansAndroid.exitApp();
    else if (window.preferansExit) await window.preferansExit();
    else throw Error(t("web.exit_app.text001"));
  } catch (error) {
    const notice = document.querySelector<HTMLButtonElement>('#notice');
    if (notice) { notice.textContent = String(error instanceof Error ? error.message : error); notice.classList.add('visible'); }
  }
}, true);
