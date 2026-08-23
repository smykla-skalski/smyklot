import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

/** Compatibility for bookmarks from before Root settings were named Runtime. */
export const load: PageLoad = ({ url }) => {
  redirect(308, `${url.pathname.replace(/\/settings\/?$/u, '/runtime/settings')}${url.search}`);
};
