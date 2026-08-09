<script lang="ts">
  import { updateFilterSelection } from '../lib/filter-menu';
  import type { FilterOption, FilterSection } from '../lib/filter-menu';

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

  let menu = $state<HTMLDetailsElement | null>(null);
  let trigger = $state<HTMLElement | null>(null);

  const options = $derived(sections.flatMap((section) => section.options));
  const selectedCount = $derived(
    selected.filter((value) => options.find((option) => option.value === value)?.exclusive !== true)
      .length,
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

<details class="filter-menu" class:align-end={align === 'end'} class:wide bind:this={menu}>
  <summary bind:this={trigger} aria-label={`${label}: ${summary}`}>
    <span class="summary-copy">{summary}</span>
    {#if multiple && selectedCount > 1}
      <span class="selection-count mono" aria-hidden="true">{selectedCount}</span>
    {/if}
    <span class="menu-chevron" aria-hidden="true"></span>
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
            <p class="section-label mono">{section.label}</p>
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
    z-index: 25;
  }

  summary {
    align-items: center;
    background: var(--control-surface);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text);
    cursor: pointer;
    display: flex;
    font-size: 0.6875rem;
    gap: 0.4rem;
    height: var(--repository-control-height, var(--control-height));
    padding: 0 0.625rem;
    transition:
      background-color 120ms ease-out,
      border-color 120ms ease-out;
    user-select: none;
  }

  summary::-webkit-details-marker {
    display: none;
  }

  summary::marker {
    content: '';
  }

  summary:hover,
  .filter-menu[open] summary {
    background: var(--strip-lift);
    border-color: color-mix(in srgb, var(--dim) 56%, transparent);
  }

  summary:focus-visible {
    box-shadow: 0 0 0 2px var(--brand);
    outline: none;
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
    display: inline-flex;
    font-size: 0.5625rem;
    font-weight: 700;
    height: 1rem;
    justify-content: center;
    min-width: 1rem;
    padding: 0 0.2rem;
  }

  .menu-chevron {
    border-bottom: 1.5px solid var(--dim);
    border-right: 1.5px solid var(--dim);
    flex: none;
    height: 0.35rem;
    margin: -0.2rem 0.05rem 0 auto;
    transform: rotate(45deg);
    transition: transform 120ms ease-out;
    width: 0.35rem;
  }

  .filter-menu[open] .menu-chevron {
    margin-top: 0.2rem;
    transform: rotate(225deg);
  }

  .filter-popover {
    background: var(--strip);
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    box-shadow: 0 12px 32px var(--shadow);
    left: 0;
    overflow: hidden;
    position: absolute;
    top: calc(100% + 0.35rem);
    width: min(17rem, calc(100vw - 2rem));
    z-index: 25;
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
    background: var(--strip-lift);
    outline: none;
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
    background: var(--signal-tint);
    border-color: var(--signal);
  }

  .selection-mark span {
    background: var(--signal);
    border-radius: 50%;
    height: 0.35rem;
    width: 0.35rem;
  }

  .selection-mark.multiple span {
    background: transparent;
    border-bottom: 1.5px solid var(--signal);
    border-right: 1.5px solid var(--signal);
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
    filter: brightness(1.08);
  }

  @media (prefers-reduced-motion: reduce) {
    summary,
    .menu-chevron {
      transition: none;
    }
  }
</style>
