import type { Handle } from '@sveltejs/kit/hooks';
import { SMYKLOT_PANEL_DEV_MOCK } from '$app/env/private';
import { rewriteMockHtml } from '../dev/mock-html';

export const handle: Handle = ({ event, resolve }) => {
  if (!SMYKLOT_PANEL_DEV_MOCK) return resolve(event);

  return resolve(event, {
    transformPageChunk: ({ html }) => rewriteMockHtml(html),
  });
};
