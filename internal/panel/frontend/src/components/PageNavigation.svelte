<script lang="ts">
  import { paginationItems } from '../lib/pagination';
  import Icon from './Icon.svelte';

  const {
    pageIndex,
    pageCount,
    disabled = false,
    onSelect,
  }: {
    pageIndex: number;
    pageCount: number;
    disabled?: boolean;
    onSelect: (pageIndex: number) => void;
  } = $props();

  const currentPage = $derived(pageIndex + 1);
  const items = $derived(paginationItems(currentPage, pageCount));
</script>

<div class="page-navigation" role="group" aria-label="Pages">
  <button
    class="page-step"
    disabled={disabled || pageIndex === 0}
    aria-label="Previous page"
    onclick={() => onSelect(pageIndex - 1)}
  >
    <Icon name="chevron-left" size={16} />
    <span class="step-label">Previous</span>
  </button>

  <div class="page-numbers">
    {#each items as item, index (`${item}-${index}`)}
      {#if typeof item === 'number'}
        <button
          class="page-number"
          class:current={item === currentPage}
          disabled={disabled || item === currentPage}
          aria-label={`Page ${item}`}
          aria-current={item === currentPage ? 'page' : undefined}
          onclick={() => onSelect(item - 1)}
        >
          {item}
        </button>
      {:else}
        <span class="ellipsis" aria-hidden="true">…</span>
      {/if}
    {/each}
  </div>

  <button
    class="page-step"
    disabled={disabled || pageIndex >= pageCount - 1}
    aria-label="Next page"
    onclick={() => onSelect(pageIndex + 1)}
  >
    <span class="step-label">Next</span>
    <Icon name="chevron-right" size={16} />
  </button>
</div>

<style>
  .page-navigation,
  .page-numbers {
    align-items: center;
    display: flex;
  }

  .page-navigation {
    gap: 0.375rem;
  }

  .page-numbers {
    gap: 0.125rem;
  }

  button {
    align-items: center;
    background: transparent;
    border: 1px solid transparent;
    box-sizing: border-box;
    color: var(--dim);
    display: inline-flex;
    font-size: var(--font-size-compact);
    font-weight: 600;
    height: var(--local-control-height, var(--control-height-compact));
    justify-content: center;
    line-height: 1;
    max-height: var(--local-control-height, var(--control-height-compact));
    min-height: var(--local-control-height, var(--control-height-compact));
    transition:
      background-color 120ms ease-out,
      border-color 120ms ease-out,
      color 120ms ease-out,
      transform 80ms ease-out;
  }

  button:hover:not(:disabled):not(.current) {
    background: var(--strip-lift);
    border-color: var(--control-border);
    color: var(--text);
  }

  button:active:not(:disabled) {
    box-shadow: inset 0 0 0 100vmax var(--press);
  }

  button:disabled {
    cursor: default;
    opacity: 0.42;
  }

  .page-step {
    align-items: center;
    border-color: var(--control-border);
    border-radius: var(--r-ctl);
    display: inline-flex;
    gap: 0.3rem;
    justify-content: center;
    padding: 0 0.5rem;
  }

  .page-number {
    background: var(--control-bg);
    border-color: var(--control-border);
    border-radius: 6px;
    min-width: var(--local-control-height, var(--control-height-compact));
    padding: 0 0.3rem;
  }

  .page-number.current {
    background: var(--brand-action);
    border-color: var(--brand-action);
    color: var(--on-brand-action);
    cursor: default;
    opacity: 1;
  }

  .ellipsis {
    color: var(--dim);
    display: grid;
    font: 600 var(--font-size-compact) / 1 var(--sans);
    height: var(--local-control-height, var(--control-height-compact));
    min-width: 1.25rem;
    place-items: center;
  }

  @media (max-width: 30rem) {
    .step-label {
      display: none;
    }

    .page-step {
      min-width: var(--local-control-height, var(--control-height-compact));
      padding: 0;
    }
  }
</style>
