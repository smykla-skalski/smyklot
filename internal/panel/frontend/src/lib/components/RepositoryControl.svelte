<script lang="ts">
  import type { RepositoryDetail, RepositorySummary } from '#lib/types.js';

  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import CodeEditor from './CodeEditor.svelte';
  import FormError from './FormError.svelte';
  import IconButton from './IconButton.svelte';
  import Link from './Link.svelte';
  import Modal from './Modal.svelte';
  import Pill, { type PillTone } from './Pill.svelte';
  import RelativeTime from './RelativeTime.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import Switch from './Switch.svelte';

  const {
    repository,
    detail,
    enablement = 'inherit',
    readOnly = false,
    busy = false,
    dirtyEnabled = false,
    dirtyUseFile = false,
    now = 0,
    onEnablement,
    onUseFile,
    onResetMigration,
  }: {
    repository: RepositorySummary;
    detail: RepositoryDetail;
    enablement?: 'inherit' | 'enabled' | 'disabled';
    readOnly?: boolean;
    busy?: boolean;
    dirtyEnabled?: boolean;
    dirtyUseFile?: boolean;
    now?: number;
    onEnablement: (value: string) => void;
    onUseFile: (enabled: boolean) => void;
    onResetMigration: () => void;
  } = $props();

  const OBSERVATION = {
    unknown: { label: 'Not checked', tone: 'neutral' },
    missing: { label: 'Missing', tone: 'neutral' },
    valid: { label: 'Valid', tone: 'success' },
    invalid: { label: 'Invalid', tone: 'danger' },
  } as const satisfies Record<string, { label: string; tone: PillTone }>;

  const status = $derived(detail.config_file_observation?.status ?? 'unknown');
  const observation = $derived(OBSERVATION[status]);
  const path = $derived(detail.config_file_path?.trim() || null);
  const searchPaths = $derived([...new Set(detail.config_file_observation?.search_paths ?? [])]);
  const superseded = $derived([...new Set(detail.config_file_superseded ?? [])]);
  const hasObservedFile = $derived(status === 'valid' || status === 'invalid');
  const hasSelectedFile = $derived(hasObservedFile && path !== null);
  const otherDetected = $derived(
    hasObservedFile ? superseded.filter((candidate) => !searchPaths.includes(candidate)) : [],
  );
  const observedAt = $derived(detail.config_file_observation?.observed_at);
  const enablementWhy = $derived(
    enablement === 'inherit'
      ? 'Follow the workspace setting'
      : enablement === 'enabled'
        ? 'Run Smyklot in this repository'
        : 'Stop Smyklot in this repository',
  );
  const fileWhy = $derived(
    status === 'missing'
      ? 'No configuration file found'
      : (path ?? (status === 'unknown' ? 'No recorded file check' : 'No selected path reported')),
  );
  const noSelectedFileWhy = $derived(
    status === 'missing'
      ? 'No configuration file found'
      : status === 'unknown'
        ? 'No recorded file check'
        : 'No selected path reported',
  );
  const useFileWhy = $derived(
    detail.ignore_repository_file
      ? 'Use workspace defaults and panel overrides without the file settings'
      : status === 'invalid'
        ? enablement === 'disabled'
          ? 'Fix the file or turn file settings off before enabling Smyklot'
          : 'Commands are blocked until the file is fixed or file settings are turned off'
        : status === 'missing'
          ? 'Use a configuration file when one is found'
          : status === 'unknown'
            ? 'Use the file after Smyklot checks it'
            : 'Apply file settings, then this repository’s panel overrides',
  );
  const repoHref = $derived(githubRepository(repository.full_name));
  const fileHref = $derived(
    repoHref === null || path === null
      ? null
      : githubFile(repoHref, repository.default_branch, path),
  );
  const migrationHref = $derived(
    repoHref !== null &&
      Number.isSafeInteger(detail.config_migration_pr) &&
      (detail.config_migration_pr ?? 0) > 0
      ? `${repoHref}/pull/${detail.config_migration_pr}`
      : null,
  );
  const parsedSettings = $derived(JSON.stringify(detail.config_file_patch, null, 2));

  let inspectedRepository = $state<string | null>(null);
  let inspectTrigger = $state<HTMLElement | null>(null);
  const inspectorOpen = $derived(inspectedRepository === repository.id);

  // Changing repositories invalidates the open inspection, including when returning
  // to the previous repository without this component being remounted.
  $effect(() => {
    if (inspectedRepository !== null && inspectedRepository !== repository.id) {
      inspectedRepository = null;
      inspectTrigger = null;
    }
  });

  function githubRepository(fullName: string): string | null {
    const parts = fullName.split('/');
    if (
      parts.length !== 2 ||
      parts.some((part) => !/^[\w.-]+$/u.test(part) || part === '.' || part === '..')
    )
      return null;
    return `https://github.com/${parts.map(encodeURIComponent).join('/')}`;
  }

  function githubFile(repositoryUrl: string, branch: string, filePath: string): string | null {
    const parts = filePath.split('/');
    if (
      branch === '' ||
      branch === '.' ||
      branch === '..' ||
      hasControlCharacter(branch) ||
      hasControlCharacter(filePath) ||
      filePath.includes('\\') ||
      parts.some((part) => part === '' || part === '.' || part === '..')
    )
      return null;
    return `${repositoryUrl}/blob/${encodeURIComponent(branch)}/${parts.map(encodeURIComponent).join('/')}`;
  }

  function hasControlCharacter(value: string): boolean {
    return [...value].some((character) => {
      const code = character.charCodeAt(0);
      return code < 32 || code === 127;
    });
  }

  function closeInspector(): void {
    const trigger = inspectTrigger;
    inspectedRepository = null;
    inspectTrigger = null;
    queueMicrotask(() => {
      if (trigger?.isConnected) trigger.focus({ preventScroll: true });
    });
  }
</script>

<!--
@component
Repository enablement and file use are editable policy. File validity is an observation
from the service, independent of both policy choices. The inspector shows only reported
paths and parsed values; it never claims to fetch or edit the original file.
-->

<Card label="Repository configuration" unsaved={dirtyEnabled || dirtyUseFile}>
  <div class="card-head"><h2 class="card-title">Repository configuration</h2></div>
  <div class="policy-rows">
    <div
      class={['policy-row', { 'is-unsaved': dirtyEnabled }]}
      data-unsaved={dirtyEnabled || undefined}
    >
      <span class="setting-say">
        <span class="setting-name">Smyklot</span>
        <span class="setting-why">{enablementWhy}</span>
      </span>
      <span class="policy-value">
        <SegmentedControl
          name={`repository-enabled-${repository.id}`}
          label={`Smyklot in ${repository.name}`}
          options={[
            { value: 'inherit', label: 'Workspace' },
            { value: 'enabled', label: 'On' },
            { value: 'disabled', label: 'Off' },
          ]}
          value={enablement}
          compact
          disabled={readOnly}
          onSelect={onEnablement}
        />
      </span>
    </div>
    <div class="policy-row">
      <span class="setting-say">
        <span class="setting-name">Configuration file</span>
        <span class={['setting-why', { 'file-path': path !== null && status !== 'missing' }]}
          >{fileWhy}</span
        >
      </span>
      <span class="policy-value">
        <Pill tone={observation.tone}>{observation.label}</Pill>
        <Button
          tone="quiet"
          aria-haspopup="dialog"
          aria-expanded={inspectorOpen}
          aria-controls={`repository-config-file-${repository.id}`}
          onclick={(event) => {
            inspectTrigger = event.currentTarget;
            inspectedRepository = repository.id;
          }}>Inspect file</Button
        >
      </span>
    </div>
    <div
      class={['policy-row', { 'is-unsaved': dirtyUseFile }]}
      data-unsaved={dirtyUseFile || undefined}
    >
      <span class="setting-say">
        <span class="setting-name">Use file settings</span>
        <span class="setting-why">{useFileWhy}</span>
      </span>
      <span class="policy-value">
        <Switch
          checked={!detail.ignore_repository_file}
          label="Use file settings"
          disabled={readOnly}
          onToggle={onUseFile}
        />
      </span>
    </div>
  </div>
</Card>

{#if inspectorOpen}
  <Modal
    id={`repository-config-file-${repository.id}`}
    open
    title="Configuration file"
    description={repository.full_name}
    variant="inspector"
    returnFocus={inspectTrigger}
    onClose={closeInspector}
  >
    {#snippet headerExtra()}
      <IconButton toolbar icon="close" label="Close file inspector" onclick={closeInspector} />
    {/snippet}
    <div class="card-stack">
      <Card label="Selected configuration file">
        <div class="card-head">
          <h3 class="card-title">{hasSelectedFile ? 'Selected file' : 'File status'}</h3>
          <Pill tone={observation.tone}>{observation.label}</Pill>
        </div>
        <div class="policy-rows">
          <div class="policy-row">
            {#if hasSelectedFile}
              <span class="setting-say">
                <span class="setting-name">Path</span>
                <span class="setting-why file-path">{path}</span>
              </span>
              {#if fileHref !== null}
                <span class="policy-value"
                  ><Button href={fileHref} target="_blank" rel="noreferrer">Open on GitHub</Button
                  ></span
                >
              {/if}
            {:else}
              <span class="setting-name">{noSelectedFileWhy}</span>
            {/if}
          </div>
          {#if observedAt !== undefined && Number.isFinite(Date.parse(observedAt))}
            <div class="policy-row">
              <span class="setting-name">Last checked</span>
              <span class="policy-value"
                ><RelativeTime class="setting-fact" value={observedAt} nowMs={now} /></span
              >
            </div>
          {/if}
        </div>
        {#if status === 'invalid'}
          <FormError
            message={detail.config_file_error || 'The configuration file could not be read'}
          />
        {/if}
      </Card>

      <Card label="Configuration file search order">
        <div class="card-head"><h3 class="card-title">File search order</h3></div>
        <p class="group-note">The first file found is used</p>
        {#if searchPaths.length > 0}
          <ol class="policy-rows search-order">
            {#each searchPaths as candidate, index (candidate)}
              <li class="policy-row search-path-row">
                <span class="search-priority setting-fact mono" aria-hidden="true">{index + 1}</span
                >
                <span class="file-path search-path">{candidate}</span>
                {#if hasObservedFile && candidate === path}<Pill tone="info">Selected</Pill>
                {:else if hasObservedFile && superseded.includes(candidate)}<Pill tone="neutral"
                    >Ignored</Pill
                  >{/if}
              </li>
            {/each}
          </ol>
        {:else}
          <p class="form-note">The service has not reported its search paths</p>
        {/if}
        {#if otherDetected.length > 0}
          <p class="form-note">
            Also detected: {otherDetected.join(', ')} · These files are not used
          </p>
        {/if}
      </Card>

      {#if status === 'valid'}
        <Card label="Parsed file settings">
          <div class="card-head"><h3 class="card-title">Parsed file settings</h3></div>
          <p class="group-note">
            Values read from the file · Original syntax and comments are not shown
          </p>
          <CodeEditor
            value={parsedSettings}
            lang="json"
            readOnly
            label="Parsed file settings"
            onChange={() => {}}
          />
        </Card>
      {/if}

      {#if detail.config_migration !== 'none'}
        <Card label="TOML migration">
          <div class="card-head"><h3 class="card-title">TOML migration</h3></div>
          <div class="policy-rows">
            <div class="policy-row">
              <span class="setting-say">
                <span class="setting-name"
                  >{detail.config_migration === 'proposed'
                    ? 'Proposal open'
                    : detail.config_migration === 'declined'
                      ? 'Proposal closed'
                      : 'Proposal blocked'}</span
                >
                <span class="setting-why"
                  >{detail.config_migration === 'proposed'
                    ? 'Review the proposed conversion to TOML'
                    : 'Retry to let Smyklot propose the conversion again'}</span
                >
              </span>
              <span class="policy-value">
                {#if detail.config_migration === 'proposed'}
                  {#if migrationHref !== null}<Link
                      href={migrationHref}
                      target="_blank"
                      rel="noreferrer">Pull request #{detail.config_migration_pr}</Link
                    >{/if}
                {:else}
                  <Button disabled={readOnly || busy} onclick={onResetMigration}
                    >{busy ? 'Requesting proposal…' : 'Retry proposal'}</Button
                  >
                {/if}
              </span>
            </div>
          </div>
        </Card>
      {/if}
    </div>
    {#snippet footer()}<Button onclick={closeInspector}>Done</Button>{/snippet}
  </Modal>
{/if}

<style>
  .search-order {
    column-gap: var(--space-2);
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr) auto;
    list-style: none;
    margin-block-start: 0;
    padding-inline-start: 0;
  }

  /* This is an ordered read-only list, so a status stays beside its path instead
     of borrowing an editable setting's minimum-width sentence and control columns. */
  .search-path-row {
    column-gap: inherit;
    container-type: normal;
    display: grid;
    grid-column: 1 / -1;
    grid-template-columns: subgrid;
  }

  .search-priority {
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
    text-align: start;
  }

  .search-path {
    line-height: var(--row-copy-leading);
    min-inline-size: 0;
    overflow-wrap: anywhere;
  }
</style>
