import { defineConfig } from 'vitest/config';

// A separate config so `npm test` stays what it is: 877 tests in two seconds,
// which is a loop worth running on every save. Driving a browser costs twenty,
// and the two do not belong in the same command. `vite.config.ts` excludes this
// directory for the same reason.
export default defineConfig({
  test: {
    environment: 'node',
    include: ['tests/browser/**/*.test.ts'],
    // Booting a dev server and a browser is most of it.
    testTimeout: 120_000,
    hookTimeout: 120_000,
    // One measurement at a time. Two files racing for the same cores would read
    // as a page that is slow to settle rather than a page that is quiet.
    fileParallelism: false,
  },
});
