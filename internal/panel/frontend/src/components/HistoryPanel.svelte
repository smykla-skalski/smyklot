<script lang="ts">
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

  import { formatDateTime, formatRelative, formatTimestamp } from '../lib/format';
  import type { FilterSection } from '../lib/filter-menu';
  import type { TimeDisplay } from '../lib/preferences';
  import {
    EPHEMERAL_PREFS,
    prefOption,
    prefText,
    type PrefsAccessor,
  } from '../lib/preferences-sync';
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
  } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import Chip from './Chip.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import HistoryDisplayMenu from './HistoryDisplayMenu.svelte';
  import Icon from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import PanelHeader from './PanelHeader.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  type HistoryType = 'audit' | 'failures';
  type HistoryContext = 'installation' | 'root';

  const HISTORY_TYPES = [
    { value: 'audit', label: 'Audit', tone: 'accent' },
    { value: 'failures', label: 'Failures', tone: 'accent' },
  ] as const;

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
    refreshVersion,
    fetchAudit,
    fetchFailures,
    context = 'installation',
    section,
    onSection,
    prefs = EPHEMERAL_PREFS,
  }: {
    targetId: string;
    refreshVersion: number;
    fetchAudit: (request: AuditHistoryRequest) => Promise<Page<AuditEntry>>;
    fetchFailures: (request: FailureHistoryRequest) => Promise<Page<DeliveryFailure>>;
    context?: HistoryContext;
    section?: HistoryType;
    onSection?: (section: HistoryType) => void;
    prefs?: PrefsAccessor;
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
      ['all', 'enablement', 'repository', 'account'],
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
  let auditPage = $state<Page<AuditEntry> | null>(null);
  let failurePage = $state<Page<DeliveryFailure> | null>(null);
  let loading = $state(false);
  let problem = $state<string | null>(null);
  let loadMoreProblem = $state<string | null>(null);
  let now = $state(Date.now());
  let requestSequence = 0;
  let failureWarmupSequence = 0;
  let historyResults = $state<HTMLDivElement>();
  let auditScroll = $state<HTMLTableSectionElement>();
  let failureScroll = $state<HTMLTableSectionElement>();

  const currentPage = $derived(historyType === 'audit' ? auditPage : failurePage);
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
  const requestKey = $derived(
    [
      targetId,
      refreshVersion,
      historyType,
      appliedQuery,
      sort,
      auditScope,
      auditChange,
      auditCategories.join(','),
      failureKind,
      limit,
    ].join(':'),
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
  const auditRenderRows = $derived.by(() =>
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
  const failureRenderRows = $derived.by(() =>
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

  $effect(() => {
    const nextQuery = search.trim();
    const timer = window.setTimeout(() => {
      appliedQuery = nextQuery;
    }, 250);
    return () => window.clearTimeout(timer);
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
    void resetAndLoad(requestKey);
  });

  $effect(() => {
    const version = ++failureWarmupSequence;
    const expectedTarget = targetId;
    const expectedRefresh = refreshVersion;
    if (failurePage !== null || historyType !== 'audit') return;
    void fetchFailures({ query: '', sort: 'newest', limit, kind: 'all' })
      .then((loaded) => {
        if (
          version === failureWarmupSequence &&
          targetId === expectedTarget &&
          refreshVersion === expectedRefresh &&
          failurePage === null
        ) {
          failurePage = loaded;
        }
      })
      .catch(() => undefined);
  });

  $effect(() => {
    const rows = auditTableRows;
    get(auditVirtualizer).setOptions({
      count: desktopTableLayout.current ? rows.length : 0,
      getScrollElement: () => auditScroll ?? null,
      getItemKey: (index) => rows[index]?.id ?? index,
    });
  });

  $effect(() => {
    const rows = failureTableRows;
    get(failureVirtualizer).setOptions({
      count: desktopTableLayout.current ? rows.length : 0,
      getScrollElement: () => failureScroll ?? null,
      getItemKey: (index) => rows[index]?.id ?? index,
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
    if (last !== undefined && last.index >= rows.length - 5) void loadNextPage();
  });

  $effect(() => {
    const tick = window.setInterval(() => {
      now = Date.now();
    }, 30_000);
    return () => window.clearInterval(tick);
  });

  function selectHistoryType(value: string): void {
    if ((value === 'audit' || value === 'failures') && value !== historyType) {
      historyType = value;
      sort = 'newest';
      onSection?.(value);
    }
  }

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
      value === 'account'
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

  async function resetAndLoad(key: string): Promise<void> {
    loadMoreProblem = null;
    scrollResultsToTop();
    await loadPage(undefined, key, false);
  }

  async function loadPage(cursor: string | undefined, key: string, append: boolean): Promise<void> {
    const sequence = ++requestSequence;
    loading = true;
    if (!append) problem = null;
    else loadMoreProblem = null;
    try {
      if (historyType === 'audit') {
        const loaded = await fetchAudit({
          cursor,
          query: appliedQuery,
          sort,
          limit,
          scope: auditScope,
          change: auditChange,
          categories: context === 'root' ? auditCategories : undefined,
        });
        if (sequence === requestSequence && key === requestKey) {
          auditPage =
            append && auditPage !== null
              ? { ...loaded, items: [...auditPage.items, ...loaded.items] }
              : loaded;
        }
      } else {
        const loaded = await fetchFailures({
          cursor,
          query: appliedQuery,
          sort,
          limit,
          kind: failureKind,
        });
        if (sequence === requestSequence && key === requestKey) {
          failurePage =
            append && failurePage !== null
              ? { ...loaded, items: [...failurePage.items, ...loaded.items] }
              : loaded;
        }
      }
    } catch (error) {
      if (sequence === requestSequence && key === requestKey) {
        const message = error instanceof Error ? error.message : String(error);
        if (append) loadMoreProblem = message;
        else problem = message;
      }
    } finally {
      if (sequence === requestSequence && key === requestKey) {
        loading = false;
      }
    }
  }

  async function loadNextPage(): Promise<void> {
    const cursor = currentPage?.next_cursor;
    if (loading || cursor === null || cursor === undefined) return;
    await loadPage(cursor, requestKey, true);
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
    if (currentPage === null) void loadPage(undefined, requestKey, false);
    else void loadNextPage();
  }

  function clearFilters(): void {
    search = '';
    appliedQuery = '';
    auditScope = 'all';
    auditChange = 'all';
    auditCategories = [];
    failureKind = 'all';
  }
</script>

{#snippet headerActions()}
  <SegmentedControl
    name="history-type"
    label="History type"
    options={HISTORY_TYPES}
    value={historyType}
    align="end"
    onSelect={selectHistoryType}
  />
{/snippet}

<section
  class="plate history-panel"
  class:root-context={context === 'root'}
  class:absolute-time={timeDisplay === 'absolute'}
  aria-labelledby={context === 'root' ? 'root-page-heading' : 'history-heading'}
>
  {#if context === 'root'}
    <div class="root-history-navigation">
      <SegmentedControl
        name="root-history-type"
        label="Root history type"
        options={HISTORY_TYPES}
        value={historyType}
        onSelect={selectHistoryType}
      />
      <p>{description}</p>
    </div>
  {:else}
    <PanelHeader id="history-heading" title="History" {description} actions={headerActions} />
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

  <div class:loading class="history-results" bind:this={historyResults} aria-busy={loading}>
    {#if problem !== null}
      <div class="result-state" role="alert">
        <strong>History could not be loaded</strong>
        <span>{problem}</span>
        <button class="btn" onclick={retry}>Try again</button>
      </div>
    {:else if loading && currentPage === null}
      <div class="table-skeleton" aria-hidden="true">
        {#each [0, 1, 2, 3, 4, 5] as index (index)}
          <span></span>
        {/each}
      </div>
      <p class="visually-hidden" role="status">Loading history</p>
    {:else if historyType === 'audit'}
      <!-- Keyboard focus lets users scroll columns that overflow the viewport. -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div class="table-scroll" role="region" tabindex="0" aria-label="Audit history table">
        <table class="history-table audit-table">
          <caption class="visually-hidden">Audit history</caption>
          <colgroup>
            <col class="actor-column" />
            <col class="target-column" />
            <col class="change-column" />
            <col class="time-column" />
          </colgroup>
          <thead>
            <tr>
              <th scope="col" aria-sort={sortDirection('actor')}>
                <button
                  class="sort-button table-sort-button"
                  type="button"
                  onclick={() => toggleSort('actor')}
                >
                  <span>Actor</span>
                  <span
                    class:descending={sortDirection('actor') === 'descending'}
                    class="sort-indicator"
                    aria-hidden="true"
                  >
                    <Icon name="sort" size={14} />
                  </span>
                </button>
              </th>
              <th scope="col" aria-sort={sortDirection('target')}>
                <div class="table-heading-layout">
                  <button
                    class="sort-button table-sort-button"
                    type="button"
                    onclick={() => toggleSort('target')}
                  >
                    <span>Target</span>
                    <span
                      class:descending={sortDirection('target') === 'descending'}
                      class="sort-indicator"
                      aria-hidden="true"
                    >
                      <Icon name="sort" size={14} />
                    </span>
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
                      showIcon
                      iconOnly
                      placement="header"
                      onChange={(values) =>
                        auditTable.getColumn('target')?.setFilterValue(values[0])}
                    />
                  {/if}
                </div>
              </th>
              <th scope="col" aria-sort={sortDirection('change')}>
                <div class="table-heading-layout">
                  <button
                    class="sort-button table-sort-button"
                    type="button"
                    onclick={() => toggleSort('change')}
                  >
                    <span>Change</span>
                    <span
                      class:descending={sortDirection('change') === 'descending'}
                      class="sort-indicator"
                      aria-hidden="true"
                    >
                      <Icon name="sort" size={14} />
                    </span>
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
                      showIcon
                      iconOnly
                      placement="header"
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
                      showIcon
                      iconOnly
                      placement="header"
                      onChange={(values) =>
                        auditTable.getColumn('change')?.setFilterValue(values[0])}
                    />
                  {/if}
                </div>
              </th>
              <th scope="col" aria-sort={sortDirection('when')}>
                <button
                  class="sort-button table-sort-button"
                  type="button"
                  onclick={() => toggleSort('when')}
                >
                  <span>When</span>
                  <span
                    class:descending={sortDirection('when') === 'descending'}
                    class="sort-indicator"
                    aria-hidden="true"
                  >
                    <Icon name="sort" size={14} />
                  </span>
                </button>
              </th>
            </tr>
          </thead>
          <tbody bind:this={auditScroll} data-panel-scroll>
            {#if desktopTableLayout.current}
              <tr
                class="virtual-spacer"
                aria-hidden="true"
                style:height={`${$auditVirtualizer.getTotalSize()}px`}><td colspan="4"></td></tr
              >
            {/if}
            {#each auditRenderRows as virtualRow (virtualRow.key)}
              {@const entry = auditEntryAt(virtualRow.index)}
              <tr
                class:virtual-row={virtualRow.virtual}
                style:height={virtualRow.virtual ? `${virtualRow.size}px` : undefined}
                style:transform={virtualRow.virtual
                  ? `translateY(${virtualRow.start}px)`
                  : undefined}
              >
                <td data-label="Actor">
                  <span class="actor">
                    <Avatar account={entry.actor} size={24} />
                    <strong>{entry.actor.display_name}</strong>
                    <span class="actor-login mono">@{entry.actor.login}</span>
                  </span>
                </td>
                <td data-label="Target">
                  {#if context === 'root' && entry.installation !== undefined}
                    <span class="cell-primary" title={`@${entry.installation.login}`}>
                      {entry.installation.display_name}
                    </span>
                  {:else if context === 'root'}
                    <Chip small>Smyklot</Chip>
                  {:else if entry.repository_full_name !== undefined}
                    <code title={entry.repository_full_name}>
                      {repositoryName(entry.repository_full_name)}
                    </code>
                  {:else}
                    <Chip small>Account</Chip>
                  {/if}
                </td>
                <td data-label="Change" title={auditDetail(entry)}>
                  <span class="change-line">
                    {#if entry.category !== undefined}
                      <span class="category-tag" aria-hidden="true">{entry.category}</span>
                    {/if}
                    <span class="cell-primary">{auditSummary(entry.summary)}</span>
                  </span>
                </td>
                <td data-label="When">
                  <time
                    class="table-time"
                    datetime={entry.created_at}
                    title={formatTimestamp(entry.created_at)}
                  >
                    {displayTime(entry.created_at)}
                  </time>
                </td>
              </tr>
            {:else}
              <tr class="empty-row">
                <td class="empty-cell" colspan="4">
                  <TableEmptyState
                    title="No configuration changes found"
                    description={hasFilters
                      ? 'Try another search or clear the active filters'
                      : 'Configuration changes will appear here'}
                    actionLabel={hasFilters ? 'Clear filters' : undefined}
                    onAction={hasFilters ? clearFilters : undefined}
                  />
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <!-- Keyboard focus lets users scroll columns that overflow the viewport. -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div
        class="table-scroll"
        role="region"
        tabindex="0"
        aria-label="Delivery failure history table"
      >
        <table class="history-table failure-table">
          <caption class="visually-hidden">Delivery failure history</caption>
          <colgroup>
            <col class="status-column" />
            <col class="repository-column" />
            <col class="failure-column" />
            <col class="time-column" />
          </colgroup>
          <thead>
            <tr>
              <th scope="col" aria-sort={sortDirection('status')}>
                <div class="table-heading-layout">
                  <button
                    class="sort-button table-sort-button"
                    type="button"
                    onclick={() => toggleSort('status')}
                  >
                    <span>Status</span>
                    <span
                      class:descending={sortDirection('status') === 'descending'}
                      class="sort-indicator"
                      aria-hidden="true"
                    >
                      <Icon name="sort" size={14} />
                    </span>
                  </button>
                  <FilterMenu
                    label="Status"
                    summary={failureKindLabel()}
                    hint="Choose which delivery failures to show"
                    sections={FAILURE_KIND_FILTERS}
                    selected={[failureKind]}
                    fallbackValue="all"
                    align="end"
                    showIcon
                    iconOnly
                    placement="header"
                    onChange={(values) =>
                      failureTable.getColumn('status')?.setFilterValue(values[0])}
                  />
                </div>
              </th>
              <th scope="col" aria-sort={sortDirection('repository')}>
                <button
                  class="sort-button table-sort-button"
                  type="button"
                  onclick={() => toggleSort('repository')}
                >
                  <span>Repository</span>
                  <span
                    class:descending={sortDirection('repository') === 'descending'}
                    class="sort-indicator"
                    aria-hidden="true"
                  >
                    <Icon name="sort" size={14} />
                  </span>
                </button>
              </th>
              <th scope="col">
                <div class="table-heading-layout"><span>Failure</span></div>
              </th>
              <th scope="col" aria-sort={sortDirection('when')}>
                <button
                  class="sort-button table-sort-button"
                  type="button"
                  onclick={() => toggleSort('when')}
                >
                  <span>When</span>
                  <span
                    class:descending={sortDirection('when') === 'descending'}
                    class="sort-indicator"
                    aria-hidden="true"
                  >
                    <Icon name="sort" size={14} />
                  </span>
                </button>
              </th>
            </tr>
          </thead>
          <tbody bind:this={failureScroll} data-panel-scroll>
            {#if desktopTableLayout.current}
              <tr
                class="virtual-spacer"
                aria-hidden="true"
                style:height={`${$failureVirtualizer.getTotalSize()}px`}><td colspan="4"></td></tr
              >
            {/if}
            {#each failureRenderRows as virtualRow (virtualRow.key)}
              {@const failure = failureAt(virtualRow.index)}
              <tr
                class={['failure-row', virtualRow.virtual && 'virtual-row']}
                style:height={virtualRow.virtual ? `${virtualRow.size}px` : undefined}
                style:transform={virtualRow.virtual
                  ? `translateY(${virtualRow.start}px)`
                  : undefined}
              >
                <td data-label="Status">
                  <span class={['failure-kind', failure.retryable ? 'retryable' : 'permanent']}>
                    <span class="cell-symbol" aria-hidden="true">
                      <Icon name={failure.retryable ? 'refresh' : 'failure'} size={18} />
                    </span>
                    {failure.retryable ? 'Retryable' : 'Permanent'}
                  </span>
                </td>
                <td data-label="Repository">
                  <code
                    title={failure.installation === undefined
                      ? failure.repository_full_name
                      : `${failure.repository_full_name} \u00b7 @${failure.installation.login}`}
                  >
                    {repositoryName(failure.repository_full_name)}
                  </code>
                </td>
                <td data-label="Failure" title={failureDetail(failure)}>
                  <span class="cell-primary">{sentenceCase(failure.reason)}</span>
                </td>
                <td data-label="When">
                  <time
                    class="table-time"
                    datetime={failure.occurred_at}
                    title={formatTimestamp(failure.occurred_at)}
                  >
                    {displayTime(failure.occurred_at)}
                  </time>
                </td>
              </tr>
            {:else}
              <tr class="empty-row">
                <td class="empty-cell" colspan="4">
                  <TableEmptyState
                    title="No delivery failures found"
                    description={hasFilters
                      ? 'Try another search or clear the active filters'
                      : 'Delivery failures will appear here'}
                    actionLabel={hasFilters ? 'Clear filters' : undefined}
                    onAction={hasFilters ? clearFilters : undefined}
                  />
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
    <InfiniteLoadSentinel
      active={!desktopTableLayout.current && !loading && currentPage?.next_cursor != null}
      cursor={currentPage?.next_cursor}
      onVisible={() => void loadNextPage()}
    />
    {#if loadMoreProblem !== null}
      <div class="load-more-alert" role="alert">
        <span>{loadMoreProblem}</span>
        <button class="btn" type="button" onclick={() => void loadNextPage()}>Try again</button>
      </div>
    {/if}
  </div>
</section>

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

  .root-history-navigation {
    align-items: center;
    display: flex;
    gap: var(--space-4);
    justify-content: space-between;
    padding-bottom: var(--space-3);
  }

  .root-history-navigation p {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    margin: 0;
    text-align: right;
  }

  .history-tools {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: 0;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: minmax(12rem, 1fr) auto;
    padding: 0 0 var(--space-3);
  }

  .history-results {
    background: var(--table-filler-bg);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 5rem;
    overflow: hidden;
    position: relative;
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

  .table-scroll {
    background: var(--surface-base);
    max-width: 100%;
    overflow-x: auto;
  }

  .history-table {
    background: var(--surface-base);
    border-collapse: collapse;
    min-width: 40rem;
    table-layout: fixed;
    width: 100%;
  }

  .history-table th,
  .history-table td {
    border-bottom: 1px solid var(--rule);
    padding: 0.625rem 0.75rem;
    text-align: left;
    vertical-align: middle;
  }

  .history-table th:first-child,
  .history-table td:first-child {
    padding-left: var(--space-4);
  }

  .history-table th:last-child,
  .history-table td:last-child {
    padding-right: var(--space-4);
  }

  .history-table thead th:first-child .sort-button {
    padding-left: var(--space-4);
  }

  .failure-table th:last-child,
  .failure-table td:last-child,
  .audit-table th:last-child,
  .audit-table td:last-child {
    text-align: right;
  }

  .history-table th {
    background: var(--table-header-bg);
    color: var(--dim);
    font: 650 var(--font-size-compact) / 1.2 var(--sans);
    height: 2.5rem;
    letter-spacing: 0.02em;
  }

  .history-table tbody tr {
    transition: background-color var(--duration-fast) var(--ease-standard);
  }

  .history-table tbody tr:hover {
    background: var(--table-row-hover);
  }

  @media (min-width: 64.001rem) {
    .history-results {
      min-height: 0;
      overflow: hidden;
    }

    .table-scroll {
      display: flex;
      flex: 1;
      min-height: 0;
      overflow-x: auto;
    }

    .history-table {
      display: flex;
      flex: 1;
      flex-direction: column;
      min-height: 0;
    }

    .history-table colgroup {
      display: none;
    }

    .history-table thead {
      display: block;
      flex: none;
    }

    .history-table tbody {
      background: var(--table-filler-bg);
      display: block;
      flex: 1;
      min-height: 0;
      overflow-y: auto;
      overscroll-behavior: contain;
      position: relative;
    }

    .history-table thead tr,
    .history-table tbody tr {
      display: grid;
      grid-template-columns: var(--history-columns);
      width: 100%;
    }

    .history-table tbody tr:not(.virtual-spacer) {
      background: var(--surface-base);
      /* Pin the grid track to the row's fixed height: auto-sizing would take
         the tallest cell's border-box, push the bottom border one pixel past
         the virtual row, and let the next row paint over every separator. */
      grid-template-rows: 100%;
    }

    .history-table tbody tr:not(.virtual-spacer) td {
      align-content: center;
      display: grid;
    }

    .history-table tbody tr:not(.virtual-spacer) td:last-child {
      justify-items: end;
    }

    .history-table tbody .empty-row {
      background: var(--surface-base);
      border: 0;
      display: flex;
      inset: 0;
      position: absolute;
    }

    .history-table tbody .empty-row .empty-cell {
      align-items: center;
      display: flex;
      grid-column: 1 / -1;
      height: 100%;
      justify-content: center;
      padding: var(--space-6);
      width: 100%;
    }

    .history-table tbody .virtual-row {
      left: 0;
      position: absolute;
      top: 0;
    }

    .history-table tbody .virtual-spacer {
      background: transparent;
      border: 0;
      display: block;
      pointer-events: none;
      width: 1px;
    }

    .virtual-spacer td {
      display: none;
    }

    .audit-table {
      --history-columns: minmax(13rem, 1.1fr) minmax(9rem, 0.8fr) minmax(0, 2.2fr) 7.5rem;
    }

    .failure-table {
      --history-columns: 8rem minmax(9rem, 0.9fr) minmax(0, 2.4fr) 7.5rem;
    }

    .absolute-time .audit-table {
      --history-columns: minmax(13rem, 1.1fr) minmax(9rem, 0.8fr) minmax(0, 2.2fr) 9.5rem;
    }

    .absolute-time .failure-table {
      --history-columns: 8rem minmax(9rem, 0.9fr) minmax(0, 2.4fr) 9.5rem;
    }
  }

  .history-table th:has(.sort-button) {
    padding: 0;
  }

  .table-heading-layout {
    align-items: center;
    display: flex;
    height: 100%;
    justify-content: space-between;
    min-width: 0;
  }

  .table-heading-layout :global(.header-filter) {
    margin-inline: var(--space-1);
  }

  .sort-button {
    align-items: center;
    background: transparent;
    border: 0;
    color: inherit;
    display: flex;
    font: inherit;
    gap: var(--space-2);
    height: 100%;
    justify-content: flex-start;
    letter-spacing: inherit;
    padding: 0.625rem 0.75rem;
    text-align: left;
    text-transform: inherit;
    min-width: 0;
    overflow: hidden;
    width: 100%;
  }

  .table-heading-layout .sort-button {
    flex: 1;
    width: auto;
  }

  .history-table th:last-child .sort-button {
    justify-content: flex-end;
  }

  .sort-indicator {
    color: var(--text-muted);
    display: grid;
    opacity: 0;
    place-items: center;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .sort-button:hover .sort-indicator,
  .sort-button:focus-visible .sort-indicator {
    opacity: 0.55;
  }

  th[aria-sort='ascending'] .sort-indicator,
  th[aria-sort='descending'] .sort-indicator {
    color: var(--brand-action-text);
    opacity: 1;
  }

  .sort-indicator.descending {
    transform: rotate(180deg);
  }

  .history-table code {
    display: inline-block;
    justify-self: start;
    max-width: 100%;
    overflow: hidden;
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

  .actor {
    align-items: baseline;
    display: flex;
    gap: 0.5rem;
    min-width: 0;
  }

  .actor :global(.avatar),
  .actor :global(.avatar-fallback) {
    align-self: center;
  }

  .actor strong {
    flex: none;
    font-size: var(--font-size-meta);
    font-weight: 650;
    line-height: 1.2;
    max-width: 60%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .actor-login {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    line-height: 1.35;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .failure-kind {
    align-items: center;
    display: inline-flex;
    font-size: var(--font-size-meta);
    font-weight: 500;
    gap: var(--space-2);
    line-height: 1;
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

  .cell-primary {
    display: block;
    overflow-wrap: anywhere;
  }

  .cell-primary {
    font-size: var(--font-size-meta);
    line-height: 1.25;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .table-time {
    align-items: center;
    color: var(--dim);
    display: inline-flex;
    font-size: var(--font-size-compact);
    line-height: 1;
    vertical-align: middle;
    white-space: nowrap;
  }

  .empty-cell {
    height: 9rem;
    text-align: center !important;
  }

  .table-skeleton {
    display: grid;
  }

  .table-skeleton span {
    animation: history-skeleton-pulse 1.35s ease-in-out infinite alternate;
    border-bottom: 1px solid var(--rule);
    display: block;
    height: 3rem;
    position: relative;
  }

  .table-skeleton span::before,
  .table-skeleton span::after {
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    content: '';
    height: 0.75rem;
    left: var(--space-4);
    position: absolute;
    top: 1rem;
    width: min(12rem, 26%);
  }

  .table-skeleton span::after {
    left: 46%;
    width: min(16rem, 32%);
  }

  @keyframes history-skeleton-pulse {
    from {
      opacity: 0.48;
    }

    to {
      opacity: 0.88;
    }
  }

  .result-state {
    align-items: center;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    justify-content: center;
    min-height: 9rem;
    padding: 1.5rem;
    text-align: center;
  }

  .result-state span {
    color: var(--dim);
    font-size: 0.75rem;
  }

  @media (max-width: 40rem) {
    .root-history-navigation {
      align-items: start;
      flex-direction: column;
      gap: var(--space-2);
    }

    .root-history-navigation p {
      text-align: left;
    }
  }

  @media (max-width: 48rem) {
    .table-scroll {
      overflow: visible;
      padding: var(--space-3);
    }

    .history-table {
      display: block;
      min-width: 0;
    }

    .history-table colgroup {
      clip-path: inset(50%);
      height: 1px;
      overflow: hidden;
      position: absolute;
      white-space: nowrap;
      width: 1px;
    }

    .history-table thead {
      display: block;
    }

    .history-table thead tr {
      border: 0;
      display: flex;
      justify-content: flex-end;
      padding: 0 0 var(--space-3);
    }

    .history-table thead th {
      padding: 0;
    }

    .history-table thead th:not(:has(.sort-button)) {
      clip-path: inset(50%);
      height: 1px;
      overflow: hidden;
      position: absolute;
      white-space: nowrap;
      width: 1px;
    }

    .history-table thead .sort-button {
      background: var(--control-bg);
      border: 1px solid var(--control-border);
      border-radius: var(--radius-control);
      color: var(--dim);
      height: var(--control-height-compact);
      padding: 0 var(--space-3);
    }

    .history-table thead .sort-button:hover,
    .history-table thead .sort-button:focus-visible {
      background: var(--control-bg-hover);
      color: var(--text);
    }

    .history-table tbody {
      display: grid;
      gap: var(--space-2);
    }

    .history-table tbody tr {
      background: var(--surface-raised);
      border: 1px solid var(--border-subtle);
      border-radius: var(--radius-control);
      display: grid;
      gap: var(--space-3);
      grid-template-columns: repeat(2, minmax(0, 1fr));
      padding: var(--space-3);
    }

    .history-table td {
      border: 0;
      display: grid;
      gap: var(--space-1);
      padding: 0;
      text-align: left !important;
    }

    .history-table td::before {
      color: var(--text-muted);
      content: attr(data-label);
      font: 650 var(--font-size-compact) / 1 var(--sans);
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }

    .history-table .empty-cell {
      display: block;
      grid-column: 1 / -1;
      height: auto;
      padding: var(--space-5);
    }

    .history-table .empty-cell::before {
      display: none;
    }
  }

  @media (max-width: 36rem) {
    .history-tools {
      grid-template-columns: 1fr 1fr;
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

    .history-table tbody tr {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
