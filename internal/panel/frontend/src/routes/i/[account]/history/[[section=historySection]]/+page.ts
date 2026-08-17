import type { PageLoad } from './$types';

import { guardHistorySection } from '#lib/route-guard.js';

export const load: PageLoad = ({ params, url }) => {
  guardHistorySection(params.section, url.pathname);
};
