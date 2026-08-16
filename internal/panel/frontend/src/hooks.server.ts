import { env } from '$env/dynamic/private';
import type { Handle } from '@sveltejs/kit';

import { rewriteMockHtml } from '../dev/mock-html';

export const handle: Handle = ({ event, resolve }) => {
  if (env.SMYKLOT_PANEL_DEV_MOCK !== '1') return resolve(event);

  return resolve(event, {
    transformPageChunk: ({ html }) => rewriteMockHtml(html),
  });
};
