import type { PageLoad } from './$types';

import { guardHistorySection } from '$lib/route-guard.ts';

export const load: PageLoad = ({ params, url }) => {
  guardHistorySection(params.section, url.pathname);
};
