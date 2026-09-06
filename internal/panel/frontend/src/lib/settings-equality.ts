/** Preserve authored order while recognizing an unchanged set of selections. */
export function restoreSelectionOrder(next: readonly string[], saved: readonly string[]): string[] {
  const same =
    next.length === saved.length &&
    JSON.stringify(next.toSorted()) === JSON.stringify(saved.toSorted());
  return [...(same ? saved : next)];
}
