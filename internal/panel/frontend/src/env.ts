import { defineEnvVars } from '@sveltejs/kit/env';

// @migration-task Review usage of dynamic environment variables. They fall back to the empty string if not present, which may not be what you want.
export const variables = defineEnvVars({
  SMYKLOT_PANEL_DEV_MOCK: { schema: (input) => input ?? '' },
});
