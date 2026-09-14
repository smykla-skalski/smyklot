<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { PanelApiError, type PanelApi } from '../api';
  import type { SyncCheckResponse } from '../types';
  import { syncCheckGuidance } from '../sync-check-guidance';
  import { words } from '../queue-words';
  import Modal from './Modal.svelte';
  import RelativeTime from './RelativeTime.svelte';
  import Button from './Button.svelte';
  import Link from './Link.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import SyncCheckSummary from './SyncCheckSummary.svelte';
  import SyncCheckEvidence from './SyncCheckEvidence.svelte';
  import DisclosureSection from './DisclosureSection.svelte';

  const {
    actorId,
    targetId,
    itemId,
    fetchCheck,
    checkEvidenceApi,
    syncResultHref,
    checkHref,
    onClose,
  }: {
    actorId: string;
    targetId: string;
    itemId: string | null;
    fetchCheck: (id: string) => Promise<SyncCheckResponse>;
    checkEvidenceApi?: Pick<PanelApi, 'fetchSyncCheckObservations'>;
    syncResultHref: (id: string) => string;
    checkHref: (id: string) => string;
    onClose: () => void;
  } = $props();
  const query = createQuery(() => ({
    queryKey: ['sync-check', actorId, targetId, itemId],
    queryFn: () => {
      if (itemId === null) throw new Error('No check selected');
      return fetchCheck(itemId);
    },
    enabled: itemId !== null,
    staleTime: 0,
    retry: false,
    refetchInterval: (query) => {
      const state = query.state.data?.execution?.state;
      return itemId !== null &&
        state &&
        !['succeeded', 'failed', 'cancelled', 'superseded'].includes(state)
        ? 15_000
        : false;
    },
  }));
  const body = $derived(query.error ? undefined : query.data);
  const denied = $derived(
    query.error instanceof PanelApiError && [401, 403].includes(query.error.status),
  );
  const missing = $derived(query.error instanceof PanelApiError && query.error.status === 404);
</script>

<Modal
  id="sync-check"
  open={itemId !== null}
  title="Repository check"
  description="What this check found and what happened during execution"
  variant="inspector"
  {onClose}
>
  {#snippet headerExtra()}<Button tone="quiet" onclick={onClose}>Close check</Button>{/snippet}
  <div class="check-content">
    <div class="read-actions">
      <Button
        aria-disabled={query.isFetching}
        onclick={() => {
          if (!query.isFetching) void query.refetch();
        }}>Refresh check</Button
      >
      <span role="status">{query.isFetching && !query.isPending ? 'Refreshing check…' : ''}</span>
    </div>
    {#if query.error}
      <ResultProblem
        title={denied
          ? 'Check access unavailable'
          : missing
            ? 'Check not found'
            : 'Check could not be loaded'}
        problem={denied
          ? 'Sign in and check your workspace access before refreshing this check.'
          : missing
            ? 'No comparison or execution record is available for this check in this workspace.'
            : 'Refresh this check to try the read again. Refreshing does not start new work.'}
      />
    {:else if !body}
      <p role="status">Loading check…</p>
    {:else}
      {#if body.execution?.state === 'failed'}
        <section class="execution-failure" aria-label="Execution failure" role="status">
          <h3>Execution failed</h3>
          <p>{body.execution.summary || 'No failure explanation was recorded'}</p>
          {#if body.result}<p>
              The comparison below was recorded separately from the worker outcome
            </p>{/if}
        </section>
      {/if}
      <SyncCheckSummary
        details={body.result}
        execution={body.execution}
        resultHref={syncResultHref}
      />
      {#if body.execution === null}
        <p class="execution-note">
          Execution details are unavailable. The comparison above is retained separately and does
          not confirm how execution finished.
        </p>
      {/if}
      {#if body.result?.outcome && checkEvidenceApi && itemId}
        {#key JSON.stringify([actorId, targetId, itemId])}
          <SyncCheckEvidence api={checkEvidenceApi} {targetId} checkId={itemId} />
        {/key}
      {/if}
      {#if !body.check.available}
        <section class="current-action" aria-label="Current workspace check availability">
          <h3>Current workspace</h3>
          <p>
            {body.check.running_check_id === itemId
              ? 'This check is running. Refresh this check to follow its result.'
              : syncCheckGuidance(body.check, targetId)}
          </p>
          {#if body.check.blocking_plan_id}
            <Link href={syncResultHref(body.check.blocking_plan_id)}>View current changes</Link>
          {/if}
          {#if body.check.running_check_id && body.check.running_check_id !== itemId}
            <Link href={checkHref(body.check.running_check_id)}>View running check</Link>
          {/if}
        </section>
      {/if}
      {#if body.execution}
        <DisclosureSection
          title="Execution details"
          description="Recorded worker state and progress"
        >
          <dl>
            <div>
              <dt>State</dt>
              <dd>{words(body.execution.state)}</dd>
            </div>
            <div>
              <dt>Attempt</dt>
              <dd>{body.execution.attempt}</dd>
            </div>
            {#if body.execution.progress_total > 0}<div>
                <dt>Progress</dt>
                <dd>{body.execution.progress_current} of {body.execution.progress_total}</dd>
              </div>{/if}
            {#if body.execution.started_at}<div>
                <dt>Started</dt>
                <dd>
                  <RelativeTime
                    value={body.execution.started_at}
                    nowMs={Date.parse(body.observed_at)}
                    exact
                  />
                </dd>
              </div>{/if}
            {#if body.execution.finished_at}<div>
                <dt>Finished</dt>
                <dd>
                  <RelativeTime
                    value={body.execution.finished_at}
                    nowMs={Date.parse(body.observed_at)}
                    exact
                  />
                </dd>
              </div>{/if}
          </dl>
          {#if body.result && body.execution.state !== 'failed' && body.execution.summary}<p>
              {body.execution.summary}
            </p>{/if}
        </DisclosureSection>
      {/if}
    {/if}
  </div>
</Modal>

<style>
  .check-content {
    display: grid;
    gap: var(--space-5);
  }
  .read-actions {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .read-actions span,
  .execution-note {
    color: var(--text-secondary);
  }
  .execution-failure,
  .current-action {
    display: grid;
    gap: var(--space-2);
  }
  h3,
  p,
  dl {
    margin: 0;
  }
  h3 {
    font-size: var(--font-size-body);
  }
  dl {
    display: grid;
    gap: var(--space-2);
  }
  dl > div {
    display: flex;
    justify-content: space-between;
    gap: var(--space-4);
  }
  dd {
    margin: 0;
    font-variant-numeric: tabular-nums;
  }
  .read-actions :global([aria-disabled='true']) {
    opacity: 0.5;
    cursor: default;
  }
</style>
