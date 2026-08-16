import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ params, url }) => {
  if (params.section === undefined) {
    redirect(307, `${url.pathname.replace(/\/$/u, '')}/audit`);
  }
};
