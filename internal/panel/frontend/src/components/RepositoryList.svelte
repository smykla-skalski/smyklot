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
    decodeRepositorySettingFilter,
    encodeRepositorySettingFilter,
    EPHEMERAL_PREFS,
    prefList,
    prefOption,
    prefText,
    type PrefsAccessor,
  } from '../lib/preferences-sync';
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
  import Chip, { type ChipTone } from './Chip.svelte';
  import ConfigEditor from './ConfigEditor.svelte';
  import FileStatusIndicator from './FileStatusIndicator.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import HelpTip from './HelpTip.svelte';
  import Icon from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import InheritControl from './InheritControl.svelte';
  import Modal from './Modal.svelte';
  import PanelHeader from './PanelHeader.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import TableEmptyState from './TableEmptyState.svelte';
  import { inkAlign } from '../lib/ink-align';

  type RepositoryEnablement = 'inherit' | 'enabled' | 'disabled';
  type RepositoryFailure = { message: string; source: RepositoryFailureSource };
  type RepositoryDetailSection = 'file' | 'behavior' | 'commands';

  const REPOSITORY_VALUE_OPTIONS = [
    { value: 'enabled', label: 'Enabled' },
    { value: 'disabled', label: 'Disabled' },
  ] as const;
  const FILE_MODE_OPTIONS = [
    { value: 'observe', label: 'Observe' },
    { value: 'bypass', label: 'Bypass' },
  ] as const;
  const FILE_STATUSES = ['valid', 'missing', 'invalid', 'bypassed'] as const;
  const FILE_STATUS_TONES = {
    valid: 'clear',
    missing: 'neutral',
    invalid: 'stop',
    bypassed: 'warning',
  } as const satisfies Record<RepositoryFileStatus, ChipTone>;
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
    defaultEnabled,
    fetchPage,
    onLoad,
    onUpdate,
    onChanged,
    readOnly = false,
    prefs = EPHEMERAL_PREFS,
  }: {
    targetId: string;
    refreshVersion: number;
    defaultEnabled: boolean;
    fetchPage: (request: RepositoryPageRequest) => Promise<Page<RepositorySummary>>;
    onLoad: (repositoryId: string) => Promise<RepositoryDetail>;
    onUpdate: (repositoryId: string, input: RepositorySettingsInput) => Promise<RepositoryDetail>;
    onChanged: (detail: RepositoryDetail) => void;
    readOnly?: boolean;
    prefs?: PrefsAccessor;
  } = $props();

  // Table state deliberately captures the preferences at mount; remote
  // changes apply on the next remount instead of mid-interaction.
  // svelte-ignore state_referenced_locally
  const initialPrefs = prefs;

  const REPOSITORY_SORTS: readonly RepositorySort[] = [
    'name_asc',
    'name_desc',
    'file_asc',
    'file_desc',
    'overrides_asc',
    'overrides_desc',
    'newest',
    'oldest',
  ];

  let search = $state(prefText(initialPrefs.get('table.repositories.search')));
  let appliedQuery = $state(prefText(initialPrefs.get('table.repositories.search')));
  let sort = $state<RepositorySort>(
    prefOption(initialPrefs.get('table.repositories.sort'), REPOSITORY_SORTS, 'name_asc'),
  );
  let stateFilter = $state<RepositoryStateFilter>(
    prefOption(initialPrefs.get('table.repositories.state'), ['all', 'enabled', 'disabled'], 'all'),
  );
  let fileFilters = $state<RepositoryFileStatus[]>(
    prefList(initialPrefs.get('table.repositories.files'), FILE_STATUSES),
  );
  let settingFilter = $state<RepositorySettingFilter>(
    decodeRepositorySettingFilter(initialPrefs.get('table.repositories.settings')),
  );

  // One persistence effect instead of a write at every mutation site: any
  // change to the tracked state syncs, and the initial run is a no-op because
  // the state was just read from the same preferences.
  $effect(() => {
    prefs.set('table.repositories.sort', sort);
    prefs.set('table.repositories.state', stateFilter);
    prefs.set('table.repositories.files', [...fileFilters]);
    prefs.set('table.repositories.settings', encodeRepositorySettingFilter(settingFilter));
    prefs.set('table.repositories.search', appliedQuery);
  });
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
    estimateSize: () => 56,
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
    // The file tab leads: it orients the dialog around the repository's own
    // configuration before overrides.
    return detailSections[repository.id] ?? 'file';
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

  /* The repository-file pane lists the behavior settings this repository
     actually overrides, the way the approved mock draws it: the file card, the
     bypass control, then whatever this repo has changed, with its own save bar.
     Someone reading the file tab is asking "what does this repository do
     differently", and the answer belongs on the same screen as the file. */
  function overriddenBehaviorKeys(detail: RepositoryDetail): ConfigKey[] {
    return BOOLEAN_FIELDS.map((field) => field.key).filter((key) =>
      Object.hasOwn(detail.config_patch, key),
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

<section class="plate repository-panel" aria-labelledby="repositories-heading">
  <PanelHeader
    id="repositories-heading"
    title="Repositories"
    description="Enablement and settings for every repository in this workspace"
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
    <!-- A refresh that failed over a loaded table has not made the table wrong. -->
    {#if problem !== null && page !== null}
      <ResultProblem
        title="Repositories could not be loaded"
        {problem}
        busy={loading}
        onRetry={() => retry()}
        overContent
      />
    {/if}

    {#if problem !== null && page === null}
      <ResultProblem
        title="Repositories could not be loaded"
        {problem}
        busy={loading}
        onRetry={() => retry()}
      />
    {:else if loading && page === null}
      <div class="table-skeleton" aria-hidden="true">
        {#each [0, 1, 2, 3, 4, 5] as index (index)}
          <span></span>
        {/each}
      </div>
      <p class="visually-hidden" role="status">Loading repositories</p>
    {:else}
      <div class="repository-table-scroll">
        <table class="repositories">
          <thead>
            <tr>
              <th class="sortable-heading" aria-sort={sortDirection('name')}>
                <div class="table-heading-layout">
                  <button class="sort-heading table-sort-button" onclick={toggleNameSort}>
                    <span class="cap-trim">Repository</span>
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
                    align="start"
                    wide
                    showIcon
                    iconOnly
                    placement="header"
                    onChange={(values) =>
                      repositoryTable.getColumn('overrides')?.setFilterValue(values)}
                  />
                </div>
              </th>
              <th class="sortable-heading" aria-sort={sortDirection('file')}>
                <div class="table-heading-layout">
                  <button class="sort-heading table-sort-button" onclick={toggleFileSort}>
                    <span class="cap-trim">File state</span>
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
              <th class="sortable-heading" aria-sort={sortDirection('updated')}>
                <button class="sort-heading table-sort-button" onclick={toggleUpdatedSort}>
                  <span class="cap-trim">Updated</span>
                  <span class="sort-indicator" aria-hidden="true"
                    ><Icon name="sort" size={14} /></span
                  >
                </button>
              </th>
              <th class="filterable-heading">
                <div class="table-heading-layout">
                  <span use:inkAlign class="heading-with-help">
                    <span class="cap-trim">Enablement</span>
                    <HelpTip
                      id="repository-enablement-help"
                      label="About enablement"
                      text="Enabled and Disabled filter the effective state. A linked chain means the value is inherited from Unconfigured repositories in Settings. Open a repository to configure repository-specific settings"
                    />
                  </span>
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
            {#if repositories.length === 0}
              <tr class="empty-row">
                <td colspan="4">
                  <TableEmptyState
                    title={hasFilters ? 'No repositories match' : 'No repositories installed'}
                    description={hasFilters
                      ? 'Try another search or clear the active filters'
                      : 'Repositories will appear after the installation catalog is refreshed'}
                    actionLabel={hasFilters ? 'Clear filters' : undefined}
                    onAction={hasFilters ? clearFilters : undefined}
                  />
                </td>
              </tr>
            {/if}
            {#if desktopTableLayout.current}
              <tr
                class="virtual-spacer"
                aria-hidden="true"
                style:height={`${$repositoryVirtualizer.getTotalSize()}px`}
                ><td colspan="4"></td></tr
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
                    <span class="repo-copy">
                      <strong>{repository.name}</strong>
                      {#if repository.config_override_count > 0}
                        <span class="override-chip">
                          {repository.config_override_count}
                          {repository.config_override_count === 1 ? 'override' : 'overrides'}
                        </span>
                      {/if}
                    </span>
                  </button>
                </td>
                <td data-label="File state">
                  <FileStatusIndicator
                    id="file-status-{repository.id}"
                    status={repository.config_file_status}
                    showLabel
                  />
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
                    {@const enablement =
                      pendingEnablement[repository.id] ?? enabledValue(repository)}
                    <InheritControl
                      label="Enablement for {repository.full_name}"
                      source="Unconfigured repositories in Settings"
                      inheritedValue={defaultEnabled ? 'enabled' : 'disabled'}
                      inheritedLabel={defaultEnabled ? 'Enabled' : 'Disabled'}
                      value={enablement === 'inherit' ? null : enablement}
                      options={REPOSITORY_VALUE_OPTIONS}
                      disabled={readOnly || working.has(repository.id)}
                      onSelect={(value) => void setEnabled(repository, value)}
                      onRestore={() => void setEnabled(repository, 'inherit')}
                    />
                  {/if}
                </td>
              </tr>

              {#if repositoryFailure !== undefined && activeRepository?.id !== repository.id}
                <tr class="visually-hidden">
                  <td colspan="4"><span role="alert">{repositoryFailure.message}</span></td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
      <InfiniteLoadSentinel
        active={!desktopTableLayout.current &&
          !loading &&
          loadMoreProblem === null &&
          page?.next_cursor != null}
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
    description="Repository settings override workspace defaults and repository-file values"
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
            {#if behaviorCount > 0}
              <span class="detail-count">{behaviorCount}</span>
            {/if}
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
            {#if commandCount > 0}
              <span class="detail-count">{commandCount}</span>
            {/if}
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
              <h3 id="repository-{repository.id}-file-heading" class="visually-hidden">
                Repository file
              </h3>
              <div class={['file-card', detail.config_file_error !== undefined && 'file-problem']}>
                <!-- 14px glyph in an 18px slot, the same pairing every other
                     icon slot in the product uses; 16px here was the one
                     outlier, in the approved card as well. -->
                <span class="file-card-icon status-{detail.repository.config_file_status}">
                  <Icon name="file" size={14} />
                </span>
                <div class="f-copy">
                  <strong>Configuration path</strong>
                  <div><code class="mono">.github/smyklot.yaml</code></div>
                  {#if detail.config_file_error !== undefined}
                    <p>{detail.config_file_error}</p>
                  {/if}
                </div>
                <Chip tone={FILE_STATUS_TONES[detail.repository.config_file_status]} dot>
                  {detail.repository.config_file_status.slice(0, 1).toUpperCase() +
                    detail.repository.config_file_status.slice(1)}
                </Chip>
              </div>
              <div class="override-row">
                <span use:inkAlign class="o-label">
                  <!-- Trimmed, so the words centre against the 18px help slot
                       on their caps rather than on a taller line box. -->
                  <span class="cap-trim">Bypass file</span>
                  <HelpTip
                    id="repository-bypass-help-{repository.id}"
                    label="About bypassing the repository file"
                    text="Repository-file settings are ignored and the exception is recorded in Audit"
                  />
                </span>
                <SegmentedControl
                  name="repository-bypass-{repository.id}"
                  label="Repository file handling"
                  options={FILE_MODE_OPTIONS}
                  value={detail.ignore_repository_file ? 'bypass' : 'observe'}
                  disabled={readOnly || working.has(repository.id)}
                  compact
                  onSelect={(value) => setBypass(repository.id, value === 'bypass')}
                />
              </div>
              {#if overriddenBehaviorKeys(detail).length > 0}
                <ConfigEditor
                  patch={detail.config_patch}
                  inherited={detail.inherited_config}
                  scope="repository"
                  idPrefix="{repository.id}-file"
                  section="behavior"
                  only={overriddenBehaviorKeys(detail)}
                  disabled={readOnly || working.has(repository.id)}
                  onSave={(patch) => setConfig(repository.id, patch)}
                />
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

  .empty-row td {
    border-bottom: 0;
    height: 12rem;
  }

  .empty-row td :global(.table-empty-state) {
    margin-inline: auto;
  }

  .table-skeleton {
    display: grid;
  }

  .table-skeleton span {
    animation: repository-skeleton-pulse 1.35s ease-in-out infinite alternate;
    border-bottom: 1px solid var(--rule);
    display: block;
    height: 3.5rem;
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
    /* Separated, not collapsed: a collapsed border is shared between adjacent
       rows, so each cell owns half of it and every row box lands on a .5. */
    border-collapse: separate;
    border-spacing: 0;
    min-width: 52rem;
    table-layout: fixed;
    width: 100%;
  }

  th,
  td {
    border-bottom: 1px solid var(--border-subtle);
    font-size: var(--font-size-meta);
    padding: var(--space-2) var(--space-3);
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
    width: 27%;
  }

  th:nth-child(2) {
    width: 11%;
  }

  th:nth-child(3) {
    width: 12%;
  }

  th:nth-child(4) {
    width: 14%;
  }

  th:nth-child(5) {
    width: 13%;
  }

  th:nth-child(6) {
    width: 23%;
  }

  th:first-child,
  td:first-child {
    padding-left: var(--space-3);
  }

  th:last-child,
  td:last-child {
    padding-right: var(--space-3);
  }

  .sortable-heading {
    padding: 0;
  }

  .filterable-heading,
  .plain-heading {
    padding-block: 0;
  }

  .table-heading-layout {
    align-items: center;
    display: flex;
    height: 2.5rem;
    justify-content: space-between;
    min-width: 0;
  }

  .table-heading-layout :global(.header-filter) {
    margin-inline: var(--space-1);
  }

  .filterable-heading .table-heading-layout {
    gap: var(--space-1);
  }

  /* Label and help mark on one centred row - the same 0.35rem the approved
     header cell uses, not whatever an inline space happens to measure. */
  .heading-with-help {
    align-items: center;
    display: flex;
    gap: 0.35rem;
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
    height: 2.5rem;
    letter-spacing: inherit;
    margin: 0;
    padding: 0 var(--space-3);
    text-transform: inherit;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    width: 100%;
  }

  .sortable-heading:first-child .sort-heading {
    padding-left: var(--space-3);
  }

  .sort-indicator {
    color: var(--text-muted);
    display: grid;
    opacity: 0;
    place-items: center;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .sort-heading:hover .sort-indicator,
  .sort-heading:focus-visible .sort-indicator {
    opacity: 0.55;
  }

  th[aria-sort='ascending'] .sort-indicator,
  th[aria-sort='descending'] .sort-indicator {
    opacity: 1;
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
      overscroll-behavior-y: contain;
      position: relative;
    }

    .repositories thead tr,
    .repositories tbody tr {
      display: grid;
      /* The approved catalog's 2fr 1fr 1.4fr 1.6fr, as percentages of the 6fr
         total so they hold at any table width. */
      grid-template-columns: 33.333% 16.667% 23.333% 26.667%;
      width: 100%;
    }

    .repositories th {
      width: auto;
    }

    /* The grid rows above repaint the row ground at a higher specificity than the plain
         `:hover` rule outside this block, so the pointer state has to be restated here or it never
         reaches the screen. */
    .repositories tbody tr:not(.virtual-spacer):hover {
      background: var(--table-row-hover);
    }

    .repositories tbody tr:not(.virtual-spacer) {
      background: var(--surface-base);
      /* Pin the grid track to the row's fixed height: auto-sizing would take
         the tallest cell's border-box, push the bottom border one pixel past
         the virtual row, and let the next row paint over every separator. */
      grid-template-rows: 100%;
    }

    .repositories tbody tr:not(.virtual-spacer) td {
      align-items: center;
      display: flex;
    }

    .repositories tbody td:last-child {
      /* The enablement control sits at the column start, under the header
         label — same left alignment as every other column. */
      justify-content: flex-start;
    }

    .repositories tbody tr.empty-row {
      align-content: center;
      grid-template-columns: minmax(0, 1fr);
      inset: 0;
      position: absolute;
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
    display: flex;
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

  /* Name and chip are siblings on one centred row, so the chip's box and the
     name's caps share a centre line rather than an inline baseline. */
  .repo-copy {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    min-width: 0;
  }

  /* `clip` rather than `hidden`: the cap trim ends the box at the baseline, so
     a hidden overflow would shave the descenders off "api-gateway". The clip
     margin lets ink outside the box survive while the ellipsis still fires. */
  .repo-copy strong {
    font: 600 var(--font-size-meta) / 1 var(--mono);
    letter-spacing: 0;
    min-width: 0;
    overflow: clip;
    overflow-clip-margin: 0.35em;
    text-box: trim-both cap alphabetic;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Overrides are exceptional, so they ride along with the name instead of
     owning a column: the same "N overrides" language the Settings page uses. */
  .override-chip {
    background: var(--surface-inset);
    border-radius: var(--r-chip);
    color: var(--text-soft);
    flex: none;
    font: 500 var(--font-size-compact) / 1 var(--mono);
    padding: 0.34rem 0.5rem;
    text-box: trim-both cap alphabetic;
    white-space: nowrap;
  }

  .cell-symbol {
    display: grid;
    flex: none;
    place-items: center;
    width: 1.125rem;
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
    display: grid;
    gap: 0.875rem;
  }

  .repository-modal-error {
    margin: 0 0 var(--space-3);
  }

  .detail-loading {
    margin: 0;
    padding: var(--space-4);
  }

  /* The shared pill-nav pattern (same as the Root installation detail tabs) —
     the app's one internal-tab treatment, replacing the old underline bar. */
  .repository-detail-navigation {
    background: color-mix(in srgb, var(--brand-action) 4%, var(--surface-inset));
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    display: flex;
    flex-direction: row;
    gap: var(--space-1);
    overflow-x: auto;
    padding: var(--space-1);
  }

  .repository-detail-navigation button {
    align-items: center;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    color: var(--text-secondary);
    display: flex;
    flex: 0 0 auto;
    font-size: var(--font-size-meta);
    font-weight: 650;
    gap: 0.4rem;
    justify-content: center;
    line-height: 1;
    padding: 0.4375rem var(--space-3);
    text-align: left;
    text-box: trim-both cap alphabetic;
    white-space: nowrap;
  }

  .repository-detail-navigation button:hover,
  .repository-detail-navigation button:focus-visible {
    background: var(--interactive-hover);
    color: var(--text-primary);
  }

  .repository-detail-navigation button.active {
    background: var(--surface-base);
    border-color: color-mix(in srgb, var(--brand-action) 30%, var(--border-subtle));
    box-shadow: 0 1px 2px var(--shadow-color);
    color: var(--brand-action-text);
  }

  /* Trimmed to the digits, so the badge is a 15px pill rather than an 18px
     circle — and the tab row it sits in stands 31px like the mock instead of
     34px. Untrimmed, the badge was the tallest thing in the row and set its
     height on its own. */
  .detail-count {
    background: var(--brand-action-tint);
    border-radius: 999px;
    box-shadow: inset 0 0 0 1px color-mix(in srgb, currentcolor 30%, transparent);
    color: var(--brand-action-text);
    display: inline-block;
    font: 700 0.625rem / 1 var(--mono);
    font-variant-numeric: tabular-nums;
    min-width: 1ch;
    padding: 4px 6px;
    text-align: center;
    text-box: trim-both cap alphabetic;
  }

  .problem-count {
    color: var(--danger);
  }

  /* The dialog is titled by the repository name — code, so it sets in mono.
     The Modal stamps its id on the h2 itself as `<modal id>-title`. */
  :global(#repository-settings-title) {
    font-family: var(--mono);
  }

  .repository-detail-content {
    min-width: 0;
    outline: 0;
  }

  /* The card keeps its 71px stature whatever its copy measures: trimming the
     two lines to their ink took 14px out of the content, and the card's height
     is a shape decision, not a consequence of the leading. */
  .file-card {
    align-items: center;
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    display: flex;
    gap: var(--space-3);
    min-height: 4.4375rem;
    padding: var(--space-3) var(--space-4);
  }

  .file-card-icon {
    color: var(--dim);
    display: grid;
    flex: none;
    height: 1.125rem;
    place-items: center;
    width: 1.125rem;
  }

  .file-card-icon.status-valid {
    color: var(--success);
  }

  .file-card-icon.status-invalid {
    color: var(--danger);
  }

  .file-card-icon.status-bypassed {
    color: var(--warning);
  }

  .f-copy {
    flex: 1;
    min-width: 0;
  }

  /* Both lines are trimmed to cap..baseline and spaced by an explicit step, so
     the copy block's BOX equals its ink and the card's flex centring centres
     what the eye reads. Untrimmed, the first line's leading and the last line's
     descender are not symmetric and the text sat 3.36px below the card's
     middle - measured, and the approved card had it too. 0.8rem keeps the
     baseline-to-baseline distance the two lines already had (21.75px). */
  .f-copy strong {
    display: block;
    font-size: var(--font-size-meta);
    line-height: 1;
    text-box: trim-both cap alphabetic;
  }

  .f-copy code {
    color: var(--dim);
    display: block;
    font-size: var(--font-size-compact);
    line-height: 1;
    margin-top: 0.8rem;
    text-box: trim-both cap alphabetic;
  }

  .f-copy p {
    color: var(--danger);
    font-size: var(--font-size-compact);
    margin: 0.15rem 0 0;
  }

  /* The file pane's override rows wear the same boxed shape as the bypass row
     above them, not the flush list style the Behavior tab uses - on this pane
     they are cards in a stack, not rows in a table. */
  /* 0.875rem, the same step the override row above it uses - the pane's stack
     rhythm, not the editor's own. */
  .file-pane :global(.config-editor) {
    margin-top: 0.875rem;
  }

  .file-pane :global(.config-editor .rows-plain) {
    display: grid;
    gap: var(--space-3);
  }

  .file-pane :global(.config-editor .row) {
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-ctl);
    min-height: 3.25rem;
    padding: var(--space-2) 0.875rem;
  }

  .file-pane .override-row {
    align-items: center;
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-ctl);
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    margin-top: 0.875rem;
    min-height: 3.25rem;
    padding: var(--space-2) 0.875rem;
  }

  .o-label {
    align-items: center;
    display: inline-flex;
    font-size: 0.875rem;
    font-weight: 600;
    gap: 0.45rem;
  }

  .override-heading {
    align-items: center;
    display: flex;
    justify-content: space-between;
    margin-bottom: var(--space-3);
  }

  .override-heading strong {
    font-size: var(--font-size-title);
  }

  .override-heading p {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    margin: var(--space-1) 0 0;
  }

  .file-problem strong,
  .form-error {
    color: var(--stop);
  }

  .warning {
    background: var(--warning-tint);
    border: 1px solid color-mix(in srgb, var(--warning) 28%, transparent);
    border-radius: var(--r-well);
    color: var(--warning);
    font-size: 0.8125rem;
    padding: 0.75rem;
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

    .file-status {
      align-items: flex-start;
      flex-direction: column;
    }
  }
</style>
