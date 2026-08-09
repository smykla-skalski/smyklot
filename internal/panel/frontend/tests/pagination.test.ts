import { describe, expect, it } from 'vitest';

import { paginationItems } from '../src/lib/pagination';

describe('paginationItems', () => {
  it('shows every page when the range is short', () => {
    expect(paginationItems(1, 1)).toEqual([1]);
    expect(paginationItems(3, 5)).toEqual([1, 2, 3, 4, 5]);
  });

  it('keeps the current page centered inside a long range', () => {
    expect(paginationItems(6, 12)).toEqual([1, 'start-ellipsis', 5, 6, 7, 'end-ellipsis', 12]);
  });

  it('expands the visible pages at the start and end', () => {
    expect(paginationItems(2, 12)).toEqual([1, 2, 3, 4, 5, 'end-ellipsis', 12]);
    expect(paginationItems(11, 12)).toEqual([1, 'start-ellipsis', 8, 9, 10, 11, 12]);
  });
});
