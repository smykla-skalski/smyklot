import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { svelteTesting } from '@testing-library/svelte/vite';
import { defineConfig } from 'vitest/config';
import { configDefaults } from 'vitest/config';

import { withRouteManifest } from './build/route-manifest.ts';
import { mockServer } from './dev/mock-server.ts';

// In dev the mock server mounts at /, so SvelteKit's router must not enforce
// the production base. The build keeps the sentinel so the Go server can
// resolve it at startup.
const isMockDev = process.env.SMYKLOT_PANEL_DEV_MOCK === '1';

// The mock no-ops unless `SMYKLOT_PANEL_DEV_MOCK=1`, so the build and the
// default dev server are unaffected by it being listed here.
export default defineConfig({
  plugins: [
    sveltekit({
      preprocess: vitePreprocess(),
      // The manifest ships beside the bundle so the Go server answers the same
      // addresses this router does. See `build/route-manifest.ts`.
      adapter: withRouteManifest(
        adapter({
          pages: 'dist',
          assets: 'dist',
          fallback: 'index.html',
        }),
        { out: 'dist', params: 'src/params' },
      ),
      csp: {
        mode: 'hash',
        directives: {
          'default-src': ['self'],
          'connect-src': ['self'],
          'img-src': ['self', 'https:'],
          'style-src': ['self'],
          // Virtualized rows, data bars, and component dimensions are dynamic
          // style attributes. Keep style elements hash-restricted while allowing
          // that narrower channel.
          'style-src-attr': ['unsafe-inline'],
          'script-src': ['self'],
          'base-uri': ['none'],
          'form-action': ['self', 'https://github.com'],
        },
      },
      paths: {
        base: isMockDev ? '' : '/__smyklot_panel_base__',
      },
      version: {
        // The Go server resolves this in every text asset, including the
        // generated service worker, from the runtime deployment version.
        name: '__smyklot_panel_version__',
      },
    }),
    svelteTesting(),
    mockServer(),
  ],
  server: {
    port: 5175,
    strictPort: true,
  },
  test: {
    environment: 'node',
    include: ['tests/**/*.test.ts'],
    // The `test` script runs Node with `--no-experimental-webstorage`, and it has to. Node 26 turns
    // Web Storage on by default, which puts a `localStorage` accessor on `globalThis` that answers
    // `undefined` unless the process was given `--localstorage-file`. jsdom installs its globals by
    // copying and declines to overwrite a key that is already there, so its own storage never lands
    // and `window.localStorage` - the same object here, because the jsdom window is flattened onto
    // `globalThis` - reads as undefined. `sessionStorage` arrives intact, which is what makes this
    // look like anything but a Node flag.
    //
    // The silence is the reason it is worth a flag. `browserStorage()` in `lib/preferences.ts`
    // catches a throw, not an undefined, so on such a host every preference falls back to its
    // default and the storage-backed paths pass while proving nothing. The pinned toolchain is
    // Node 24, where the global does not exist and the copy works - so this bites a newer local
    // Node only, and quietly. The flag has to travel as `NODE_OPTIONS` rather than
    // `poolOptions.forks.execArgv`, which vitest overwrites when it builds the worker.
    server: {
      deps: {
        inline: [/svelte/, /@testing-library/],
      },
    },
    // The browser budget boots a dev server and drives Chrome, which costs ten
    // times what everything else here costs put together. It runs from
    // `vitest.browser.config.ts`, so that `npm test` stays a loop worth running
    // on every save.
    exclude: [...configDefaults.exclude, 'tests/browser/**'],
  },
});
