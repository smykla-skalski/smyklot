<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { PanelApiError, type PanelApi } from '../api';
  import { words } from '../queue-words';
  import { syncCheckGuidance } from '../sync-check-guidance';
  import Button from './Button.svelte';
  import Link from './Link.svelte';
  import Modal from './Modal.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import RelativeTime from './RelativeTime.svelte';
  import SyncCheckSummary from './SyncCheckSummary.svelte';
  import DisclosureSection from './DisclosureSection.svelte';

  const {
    actorId,
    targetId,
    selection,
    fetchOperation,
    checkHref,
    resultHref,
    onClose,
  }: {
    actorId: string;
    targetId: string;
    selection: { action: 'check' | 'dispatch'; requestKey: string };
    fetchOperation: PanelApi['fetchSyncOperation'];
    checkHref: (id: string) => string;
    resultHref: (id: string) => string;
    onClose: () => void | Promise<void>;
  } = $props();
  const query = createQuery(() => ({
    queryKey: ['sync-operation', actorId, targetId, selection.action, selection.requestKey],
    queryFn: () => fetchOperation(targetId, selection.action, selection.requestKey),
    enabled: actorId !== '',
    retry: false,
    staleTime: 0,
    refetchInterval: (query) => {
      const state = query.state.data?.execution?.state;
      return !query.state.error &&
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
  const planTitle = $derived(
    body?.execution?.state === 'failed' && body.plan?.state === 'applied'
      ? 'Plan recorded as applied'
      : body?.plan
        ? ({
            computed: 'Changes awaiting approval',
            approved: 'Changes waiting to run',
            applying: 'Changes being applied',
            applied: 'Changes applied',
            failed: 'Changes finished with failures',
            stale: 'Changes need a fresh check',
            expired: 'Approval expired',
            discarded: 'Changes discarded',
          }[body.plan.state] ?? words(body.plan.state))
        : 'Change details unavailable',
  );
</script>

<!--
@component
Read-only history for an accepted sync request, scoped to the actor, workspace, action and request key. Original acceptance remains distinct from current execution and comparison evidence. Refreshing never resubmits the request or establishes the outcome of an unconfirmed request.
-->

<Modal
  id="sync-operation"
  open
  title="Sync request"
  description="What you asked for, what happened, and where to go next"
  variant="inspector"
  onClose={() => void onClose()}
>
  {#snippet headerExtra()}<Button tone="quiet" onclick={() => void onClose()}>Close request</Button
    >{/snippet}
  <div class="operation-content">
    <div class="read-actions">
      <Button
        aria-disabled={query.isFetching}
        onclick={() => {
          if (!query.isFetching) void query.refetch();
        }}>Refresh request</Button
      >
      <span role="status">{query.isFetching && !query.isPending ? 'Refreshing request…' : ''}</span>
    </div>
    {#if query.error}
      <ResultProblem
        title={denied
          ? 'Request access unavailable'
          : missing
            ? 'Request not found'
            : 'Request could not be loaded'}
        problem={denied
          ? 'Sign in and check your workspace access before refreshing this request.'
          : missing
            ? 'No accepted request with this identity is available to your account in this workspace. This does not prove that an unconfirmed request failed.'
            : 'Refresh this request to try the read again. Refreshing does not start work.'}
      />
    {:else if !body}
      <p role="status">Loading request…</p>
    {:else}
      <section class="requested" aria-label="Original request">
        <h3>
          {body.acceptance.action === 'check' ? 'Check repositories' : 'Run prepared changes'}
        </h3>
        <p class="reason">{body.acceptance.reason}</p>
        <p class="muted">
          Accepted <RelativeTime
            value={body.acceptance.accepted_at}
            nowMs={Date.parse(body.observed_at)}
            exact
          />
        </p>
      </section>
      {#if body.execution?.state === 'failed'}
        <section class="outcome" role="status" aria-label="Execution failure">
          <h3>Execution failed</h3>
          <p>{body.execution.summary || 'No failure explanation was recorded'}</p>
          {#if body.comparison}<p class="muted">
              The comparison below was recorded separately from execution
            </p>{/if}
          {#if body.plan}<p class="muted">
              The plan result below was recorded separately from execution
            </p>{/if}
        </section>
      {/if}
      {#if body.acceptance.action === 'check'}
        <SyncCheckSummary details={body.comparison} execution={body.execution} {resultHref} />
        {#if body.comparison}<Link href={checkHref(body.acceptance.check_id)}
            >View repository results</Link
          >{/if}
      {:else}
        <section class="outcome" aria-label="Requested changes">
          <h3>{planTitle}</h3>
          {#if body.plan}
            <p>
              {body.plan.counts.create} to create · {body.plan.counts.update} to update · {body.plan
                .counts.delete} to delete
            </p>
            <p class="muted">
              These counts describe the prepared changes. Open their results to see what happened to
              each change.
            </p>
            <Link href={resultHref(body.acceptance.plan_id)}>View change results</Link>
          {:else}<p>
              The original request was accepted, but its change details are unavailable. Acceptance
              does not confirm that changes were applied.
            </p>{/if}
        </section>
      {/if}
      {#if body.execution === null}<p class="muted">
          Execution details are unavailable. This does not establish how the work finished.
        </p>{/if}
      {#if !body.check.available}
        <section class="current-action" aria-label="Current workspace">
          <h3>Current workspace</h3>
          <p>
            {body.acceptance.action === 'check' &&
            body.check.running_check_id === body.acceptance.check_id
              ? 'This check is still in progress. Refresh this request to follow its result.'
              : syncCheckGuidance(body.check, targetId)}
          </p>
          {#if body.check.blocking_plan_id && (body.acceptance.action !== 'dispatch' || body.check.blocking_plan_id !== body.acceptance.plan_id)}<Link
              href={resultHref(body.check.blocking_plan_id)}>View current changes</Link
            >{/if}
          {#if body.check.running_check_id && (body.acceptance.action !== 'check' || body.check.running_check_id !== body.acceptance.check_id)}<Link
              href={checkHref(body.check.running_check_id)}>View running check</Link
            >{/if}
        </section>
      {/if}
      {#if body.execution}
        <DisclosureSection title="Execution details" description="Recorded progress and timing">
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
        </DisclosureSection>
      {/if}
    {/if}
  </div>
</Modal>

<style>
  .operation-content {
    display: grid;
    gap: var(--space-5);
  }
  .read-actions {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .read-actions span,
  .muted {
    color: var(--text-secondary);
  }
  .requested,
  .outcome,
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
  .reason {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
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
