<!--
@component
Forward and backward navigation for a cursor-backed list. The parent retains visited
boundaries and resets them when page size changes. A live total is shown as a total,
not as an invented page count or an offset that can shift when new rows arrive.
Loading disables navigation and page-size changes. The count is announced politely.
-->

<script lang="ts">
  import Button from './Button.svelte';
  import PageSizeSelect from './PageSizeSelect.svelte';

  const {
    label,
    count,
    total,
    pageSize,
    canPrevious,
    canNext,
    busy = false,
    onPrevious,
    onNext,
    onPageSizeSelect,
  }: {
    label: string;
    count: number;
    total: number;
    pageSize: number;
    canPrevious: boolean;
    canNext: boolean;
    busy?: boolean;
    onPrevious: () => void;
    onNext: () => void;
    onPageSizeSelect: (size: number) => void;
  } = $props();
</script>

<footer class="cursor-pagination" aria-label={`${label} pagination`}>
  <p aria-live="polite">{count} shown · {total} total</p>
  <div class="moves">
    <Button tone="quiet" disabled={busy || !canPrevious} onclick={onPrevious}>Previous</Button>
    <Button tone="quiet" disabled={busy || !canNext} onclick={onNext}>Next</Button>
  </div>
  <PageSizeSelect
    disabled={busy}
    value={pageSize}
    label={`${label} per page`}
    onSelect={onPageSizeSelect}
  />
</footer>

<style>
  .cursor-pagination {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    min-height: var(--pagination-height);
    padding: var(--space-1) var(--space-4);
    border-top: 1px solid var(--border-subtle);
    background: var(--pagination-bg);
  }
  p {
    margin: 0;
    color: var(--text-muted);
    font-size: var(--font-size-micro);
  }
  .moves {
    display: flex;
    gap: var(--space-2);
  }
</style>
