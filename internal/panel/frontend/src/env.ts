import { defineEnvVars } from '@sveltejs/kit/env';

/**
 * The environment SvelteKit is allowed to read, which SvelteKit 3 wants declared.
 *
 * One variable, and only the dev server reads it: `hooks.server.ts` rewrites the page
 * for the mock API when it is `1`. Absent falls back to the empty string on purpose -
 * the mock is off unless something turns it on, and a missing variable is the ordinary
 * way for it to be off rather than a condition worth failing the boot over.
 */
export const variables = defineEnvVars({
  SMYKLOT_PANEL_DEV_MOCK: { schema: (input: string | undefined) => input ?? '' },
});
