<script lang="ts">
  import type { QueueItem } from '../types';
  import { checkOutcome, checkOutcomeTitle, CHECK_OBSERVATION_LABELS } from '../sync-check';
  import { SYNC_SECTION_LABELS } from '../routes';
  import { formatDateTime } from '../format';
  import Link from './Link.svelte';

  const {
    details,
    execution,
    resultHref,
  }: {
    details?: { outcome?: unknown; result_plan_id?: unknown } | null;
    execution: Pick<QueueItem, 'state' | 'summary' | 'blocked_reason'> | null;
    resultHref?: (id: string) => string;
  } = $props();
  const outcome = $derived(checkOutcome(details?.outcome));
  const planId = $derived(
    typeof details?.result_plan_id === 'string' ? details.result_plan_id.trim() : '',
  );
  const terminal = $derived(
    ['succeeded', 'failed', 'cancelled', 'superseded'].includes(execution?.state ?? ''),
  );
</script>

<!--
@component
The retained outcome of one repository check, above execution metadata. Fresh
comparison counts and reused observations stay separate. Missing historical evidence
never becomes a claim that repositories match. Use SyncCheckEvidence for the bounded
repository details, and SyncOverview for current state rather than this snapshot.
-->
<section class="check-summary" aria-label="Check outcome">
  {#if outcome}
    <h3>{execution?.state === 'failed' ? 'Comparison recorded' : checkOutcomeTitle(outcome)}</h3>
    <p>{outcome.summary}</p>
    <p class="muted">
      Recorded <time datetime={outcome.completed_at}
        >{formatDateTime(outcome.completed_at, { named: true, seconds: true })}</time
      >. This is the result of this check, not the current repository status.
    </p>
    {#if Object.values(outcome.counts).some((count) => count && count > 0)}
      <dl aria-label="Fresh comparisons">
        {#each Object.entries(CHECK_OBSERVATION_LABELS) as [key, label] (key)}
          {@const count = outcome.counts[key as keyof typeof outcome.counts] ?? 0}
          {#if count > 0}<div>
              <dt>{label}</dt>
              <dd>{count}</dd>
            </div>{/if}
        {/each}
      </dl>
      <p class="muted">
        Counts are fresh comparisons of one repository and one category, not unique repositories.
      </p>
    {/if}
    {#if outcome.cached > 0}<p>
        {outcome.cached} earlier {outcome.cached === 1 ? 'observation' : 'observations'} reused. Their
        original observation times are shown below.
      </p>{/if}
    {#if outcome.missing_permissions?.length}<p>
        Not checked because GitHub permissions were missing: {outcome.missing_permissions
          .map((kind) => SYNC_SECTION_LABELS[kind])
          .join(', ')}. Review the app’s permissions in GitHub before checking again.
      </p>{/if}
  {:else}
    <h3>
      {execution?.state === 'running'
        ? 'Check in progress'
        : terminal || execution === null
          ? 'Check outcome unavailable'
          : 'Waiting to check repositories'}
    </h3>
    <p>
      {execution?.summary ||
        (terminal || execution === null
          ? 'No outcome summary was recorded.'
          : 'The result will appear here when the check finishes.')}
    </p>
    {#if terminal || execution === null}<p class="muted">
        This record has no retained comparison evidence. It does not confirm that repositories match
        their saved settings.
      </p>{/if}
    {#if execution?.blocked_reason}<p>{execution?.blocked_reason}</p>{/if}
  {/if}
  {#if outcome?.disposition === 'deferred'}
    {#if outcome.blocking_plan_id && resultHref}
      <Link href={resultHref(outcome.blocking_plan_id)}>View earlier changes</Link>
    {:else if !outcome.blocking_plan_id}
      <p class="muted">
        This check did not record which earlier changes prevented it from proceeding
      </p>
    {/if}
  {/if}
  {#if planId && resultHref}<Link href={resultHref(planId)}>View changes from this check</Link>{/if}
</section>

<style>
  .check-summary {
    display: grid;
    gap: var(--space-3);
  }
  h3,
  p,
  dl {
    margin: 0;
  }
  h3 {
    font-size: var(--font-size-body);
  }
  p {
    font-size: var(--font-size-meta);
    line-height: var(--leading-meta);
  }
  .muted {
    color: var(--text-muted);
  }
  dl {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-2) var(--space-4);
  }
  dl > div {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
    font-size: var(--font-size-meta);
  }
  dt {
    color: var(--text-secondary);
  }
  dd {
    margin: 0;
    font-weight: 650;
    font-variant-numeric: tabular-nums;
  }
</style>
