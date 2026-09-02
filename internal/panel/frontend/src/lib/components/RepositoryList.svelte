<script lang="ts">
  import { untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { useDebounce, useInterval } from 'runed';
  import { createInfiniteQuery, createQuery, useQueryClient } from '@tanstack/svelte-query';

  import { BOOLEAN_FIELDS } from '../config';
  import type { FilterSection } from '../filter-menu';
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
  import { rowOpensOn } from '../row-open';
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
    SyncStatus,
  } from '../types';
  import { repositorySentence } from '../repository-sentence';
  import Skeleton from './Skeleton.svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import Chip from './Chip.svelte';
  import FileStatusIndicator from './FileStatusIndicator.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import RepositorySettings from './RepositorySettings.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import Switch from './Switch.svelte';
  import ListToolsMenu from './ListToolsMenu.svelte';
  import EmptyState from './EmptyState.svelte';

  type RepositoryEnablement = 'inherit' | 'enabled' | 'disabled';
  type RepositoryFailure = { message: string; source: RepositoryFailureSource };

  const FILE_STATUSES = ['valid', 'missing', 'invalid', 'bypassed'] as const;
  const CONFIG_FILTER_KEYS: readonly ConfigKey[] = [
    ...BOOLEAN_FIELDS.map((field) => field.key),
    'command_prefix',
    'allowed_commands',
    'command_aliases',
  ];
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
  const {
    targetId,
    defaultEnabled,
    fetchPage,
    onLoad,
    onResetConfigMigration,
    onChanged,
    onLoadSyncOverride = null,
    onLoadSyncStatus = null,
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
     * else's workspace: sync is configured on the workspace's own page
     * and has no Root address, so a pane offering to edit it there would be a
     * pane whose every save is a 404.
     */
    onLoadSyncOverride?: ((repositoryId: string) => Promise<SyncOverride>) | null;
    /**
     * The fleet, for the one line the open repository's page says about itself -
     * what the plan would change there. Null on the surface that has no sync.
     */
    onLoadSyncStatus?: (() => Promise<SyncStatus>) | null;
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
    type: 'workspace',
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
     workspace and every read is scoped to one, so two organizations owning a
     repository of the same name never meet. */
  const activeRepositoryKey = $derived(session.currentRepository?.name ?? null);

  /* Whether the page draws its File sync card, worked out once here and handed to it:
     Root manages somebody else's workspace and sync has no Root address, so there is
     nowhere to ask - and the page reading `onLoadSyncOverride !== null` for itself would
     be the same question asked in two places. */
  const offersSync = $derived(onLoadSyncOverride !== null);
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
   * Only where there is somewhere to ask. The page is one scroll now, so the File sync
   * card is on screen with the rest and its read happens with them - what still gates
   * it is whether this surface offers sync at all: the Root view of somebody else's
   * workspace has no endpoint to ask.
   */
  const syncOverrideKey = (repositoryId: string) => ['sync-override', targetId, repositoryId];
  const syncOverrideQuery = createQuery(() => ({
    queryKey: syncOverrideKey(activeRepositoryId ?? ''),
    enabled: offersSync && activeRepositoryId !== null,
    queryFn: () => {
      if (onLoadSyncOverride === null || activeRepositoryId === null) {
        throw new Error('open a repository first');
      }

      return onLoadSyncOverride(activeRepositoryId);
    },
  }));
  /* Asked only while a repository page is open, and shared with every other
     reader of the fleet through the key the sync view already uses. */
  const syncStatusQuery = createQuery(() => ({
    queryKey: ['sync-status', targetId],
    enabled: onLoadSyncStatus !== null && activeRepositoryId !== null,
    queryFn: () => {
      if (onLoadSyncStatus === null) throw new Error('this surface has no sync');

      return onLoadSyncStatus();
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
  /**
   * The three states the list leads with, and how many repositories are in each.
   *
   * Two questions rather than three: every repository is one or the other, so the
   * count of those that are off is the count of all of them less the count that are
   * on. Both carry the search and the narrower filters, because a count that ignores
   * what is on screen is a count of something else.
   */
  const stateCountsQuery = createQuery(() => ({
    queryKey: ['repository-state-counts', filterKey],
    queryFn: async (): Promise<{ all: number; enabled: number }> => {
      const shared = {
        query: appliedQuery,
        sort,
        limit: 1,
        files: [...fileFilters],
        setting: settingFilter,
      };
      const [all, enabled] = await Promise.all([
        fetchPage({ ...shared, state: 'all' }),
        fetchPage({ ...shared, state: 'enabled' }),
      ]);
      return { all: all.total, enabled: enabled.total };
    },
  }));

  const STATE_SEGMENTS = $derived.by(() => {
    const counts = stateCountsQuery.data;
    const badge = (value: number | undefined): string | undefined =>
      value === undefined ? undefined : String(value);
    return [
      { value: 'all', label: 'All', badge: badge(counts?.all) },
      { value: 'enabled', label: 'On', badge: badge(counts?.enabled), tone: 'on' as const },
      {
        value: 'disabled',
        label: 'Off',
        badge: badge(counts === undefined ? undefined : Math.max(0, counts.all - counts.enabled)),
        tone: 'off' as const,
      },
    ];
  });

  /**
   * What the list is showing of what there is, said once at its foot.
   *
   * The space before "of" is a non-breaking one, written as an escape: the count and
   * what it is a count of are one atom, and a literal one in the source is invisible
   * to everybody reading it afterwards.
   */
  const shownRange = $derived.by(() => {
    const total = page?.total ?? repositories.length;
    if (repositories.length === 0) return 'Nothing to show';
    return `Showing 1-${repositories.length}\u{a0}of ${total}`;
  });

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

  /* The order the list is read in, now that there are no column headings to carry
     it: one sort at a time, each with its two directions, chosen from the tools
     menu beside the search. */
  const SORT_PAIRS = {
    name: ['name_asc', 'name_desc'],
    file: ['file_asc', 'file_desc'],
    overrides: ['overrides_asc', 'overrides_desc'],
    updated: ['oldest', 'newest'],
  } as const satisfies Record<string, readonly [RepositorySort, RepositorySort]>;

  function toggleNameSort(): void {
    toggleColumnSort('name');
  }

  function toggleUpdatedSort(): void {
    toggleColumnSort('updated');
  }

  function toggleFileSort(): void {
    toggleColumnSort('file');
  }

  function toggleColumnSort(column: keyof typeof SORT_PAIRS): void {
    const [ascending, descending] = SORT_PAIRS[column];
    sort = sort === ascending ? descending : ascending;
  }

  function sortDirection(column: keyof typeof SORT_PAIRS): 'ascending' | 'descending' | undefined {
    const [ascending, descending] = SORT_PAIRS[column];
    if (sort === ascending) return 'ascending';
    return sort === descending ? 'descending' : undefined;
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

<!--
@component
Every repository in one workspace, and what the bot is doing with each. The list is the
workspace's main page: a repository's own page stands in place of it rather than over
it, so the navigation still reads Repositories.

`defaultEnabled` is what an unconfigured repository would do, which is why the list can
say something about repositories that have no settings at all - the common case, and the
one a reader most needs to understand.

Rows load on a cursor rather than by page, so there is no total and no pager; the count
a workspace has is not a number worth blocking the first screenful on.
-->

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
    failure={failures[repository.id]?.message ?? null}
    {readOnly}
    busy={working.has(repository.id)}
    backHref={session.repositoriesHref()}
    onBack={closeRepository}
    onChange={(next, controls) => stageRepositoryDocument(repository.id, next, controls)}
    onResetMigration={() => resetConfigMigration(repository.id)}
    enablement={draftedEnablement(repository)}
    onEnablement={(next) => void setEnabled(repository, next)}
    {offersSync}
    fleet={syncStatusQuery.data ?? null}
    syncOverride={syncOverrideQuery.data}
    syncEnvelope={activeSyncEnvelope}
    {syncReadProblem}
    {now}
    onChangeSync={(next, control) => stageSyncEnvelope(repository.id, next, control)}
    onFormattingValidity={(valid) =>
      drafts.setValidationProblem(
        settingsScope,
        `repositories.${repository.id}.config_patch.formatting`,
        valid ? null : 'Formatting widths must be whole numbers within their documented bounds',
      )}
    dirtyControls={drafts
      .dirtyControls(settingsScope)
      .filter(({ id }) => id.startsWith(`repositories.${repository.id}.`))
      .map(({ id }) => id)}
  />
{:else}
  <section class="plate repository-panel" aria-labelledby="repositories-heading">
    <PageHeader
      id="repositories-heading"
      title="Repositories"
      description="Where Smyklot answers commands. Sync is separate - the sync pages name their own scope"
    />

    <div class="filter-bar">
      <SearchField
        label="Search repositories"
        placeholder="Find a repository"
        value={search}
        onInput={(value) => (search = value)}
      />
      <!-- The one filter the list leads with: whether Smyklot answers there at all.
           Everything narrower stays in the tools menu beside it. -->
      <SegmentedControl
        name="repository-state"
        label="Show"
        options={STATE_SEGMENTS}
        value={stateFilter}
        onSelect={(value) => selectStateFilter([value])}
      />
      <!-- Everything the column headings carry, for the widths where there are no
         column headings: the table becomes a stack of cards on a phone and its
         three sorts and three filters went with the `thead`, leaving the search
         field alone on the page. Sharing the same state as the headings rather
         than a copy of it. -->
      <!-- The narrower questions, on the same bar: which repositories carry their own
           settings, and what their configuration file is doing. The list leads with
           the switch a reader came for; these are one press away rather than three
           column headings wide. -->
      <span class="push-end">
        <ListToolsMenu
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
              onChange: selectSettingFilter,
            },
            {
              label: 'File state',
              hint: 'Select one or more file states',
              sections: FILE_FILTER_SECTIONS,
              selected: fileFilters,
              multiple: true,
              onChange: selectFileFilters,
            },
          ]}
        />
      </span>
    </div>

    <div
      class={['repository-results list-region', loading && 'loading']}
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
      {:else if repositories.length === 0}
        <Card>
          <EmptyState
            title={hasFilters ? 'No repositories match' : 'No repositories installed'}
            description={hasFilters
              ? 'Try another search or clear the active filters'
              : 'Repositories will appear after the workspace catalog is refreshed'}
            actionLabel={hasFilters ? 'Clear filters' : undefined}
            onAction={hasFilters ? clearFilters : undefined}
          />
        </Card>
      {:else}
        <Card>
          <ul class="object-list">
            {#each repositories as repository (repository.id)}
              {@const dirty = repositoryDirty(repository.id)}
              {@const enablement =
                pendingEnablement[repository.id] ?? draftedEnablement(repository)}
              {@const on = enablement === 'inherit' ? defaultEnabled : enablement === 'enabled'}
              <li>
                <!-- The whole row opens the repository and the switch still answers
                     inside it: the address is a layer UNDER the content rather than a
                     wrapper around it, so one press flips the bot and any other press
                     opens the page. -->
                <div
                  class="object-row repository-row"
                  class:is-unsaved={dirty}
                  data-unsaved={dirty || undefined}
                >
                  <a
                    class="row-hit"
                    href={session.repositoryHref(repository.name)}
                    aria-label="Open {repository.name}"
                    onclick={(event) => {
                      if (!rowOpensOn(event)) return;
                      event.preventDefault();
                      openRepository(repository);
                    }}
                  >
                  </a>
                  <span class="object-main">
                    <span class="object-name-row">
                      <!-- The full name on hover, because this one truncates and a
                           reader who cannot read it has nowhere else to look. -->
                      <span class="object-name" title={repository.name}>{repository.name}</span>
                      {#if dirty}
                        <Chip tone="warning" small>Unsaved changes</Chip>
                      {/if}
                      <!-- A file state speaks only when it is worth saying. Most
                           repositories carry no file at all, and a pill on every row
                           saying so is a column of noise. -->
                      {#if repository.config_file_status !== 'missing'}
                        <FileStatusIndicator
                          id="file-status-{repository.id}"
                          status={repository.config_file_status}
                          showLabel
                        />
                      {/if}
                    </span>
                    <span class="object-sum">{repositorySentence(repository, on)}</span>
                  </span>
                  <span class="object-side">
                    {#if !repository.available}
                      <Chip small>Unavailable</Chip>
                    {:else}
                      <Switch
                        bare
                        checked={on}
                        label="Smyklot in {repository.full_name}"
                        disabled={readOnly || pendingEnablement[repository.id] !== undefined}
                        onToggle={(next) =>
                          void setEnabled(repository, next ? 'enabled' : 'disabled')}
                      />
                    {/if}
                    <span class="row-chevron" aria-hidden="true">
                      <Icon name="chevron-right" size="xs" />
                    </span>
                  </span>
                </div>
                {#if failures[repository.id] !== undefined}
                  <p class="visually-hidden" role="alert">{failures[repository.id]?.message}</p>
                {/if}
              </li>
            {/each}
          </ul>
          <div class="list-foot">
            <span>{shownRange}</span>
            {#if page?.next_cursor != null}
              <Button tone="quiet" disabled={loading} onclick={() => void loadNextPage()}>
                Show more
              </Button>
            {/if}
          </div>
        </Card>
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

  .repository-results.loading {
    cursor: progress;
  }

  /* The search leads the bar and takes the room a name needs; the segments and the
     tools menu sit beside it. */
  .filter-bar :global(.search-field) {
    flex: 1 1 12rem;
    max-inline-size: 20rem;
    min-inline-size: 0;
  }

  /* The row's way in, at the end of the row where a reader looks for one. The
     MARK, not the target: the row is what gets pressed, and a chevron that lit up
     on its own hover would promise a smaller hit area than there is. */
  .row-chevron {
    align-items: center;
    color: var(--text-muted);
    display: inline-flex;
    justify-content: center;
    transition: color var(--duration-fast) var(--ease-standard);
  }

  .repository-row:has(.row-hit:hover) .row-chevron,
  .repository-row:has(:focus-visible) .row-chevron {
    color: var(--text-primary);
  }

  /* A visible destination marker, not only the row's inset: a reader who left a
     draft somewhere finds their way back to it. */
  .repository-row.is-unsaved {
    box-shadow: inset 2px 0 var(--brand-action);
  }

  /* The name is a repository's, so it keeps the mono voice the rest of the product
     gives one - and it wraps rather than truncating, because a name is the row's
     subject and a cut-off subject is a row about nothing. */
  /* A repository is named in the mono voice; the breaking is the shared rule's. */
  .repository-row .object-name {
    font-family: var(--mono);
    font-weight: 400;
    letter-spacing: 0;
  }

  /* A row is a way in, and says so. */
  .repository-row {
    cursor: pointer;
  }

  /* On a phone the row stacks: the name and its sentence take the width, and the
     switch and the chevron sit under them rather than squeezing the words into a
     column two characters wide. */
  @media (max-width: 36rem) {
    .repository-row {
      grid-template-columns: minmax(0, 1fr);
      row-gap: var(--space-3);
    }

    .repository-row .object-side {
      justify-content: flex-start;
    }
  }
</style>
