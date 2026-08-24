// @vitest-environment jsdom
import { QueryClient } from '@tanstack/svelte-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const navigation = vi.hoisted(() => ({ goto: vi.fn() }));
const routePage = vi.hoisted(() => ({
  params: {} as Record<string, string>,
  /** What SvelteKit says went wrong loading this page, which gates what is recorded. */
  error: null as { message: string } | null,
  // The route SvelteKit matched. The panel reads what is open from this and the params
  // together, so a fixture that sets one without the other is not an address.
  route: { id: null } as { id: string | null },
  url: new URL('https://panel.example/'),
}));

vi.mock('$app/navigation', () => navigation);
vi.mock('$app/state', () => ({ page: routePage }));

import { basePath } from '../src/lib/paths.ts';
import { at } from './support/addresses.ts';
import { PanelSession } from '../src/lib/session.svelte.ts';

import type { PanelApi } from '../src/lib/api.ts';
import type { PanelBuild } from '../src/lib/base.ts';
import type { PanelChangeEvent } from '../src/lib/events.ts';
import type { PanelTarget, PanelViewer } from '../src/lib/types.ts';

class TestMediaQueryList extends EventTarget implements MediaQueryList {
  matches = false;
  media = '';
  onchange = null;

  addListener(): void {}
  removeListener(): void {}
}

/**
 * A `Storage` for the session to keep preferences and the page it left in.
 *
 * jsdom has one, but Vitest 4 does not carry it out onto the environment's
 * globals, so `localStorage` is undefined here and on `window` both. The
 * session reads preferences through `window.localStorage` inside a `try` and
 * treats its absence as "no preferences", which is why nothing about this is
 * visible until the reset below asks the missing object to clear itself.
 *
 * Stubbed rather than worked around, because storage that is quietly always
 * empty is the worse outcome: every preference this file sets would be written
 * to nothing and read back as a default, and the tests would agree with each
 * other while proving the opposite of what they say.
 */
function memoryStorage(): Storage {
  const entries = new Map<string, string>();

  return {
    get length(): number {
      return entries.size;
    },
    clear: (): void => void entries.clear(),
    getItem: (key: string): string | null => entries.get(key) ?? null,
    key: (index: number): string | null => [...entries.keys()][index] ?? null,
    removeItem: (key: string): void => void entries.delete(key),
    setItem: (key: string, value: string): void => void entries.set(key, String(value)),
  };
}

describe('PanelSession [Unit]', () => {
  beforeEach(() => {
    navigation.goto.mockReset();
    routePage.params = {};
    routePage.route = { id: '/root' };
    routePage.url = at('/root');
    vi.stubGlobal('matchMedia', () => new TestMediaQueryList());
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    // A fresh one per test rather than one cleared between them, so nothing can
    // be carried over by a key this file does not know to remove. Both, because
    // the page a reader left is the tab's rather than the browser's.
    vi.stubGlobal('localStorage', memoryStorage());
    vi.stubGlobal('sessionStorage', memoryStorage());
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('leaves an unauthorized Root route even when there is no installation to return to', () => {
    const session = createSession();

    session.returnToPanel();

    expect(navigation.goto).toHaveBeenCalledWith(`${basePath}/`, { replace: true });
    expect(session.returnHref()).toBe(`${basePath}/`);
  });

  it('replaces an unauthorized Root route when an installation exists', () => {
    const session = createSession();
    session.targets = [
      {
        id: 'target-1',
        account: { login: 'acme' },
      } as PanelTarget,
    ];

    session.returnToPanel(true);

    expect(navigation.goto).toHaveBeenCalledWith(`${basePath}/i/acme/defaults`, { replace: true });
  });

  it('records nothing from a page that failed to load', () => {
    const session = createSession();
    session.targets = [{ id: 'target-1', account: { login: 'acme' } } as PanelTarget];
    session.selectedId = 'target-1';
    routePage.url = at('/i/acme/history/failures');
    routePage.params = { account: 'acme', section: 'failures' };
    routePage.route = { id: '/i/[account]/history/[[section=historySection]]' };
    session.syncRouteContext();

    // A pasted link naming a repository that cannot load. The address names the
    // repositories view and the chrome shows it, but the reader was never on it, so
    // Return has to take them back to where they actually were.
    routePage.url = at('/i/acme/repositories/bogus');
    routePage.params = { account: 'acme', repository: 'bogus' };
    routePage.route = {
      id: '/i/[account]/repositories/[repository]/[[section=repositorySection]]',
    };
    routePage.error = { message: 'Panel view not found' };
    session.syncRouteContext();

    expect(session.currentView, 'the chrome should still name the address').toBe('repositories');
    expect(session.returnHref()).toBe(`${basePath}/i/acme/history/failures`);

    routePage.error = null;
  });

  it('returns from Root to the workspace view it left', () => {
    const session = createSession();
    session.viewer = { system_role: 'root' } as PanelViewer;
    session.targets = [{ id: 'target-1', account: { login: 'acme' } } as PanelTarget];
    session.selectedId = 'target-1';
    routePage.url = at('/i/acme/repositories');
    routePage.params = { account: 'acme', view: 'repositories' };
    routePage.route = { id: '/i/[account]/[view=panelView]' };
    session.syncRouteContext();

    session.enterRoot();
    expect(navigation.goto).toHaveBeenLastCalledWith(`${basePath}/root`, { replace: false });

    // Visiting another installation view inside Root does not replace the
    // workspace context the Return action promises to restore.
    routePage.url = at('/root/installations/acme/defaults');
    routePage.params = { account: 'acme', view: 'defaults' };
    routePage.route = { id: '/root/installations/[account]/[view=rootInstallationView]' };
    session.syncRouteContext();
    session.returnToPanel();

    expect(navigation.goto).toHaveBeenLastCalledWith(`${basePath}/i/acme/repositories`, {
      replace: false,
    });
    expect(session.returnHref()).toBe(`${basePath}/i/acme/repositories`);
  });

  it('returns from Root to the repository page it left, not to the list', () => {
    const session = createSession();
    session.viewer = { system_role: 'root' } as PanelViewer;
    session.targets = [{ id: 'target-1', account: { login: 'acme' } } as PanelTarget];
    session.selectedId = 'target-1';
    openRepository(session);

    session.enterRoot();
    routePage.url = at('/root/installations/acme/defaults');
    routePage.params = { account: 'acme', view: 'defaults' };
    routePage.route = { id: '/root/installations/[account]/[view=rootInstallationView]' };
    session.syncRouteContext();

    expect(session.returnHref()).toBe(`${basePath}/i/acme/repositories/api-gateway/commands`);
  });

  it('lets repository detail pages scroll with the document', () => {
    const session = createSession();
    session.targets = [{ id: 'target-1', account: { login: 'acme' } } as PanelTarget];
    session.selectedId = 'target-1';

    openRepository(session);
    expect(session.tableScrollView).toBe(false);

    routePage.url = at('/i/acme/repositories');
    routePage.params = { account: 'acme', view: 'repositories' };
    routePage.route = { id: '/i/[account]/[view=panelView]' };
    session.syncRouteContext();
    expect(session.tableScrollView).toBe(true);
  });

  it('lets Root repository detail pages scroll with the document', () => {
    const session = createSession();
    session.viewer = { system_role: 'root' } as PanelViewer;
    routePage.url = at('/root/installations/acme/repositories/api-gateway/commands');
    routePage.params = { account: 'acme', repository: 'api-gateway', section: 'commands' };
    routePage.route = {
      id: '/root/installations/[account]/repositories/[repository]/[[section=repositorySection]]',
    };

    expect(session.tableScrollView).toBe(false);
  });

  it('still knows the page it left when the console is reloaded', () => {
    openRepository(createSession());

    // A reload builds the session again from nothing but the tab's storage.
    const reloaded = createSession();
    reloaded.targets = [{ id: 'target-1', account: { login: 'acme' } } as PanelTarget];
    routePage.url = at('/root');
    routePage.params = {};
    routePage.route = { id: '/root' };

    expect(reloaded.returnHref()).toBe(`${basePath}/i/acme/repositories/api-gateway/commands`);
  });

  it('ignores a remembered page whose installation is not this reader’s', () => {
    openRepository(createSession());

    const reloaded = createSession();
    reloaded.targets = [{ id: 'target-2', account: { login: 'other-org' } } as PanelTarget];
    routePage.url = at('/root');
    routePage.params = {};
    routePage.route = { id: '/root' };

    expect(reloaded.returnHref()).toBe(`${basePath}/i/other-org/defaults`);
  });

  it('opens the console on the page it was last left on', () => {
    const session = createSession();
    session.viewer = { system_role: 'root' } as PanelViewer;
    session.targets = [{ id: 'target-1', account: { login: 'acme' } } as PanelTarget];
    session.selectedId = 'target-1';

    // Nothing is remembered on the first crossing, so the console opens at its front.
    session.enterRoot();
    expect(navigation.goto).toHaveBeenLastCalledWith(`${basePath}/root`, { replace: false });

    openConsoleQueue(session);
    openRepository(session);

    expect(session.rootEntryHref()).toBe(`${basePath}/root/queue/recent`);
    session.enterRoot();
    expect(navigation.goto).toHaveBeenLastCalledWith(`${basePath}/root/queue/recent`, {
      replace: false,
    });
  });

  it('navigates between the three Runtime leaves', () => {
    const session = createSession();
    session.viewer = { system_role: 'root' } as PanelViewer;
    routePage.url = at('/root/runtime/database');
    routePage.params = {};
    routePage.route = { id: '/root/runtime/database' };

    expect(session.rootValue).toBe('runtime');
    expect(session.rootRuntimeHref('service')).toBe(`${basePath}/root/runtime/service`);
    expect(session.rootRuntimeHref('database')).toBe(`${basePath}/root/runtime/database`);
    expect(session.rootRuntimeHref('settings')).toBe(`${basePath}/root/runtime/settings`);

    session.selectRootRuntimeSection('service');
    expect(navigation.goto).toHaveBeenLastCalledWith(`${basePath}/root/runtime/service`, {
      replace: false,
    });
  });

  it('keeps each side of the crossing pointing at its own page', () => {
    const session = createSession();
    session.viewer = { system_role: 'root' } as PanelViewer;
    session.targets = [{ id: 'target-1', account: { login: 'acme' } } as PanelTarget];
    session.selectedId = 'target-1';
    openRepository(session);
    openConsoleQueue(session);

    // A reload of either side reads both back, and neither has overwritten the other.
    const reloaded = createSession();
    reloaded.viewer = { system_role: 'root' } as PanelViewer;
    reloaded.targets = [{ id: 'target-1', account: { login: 'acme' } } as PanelTarget];

    expect(reloaded.rootEntryHref()).toBe(`${basePath}/root/queue/recent`);
    expect(reloaded.returnHref()).toBe(`${basePath}/i/acme/repositories/api-gateway/commands`);
  });

  it('retains and can reopen the workspace view while the inbox is open', async () => {
    const session = createSession();
    session.targets = [{ id: 'target-1', account: { login: 'acme' } } as PanelTarget];
    session.selectedId = 'target-1';
    routePage.url = at('/i/acme/history/failures');
    routePage.params = { account: 'acme', section: 'failures' };
    routePage.route = { id: '/i/[account]/history/[[section=historySection]]' };
    session.syncRouteContext();
    routePage.url = at('/inbox');
    routePage.params = {};
    routePage.route = { id: '/inbox' };

    expect(session.targetHref(session.targets[0]!)).toBe(`${basePath}/i/acme/history/failures`);
    expect(session.returnHref()).toBe(`${basePath}/i/acme/history/failures`);

    session.selectView('history');
    expect(navigation.goto).toHaveBeenLastCalledWith(`${basePath}/i/acme/history/failures`, {
      replace: false,
    });

    await session.selectTarget('target-1');
    expect(navigation.goto).toHaveBeenLastCalledWith(`${basePath}/i/acme/history/failures`, {
      replace: false,
    });
  });

  it('refreshes every repository-count aggregate after a remote repository change', () => {
    const queryClient = new QueryClient();
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries').mockResolvedValue();
    const session = createSession(queryClient);
    const event: PanelChangeEvent = {
      version: 1,
      type: 'repository.changed',
      target_id: 'target-1',
      repository_id: 'repository-1',
    };

    session.invalidateChange(event);

    const keys = invalidate.mock.calls.map(([filters]) => filters?.queryKey);
    expect(keys).toEqual(
      expect.arrayContaining([
        ['repositories', 'target-1'],
        ['repository', 'target-1'],
        ['targets'],
        ['root-installations'],
        ['root-overview'],
      ]),
    );
  });

  /**
   * Keys match by prefix, and what a repository says about a kind of sync is
   * under one of its own, so none of the keys above reaches it. Left out, a
   * colleague's save leaves this browser rendering the document it already had
   * and sending the revision that came with it, so every Save it tries is
   * answered 409 "settings changed in another session" until the page is
   * reloaded.
   */
  it('refreshes what a repository adjusts after a remote repository change', () => {
    const queryClient = new QueryClient();
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries').mockResolvedValue();
    const session = createSession(queryClient);

    session.invalidateChange({
      version: 1,
      type: 'repository.changed',
      target_id: 'target-1',
      repository_id: 'repository-1',
    });

    const keys = invalidate.mock.calls.map(([filters]) => filters?.queryKey);
    expect(keys).toEqual(expect.arrayContaining([['sync-override', 'target-1']]));
  });

  it('publishes queue revisions for direct queue and schedule views', () => {
    const session = createSession();

    session.invalidateChange({ version: 1, type: 'queue.changed', target_id: 'target-1' });
    session.invalidateChange({ version: 1, type: 'queue.changed' });

    expect(session.queueRevision).toBe(2);
  });
});

/** Puts a session on one repository's Commands pane, which is the page Return has to name. */
function openRepository(session: PanelSession): PanelSession {
  routePage.url = at('/i/acme/repositories/api-gateway/commands');
  routePage.params = { account: 'acme', repository: 'api-gateway', section: 'commands' };
  routePage.route = { id: '/i/[account]/repositories/[repository]/[[section=repositorySection]]' };
  session.syncRouteContext();

  return session;
}

/** And on a console page that is not its front one, which is what Root console has to name. */
function openConsoleQueue(session: PanelSession): PanelSession {
  routePage.url = at('/root/queue/recent');
  routePage.params = {};
  routePage.route = { id: '/root/queue/recent' };
  session.syncRouteContext();

  return session;
}

function createSession(queryClient = new QueryClient()): PanelSession {
  const session = new PanelSession({} as PanelApi, {} as PanelBuild, queryClient);
  session.viewer = { system_role: 'none' } as PanelViewer;
  session.loading = false;
  return session;
}
