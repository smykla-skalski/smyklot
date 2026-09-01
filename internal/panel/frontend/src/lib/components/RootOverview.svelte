<!--
@component
Where the console opens: what needs an operator, then the queue, then what has
failed. The service and its database are one quiet line at the foot, loud only
when something is wrong.

The verdict counts asks rather than reporting health - an operator opens this
page to find out whether anything is waiting for them, and "nothing needs
attention" is the good answer said in words. Every count on it is a row below,
so the number and the list cannot disagree.

Each card reads its own endpoint, so one slow answer does not hold up the rest.
-->

<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { useInterval } from 'runed';
  import { fade } from 'svelte/transition';

  import { collapse, LiveList, rowMotion } from '#lib/live-list.svelte.js';

  import { queueListKey, ROOT_OVERVIEW_ACTIVE_QUEUE } from '#lib/queue-cache.js';
  import { queueLine } from '#lib/queue-words.js';
  import type { PanelApi } from '../api';
  import { failureAct } from '../failures';
  import { formatLatency, sentenceCase } from '../format';
  import { getPanelSession } from '../session.svelte';
  import type {
    QueueItem,
    RootWorkspace,
    RootOverview,
    RootOverviewFailure,
    ScheduleRequest,
  } from '../types';
  import { cadenceWords, workloadTitle } from '../workloads';

  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import Icon from './Icon.svelte';
  import Pill from './Pill.svelte';
  import RelativeTime from './RelativeTime.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import { queueHeading, queueSubject } from './WorkspaceOverview.svelte';

  const { api }: { api: PanelApi } = $props();

  const session = getPanelSession();

  /* ONE clock for the page, and it moves - see `WorkspaceOverview` for what a stopped
     one costs. Every row reads its relative time against this, so the cards agree on the
     instant, and the queue's own countdown arrives instead of standing still. */
  let nowMs = $state(Date.now());
  useInterval(1_000, { callback: () => (nowMs = Date.now()) });

  const overviewQuery = createQuery(() => ({
    queryKey: ['root-overview'],
    queryFn: () => api.fetchRootOverview(),
  }));

  const queueQuery = createQuery(() => ({
    queryKey: queueListKey(undefined, ROOT_OVERVIEW_ACTIVE_QUEUE),
    queryFn: () => api.fetchRootQueue(ROOT_OVERVIEW_ACTIVE_QUEUE),
  }));

  /* The overview counts workspaces whose owner list will not sync; this names
     them. A count an operator cannot act on is a count they learn to ignore. */
  const workspacesQuery = createQuery(() => ({
    queryKey: ['root-workspaces'],
    queryFn: () => api.fetchRootWorkspaces(),
  }));

  const requestsQuery = createQuery(() => ({
    queryKey: ['root-schedule-requests'],
    queryFn: () => api.fetchRootScheduleRequests(),
  }));

  const overview = $derived<RootOverview | null>(overviewQuery.data ?? null);
  const workspaces = $derived<RootWorkspace[]>(workspacesQuery.data ?? []);
  const inFlight = $derived<QueueItem[]>(queueQuery.data?.items ?? []);
  const active = new LiveList(
    () => inFlight,
    (item) => item.id,
  );
  /* Three, like the cards beside it: the overview says whether deliveries are
     failing, and the failures page says which. */
  const failures = $derived<RootOverviewFailure[]>((overview?.recent_failures ?? []).slice(0, 3));

  const blocked = $derived(
    workspaces.filter((workspace) => workspace.ownership.status !== 'fresh'),
  );
  const waiting = $derived<ScheduleRequest[]>(
    (requestsQuery.data ?? []).filter((request) => request.state === 'pending'),
  );
  const elevations = $derived(overview?.active_elevations ?? 0);
  const unread = $derived(overview?.unread_security_events ?? 0);

  const attention = $derived(
    blocked.length + waiting.length + (elevations > 0 ? 1 : 0) + (unread > 0 ? 1 : 0),
  );

  const database = $derived(overview?.service.database ?? null);
  const wellness = $derived(
    overview === null
      ? 'reading the service'
      : overview.service.storage === 'healthy'
        ? 'all systems normal'
        : `the database is ${overview.service.storage}`,
  );

  const workspacesHref = session.rootWorkspacesHref();
  const schedulesHref = session.rootHrefFor('schedules');
  const queueHref = session.queueHref();
  const failuresHref = session.rootFailuresHref();
  const auditHref = session.rootAuditHref();
  const inboxHref = session.inboxHref();
  const serviceHref = session.rootRuntimeHref('service');

  function workspaceName(targetId: string): string {
    const workspace = workspaces.find((candidate) => candidate.id === targetId);
    return workspace?.account.display_name ?? targetId;
  }

  /* Where a queue row is happening. The console reads every workspace's work at
     once, so the workspace comes first and the pull request qualifies it - the
     workspace view drops the name, because there it is the only answer. */
  function queueWhere(item: QueueItem): string | null {
    const subject = queueSubject(item);
    const workspace = item.target_id === undefined ? null : workspaceName(item.target_id);
    if (workspace === null) return subject;

    return subject === null ? workspace : `${workspace} · ${subject}`;
  }

  function repositoryName(fullName: string): string {
    return fullName.split('/').at(-1) ?? fullName;
  }

  /* Days, because the foot of the console answers "has it restarted lately"
     rather than "how long exactly". */
  function uptime(seconds: number): string {
    const days = Math.floor(seconds / 86_400);
    if (days > 0) return `up ${days} ${days === 1 ? 'day' : 'days'}`;

    const hours = Math.floor(seconds / 3_600);
    if (hours > 0) return `up ${hours} ${hours === 1 ? 'hour' : 'hours'}`;

    return `up ${Math.max(1, Math.floor(seconds / 60))} minutes`;
  }
</script>

<div class="view-frame">
  <RootPageHeader title="Overview" headingId="root-page-heading" />

  <Card>
    <div class="card-head verdict-head">
      <h2 class="card-title">
        {#if attention === 0}
          Nothing needs attention
        {:else}
          <span class="is-drift">{attention} {attention === 1 ? 'item' : 'items'}</span> need attention
        {/if}
      </h2>
      {#if overview !== null}
        <span class="card-note"
          >{overview.catalog.workspaces}
          {overview.catalog.workspaces === 1 ? 'workspace' : 'workspaces'} ·
          {overview.catalog.enabled_repositories}
          {overview.catalog.enabled_repositories === 1 ? 'repository' : 'repositories'} with commands
          on · {wellness}</span
        >
      {/if}
    </div>

    {#if attention === 0}
      <div class="state-panel">
        <span
          ><strong>Quiet.</strong> No workspace is blocked and nothing is waiting on a decision. Work
          lands here as it arrives</span
        >
      </div>
    {:else}
      <div class="object-list">
        {#each blocked as workspace (workspace.id)}
          <a class="object-row" href={workspacesHref}>
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name">{workspace.account.display_name}</span>
                <span class="mx-mark mx-refused"><span class="t">owner list blocked</span></span>
              </span>
              <span class="object-sum is-refused"
                >{workspace.ownership.detail ??
                  (workspace.ownership.status === 'permission_pending'
                    ? 'GitHub wants an organization owner to approve the Members permission before the owner list can sync'
                    : 'The last owner synchronisation failed, so approvals there are decided on an ageing list')}</span
              >
            </span>
            <span class="object-side"><Icon name="chevron-right" size="xs" /></span>
          </a>
        {/each}

        {#each waiting as request (request.id)}
          <a class="object-row" href={schedulesHref}>
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name">{workloadTitle(request.kind)}</span>
                <span class="mx-mark mx-pending"><span class="t">request waiting</span></span>
              </span>
              <span class="object-sum"
                >{workspaceName(request.target_id)} asks to run it
                <span class="nowrap-atom">{cadenceWords(request.cadence)}</span
                >{#if request.reason !== ''}&nbsp;· {request.reason}{/if} · approve or decline on Schedules</span
              >
            </span>
            <span class="object-side"><Icon name="chevron-right" size="xs" /></span>
          </a>
        {/each}

        {#if elevations > 0}
          <a class="object-row" href={auditHref}>
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name"
                  >{elevations === 1
                    ? 'An operator is visiting a workspace'
                    : 'Operators are visiting workspaces'}</span
                >
                <span class="mx-mark mx-pending"
                  ><span class="t"
                    >{elevations}
                    {elevations === 1 ? 'visit' : 'visits'}</span
                  ></span
                >
              </span>
              <span class="object-sum"
                >Somebody may write in a workspace they do not own for fifteen minutes · the audit
                says who and what changed</span
              >
            </span>
            <span class="object-side"><Icon name="chevron-right" size="xs" /></span>
          </a>
        {/if}

        {#if unread > 0}
          <a class="object-row" href={inboxHref}>
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name">Read the security notifications</span>
                <span class="mx-mark mx-pending"><span class="t">{unread} unread</span></span>
              </span>
              <span class="object-sum"
                >GitHub told the owners something about this App and nobody has read it yet</span
              >
            </span>
            <span class="object-side"><Icon name="chevron-right" size="xs" /></span>
          </a>
        {/if}
      </div>
    {/if}
  </Card>

  <Card>
    <!-- In the head, where it cannot push the list down - see `WorkspaceOverview`. -->
    <div class="card-head">
      <h2 class="card-title">Queue</h2>
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
          ><strong>Nothing is in flight.</strong> Work the service has accepted appears here while it
          runs</span
        >
      </div>
    {:else}
      <div class="object-list">
        {#each active.rows as item (item.id)}
          {@const where = queueWhere(item)}
          {@const line = queueLine(item, nowMs)}
          <!-- No `animate:flip` - see `WorkspaceOverview` for the 51px it costs. -->
          <div
            class="object-row"
            data-queue-item={item.id}
            in:fade={rowMotion.arriving}
            out:collapse={rowMotion.leaving}
          >
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name">{queueHeading(item)}</span>
                {#if where !== null}
                  <span class="pill"><span class="t">{where}</span></span>
                {/if}
              </span>
              <!-- The queue's own sentence, so a row read here and the same row read
                   on the queue page say the same thing about itself. The time is in
                   it rather than beside it: what a console is asked is when the work
                   runs, and that is the end of the sentence. The separator rides the
                   words for the reason the queue's own row does - markup whitespace
                   beside a block is trimmed, and "runs" would take the time straight
                   onto its own last letter. -->
              <span class="object-sum"
                >{line.when === undefined
                  ? line.lead
                  : `${line.lead} `}{#if line.when !== undefined}<time
                    datetime={line.when.iso}
                    title={line.when.exact}>{line.when.relative}</time
                  >{line.tail ?? ''}{/if}</span
              >
            </span>
          </div>
        {/each}
      </div>
    {/if}
  </Card>

  <Card>
    <div class="card-head">
      <h2 class="card-title">Recent failures</h2>
      <a class="btn btn-quiet" href={failuresHref}
        ><span class="button-label">Open failures</span></a
      >
    </div>
    {#if failures.length === 0}
      <div class="state-panel">
        <span
          ><strong>Nothing has failed lately.</strong> A failure in any workspace lands here with its
          cause</span
        >
      </div>
    {:else}
      <div class="object-list">
        {#each failures as item (item.failure.id)}
          <a class="object-row" href={failuresHref}>
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name">
                  {failureAct(item.failure.stage)}
                  <code class="file-path">{repositoryName(item.failure.repository_full_name)}</code>
                </span>
                <Pill tone={item.failure.retryable ? 'warning' : 'danger'}>
                  {item.failure.retryable ? 'Retrying' : 'Needs a fix'}
                </Pill>
              </span>
              <span class="object-sum"
                >{item.workspace.display_name} · {sentenceCase(item.failure.reason)}
                {item.failure.retryable ? '· Smyklot retries on its own ·' : '·'}
                <RelativeTime value={item.failure.occurred_at} {nowMs} /></span
              >
            </span>
            <span class="object-side"><Icon name="chevron-right" size="xs" /></span>
          </a>
        {/each}
      </div>
    {/if}
  </Card>

  <!-- The plumbing, at the foot and in one line each: green is the whole story
       until it is not, and then the words say what changed. -->
  <Card>
    <div class="fact-row">
      <span class="fact-bit">
        <span class="fact-dot"></span>
        <span
          >{#if overview === null}Reading the service{:else}Service healthy · {overview.service
              .version || 'development'} ·
            <span class="nowrap-atom">{uptime(overview.service.uptime_seconds)}</span>{/if}</span
        >
      </span>
      <span class="fact-bit">
        <span
          class="fact-dot"
          class:is-warn={database?.state === 'degraded'}
          class:is-bad={database?.state === 'unavailable'}
        ></span>
        <span
          >{#if database === null}Reading the database{:else}{database.engine}
            {database.state === 'healthy' ? 'healthy' : database.state} · answers in
            <span class="nowrap-atom">{formatLatency(database.latency_ms)}</span>{/if}</span
        >
      </span>
      <span class="fact-bit"><a href={serviceHref}>Service health</a></span>
    </div>
  </Card>
</div>
