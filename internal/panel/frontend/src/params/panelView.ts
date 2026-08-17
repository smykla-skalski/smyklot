import type { ParamMatcher } from '@sveltejs/kit';

import { PANEL_VIEWS } from '../lib/routes.ts';

/**
 * The views an installation address may name, taken from the list itself.
 *
 * A second copy here is a copy that drifts, and it did: the sync view was added
 * to every other list and this one still refused it, so the row in the
 * navigation led to the not-found page and a reload of the address did too.
 *
 * The same list has to reach one place further. The Go server decides whether an
 * address gets the application shell or the not-found page, and it decides a
 * request before any of this has loaded, so the build reads `pattern` into the
 * route manifest it matches against. Built from `PANEL_VIEWS` rather than
 * written out, for the reason above.
 *
 * Imported by its path rather than through `$lib`: the build reads this module
 * with plain Node, which knows nothing of SvelteKit's aliases.
 */
export const pattern = `^(?:${PANEL_VIEWS.join('|')})$`;

const accepted = new RegExp(pattern);

export const match: ParamMatcher = (param) => accepted.test(param);
