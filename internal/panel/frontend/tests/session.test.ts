// @vitest-environment jsdom
import { QueryClient } from '@tanstack/svelte-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const navigation = vi.hoisted(() => ({ goto: vi.fn() }));
const routePage = vi.hoisted(() => ({
  params: {} as Record<string, string>,
  url: new URL('https://panel.example/root'),
}));

vi.mock('$app/navigation', () => navigation);
vi.mock('$app/paths', () => ({ base: '', resolve: (path: string) => path }));
vi.mock('$app/state', () => ({ page: routePage }));

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

describe('PanelSession [Unit]', () => {
  beforeEach(() => {
    navigation.goto.mockReset();
    routePage.params = {};
    routePage.url = new URL('https://panel.example/root');
    vi.stubGlobal('matchMedia', () => new TestMediaQueryList());
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('leaves an unauthorized Root route even when there is no installation to return to', () => {
    const session = createSession();

    session.returnToPanel();

    expect(navigation.goto).toHaveBeenCalledWith('/', { replaceState: true });
    expect(session.returnHref()).toBe('/');
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

    expect(navigation.goto).toHaveBeenCalledWith('/i/acme/settings', { replaceState: true });
  });

  it('returns from Root to the workspace view it left', () => {
    const session = createSession();
    session.viewer = { system_role: 'root' } as PanelViewer;
    session.targets = [{ id: 'target-1', account: { login: 'acme' } } as PanelTarget];
    session.selectedId = 'target-1';
    routePage.url = new URL('https://panel.example/i/acme/repositories');
    routePage.params = { account: 'acme', view: 'repositories' };
    session.syncRouteContext(routePage.params.view, routePage.params.rest);

    session.enterRoot();
    expect(navigation.goto).toHaveBeenLastCalledWith('/root', { replaceState: false });

    // Visiting another installation view inside Root does not replace the
    // workspace context the Return action promises to restore.
    routePage.url = new URL('https://panel.example/root/installations/acme/settings');
    routePage.params = { account: 'acme', view: 'settings' };
    session.syncRouteContext(routePage.params.view, routePage.params.rest);
    session.returnToPanel();

    expect(navigation.goto).toHaveBeenLastCalledWith('/i/acme/repositories', {
      replaceState: false,
    });
    expect(session.returnHref()).toBe('/i/acme/repositories');
  });

  it('retains and can reopen the workspace view while the inbox is open', async () => {
    const session = createSession();
    session.targets = [{ id: 'target-1', account: { login: 'acme' } } as PanelTarget];
    session.selectedId = 'target-1';
    routePage.url = new URL('https://panel.example/i/acme/history/failures');
    routePage.params = { account: 'acme', view: 'history', rest: 'failures' };
    session.syncRouteContext('history', 'failures');
    routePage.url = new URL('https://panel.example/inbox');
    routePage.params = {};

    expect(session.targetHref(session.targets[0]!)).toBe('/i/acme/history/failures');
    expect(session.returnHref()).toBe('/i/acme/history/failures');

    session.selectView('history');
    expect(navigation.goto).toHaveBeenLastCalledWith('/i/acme/history/failures', {
      replaceState: false,
    });

    await session.selectTarget('target-1');
    expect(navigation.goto).toHaveBeenLastCalledWith('/i/acme/history/failures', {
      replaceState: false,
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
