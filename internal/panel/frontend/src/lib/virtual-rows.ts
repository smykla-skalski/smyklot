/**
 * The shape a virtualised table's rows are rendered from.
 *
 * Three tables build this the same way - a ternary between the virtualiser's items
 * and a plain list, because below the breakpoint the rows are cards and there is
 * nothing to virtualise. That ternary types as `A[] | B[]`, and a union of arrays has
 * no single element type, so `DataTable`'s generic has nothing to infer from it.
 * Stating the shape collapses the union where the two branches meet.
 *
 * `key` is as wide as `@tanstack/virtual`'s own, which is as wide as `{#each}`'s.
 */
export type VirtualRenderRow = {
  index: number;
  key: string | number | bigint;
  /** The row's measured height; 0 when the list is not virtualised. */
  size: number;
  /** The row's offset down the list; 0 when the list is not virtualised. */
  start: number;
  virtual: boolean;
};
