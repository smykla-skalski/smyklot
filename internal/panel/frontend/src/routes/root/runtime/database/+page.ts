import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

/**
 * The database is part of Service health, and was its own page.
 *
 * The address stays because operators keep links, and because a 404 is a worse
 * answer than the page the reader was looking for - the store's engine, its
 * responsiveness and its pool are all on the service page now.
 */
export const load: PageLoad = ({ url }) => {
  redirect(
    308,
    `${url.pathname.replace(/\/runtime\/database\/?$/u, '/runtime/service')}${url.search}`,
  );
};
