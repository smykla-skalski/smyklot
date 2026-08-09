export type PaginationItem = number | 'start-ellipsis' | 'end-ellipsis';

const MAX_VISIBLE_ITEMS = 7;
const EDGE_PAGE_COUNT = 5;

export function paginationItems(currentPage: number, pageCount: number): PaginationItem[] {
  const total = Math.max(1, Math.floor(pageCount));
  const current = Math.min(total, Math.max(1, Math.floor(currentPage)));
  if (total <= MAX_VISIBLE_ITEMS) return pageRange(1, total);

  if (current <= 4) {
    return [...pageRange(1, EDGE_PAGE_COUNT), 'end-ellipsis', total];
  }
  if (current >= total - 3) {
    return [1, 'start-ellipsis', ...pageRange(total - EDGE_PAGE_COUNT + 1, total)];
  }

  return [1, 'start-ellipsis', current - 1, current, current + 1, 'end-ellipsis', total];
}

function pageRange(start: number, end: number): number[] {
  return Array.from({ length: end - start + 1 }, (_, index) => start + index);
}
