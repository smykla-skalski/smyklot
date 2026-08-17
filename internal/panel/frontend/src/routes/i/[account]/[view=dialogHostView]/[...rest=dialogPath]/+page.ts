import type { PageLoad } from './$types';

import { guardDialogRest } from '$lib/route-guard.ts';

export const load: PageLoad = ({ params }) => {
  guardDialogRest(params.view, params.rest);
};
