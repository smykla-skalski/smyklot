import type { PageLoad } from './$types';

import { guardPanelViewRest } from '$lib/route-guard.ts';

export const load: PageLoad = ({ params, url }) => {
  guardPanelViewRest(params.view, params.rest, url.pathname);
};
