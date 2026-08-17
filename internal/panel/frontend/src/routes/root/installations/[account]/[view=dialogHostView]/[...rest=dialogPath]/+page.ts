import type { PageLoad } from './$types';

import { guardDialogRest } from '#lib/route-guard.js';

export const load: PageLoad = ({ params }) => {
  guardDialogRest(params.view, params.rest);
};
