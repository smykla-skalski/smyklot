import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { svelteTesting } from '@testing-library/svelte/vite';
import { defineConfig } from 'vitest/config';
import { configDefaults } from 'vitest/config';

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
      adapter: adapter({
        pages: 'dist',
        assets: 'dist',
        fallback: 'index.html',
      }),
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
