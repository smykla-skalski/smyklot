<script lang="ts" generics="Row">
  import type { Snippet } from 'svelte';
  import { flip, type FlipParams } from 'svelte/animate';
  import { fade, type FadeParams } from 'svelte/transition';

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
   * The markup swap is easy; the **CSS is the whole job**. Exactly which rules break is
   * worth knowing precisely, because the first guess is wrong in both directions:
   *
   * - **Breaks:** every rule against `table`, `thead`, `tbody`, `tr` and the wrapper
   *   class. This component renders those, so they carry its scope and not the
   *   caller's. Rewrite each as `:global(.that-table tbody tr)`, anchored through the
   *   class the caller passes, so it reaches that table's rows and no others.
   * - **Survives:** rules against `th` and `td`. A `cells` or `head` snippet is written
   *   in the caller's file, so its cells are the caller's markup and carry the caller's
   *   scope. Column widths, per-cell alignment and `data-label` rules all keep working
   *   untouched - which is most of what a table's stylesheet actually is.
   *
   * The trap is the other way round: a surviving `td` rule is only `td.svelte-caller`,
   * one class, and `.data-table :is(tbody th, td)` in `app.css` is one class and two
   * elements. **The shared floor outranks it.** So a table that padded its cells
   * differently does not keep that padding by doing nothing - it has to say so through
   * `--table-cell-pad-block` and `--table-cell-pad-inline`. That is why those are knobs.
   *
   * So each rule has one of three homes:
   *
   * - **This shell, every table** - the pinned layout, the stacked layout, the cell
   *   separator. `app.css`, beside `.table-card` and `.data-row`.
   * - **This shell, this table** - min-width, cell padding, band height, `table-layout`.
   *   A custom property the caller sets on the class it passes.
   * - **This table's columns** - widths, a cell that wraps differently. Stays exactly
   *   where it is, in the caller.
   *
   * Do them one table at a time and run `tests/browser/table-columns.test.ts` between
   * each - it is the check that catches a grid that stopped reaching the body, and at
   * ~74s it is the slowest in the suite for a reason.
   *
   * ## What the band centres against
   *
   * A cell's separator is on its BOTTOM only, so a row's border box is 1px taller than
   * the band a reader sees and its centre sits half a pixel below the visible one.
   * Measure a row's contents against the border box and everything reads 0.5px high,
   * every time, in every table - which looks exactly like a defect and is not one. The
   * reference is the CONTENT box, above the rule: measured there, the identity stack
   * and the chip land at 0.0000px and the heading label at -0.0039px, the remainder
   * being the cap trim's own subpixel.
   *
   * ## `pinned` and `stacked` are opt-in, and three tables decline
   *
   * They are shared layouts, not the layout. A table takes one only if its own is the
   * same layout, and two here are not:
   *
   * - `UserManagement`'s pair lay a row out as a **grid** so the header and the body
   *   can share one set of column tracks, where `pinned` says `display: table`. And
   *   their filters live in the column headings with no tools menu behind them, so
   *   `stacked` - which hides `thead` at 64rem - takes away the only way to filter.
   *   Both faults were invisible to `svelte-check` and both were caught by a browser
   *   suite: headings 195px from their cells, and no reachable filter at 320.
   * - `RepositoryList` does the same with grid rows, at its own breakpoint.
   *
   * The lesson is the shape of every trap in this file: a shared rule at two classes
   * beats a caller's at one, and CSS reports nothing when it wins. Prefer declining a
   * shared layout to fighting it.
   *
   * ## Row motion, and why it is a branch rather than a prop
   *
   * `QueueView` animates: its rows carry `animate:flip`, `in:fade` and `out:fade`.
   * Those are **compile-time directives**. They cannot be spread through `rowAttrs`
   * like an event handler, and a transition function cannot be passed in and applied -
   * `in:{someProp}` is not a thing Svelte compiles. They have to be written on the
   * element itself, by whoever writes the `{#each}`, and that is this component.
   *
   * `animate:` also has to be on an element that is the *immediate* child of a keyed
   * each block, so it cannot be hidden behind an `{#if}` inside one either. What is
   * left is branching the `{#each}` itself, which is what the body below does.
   *
   * That branch is why the directives are not simply always applied with a zero
   * duration: flip measures every row's box before and after every update, and eight
   * of the nine tables never move a row. `motion` is how the ninth asks, and the other
   * eight pay nothing.
   */
  /* `let`, not `const`: `body` is `$bindable`, and a bindable prop has to be
     assignable. */
  let {
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
    lead,
    bodyAttrs,
    colgroup,
    columnWidths,
    afterRow,
    tableClass = '',
    body = $bindable(),
    scrollable = true,
    pinned = false,
    stacked = false,
    motion,
  }: {
    rows: readonly Row[];
    /**
     * Stable identity, so a re-sort moves rows rather than rebuilding them.
     *
     * The return type is as wide as `{#each}`'s own key, which is what this feeds:
     * `@tanstack/virtual` types its key as `string | number | bigint`, and narrowing
     * to `string` here made two tables' keys a type error rather than a decision.
     */
    rowKey: (row: Row) => PropertyKey | bigint;
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
    /**
     * Attributes for the `<tbody>`.
     *
     * The queue watches its body for a pointer or focus arriving and leaving, and asks
     * the question at the edges of the table rather than at every cell boundary
     * inside it - which is only possible on the body element itself.
     */
    bodyAttrs?: Record<string, unknown>;
    /**
     * The `<tbody>` element, for a virtualiser that has to measure and scroll it.
     *
     * `bind:body` rather than a callback: the virtualisers here take a
     * `getScrollElement` that is read during setup, and a callback that fires after
     * mount is one frame too late for the first measurement.
     */
    body?: HTMLTableSectionElement;
    /**
     * Rendered inside the body, before the rows.
     *
     * This exists for one thing: the virtual spacer, the aria-hidden row whose height
     * is the full size of the list so the scrollport has something to scroll. It has
     * to be inside `<tbody>` and it is not a row, so it is neither `rows` nor `head`.
     */
    lead?: Snippet;
    /** The caller's own layout for the shell. Never its surface, which is `.table-card`. */
    class?: string;
    /**
     * Classes for the `<table>` itself, distinct from the shell's.
     *
     * A table whose columns are declared in a `colgroup` names them from here, because
     * a `<col>` is styled through the table and not through the card around it.
     */
    tableClass?: string;
    /** A `<colgroup>`, for a table that declares its columns rather than sizing cells. */
    colgroup?: Snippet;
    /**
     * One entry per column, in order, as a CSS length - `undefined` leaves a column
     * to take what is left.
     *
     * This is where a column width belongs, and the reason is not tidiness. Written
     * as CSS in the view, a width is a selector like `.my-table :is(th, td):nth-child(3)`
     * - and the table it names is rendered by *this* component, so the view's scope
     * class never lands on it and the rule dies without a word from the compiler.
     * Every table that tried it lost its columns to an equal split. As data it cannot
     * miss: this renders the `<colgroup>` itself, which is what `table-layout: fixed`
     * reads before it looks at a single cell.
     */
    columnWidths?: readonly (string | undefined)[];
    /**
     * A second `<tr>` after a row's own, when it has something to say.
     *
     * One table announces a per-row failure through a visually-hidden alert row, and
     * an alert has to be its own row: put inside the row's last cell it is read as
     * part of the last column, and put outside the table it is read away from the
     * row it belongs to.
     */
    afterRow?: Snippet<[Row]>;
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
    /**
     * Row motion, for a table whose rows move rather than being rebuilt. Omit it and
     * the rows are static and cost nothing - see the note above on why this is a
     * branch and not a set of directives applied to everything.
     */
    motion?: {
      flip: FlipParams;
      arriving: FadeParams;
      leaving: FadeParams;
    };
  } = $props();

  /* Loud rather than silent. `animate:` requires the row to be the only child of its
     keyed each block, so the animated branch cannot also render `afterRow`, and a
     table that asked for both would quietly lose every second row. */
  $effect(() => {
    if (motion !== undefined && afterRow !== undefined) {
      throw new Error(
        'DataTable: `motion` and `afterRow` cannot be combined - an animated row must be the only child of its each block',
      );
    }
  });
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
  <table class={tableClass}>
    <caption class="visually-hidden">{caption}</caption>
    {#if colgroup !== undefined}
      {@render colgroup()}
    {:else if columnWidths !== undefined}
      <!-- The width goes on through a spread rather than a literal `style=`, which
           `tests/csp-safety.test.ts` refuses in this directory. -->
      <colgroup>
        {#each columnWidths as width, index (index)}
          <!--
            Through a variable rather than the value directly, because a `<col>`
            width BEATS a cell width in fixed table layout - so a width written here
            would silently outrank every responsive rule a table has for that column.
            `--table-col-N` is where a media query retunes one, which is the same
            knob-shaped seam the rest of this shell uses.
          -->
          <col {...{ style: `width:var(--table-col-${index + 1}, ${width ?? 'auto'})` }} />
        {/each}
      </colgroup>
    {/if}
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
    <tbody bind:this={body} data-panel-scroll onscroll={onBodyScroll} {...bodyAttrs ?? {}}>
      {#if lead !== undefined}
        {@render lead()}
      {/if}
      <!--
        Two `{#each}` blocks over the same rows, which is not duplication for its own
        sake. `animate:` has to sit on an element that is the *immediate* child of a
        keyed each, so it cannot be put behind an `{#if}` inside one, and a directive
        is compile-time - it cannot travel through `rowAttrs` or arrive as a prop. The
        only way to make row motion optional is to branch the block itself.

        Optional is the point. `flip` measures every row's box before and after every
        update, and one table animates. The branch is what lets the other eight pay
        nothing for it.
      -->
      {#if rows.length === 0}
        {#if empty !== undefined}
          <tr class="state-row">
            <td colspan={columnCount ?? columns?.length ?? 1} class="empty-cell">
              {@render empty()}
            </td>
          </tr>
        {/if}
      {:else if motion !== undefined}
        <!-- No `afterRow` here, and the script above refuses the pair rather than
             dropping it quietly: the compiler wants the animated row to be the ONLY
             child of its keyed each, not merely the first. -->
        {#each rows as row (rowKey(row))}
          <tr
            class="data-row"
            animate:flip={motion.flip}
            in:fade={motion.arriving}
            out:fade={motion.leaving}
            {...rowAttrs?.(row) ?? {}}
          >
            {@render cells(row)}
          </tr>
        {/each}
      {:else}
        {#each rows as row (rowKey(row))}
          <tr class="data-row" {...rowAttrs?.(row) ?? {}}>
            {@render cells(row)}
          </tr>
          {#if afterRow !== undefined}
            {@render afterRow(row)}
          {/if}
        {/each}
      {/if}
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

  /* The band's height and its zero block padding moved to `.data-table thead th` in
     `app.css`. Scoped here they reached only the headings this component renders
     from `columns`, and six of the seven tables pass a `head` snippet whose cells
     wear the caller's scope - so the rules missed them, the band was whatever its
     contents happened to be, and the `--table-heading-height` three of them set was
     never read.

     The knob is still how a table asks for a different band. NOT via
     `box-sizing: content-box` - the sticky-header layout gives thead and tbody rows
     the same percentage column widths, and under content-box the header's
     percentages stop including its 24px of padding, so the two grids drift apart by
     a whole cell. */

  /* The one row that is not a row. Six tables declared this pair identically, and
     the two that also had to say `background: transparent` were the two whose row
     rules were general enough to paint it - which is not a state, it is a table
     saying "the thing I painted was not meant to include this". `.state-row` never
     gets `.data-row`, so nothing paints it in the first place. */
  .empty-cell {
    color: var(--text-secondary);
    /* 12rem in the tables that had no rows to compare it against, 10rem in the two
       whose empty state sits under a toolbar. Both are deliberate, so it is a knob:
       folding them together would shift one of them by 32px on a page nobody would
       think to re-check. */
    height: var(--table-empty-height, 12rem);
    text-align: center;
  }
</style>
