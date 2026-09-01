import { describe, expect, it } from 'vitest';

import { panelAddress } from '../src/lib/addresses';
import { basePath } from '../src/lib/paths';
import { parsePanelRoute } from '../src/lib/routes';

/**
 * The address of one repository's page.
 *
 * It is the address it had as a dialog, and that is the point: the page replaced
 * the dialog without breaking a single link somebody had already sent. What the
 * segments MEAN changed, so these read the route rather than the path - a
 * `repository` on the route is a page, a `dialog` on it is something standing
 * over the list, and the two are no longer the same address.
 *
 * The pane is gone with the tabs that named it. A repository's page is one scroll,
 * so the repository IS the whole address and anything after it resolves to nothing.
 *
 * Round trips through the real parser rather than through the grammar helpers,
 * because what matters is that a link somebody pastes resolves to what the panel
 * would have written for it.
 */
function roundTrip(path: string): string {
  const route = parsePanelRoute('', path);
  expect(route, `${path} did not resolve`).not.toBeNull();

  return panelAddress(route!).slice(basePath.length);
}

describe('one repository as a page [Unit]', () => {
  it('reads the repository out of the repositories path', () => {
    expect(parsePanelRoute('', '/workspace/acme/repositories/api-gateway')).toEqual({
      account: 'acme',
      view: 'repositories',
      repository: { name: 'api-gateway' },
    });
  });

  it('is a page, never a dialog', () => {
    // The whole point of the change: nothing about this address says something
    // is standing on top of the list, because nothing is.
    const route = parsePanelRoute('', '/workspace/acme/repositories/api-gateway');
    expect(route).not.toBeNull();
    expect(route).not.toHaveProperty('dialog');
  });

  it('writes the repository and nothing after it', () => {
    expect(roundTrip('/workspace/acme/repositories/api-gateway')).toBe(
      '/workspace/acme/repositories/api-gateway',
    );
  });

  it('reads a repository named like one of the panes that used to exist', () => {
    /* A name is only ever read in the first position, so a repository called
       `file` is a repository and not the remains of a pane. */
    expect(parsePanelRoute('', '/workspace/acme/repositories/file')).toEqual({
      account: 'acme',
      view: 'repositories',
      repository: { name: 'file' },
    });
  });

  it('leaves the bare list alone', () => {
    expect(parsePanelRoute('', '/workspace/acme/repositories')).toEqual({
      account: 'acme',
      view: 'repositories',
    });
  });

  /**
   * The retired pane addresses answer nothing.
   *
   * Five of them were real - `/file`, `/behavior`, `/commands`, `/formatting`, `/sync` -
   * and a link to one may still be in somebody's message. It resolves to no route, which
   * the wire answers 404, rather than the page quietly opening and pretending the link
   * meant what it says. The same treatment the retired Schedules page got.
   */
  it('refuses the panes it used to have', () => {
    for (const pane of ['file', 'behavior', 'commands', 'formatting', 'sync']) {
      expect(
        parsePanelRoute('', `/workspace/acme/repositories/api-gateway/${pane}`),
        `/${pane} still resolves`,
      ).toBeNull();
    }
    expect(parsePanelRoute('', '/workspace/acme/repositories/api-gateway/nonsense')).toBeNull();
    expect(parsePanelRoute('', '/workspace/acme/repositories/api-gateway/file/extra')).toBeNull();
  });

  it('escapes a repository name that needs it', () => {
    const path = panelAddress({
      account: 'acme',
      view: 'repositories',
      repository: { name: 'a b/c' },
    });
    expect(path).toBe(`${basePath}/workspace/acme/repositories/a%20b%2Fc`);
    expect(parsePanelRoute(basePath, path)).toEqual({
      account: 'acme',
      view: 'repositories',
      repository: { name: 'a b/c' },
    });
  });

  it('refuses a name that decodes to nothing', () => {
    expect(parsePanelRoute('', '/workspace/acme/repositories/%zz')).toBeNull();
    expect(parsePanelRoute('', '/workspace/acme/repositories/%20')).toBeNull();
  });
});

describe('one repository through the Root console [Unit]', () => {
  it('takes the same shape of address it has in a workspace', () => {
    expect(parsePanelRoute('', '/root/workspaces/acme/repositories/api-gateway')).toEqual({
      rootView: 'workspace',
      account: 'acme',
      view: 'repositories',
      repository: { name: 'api-gateway' },
    });
    expect(roundTrip('/root/workspaces/acme/repositories/api-gateway')).toBe(
      '/root/workspaces/acme/repositories/api-gateway',
    );
  });

  it('refuses the retired panes there too', () => {
    expect(parsePanelRoute('', '/root/workspaces/acme/repositories/api/behavior')).toBeNull();
    expect(parsePanelRoute('', '/root/workspaces/acme/repositories/api/nonsense')).toBeNull();
  });

  /* Sync is not a console view, and defaults hosts nothing after it. Neither
     takes a repository, and the trailing segment has to stay refused rather than
     be read as one now that the repositories view reads its own. */
  it('does not let another view carry a repository', () => {
    expect(parsePanelRoute('', '/workspace/acme/settings/api-gateway')).toBeNull();
    expect(parsePanelRoute('', '/workspace/acme/settings/api-gateway')).toBeNull();
    expect(parsePanelRoute('', '/workspace/acme/sync/api-gateway')).toBeNull();
    expect(parsePanelRoute('', '/root/workspaces/acme/settings/api-gateway')).toBeNull();
    expect(parsePanelRoute('', '/root/workspaces/acme/settings/api-gateway')).toBeNull();
  });
});
