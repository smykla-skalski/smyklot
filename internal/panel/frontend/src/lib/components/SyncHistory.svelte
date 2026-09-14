<!--
@component
Workspace-scoped prepared changes and their retained results. The list fetches bounded
summaries and opens result details through a stable address. Visited cursor boundaries
and page size survive inspector navigation in the workspace query cache. Unknown
states remain explicit, request failures provide retry, and empty results are distinct
from unavailable data. Completion describes processing, not a proposal being merged.
-->

<script lang="ts">
  import { untrack } from 'svelte';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import type { Page, SyncPlanSummary } from '../types';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import CursorPaginationBar from './CursorPaginationBar.svelte';
  import EmptyState from './EmptyState.svelte';
  import Link from './Link.svelte';
  import PageHeader from './PageHeader.svelte';
  import RelativeTime from './RelativeTime.svelte';
  import ResultProblem from './ResultProblem.svelte';

  const {
    targetId,
    nowMs,
    fetchHistory,
    resultHref,
    onOpenResult,
    onStatus,
  }: {
    targetId: string;
    nowMs: number;
    fetchHistory: (
      targetId: string,
      request: { limit: number; cursor?: string },
    ) => Promise<Page<SyncPlanSummary>>;
    resultHref: (id: string) => string;
    onOpenResult: (id: string) => void;
    onStatus: () => void;
  } = $props();
  const client = useQueryClient();
  type Navigation = { cursors: (string | null)[]; pageSize: number };
  const navigationKey = untrack(() => ['sync-history-navigation', targetId]);
  let navigation = $state<Navigation>(
    untrack(() => client.getQueryData<Navigation>(navigationKey)) ?? {
      cursors: [null],
      pageSize: 20,
    },
  );
  const cursor = $derived(navigation.cursors.at(-1) ?? undefined);
  const query = createQuery(() => ({
    queryKey: ['sync-history', targetId, navigation.pageSize, cursor],
    queryFn: () => fetchHistory(targetId, { limit: navigation.pageSize, cursor }),
  }));
  const page = $derived(query.data);
  const words: Record<SyncPlanSummary['state'], string> = {
    computed: 'Needs review',
    approved: 'Queued',
    applying: 'In progress',
    applied: 'Completed',
    failed: 'Needs attention',
    stale: 'No longer current',
    expired: 'Expired',
    discarded: 'Declined',
  };
  function navigate(next: Navigation): void {
    navigation = next;
    client.setQueryData(navigationKey, next);
  }
  function next(): void {
    if (page?.next_cursor)
      navigate({ ...navigation, cursors: [...navigation.cursors, page.next_cursor] });
  }
</script>

<section class="view-frame" aria-labelledby="sync-history-heading">
  <PageHeader
    id="sync-history-heading"
    section="Sync"
    title="Sync history"
    description="Prepared changes and their results. Shared-file proposals may still need merging."
  >
    {#snippet actions()}<Button tone="quiet" onclick={onStatus}>Sync status</Button>{/snippet}
  </PageHeader>
  {#if query.error}<ResultProblem
      title="Sync history could not be loaded"
      problem={query.error.message}
      busy={query.isFetching}
      overContent={page !== undefined}
      onRetry={() => void query.refetch()}
    />{/if}
  {#if query.error && navigation.cursors.length > 1}<Button
      tone="quiet"
      onclick={() => navigate({ ...navigation, cursors: navigation.cursors.slice(0, -1) })}
      >Previous page</Button
    >{/if}
  {#if page !== undefined}
    <Card>
      {#if page.items.length === 0}
        <EmptyState
          title="No sync results here"
          description="Prepared changes appear here when sync finds work to do."
          actionLabel="View sync status"
          onAction={onStatus}
        />
      {:else}
        <table aria-label="Sync history">
          <thead
            ><tr
              ><th scope="col">Prepared</th><th scope="col">Changes</th><th scope="col">Status</th
              ><th scope="col">Finished</th><th scope="col">Details</th></tr
            ></thead
          >
          <tbody
            >{#each page.items as plan (plan.id)}<tr>
                <td><RelativeTime value={plan.computed_at} {nowMs} /></td>
                <td>{plan.counts.create + plan.counts.update + plan.counts.delete}</td>
                <td>{words[plan.state] ?? 'Unknown'}</td>
                <td
                  >{#if plan.finished_at}<RelativeTime
                      value={plan.finished_at}
                      {nowMs}
                    />{:else}—{/if}</td
                >
                <td
                  ><Link
                    href={resultHref(plan.id)}
                    onclick={(event) => {
                      if (
                        event.button === 0 &&
                        !event.metaKey &&
                        !event.ctrlKey &&
                        !event.shiftKey &&
                        !event.altKey
                      ) {
                        event.preventDefault();
                        onOpenResult(plan.id);
                      }
                    }}>View result</Link
                  ></td
                >
              </tr>{/each}</tbody
          >
        </table>
      {/if}
      <CursorPaginationBar
        label="Sync history"
        count={page.items.length}
        total={page.total}
        pageSize={navigation.pageSize}
        canPrevious={navigation.cursors.length > 1}
        canNext={page.next_cursor !== null}
        busy={query.isFetching}
        onPrevious={() => navigate({ ...navigation, cursors: navigation.cursors.slice(0, -1) })}
        onNext={next}
        onPageSizeSelect={(pageSize) => navigate({ cursors: [null], pageSize })}
      />
    </Card>
  {:else if !query.error}<p role="status">Loading sync history…</p>{/if}
</section>

<style>
  table {
    width: 100%;
    border-collapse: collapse;
    table-layout: fixed;
    text-align: left;
    font-size: var(--font-size-meta);
  }
  th {
    color: var(--text-muted);
    font-weight: 500;
  }
  th,
  td {
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
  }
  tbody tr:last-child td {
    border-bottom: 0;
  }
</style>
