<script lang="ts">
  import { untrack } from 'svelte';
  import { createInfiniteQuery, type InfiniteData } from '@tanstack/svelte-query';
  import {
    columnFilteringFeature,
    createColumnHelper,
    createTable,
    filterFn_includesString,
    rowSortingFeature,
    tableFeatures,
  } from '@tanstack/svelte-table';
  import type { ColumnFiltersState, SortingState, Updater } from '@tanstack/svelte-table';
  import { createVirtualizer } from '@tanstack/svelte-virtual';
  import { MediaQuery } from 'svelte/reactivity';
  import { get } from 'svelte/store';
  import { useDebounce, useInterval } from 'runed';

  import { formatDateTime, formatRelative, formatTimestamp } from '../format';
  import type { FilterSection } from '../filter-menu';
  import type { VirtualRenderRow } from '../virtual-rows.js';
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
    Page,
    SyncConfigBatchResponse,
    SyncConfigCheckpoint,
    SyncConfigRestoreInput,
  } from '../types';
  import DataTable from './DataTable.svelte';
  import Skeleton from './Skeleton.svelte';
  import SortIndicator from './SortIndicator.svelte';
  import Button from './Button.svelte';
  import Avatar from './Avatar.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import HistoryDisplayMenu from './HistoryDisplayMenu.svelte';
  import Icon from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import PageHeader from './PageHeader.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import SearchField from './SearchField.svelte';
  import TableEmptyState from './TableEmptyState.svelte';
  import SyncCheckpointDialog from './SyncCheckpointDialog.svelte';

  type HistoryType = 'audit' | 'failures';
  type HistoryContext = 'installation' | 'root';

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
        { value: 'enablement', label: 'Enablement' },
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
        { value: 'elevation', label: 'Elevation' },
        { value: 'notification', label: 'Notification' },
        { value: 'runtime', label: 'Runtime' },
      ],
    },
  ] satisfies readonly FilterSection[];

  const FAILURE_KIND_FILTERS = [
    {
      options: [
        { value: 'all', label: 'All failures' },
        { value: 'permanent', label: 'Permanent' },
        { value: 'retryable', label: 'Retryable' },
      ],
    },
  ] satisfies readonly FilterSection[];
  const HISTORY_TABLE_FEATURES = tableFeatures({
    columnFilteringFeature,
    filterFns: { includesString: filterFn_includesString },
    rowSortingFeature,
  });
  const auditColumn = createColumnHelper<typeof HISTORY_TABLE_FEATURES, AuditEntry>();
  const AUDIT_COLUMNS = auditColumn.columns([
    auditColumn.accessor((entry) => entry.actor.display_name, {
      id: 'actor',
      enableColumnFilter: false,
    }),
    auditColumn.accessor((entry) => entry.repository_full_name ?? 'Account', { id: 'target' }),
    auditColumn.accessor('summary', { id: 'change' }),
    auditColumn.accessor('created_at', { id: 'when', enableColumnFilter: false }),
  ]);
  const failureColumn = createColumnHelper<typeof HISTORY_TABLE_FEATURES, DeliveryFailure>();
  const FAILURE_COLUMNS = failureColumn.columns([
    failureColumn.accessor((failure) => (failure.retryable ? 'retryable' : 'permanent'), {
      id: 'status',
    }),
    failureColumn.accessor('repository_full_name', { id: 'repository', enableColumnFilter: false }),
    failureColumn.accessor('reason', {
      id: 'failure',
      enableColumnFilter: false,
      enableSorting: false,
    }),
    failureColumn.accessor('occurred_at', { id: 'when', enableColumnFilter: false }),
  ]);

  const {
    targetId,
    fetchAudit,
    fetchFailures,
    context = 'installation',
    rootRole = 'Root',
    section,
    prefs = EPHEMERAL_PREFS,
    readOnly = true,
    hasUnsavedSyncDrafts = false,
    fetchSyncCheckpoint,
    restoreSyncCheckpoint,
    onSyncRestored,
  }: {
    targetId: string;
    fetchAudit: (request: AuditHistoryRequest) => Promise<Page<AuditEntry>>;
    fetchFailures: (request: FailureHistoryRequest) => Promise<Page<DeliveryFailure>>;
    context?: HistoryContext;
    rootRole?: string;
    section?: HistoryType;
    prefs?: PrefsAccessor;
    readOnly?: boolean;
    hasUnsavedSyncDrafts?: boolean;
    fetchSyncCheckpoint?: (targetId: string, checkpointId: string) => Promise<SyncConfigCheckpoint>;
    restoreSyncCheckpoint?: (
      targetId: string,
      checkpointId: string,
      input: SyncConfigRestoreInput,
    ) => Promise<SyncConfigBatchResponse>;
    onSyncRestored?: (result: SyncConfigBatchResponse) => void;
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
      ['all', 'enablement', 'repository', 'account', 'sync'],
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
  let auditScroll = $state<HTMLTableSectionElement>();
  let failureScroll = $state<HTMLTableSectionElement>();
  let checkpointId = $state<string | null>(null);
  let checkpointTargetId = $state<string | null>(null);
  let checkpointTrigger = $state<HTMLElement | null>(null);

  const hasFilters = $derived(
    appliedQuery !== '' ||
      (historyType === 'audit'
        ? context === 'root'
          ? auditCategories.length > 0
          : auditScope !== 'all' || auditChange !== 'all'
        : failureKind !== 'all'),
  );
  const description = $derived(
    context === 'root'
      ? historyType === 'audit'
        ? 'All application and installation events'
        : 'Webhook delivery failures across every installation'
      : historyType === 'audit'
        ? 'Account and repository configuration changes'
        : 'Webhook deliveries that need investigation',
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
  const auditTable = createTable({
    features: HISTORY_TABLE_FEATURES,
    columns: AUDIT_COLUMNS,
    get data() {
      return auditRows;
    },
    getRowId: (entry) => entry.id,
    manualFiltering: true,
    manualSorting: true,
    state: {
      get sorting() {
        return historySortingState('audit');
      },
      get columnFilters() {
        return [
          { id: 'target', value: auditScope },
          { id: 'change', value: context === 'root' ? auditCategories : auditChange },
        ];
      },
    },
    onSortingChange: (next) => selectHistorySorting('audit', next),
    onColumnFiltersChange: selectAuditColumnFilters,
  });
  const failureTable = createTable({
    features: HISTORY_TABLE_FEATURES,
    columns: FAILURE_COLUMNS,
    get data() {
      return failureRows;
    },
    getRowId: (failure) => failure.id,
    manualFiltering: true,
    manualSorting: true,
    state: {
      get sorting() {
        return historySortingState('failures');
      },
      get columnFilters() {
        return [{ id: 'status', value: failureKind }];
      },
    },
    onSortingChange: (next) => selectHistorySorting('failures', next),
    onColumnFiltersChange: selectFailureColumnFilters,
  });
  const auditTableRows = $derived(auditTable.getRowModel().rows);
  const failureTableRows = $derived(failureTable.getRowModel().rows);
  const desktopTableLayout = new MediaQuery('min-width: 64.001rem', true);
  const auditVirtualizer = createVirtualizer<HTMLTableSectionElement, HTMLTableRowElement>({
    count: 0,
    estimateSize: () => 48,
    getScrollElement: () => auditScroll ?? null,
    overscan: 6,
  });
  const failureVirtualizer = createVirtualizer<HTMLTableSectionElement, HTMLTableRowElement>({
    count: 0,
    estimateSize: () => 48,
    getScrollElement: () => failureScroll ?? null,
    overscan: 6,
  });
  const auditRenderRows: VirtualRenderRow[] = $derived.by(() =>
    desktopTableLayout.current
      ? $auditVirtualizer.getVirtualItems().map((row) => ({ ...row, virtual: true as const }))
      : auditTableRows.map((row, index) => ({
          index,
          key: row.id,
          size: 0,
          start: 0,
          virtual: false as const,
        })),
  );
  const failureRenderRows: VirtualRenderRow[] = $derived.by(() =>
    desktopTableLayout.current
      ? $failureVirtualizer.getVirtualItems().map((row) => ({ ...row, virtual: true as const }))
      : failureTableRows.map((row, index) => ({
          index,
          key: row.id,
          size: 0,
          start: 0,
          virtual: false as const,
        })),
  );

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
  // `section` (the installation views pass none) would snap every local
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

  $effect(() => {
    const rows = auditTableRows;
    const desktop = desktopTableLayout.current;
    untrack(() => {
      get(auditVirtualizer).setOptions({
        count: desktop ? rows.length : 0,
        getScrollElement: () => auditScroll ?? null,
        getItemKey: (index) => rows[index]?.id ?? index,
      });
    });
  });

  $effect(() => {
    const rows = failureTableRows;
    const desktop = desktopTableLayout.current;
    untrack(() => {
      get(failureVirtualizer).setOptions({
        count: desktop ? rows.length : 0,
        getScrollElement: () => failureScroll ?? null,
        getItemKey: (index) => rows[index]?.id ?? index,
      });
    });
  });

  $effect(() => {
    if (!desktopTableLayout.current) return;
    const rows = historyType === 'audit' ? auditTableRows : failureTableRows;
    const items =
      historyType === 'audit'
        ? $auditVirtualizer.getVirtualItems()
        : $failureVirtualizer.getVirtualItems();
    const last = items.at(-1);
    if (last !== undefined && last.index >= rows.length - 5) {
      untrack(() => void loadNextPage());
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

  function toggleSort(
    column: 'actor' | 'target' | 'change' | 'status' | 'repository' | 'when',
  ): void {
    const table = historyType === 'audit' ? auditTable : failureTable;
    const target = table.getColumn(column);
    target?.toggleSorting(target.getIsSorted() === 'asc');
  }

  function sortDirection(
    column: 'actor' | 'target' | 'change' | 'status' | 'repository' | 'when',
  ): 'ascending' | 'descending' | undefined {
    const table = historyType === 'audit' ? auditTable : failureTable;
    const direction = table.getColumn(column)?.getIsSorted();
    return direction === 'asc' ? 'ascending' : direction === 'desc' ? 'descending' : undefined;
  }

  function historySortingState(type: HistoryType): SortingState {
    const allowed =
      type === 'audit'
        ? new Set<HistorySort>([
            'actor_asc',
            'actor_desc',
            'target_asc',
            'target_desc',
            'change_asc',
            'change_desc',
          ])
        : new Set<HistorySort>(['status_asc', 'status_desc', 'repository_asc', 'repository_desc']);
    if (sort === 'newest' || sort === 'oldest') return [{ id: 'when', desc: sort === 'newest' }];
    if (!allowed.has(sort)) return [{ id: 'when', desc: true }];
    const [id, direction] = sort.split('_');
    return [{ id: id ?? 'when', desc: direction === 'desc' }];
  }

  function selectHistorySorting(type: HistoryType, next: Updater<SortingState>): void {
    const current = historySortingState(type);
    const resolved = typeof next === 'function' ? next(current) : next;
    const selected = resolved[0];
    if (selected === undefined) return;
    if (selected.id === 'when') {
      sort = selected.desc ? 'newest' : 'oldest';
      return;
    }
    const candidate = `${selected.id}_${selected.desc ? 'desc' : 'asc'}` as HistorySort;
    sort = candidate;
  }

  function selectAuditColumnFilters(next: Updater<ColumnFiltersState>): void {
    const current: ColumnFiltersState = [
      { id: 'target', value: auditScope },
      { id: 'change', value: auditChange },
    ];
    const resolved = typeof next === 'function' ? next(current) : next;
    const target = resolved.find((filter) => filter.id === 'target')?.value;
    const change = resolved.find((filter) => filter.id === 'change')?.value;
    selectAuditScope(target === undefined ? ['all'] : [String(target)]);
    if (context === 'root') {
      selectAuditCategories(
        Array.isArray(change)
          ? change.map(String)
          : change === undefined
            ? ['all']
            : [String(change)],
      );
    } else {
      selectAuditChange(change === undefined ? ['all'] : [String(change)]);
    }
  }

  function selectFailureColumnFilters(next: Updater<ColumnFiltersState>): void {
    const current: ColumnFiltersState = [{ id: 'status', value: failureKind }];
    const resolved = typeof next === 'function' ? next(current) : next;
    const value = resolved.find((filter) => filter.id === 'status')?.value;
    selectFailureKind(value === undefined ? ['all'] : [String(value)]);
  }

  function auditEntryAt(index: number): AuditEntry {
    const row = auditTableRows[index];
    if (row === undefined) throw new Error(`missing virtual audit row ${index}`);
    return row.original;
  }

  function failureAt(index: number): DeliveryFailure {
    const row = failureTableRows[index];
    if (row === undefined) throw new Error(`missing virtual failure row ${index}`);
    return row.original;
  }

  function selectAuditScope(values: string[]): void {
    const value = values[0];
    if (value === 'all' || value === 'account' || value === 'repositories') auditScope = value;
  }

  function selectAuditChange(values: string[]): void {
    const value = values[0];
    if (
      value === 'all' ||
      value === 'enablement' ||
      value === 'repository' ||
      value === 'account' ||
      value === 'sync'
    ) {
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

  function auditScopeLabel(): string {
    return (
      AUDIT_SCOPE_FILTERS[0]?.options.find((option) => option.value === auditScope)?.label ?? ''
    );
  }

  function auditChangeLabel(): string {
    return (
      AUDIT_CHANGE_FILTERS[0]?.options.find((option) => option.value === auditChange)?.label ?? ''
    );
  }

  function auditCategoryLabel(): string {
    if (auditCategories.length === 0) return 'All event categories';
    if (auditCategories.length === 1) {
      return (
        ROOT_AUDIT_CATEGORY_FILTERS[0]?.options.find(
          (option) => option.value === auditCategories[0],
        )?.label ?? 'Event category'
      );
    }
    return `${auditCategories.length} event categories`;
  }

  function failureKindLabel(): string {
    return (
      FAILURE_KIND_FILTERS[0]?.options.find((option) => option.value === failureKind)?.label ?? ''
    );
  }

  function displayTime(value: string): string {
    return timeDisplay === 'relative' ? formatRelative(value, now) : formatDateTime(value);
  }

  function auditSummary(value: string): string {
    return sentenceCase(value.replace(/\s+for\s*$/i, ''));
  }

  function sentenceCase(value: string): string {
    const text = value.trim().replace(/\.+$/, '');
    if (text === '') return text;

    return text.charAt(0).toLocaleUpperCase() + text.slice(1);
  }

  function auditDetail(entry: AuditEntry): string {
    const parts = [entry.category, entry.action].filter(
      (part): part is string => part !== undefined,
    );
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

  function scrollResultsToTop(): void {
    if (isDesktopTableLayout()) {
      historyResults?.querySelector<HTMLElement>('[data-panel-scroll]')?.scrollTo({ top: 0 });
    }
  }

  function isDesktopTableLayout(): boolean {
    return window.matchMedia('(min-width: 64.001rem)').matches;
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

  function openCheckpoint(entry: AuditEntry, trigger: HTMLElement): void {
    if (entry.sync_config_checkpoint_id === undefined || fetchSyncCheckpoint === undefined) return;
    checkpointTrigger = trigger;
    checkpointId = entry.sync_config_checkpoint_id;
    checkpointTargetId = entry.target_id ?? targetId;
  }

  function closeCheckpoint(): void {
    checkpointId = null;
    checkpointTargetId = null;
  }

  function syncRestored(result: SyncConfigBatchResponse): void {
    onSyncRestored?.(result);
    void auditQuery.refetch();
  }

  function unavailableRestore(): Promise<SyncConfigBatchResponse> {
    return Promise.reject(new Error('this account cannot restore Sync configuration'));
  }
</script>

<section
  class="plate history-panel"
  class:root-context={context === 'root'}
  class:absolute-time={timeDisplay === 'absolute'}
  aria-labelledby={context === 'root' ? 'root-page-heading' : 'history-heading'}
>
  {#if context === 'root'}
    <RootPageHeader
      role={rootRole}
      title={historyType === 'audit' ? 'Audit' : 'Failures'}
      subtitle={description}
    />
  {:else}
    <PageHeader
      id="history-heading"
      eyebrow="History"
      title={historyType === 'audit' ? 'Audit' : 'Failures'}
      {description}
    />
  {/if}

  <div class="history-tools">
    <SearchField
      label="Search history"
      placeholder={historyType === 'audit' ? 'Search changes' : 'Search failures'}
      value={search}
      onInput={(value) => (search = value)}
    />

    <HistoryDisplayMenu value={timeDisplay} onSelect={selectTimeDisplay} />
  </div>

  <div
    class:loading
    class="history-results table-region"
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
      <DataTable
        class="table-scroll"
        tableClass="history-table audit-table"
        caption="Audit history"
        regionLabel="Audit history table"
        rows={auditRenderRows}
        rowKey={(virtualRow) => virtualRow.key}
        columnCount={4}
        bind:body={auditScroll}
        rowAttrs={(virtualRow) =>
          virtualRow.virtual
            ? {
                class: 'data-row virtual-row',
                /* The offset travels as `--row-y`, never as an inline `transform`.
                   `.data-row` composes the translate with the press scale in the
                   stylesheet; an inline transform replaces the whole thing, and the
                   scale is silently dropped. */
                style: `height:${virtualRow.size}px;--row-y:${virtualRow.start}px`,
              }
            : { class: 'data-row' }}
      >
        {#snippet colgroup()}
          <colgroup>
            <col class="actor-column" />
            <col class="target-column" />
            <col class="change-column" />
            <col class="time-column" />
          </colgroup>
        {/snippet}
        {#snippet head()}
          <tr>
            <th scope="col" aria-sort={sortDirection('actor')}>
              <div class="table-heading">
                <button class="table-sort-button" type="button" onclick={() => toggleSort('actor')}>
                  <span class="table-heading-label">Actor</span>
                  <SortIndicator />
                </button>
              </div>
            </th>
            <th scope="col" aria-sort={sortDirection('target')}>
              <div class="table-heading">
                <button
                  class="table-sort-button"
                  type="button"
                  onclick={() => toggleSort('target')}
                >
                  <span class="table-heading-label">Target</span>
                  <SortIndicator />
                </button>
                {#if context === 'installation'}
                  <FilterMenu
                    label="Target"
                    summary={auditScopeLabel()}
                    hint="Choose which configuration changes to show"
                    sections={AUDIT_SCOPE_FILTERS}
                    selected={[auditScope]}
                    fallbackValue="all"
                    align="end"
                    onChange={(values) => auditTable.getColumn('target')?.setFilterValue(values[0])}
                  />
                {/if}
              </div>
            </th>
            <th scope="col" aria-sort={sortDirection('change')}>
              <div class="table-heading">
                <button
                  class="table-sort-button"
                  type="button"
                  onclick={() => toggleSort('change')}
                >
                  <span class="table-heading-label">Change</span>
                  <SortIndicator />
                </button>
                {#if context === 'root'}
                  <FilterMenu
                    label="Event category"
                    summary={auditCategoryLabel()}
                    hint="Choose which application events to show"
                    sections={ROOT_AUDIT_CATEGORY_FILTERS}
                    selected={auditCategories.length === 0 ? ['all'] : auditCategories}
                    fallbackValue="all"
                    align="end"
                    multiple
                    onChange={(values) => auditTable.getColumn('change')?.setFilterValue(values)}
                  />
                {:else}
                  <FilterMenu
                    label="Change"
                    summary={auditChangeLabel()}
                    hint="Choose which configuration changes to show"
                    sections={AUDIT_CHANGE_FILTERS}
                    selected={[auditChange]}
                    fallbackValue="all"
                    align="end"
                    onChange={(values) => auditTable.getColumn('change')?.setFilterValue(values[0])}
                  />
                {/if}
              </div>
            </th>
            <th scope="col" aria-sort={sortDirection('when')}>
              <div class="table-heading">
                <button class="table-sort-button" type="button" onclick={() => toggleSort('when')}>
                  <span class="table-heading-label">When</span>
                  <SortIndicator />
                </button>
              </div>
            </th>
          </tr>
        {/snippet}
        {#snippet lead()}
          {#if desktopTableLayout.current}
            <tr
              class="virtual-spacer"
              aria-hidden="true"
              style:height={`${$auditVirtualizer.getTotalSize()}px`}><td colspan="4"></td></tr
            >
          {/if}
        {/snippet}
        {#snippet cells(virtualRow)}
          {@const entry = auditEntryAt(virtualRow.index)}
          <td data-label="Actor">
            <span class="actor">
              <Avatar account={entry.actor} size={24} />
              <span class="actor-copy">
                <strong class="band-trim">{entry.actor.display_name}</strong>
                <small class="actor-login mono band-trim">@{entry.actor.login}</small>
              </span>
            </span>
          </td>
          <td data-label="Target">
            {#if context === 'root' && entry.installation !== undefined}
              <span class="cell-primary band-trim" title={`@${entry.installation.login}`}>
                {entry.installation.display_name}
              </span>
            {:else if context === 'root'}
              <span class="cell-primary band-trim">Smyklot</span>
            {:else if entry.repository_full_name !== undefined}
              <code class="band-trim" title={entry.repository_full_name}>
                {repositoryName(entry.repository_full_name)}
              </code>
            {:else}
              <span class="cell-primary band-trim">Account</span>
            {/if}
          </td>
          <td data-label="Change" title={auditDetail(entry)}>
            <span class="change-line">
              {#if entry.category !== undefined}
                <!-- Symmetric about its own band, so the equal padding above and
                     below is the whole of what centres the word on the tag. -->
                <span class="category-tag band-trim" aria-hidden="true">{entry.category}</span>
              {/if}
              {#if entry.sync_config_checkpoint_id !== undefined && fetchSyncCheckpoint !== undefined}
                <button
                  class="checkpoint-trigger band-trim"
                  type="button"
                  aria-label={`${auditSummary(entry.summary)}. Inspect Sync configuration snapshot`}
                  onclick={(event) => openCheckpoint(entry, event.currentTarget)}
                >
                  <span>{auditSummary(entry.summary)}</span>
                  <small>Inspect</small>
                </button>
              {:else}
                <span class="cell-primary band-trim">{auditSummary(entry.summary)}</span>
              {/if}
            </span>
          </td>
          <td data-label="When">
            <time
              class="table-time band-trim"
              datetime={entry.created_at}
              title={formatTimestamp(entry.created_at)}
            >
              {displayTime(entry.created_at)}
            </time>
          </td>
        {/snippet}
        {#snippet empty()}
          <TableEmptyState
            title="No configuration changes found"
            description={hasFilters
              ? 'Try another search or clear the active filters'
              : 'Configuration changes will appear here'}
            actionLabel={hasFilters ? 'Clear filters' : undefined}
            onAction={hasFilters ? clearFilters : undefined}
          />
        {/snippet}
      </DataTable>
    {:else}
      <DataTable
        class="table-scroll"
        tableClass="history-table failure-table"
        caption="Delivery failure history"
        regionLabel="Delivery failure history table"
        rows={failureRenderRows}
        rowKey={(virtualRow) => virtualRow.key}
        columnCount={4}
        bind:body={failureScroll}
        rowAttrs={(virtualRow) =>
          virtualRow.virtual
            ? {
                class: 'failure-row data-row virtual-row',
                /* The offset travels as `--row-y`, never as an inline `transform`.
                   `.data-row` composes the translate with the press scale in the
                   stylesheet; an inline transform replaces the whole thing, and the
                   scale is silently dropped. */
                style: `height:${virtualRow.size}px;--row-y:${virtualRow.start}px`,
              }
            : { class: 'failure-row data-row' }}
      >
        {#snippet colgroup()}
          <colgroup>
            <col class="status-column" />
            <col class="repository-column" />
            <col class="failure-column" />
            <col class="time-column" />
          </colgroup>
        {/snippet}
        {#snippet head()}
          <tr>
            <th scope="col" aria-sort={sortDirection('status')}>
              <div class="table-heading">
                <button
                  class="table-sort-button"
                  type="button"
                  onclick={() => toggleSort('status')}
                >
                  <span class="table-heading-label">Status</span>
                  <SortIndicator />
                </button>
                <FilterMenu
                  label="Status"
                  summary={failureKindLabel()}
                  hint="Choose which delivery failures to show"
                  sections={FAILURE_KIND_FILTERS}
                  selected={[failureKind]}
                  fallbackValue="all"
                  align="end"
                  onChange={(values) => failureTable.getColumn('status')?.setFilterValue(values[0])}
                />
              </div>
            </th>
            <th scope="col" aria-sort={sortDirection('repository')}>
              <div class="table-heading">
                <button
                  class="table-sort-button"
                  type="button"
                  onclick={() => toggleSort('repository')}
                >
                  <span class="table-heading-label">Repository</span>
                  <SortIndicator />
                </button>
              </div>
            </th>
            <th scope="col">
              <div class="table-heading"><span class="table-heading-label">Failure</span></div>
            </th>
            <th scope="col" aria-sort={sortDirection('when')}>
              <div class="table-heading">
                <button class="table-sort-button" type="button" onclick={() => toggleSort('when')}>
                  <span class="table-heading-label">When</span>
                  <SortIndicator />
                </button>
              </div>
            </th>
          </tr>
        {/snippet}
        {#snippet lead()}
          {#if desktopTableLayout.current}
            <tr
              class="virtual-spacer"
              aria-hidden="true"
              style:height={`${$failureVirtualizer.getTotalSize()}px`}><td colspan="4"></td></tr
            >
          {/if}
        {/snippet}
        {#snippet cells(virtualRow)}
          {@const failure = failureAt(virtualRow.index)}
          <td data-label="Status">
            <span class={['failure-kind', failure.retryable ? 'retryable' : 'permanent']}>
              <span class="cell-symbol" aria-hidden="true">
                <Icon name={failure.retryable ? 'refresh' : 'failure'} size={14} />
              </span>
              <span class="cap-trim">{failure.retryable ? 'Retryable' : 'Permanent'}</span>
            </span>
          </td>
          <td data-label="Repository">
            <code
              class="band-trim"
              title={failure.installation === undefined
                ? failure.repository_full_name
                : `${failure.repository_full_name} \u00b7 @${failure.installation.login}`}
            >
              {repositoryName(failure.repository_full_name)}
            </code>
          </td>
          <td data-label="Failure" title={failureDetail(failure)}>
            <span class="cell-primary band-trim">{sentenceCase(failure.reason)}</span>
          </td>
          <td data-label="When">
            <time
              class="table-time band-trim"
              datetime={failure.occurred_at}
              title={formatTimestamp(failure.occurred_at)}
            >
              {displayTime(failure.occurred_at)}
            </time>
          </td>
        {/snippet}
        {#snippet empty()}
          <TableEmptyState
            title="No delivery failures found"
            description={hasFilters
              ? 'Try another search or clear the active filters'
              : 'Delivery failures will appear here'}
            actionLabel={hasFilters ? 'Clear filters' : undefined}
            onAction={hasFilters ? clearFilters : undefined}
          />
        {/snippet}
      </DataTable>
    {/if}
    <InfiniteLoadSentinel
      active={!desktopTableLayout.current &&
        !loading &&
        loadMoreProblem === null &&
        currentPage?.next_cursor != null}
      cursor={currentPage?.next_cursor}
      onVisible={() => void loadNextPage()}
    />
    {#if loadMoreProblem !== null}
      <div class="load-more-alert" role="alert">
        <span>{loadMoreProblem}</span>
        <Button onclick={() => void loadNextPage()}>Try again</Button>
      </div>
    {/if}
  </div>
</section>

{#if checkpointId !== null && checkpointTargetId !== null && fetchSyncCheckpoint !== undefined}
  <SyncCheckpointDialog
    open
    targetId={checkpointTargetId}
    {checkpointId}
    readOnly={readOnly || restoreSyncCheckpoint === undefined}
    hasUnsavedDrafts={hasUnsavedSyncDrafts}
    returnFocus={checkpointTrigger}
    fetchCheckpoint={fetchSyncCheckpoint}
    restoreCheckpoint={restoreSyncCheckpoint ?? unavailableRestore}
    onRestored={syncRestored}
    onClose={closeCheckpoint}
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

  .history-tools {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: 0;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: auto minmax(12rem, 1fr) auto;
    padding: 0 0 var(--space-3);
  }

  /* Layout, keyline and corner come from `.table-region` in `app.css`. */
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

  /* Surface, keyline, corner and lift come from `.table-card` in `app.css`. */
  :global(.table-scroll) {
    max-width: 100%;
  }

  :global(.history-table) {
    background: var(--surface-base);
    /* Separated, not collapsed: a collapsed border is shared between adjacent
       rows, so each cell owns half of it and every row box lands on a .5. */
    border-collapse: separate;
    border-spacing: 0;
    min-width: 40rem;
    table-layout: fixed;
    width: 100%;
  }

  /* The header's own rule comes from `thead th` in `app.css`, so every table in
     the product draws it the same. This is the separator between rows. */
  :global(.history-table td) {
    border-bottom: 1px solid var(--rule);
    font-size: var(--font-size-meta);
    padding: 0.625rem 0.75rem;
    text-align: left;
    vertical-align: middle;
  }

  :global(.history-table td:first-child) {
    padding-left: var(--space-3);
  }

  :global(.history-table td:last-child) {
    padding-right: var(--space-3);
  }

  :global(.failure-table th:last-child),
  :global(.failure-table td:last-child),
  :global(.audit-table th:last-child),
  :global(.audit-table td:last-child) {
    text-align: right;
  }

  /* The header band is 2.5rem of content plus its own rule. Putting the height
     on the th instead would fold the border into it and leave the band 1px
     shallower than the other four tables. The rest of the heading - the cell
     with no padding, the button carrying it, the inset a wordless heading takes
     - is shared, in `thead th` and `.table-heading` in `app.css`. */
  :global(.history-table thead .table-heading) {
    height: 2.5rem;
  }

  :global(.history-table tbody tr) {
    transition: background-color var(--duration-fast) var(--ease-standard);
  }

  .checkpoint-trigger {
    align-items: baseline;
    background: transparent;
    border: 0;
    color: inherit;
    cursor: pointer;
    display: inline-flex;
    font: inherit;
    gap: var(--space-2);
    min-width: 0;
    padding: 0;
    text-align: left;
  }

  .checkpoint-trigger span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .checkpoint-trigger small {
    color: var(--accent);
    flex: none;
    font-size: var(--font-size-compact);
  }

  .checkpoint-trigger:hover span,
  .checkpoint-trigger:focus-visible span {
    text-decoration: underline;
  }

  @media (min-width: 64.001rem) {
    .history-results {
      min-height: 0;
      overflow: hidden;
    }

    :global(.table-scroll) {
      display: flex;
      flex: 1;
      min-height: 0;
      overflow-x: auto;
    }

    :global(.history-table) {
      display: flex;
      flex: 1;
      flex-direction: column;
      min-height: 0;
    }

    :global(.history-table colgroup) {
      display: none;
    }

    :global(.history-table thead) {
      display: block;
      flex: none;
    }

    :global(.history-table tbody) {
      background: var(--table-filler-bg);
      display: block;
      flex: 1;
      min-height: 0;
      overflow-y: auto;
      position: relative;
    }

    :global(.history-table thead tr),
    :global(.history-table tbody tr) {
      display: grid;
      grid-template-columns: var(--history-columns);
      width: 100%;
    }

    /* Pin the grid track to the row's fixed height: auto-sizing would take the
       tallest cell's border-box, push the bottom border one pixel past the
       virtual row, and let the next row paint over every separator. */
    :global(.history-table tbody tr:not(.virtual-spacer)) {
      grid-template-rows: 100%;
    }

    /* Only the rows not handed to `.data-row` - see the same pair in
       `UserManagement`. Painted on every row, one wearing the class never showed
       the shared hover, because this rule outranks `app.css` by a class. */
    :global(.history-table tbody tr:not(.virtual-spacer, .data-row)) {
      background: var(--surface-base);
    }

    :global(.history-table tbody tr:not(.virtual-spacer) td) {
      align-content: center;
      display: grid;
    }

    :global(.history-table tbody tr:not(.virtual-spacer) td:last-child) {
      justify-items: end;
    }

    /* In flow, and tall enough to be seen.
       ------------------------------------
       This used to fill the body it sits in - `position: absolute; inset: 0` -
       which works only while something else is giving that body a height. Nothing
       does: the rows are the height, they are what is missing, and the workspace
       is sized to its content rather than to the window on purpose. So the body
       measured zero, the row measured zero inside it, and a search that matched
       nothing answered with a column header and a strip of background. Both
       sections, and it was the failures one that got looked at.

       The row carries the height itself now, which is also what makes the card
       around it the size of an answer rather than the size of a header. */
    :global(.history-table tbody .state-row) {
      background: var(--surface-base);
      border: 0;
      display: flex;
      min-height: 9rem;
    }

    :global(.history-table tbody .state-row .empty-cell) {
      align-items: center;
      display: flex;
      flex: 1;
      grid-column: 1 / -1;
      justify-content: center;
      padding: var(--space-6);
    }

    :global(.history-table tbody .virtual-row) {
      left: 0;
      position: absolute;
      top: 0;
    }

    :global(.history-table tbody .virtual-spacer) {
      background: transparent;
      border: 0;
      display: block;
      pointer-events: none;
      width: 1px;
    }

    .virtual-spacer td {
      display: none;
    }

    :global(.audit-table) {
      --history-columns: minmax(13rem, 1.1fr) minmax(9rem, 0.8fr) minmax(0, 2.2fr) 7.5rem;
    }

    :global(.failure-table) {
      --history-columns: minmax(0, 1.1fr) minmax(0, 1.2fr) minmax(0, 2.4fr) minmax(0, 1fr);
    }

    :global(.absolute-time .audit-table) {
      --history-columns: minmax(13rem, 1.1fr) minmax(9rem, 0.8fr) minmax(0, 2.2fr) 9.5rem;
    }

    :global(.absolute-time .failure-table) {
      --history-columns: 8rem minmax(9rem, 0.9fr) minmax(0, 2.4fr) 9.5rem;
    }
  }

  /* The heading's row, its button and its arrow are all shared now - see
     `.table-heading`, `.table-sort-button` and `.sort-indicator` in `app.css`.
     What was here was a second copy of the reset, a `:global(.header-filter)`
     addressed to a class the popover stopped rendering, and a `flex: 1` the
     shared class states itself. */
  :global(.history-table th:last-child .table-sort-button) {
    /* Right-aligned sortable column: the indicator leads, so the label ink lands
       on the same edge as the times below it. Which side the arrow takes follows
       the column's alignment - the one rule every design system agrees on, and
       the reason an end-aligned heading does not read as indented. */
    flex-direction: row-reverse;
    justify-content: flex-start;
  }

  /* One repository token for the whole panel: the audit table's Target and the
     failure table's Repository name the same thing, so they wear the same mono
     chip the repositories pane uses rather than one chip and one bare string.
     `clip` rather than `hidden` - the trim ends the box at the baseline and a
     hidden overflow would shave the descenders. */
  :global(.history-table code) {
    background: var(--surface-inset);
    border-radius: var(--r-chip);
    color: var(--text-soft);
    display: inline-block;
    font: 500 var(--font-size-compact) / 1 var(--mono);
    justify-self: start;
    max-width: 100%;
    overflow: clip;
    overflow-clip-margin: 0.35em;
    padding: 0.34rem 0.5rem;
    text-overflow: ellipsis;
    vertical-align: middle;
    white-space: nowrap;
    width: max-content;
  }

  .actor-column {
    width: 8.5rem;
  }

  .target-column,
  .repository-column {
    width: 11rem;
  }

  .status-column {
    width: 7rem;
  }

  .time-column {
    width: 7.5rem;
  }

  .absolute-time .time-column {
    width: 9.5rem;
  }

  /* Name over handle, the same identity block the installations catalog uses.
     Both lines are trimmed to cap..baseline, so the pair's box equals its ink
     and centring the row centres what the eye reads rather than a line box. */
  .actor {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    min-width: 0;
  }

  .actor-copy {
    display: block;
    min-width: 0;
  }

  .actor strong {
    display: block;
    font: 650 var(--font-size-meta) / 1 var(--sans);
    /* clip, not hidden: the trim ends the box at the baseline, so a hidden
       overflow would shave the descenders off a name like "Bart Smykla". */
    overflow: clip;
    overflow-clip-margin: 0.35em;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .actor-login {
    color: var(--text-muted);
    display: block;
    font: 400 var(--font-size-micro) / 1 var(--mono);
    margin-top: 0.45rem;
    min-width: 0;
    overflow: clip;
    overflow-clip-margin: 0.35em;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .failure-kind {
    align-items: center;
    display: inline-flex;
    font: 600 var(--font-size-meta) / 1.5 var(--sans);
    gap: var(--space-2);
    white-space: nowrap;
  }

  .failure-kind.retryable {
    color: var(--warning);
  }

  .failure-kind.permanent {
    color: var(--stop);
  }

  .cell-symbol {
    display: grid;
    flex: none;
    height: 1.125rem;
    place-items: center;
    width: 1.125rem;
  }

  .change-line {
    align-items: center;
    display: flex;
    gap: 0.5rem;
    min-width: 0;
  }

  /* The event category rides along as a quiet tag; the raw event code lives
     in the row tooltip rather than repeating under every line. */
  .category-tag {
    background: var(--neutral-tint);
    border-radius: 5px;
    color: var(--text-soft);
    flex: none;
    font: 650 0.65rem / 1 var(--sans);
    letter-spacing: 0.04em;
    padding: 0.2rem 0.35rem;
    text-transform: uppercase;
  }

  /* One rule, not the two that had grown here - the second quietly replaced the
     first's `overflow-wrap` reasoning with `nowrap` and an ellipsis.
     `overflow: clip` rather than `hidden`, with a margin: the trim ends this box
     on the baseline, so `hidden` would shave the tail off every g, p and y in the
     table. The margin is vertical room the clip gives back; horizontally there is
     nothing to give back, since the ellipsis truncates inside the box.
     `overflow-wrap` does nothing against `nowrap` here, and earns its place on a
     phone, where the card lets the text wrap and a long name has to break. */
  .cell-primary {
    display: block;
    font-size: var(--font-size-meta);
    line-height: 1.5;
    overflow: clip;
    overflow-clip-margin: 0.35em;
    overflow-wrap: anywhere;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Block-level, so `.band-trim` has line boxes to trim and the cell has a box to
     centre. As an inline-flex it had neither: the trim was a no-op on a flex
     container, and inline it rode the row's strut instead of the cell's middle,
     which put the timestamp 0.59px below every other column. */
  .table-time {
    color: var(--dim);
    display: block;
    font-size: var(--font-size-meta);
    line-height: 1.5;
    white-space: nowrap;
    width: fit-content;
  }

  /* Shorter than the shared 12rem, because this panel's empty state sits under a
     toolbar and a section switch and a full one pushed both off a laptop screen. */
  :global(.table-scroll) {
    --table-empty-height: 9rem;
  }

  :global(.table-scroll .empty-cell) {
    text-align: center !important;
  }

  @media (max-width: 48rem) {
    :global(.table-scroll) {
      overflow: visible;
      padding: var(--space-3);
    }

    :global(.history-table) {
      display: block;
      min-width: 0;
    }

    :global(.history-table colgroup) {
      clip-path: inset(50%);
      height: 1px;
      overflow: hidden;
      position: absolute;
      white-space: nowrap;
      width: 1px;
    }

    :global(.history-table thead) {
      display: block;
    }

    /* Wrapped, and from the start rather than the end. Four sort chips do not
       fit one phone-width line, and a flex row justified to the end overflows
       backwards: the first chip - Actor - hung 52px off the left of the screen,
       where nothing can scroll to it and nothing says it is there. */
    :global(.history-table thead tr) {
      border: 0;
      display: flex;
      flex-wrap: wrap;
      justify-content: flex-start;
      padding: 0 0 var(--space-3);
    }

    :global(.history-table thead th) {
      padding: 0;
    }

    :global(.history-table thead th:not(:has(.table-sort-button))) {
      clip-path: inset(50%);
      height: 1px;
      overflow: hidden;
      position: absolute;
      white-space: nowrap;
      width: 1px;
    }

    /* On a phone a heading is a chip in a wrapped row, not a band, so it takes a
       real ground and its own width - which is the one place the shared full-cell
       target does not apply, because there is no cell left to fill. The funnel
       goes back into the flow beside the words for the same reason: there is no
       cell for it to ride. */
    :global(.history-table thead .table-heading),
    :global(.history-table thead .table-sort-button) {
      height: var(--control-height-compact);
      width: auto;
    }

    :global(.history-table thead .filter-trigger) {
      inset: auto;
      margin-block: 0;
      position: relative;
    }

    :global(.history-table thead .table-sort-button) {
      background: var(--control-bg);
      border: 1px solid var(--control-border);
      border-radius: var(--radius-control);
      color: var(--dim);
      padding-inline: var(--space-3);
    }

    :global(.history-table thead .table-sort-button:hover),
    :global(.history-table thead .table-sort-button:focus-visible) {
      background: var(--control-bg-hover);
      color: var(--text);
    }

    :global(.history-table tbody) {
      display: grid;
      gap: var(--space-2);
    }

    :global(.history-table tbody tr) {
      background: var(--surface-raised);
      border: 1px solid var(--border-subtle);
      border-radius: var(--radius-control);
      display: grid;
      gap: var(--space-3);
      grid-template-columns: repeat(2, minmax(0, 1fr));
      padding: var(--space-3);
    }

    :global(.history-table td) {
      border: 0;
      display: grid;
      gap: var(--space-1);
      padding: 0;
      text-align: left !important;
    }

    :global(.history-table td::before) {
      color: var(--text-muted);
      content: attr(data-label);
      font: 650 var(--font-size-compact) / 1 var(--sans);
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }

    /* What the entry is actually about, given the card's width to say it in.
       Sharing a two-column row with the timestamp, and standing beside the
       category tag inside its own cell, the description had 30px: every audit
       entry on the Root console read "CONFIGURATION Up…" while the half of the
       card beside it was empty. */
    :global(.history-table td[data-label='Change']),
    :global(.history-table td[data-label='Failure']) {
      grid-column: 1 / -1;
    }

    /* A card has no column to keep to, so the line wraps rather than being cut.
       Truncation is a table's answer to a fixed column width, and there is no
       fixed column here. */
    :global(.history-table .cell-primary) {
      overflow: visible;
      white-space: normal;
    }

    :global(.history-table .empty-cell) {
      display: block;
      grid-column: 1 / -1;
      height: auto;
      padding: var(--space-5);
    }

    :global(.history-table .empty-cell::before) {
      display: none;
    }
  }

  @media (max-width: 36rem) {
    .history-tools {
      grid-template-columns: 1fr auto;
    }

    .history-tools :global(.search-field) {
      grid-column: 1 / -1;
    }
  }

  @media (max-width: 22rem) {
    .history-tools {
      grid-template-columns: 1fr;
    }

    .history-tools :global(.search-field) {
      grid-column: auto;
    }

    :global(.history-table tbody tr) {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
