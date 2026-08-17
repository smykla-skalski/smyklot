/**
 * Where each side of the panel was last left.
 *
 * There are two: a workspace under `/i/<account>`, and the Root console under `/root`.
 * Crossing between them is a return rather than a jump - Root console opens the page the
 * console was left on, and Exit Root opens the page the console was entered from. Each
 * side remembers its own, so neither crossing overwrites the other's answer.
 *
 * Kept for the tab rather than for the browser. Two tabs can be in two different places,
 * and where one of them came from is a fact about that journey rather than an answer
 * worth carrying to another device. It does outlive a reload: the console is somewhere
 * people sit and reload, and both crossings have to still mean the page they left.
 *
 * What is stored is the route SvelteKit matched and the parameters it matched with,
 * never an address. An address is written against the route tree of the build that wrote
 * it and a tab outlives a deploy, so a remembered address can name a route that has since
 * been renamed. A remembered id is read back through `panelRouteAt`, which answers `null`
 * for an id it does not know - so a route that has been renamed reads as nothing
 * remembered rather than as a link to a page that is not there any more.
 */

import type { RouteId } from '$app/types';

import { panelRouteAt } from './addresses.ts';
import { browserStorage } from './preferences.ts';
import type { InstallationRoute, PanelRoute, RootRoute } from './routes.ts';

/** The two sides of the panel, each of which remembers where it was left. */
export type PanelSide = 'workspace' | 'console';

const KEYS: Readonly<Record<PanelSide, string>> = {
  workspace: 'smyklot.panel.last-page.workspace',
  console: 'smyklot.panel.last-page.console',
};

/**
 * The page a workspace was last on, which is where Exit Root goes.
 *
 * The narrowing is the reading: a stored page that is not a workspace page - a key
 * written by an older build, or edited by hand - is one nothing here can use, so it reads
 * as nothing remembered.
 */
export function readLastWorkspacePage(
  storage: Storage | null = browserStorage('session'),
): InstallationRoute | null {
  const route = readLastPage('workspace', storage);
  if (route === null || 'rootView' in route || 'personal' in route) return null;

  return route;
}

/** The page the Root console was last on, which is where Root console goes. */
export function readLastConsolePage(
  storage: Storage | null = browserStorage('session'),
): RootRoute | null {
  const route = readLastPage('console', storage);
  if (route === null || !('rootView' in route)) return null;

  return route;
}

export function writeLastPage(
  side: PanelSide,
  id: RouteId | null,
  params: Readonly<Record<string, string | undefined>>,
  storage: Storage | null = browserStorage('session'),
): void {
  if (id === null) return;

  try {
    storage?.setItem(KEYS[side], JSON.stringify({ id, params }));
  } catch {
    // A store that is full or refused costs a reader the page they left, and nothing else.
  }
}

function readLastPage(side: PanelSide, storage: Storage | null): PanelRoute | null {
  if (storage === null) return null;

  try {
    const stored = storage.getItem(KEYS[side]);

    return stored === null ? null : routeOf(JSON.parse(stored));
  } catch {
    return null;
  }
}

function routeOf(value: unknown): PanelRoute | null {
  if (typeof value !== 'object' || value === null) return null;
  const { id, params } = value as Record<string, unknown>;
  if (typeof id !== 'string') return null;
  const matched = asParams(params);

  // The cast claims nothing that is not then checked: `panelRouteAt` answers `null` for
  // every id it does not know, which is what an id from an older build reads as.
  return matched === null ? null : panelRouteAt(id as RouteId, matched);
}

function asParams(value: unknown): Record<string, string> | null {
  if (typeof value !== 'object' || value === null) return null;
  const params: Record<string, string> = {};
  for (const [key, entry] of Object.entries(value)) {
    if (typeof entry !== 'string') return null;
    params[key] = entry;
  }

  return params;
}
