import { mount } from 'svelte';

import App from './App.svelte';
import InvitationPage from './components/InvitationPage.svelte';
import './app.css';
import { createPanelApi } from './lib/api';
import { readBasePath, readPanelBuild } from './lib/base';
import {
  applyDocumentTheme,
  DEFAULT_THEME_DISPLAY,
  isThemeDisplay,
  resolveThemeDisplay,
} from './lib/preferences';
import { effectivePref, migrateLegacyPreferences, readPrefsDoc } from './lib/preferences-sync';
import { createPanelRouter, parseInvitationToken } from './lib/routes';
import { registerPanelServiceWorker } from './lib/service-worker';

migrateLegacyPreferences();

const target = document.querySelector('#app');
// The synced document is read before mount — regardless of which account it
// belongs to — so the first paint carries no theme flash.
const storedTheme = effectivePref(readPrefsDoc(), 'theme');
const theme = resolveThemeDisplay(
  typeof storedTheme === 'string' && isThemeDisplay(storedTheme)
    ? storedTheme
    : DEFAULT_THEME_DISPLAY,
);

applyDocumentTheme(document, theme);

if (target === null) {
  throw new Error('the panel page is missing its #app mount point');
}

try {
  const base = readBasePath(document);
  const api = createPanelApi(base, (input, init) => fetch(input, init));
  const build = readPanelBuild(document);
  void registerPanelServiceWorker(base, build.version).catch((error: unknown) => {
    console.warn('Smyklot offline cache could not start', error);
  });
  const invitationToken = parseInvitationToken(base, window.location.pathname);
  // Built from the mount point rather than imported, because Vite would bake the
  // sentinel into the JS bundle and only `index.html` is rewritten when serving.
  if (invitationToken === null) {
    mount(App, {
      target,
      props: { api, build, router: createPanelRouter(base, window) },
    });
  } else {
    mount(InvitationPage, {
      target,
      props: { api, token: invitationToken, build },
    });
  }
} catch (error) {
  // Reaching here means the page itself was served wrong, so there is no
  // working app to show the failure inside.
  target.textContent = error instanceof Error ? error.message : String(error);
}
