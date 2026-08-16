<script lang="ts">
  import { updateFilterSelection } from '../filter-menu';
  import type { FilterOption, FilterSection } from '../filter-menu';
  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';

  const {
    label,
    summary,
    hint,
    sections,
    selected,
    multiple = false,
    fallbackValue,
    align = 'start',
    wide = false,
    onChange,
  }: {
    label: string;
    summary: string;
    hint: string;
    sections: readonly FilterSection[];
    selected: readonly string[];
    multiple?: boolean;
    fallbackValue?: string;
    align?: 'start' | 'end';
    wide?: boolean;
    onChange: (values: string[]) => void;
  } = $props();

  let triggerButton = $state<HTMLElement | null>(null);
  let open = $state(false);

  const options = $derived(sections.flatMap((section) => section.options));
  // The fallback ("all") is the unfiltered state, not a selection worth counting.
  const selectedCount = $derived(
    selected.filter(
      (value) =>
        value !== fallbackValue &&
        options.find((option) => option.value === value)?.exclusive !== true,
    ).length,
  );
  const canClear = $derived(
    fallbackValue === undefined
      ? selected.length > 0
      : selected.length !== 1 || selected[0] !== fallbackValue,
  );

  function choose(option: FilterOption): void {
    onChange(updateFilterSelection(selected, option, options, multiple, fallbackValue));
    if (!multiple) close();
  }

  function clear(): void {
    onChange(fallbackValue === undefined ? [] : [fallbackValue]);
  }

  /* The layer returns focus to the trigger itself when it closes with focus
     still inside it, so this looks like a second copy of that. It stays: the
     layer can only act on where focus actually is, and Safari does not focus a
     button that was clicked, so closing from Done or an option leaves focus
     loose there and nothing to return. Relaxing the layer's rule to cover that
     would also fire when somebody clicked away to dismiss it, which is the one
     case where the trigger should not take focus back. */
  function close(): void {
    open = false;
    triggerButton?.focus();
  }
</script>

<Popover bind:open {align} itemSelector=".filter-option">
  {#snippet trigger(attributes)}
    <!-- Open state is read from the trigger's own `aria-expanded`, which the
         layer writes: mirroring it into a class here made two conventions out of
         one fact, and the other two menus already read the attribute.

         A funnel in a column heading is the only shape this has: every caller
         asked for it, so the component no longer offers another. It carried a
         `placement` prop, an `iconOnly` prop and a `showIcon` prop whose other
         values nothing had ever passed - a labelled toolbar control, with a
         chevron and a visible summary, that could not be reached from anywhere
         and so could not be seen to have rotted. -->
    <button
      class="filter-trigger"
      class:filtered={canClear}
      type="button"
      bind:this={triggerButton}
      aria-haspopup="listbox"
      aria-label={`${label}: ${summary}`}
      {...attributes}
    >
      <Icon name="filter" size={14} />
      {#if selectedCount > 0}
        <span class="selection-count" aria-hidden="true">{selectedCount}</span>
      {/if}
    </button>
  {/snippet}

  <div class="filter-body" class:wide>
    <header>
      <strong>{label}</strong>
      <span>{hint}</span>
    </header>

    <div
      class="filter-options"
      role="listbox"
      aria-label={label}
      aria-multiselectable={multiple ? 'true' : undefined}
    >
      {#each sections as section, sectionIndex (section.label ?? sectionIndex)}
        <div
          class="filter-section"
          role="group"
          aria-label={section.label === undefined ? undefined : section.label}
        >
          {#if section.label !== undefined}
            <p class="section-label">{section.label}</p>
          {/if}
          {#each section.options as option (option.value)}
            {@const isSelected = selected.includes(option.value)}
            <button
              type="button"
              class="filter-option"
              class:selected={isSelected}
              role="option"
              aria-selected={isSelected}
              onclick={() => choose(option)}
            >
              <span class:multiple class="selection-mark" aria-hidden="true">
                {#if isSelected}<span></span>{/if}
              </span>
              {#if option.tone !== undefined && option.tone !== 'default'}
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
        </div>
      {/each}
    </div>

    {#if multiple}
      <footer>
        <button type="button" class="clear-button" disabled={!canClear} onclick={clear}
          >Clear</button
        >
        <button type="button" class="done-button" onclick={close}>Done</button>
      </footer>
    {/if}
  </div>
</Popover>

<style>
  /* No `position: relative` on an ancestor and no z-index: the layer is in the
     top layer, which nothing in the page can be stacked over or clipped by. The
     trigger is positioned only so the count can ride its corner. */
  .filter-trigger {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: var(--radius-control);
    color: var(--text-muted);
    cursor: pointer;
    display: flex;
    flex: none;
    height: 1.75rem;
    justify-content: center;
    line-height: 1;
    padding: 0;
    position: relative;
    transition:
      background-color var(--duration-fast) var(--ease-out),
      color var(--duration-fast) var(--ease-out),
      transform var(--duration-press) var(--ease-standard);
    user-select: none;
    width: 1.75rem;
  }

  .filter-trigger:hover,
  .filter-trigger[aria-expanded='true'] {
    background: color-mix(in srgb, var(--text-primary) 8%, transparent);
    color: var(--text-primary);
  }

  .filter-trigger:active {
    background: color-mix(in srgb, var(--text-primary) 14%, transparent);
    color: var(--text-primary);
    transform: scale(var(--press-scale-disc));
  }

  .filter-trigger.filtered {
    background: var(--brand-action);
    color: var(--on-brand-action);
  }

  .filter-trigger.filtered:hover,
  .filter-trigger.filtered[aria-expanded='true'] {
    background: var(--brand-action-hover);
    color: var(--on-brand-action);
  }

  .filter-trigger.filtered:active {
    background: var(--brand-action-pressed);
    color: var(--on-brand-action);
  }

  /* The count rides the funnel's corner so an active filter says how many
     values it holds without reclaiming header width. */
  .selection-count {
    align-items: center;
    background: var(--surface-base);
    border-radius: var(--radius-chip);
    box-shadow: 0 0 0 1px var(--border-subtle);
    color: var(--brand-action-text);
    display: inline-grid;
    font: 700 0.5625rem / 1 var(--sans);
    height: 0.875rem;
    padding: 2px 4px;
    place-items: center;
    position: absolute;
    right: -4px;
    top: -4px;
  }

  /* A column that can shrink, so the header and the footer keep their size and
     the options between them take whatever the layer has left. Which edge it
     lines up with is the layer's business now, not a left/right pair here. */
  .filter-body {
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
    width: min(17rem, calc(100vw - 2rem));
  }

  .filter-body.wide {
    width: min(21rem, calc(100vw - 2rem));
  }

  .filter-body > header {
    border-bottom: 1px solid var(--rule);
    display: flex;
    flex-direction: column;
    padding: 0.7rem 0.75rem 0.625rem;
  }

  .filter-body > header strong {
    font-size: 0.75rem;
    line-height: 1.25;
  }

  .filter-body > header span {
    color: var(--dim);
    font-size: 0.625rem;
    line-height: 1.35;
    margin-top: 0.1rem;
  }

  /* `min-height: 0`, which is what lets a flex item scroll rather than push its
     siblings out of the layer. The height it may take is whatever the layer
     measured for itself, so there is no 24rem guess here any more. */
  .filter-options {
    min-height: 0;
    overflow-y: auto;
    padding: 0.35rem;
  }

  .filter-section + .filter-section {
    border-top: 1px solid var(--rule);
    margin-top: 0.35rem;
    padding-top: 0.35rem;
  }

  .section-label {
    color: var(--dim);
    font-size: 0.5625rem;
    font-weight: 700;
    letter-spacing: 0.1em;
    margin: 0;
    padding: 0.35rem 0.45rem 0.25rem;
    text-transform: uppercase;
  }

  .filter-option {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: 6px;
    color: var(--text);
    display: flex;
    gap: 0.5rem;
    min-height: 2.25rem;
    padding: 0.4rem 0.45rem;
    text-align: left;
    width: 100%;
  }

  .filter-option:hover,
  .filter-option:focus-visible {
    background: var(--interactive-hover);
  }

  .filter-option:focus-visible {
    outline-offset: -1px;
  }

  .filter-option:active {
    box-shadow: inset 0 0 0 100vmax var(--press);
  }

  .selection-mark {
    border: 1.5px solid var(--dim);
    border-radius: 50%;
    display: grid;
    flex: none;
    height: 0.875rem;
    place-items: center;
    width: 0.875rem;
  }

  .selection-mark.multiple {
    border-radius: 4px;
  }

  .filter-option.selected .selection-mark {
    background: var(--brand-action-tint);
    border-color: var(--brand-action);
  }

  .selection-mark span {
    background: var(--brand-action);
    border-radius: 50%;
    height: 0.35rem;
    width: 0.35rem;
  }

  .selection-mark.multiple span {
    background: transparent;
    border-bottom: 1.5px solid var(--brand-action);
    border-right: 1.5px solid var(--brand-action);
    border-radius: 0;
    height: 0.42rem;
    transform: rotate(45deg) translate(-0.05rem, -0.05rem);
    width: 0.22rem;
  }

  .tone {
    background: var(--dim);
    border-radius: 50%;
    flex: none;
    height: 0.4rem;
    width: 0.4rem;
  }

  .tone-on,
  .tone-valid {
    background: var(--clear);
  }

  .tone-off,
  .tone-invalid {
    background: var(--stop);
  }

  .tone-bypassed {
    background: var(--warning);
  }

  .option-copy {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .option-copy strong {
    font-size: 0.6875rem;
    font-weight: 550;
    line-height: 1.25;
  }

  .option-copy span {
    color: var(--dim);
    font-size: 0.625rem;
    line-height: 1.25;
    margin-top: 0.1rem;
  }

  .filter-body > footer {
    align-items: center;
    border-top: 1px solid var(--rule);
    display: flex;
    justify-content: space-between;
    padding: 0.45rem;
  }

  .filter-body > footer button {
    border: 0;
    border-radius: 6px;
    font-size: 0.6875rem;
    font-weight: 650;
    height: 1.875rem;
    padding: 0 0.65rem;
  }

  .clear-button {
    background: transparent;
    color: var(--dim);
  }

  .clear-button:hover:not(:disabled) {
    background: var(--strip-lift);
    color: var(--text);
  }

  .clear-button:disabled {
    cursor: default;
    opacity: 0.4;
  }

  .done-button {
    background: var(--admin);
    color: var(--on-admin);
  }

  .done-button:hover {
    background: color-mix(in srgb, var(--admin) 88%, var(--strip));
  }

  @media (prefers-reduced-motion: reduce) {
    .filter-trigger {
      transition: none;
    }
  }
</style>
