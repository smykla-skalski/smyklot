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
  import { untrack } from 'svelte';
  import { createAttachmentKey } from 'svelte/attachments';
  import { MediaQuery, SvelteSet } from 'svelte/reactivity';
  import { get } from 'svelte/store';
  import { useDebounce, useInterval } from 'runed';
  import { createInfiniteQuery, createQuery, useQueryClient } from '@tanstack/svelte-query';

  import { BOOLEAN_FIELDS } from '../config';
  import type { FilterSection } from '../filter-menu';
  import type { VirtualRenderRow } from '../virtual-rows.js';
  import { formatRelative, formatTimestamp } from '../format';
  import { availableRepositorySections, type RepositorySection } from '../routes';
  import {
    adoptRepositorySettings,
    overlayRepositorySettingsDocument,
    repositorySettingsDraftDocument,
    stageRepositorySettingsControl,
    type RepositorySettingsControlId,
    type RepositorySettingsDocument,
  } from '../repository-settings';
  import {
    adoptSyncOverrideSettings,
    stageSyncOverrideControl,
    syncOverrideDraftEnvelope,
    type SyncOverrideControlId,
    type SyncOverrideEditorEnvelope,
  } from '../repository-sync-override-settings';
  import { getPanelSession } from '../session.svelte';
  import { getSettingsDraftRegistry, type SettingsScope } from '../settings-drafts.svelte';
  import { pressableRow, rowOpensOn } from '../table-row';
  import {
    decodeRepositorySettingFilter,
    encodeRepositorySettingFilter,
    EPHEMERAL_PREFS,
    prefList,
    prefOption,
    prefText,
    type PrefsAccessor,
  } from '../preferences-sync';
  import type { RepositoryFailureSource } from '../repository';
  import type {
    ConfigKey,
    Page,
    RepositoryDetail,
    RepositoryFileStatus,
    RepositoryPageRequest,
    RepositorySettingFilter,
    RepositorySort,
    RepositoryStateFilter,
    RepositorySummary,
    SyncOverride,
  } from '../types';
  import Skeleton from './Skeleton.svelte';
  import SortIndicator from './SortIndicator.svelte';
  import Button from './Button.svelte';
  import Chip from './Chip.svelte';
  import DataTable from './DataTable.svelte';
  import FileStatusIndicator from './FileStatusIndicator.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import HelpTip from './HelpTip.svelte';
  import Icon from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import InheritControl from './InheritControl.svelte';
  import PageHeader from './PageHeader.svelte';
  import RepositorySettings from './RepositorySettings.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import SearchField from './SearchField.svelte';
  import TableToolsMenu from './TableToolsMenu.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  type RepositoryEnablement = 'inherit' | 'enabled' | 'disabled';
  type RepositoryFailure = { message: string; source: RepositoryFailureSource };

  const REPOSITORY_VALUE_OPTIONS = [
    { value: 'enabled', label: 'Enabled' },
    { value: 'disabled', label: 'Disabled' },
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
    onResetConfigMigration,
    onChanged,
    onLoadSyncOverride = null,
    readOnly = false,
    prefs = EPHEMERAL_PREFS,
  }: {
    targetId: string;
    defaultEnabled: boolean;
    fetchPage: (request: RepositoryPageRequest) => Promise<Page<RepositorySummary>>;
    onLoad: (repositoryId: string) => Promise<RepositoryDetail>;
    onResetConfigMigration: (targetId: string, repositoryId: string) => Promise<RepositoryDetail>;
    onChanged: (targetId: string, detail: RepositoryDetail) => void;
    /**
     * What this repository says about the files the organization keeps in step.
     * Read when the sync pane is opened rather than with the rest of the
     * detail: most visits are about the configuration file, and a second
     * request per repository for a pane nobody opened is a request per row.
     *
     * Null where there is nowhere to ask, which is the Root view of somebody
     * else's installation: sync is configured on the installation's own page
     * and has no Root address, so a pane offering to edit it there would be a
     * pane whose every save is a 404.
     */
    onLoadSyncOverride?: ((repositoryId: string) => Promise<SyncOverride>) | null;
    readOnly?: boolean;
    prefs?: PrefsAccessor;
  } = $props();

  /* Data comes in as props, because a workspace and the console read the same
     repositories through different endpoints. Where the reader IS does not: one
     repository has one address in each surface, and the session is what knows
     which surface this is being drawn in. */
  const session = getPanelSession();
  const drafts = getSettingsDraftRegistry();
  const settingsScope = $derived({
    type: 'installation',
    targetId,
  } as const satisfies SettingsScope);

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

  /* The one immediate repository command is configuration migration reset.
     Ordinary settings only stage into the application-wide draft registry. */
  let pendingEnablement = $state<Record<string, RepositoryEnablement>>({});
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

  /* The open repository is whatever the address names, not a click the component
     remembers, so a reload lands back on the repository that was open.

     It is named the way a person would name it - `api-gateway`, not an id - and
     the detail read accepts either, so a repository further down a paginated list
     still opens on a cold load without first finding it in the list. That
     response carries the summary the header needs. Names are unique within an
     installation and every read is scoped to one, so two organizations owning a
     repository of the same name never meet. */
  const activeRepositoryKey = $derived(session.currentRepository?.name ?? null);

  /* Which panes this surface has, worked out once here and handed to the page:
     whether there is anywhere to ask about sync is this component's fact, and
     the page reading `onLoadSyncOverride !== null` for itself would be the same
     question asked in two places. */
  const availableSections = $derived(availableRepositorySections(onLoadSyncOverride !== null));

  /* An address naming a pane this view cannot offer lands on the first one
     rather than on an empty box. Root manages somebody else's installation and
     sync has no Root address, so this is a link followed rather than a switch
     pressed. */
  const activeSection = $derived<RepositorySection>(
    offeredSection(session.currentRepository?.section ?? 'file'),
  );
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
  /**
   * What a repository says about the files.
   *
   * A query rather than a field this component fills in once, so the stream
   * saying something changed invalidates it with everything else. Kept in a
   * record it was read the first time the pane opened and never again - and the
   * revision it held is what the next save sends as the one it expects, so a
   * colleague saving first left this page unable to save at all until the whole
   * thing was reloaded.
   *
   * Only when the pane is opened. Most visits to this page are about the
   * repository's own configuration file, and reading this with the rest of the
   * detail would be a second request per repository for a pane nobody looked at.
   */
  const syncOverrideKey = (repositoryId: string) => ['sync-override', targetId, repositoryId];
  const syncOverrideQuery = createQuery(() => ({
    queryKey: syncOverrideKey(activeRepositoryId ?? ''),
    enabled: onLoadSyncOverride !== null && activeRepositoryId !== null && activeSection === 'sync',
    queryFn: () => {
      if (onLoadSyncOverride === null || activeRepositoryId === null) {
        throw new Error('open a repository first');
      }

      return onLoadSyncOverride(activeRepositoryId);
    },
  }));
  const activeRepositoryDetail = $derived.by(() => {
    if (activeRepositoryId === null) return undefined;
    const canonical = details[activeRepositoryId];
    if (canonical === undefined) return undefined;
    return overlayRepositorySettingsDocument(
      canonical,
      repositorySettingsDraftDocument(drafts, targetId, canonical),
    );
  });
  const activeRepository = $derived(
    activeRepositoryId === null
      ? null
      : (activeRepositoryDetail?.repository ??
          repositories.find((repository) => repository.id === activeRepositoryId) ??
          (repositoryDetailQuery.data?.repository.id === activeRepositoryId
            ? repositoryDetailQuery.data.repository
            : null) ??
          details[activeRepositoryId]?.repository ??
          null),
  );
  const activeSyncEnvelope = $derived.by(() => {
    const repositoryId = activeRepositoryId;
    const stored = syncOverrideQuery.data;
    if (repositoryId === null || stored === undefined || stored.unreadable) return undefined;
    return syncOverrideDraftEnvelope(drafts, targetId, repositoryId, stored);
  });
  const syncReadProblem = $derived(
    syncOverrideQuery.error === null ? null : errorMessage(syncOverrideQuery.error),
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
  const repositoryRenderRows: VirtualRenderRow[] = $derived.by(() =>
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
    const repositoryId = activeRepositoryId;
    const stored = syncOverrideQuery.data;
    if (repositoryId === null || stored === undefined || stored.unreadable) return;
    untrack(() => adoptSyncOverrideSettings(drafts, targetId, repositoryId, stored));
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

  function openRepository(repository: RepositorySummary): void {
    session.openRepository(repository.name);
  }

  function closeRepository(): void {
    session.closeRepository();
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
      rememberDetail(detail, key);
      if (failures[repositoryId]?.source === 'read') clearFailure(repositoryId);
    });
  });

  function offeredSection(section: RepositorySection): RepositorySection {
    return availableSections.includes(section) ? section : 'file';
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
    adoptRepositorySettings(drafts, targetId, detail);
    details = { ...details, [detail.repository.id]: detail };
    namedRepositories = { ...namedRepositories, [requested]: detail.repository.id };
  }

  function finishWorking(repositoryId: string): void {
    working.delete(repositoryId);
  }

  async function setEnabled(repository: RepositorySummary, value: string): Promise<void> {
    if (readOnly) return;
    if (value !== 'inherit' && value !== 'enabled' && value !== 'disabled') return;
    if (value === draftedEnablement(repository)) return;

    pendingEnablement = { ...pendingEnablement, [repository.id]: value };
    try {
      const detail = details[repository.id] ?? (await loadDetail(repository.id));
      const enabledOverride = value === 'inherit' ? null : value === 'enabled';
      stageRepositoryDocument(
        repository.id,
        {
          ...repositorySettingsDraftDocument(drafts, targetId, detail),
          enabled_override: enabledOverride,
        },
        [`repositories.${repository.id}.enabled_override`],
      );
    } catch (error) {
      setFailure(repository.id, error, 'read');
    } finally {
      const next = { ...pendingEnablement };
      delete next[repository.id];
      pendingEnablement = next;
    }
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

  function stageRepositoryDocument(
    repositoryId: string,
    next: RepositorySettingsDocument,
    controls: readonly RepositorySettingsControlId[],
  ): void {
    const canonical = details[repositoryId];
    if (canonical === undefined) return;
    for (const control of controls) {
      if (!stageRepositorySettingsControl(drafts, targetId, canonical, next, control)) {
        setFailure(repositoryId, new Error('This repository setting is not valid'), 'write');
        return;
      }
    }
    clearFailure(repositoryId);
  }

  function stageSyncEnvelope(
    repositoryId: string,
    next: SyncOverrideEditorEnvelope,
    control: SyncOverrideControlId,
  ): void {
    const stored = syncOverrideQuery.data;
    if (stored === undefined || stored.unreadable) return;
    if (!stageSyncOverrideControl(drafts, targetId, repositoryId, stored, next, control)) {
      setFailure(
        repositoryId,
        new Error('This repository file sync setting is not valid'),
        'write',
      );
      return;
    }
    clearFailure(repositoryId);
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

  function draftedEnablement(repository: RepositorySummary): RepositoryEnablement {
    const detail = details[repository.id];
    if (detail === undefined) return enabledValue(repository);
    const enabled = repositorySettingsDraftDocument(drafts, targetId, detail).enabled_override;
    return enabled === null ? 'inherit' : enabled ? 'enabled' : 'disabled';
  }

  function repositoryDirty(repositoryId: string): boolean {
    return drafts.dirtyAt(settingsScope, { section: 'repositories', path: [repositoryId] });
  }

  function controlDirty(controlId: string): boolean {
    return drafts.isControlDirty(settingsScope, controlId);
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
  /* One key for every row, created once: `createAttachmentKey` mints a fresh symbol
     each call, and minting one per row would give each row a different key and leak
     a property per render. */
  const ROW_PRESS = createAttachmentKey();
</script>

<!-- One repository's page stands in place of the list rather than over it: the
     address names a place inside this view, so the navigation still reads
     Repositories and leaving the page returns to the rows.

     The list is not unmounted while the page is open - it is the same component,
     so its query, its scroll position and its filters are all still there when
     the reader comes back, and coming back costs no request. -->
{#if activeRepository !== null}
  {@const repository = activeRepository}
  <RepositorySettings
    {repository}
    detail={activeRepositoryDetail}
    section={activeSection}
    failure={failures[repository.id]?.message ?? null}
    {readOnly}
    busy={working.has(repository.id)}
    backHref={session.repositoriesHref()}
    onBack={closeRepository}
    onSection={(section) => session.selectRepositorySection(section)}
    onChange={(next, controls) => stageRepositoryDocument(repository.id, next, controls)}
    onResetMigration={() => resetConfigMigration(repository.id)}
    sections={availableSections}
    syncOverride={syncOverrideQuery.data}
    syncEnvelope={activeSyncEnvelope}
    {syncReadProblem}
    {now}
    onChangeSync={(next, control) => stageSyncEnvelope(repository.id, next, control)}
    dirtyControls={drafts
      .dirtyControls(settingsScope)
      .filter(({ id }) => id.startsWith(`repositories.${repository.id}.`))
      .map(({ id }) => id)}
  />
{:else}
  <section class="plate repository-panel" aria-labelledby="repositories-heading">
    <PageHeader
      id="repositories-heading"
      eyebrow="Workspace"
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
            onChange: (values) =>
              repositoryTable.getColumn('enablement')?.setFilterValue(values[0]),
          },
        ]}
      />
    </div>

    <div
      class={['repository-results table-region', loading && 'loading']}
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
        <Skeleton
          label="Loading repositories"
          --skeleton-bar-a-width="min(14rem, 32%)"
          --skeleton-bar-b-left="48%"
        />
      {:else}
        <DataTable
          class="repository-table-scroll"
          tableClass="repositories"
          caption="Installed repositories"
          regionLabel="Repositories table"
          rows={repositoryRenderRows}
          rowKey={(virtualRow) => virtualRow.key}
          columnCount={5}
          bind:body={repositoryScroll}
          rowAttrs={(virtualRow) => ({
            class: [
              'repository-row data-row',
              virtualRow.virtual && 'virtual-row',
              repositoryDirty(repositoryAt(virtualRow.index).id) && 'is-unsaved',
            ]
              .filter(Boolean)
              .join(' '),
            'data-unsaved': repositoryDirty(repositoryAt(virtualRow.index).id) || undefined,
            style: virtualRow.virtual
              ? `height:${virtualRow.size}px;--row-y:${virtualRow.start}px`
              : '--row-y:0px',
            onclick: (event: MouseEvent) => {
              if (rowOpensOn(event)) openRepository(repositoryAt(virtualRow.index));
            },
            /* An attachment reaches an element through a spread under a key from
               `createAttachmentKey`, which is what `{@attach}` compiles to. The row
               is `DataTable`'s element now, so this is the only way to put one on
               it - and it is the supported way, not a way around anything. */
            [ROW_PRESS]: pressableRow,
          })}
        >
          {#snippet head()}
            <tr>
              <th class="sortable-heading" aria-sort={sortDirection('name')}>
                <div class="table-heading">
                  <button class="table-sort-button" onclick={toggleNameSort}>
                    <span class="table-heading-label">Repository</span>
                    <SortIndicator />
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
                <div class="table-heading">
                  <button class="table-sort-button" onclick={toggleFileSort}>
                    <span class="table-heading-label">File state</span>
                    <SortIndicator />
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
                <div class="table-heading">
                  <button class="table-sort-button" onclick={toggleUpdatedSort}>
                    <span class="table-heading-label">Updated</span>
                    <SortIndicator />
                  </button>
                </div>
              </th>
              <th class="filterable-heading enablement-heading">
                <div class="table-heading">
                  <span class="table-heading-label heading-with-help">
                    <span class="table-heading-label">Enablement</span>
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
          {/snippet}
          {#snippet lead()}
            {#if desktopTableLayout.current}
              <tr
                class="virtual-spacer"
                aria-hidden="true"
                style:height={`${$repositoryVirtualizer.getTotalSize()}px`}
                ><td colspan="5"></td></tr
              >
            {/if}
          {/snippet}
          {#snippet empty()}
            <TableEmptyState
              title={hasFilters ? 'No repositories match' : 'No repositories installed'}
              description={hasFilters
                ? 'Try another search or clear the active filters'
                : 'Repositories will appear after the installation catalog is refreshed'}
              actionLabel={hasFilters ? 'Clear filters' : undefined}
              onAction={hasFilters ? clearFilters : undefined}
            />
          {/snippet}
          {#snippet afterRow(virtualRow)}
            {@const repositoryFailure = failures[repositoryAt(virtualRow.index).id]}
            {#if repositoryFailure !== undefined}
              <tr class="visually-hidden">
                <td colspan="5"><span role="alert">{repositoryFailure.message}</span></td>
              </tr>
            {/if}
          {/snippet}
          {#snippet cells(virtualRow)}
            {@const repository = repositoryAt(virtualRow.index)}
            <!-- The keyboard reaches this row through the link in its first
                     cell, which is the element that carries the address and the
                     accessible name. The handler here only widens what a POINTER
                     can press to the whole row, so there is no second control to
                     give a key handler to and no role to claim. -->
            <td>
              <!-- The whole row opens the repository, and the name is the
                       link that says so: it carries the address, so it can be
                       opened in a new tab, and the row's own handler defers to
                       it. What the pointer presses is 56px tall; what a reader
                       tabs to and a crawler follows is one link, not five. -->
              <!-- The full name on hover, because this one truncates and a
                         reader who cannot read it has nowhere else to look. The
                         native tooltip rather than the product's own: it is one
                         per row for as many rows as the account has, and the
                         accessible name already carries the whole string, so
                         this is only for a pointer. -->
              <a
                class="repo-copy"
                href={session.repositoryHref(repository.name)}
                title={repository.name}
              >
                <strong class="band-trim">{repository.name}</strong>
                {#if repositoryDirty(repository.id)}
                  <span class="repository-unsaved-marker">
                    <Chip tone="warning" small>Unsaved changes</Chip>
                  </span>
                {/if}
                {#if repository.config_override_count > 0}
                  <span class="override-chip band-trim">
                    {repository.config_override_count}
                    {repository.config_override_count === 1 ? 'override' : 'overrides'}
                  </span>
                {/if}
              </a>
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
            <td
              data-label="Enablement"
              class:is-unsaved={controlDirty(`repositories.${repository.id}.enabled_override`)}
              data-unsaved={controlDirty(`repositories.${repository.id}.enabled_override`) ||
                undefined}
            >
              {#if !repository.available}
                <Chip small>Unavailable</Chip>
              {:else}
                {@const enablement =
                  pendingEnablement[repository.id] ?? draftedEnablement(repository)}
                <InheritControl
                  label="Enablement for {repository.full_name}"
                  source="Unconfigured repositories in Settings"
                  inheritedValue={defaultEnabled ? 'enabled' : 'disabled'}
                  inheritedLabel={defaultEnabled ? 'Enabled' : 'Disabled'}
                  value={enablement === 'inherit' ? null : enablement}
                  options={REPOSITORY_VALUE_OPTIONS}
                  disabled={readOnly || pendingEnablement[repository.id] !== undefined}
                  onSelect={(value) => void setEnabled(repository, value)}
                  onRestore={() => void setEnabled(repository, 'inherit')}
                />
              {/if}
            </td>
            <td class="row-action" data-label="Settings">
              <!-- The chevron the Root console's cards carry, for the same
                       reason: it marks the row as a way in without claiming to
                       be the thing pressed, which the whole row is. Nothing to
                       focus - the name is the row's one link, and a second stop
                       on the same address is a stop a keyboard reader has to
                       pass for nothing. -->
              <span class="row-chevron" aria-hidden="true">
                <Icon name="chevron-right" size={16} />
              </span>
            </td>
          {/snippet}
        </DataTable>
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
          <Button onclick={() => void loadNextPage()}>Try again</Button>
        </div>
      {/if}
    </div>
  </section>
{/if}

<style>
  .repository-panel {
    --local-control-height: var(--control-height-compact);

    /*
     * Four of these five columns hold something with a last value, so they are
     * given exactly what that value measures and nothing flexes them. The
     * repository name has no last value - GitHub allows a hundred characters -
     * so it is the one that takes the slack, and the one that truncates.
     *
     * Each number below is the wider of two measurements taken in the browser:
     * what the heading needs with its sort and filter controls in it, and what
     * the widest cell content needs with the cell's own 12px of padding either
     * side. A column sized to only one of those has a heading that wraps or a
     * value that is cut off, depending on which one was forgotten.
     */

    /* Heading 152px; "Bypassed" with its dot is 84 + 24. */
    --file-column: 9.5rem;
    /* "15 September 2026" is 121 + 24, which beats the 123px heading. The cell
       shows a relative time by default and a date when the reader asks for one,
       so it is sized for the longer of the two rather than for today's. */
    --updated-column: 9.25rem;
    /* The inheritance marker plus the Enabled/Disabled switch, and the chevron,
       each with the cell's own padding. The action column was 6.8125rem for a
       button carrying the word "Configure"; the word is gone, and a column still
       reserving room for it is 69px of nothing at the end of every row. */
    --enablement-column: 13.25rem;
    --action-column: 2.5rem;
    /* Around 22 characters of the mono face - enough to tell two repositories
       apart before the ellipsis, and above the 167px the heading needs. */
    --repository-column-floor: 11rem;

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

  /* Layout, keyline and corner come from `.table-region` in `app.css`. This drew
     its own keyline and its own corner around a card that already has both, so
     the one table given a border before the rest was the one that ended up with
     two. The filler behind a short table is `tbody`'s, below. */
  .repository-results.loading {
    cursor: progress;
  }

  :global(.repository-table-scroll .empty-cell) {
    border-bottom: 0;
    height: 12rem;
  }

  /* Surface, keyline, corner and lift come from `.table-card` in `app.css`. */
  :global(.repository-table-scroll) {
    max-width: 100%;
  }

  :global(.repositories) {
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
  /* `td` alone: a `th`'s padding is the heading's to give away - see `thead th`
     in `app.css` - and a class selector here would take it back without saying
     so. */
  th,
  td {
    text-align: left;
    vertical-align: middle;
  }

  td {
    padding: var(--space-2) var(--space-3);
  }

  td {
    border-bottom: 1px solid var(--rule);
    font-size: var(--font-size-meta);
  }

  /* Typography and ground come from `thead th` in `app.css`. This one used
     `--text-muted` where the other five used `--dim`, which is the drift the
     shared rule exists to end. */

  /* The same five widths the grid above uses, because they are the same five
     columns: `table-layout: fixed` takes a length as readily as a percentage,
     and a column's content does not change size because the layout algorithm
     did. `auto` on the first is how a fixed table says "take what is left",
     which is the same job the `1fr` does up there.

     These were six percentages summing to 77%, for a table that has had five
     columns for as long as this file has existed. The sixth never matched
     anything, and a fixed table whose widths do not add up scales all of them to
     fit - so every column here was 1.3 times the number written beside it. */
  th:first-child {
    width: auto;
  }

  th:nth-child(2) {
    width: var(--file-column);
  }

  th:nth-child(3) {
    width: var(--updated-column);
  }

  th:nth-child(4) {
    width: var(--enablement-column);
  }

  th:nth-child(5) {
    width: var(--action-column);
  }

  td:first-child {
    padding-left: var(--space-3);
  }

  td:last-child {
    padding-right: var(--space-3);
  }

  /* The heading's shape - the cell with no padding, the row inside it, the
     button carrying the inset and the filter over it - is shared, in `thead th`
     and `.table-heading` in `app.css`. What was here was a second copy of the
     button's reset, a `:global(.header-filter)` addressed to a class the popover
     stopped rendering, and a `justify-content: space-between` that stopped
     mattering when the filter came out of the flow. */
  :global(.repositories thead .table-heading) {
    height: 2.5rem;
  }

  /* Label and help mark on one centred row - the same 0.35rem the approved
     header cell uses, not whatever an inline space happens to measure. */
  .heading-with-help {
    align-items: center;
    display: flex;
    gap: 0.35rem;
  }

  :global(.repository-row) {
    transition: background-color var(--duration-fast) var(--ease-standard);
  }

  @media (min-width: 64.001rem) {
    :global(.repository-table-scroll) {
      display: flex;
      flex: 1;
      min-height: 0;
      overflow-x: auto;
    }

    :global(.repositories) {
      display: flex;
      flex: 1;
      flex-direction: column;
      min-height: 0;
    }

    :global(.repositories thead) {
      display: block;
      flex: none;
    }

    :global(.repositories tbody) {
      background: var(--table-filler-bg);
      display: block;
      flex: 1;
      min-height: 0;
      overflow-y: auto;
      position: relative;
    }

    :global(.repositories thead tr),
    :global(.repositories tbody tr) {
      display: grid;
      /* One flexible track, and it is the one whose content has no limit.
         Everything else is a fixed length - see the numbers and how they were
         measured at the top of this block.

         The floor on that track is the whole point. A bare `1fr` is
         `minmax(auto, 1fr)`, and `auto` resolves to the track's min-content,
         which for a `nowrap` mono name is the entire name: a hundred-character
         repository took 812px of a 977px row and pushed the other four columns
         off the end of it. With a floor the track can shrink instead, and the
         cell's own clip-and-ellipsis does what it was written to do.

         Fixed lengths rather than `max-content`: every row is its own grid, so a
         content-sized track is measured per row and the header stopped agreeing
         with the body about where a column began.
         tests/browser/table-columns.test.ts holds every column to its content. */
      grid-template-columns:
        minmax(var(--repository-column-floor), 1fr)
        var(--file-column)
        var(--updated-column)
        var(--enablement-column)
        var(--action-column);
      width: 100%;
    }

    :global(.repositories th) {
      width: auto;
    }

    /* The grid rows above repaint the row ground at a higher specificity than the plain
         `:hover` rule outside this block, so the pointer state has to be restated here or it never
         reaches the screen. */
    /* Not the empty state: a row hover says "this row is a thing you can act on",
       and a message explaining that there are no rows is not one. It also put the
       message's text on the hover ground, which is not a pairing any contrast was
       chosen for. */
    /* No `background` - `.data-row` in `app.css` carries the resting ground and
       every state with it. */
    :global(.repositories tbody tr:where(:not(.virtual-spacer))) {
      /* Pin the grid track to the row's fixed height: auto-sizing would take
         the tallest cell's border-box, push the bottom border one pixel past
         the virtual row, and let the next row paint over every separator. */
      grid-template-rows: 100%;
    }

    :global(.repositories tbody tr:not(.virtual-spacer) td) {
      align-items: center;
      display: flex;
    }

    /* The last row keeps its separator, on purpose, even though it lands on the
       table's own bottom edge and the two hairlines read as one slightly thick
       line. Overscrolling pulls the rows away from that edge, and a last row
       with no line of its own ends in nothing while it is held there - an open
       table with its contents hanging out of it. A doubled hairline at rest is
       the smaller of the two faults. */

    :global(.repositories tbody td:last-child) {
      /* The enablement control sits at the column start, under the header
         label — same left alignment as every other column. */
      justify-content: flex-start;
    }

    /* In the flow, with a height of its own. It used to be stretched across the
       tbody with `inset: 0`, which worked while the table always filled the
       pane; now that a table is as tall as its contents, the contents of an
       empty one is this, and something absolutely positioned contributes no
       height at all - the message vanished and left a bare header. */
    :global(.repositories tbody tr.state-row) {
      align-content: center;
      grid-template-columns: minmax(0, 1fr);
      min-height: 12rem;
    }

    :global(.repositories tbody .virtual-row) {
      left: 0;
      position: absolute;
      top: 0;
    }

    /* Restated at the virtual row's own specificity: the rule above that places
       it carries a class, an element and a class, and out-ranked the press. */
    :global(.repositories tbody .virtual-spacer) {
      background: transparent;
      border: 0;
      display: block;
      pointer-events: none;
      width: 1px;
    }

    :global(.virtual-spacer td) {
      display: none;
    }
  }

  /* Hover, press and focus come from `.data-row` in `app.css`; this only says
     the row is a way in. */
  :global(.repository-row) {
    cursor: pointer;
  }

  :global(.repository-row.is-unsaved) {
    box-shadow: inset 2px 0 var(--brand-action);
  }

  :global(.repository-row td.is-unsaved) {
    background: color-mix(in srgb, var(--brand-action-tint) 45%, transparent);
  }

  /* The row's way in, at the end of the row where a reader looks for one. */
  .row-action {
    justify-content: flex-end;
  }

  /* The mark, not the target: the row is what gets pressed, and a chevron that
     lit up on its own hover would promise a smaller hit area than there is. It
     follows the row instead - dim at rest, and the row's own text colour once
     the row is under a pointer or holds focus. */
  .row-chevron {
    align-items: center;
    color: var(--text-muted);
    display: inline-flex;
    justify-content: center;
    transition: color var(--duration-fast) var(--ease-standard);
  }

  :global(.repository-row:hover .row-chevron),
  :global(.repository-row:has(:focus-visible) .row-chevron) {
    color: var(--text-primary);
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
  /* A link that reads as the name it is. The row around it carries the press
     affordance, so underlining this too would draw a second control inside a
     control - the colour is inherited and only the focus ring marks it out.

     `background-image: none` cancels the product's global hover layer, which
     `app.css` paints on every `a[href]:hover`. That rule is right for a link
     standing on its own and wrong for this one: the whole row is the target, the
     row already washes, and a second wash on the name drew a lighter patch
     around the words - a hover inside a hover. It is a `background-image` and
     not a colour, which is the only reason it survived this long: every probe
     that went looking for it read `backgroundColor` and found nothing.

     Stated on `:hover` rather than on the link, because that is what it has to
     outrank: `a[href]:hover` carries a pseudo-class, and a plain declaration
     here loses to it. */
  .repo-copy {
    align-items: baseline;
    color: inherit;
    display: flex;
    gap: var(--space-2);
    min-width: 0;
    text-decoration: none;
  }

  /* `:not(:active)` so this only cancels the HOVER wash. Without it the rule
     also outranked `a[href]:active`, which is what paints the press - so the
     name, and therefore the row, gave no feedback when pressed. */
  .repo-copy:hover:not(:active) {
    background-image: none;
  }

  /* Around the name, not around the cell: the ring should sit on the thing that
     was reached, and a cell-wide ring in a virtualised row is a rectangle whose
     right edge is wherever the column happens to end. */
  .repo-copy:focus-visible {
    border-radius: var(--r-chip);
    outline: 2px solid var(--focus);
    outline-offset: 3px;
  }

  /* `clip` rather than `hidden`: the cap trim ends the box at the baseline, so
     a hidden overflow would shave the descenders off "api-gateway". The clip
     margin lets ink outside the box survive while the ellipsis still fires. */
  .repo-copy strong {
    /* Not bold. The name is the row's subject, and the row is already the thing
       being pressed - weight on top of that made every name read as its own
       control in a table of them. */
    font: 400 var(--font-size-meta) / 1 var(--mono);
    letter-spacing: 0;
    min-width: 0;
    overflow: clip;
    overflow-clip-margin: 0.35em;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* A visible destination marker, not only the row's colour and inset. The
     shared small chip keeps the state legible and keyed in every theme, while
     this wrapper keeps it from shrinking away beside a long repository name. */
  .repository-unsaved-marker {
    display: inline-flex;
    flex: none;
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
    white-space: nowrap;
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

  @media (max-width: 74rem) {
    .repository-tools {
      grid-template-columns: 1fr 1fr;
    }

    .repository-tools :global(.search-field) {
      grid-column: 1 / -1;
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

    :global(.repositories) {
      min-width: 0;
    }

    :global(.repositories thead) {
      display: none;
    }

    :global(.repositories),
    :global(.repositories tbody),
    :global(.repository-row),
    :global(.repository-table-scroll .repository-row td) {
      display: block;
      width: 100%;
    }

    :global(.repository-row) {
      border-bottom: 1px solid var(--border-subtle);
      padding: var(--card-inset);
    }

    :global(.repository-table-scroll .repository-row td) {
      align-items: center;
      border: 0;
      display: flex;
      flex-wrap: wrap;
      min-inline-size: 0;
      justify-content: space-between;
      padding: var(--space-2) 0;
    }

    :global(.repository-table-scroll .repository-row td:first-child) {
      padding-top: 0;
    }

    :global(.repository-table-scroll .repository-row td[data-label]::before) {
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
      /* A repository name has no spaces in it, so `normal` wrapping has nowhere
         to break and the line simply runs on: a hundred-character name needed
         423px of a 325px card and 98px of it was cut off - no ellipsis, because
         nothing here truncates, just gone. `anywhere` breaks mid-run, which is
         what a card can afford and a table row cannot. */
      overflow-wrap: anywhere;
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
    :global(.repository-table-scroll .repository-row td[data-label]) {
      min-height: calc(var(--control-height-compact) + var(--space-2));
      padding-block: var(--space-1);
    }

    /* The heading takes the room the rows gave back, and takes the same amount
       on both sides. Above it is the card's own padding plus its own, so the
       top is that one figure less the padding already there and the bottom is
       the figure itself - written once, and equal by construction rather than
       by two numbers that happen to add up. */
    :global(.repository-row) {
      --card-inset: var(--space-3);
      --heading-room: var(--space-5);
    }

    :global(.repository-table-scroll .repository-row td:first-child) {
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
  }

  /* Its edge is drawn on hover, which on a device that cannot hover means never:
     "Configure" was permanently a word in the corner of the card with nothing to
     say it could be pressed. Keyed on `hover: none` rather than on a width,
     because the absence of hover is the whole reason it was invisible. */
  @media (hover: none) {
  }
</style>
