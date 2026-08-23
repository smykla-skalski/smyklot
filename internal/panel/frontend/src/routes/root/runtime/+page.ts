import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

/** Runtime opens on its editable settings leaf. */
export const load: PageLoad = ({ url }) => {
  redirect(308, `${url.pathname.replace(/\/runtime\/?$/u, '/runtime/settings')}${url.search}`);
};
