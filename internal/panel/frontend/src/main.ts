import { mount } from 'svelte';

import App from './App.svelte';
import ErrorPage from './components/ErrorPage.svelte';
import InvitationPage from './components/InvitationPage.svelte';
import './app.css';
import { createPanelApi } from './lib/api';
import { readBasePath, readPanelBuild } from './lib/base';
import { readPanelFailure } from './lib/panel-error';
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
  // The server serves this same bundle when it is answering with an error, and
  // says so in the document. That is checked before the address is, because the
  // address is what failed: a 404 arrives at a path that looks like a panel route
  // and a failed sign-in arrives back at one, and neither should be booted into.
  const failure = readPanelFailure(document);
  const invitationToken = parseInvitationToken(base, window.location.pathname);
  // Built from the mount point rather than imported, because Vite would bake the
  // sentinel into the JS bundle and only `index.html` is rewritten when serving.
  if (failure !== null) {
    mount(ErrorPage, {
      target,
      props: { api, base, build, failure },
    });
  } else if (invitationToken === null) {
    mount(App, {
      target,
      props: { api, base, build, router: createPanelRouter(base, window) },
    });
  } else {
    mount(InvitationPage, {
      target,
      props: { api, base, token: invitationToken, build },
    });
  }
} catch (error) {
  // Reaching here means the page itself was served wrong, so there is no
  // working app to show the failure inside.
  target.textContent = error instanceof Error ? error.message : String(error);
}
