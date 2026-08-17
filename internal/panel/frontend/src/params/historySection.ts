import type { ParamMatcher } from '@sveltejs/kit';

/** The two tables history is read through. See `panelView` for why this is a pattern. */
export const pattern = '^(?:audit|failures)$';

const accepted = new RegExp(pattern);

export const match: ParamMatcher = (param) => accepted.test(param);
