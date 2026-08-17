<script lang="ts">
  /**
   * One control for sorting and filtering a table, for the widths where the
   * table's own headings are not there to carry either.
   *
   * A column heading is a fine place for a sort button and a funnel while there
   * are columns. On a phone the table is a stack of cards and the headings are
   * gone, and on the repositories table that meant three sorts and three filters
   * disappeared with them - the page offered a search field and nothing else.
   * This gathers them into a single menu beside that field.
   *
   * It owns no state. Sorting and filtering already live where the table keeps
   * them, and a second copy here would be a second answer to the same question.
   */
  import { updateFilterSelection, type FilterOption, type FilterSection } from '../filter-menu';
  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';

  export interface ToolsSort {
    label: string;
    /** Which way this column is sorted, or nothing when the table is sorted by another. */
    direction: 'ascending' | 'descending' | undefined;
    onToggle: () => void;
  }

  export interface ToolsFilter {
    label: string;
    hint: string;
    sections: readonly FilterSection[];
    selected: readonly string[];
    multiple?: boolean;
    /** The value that means "unfiltered", so it is not counted as a choice. */
    fallbackValue?: string;
    onChange: (values: string[]) => void;
  }

  const {
    label = 'Sort and filter',
    sorts,
    filters,
  }: {
    label?: string;
    sorts: readonly ToolsSort[];
    filters: readonly ToolsFilter[];
  } = $props();

  let triggerButton = $state<HTMLElement | null>(null);
  let open = $state(false);

  /** What the badge counts: choices that narrow the table, not the defaults. */
  const activeFilters = $derived(
    filters.reduce((total, filter) => {
      const options = filter.sections.flatMap((section) => section.options);
      return (
        total +
        filter.selected.filter(
          (value) =>
            value !== filter.fallbackValue &&
            options.find((option) => option.value === value)?.exclusive !== true,
        ).length
      );
    }, 0),
  );

  function choose(filter: ToolsFilter, option: FilterOption): void {
    const options = filter.sections.flatMap((section) => section.options);
    filter.onChange(
      updateFilterSelection(
        filter.selected,
        option,
        options,
        filter.multiple === true,
        filter.fallbackValue,
      ),
    );
  }

  /* Filters only. The sort is not a filter and there is always one, so a single
     "clear" that also threw the ordering away would be doing two jobs under one
     word. */
  function clearAll(): void {
    for (const filter of filters) {
      filter.onChange(filter.fallbackValue === undefined ? [] : [filter.fallbackValue]);
    }
  }

  /* The menu stays open through every choice it offers. A column heading's funnel
     can close on a single-select because the reader came for that one answer;
     here they came for the set, and closing after the first would put the rest
     behind a second press. */
  function close(): void {
    open = false;
    triggerButton?.focus();
  }
</script>

<Popover bind:open align="end" itemSelector=".tools-option">
  {#snippet trigger(attributes)}
    <button
      class="tools-trigger"
      class:filtered={activeFilters > 0}
      type="button"
      bind:this={triggerButton}
      aria-label={`${label}${activeFilters > 0 ? `, ${activeFilters} active` : ''}`}
      {...attributes}
    >
      <!-- No word on it. It sorts and it filters, and a button labelled with one
           of the two says the wrong thing about the other; the sliders carry
           both, and the accessible name says it in full. -->
      <Icon name="sliders" size={16} />
      {#if activeFilters > 0}
        <span class="tools-count" aria-hidden="true">{activeFilters}</span>
      {/if}
    </button>
  {/snippet}

  <div class="tools-body">
    <div class="tools-scroll">
      <!-- A table whose rows carry their own order - the queue is sorted by what
           happens next - passes no sorts, and an empty group would announce a
           heading with nothing under it. -->
      {#if sorts.length > 0}
        <div class="tools-group" role="group" aria-label="Sort">
          <p class="tools-label">Sort by</p>
          {#each sorts as sort (sort.label)}
            <button
              type="button"
              class="tools-option"
              class:selected={sort.direction !== undefined}
              aria-pressed={sort.direction !== undefined}
              onclick={sort.onToggle}
            >
              <span class="selection-mark" aria-hidden="true">
                {#if sort.direction !== undefined}<span></span>{/if}
              </span>
              <span class="option-copy">
                <strong>{sort.label}</strong>
                {#if sort.direction !== undefined}
                  <span>{sort.direction === 'ascending' ? 'Ascending' : 'Descending'}</span>
                {/if}
              </span>
              {#if sort.direction !== undefined}
                <!-- The arrow says which way, and the press flips it. Rotated rather
                 than a second glyph, so the two directions are one shape. -->
                <span class:descending={sort.direction === 'descending'} class="sort-arrow">
                  <Icon name="chevron-up" size={14} />
                </span>
              {/if}
            </button>
          {/each}
        </div>
      {/if}

      {#each filters as filter (filter.label)}
        <div
          class="tools-group"
          role="listbox"
          aria-label={filter.label}
          aria-multiselectable={filter.multiple === true ? 'true' : undefined}
        >
          <p class="tools-label">{filter.label}</p>
          {#each filter.sections as section, index (section.label ?? index)}
            {#each section.options as option (option.value)}
              {@const isSelected = filter.selected.includes(option.value)}
              <button
                type="button"
                class="tools-option"
                class:selected={isSelected}
                role="option"
                aria-selected={isSelected}
                onclick={() => choose(filter, option)}
              >
                <span
                  class:multiple={filter.multiple === true}
                  class="selection-mark"
                  aria-hidden="true"
                >
                  {#if isSelected}<span></span>{/if}
                </span>
                {#if option.tone !== undefined}
                  <span class="tone tone-{option.tone}" aria-hidden="true"></span>
                {/if}
                <span class="option-copy">
                  <strong>{option.label}</strong>
                  {#if option.description !== undefined}
                    <span>{option.description}</span>
                  {/if}
                </span>
              </button>
            {/each}
          {/each}
        </div>
      {/each}
    </div>

    <footer>
      <button type="button" class="clear-button" disabled={activeFilters === 0} onclick={clearAll}>
        Clear filters
      </button>
      <button type="button" class="done-button" onclick={close}>Done</button>
    </footer>
  </div>
</Popover>

<style>
  /* The height is the search field's, which is the compact control height the
     whole panel uses for a field. The two stand on one line and a control that
     is 6px taller than the thing beside it reads as a mistake before it reads
     as emphasis. Square while there is nothing to count, and only as much wider
     as the count needs. */
  .tools-trigger {
    align-items: center;
    background: var(--control-surface);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-secondary);
    display: inline-flex;
    flex: none;
    gap: var(--space-1);
    height: var(--control-height-compact);
    justify-content: center;
    min-width: var(--control-height-compact);
    padding: 0 var(--space-2);
    position: relative;
  }

  .tools-trigger:hover,
  .tools-trigger[aria-expanded='true'] {
    background: var(--interactive-hover);
    color: var(--text);
  }

  .tools-trigger.filtered {
    border-color: color-mix(in srgb, var(--brand-action) 45%, var(--control-border));
    color: var(--brand-action-text);
  }

  /* Matched to the field beside it rather than grown past it, so the thumb gets
     its 44px from an overlay that paints nothing. Same construction as the top
     bar's controls. */
  @media (pointer: coarse) {
    .tools-trigger::after {
      content: '';
      inset: calc((2.75rem - 100%) / -2) 0;
      position: absolute;
    }
  }

  .tools-count {
    background: var(--brand-action-tint);
    border-radius: 0.25rem;
    color: var(--brand-action-text);
    font: 700 var(--font-size-nano) / 1 var(--mono);
    min-width: 0.875rem;
    padding: 0.15rem 0.2rem;
    text-align: center;
  }

  /* The list is long - three filters, one of them every configuration key - so
     the body scrolls and the footer does not go with it. A grid of three rows
     rather than one scrolling block: the actions stay reachable without
     scrolling to the end of the overrides to find them. */
  .tools-body {
    display: grid;
    grid-template-rows: minmax(0, 1fr) auto;
    max-height: min(24rem, 60dvh);
    width: min(17.5rem, calc(100vw - 2rem));
  }

  /* No padding at the top. A sticky heading pinned to `top: 0` pins to the
     scrollport's edge, so any padding above it is a strip the heading does not
     cover and the rows scroll through it. */
  .tools-scroll {
    overflow: auto;
    overscroll-behavior: contain;
    padding: 0 var(--space-2) var(--space-2);
  }

  .tools-group {
    display: grid;
  }

  /* Sticky, because the groups are long enough that the reader arrives in the
     middle of one: an option list with the name of its group scrolled off says
     nothing about what it is choosing between.

     The space between groups is this heading's own padding rather than a margin
     on the group. A margin sits outside the sticky box and nothing paints it, so
     it was a hole above every pinned heading with the list running through it.
     Inside the padding, the background covers it. `--layer-bg` and not a surface
     token: it is the menu's own ground, whichever skin the menu is wearing. */
  .tools-label {
    background: var(--layer-bg);
    color: var(--text-muted);
    font: 650 var(--font-size-micro) / 1 var(--sans);
    letter-spacing: 0.08em;
    margin: 0;
    padding: var(--space-3) var(--space-2) var(--space-2);
    position: sticky;
    text-transform: uppercase;
    top: 0;
    z-index: 1;
  }

  .tools-option {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: var(--r-ctl);
    color: var(--text);
    display: flex;
    font: inherit;
    gap: var(--space-2);
    /* A menu row on a phone is a thumb target before it is a line of text. */
    min-height: 2.5rem;
    padding: var(--space-2);
    text-align: start;
    width: 100%;
  }

  .tools-option:hover {
    background: var(--interactive-hover);
  }

  .tools-option:active {
    background: var(--interactive-pressed-bg);
  }

  .selection-mark {
    border: 1px solid var(--control-border);
    border-radius: 50%;
    display: grid;
    flex: none;
    height: 1rem;
    place-items: center;
    width: 1rem;
  }

  /* Square for a set, round for a choice: the shape says whether picking this
     one puts the last one back. */
  .selection-mark.multiple {
    border-radius: 0.25rem;
  }

  .tools-option.selected .selection-mark {
    background: var(--brand-action);
    border-color: var(--brand-action);
  }

  .selection-mark span {
    background: var(--on-brand-action);
    border-radius: 50%;
    height: 0.375rem;
    width: 0.375rem;
  }

  .selection-mark.multiple span {
    border-radius: 1px;
  }

  .tone {
    border-radius: 50%;
    flex: none;
    height: 0.5rem;
    width: 0.5rem;
  }

  .tone-on,
  .tone-valid {
    background: var(--success);
  }

  .tone-off,
  .tone-missing {
    background: var(--text-muted);
  }

  .tone-invalid {
    background: var(--danger);
  }

  .tone-bypassed {
    background: var(--warning);
  }

  /* Every member of `FilterTone` needs a fill here: a tone with no rule is a dot
     with no background, which is a hole in the row rather than a colour. */
  .tone-neutral {
    background: var(--text-muted);
  }

  .tone-signal {
    background: var(--brand-action);
  }

  .option-copy {
    display: grid;
    gap: 0.1rem;
    min-width: 0;
  }

  .option-copy strong {
    font: 600 var(--font-size-meta) / 1.3 var(--sans);
  }

  .option-copy span {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .sort-arrow {
    color: var(--brand-action-text);
    display: grid;
    margin-inline-start: auto;
    place-items: center;
    transition: transform var(--duration-fast) var(--ease-standard);
  }

  .sort-arrow.descending {
    transform: rotate(180deg);
  }

  footer {
    align-items: center;
    background: var(--surface-base);
    border-top: 1px solid var(--border-subtle);
    display: flex;
    gap: var(--space-2);
    justify-content: space-between;
    padding: var(--space-2);
  }

  .clear-button {
    background: transparent;
    border: 0;
    border-radius: var(--r-ctl);
    color: var(--text-secondary);
    font: 600 var(--font-size-control) / 1 var(--sans);
    min-height: 2.25rem;
    padding: 0 var(--space-2);
  }

  .clear-button:hover:not(:disabled) {
    background: var(--interactive-hover);
    color: var(--text);
  }

  .clear-button:disabled {
    color: var(--text-muted);
    opacity: 0.55;
  }

  .done-button {
    background: var(--brand-action);
    border: 0;
    border-radius: var(--r-ctl);
    color: var(--on-brand-action);
    font: 600 var(--font-size-control) / 1 var(--sans);
    min-height: 2.25rem;
    padding: 0 var(--space-4);
  }

  @media (prefers-reduced-motion: reduce) {
    .sort-arrow {
      transition: none;
    }
  }
</style>
