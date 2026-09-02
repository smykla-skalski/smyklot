<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { onMount, untrack } from 'svelte';
  import { useDebounce } from 'runed';
  import { SvelteURLSearchParams } from 'svelte/reactivity';
  import type { PanelApi } from '#lib/api.js';
  import { sentenceCase } from '#lib/format.js';
  import { LiveList } from '#lib/live-list.svelte.js';
  import { queueDetailKey, queueListKey, queueListScopeKey } from '#lib/queue-cache.js';
  import { QUEUE_SECTIONS, type QueueSection } from '#lib/routes.js';
  import type {
    QueueActionInput,
    QueueActionType,
    QueueDetail,
    QueueItem,
    QueuePage,
    QueuePriority,
    QueueSchedulePreview,
    QueueWorkload,
  } from '#lib/types.js';
  import { receipts } from '#lib/receipts.svelte.js';
  import { workloadTitle } from '#lib/workloads.js';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import Plate from './Plate.svelte';
  import QueueActionDialog from './QueueActionDialog.svelte';
  import QueueDetailDialog from './QueueDetailDialog.svelte';
  import QueueList from './QueueList.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import ListToolsMenu, { type ToolsFilter } from './ListToolsMenu.svelte';

  const {
    api,
    targetId,
    canControl = false,
    section = 'active',
    planHref,
    onOpenPlan,
    onSelectSection,
  }: {
    api: PanelApi;
    targetId?: string;
    canControl?: boolean;
    section?: QueueSection;
    /**
     * Where this workspace's sync plan is read and applied.
     *
     * A plan waiting for a decision carries the way to make it. Absent in the console,
     * which manages somebody else's sync through its own endpoints and has no plan
     * address to send anybody to.
     */
    planHref?: string;
    onOpenPlan?: (event: MouseEvent) => void;
    /**
     * Which of the queue's five views to show. Each is still its own address - the
     * segments change it - so a link straight to the decisions keeps working.
     */
    onSelectSection?: (value: QueueSection) => void;
  } = $props();

  /** What the queue calls finished, in every place that has to ask. */
  const DONE_STATES: QueueItem['state'][] = ['succeeded', 'failed', 'cancelled', 'superseded'];
  const LIVE_STATES: QueueItem['state'][] = [
    'running',
    'ready',
    'scheduled',
    'blocked',
    'retrying',
  ];
  /**
   * The three questions a reader has of a queue, in the order they have them.
   *
   * A decision is theirs to make and nothing moves until they make it, so it leads.
   * Then what the service is doing on its own, and last what it already did.
   */
  const CARDS = [
    { id: 'decision', states: ['awaiting_approval'] as QueueItem['state'][] },
    { id: 'live', states: LIVE_STATES },
    { id: 'done', states: DONE_STATES },
  ] as const;
  type CardID = (typeof CARDS)[number]['id'];
  /** A day, which is what "lately" means where finished work stands beside live work. */
  const DAY_MS = 86_400_000;
  /**
   * A MOVING BOUNDARY IS A NEW QUESTION EVERY MILLISECOND, and a new question is a new
   * cache key: seeded from `Date.now()` at mount, the done card's "in the last day" asked
   * the server again every time a reader came back to the Queue, because the boundary had
   * moved by the time it took them to walk away and return. Floored to the hour it is the
   * same question for an hour at a time - which is all the precision "the last day" ever
   * had - so returning to a view already read costs nothing, and the one refetch an hour
   * is the window genuinely having moved.
   */
  const HOUR_MS = 3_600_000;
  const emptyFacets: QueuePage['facets'] = {
    targets: [],
    repositories: [],
    profiles: [],
    states: [],
    workloads: [],
    priorities: [],
  };
  const queryClient = useQueryClient();
  let workload = $state<QueueWorkload | 'all'>('all');
  let priority = $state<QueuePriority | 'all'>('all');
  let stateFilter = $state('all');
  let profile = $state('all');
  let workspace = $state('all');
  let repository = $state('all');
  let timeRange = $state<'all' | '24h' | '7d'>('all');
  let announcement = $state('');
  let selected = $state<QueueItem | null>(null);
  let selectedAction = $state<QueueActionType | null>(null);
  let actionBusy = $state(false);
  let actionError = $state('');
  let detailOpen = $state(false);
  let detailItemID = $state<string | null>(null);
  let now = $state(Date.now());
  let rangeNow = $state(Date.now());
  /* HOW MUCH OF EACH CARD IS ON SCREEN, not which page of the queue is.
     A card is a list and a list counts itself: one number per card, raised by that
     card's own foot. Counting the whole query and printing it under whichever card
     came last said "1-4 of 4" beneath a card holding one row. */
  let revealed = $state({ decision: 0, live: 0, done: 0 });
  let search = $state('');
  /* What the page is actually filtered by, one step behind what is being typed: every
     keystroke is a request, and the queue is the one list where a reader is often
     hunting for a repository they can only half remember. */
  let appliedSearch = $state('');
  const pageSize = 50;
  /* One query per card, because one card is one list. The alternative - a single
     query grouped in the browser - cannot say how much of a group is on screen or
     bring more of one group down, which is what a foot inside a card promises. */
  const decisionAsk = $derived.by(() => cardQuery('decision'));
  const liveAsk = $derived.by(() => cardQuery('live'));
  const doneAsk = $derived.by(() => cardQuery('done'));

  const decisionQuery = createQuery(() => ({
    queryKey: queueListKey(targetId, decisionAsk),
    queryFn: () => fetchQueue(decisionAsk),
    enabled: shows('decision'),
  }));
  const liveQuery = createQuery(() => ({
    queryKey: queueListKey(targetId, liveAsk),
    queryFn: () => fetchQueue(liveAsk),
    enabled: shows('live'),
  }));
  const doneQuery = createQuery(() => ({
    queryKey: queueListKey(targetId, doneAsk),
    queryFn: () => fetchQueue(doneAsk),
    enabled: shows('done'),
  }));
  const detailQuery = createQuery(() => ({
    queryKey: queueDetailKey(targetId, detailItemID ?? ''),
    queryFn: () => fetchDetail(detailItemID),
    enabled: detailOpen && detailItemID !== null,
  }));

  const cardQueries = { decision: decisionQuery, live: liveQuery, done: doneQuery };
  /* The facets belong to the whole page rather than to one card, so they are read off
     whichever card the view leads with - the same filter answers for all of them. */
  const facets = $derived<QueuePage['facets']>(
    (shows('decision') ? decisionQuery.data?.facets : undefined) ??
      liveQuery.data?.facets ??
      doneQuery.data?.facets ??
      emptyFacets,
  );
  const answered = $derived(
    CARDS.filter((card) => shows(card.id)).every((card) => cardQueries[card.id].data !== undefined),
  );
  const loading = $derived(
    CARDS.filter((card) => shows(card.id)).some((card) => cardQueries[card.id].isFetching),
  );
  /* A REFRESH THAT ANSWERS QUICKLY SAYS NOTHING.
     ------------------------------------------------------------------------
     Most of these reads come back inside a frame or two, and a word that arrives and
     leaves inside 150ms is a flicker rather than a status - the eye catches the change
     and has nothing to read by the time it looks. So the say waits out the window in
     which a reader still experiences the app as instant, and once it HAS been said it
     stays long enough to be read, however fast the answer then arrives. Without the
     floor the delay only narrows the flicker's window rather than closing it.

     What it must never do is move anything already on the page. It is drawn inside the
     header's action group, which is packed to the right, so the word grows the group's
     left edge and the button it belongs to does not move a pixel. */
  const SAY_AFTER_MS = 400;
  const SAY_AT_LEAST_MS = 600;
  let updating = $state(false);
  let saidAt = 0;
  let waitTimer: ReturnType<typeof setTimeout> | null = null;
  let holdTimer: ReturnType<typeof setTimeout> | null = null;

  $effect(() => {
    const busy = loading;
    untrack(() => {
      if (busy) {
        if (holdTimer !== null) {
          clearTimeout(holdTimer);
          holdTimer = null;
        }
        if (updating || waitTimer !== null) return;
        waitTimer = setTimeout(() => {
          waitTimer = null;
          updating = true;
          saidAt = Date.now();
        }, SAY_AFTER_MS);
        return;
      }
      if (waitTimer !== null) {
        clearTimeout(waitTimer);
        waitTimer = null;
      }
      if (!updating || holdTimer !== null) return;
      holdTimer = setTimeout(
        () => {
          holdTimer = null;
          updating = false;
        },
        Math.max(0, SAY_AT_LEAST_MS - (Date.now() - saidAt)),
      );
    });
  });

  /* Reads nothing, so it runs once and its teardown is the component's. A cleanup on the
     effect above would fire on every change of `loading`, which is precisely when the
     timer it would clear is the one still counting. */
  $effect(() => () => {
    if (waitTimer !== null) clearTimeout(waitTimer);
    if (holdTimer !== null) clearTimeout(holdTimer);
  });

  const error = $derived(
    CARDS.filter((card) => shows(card.id))
      .map((card) => errorMessage(cardQueries[card.id].error))
      .find((message) => message !== '') ?? '',
  );
  const detail = $derived<QueueDetail | null>(detailQuery.data ?? null);
  const detailLoading = $derived(detailQuery.isFetching);
  const detailError = $derived(errorMessage(detailQuery.error));

  const workloads = $derived(facets.workloads);
  const profiles = $derived(facets.profiles);
  const workspaces = $derived(facets.targets);
  const repositories = $derived(facets.repositories);

  /* The queue speaks in target ids - the rows carry one and the facets are a list of
     them - and nobody reading the console knows a workspace by its id. The catalog is
     what turns one into a name, and the console has it cached already: the workspaces
     page and the overview both read this key. */
  const catalogQuery = createQuery(() => ({
    queryKey: ['root-workspaces'],
    queryFn: () => api.fetchRootWorkspaces(),
    enabled: targetId === undefined,
  }));
  const catalog = $derived(
    new Map((catalogQuery.data ?? []).map((row) => [row.id, row.account.display_name])),
  );

  function workspaceName(id: string): string {
    return catalog.get(id) ?? id;
  }

  /* Named on every row, because this page is every workspace at once: the filter above
     says "Every workspace" until somebody narrows it, and a row that names only its
     repository leaves a reader guessing whose repository it is. A workspace's own queue
     drops the name - there it is the only answer. */
  function rowWorkspace(item: QueueItem): string | null {
    if (targetId !== undefined || item.target_id === undefined) return null;

    return workspaceName(item.target_id);
  }
  const states = $derived(facets.states.filter((value) => sectionStates(section).includes(value)));
  /**
   * The cards a view is made of, each with its own rows, its own count and its own way
   * to bring more of itself down.
   */
  /* THE ROWS HOLD STILL UNTIL THE READER ASKS FOR THE NEW ONES.
     ------------------------------------------------------------------------
     These three cards are the live state of the service, and the service does not wait
     to be read: a job finishing took its row out from under a pointer and lifted every
     card below it. So each card draws the set it was given, with every row's contents
     read fresh from the live copy - a countdown still counts, a mark still changes - and
     what a new or departed row does is raise the count in the header instead.

     Overview holds its Active work card the same way, and says the same words. */
  const held = {
    decision: new LiveList<QueueItem>(
      () => decisionQuery.data?.items ?? [],
      (item) => item.id,
    ),
    live: new LiveList<QueueItem>(
      () => liveQuery.data?.items ?? [],
      (item) => item.id,
    ),
    done: new LiveList<QueueItem>(
      () => doneQuery.data?.items ?? [],
      (item) => item.id,
    ),
  };

  function takeAll(): void {
    for (const card of CARDS) held[card.id].refresh();
  }

  /* The reader's OWN changes are not something to be told about: a search, a section, a
     press of "Show more" all change which rows belong on the page, and holding those
     back would answer a press with a notice asking for a second one. */
  $effect(() => {
    void appliedSearch;
    void section;
    void revealed;
    untrack(takeAll);
  });

  const behind = $derived(
    CARDS.filter((card) => shows(card.id)).reduce((sum, card) => sum + held[card.id].changed, 0),
  );

  const cards = $derived(
    CARDS.filter((card) => shows(card.id))
      .map((card) => {
        const page = cardQueries[card.id].data;
        const rows = held[card.id].rows;
        const total = page?.total ?? 0;

        return {
          id: card.id,
          title: cardTitle(card.id),
          items: rows,
          count: total === 0 ? 'Nothing to show' : `Showing 1-${rows.length}\u{a0}of ${total}`,
          more: (page?.next_offset ?? 0) !== 0,
          busy: cardQueries[card.id].isFetching,
          onMore: () => (revealed = { ...revealed, [card.id]: revealed[card.id] + pageSize }),
        };
      })
      .filter((card) => card.items.length > 0),
  );
  const queueFilters = $derived<ToolsFilter[]>([
    {
      label: 'Job',
      hint: 'Show one kind of background work',
      sections: [
        {
          options: [
            { value: 'all', label: 'All jobs' },
            /* The job's own title, the one Schedules prints - underscores taken out
               of the wire name spelled "pending ci" and left it there. */
            ...workloads.map((kind) => ({ value: kind, label: workloadTitle(kind) })),
          ],
        },
      ],
      selected: [workload],
      fallbackValue: 'all',
      onChange: (values: string[]) => {
        workload = (values[0] ?? 'all') as QueueWorkload | 'all';
        revealed = { decision: 0, live: 0, done: 0 };
      },
    },
    {
      label: 'State',
      hint: 'Narrow the current Queue view',
      sections: [
        {
          options: [
            { value: 'all', label: 'All states' },
            ...states.map((value) => ({
              value,
              label: sentenceCase(value.replaceAll('_', ' ')),
            })),
          ],
        },
      ],
      selected: [stateFilter],
      fallbackValue: 'all',
      onChange: (values: string[]) => {
        stateFilter = values[0] ?? 'all';
        revealed = { decision: 0, live: 0, done: 0 };
      },
    },
    {
      label: 'Hours',
      hint: 'Filter by the hours a job runs in',
      sections: [
        {
          options: [
            { value: 'all', label: 'All hours' },
            ...profiles.map((value) => ({ value, label: value })),
          ],
        },
      ],
      selected: [profile],
      fallbackValue: 'all',
      onChange: (values: string[]) => {
        profile = values[0] ?? 'all';
        revealed = { decision: 0, live: 0, done: 0 };
      },
    },
    ...(targetId === undefined
      ? [
          {
            label: 'Workspace',
            hint: 'Limit work to one workspace',
            sections: [
              {
                options: [
                  { value: 'all', label: 'Every workspace' },
                  ...workspaces.map((value) => ({ value, label: workspaceName(value) })),
                ],
              },
            ],
            selected: [workspace],
            fallbackValue: 'all',
            onChange: (values: string[]) => {
              workspace = values[0] ?? 'all';
              revealed = { decision: 0, live: 0, done: 0 };
            },
          },
        ]
      : []),
    {
      label: 'Repository',
      hint: 'Limit work to one repository',
      sections: [
        {
          options: [
            { value: 'all', label: 'All repositories' },
            ...repositories.map((value) => ({ value, label: value })),
          ],
        },
      ],
      selected: [repository],
      fallbackValue: 'all',
      onChange: (values: string[]) => {
        repository = values[0] ?? 'all';
        revealed = { decision: 0, live: 0, done: 0 };
      },
    },
    {
      label: 'Created',
      hint: 'Limit work by creation time',
      sections: [
        {
          options: [
            { value: 'all', label: 'Any time' },
            { value: '24h', label: 'Last 24 hours' },
            { value: '7d', label: 'Last 7 days' },
          ],
        },
      ],
      selected: [timeRange],
      fallbackValue: 'all',
      onChange: (values: string[]) => {
        timeRange = (values[0] ?? 'all') as 'all' | '24h' | '7d';
        rangeNow = Date.now();
        revealed = { decision: 0, live: 0, done: 0 };
      },
    },
    {
      label: 'Priority',
      hint: 'Show one dispatch priority band',
      sections: [
        {
          options: [
            { value: 'all', label: 'All priorities' },
            { value: 'urgent', label: 'Urgent' },
            { value: 'high', label: 'High' },
            { value: 'normal', label: 'Normal' },
            { value: 'low', label: 'Low' },
          ],
        },
      ],
      selected: [priority],
      fallbackValue: 'all',
      onChange: (values: string[]) => {
        priority = (values[0] ?? 'all') as QueuePriority | 'all';
        revealed = { decision: 0, live: 0, done: 0 };
      },
    },
  ]);

  const debouncedSearch = useDebounce((query: string) => {
    appliedSearch = query;
    revealed = { decision: 0, live: 0, done: 0 };
  }, 250);

  $effect(() => {
    const value = search.trim();
    untrack(() => void debouncedSearch(value));
  });

  onMount(() => {
    const clock = window.setInterval(() => (now = Date.now()), 1_000);
    const rangeClock = window.setInterval(() => {
      if (timeRange !== 'all') rangeNow = Date.now();
    }, 60_000);
    return () => {
      window.clearInterval(clock);
      window.clearInterval(rangeClock);
    };
  });

  function errorMessage(cause: unknown): string {
    if (cause === null || cause === undefined) return '';
    return cause instanceof Error ? cause.message : String(cause);
  }

  async function fetchDetail(itemID: string | null): Promise<QueueDetail> {
    if (itemID === null) throw new Error('Queue item is no longer selected');
    return targetId === undefined
      ? api.fetchRootQueueItem(itemID)
      : api.fetchTargetQueueItem(targetId, itemID);
  }

  /* APPROVALS LEAD THE PAGE, so the page a reader arrives on carries them. They have
     their own address as well, for a link straight to the decision - what they do not
     have is a separate place you must know about before you can find out that anything
     is waiting for you. */
  function sectionStates(value: QueueSection): QueueItem['state'][] {
    if (value === 'approvals') return ['awaiting_approval'];
    if (value === 'waiting') return ['scheduled', 'blocked', 'ready', 'retrying'];
    if (value === 'running') return ['running'];
    if (value === 'history') return DONE_STATES;
    return ['awaiting_approval', 'scheduled', 'blocked', 'ready', 'running', 'retrying'];
  }

  const SECTION_LABELS: Record<QueueSection, string> = {
    active: 'All',
    approvals: 'Needs a decision',
    waiting: 'Waiting',
    running: 'Running',
    history: 'Done',
  };

  /**
   * How much work is in each of the three views, so the segments can say it.
   *
   * One count per view rather than one per state: the segments ARE the views, and a
   * count of something the segment does not select is a number nobody can act on.
   */
  const sectionCountsQuery = createQuery(() => ({
    queryKey: ['queue-section-counts', targetId ?? 'root', workload, priority, profile],
    queryFn: async (): Promise<Record<QueueSection, number>> => {
      const pages = await Promise.all(
        QUEUE_SECTIONS.map((value) => {
          const params = new SvelteURLSearchParams({ limit: '1', offset: '0' });
          params.set('state', sectionStates(value).join(','));
          if (workload !== 'all') params.set('workload', workload);
          if (priority !== 'all') params.set('priority', priority);
          if (profile !== 'all') params.set('profile', profile);
          const search = `?${params.toString()}`;
          return targetId === undefined
            ? api.fetchRootQueue(search)
            : api.fetchTargetQueue(targetId, search);
        }),
      );
      return Object.fromEntries(
        QUEUE_SECTIONS.map((value, index) => [value, pages[index]?.total ?? 0]),
      ) as Record<QueueSection, number>;
    },
  }));

  const SECTION_SEGMENTS = $derived.by(() => {
    const counts = sectionCountsQuery.data;
    return QUEUE_SECTIONS.map((value) => ({
      value,
      label: SECTION_LABELS[value],
      badge: counts === undefined ? undefined : String(counts[value]),
    }));
  });

  /** Whether this view draws that card at all. */
  function shows(card: CardID): boolean {
    if (section === 'active') return true;
    if (section === 'approvals') return card === 'decision';
    if (section === 'history') return card === 'done';

    return card === 'live';
  }

  /**
   * What a card is called where it stands.
   *
   * The heading answers the question the card is the answer to, so a view narrowed to
   * one of them takes its own name - "Running and waiting" over a list a reader has
   * already narrowed to the running work answers a question they did not ask.
   */
  function cardTitle(card: CardID): string {
    if (section !== 'active') return SECTION_LABELS[section];
    if (card === 'decision') return 'Needs a decision';
    if (card === 'live') return 'Running and waiting';

    return 'Done in the last day';
  }

  /**
   * What a card asks for.
   *
   * Its own states, narrowed by the view and by the state filter, and its own limit -
   * so its foot counts what it holds and brings more of itself down. The done card
   * beside live work is bounded by when work FINISHED rather than when it was
   * accepted: a merge held for a day of checks is old work that ended a minute ago.
   */
  function cardQuery(card: CardID): string {
    const states = (CARDS.find((one) => one.id === card)?.states ?? []).filter(
      (state) => sectionStates(section).includes(state) || section === 'history',
    );
    const asked =
      stateFilter === 'all'
        ? states
        : states.filter((state) => state === (stateFilter as QueueItem['state']));
    const query = new SvelteURLSearchParams({
      limit: String(pageSize + revealed[card]),
      offset: '0',
    });
    query.set('state', asked.join(','));
    if (card === 'done' && section === 'active') {
      const settled = Math.floor(rangeNow / HOUR_MS) * HOUR_MS;
      query.set('finished_after', new Date(settled - DAY_MS).toISOString());
    }
    if (workload !== 'all') query.set('workload', workload);
    if (priority !== 'all') query.set('priority', priority);
    if (profile !== 'all') query.set('profile', profile);
    if (targetId === undefined && workspace !== 'all') {
      query.set('workspace', workspace);
    }
    if (repository !== 'all') query.set('repository', repository);
    if (appliedSearch !== '') query.set('search', appliedSearch);
    if (timeRange !== 'all') {
      const age = timeRange === '24h' ? DAY_MS : 7 * DAY_MS;
      query.set('created_after', new Date(rangeNow - age).toISOString());
    }

    return `?${query.toString()}`;
  }

  function fetchQueue(query: string): Promise<QueuePage> {
    return targetId === undefined
      ? api.fetchRootQueue(query)
      : api.fetchTargetQueue(targetId, query);
  }

  /* Take the new rows AFTER the read, not before: taken first, the answer already on its
     way arrives as another set the reader has not asked for, and the notice comes back
     with a count the press was meant to clear. */
  async function load(): Promise<void> {
    await Promise.all(
      CARDS.filter((card) => shows(card.id)).map((card) => cardQueries[card.id].refetch()),
    );
    takeAll();
  }

  function openAction(item: QueueItem, action: QueueActionType): void {
    if (!canControl && targetId !== undefined) return;
    selected = item;
    selectedAction = action;
    actionError = '';
  }

  function openDetail(item: QueueItem): void {
    detailOpen = true;
    detailItemID = item.id;
  }

  function closeDetail(): void {
    detailOpen = false;
    detailItemID = null;
  }

  function closeAction(): void {
    if (actionBusy) return;
    selected = null;
    selectedAction = null;
    actionError = '';
  }

  /** What an act did, said as the act rather than as the state it left behind. */
  function actionReceipt(action: QueueActionType, title: string): string {
    switch (action) {
      case 'run_now':
        return `Running now - ${title}`;
      case 'next_window':
        return `${title} runs in its next hours`;
      case 'schedule_at':
        return `${title} is scheduled`;
      case 'set_priority':
        return `${title} moved to another priority`;
      default:
        return `Cancelled - ${title}`;
    }
  }

  async function submitAction(input: QueueActionInput): Promise<void> {
    if (selected === null) return;
    actionBusy = true;
    try {
      const updated =
        targetId === undefined
          ? await api.actOnRootQueue(selected.id, input)
          : await api.actOnTargetQueue(targetId, selected.id, input);
      queryClient.setQueriesData<QueuePage>({ queryKey: queueListScopeKey(targetId) }, (current) =>
        current === undefined
          ? current
          : {
              ...current,
              items: current.items.map((item) => (item.id === updated.id ? updated : item)),
            },
      );
      queryClient.setQueryData<QueueDetail>(queueDetailKey(targetId, updated.id), (current) =>
        current === undefined ? current : { ...current, item: updated },
      );
      announcement = `${updated.title}: ${updated.state.replaceAll('_', ' ')}`;
      /* An act on the queue leaves the dialog it was made in, and the row it changed is
         one of many - so what happened is said once, in the words the row uses. */
      receipts.say(actionReceipt(input.type, updated.title));
      selected = null;
      selectedAction = null;
      actionError = '';
      void queryClient.invalidateQueries({ queryKey: queueListScopeKey(targetId) });
    } catch (cause) {
      actionError = cause instanceof Error ? cause.message : String(cause);
      if (actionError.toLowerCase().includes('changed')) {
        void queryClient.invalidateQueries({ queryKey: queueListScopeKey(targetId) });
      }
    } finally {
      actionBusy = false;
    }
  }

  function previewAction(input: QueueActionInput): Promise<QueueSchedulePreview> {
    if (selected === null) return Promise.reject(new Error('Queue item is no longer selected'));
    return targetId === undefined
      ? api.previewRootQueueAction(selected.id, input)
      : api.previewTargetQueueAction(targetId, selected.id, input);
  }
</script>

<!--
@component
The queue as one workspace sees it, which is the same work with a different question
asked of it: not "what is the service doing" but "what is happening to my repositories".

`targetId` is what separates the two. With one, this is a workspace's own page and
headed as such; without, it stands inside the Root console's page and takes its heading
from there - which is why the labelling is chosen rather than fixed, and why a second
`<h1>` never appears.

`canControl` draws the acts. A reader who may watch but not act sees the same rows
without the buttons, rather than buttons that refuse.
-->

<section
  class="general-queue"
  aria-labelledby={targetId === undefined ? 'root-page-heading' : 'queue-heading'}
>
  {#if targetId === undefined}
    <RootPageHeader
      title="Queue"
      subtitle="Everything the service is doing, across every workspace"
    >
      <span class="queue-refresh">
        <span class="queue-state" class:is-saying={updating} aria-hidden="true">Updating…</span>
        <span class="card-meta queue-behind" class:is-saying={behind > 0} aria-hidden="true">
          <span class="queue-behind-count">{behind}</span>
          {behind === 1 ? 'item' : 'items'} behind
        </span>
        <Button onclick={() => void load()}>
          {#snippet icon()}<Icon name="refresh" size="sm" strokeWidth={2} />{/snippet}
          Refresh
        </Button>
      </span>
    </RootPageHeader>
  {:else}
    <PageHeader
      id="queue-heading"
      title="Queue"
      description="Work Smyklot is doing or waiting on in this workspace"
    >
      {#snippet actions()}
        <span class="queue-refresh">
          <span class="queue-state" class:is-saying={updating} aria-hidden="true">Updating…</span>
          <span class="card-meta queue-behind" class:is-saying={behind > 0} aria-hidden="true">
            <span class="queue-behind-count">{behind}</span>
            {behind === 1 ? 'item' : 'items'} behind
          </span>
          <Button onclick={() => void load()}>
            {#snippet icon()}<Icon name="refresh" size="sm" strokeWidth={2} />{/snippet}
            Refresh
          </Button>
        </span>
      {/snippet}
    </PageHeader>
  {/if}

  <div class="filter-bar">
    <SearchField
      label="Search the queue"
      placeholder="Search the queue"
      value={search}
      onInput={(value) => (search = value)}
    />
    <!-- The queue's five views, on the page they belong to rather than in the sidebar:
         which slice of one page a reader is looking at is a filter, and the tree names
         pages. Each is still an address, so a link to the decisions goes straight
         there. -->
    {#if onSelectSection !== undefined}
      <SegmentedControl
        name="queue-section"
        label="Show"
        options={SECTION_SEGMENTS}
        value={section}
        onSelect={(value) => onSelectSection(value as QueueSection)}
      />
    {/if}
    <span class="push-end">
      <ListToolsMenu label="Filter queue" sorts={[]} filters={queueFilters} />
    </span>
  </div>

  <p class="visually-hidden" aria-live="polite">{announcement}</p>
  <!-- Its own region, because the word beside the button is drawn with `visibility` and a
       hidden node is not in the accessibility tree to be announced from. -->
  <p class="visually-hidden" aria-live="polite">{updating ? 'Updating…' : ''}</p>
  <p class="visually-hidden" aria-live="polite">
    {behind > 0 ? `${behind} ${behind === 1 ? 'item' : 'items'} behind` : ''}
  </p>
  {#if loading && !answered}
    <Plate label="Loading…"><p class="dim" role="status">Reading the durable queue…</p></Plate>
  {:else if error !== '' && !answered}
    <Plate label="Queue unavailable" tone="alarm">
      <p>{error}</p>
      <Button onclick={() => void load()}>Try again</Button>
    </Plate>
  {:else}
    {#if error !== ''}
      <Plate label="Queue update delayed" tone="alarm">
        <p>{error}</p>
        <Button onclick={() => void load()}>Try again</Button>
      </Plate>
    {/if}
    <QueueList
      {cards}
      workspace={rowWorkspace}
      clock={() => now}
      reviewHref={(item) => (item.kind === 'sync_apply' ? (planHref ?? null) : null)}
      onReview={(_item, event) => onOpenPlan?.(event)}
      onOpen={openDetail}
      onAction={openAction}
    />
  {/if}
</section>

{#key `${selected?.id ?? ''}:${selectedAction ?? ''}`}
  <QueueActionDialog
    item={selected}
    action={selectedAction}
    busy={actionBusy}
    error={actionError}
    onClose={closeAction}
    onPreview={previewAction}
    onSubmit={(input) => void submitAction(input)}
  />
{/key}

<QueueDetailDialog
  open={detailOpen}
  {detail}
  loading={detailLoading}
  error={detailError}
  onClose={closeDetail}
/>

<style>
  /* No gap: the rhythm is the blocks' own. A toolbar states its distance to what it
     filters and a card states its distance to the card above it, so a container gap is
     added to both - the filter bar stood 28px off the first card where the sheet says
     16, and every card pair with it. */
  .general-queue {
    display: flex;
    flex-direction: column;
    min-height: 0;
    min-width: 0;
  }
  /* A refresh in flight, said beside the control that started it. It used to live at the
     other end of the filter bar next to the tools menu, which is where a reader who has
     just pressed Refresh is not looking.

     THE WORD IS ALWAYS THERE AND ONLY ITS INK COMES AND GOES. Written as text that
     arrives, it changed the width of the action group, and the group wraps: under about
     900px the word took a line of its own, grew the header 20px and pushed the whole
     page down every time a refresh ran long. Held with `visibility`, its box is measured
     once and nothing on the page moves for it, ever - which is the whole point of saying
     something while a reader waits. The announcement is a hidden live region of its own,
     because a node held this way is not in the accessibility tree to be announced from. */
  .queue-state {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    text-box: trim-both cap alphabetic;
    visibility: hidden;
  }

  .queue-state.is-saying {
    visibility: visible;
  }

  /* ONE ITEM IN THE HEADER, so what it says can never re-lay the header out.
     ------------------------------------------------------------------------
     Both of these words arrive because the SERVICE moved on, not because the reader
     asked for anything, so neither may move the page it is reporting on. Left as
     siblings in the action group they did: that group wraps, and a flex line WRAPS
     before it shrinks, so `min-inline-size: 0` on the words changed nothing - at 980px
     they took a line of their own, grew the header 20px and pushed the page down.

     As one item the group has nothing to wrap. Inside it the line does not wrap either,
     so the words shrink instead, and the button - the thing that answers them - keeps
     its full width at every width. On a phone the count clips rather than the control. */
  .queue-refresh {
    align-items: center;
    display: flex;
    flex-wrap: nowrap;
    /* Wider than the gap between two controls: what stands to the left of the button
       here is a sentence rather than another control, and at the control gap it read as
       the button's own label run on. */
    gap: var(--space-4);
    min-inline-size: 0;
  }

  /* AND ITS PLACE IS RESERVED, for the same reason the word above it is: a count that
     grew the group took width from the header's copy column, and the title wrapped -
     20px of page, on a change the reader did not make. Held with `visibility` the group
     is its full width from the first paint and stays there, so the only thing left that
     could move is a digit, which tabular figures pin to one width. */
  .queue-behind {
    visibility: hidden;
    white-space: nowrap;
  }

  /* AND ON A NARROW SCREEN THE WORDS GIVE, NOT THE BUTTON. Both are held at their full
     width so nothing moves, which is right until the header has less room than that:
     under about 340px the group ran five pixels past the page and Chrome drew the whole
     thing at 98%. `min-inline-size: 0` lifts the automatic minimum a nowrap phrase would
     otherwise hold, so these two clip while the control that answers them keeps its
     label - a button's own min-content is that label, so flex never takes it below.

     ACROSS ONLY. These boxes are trimmed to their cap band, so the ink stands proud of
     the box at both ends by design, and `overflow: hidden` cut 2.64px off it. `clip` is
     the one value that takes an axis on its own - `hidden` on one axis makes the other a
     scroll container, which is why the pair cannot be written with it. */
  .queue-state,
  .queue-behind {
    min-inline-size: 0;
    overflow-x: clip;
  }

  .queue-behind.is-saying {
    visibility: visible;
  }

  /* Two digits' worth, right-aligned into it: 99 items behind is a queue nobody is
     reading down, and a third digit widens the group by one figure rather than reflowing
     the sentence. */
  .queue-behind-count {
    display: inline-block;
    font-variant-numeric: tabular-nums;
    min-inline-size: 2ch;
    text-align: end;
  }
</style>
