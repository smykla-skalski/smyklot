import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

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
      paths: {
        base: isMockDev ? '' : '/__smyklot_panel_base__',
      },
    }),
    mockServer(),
  ],
  server: {
    port: 5275,
    strictPort: true,
  },
  test: {
    environment: 'node',
    include: ['tests/**/*.test.ts'],
    setupFiles: ['tests/component-setup.ts'],
    server: {
      deps: {
        inline: [/svelte/, /@testing-library/],
      },
    },
  },
});
