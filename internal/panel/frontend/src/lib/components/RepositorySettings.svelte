<script lang="ts">
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
    RepositoryFileStatus,
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
  import Card from './Card.svelte';
  import PageHeader from './PageHeader.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import Popover from './Popover.svelte';
  import RepositorySyncPane from './RepositorySyncPane.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  const FILE_STATUS_PILLS = {
    valid: 'pill-success',
    missing: 'pill-muted',
    invalid: 'pill-danger',
    bypassed: 'pill-warning',
  } as const satisfies Record<RepositoryFileStatus, string>;
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
    failure = null,
    readOnly = false,
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
    syncReadProblem = null,
    now = 0,
    onChangeSync = () => {},
    onFormattingValidity = () => {},
    dirtyControls = [],
  }: {
    repository: RepositorySummary;
    detail: RepositoryDetail | undefined;
    failure?: string | null;
    readOnly?: boolean;
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
    syncReadProblem?: string | null;
    /** The clock the pane's relative times are read against. */
    now?: number;
    onChangeSync?: (next: SyncOverrideEditorEnvelope, control: SyncOverrideControlId) => void;
    onFormattingValidity?: (valid: boolean) => void;
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

  /* ---------- The quiet-period seconds, staged while typing ---------- */

  let quietDraft = $state<string | null>(null);
  const quietShown = $derived(
    quietDraft ?? detail?.pending_ci_quiet_period_seconds_override?.toString() ?? '',
  );

  function typeQuiet(value: string): void {
    quietDraft = value;
    const document = currentDocument();
    if (document === null) return;
    const trimmed = value.trim();
    const quiet = trimmed === '' ? null : Number(trimmed);
    if (quiet !== null && (!Number.isInteger(quiet) || quiet < 0 || quiet > 86_400)) return;
    stage(
      { ...document, pending_ci_quiet_period_seconds_override: quiet },
      controlId('pending_ci_quiet_period_seconds_override'),
    );
  }

  function finishQuiet(): void {
    if (quietDraft === null) return;
    const trimmed = quietDraft.trim();
    const quiet = trimmed === '' ? null : Number(trimmed);
    if (quiet !== null && (!Number.isInteger(quiet) || quiet < 0 || quiet > 86_400)) return;
    quietDraft = null;
  }

  /* ---------- The path-index interval, an amount beside a unit ---------- */

  const PATH_INDEX_UNITS: readonly DurationUnit[] = ['minutes', 'hours', 'days'];
  const UNIT_SECONDS: Record<DurationUnit, number> = {
    seconds: 1,
    minutes: 60,
    hours: 3_600,
    days: 86_400,
  };
  let indexAmountDraft = $state<string | null>(null);
  let indexUnitDraft = $state<DurationUnit | null>(null);

  function indexParts(): { amount: number; unit: DurationUnit } {
    const seconds =
      detail?.path_index_interval_seconds_override ??
      detail?.path_index_interval_seconds_inherited ??
      3_600;
    return durationParts(seconds, PATH_INDEX_UNITS);
  }

  const indexAmountShown = $derived(indexAmountDraft ?? indexParts().amount.toString());
  const indexUnitShown = $derived(indexUnitDraft ?? indexParts().unit);

  function typeIndexAmount(value: string): void {
    indexAmountDraft = value;
    indexUnitDraft = indexUnitShown;
    stageIndexDraft(value, indexUnitShown);
  }

  function pickIndexUnit(unit: DurationUnit): void {
    indexAmountDraft = indexAmountShown;
    indexUnitDraft = unit;
    if (stageIndexDraft(indexAmountShown, unit)) {
      indexAmountDraft = null;
      indexUnitDraft = null;
    }
  }

  function stageIndexDraft(amount: string, unit: DurationUnit): boolean {
    const document = currentDocument();
    if (document === null) return false;
    const seconds = Math.round(Number(amount) * UNIT_SECONDS[unit]);
    if (!Number.isFinite(seconds) || seconds < 60 || seconds > 604_800) return false;
    stage(
      { ...document, path_index_interval_seconds_override: seconds },
      controlId('path_index_interval_seconds_override'),
    );
    return true;
  }

  function finishIndexDraft(): void {
    if (indexAmountDraft === null || indexUnitDraft === null) return;
    if (stageIndexDraft(indexAmountDraft, indexUnitDraft)) {
      indexAmountDraft = null;
      indexUnitDraft = null;
    }
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

  /* The file card used to repeat whatever this repository overrides, because Behavior was
     behind a tab and a reader on the file pane could not see it. Both are on the page
     now, so the repeat was the same rows twice on one screen. */

  /** What the switch's answer means here, said rather than left to the words on it. */
  function enablementWhy(value: 'inherit' | 'enabled' | 'disabled'): string {
    if (value === 'enabled') return 'On - commands and merges run in this repository';
    if (value === 'disabled') return 'Off - Smyklot stands down here, whatever the workspace says';

    return repository.effective_enabled
      ? 'The workspace has it on, so it runs here'
      : 'The workspace has it off, so it stands down here';
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
      <!-- CONTROL FIRST: whether Smyklot answers here at all, and whether it reads the
           repository's own file, come before anything either of them decides. Written
           below with the rest of the detail and rendered here, because both halves of
           that card need `detail` and this is where the page wants it read. -->
      {@render controlCard()}
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
                    <button
                      {...attributes}
                      class="value-select"
                      type="button"
                      aria-label="{detail.pending_ci_mode_override === 'checks'
                        ? 'Checks'
                        : 'Labels'} - repository protection"
                      {disabled}
                    >
                      <span class="t"
                        >{detail.pending_ci_mode_override === 'checks' ? 'Checks' : 'Labels'}</span
                      >
                    </button>
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
            <!-- The unit beside the number, as the workspace page says it. -->
            <span class="policy-value entry-suffix">
              <input
                id="repository-quiet-{repository.id}"
                class="num-inline"
                inputmode="numeric"
                placeholder={detail.pending_ci_quiet_period_seconds_inherited?.toString() ??
                  'Global default'}
                value={quietShown}
                disabled={readOnly}
                oninput={(event) => typeQuiet(event.currentTarget.value)}
                onblur={finishQuiet}
              />
              <span class="entry-unit">seconds</span>
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
                <input
                  class="num-inline num-short"
                  inputmode="numeric"
                  aria-label="File index interval amount"
                  value={indexAmountShown}
                  disabled={readOnly}
                  oninput={(event) => typeIndexAmount(event.currentTarget.value)}
                  onblur={finishIndexDraft}
                />
                <Popover
                  role="listbox"
                  label="File index interval unit"
                  align="end"
                  itemSelector=".menu-item"
                >
                  {#snippet trigger(attributes)}
                    <button
                      {...attributes}
                      class="value-select"
                      type="button"
                      aria-label="{indexUnitShown} - file index interval unit"
                      {disabled}
                    >
                      <span class="t">{indexUnitShown}</span>
                    </button>
                  {/snippet}
                  <div class="menu-list">
                    {#each PATH_INDEX_UNITS as unit (unit)}
                      <button
                        class="menu-item"
                        role="option"
                        aria-selected={indexUnitShown === unit}
                        onclick={() => pickIndexUnit(unit)}
                      >
                        <span class="menu-check">
                          {#if indexUnitShown === unit}<Icon name="check" size="base" />{/if}
                        </span>
                        <ClippedLabel class="mi-label" text={unit} />
                      </button>
                    {/each}
                  </div>
                </Popover>
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
        {#if detail.pending_ci_gate !== undefined}
          <p class="gate-note" class:gate-problem={detail.pending_ci_gate.readiness === 'blocked'}>
            {detail.pending_ci_gate.reason}
          </p>
        {/if}
      </Card>

      <!-- ONE SCROLL, NOT FIVE PANES. The switch over File / Behavior / Commands /
           Formatting / Sync made a reader press four times to see what one repository
           is set to, and hid from them that most of those panes were empty. The cards
           are the same cards; they are all here at once - and the wrapper that used to
           hold the open pane is gone with the panes, because it declared the page's own
           grid and gap a second time. -->
      {#snippet controlCard()}
        <Card labelledby="repository-file-head">
          <div class="card-head">
            <h2 class="card-title" id="repository-file-head">Repository control</h2>
            <span class="pill {FILE_STATUS_PILLS[detail.repository.config_file_status]}"
              ><span class="t">{capitalize(detail.repository.config_file_status)}</span></span
            >
          </div>
          <div class={['file-card', detail.config_file_error !== undefined && 'file-problem']}>
            <!-- 14px glyph in an 18px slot, the same pairing every other icon
                 slot in the product uses. -->
            <span class="file-card-icon status-{detail.repository.config_file_status}">
              <Icon name="file" size="sm" />
            </span>
            <div class="f-copy">
              <strong>Configuration path</strong>
              <!-- The file is looked for in four places plus a chosen one, so
                   this names the one that won rather than the one that used to
                   be the only candidate. -->
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
                    disabled={readOnly || busy}
                    onclick={onResetMigration}
                  >
                    Let it ask
                  </button>
                </p>
              {/if}
            </div>
          </div>
          <div class="policy-rows">
            <div
              class={['policy-row', { 'is-unsaved': controlDirty(controlId('enabled_override')) }]}
              data-unsaved={controlDirty(controlId('enabled_override')) || undefined}
            >
              <span class="setting-say">
                <span class="setting-name">Smyklot</span>
                <span class="setting-why">{enablementWhy(enablement)}</span>
              </span>
              <span class="policy-value">
                <!-- Three, not two. A repository can FOLLOW the workspace, and that is a
                     different answer from being switched on here to the same value: the
                     workspace changing carries the first and leaves the second alone. -->
                <SegmentedControl
                  name="repository-enabled-{repository.id}"
                  label="Smyklot in {repository.name}"
                  options={[
                    { value: 'inherit', label: 'From the workspace' },
                    { value: 'enabled', label: 'On' },
                    { value: 'disabled', label: 'Off' },
                  ]}
                  value={enablement}
                  {disabled}
                  compact
                  onSelect={(next) => onEnablement(next)}
                />
              </span>
            </div>
            <div
              class={[
                'policy-row',
                { 'is-unsaved': controlDirty(controlId('ignore_repository_file')) },
              ]}
              data-unsaved={controlDirty(controlId('ignore_repository_file')) || undefined}
            >
              <span class="setting-say">
                <span class="setting-name">Repository file</span>
                <span class="setting-why"
                  >When bypassed, the file's settings are ignored and the exception is recorded in
                  Audit</span
                >
              </span>
              <span class="policy-value">
                <SegmentedControl
                  name="repository-bypass-{repository.id}"
                  label="Repository file handling"
                  options={[
                    { value: 'observe', label: 'Followed' },
                    { value: 'bypass', label: 'Bypassed' },
                  ]}
                  value={detail.ignore_repository_file ? 'bypass' : 'observe'}
                  {disabled}
                  compact
                  onSelect={(value) => setBypass(value === 'bypass')}
                />
              </span>
            </div>
          </div>
        </Card>
      {/snippet}

      <ConfigEditor
        patch={detail.config_patch}
        inherited={detail.inherited_config}
        scope="repository"
        idPrefix={repository.id}
        {disabled}
        dirtyKeys={dirtyConfigKeys}
        onChange={setConfig}
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

  .value-select {
    align-items: center;
    appearance: none;
    background:
      linear-gradient(45deg, transparent 49%, var(--text-secondary) 51%) calc(100% - 14px) 55% / 5px
        5px no-repeat,
      linear-gradient(135deg, var(--text-secondary) 49%, transparent 51%) calc(100% - 9px) 55% / 5px
        5px no-repeat,
      var(--control-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    cursor: pointer;
    display: inline-flex;
    font-size: var(--font-size-control);
    min-block-size: var(--tier-quiet);
    padding: 0 1.5rem 0 var(--space-2);
  }

  /* Ink-true, so the chosen word shares the row's centre with the say
     beside it rather than riding its line box's leading. */
  .value-select .t {
    text-box: trim-both cap alphabetic;
  }

  .value-select[data-state='open'] {
    background:
      linear-gradient(45deg, transparent 49%, var(--text-secondary) 51%) calc(100% - 14px) 55% / 5px
        5px no-repeat,
      linear-gradient(135deg, var(--text-secondary) 49%, transparent 51%) calc(100% - 9px) 55% / 5px
        5px no-repeat,
      var(--control-bg-pressed);
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

  .num-inline {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font-family: var(--mono);
    font-size: var(--font-size-control);
    min-block-size: var(--tier-quiet);
    padding: 0 var(--space-2);
    text-align: end;
    width: 8.5rem;
  }

  .num-inline.num-short {
    width: 5rem;
  }

  .num-inline::placeholder {
    color: var(--text-muted);
  }

  .num-inline:focus-visible {
    border-color: var(--brand-action);
    outline: 2px solid var(--focus);
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

  /* The card keeps its 71px stature whatever its copy measures: trimming the two
     lines to their ink took 14px out of the content, and the card's height is a
     shape decision, not a consequence of the leading. */
  .file-card {
    align-items: center;
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    display: flex;
    gap: var(--space-3);
    margin-bottom: var(--space-2);
    min-height: 4.4375rem;
    padding: var(--space-3) var(--space-4);
  }

  /* In a rounded plate of its own, like every other symbol that stands beside a
     title inside a card. The plate is keyed to the glyph's own colour, so it
     carries the file's state rather than a fixed brand tint. */
  .file-card-icon {
    align-items: center;
    background: color-mix(in srgb, currentcolor 10%, transparent);
    border: 1px solid color-mix(in srgb, currentcolor 24%, transparent);
    border-radius: var(--radius-control);
    color: var(--text-muted);
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
     the copy block's BOX equals its ink and the card's flex centring centres what
     the eye reads. Untrimmed, the first line's leading and the last line's
     descender are not symmetric and the text sat 3.36px below the card's middle.
     0.8rem keeps the baseline-to-baseline distance the two lines already had. */
  .f-copy strong {
    display: block;
    font-size: var(--font-size-meta);
    line-height: var(--leading-flat);
    text-box: trim-both cap alphabetic;
  }

  .f-copy code {
    color: var(--text-muted);
    display: block;
    font-size: var(--font-size-compact);
    line-height: var(--leading-flat);
    margin-top: 0.8rem;
    overflow-wrap: anywhere;
    text-box: trim-both cap alphabetic;
  }

  /* Trimmed like the two lines above it, and for the same reason: the card
     centres its copy block as a BOX, so the block's box has to equal its ink or
     the centring is of something the reader cannot see. */
  .f-copy p {
    color: var(--danger);
    font-size: var(--font-size-compact);
    line-height: var(--leading-flat);
    margin: 0.5rem 0 0;
    text-box: trim-both cap alphabetic;
  }

  /* A file the repository still carries and Smyklot is not reading is worth
     saying, and is not a failure - so it wears the dim tone the path above it
     wears rather than the danger tone the parse error does. */
  .f-copy p.f-note {
    color: var(--text-muted);
  }

  /* An inline continuation of the sentence above it, not a control in its own
     right: it sits on the same line, at the same size, and is underlined the way
     a link in prose is. Giving it a button's chrome would make refusing a
     migration look like it had a button to undo it, which is the opposite of
     what a durable refusal means. */
  .f-again {
    background: none;
    border: 0;
    color: var(--text-primary);
    cursor: pointer;
    font: inherit;
    margin-left: 0.35rem;
    padding: 0;
    text-decoration: underline;
    text-underline-offset: 0.15em;
  }

  .f-again:disabled {
    color: var(--text-muted);
    cursor: default;
    text-decoration: none;
  }

  /* The overridden behavior rows continue the card's own row list under the
     bypass row, separated by the same drawn hairline the rows use - so the
     rows on either side of that line keep the full separator rhythm. */
  .file-overrides {
    border-top: 1px solid var(--border-subtle);
  }

  .file-problem strong,
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
