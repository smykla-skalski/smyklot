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
  import { useDebounce, useInterval } from 'runed';
  import { createInfiniteQuery, createQuery, useQueryClient } from '@tanstack/svelte-query';

  import { BOOLEAN_FIELDS } from '../config';
  import { dialogRoute } from '../dialog-route.svelte';
  import type { FilterSection } from '../filter-menu';
  import { formatRelative, formatTimestamp } from '../format';
  import {
    decodeRepositorySettingFilter,
    encodeRepositorySettingFilter,
    EPHEMERAL_PREFS,
    prefList,
    prefOption,
    prefText,
    type PrefsAccessor,
  } from '../preferences-sync';
  import { shouldReloadRepositoryAfterSaveFailure } from '../repository';
  import type { RepositoryFailureSource } from '../repository';
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
  } from '../types';
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
  import TableToolsMenu from './TableToolsMenu.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  type RepositoryEnablement = 'inherit' | 'enabled' | 'disabled';
  type RepositoryFailure = { message: string; source: RepositoryFailureSource };
  type RepositoryDetailSection = 'file' | 'behavior' | 'commands';

  /** Names the dialog in the address, and is the `id` the dialog carries. */
  const REPOSITORY_DIALOG = 'repository-settings';

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
    defaultEnabled,
    fetchPage,
    onLoad,
    onUpdate,
    onResetConfigMigration,
    onChanged,
    readOnly = false,
    prefs = EPHEMERAL_PREFS,
  }: {
    targetId: string;
    defaultEnabled: boolean;
    fetchPage: (request: RepositoryPageRequest) => Promise<Page<RepositorySummary>>;
    onLoad: (repositoryId: string) => Promise<RepositoryDetail>;
    onUpdate: (repositoryId: string, input: RepositorySettingsInput) => Promise<RepositoryDetail>;
    onResetConfigMigration: (targetId: string, repositoryId: string) => Promise<RepositoryDetail>;
    onChanged: (targetId: string, detail: RepositoryDetail) => void;
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
  let details = $state<Record<string, RepositoryDetail>>({});
  let failures = $state<Record<string, RepositoryFailure>>({});
  let pendingEnablement = $state<Record<string, RepositoryEnablement>>({});
  let repositoryReturnFocus = $state<HTMLElement | null>(null);
  const working = new SvelteSet<string>();
  const queryClient = useQueryClient();
  let now = $state(Date.now());
  useInterval(30_000, { callback: () => (now = Date.now()) });
  let repositoryResults = $state<HTMLDivElement>();
  let repositoryScroll = $state<HTMLTableSectionElement>();
  /** Names learned from a repository read by name, so the id is known next time. */
  let namedRepositories = $state<Record<string, string>>({});
  const repositoryQuery = createInfiniteQuery(() => ({
    queryKey: [
      'repositories',
      targetId,
      appliedQuery,
      sort,
      stateFilter,
      [...fileFilters],
      settingFilter,
      limit,
    ],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      fetchPage({
        cursor: pageParam,
        query: appliedQuery,
        sort,
        limit,
        state: stateFilter,
        files: [...fileFilters],
        setting: settingFilter,
      }),
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  }));
  const page = $derived.by((): Page<RepositorySummary> | null => {
    const pages = repositoryQuery.data?.pages;
    if (pages === undefined || pages.length === 0) return null;
    const last = pages.at(-1);
    if (last === undefined) return null;
    return { ...last, items: pages.flatMap((entry) => entry.items) };
  });
  const repositories = $derived(page?.items ?? []);
  const loading = $derived(repositoryQuery.isFetching);
  const problem = $derived(
    repositoryQuery.isError && !repositoryQuery.isFetchNextPageError
      ? errorMessage(repositoryQuery.error)
      : null,
  );
  const loadMoreProblem = $derived(
    repositoryQuery.isFetchNextPageError ? errorMessage(repositoryQuery.error) : null,
  );

  /* The dialog is whatever the address names, not a click the component
     remembers, so a reload lands back on the repository that was open.

     It is named the way a person would name it - `api-gateway`, not an id - and
     the detail read accepts either, so a repository further down a paginated list
     still opens on a cold load without first finding it in the list. That
     response carries the summary the header needs. Names are unique within an
     installation and every read is scoped to one, so two organizations owning a
     repository of the same name never meet. */
  const activeRepositoryKey = $derived(dialogRoute.param(REPOSITORY_DIALOG, 'repository') ?? null);
  const repositoryDetailQuery = createQuery(() => ({
    queryKey: ['repository', targetId, activeRepositoryKey],
    enabled: activeRepositoryKey !== null,
    queryFn: () => {
      if (activeRepositoryKey === null) throw new Error('select a repository first');
      return onLoad(activeRepositoryKey);
    },
  }));

  /* The address carries a name; everything inside this component is keyed by id.
     The name is resolved from the loaded page when it is there, and otherwise
     from the detail that reading it by name returned. */
  const activeRepositoryId = $derived.by(() => {
    const key = activeRepositoryKey;
    if (key === null) return null;

    return (
      repositories.find((repository) => repository.name === key)?.id ??
      (repositoryDetailQuery.data?.repository.name === key
        ? repositoryDetailQuery.data.repository.id
        : null) ??
      namedRepositories[key] ??
      null
    );
  });
  const activeRepository = $derived(
    activeRepositoryId === null
      ? null
      : (repositories.find((repository) => repository.id === activeRepositoryId) ??
          (repositoryDetailQuery.data?.repository.id === activeRepositoryId
            ? repositoryDetailQuery.data.repository
            : null) ??
          details[activeRepositoryId]?.repository ??
          null),
  );
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

  const debouncedSearch = useDebounce((query: string) => {
    appliedQuery = query;
  }, 250);

  $effect(() => {
    const value = search.trim();
    untrack(() => void debouncedSearch(value));
  });

  $effect(() => {
    void filterKey;
    untrack(() => scrollResultsToTop());
  });

  $effect(() => {
    const rows = repositoryRows;
    const desktop = desktopTableLayout.current;
    untrack(() => {
      get(repositoryVirtualizer).setOptions({
        count: desktop ? rows.length : 0,
        getScrollElement: () => repositoryScroll ?? null,
        getItemKey: (index) => rows[index]?.id ?? index,
      });
    });
  });

  $effect(() => {
    if (!desktopTableLayout.current) return;
    const rows = repositoryRows;
    const last = $repositoryVirtualizer.getVirtualItems().at(-1);
    if (last !== undefined && last.index >= rows.length - 5) {
      untrack(() => void loadNextPage());
    }
  });

  async function loadNextPage(): Promise<void> {
    if (repositoryQuery.isFetchingNextPage || !repositoryQuery.hasNextPage) return;
    await repositoryQuery.fetchNextPage();
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
    if (repositoryQuery.isFetchNextPageError) void repositoryQuery.fetchNextPage();
    else void repositoryQuery.refetch();
  }

  function errorMessage(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
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

  function openRepository(repository: RepositorySummary, trigger: HTMLElement): void {
    repositoryReturnFocus = trigger;
    dialogRoute.open(REPOSITORY_DIALOG, { repository: repository.name });
  }

  function closeRepository(): void {
    dialogRoute.close();
  }

  /* Whatever the address names has to be on screen, however it got there: a click
     on a row, the Back button, or a link opened cold in a new window. Watching the
     address rather than acting inside the click handler is what makes the last of
     those work, since no click happened. */
  $effect(() => {
    const key = activeRepositoryKey;
    const detail = repositoryDetailQuery.data;
    const error = repositoryDetailQuery.error;
    if (key === null) return;

    untrack(() => {
      if (detail === undefined) {
        const repositoryId = activeRepositoryId;
        if (error !== null && repositoryId !== null) setFailure(repositoryId, error, 'read');
        return;
      }

      const repositoryId = detail.repository.id;
      const firstRead = details[repositoryId] === undefined;
      rememberDetail(detail, key);
      if (failures[repositoryId]?.source === 'read') clearFailure(repositoryId);
      if (firstRead) {
        void tick().then(() => {
          if (activeRepositoryId !== repositoryId) return;
          /* The switch only exists once the detail is in, so this is the first
             moment there is anything inside the dialog to land on. */
          document
            .querySelector<HTMLInputElement>(
              `input[name="repository-${repositoryId}-section"]:checked`,
            )
            ?.focus();
        });
      }
    });
  });

  /* The pane the dialog is showing rides the address too, so a link points at the
     commands a colleague was asked to look at rather than at the file pane
     everyone starts on. It replaces rather than pushes: flipping the switch is
     part of reading one repository, not a second place to come back from. */
  function detailSection(repository: RepositorySummary): RepositoryDetailSection {
    if (repository.id === activeRepositoryId) {
      const requested = dialogRoute.param(REPOSITORY_DIALOG, 'section');
      if (requested !== undefined && isDetailSection(requested)) return requested;
    }
    // The file section leads: it orients the dialog around the repository's own
    // configuration before overrides.
    return 'file';
  }

  function isDetailSection(value: string): value is RepositoryDetailSection {
    return value === 'file' || value === 'behavior' || value === 'commands';
  }

  function selectDetailSection(
    repository: RepositorySummary,
    section: RepositoryDetailSection,
  ): void {
    dialogRoute.update(REPOSITORY_DIALOG, { repository: repository.name, section });
  }

  /* Names the pane for the reader, now that the switch above it is a control with
     its own label rather than a strip of tabs the pane could point back at. */
  function sectionLabel(section: RepositoryDetailSection): string {
    if (section === 'file') return 'File';
    return section === 'behavior' ? 'Behavior' : 'Commands';
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

  /**
   * Reads one repository, by id or by the name an address carries.
   *
   * Stored under the id the answer carries rather than under what was asked for,
   * so everything else in this component keeps one key whichever way the
   * repository was reached.
   */
  async function loadDetail(repository: string): Promise<RepositoryDetail> {
    const detail = await queryClient.fetchQuery({
      queryKey: ['repository', targetId, repository],
      queryFn: () => onLoad(repository),
      staleTime: 0,
    });
    rememberDetail(detail, repository);
    return detail;
  }

  function rememberDetail(detail: RepositoryDetail, requested: string): void {
    details = { ...details, [detail.repository.id]: detail };
    namedRepositories = { ...namedRepositories, [requested]: detail.repository.id };
  }

  function finishWorking(repositoryId: string): void {
    working.delete(repositoryId);
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
      rememberDetail(updated, updated.repository.name);
      queryClient.setQueriesData<RepositoryDetail>(
        { queryKey: ['repository', targetId] },
        (current) =>
          current?.repository.id === repositoryId || current === detail ? updated : current,
      );
      onChanged(targetId, updated);
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

  // A refused migration is durable and never expires, so this is the only way
  // back from it. It goes through the same working and failure plumbing every
  // other repository write does rather than inventing a second one.
  async function resetConfigMigration(repositoryId: string): Promise<void> {
    if (readOnly || working.has(repositoryId)) return;
    working.add(repositoryId);
    clearFailure(repositoryId);
    try {
      const detail = details[repositoryId];
      const updated = await onResetConfigMigration(targetId, repositoryId);
      rememberDetail(updated, updated.repository.name);
      queryClient.setQueriesData<RepositoryDetail>(
        { queryKey: ['repository', targetId] },
        (current) =>
          current?.repository.id === repositoryId || current === detail ? updated : current,
      );
      onChanged(targetId, updated);
    } catch (error) {
      setFailure(repositoryId, error, 'write');
    } finally {
      finishWorking(repositoryId);
    }
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
    <!-- Everything the column headings carry, for the widths where there are no
         column headings: the table becomes a stack of cards on a phone and its
         three sorts and three filters went with the `thead`, leaving the search
         field alone on the page. Sharing the same state as the headings rather
         than a copy of it. -->
    <TableToolsMenu
      sorts={[
        { label: 'Repository', direction: sortDirection('name'), onToggle: toggleNameSort },
        { label: 'File state', direction: sortDirection('file'), onToggle: toggleFileSort },
        { label: 'Updated', direction: sortDirection('updated'), onToggle: toggleUpdatedSort },
      ]}
      filters={[
        {
          label: 'Overrides',
          hint: 'Match any selected repository override',
          sections: SETTING_FILTER_SECTIONS,
          selected: settingSelection,
          multiple: true,
          fallbackValue: 'all',
          onChange: (values) => repositoryTable.getColumn('overrides')?.setFilterValue(values),
        },
        {
          label: 'File state',
          hint: 'Select one or more file states',
          sections: FILE_FILTER_SECTIONS,
          selected: fileFilters,
          multiple: true,
          onChange: (values) => repositoryTable.getColumn('file')?.setFilterValue(values),
        },
        {
          label: 'Enablement',
          hint: "Filter by Smyklot's effective state",
          sections: STATE_FILTER_SECTIONS,
          selected: [stateFilter],
          fallbackValue: 'all',
          onChange: (values) => repositoryTable.getColumn('enablement')?.setFilterValue(values[0]),
        },
      ]}
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
      <div class="repository-table-scroll table-card">
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
              <th class="filterable-heading enablement-heading">
                <div class="table-heading-layout">
                  <span class="heading-with-help">
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
                    onChange={(values) =>
                      repositoryTable.getColumn('enablement')?.setFilterValue(values[0])}
                  />
                </div>
              </th>
              <th class="action-heading">
                <!-- The column has a heading for the row's action, said only to a
                     screen reader: a word over a column of identical buttons is
                     noise to anyone who can see them. -->
                <span class="visually-hidden">Settings</span>
              </th>
            </tr>
          </thead>
          <tbody bind:this={repositoryScroll} data-panel-scroll>
            {#if repositories.length === 0}
              <tr class="empty-row">
                <td colspan="5">
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
                ><td colspan="5"></td></tr
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
                  <!-- The name is a name. What opens the dialog is the button at
                       the end of the row, where a reader looks for something to
                       press, rather than a cell that gives no sign of being one
                       until the pointer is already on it. -->
                  <span class="repo-copy">
                    <strong>{repository.name}</strong>
                    {#if repository.config_override_count > 0}
                      <span class="override-chip">
                        {repository.config_override_count}
                        {repository.config_override_count === 1 ? 'override' : 'overrides'}
                      </span>
                    {/if}
                  </span>
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
                    <span class="cap-trim">{formatRelative(repository.updated_at, now)}</span>
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
                <td class="row-action" data-label="Settings">
                  <button
                    class="btn btn-row btn-quiet configure"
                    aria-haspopup="dialog"
                    aria-label={`Configure ${repository.full_name}`}
                    onclick={(event) => openRepository(repository, event.currentTarget)}
                  >
                    <span class="cap-trim">Configure</span>
                  </button>
                </td>
              </tr>

              {#if repositoryFailure !== undefined && activeRepository?.id !== repository.id}
                <tr class="visually-hidden">
                  <td colspan="5"><span role="alert">{repositoryFailure.message}</span></td>
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
    id={REPOSITORY_DIALOG}
    open
    title={repository.name}
    description="Repository settings override workspace defaults and repository-file values"
    returnFocus={repositoryReturnFocus}
    onClose={closeRepository}
  >
    <!-- Beside the title, where every other switch in the product sits, and the
         product's own switch rather than a set of buttons dressed as one. -->
    {#snippet headerExtra()}
      {#if detail !== undefined}
        <SegmentedControl
          name="repository-{repository.id}-section"
          label="Settings for {repository.name}"
          compact
          options={[
            {
              value: 'file',
              label: 'File',
              badge: detail.config_file_error === undefined ? undefined : 1,
            },
            {
              value: 'behavior',
              label: 'Behavior',
              badge:
                configSectionCount(detail, 'behavior') === 0
                  ? undefined
                  : configSectionCount(detail, 'behavior'),
            },
            {
              value: 'commands',
              label: 'Commands',
              badge:
                configSectionCount(detail, 'commands') === 0
                  ? undefined
                  : configSectionCount(detail, 'commands'),
            },
          ]}
          value={detailSection(repository)}
          onSelect={(next) => selectDetailSection(repository, next as RepositoryDetailSection)}
        />
      {/if}
    {/snippet}
    {#if repositoryFailure !== undefined}
      <p class="form-error repository-modal-error" role="alert">{repositoryFailure.message}</p>
    {/if}

    <div class="repository-detail">
      {#if detail === undefined}
        <p class="detail-loading dim">Reading repository settings…</p>
      {:else}
        <!-- Keyboard focus lets a reader scroll a pane taller than the dialog. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <div
          class="repository-detail-content"
          id="repository-{repository.id}-detail-panel"
          role="group"
          aria-label="{sectionLabel(activeSection)} settings for {repository.name}"
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
                  <!-- The file is looked for in four places plus a chosen one,
                       so this names the one that won rather than the one that
                       used to be the only candidate. -->
                  <div><code class="mono">{detail.config_file_path || '—'}</code></div>
                  {#if detail.config_file_superseded !== undefined}
                    <p class="f-note">
                      Also present and not read: {detail.config_file_superseded.join(', ')}
                    </p>
                  {/if}
                  {#if detail.config_file_error !== undefined}
                    <p>{detail.config_file_error}</p>
                  {/if}
                  {#if detail.config_migration === 'proposed'}
                    <p class="f-note">
                      Smyklot proposed moving this to TOML{#if detail.config_migration_pr !== undefined}&nbsp;in
                        #{detail.config_migration_pr}{/if}
                    </p>
                  {:else if detail.config_migration !== 'none'}
                    <p class="f-note">
                      {detail.config_migration === 'declined'
                        ? 'The TOML migration was closed, so Smyklot will not ask again'
                        : 'GitHub refused the TOML migration, so Smyklot will not ask again'}
                      <button
                        type="button"
                        class="f-again"
                        disabled={readOnly || working.has(repository.id)}
                        onclick={() => resetConfigMigration(repository.id)}>Let it ask</button
                      >
                    </p>
                  {/if}
                </div>
                <Chip tone={FILE_STATUS_TONES[detail.repository.config_file_status]} dot>
                  {detail.repository.config_file_status.slice(0, 1).toUpperCase() +
                    detail.repository.config_file_status.slice(1)}
                </Chip>
              </div>
              <div class="override-row">
                <span class="o-label">
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

    /* What the two control columns actually measure: the inheritance marker plus
       the Enabled/Disabled switch, and the Configure button, each with the
       cell's own left and right padding. */
    --enablement-column: 13.25rem;
    --action-column: 6.8125rem;

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

  /* Only where the column headings are not. They carry the same three sorts and
     three filters while the table is a table, and two ways to set one value is
     one way too many - the menu exists because the headings go away, not
     instead of them. */
  .repository-tools :global(.tools-trigger) {
    display: none;
  }

  @media (max-width: 48rem) {
    .repository-tools :global(.tools-trigger) {
      display: inline-flex;
    }
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

  /* Surface, keyline, corner and lift come from `.table-card` in `app.css`. */
  .repository-table-scroll {
    max-width: 100%;
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

  /* Padding is shared; the rule under the header and the header's own type are
     not. A `th, td` font-size here outranks `thead th` in `app.css` - a class
     selector beats two element ones - so this table's heading was rendering at
     13px while the other five were at 11. */
  th,
  td {
    padding: var(--space-2) var(--space-3);
    text-align: left;
    vertical-align: middle;
  }

  td {
    border-bottom: 1px solid var(--rule);
    font-size: var(--font-size-meta);
  }

  /* Typography and ground come from `thead th` in `app.css`. This one used
     `--text-muted` where the other five used `--dim`, which is the drift the
     shared rule exists to end. */

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
      position: relative;
    }

    .repositories thead tr,
    .repositories tbody tr {
      display: grid;
      /* The two columns on the right hold controls of a known size, so they get
         exactly that and no share of the table: a fraction left the switch and
         the button against a gap that grew with the window. Everything else goes
         to the three text columns, in the approved catalog's 2:1:1.4.

         Fixed lengths rather than `max-content`: every row is its own grid, so a
         content-sized track is measured per row and the header stopped agreeing
         with the body about where a column began. The two numbers are the
         controls' own widths plus the cell padding, and
         tests/table-columns.test.ts holds them to that. */
      grid-template-columns: 2fr 1fr 1.4fr var(--enablement-column) var(--action-column);
      width: 100%;
    }

    .repositories th {
      width: auto;
    }

    /* The grid rows above repaint the row ground at a higher specificity than the plain
         `:hover` rule outside this block, so the pointer state has to be restated here or it never
         reaches the screen. */
    /* Not the empty state: a row hover says "this row is a thing you can act on",
       and a message explaining that there are no rows is not one. It also put the
       message's text on the hover ground, which is not a pairing any contrast was
       chosen for. */
    .repositories tbody tr:not(.virtual-spacer, .empty-row):hover {
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

    /* The last row keeps its separator, on purpose, even though it lands on the
       table's own bottom edge and the two hairlines read as one slightly thick
       line. Overscrolling pulls the rows away from that edge, and a last row
       with no line of its own ends in nothing while it is held there - an open
       table with its contents hanging out of it. A doubled hairline at rest is
       the smaller of the two faults. */

    .repositories tbody td:last-child {
      /* The enablement control sits at the column start, under the header
         label — same left alignment as every other column. */
      justify-content: flex-start;
    }

    /* In the flow, with a height of its own. It used to be stretched across the
       tbody with `inset: 0`, which worked while the table always filled the
       pane; now that a table is as tall as its contents, the contents of an
       empty one is this, and something absolutely positioned contributes no
       height at all - the message vanished and left a bare header. */
    .repositories tbody tr.empty-row {
      align-content: center;
      grid-template-columns: minmax(0, 1fr);
      min-height: 12rem;
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

  /* The row's action, at the end of the row where a reader looks for one. */
  .row-action {
    justify-content: flex-end;
  }

  /* Quiet until it is wanted. One of these in every row, and drawn as a button
     each time, they became a column of frames competing with the data they sit
     beside. It carries its word only, and takes its edges when a pointer is on
     it or a keyboard reaches it. */
  .configure {
    border-color: transparent;
  }

  .configure:hover:not(:disabled),
  .configure:focus-visible {
    background: var(--surface-control);
    border-color: var(--control-border);
    color: var(--text);
  }

  /* The word starts where the controls under it start. Every cell in this column
     opens with an inheritance marker, so the label indents past one: aligned to
     the cell's edge instead, it sat a chain's width to the left of everything it
     names. */
  .enablement-heading {
    padding-inline-start: calc(var(--space-3) + var(--inherit-marker-offset));
  }

  /* Name and chip are siblings on one centred row, so the chip's box and the
     name's caps share a centre line rather than an inline baseline. */
  /* Baselines, not box centres. The name and the badge are two runs of text on one
     line, and two runs of text sit on a shared baseline - centring their boxes
     instead lines up their middles, which for two different sizes (13px name,
     12px badge, cap heights 9.69 and 8.76) puts the smaller one's baseline above
     the larger one's and the badge reads as floating. Measured: 0.36px of baseline
     drift centred, 0.00 here. */
  .repo-copy {
    align-items: baseline;
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

  /* In a rounded plate of its own, like every other symbol that stands beside a
     title inside a card - the installation prompt's `+`, the add dialog's
     workspace mark. Bare, it was a 14px glyph floating in the card's padding
     with nothing to sit in, and it read as an icon that had lost its button.

     The plate is keyed to the glyph's own colour, so it carries the file's
     state - valid, invalid, bypassed - rather than a fixed brand tint. */
  .file-card-icon {
    align-items: center;
    background: color-mix(in srgb, currentcolor 10%, transparent);
    border: 1px solid color-mix(in srgb, currentcolor 24%, transparent);
    border-radius: var(--radius-control);
    color: var(--dim);
    display: inline-flex;
    flex: none;
    height: 2.25rem;
    justify-content: center;
    width: 2.25rem;
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

  /* A file the repository still carries and Smyklot is not reading is worth
     saying, and is not a failure - so it wears the dim tone the path above it
     wears rather than the danger tone the parse error does. */
  .f-copy p.f-note {
    color: var(--dim);
  }

  /* An inline continuation of the sentence above it, not a control in its own
     right: it sits on the same line, at the same size, and is underlined the
     way a link in prose is. Giving it a button's chrome would make refusing a
     migration look like it had a button to undo it, which is the opposite of
     what a durable refusal means. */
  .f-again {
    background: none;
    border: 0;
    color: var(--text);
    cursor: pointer;
    font: inherit;
    margin-left: 0.35rem;
    padding: 0;
    text-decoration: underline;
    text-underline-offset: 0.15em;
  }

  .f-again:disabled {
    color: var(--dim);
    cursor: default;
    text-decoration: none;
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
    /* Search on the left, one control on the right holding everything the
       column headings carried. */
    .repository-tools {
      grid-template-columns: minmax(0, 1fr) auto;
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
      padding: var(--card-inset);
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

    /* The name is the card's heading and was not dressed as one: at
       `--font-size-meta` and 600 it came out 13px, under the 12px/650 uppercase
       labels standing beneath it, so the thing the card is *about* read as the
       lightest line on it. A step up in size and weight puts it back on top.
       Mono still - it is a repository name and the column above it is mono. */
    /* A card heading, not a table cell, so it wraps rather than being cut: at
       320 the longest name here is 6px over its share of the line and lost its
       tail to an ellipsis, which is a poor trade for a heading. */
    .repo-copy {
      flex-wrap: wrap;
    }

    /* Trimmed to the cap band so the room either side of it is the room the
       reader sees. Untrimmed, the box carries ascender and descender air the
       name never uses, and every figure measured against it is off by however
       much of it this font happens to reserve. */
    .repo-copy strong {
      font-size: var(--font-size-body);
      font-weight: 700;
      overflow: visible;
      text-box: trim-both cap alphabetic;
      white-space: normal;
    }

    /* One height for every label-and-value row, set by the tallest thing that
       can stand in one: a segmented control, 2.125rem. Each row used to be as
       tall as whatever it happened to hold - 38px for the file state, 50px for
       the three carrying a control, a time that is given a control's height so
       it lines up in the *table* - so the card read as a stack of unrelated
       spacings. Padding comes down as the floor goes up, so the rows that were
       bloated by their control end up shorter than before rather than everything
       ending up taller. */
    .repository-row td[data-label] {
      min-height: calc(var(--control-height-compact) + var(--space-2));
      padding-block: var(--space-1);
    }

    /* The heading takes the room the rows gave back, and takes the same amount
       on both sides. Above it is the card's own padding plus its own, so the
       top is that one figure less the padding already there and the bottom is
       the figure itself - written once, and equal by construction rather than
       by two numbers that happen to add up. */
    .repository-row {
      --card-inset: var(--space-3);
      --heading-room: var(--space-5);
    }

    .repository-row td:first-child {
      border-bottom: 1px solid var(--border-subtle);
      /* Below the rule as much as above it, measured in ink rather than in
         boxes. Taken as a margin because the rows all stand at one height and
         padding on the first would have made it the odd one out, and it is
         `--heading-room` less what the row already puts between the line and its
         first glyph: its own padding, and the slack its 2.125rem control height
         leaves around a 13px label. Matching the boxes instead read as 7px too
         much space under the line, which is exactly that slack. */
      margin-bottom: calc(var(--heading-room) - var(--space-1) - var(--space-2));
      padding-block: calc(var(--heading-room) - var(--card-inset)) var(--heading-room);
    }

    .file-status {
      align-items: flex-start;
      flex-direction: column;
    }
  }

  /* Its edge is drawn on hover, which on a device that cannot hover means never:
     "Configure" was permanently a word in the corner of the card with nothing to
     say it could be pressed. Keyed on `hover: none` rather than on a width,
     because the absence of hover is the whole reason it was invisible. */
  @media (hover: none) {
    .configure {
      border-color: var(--control-border);
      color: var(--text);
    }
  }
</style>
