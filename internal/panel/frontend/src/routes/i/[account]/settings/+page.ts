import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

/** Compatibility for bookmarks from before workspace settings were named defaults. */
export const load: PageLoad = ({ url }) => {
  redirect(308, `${url.pathname.replace(/\/settings\/?$/u, '/defaults')}${url.search}`);
};
