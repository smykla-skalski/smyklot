import { describe, expect, it } from 'vitest';

import { dialogRoute, dialogSearch, parseDialog } from '../src/lib/dialog-route.svelte';

/**
 * A history stack, not a single slot.
 *
 * What this router does on close is press Back, so a fake that only remembers the
 * current URL cannot tell a correct implementation from one that leaves the
 * dialog in the address. The entries and the pointer are the whole point.
 */
function fakeBrowser(initialUrl = '/panel') {
  const entries: { url: string; state: unknown }[] = [{ url: initialUrl, state: null }];
  let index = 0;
  const listeners = new Set<() => void>();

  const entry = (): { url: string; state: unknown } => {
    const found = entries[index];
    if (found === undefined) throw new Error(`no history entry at ${index}`);
    return found;
  };

  const parts = (): { pathname: string; search: string } => {
    const [pathname = '', query] = entry().url.split('?');
    return { pathname, search: query === undefined ? '' : `?${query}` };
  };

  const browser = {
    location: {
      get pathname(): string {
        return parts().pathname;
      },
      get search(): string {
        return parts().search;
      },
    },
    history: {
      get state(): unknown {
        return entry().state;
      },
      pushState(state: unknown, _unused: string, url: string): void {
        entries.splice(index + 1);
        entries.push({ url, state });
        index += 1;
      },
      replaceState(state: unknown, _unused: string, url: string): void {
        entries[index] = { url, state };
      },
      back(): void {
        if (index === 0) return;
        index -= 1;
        for (const listener of listeners) listener();
      },
    },
    addEventListener: (_type: 'popstate', listener: () => void) => listeners.add(listener),
    removeEventListener: (_type: 'popstate', listener: () => void) => listeners.delete(listener),
  };

  return {
    browser,
    url: (): string => entry().url,
    depth: (): number => entries.length,
    forward(): void {
      if (index + 1 >= entries.length) return;
      index += 1;
      for (const listener of listeners) listener();
    },
    /** A reader pressing Back, as opposed to the router spending its own entry. */
    back(): void {
      browser.history.back();
    },
  };
}

describe('dialog addresses [Unit]', () => {
  it('reads the dialog and its parameters out of a query string', () => {
    expect(parseDialog('?dialog=repository-settings&repository=42&section=commands')).toEqual({
      name: 'repository-settings',
      params: { repository: '42', section: 'commands' },
    });
  });

  it('reads no dialog from a query that names none', () => {
    expect(parseDialog('')).toBeNull();
    expect(parseDialog('?page=2')).toBeNull();
    expect(parseDialog('?dialog=')).toBeNull();
    expect(parseDialog('?dialog=%20%20')).toBeNull();
  });

  it('writes an empty search for no dialog, so it can be assigned onto a path', () => {
    expect(dialogSearch(null)).toBe('');
    expect(dialogSearch({ name: 'add-user', params: {} })).toBe('?dialog=add-user');
  });

  it('escapes parameters rather than letting them end the query', () => {
    const search = dialogSearch({ name: 'user-action', params: { user: 'a&dialog=other' } });
    expect(parseDialog(search)).toEqual({
      name: 'user-action',
      params: { user: 'a&dialog=other' },
    });
  });
});

describe('dialog router [Unit]', () => {
  it('puts the open dialog in the address and takes it back out', () => {
    const fake = fakeBrowser();
    const detach = dialogRoute.attach(fake.browser);

    dialogRoute.open('add-user');
    expect(dialogRoute.isOpen('add-user')).toBe(true);
    expect(fake.url()).toBe('/panel?dialog=add-user');

    dialogRoute.close();
    expect(dialogRoute.current).toBeNull();
    expect(fake.url()).toBe('/panel');

    detach();
  });

  it('leaves nothing to press Back through after a dialog is dismissed', () => {
    const fake = fakeBrowser();
    const detach = dialogRoute.attach(fake.browser);

    dialogRoute.open('repository-settings', { repository: '42' });
    dialogRoute.close();

    /* Closing spent the entry it had added, so the reader is back where they
       started and Back leaves the panel rather than re-opening what they just
       dismissed. Dropping the query in place instead would leave one press of
       Back between them and the way out. */
    fake.back();
    expect(fake.url()).toBe('/panel');
    expect(dialogRoute.current).toBeNull();

    detach();
  });

  it('re-opens what the address names when navigation brings it back', () => {
    const fake = fakeBrowser();
    const detach = dialogRoute.attach(fake.browser);

    dialogRoute.open('decision-history', { user: 'octocat' });
    dialogRoute.close();
    fake.forward();
    expect(dialogRoute.current).toEqual({ name: 'decision-history', params: { user: 'octocat' } });

    detach();
  });

  it('opens what a pasted address names, and closing it stays in the panel', () => {
    const fake = fakeBrowser('/panel?dialog=add-user');
    const detach = dialogRoute.attach(fake.browser);

    expect(dialogRoute.isOpen('add-user')).toBe(true);

    /* Nothing of ours is behind this entry - going back would leave the panel for
       whatever the reader was looking at before it. */
    dialogRoute.close();
    expect(fake.url()).toBe('/panel');
    expect(fake.depth()).toBe(1);

    detach();
  });

  it('swaps one dialog for another without stacking a second entry', () => {
    const fake = fakeBrowser();
    const detach = dialogRoute.attach(fake.browser);

    dialogRoute.open('user-action', { user: 'octocat', action: 'suspend' });
    dialogRoute.open('decision-history', { user: 'octocat' });
    expect(fake.depth()).toBe(2);

    dialogRoute.close();
    expect(dialogRoute.current).toBeNull();
    expect(fake.url()).toBe('/panel');

    detach();
  });

  it('changes a parameter in place, so the switch inside a dialog is not a stop on the way back', () => {
    const fake = fakeBrowser();
    const detach = dialogRoute.attach(fake.browser);

    dialogRoute.open('repository-settings', { repository: '42' });
    dialogRoute.update('repository-settings', { section: 'commands' });
    expect(fake.url()).toBe('/panel?dialog=repository-settings&repository=42&section=commands');
    expect(fake.depth()).toBe(2);

    dialogRoute.close();
    expect(fake.url()).toBe('/panel');

    detach();
  });

  it('ignores an update aimed at a dialog that is not the open one', () => {
    const fake = fakeBrowser();
    const detach = dialogRoute.attach(fake.browser);

    dialogRoute.open('add-user');
    dialogRoute.update('repository-settings', { section: 'commands' });
    expect(fake.url()).toBe('/panel?dialog=add-user');

    dialogRoute.close();
    detach();
  });

  it('reads a parameter only for the dialog that is open', () => {
    const fake = fakeBrowser();
    const detach = dialogRoute.attach(fake.browser);

    dialogRoute.open('repository-settings', { repository: '42' });
    expect(dialogRoute.param('repository-settings', 'repository')).toBe('42');
    expect(dialogRoute.param('add-user', 'repository')).toBeUndefined();

    dialogRoute.close();
    detach();
  });
});
