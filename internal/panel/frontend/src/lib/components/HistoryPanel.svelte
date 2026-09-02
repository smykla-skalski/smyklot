<script lang="ts">
  import { untrack } from 'svelte';
  import { createInfiniteQuery, createQuery, type InfiniteData } from '@tanstack/svelte-query';
  import { useDebounce, useInterval } from 'runed';

  import { failureAct } from '../failures';
  import { sentenceCase } from '../format';
  import type { FilterSection } from '../filter-menu';
  import type { TimeDisplay } from '../preferences';
  import { EPHEMERAL_PREFS, prefOption, prefText, type PrefsAccessor } from '../preferences-sync';
  import type {
    AuditEntry,
    AuditCategory,
    AuditChange,
    AuditHistoryRequest,
    AuditScope,
    DeliveryFailure,
    FailureHistoryRequest,
    FailureKind,
    HistorySort,
    WorkspaceSettingsBatchResponse,
    SettingsCheckpoint,
    SettingsRestoreInput,
    Page,
    RootRuntimeSettings,
  } from '../types';
  import { receipts } from '../receipts.svelte';
  import Skeleton from './Skeleton.svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import Pill from './Pill.svelte';
  import RelativeTime from './RelativeTime.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import EmptyState from './EmptyState.svelte';
  import ListToolsMenu, { type ToolsFilter, type ToolsSort } from './ListToolsMenu.svelte';
  import SettingsCheckpointDialog from './SettingsCheckpointDialog.svelte';

  type HistoryType = 'audit' | 'failures';
  type HistoryContext = 'workspace' | 'root';

  const AUDIT_SCOPE_FILTERS = [
    {
      options: [
        { value: 'all', label: 'All changes' },
        { value: 'account', label: 'Account changes' },
        { value: 'repositories', label: 'Repository changes' },
      ],
    },
  ] satisfies readonly FilterSection[];

  const AUDIT_CHANGE_FILTERS = [
    {
      options: [
        { value: 'all', label: 'All changes' },
        { value: 'repository', label: 'Repository settings' },
        { value: 'account', label: 'Account settings' },
        { value: 'sync', label: 'Sync configuration' },
      ],
    },
  ] satisfies readonly FilterSection[];

  const ROOT_AUDIT_CATEGORY_FILTERS = [
    {
      options: [
        { value: 'all', label: 'All event categories', exclusive: true },
        { value: 'configuration', label: 'Configuration' },
        { value: 'access', label: 'Access' },
        { value: 'ownership', label: 'Ownership' },
        { value: 'elevation', label: 'Operator visit' },
        { value: 'notification', label: 'Notification' },
        { value: 'runtime', label: 'Runtime' },
      ],
    },
  ] satisfies readonly FilterSection[];

  const {
    targetId,
    fetchAudit,
    exportAudit,
    fetchFailures,
    context = 'workspace',
    section,
    prefs = EPHEMERAL_PREFS,
    readOnly = true,
    hasUnsavedSettingsDrafts = false,
    hasUnsavedRootSettingsDrafts = false,
    hasUnsavedSettingsDraftsForTarget,
    fetchSettingsBaseline,
    fetchSettingsCheckpoint,
    restoreSettingsCheckpoint,
    fetchRootSettingsBaseline,
    fetchRootSettingsCheckpoint,
    restoreRootSettingsCheckpoint,
    onSettingsRestored,
    onRootSettingsRestored,
    repositoryHref,
  }: {
    targetId: string;
    fetchAudit: (request: AuditHistoryRequest) => Promise<Page<AuditEntry>>;
    /**
     * Where the same filtered audit can be downloaded, for the callers that have
     * such an address. Absent is how a page says it offers no export.
     */
    exportAudit?: (request: AuditHistoryRequest) => string;
    fetchFailures: (request: FailureHistoryRequest) => Promise<Page<DeliveryFailure>>;
    context?: HistoryContext;
    section?: HistoryType;
    prefs?: PrefsAccessor;
    readOnly?: boolean;
    hasUnsavedSettingsDrafts?: boolean;
    hasUnsavedRootSettingsDrafts?: boolean;
    hasUnsavedSettingsDraftsForTarget?: (targetId: string) => boolean;
    fetchSettingsBaseline?: (targetId: string) => Promise<SettingsCheckpoint>;
    fetchSettingsCheckpoint?: (
      targetId: string,
      checkpointId: string,
    ) => Promise<SettingsCheckpoint>;
    restoreSettingsCheckpoint?: (
      targetId: string,
      checkpointId: string,
      input: SettingsRestoreInput,
    ) => Promise<WorkspaceSettingsBatchResponse>;
    fetchRootSettingsBaseline?: () => Promise<SettingsCheckpoint>;
    fetchRootSettingsCheckpoint?: (checkpointId: string) => Promise<SettingsCheckpoint>;
    restoreRootSettingsCheckpoint?: (
      checkpointId: string,
      input: SettingsRestoreInput,
    ) => Promise<RootRuntimeSettings>;
    onSettingsRestored?: (result: WorkspaceSettingsBatchResponse, targetId: string) => void;
    onRootSettingsRestored?: (result: RootRuntimeSettings) => void;
    /**
     * Where a failure's repository lives, when the caller has an address for it.
     *
     * A failure row's one act is to open what it failed on. The whole failure is
     * handed over rather than its repository's name: the console reads failures
     * across every workspace and needs the workspace to build an address at all,
     * and a row whose workspace is no longer readable gets no act rather than a
     * wrong one.
     */
    repositoryHref?: (failure: DeliveryFailure) => string | null;
  } = $props();

  // Table state deliberately captures the preferences at mount; remote
  // changes apply on the next remount instead of mid-interaction.
  // svelte-ignore state_referenced_locally
  const initialPrefs = prefs;
  // svelte-ignore state_referenced_locally
  const initialSection = section;

  const HISTORY_SORTS: readonly HistorySort[] = [
    'newest',
    'oldest',
    'actor_asc',
    'actor_desc',
    'target_asc',
    'target_desc',
    'change_asc',
    'change_desc',
    'status_asc',
    'status_desc',
    'repository_asc',
    'repository_desc',
  ];

  let historyType = $state<HistoryType>(
    initialSection ??
      prefOption(initialPrefs.get('table.history.type'), ['audit', 'failures'], 'audit'),
  );
  let search = $state(prefText(initialPrefs.get('table.history.search')));
  let appliedQuery = $state(prefText(initialPrefs.get('table.history.search')));
  let sort = $state<HistorySort>(
    prefOption(initialPrefs.get('table.history.sort'), HISTORY_SORTS, 'newest'),
  );
  let auditScope = $state<AuditScope>(
    prefOption(initialPrefs.get('table.history.scope'), ['all', 'account', 'repositories'], 'all'),
  );
  let auditChange = $state<AuditChange>(
    prefOption(
      initialPrefs.get('table.history.change'),
      ['all', 'repository', 'account', 'sync'],
      'all',
    ),
  );
  let auditCategories = $state<AuditCategory[]>([]);
  let failureKind = $state<FailureKind>(
    prefOption(
      initialPrefs.get('table.history.failure_kind'),
      ['all', 'retryable', 'permanent'],
      'all',
    ),
  );
  let timeDisplay = $state<TimeDisplay>(
    prefOption(initialPrefs.get('history.time_display'), ['relative', 'absolute'], 'relative'),
  );
  const limit = 20;
  let now = $state(Date.now());
  useInterval(30_000, { callback: () => (now = Date.now()) });
  let historyResults = $state<HTMLDivElement>();
  let settingsCheckpointId = $state<string | null>(null);
  let settingsCheckpointTargetId = $state<string | null>(null);
  let settingsCheckpointRoot = $state(false);
  let settingsCheckpointBaseline = $state(false);
  let settingsCheckpointTrigger = $state<HTMLElement | null>(null);

  const hasFilters = $derived(
    appliedQuery !== '' ||
      (historyType === 'audit'
        ? context === 'root'
          ? auditCategories.length > 0
          : auditScope !== 'all' || auditChange !== 'all'
        : failureKind !== 'all'),
  );
  /* A failure page stopped saying "webhook deliveries": the reader came to fix
     something, not to meet the transport it arrived on. */
  const description = $derived(
    context === 'root'
      ? historyType === 'audit'
        ? "Every change made anywhere through Smyklot - the service's own included"
        : 'What Smyklot tried and could not finish, in any workspace - the cause and the action that can help'
      : historyType === 'audit'
        ? 'Every change made through Smyklot: who, what, and where'
        : 'Work that stopped, with the cause and the action that can help',
  );
  const auditQuery = createInfiniteQuery(() => ({
    queryKey: [
      'audit',
      targetId,
      appliedQuery,
      sort,
      auditScope,
      auditChange,
      [...auditCategories],
      context,
      limit,
    ],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      fetchAudit({
        cursor: pageParam,
        query: appliedQuery,
        sort,
        limit,
        scope: auditScope,
        change: auditChange,
        categories: context === 'root' ? [...auditCategories] : undefined,
      }),
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  }));
  /* The export carries the filters the page is wearing, so the file holds what the
     reader is looking at rather than everything. The limit is the server's to
     choose - it writes every page there is - so a token one is sent. */
  const exportHref = $derived(
    exportAudit === undefined
      ? null
      : exportAudit({
          query: appliedQuery,
          sort,
          limit,
          scope: auditScope,
          change: auditChange,
          categories: [...auditCategories],
        }),
  );

  /* The download is the browser's, so nothing here knows when the file arrived - what
     the receipt reports is what was asked for, which is the part a reader cannot see
     from the file's name. */
  function sayExported(): void {
    const filtered =
      appliedQuery.trim() !== '' ||
      auditCategories.length > 0 ||
      auditScope !== 'all' ||
      auditChange !== 'all';
    receipts.say(
      filtered
        ? 'Exporting the audit as CSV, filtered the way this page is'
        : 'Exporting the whole audit as CSV',
    );
  }

  const failureQuery = createInfiniteQuery(() => ({
    queryKey: ['failures', targetId, appliedQuery, sort, failureKind, limit],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      fetchFailures({
        cursor: pageParam,
        query: appliedQuery,
        sort,
        limit,
        kind: failureKind,
      }),
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  }));
  const auditPage = $derived(flattenPages(auditQuery.data));
  const failurePage = $derived(flattenPages(failureQuery.data));
  const currentPage = $derived(historyType === 'audit' ? auditPage : failurePage);
  const loading = $derived(
    historyType === 'audit' ? auditQuery.isFetching : failureQuery.isFetching,
  );
  const activeError = $derived(historyType === 'audit' ? auditQuery.error : failureQuery.error);
  const nextPageError = $derived(
    historyType === 'audit' ? auditQuery.isFetchNextPageError : failureQuery.isFetchNextPageError,
  );
  const problem = $derived(
    !nextPageError && activeError !== null ? errorMessage(activeError) : null,
  );
  const loadMoreProblem = $derived(
    nextPageError && activeError !== null ? errorMessage(activeError) : null,
  );
  const auditRows = $derived(auditPage?.items ?? []);
  const failureRows = $derived(failurePage?.items ?? []);
  /**
   * The audit, grouped by the day it happened on.
   *
   * A day is a heading rather than a column, because that is how a reader asks for it:
   * "what happened today", never "sort by date descending". The grouping follows the
   * order the server returned, so it holds whichever way the list is sorted - only a
   * newest-first list produces the runs a reader expects, which is the default.
   */
  const auditDays = $derived.by(() => {
    const days: Array<{ key: string; day: string; head: string; entries: AuditEntry[] }> = [];
    for (const entry of auditRows) {
      const day = entry.created_at.slice(0, 10);
      const last = days.at(-1);
      if (last !== undefined && last.day === day) last.entries.push(entry);
      else {
        /* The RUN is the key, not the day: sorted by actor rather than by time the
           same day opens more than one run, and two groups keyed alike is a crash. */
        days.push({
          key: `${day}#${days.length}`,
          day,
          head: dayHead(entry.created_at),
          entries: [entry],
        });
      }
    }
    return days;
  });

  /** "Today", "Yesterday", and then the day named in full. */
  function dayHead(value: string): string {
    const midnight = (at: Date): number =>
      new Date(at.getFullYear(), at.getMonth(), at.getDate()).getTime();
    const day = new Date(value);
    const days = Math.round((midnight(new Date(now)) - midnight(day)) / 86_400_000);
    if (days === 0) return 'Today';
    if (days === 1) return 'Yesterday';
    return day.toLocaleDateString(undefined, {
      weekday: days < 7 ? 'long' : undefined,
      day: 'numeric',
      month: 'short',
      year: day.getFullYear() === new Date(now).getFullYear() ? undefined : 'numeric',
    });
  }

  const debouncedSearch = useDebounce((value: string) => (appliedQuery = value), 250);
  $effect(() => {
    const value = search.trim();
    untrack(() => void debouncedSearch(value));
  });

  $effect(() => {
    const resultKey = JSON.stringify([
      targetId,
      historyType,
      appliedQuery,
      sort,
      auditScope,
      auditChange,
      auditCategories,
      failureKind,
    ]);
    untrack(() => {
      if (resultKey !== '') scrollResultsToTop();
    });
  });

  // Follow the prop only when it actually changes. Comparing it against the
  // local state instead would fight the toggle: a caller that never updates
  // `section` (the workspace views pass none) would snap every local
  // switch straight back to its constant default.
  // svelte-ignore state_referenced_locally
  let observedSection = section;
  $effect(() => {
    if (section === observedSection) return;
    observedSection = section;
    if (section !== undefined && section !== historyType) {
      historyType = section;
      sort = 'newest';
    }
  });

  function selectTimeDisplay(value: TimeDisplay): void {
    timeDisplay = value;
  }

  // One persistence effect instead of a write at every mutation site: any
  // change to the tracked state syncs, and the initial run is a no-op because
  // the state was just read from the same preferences.
  $effect(() => {
    prefs.set('table.history.type', historyType);
    prefs.set('table.history.sort', sort);
    prefs.set('table.history.scope', auditScope);
    prefs.set('table.history.change', auditChange);
    prefs.set('table.history.failure_kind', failureKind);
    prefs.set('table.history.search', appliedQuery);
    prefs.set('history.time_display', timeDisplay);
  });

  /* The order each record is read in, chosen from the tools menu beside the search now
     that there are no column headings to carry it: one sort at a time, each with its
     two directions. `when` is the pair every list has and the one both default to. */
  const SORT_PAIRS = {
    actor: ['actor_asc', 'actor_desc'],
    target: ['target_asc', 'target_desc'],
    change: ['change_asc', 'change_desc'],
    status: ['status_asc', 'status_desc'],
    repository: ['repository_asc', 'repository_desc'],
    when: ['oldest', 'newest'],
  } as const satisfies Record<string, readonly [HistorySort, HistorySort]>;

  function toggleSort(column: keyof typeof SORT_PAIRS): void {
    const [ascending, descending] = SORT_PAIRS[column];
    sort = sort === ascending ? descending : ascending;
  }

  function sortDirection(column: keyof typeof SORT_PAIRS): 'ascending' | 'descending' | undefined {
    const [ascending, descending] = SORT_PAIRS[column];
    if (sort === ascending) return 'ascending';
    return sort === descending ? 'descending' : undefined;
  }

  function toolSort(label: string, column: keyof typeof SORT_PAIRS): ToolsSort {
    return { label, direction: sortDirection(column), onToggle: () => toggleSort(column) };
  }

  const toolSorts = $derived(
    historyType === 'audit'
      ? [
          toolSort('When', 'when'),
          toolSort('Actor', 'actor'),
          toolSort(context === 'root' ? 'Workspace' : 'Target', 'target'),
          toolSort('Change', 'change'),
        ]
      : [toolSort('When', 'when'), toolSort('Repository', 'repository')],
  );

  /* How the rows read, in the same menu as what they show. It was a control of its
     own beside this one - two sliders buttons on one bar, asking a reader to know
     which of them held "times as dates" before pressing either. */
  const toolDisplay = $derived<ToolsFilter[]>([
    {
      label: 'Time display',
      hint: 'Choose how event times appear',
      sections: [
        {
          options: [
            { value: 'relative', label: 'Relative', description: 'How long ago it happened' },
            { value: 'absolute', label: 'Date and time', description: 'The moment it happened' },
          ],
        },
      ],
      selected: [timeDisplay],
      onChange: (values) => selectTimeDisplay(values[0] === 'absolute' ? 'absolute' : 'relative'),
    },
  ]);

  /* The narrower questions, on the same bar: what KIND of change, and - in the console -
     which category of event. The failure state is not here; it leads the list. */
  const toolFilters = $derived.by((): ToolsFilter[] => {
    if (historyType === 'failures') return [];
    if (context === 'root') {
      return [
        {
          label: 'Event category',
          hint: 'Choose which application events to show',
          sections: ROOT_AUDIT_CATEGORY_FILTERS,
          selected: auditCategories.length === 0 ? ['all'] : auditCategories,
          multiple: true,
          fallbackValue: 'all',
          onChange: selectAuditCategories,
        },
      ];
    }
    return [
      {
        label: 'Target',
        hint: 'Choose which configuration changes to show',
        sections: AUDIT_SCOPE_FILTERS,
        selected: [auditScope],
        fallbackValue: 'all',
        onChange: selectAuditScope,
      },
      {
        label: 'Change',
        hint: 'Choose which configuration changes to show',
        sections: AUDIT_CHANGE_FILTERS,
        selected: [auditChange],
        fallbackValue: 'all',
        onChange: selectAuditChange,
      },
    ];
  });

  /**
   * How many failures are in each state, so the segments can say it.
   *
   * Both counts carry the search, because a count that ignores what is on screen is a
   * count of something else. Only asked for while the failures are being read - the
   * audit never shows these segments, and a page that is not showing a number should
   * not be fetching one.
   */
  const failureCountsQuery = createQuery(() => ({
    queryKey: ['failure-state-counts', targetId, appliedQuery, historyType],
    enabled: historyType === 'failures',
    queryFn: async (): Promise<{ all: number; retryable: number }> => {
      const shared = { query: appliedQuery, sort: 'newest' as HistorySort, limit: 1 };
      const [all, retryable] = await Promise.all([
        fetchFailures({ ...shared, kind: 'all' }),
        fetchFailures({ ...shared, kind: 'retryable' }),
      ]);
      return { all: all.total, retryable: retryable.total };
    },
  }));

  const FAILURE_SEGMENTS = $derived.by(() => {
    const counts = failureCountsQuery.data;
    const badge = (value: number | undefined): string | undefined =>
      value === undefined ? undefined : String(value);
    return [
      { value: 'all', label: 'All', badge: badge(counts?.all) },
      { value: 'retryable', label: 'Retrying', badge: badge(counts?.retryable) },
      {
        value: 'permanent',
        label: 'Needs a fix',
        badge: badge(counts === undefined ? undefined : Math.max(0, counts.all - counts.retryable)),
      },
    ];
  });

  function selectAuditScope(values: string[]): void {
    const value = values[0];
    if (value === 'all' || value === 'account' || value === 'repositories') auditScope = value;
  }

  function selectAuditChange(values: string[]): void {
    const value = values[0];
    if (value === 'all' || value === 'repository' || value === 'account' || value === 'sync') {
      auditChange = value;
    }
  }

  function selectAuditCategories(values: string[]): void {
    const allowed = new Set<AuditCategory>([
      'configuration',
      'access',
      'ownership',
      'elevation',
      'notification',
      'runtime',
    ]);
    auditCategories = values.filter((value): value is AuditCategory =>
      allowed.has(value as AuditCategory),
    );
  }

  function selectFailureKind(values: string[]): void {
    const value = values[0];
    if (value === 'all' || value === 'permanent' || value === 'retryable') failureKind = value;
  }

  function auditSummary(value: string): string {
    return sentenceCase(value.replace(/\s+for\s*$/i, ''));
  }

  /* An audit line is one sentence: who, what they did, and what they did it to. The
     three parts are separate so the object can be set in the mono voice inside the
     line rather than parked in a column of its own. */
  function auditActor(entry: AuditEntry): string {
    return entry.actor.display_name;
  }

  /** Lower-cased, because the actor already opened the sentence. */
  function auditVerb(entry: AuditEntry): string {
    const summary = auditSummary(entry.summary);
    return summary.charAt(0).toLocaleLowerCase() + summary.slice(1);
  }

  /**
   * What the change was made to, or nothing when the sentence already carries it.
   *
   * The repository wherever there is one, in both consoles: a console row that named
   * only the workspace said "in smykla-skalski" about a change to one repository
   * inside it, and the repository - the thing that changed - was nowhere on the row.
   * The workspace is the sum's job then.
   */
  function auditObject(entry: AuditEntry): string | null {
    if (entry.repository_full_name !== undefined) {
      return repositoryName(entry.repository_full_name);
    }

    return context === 'root' ? (entry.workspace?.login ?? null) : null;
  }

  /** The whole line as one string, for a label a screen reader reads in one go. */
  function auditSentence(entry: AuditEntry): string {
    const object = auditObject(entry);
    return `${auditActor(entry)} ${auditVerb(entry)}${object === null ? '' : ` in ${object}`}`;
  }

  /* The space before "of" is a non-breaking one, written as an escape: the count and
     what it counts are one atom. */
  function shownRange(shown: number, total: number | undefined): string {
    if (shown === 0) return 'Nothing to show';
    return `Showing 1-${shown}\u{a0}of ${total ?? shown}`;
  }

  /**
   * The rest of the line: where it happened, and to whom.
   *
   * The category and the wire action used to lead it - "configuration \u00b7
   * repository.config.updated" - which is the name of a branch of the code and the
   * name of a database column. The sentence above already says what was done, and
   * the category was a second, coarser word for the same thing.
   */
  function auditDetail(entry: AuditEntry): string {
    const parts: string[] = [];
    /* Named where the row's own object is a repository, because then the sentence
       has not said which workspace that repository is in. */
    if (context === 'root' && entry.repository_full_name !== undefined) {
      parts.push(entry.workspace?.display_name ?? 'The service itself');
    }
    if (entry.subject !== undefined) parts.push(`@${entry.subject.login}`);

    return parts.join(' \u00b7 ');
  }

  function failureDetail(failure: DeliveryFailure): string {
    return `${failure.event} \u00b7 ${failure.stage} \u00b7 delivery ${failure.delivery_id}`;
  }

  function repositoryName(fullName: string): string {
    const name = fullName.slice(fullName.lastIndexOf('/') + 1);
    return name === '' ? fullName : name;
  }

  async function loadNextPage(): Promise<void> {
    if (historyType === 'audit') {
      if (auditQuery.hasNextPage && !auditQuery.isFetchingNextPage)
        await auditQuery.fetchNextPage();
      return;
    }
    if (failureQuery.hasNextPage && !failureQuery.isFetchingNextPage) {
      await failureQuery.fetchNextPage();
    }
  }

  /* Back to the first row when the question changes: the list is the page's own scroll
     now, so what has to move is the window rather than a pane inside it. */
  function scrollResultsToTop(): void {
    const top = historyResults?.getBoundingClientRect().top;
    if (top === undefined || top >= 0) return;
    window.scrollBy({ top, behavior: 'instant' });
  }

  function retry(): void {
    if (historyType === 'audit') {
      if (auditQuery.isFetchNextPageError) void auditQuery.fetchNextPage();
      else void auditQuery.refetch();
      return;
    }
    if (failureQuery.isFetchNextPageError) void failureQuery.fetchNextPage();
    else void failureQuery.refetch();
  }

  function flattenPages<T>(data: InfiniteData<Page<T>> | undefined): Page<T> | null {
    const pages = data?.pages;
    if (pages === undefined || pages.length === 0) return null;
    const last = pages.at(-1);
    return last === undefined ? null : { ...last, items: pages.flatMap((page) => page.items) };
  }

  function errorMessage(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }

  function clearFilters(): void {
    search = '';
    appliedQuery = '';
    auditScope = 'all';
    auditChange = 'all';
    auditCategories = [];
    failureKind = 'all';
  }

  function openSettingsCheckpoint(entry: AuditEntry, trigger: HTMLElement): void {
    if (entry.settings_checkpoint_id === undefined || !canInspectSettingsCheckpoint(entry)) return;
    settingsCheckpointTrigger = trigger;
    settingsCheckpointId = entry.settings_checkpoint_id;
    settingsCheckpointBaseline = false;
    settingsCheckpointRoot = context === 'root' && entry.target_id === undefined;
    settingsCheckpointTargetId = settingsCheckpointRoot ? null : (entry.target_id ?? targetId);
  }

  function openSettingsBaseline(trigger: HTMLElement): void {
    settingsCheckpointTrigger = trigger;
    settingsCheckpointId = 'baseline';
    settingsCheckpointBaseline = true;
    settingsCheckpointRoot = context === 'root';
    settingsCheckpointTargetId = settingsCheckpointRoot ? null : targetId;
  }

  function closeSettingsCheckpoint(): void {
    settingsCheckpointId = null;
    settingsCheckpointTargetId = null;
    settingsCheckpointRoot = false;
    settingsCheckpointBaseline = false;
  }

  function canInspectSettingsCheckpoint(entry: AuditEntry): boolean {
    if (context === 'root' && entry.target_id === undefined) {
      return fetchRootSettingsCheckpoint !== undefined;
    }
    return fetchSettingsCheckpoint !== undefined;
  }

  async function fetchOpenedSettingsCheckpoint(checkpointId: string): Promise<SettingsCheckpoint> {
    if (settingsCheckpointBaseline) {
      if (settingsCheckpointRoot) {
        if (fetchRootSettingsBaseline === undefined)
          throw new Error('Initial settings snapshot is unavailable');
        return fetchRootSettingsBaseline();
      }
      if (fetchSettingsBaseline === undefined || settingsCheckpointTargetId === null) {
        throw new Error('Initial settings snapshot is unavailable');
      }
      return fetchSettingsBaseline(settingsCheckpointTargetId);
    }
    if (settingsCheckpointRoot) {
      if (fetchRootSettingsCheckpoint === undefined)
        throw new Error('Settings snapshot is unavailable');
      return fetchRootSettingsCheckpoint(checkpointId);
    }
    if (fetchSettingsCheckpoint === undefined || settingsCheckpointTargetId === null) {
      throw new Error('Settings snapshot is unavailable');
    }
    return fetchSettingsCheckpoint(settingsCheckpointTargetId, checkpointId);
  }

  function canRestoreOpenedSettingsCheckpoint(): boolean {
    return settingsCheckpointRoot
      ? restoreRootSettingsCheckpoint !== undefined
      : restoreSettingsCheckpoint !== undefined && settingsCheckpointTargetId !== null;
  }

  async function restoreOpenedSettingsCheckpoint(
    checkpointId: string,
    input: SettingsRestoreInput,
  ): Promise<void> {
    if (settingsCheckpointRoot) {
      if (restoreRootSettingsCheckpoint === undefined) return;
      const result = await restoreRootSettingsCheckpoint(checkpointId, input);
      onRootSettingsRestored?.(result);
    } else {
      if (restoreSettingsCheckpoint === undefined || settingsCheckpointTargetId === null) return;
      const result = await restoreSettingsCheckpoint(
        settingsCheckpointTargetId,
        checkpointId,
        input,
      );
      onSettingsRestored?.(result, settingsCheckpointTargetId);
    }
    void auditQuery.refetch();
  }
</script>

<!--
@component
What has happened here, and what failed. Two records that answer different questions -
the audit says what was decided, the failures say what could not be done - kept in one
pane because a reader looking at either usually wants both.

`context` is the whole of the difference between the workspace's copy and the Root
console's: the same records, the same table, and a wider scope. It is a prop rather than
a second component because the two drifted apart the last time they were separate.

The settings checkpoints are read from here too, which is why it takes the baseline and
restore functions: history is where a reader goes to undo, so the way back has to be
where the record is.
-->

{#snippet auditLine(entry: AuditEntry, opens: boolean)}
  <span class="object-main">
    <span class="object-name-row">
      <!-- The sentence NAMES its object, or it is not an audit line: "changed two
           settings in smyklot", never "Updated two repository settings". -->
      <span class="object-name">
        {auditActor(entry)}
        {auditVerb(entry)}{#if auditObject(entry) !== null}&nbsp;in
          <code class="file-path">{auditObject(entry)}</code>{/if}
      </span>
      <!-- The one act somebody outside this workspace took inside it. -->
      {#if entry.elevation_id !== undefined}
        <Pill tone="warning">Operator</Pill>
      {/if}
      <!-- The console reads the service's own changes beside every workspace's, and
           a row about the service belongs to no workspace to name. -->
      {#if context === 'root' && entry.workspace === undefined}
        <Pill>service</Pill>
      {/if}
    </span>
    <span class="object-sum">
      {#if auditDetail(entry) !== ''}{auditDetail(entry)} ·
      {/if}
      <RelativeTime value={entry.created_at} nowMs={now} exact={timeDisplay === 'absolute'} />
    </span>
  </span>
  <span class="object-side">
    {#if opens}
      <span class="row-chevron" aria-hidden="true"><Icon name="chevron-right" size="xs" /></span>
    {/if}
  </span>
{/snippet}

<section
  class="plate history-panel"
  class:root-context={context === 'root'}
  class:absolute-time={timeDisplay === 'absolute'}
  aria-labelledby={context === 'root' ? 'root-page-heading' : 'history-heading'}
>
  {#if context === 'root'}
    <RootPageHeader title={historyType === 'audit' ? 'Audit' : 'Failures'} subtitle={description}>
      <!-- The whole filtered record as a file, for the reader whose next question
           is one no page answers. A link, not a fetch: the browser downloads it
           with the session it already has, and nothing here holds a year of audit
           in memory to hand it over. -->
      {#if historyType === 'audit' && exportHref !== null}
        <Button href={exportHref} download onclick={sayExported}>Export</Button>
      {/if}
    </RootPageHeader>
  {:else}
    <PageHeader
      id="history-heading"
      section="Activity"
      title={historyType === 'audit' ? 'Audit' : 'Failures'}
      {description}
    >
      {#snippet actions()}
        {#if historyType === 'audit' && exportHref !== null}
          <Button href={exportHref} download onclick={sayExported}>Export</Button>
        {/if}
      {/snippet}
    </PageHeader>
  {/if}

  <div class="filter-bar">
    <SearchField
      label="Search history"
      placeholder={historyType === 'audit' ? 'Search changes' : 'Search failures'}
      value={search}
      onInput={(value) => (search = value)}
    />

    <!-- What a failure needs from the reader is the question the list leads with:
         whether Smyklot is still trying, or whether it has stopped and is waiting for
         somebody. The audit has no such question - it is a record, not a queue. -->
    {#if historyType === 'failures'}
      <SegmentedControl
        name="failure-state"
        label="Show"
        options={FAILURE_SEGMENTS}
        value={failureKind}
        onSelect={(value) => selectFailureKind([value])}
      />
    {/if}

    {#if historyType === 'audit' && ((context === 'root' && fetchRootSettingsBaseline !== undefined) || (context === 'workspace' && fetchSettingsBaseline !== undefined))}
      <Button tone="quiet" onclick={(event) => openSettingsBaseline(event.currentTarget)}>
        Initial snapshot
      </Button>
    {/if}

    <span class="push-end">
      <ListToolsMenu sorts={toolSorts} filters={toolFilters} display={toolDisplay} />
    </span>
  </div>

  <div
    class:loading
    class="history-results list-region"
    bind:this={historyResults}
    aria-busy={loading}
  >
    <!-- A refresh that failed over a loaded table has not made the table wrong. -->
    {#if problem !== null && currentPage !== null}
      <ResultProblem
        title="History could not be loaded"
        {problem}
        busy={loading}
        onRetry={() => retry()}
        overContent
      />
    {/if}

    {#if problem !== null && currentPage === null}
      <ResultProblem
        title="History could not be loaded"
        {problem}
        busy={loading}
        onRetry={() => retry()}
      />
    {:else if loading && currentPage === null}
      <Skeleton
        label="Loading history"
        --skeleton-row-height="3rem"
        --skeleton-bar-a-width="min(12rem, 26%)"
        --skeleton-bar-b-left="46%"
        --skeleton-bar-b-width="min(16rem, 32%)"
      />
    {:else if historyType === 'audit'}
      {#if auditRows.length === 0}
        <Card>
          <EmptyState
            title="Nothing yet"
            description={hasFilters
              ? 'Try another search or clear the active filters'
              : 'Every change made through Smyklot here will land on this page, day by day'}
            actionLabel={hasFilters ? 'Clear filters' : undefined}
            onAction={hasFilters ? clearFilters : undefined}
          />
        </Card>
      {:else}
        <Card>
          {#each auditDays as day (day.key)}
            <h2 class="day-head">{day.head}</h2>
            <ul class="object-list">
              {#each day.entries as entry (entry.id)}
                {@const opens =
                  entry.settings_checkpoint_id !== undefined && canInspectSettingsCheckpoint(entry)}
                <li>
                  {#if opens}
                    <!-- A row that opens the exact before and after IS the button; one
                         that has nothing behind it stays a static line rather than
                         promising an opening that never comes. -->
                    <button
                      class="object-row"
                      type="button"
                      aria-haspopup="dialog"
                      aria-label={`${auditSentence(entry)}, inspect the settings snapshot`}
                      onclick={(event) => openSettingsCheckpoint(entry, event.currentTarget)}
                    >
                      {@render auditLine(entry, true)}
                    </button>
                  {:else}
                    <div class="object-row">{@render auditLine(entry, false)}</div>
                  {/if}
                </li>
              {/each}
            </ul>
          {/each}
          <div class="list-foot">
            <span>{shownRange(auditRows.length, auditPage?.total)}</span>
            {#if auditPage?.next_cursor != null}
              <Button tone="quiet" disabled={loading} onclick={() => void loadNextPage()}>
                Older entries
              </Button>
            {/if}
          </div>
        </Card>
      {/if}
    {:else}
      {#if failureRows.length === 0}
        <Card>
          <EmptyState
            title={hasFilters ? 'Nothing matches' : 'No failures'}
            description={hasFilters
              ? 'Try another search, or show them all'
              : 'When Smyklot cannot finish something here, the row lands on this page with its cause and the one act that helps'}
            actionLabel={hasFilters ? 'Show them all' : undefined}
            onAction={hasFilters ? clearFilters : undefined}
          />
        </Card>
      {:else}
        <Card>
          <ul class="object-list">
            {#each failureRows as failure (failure.id)}
              {@const href = repositoryHref?.(failure) ?? null}
              <li>
                <div class="object-row">
                  <span class="object-main">
                    <span class="object-name-row">
                      <!-- What was tried, and what it was tried on. -->
                      <span class="object-name">
                        {failureAct(failure.stage)}
                        <code class="file-path">{repositoryName(failure.repository_full_name)}</code
                        >
                      </span>
                      <!-- A verdict, not a taxonomy: "Retryable/Permanent" told a
                           reader which branch of the code they were in. This says
                           whether anybody has to do anything. -->
                      <Pill tone={failure.retryable ? 'warning' : 'danger'}>
                        {failure.retryable ? 'Retrying' : 'Needs a fix'}
                      </Pill>
                    </span>
                    <span class="object-sum" title={failureDetail(failure)}>
                      <!-- Whose work failed, on the page that reads every workspace's:
                           the repository above is one of many called `api-gateway`. -->
                      {#if context === 'root'}{failure.workspace?.display_name ??
                          'The service itself'} ·
                      {/if}{sentenceCase(failure.reason)}
                      {failure.retryable ? '\u00b7 Smyklot retries on its own \u00b7' : '\u00b7'}
                      <RelativeTime
                        value={failure.occurred_at}
                        nowMs={now}
                        exact={timeDisplay === 'absolute'}
                      />
                    </span>
                  </span>
                  <span class="object-side">
                    {#if href !== null}
                      <Button
                        tone="quiet"
                        {href}
                        aria-label="Open {repositoryName(failure.repository_full_name)}"
                      >
                        Open the repository
                      </Button>
                    {/if}
                  </span>
                </div>
              </li>
            {/each}
          </ul>
          <div class="list-foot">
            <span>{shownRange(failureRows.length, failurePage?.total)}</span>
            {#if failurePage?.next_cursor != null}
              <Button tone="quiet" disabled={loading} onclick={() => void loadNextPage()}>
                Show more
              </Button>
            {/if}
          </div>
        </Card>
      {/if}
    {/if}
    {#if loadMoreProblem !== null}
      <div class="load-more-alert" role="alert">
        <span>{loadMoreProblem}</span>
        <Button onclick={() => void loadNextPage()}>Try again</Button>
      </div>
    {/if}
  </div>
</section>

{#if settingsCheckpointId !== null && (settingsCheckpointRoot || settingsCheckpointTargetId !== null)}
  <SettingsCheckpointDialog
    open
    identity={`${settingsCheckpointRoot ? 'root' : (settingsCheckpointTargetId ?? '')}:${settingsCheckpointBaseline ? 'baseline' : 'history'}`}
    checkpointId={settingsCheckpointId}
    {readOnly}
    hasUnsavedDrafts={settingsCheckpointRoot
      ? hasUnsavedRootSettingsDrafts
      : settingsCheckpointTargetId !== null && hasUnsavedSettingsDraftsForTarget !== undefined
        ? hasUnsavedSettingsDraftsForTarget(settingsCheckpointTargetId)
        : hasUnsavedSettingsDrafts}
    returnFocus={settingsCheckpointTrigger}
    fetchCheckpoint={fetchOpenedSettingsCheckpoint}
    restoreCheckpoint={canRestoreOpenedSettingsCheckpoint()
      ? restoreOpenedSettingsCheckpoint
      : undefined}
    onClose={closeSettingsCheckpoint}
  />
{/if}

<style>
  .history-panel {
    --local-control-height: var(--control-height-compact);

    background: transparent;
    border: 0;
    border-radius: 0;
    box-shadow: none;
    display: flex;
    flex: 1;
    flex-direction: column;
    margin-bottom: 0;
    min-height: 0;
    overflow: visible;
  }

  .history-results {
    min-height: 5rem;
  }

  .load-more-alert {
    align-items: center;
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-control);
    bottom: var(--space-3);
    box-shadow: var(--shadow-popover);
    display: flex;
    gap: var(--space-3);
    left: 50%;
    max-width: calc(100% - 2 * var(--space-4));
    padding: var(--space-2) var(--space-3);
    position: absolute;
    transform: translateX(-50%);
    z-index: var(--layer-menu);
  }

  .load-more-alert span {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
  }

  .history-results.loading {
    cursor: progress;
  }

  /* An audit row that opens the exact before and after says so with the arrow every
     other opening row in the panel uses. */
  .row-chevron {
    color: var(--text-muted);
    display: inline-grid;
    place-items: center;
  }

  /* The row's own stacking on a phone is `app.css`'s - every object list does it the
     same way, and this one has nothing to add. */
</style>
