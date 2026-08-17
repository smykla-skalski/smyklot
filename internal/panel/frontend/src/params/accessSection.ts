import type { ParamMatcher } from '@sveltejs/kit';

/** The tables the Root console's access page is split into. See `panelView` for why this is a pattern. */
export const pattern = '^(?:users|invitations)$';

const accepted = new RegExp(pattern);

export const match: ParamMatcher = (param) => accepted.test(param);
