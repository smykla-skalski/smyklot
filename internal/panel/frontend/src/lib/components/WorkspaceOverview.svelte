<script module lang="ts">
  import type { QueueItem, SyncPlan, SyncStatus } from '../types';

  /** In flight: what is running now, and what is waiting to. */
  export const ACTIVE_QUEUE_STATES = ['running', 'retrying', 'ready', 'scheduled', 'blocked'];

  /**
   * A queue row's heading. The pull request's own title where the arm kept one,
   * and the act otherwise - a row armed before Smyklot carried titles reads as
   * what it will do, which is what it always read as.
   */
  export function queueHeading(item: QueueItem): string {
    if (item.kind !== 'pending_ci') return item.title;

    return item.details?.pull_request_title ?? item.title;
  }

  /** `platform-infra #184`, or nothing when the row is about no pull request. */
  export function queueSubject(item: QueueItem): string | null {
    const repository = (item.repository_name ?? '').split('/').at(-1) ?? '';
    const number = item.kind === 'pending_ci' ? item.details?.pull_request : undefined;
    if (repository === '') return null;

    return number === undefined ? repository : `${repository} #${number}`;
  }

  /** How many repositories a plan would change, which is not how many changes it holds. */
  export function planReach(plan: SyncPlan | null): number {
    if (plan === null) return 0;

    return new Set(plan.actions.map((action) => action.repository)).size;
  }

  /** Repositories the last sweep found out of step - a plan is waiting for them. */
  export function driftedRepositories(status: SyncStatus | null): number {
    if (status === null) return 0;

    return status.repositories.filter((row) =>
      Object.values(row.cells).some((cell) => cell.state === 'pending'),
    ).length;
  }
</script>

<!--
@component
Where a workspace opens: what needs somebody, what is in flight, what just
happened, and the pulse of the whole thing at the foot.

The verdict leads, and it is a count of things a person has to act on rather
than a health badge - "nothing needs review" is the good state and says so in
words. Everything below it is somebody else's page shown small: three rows of
what would otherwise be four visits.
-->

<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';

  import { formatUntil, sentenceCase } from '../format';
  import { getPanelSession } from '../session.svelte';
  import type { AuditEntry, PanelTarget, RepositorySummary } from '../types';

  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import RelativeTime from './RelativeTime.svelte';

  const { target }: { target: PanelTarget } = $props();

  const session = getPanelSession();
  const api = $derived(session.api);
  const targetId = $derived(target.id);

  /* One clock for the page, floored to the minute: every row reads a relative
     time against it, and a value that moves on its own would make three cards
     re-render at three different instants. */
  const nowMs = Math.floor(Date.now() / 60_000) * 60_000;
  /* Floored to the hour, so "the last day" is a stable cache key across a
     remount rather than a new question every time the page opens. */
  const dayAgo = new Date(Math.floor(nowMs / 3_600_000) * 3_600_000 - 86_400_000).toISOString();

  const planQuery = createQuery(() => ({
    queryKey: ['sync-plan', targetId],
    queryFn: () => api.fetchSyncPlan(targetId),
  }));

  const statusQuery = createQuery(() => ({
    queryKey: ['sync-status', targetId],
    queryFn: () => api.fetchSyncStatus(targetId),
  }));

  /* Repositories whose own file will not parse: commands there go unanswered
     until somebody fixes it, which is the one repository problem a reader has
     to act on rather than read about. */
  const brokenQuery = createQuery(() => ({
    queryKey: ['repositories-broken-file', targetId],
    queryFn: () =>
      api.fetchRepositories(targetId, {
        query: '',
        sort: 'name_asc',
        limit: 3,
        state: 'all',
        files: ['invalid'],
        setting: { mode: 'all' },
      }),
  }));

  const countsQuery = createQuery(() => ({
    queryKey: ['repositories-enabled-count', targetId],
    queryFn: async () => {
      const [all, enabled] = await Promise.all([
        api.fetchRepositories(targetId, {
          query: '',
          sort: 'name_asc',
          limit: 1,
          state: 'all',
          files: [],
          setting: { mode: 'all' },
        }),
        api.fetchRepositories(targetId, {
          query: '',
          sort: 'name_asc',
          limit: 1,
          state: 'enabled',
          files: [],
          setting: { mode: 'all' },
        }),
      ]);

      return { all: all.total, enabled: enabled.total };
    },
  }));

  const activeQuery = createQuery(() => ({
    queryKey: ['overview-queue', targetId],
    queryFn: () =>
      api.fetchTargetQueue(targetId, `?state=${ACTIVE_QUEUE_STATES.join(',')}&limit=3&offset=0`),
  }));

  const auditQuery = createQuery(() => ({
    queryKey: ['overview-audit', targetId],
    queryFn: () =>
      api.fetchAudit(targetId, {
        query: '',
        sort: 'newest',
        limit: 3,
        scope: 'all',
        change: 'all',
        categories: [],
      }),
  }));

  const failuresQuery = createQuery(() => ({
    queryKey: ['overview-failures', targetId, dayAgo],
    queryFn: () =>
      api.fetchFailures(targetId, {
        query: '',
        sort: 'newest',
        limit: 1,
        kind: 'all',
        since: dayAgo,
      }),
  }));

  const plan = $derived(planQuery.data?.plan ?? null);
  const fleet = $derived(statusQuery.data ?? null);
  const broken: RepositorySummary[] = $derived(brokenQuery.data?.items ?? []);
  const inFlight: QueueItem[] = $derived(activeQuery.data?.items ?? []);
  const lately: AuditEntry[] = $derived(auditQuery.data?.items ?? []);
  const failures = $derived(failuresQuery.data?.total ?? 0);
  const counts = $derived(countsQuery.data ?? null);

  const planChanges = $derived(plan?.actions.length ?? 0);
  const planWaiting = $derived(plan !== null && plan.state === 'computed' && planChanges > 0);

  /* What a person has to act on, which is not everything that is wrong: work
     already moving needs nobody, and a repository out of step is what the plan
     is FOR. Each item is a row below, so the count and the list cannot
     disagree. */
  const attention = $derived((planWaiting ? 1 : 0) + broken.length);

  const checked = $derived(fleet?.checked_at ?? plan?.computed_at ?? null);

  const drifted = $derived(driftedRepositories(fleet));
  const reach = $derived(planReach(plan));

  function auditLine(entry: AuditEntry): string {
    return sentenceCase(entry.summary.replace(/\s+for\s*$/i, ''));
  }

  const repositoriesHref = $derived(session.viewHref('repositories'));
  const queueHref = $derived(session.queueSectionHref('active'));
  const auditHref = $derived(session.historyHref('audit'));
  const failuresHref = $derived(session.historyHref('failures'));
  const planHref = $derived(session.syncSectionHref('plan'));
  const syncHref = $derived(session.syncSectionHref('overview'));
</script>

<div class="view-frame">
  <PageHeader id="overview-heading" title="Overview" />

  <div class="card">
    <div class="card-head verdict-head">
      <h2 class="card-title">
        {#if attention === 0}
          Nothing needs review
        {:else}
          <span class="is-drift">{attention} {attention === 1 ? 'item' : 'items'}</span> need review
        {/if}
      </h2>
      {#if checked !== null}
        <span class="card-meta"
          >{attention === 0 ? 'Everything' : 'Everything else'} is running on its own · checked
          <strong><RelativeTime value={checked} {nowMs} /></strong></span
        >
      {/if}
    </div>

    {#if attention === 0}
      <p class="state-panel">
        <span
          ><strong>Quiet.</strong> No plan is waiting and no repository file is broken. Work lands here
          as it arrives</span
        >
      </p>
    {:else}
      <div class="object-list">
        {#if planWaiting}
          <a class="object-row" href={planHref}>
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name">Review the sync plan</span>
                <span class="mx-mark mx-pending"
                  ><span class="t"
                    >{planChanges}
                    {planChanges === 1 ? 'change' : 'changes'}</span
                  ></span
                >
              </span>
              <span class="object-sum"
                >A plan across {reach}
                {reach === 1 ? 'repository' : 'repositories'} waits for your approval{#if plan !== null}
                  - it expires
                  <span class="nowrap-atom">{formatUntil(plan.expires_at, nowMs)}</span>{/if}</span
              >
            </span>
            <span class="object-side"><Icon name="chevron-right" size="xs" /></span>
          </a>
        {/if}
        {#each broken as repository (repository.id)}
          <a class="object-row" href={repositoriesHref}>
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name"
                  >Fix the repository file of <span class="file-path">{repository.name}</span></span
                >
                <span class="mx-mark mx-refused"><span class="t">commands paused</span></span>
              </span>
              <span class="object-sum is-refused"
                >Its configuration file does not parse - commands there go unanswered until it is
                fixed or bypassed</span
              >
            </span>
            <span class="object-side"><Icon name="chevron-right" size="xs" /></span>
          </a>
        {/each}
      </div>
    {/if}
  </div>

  <div class="card block-gap-top">
    <div class="card-head">
      <h2 class="card-title">Active work</h2>
      <a class="btn btn-quiet" href={queueHref}><span class="button-label">Open the queue</span></a>
    </div>
    {#if inFlight.length === 0}
      <p class="state-panel"><span>Nothing is in flight</span></p>
    {:else}
      <div class="object-list">
        {#each inFlight as item (item.id)}
          {@const subject = queueSubject(item)}
          <div class="object-row">
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name">{queueHeading(item)}</span>
                {#if subject !== null}
                  <span class="pill"><span class="t">{subject}</span></span>
                {/if}
              </span>
              <span class="object-sum"
                >{item.blocked_reason ?? item.summary ?? sentenceCase(item.state)}</span
              >
            </span>
            <span class="object-side">
              {#if item.estimated_start_at !== undefined}
                <span class="mx-mark {item.state === 'running' ? 'mx-instep' : 'mx-pending'}"
                  ><RelativeTime class="t" value={item.estimated_start_at} {nowMs} future /></span
                >
              {/if}
            </span>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <div class="card block-gap-top">
    <div class="card-head">
      <h2 class="card-title">Recent activity</h2>
      <a class="btn btn-quiet" href={auditHref}><span class="button-label">Open the audit</span></a>
    </div>
    {#if lately.length === 0}
      <p class="state-panel"><span>Nothing has happened yet</span></p>
    {:else}
      <div class="object-list">
        {#each lately as entry (entry.id)}
          <div class="object-row">
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name">{auditLine(entry)}</span>
                {#if entry.repository_full_name !== undefined}
                  <span class="pill"
                    ><span class="t"
                      >{entry.repository_full_name.split('/').at(-1) ??
                        entry.repository_full_name}</span
                    ></span
                  >
                {/if}
              </span>
              <span class="object-sum"
                >By {entry.actor.display_name} ·
                <RelativeTime value={entry.created_at} {nowMs} /></span
              >
            </span>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <!-- The workspace's pulse: three facts, each a link to the page that owns it. -->
  <div class="card block-gap-top">
    <div class="fact-row">
      <span class="fact-bit">
        <span class="fact-dot"></span>
        <a href={repositoriesHref}
          >{#if counts === null}Reading repositories{:else}Commands on in {counts.enabled} of {counts.all}
            {counts.all === 1 ? 'repository' : 'repositories'}{/if}</a
        >
      </span>
      <span class="fact-bit">
        <span class="fact-dot" class:is-warn={drifted > 0}></span>
        <a href={syncHref}
          >{#if drifted === 0}Every repository in step with sync{:else}{drifted}
            {drifted === 1 ? 'repository' : 'repositories'} out of step with sync{/if}</a
        >
      </span>
      <span class="fact-bit">
        <span class="fact-dot" class:is-bad={failures > 0}></span>
        <a href={failuresHref}
          >{failures === 0 ? 'No' : failures}
          {failures === 1 ? 'failure' : 'failures'} in the last day</a
        >
      </span>
    </div>
  </div>
</div>

<style>
  /* The verdict is a heading and a note on one line, and the note is the
     quieter half - the count is what a reader came for. */
  .verdict-head {
    align-items: baseline;
  }

  .card-title .is-drift {
    color: var(--diff-chg-ink);
  }

  .state-panel {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    line-height: var(--leading-meta);
    margin: 0;
    padding: var(--space-3);
  }

  .object-sum.is-refused {
    color: var(--danger);
  }

  /* Each fact is a dot and a link, and the dot is the only colour on the strip:
     three coloured sentences would be three alarms. */
  .fact-row {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-5);
  }

  .fact-bit {
    align-items: center;
    color: var(--text-secondary);
    display: flex;
    font-size: var(--font-size-meta);
    gap: var(--space-2);
    line-height: var(--leading-meta);
  }

  .fact-bit a {
    color: inherit;
    text-decoration: none;
  }

  .fact-bit a:hover {
    color: var(--text-primary);
    text-decoration: underline;
  }

  .fact-dot {
    background: var(--success);
    block-size: 6px;
    border-radius: 50%;
    flex: none;
    inline-size: 6px;
  }

  .fact-dot.is-warn {
    background: var(--warning);
  }

  .fact-dot.is-bad {
    background: var(--danger);
  }

  @media (max-width: 47.9375rem) {
    /* One fact per line: three on a phone wrap into a block nobody reads as a
       list of three separate places to go. */
    .fact-row {
      display: grid;
      gap: var(--space-2);
    }

    /* The verdict and its note stop sharing a line. The head is one auto-flow
       row, so at 375px the count wrapped to four lines and the note was drawn
       across them - two sentences in one place, neither readable. */
    .verdict-head {
      align-items: start;
      grid-auto-flow: row;
      grid-template-columns: minmax(0, 1fr);
      row-gap: var(--space-2);
    }

    /* The whole row is the target on a phone, so the chevron that stacked under
       the sentence pointed at nothing and cost a line. */
    .object-list :global(.object-row > .object-side:empty),
    .object-list :global(a.object-row > .object-side) {
      display: none;
    }
  }
</style>
