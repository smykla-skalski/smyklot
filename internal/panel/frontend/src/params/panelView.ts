import type { ParamMatcher } from '@sveltejs/kit';

import { PANEL_VIEWS } from '$lib/routes';

/**
 * The views an installation address may name, taken from the list itself.
 *
 * A second copy here is a copy that drifts, and it did: the sync view was added
 * to every other list and this one still refused it, so the row in the
 * navigation led to the not-found page and a reload of the address did too.
 */
const views: ReadonlySet<string> = new Set(PANEL_VIEWS);

export const match: ParamMatcher = (param) => views.has(param);
