<script lang="ts">
  import {
    columnFilteringFeature,
    createColumnHelper,
    createTable,
    filterFn_equals,
    filterFn_inNumberRange,
    filterFn_includesString,
    rowSortingFeature,
    tableFeatures,
  } from '@tanstack/svelte-table';
  import type { ColumnFiltersState, SortingState, Updater } from '@tanstack/svelte-table';
  import { createVirtualizer } from '@tanstack/svelte-virtual';
  import { tick, untrack } from 'svelte';
  import { MediaQuery, SvelteSet } from 'svelte/reactivity';
  import { get } from 'svelte/store';

  import { BOOLEAN_FIELDS } from '../lib/config';
  import type { FilterSection } from '../lib/filter-menu';
  import { formatRelative, formatTimestamp } from '../lib/format';
  import {
    shouldClearFailureAfterAutomaticRefresh,
    shouldReloadRepositoryAfterSaveFailure,
    shouldReplaceFailureWithReadError,
  } from '../lib/repository';
  import type { RepositoryFailureSource } from '../lib/repository';
  import type {
    ConfigPatch,
    ConfigKey,
    Page,
    RepositoryDetail,
    RepositoryFileStatus,
    RepositoryPageRequest,
    RepositorySettingsInput,
    RepositorySettingFilter,
    RepositorySort,
    RepositoryStateFilter,
    RepositorySummary,
  } from '../lib/types';
  import Chip from './Chip.svelte';
  import ConfigEditor from './ConfigEditor.svelte';
  import FileStatusIndicator from './FileStatusIndicator.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import HelpTip from './HelpTip.svelte';
  import Icon from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import Modal from './Modal.svelte';
  import PanelHeader from './PanelHeader.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  type RepositoryEnablement = 'inherit' | 'enabled' | 'disabled';
  type RepositoryFailure = { message: string; source: RepositoryFailureSource };
  type RepositoryDetailSection = 'file' | 'behavior' | 'commands';

  const REPOSITORY_ENABLEMENT_OPTIONS = [
    { value: 'inherit', label: 'Default' },
    { value: 'enabled', label: 'Enabled', tone: 'on' },
    { value: 'disabled', label: 'Disabled', tone: 'off' },
  ] as const;
  const FILE_STATUSES = ['valid', 'missing', 'invalid', 'bypassed'] as const;
  const CONFIG_FILTER_KEYS: readonly ConfigKey[] = [
    ...BOOLEAN_FIELDS.map((field) => field.key),
    'command_prefix',
    'allowed_commands',
    'command_aliases',
  ];
  const STATE_FILTER_SECTIONS = [
    {
      options: [
        { value: 'all', label: 'All states', description: 'Show every repository' },
        {
          value: 'enabled',
          label: 'Enabled',
          description: 'Smyklot handles the repository',
          tone: 'on',
        },
        {
          value: 'disabled',
          label: 'Disabled',
          description: 'Smyklot ignores the repository',
          tone: 'off',
        },
      ],
    },
  ] as const satisfies readonly FilterSection[];
  const FILE_FILTER_SECTIONS = [
    {
      options: [
        { value: 'valid', label: 'Valid', tone: 'valid' },
        { value: 'missing', label: 'Missing', tone: 'missing' },
        { value: 'invalid', label: 'Invalid', tone: 'invalid' },
        { value: 'bypassed', label: 'Bypassed', tone: 'bypassed' },
      ],
    },
  ] as const satisfies readonly FilterSection[];
  const SETTING_FILTER_SECTIONS = [
    {
      options: [
        { value: 'all', label: 'All settings', exclusive: true },
        {
          value: 'custom',
          label: 'Any custom setting',
          description: 'At least one repository override',
          exclusive: true,
        },
        {
          value: 'none',
          label: 'No custom settings',
          description: 'Uses account and repository-file values',
          exclusive: true,
        },
      ],
    },
    {
      label: 'Behavior',
      options: BOOLEAN_FIELDS.map((field) => ({ value: field.key, label: field.label })),
    },
    {
      label: 'Commands',
      options: [
        { value: 'command_prefix', label: 'Prefix' },
        { value: 'allowed_commands', label: 'Allowed' },
        { value: 'command_aliases', label: 'Aliases' },
      ],
    },
  ] as const satisfies readonly FilterSection[];
  const REPOSITORY_TABLE_FEATURES = tableFeatures({
    columnFilteringFeature,
    filterFns: {
      equals: filterFn_equals,
      inNumberRange: filterFn_inNumberRange,
      includesString: filterFn_includesString,
    },
    rowSortingFeature,
  });
  const repositoryColumn = createColumnHelper<
    typeof REPOSITORY_TABLE_FEATURES,
    RepositorySummary
  >();
  const REPOSITORY_COLUMNS = repositoryColumn.columns([
    repositoryColumn.accessor('name', { id: 'repository', enableColumnFilter: false }),
    repositoryColumn.accessor('private', {
      id: 'visibility',
      enableColumnFilter: false,
      enableSorting: false,
    }),
    repositoryColumn.accessor('default_branch', {
      id: 'branch',
      enableColumnFilter: false,
      enableSorting: false,
    }),
    repositoryColumn.accessor('config_file_status', { id: 'file' }),
    repositoryColumn.accessor('config_override_count', { id: 'overrides' }),
    repositoryColumn.accessor('updated_at', { id: 'updated', enableColumnFilter: false }),
    repositoryColumn.accessor('effective_enabled', {
      id: 'enablement',
      enableSorting: false,
    }),
  ]);
  const {
    targetId,
    refreshVersion,
    fetchPage,
    onLoad,
    onUpdate,
    onChanged,
    readOnly = false,
  }: {
    targetId: string;
    refreshVersion: number;
    fetchPage: (request: RepositoryPageRequest) => Promise<Page<RepositorySummary>>;
    onLoad: (repositoryId: string) => Promise<RepositoryDetail>;
    onUpdate: (repositoryId: string, input: RepositorySettingsInput) => Promise<RepositoryDetail>;
    onChanged: (detail: RepositoryDetail) => void;
    readOnly?: boolean;
  } = $props();

  let search = $state('');
  let appliedQuery = $state('');
  let sort = $state<RepositorySort>('name_asc');
  let stateFilter = $state<RepositoryStateFilter>('all');
  let fileFilters = $state<RepositoryFileStatus[]>([]);
  let settingFilter = $state<RepositorySettingFilter>({ mode: 'all' });
  const limit = 20;
  let page = $state<Page<RepositorySummary> | null>(null);
  let loading = $state(false);
  let problem = $state<string | null>(null);
  let loadMoreProblem = $state<string | null>(null);
  let details = $state<Record<string, RepositoryDetail>>({});
  let failures = $state<Record<string, RepositoryFailure>>({});
  let pendingEnablement = $state<Record<string, RepositoryEnablement>>({});
  let detailSections = $state<Record<string, RepositoryDetailSection>>({});
  let activeRepository = $state<RepositorySummary | null>(null);
  let repositoryReturnFocus = $state<HTMLElement | null>(null);
  const visibleDetails = new SvelteSet<string>();
  const working = new SvelteSet<string>();
  const pendingRefreshes = new SvelteSet<string>();
  let now = $state(Date.now());
  let requestSequence = 0;
  let observedRefreshVersion: number | undefined;
  let repositoryResults = $state<HTMLDivElement>();
  let repositoryScroll = $state<HTMLTableSectionElement>();

  const repositories = $derived(page?.items ?? []);
  const settingSelection = $derived(
    settingFilter.mode === 'keys' ? settingFilter.keys : [settingFilter.mode],
  );
  const stateSummary = $derived(optionLabel(STATE_FILTER_SECTIONS, stateFilter));
  const fileSummary = $derived(
    fileFilters.length === 0
      ? 'All files'
      : fileFilters.length === 1
        ? optionLabel(FILE_FILTER_SECTIONS, fileFilters[0] ?? '')
        : `${fileFilters.length} file states`,
  );
  const settingSummary = $derived(
    settingFilter.mode === 'keys'
      ? settingFilter.keys.length === 1
        ? optionLabel(SETTING_FILTER_SECTIONS, settingFilter.keys[0] ?? '')
        : `${settingFilter.keys.length} overrides`
      : optionLabel(SETTING_FILTER_SECTIONS, settingFilter.mode),
  );
  const hasFilters = $derived(
    appliedQuery !== '' ||
      stateFilter !== 'all' ||
      fileFilters.length > 0 ||
      settingFilter.mode !== 'all',
  );
  const filterKey = $derived(
    [
      targetId,
      appliedQuery,
      sort,
      stateFilter,
      fileFilters.join(','),
      settingFilter.mode,
      settingFilter.mode === 'keys' ? settingFilter.keys.join(',') : '',
      limit,
    ].join(':'),
  );
  const repositoryTable = createTable({
    features: REPOSITORY_TABLE_FEATURES,
    columns: REPOSITORY_COLUMNS,
    get data() {
      return repositories;
    },
    getRowId: (repository) => repository.id,
    manualFiltering: true,
    manualSorting: true,
    state: {
      get sorting() {
        return repositorySortingState(sort);
      },
      get columnFilters() {
        return repositoryColumnFilters();
      },
    },
    onSortingChange: selectRepositorySorting,
    onColumnFiltersChange: selectRepositoryColumnFilters,
  });
  const repositoryRows = $derived(repositoryTable.getRowModel().rows);
  const desktopTableLayout = new MediaQuery('min-width: 64.001rem', true);
  const repositoryVirtualizer = createVirtualizer<HTMLTableSectionElement, HTMLTableRowElement>({
    count: 0,
    estimateSize: () => 65,
    getScrollElement: () => repositoryScroll ?? null,
    overscan: 6,
  });
  const repositoryRenderRows = $derived.by(() =>
    desktopTableLayout.current
      ? $repositoryVirtualizer.getVirtualItems().map((row) => ({ ...row, virtual: true as const }))
      : repositoryRows.map((row, index) => ({
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

  $effect(() => {
    void resetAndLoad(filterKey);
  });

  $effect(() => {
    const rows = repositoryRows;
    get(repositoryVirtualizer).setOptions({
      count: desktopTableLayout.current ? rows.length : 0,
      getScrollElement: () => repositoryScroll ?? null,
      getItemKey: (index) => rows[index]?.id ?? index,
    });
  });

  $effect(() => {
    if (!desktopTableLayout.current) return;
    const rows = repositoryRows;
    const last = $repositoryVirtualizer.getVirtualItems().at(-1);
    if (last !== undefined && last.index >= rows.length - 5) void loadNextPage();
  });

  $effect(() => {
    const tick = setInterval(() => {
      now = Date.now();
    }, 30_000);
    return () => clearInterval(tick);
  });

  $effect(() => {
    const version = refreshVersion;
    if (observedRefreshVersion === undefined) {
      observedRefreshVersion = version;
      return;
    }
    if (version === observedRefreshVersion) return;
    observedRefreshVersion = version;
    untrack(() => {
      void resetAndLoad(filterKey);
      refreshVisibleRepository(version);
    });
  });

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
      const loaded = await fetchPage({
        cursor,
        query: appliedQuery,
        sort,
        limit,
        state: stateFilter,
        files: fileFilters,
        setting: settingFilter,
      });
      if (sequence !== requestSequence || key !== filterKey) return;
      page =
        append && page !== null ? { ...loaded, items: [...page.items, ...loaded.items] } : loaded;
    } catch (error) {
      if (sequence === requestSequence && key === filterKey) {
        const message = error instanceof Error ? error.message : String(error);
        if (append) loadMoreProblem = message;
        else problem = message;
      }
    } finally {
      if (sequence === requestSequence && key === filterKey) {
        loading = false;
      }
    }
  }

  async function loadNextPage(): Promise<void> {
    if (loading || page?.next_cursor === null || page?.next_cursor === undefined) return;
    await loadPage(page.next_cursor, filterKey, true);
  }

  function scrollResultsToTop(): void {
    if (isDesktopTableLayout()) {
      repositoryResults?.querySelector<HTMLElement>('[data-panel-scroll]')?.scrollTo({ top: 0 });
    }
  }

  function isDesktopTableLayout(): boolean {
    return window.matchMedia('(min-width: 64.001rem)').matches;
  }

  function retry(): void {
    if (page === null) void loadPage(undefined, filterKey, false);
    else void loadNextPage();
  }

  function repositoryAt(index: number): RepositorySummary {
    const row = repositoryRows[index];
    if (row === undefined) throw new Error(`missing virtual repository row ${index}`);
    return row.original;
  }

  function repositorySortingState(value: RepositorySort): SortingState {
    const mapping: Record<RepositorySort, { id: string; desc: boolean }> = {
      name_asc: { id: 'repository', desc: false },
      name_desc: { id: 'repository', desc: true },
      file_asc: { id: 'file', desc: false },
      file_desc: { id: 'file', desc: true },
      overrides_asc: { id: 'overrides', desc: false },
      overrides_desc: { id: 'overrides', desc: true },
      newest: { id: 'updated', desc: true },
      oldest: { id: 'updated', desc: false },
    };
    return [mapping[value]];
  }

  function repositoryColumnFilters(): ColumnFiltersState {
    return [
      { id: 'enablement', value: stateFilter },
      { id: 'file', value: fileFilters },
      { id: 'overrides', value: settingSelection },
    ];
  }

  function selectRepositorySorting(next: Updater<SortingState>): void {
    const resolved = typeof next === 'function' ? next(repositorySortingState(sort)) : next;
    const selected = resolved[0];
    if (selected === undefined) return;
    const mapping: Record<string, readonly [RepositorySort, RepositorySort]> = {
      repository: ['name_asc', 'name_desc'],
      file: ['file_asc', 'file_desc'],
      overrides: ['overrides_asc', 'overrides_desc'],
      updated: ['oldest', 'newest'],
    };
    const options = mapping[selected.id];
    if (options !== undefined) sort = options[selected.desc ? 1 : 0];
  }

  function selectRepositoryColumnFilters(next: Updater<ColumnFiltersState>): void {
    const current = repositoryColumnFilters();
    const resolved = typeof next === 'function' ? next(current) : next;
    const value = (id: string): string[] => {
      const selected = resolved.find((filter) => filter.id === id)?.value;
      return Array.isArray(selected)
        ? selected.map(String)
        : selected === undefined
          ? []
          : [String(selected)];
    };
    selectStateFilter(value('enablement'));
    selectFileFilters(value('file'));
    selectSettingFilter(value('overrides'));
  }

  function clearFilters(): void {
    search = '';
    appliedQuery = '';
    stateFilter = 'all';
    fileFilters = [];
    settingFilter = { mode: 'all' };
  }

  function refreshVisibleRepository(version: number): void {
    if (!Number.isSafeInteger(version)) return;
    untrack(() => {
      for (const repositoryId of visibleDetails) requestRefresh(repositoryId);
    });
  }

  async function openRepository(
    repository: RepositorySummary,
    trigger: HTMLElement,
  ): Promise<void> {
    activeRepository = repository;
    repositoryReturnFocus = trigger;
    visibleDetails.clear();
    visibleDetails.add(repository.id);
    if (details[repository.id] === undefined) {
      await refresh(repository.id);
      if (activeRepository?.id !== repository.id) return;
      await tick();
      const section = detailSection(repository);
      document
        .querySelector<HTMLButtonElement>(`#repository-${repository.id}-${section}-tab`)
        ?.focus();
    }
  }

  function closeRepository(): void {
    if (activeRepository !== null) visibleDetails.delete(activeRepository.id);
    activeRepository = null;
  }

  function detailSection(repository: RepositorySummary): RepositoryDetailSection {
    return (
      detailSections[repository.id] ??
      (repository.config_file_status === 'invalid' ? 'file' : 'behavior')
    );
  }

  function selectDetailSection(repositoryId: string, section: RepositoryDetailSection): void {
    detailSections = { ...detailSections, [repositoryId]: section };
  }

  function moveDetailSection(
    event: KeyboardEvent,
    repositoryId: string,
    section: RepositoryDetailSection,
  ): void {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    const sections: readonly RepositoryDetailSection[] = ['file', 'behavior', 'commands'];
    const current = sections.indexOf(section);
    let next = current;
    if (event.key === 'Home') next = 0;
    if (event.key === 'End') next = sections.length - 1;
    if (event.key === 'ArrowRight') {
      next = (current + 1) % sections.length;
    }
    if (event.key === 'ArrowLeft') {
      next = (current - 1 + sections.length) % sections.length;
    }
    const selected = sections[next];
    if (selected === undefined) return;
    selectDetailSection(repositoryId, selected);
    queueMicrotask(() =>
      document
        .querySelector<HTMLButtonElement>(`#repository-${repositoryId}-${selected}-tab`)
        ?.focus(),
    );
  }

  function configSectionCount(detail: RepositoryDetail, section: 'behavior' | 'commands'): number {
    const keys: readonly ConfigKey[] =
      section === 'behavior'
        ? BOOLEAN_FIELDS.map((field) => field.key)
        : ['command_prefix', 'allowed_commands', 'command_aliases'];
    return keys.filter((key) => Object.hasOwn(detail.config_patch, key)).length;
  }

  function toggleNameSort(): void {
    toggleColumnSort('repository');
  }

  function toggleUpdatedSort(): void {
    toggleColumnSort('updated');
  }

  function toggleFileSort(): void {
    toggleColumnSort('file');
  }

  function toggleOverridesSort(): void {
    toggleColumnSort('overrides');
  }

  function toggleColumnSort(columnId: string): void {
    const column = repositoryTable.getColumn(columnId);
    column?.toggleSorting(column.getIsSorted() === 'asc');
  }

  function sortDirection(
    column: 'name' | 'file' | 'overrides' | 'updated',
  ): 'ascending' | 'descending' | undefined {
    const ids = { name: 'repository', file: 'file', overrides: 'overrides', updated: 'updated' };
    const direction = repositoryTable.getColumn(ids[column])?.getIsSorted();
    return direction === 'asc' ? 'ascending' : direction === 'desc' ? 'descending' : undefined;
  }

  async function refresh(
    repositoryId: string,
    clearExistingFailure = true,
  ): Promise<RepositoryDetail | null> {
    if (working.has(repositoryId)) {
      pendingRefreshes.add(repositoryId);
      return null;
    }
    working.add(repositoryId);
    if (clearExistingFailure) clearFailure(repositoryId);
    try {
      const detail = await loadDetail(repositoryId);
      if (
        !clearExistingFailure &&
        shouldClearFailureAfterAutomaticRefresh(failures[repositoryId]?.source)
      ) {
        clearFailure(repositoryId);
      }
      return detail;
    } catch (error) {
      if (
        clearExistingFailure ||
        shouldReplaceFailureWithReadError(failures[repositoryId]?.source)
      ) {
        setFailure(repositoryId, error, 'read');
      }
      return null;
    } finally {
      finishWorking(repositoryId);
    }
  }

  async function loadDetail(repositoryId: string): Promise<RepositoryDetail> {
    const detail = await onLoad(repositoryId);
    details = { ...details, [repositoryId]: detail };
    return detail;
  }

  function requestRefresh(repositoryId: string): void {
    if (working.has(repositoryId)) {
      pendingRefreshes.add(repositoryId);
      return;
    }
    void refresh(repositoryId, false);
  }

  function finishWorking(repositoryId: string): void {
    working.delete(repositoryId);
    if (pendingRefreshes.delete(repositoryId)) void refresh(repositoryId, false);
  }

  async function save(
    repositoryId: string,
    change: (detail: RepositoryDetail) => RepositorySettingsInput,
  ): Promise<RepositoryDetail | null> {
    if (working.has(repositoryId)) return null;
    working.add(repositoryId);
    clearFailure(repositoryId);
    try {
      let detail = details[repositoryId];
      if (detail === undefined) {
        try {
          detail = await loadDetail(repositoryId);
        } catch (error) {
          setFailure(repositoryId, error, 'read');
          return null;
        }
      }
      const updated = await onUpdate(repositoryId, change(detail));
      details = { ...details, [repositoryId]: updated };
      onChanged(updated);
      return updated;
    } catch (error) {
      if (shouldReloadRepositoryAfterSaveFailure(error)) {
        try {
          await loadDetail(repositoryId);
        } catch {
          // Keep the original write failure visible. A later catalog event or
          // expansion can retry the detail read.
        }
      }
      setFailure(repositoryId, error, 'write');
      return null;
    } finally {
      finishWorking(repositoryId);
    }
  }

  async function setEnabled(repository: RepositorySummary, value: string): Promise<void> {
    if (value !== 'inherit' && value !== 'enabled' && value !== 'disabled') return;
    if (value === enabledValue(repository)) return;

    pendingEnablement = { ...pendingEnablement, [repository.id]: value };
    try {
      const enabledOverride = value === 'inherit' ? null : value === 'enabled';
      await save(repository.id, (detail) => ({
        enabled_override: enabledOverride,
        config_patch: detail.config_patch,
        ignore_repository_file: detail.ignore_repository_file,
        expected_revision: detail.revision,
      }));
    } finally {
      const next = { ...pendingEnablement };
      delete next[repository.id];
      pendingEnablement = next;
    }
  }

  async function setBypass(repositoryId: string, ignored: boolean): Promise<void> {
    await save(repositoryId, (detail) => ({
      enabled_override: detail.repository.enabled_override,
      config_patch: detail.config_patch,
      ignore_repository_file: ignored,
      expected_revision: detail.revision,
    }));
  }

  async function setConfig(repositoryId: string, configPatch: ConfigPatch): Promise<void> {
    await save(repositoryId, (detail) => ({
      enabled_override: detail.repository.enabled_override,
      config_patch: configPatch,
      ignore_repository_file: detail.ignore_repository_file,
      expected_revision: detail.revision,
    }));
  }

  function setFailure(repositoryId: string, error: unknown, source: RepositoryFailureSource): void {
    failures = {
      ...failures,
      [repositoryId]: {
        message: error instanceof Error ? error.message : String(error),
        source,
      },
    };
  }

  function clearFailure(repositoryId: string): void {
    if (failures[repositoryId] === undefined) return;
    const next = { ...failures };
    delete next[repositoryId];
    failures = next;
  }

  function enabledValue(repository: RepositorySummary): RepositoryEnablement {
    if (repository.enabled_override === null) return 'inherit';
    return repository.enabled_override ? 'enabled' : 'disabled';
  }

  function optionLabel(sections: readonly FilterSection[], value: string): string {
    return (
      sections.flatMap((section) => section.options).find((option) => option.value === value)
        ?.label ?? value
    );
  }

  function selectStateFilter(values: string[]): void {
    const value = values[0];
    if (value === 'all' || value === 'enabled' || value === 'disabled') stateFilter = value;
  }

  function selectFileFilters(values: string[]): void {
    fileFilters = values.filter(isRepositoryFileStatus);
  }

  function selectSettingFilter(values: string[]): void {
    if (values.length === 1) {
      const value = values[0];
      if (value === 'all' || value === 'custom' || value === 'none') {
        settingFilter = { mode: value };
        return;
      }
    }

    const keys = values.filter(isConfigKey);
    settingFilter = keys.length === 0 ? { mode: 'all' } : { mode: 'keys', keys };
  }

  function isRepositoryFileStatus(value: string): value is RepositoryFileStatus {
    return FILE_STATUSES.some((status) => status === value);
  }

  function isConfigKey(value: string): value is ConfigKey {
    return CONFIG_FILTER_KEYS.some((key) => key === value);
  }
</script>

{#snippet headerActions()}
  <HelpTip
    id="repository-controls-help"
    label="About repository controls"
    text="On and Off filter the effective state. Default follows Enable repositories by default in Settings. Open a repository to configure repository-specific settings"
  />
{/snippet}

<section class="plate repository-panel" aria-labelledby="repositories-heading">
  <PanelHeader
    id="repositories-heading"
    title="Repositories"
    description="Choose which repositories Smyklot handles and where settings differ"
    actions={headerActions}
  />

  <div class="repository-tools">
    <SearchField
      label="Search repositories"
      placeholder="Search repositories"
      value={search}
      onInput={(value) => (search = value)}
    />
  </div>

  <div
    class={['repository-results', loading && 'loading']}
    bind:this={repositoryResults}
    aria-busy={loading}
  >
    {#if problem !== null}
      <div class="result-state" role="alert">
        <strong>Repositories could not be loaded</strong>
        <span>{problem}</span>
        <button class="btn" onclick={retry}>Try again</button>
      </div>
    {:else if loading && page === null}
      <div class="table-skeleton" aria-hidden="true">
        {#each [0, 1, 2, 3, 4, 5] as index (index)}
          <span></span>
        {/each}
      </div>
      <p class="visually-hidden" role="status">Loading repositories</p>
    {:else if repositories.length === 0}
      <div class="result-state table-empty">
        <TableEmptyState
          title={hasFilters ? 'No repositories match' : 'No repositories installed'}
          description={hasFilters
            ? 'Try another search or clear the active filters'
            : 'Repositories will appear after the installation catalog is refreshed'}
          actionLabel={hasFilters ? 'Clear filters' : undefined}
          onAction={hasFilters ? clearFilters : undefined}
        />
      </div>
    {:else}
      <div class="repository-table-scroll">
        <table class="repositories">
          <thead>
            <tr>
              <th class="sortable-heading" aria-sort={sortDirection('name')}>
                <button class="sort-heading table-sort-button" onclick={toggleNameSort}>
                  Repository
                  <span class="sort-indicator" aria-hidden="true"
                    ><Icon name="sort" size={14} /></span
                  >
                </button>
              </th>
              <th>Visibility</th>
              <th>Default branch</th>
              <th class="sortable-heading" aria-sort={sortDirection('file')}>
                <div class="table-heading-layout">
                  <button class="sort-heading table-sort-button" onclick={toggleFileSort}>
                    File state
                    <span class="sort-indicator" aria-hidden="true"
                      ><Icon name="sort" size={14} /></span
                    >
                  </button>
                  <FilterMenu
                    label="File state"
                    summary={fileSummary}
                    hint="Select one or more file states"
                    sections={FILE_FILTER_SECTIONS}
                    selected={fileFilters}
                    multiple
                    align="end"
                    showIcon
                    iconOnly
                    placement="header"
                    onChange={(values) => repositoryTable.getColumn('file')?.setFilterValue(values)}
                  />
                </div>
              </th>
              <th class="numeric-heading sortable-heading" aria-sort={sortDirection('overrides')}>
                <div class="table-heading-layout">
                  <button class="sort-heading table-sort-button" onclick={toggleOverridesSort}>
                    Overrides
                    <span class="sort-indicator" aria-hidden="true"
                      ><Icon name="sort" size={14} /></span
                    >
                  </button>
                  <FilterMenu
                    label="Overrides"
                    summary={settingSummary}
                    hint="Match any selected repository override"
                    sections={SETTING_FILTER_SECTIONS}
                    selected={settingSelection}
                    multiple
                    fallbackValue="all"
                    align="end"
                    wide
                    showIcon
                    iconOnly
                    placement="header"
                    onChange={(values) =>
                      repositoryTable.getColumn('overrides')?.setFilterValue(values)}
                  />
                </div>
              </th>
              <th class="sortable-heading" aria-sort={sortDirection('updated')}>
                <button class="sort-heading table-sort-button" onclick={toggleUpdatedSort}>
                  Updated
                  <span class="sort-indicator" aria-hidden="true"
                    ><Icon name="sort" size={14} /></span
                  >
                </button>
              </th>
              <th class="filterable-heading">
                <div class="table-heading-layout">
                  <span>Enablement</span>
                  <FilterMenu
                    label="Enablement"
                    summary={stateSummary}
                    hint="Filter by Smyklot's effective state"
                    sections={STATE_FILTER_SECTIONS}
                    selected={[stateFilter]}
                    fallbackValue="all"
                    align="end"
                    showIcon
                    iconOnly
                    placement="header"
                    onChange={(values) =>
                      repositoryTable.getColumn('enablement')?.setFilterValue(values[0])}
                  />
                </div>
              </th>
            </tr>
          </thead>
          <tbody bind:this={repositoryScroll} data-panel-scroll>
            {#if desktopTableLayout.current}
              <tr
                class="virtual-spacer"
                aria-hidden="true"
                style:height={`${$repositoryVirtualizer.getTotalSize()}px`}
                ><td colspan="7"></td></tr
              >
            {/if}
            {#each repositoryRenderRows as virtualRow (virtualRow.key)}
              {@const repository = repositoryAt(virtualRow.index)}
              {@const repositoryFailure = failures[repository.id]}
              <tr
                class={['repository-row', virtualRow.virtual && 'virtual-row']}
                style:height={virtualRow.virtual ? `${virtualRow.size}px` : undefined}
                style:transform={virtualRow.virtual
                  ? `translateY(${virtualRow.start}px)`
                  : undefined}
              >
                <td>
                  <button
                    class="expand"
                    aria-haspopup="dialog"
                    aria-label={`Configure ${repository.full_name}`}
                    onclick={(event) => void openRepository(repository, event.currentTarget)}
                  >
                    <span class="caret-control" aria-hidden="true">
                      <Icon name="settings" size={15} />
                    </span>
                    <span class="repo-copy">
                      <strong>{repository.name}</strong>
                    </span>
                  </button>
                </td>
                <td data-label="Visibility">
                  <span class={['visibility', repository.private ? 'private' : 'public']}>
                    <Icon name={repository.private ? 'lock' : 'globe'} size={15} />
                    {repository.private ? 'Private' : 'Public'}
                  </span>
                </td>
                <td data-label="Default branch"
                  ><code class="branch">{repository.default_branch || 'Not reported'}</code></td
                >
                <td data-label="File state">
                  <FileStatusIndicator
                    id="file-status-{repository.id}"
                    status={repository.config_file_status}
                    showLabel
                  />
                </td>
                <td class="numeric-cell mono" data-label="Overrides">
                  <span class="numeric-value">{repository.config_override_count}</span>
                </td>
                <td data-label="Updated">
                  <time
                    class="updated"
                    datetime={repository.updated_at}
                    title={formatTimestamp(repository.updated_at)}
                  >
                    {formatRelative(repository.updated_at, now)}
                  </time>
                </td>
                <td data-label="Enablement">
                  {#if !repository.available}
                    <Chip small>Unavailable</Chip>
                  {:else}
                    <SegmentedControl
                      name="repository-enablement-{repository.id}"
                      label="Enablement for {repository.full_name}"
                      options={REPOSITORY_ENABLEMENT_OPTIONS}
                      value={pendingEnablement[repository.id] ?? enabledValue(repository)}
                      disabled={readOnly || working.has(repository.id)}
                      align="end"
                      compact
                      onSelect={(value) => void setEnabled(repository, value)}
                    />
                  {/if}
                </td>
              </tr>

              {#if repositoryFailure !== undefined && activeRepository?.id !== repository.id}
                <tr class="visually-hidden">
                  <td colspan="7"><span role="alert">{repositoryFailure.message}</span></td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
      <InfiniteLoadSentinel
        active={!desktopTableLayout.current && !loading && page?.next_cursor != null}
        cursor={page?.next_cursor}
        onVisible={() => void loadNextPage()}
      />
    {/if}
    {#if loadMoreProblem !== null}
      <div class="load-more-alert" role="alert">
        <span>{loadMoreProblem}</span>
        <button class="btn" type="button" onclick={() => void loadNextPage()}>Try again</button>
      </div>
    {/if}
  </div>
</section>

{#if activeRepository !== null}
  {@const repository = activeRepository}
  {@const detail = details[repository.id]}
  {@const repositoryFailure = failures[repository.id]}
  {@const activeSection = detailSection(repository)}
  <Modal
    id="repository-settings"
    open
    title={repository.name}
    description="Repository settings override installation defaults and repository-file values"
    variant="wide"
    returnFocus={repositoryReturnFocus}
    onClose={closeRepository}
  >
    {#if repositoryFailure !== undefined}
      <p class="form-error repository-modal-error" role="alert">{repositoryFailure.message}</p>
    {/if}

    <div class="repository-detail">
      {#if detail === undefined}
        <p class="detail-loading dim">Reading repository settings…</p>
      {:else}
        {@const behaviorCount = configSectionCount(detail, 'behavior')}
        {@const commandCount = configSectionCount(detail, 'commands')}
        <div
          class="repository-detail-navigation"
          aria-label="Settings for {repository.name}"
          role="tablist"
          aria-orientation="horizontal"
        >
          <button
            id="repository-{repository.id}-file-tab"
            class:active={activeSection === 'file'}
            type="button"
            role="tab"
            aria-selected={activeSection === 'file'}
            aria-controls="repository-{repository.id}-detail-panel"
            tabindex={activeSection === 'file' ? 0 : -1}
            data-modal-focus
            onclick={() => selectDetailSection(repository.id, 'file')}
            onkeydown={(event) => moveDetailSection(event, repository.id, 'file')}
          >
            <span>Repository file</span>
            {#if detail.config_file_error !== undefined}
              <span class="detail-count problem-count" aria-label="1 file error">1</span>
            {/if}
          </button>
          <button
            id="repository-{repository.id}-behavior-tab"
            class:active={activeSection === 'behavior'}
            type="button"
            role="tab"
            aria-selected={activeSection === 'behavior'}
            aria-controls="repository-{repository.id}-detail-panel"
            tabindex={activeSection === 'behavior' ? 0 : -1}
            onclick={() => selectDetailSection(repository.id, 'behavior')}
            onkeydown={(event) => moveDetailSection(event, repository.id, 'behavior')}
          >
            <span>Behavior</span>
            <span class={['detail-count', behaviorCount === 0 && 'zero-count']}
              >{behaviorCount}</span
            >
          </button>
          <button
            id="repository-{repository.id}-commands-tab"
            class:active={activeSection === 'commands'}
            type="button"
            role="tab"
            aria-selected={activeSection === 'commands'}
            aria-controls="repository-{repository.id}-detail-panel"
            tabindex={activeSection === 'commands' ? 0 : -1}
            onclick={() => selectDetailSection(repository.id, 'commands')}
            onkeydown={(event) => moveDetailSection(event, repository.id, 'commands')}
          >
            <span>Commands</span>
            <span class={['detail-count', commandCount === 0 && 'zero-count']}>{commandCount}</span>
          </button>
        </div>

        <div
          class="repository-detail-content"
          id="repository-{repository.id}-detail-panel"
          role="tabpanel"
          aria-labelledby="repository-{repository.id}-{activeSection}-tab"
          tabindex="0"
        >
          {#if activeSection === 'file'}
            <section class="file-pane" aria-labelledby="repository-{repository.id}-file-heading">
              <header class="detail-pane-heading">
                <div>
                  <h3 id="repository-{repository.id}-file-heading">Repository file</h3>
                  <p>Observe or bypass the configuration stored in the repository</p>
                </div>
                <FileStatusIndicator
                  id="file-status-detail-{repository.id}"
                  status={detail.repository.config_file_status}
                  showLabel
                />
              </header>
              <div
                class={['file-status', detail.config_file_error !== undefined && 'file-problem']}
              >
                <div class="file-copy">
                  <strong>Configuration path</strong>
                  <code>.github/smyklot.yaml</code>
                  {#if detail.config_file_error !== undefined}
                    <p>{detail.config_file_error}</p>
                  {/if}
                </div>
                <label class="switch switch-labelled">
                  <strong>Bypass file</strong>
                  <input
                    type="checkbox"
                    checked={detail.ignore_repository_file}
                    disabled={readOnly || working.has(repository.id)}
                    onchange={(event) => setBypass(repository.id, event.currentTarget.checked)}
                  />
                  <span aria-hidden="true"></span>
                </label>
              </div>
              {#if detail.ignore_repository_file}
                <p class="warning" role="status">
                  Repository-file settings are ignored and the exception is recorded in Audit
                </p>
              {/if}
            </section>
          {:else}
            <div class="override-heading">
              <div>
                <strong
                  >{activeSection === 'behavior'
                    ? 'Behavior overrides'
                    : 'Command overrides'}</strong
                >
                <p>Only values changed here override inherited configuration</p>
              </div>
              <HelpTip
                id="repository-overrides-{repository.id}-{activeSection}"
                label="About repository overrides"
                text="Only settings changed here override configuration defaults from Settings and repository-file settings"
              />
            </div>
            <ConfigEditor
              patch={detail.config_patch}
              inherited={detail.inherited_config}
              scope="repository"
              idPrefix={repository.id}
              section={activeSection}
              disabled={readOnly || working.has(repository.id)}
              onSave={(patch) => setConfig(repository.id, patch)}
            />
          {/if}
        </div>
      {/if}
    </div>

    {#snippet footer()}
      <button class="btn" type="button" onclick={closeRepository}>Close</button>
    {/snippet}
  </Modal>
{/if}

<style>
  .repository-panel {
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

  .repository-tools {
    align-items: center;
    background: transparent;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: minmax(16rem, 1fr);
    padding: 0 0 var(--space-3);
  }

  .repository-results {
    background: var(--table-filler-bg);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    overflow: hidden;
    position: relative;
  }

  .repository-results.loading {
    cursor: progress;
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

  .result-state.table-empty {
    border: 0;
    flex: 1;
    margin: 0;
    min-height: 12rem;
  }

  .result-state span {
    color: var(--dim);
    font-size: var(--font-size-meta);
  }

  .table-skeleton {
    display: grid;
  }

  .table-skeleton span {
    animation: repository-skeleton-pulse 1.35s ease-in-out infinite alternate;
    border-bottom: 1px solid var(--rule);
    display: block;
    height: 3.375rem;
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
    width: min(14rem, 32%);
  }

  .table-skeleton span::after {
    left: 48%;
    width: min(8rem, 18%);
  }

  @keyframes repository-skeleton-pulse {
    from {
      opacity: 0.48;
    }

    to {
      opacity: 0.88;
    }
  }

  .repository-table-scroll {
    background: var(--surface-base);
    overflow-x: auto;
  }

  .repositories {
    background: var(--surface-base);
    border-collapse: collapse;
    min-width: 58rem;
    table-layout: fixed;
    width: 100%;
  }

  th,
  td {
    border-bottom: 1px solid var(--border-subtle);
    padding: var(--space-3);
    text-align: left;
    vertical-align: middle;
  }

  th {
    background: var(--table-header-bg);
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    font-weight: 650;
    letter-spacing: 0.02em;
  }

  th:first-child {
    width: 23%;
  }

  th:nth-child(2) {
    width: 10%;
  }

  th:nth-child(3) {
    width: 11%;
  }

  th:nth-child(4) {
    width: 13%;
  }

  th:nth-child(5) {
    width: 13%;
  }

  th:nth-child(6) {
    width: 12%;
  }

  th:nth-child(7) {
    width: 18%;
  }

  th:first-child,
  td:first-child {
    padding-left: var(--space-4);
  }

  th:last-child,
  td:last-child {
    padding-right: var(--space-4);
    text-align: right;
  }

  .numeric-heading,
  .numeric-cell {
    text-align: center;
  }

  .numeric-heading .sort-heading {
    justify-content: center;
  }

  .sortable-heading {
    padding: 0;
  }

  .filterable-heading {
    padding-block: 0;
  }

  .table-heading-layout {
    align-items: center;
    display: flex;
    height: 2.75rem;
    justify-content: space-between;
    min-width: 0;
  }

  .table-heading-layout :global(.header-filter) {
    margin-inline: var(--space-1);
  }

  .filterable-heading .table-heading-layout {
    gap: var(--space-1);
  }

  .sortable-heading:first-child {
    padding-left: 0;
  }

  .sort-heading {
    align-items: center;
    background: transparent;
    border: 0;
    color: inherit;
    display: flex;
    font: inherit;
    gap: var(--space-2);
    height: 2.75rem;
    letter-spacing: inherit;
    margin: 0;
    padding: 0 var(--space-3);
    text-transform: inherit;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    width: auto;
  }

  .sortable-heading:first-child .sort-heading {
    padding-left: var(--space-4);
  }

  .sort-indicator {
    color: var(--text-muted);
    display: grid;
    place-items: center;
  }

  th[aria-sort='ascending'] .sort-indicator {
    color: var(--brand-action-text);
  }

  th[aria-sort='descending'] .sort-indicator {
    color: var(--brand-action-text);
    transform: rotate(180deg);
  }

  .repository-row {
    transition: background-color var(--duration-fast) var(--ease-standard);
  }

  @media (min-width: 64.001rem) {
    .repository-results {
      overflow: hidden;
    }

    .repository-table-scroll {
      display: flex;
      flex: 1;
      min-height: 0;
      overflow-x: auto;
    }

    .repositories {
      display: flex;
      flex: 1;
      flex-direction: column;
      min-height: 0;
    }

    .repositories thead {
      display: block;
      flex: none;
    }

    .repositories tbody {
      background: var(--table-filler-bg);
      display: block;
      flex: 1;
      min-height: 0;
      overflow-y: auto;
      overscroll-behavior: contain;
      position: relative;
    }

    .repositories thead tr,
    .repositories tbody tr {
      display: grid;
      grid-template-columns: 23% 10% 11% 13% 13% 12% 18%;
      width: 100%;
    }

    .repositories th {
      width: auto;
    }

    .repositories tbody tr:not(.virtual-spacer) {
      background: var(--surface-base);
    }

    .repositories tbody tr:not(.virtual-spacer) td {
      align-items: center;
      display: flex;
    }

    .repositories tbody .numeric-cell {
      justify-content: center;
    }

    .repositories tbody td:last-child {
      justify-content: flex-end;
    }

    .repositories tbody .virtual-row {
      left: 0;
      position: absolute;
      top: 0;
    }

    .repositories tbody .virtual-spacer {
      background: transparent;
      border: 0;
      display: block;
      pointer-events: none;
      width: 1px;
    }

    .virtual-spacer td {
      display: none;
    }
  }

  .repository-row:hover {
    background: var(--table-row-hover);
  }

  .expand {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: var(--radius-control);
    color: var(--text-primary);
    display: grid;
    gap: var(--space-1);
    grid-template-columns: 1.75rem minmax(0, 1fr);
    height: var(--control-height-compact);
    margin: 0;
    min-width: 0;
    padding: 0;
    text-align: left;
    width: 100%;
  }

  .expand:hover {
    background: var(--interactive-hover);
  }

  .expand:focus-visible {
    background: transparent;
    outline: 0;
  }

  .expand:focus-visible .caret-control {
    background: var(--brand-action-tint);
    box-shadow: inset 0 0 0 2px var(--focus);
  }

  .caret-control {
    align-items: center;
    border-radius: var(--radius-control);
    display: inline-flex;
    height: var(--control-height-compact);
    justify-content: center;
    width: 1.75rem;
  }

  .repo-copy {
    display: flex;
    flex-direction: column;
    height: var(--control-height-compact);
    justify-content: center;
    min-width: 0;
  }

  .repo-copy strong {
    display: block;
    line-height: 1.25;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .visibility {
    align-items: center;
    color: var(--info);
    display: flex;
    font-size: var(--font-size-meta);
    font-weight: 550;
    gap: var(--space-1);
    height: var(--control-height-compact);
    line-height: 1;
    width: max-content;
  }

  .visibility.public {
    color: var(--text-muted);
  }

  .branch {
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    color: var(--text-primary);
    font-size: var(--font-size-compact);
    justify-self: start;
    width: max-content;
  }

  .numeric-value {
    align-items: center;
    display: inline-flex;
    height: var(--control-height-compact);
    line-height: 1;
  }

  .updated {
    align-items: center;
    color: var(--text-muted);
    display: flex;
    font-size: var(--font-size-meta);
    height: var(--control-height-compact);
    line-height: 1;
    white-space: nowrap;
  }

  td:last-child :global(fieldset) {
    margin-left: 0;
  }

  .repository-detail {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    display: flex;
    flex-direction: column;
    min-height: min(32rem, 62vh);
    overflow: hidden;
  }

  .repository-modal-error {
    margin: 0 0 var(--space-3);
  }

  .detail-loading {
    margin: 0;
    padding: var(--space-4);
  }

  .repository-detail-navigation {
    background: var(--surface-inset);
    border-bottom: 1px solid var(--border-subtle);
    display: flex;
    flex-direction: row;
    gap: var(--space-1);
    overflow-x: auto;
    padding: 0 var(--space-3);
  }

  .repository-detail-navigation button {
    align-items: center;
    background: transparent;
    border: 0;
    border-bottom: 2px solid transparent;
    border-radius: 0;
    color: var(--text-muted);
    display: flex;
    font-size: var(--font-size-meta);
    font-weight: 550;
    gap: var(--space-2);
    flex: 0 0 auto;
    height: 2.75rem;
    justify-content: center;
    padding: 0 var(--space-3);
    text-align: left;
    white-space: nowrap;
  }

  .repository-detail-navigation button:hover,
  .repository-detail-navigation button:focus-visible {
    background: var(--interactive-hover);
    color: var(--text-primary);
  }

  .repository-detail-navigation button.active {
    background: transparent;
    border-bottom-color: var(--brand-action);
    color: var(--brand-action-text);
    font-weight: 650;
  }

  .detail-count {
    color: var(--brand-action-text);
    display: inline-grid;
    font: 650 var(--font-size-micro) / 1 var(--mono);
    font-variant-numeric: tabular-nums;
    min-width: 1ch;
    place-items: center;
  }

  .zero-count {
    color: var(--text-muted);
  }

  .problem-count {
    color: var(--danger);
  }

  .repository-detail-content {
    min-width: 0;
    outline: 0;
    padding: var(--space-4) var(--space-6);
  }

  .file-status {
    align-items: center;
    display: flex;
    gap: 1rem;
    justify-content: space-between;
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    margin-top: var(--space-4);
    padding: var(--space-4);
  }

  .file-copy {
    display: grid;
    gap: 0.15rem;
  }

  .file-copy code {
    color: var(--dim);
    font-size: 0.6875rem;
  }

  .file-status p {
    color: var(--dim);
    font-size: 0.8125rem;
    margin: 0.15rem 0 0;
  }

  .override-heading {
    align-items: center;
    display: flex;
    justify-content: space-between;
    margin-bottom: var(--space-3);
  }

  .override-heading strong,
  .detail-pane-heading h3 {
    font-size: var(--font-size-title);
  }

  .override-heading p,
  .detail-pane-heading p {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    margin: var(--space-1) 0 0;
  }

  .file-problem strong,
  .form-error {
    color: var(--stop);
  }

  .detail-pane-heading {
    align-items: center;
    display: flex;
    justify-content: space-between;
  }

  .detail-pane-heading h3 {
    margin: 0;
  }

  .warning {
    background: var(--warning-tint);
    border: 1px solid color-mix(in srgb, var(--warning) 28%, transparent);
    border-radius: var(--r-well);
    color: var(--warning);
    font-size: 0.8125rem;
    padding: 0.75rem;
  }

  .switch-labelled {
    align-items: center;
    display: flex;
    flex: none;
    gap: 0.5rem;
  }

  @media (max-width: 74rem) {
    .repository-tools {
      grid-template-columns: 1fr 1fr;
    }

    .repository-tools :global(.search-field) {
      grid-column: 1 / -1;
    }

    .repository-detail-content {
      padding-inline: var(--space-4);
    }
  }

  @media (max-width: 48rem) {
    .repository-tools {
      grid-template-columns: 1fr;
    }

    .repository-tools :global(.search-field) {
      grid-column: 1;
    }

    .repositories {
      min-width: 0;
    }

    .repositories thead {
      display: none;
    }

    .repositories,
    .repositories tbody,
    .repository-row,
    .repository-row td {
      display: block;
      width: 100%;
    }

    .repository-row {
      border-bottom: 1px solid var(--border-subtle);
      padding: var(--space-3);
    }

    .repository-row td {
      align-items: center;
      border: 0;
      display: flex;
      justify-content: space-between;
      padding: var(--space-2) 0;
    }

    .repository-row td:first-child {
      padding-top: 0;
    }

    .repository-row td[data-label]::before {
      color: var(--text-muted);
      content: attr(data-label);
      font-size: var(--font-size-compact);
      font-weight: 650;
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }

    .numeric-cell {
      text-align: right;
    }

    .file-status {
      align-items: flex-start;
      flex-direction: column;
    }
  }
</style>
