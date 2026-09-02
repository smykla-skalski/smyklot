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

<!--
@component
The count and the move, under a collection whose end is known. It is a family of its
own rather than a card's footer: the footer band belongs to the card and carries the
card's distance, and this pair goes wherever it is put.

Where it does NOT go is under a table that loads on a cursor. Nothing there has counted
the rows, so there is no total to say and no page to turn - a bar that reported "of 29"
against a cursor would be reporting a number nobody measured. Those tables end where
their data ends and say so in their own body.

The range is announced politely as it changes, because a reader who pressed Next needs
to hear where they landed and the rows themselves will not say it.
-->

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
    background: var(--pagination-bg);
    border-top: 1px solid var(--border-subtle);
    display: grid;
    gap: var(--space-2);
    grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
    min-height: var(--pagination-height);
    padding: var(--space-1) var(--space-4);
  }

  .pagination-range {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    justify-self: start;
    margin: 0;
    white-space: nowrap;
  }

  .pagination-range strong {
    color: var(--text-primary);
    font-weight: 600;
  }

  .pagination-pages {
    justify-self: center;
    min-width: 0;
  }

  .pagination-size {
    display: flex;
    justify-self: end;
  }

  @media (max-width: 36rem) {
    .pagination-bar {
      grid-template-columns: 1fr auto;
      padding-block: var(--space-2);
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
