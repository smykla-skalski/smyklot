<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';

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
  import Modal from './Modal.svelte';
  import PaginationBar from './PaginationBar.svelte';
  import PanelHeader from './PanelHeader.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  type RepositoryEnablement = 'inherit' | 'enabled' | 'disabled';
  type RepositoryFailure = { message: string; source: RepositoryFailureSource };
  type RepositoryDetailSection = 'file' | 'behavior' | 'commands';

  const REPOSITORY_ENABLEMENT_OPTIONS = [
    { value: 'inherit', label: 'Default' },
    { value: 'enabled', label: 'On', tone: 'on' },
    { value: 'disabled', label: 'Off', tone: 'off' },
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
          label: 'On',
          description: 'Smyklot handles the repository',
          tone: 'on',
        },
        {
          value: 'disabled',
          label: 'Off',
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
  let limit = $state<number>(20);
  let page = $state<Page<RepositorySummary> | null>(null);
  let pageIndex = $state(0);
  let loading = $state(false);
  let problem = $state<string | null>(null);
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
  let repositoryTools: HTMLDivElement;
  let scrollAfterPageSizeChange = false;

  const repositories = $derived(page?.items ?? []);
  const total = $derived(page?.total ?? 0);
  const itemCount = $derived(repositories.length);
  const pageCount = $derived(Math.max(1, Math.ceil(total / limit)));
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
      void loadPage(pageIndex, filterKey);
      refreshVisibleRepository(version);
    });
  });

  async function resetAndLoad(key: string): Promise<void> {
    pageIndex = 0;
    page = null;
    await loadPage(0, key);
  }

  async function loadPage(index: number, key: string): Promise<void> {
    const sequence = ++requestSequence;
    loading = true;
    problem = null;
    try {
      const loaded = await fetchPage({
        cursor: index === 0 ? undefined : String(index * limit),
        query: appliedQuery,
        sort,
        limit,
        state: stateFilter,
        files: fileFilters,
        setting: settingFilter,
      });
      if (sequence !== requestSequence || key !== filterKey) return;
      if (index > 0 && loaded.total <= index * limit) {
        await resetAndLoad(key);
        return;
      }
      page = loaded;
    } catch (error) {
      if (sequence === requestSequence && key === filterKey) {
        problem = error instanceof Error ? error.message : String(error);
      }
    } finally {
      if (sequence === requestSequence && key === filterKey) {
        loading = false;
        await scrollToResultsAfterPageSizeChange();
      }
    }
  }

  function selectPageSize(nextLimit: number): void {
    if (nextLimit === limit) return;
    scrollAfterPageSizeChange = true;
    limit = nextLimit;
  }

  async function scrollToResultsAfterPageSizeChange(): Promise<void> {
    if (!scrollAfterPageSizeChange) return;
    scrollAfterPageSizeChange = false;
    await tick();
    repositoryTools.scrollIntoView({ block: 'start' });
  }

  async function selectPage(nextIndex: number): Promise<void> {
    const bounded = Math.min(pageCount - 1, Math.max(0, nextIndex));
    if (bounded === pageIndex || loading) return;
    pageIndex = bounded;
    await loadPage(bounded, filterKey);
  }

  function retry(): void {
    void loadPage(pageIndex, filterKey);
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
    sort = sort === 'name_asc' ? 'name_desc' : 'name_asc';
  }

  function toggleUpdatedSort(): void {
    sort = sort === 'newest' ? 'oldest' : 'newest';
  }

  function sortDirection(column: 'name' | 'updated'): 'ascending' | 'descending' | 'none' {
    if (column === 'name') {
      if (sort === 'name_asc') return 'ascending';
      if (sort === 'name_desc') return 'descending';
      return 'none';
    }
    if (sort === 'oldest') return 'ascending';
    if (sort === 'newest') return 'descending';
    return 'none';
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

  <div class="repository-tools" bind:this={repositoryTools}>
    <SearchField
      label="Search repositories"
      placeholder="Search repositories"
      value={search}
      onInput={(value) => (search = value)}
    />

    <FilterMenu
      label="Repository state"
      summary={stateSummary}
      hint="Filter by Smyklot's effective state"
      sections={STATE_FILTER_SECTIONS}
      selected={[stateFilter]}
      onChange={selectStateFilter}
    />

    <FilterMenu
      label="Repository files"
      summary={fileSummary}
      hint="Select one or more file states"
      sections={FILE_FILTER_SECTIONS}
      selected={fileFilters}
      multiple
      onChange={selectFileFilters}
    />

    <FilterMenu
      label="Custom settings"
      summary={settingSummary}
      hint="Match any selected repository override"
      sections={SETTING_FILTER_SECTIONS}
      selected={settingSelection}
      multiple
      fallbackValue="all"
      align="end"
      wide
      onChange={selectSettingFilter}
    />
  </div>

  <div class={['repository-results', loading && 'loading']} aria-busy={loading}>
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
      <div class="result-state dim">
        <strong>{hasFilters ? 'No repositories match' : 'No repositories installed'}</strong>
        <span>
          {hasFilters
            ? 'Try another search or clear the current filters'
            : 'Repositories will appear after the installation catalog is refreshed'}
        </span>
        {#if hasFilters}
          <button class="btn" type="button" onclick={clearFilters}>Clear filters</button>
        {/if}
      </div>
    {:else}
      <div class="repository-table-scroll">
        <table class="repositories">
          <thead>
            <tr>
              <th class="sortable-heading" aria-sort={sortDirection('name')}>
                <button class="sort-heading" onclick={toggleNameSort}>
                  Repository
                  <span class="sort-indicator" aria-hidden="true"
                    ><Icon name="sort" size={14} /></span
                  >
                </button>
              </th>
              <th>Visibility</th>
              <th>Default branch</th>
              <th>File state</th>
              <th class="numeric-heading">Overrides</th>
              <th class="sortable-heading" aria-sort={sortDirection('updated')}>
                <button class="sort-heading" onclick={toggleUpdatedSort}>
                  Updated
                  <span class="sort-indicator" aria-hidden="true"
                    ><Icon name="sort" size={14} /></span
                  >
                </button>
              </th>
              <th>Enablement</th>
            </tr>
          </thead>
          <tbody>
            {#each repositories as repository (repository.id)}
              {@const repositoryFailure = failures[repository.id]}
              <tr class="repository-row">
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
                <tr class="repository-message-row">
                  <td colspan="7"
                    ><p class="form-error" role="alert">{repositoryFailure.message}</p></td
                  >
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>

  {#if problem === null && page !== null}
    <PaginationBar
      label="Repositories"
      {pageIndex}
      {pageCount}
      pageSize={limit}
      {itemCount}
      {total}
      disabled={loading}
      onPageSelect={(nextIndex) => void selectPage(nextIndex)}
      onPageSizeSelect={selectPageSize}
    />
  {/if}
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
    margin-bottom: 0;
    overflow: visible;
  }

  .repository-tools {
    align-items: center;
    background: transparent;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: minmax(16rem, 1fr) 7rem 7.5rem 10.5rem;
    padding: 0 0 var(--space-3);
  }

  .repository-results {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-bottom: 0;
    border-radius: var(--radius-surface) var(--radius-surface) 0 0;
    overflow: hidden;
    transition: opacity 120ms ease-out;
  }

  .repository-panel :global(.pagination-bar) {
    border: 1px solid var(--border-subtle);
    border-radius: 0 0 var(--radius-surface) var(--radius-surface);
  }

  .repository-results.loading {
    opacity: 0.55;
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
    overflow-x: auto;
  }

  .repositories {
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
    width: 25%;
  }

  th:nth-child(2) {
    width: 10%;
  }

  th:nth-child(3) {
    width: 12%;
  }

  th:nth-child(4) {
    width: 12%;
  }

  th:nth-child(5) {
    width: 9%;
  }

  th:nth-child(6) {
    width: 14%;
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

  .sortable-heading {
    padding: 0;
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
    width: 100%;
  }

  .sortable-heading:first-child .sort-heading {
    padding-left: var(--space-4);
  }

  .sort-heading:hover,
  .sort-heading:focus-visible {
    background: var(--interactive-hover);
    color: var(--text-primary);
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
    margin: calc(var(--space-1) * -1) 0;
    min-width: 0;
    padding: var(--space-1) 0;
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
    transform: translateY(-2px);
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
    transform: translateY(-1px);
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
    transform: translateY(-1px);
    white-space: nowrap;
  }

  td:last-child :global(fieldset) {
    margin-left: 0;
  }

  .repository-message-row td {
    padding-block: var(--space-2);
  }

  .repository-message-row .form-error {
    margin: 0;
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

  .repository-panel :global(.pagination-bar) {
    border: 1px solid var(--border-subtle);
    border-radius: 0 0 var(--radius-surface) var(--radius-surface);
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
    .repository-row td,
    .repository-message-row,
    .repository-message-row td {
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
