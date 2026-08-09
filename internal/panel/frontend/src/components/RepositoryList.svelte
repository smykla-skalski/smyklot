<script lang="ts">
  import { untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';

  import { BOOLEAN_FIELDS } from '../lib/config';
  import { formatRelative, formatTimestamp } from '../lib/format';
  import {
    shouldClearFailureAfterAutomaticRefresh,
    shouldReloadRepositoryAfterSaveFailure,
    shouldReplaceFailureWithReadError,
  } from '../lib/repository';
  import type { RepositoryFailureSource } from '../lib/repository';
  import type {
    ConfigPatch,
    Page,
    RepositoryDetail,
    RepositoryFileFilter,
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
  import HelpTip from './HelpTip.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  type RepositoryEnablement = 'inherit' | 'enabled' | 'disabled';
  type RepositoryFailure = { message: string; source: RepositoryFailureSource };

  const REPOSITORY_ENABLEMENT_OPTIONS = [
    { value: 'inherit', label: 'Default' },
    { value: 'enabled', label: 'On', tone: 'on' },
    { value: 'disabled', label: 'Off', tone: 'off' },
  ] as const;
  const PAGE_SIZES = [10, 20, 50] as const;

  const {
    targetId,
    refreshVersion,
    fetchPage,
    onLoad,
    onUpdate,
    onChanged,
  }: {
    targetId: string;
    refreshVersion: number;
    fetchPage: (request: RepositoryPageRequest) => Promise<Page<RepositorySummary>>;
    onLoad: (repositoryId: string) => Promise<RepositoryDetail>;
    onUpdate: (repositoryId: string, input: RepositorySettingsInput) => Promise<RepositoryDetail>;
    onChanged: (detail: RepositoryDetail) => void;
  } = $props();

  let search = $state('');
  let appliedQuery = $state('');
  let sort = $state<RepositorySort>('name_asc');
  let stateFilter = $state<RepositoryStateFilter>('all');
  let fileFilter = $state<RepositoryFileFilter>('all');
  let settingFilter = $state<RepositorySettingFilter>('all');
  let limit = $state<number>(20);
  let page = $state<Page<RepositorySummary> | null>(null);
  let cursors = $state<Array<string | undefined>>([undefined]);
  let pageIndex = $state(0);
  let loading = $state(false);
  let problem = $state<string | null>(null);
  let details = $state<Record<string, RepositoryDetail>>({});
  let failures = $state<Record<string, RepositoryFailure>>({});
  let pendingEnablement = $state<Record<string, RepositoryEnablement>>({});
  const opened = new SvelteSet<string>();
  const working = new SvelteSet<string>();
  const pendingRefreshes = new SvelteSet<string>();
  let now = $state(Date.now());
  let requestSequence = 0;
  let observedRefreshVersion: number | undefined;

  const repositories = $derived(page?.items ?? []);
  const total = $derived(page?.total ?? 0);
  const itemCount = $derived(repositories.length);
  const rangeStart = $derived(total === 0 ? 0 : pageIndex * limit + 1);
  const rangeEnd = $derived(total === 0 ? 0 : rangeStart + itemCount - 1);
  const pageCount = $derived(Math.max(1, Math.ceil(total / limit)));
  const hasFilters = $derived(
    appliedQuery !== '' || stateFilter !== 'all' || fileFilter !== 'all' || settingFilter !== 'all',
  );
  const filterKey = $derived(
    [targetId, appliedQuery, sort, stateFilter, fileFilter, settingFilter, limit].join(':'),
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
      void loadPage(cursors[pageIndex], filterKey);
      refreshExpandedRepositories(version);
    });
  });

  async function resetAndLoad(key: string): Promise<void> {
    cursors = [undefined];
    pageIndex = 0;
    page = null;
    await loadPage(undefined, key);
  }

  async function loadPage(cursor: string | undefined, key: string): Promise<void> {
    const sequence = ++requestSequence;
    loading = true;
    problem = null;
    try {
      const loaded = await fetchPage({
        cursor,
        query: appliedQuery,
        sort,
        limit,
        state: stateFilter,
        file: fileFilter,
        setting: settingFilter,
      });
      if (sequence !== requestSequence || key !== filterKey) return;
      if (pageIndex > 0 && loaded.total <= pageIndex * limit) {
        await resetAndLoad(key);
        return;
      }
      page = loaded;
    } catch (error) {
      if (sequence === requestSequence && key === filterKey) {
        problem = error instanceof Error ? error.message : String(error);
      }
    } finally {
      if (sequence === requestSequence && key === filterKey) loading = false;
    }
  }

  async function nextPage(): Promise<void> {
    const cursor = page?.next_cursor;
    if (cursor === null || cursor === undefined || loading) return;
    const nextIndex = pageIndex + 1;
    cursors = [...cursors.slice(0, nextIndex), cursor];
    pageIndex = nextIndex;
    await loadPage(cursor, filterKey);
  }

  async function previousPage(): Promise<void> {
    if (pageIndex === 0 || loading) return;
    const previousIndex = pageIndex - 1;
    pageIndex = previousIndex;
    await loadPage(cursors[previousIndex], filterKey);
  }

  function retry(): void {
    void loadPage(cursors[pageIndex], filterKey);
  }

  function refreshExpandedRepositories(version: number): void {
    if (!Number.isSafeInteger(version)) return;
    untrack(() => {
      for (const repositoryId of opened) requestRefresh(repositoryId);
    });
  }

  async function toggle(repositoryId: string): Promise<void> {
    if (opened.delete(repositoryId)) return;
    opened.add(repositoryId);
    if (details[repositoryId] === undefined) await refresh(repositoryId);
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

  function overrideLabel(count: number): string {
    return `${count} ${count === 1 ? 'override' : 'overrides'}`;
  }
</script>

<section class="plate repository-panel" aria-labelledby="repositories-heading">
  <header class="repository-header">
    <div class="repository-heading">
      <h2 id="repositories-heading">Installed repositories</h2>
      <p>Choose which repositories Smyklot handles and where settings differ</p>
    </div>
    <HelpTip
      id="repository-controls-help"
      label="About repository controls"
      text="On and Off filter the effective state. Default follows Enable repositories by default in Settings. Expand a repository to configure repository-specific settings"
    />
  </header>

  <div class="repository-tools">
    <label class="search-field">
      <span class="visually-hidden">Search repositories</span>
      <span class="search-icon" aria-hidden="true"></span>
      <input
        class="text-input"
        type="search"
        placeholder="Search repositories"
        bind:value={search}
      />
    </label>

    <label class="filter-field state-field">
      <select class="select-input" bind:value={stateFilter} aria-label="Repository state">
        <option value="all">All states</option>
        <option value="enabled">On</option>
        <option value="disabled">Off</option>
      </select>
    </label>

    <label class="filter-field file-field">
      <select class="select-input" bind:value={fileFilter} aria-label="Repository file status">
        <option value="all">All files</option>
        <option value="valid">Valid</option>
        <option value="missing">Missing</option>
        <option value="invalid">Invalid</option>
        <option value="bypassed">Bypassed</option>
      </select>
    </label>

    <label class="filter-field setting-field">
      <select class="select-input" bind:value={settingFilter} aria-label="Custom setting">
        <option value="all">All settings</option>
        <option value="custom">Any custom setting</option>
        <option value="none">No custom settings</option>
        <optgroup label="Behavior">
          {#each BOOLEAN_FIELDS as field (field.key)}
            <option value={field.key}>{field.label}</option>
          {/each}
        </optgroup>
        <optgroup label="Commands">
          <option value="command_prefix">Prefix</option>
          <option value="allowed_commands">Allowed</option>
          <option value="command_aliases">Aliases</option>
        </optgroup>
      </select>
    </label>

    <label class="filter-field order-field">
      <select class="select-input" bind:value={sort} aria-label="Repository sort order">
        <option value="name_asc">Name A–Z</option>
        <option value="name_desc">Name Z–A</option>
        <option value="newest">Recently updated</option>
        <option value="oldest">Least recently updated</option>
      </select>
    </label>
  </div>

  <div class:loading class="repository-results" aria-busy={loading}>
    {#if problem !== null}
      <div class="result-state" role="alert">
        <strong>Repositories could not be loaded</strong>
        <span>{problem}</span>
        <button class="btn" onclick={retry}>Try again</button>
      </div>
    {:else if loading && page === null}
      <div class="result-state dim">Reading repositories…</div>
    {:else if repositories.length === 0}
      <div class="result-state dim">
        {hasFilters ? 'No repositories match these filters' : 'No repositories are installed'}
      </div>
    {:else}
      <ul class="repositories">
        {#each repositories as repository (repository.id)}
          {@const isOpen = opened.has(repository.id)}
          {@const detail = details[repository.id]}
          {@const repositoryFailure = failures[repository.id]}
          <li class="repository">
            <div class="repository-head">
              <button
                class="btn btn-quiet expand"
                aria-expanded={isOpen}
                aria-controls="repository-{repository.id}"
                onclick={() => toggle(repository.id)}
              >
                <span class="caret" class:caret-open={isOpen} aria-hidden="true"></span>
                <span class="repo-copy">
                  <strong>{repository.name}</strong>
                  <span class="repo-meta mono" title={formatTimestamp(repository.updated_at)}>
                    {overrideLabel(repository.config_override_count)}{repository.private
                      ? ' · private'
                      : ''} · updated {formatRelative(repository.updated_at, now)}
                  </span>
                </span>
              </button>

              <div class="repository-actions">
                {#if !repository.available}
                  <Chip small>Unavailable</Chip>
                {/if}
                <FileStatusIndicator
                  id="file-status-{repository.id}"
                  status={repository.config_file_status}
                />

                <SegmentedControl
                  name="repository-enablement-{repository.id}"
                  label="Enablement for {repository.full_name}"
                  options={REPOSITORY_ENABLEMENT_OPTIONS}
                  value={pendingEnablement[repository.id] ?? enabledValue(repository)}
                  disabled={working.has(repository.id) || !repository.available}
                  align="end"
                  onSelect={(value) => void setEnabled(repository, value)}
                />
              </div>
            </div>

            {#if repositoryFailure !== undefined}
              <p class="form-error" role="alert">{repositoryFailure.message}</p>
            {/if}

            {#if isOpen}
              <div class="repository-detail" id="repository-{repository.id}">
                {#if detail === undefined}
                  <p class="dim">Reading repository settings…</p>
                {:else}
                  <div
                    class="file-status"
                    class:file-problem={detail.config_file_error !== undefined}
                  >
                    <div class="file-copy">
                      <strong>Repository file</strong>
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
                        disabled={working.has(repository.id)}
                        onchange={(event) => setBypass(repository.id, event.currentTarget.checked)}
                      />
                      <span aria-hidden="true"></span>
                    </label>
                  </div>
                  {#if detail.ignore_repository_file}
                    <p class="warning" role="status">
                      Repository-file settings are being ignored; this exception is recorded in the
                      audit log
                    </p>
                  {/if}
                  <div class="override-heading">
                    <strong>Repository overrides</strong>
                    <HelpTip
                      id="repository-overrides-{repository.id}"
                      label="About repository overrides"
                      text="Only settings changed here override configuration defaults from Settings and repository-file settings"
                    />
                  </div>
                  <ConfigEditor
                    patch={detail.config_patch}
                    inherited={detail.inherited_config}
                    scope="repository"
                    idPrefix={repository.id}
                    disabled={working.has(repository.id)}
                    onSave={(patch) => setConfig(repository.id, patch)}
                  />
                {/if}
              </div>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  {#if problem === null && page !== null}
    <footer class="pagination" aria-label="Repository pagination">
      <p class="range mono" aria-live="polite">
        <strong>{rangeStart}–{rangeEnd}</strong>
        of {total}
      </p>

      <div class="page-actions">
        <button
          class="btn page-button"
          disabled={pageIndex === 0 || loading}
          onclick={previousPage}
        >
          <span aria-hidden="true">←</span>
          Previous
        </button>
        <span class="page-number mono">Page {pageIndex + 1} of {pageCount}</span>
        <button
          class="btn page-button"
          disabled={page?.next_cursor === null || loading}
          onclick={nextPage}
        >
          Next
          <span aria-hidden="true">→</span>
        </button>
      </div>

      <label class="rows-field">
        <span>Rows</span>
        <select class="select-input" bind:value={limit} aria-label="Repositories per page">
          {#each PAGE_SIZES as size (size)}
            <option value={size}>{size}</option>
          {/each}
        </select>
      </label>
    </footer>
  {/if}
</section>

<style>
  .repository-panel {
    --repository-control-height: 30px;

    overflow: hidden;
  }

  .repository-header {
    align-items: center;
    border-bottom: 1px solid var(--rule);
    display: flex;
    gap: 1rem;
    justify-content: space-between;
    min-height: 4rem;
    padding: 0.625rem 1.125rem;
  }

  .repository-heading {
    min-width: 0;
  }

  .repository-heading h2 {
    font-size: 0.9375rem;
    line-height: 1.2;
    margin: 0;
  }

  .repository-heading p {
    color: var(--dim);
    font-size: 0.75rem;
    line-height: 1.35;
    margin: 0.15rem 0 0;
  }

  .repository-tools {
    align-items: center;
    background: var(--well);
    border-bottom: 1px solid var(--rule);
    display: grid;
    gap: 0.5rem;
    grid-template-columns: minmax(12rem, 1fr) 7rem 7.5rem 11.5rem 10.5rem;
    padding: 0.625rem 1.125rem;
  }

  .search-field,
  .filter-field {
    min-width: 0;
  }

  .search-field {
    align-items: center;
    display: flex;
    position: relative;
  }

  .search-field input,
  .filter-field select {
    font-size: 0.6875rem;
    height: var(--repository-control-height);
    width: 100%;
  }

  .search-field input {
    padding-left: 2rem;
  }

  .search-icon {
    border: 1.5px solid var(--dim);
    border-radius: 50%;
    height: 0.65rem;
    left: 0.75rem;
    opacity: 0.8;
    pointer-events: none;
    position: absolute;
    width: 0.65rem;
  }

  .search-icon::after {
    background: var(--dim);
    content: '';
    height: 1.5px;
    position: absolute;
    right: -0.3rem;
    top: 0.5rem;
    transform: rotate(45deg);
    width: 0.35rem;
  }

  .repository-results {
    min-height: 5rem;
    transition: opacity 120ms ease-out;
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
    font-size: 0.75rem;
  }

  .repositories {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .repository {
    padding: 0.75rem 1.125rem;
  }

  .repository + .repository {
    border-top: 1px solid var(--rule);
  }

  .repository-head {
    align-items: center;
    border-radius: var(--r-ctl);
    display: grid;
    gap: 0.5rem;
    grid-template-columns: minmax(0, 1fr) auto;
    margin: -5px;
    padding: 5px 5px 5px 0;
    transition: background-color 120ms ease-out;
  }

  .repository-head:hover,
  .repository-head:focus-within {
    background: var(--strip-lift);
  }

  .expand {
    background: transparent;
    display: grid;
    gap: 0;
    grid-template-columns: 1.5rem minmax(0, 1fr);
    min-width: 0;
    padding: 0;
    text-align: left;
  }

  .expand:hover:not(:disabled) {
    background: transparent;
  }

  .caret {
    border-bottom: 3.5px solid transparent;
    border-left: 4px solid currentcolor;
    border-top: 3.5px solid transparent;
    height: 0;
    justify-self: center;
    transition: transform 120ms ease-out;
    width: 0;
  }

  .caret-open {
    transform: rotate(90deg);
  }

  .repo-copy {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    min-width: 0;
  }

  .repo-copy strong,
  .repo-copy span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .repo-meta {
    color: var(--dim);
    font-size: 0.6875rem;
    opacity: 0.72;
  }

  .pagination {
    align-items: center;
    background: var(--well);
    border-top: 1px solid var(--rule);
    display: grid;
    gap: 0.75rem;
    grid-template-columns: 1fr auto 1fr;
    min-height: 3.75rem;
    padding: 0.625rem 1.125rem;
  }

  .range,
  .page-number {
    color: var(--dim);
    font-size: 0.625rem;
    margin: 0;
    white-space: nowrap;
  }

  .range strong {
    color: var(--text);
    font-weight: 600;
  }

  .page-actions {
    align-items: center;
    display: flex;
    gap: 0.5rem;
  }

  .page-button {
    background: var(--strip-lift);
    font-size: 0.75rem;
    padding: 0 0.625rem;
  }

  .rows-field {
    align-items: center;
    display: flex;
    gap: 0.5rem;
    justify-self: end;
  }

  .rows-field > span {
    color: var(--dim);
    font: 600 0.5625rem/1 var(--mono);
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .rows-field select {
    font-size: 0.75rem;
    width: 4.25rem;
  }

  .repository-actions {
    align-items: center;
    display: flex;
    gap: 0.25rem;
    justify-content: flex-end;
  }

  .repository-detail {
    background: var(--well);
    border: 1px solid var(--rule);
    border-radius: var(--r-well);
    margin-top: 0.75rem;
    padding: 1rem;
  }

  .file-status {
    align-items: center;
    display: flex;
    gap: 1rem;
    justify-content: space-between;
    margin-bottom: 0.75rem;
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
    border-top: 1px solid var(--rule);
    display: flex;
    justify-content: space-between;
    margin-top: 0.875rem;
    padding-top: 0.5rem;
  }

  .override-heading > strong {
    font-size: 0.8125rem;
  }

  .file-problem strong,
  .form-error {
    color: var(--stop);
  }

  .warning {
    background: var(--warning-tint);
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

  @media (max-width: 54rem) {
    .repository-tools {
      grid-template-columns: 1fr 1fr;
    }

    .search-field {
      grid-column: 1 / -1;
    }
  }

  @media (max-width: 36rem) {
    .pagination {
      grid-template-columns: 1fr auto;
    }

    .page-actions {
      grid-column: 1 / -1;
      grid-row: 1;
      justify-content: space-between;
    }

    .range {
      grid-column: 1;
      grid-row: 2;
    }

    .rows-field {
      grid-column: 2;
      grid-row: 2;
    }
  }

  @media (max-width: 30rem) {
    .repository-head {
      gap: 0.25rem;
    }

    .repository-tools {
      grid-template-columns: 1fr;
    }

    .search-field {
      grid-column: 1;
    }

    .repository-header {
      align-items: flex-start;
    }
  }

  @media (max-width: 38rem) {
    .file-status {
      align-items: flex-start;
      flex-direction: column;
    }
  }
</style>
