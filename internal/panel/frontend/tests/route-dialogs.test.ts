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
  it('separates the add dialog from a person by how many segments follow', () => {
    expect(parsePanelRoute('', '/workspace/acme/access/users/add')).toEqual({
      account: 'acme',
      view: 'users',
      dialog: { name: 'add-user', params: {} },
    });
    /* Somebody whose login is `add` is still reachable: every dialog about a
       person carries a verb, so it is two segments and never one. */
    expect(parsePanelRoute('', '/workspace/acme/access/users/add/history')).toEqual({
      account: 'acme',
      view: 'users',
      dialog: { name: 'decision-history', params: { user: 'add' } },
    });
  });

  it('reads the confirmations about a person', () => {
    expect(parsePanelRoute('', '/workspace/acme/access/users/octocat/suspend')).toEqual({
      account: 'acme',
      view: 'users',
      dialog: { name: 'user-action', params: { user: 'octocat', action: 'suspend' } },
    });
    /* The panel's own word is `remove_access`; an address says it the way every
       other segment is written. */
    expect(parsePanelRoute('', '/workspace/acme/access/users/octocat/remove-access')).toEqual({
      account: 'acme',
      view: 'users',
      dialog: { name: 'user-action', params: { user: 'octocat', action: 'remove_access' } },
    });
    expect(roundTrip('/workspace/acme/access/users/octocat/suspend')).toBe(
      '/workspace/acme/access/users/octocat/suspend',
    );
    expect(roundTrip('/workspace/acme/access/users/octocat/remove-access')).toBe(
      '/workspace/acme/access/users/octocat/remove-access',
    );
  });

  it('refuses a verb no dialog answers to', () => {
    expect(parsePanelRoute('', '/workspace/acme/access/users/octocat/befriend')).toBeNull();
    expect(parsePanelRoute('', '/workspace/acme/access/users/octocat/suspend/now')).toBeNull();
  });

  it('keeps history a section rather than a dialog', () => {
    expect(parsePanelRoute('', '/workspace/acme/history/audit')).toEqual({
      account: 'acme',
      view: 'history',
      section: 'audit',
    });
    expect(parsePanelRoute('', '/workspace/acme/history/octocat')).toBeNull();
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

  it('escapes a login that needs it', () => {
    const path = panelAddress({
      account: 'acme',
      view: 'users',
      dialog: { name: 'user-action', params: { user: 'a b/c', action: 'suspend' } },
    });
    expect(path).toBe(`${basePath}/workspace/acme/access/users/a%20b%2Fc/suspend`);
    expect(parsePanelRoute(basePath, path)).toEqual({
      account: 'acme',
      view: 'users',
      dialog: { name: 'user-action', params: { user: 'a b/c', action: 'suspend' } },
    });
  });
});

describe('dialog addresses in the Root console [Unit]', () => {
  it('gives the Root tables the same grammar as a workspace', () => {
    expect(parsePanelRoute('', '/root/access/users/add')).toEqual({
      rootView: 'access-users',
      dialog: { name: 'root-add-workspace-user', params: {} },
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

  it('carries a dialog on a workspace seen through the Root console', () => {
    expect(parsePanelRoute('', '/root/workspaces/acme/access/users/octocat/suspend')).toEqual({
      rootView: 'workspace',
      account: 'acme',
      view: 'users',
      dialog: { name: 'user-action', params: { user: 'octocat', action: 'suspend' } },
    });
    expect(roundTrip('/root/workspaces/acme/access/users/octocat/history')).toBe(
      '/root/workspaces/acme/access/users/octocat/history',
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
