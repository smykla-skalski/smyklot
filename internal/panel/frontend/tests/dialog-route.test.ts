import { beforeEach, describe, expect, it, vi } from 'vitest';

const navigation = vi.hoisted(() => ({
  goto: vi.fn(),
  pushState: vi.fn(),
  replaceState: vi.fn(),
}));

const routePage = vi.hoisted(() => ({
  url: new URL('https://panel.example/i/acme/repositories'),
  params: { account: 'acme', view: 'repositories' } as Record<string, string>,
  state: {} as Record<string, unknown>,
}));

vi.mock('$app/navigation', () => navigation);
vi.mock('$app/paths', () => ({ base: '', resolve: (path: string) => path }));
vi.mock('$app/state', () => ({ page: routePage }));

import { dialogRoute } from '../src/lib/dialog-route.svelte';

describe('SvelteKit dialog route adapter', () => {
  beforeEach(() => {
    navigation.goto.mockReset();
    navigation.pushState.mockReset();
    navigation.replaceState.mockReset();
    routePage.url = new URL('https://panel.example/i/acme/repositories');
    routePage.params = { account: 'acme', view: 'repositories' };
    routePage.state = {};
  });

  it('opens a shareable path as an owned shallow entry', () => {
    dialogRoute.open('repository-settings', { repository: 'api-gateway' });

    expect(navigation.pushState).toHaveBeenCalledWith(
      '/i/acme/repositories/api-gateway',
      expect.objectContaining({
        dialog: {
          name: 'repository-settings',
          params: { repository: 'api-gateway' },
        },
        smyklotDialogEntry: true,
      }),
    );
    expect(navigation.goto).not.toHaveBeenCalled();
  });

  it('closes an owned entry with browser history so Forward can reopen it', () => {
    const back = vi.fn();
    vi.stubGlobal('history', { back });
    routePage.state = {
      dialog: { name: 'repository-settings', params: { repository: 'api-gateway' } },
      smyklotDialogEntry: true,
    };

    dialogRoute.close();

    expect(back).toHaveBeenCalledOnce();
    expect(navigation.goto).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });

  it('closes a cold deep link by replacing it with the host route', () => {
    routePage.url = new URL('https://panel.example/i/acme/repositories/api-gateway/file');
    routePage.params = {
      account: 'acme',
      view: 'repositories',
      rest: 'api-gateway/file',
    };

    dialogRoute.close();

    expect(navigation.goto).toHaveBeenCalledWith('/i/acme/repositories', {
      replaceState: true,
      state: {},
    });
  });
});
