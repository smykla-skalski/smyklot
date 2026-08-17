import { SMYKLOT_PANEL_DEV_MOCK } from '$app/env/private';
import type { Handle } from '@sveltejs/kit';

import { rewriteMockHtml } from '../dev/mock-html';

export const handle: Handle = ({ event, resolve }) => {
  if (SMYKLOT_PANEL_DEV_MOCK !== '1') return resolve(event);

  return resolve(event, {
    transformPageChunk: ({ html }) => rewriteMockHtml(html),
  });
};
