import { defineParams } from '@sveltejs/kit/params';

import { DIALOG_HOST_VIEWS } from './lib/route-dialogs.ts';
import { HISTORY_SECTIONS, PANEL_VIEWS, ROOT_INSTALLATION_VIEWS } from './lib/routes.ts';

/**
 * Every parameter matcher, written as the expression it accepts.
 *
 * The pattern is the declaration and the matcher is derived from it, because the same
 * rule has to reach one place further than the router. The Go server decides whether
 * an address gets the application shell or the not-found page, and it decides a
 * request before any of this has loaded, so the build reads these into the route
 * manifest it matches against - see `build/route-manifest.ts`. A matcher written as
 * code rather than as a pattern is a rule the server cannot be handed, and the two
 * copies drift: the server used to carry its own in Go, and it did.
 *
 * Deriving the matchers rather than writing them beside the patterns is what makes
 * that hold. A matcher that accepted something its pattern does not cannot be
 * expressed here, so the manifest can never be narrower than the router.
 *
 * The two lists are imported by path rather than through `#lib`, because the build
 * reads this module with plain Node, which knows nothing of SvelteKit's aliases.
 */
export const patterns = {
  /** The tables the Root console's access page is split into. */
  accessSection: '^(?:users|invitations)$',

  /**
   * The views that have anything after them in an address.
   *
   * Settings, sync and history host no dialog, so nothing follows them and the route
   * that takes a trailing segment must not match them. That is the whole point of a
   * separate matcher: an address like `/i/acme/settings/anything` resolves to no route
   * at all, and the server answers 404 with the panel's own page rather than 200 with
   * a not-found drawn after the shell has booted.
   */
  dialogHostView: `^(?:${DIALOG_HOST_VIEWS.join('|')})$`,

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
   * the separator rather than a single segment. Empty is the common case: it is what a
   * view with no dialog open passes.
   */
  dialogPath: '^(?:[^/]+(?:/[^/]+)?)?$',

  /** The two tables history is read through. */
  historySection: `^(?:${HISTORY_SECTIONS.join('|')})$`,

  /**
   * An invitation token: 32 bytes, base64url, so 43 characters and no padding.
   *
   * The shape is worth checking before the page loads. An address that cannot be a
   * token is not an invitation that expired, and answering it with the invitation page
   * would spend a round trip to say so. The Go server refuses the same shape from this
   * pattern, which is where it used to be spelled a second time by hand.
   */
  invitationToken: '^[A-Za-z0-9_-]{43}$',

  /**
   * The views an installation address may name, taken from the list itself.
   *
   * A second copy is a copy that drifts, and it did: the sync view was added to every
   * other list and this one still refused it, so the row in the navigation led to the
   * not-found page and a reload of the address did too.
   */
  panelView: `^(?:${PANEL_VIEWS.join('|')})$`,

  /**
   * The views the Root console renders for an installation, which are fewer than the
   * installation itself has.
   *
   * A separate matcher from `panelView` because the difference is a boundary and not an
   * oversight: an address the console has no page for must answer with the not-found
   * page rather than with a shell that says the view is unavailable, which reads as a
   * fault. The two lists used to be told apart by a third copy of both, written in Go.
   */
  rootInstallationView: `^(?:${ROOT_INSTALLATION_VIEWS.join('|')})$`,
} as const satisfies Record<string, string>;

/** The name of a matcher, as a route spells it in `[view=panelView]`. */
type ParamName = keyof typeof patterns;

/**
 * The matchers themselves, one per pattern and derived from it.
 *
 * Derived rather than listed a second time, for the reason above: a list written twice
 * is a list that drifts, and the compiler cannot tell that the second one is short. A
 * SvelteKit 3 matcher returns the parsed parameter or `undefined`; these parse nothing,
 * so an accepted parameter comes back as it arrived.
 */
export const params = defineParams(
  Object.fromEntries(
    Object.entries(patterns).map(([name, pattern]) => {
      const accepted = new RegExp(pattern);

      return [name, (param: string) => (accepted.test(param) ? param : undefined)];
    }),
  ) as { [K in ParamName]: (param: string) => string | undefined },
);
