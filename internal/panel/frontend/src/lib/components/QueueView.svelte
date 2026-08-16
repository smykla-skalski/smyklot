<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';

  import type { PanelApi } from '$lib/api';
  import type { FilterSection } from '$lib/filter-menu';
  import { formatTimestamp } from '$lib/format';
  import {
    byMostRecent,
    bySoonest,
    cleanupState,
    endReason,
    outcomeState,
    queueNext,
    queueState,
    shortAge,
  } from '$lib/queue';
  import type { PendingCIRequest } from '$lib/types';
  import ActionMenu, { type ActionMenuItem } from './ActionMenu.svelte';
  import AppTooltip from './AppTooltip.svelte';
  import Chip from './Chip.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import Icon from './Icon.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import TableEmptyState from './TableEmptyState.svelte';
  import TableToolsMenu from './TableToolsMenu.svelte';

  type QueueSection = 'waiting' | 'recent';

  /* The reconciler's `PassingQuiet`, which is the whole span the ring draws.
     From `internal/pendingci/policy.go`; a request that has passed waits this
     long without anything changing, and then it merges. */
  const QUIET_SECONDS = 30;

  const {
    api,
    rootRole,
    section,
    onSection,
    onOpenRequest,
  }: {
    api: PanelApi;
    rootRole: string;
    section: QueueSection;
    onSection: (section: QueueSection) => void;
    onOpenRequest: (requestId: string) => void;
  } = $props();

  /* Each value carries the glyph its column carries, so the menu and the table
     say the same state the same way - and so no state here is told apart by hue
     alone, which three of these five pairs cannot survive under one dichromacy or
     another. */
  const STATE_FILTERS = [
    {
      options: [
        { value: 'passing', label: 'Passing', tone: 'valid', icon: 'success' },
        { value: 'pending', label: 'Running', tone: 'default', icon: 'pending' },
        { value: 'failing', label: 'Failing', tone: 'invalid', icon: 'failure' },
        { value: 'indeterminate', label: 'Unreadable', tone: 'bypassed', icon: 'alert' },
        { value: 'no_checks', label: 'No checks', tone: 'missing', icon: 'circle-dashed' },
      ],
    },
  ] satisfies readonly FilterSection[];

  /* The past is filtered by how a request ENDED, not by what its checks last
     said - and cancelled is not a failure, so it does not take the invalid tone.
     Somebody chose it, or the pull request moved on underneath it. */
  const OUTCOME_FILTERS = [
    {
      options: [
        { value: 'merged', label: 'Merged', tone: 'valid', icon: 'success' },
        { value: 'cancelled', label: 'Cancelled', tone: 'default', icon: 'circle-dashed' },
        { value: 'superseded', label: 'Superseded', tone: 'bypassed', icon: 'alert' },
      ],
    },
  ] satisfies readonly FilterSection[];

  /* Plain labels, no chips: a value is drawn as a chip here because its column
     draws it as one, and there is no schedule column - the schedule is what the
     Next column's second line explains in words. */
  const SCHEDULE_FILTERS = [
    {
      options: [
        { value: 'active', label: 'Active', description: 'Checked every few minutes' },
        { value: 'deferred', label: 'Deferred', description: 'Nothing has moved for an hour' },
      ],
    },
  ] satisfies readonly FilterSection[];

  /* The same key the Root overview reads under, so the two share one answer:
     opening the queue from the dashboard does not fetch what the dashboard has
     already got, and cancelling a request refreshes both. */
  const overviewQuery = createQuery(() => ({
    queryKey: ['root-overview'],
    queryFn: () => api.fetchRootOverview(),
  }));
  const waiting = $derived<PendingCIRequest[]>(
    overviewQuery.data === undefined
      ? []
      : [...overviewQuery.data.pending_ci.active, ...overviewQuery.data.pending_ci.deferred],
  );
  const recent = $derived<PendingCIRequest[]>(overviewQuery.data?.pending_ci.recent ?? []);
  const loading = $derived(overviewQuery.isFetching);
  /** Whether the queue has ever answered, which is what separates "waiting" from "empty". */
  const loaded = $derived(overviewQuery.data !== undefined);
  let actionProblem = $state<string | null>(null);
  const problem = $derived(
    actionProblem ??
      (overviewQuery.error === null
        ? null
        : overviewQuery.error instanceof Error
          ? overviewQuery.error.message
          : String(overviewQuery.error)),
  );
  let search = $state('');
  let query = $state('');
  let states = $state<string[]>([]);
  let schedules = $state<string[]>([]);
  let pendingAction = $state<string | null>(null);
  /* Ticks so a countdown counts. One second, because the last ten of a merge are
     the point of the column and a minute's granularity would miss them. */
  let now = $state(Date.now());

  const rows = $derived.by(() => {
    const source = section === 'waiting' ? waiting : recent;
    const needle = query.trim().toLocaleLowerCase();
    return source
      .filter(
        (request) =>
          states.length === 0 ||
          states.includes(section === 'recent' ? request.lifecycle : request.last_observed_state),
      )
      .filter((request) => schedules.length === 0 || schedules.includes(request.schedule))
      .filter(
        (request) =>
          needle === '' ||
          [request.repository_full_name, `#${request.pull_request}`, request.requester].some(
            (field) => field.toLocaleLowerCase().includes(needle),
          ),
      )
      .sort(section === 'recent' ? byMostRecent : bySoonest);
  });

  const hasFilters = $derived(query !== '' || states.length > 0 || schedules.length > 0);

  $effect(() => {
    const tick = setInterval(() => {
      now = Date.now();
    }, 1000);
    return () => clearInterval(tick);
  });

  $effect(() => {
    const next = search.trim();
    const timeout = window.setTimeout(() => (query = next), 180);
    return () => window.clearTimeout(timeout);
  });

  async function load(): Promise<void> {
    actionProblem = null;
    await overviewQuery.refetch();
  }

  function ownerOf(request: PendingCIRequest): string {
    const slash = request.repository_full_name.lastIndexOf('/');
    return slash === -1 ? '' : `${request.repository_full_name.slice(0, slash)}/`;
  }

  function repositoryOf(request: PendingCIRequest): string {
    const slash = request.repository_full_name.lastIndexOf('/');
    return slash === -1
      ? request.repository_full_name
      : request.repository_full_name.slice(slash + 1);
  }

  function contractOf(request: PendingCIRequest): string {
    const method = request.merge_method;
    const label = `${method.slice(0, 1).toUpperCase()}${method.slice(1)}`;
    return request.required_checks_only ? `${label} · required only` : label;
  }

  function githubHref(request: PendingCIRequest): string {
    return `https://github.com/${request.repository_full_name}/pull/${request.pull_request}`;
  }

  /* Destructive weight is inverted from the old panel: a filled danger button
     appears once, on the confirmation, and never in a row. Here Cancel is a menu
     item like any other, and it is simply not offered on a request that has
     already finished. */
  function actionsFor(request: PendingCIRequest): ActionMenuItem[] {
    /* Held while a mutation is in flight, because both of these carry the revision they were
       drawn with. A second Cancel sends the same one, the row has already moved past it, and the
       store answers 409 - so the reader is shown a red banner over a cancel that worked. */
    const armed = request.lifecycle === 'armed' && pendingAction === null;
    return [
      {
        id: 'open',
        icon: 'link',
        label: 'Open request',
        description: 'The timeline and the facts',
      },
      {
        id: 'check',
        icon: 'refresh',
        label: 'Check now',
        description: 'Read the checks again without waiting',
        disabled: !armed,
      },
      {
        id: 'cancel',
        icon: 'ban',
        label: 'Cancel',
        description: 'Stop the merge; the pull request is left alone',
        tone: 'danger',
        disabled: !armed,
      },
    ];
  }

  async function choose(request: PendingCIRequest, action: string): Promise<void> {
    if (action === 'open') {
      onOpenRequest(request.id);
      return;
    }
    const key = `${action}:${request.id}`;
    pendingAction = key;
    actionProblem = null;
    try {
      if (action === 'check') await api.checkRootPendingCI(request.id, request.revision);
      if (action === 'cancel') await api.cancelRootPendingCI(request.id, request.revision);
      await load();
    } catch (error) {
      actionProblem = error instanceof Error ? error.message : String(error);
    } finally {
      if (pendingAction === key) pendingAction = null;
    }
  }

  function openRow(event: MouseEvent, request: PendingCIRequest): void {
    if (event.defaultPrevented || event.button !== 0) return;
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    if ((event.target as HTMLElement).closest('a, button')) return;
    event.preventDefault();
    onOpenRequest(request.id);
  }

  function openFromKeyboard(event: KeyboardEvent, request: PendingCIRequest): void {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    if ((event.target as HTMLElement).closest('a, button')) return;
    event.preventDefault();
    onOpenRequest(request.id);
  }

  function clearFilters(): void {
    search = '';
    query = '';
    states = [];
    schedules = [];
  }
</script>

<RootPageHeader
  role={rootRole}
  title="Queue"
  subtitle="Work the service is holding until it can act"
>
  <SegmentedControl
    name="queue-section"
    label="Queue section"
    compact
    options={[
      { value: 'waiting', label: 'Waiting', tone: 'accent', badge: waiting.length },
      { value: 'recent', label: 'Recent', tone: 'accent' },
    ]}
    value={section}
    onSelect={(next) => onSection(next as QueueSection)}
  />
</RootPageHeader>

<div class="queue-toolbar">
  <SearchField
    label="Search the queue"
    placeholder="Repository, number or author"
    value={search}
    onInput={(next) => (search = next)}
  />
  <!-- The same two filters the headings carry, for the widths where there are no
       headings to carry them. Below 48rem the table is a stack of cards and the
       band is hidden, and both menus live inside it - so the page offered a
       search field and nothing else. Sharing the state rather than a copy of it,
       and no sorts: the queue is ordered by what happens next, not by a column.
       -->
  <TableToolsMenu
    label="Filter the queue"
    sorts={[]}
    filters={[
      {
        label: section === 'recent' ? 'Outcome' : 'Checks',
        hint:
          section === 'recent' ? 'Select one or more outcomes' : 'Select one or more check states',
        sections: section === 'recent' ? OUTCOME_FILTERS : STATE_FILTERS,
        selected: states,
        multiple: true,
        onChange: (values) => (states = values),
      },
      {
        label: 'Schedule',
        hint: 'Select one or more schedules',
        sections: SCHEDULE_FILTERS,
        selected: schedules,
        multiple: true,
        onChange: (values) => (schedules = values),
      },
    ]}
  />
  <span class="toolbar-spacer"></span>
  <!-- What actually moves this queue along, said once rather than implied by
       every row's timer. The timers are the safety net. -->
  <span class="status-pill"
    ><span class="status-pill-dot live"></span><span class="cap-trim">Webhook driven</span></span
  >
</div>

{#if problem !== null}
  <!-- Over the table rather than in place of it: a refresh that fails has not
       made the rows already on screen wrong. -->
  <ResultProblem
    title="The queue could not be read"
    {problem}
    onRetry={() => void load()}
    busy={loading}
    overContent={rows.length > 0}
  />
{/if}

<div class="table-card queue-card">
  <table
    class="queue-table"
    class:waiting-table={section === 'waiting'}
    class:recent-table={section === 'recent'}
  >
    <thead>
      <tr>
        <th scope="col">
          <div class="heading-layout">
            <span class="heading-label band-trim"
              >{section === 'recent' ? 'Outcome' : 'Checks'}</span
            >
            <FilterMenu
              label={section === 'recent' ? 'Outcome' : 'Checks'}
              summary={states.length === 0 ? 'All states' : `${states.length} selected`}
              hint={section === 'recent'
                ? 'Cancelled is not a failure: somebody chose it'
                : 'Filter by what the checks last said'}
              sections={section === 'recent' ? OUTCOME_FILTERS : STATE_FILTERS}
              selected={states}
              multiple
              align="start"
              onChange={(values) => (states = values)}
            />
          </div>
        </th>
        <th scope="col"><span class="heading-label band-trim">Pull request</span></th>
        {#if section === 'recent'}
          <th scope="col" class="cleanup-column"
            ><span class="heading-label band-trim">Cleanup</span></th
          >
          <th scope="col"><span class="heading-label band-trim">Why it ended</span></th>
          <th scope="col"><span class="heading-label band-trim">Finished</span></th>
        {:else}
          <th scope="col">
            <div class="heading-layout">
              <span class="heading-label band-trim">Next</span>
              <FilterMenu
                label="Schedule"
                summary={schedules.length === 0 ? 'Any schedule' : `${schedules.length} selected`}
                hint="Deferred means nothing has moved for an hour"
                sections={SCHEDULE_FILTERS}
                selected={schedules}
                multiple
                align="start"
                onChange={(values) => (schedules = values)}
              />
            </div>
          </th>
          <th scope="col"><span class="heading-label band-trim">Armed</span></th>
        {/if}
        <th scope="col"><span class="visually-hidden">Actions</span></th>
      </tr>
    </thead>
    <tbody>
      {#each rows as request (request.id)}
        {@const state = section === 'waiting' ? queueState(request) : outcomeState(request)}
        {@const next = queueNext(request, now)}
        <tr
          class="queue-row"
          tabindex="0"
          onclick={(event) => openRow(event, request)}
          onkeydown={(event) => openFromKeyboard(event, request)}
        >
          <td data-label={section === 'recent' ? 'Outcome' : 'Checks'}>
            <Chip tone={state.tone} icon={state.icon}>{state.label}</Chip>
          </td>
          <td data-label="Pull request">
            <a
              class="pr-name"
              href={githubHref(request)}
              rel="noreferrer"
              target="_blank"
              title={`${request.repository_full_name} #${request.pull_request} on GitHub`}
            >
              <span class="pr-owner">{ownerOf(request)}</span>
              <span class="pr-repo">{repositoryOf(request)}</span>
              <span class="pr-num">#{request.pull_request}</span>
              <Icon name="link" size={14} strokeWidth={2} />
            </a>
            <div class="pr-meta">
              <span class="contract">{contractOf(request)}</span>
              <span class="sep" aria-hidden="true">·</span>
              <span class="sha">{request.head_sha.slice(0, 8)}</span>
              <span class="sep" aria-hidden="true">·</span>
              <span>@{request.requester}</span>
            </div>
          </td>
          {#if section === 'recent'}
            {@const cleanup = cleanupState(request)}
            <!-- The words go and the mark stays below 64rem, with the whole
                 sentence on the tooltip: there is usually room to say "Cleanup
                 failed" in full, and where there is not, a mark that can be
                 hovered beats a truncated word. -->
            <AppTooltip text={cleanup.detail}>
              {#snippet children(props)}
                <td {...props} class="cleanup-column" data-label="Cleanup">
                  <Chip tone={cleanup.tone} icon={cleanup.icon} small>
                    <span class="cleanup-label">{cleanup.label}</span>
                  </Chip>
                </td>
              {/snippet}
            </AppTooltip>
            <td data-label="Why it ended"><div class="reason">{endReason(request)}</div></td>
            <td data-label="Finished">
              <span
                class="age band-trim"
                title={formatTimestamp(request.finished_at ?? request.updated_at)}
                >{shortAge(request.finished_at ?? request.updated_at, now)}</span
              >
            </td>
          {:else}
            <td data-label="Next">
              <div
                class="next-lead"
                class:due={next.merging}
                class:idle={!next.merging}
                class:imminent={next.merging && next.seconds !== null && next.seconds <= 10}
                class:final={next.merging && next.seconds !== null && next.seconds <= 5}
              >
                {#if next.merging && next.seconds !== null}
                  <!-- The quiet period drawn as what it is: a wait with an end.
                       Circumference is 2πr at r=5.6, so the dash offset is the
                       part already spent - the ring empties as the merge nears.
                       Sized in `cap` units so it sits in the band of the words
                       beside it rather than making the line taller than them. -->
                  <svg class="ring" viewBox="0 0 14 14" fill="none" aria-hidden="true">
                    <circle
                      cx="7"
                      cy="7"
                      r="5.6"
                      stroke="currentColor"
                      stroke-opacity="0.25"
                      stroke-width="1.8"
                    />
                    <circle
                      cx="7"
                      cy="7"
                      r="5.6"
                      stroke="currentColor"
                      stroke-width="1.8"
                      stroke-linecap="round"
                      stroke-dasharray="35.2"
                      stroke-dashoffset={35.2 * (1 - Math.min(1, next.seconds / QUIET_SECONDS))}
                      transform="rotate(-90 7 7)"
                    />
                  </svg>
                {/if}
                <span class="band-trim">{next.lead}</span>
              </div>
              <div class="next-sub">{next.sub}</div>
            </td>
            <td data-label="Armed">
              <span class="age band-trim" title={formatTimestamp(request.requested_at)}
                >{shortAge(request.requested_at, now)}</span
              >
            </td>
          {/if}
          <td class="row-actions" data-label="Actions">
            <ActionMenu
              label={`Actions for ${request.repository_full_name} #${request.pull_request}`}
              items={actionsFor(request)}
              onSelect={(action) => void choose(request, action)}
            />
          </td>
        </tr>
      {:else}
        <tr class="empty-row">
          <td class="empty-cell" colspan={section === 'recent' ? 6 : 5}>
            <!-- Qualified with `!loaded`: a refresh that is in flight over rows
                 already on screen must not replace them with a placeholder, and
                 an empty queue that HAS loaded is a real answer rather than a
                 wait. `tests/loading-placeholders.test.ts` asks for this. -->
            {#if loading && !loaded}
              Reading the queue…
            {:else}
              <TableEmptyState
                title={hasFilters
                  ? 'Nothing matches'
                  : section === 'recent'
                    ? 'Nothing has finished yet'
                    : 'Nothing is waiting'}
                description={hasFilters
                  ? 'No request here matches these filters'
                  : section === 'recent'
                    ? 'Requests appear here once they merge, are cancelled, or are replaced'
                    : 'Every armed merge has been dealt with'}
                actionLabel={hasFilters ? 'Clear filters' : undefined}
                onAction={hasFilters ? clearFilters : undefined}
              />
            {/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  /* Stated once and read everywhere below: the row a two-line cell rests in, and
     the baseline-to-cap gap inside such a cell - a real gap now that both boxes
     end where their letters do. */
  .queue-card {
    --row-height: 3.75rem;
    --line-gap: 0.5rem;
  }

  .queue-toolbar {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    padding: var(--space-4) 0;
  }

  .toolbar-spacer {
    flex: 1;
  }

  .queue-toolbar :global(.search-field) {
    flex: none;
    width: 20rem;
  }

  /* Only where the column headings are not. They carry the same two filters while
     the table is a table, and two ways to set one value is one way too many - the
     menu exists because the headings go away, not instead of them. */
  .queue-toolbar :global(.tools-trigger) {
    display: none;
  }

  .queue-table {
    /* Separated, not collapsed: a collapsed border is shared between adjacent
       rows, so each cell owns half of it and every row box lands on a .5. */
    border-collapse: separate;
    border-spacing: 0;
    table-layout: fixed;
    width: 100%;
  }

  .queue-table th {
    height: 2.5rem;
    padding: 0 var(--space-3);
  }

  /* Column widths as rules rather than a `<colgroup>`: a `style` attribute in
     Svelte markup is silently dropped by the panel's `style-src 'self'`, so the
     table would have laid itself out however it liked in production while
     looking right in development. `tests/csp-safety.test.ts` catches it.

     The pull request takes what is left - it is the one column whose content
     has no bound - and every other column is the width of its own worst case.
     The two sections carry different columns, so each states its own. */
  .queue-table :is(th, td):first-child {
    width: 9.5rem;
  }

  .waiting-table :is(th, td):nth-child(3) {
    width: 13.5rem;
  }

  .waiting-table :is(th, td):nth-child(4) {
    width: 6.5rem;
  }

  .waiting-table :is(th, td):nth-child(5) {
    width: 5rem;
  }

  .recent-table :is(th, td):nth-child(3) {
    width: 9.5rem;
  }

  .recent-table :is(th, td):nth-child(4) {
    width: 15rem;
  }

  .recent-table :is(th, td):nth-child(5) {
    width: 6.5rem;
  }

  .recent-table :is(th, td):nth-child(6) {
    width: 3.5rem;
  }

  .queue-table th:first-child,
  .queue-table td:first-child {
    padding-left: var(--space-4);
  }

  .queue-table th:last-child,
  .queue-table td:last-child {
    padding-right: var(--space-4);
  }

  /* A block-level flex row, so the cell holds no anonymous line box and the
     table's own font cannot place the baseline. An inline label in a 15px cell
     sat 1.81px below where its 13px box said it was. */
  .heading-layout {
    align-items: center;
    display: flex;
    gap: 0.3rem;
  }

  .heading-layout :global(.header-filter) {
    margin-inline-start: auto;
  }

  /* The row height is stated, and the content is centred in it. Padding is the
     floor for a cell that outgrows the row, not the thing that sets its size. */
  .queue-table td {
    border-bottom: 1px solid var(--rule);
    height: var(--row-height);
    padding: var(--space-2) var(--space-3);
    vertical-align: middle;
  }

  .queue-table tbody tr:last-child td {
    border-bottom: 0;
  }

  .queue-row {
    cursor: pointer;
    transition: background-color var(--duration-fast) var(--ease-out);
  }

  .queue-row:hover {
    background: var(--table-row-hover);
  }

  .queue-row:active {
    background: var(--table-row-pressed);
  }

  .queue-row:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: -2px;
  }

  /* Both lines are flex rows of trimmed boxes on a shared baseline, so each
     row's box IS its band and the stack has no slack at either end. Centre the
     stack and the letters are centred. */
  .pr-name {
    align-items: baseline;
    border-radius: var(--r-chip);
    color: inherit;
    display: flex;
    font-size: var(--font-size-meta);
    gap: 0.15rem;
    min-width: 0;
    text-decoration: none;
    width: fit-content;
  }

  .pr-name > :global(*) {
    line-height: 1;
    text-box: trim-both cap alphabetic;
  }

  .pr-owner {
    color: var(--dim);
    flex: none;
  }

  .pr-repo {
    color: var(--text);
    font-weight: 700;
  }

  .pr-owner,
  .pr-repo,
  .contract {
    min-width: 0;
    /* `clip` and not `hidden`: the trim ends these boxes on the baseline, so
       `hidden` would shave the tail off every g, p and y. */
    overflow: clip;
    overflow-clip-margin: 0.4em;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .pr-num {
    color: var(--text-soft);
    flex: none;
    font-weight: 600;
    margin-left: 0.15rem;
  }

  /* The mark that appears on hover says "this opens GitHub", so the thing it
     marks has to answer the pointer too - and answer a press differently from
     a hover, which is the part that was missing. */
  .pr-name:hover .pr-repo,
  .pr-name:hover .pr-num,
  .pr-name:hover .pr-owner {
    color: var(--accent);
  }

  .pr-name:hover .pr-repo {
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .pr-name:active {
    transform: scale(var(--press-scale-compact));
    transform-origin: left center;
  }

  .pr-name:active .pr-repo,
  .pr-name:active .pr-num,
  .pr-name:active .pr-owner {
    color: var(--brand-action-hover);
  }

  /* One cap tall, so the mark sits exactly in the band it stands beside and the
     line it is on stays the height of its own letters. */
  .pr-name :global(svg) {
    block-size: 1cap;
    color: var(--dim);
    flex: none;
    inline-size: 1cap;
    margin-left: 0.35rem;
    opacity: 0;
    transition: opacity var(--duration-fast) var(--ease-out);
  }

  .queue-row:hover .pr-name :global(svg),
  .queue-row:focus-within .pr-name :global(svg) {
    color: var(--accent);
    opacity: 1;
  }

  /* One line, always. It used to wrap when a request counted required checks
     only, and a row that grows by a line breaks the scan. The merge contract
     is the part that gives way: the commit and the author are what someone
     needs to act, so they never shrink. */
  .pr-meta {
    align-items: baseline;
    color: var(--dim);
    display: flex;
    flex-wrap: nowrap;
    font-size: var(--font-size-compact);
    gap: 0.3rem;
    margin-top: var(--line-gap);
    min-width: 0;
  }

  .pr-meta > :global(*) {
    flex: none;
    line-height: 1;
    text-box: trim-both cap alphabetic;
  }

  .pr-meta .contract {
    flex: 0 1 auto;
  }

  /* Same size as the words beside it: at a smaller size the two runs sit on one
     baseline but their cap bands no longer share a centre. */
  .pr-meta .sha {
    font-family: var(--mono);
    letter-spacing: -0.01em;
  }

  .pr-meta .sep {
    opacity: 0.55;
  }

  .next-lead {
    align-items: baseline;
    display: flex;
    font-size: var(--font-size-meta);
    font-weight: 600;
    gap: 0.4rem;
  }

  /* One cap tall, like the link mark: the ring belongs in the band of the words
     it stands beside, not above and below them. `overflow: visible` so the round
     cap of the stroke is not clipped by the 14-unit box it is drawn in. */
  .ring {
    block-size: 1cap;
    flex: none;
    inline-size: 1cap;
    overflow: visible;
  }

  .next-lead.due {
    color: var(--clear);
    transition: color var(--duration-normal) var(--ease-out);
  }

  .next-lead.idle {
    color: var(--text-soft);
  }

  /* Two steps, because they say different things. Under ten seconds it is
     close: the tone leaves success for attention. Under five it is the last
     moment to stop it: the tone goes to danger and it starts to blink. Amber
     first and not danger - nothing is wrong, the merge is about to happen on
     purpose - and the state chip beside it still says the checks passed, so the
     two are never read as one claim. */
  .next-lead.due.imminent {
    color: var(--warning);
  }

  .next-lead.due.final {
    animation: countdown-pulse 700ms var(--ease-out) infinite alternate;
    color: var(--stop);
  }

  @keyframes countdown-pulse {
    from {
      opacity: 1;
    }

    to {
      opacity: 0.35;
    }
  }

  .next-sub {
    color: var(--dim);
    font-size: var(--font-size-compact);
    margin-top: var(--line-gap);
    text-box: trim-both cap alphabetic;
  }

  .age {
    color: var(--text-soft);
    display: block;
    font-size: var(--font-size-meta);
  }

  .reason {
    color: var(--dim);
    font-size: var(--font-size-compact);
    text-box: trim-both cap alphabetic;
  }

  /* The label goes and the mark stays where the column has run out of room. The
     whole sentence is on the cell's tooltip either way, so nothing is lost by
     the words leaving - and a mark that can be hovered beats a word cut short. */
  @media (max-width: 64rem) {
    .cleanup-label {
      display: none;
    }

    .queue-table :is(th, td).cleanup-column {
      width: 3.5rem;
    }
  }

  /* A phone gets the cards the other tables give it, for the reason measured in
     `tests/browser/mobile-layout.test.ts`: these columns are stated in rem and a
     fixed table will not go under their sum, so the waiting table asked for
     498px and the recent one 578px on a 375px screen. Chrome does not scroll
     that sideways - it widens the layout viewport and scales the console down,
     which took it to 75% and 65%. The headings are already on every cell as
     `data-label`, so the band can go and the labels stay. */
  @media (max-width: 48rem) {
    /* The search takes the line and the pill drops under it. The spacer that
       holds them apart on a wide screen is what would keep them on one line
       here, and `Webhook driven` does not wrap, so the pair set the width of the
       whole console rather than giving way. */
    .queue-toolbar {
      flex-wrap: wrap;
    }

    .toolbar-spacer {
      display: none;
    }

    /* The search gives up the width the menu needs, rather than the menu dropping
       to a line of its own beside an empty half. */
    .queue-toolbar :global(.search-field) {
      flex: 1;
      width: auto;
    }

    .queue-toolbar :global(.tools-trigger) {
      display: inline-flex;
    }

    .queue-table {
      min-width: 0;
      table-layout: auto;
    }

    .queue-table thead {
      display: none;
    }

    /* Every row, not only `.queue-row`. The empty row left as a `table-row` inside
       a block tbody gets an anonymous table of its own and shrinks to fit its
       words, so the card's default state - an idle queue - sat flush left in a
       box narrower than the card, centring its text in the wrong one. */
    .queue-table,
    .queue-table tbody,
    .queue-table tr,
    .queue-table td {
      display: block;
      width: 100%;
    }

    .empty-cell {
      /* `colspan` means nothing once the cells are blocks; the row is the width. */
      display: flex;
      justify-content: center;
    }

    /* Every column width above is stated against a band that is gone; left
       standing they would size the cards instead. */
    .queue-table :is(th, td):first-child,
    .waiting-table :is(th, td):nth-child(3),
    .waiting-table :is(th, td):nth-child(4),
    .waiting-table :is(th, td):nth-child(5),
    .recent-table :is(th, td):nth-child(3),
    .recent-table :is(th, td):nth-child(4),
    .recent-table :is(th, td):nth-child(5),
    .recent-table :is(th, td):nth-child(6) {
      width: auto;
    }

    .queue-row {
      border-bottom: 1px solid var(--border-subtle);
      padding: var(--space-3);
    }

    .queue-row td {
      align-items: center;
      border: 0;
      display: flex;
      gap: var(--space-3);
      justify-content: space-between;
      min-height: calc(var(--control-height-compact) + var(--space-2));
      padding: var(--space-1) 0;
      text-align: left;
    }

    .queue-row td[data-label]::before {
      color: var(--text-muted);
      content: attr(data-label);
      flex: none;
      font-size: var(--font-size-compact);
      font-weight: 650;
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }

    /* The pull request is the card's heading rather than a labelled row: it is
       what the card is about, and it is the one column whose text has no bound,
       so it wraps instead of being cut. */
    .queue-row td:nth-child(2) {
      border-bottom: 1px solid var(--border-subtle);
      display: block;
      padding-bottom: var(--space-3);
    }

    .queue-row td:nth-child(2)::before {
      content: none;
    }

    .cleanup-label {
      display: inline;
    }

    .queue-table :is(th, td).cleanup-column {
      width: auto;
    }
  }

  .row-actions {
    text-align: right;
  }

  .empty-cell {
    color: var(--dim);
    padding: var(--space-6) var(--space-4);
    text-align: center;
  }
</style>
