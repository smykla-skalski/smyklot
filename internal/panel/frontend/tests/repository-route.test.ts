import { describe, expect, it } from 'vitest';

import { panelAddress } from '../src/lib/addresses';
import { basePath } from '../src/lib/paths';
import {
  REPOSITORY_SECTIONS,
  availableRepositorySections,
  parsePanelRoute,
} from '../src/lib/routes';

/**
 * The address of one repository's page.
 *
 * It is the address it had as a dialog, and that is the point: the page replaced
 * the dialog without breaking a single link somebody had already sent. What the
 * segments MEAN changed, so these read the route rather than the path - a
 * `repository` on the route is a page, a `dialog` on it is something standing
 * over the list, and the two are no longer the same address.
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
  it('reads the repository and its pane out of the repositories path', () => {
    expect(parsePanelRoute('', '/i/acme/repositories/api-gateway')).toEqual({
      account: 'acme',
      view: 'repositories',
      repository: { name: 'api-gateway', section: 'file' },
    });
    expect(parsePanelRoute('', '/i/acme/repositories/api-gateway/commands')).toEqual({
      account: 'acme',
      view: 'repositories',
      repository: { name: 'api-gateway', section: 'commands' },
    });
  });

  it('is a page, never a dialog', () => {
    // The whole point of the change: nothing about this address says something
    // is standing on top of the list, because nothing is.
    const route = parsePanelRoute('', '/i/acme/repositories/api-gateway/behavior');
    expect(route).not.toBeNull();
    expect(route).not.toHaveProperty('dialog');
  });

  it('writes the pane only when it is not the one the page opens on', () => {
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
    expect(parsePanelRoute('', '/i/acme/repositories/file')).toEqual({
      account: 'acme',
      view: 'repositories',
      repository: { name: 'file', section: 'file' },
    });
    expect(parsePanelRoute('', '/i/acme/repositories/behavior/behavior')).toEqual({
      account: 'acme',
      view: 'repositories',
      repository: { name: 'behavior', section: 'behavior' },
    });
  });

  it('leaves the bare list alone', () => {
    expect(parsePanelRoute('', '/i/acme/repositories')).toEqual({
      account: 'acme',
      view: 'repositories',
    });
  });

  it('refuses a pane that is not one', () => {
    // An address that does not resolve, rather than the page quietly opening on
    // the pane it starts on - a mistyped pane should say so.
    expect(parsePanelRoute('', '/i/acme/repositories/api-gateway/nonsense')).toBeNull();
    expect(parsePanelRoute('', '/i/acme/repositories/api-gateway/file/extra')).toBeNull();
  });

  it('escapes a repository name that needs it', () => {
    const path = panelAddress({
      account: 'acme',
      view: 'repositories',
      repository: { name: 'a b/c', section: 'file' },
    });
    expect(path).toBe(`${basePath}/i/acme/repositories/a%20b%2Fc`);
    expect(parsePanelRoute(basePath, path)).toEqual({
      account: 'acme',
      view: 'repositories',
      repository: { name: 'a b/c', section: 'file' },
    });
  });

  it('refuses a name that decodes to nothing', () => {
    expect(parsePanelRoute('', '/i/acme/repositories/%zz')).toBeNull();
    expect(parsePanelRoute('', '/i/acme/repositories/%20')).toBeNull();
  });
});

describe('one repository through the Root console [Unit]', () => {
  it('takes the same shape of address it has in a workspace', () => {
    expect(
      parsePanelRoute('', '/root/installations/acme/repositories/api-gateway/behavior'),
    ).toEqual({
      rootView: 'installation',
      account: 'acme',
      view: 'repositories',
      repository: { name: 'api-gateway', section: 'behavior' },
    });
    expect(roundTrip('/root/installations/acme/repositories/api-gateway/behavior')).toBe(
      '/root/installations/acme/repositories/api-gateway/behavior',
    );
    expect(roundTrip('/root/installations/acme/repositories/api-gateway/file')).toBe(
      '/root/installations/acme/repositories/api-gateway',
    );
  });

  it('refuses a pane that is not one there either', () => {
    expect(parsePanelRoute('', '/root/installations/acme/repositories/api/nonsense')).toBeNull();
  });

  /* Sync is not a console view, and defaults hosts nothing after it. Neither
     takes a repository, and the trailing segment has to stay refused rather than
     be read as one now that the repositories view reads its own. */
  it('does not let another view carry a repository', () => {
    expect(parsePanelRoute('', '/i/acme/defaults/api-gateway')).toBeNull();
    expect(parsePanelRoute('', '/i/acme/settings/api-gateway')).toBeNull();
    expect(parsePanelRoute('', '/i/acme/sync/api-gateway')).toBeNull();
    expect(parsePanelRoute('', '/root/installations/acme/defaults/api-gateway')).toBeNull();
    expect(parsePanelRoute('', '/root/installations/acme/settings/api-gateway')).toBeNull();
  });

  /**
   * Which panes a surface offers, as against which panes exist.
   *
   * The sync pane is reachable at a Root address - the router accepts the
   * segment, because the router's list is the complete one and the Go server is
   * handed the same pattern - and the console has nowhere to ask about sync, so
   * the page lands on the first pane instead of drawing an empty box. That
   * fallback had no test at either end, and it is the whole reason this
   * function exists rather than the list being read directly.
   */
  describe('the panes a surface offers', () => {
    it('offers every pane where sync can be asked about', () => {
      expect(availableRepositorySections(true)).toEqual(['file', 'behavior', 'commands', 'sync']);
    });

    it('leaves sync out where there is nowhere to ask', () => {
      expect(availableRepositorySections(false)).toEqual(['file', 'behavior', 'commands']);
    });

    /* Filtered from the router's own list rather than written out again, so a
       fifth pane is offered without anybody remembering this file. */
    it('offers panes the router knows, in the order it lists them', () => {
      expect(availableRepositorySections(true)).toEqual([...REPOSITORY_SECTIONS]);
    });
  });
});
