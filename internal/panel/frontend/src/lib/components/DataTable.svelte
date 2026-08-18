<script lang="ts" generics="Row">
  import type { Snippet } from 'svelte';

  /**
   * The shell every table in the panel shares.
   *
   * Nine tables across seven files wrote this by hand, under six different wrapper
   * class names - `table-scroll`, `user-table-wrap`, `installation-table-shell`,
   * `repository-table-scroll`, `queue-card` - each carrying a copy of the same
   * comment saying the surface comes from `.table-card`. Three separate bugs had been
   * patched in three of those files.
   *
   * **A row is not a component, and must never become one.** That was tried, measured
   * and reverted: a `<tr>` rendered by a child carries the *child's* Svelte scope
   * class, so every `.repositories tbody tr` rule in the parent stops matching - the
   * desktop grid never reached the body, and `tests/browser/table-columns.test.ts`
   * caught headings sitting up to 3.4k pixels from their cells. So this component
   * renders the `<tr>` itself and takes a snippet for the cells inside it: the row
   * stays in one scope, which is what lets one stylesheet own every row state.
   *
   * For the same reason there is **no rule here that paints `tbody tr`**. `.data-row`
   * in `app.css` owns the resting ground, the hover, the press and the focus, and a
   * scoped `tbody tr` here would carry this component's class and outrank it - which
   * would kill hover in every table at once. Anything a table needs to paint that
   * `.data-row` does not, it paints on `tr:not(.virtual-spacer, .data-row)`.
   *
   * ## Migrating a table onto this
   *
   * The markup swap is easy; the **CSS is the whole job**, and it is why converting a
   * table is not a five-minute change. Every rule a table writes against `table`,
   * `thead`, `tbody`, `tr`, `th` or `td` stops matching the moment this component
   * renders those elements - they carry this scope, not the caller's. Converting
   * `RootInstallations` broke eighteen selectors at once, which is the honest measure
   * of the work.
   *
   * There are only two good answers, and which one applies is per rule:
   *
   * - The rule is about **this shell** - heading colour, cell padding, the row grid.
   *   It belongs in `app.css` beside `.table-card` and `.data-row`, where every table
   *   already reads it from, or here if it is structural.
   * - The rule is about **that table's columns** - a `grid-template-columns`, a
   *   column's width, a cell that wraps differently. It stays with the caller and is
   *   anchored through the class it passes in: `:global(.repositories thead)`.
   *
   * Do them one table at a time and run `tests/browser/table-columns.test.ts` between
   * each - it is the check that catches a grid that stopped reaching the body, and at
   * ~74s it is the slowest in the suite for a reason.
   */
  const {
    rows,
    rowKey,
    caption,
    regionLabel,
    columns,
    cells,
    rowAttrs,
    head,
    foot,
    empty,
    columnCount,
    class: extra = '',
    onBodyScroll,
    scrollable = true,
    pinned = false,
    stacked = false,
  }: {
    rows: readonly Row[];
    /** Stable identity, so a re-sort moves rows rather than rebuilding them. */
    rowKey: (row: Row) => string;
    /** Names the table for a screen reader; drawn only for one. */
    caption: string;
    /** Names the scroll region, which is focusable so a keyboard can pan it. */
    regionLabel: string;
    /** Plain headings. A table that sorts or filters passes `head` instead. */
    columns?: ReadonlyArray<{ label: string; class?: string }>;
    /** The `<th>` and `<td>` of one row - never the `<tr>`, which is this component's. */
    cells: Snippet<[Row]>;
    /** Per-row attributes: `tabindex`, click and key handlers, attachments. */
    rowAttrs?: (row: Row) => Record<string, unknown>;
    /** A whole `<tr>` of headings, for a table whose columns sort or filter. */
    head?: Snippet;
    /**
     * What stands in the body when there are no rows.
     *
     * Wrapped in the `<tr>` and the spanning `<td>` here rather than by the caller:
     * nine tables wrote that pair by hand, and the `colspan` has to agree with the
     * number of columns or the cell stops spanning them. `columnCount` is how a table
     * with its own `head` says how many it has.
     */
    empty?: Snippet;
    /** Only needed alongside `head`; `columns` counts itself. */
    columnCount?: number;
    /** Anything after the body - a sentinel, a load-more notice. */
    foot?: Snippet;
    /**
     * The body scrolled.
     *
     * On `tbody` rather than the card, because a pinned table scrolls its rows and
     * not its shell - a listener on the outer element never fires there, which is
     * how one table's load-on-scroll quietly stopped at the first page.
     */
    onBodyScroll?: (event: Event) => void;
    /** The caller's own layout for the shell. Never its surface, which is `.table-card`. */
    class?: string;
    /** Off for a table that is short by construction and should not own a scrollport. */
    scrollable?: boolean;
    /**
     * Fill the workspace, pin the header band, scroll only the rows.
     *
     * The layout is in `app.css` because it reaches `thead`, `tbody` and the rows -
     * and it needs each caller's column widths, which cannot be here. A pinned table
     * must state them, or its header and its body lay out independently.
     */
    pinned?: boolean;
    /**
     * Below 64rem a row becomes a card and each cell carries its own label.
     *
     * The label is `data-label` on the `<td>`, so a `cells` snippet for a stacked
     * table has to set it - a cell without one stacks under a blank label. Layout in
     * `app.css`, for the same reason `pinned` is: it reaches the cells.
     */
    stacked?: boolean;
  } = $props();
</script>

<!--
  `role="region"` with `tabindex` so a keyboard can scroll columns that overflow the
  viewport. Every table declared this except `RepositoryList`, which had lost it - so
  one table could not be panned without a pointer. It is here now, so none of them can
  lose it again.
-->
<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<div
  class="data-table table-card {extra}"
  class:scrollable
  class:pinned
  class:stacked
  role="region"
  tabindex="0"
  aria-label={regionLabel}
>
  <table>
    <caption class="visually-hidden">{caption}</caption>
    <thead>
      {#if head !== undefined}
        {@render head()}
      {:else if columns !== undefined}
        <tr>
          {#each columns as column (column.label)}
            <!--
              The same anatomy a sorting heading has, minus the button: the band's
              padding lives on `.table-heading > *` rather than on the cell, so a
              heading that skipped the wrapper sat flush against the column edge.
              `.table-heading-label` is where the cap trim is, which is why there
              is no `cap-trim` here - two trims on one box is one too many.
            -->
            <th scope="col" class={column.class}>
              <div class="table-heading">
                <span class="table-heading-label">{column.label}</span>
              </div>
            </th>
          {/each}
        </tr>
      {/if}
    </thead>
    <!-- `data-panel-scroll` is what the panel's scroll bookkeeping looks for. -->
    <tbody data-panel-scroll onscroll={onBodyScroll}>
      {#each rows as row (rowKey(row))}
        <tr class="data-row" {...rowAttrs?.(row) ?? {}}>
          {@render cells(row)}
        </tr>
      {:else}
        {#if empty !== undefined}
          <tr class="state-row">
            <td colspan={columnCount ?? columns?.length ?? 1} class="empty-cell">
              {@render empty()}
            </td>
          </tr>
        {/if}
      {/each}
    </tbody>
  </table>
  {#if foot !== undefined}
    {@render foot()}
  {/if}
</div>

<style>
  .scrollable {
    overflow-y: auto;
  }

  /* `separate`, not `collapse`: a collapsed border is shared between adjacent rows,
     so each cell owns half of it and every row box lands on a .5 - the header
     measured 40.5 against the approved table's 41, and every row 59.5 inside a 60px
     row. Separated borders keep each box whole. Six of the seven tables had worked
     this out independently; the seventh had not, and its rows were the half-pixel
     ones. */
  table {
    border-collapse: separate;
    border-spacing: 0;
    /* The one number that is genuinely per table - the width below which its
       columns stop being readable and the card should scroll instead. It ranged
       from 40rem to 52rem across the seven, which is a real difference and not a
       drift, so it stays a knob rather than becoming a floor. */
    min-width: var(--table-min-width, 0);
    /* `fixed` in the tables that state column widths, `auto` in the ones that let
       their content decide. Both are in use and both are right for their table. */
    table-layout: var(--table-layout, auto);
    width: 100%;
  }

  /* Only ever the headings this component draws itself: a `head` snippet's cells
     carry the caller's scope, so that table keeps its own band rule - which is what
     `app.css` means where it says the height stays with each table.

     2.5rem of band plus its own rule. NOT via `box-sizing: content-box` - the
     sticky-header layout gives thead and tbody rows the same percentage column
     widths, and under content-box the header's percentages stop including its 24px
     of padding, so the two grids drift apart by a whole cell. */
  thead th {
    height: var(--table-heading-height, calc(2.5rem + 1px));
    padding-block: 0;
  }

  /* The one row that is not a row. Six tables declared this pair identically, and
     the two that also had to say `background: transparent` were the two whose row
     rules were general enough to paint it - which is not a state, it is a table
     saying "the thing I painted was not meant to include this". `.state-row` never
     gets `.data-row`, so nothing paints it in the first place. */
  .empty-cell {
    color: var(--text-secondary);
    height: 12rem;
    text-align: center;
  }
</style>
