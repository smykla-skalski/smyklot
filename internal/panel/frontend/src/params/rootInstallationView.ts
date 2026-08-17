import type { ParamMatcher } from '@sveltejs/kit';

import { ROOT_INSTALLATION_VIEWS } from '../lib/routes.ts';

/**
 * The views the Root console renders for an installation, which are fewer than
 * the installation itself has.
 *
 * A separate matcher from `panelView` because the difference is a boundary and
 * not an oversight: an address the console has no page for must answer with the
 * not-found page rather than with a shell that says the view is unavailable,
 * which reads as a fault. The two lists used to be told apart by a third copy of
 * both, written in Go; this is what tells them apart now, and it is built from
 * the list rather than repeating it.
 *
 * See `panelView` for why this is a pattern and why the import is a path.
 */
export const pattern = `^(?:${ROOT_INSTALLATION_VIEWS.join('|')})$`;

const accepted = new RegExp(pattern);

export const match: ParamMatcher = (param) => accepted.test(param);
