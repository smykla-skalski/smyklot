<script lang="ts">
  import type { BypassActorLookup } from '../types';
  import BypassPolicyEditor from './BypassPolicyEditor.svelte';
  import { CONFIG_KEYS } from '../config';
  import { durationParts, formatDuration, type DurationUnit } from '../duration';
  import {
    FORMATTING_FIELDS,
    formattingOverrideCount,
    type FormattingFieldKey,
    type FormattingPatch,
  } from '../formatting';
  import {
    buildRepositorySettingsDocument,
    type RepositorySettingsControlId,
    type RepositorySettingsDocument,
  } from '../repository-settings';
  import type {
    SyncOverrideControlId,
    SyncOverrideEditorEnvelope,
  } from '../repository-sync-override-settings';
  import type {
    ConfigKey,
    ConfigPatch,
    PendingCIMode,
    RepositoryDetail,
    RepositorySummary,
    SyncOverride,
    SyncStatus,
  } from '../types';
  import { repositorySentence } from '../repository-sentence';
  import ClippedLabel from './ClippedLabel.svelte';
  import ConfigEditor from './ConfigEditor.svelte';
  import FormattingEditor from './FormattingEditor.svelte';
  import DisclosureSection from './DisclosureSection.svelte';
  import Icon from './Icon.svelte';
  import DurationInput from './DurationInput.svelte';
  import Card from './Card.svelte';
  import PageHeader from './PageHeader.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import Popover from './Popover.svelte';
  import PickerTrigger from './PickerTrigger.svelte';
  import RepositoryControl from './RepositoryControl.svelte';
  import RepositorySyncPane from './RepositorySyncPane.svelte';

  const GATE_PILLS = {
    ready: 'pill-success',
    provisioning: 'pill-muted',
    draining: 'pill-warning',
    blocked: 'pill-danger',
  } as const;
  const PENDING_CI_CHOICES = [
    { value: 'checks', label: 'Checks' },
    { value: 'labels', label: 'Labels' },
  ] as const;

  const {
    repository,
    detail,
    savedFormatting,
    failure = null,
    readOnly = false,
    organizationActors = true,
    busy = false,
    backHref,
    onBack,
    onChange,
    onResetMigration,
    enablement = 'inherit',
    onEnablement = () => {},
    offersSync = true,
    fleet = null,
    syncOverride = undefined,
    syncEnvelope = undefined,
    revealSyncFile = null,
    syncReadProblem = null,
    now = 0,
    onChangeSync = () => {},
    onFormattingValidity = () => {},
    onDurationValidity = () => {},
    dirtyControls = [],
    lookupBypassActors,
  }: {
    repository: RepositorySummary;
    lookupBypassActors?: BypassActorLookup;
    detail: RepositoryDetail | undefined;
    savedFormatting?: FormattingPatch;
    failure?: string | null;
    readOnly?: boolean;
    organizationActors?: boolean;
    busy?: boolean;
    backHref: string;
    onBack: () => void;
    onChange: (
      next: RepositorySettingsDocument,
      controls: readonly RepositorySettingsControlId[],
    ) => void;
    onResetMigration: () => void;
    /** Whether Smyklot answers here: on, off, or whatever the workspace says. */
    enablement?: 'inherit' | 'enabled' | 'disabled';
    onEnablement?: (next: string) => void;
    /**
     * Whether this surface draws the File sync card.
     *
     * Handed in rather than worked out here, because it is a fact about where this is
     * being drawn. The Root view of somebody else's workspace has none: sync is
     * configured on the workspace's own page and has no Root address, so a card
     * offering to edit it there would be one whose every save is a 404.
     */
    offersSync?: boolean;
    /**
     * The fleet, for the sync half of the sentence under the name. Null until
     * the read comes back, and on the surface that has no sync to read.
     */
    fleet?: SyncStatus | null;
    /** Undefined until the read comes back. */
    syncOverride?: SyncOverride | undefined;
    syncEnvelope?: SyncOverrideEditorEnvelope | undefined;
    revealSyncFile?: string | null;
    syncReadProblem?: string | null;
    /** The clock the pane's relative times are read against. */
    now?: number;
    onChangeSync?: (next: SyncOverrideEditorEnvelope, control: SyncOverrideControlId) => void;
    onFormattingValidity?: (valid: boolean) => void;
    onDurationValidity?: (control: RepositorySettingsControlId, problem: string | null) => void;
    dirtyControls?: readonly string[];
  } = $props();

  const disabled = $derived(readOnly);
  const dirtyControlSet = $derived(new Set(dirtyControls));
  const dirtyConfigKeys = $derived(CONFIG_KEYS.filter((key) => controlDirty(configControl(key))));
  const dirtyFormattingKeys = $derived(
    FORMATTING_FIELDS.filter((field) => controlDirty(controlId(`config_patch.${field.key}`))).map(
      (field) => field.key,
    ),
  );
  const titleId = 'repository-page-title';

  function controlId(suffix: string): RepositorySettingsControlId {
    return `repositories.${repository.id}.${suffix}` as RepositorySettingsControlId;
  }

  function configControl(key: ConfigKey): RepositorySettingsControlId {
    return controlId(`config_patch.${key}`);
  }

  function controlDirty(control: string): boolean {
    return dirtyControlSet.has(control);
  }

  function stage(
    next: RepositorySettingsDocument,
    ...controls: RepositorySettingsControlId[]
  ): void {
    onChange(next, controls);
  }

  function currentDocument(): RepositorySettingsDocument | null {
    return detail === undefined ? null : buildRepositorySettingsDocument(detail);
  }

  /* ---------- Merge after CI, staged change by change ---------- */

  function overrideMode(): void {
    const document = currentDocument();
    if (detail === undefined || document === null) return;
    stage(
      { ...document, pending_ci_mode_override: detail.pending_ci_mode_inherited },
      controlId('pending_ci_mode_override'),
    );
  }

  function overridePatterns(): void {
    const document = currentDocument();
    if (detail === undefined || document === null) return;
    /* A JSON clone, not structuredClone: the detail arrives as a $state proxy,
       which structuredClone refuses to clone. */
    const patterns = {
      include: [...detail.pending_ci_branch_patterns_inherited.include],
      exclude: [...detail.pending_ci_branch_patterns_inherited.exclude],
    };
    stage(
      { ...document, pending_ci_branch_patterns_override: patterns },
      controlId('pending_ci_branch_patterns_override.include'),
      controlId('pending_ci_branch_patterns_override.exclude'),
    );
  }

  function setIncludes(next: string[]): void {
    const document = currentDocument();
    if (
      detail === undefined ||
      document === null ||
      detail.pending_ci_branch_patterns_override === null
    )
      return;
    if (next.length === 0) return;
    stage(
      {
        ...document,
        pending_ci_branch_patterns_override: {
          include: next,
          exclude: detail.pending_ci_branch_patterns_override.exclude,
        },
      },
      controlId('pending_ci_branch_patterns_override.include'),
    );
  }

  function setExcludes(next: string[]): void {
    const document = currentDocument();
    if (
      detail === undefined ||
      document === null ||
      detail.pending_ci_branch_patterns_override === null
    )
      return;
    stage(
      {
        ...document,
        pending_ci_branch_patterns_override: {
          include: detail.pending_ci_branch_patterns_override.include,
          exclude: next,
        },
      },
      controlId('pending_ci_branch_patterns_override.exclude'),
    );
  }

  function setPatterns(
    patterns: RepositorySettingsDocument['pending_ci_branch_patterns_override'],
  ): void {
    const document = currentDocument();
    if (document === null) return;
    stage(
      { ...document, pending_ci_branch_patterns_override: patterns },
      controlId('pending_ci_branch_patterns_override.include'),
      controlId('pending_ci_branch_patterns_override.exclude'),
    );
  }

  function setMode(mode: PendingCIMode | null): void {
    const document = currentDocument();
    if (document === null) return;
    stage({ ...document, pending_ci_mode_override: mode }, controlId('pending_ci_mode_override'));
  }

  const PATH_INDEX_UNITS: readonly DurationUnit[] = ['seconds', 'minutes', 'hours', 'days'];

  function setQuiet(seconds: number | null): void {
    const document = currentDocument();
    if (document === null) return;
    stage(
      { ...document, pending_ci_quiet_period_seconds_override: seconds },
      controlId('pending_ci_quiet_period_seconds_override'),
    );
  }

  function setPathIndex(seconds: number | null): void {
    const document = currentDocument();
    if (document === null) return;
    stage(
      { ...document, path_index_interval_seconds_override: seconds },
      controlId('path_index_interval_seconds_override'),
    );
  }

  function setBypass(bypass: boolean): void {
    const document = currentDocument();
    if (document === null) return;
    stage({ ...document, ignore_repository_file: bypass }, controlId('ignore_repository_file'));
  }

  function setConfig(patch: ConfigPatch, key: ConfigKey): void {
    const document = currentDocument();
    if (document === null) return;
    stage({ ...document, config_patch: patch }, configControl(key));
  }

  function setFormatting(formatting: FormattingPatch, key: FormattingFieldKey): void {
    const document = currentDocument();
    if (document === null) return;
    const configPatch = { ...document.config_patch };
    if (formattingOverrideCount(formatting) === 0) delete configPatch.formatting;
    else configPatch.formatting = formatting;
    stage({ ...document, config_patch: configPatch }, controlId(`config_patch.${key}`));
  }

  function capitalize(value: string): string {
    return value.slice(0, 1).toUpperCase() + value.slice(1);
  }
</script>

<!--
@component
One repository's own page.

This was a dialog until it was three panes with a save bar in each, which is
a screen someone works in rather than something standing over the list for a
moment. It reads as any other object page of the panel does - a way back, a
mono title with the switch beside it, then the pane - and it is addressable,
so a link points at the pane a colleague was asked to look at.
-->

<div class="view-frame">
  <section class="repository-page" aria-labelledby={titleId}>
    <PageHeader
      ancestors={[{ label: 'Repositories', href: backHref, onSelect: onBack }]}
      id={titleId}
      section="Repository"
      title={repository.name}
      mono
      description={repositorySentence(repository, repository.effective_enabled, fleet, true)}
    />

    {#if failure !== null}
      <p class="form-error repository-page-error" role="alert">{failure}</p>
    {/if}

    {#if detail === undefined}
      <p class="detail-loading" role="status">Reading repository settings…</p>
    {:else}
      <RepositoryControl
        {repository}
        {detail}
        {enablement}
        {readOnly}
        {busy}
        {now}
        dirtyEnabled={controlDirty(controlId('enabled_override'))}
        dirtyUseFile={controlDirty(controlId('ignore_repository_file'))}
        {onEnablement}
        onUseFile={(use) => setBypass(!use)}
        {onResetMigration}
      />
      {#if syncOverride?.problem || syncReadProblem}{@render syncCard()}{/if}

      <Card labelledby="repository-merge-ci">
        <div class="card-head">
          <h2 class="card-title" id="repository-merge-ci">Merging</h2>
          {#if detail.pending_ci_gate !== undefined}
            <span class="pill {GATE_PILLS[detail.pending_ci_gate.readiness]}"
              ><span class="t">{capitalize(detail.pending_ci_gate.readiness)}</span></span
            >
          {/if}
        </div>
        <div class="policy-rows">
          <div
            class={[
              'policy-row',
              { 'is-unsaved': controlDirty(controlId('pending_ci_mode_override')) },
            ]}
            data-unsaved={controlDirty(controlId('pending_ci_mode_override')) || undefined}
          >
            <span class="setting-say">
              <span class="setting-name">Repository protection</span>
              <span class="setting-why"
                >Checks mode creates an app-bound required check and merges the exact authorized
                head</span
              >
            </span>
            {#if detail.pending_ci_mode_override === null}
              <span class="policy-value">
                <span class="setting-unmanaged"
                  >From the workspace: {detail.pending_ci_mode_inherited}</span
                >
              </span>
              <button
                class="setting-clear"
                title="Override the workspace mode"
                {disabled}
                onclick={overrideMode}
              >
                <Icon name="plus" size="micro" />
              </button>
            {:else}
              <span class="policy-value">
                <Popover
                  role="listbox"
                  label="Repository protection choices"
                  align="end"
                  itemSelector=".menu-item"
                >
                  {#snippet trigger(attributes)}
                    <PickerTrigger
                      {...attributes}
                      type="button"
                      aria-label="{detail.pending_ci_mode_override === 'checks'
                        ? 'Checks'
                        : 'Labels'} - repository protection"
                      {disabled}
                    >
                      {detail.pending_ci_mode_override === 'checks' ? 'Checks' : 'Labels'}
                    </PickerTrigger>
                  {/snippet}
                  <div class="menu-list">
                    {#each PENDING_CI_CHOICES as option (option.value)}
                      <button
                        class="menu-item"
                        role="option"
                        aria-selected={detail.pending_ci_mode_override === option.value}
                        onclick={() => setMode(option.value)}
                      >
                        <span class="menu-check">
                          {#if detail.pending_ci_mode_override === option.value}<Icon
                              name="check"
                              size="base"
                            />{/if}
                        </span>
                        <ClippedLabel class="mi-label" text={option.label} />
                      </button>
                    {/each}
                  </div>
                </Popover>
              </span>
              <button
                class="setting-clear"
                title="Stop overriding - follow workspace settings"
                {disabled}
                onclick={() => setMode(null)}
              >
                <Icon name="close" size="micro" />
              </button>
            {/if}
          </div>
          <div
            class={[
              'policy-row',
              {
                'is-unsaved': controlDirty(
                  controlId('pending_ci_branch_patterns_override.include'),
                ),
              },
            ]}
            data-unsaved={controlDirty(controlId('pending_ci_branch_patterns_override.include')) ||
              undefined}
          >
            <span class="setting-say">
              <span class="setting-name">Protected refs</span>
              <span class="setting-why"
                >Raw GitHub ruleset patterns, such as <code>~DEFAULT_BRANCH</code></span
              >
            </span>
            {#if detail.pending_ci_branch_patterns_override === null}
              <span class="policy-value">
                <span class="setting-unmanaged"
                  >From the workspace: {detail.pending_ci_branch_patterns_inherited.include.join(
                    ', ',
                  )}</span
                >
              </span>
              <button
                class="setting-clear"
                title="Override the protected branch patterns"
                {disabled}
                onclick={overridePatterns}
              >
                <Icon name="plus" size="micro" />
              </button>
            {:else}
              <span class="policy-value"></span>
              <button
                class="setting-clear"
                title="Stop overriding - follow workspace settings"
                {disabled}
                onclick={() => setPatterns(null)}
              >
                <Icon name="close" size="micro" />
              </button>
              <div class="pattern-line">
                <PatternEntries
                  patterns={detail.pending_ci_branch_patterns_override.include}
                  readOnly={disabled}
                  onChange={setIncludes}
                />
              </div>
            {/if}
          </div>
          {#if detail.pending_ci_branch_patterns_override !== null}
            <div
              class={[
                'policy-row',
                {
                  'is-unsaved': controlDirty(
                    controlId('pending_ci_branch_patterns_override.exclude'),
                  ),
                },
              ]}
              data-unsaved={controlDirty(
                controlId('pending_ci_branch_patterns_override.exclude'),
              ) || undefined}
            >
              <span class="setting-say">
                <span class="setting-name">Excluded refs</span>
                <span class="setting-why"
                  >Optional patterns that should keep the inherited merge behavior</span
                >
              </span>
              <div class="pattern-line">
                <PatternEntries
                  patterns={detail.pending_ci_branch_patterns_override.exclude}
                  readOnly={disabled}
                  onChange={setExcludes}
                />
              </div>
            </div>
          {/if}
          <div
            class={[
              'policy-row',
              {
                'is-unsaved': controlDirty(controlId('pending_ci_quiet_period_seconds_override')),
              },
            ]}
            data-unsaved={controlDirty(controlId('pending_ci_quiet_period_seconds_override')) ||
              undefined}
          >
            <span class="setting-say">
              <label class="setting-name" for="repository-quiet-{repository.id}"
                >Quiet period after checks pass</label
              >
              <span class="setting-why"
                >Checks must pass and stay green this long before Smyklot merges. Blank inherits</span
              >
            </span>
            <span class="policy-value">
              <DurationInput
                id="repository-quiet-{repository.id}"
                label="Quiet period after checks pass"
                amountLabel="Quiet period after checks pass"
                value={detail.pending_ci_quiet_period_seconds_override}
                inherited={detail.pending_ci_quiet_period_seconds_inherited ?? undefined}
                maximum={86_400}
                allowEmpty
                disabled={readOnly}
                onChange={setQuiet}
                onValidityChange={(problem) =>
                  onDurationValidity(
                    controlId('pending_ci_quiet_period_seconds_override'),
                    problem,
                  )}
              />
            </span>
          </div>
          <div
            class={[
              'policy-row',
              { 'is-unsaved': controlDirty(controlId('path_index_interval_seconds_override')) },
            ]}
            data-unsaved={controlDirty(controlId('path_index_interval_seconds_override')) ||
              undefined}
          >
            <span class="setting-say">
              <span class="setting-name">File index</span>
              <span class="setting-why">How often this repository's file list is read again</span>
            </span>
            {#if detail.path_index_interval_seconds_override === null}
              <span class="policy-value">
                <span class="setting-unmanaged"
                  >From the workspace: every {formatDuration(
                    durationParts(detail.path_index_interval_seconds_inherited, PATH_INDEX_UNITS),
                  )}</span
                >
              </span>
              <button
                class="setting-clear"
                title="Answer for this repository"
                {disabled}
                onclick={() => setPathIndex(detail.path_index_interval_seconds_inherited)}
              >
                <Icon name="plus" size="micro" />
              </button>
            {:else}
              <span class="policy-value">
                <DurationInput
                  label="File index interval"
                  value={detail.path_index_interval_seconds_override}
                  units={PATH_INDEX_UNITS}
                  minimum={60}
                  maximum={604_800}
                  disabled={readOnly}
                  onChange={setPathIndex}
                  onValidityChange={(problem) =>
                    onDurationValidity(controlId('path_index_interval_seconds_override'), problem)}
                />
              </span>
              <button
                class="setting-clear"
                title="Stop answering - take the value from the workspace"
                {disabled}
                onclick={() => setPathIndex(null)}
              >
                <Icon name="close" size="micro" />
              </button>
            {/if}
          </div>
        </div>
        {#if detail.pending_ci_gate !== undefined && detail.pending_ci_gate.readiness !== 'ready' && detail.pending_ci_gate.reason.trim() !== ''}
          <p
            class="gate-note"
            class:gate-problem={detail.pending_ci_gate.readiness === 'blocked'}
            role={detail.pending_ci_gate.readiness === 'blocked' ? 'alert' : 'status'}
          >
            {detail.pending_ci_gate.reason}
          </p>
        {/if}
      </Card>
      <Card unsaved={controlDirty(controlId('pending_ci_bypass_policy_override'))}>
        <div class="card-head"><h2 class="card-title">Merge exceptions</h2></div>
        <BypassPolicyEditor
          {organizationActors}
          value={detail.pending_ci_bypass_policy_override ?? null}
          inherited={detail.pending_ci_bypass_policy_inherited ?? null}
          lookup={lookupBypassActors}
          readOnly={disabled}
          onChange={(value) => {
            const document = currentDocument();
            if (document)
              stage(
                {
                  ...document,
                  pending_ci_bypass_policy_override: value,
                } as RepositorySettingsDocument,
                controlId('pending_ci_bypass_policy_override'),
              );
          }}
        />
      </Card>

      <ConfigEditor
        patch={detail.config_patch}
        inherited={detail.inherited_config}
        scope="repository"
        idPrefix={repository.id}
        {disabled}
        dirtyKeys={dirtyConfigKeys}
        onChange={setConfig}
        onValidity={(problem) =>
          onDurationValidity(controlId('config_patch.command_aliases'), problem)}
      />

      {#if !syncOverride?.problem && !syncReadProblem}{@render syncCard()}{/if}
      {#snippet syncCard()}
        {#if offersSync}
          {#if syncOverride === undefined && syncReadProblem !== null}
            <!-- A read that failed is not a read still going, and the two read
                 identically in a dim line saying "Reading…". -->
            <p class="form-error" role="alert">{syncReadProblem}</p>
          {:else if syncOverride === undefined}
            <p class="detail-loading" role="status">Reading what this repository adjusts…</p>
          {:else}
            <RepositorySyncPane
              stored={syncOverride}
              repositoryId={repository.id}
              envelope={syncEnvelope}
              revealPath={revealSyncFile}
              {readOnly}
              {now}
              dirtyEnabled={controlDirty(`repositories.${repository.id}.sync.files.enabled`)}
              dirtyDocument={controlDirty(`repositories.${repository.id}.sync.files.document`)}
              onChange={onChangeSync}
            />
          {/if}
        {/if}
      {/snippet}
      <DisclosureSection
        title="Formatting preferences"
        description="Advanced · inherits workspace defaults"
      >
        <FormattingEditor
          patch={detail.config_patch.formatting ?? {}}
          savedPatch={savedFormatting}
          inherited={detail.inherited_config.formatting}
          sources={detail.formatting_sources}
          scope="repository"
          idPrefix={repository.id}
          {disabled}
          dirtyKeys={dirtyFormattingKeys}
          onChange={setFormatting}
          onValidity={onFormattingValidity}
        />
      </DisclosureSection>
    {/if}
  </section>
</div>

<style>
  .repository-page {
    display: grid;
    gap: var(--space-4);
    min-width: 0;
  }

  .pane-tools {
    display: flex;
    justify-content: flex-start;
    /* On a phone the four panes cannot share the width; the strip scrolls
       inside itself rather than handing the page a wider viewport. */
    overflow-x: auto;
  }

  .repository-page-error {
    margin: 0;
  }

  .detail-loading {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    margin: 0;
    padding: var(--space-4) 0;
  }

  .group-head {
    align-items: end;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    margin-bottom: var(--space-2);
  }

  .group-name {
    font-size: var(--font-size-title);
    font-weight: 600;
    margin: 0;
    min-block-size: 12px;
    text-box: trim-both cap alphabetic;
  }

  .pill {
    align-items: center;
    block-size: var(--tier-mark);
    border-radius: var(--radius-chip);
    display: inline-flex;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: 0.25rem;
    line-height: var(--leading-flat);
    padding: 0 0.5rem;
  }

  .pill .t {
    display: block;
    text-box: trim-both cap alphabetic;
  }

  .pill-success {
    background: var(--success-tint);
    color: var(--success);
  }

  .pill-warning {
    background: var(--warning-tint);
    color: var(--warning);
  }

  .pill-danger {
    background: var(--danger-tint);
    color: var(--danger);
  }

  .pill-muted {
    background: var(--surface-inset);
    color: var(--text-muted);
  }

  .menu-item {
    align-items: center;
    background: none;
    border: 0;
    border-radius: 6px;
    block-size: 32px;
    color: var(--text-primary);
    cursor: pointer;
    display: flex;
    font-size: var(--font-size-control);
    gap: var(--space-2);
    inline-size: 100%;
    padding-inline: var(--space-3);
    text-align: start;
  }

  .menu-item:hover {
    background: var(--interactive-hover-layer);
  }

  .menu-item:focus-visible {
    background: var(--interactive-hover-layer);
    outline: none;
  }

  .menu-item:active {
    background: var(--interactive-pressed);
  }

  .menu-check {
    display: inline-flex;
    flex: none;
    inline-size: 16px;
    justify-content: center;
  }

  .menu-item :global(.mi-label) {
    min-inline-size: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* A block row keeps its sentence on the first line and lays the entries on a full-width
     second one. `flex-basis: 100%` is what takes that line under the row law - the old
     `grid-column: 1 / -1` addressed a grid the row no longer is. */
  .pattern-line {
    flex-basis: 100%;
    margin-block: var(--space-1) 0;
  }

  .gate-note {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    /* Ink-true with even padding, so the words sit on the note's centre. */
    line-height: var(--leading-meta);
    margin: var(--space-3) 0 0;
    padding: var(--space-3);
    text-box: trim-both cap alphabetic;
  }

  .gate-note.gate-problem {
    color: var(--danger);
  }

  .form-error {
    color: var(--danger);
  }

  /* On a phone the head's three parts cannot share one line - the tally or
     pill drops under the title instead of holding the card wide. */
  @media (max-width: 30rem) {
    .group-head {
      flex-wrap: wrap;
    }
  }
</style>
