import {t} from './i18n';

// Keep confirmations inside the app instead of exposing the WebView origin.
export function confirmDialog(message: string, acceptLabel: string): Promise<boolean> {
  return new Promise(resolve => {
    const dialog = document.createElement('dialog');
    dialog.className = 'app-confirm';
    dialog.setAttribute('aria-labelledby', 'app-confirm-title');
    dialog.setAttribute('aria-describedby', 'app-confirm-message');
    const title = document.createElement('h2');
    title.id = 'app-confirm-title';
    title.textContent = t('web.confirm.title');
    const text = document.createElement('p');
    text.id = 'app-confirm-message';
    text.textContent = message;
    const actions = document.createElement('div');
    actions.className = 'toolbar';
    const cancel = document.createElement('button');
    cancel.type = 'button';
    cancel.textContent = t('web.confirm.cancel');
    const accept = document.createElement('button');
    accept.type = 'button';
    accept.className = 'danger';
    accept.textContent = acceptLabel;
    cancel.onclick = () => dialog.close('cancel');
    accept.onclick = () => dialog.close('accept');
    dialog.addEventListener('cancel', event => {
      event.preventDefault();
      dialog.close('cancel');
    });
    dialog.addEventListener('close', () => {
      const accepted = dialog.returnValue === 'accept';
      dialog.remove();
      resolve(accepted);
    }, {once:true});
    actions.append(cancel, accept);
    dialog.append(title, text, actions);
    document.body.append(dialog);
    dialog.showModal();
    cancel.focus();
  });
}
