import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';

import { mockServer } from './dev/mock-server';

// The panel's mount point is a runtime flag (`--base-path`), but Vite bakes
// `base` into the emitted asset URLs at build time. Building against a sentinel
// and having the Go asset handler substitute the configured prefix into
// `index.html` keeps one build correct under any mount point. Nothing outside
// `index.html` is rewritten, so the sentinel must not appear in the bundles.
export default defineConfig({
  base: '/__smyklot_panel_base__/',
  // The mock no-ops unless `SMYKLOT_PANEL_DEV_MOCK=1`, so the build and the
  // default dev server are unaffected by it being listed here.
  plugins: [svelte(), mockServer()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'node',
    include: ['tests/**/*.test.ts'],
  },
});
