<script lang="ts">
  import { untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';

  import { formatRelative, formatTimestamp } from '../lib/format';
  import {
    shouldClearFailureAfterAutomaticRefresh,
    shouldReloadRepositoryAfterSaveFailure,
    shouldReplaceFailureWithReadError,
  } from '../lib/repository';
  import type { RepositoryFailureSource } from '../lib/repository';
  import type {
    ConfigPatch,
    RepositoryDetail,
    RepositorySettingsInput,
    RepositorySummary,
  } from '../lib/types';
  import Chip from './Chip.svelte';
  import ConfigEditor from './ConfigEditor.svelte';
  import FileStatusIndicator from './FileStatusIndicator.svelte';
  import HelpTip from './HelpTip.svelte';
  import Plate from './Plate.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  type RepositoryEnablement = 'inherit' | 'enabled' | 'disabled';
  type RepositoryFailure = { message: string; source: RepositoryFailureSource };

  const REPOSITORY_ENABLEMENT_OPTIONS = [
    { value: 'inherit', label: 'Default' },
    { value: 'enabled', label: 'On', tone: 'on' },
    { value: 'disabled', label: 'Off', tone: 'off' },
  ] as const;

  const {
    repositories,
    refreshVersion,
    onLoad,
    onUpdate,
    onChanged,
  }: {
    repositories: RepositorySummary[];
    refreshVersion: number;
    onLoad: (repositoryId: string) => Promise<RepositoryDetail>;
    onUpdate: (repositoryId: string, input: RepositorySettingsInput) => Promise<RepositoryDetail>;
    onChanged: (detail: RepositoryDetail) => void;
  } = $props();

  let query = $state('');
  let details = $state<Record<string, RepositoryDetail>>({});
  let failures = $state<Record<string, RepositoryFailure>>({});
  let pendingEnablement = $state<Record<string, RepositoryEnablement>>({});
  const opened = new SvelteSet<string>();
  const working = new SvelteSet<string>();
  const pendingRefreshes = new SvelteSet<string>();
  let now = $state(Date.now());

  const visible = $derived.by(() => {
    const needle = query.trim().toLocaleLowerCase();
    if (needle === '') return repositories;
    return repositories.filter((repository) =>
      repository.full_name.toLocaleLowerCase().includes(needle),
    );
  });

  $effect(() => {
    const tick = setInterval(() => {
      now = Date.now();
    }, 30_000);
    return () => clearInterval(tick);
  });

  $effect(() => {
    refreshExpandedRepositories(refreshVersion);
  });

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

<Plate label="Installed repositories">
  {#snippet status()}
    <div class="repository-header-status">
      <span class="tally mono">{visible.length} of {repositories.length}</span>
      <HelpTip
        id="repository-controls-help"
        label="About repository controls"
        text="Default follows Enable repositories by default in Settings. On and Off override it. Expand a repository to configure repository-specific settings"
      />
    </div>
  {/snippet}

  <label class="search">
    <span class="visually-hidden">Filter repositories</span>
    <input class="text-input" type="search" placeholder="Filter repositories" bind:value={query} />
  </label>

  {#if visible.length === 0}
    <p class="dim">No repositories match this filter</p>
  {:else}
    <ul class="repositories">
      {#each visible as repository (repository.id)}
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
</Plate>

<style>
  .tally,
  .repo-meta {
    color: var(--dim);
    font-size: 0.6875rem;
  }

  .repository-header-status {
    align-items: center;
    display: flex;
    gap: 0.25rem;
  }

  .search {
    display: block;
    margin-bottom: 0.75rem;
  }

  .search input {
    width: 100%;
  }

  .repositories {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .repository {
    border-top: 1px solid var(--rule);
    padding: 0.75rem 0;
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

  @media (max-width: 30rem) {
    .repository-head {
      gap: 0.25rem;
    }
  }

  @media (max-width: 38rem) {
    .file-status {
      align-items: flex-start;
      flex-direction: column;
    }
  }
</style>
