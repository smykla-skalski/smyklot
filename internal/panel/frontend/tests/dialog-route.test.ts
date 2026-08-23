import { beforeEach, describe, expect, it, vi } from 'vitest';

const navigation = vi.hoisted(() => ({ goto: vi.fn() }));

// Seeded with a placeholder: `vi.hoisted` runs before the imports `at()` needs, and
// every spec sets the address it means in `beforeEach`.
const routePage = vi.hoisted(() => ({
  url: new URL('https://panel.example/'),
  params: { account: 'acme', view: 'repositories' } as Record<string, string>,
  // The route the address matched. Base-free by definition, which is why the adapter
  // asks it rather than the pathname whether this is a Root installation.
  route: { id: null } as { id: string | null },
  state: {} as Record<string, unknown>,
}));

vi.mock('$app/navigation', () => navigation);
vi.mock('$app/state', () => ({ page: routePage }));

import { basePath } from '../src/lib/paths.ts';
import { at } from './support/addresses.ts';
import { dialogRoute, legacyInboxRoute, parseDialog } from '../src/lib/dialog-route.svelte';

describe('SvelteKit dialog route adapter', () => {
  beforeEach(() => {
    navigation.goto.mockReset();
    navigation.goto.mockImplementation((_url, options?: { state?: Record<string, unknown> }) => {
      if (options?.state !== undefined) routePage.state = options.state;
    });
    routePage.url = at('/i/acme/access/users');
    routePage.params = { account: 'acme', section: 'users' };
    routePage.route = {
      id: '/i/[account]/access/[section=accessSection]/[...rest=dialogPath]',
    };
    routePage.state = {};
  });

  it('opens a shareable path as an owned shallow entry', () => {
    dialogRoute.open('user-action', { user: 'octocat', action: 'suspend' });

    expect(navigation.goto).toHaveBeenCalledWith(
      `${basePath}/i/acme/access/users/octocat/suspend`,
      {
        shallow: true,
        replace: false,
        state: expect.objectContaining({
          dialog: {
            name: 'user-action',
            params: { user: 'octocat', action: 'suspend' },
          },
          smyklotDialogEntry: true,
        }),
      },
    );
  });

  it('closes an owned entry with browser history so Forward can reopen it', () => {
    const back = vi.fn();
    vi.stubGlobal('history', { back });
    routePage.state = {
      dialog: { name: 'user-action', params: { user: 'octocat', action: 'suspend' } },
      smyklotDialogEntry: true,
    };

    dialogRoute.close();

    expect(back).toHaveBeenCalledOnce();
    expect(navigation.goto).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });

  it('replaces an open reissue confirmation with its generated-link dialog', () => {
    routePage.route = { id: '/root/access/[section=accessSection]/[...rest=dialogPath]' };
    routePage.url = at('/root/access/invitations/invitation-1/reissue');
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

    expect(navigation.goto).toHaveBeenCalledWith(`${basePath}/root/access/invitations/new`, {
      shallow: true,
      replace: true,
      state: expect.objectContaining({
        dialog: { name: 'root-invitation-create', params: {} },
        smyklotDialogEntry: true,
      }),
    });
  });

  /**
   * Shallow, and the assertion that it is not a navigation is the point of the
   * test rather than a detail of it. A `goto` here leaves the route that hosts a
   * dialog for the route that does not, and those are two page components: the
   * view underneath was torn down and built again on every close, so a search
   * half typed into the table went back to the last stored one and the reader's
   * place in the list went with it.
   */
  it('closes a cold deep link without leaving the route it is on', () => {
    routePage.url = at('/i/acme/access/users/octocat/suspend');
    routePage.params = {
      account: 'acme',
      section: 'users',
      rest: 'octocat/suspend',
    };

    dialogRoute.close();

    expect(navigation.goto).toHaveBeenCalledWith(`${basePath}/i/acme/access/users`, {
      shallow: true,
      replace: true,
      state: expect.objectContaining({ smyklotDialogClosed: true }),
    });
    // The route's own parameters still name the dialog, because nothing
    // re-resolved them. What says the dialog is shut is the state above.
    expect(dialogRoute.current).toBeNull();
  });

  it('keeps a pathless dialog in the query across reloads', () => {
    routePage.url = at('/root/installations/acme/settings');
    routePage.params = { account: 'acme', view: 'settings' };
    routePage.route = { id: '/root/installations/[account]/[view=rootInstallationView]' };

    dialogRoute.open('root-elevation', { reason: 'change settings' });

    expect(navigation.goto).toHaveBeenCalledWith(
      `${basePath}/root/installations/acme/settings?dialog=root-elevation&reason=change+settings`,
      {
        shallow: true,
        replace: false,
        state: expect.objectContaining({
          dialog: { name: 'root-elevation', params: { reason: 'change settings' } },
          smyklotDialogEntry: true,
        }),
      },
    );

    routePage.state = {};
    routePage.url = at(
      '/root/installations/acme/settings?dialog=root-elevation&reason=change+settings',
    );
    expect(dialogRoute.current).toEqual({
      name: 'root-elevation',
      params: { reason: 'change settings' },
    });
  });

  it('removes a cold query dialog from the address when it closes', () => {
    routePage.url = at('/root/installations/acme/settings?dialog=root-elevation');
    routePage.params = { account: 'acme', view: 'settings' };

    dialogRoute.close();

    expect(navigation.goto).toHaveBeenCalledWith(`${basePath}/root/installations/acme/settings`, {
      shallow: true,
      replace: true,
      state: expect.objectContaining({ smyklotDialogClosed: true }),
    });
    expect(dialogRoute.current).toBeNull();

    dialogRoute.open('root-elevation');
    expect(navigation.goto).toHaveBeenLastCalledWith(
      expect.any(String),
      expect.objectContaining({ shallow: true }),
    );
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
