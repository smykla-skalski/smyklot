<script lang="ts">
  import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query';
  import type { PanelApi } from '../api';
  import { formatRelative, formatTimestamp } from '../format';
  import type { PendingCIDetail, PendingCIRequest, RootOverview } from '../types';
  import Chip from './Chip.svelte';
  import Icon from './Icon.svelte';
  import PendingCIAudit from './PendingCIAudit.svelte';

  const {
    api,
    queue,
    now,
  }: {
    api: PanelApi;
    queue: RootOverview['pending_ci'];
    now: number;
  } = $props();

  let selectedRequest = $state<string | null>(null);
  const queryClient = useQueryClient();
  const total = $derived(queue.active.length + queue.deferred.length);

  function actionKey(action: 'check' | 'cancel', request: PendingCIRequest): string {
    return `${action}:${request.id}`;
  }

  const auditQuery = createQuery(() => ({
    queryKey: ['root-pending-ci', selectedRequest],
    queryFn: () => {
      if (selectedRequest === null) throw new Error('select a pending CI request first');
      return api.fetchRootPendingCI(selectedRequest);
    },
    enabled: selectedRequest !== null,
  }));
  const auditDetail = $derived<PendingCIDetail | null>(
    selectedRequest === null ? null : (auditQuery.data ?? null),
  );
  const auditLoading = $derived(selectedRequest !== null && auditQuery.isFetching);
  const auditError = $derived(
    selectedRequest === null || auditQuery.error === null
      ? null
      : auditQuery.error instanceof Error
        ? auditQuery.error.message
        : String(auditQuery.error),
  );

  const actionMutation = createMutation(() => ({
    mutationFn: ({ action, request }: { action: 'check' | 'cancel'; request: PendingCIRequest }) =>
      action === 'check'
        ? api.checkRootPendingCI(request.id, request.revision)
        : api.cancelRootPendingCI(request.id, request.revision),
    onSuccess: async (_updated, { request }) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['root-overview'] }),
        queryClient.invalidateQueries({ queryKey: ['root-pending-ci', request.id] }),
      ]);
    },
  }));
  const pendingAction = $derived(
    actionMutation.isPending && actionMutation.variables !== undefined
      ? actionKey(actionMutation.variables.action, actionMutation.variables.request)
      : null,
  );
  const failure = $derived(
    actionMutation.error === null
      ? null
      : actionMutation.error instanceof Error
        ? actionMutation.error.message
        : String(actionMutation.error),
  );

  function checkNow(request: PendingCIRequest): void {
    actionMutation.mutate({ action: 'check', request });
  }

  function cancel(request: PendingCIRequest): void {
    actionMutation.mutate({ action: 'cancel', request });
  }

  function stateLabel(request: PendingCIRequest): string {
    if (request.last_observed_state === '') return 'Awaiting first check';
    return request.last_observed_state.replaceAll('_', ' ');
  }

  function methodLabel(request: PendingCIRequest): string {
    return `${request.merge_method}${request.required_checks_only ? ' · required checks' : ''}`;
  }

  function triggerLabel(request: PendingCIRequest): string {
    switch (request.next_check_trigger) {
      case 'webhook':
        return 'Webhook reconciliation';
      case 'quiet_period':
        return 'Quiet-period verification';
      case 'manual':
        return 'Panel reconciliation';
      case 'cleanup':
        return 'Cleanup';
      case 'command':
        return 'Initial reconciliation';
      case 'fallback':
        return 'Fallback reconciliation';
    }
  }

  function toggleAudit(request: PendingCIRequest): void {
    if (selectedRequest === request.id) {
      selectedRequest = null;
      return;
    }
    selectedRequest = request.id;
  }
</script>

<article class="pending-ci-panel">
  <header>
    <div>
      <h3>Pending CI merges</h3>
      <p>{total === 0 ? 'No merge requests are waiting' : `${total} armed merge requests`}</p>
    </div>
    <div class="queue-counts" aria-label="Pending CI queue counts">
      <Chip tone="accent">{queue.active.length} Active</Chip>
      <Chip tone="neutral">{queue.deferred.length} Deferred</Chip>
    </div>
  </header>

  {#if failure !== null}
    <p class="queue-error" role="alert"><Icon name="failure" size={14} /> {failure}</p>
  {/if}

  {#if total === 0}
    <div class="empty-queue">
      <span><Icon name="success" size={20} /></span>
      <div>
        <strong>No pending CI work</strong>
        <p>New “after CI” commands will appear here.</p>
      </div>
    </div>
  {:else}
    {#snippet queueSection(title: string, items: PendingCIRequest[], deferred: boolean)}
      <section class="queue-section" aria-label={`${title} pending CI requests`}>
        <div class="queue-heading">
          <h4>{title}</h4>
          <p>
            {deferred ? 'Six-hour safety fallback' : 'Webhook-driven with five-minute fallback'}
          </p>
        </div>
        <div class="queue-list">
          {#each items as request (request.id)}
            <article class="queue-item">
              <div class="request-main">
                <a
                  href={`https://github.com/${request.repository_full_name}/pull/${request.pull_request}`}
                  target="_blank"
                  rel="noreferrer"
                >
                  {request.repository_full_name}#{request.pull_request}
                  <Icon name="link" size={13} />
                </a>
                <div class="request-meta">
                  <code>{methodLabel(request)}</code>
                  <span aria-hidden="true">·</span>
                  <span>{stateLabel(request)}</span>
                  <span aria-hidden="true">·</span>
                  <span>by @{request.requester}</span>
                </div>
                <div class="request-timing">
                  <span>
                    {triggerLabel(request)}
                    <time
                      datetime={request.next_check_at}
                      title={formatTimestamp(request.next_check_at)}
                      >{formatRelative(request.next_check_at, now)}</time
                    >
                  </span>
                  <code title={request.head_sha}>{request.head_sha.slice(0, 8)}</code>
                </div>
              </div>
              <div class="request-actions">
                <button
                  class="btn btn-quiet"
                  type="button"
                  aria-expanded={selectedRequest === request.id}
                  onclick={() => toggleAudit(request)}
                >
                  <Icon name="history" size={14} />
                  Timeline
                </button>
                <button
                  class="btn btn-quiet"
                  type="button"
                  disabled={pendingAction !== null}
                  onclick={() => checkNow(request)}
                >
                  <Icon name="refresh" size={14} />
                  {pendingAction === actionKey('check', request) ? 'Checking…' : 'Check now'}
                </button>
                <button
                  class="btn btn-stop"
                  type="button"
                  disabled={pendingAction !== null}
                  onclick={() => cancel(request)}
                >
                  <Icon name="close" size={14} />
                  {pendingAction === actionKey('cancel', request) ? 'Cancelling…' : 'Cancel'}
                </button>
              </div>
              {#if selectedRequest === request.id}
                <PendingCIAudit detail={auditDetail} loading={auditLoading} error={auditError} />
              {/if}
            </article>
          {:else}
            <p class="queue-empty">No {title.toLowerCase()} requests</p>
          {/each}
        </div>
      </section>
    {/snippet}

    <div class="queue-columns">
      {@render queueSection('Active', queue.active, false)}
      {@render queueSection('Deferred', queue.deferred, true)}
    </div>
  {/if}

  <section class="recent-section" aria-label="Recent pending CI requests">
    <div class="queue-heading">
      <h4>Recent outcomes</h4>
      <p>Durable request history and webhook causality</p>
    </div>
    <div class="queue-list">
      {#each queue.recent as request (request.id)}
        <article class="queue-item recent-item">
          <div class="request-main">
            <a
              href={`https://github.com/${request.repository_full_name}/pull/${request.pull_request}`}
              target="_blank"
              rel="noreferrer"
            >
              {request.repository_full_name}#{request.pull_request}
              <Icon name="link" size={13} />
            </a>
            <div class="request-meta">
              <code>{methodLabel(request)}</code>
              <span aria-hidden="true">·</span>
              <span>{request.lifecycle}</span>
              {#if request.reason}
                <span aria-hidden="true">·</span>
                <span>{request.reason}</span>
              {/if}
            </div>
            {#if request.finished_at}
              <div class="request-timing">
                <span>
                  Finished
                  <time datetime={request.finished_at} title={formatTimestamp(request.finished_at)}>
                    {formatRelative(request.finished_at, now)}
                  </time>
                </span>
              </div>
            {/if}
          </div>
          <div class="request-actions">
            <button
              class="btn btn-quiet"
              type="button"
              aria-expanded={selectedRequest === request.id}
              onclick={() => toggleAudit(request)}
            >
              <Icon name="history" size={14} />
              Timeline
            </button>
          </div>
          {#if selectedRequest === request.id}
            <PendingCIAudit detail={auditDetail} loading={auditLoading} error={auditError} />
          {/if}
        </article>
      {:else}
        <p class="queue-empty">No completed requests yet</p>
      {/each}
    </div>
  </section>
</article>

<style>
  .pending-ci-panel {
    background: var(--surface-base);
    border: 1px solid color-mix(in srgb, var(--brand-action) 13%, var(--border-subtle));
    border-radius: var(--radius-surface);
    box-shadow: var(--shadow-plate);
    padding: var(--space-5);
  }

  .pending-ci-panel > header,
  .queue-counts,
  .request-meta,
  .request-timing,
  .request-actions,
  .empty-queue,
  .queue-error {
    align-items: center;
    display: flex;
  }

  .pending-ci-panel > header {
    justify-content: space-between;
  }

  h3,
  h4,
  p {
    margin: 0;
  }

  h3 {
    font-size: var(--font-size-title);
    letter-spacing: -0.025em;
  }

  header p,
  .queue-heading p,
  .request-timing,
  .empty-queue p {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
  }

  .queue-counts,
  .request-actions {
    gap: var(--space-2);
  }

  .queue-columns {
    display: grid;
    gap: var(--space-4);
    grid-template-columns: repeat(2, minmax(0, 1fr));
    margin-top: var(--space-5);
  }

  .recent-section {
    margin-top: var(--space-5);
  }

  .recent-item {
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .queue-heading {
    border-bottom: 1px solid var(--border-subtle);
    padding-bottom: var(--space-2);
  }

  .queue-heading h4 {
    font-size: var(--font-size-meta);
  }

  .queue-list {
    display: grid;
  }

  .queue-item {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: minmax(0, 1fr) auto;
    padding: var(--space-3) 0;
  }

  .queue-item + .queue-item {
    border-top: 1px solid var(--border-subtle);
  }

  .request-main {
    min-width: 0;
  }

  .request-main > a {
    align-items: center;
    color: var(--text-primary);
    display: inline-flex;
    font: 650 var(--font-size-meta) / 1.4 var(--sans);
    gap: 0.3rem;
    max-width: 100%;
    text-decoration: none;
  }

  .request-main > a:hover {
    color: var(--accent);
  }

  .request-meta,
  .request-timing {
    flex-wrap: wrap;
    gap: 0.35rem;
    margin-top: 0.25rem;
  }

  .request-meta {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
  }

  .request-meta code,
  .request-timing code {
    color: inherit;
    font-size: inherit;
  }

  .request-timing {
    justify-content: space-between;
  }

  .request-actions {
    align-self: center;
  }

  .request-actions .btn {
    min-height: 2rem;
    padding: 0.35rem 0.55rem;
  }

  .queue-error {
    background: var(--stop-tint);
    border-radius: var(--radius-control);
    color: var(--stop);
    gap: var(--space-2);
    margin-top: var(--space-3);
    padding: var(--space-2) var(--space-3);
  }

  .empty-queue {
    gap: var(--space-3);
    justify-content: center;
    min-height: 7rem;
  }

  .empty-queue > span {
    align-items: center;
    background: var(--accent-tint);
    border-radius: var(--radius-control);
    color: var(--accent);
    display: inline-flex;
    height: 2rem;
    justify-content: center;
    width: 2rem;
  }

  .queue-empty {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    padding: var(--space-4) 0;
  }

  @media (max-width: 72rem) {
    .queue-columns {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 48rem) {
    .pending-ci-panel > header,
    .queue-item {
      align-items: stretch;
      grid-template-columns: 1fr;
    }

    .pending-ci-panel > header {
      display: grid;
      gap: var(--space-3);
    }

    .request-actions .btn {
      flex: 1;
    }
  }
</style>
