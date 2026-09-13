<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { PanelApiError, type PanelApi } from '../api';
  import { CHECK_OBSERVATION_LABELS } from '../sync-check';
  import { SYNC_SECTION_LABELS } from '../routes';
  import { formatDateTime } from '../format';
  import Button from './Button.svelte';
  import CursorPaginationBar from './CursorPaginationBar.svelte';
  import Link from './Link.svelte';

  const {
    api,
    targetId,
    checkId,
  }: {
    api: Pick<PanelApi, 'fetchSyncCheckObservations'>;
    targetId: string;
    checkId: string;
  } = $props();
  const client = useQueryClient();
  type Navigation = { cursors: (string | null)[]; pageSize: number };
  const navigationKey = untrack(() => ['sync-check-evidence-navigation', targetId, checkId]);
  let navigation = $state<Navigation>(
    untrack(() => client.getQueryData<Navigation>(navigationKey)) ?? {
      cursors: [null],
      pageSize: 10,
    },
  );
  const cursor = $derived(navigation.cursors.at(-1) ?? undefined);
  const query = createQuery(() => ({
    queryKey: ['sync-check-evidence', targetId, checkId, navigation.pageSize, cursor],
    queryFn: () =>
      api.fetchSyncCheckObservations(targetId, checkId, { limit: navigation.pageSize, cursor }),
    retry: false,
    staleTime: 0,
    refetchInterval: false,
  }));
  const denied = $derived(
    query.error instanceof PanelApiError && [401, 403, 404].includes(query.error.status),
  );
  const page = $derived(denied ? undefined : query.data);
  let heading: HTMLHeadingElement | undefined = $state();
  async function navigate(next: Navigation) {
    navigation = next;
    client.setQueryData(navigationKey, next);
    await tick();
    heading?.focus();
  }
</script>

<!--
@component
Paged evidence from one completed check, preserving historical names and observation
times. Mount keyed by workspace and check so navigation cannot cross subjects. Visited
boundaries survive reopening in the shared query cache. Access failures hide cached
rows; transient failures offer retry. SyncCheckSummary provides the outcome above it.
-->
<section class="evidence" aria-label="Repository evidence">
  <h3 bind:this={heading} tabindex="-1">Repository evidence</h3>
  <p class="explanation">
    Names and observations are preserved as they were at this check. Reused observations were not
    checked again.
  </p>
  {#if query.error}
    <div role="alert" class="problem">
      <p>
        {denied
          ? 'The evidence is no longer available, or you no longer have access to it.'
          : 'Repository evidence could not be loaded. Try again.'}
      </p>
      {#if !denied}<Button disabled={query.isFetching} onclick={() => void query.refetch()}
          >Try again</Button
        >{/if}
      {#if navigation.cursors.length > 1}<Button
          tone="quiet"
          onclick={() => navigate({ ...navigation, cursors: navigation.cursors.slice(0, -1) })}
          >Previous page</Button
        >{/if}
    </div>
  {/if}
  {#if page}
    {#if page.items.length === 0}
      <p>No repository comparisons were recorded for this check. This does not confirm a match.</p>
    {:else}
      <ol class="observations">
        {#each page.items as observation (`${observation.repository_id}:${observation.kind}`)}
          <li>
            <div class="subject">
              <strong>{observation.repository}</strong><span
                >{SYNC_SECTION_LABELS[observation.kind]}</span
              >
            </div>
            <p>
              {CHECK_OBSERVATION_LABELS[observation.outcome] ?? 'Not confirmed'}{observation.cached
                ? ' · Reused observation'
                : ''}
            </p>
            <p class="observed">
              Observed <time datetime={observation.observed_at}
                >{formatDateTime(observation.observed_at, { named: true, seconds: true })}</time
              >
            </p>
            {#if observation.reason}<p>{observation.reason}</p>{/if}
            {#if observation.proposal_url}<Link
                href={observation.proposal_url}
                target="_blank"
                rel="noopener noreferrer">View proposal on GitHub (opens a new tab)</Link
              >{/if}
          </li>
        {/each}
      </ol>
    {/if}
    {#if page.total > 0}<CursorPaginationBar
        label="Repository evidence"
        count={page.items.length}
        total={page.total}
        pageSize={navigation.pageSize}
        canPrevious={navigation.cursors.length > 1}
        canNext={page.next_cursor !== null}
        busy={query.isFetching}
        onPrevious={() => navigate({ ...navigation, cursors: navigation.cursors.slice(0, -1) })}
        onNext={() => {
          if (page?.next_cursor)
            navigate({ ...navigation, cursors: [...navigation.cursors, page.next_cursor] });
        }}
        onPageSizeSelect={(pageSize) => navigate({ cursors: [null], pageSize })}
      />{/if}
  {:else if !query.error}<p role="status">Loading repository evidence…</p>{/if}
</section>

<style>
  .evidence {
    display: grid;
    gap: var(--space-3);
    border-top: 1px solid var(--border-subtle);
    padding-top: var(--space-4);
  }
  h3,
  p {
    margin: 0;
  }
  h3 {
    font-size: var(--font-size-body);
  }
  p,
  .subject {
    font-size: var(--font-size-meta);
    line-height: var(--leading-meta);
  }
  .explanation,
  .observed {
    color: var(--text-muted);
  }
  .observations {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  li {
    display: grid;
    gap: var(--space-1);
    padding-block: var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    overflow-wrap: anywhere;
  }
  li:first-child {
    padding-top: 0;
  }
  li:last-child {
    border-bottom: 0;
  }
  .subject {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
  }
  .subject span {
    color: var(--text-muted);
    flex-shrink: 0;
  }
  .problem {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }
  .problem p {
    flex-basis: 100%;
  }
</style>
