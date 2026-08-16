import { describe, expect, it } from 'vitest';

import { dialogSearch, parseDialog } from '../src/lib/dialog-route.svelte';

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
