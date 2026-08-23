import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

import { guardDialogRest } from '#lib/route-guard.js';

export const load: PageLoad = ({ params, url }) => {
  guardDialogRest(params.view, params.rest);
  redirect(308, legacyAccessRedirect(url.pathname, url.search, params.rest));
};

function legacyAccessRedirect(pathname: string, search: string, rest: string | undefined): string {
  const parts = pathname.split('/');
  const restDepth = rest === undefined || rest === '' ? 0 : rest.split('/').length;
  parts.splice(parts.length - restDepth - 1, 0, 'access');
  return `${parts.join('/')}${search}`;
}
