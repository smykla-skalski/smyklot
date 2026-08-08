import { mount } from 'svelte';

import App from './App.svelte';
import './app.css';
import { createPanelApi } from './lib/api';
import { MONITOR_ICON_PATH, panelUrl, readBasePath, readPanelBuild } from './lib/base';

const target = document.querySelector('#app');

if (target === null) {
  throw new Error('the panel page is missing its #app mount point');
}

try {
  const base = readBasePath(document);
  const api = createPanelApi(base, (input, init) => fetch(input, init));
  // Built from the mount point rather than imported, because Vite would bake the
  // sentinel into the JS bundle and only `index.html` is rewritten when serving.
  mount(App, {
    target,
    props: {
      api,
      iconUrl: panelUrl(base, MONITOR_ICON_PATH),
      build: readPanelBuild(document),
    },
  });
} catch (error) {
  // Reaching here means the page itself was served wrong, so there is no
  // working app to show the failure inside.
  target.textContent = error instanceof Error ? error.message : String(error);
}
