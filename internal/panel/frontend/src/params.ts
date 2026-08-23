import { defineParams } from '@sveltejs/kit/params';

import { DIALOG_HOST_VIEWS } from './lib/route-dialogs.ts';
import {
  ACCESS_SECTIONS,
  DIRECT_PANEL_VIEWS,
  DIRECT_ROOT_INSTALLATION_VIEWS,
  HISTORY_SECTIONS,
  REPOSITORY_SECTIONS,
  WRITTEN_SYNC_SECTIONS,
} from './lib/routes.ts';

/**
 * A matcher over a fixed list of segments.
 *
 * The list gives both halves at once: the expression the Go server is handed, and the
 * type the router hands the page. A matcher that accepted something outside the list,
 * or typed a parameter wider than the list allows, cannot be written this way.
 */
function oneOf<T extends readonly string[]>(values: T) {
  return matching<T[number]>(`^(?:${values.join('|')})$`);
}

/** A matcher over a shape rather than a list, where there is nothing to enumerate. */
function matching<T extends string = string>(pattern: string) {
  const accepted = new RegExp(pattern);

  return {
    pattern,
    match: (param: string): T | undefined => (accepted.test(param) ? (param as T) : undefined),
  };
}

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
const MATCHERS = {
  /** The tables the Root console's access page is split into. */
  accessSection: oneOf(ACCESS_SECTIONS),

  /**
   * The views that have anything after them in an address.
   *
   * Settings, sync and history host no dialog, so nothing follows them and the route
   * that takes a trailing segment must not match them. That is the whole point of a
   * separate matcher: an address like `/i/acme/settings/anything` resolves to no route
   * at all, and the server answers 404 with the panel's own page rather than 200 with
   * a not-found drawn after the shell has booted.
   */
  dialogHostView: oneOf(DIALOG_HOST_VIEWS),

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
  dialogPath: matching('^(?:[^/]+(?:/[^/]+)?)?$'),

  /** The two tables history is read through. */
  historySection: oneOf(HISTORY_SECTIONS),

  /**
   * An invitation token: 32 bytes, base64url, so 43 characters and no padding.
   *
   * The shape is worth checking before the page loads. An address that cannot be a
   * token is not an invitation that expired, and answering it with the invitation page
   * would spend a round trip to say so. The Go server refuses the same shape from this
   * pattern, which is where it used to be spelled a second time by hand.
   */
  invitationToken: matching('^[A-Za-z0-9_-]{43}$'),

  /**
   * The views an installation address may name, taken from the list itself.
   *
   * A second copy is a copy that drifts, and it did: the sync view was added to every
   * other list and this one still refused it, so the row in the navigation led to the
   * not-found page and a reload of the address did too.
   */
  panelView: oneOf(DIRECT_PANEL_VIEWS),

  /**
   * The panes one repository's page can open on.
   *
   * The segment is optional in the route, so what makes it worth matching is the segment
   * that is not one: `/i/acme/repositories/api-gateway/nonsense` resolves to no route at
   * all and is answered 404 from the wire, rather than reaching the page and quietly
   * opening the pane it starts on.
   */
  repositorySection: oneOf(REPOSITORY_SECTIONS),

  /**
   * The views the Root console renders for an installation, which are fewer than the
   * installation itself has.
   *
   * A separate matcher from `panelView` because the difference is a boundary and not an
   * oversight: an address the console has no page for must answer with the not-found
   * page rather than with a shell that says the view is unavailable, which reads as a
   * fault. The two lists used to be told apart by a third copy of both, written in Go.
   */
  rootInstallationView: oneOf(DIRECT_ROOT_INSTALLATION_VIEWS),

  /**
   * The sync sections written into an address; the overview leaves the bare
   * view, the way a repository's `file` pane does.
   */
  syncSection: oneOf(WRITTEN_SYNC_SECTIONS),

  /**
   * A template's path in an address: one or more non-empty segments, slashes
   * and all - a rest parameter is handed the whole tail as one string. Empty
   * is refused, so the bare files section stays on the section route.
   */
  syncFilePath: matching('^[^/]+(?:/[^/]+)*$'),
};

/** One half of every matcher, keyed the way `MATCHERS` is. */
function each<K extends 'pattern' | 'match'>(half: K) {
  return Object.fromEntries(
    Object.entries(MATCHERS).map(([name, matcher]) => [name, matcher[half]]),
  ) as { [N in keyof typeof MATCHERS]: (typeof MATCHERS)[N][K] };
}

/**
 * Read by `build/route-manifest.ts`, which hands each one to the Go server.
 *
 * Marked pure so the bundler drops it: nothing in the browser reads it, but it cannot
 * prove that of an `Object.fromEntries` on its own and would ship the fold.
 */
export const patterns = /* @__PURE__ */ each('pattern');

/**
 * The matchers the router runs, which are the same objects the patterns came from.
 *
 * A SvelteKit 3 matcher returns the parsed parameter rather than a boolean, and that
 * return type is what the router gives `page.params`. So `page.params.view` arrives as
 * the union of the views rather than as `string`, and the casts that used to stand at
 * every route component are gone.
 */
export const params = defineParams(each('match'));
