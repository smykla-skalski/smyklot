import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

/** Runtime opens on its first navigation leaf. */
export const load: PageLoad = ({ url }) => {
  redirect(308, `${url.pathname.replace(/\/runtime\/?$/u, '/runtime/service')}${url.search}`);
};
