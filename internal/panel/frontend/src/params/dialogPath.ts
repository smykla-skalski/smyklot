import type { ParamMatcher } from '@sveltejs/kit';

/**
 * What a view may carry after it: nothing, or the one or two segments of a dialog
 * standing on that view.
 *
 * The segments are counted rather than read. Their grammar is a repository somebody
 * chose or a login somebody registered, and neither is knowable here - but the depth
 * is, and depth is the part worth refusing, because a rest parameter otherwise
 * matches to any depth and every one of them would render the same page.
 *
 * A rest parameter is handed the whole tail as one string, so this matches across
 * the separator rather than a single segment. Empty is the common case: it is what
 * a view with no dialog open passes.
 */
export const pattern = '^(?:[^/]+(?:/[^/]+)?)?$';

const accepted = new RegExp(pattern);

export const match: ParamMatcher = (param) => accepted.test(param);
