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

import { dialogRoute, legacyInboxRoute, parseDialog } from '../src/lib/dialog-route.svelte';

describe('SvelteKit dialog route adapter', () => {
  beforeEach(() => {
    navigation.goto.mockReset();
    navigation.pushState.mockReset();
    navigation.replaceState.mockReset();
    navigation.pushState.mockImplementation((_url, state) => {
      routePage.state = state;
    });
    navigation.replaceState.mockImplementation((_url, state) => {
      routePage.state = state;
    });
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

  it('replaces an open reissue confirmation with its generated-link dialog', () => {
    routePage.url = new URL('https://panel.example/root/access/invitations/invitation-1/reissue');
    routePage.params = {
      section: 'invitations',
      rest: 'invitation-1/reissue',
    };
    routePage.state = {
      dialog: {
        name: 'root-invitation-action',
        params: { invitation: 'invitation-1', action: 'reissue' },
      },
      smyklotDialogEntry: true,
    };

    dialogRoute.open('root-invitation-create');

    expect(navigation.pushState).not.toHaveBeenCalled();
    expect(navigation.replaceState).toHaveBeenCalledWith(
      '/root/access/invitations/new',
      expect.objectContaining({
        dialog: { name: 'root-invitation-create', params: {} },
        smyklotDialogEntry: true,
      }),
    );
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
      state: expect.objectContaining({ smyklotDialogClosed: true }),
    });
  });

  it('keeps a pathless dialog in the query across reloads', () => {
    routePage.url = new URL('https://panel.example/root/installations/acme/settings');
    routePage.params = { account: 'acme', view: 'settings' };

    dialogRoute.open('root-elevation', { reason: 'change settings' });

    expect(navigation.pushState).toHaveBeenCalledWith(
      '/root/installations/acme/settings?dialog=root-elevation&reason=change+settings',
      expect.objectContaining({
        dialog: { name: 'root-elevation', params: { reason: 'change settings' } },
        smyklotDialogEntry: true,
      }),
    );

    routePage.state = {};
    routePage.url = new URL(
      'https://panel.example/root/installations/acme/settings?dialog=root-elevation&reason=change+settings',
    );
    expect(dialogRoute.current).toEqual({
      name: 'root-elevation',
      params: { reason: 'change settings' },
    });
  });

  it('removes a cold query dialog from the address when it closes', () => {
    routePage.url = new URL(
      'https://panel.example/root/installations/acme/settings?dialog=root-elevation',
    );
    routePage.params = { account: 'acme', view: 'settings' };

    dialogRoute.close();

    expect(navigation.replaceState).toHaveBeenCalledWith(
      '/root/installations/acme/settings',
      expect.objectContaining({ smyklotDialogClosed: true }),
    );
    expect(dialogRoute.current).toBeNull();

    dialogRoute.open('root-elevation');
    expect(navigation.pushState).toHaveBeenCalled();
    expect(dialogRoute.current).toEqual({ name: 'root-elevation', params: {} });
  });

  it('recognizes bookmarks from when the inbox was a dialog', () => {
    expect(legacyInboxRoute('?dialog=security-notifications')).toBe(true);
    expect(legacyInboxRoute('?dialog=root-elevation')).toBe(false);
    expect(parseDialog('?dialog=root-elevation&reason=incident')).toEqual({
      name: 'root-elevation',
      params: { reason: 'incident' },
    });
  });
});
