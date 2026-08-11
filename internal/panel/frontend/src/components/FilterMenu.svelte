<script lang="ts">
  import { updateFilterSelection } from '../lib/filter-menu';
  import type { FilterOption, FilterSection } from '../lib/filter-menu';
  import Icon from './Icon.svelte';

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
    showIcon = false,
    iconOnly = false,
    placement = 'toolbar',
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
    showIcon?: boolean;
    iconOnly?: boolean;
    placement?: 'toolbar' | 'header';
    onChange: (values: string[]) => void;
  } = $props();

  let menu = $state<HTMLDetailsElement | null>(null);
  let trigger = $state<HTMLElement | null>(null);

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

  $effect(() => {
    function closeFromOutside(event: PointerEvent): void {
      if (menu?.open === true && event.target instanceof Node && !menu.contains(event.target)) {
        close(false);
      }
    }

    function closeFromKeyboard(event: KeyboardEvent): void {
      if (event.key !== 'Escape' || menu?.open !== true) return;
      event.preventDefault();
      close(true);
    }

    document.addEventListener('pointerdown', closeFromOutside);
    document.addEventListener('keydown', closeFromKeyboard);
    return () => {
      document.removeEventListener('pointerdown', closeFromOutside);
      document.removeEventListener('keydown', closeFromKeyboard);
    };
  });

  function choose(option: FilterOption): void {
    onChange(updateFilterSelection(selected, option, options, multiple, fallbackValue));
    if (!multiple) close(true);
  }

  function clear(): void {
    onChange(fallbackValue === undefined ? [] : [fallbackValue]);
  }

  function close(restoreFocus: boolean): void {
    if (menu !== null) menu.open = false;
    if (restoreFocus) trigger?.focus();
  }

  function moveOptionFocus(event: KeyboardEvent): void {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    const buttons = Array.from(menu?.querySelectorAll<HTMLButtonElement>('.filter-option') ?? []);
    if (buttons.length === 0) return;

    event.preventDefault();
    const current = buttons.indexOf(event.currentTarget as HTMLButtonElement);
    let next = event.key === 'Home' ? 0 : buttons.length - 1;
    if (event.key === 'ArrowDown') next = (current + 1) % buttons.length;
    if (event.key === 'ArrowUp') next = (current - 1 + buttons.length) % buttons.length;
    buttons[next]?.focus();
  }
</script>

<details
  class="filter-menu"
  class:align-end={align === 'end'}
  class:wide
  class:header-filter={placement === 'header'}
  class:filtered={canClear}
  bind:this={menu}
>
  <summary class:icon-only={iconOnly} bind:this={trigger} aria-label={`${label}: ${summary}`}>
    {#if showIcon}<Icon name="filter" size={14} />{/if}
    <span class="summary-copy">{summary}</span>
    {#if (multiple || placement === 'header') && selectedCount > 0}
      <span class="selection-count" aria-hidden="true">{selectedCount}</span>
    {/if}
    <span class="menu-chevron" aria-hidden="true"><Icon name="chevron-down" size={16} /></span>
  </summary>

  <div class="filter-popover">
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
              onkeydown={moveOptionFocus}
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
        <button type="button" class="done-button" onclick={() => close(true)}>Done</button>
      </footer>
    {/if}
  </div>
</details>

<style>
  .filter-menu {
    min-width: 0;
    position: relative;
  }

  .filter-menu[open] {
    z-index: var(--layer-menu);
  }

  summary {
    align-items: center;
    background: var(--control-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text);
    cursor: pointer;
    display: flex;
    font-size: var(--font-size-body);
    gap: 0.4rem;
    height: var(--local-control-height, var(--control-height));
    line-height: 1;
    padding: 0 0.625rem;
    transition:
      background-color var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out),
      color var(--duration-fast) var(--ease-out),
      transform var(--duration-press) var(--ease-standard);
    user-select: none;
  }

  summary.icon-only {
    flex: none;
    justify-content: space-between;
    padding: 0 0.625rem;
    position: relative;
    width: 3.75rem;
  }

  .header-filter {
    flex: none;
  }

  .header-filter summary {
    background: transparent;
    border: 0;
    border-radius: var(--radius-control);
    color: var(--text-muted);
    height: 1.75rem;
    justify-content: center;
    padding: 0;
    position: relative;
    width: 1.75rem;
  }

  .header-filter summary.icon-only {
    width: 1.75rem;
  }

  .header-filter summary .menu-chevron {
    display: none;
  }

  /* The count rides the funnel's corner so an active filter says how many
     values it holds without reclaiming header width. Specificity beats the
     icon-only in-button placement below. */
  .filter-menu.header-filter summary .selection-count {
    background: var(--surface-base);
    border-radius: var(--radius-chip);
    box-shadow: 0 0 0 1px var(--border-subtle);
    color: var(--brand-action-text);
    font: 700 0.5625rem / 1 var(--sans);
    margin: 0;
    min-width: 0;
    padding: 2px 4px;
    position: absolute;
    right: -4px;
    top: -4px;
  }

  .header-filter.filtered summary {
    background: var(--brand-action);
    color: var(--on-brand-action);
  }

  .header-filter summary:hover,
  .header-filter[open] summary {
    background: color-mix(in srgb, var(--text-primary) 8%, transparent);
    border-color: transparent;
    color: var(--text-primary);
  }

  .header-filter.filtered summary:hover,
  .header-filter.filtered[open] summary {
    background: var(--brand-action-hover);
    color: var(--on-brand-action);
  }

  .header-filter.filtered summary:active {
    background: var(--brand-action-pressed);
    color: var(--on-brand-action);
  }

  .header-filter summary:active {
    background: color-mix(in srgb, var(--text-primary) 14%, transparent);
    color: var(--text-primary);
    transform: scale(0.9);
  }

  summary.icon-only .summary-copy {
    display: none;
  }

  summary.icon-only .selection-count {
    height: 0.875rem;
    min-width: 0.875rem;
    padding: 0 0.15rem;
    position: absolute;
    right: 1px;
    top: 1px;
  }

  summary::-webkit-details-marker {
    display: none;
  }

  summary::marker {
    content: '';
  }

  summary:hover,
  .filter-menu[open] summary {
    background: var(--control-bg-hover);
    border-color: var(--control-border-hover);
  }

  summary:active {
    background: var(--interactive-pressed-bg);
    border-color: var(--control-border-hover);
    transform: translateY(1px) scale(0.98);
  }

  .summary-copy {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .selection-count {
    align-items: center;
    background: var(--signal-tint);
    border-radius: 999px;
    color: var(--signal);
    display: inline-grid;
    font-size: 0.5625rem;
    font-weight: 700;
    height: 1rem;
    line-height: 1;
    min-width: 1rem;
    padding: 0 0.2rem;
    place-items: center;
  }

  .menu-chevron {
    color: var(--text-muted);
    display: grid;
    flex: none;
    margin-left: auto;
    place-items: center;
    transition: transform var(--duration-fast) var(--ease-out);
  }

  .filter-menu[open] .menu-chevron {
    transform: rotate(180deg);
  }

  .filter-popover {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    left: 0;
    overflow: hidden;
    position: absolute;
    top: calc(100% + 0.35rem);
    width: min(17rem, calc(100vw - 2rem));
    z-index: var(--layer-menu);
  }

  .align-end .filter-popover {
    left: auto;
    right: 0;
  }

  .wide .filter-popover {
    width: min(21rem, calc(100vw - 2rem));
  }

  .filter-popover > header {
    border-bottom: 1px solid var(--rule);
    display: flex;
    flex-direction: column;
    padding: 0.7rem 0.75rem 0.625rem;
  }

  .filter-popover > header strong {
    font-size: 0.75rem;
    line-height: 1.25;
  }

  .filter-popover > header span {
    color: var(--dim);
    font-size: 0.625rem;
    line-height: 1.35;
    margin-top: 0.1rem;
  }

  .filter-options {
    max-height: min(24rem, calc(100vh - 12rem));
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

  /* A dot beside a label takes the shared nudge; see --ink-nudge in app.css. */
  .tone {
    background: var(--dim);
    border-radius: 50%;
    flex: none;
    height: 0.4rem;
    translate: 0 var(--ink-nudge);
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

  .filter-popover > footer {
    align-items: center;
    border-top: 1px solid var(--rule);
    display: flex;
    justify-content: space-between;
    padding: 0.45rem;
  }

  .filter-popover > footer button {
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
    summary,
    .menu-chevron {
      transition: none;
    }
  }
</style>
