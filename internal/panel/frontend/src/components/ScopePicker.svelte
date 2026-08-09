<script lang="ts">
  import { fuzzyCandidates } from '../lib/fuzzy';
  import type { PanelTarget } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import Icon from './Icon.svelte';

  const {
    global,
    targetId,
    targets,
    canSelectGlobal,
    variant = 'toolbar',
    label = 'User scope',
    onSelect,
  }: {
    global: boolean;
    targetId: string;
    targets: readonly PanelTarget[];
    canSelectGlobal: boolean;
    variant?: 'toolbar' | 'field';
    label?: string;
    onSelect: (targetId: string | null) => void;
  } = $props();

  let picker = $state<HTMLDetailsElement | null>(null);
  let trigger = $state<HTMLElement | null>(null);
  let searchInput = $state<HTMLInputElement | null>(null);
  let query = $state('');

  const selectedTarget = $derived(targets.find((target) => target.id === targetId));
  const candidates = $derived(
    fuzzyCandidates(
      targets.map((target) => ({
        ...target,
        label: target.account.display_name,
        keywords: [target.account.login, target.type],
      })),
      query,
    ),
  );
  const organizationCandidates = $derived(
    candidates.filter((target) => target.type === 'Organization'),
  );
  const personalCandidates = $derived(candidates.filter((target) => target.type === 'User'));
  const globalMatches = $derived(
    fuzzyCandidates(
      [{ id: 'global', label: 'Global', keywords: ['all installations', 'all organizations'] }],
      query,
    ).length > 0,
  );

  function outside(event: PointerEvent): void {
    if (picker?.open === true && event.target instanceof Node && !picker.contains(event.target)) {
      close(false);
    }
  }

  function escape(event: KeyboardEvent): void {
    if (event.key !== 'Escape' || picker?.open !== true) return;
    event.preventDefault();
    close(true);
  }

  function toggled(): void {
    if (picker?.open !== true) return;
    query = '';
    queueMicrotask(() => searchInput?.focus());
  }

  function choose(nextTarget: string | null): void {
    close(true);
    onSelect(nextTarget);
  }

  function close(restoreFocus: boolean): void {
    if (picker !== null) picker.open = false;
    query = '';
    if (restoreFocus) trigger?.focus();
  }

  function move(event: KeyboardEvent): void {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    const buttons = Array.from(picker?.querySelectorAll<HTMLButtonElement>('.scope-option') ?? []);
    if (buttons.length === 0) return;
    event.preventDefault();
    const current = buttons.indexOf(event.currentTarget as HTMLButtonElement);
    let next = event.key === 'Home' ? 0 : buttons.length - 1;
    if (event.key === 'ArrowDown') next = (current + 1) % buttons.length;
    if (event.key === 'ArrowUp') next = (current - 1 + buttons.length) % buttons.length;
    buttons[next]?.focus();
  }
</script>

<svelte:document onpointerdown={outside} onkeydowncapture={escape} />

{#snippet scopeOption(target: PanelTarget)}
  {@const selected = !global && target.id === targetId}
  <button
    class="scope-option"
    class:selected
    type="button"
    role="option"
    aria-selected={selected}
    onclick={() => choose(target.id)}
    onkeydown={move}
  >
    <Avatar account={target.account} size={26} />
    <span class="option-copy">
      <strong>{target.account.display_name}</strong>
      <span>@{target.account.login}</span>
    </span>
    <span class="option-check" aria-hidden="true">
      {#if selected}<Icon name="success" size={16} />{/if}
    </span>
  </button>
{/snippet}

<details
  class="scope-picker"
  class:scope-field={variant === 'field'}
  bind:this={picker}
  ontoggle={toggled}
>
  <summary bind:this={trigger} aria-label={label}>
    {#if global}
      <span class="global-mark" aria-hidden="true"><Icon name="globe" size={16} /></span>
      <span class="scope-copy">
        <strong>Global</strong>
        {#if variant === 'field'}<small>All installations</small>{/if}
      </span>
    {:else if selectedTarget !== undefined}
      <Avatar account={selectedTarget.account} size={22} />
      <span class="scope-copy">
        <strong>{selectedTarget.account.display_name}</strong>
        {#if variant === 'field'}<small>@{selectedTarget.account.login}</small>{/if}
      </span>
    {/if}
    <span class="scope-chevron" aria-hidden="true"><Icon name="chevron-down" size={16} /></span>
  </summary>

  <div class="scope-popover">
    <label class="scope-search">
      <span class="visually-hidden">Search user scopes</span>
      <span class="search-icon" aria-hidden="true"><Icon name="search" size={18} /></span>
      <input
        class="text-input"
        type="search"
        placeholder="Search installations"
        bind:this={searchInput}
        bind:value={query}
      />
    </label>

    <div class="scope-options" role="listbox" aria-label="User scope">
      {#if canSelectGlobal && globalMatches}
        <button
          class="scope-option global-option"
          class:selected={global}
          type="button"
          role="option"
          aria-selected={global}
          onclick={() => choose(null)}
          onkeydown={move}
        >
          <span class="global-mark" aria-hidden="true"><Icon name="globe" size={16} /></span>
          <span class="option-copy">
            <strong>Global</strong>
            <span>Access across all installations</span>
          </span>
          <span class="option-check" aria-hidden="true">
            {#if global}<Icon name="success" size={16} />{/if}
          </span>
        </button>
        <div class="scope-separator" aria-hidden="true"></div>
      {/if}

      {#if organizationCandidates.length > 0}
        <p class="scope-group-label" aria-hidden="true">Organizations</p>
        {#each organizationCandidates as target (target.id)}
          {@render scopeOption(target)}
        {/each}
      {/if}

      {#if personalCandidates.length > 0}
        <p class="scope-group-label" aria-hidden="true">Personal</p>
        {#each personalCandidates as target (target.id)}
          {@render scopeOption(target)}
        {/each}
      {/if}

      {#if candidates.length === 0}
        <p class="scope-empty">No installations match “{query.trim()}”</p>
      {/if}
    </div>
  </div>
</details>

<style>
  .scope-picker {
    min-width: 0;
    position: relative;
  }

  .scope-picker[open] {
    z-index: var(--layer-popover);
  }

  summary {
    align-items: center;
    background: var(--control-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    display: flex;
    gap: 0.5rem;
    height: var(--local-control-height, var(--control-height));
    line-height: 1;
    max-width: 18rem;
    min-width: 11rem;
    padding: 0 0.625rem;
    transition:
      background-color var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out),
      transform var(--duration-press) var(--ease-out);
    user-select: none;
  }

  summary::-webkit-details-marker {
    display: none;
  }

  summary::marker {
    content: '';
  }

  summary:hover,
  .scope-picker[open] summary {
    background: var(--control-bg-hover);
    border-color: var(--control-border-hover);
  }

  summary:active {
    transform: translateY(1px);
  }

  .scope-field,
  .scope-field summary {
    width: 100%;
  }

  .scope-field summary {
    height: 2.75rem;
    max-width: none;
  }

  .scope-copy {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
  }

  .scope-copy strong {
    font-size: 0.75rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .scope-copy small {
    color: var(--dim);
    font: 0.5625rem/1.2 var(--mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .global-mark {
    align-items: center;
    background: var(--accent-tint);
    border: 1px solid color-mix(in srgb, var(--accent) 55%, transparent);
    border-radius: 50%;
    color: var(--accent);
    display: inline-flex;
    flex: none;
    height: 1.375rem;
    justify-content: center;
    width: 1.375rem;
  }

  .scope-chevron {
    color: var(--text-muted);
    display: grid;
    flex: none;
    place-items: center;
    transition: transform var(--duration-fast) var(--ease-standard);
  }

  .scope-picker[open] .scope-chevron {
    transform: rotate(180deg);
  }

  .scope-popover {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    overflow: hidden;
    position: absolute;
    right: 0;
    top: calc(100% + 0.35rem);
    width: min(22rem, calc(100vw - 2rem));
    z-index: var(--layer-popover);
  }

  .scope-field .scope-popover {
    left: 0;
    right: 0;
    width: 100%;
  }

  .scope-search {
    display: block;
    padding: 0.625rem;
    position: relative;
  }

  .scope-search .text-input {
    font-size: 0.75rem;
    padding-left: 2rem;
    width: 100%;
  }

  .search-icon {
    color: var(--text-muted);
    display: grid;
    left: 1.25rem;
    place-items: center;
    position: absolute;
    top: 1.28rem;
  }

  .scope-options {
    border-top: 1px solid var(--rule);
    max-height: min(23rem, 55vh);
    overflow: auto;
    padding: 0.3rem;
  }

  .scope-field .scope-options {
    max-height: min(17rem, 38vh);
  }

  .scope-option {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: calc(var(--r-ctl) - 2px);
    color: var(--text);
    display: flex;
    gap: 0.625rem;
    min-height: 2.75rem;
    padding: 0.45rem 0.55rem;
    text-align: left;
    transition:
      background-color var(--duration-fast) var(--ease-out),
      transform var(--duration-press) var(--ease-out);
    width: 100%;
  }

  .scope-option:hover,
  .scope-option:focus-visible {
    background: var(--interactive-hover);
  }

  .scope-option.selected {
    background: var(--brand-action-tint);
  }

  .scope-option:active {
    box-shadow: inset 0 0 0 100vmax var(--press);
    transform: translateY(1px);
  }

  .global-option {
    background: var(--brand-action-tint);
  }

  .option-copy {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
  }

  .option-copy strong {
    font-size: 0.75rem;
    line-height: 1.25;
  }

  .option-copy span {
    color: var(--dim);
    font: 0.625rem/1.35 var(--mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .option-check {
    color: var(--clear);
    font-weight: 700;
    width: 1rem;
  }

  .scope-separator {
    background: var(--rule);
    height: 1px;
    margin: 0.25rem 0.4rem;
  }

  .scope-group-label {
    background: var(--popover-bg);
    color: var(--dim);
    font: 650 var(--font-size-compact) / 1 var(--sans);
    letter-spacing: 0.04em;
    margin: 0;
    padding: 0.45rem 0.55rem 0.25rem;
    position: sticky;
    top: -0.3rem;
    text-transform: uppercase;
    z-index: 1;
  }

  .scope-empty {
    color: var(--dim);
    font-size: 0.75rem;
    margin: 0;
    padding: 0.875rem;
    text-align: center;
  }

  @media (prefers-reduced-motion: reduce) {
    summary:active,
    .scope-option:active {
      transform: none;
    }
  }
</style>
