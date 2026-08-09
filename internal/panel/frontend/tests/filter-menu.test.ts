import { describe, expect, it } from 'vitest';

import { updateFilterSelection } from '../src/lib/filter-menu';
import type { FilterOption } from '../src/lib/filter-menu';

const OPTIONS: FilterOption[] = [
  { value: 'all', label: 'All', exclusive: true },
  { value: 'none', label: 'None', exclusive: true },
  { value: 'alpha', label: 'Alpha' },
  { value: 'beta', label: 'Beta' },
];

describe('updateFilterSelection', () => {
  it('replaces the current choice in single-select menus', () => {
    expect(updateFilterSelection(['all'], OPTIONS[2]!, OPTIONS, false)).toEqual(['alpha']);
  });

  it('combines regular choices and removes exclusive presets', () => {
    expect(updateFilterSelection(['all'], OPTIONS[2]!, OPTIONS, true, 'all')).toEqual(['alpha']);
    expect(updateFilterSelection(['alpha'], OPTIONS[3]!, OPTIONS, true, 'all')).toEqual([
      'alpha',
      'beta',
    ]);
  });

  it('restores the fallback after the final regular choice is removed', () => {
    expect(updateFilterSelection(['alpha'], OPTIONS[2]!, OPTIONS, true, 'all')).toEqual(['all']);
  });

  it('keeps exclusive presets mutually exclusive', () => {
    expect(updateFilterSelection(['alpha', 'beta'], OPTIONS[1]!, OPTIONS, true, 'all')).toEqual([
      'none',
    ]);
  });
});
