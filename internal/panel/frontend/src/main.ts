import { mount } from 'svelte';

import App from './App.svelte';
import InvitationPage from './components/InvitationPage.svelte';
import './app.css';
import { createPanelApi } from './lib/api';
import { PANEL_ICON_PATH, panelUrl, readBasePath, readPanelBuild } from './lib/base';
import { readThemeDisplay, resolveThemeDisplay } from './lib/preferences';
import { createPanelRouter, parseInvitationToken } from './lib/routes';

const target = document.querySelector('#app');
const theme = resolveThemeDisplay(readThemeDisplay());

document.documentElement.dataset.theme = theme;

if (target === null) {
  throw new Error('the panel page is missing its #app mount point');
}

try {
  const base = readBasePath(document);
  const api = createPanelApi(base, (input, init) => fetch(input, init));
  const iconUrl = panelUrl(base, PANEL_ICON_PATH);
  const build = readPanelBuild(document);
  const invitationToken = parseInvitationToken(base, window.location.pathname);
  // Built from the mount point rather than imported, because Vite would bake the
  // sentinel into the JS bundle and only `index.html` is rewritten when serving.
  if (invitationToken === null) {
    mount(App, {
      target,
      props: { api, iconUrl, build, router: createPanelRouter(base, window) },
    });
  } else {
    mount(InvitationPage, {
      target,
      props: { api, token: invitationToken, iconUrl, build },
    });
  }
} catch (error) {
  // Reaching here means the page itself was served wrong, so there is no
  // working app to show the failure inside.
  target.textContent = error instanceof Error ? error.message : String(error);
}
