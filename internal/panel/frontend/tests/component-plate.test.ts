// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';

import { parseDialog, dialogSearch } from '../src/lib/dialog-route.svelte';

describe('dialog address parsing [Component]', () => {
  it('parses a dialog with params from a query string', () => {
    const result = parseDialog('?dialog=repository-settings&repository=42&section=commands');
    expect(result).toEqual({
      name: 'repository-settings',
      params: { repository: '42', section: 'commands' },
    });
  });

  it('returns null for a query with no dialog', () => {
    expect(parseDialog('')).toBeNull();
    expect(parseDialog('?page=2')).toBeNull();
    expect(parseDialog('?dialog=')).toBeNull();
  });

  it('serializes a dialog back to a query string', () => {
    expect(dialogSearch({ name: 'add-user', params: {} })).toBe('?dialog=add-user');
    expect(dialogSearch(null)).toBe('');
  });
});
