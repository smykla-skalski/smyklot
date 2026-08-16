import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

import { mockServer } from './dev/mock-server';

// The mock no-ops unless `SMYKLOT_PANEL_DEV_MOCK=1`, so the build and the
// default dev server are unaffected by it being listed here.
export default defineConfig({
  plugins: [sveltekit(), mockServer()],
  test: {
    environment: 'node',
    include: ['tests/**/*.test.ts'],
  },
});
