<script lang="ts">
  import { paginationItems } from '../lib/pagination';

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
    <span aria-hidden="true">←</span>
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
    <span aria-hidden="true">→</span>
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
    background: transparent;
    border: 1px solid transparent;
    color: var(--dim);
    font-size: 0.6875rem;
    font-weight: 600;
    height: 1.875rem;
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
    transform: translateY(1px);
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
    border-radius: 6px;
    min-width: 1.875rem;
    padding: 0 0.3rem;
  }

  .page-number.current {
    background: var(--signal-tint);
    border-color: color-mix(in srgb, var(--signal) 48%, transparent);
    color: var(--signal);
    cursor: default;
    opacity: 1;
  }

  .ellipsis {
    color: var(--dim);
    display: grid;
    font: 600 0.6875rem/1 var(--mono);
    height: 1.875rem;
    min-width: 1.25rem;
    place-items: center;
  }

  @media (max-width: 30rem) {
    .step-label {
      display: none;
    }

    .page-step {
      min-width: 1.875rem;
      padding: 0;
    }
  }
</style>
