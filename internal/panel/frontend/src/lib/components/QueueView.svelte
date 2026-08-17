<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';

  import type { PanelApi } from '#lib/api.js';
  import type { FilterSection } from '#lib/filter-menu.js';
  import { formatTimestamp } from '#lib/format.js';
  import {
    cleanupState,
    endReason,
    outcomeState,
    queueNext,
    queueState,
    shortAge,
  } from '#lib/queue.js';
  import type { PendingCIRequest } from '#lib/types.js';
  import ActionMenu, { type ActionMenuItem } from './ActionMenu.svelte';
  import AppTooltip from './AppTooltip.svelte';
  import Chip from './Chip.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import SortIndicator from './SortIndicator.svelte';
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
     alone, which three of these six pairs cannot survive under one dichromacy or
     another.

     Every state the column can DRAW is offered, which is what the last entry is
     for: a request carries no observed state between the command and its first
     reconciliation, the column reads that as "Scheduled", and the menu offered
     five values while the table showed six. `tests/queue-vocabulary.test.ts`
     compares the two lists so they cannot part again. Filtering is on the value
     the service sent, so the empty string is the value here - a state the panel
     does not recognise draws as Scheduled too, and is not reachable from this
     menu, which is the honest answer: there is nothing to name it by. */
  const STATE_FILTERS = [
    {
      options: [
        { value: 'passing', label: 'Passing', tone: 'valid', icon: 'success' },
        { value: 'pending', label: 'Running', tone: 'neutral', icon: 'pending' },
        { value: 'failing', label: 'Failing', tone: 'invalid', icon: 'failure' },
        { value: 'indeterminate', label: 'Unreadable', tone: 'bypassed', icon: 'alert' },
        { value: 'no_checks', label: 'No checks', tone: 'missing', icon: 'minus-circle' },
        { value: '', label: 'Scheduled', tone: 'neutral', icon: 'circle-dashed' },
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
        { value: 'cancelled', label: 'Cancelled', tone: 'neutral', icon: 'circle-dashed' },
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
  /* Chips, because the Cleanup column draws chips, and the same three words. */
  const CLEANUP_FILTERS = [
    {
      options: [
        { value: 'done', label: 'Done', tone: 'valid', icon: 'check' },
        { value: 'pending', label: 'Pending', tone: 'bypassed', icon: 'pending' },
        { value: 'failed', label: 'Failed', tone: 'invalid', icon: 'alert' },
      ],
    },
  ] satisfies readonly FilterSection[];

  /**
   * What each section can be ordered by, and what each column means as an order.
   *
   * Only the columns whose values HAVE an order: a repository name sorts as well as any string
   * and says nothing, and the reason a request ended is a sentence. The two the table opens on -
   * what happens soonest, and what finished last - are the two the mock marks as sorted.
   */
  const ORDERS = {
    checks: (request: PendingCIRequest) => queueState(request).label,
    next: (request: PendingCIRequest) => request.next_check_at,
    armed: (request: PendingCIRequest) => request.requested_at,
    outcome: (request: PendingCIRequest) => outcomeState(request).label,
    cleanup: (request: PendingCIRequest) => cleanupState(request).label,
    finished: (request: PendingCIRequest) => request.finished_at ?? request.updated_at,
  } as const;

  type SortColumn = keyof typeof ORDERS;

  interface SectionOrder {
    columns: readonly SortColumn[];
    opens: SortColumn;
    ascending: boolean;
  }

  /** Which of them each section actually draws, and the one it opens on. */
  const SECTION_ORDERS: Record<QueueSection, SectionOrder> = {
    waiting: { columns: ['checks', 'next', 'armed'], opens: 'next', ascending: true },
    recent: { columns: ['outcome', 'cleanup', 'finished'], opens: 'finished', ascending: false },
  };

  let search = $state('');
  let query = $state('');
  let states = $state<string[]>([]);
  let schedules = $state<string[]>([]);
  let pullRequests = $state<string[]>([]);
  let cleanups = $state<string[]>([]);
  /* The order each section opens on: soonest first for what is still to happen, newest first for
     what already has. Reading the waiting order onto the past puts the oldest outcome at the top,
     which is the opposite of what somebody scanning for "what just happened" wants. */
  let sortColumn = $state<SortColumn>('next');
  let ascending = $state(true);
  let pendingAction = $state<string | null>(null);
  /* Ticks so a countdown counts. One second, because the last ten of a merge are
     the point of the column and a minute's granularity would miss them. */
  let now = $state(Date.now());

  /** What this section is holding, which is what the section's own filters offer. */
  const section_rows = $derived<PendingCIRequest[]>(section === 'waiting' ? waiting : recent);

  /**
   * The four things a Pull request cell says, filtered from where they are drawn.
   *
   * The cell is not one value: it is the repository and the number on the first line, then the
   * merge contract, the commit and whoever armed it on the second. A menu on that heading that
   * offered only the repository could not find "everything @lin has rebasing", which is the
   * question the cell is laid out to answer.
   *
   * The values are namespaced because one menu carries one selection, and the facets have to AND
   * across while OR-ing within: two repositories and one author means either repository, by that
   * author. Everything except the contract is taken from the data, so nothing is offered that
   * selects nothing, and no list needs keeping the day a workspace gains a repository.
   */
  function facet(prefix: string, values: readonly string[], label?: string): FilterSection {
    return {
      ...(label === undefined ? {} : { label }),
      options: [...new Set(values)]
        .sort((first, second) => first.localeCompare(second))
        .map((value) => ({ value: `${prefix}:${value}`, label: value })),
    };
  }

  const PULL_REQUEST_FILTERS = $derived<readonly FilterSection[]>([
    facet(
      'repository',
      section_rows.map((request) => request.repository_full_name),
      'Repository',
    ),
    facet(
      'method',
      section_rows.map(
        (request) =>
          `${request.merge_method.slice(0, 1).toUpperCase()}${request.merge_method.slice(1)}`,
      ),
      'Merge method',
    ),
    facet(
      'author',
      section_rows.map((request) => `@${request.requester}`),
      'Armed by',
    ),
    facet(
      'checks',
      section_rows.map((request) =>
        request.required_checks_only ? 'Required only' : 'All checks',
      ),
      'Checks it waits on',
    ),
  ]);

  /** What a request answers for each facet, in the same words the menu offers. */
  function facetsOf(request: PendingCIRequest): string[] {
    const method = `${request.merge_method.slice(0, 1).toUpperCase()}${request.merge_method.slice(1)}`;
    return [
      `repository:${request.repository_full_name}`,
      `method:${method}`,
      `author:@${request.requester}`,
      `checks:${request.required_checks_only ? 'Required only' : 'All checks'}`,
    ];
  }

  /**
   * Selected within a facet is OR, across facets is AND.
   *
   * A flat `includes` would have made every facet an OR of everything, so choosing a repository and
   * an author would widen the table rather than narrow it.
   */
  function matchesPullRequest(request: PendingCIRequest): boolean {
    if (pullRequests.length === 0) return true;
    const answers = facetsOf(request);
    const prefixes = [...new Set(pullRequests.map((one) => one.slice(0, one.indexOf(':'))))];

    return prefixes.every((prefix) =>
      pullRequests
        .filter((one) => one.startsWith(`${prefix}:`))
        .some((one) => answers.includes(one)),
    );
  }

  const rows = $derived.by(() => {
    const needle = query.trim().toLocaleLowerCase();
    return section_rows
      .filter(
        (request) =>
          states.length === 0 ||
          states.includes(section === 'recent' ? request.lifecycle : request.last_observed_state),
      )
      .filter((request) => schedules.length === 0 || schedules.includes(request.schedule))
      .filter(matchesPullRequest)
      .filter((request) => cleanups.length === 0 || cleanups.includes(cleanupState(request).value))
      .filter(
        (request) =>
          needle === '' ||
          [request.repository_full_name, `#${request.pull_request}`, request.requester].some(
            (field) => field.toLocaleLowerCase().includes(needle),
          ),
      )
      .sort((first, second) => {
        const read = ORDERS[sortColumn];
        const order = read(first).localeCompare(read(second));
        /* A stable tie-break, so two requests that share an order do not swap places on every
           tick of the countdown clock. */
        return (ascending ? order : -order) || first.id.localeCompare(second.id);
      });
  });

  const hasFilters = $derived(
    query !== '' ||
      states.length > 0 ||
      schedules.length > 0 ||
      pullRequests.length > 0 ||
      cleanups.length > 0,
  );

  /* The two sections carry different columns, so an order set on one can be an order the other
     does not draw - and a table sorted by a heading that is not on it has no way back. Switching
     sections therefore returns to that section's own opening order, and only when the current one
     is not among its columns: a reader who sorted Recent by Outcome and stepped away keeps it. */
  $effect(() => {
    const own = SECTION_ORDERS[section];
    if (own.columns.includes(untrack(() => sortColumn))) return;
    sortColumn = own.opens;
    ascending = own.ascending;
  });

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

  /**
   * Whether a request can still be acted on.
   *
   * Held while a mutation is in flight, because every action carries the revision it was drawn
   * with. A second Cancel sends the same one, the row has already moved past it, and the store
   * answers 409 - so the reader is shown a red banner over a cancel that worked.
   */
  function actionable(request: PendingCIRequest): boolean {
    return request.lifecycle === 'armed' && pendingAction === null;
  }

  /** The key `pendingAction` holds while this request's own check is in flight. */
  function checkKey(request: PendingCIRequest): string {
    return `check:${request.id}`;
  }

  /* Destructive weight is inverted from the old panel: a filled danger button
     appears once, on the confirmation, and never in a row. Here Cancel is a menu
     item like any other, and it is simply not offered on a request that has
     already finished. */
  function actionsFor(request: PendingCIRequest): ActionMenuItem[] {
    const armed = actionable(request);
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
    const key = action === 'check' ? checkKey(request) : `${action}:${request.id}`;
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

  /**
   * A press on the column already sorted turns it round; a press on another takes it over.
   *
   * A new column starts ascending except where descending is what the column is for: the last
   * thing to finish is what somebody scanning the past is looking for, and offering it oldest-first
   * makes them press twice to get there.
   */
  function toggleSort(column: SortColumn): void {
    if (sortColumn === column) {
      ascending = !ascending;
      return;
    }
    sortColumn = column;
    ascending = column !== 'finished';
  }

  function sortDirection(column: SortColumn): 'ascending' | 'descending' | undefined {
    if (sortColumn !== column) return undefined;

    return ascending ? 'ascending' : 'descending';
  }

  function clearFilters(): void {
    search = '';
    query = '';
    states = [];
    schedules = [];
    pullRequests = [];
    cleanups = [];
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
  {#if hasFilters}
    <!-- Only while there is something to clear, and quiet while it is there: the
         way out of a narrowed table should be beside the thing that narrowed it,
         not somewhere in the headings, and it should not compete with the search
         for attention. `btn-quiet` is the product's own no-fill, no-border
         variant - it is a word until it is hovered. -->
    <button class="btn btn-quiet btn-row" type="button" onclick={clearFilters}>
      <span class="button-label">Clear filters</span>
    </button>
  {/if}
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

<div class="table-card queue-card">
  {#if problem !== null && rows.length > 0}
    <!-- Inside the card, above the headings: a refresh that fails has not made
         the rows already on screen wrong, so the failure is a band over the table
         it belongs to rather than a slab floating above the card. -->
    <ResultProblem
      title="The queue could not be read"
      {problem}
      onRetry={() => void load()}
      busy={loading}
      overContent
    />
  {/if}
  <table
    class="queue-table"
    class:waiting-table={section === 'waiting'}
    class:recent-table={section === 'recent'}
  >
    <thead>
      <tr>
        <th scope="col" aria-sort={sortDirection(section === 'recent' ? 'outcome' : 'checks')}>
          <div class="table-heading">
            <button
              class="table-sort-button"
              type="button"
              onclick={() => toggleSort(section === 'recent' ? 'outcome' : 'checks')}
            >
              <span class="table-heading-label">{section === 'recent' ? 'Outcome' : 'Checks'}</span>
              <SortIndicator />
            </button>
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
        <th scope="col">
          <div class="table-heading">
            <span class="table-heading-label">Pull request</span>
            <!-- Everything the cell under it says, in four sections: the
                 repository, the merge contract, whoever armed it and what it is
                 waiting on. A menu here that offered only the repository could not
                 answer "everything @lin has rebasing", which is what having all of
                 it on one line is for. `wide`, because four sections of full
                 repository names do not fit the narrow layer. -->
            <FilterMenu
              label="Pull request"
              summary={pullRequests.length === 0 ? 'Everything' : `${pullRequests.length} selected`}
              hint="Only what this queue is holding"
              sections={PULL_REQUEST_FILTERS}
              selected={pullRequests}
              multiple
              wide
              align="start"
              onChange={(values) => (pullRequests = values)}
            />
          </div>
        </th>
        {#if section === 'recent'}
          <th scope="col" class="cleanup-column" aria-sort={sortDirection('cleanup')}>
            <div class="table-heading">
              <button class="table-sort-button" type="button" onclick={() => toggleSort('cleanup')}>
                <span class="table-heading-label">Cleanup</span>
                <SortIndicator />
              </button>
              <FilterMenu
                label="Cleanup"
                summary={cleanups.length === 0 ? 'Any state' : `${cleanups.length} selected`}
                hint="The label and the reactions the bot leaves behind"
                sections={CLEANUP_FILTERS}
                selected={cleanups}
                multiple
                align="start"
                onChange={(values) => (cleanups = values)}
              />
            </div>
          </th>
          <th scope="col">
            <div class="table-heading"><span class="table-heading-label">Why it ended</span></div>
          </th>
          <th scope="col" aria-sort={sortDirection('finished')}>
            <div class="table-heading">
              <button
                class="table-sort-button"
                type="button"
                onclick={() => toggleSort('finished')}
              >
                <span class="table-heading-label">Finished</span>
                <SortIndicator />
              </button>
            </div>
          </th>
        {:else}
          <th scope="col" aria-sort={sortDirection('next')}>
            <div class="table-heading">
              <button class="table-sort-button" type="button" onclick={() => toggleSort('next')}>
                <span class="table-heading-label">Next</span>
                <SortIndicator />
              </button>
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
          <!-- Sorted but not filtered: this column draws an age, which has an
               order and no values worth listing. Who armed it is filtered where
               the name is written, on the Pull request heading - the mock put it
               here, which would have been the same values behind a second trigger
               in a column that never shows them. -->
          <th scope="col" aria-sort={sortDirection('armed')}>
            <div class="table-heading">
              <button class="table-sort-button" type="button" onclick={() => toggleSort('armed')}>
                <span class="table-heading-label">Armed</span>
                <SortIndicator />
              </button>
            </div>
          </th>
        {/if}
        <th scope="col"><span class="visually-hidden">Actions</span></th>
      </tr>
    </thead>
    <tbody>
      {#each rows as request (request.id)}
        {@const state = section === 'waiting' ? queueState(request) : outcomeState(request)}
        {@const next = queueNext(request, now)}
        <tr
          class="queue-row data-row"
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
            <div class="row-action-group">
              <!-- The one action worth a press of its own. Reading the checks
                   again is what a reader comes to this table to do when a run
                   has just finished and the queue has not noticed yet, and
                   burying it under a menu costs two presses for the thing they
                   came for. It stays in the menu as well, where it can say what
                   it does. Only on a waiting request: nothing on the Recent
                   table has checks left to read. -->
              {#if section === 'waiting'}
                <IconButton
                  icon="refresh"
                  label={`Check ${request.repository_full_name} #${request.pull_request} now`}
                  disabled={!actionable(request)}
                  busy={pendingAction === checkKey(request)}
                  onclick={() => void choose(request, 'check')}
                />
              {/if}
              <ActionMenu
                label={`Actions for ${request.repository_full_name} #${request.pull_request}`}
                items={actionsFor(request)}
                onSelect={(action) => void choose(request, action)}
              />
            </div>
          </td>
        </tr>
      {:else}
        <tr class="empty-row">
          <td class="empty-cell" colspan={section === 'recent' ? 6 : 5}>
            <!-- One of three, never two. A queue that could not be read used to
                 put its failure above the card AND "Nothing has finished yet"
                 inside it, which are contradictory answers to the same question -
                 the reader was told both that the table is empty and that the
                 table is unknown.

                 Qualified with `!loaded`: a refresh that is in flight over rows
                 already on screen must not replace them with a placeholder, and
                 an empty queue that HAS loaded is a real answer rather than a
                 wait. `tests/loading-placeholders.test.ts` asks for this. -->
            {#if problem !== null}
              <ResultProblem
                title="The queue could not be read"
                {problem}
                onRetry={() => void load()}
                busy={loading}
              />
            {:else if loading && !loaded}
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

  /* The shape of a heading - no padding on the cell, the button carrying it, the
     filter riding over the target - is `thead th` and `.table-heading` in
     `app.css`, shared with the five other tables. All this table states is the
     band's height and the wider inset its outermost columns take. */
  .queue-table th {
    height: 2.5rem;
  }

  .queue-table th:first-child {
    --heading-pad-start: var(--space-4);
  }

  .queue-table th:last-child {
    --heading-pad-end: var(--space-4);
  }

  /* Column widths as rules rather than a `<colgroup>`: a `style` attribute in
     Svelte markup is silently dropped by the panel's `style-src 'self'`, so the
     table would have laid itself out however it liked in production while
     looking right in development. `tests/csp-safety.test.ts` catches it.

     The pull request takes what is left - it is the one column whose content has
     no bound - and every other column is the wider of what its heading needs
     with its own controls and what the widest value the SERVICE can produce
     needs, plus the cell's padding, rounded up to the next quarter rem. Every
     number below was measured that way in the browser rather than chosen, with
     each column's whole vocabulary put through it; `tests/browser/
     queue-columns.test.ts` measures the same thing again and fails if a value
     stops fitting.

     The two sections carry different columns AND different worst cases, so each
     states its own: 8.25rem where the widest state is "Unreadable", 8.5rem where
     the widest outcome is "Superseded". Holding them equal would put the
     difference at the front of every row of whichever section did not need it,
     which is the defect these numbers exist to end. */
  .waiting-table :is(th, td):first-child {
    width: 8.5rem;
  }

  .recent-table :is(th, td):first-child {
    width: 9.25rem;
  }

  /* "Checks again in 59 minutes" over "First look since it was armed". */
  .waiting-table :is(th, td):nth-child(3) {
    width: 12.25rem;
  }

  /* "just now" is wider than any age this abbreviates to, and this heading has
     no controls of its own. */
  .waiting-table :is(th, td):nth-child(4) {
    width: 5.75rem;
  }

  /* Two 1.75rem buttons, the gap between them, and the cell's own 12px and 16px:
     88px exactly. The mock states 5rem here and flex-shrinks its two buttons to
     24.8px wide to fit them, which leaves a pair of rounded rectangles where two
     squares were drawn. Better to give the column the 8px than to keep the
     number and lose the shape. */
  .waiting-table :is(th, td):nth-child(5) {
    width: 5.5rem;
  }

  /* The heading with its filter, wider than any of Done, Pending or Failed. */
  .recent-table :is(th, td):nth-child(3) {
    width: 8.75rem;
  }

  /* The one column here whose text has no bound, so it is sized like the
     repository name is: a floor with a stated reason rather than a worst case.
     12rem is where every reason the service can write today fits inside the two
     lines the row already has - the longest, "pull request merged outside
     pending CI reconciliation", needs 160px of content and gets 164px. One line
     for all of them would have taken 20.75rem. */
  .recent-table :is(th, td):nth-child(4) {
    width: 12rem;
  }

  /* The heading again: "Finished" is wider than "just now". */
  .recent-table :is(th, td):nth-child(5) {
    width: 6.5rem;
  }

  /* One button, and the same 12px and 16px the waiting table's pair get. */
  .recent-table :is(th, td):nth-child(6) {
    width: 3.5rem;
  }

  .queue-table td:first-child {
    padding-left: var(--space-4);
  }

  .queue-table td:last-child {
    padding-right: var(--space-4);
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

  /* Not clamped, and that is the measured answer rather than the lazy one. A
     `-webkit-line-clamp` needs `overflow: clip` with a clip margin, because
     `text-box: trim-both` ends this box at the baseline and a bare `hidden`
     shaves the descenders off - and the margin then inflates the box unevenly
     enough to put the cell 2.50px off the cells beside it, which
     `tests/browser/vertical-alignment.test.ts` reports in every row of this
     table. Nothing needs the clamp: the column is 12rem because that is where
     every reason the service writes fits inside two lines, so the cap is the
     WIDTH, and it is enforced by `tests/browser/queue-columns.test.ts`. */
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
    .waiting-table :is(th, td):first-child,
    .recent-table :is(th, td):first-child,
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

  /* Block-level and pushed over, not inline: an inline-flex inside a cell rides
     the table's own strut rather than the cell's centre, which is the trap the
     whole alignment sweep exists to close. */
  .row-action-group {
    display: flex;
    gap: var(--space-1);
    margin-left: auto;
    width: fit-content;
  }

  /* No vertical padding: what fills this cell brings its own air now, and the
     cell's own on top of it was room the empty state did not ask for. Qualified
     with the table, because `.queue-table td` sets the padding every cell shares
     and a bare `.empty-cell` loses to it. */
  .queue-table td.empty-cell {
    color: var(--dim);
    padding: 0 var(--space-4);
    text-align: center;
  }

  /* Except for the waiting line, which is a bare string with no component of its
     own to bring any. */
  .queue-table td.empty-cell:not(:has(*)) {
    padding-block: var(--space-8);
  }
</style>
