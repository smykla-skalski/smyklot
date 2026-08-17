import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { ROOT_USER_ACTIONS } from '../src/lib/route-dialogs';
import { panelAddress } from '../src/lib/addresses';
import { basePath } from '../src/lib/paths';
import { parsePanelRoute } from '../src/lib/routes';

/**
 * The addresses dialogs are read from and written to.
 *
 * Written as round trips through the real route parser rather than against the
 * grammar helpers, because what matters is that a link somebody pastes resolves
 * to the same thing the panel would have written for it.
 */
function roundTrip(path: string): string {
  const route = parsePanelRoute('', path);
  expect(route).not.toBeNull();

  return panelAddress(route!).slice(basePath.length);
}

describe('dialog addresses on a view [Unit]', () => {
  it('reads a repository dialog as part of the repositories path', () => {
    expect(parsePanelRoute('', '/i/acme/repositories/api-gateway')).toEqual({
      account: 'acme',
      view: 'repositories',
      dialog: {
        name: 'repository-settings',
        params: { repository: 'api-gateway', section: 'file' },
      },
    });
    expect(parsePanelRoute('', '/i/acme/repositories/api-gateway/commands')).toEqual({
      account: 'acme',
      view: 'repositories',
      dialog: {
        name: 'repository-settings',
        params: { repository: 'api-gateway', section: 'commands' },
      },
    });
  });

  it('writes the pane only when it is not the one the dialog opens on', () => {
    expect(roundTrip('/i/acme/repositories/api-gateway')).toBe('/i/acme/repositories/api-gateway');
    expect(roundTrip('/i/acme/repositories/api-gateway/file')).toBe(
      '/i/acme/repositories/api-gateway',
    );
    expect(roundTrip('/i/acme/repositories/api-gateway/behavior')).toBe(
      '/i/acme/repositories/api-gateway/behavior',
    );
  });

  it('reads a repository named like a pane', () => {
    /* A name is only ever read in the first position, so a repository called
       `file` is not mistaken for the File pane of nothing. */
    expect(parsePanelRoute('', '/i/acme/repositories/file/file')).toEqual({
      account: 'acme',
      view: 'repositories',
      dialog: { name: 'repository-settings', params: { repository: 'file', section: 'file' } },
    });
    expect(parsePanelRoute('', '/i/acme/repositories/file')).toEqual({
      account: 'acme',
      view: 'repositories',
      dialog: { name: 'repository-settings', params: { repository: 'file', section: 'file' } },
    });
  });

  it('refuses a pane that is not one', () => {
    // An address that does not resolve, rather than the bare list with nothing
    // open - a mistyped pane should say so.
    expect(parsePanelRoute('', '/i/acme/repositories/api-gateway/nonsense')).toBeNull();
    expect(parsePanelRoute('', '/i/acme/repositories/api-gateway/file/extra')).toBeNull();
  });

  it('separates the add dialog from a person by how many segments follow', () => {
    expect(parsePanelRoute('', '/i/acme/users/add')).toEqual({
      account: 'acme',
      view: 'users',
      dialog: { name: 'add-user', params: {} },
    });
    /* Somebody whose login is `add` is still reachable: every dialog about a
       person carries a verb, so it is two segments and never one. */
    expect(parsePanelRoute('', '/i/acme/users/add/history')).toEqual({
      account: 'acme',
      view: 'users',
      dialog: { name: 'decision-history', params: { user: 'add' } },
    });
  });

  it('reads the confirmations about a person', () => {
    expect(parsePanelRoute('', '/i/acme/users/octocat/suspend')).toEqual({
      account: 'acme',
      view: 'users',
      dialog: { name: 'user-action', params: { user: 'octocat', action: 'suspend' } },
    });
    /* The panel's own word is `remove_access`; an address says it the way every
       other segment is written. */
    expect(parsePanelRoute('', '/i/acme/users/octocat/remove-access')).toEqual({
      account: 'acme',
      view: 'users',
      dialog: { name: 'user-action', params: { user: 'octocat', action: 'remove_access' } },
    });
    expect(roundTrip('/i/acme/users/octocat/suspend')).toBe('/i/acme/users/octocat/suspend');
    expect(roundTrip('/i/acme/users/octocat/remove-access')).toBe(
      '/i/acme/users/octocat/remove-access',
    );
  });

  it('refuses a verb no dialog answers to', () => {
    expect(parsePanelRoute('', '/i/acme/users/octocat/befriend')).toBeNull();
    expect(parsePanelRoute('', '/i/acme/users/octocat/suspend/now')).toBeNull();
  });

  it('keeps history a section rather than a dialog', () => {
    expect(parsePanelRoute('', '/i/acme/history/audit')).toEqual({
      account: 'acme',
      view: 'history',
      section: 'audit',
    });
    expect(parsePanelRoute('', '/i/acme/history/octocat')).toBeNull();
  });

  // Both halves of this module read a raw pathname, so both decode. One of them stopped
  // when the decode moved out of `parseDialogSegments`, and the Root console's own tables
  // were the half that stopped.
  it('decodes a name in the Root console the same way', () => {
    expect(parsePanelRoute('', '/root/access/users/oct%40cat/ban')).toEqual({
      rootView: 'access-users',
      dialog: { name: 'root-user-action', params: { user: 'oct@cat', action: 'ban' } },
    });
  });

  it('escapes a repository name that needs it', () => {
    const path = panelAddress({
      account: 'acme',
      view: 'repositories',
      dialog: { name: 'repository-settings', params: { repository: 'a b/c', section: 'file' } },
    });
    expect(path).toBe(`${basePath}/i/acme/repositories/a%20b%2Fc`);
    expect(parsePanelRoute(basePath, path)).toEqual({
      account: 'acme',
      view: 'repositories',
      dialog: { name: 'repository-settings', params: { repository: 'a b/c', section: 'file' } },
    });
  });
});

describe('dialog addresses in the Root console [Unit]', () => {
  it('gives the Root tables the same grammar as an installation', () => {
    expect(parsePanelRoute('', '/root/access/users/add')).toEqual({
      rootView: 'access-users',
      dialog: { name: 'root-add-installation-user', params: {} },
    });
    expect(parsePanelRoute('', '/root/access/users/octocat/ban')).toEqual({
      rootView: 'access-users',
      dialog: { name: 'root-user-action', params: { user: 'octocat', action: 'ban' } },
    });
    expect(parsePanelRoute('', '/root/access/users/octocat/promote-root')).toEqual({
      rootView: 'access-users',
      dialog: { name: 'root-user-action', params: { user: 'octocat', action: 'promote_root' } },
    });
    expect(roundTrip('/root/access/users/octocat/promote-root')).toBe(
      '/root/access/users/octocat/promote-root',
    );
  });

  it('reads the Root invitation dialogs', () => {
    expect(parsePanelRoute('', '/root/access/invitations/new')).toEqual({
      rootView: 'access-invitations',
      dialog: { name: 'root-invitation-create', params: {} },
    });
    expect(parsePanelRoute('', '/root/access/invitations/inv-1/revoke')).toEqual({
      rootView: 'access-invitations',
      dialog: {
        name: 'root-invitation-action',
        params: { invitation: 'inv-1', action: 'revoke' },
      },
    });
  });

  it('carries a dialog on an installation seen through the Root console', () => {
    expect(
      parsePanelRoute('', '/root/installations/acme/repositories/api-gateway/behavior'),
    ).toEqual({
      rootView: 'installation',
      account: 'acme',
      view: 'repositories',
      dialog: {
        name: 'repository-settings',
        params: { repository: 'api-gateway', section: 'behavior' },
      },
    });
    expect(roundTrip('/root/installations/acme/users/octocat/history')).toBe(
      '/root/installations/acme/users/octocat/history',
    );
  });

  it('leaves the bare tables alone', () => {
    expect(parsePanelRoute('', '/root/access/users')).toEqual({ rootView: 'access-users' });
    expect(parsePanelRoute('', '/root/access/invitations')).toEqual({
      rootView: 'access-invitations',
    });
    expect(parsePanelRoute('', '/root/access/users/octocat')).toBeNull();
  });
});

/**
 * The two lists that have to agree, tied together.
 *
 * `RootAccess` keeps its own list of the actions it offers, in the underscore
 * spelling its handlers switch on; the grammar keeps the same five in the
 * hyphenated spelling an address uses. Nothing connected them, so an action
 * added to the menu and not to the grammar would write an address that does not
 * resolve, and nobody would find out until somebody reloaded one. That is the
 * shape that already cost this panel every dialog link it wrote.
 */
describe('the Root user actions the console offers [Unit]', () => {
  const component = readFileSync(
    new URL('../src/lib/components/RootAccess.svelte', import.meta.url),
    'utf8',
  );

  it('are exactly the actions an address can name', () => {
    const declared = /const USER_ACTIONS = \[([^\]]*)\]/u.exec(component)?.[1];
    expect(declared).toBeDefined();
    const offered = [...(declared ?? '').matchAll(/'([a-z_]+)'/gu)].map((match) =>
      match[1]?.replaceAll('_', '-'),
    );

    expect(offered).toEqual([...ROOT_USER_ACTIONS]);
  });
});
