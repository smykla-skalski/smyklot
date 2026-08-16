import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';
import { configDefaults } from 'vitest/config';

import { mockServer } from './dev/mock-server';

// The panel's mount point is a runtime flag (`--base-path`), but Vite bakes
// `base` into the emitted asset URLs at build time. Building against a sentinel
// and having the Go asset handler substitute the configured prefix into
// `index.html` keeps one build correct under any mount point. Nothing outside
// `index.html` is rewritten, so the sentinel must not appear in the bundles.
export default defineConfig({
  base: '/__smyklot_panel_base__/',
  experimental: {
    // Assets referenced from JS and CSS resolve relative to their importer
    // instead of the baked base, so the sentinel stays out of the bundles.
    // index.html keeps the default handling: the Go server rewrites it.
    renderBuiltUrl(_filename, { hostType }) {
      return hostType === 'html' ? undefined : { relative: true };
    },
  },
  // The mock no-ops unless `SMYKLOT_PANEL_DEV_MOCK=1`, so the build and the
  // default dev server are unaffected by it being listed here.
  plugins: [svelte(), mockServer()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // The service worker reads this graph at install time so every hashed
    // application asset is available before it takes control.
    manifest: 'cache-manifest.json',
  },
  test: {
    environment: 'node',
    include: ['tests/**/*.test.ts'],
    // The browser budget boots a dev server and drives Chrome, which costs ten
    // times what everything else here costs put together. It runs from
    // `vitest.browser.config.ts`, so that `npm test` stays a loop worth running
    // on every save.
    exclude: [...configDefaults.exclude, 'tests/browser/**'],
  },
});
