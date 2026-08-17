import type { PageLoad } from './$types';

import { guardRootAccessRest } from '#lib/route-guard.js';

export const load: PageLoad = ({ params }) => {
  guardRootAccessRest(params.section, params.rest);
};
