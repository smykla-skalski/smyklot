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
  import { useInterval } from 'runed';
  import { fade } from 'svelte/transition';

  import { collapse, LiveList, rowMotion } from '#lib/live-list.svelte.js';

  import { formatUntil, sentenceCase } from '../format';
  import { getPanelSession } from '../session.svelte';
  import type { AuditEntry, PanelTarget, RepositorySummary } from '../types';

  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import RelativeTime from './RelativeTime.svelte';

  const { target }: { target: PanelTarget } = $props();

  const session = getPanelSession();
  const api = $derived(session.api);
  const targetId = $derived(target.id);

  /* ONE clock for the page, and it moves. Every row reads its relative time against
     this, so the cards agree on the instant - but it was a const floored to the minute,
     which meant the page's whole sense of time was the moment it opened. Active work
     counts down to when Smyklot will act, and a countdown against a stopped clock never
     arrives: a row said "in 4 minutes" for as long as the tab stayed open, and the
     estimate the reader was watching had passed several minutes ago. */
  let nowMs = $state(Date.now());
  useInterval(1_000, { callback: () => (nowMs = Date.now()) });
  /* The query key does NOT move with it. Floored to the hour and read once, so "the last
     day" is one question this page asks rather than a new one every second. */
  const dayAgo = new Date(
    Math.floor(Date.now() / 3_600_000) * 3_600_000 - 86_400_000,
  ).toISOString();

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
  const active = new LiveList(
    () => inFlight,
    (item) => item.id,
  );
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

  <Card>
    <div class="card-head verdict-head">
      <h2 class="card-title">
        {#if attention === 0}
          Nothing needs review
        {:else}
          <span class="is-drift">{attention} {attention === 1 ? 'item' : 'items'}</span> need review
        {/if}
      </h2>
      {#if checked !== null}
        <span class="card-note"
          >{attention === 0 ? 'Everything' : 'Everything else'} is running on its own · checked
          <strong><RelativeTime value={checked} {nowMs} /></strong></span
        >
      {/if}
    </div>

    {#if attention === 0}
      <div class="state-panel">
        <span
          ><strong>Quiet.</strong> No plan is waiting and no repository file is broken. Work lands here
          as it arrives</span
        >
      </div>
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
  </Card>

  <Card>
    <!-- IN THE HEAD, because a notice that pushes the list down is the one thing the
         guidance on shifting layouts names outright: do not insert content above
         content the reader is already looking at, unless they asked for it. The head is
         where it cannot - `.card-head` holds a stated `min-block-size` and negative-
         margins the controls in it precisely so that what it carries never changes the
         card's height - and it costs no reserved row and covers nothing, which the two
         other ways of not shifting each do.

         `role="status"` because this appears without the reader doing anything: it has
         to reach a screen reader without taking focus from whatever they were reading.
         The count is on the same element as the words, so what is announced is the
         whole sentence rather than a bare number. -->
    <div class="card-head">
      <h2 class="card-title">Active work</h2>
      {#if active.changed > 0}
        <span class="card-meta" role="status"
          >{active.changed}
          {active.changed === 1 ? 'item' : 'items'} behind</span
        >
        <Button tone="quiet" row onclick={() => active.refresh()}>Refresh now</Button>
      {/if}
      <a class="btn btn-quiet" href={queueHref}><span class="button-label">Open the queue</span></a>
    </div>
    {#if active.rows.length === 0}
      <div class="state-panel">
        <span
          ><strong>Nothing is in flight.</strong> A command lands here the moment Smyklot accepts it</span
        >
      </div>
    {:else}
      <div class="object-list">
        {#each active.rows as item (item.id)}
          {@const subject = queueSubject(item)}
          <!-- No `animate:flip` here, and that is the point. Svelte takes an outroing
               row OUT OF FLOW so its siblings can be flipped into place, which hands
               back the row's whole height in one frame - the card shuts by 51px and
               every card below it jumps, which is the thing being prevented. In flow,
               `collapse` gives the space back over its own duration and the rows below
               follow it down. The queue's table can afford the flip because a table
               holds its own rows; a list of blocks has nothing holding the space. -->
          <div class="object-row" in:fade={rowMotion.arriving} out:collapse={rowMotion.leaving}>
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
            <!-- The mark says which of the two states the row is in, and the time is
                 what it reads: a row that is running wears the quiet mark, and one
                 still waiting wears the chip. Two shapes in one column looked wrong
                 on the dev fixture, where every row's estimate is the same instant
                 and the words said nothing the shapes did not - it is the fixture
                 that was wrong. -->
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
  </Card>

  <Card>
    <div class="card-head">
      <h2 class="card-title">Recent activity</h2>
      <a class="btn btn-quiet" href={auditHref}><span class="button-label">Open the audit</span></a>
    </div>
    {#if lately.length === 0}
      <div class="state-panel">
        <span
          ><strong>Nothing has happened yet.</strong> Every change made through Smyklot here lands on
          this list</span
        >
      </div>
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
  </Card>

  <!-- The workspace's pulse: three facts, each a link to the page that owns it. -->
  <Card>
    <div class="fact-row">
      <!-- A fact, not a place: what the repositories page would say about itself is
           already this sentence, so the other two carry the strip's links. -->
      <span class="fact-bit">
        <span class="fact-dot"></span>
        <span
          >{#if counts === null}Reading repositories{:else}Commands on in {counts.enabled} of {counts.all}
            {counts.all === 1 ? 'repository' : 'repositories'}{/if}</span
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
  </Card>
</div>
