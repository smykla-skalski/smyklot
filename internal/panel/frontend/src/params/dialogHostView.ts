import type { ParamMatcher } from '@sveltejs/kit';

import { DIALOG_HOST_VIEWS } from '../lib/route-dialogs.ts';

/**
 * The views that have anything after them in an address.
 *
 * Settings, sync and history host no dialog, so nothing follows them and the
 * route that takes a trailing segment must not match them. That is the whole
 * point of a separate matcher: an address like `/i/acme/settings/anything`
 * resolves to no route at all, and the server answers 404 with the panel's own
 * page rather than 200 with a not-found drawn after the shell has booted.
 *
 * See `panelView` for why this is a pattern and why the import is a path.
 */
export const pattern = `^(?:${DIALOG_HOST_VIEWS.join('|')})$`;

const accepted = new RegExp(pattern);

export const match: ParamMatcher = (param) => accepted.test(param);
