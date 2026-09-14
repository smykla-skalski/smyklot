<script lang="ts">
  import { tick } from 'svelte';
  import { createQuery } from '@tanstack/svelte-query';
  import { PanelApiError } from '../api';
  import type { SyncRequestAcceptance, SyncRequestHistory } from '../types';
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import Link from './Link.svelte';
  import Modal from './Modal.svelte';
  import RelativeTime from './RelativeTime.svelte';
  import ResultProblem from './ResultProblem.svelte';

  const {
    actorId,
    targetId,
    nowMs,
    fetchRequests,
    requestHref,
    onOpenRequest,
    onReady,
  }: {
    actorId: string;
    targetId: string;
    nowMs: number;
    fetchRequests: (
      target: string,
      request: { limit: number; cursor?: string },
    ) => Promise<SyncRequestHistory>;
    requestHref: (action: 'check' | 'dispatch', key: string) => string;
    onOpenRequest: (action: 'check' | 'dispatch', key: string) => void;
    onReady?: (element: HTMLButtonElement) => void;
  } = $props();
  let open = $state(false);
  let trigger = $state<HTMLButtonElement | null>(null);
  let cursors = $state<(string | null)[]>([null]);
  const cursor = $derived(cursors.at(-1) ?? undefined);
  const query = createQuery(() => ({
    queryKey: ['sync-requests', actorId, targetId, cursor],
    queryFn: () => fetchRequests(targetId, { limit: 20, cursor }),
    enabled: open && actorId !== '',
    retry: false,
    staleTime: 0,
  }));
  const denied = $derived(
    query.error instanceof PanelApiError && [401, 403, 404].includes(query.error.status),
  );
  const staleCursor = $derived(
    query.error instanceof PanelApiError && query.error.code === 'invalid_request_history_query',
  );
  const page = $derived(query.error ? undefined : query.data);

  function refresh(): void {
    if (query.isFetching) return;
    if (staleCursor && cursors.length > 1) cursors = [null];
    else void query.refetch();
  }
  async function close(): Promise<void> {
    open = false;
    await tick();
    trigger?.focus();
  }
  async function inspect(event: MouseEvent, item: SyncRequestAcceptance): Promise<void> {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
      return;
    event.preventDefault();
    open = false;
    await tick();
    onOpenRequest(item.action, item.request_key);
  }
</script>

<!--
@component
The current actor's accepted requests in one workspace, with cursor pagination and links to their exact identities. This is historical acceptance, not proof of completion or a way to identify unconfirmed work by its time or reason. Closing restores focus to the entry button.
-->

<div class="requests-entry">
  <Button
    id="sync-requests-trigger"
    {@attach (element) => onReady?.(element as HTMLButtonElement)}
    tone="quiet"
    bind:element={trigger}
    onclick={() => (open = true)}>Your sync requests</Button
  >
</div>
<Modal
  id="sync-requests"
  {open}
  title="Your sync requests"
  description="Requests you made in this workspace. Acceptance does not mean the work finished."
  variant="wide"
  returnFocus={trigger}
  onClose={() => void close()}
>
  {#snippet headerExtra()}<Button tone="quiet" onclick={() => void close()}>Close requests</Button
    >{/snippet}
  <div class="requests-content">
    <div class="requests-tools">
      <p>
        Opening details does not start work. Similar times or reasons cannot identify an unconfirmed
        request.
      </p>
      <Button onclick={refresh} aria-disabled={query.isFetching}>
        {staleCursor ? 'Latest requests' : 'Refresh requests'}
      </Button>
    </div>
    {#if query.error}
      <ResultProblem
        title={denied ? 'Your request history is unavailable' : 'Requests could not be loaded'}
        problem={denied
          ? 'Sign in and check your workspace access before refreshing this history.'
          : staleCursor
            ? 'This page can no longer be continued. Open the latest requests to start again.'
            : 'Refresh requests to try this read again. Refreshing does not start sync.'}
      />
    {:else if query.isPending}
      <p role="status">Loading your accepted requests…</p>
    {:else if page?.items.length === 0}
      <EmptyState
        title="No accepted requests found"
        description="No requests made by your account are recorded here. This does not prove that an unconfirmed request failed."
      />
    {:else if page}
      <p class="requests-count" role="status">
        {`Page ${cursors.length} · ${page.items.length} accepted ${page.items.length === 1 ? 'request' : 'requests'}`}
      </p>
      <ol class="request-list" aria-label="Your accepted sync requests">
        {#each page.items as item (JSON.stringify([item.action, item.request_key]))}
          <li>
            <div class="request-details">
              <h3>{item.action === 'check' ? 'Check repositories' : 'Run prepared changes'}</h3>
              <p class="request-time">
                Accepted <RelativeTime value={item.accepted_at} {nowMs} exact />
              </p>
              <p class="request-reason">{item.reason}</p>
            </div>
            <Link
              href={requestHref(item.action, item.request_key)}
              onclick={(event) => void inspect(event, item)}>View request</Link
            >
          </li>
        {/each}
      </ol>
    {/if}
    {#if !staleCursor && (cursors.length > 1 || page?.next_cursor)}
      <nav class="requests-navigation" aria-label="Accepted request pages">
        <Button
          aria-disabled={cursors.length === 1 || query.isFetching}
          onclick={() => {
            if (cursors.length > 1 && !query.isFetching) cursors = cursors.slice(0, -1);
          }}>Newer requests</Button
        >
        <Button
          aria-disabled={!page?.next_cursor || query.isFetching || !!query.error}
          onclick={() => {
            if (page?.next_cursor && !query.isFetching && !query.error)
              cursors = [...cursors, page.next_cursor];
          }}>Older requests</Button
        >
      </nav>
    {/if}
  </div>
</Modal>

<style>
  .requests-entry {
    display: flex;
    justify-content: flex-end;
  }
  .requests-content {
    display: grid;
    gap: var(--space-4);
  }
  .requests-tools {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
  }
  .requests-tools p {
    margin: 0;
    color: var(--text-secondary);
    max-width: 64ch;
  }
  .requests-tools :global(.btn) {
    flex-shrink: 0;
  }
  .requests-count {
    margin: 0;
    color: var(--text-secondary);
  }
  .request-list {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .request-list li {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-5);
    padding-block: var(--space-4);
    border-top: 1px solid var(--border-subtle);
  }
  .request-list li :global(a) {
    flex-shrink: 0;
  }
  .request-details {
    min-width: 0;
  }
  .request-details h3 {
    margin: 0;
    font-size: var(--font-size-body);
  }
  .request-details p {
    margin: var(--space-2) 0 0;
  }
  .request-time {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
  }
  .request-reason {
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }
  .requests-navigation {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
  }
  .requests-content :global([aria-disabled='true']) {
    opacity: 0.5;
    cursor: default;
  }
</style>
