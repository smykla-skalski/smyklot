import type { ParamMatcher } from '@sveltejs/kit';

/**
 * An invitation token: 32 bytes, base64url, so 43 characters and no padding.
 *
 * The shape is worth checking before the page loads. An address that cannot be a
 * token is not an invitation that expired, and answering it with the invitation page
 * would spend a round trip to say so. The Go server refuses the same shape from this
 * pattern, which is where it used to be spelled a second time by hand.
 */
export const pattern = '^[A-Za-z0-9_-]{43}$';

const accepted = new RegExp(pattern);

export const match: ParamMatcher = (param) => accepted.test(param);
