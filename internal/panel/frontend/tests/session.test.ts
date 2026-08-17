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
 * A `Storage` for the session to keep preferences in.
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
    // be carried over by a key this file does not know to remove.
    vi.stubGlobal('localStorage', memoryStorage());
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

    expect(navigation.goto).toHaveBeenCalledWith(`${basePath}/i/acme/settings`, { replace: true });
  });

  it('records nothing from a page that failed to load', () => {
    const session = createSession();
    session.targets = [{ id: 'target-1', account: { login: 'acme' } } as PanelTarget];
    session.selectedId = 'target-1';
    routePage.url = at('/i/acme/history/failures');
    routePage.params = { account: 'acme', section: 'failures' };
    routePage.route = { id: '/i/[account]/history/[[section=historySection]]' };
    session.syncRouteContext();

    // A pasted link naming a dialog that does not exist. The address names the
    // repositories view and the chrome shows it, but the reader was never on it, so
    // Return has to take them back to where they actually were.
    routePage.url = at('/i/acme/repositories/bogus/bogus2');
    routePage.params = { account: 'acme', view: 'repositories', rest: 'bogus/bogus2' };
    routePage.route = { id: '/i/[account]/[view=dialogHostView]/[...rest=dialogPath]' };
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
    routePage.url = at('/root/installations/acme/settings');
    routePage.params = { account: 'acme', view: 'settings' };
    routePage.route = { id: '/root/installations/[account]/[view=rootInstallationView]' };
    session.syncRouteContext();
    session.returnToPanel();

    expect(navigation.goto).toHaveBeenLastCalledWith(`${basePath}/i/acme/repositories`, {
      replace: false,
    });
    expect(session.returnHref()).toBe(`${basePath}/i/acme/repositories`);
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
});

function createSession(queryClient = new QueryClient()): PanelSession {
  const session = new PanelSession({} as PanelApi, {} as PanelBuild, queryClient);
  session.viewer = { system_role: 'none' } as PanelViewer;
  session.loading = false;
  return session;
}
