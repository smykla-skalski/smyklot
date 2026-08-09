<script lang="ts">
  import PageNavigation from './PageNavigation.svelte';
  import PageSizeSelect from './PageSizeSelect.svelte';

  const {
    label,
    pageIndex,
    pageCount,
    pageSize,
    itemCount,
    total,
    disabled = false,
    onPageSelect,
    onPageSizeSelect,
  }: {
    label: string;
    pageIndex: number;
    pageCount: number;
    pageSize: number;
    itemCount: number;
    total: number;
    disabled?: boolean;
    onPageSelect: (pageIndex: number) => void;
    onPageSizeSelect: (pageSize: number) => void;
  } = $props();

  const rangeStart = $derived(total === 0 ? 0 : pageIndex * pageSize + 1);
  const rangeEnd = $derived(total === 0 ? 0 : rangeStart + itemCount - 1);
</script>

<footer class="pagination-bar" aria-label={`${label} pagination`}>
  <p class="pagination-range mono" aria-live="polite">
    <strong>{rangeStart}–{rangeEnd}</strong>
    of {total}
  </p>

  <div class="pagination-pages">
    <PageNavigation {pageIndex} {pageCount} {disabled} onSelect={onPageSelect} />
  </div>

  <div class="pagination-size">
    <PageSizeSelect
      value={pageSize}
      label={`${label} per page below results`}
      onSelect={onPageSizeSelect}
    />
  </div>
</footer>

<style>
  .pagination-bar {
    --local-control-height: var(--control-height-compact);

    align-items: center;
    background: var(--surface-inset);
    border-top: 1px solid var(--rule);
    display: grid;
    gap: var(--space-2);
    grid-template-columns: minmax(0, 1fr) auto auto;
    min-height: var(--pagination-height);
    padding: var(--space-3) var(--space-4);
  }

  .pagination-range {
    color: var(--dim);
    font-size: var(--font-size-micro);
    margin: 0;
    white-space: nowrap;
  }

  .pagination-range strong {
    color: var(--text);
    font-weight: 600;
  }

  .pagination-pages {
    min-width: 0;
  }

  .pagination-size {
    display: flex;
  }

  @media (max-width: 36rem) {
    .pagination-bar {
      grid-template-columns: 1fr auto;
    }

    .pagination-pages {
      grid-column: 1 / -1;
      grid-row: 1;
    }

    .pagination-range {
      grid-column: 1;
      grid-row: 2;
    }

    .pagination-size {
      grid-column: 2;
      grid-row: 2;
    }
  }
</style>
