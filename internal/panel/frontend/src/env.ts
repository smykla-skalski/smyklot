import { defineEnvVars } from '@sveltejs/kit/env';

/**
 * The environment SvelteKit is allowed to read, which SvelteKit 3 wants declared.
 *
 * One variable, and only the dev server reads it: `hooks.server.ts` rewrites the page
 * for the mock API when it is on. The schema decides it here rather than handing the
 * raw string on, so `=== '1'` is written in one place instead of at every reader, and
 * an absent variable is simply off - the ordinary way for a mock to be off, not a
 * condition worth failing the boot over.
 */
export const variables = defineEnvVars({
  SMYKLOT_PANEL_DEV_MOCK: { schema: (input: string | undefined) => input === '1' },
});
