/**
 * Test setup for jsdom-based component tests.
 *
 * Loaded by every test file with `// @vitest-environment jsdom` at the top.
 * Provides the DOM globals that Svelte 5 + testing-library need to mount
 * components in a server-side test runner.
 */
import { cleanup } from '@testing-library/svelte';
import { afterEach } from 'vitest';

// testing-library mounts Svelte components into the document; without
// cleanup between tests, a previous render's nodes persist and pollute
// the next test's DOM queries.
afterEach(() => {
  cleanup();
});
