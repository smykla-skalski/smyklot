import type { RouteId } from '$app/types';
import { describe, expect, it } from 'vitest';

import { panelAddress, panelRouteAt } from '../src/lib/addresses.ts';
import { basePath } from '../src/lib/paths.ts';
import { parsePanelRoute, type PanelRoute } from '../src/lib/routes.ts';

/**
 * Every shape the panel can be looking at, with the route SvelteKit matches it to.
 *
 * Written out rather than derived, because deriving it would mean re-running the very
 * mapping under test - but typed as `RouteId`, so an id that no longer names a route
 * fails the type check rather than sitting here as a string nobody compares.
 *
 * What proves the ids are the ones SvelteKit reports is the browser suite, which drives
 * real navigation. What this proves is that reading a route back gives the shape that
 * wrote it, and that the reading agrees with the hand parser the mock server still uses -
 * two implementations, checked against each other.
 */
const CASES: Array<{ route: PanelRoute; id: RouteId; params: Record<string, string> }> = [
  { route: { personal: 'inbox' }, id: '/inbox', params: {} },
  {
    route: { account: 'acme', view: 'defaults' },
    id: '/i/[account]/[view=panelView]',
    params: { account: 'acme', view: 'defaults' },
  },
  {
    route: { account: 'acme', view: 'repositories' },
    id: '/i/[account]/[view=panelView]',
    params: { account: 'acme', view: 'repositories' },
  },
  {
    route: { account: 'acme', view: 'sync' },
    id: '/i/[account]/[view=panelView]',
    params: { account: 'acme', view: 'sync' },
  },
  {
    route: { account: 'smykla skalski', view: 'repositories' },
    id: '/i/[account]/[view=panelView]',
    params: { account: 'smykla skalski', view: 'repositories' },
  },
  {
    route: { account: 'acme', view: 'history', section: 'failures' },
    id: '/i/[account]/history/[[section=historySection]]',
    params: { account: 'acme', section: 'failures' },
  },
  {
    route: { account: 'acme', view: 'sync', sync: 'plan' },
    id: '/i/[account]/sync/[section=syncSection]',
    params: { account: 'acme', section: 'plan' },
  },
  {
    route: { account: 'acme', view: 'sync', sync: 'rulesets' },
    id: '/i/[account]/sync/[section=syncSection]',
    params: { account: 'acme', section: 'rulesets' },
  },
  {
    route: { account: 'acme', view: 'sync', sync: 'rulesets', syncRuleset: 'main-protection' },
    id: '/i/[account]/sync/rulesets/[ruleset]',
    params: { account: 'acme', ruleset: 'main-protection' },
  },
  {
    // A bare repository segment means the pane the page opens on, which reading it back
    // says out loud - so the shape that comes out is the normalised one.
    route: {
      account: 'acme',
      view: 'repositories',
      repository: { name: 'api-gateway', section: 'file' },
    },
    id: '/i/[account]/repositories/[repository]/[[section=repositorySection]]',
    params: { account: 'acme', repository: 'api-gateway' },
  },
  {
    route: { account: 'acme', view: 'users' },
    id: '/i/[account]/access/[section=accessSection]/[...rest=dialogPath]',
    params: { account: 'acme', section: 'users', rest: '' },
  },
  {
    route: {
      account: 'acme',
      view: 'repositories',
      repository: { name: 'api-gateway', section: 'commands' },
    },
    id: '/i/[account]/repositories/[repository]/[[section=repositorySection]]',
    params: { account: 'acme', repository: 'api-gateway', section: 'commands' },
  },
  {
    route: {
      account: 'acme',
      view: 'users',
      dialog: { name: 'user-action', params: { user: 'octocat', action: 'suspend' } },
    },
    id: '/i/[account]/access/[section=accessSection]/[...rest=dialogPath]',
    params: { account: 'acme', section: 'users', rest: 'octocat/suspend' },
  },
  { route: { rootView: 'overview' }, id: '/root', params: {} },
  { route: { rootView: 'installations' }, id: '/root/installations', params: {} },
  { route: { rootView: 'queue' }, id: '/root/queue', params: {} },
  { route: { rootView: 'queue-recent' }, id: '/root/queue/recent', params: {} },
  {
    route: { rootView: 'queue-request', request: 'req-1' },
    id: '/root/queue/request/[id]',
    params: { id: 'req-1' },
  },
  { route: { rootView: 'runtime-service' }, id: '/root/runtime/service', params: {} },
  { route: { rootView: 'runtime-database' }, id: '/root/runtime/database', params: {} },
  { route: { rootView: 'runtime-settings' }, id: '/root/runtime/settings', params: {} },
  {
    route: { rootView: 'history-audit' },
    id: '/root/history/[[section=historySection]]',
    params: { section: 'audit' },
  },
  {
    route: { rootView: 'history-failures' },
    id: '/root/history/[[section=historySection]]',
    params: { section: 'failures' },
  },
  {
    route: { rootView: 'access-users' },
    id: '/root/access/[section=accessSection]/[...rest=dialogPath]',
    params: { section: 'users', rest: '' },
  },
  {
    route: { rootView: 'access-invitations' },
    id: '/root/access/[section=accessSection]/[...rest=dialogPath]',
    params: { section: 'invitations', rest: '' },
  },
  {
    route: {
      rootView: 'access-users',
      dialog: { name: 'root-user-action', params: { user: 'octocat', action: 'ban' } },
    },
    id: '/root/access/[section=accessSection]/[...rest=dialogPath]',
    params: { section: 'users', rest: 'octocat/ban' },
  },
  {
    route: { rootView: 'installation', account: 'acme', view: 'defaults' },
    id: '/root/installations/[account]/[view=rootInstallationView]',
    params: { account: 'acme', view: 'defaults' },
  },
  {
    route: { rootView: 'installation', account: 'acme', view: 'repositories' },
    id: '/root/installations/[account]/[view=rootInstallationView]',
    params: { account: 'acme', view: 'repositories' },
  },
  {
    route: { rootView: 'installation', account: 'acme', view: 'invitations' },
    id: '/root/installations/[account]/access/[section=accessSection]/[...rest=dialogPath]',
    params: { account: 'acme', section: 'invitations', rest: '' },
  },
  {
    route: { rootView: 'installation', account: 'acme', view: 'history', section: 'failures' },
    id: '/root/installations/[account]/history/[[section=historySection]]',
    params: { account: 'acme', section: 'failures' },
  },
  {
    route: {
      rootView: 'installation',
      account: 'acme',
      view: 'repositories',
      repository: { name: 'api-gateway', section: 'behavior' },
    },
    id: '/root/installations/[account]/repositories/[repository]/[[section=repositorySection]]',
    params: { account: 'acme', repository: 'api-gateway', section: 'behavior' },
  },
];

/** Drops the keys `panelRouteAt` leaves undefined, so `toEqual` compares like with like. */
function defined(route: PanelRoute | null): PanelRoute | null {
  if (route === null) return null;

  return Object.fromEntries(
    Object.entries(route).filter(([, value]) => value !== undefined),
  ) as PanelRoute;
}

describe('panel addresses [Unit]', () => {
  it.each(CASES.map((entry) => [entry.id, entry] as const))(
    'reads %s back into the route that wrote it',
    (_id, { route, id, params }) => {
      expect(defined(panelRouteAt(id, params))).toEqual(route);
    },
  );

  it('reads the same route the hand parser reads from the address', () => {
    for (const { route, id, params } of CASES) {
      const address = panelAddress(route);

      expect(defined(panelRouteAt(id, params)), address).toEqual(
        parsePanelRoute(basePath, address),
      );
    }
  });

  it('opens the Runtime parent on its first leaf', () => {
    expect(panelRouteAt('/root/runtime', {})).toEqual({ rootView: 'runtime-service' });
    expect(panelRouteAt('/root/runtime/service', {})).toEqual({ rootView: 'runtime-service' });
  });

  it('carries a name through the address without decoding it twice', () => {
    // The router hands its parameters over decoded. Decoding them again loses a per-cent
    // sign and throws outright when the two characters after it are not hexadecimal, so
    // the dialog came back as none and the address resolved to the bare view.
    const route: PanelRoute = {
      account: 'acme',
      view: 'users',
      dialog: { name: 'decision-history', params: { user: 'a%b' } },
    };
    const address = panelAddress(route);

    expect(address).toBe(`${basePath}/i/acme/access/users/a%25b/history`);
    expect(
      defined(
        panelRouteAt('/i/[account]/access/[section=accessSection]/[...rest=dialogPath]', {
          account: 'acme',
          section: 'users',
          rest: 'a%b/history',
        }),
      ),
    ).toEqual(route);
    // The hand parser still holds a raw pathname, so it decodes and agrees.
    expect(parsePanelRoute(basePath, address)).toEqual(route);
  });

  it('carries a repository name the same way, as a segment of its own', () => {
    const route: PanelRoute = {
      account: 'acme',
      view: 'repositories',
      repository: { name: 'a%b', section: 'file' },
    };
    const address = panelAddress(route);

    expect(address).toBe(`${basePath}/i/acme/repositories/a%25b`);
    expect(
      defined(
        panelRouteAt('/i/[account]/repositories/[repository]/[[section=repositorySection]]', {
          account: 'acme',
          repository: 'a%b',
        }),
      ),
    ).toEqual(route);
    expect(parsePanelRoute(basePath, address)).toEqual(route);
  });

  it('carries a template path without decoding its percent signs twice', () => {
    const route: PanelRoute = {
      account: 'acme',
      view: 'sync',
      sync: 'files',
      syncFile: 'docs/100%bad.json',
    };
    const address = panelAddress(route);

    expect(address).toBe(`${basePath}/i/acme/sync/files/docs/100%25bad.json`);
    expect(
      defined(
        panelRouteAt('/i/[account]/sync/files/[...file=syncFilePath]', {
          account: 'acme',
          file: 'docs/100%bad.json',
        }),
      ),
    ).toEqual(route);
    expect(parsePanelRoute(basePath, address)).toEqual(route);
  });

  it('names the view even when the segments after it name no dialog', () => {
    // The tail names nothing, but the address still names the view underneath, and that is
    // what the chrome around the page shows. Whether the page resolved is SvelteKit's
    // answer: the load guard raises 404 and `page.error` carries it, which is what stops
    // the view being recorded as one the reader was on.
    expect(
      defined(
        panelRouteAt('/i/[account]/access/[section=accessSection]/[...rest=dialogPath]', {
          account: 'acme',
          section: 'users',
          rest: 'bogus/bogus2',
        }),
      ),
    ).toEqual({ account: 'acme', view: 'users' });
    expect(
      defined(
        panelRouteAt('/root/access/[section=accessSection]/[...rest=dialogPath]', {
          section: 'users',
          rest: 'octocat/befriend',
        }),
      ),
    ).toEqual({ rootView: 'access-users' });
  });

  it('has no route for an address outside the panel', () => {
    expect(panelRouteAt('/', {})).toBeNull();
    expect(panelRouteAt('/invite/[token=invitationToken]', { token: 'x' })).toBeNull();
    expect(panelRouteAt(null, {})).toBeNull();
  });
});
