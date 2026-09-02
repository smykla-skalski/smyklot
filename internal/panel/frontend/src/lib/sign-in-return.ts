/**
 * Getting a reader back to the address they asked for, and telling them what
 * stopped them.
 *
 * A pasted link to a deep page is the ordinary way into one. The sign-in round
 * trip used to drop it - everybody landed on the front page of everything - and
 * the card said "you come back here afterwards" while doing it.
 *
 * Both halves cross as the URL, because the round trip leaves the app entirely
 * and comes back on a fresh document: nothing in memory survives GitHub.
 */
import type { PanelFailure } from './panel-error.ts';
import { basePath } from './paths.ts';

/** What the server calls the failed sign-in it is handing back. */
export const SIGN_IN_FAILED_PARAM = 'signin_failed';

/**
 * The sign-in failure the server redirected with, if any.
 *
 * `status:code`, which is the key the panel's one table of error words is
 * already written under - so a failure is worded the same here as on the pages,
 * and rewording it is a single edit. Anything malformed is nothing: a reader
 * whose address was mangled wants the sign-in card, not a complaint about the
 * address.
 */
export function readSignInFailure(search: string): PanelFailure | null {
  const value = new URLSearchParams(search).get(SIGN_IN_FAILED_PARAM);
  if (value === null) return null;
  const cut = value.indexOf(':');
  if (cut <= 0) return null;
  const status = Number.parseInt(value.slice(0, cut), 10);
  if (!Number.isInteger(status) || status < 400 || status > 599) return null;
  const code = value.slice(cut + 1);
  if (!/^[a-z_]{1,64}$/u.test(code)) return null;

  return { status, code, message: '' };
}

/** The panel's own front page, which is not worth coming back to deliberately. */
function isLanding(pathname: string): boolean {
  const trimmed = pathname.replace(/\/+$/u, '');

  return trimmed === basePath.replace(/\/+$/u, '');
}

/**
 * The address to come back to after signing in, or null for the landing page.
 *
 * The reader's current address, because a signed-out reader is shown the sign-in
 * card wherever they are - so where they are IS what they asked for. The failure
 * parameter is dropped: a second attempt should not carry the first one's error
 * back to a page that has already shown it.
 *
 * Only a path, never an origin. The server refuses anything else outright - see
 * `safeReturnPath` in internal/panel/auth.go - and this has no business
 * constructing something it would have to refuse.
 */
export function signedOutReturn(pathname: string, search: string): string | null {
  if (isLanding(pathname)) return null;
  const query = new URLSearchParams(search);
  query.delete(SIGN_IN_FAILED_PARAM);
  const remaining = query.toString();

  return remaining === '' ? pathname : `${pathname}?${remaining}`;
}
