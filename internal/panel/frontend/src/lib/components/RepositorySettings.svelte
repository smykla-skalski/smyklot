<script lang="ts">
  import { BOOLEAN_FIELDS } from '../config';
  import { REPOSITORY_SECTIONS, type RepositorySection } from '../routes';
  import type {
    ConfigKey,
    ConfigPatch,
    PendingCIBranchPatterns,
    PendingCIMode,
    RepositoryDetail,
    RepositoryFileStatus,
    RepositorySummary,
    SyncOverride,
  } from '../types';
  import ClippedLabel from './ClippedLabel.svelte';
  import ConfigEditor from './ConfigEditor.svelte';
  import Icon from './Icon.svelte';
  import PanePath from './PanePath.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import Popover from './Popover.svelte';
  import RepositorySyncPane from './RepositorySyncPane.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  /**
   * One repository's own page.
   *
   * This was a dialog until it was three panes with a save bar in each, which is
   * a screen someone works in rather than something standing over the list for a
   * moment. It reads as any other object page of the panel does - a way back, a
   * mono title with the switch beside it, then the pane - and it is addressable,
   * so a link points at the pane a colleague was asked to look at.
   */

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
    section,
    failure = null,
    readOnly = false,
    busy = false,
    backHref,
    onBack,
    onSection,
    onBypass,
    onSaveConfig,
    onSavePendingCI,
    onResetMigration,
    sections = REPOSITORY_SECTIONS,
    syncOverride = undefined,
    syncSaving = false,
    syncReadProblem = null,
    syncSaveProblem = null,
    now = 0,
    onSaveSync = () => {},
  }: {
    repository: RepositorySummary;
    detail: RepositoryDetail | undefined;
    section: RepositorySection;
    failure?: string | null;
    readOnly?: boolean;
    busy?: boolean;
    backHref: string;
    onBack: () => void;
    onSection: (section: RepositorySection) => void;
    /** Rejects when the save was refused - the receipt only shows on success. */
    onBypass: (bypass: boolean) => void | Promise<void>;
    /* The editor awaits this to know whether its receipt may show, so the
       promise is part of the contract rather than something to fire and drop. */
    onSaveConfig: (patch: ConfigPatch) => Promise<void>;
    onSavePendingCI: (
      mode: PendingCIMode | null,
      patterns: PendingCIBranchPatterns | null,
      quiet: number | null,
    ) => Promise<void>;
    onResetMigration: () => void;
    /**
     * The panes this surface offers, in the order the switch shows them.
     *
     * Handed in rather than worked out here, because which panes there are is
     * a fact about where this is being drawn. The Root view of somebody else's
     * installation has no sync pane: sync is configured on the installation's
     * own page and has no Root address, so a pane offering to edit it there
     * would be a pane whose every save is a 404.
     */
    sections?: readonly RepositorySection[];
    /** Undefined until the pane is opened and the read comes back. */
    syncOverride?: SyncOverride | undefined;
    syncSaving?: boolean;
    /* Two problems, because they belong to different things: a read that did
       not answer leaves the pane with nothing to draw, and a save that was
       refused leaves it drawing what the reader typed. */
    syncReadProblem?: string | null;
    syncSaveProblem?: string | null;
    /** The clock the pane's relative times are read against. */
    now?: number;
    onSaveSync?: (enabled: boolean | null, document: Record<string, unknown>) => void;
  } = $props();

  const disabled = $derived(readOnly || busy);
  const titleId = 'repository-page-title';

  /* ---------- The saved receipts ---------- */

  let mergeSavedOn = $state(false);
  let fileSavedOn = $state(false);
  let mergeTimer: ReturnType<typeof setTimeout> | undefined;
  let fileTimer: ReturnType<typeof setTimeout> | undefined;

  function whisper(card: 'merge' | 'file'): void {
    if (card === 'merge') {
      mergeSavedOn = true;
      clearTimeout(mergeTimer);
      mergeTimer = setTimeout(() => (mergeSavedOn = false), 1400);
    } else {
      fileSavedOn = true;
      clearTimeout(fileTimer);
      fileTimer = setTimeout(() => (fileSavedOn = false), 1400);
    }
  }

  /* ---------- Merge after CI, saved change by change ---------- */

  let savingPendingCI = $state(false);

  async function pushPendingCI(change: {
    mode?: PendingCIMode | null;
    patterns?: PendingCIBranchPatterns | null;
    quiet?: number | null;
  }): Promise<void> {
    if (detail === undefined || savingPendingCI) return;
    savingPendingCI = true;
    try {
      await onSavePendingCI(
        change.mode !== undefined ? change.mode : detail.pending_ci_mode_override,
        change.patterns !== undefined
          ? change.patterns
          : detail.pending_ci_branch_patterns_override,
        change.quiet !== undefined ? change.quiet : detail.pending_ci_quiet_period_seconds_override,
      );
      whisper('merge');
    } catch {
      /* The page's failure line reports it. */
    } finally {
      savingPendingCI = false;
    }
  }

  function overrideMode(): void {
    if (detail === undefined) return;
    void pushPendingCI({ mode: detail.pending_ci_mode_inherited });
  }

  function overridePatterns(): void {
    if (detail === undefined) return;
    /* A JSON clone, not structuredClone: the detail arrives as a $state proxy,
       which structuredClone refuses to clone. */
    void pushPendingCI({
      patterns: JSON.parse(
        JSON.stringify(detail.pending_ci_branch_patterns_inherited),
      ) as PendingCIBranchPatterns,
    });
  }

  function setIncludes(next: string[]): void {
    if (detail === undefined || detail.pending_ci_branch_patterns_override === null) return;
    if (next.length === 0) return;
    void pushPendingCI({
      patterns: { include: next, exclude: detail.pending_ci_branch_patterns_override.exclude },
    });
  }

  function setExcludes(next: string[]): void {
    if (detail === undefined || detail.pending_ci_branch_patterns_override === null) return;
    void pushPendingCI({
      patterns: { include: detail.pending_ci_branch_patterns_override.include, exclude: next },
    });
  }

  /* ---------- The quiet-period seconds, saved after a typing rest ---------- */

  const SAVE_REST_MS = 900;
  let quietDraft = $state<string | null>(null);
  let quietTimer: ReturnType<typeof setTimeout> | undefined;
  const quietShown = $derived(
    quietDraft ?? detail?.pending_ci_quiet_period_seconds_override?.toString() ?? '',
  );

  function typeQuiet(value: string): void {
    quietDraft = value;
    clearTimeout(quietTimer);
    quietTimer = setTimeout(saveQuiet, SAVE_REST_MS);
  }

  function saveQuiet(): void {
    clearTimeout(quietTimer);
    quietTimer = undefined;
    if (detail === undefined || quietDraft === null) return;
    const trimmed = quietDraft.trim();
    const quiet = trimmed === '' ? null : Number(trimmed);
    if (quiet !== null && (!Number.isInteger(quiet) || quiet < 0 || quiet > 86_400)) return;
    if (quiet === detail.pending_ci_quiet_period_seconds_override) {
      quietDraft = null;
      return;
    }
    quietDraft = null;
    void pushPendingCI({ quiet });
  }

  async function setBypass(bypass: boolean): Promise<void> {
    try {
      await onBypass(bypass);
      whisper('file');
    } catch {
      /* The page's failure line reports it. */
    }
  }

  /* The repository-file pane lists the behavior settings this repository
     actually overrides, the way the approved design draws it: the file card, the
     bypass control, then whatever this repo has changed. Someone reading the
     file pane is asking "what does this repository do differently", and the
     answer belongs on the same screen as the file. */
  function overriddenBehaviorKeys(one: RepositoryDetail): ConfigKey[] {
    return BOOLEAN_FIELDS.map((field) => field.key).filter((key) =>
      Object.hasOwn(one.config_patch, key),
    );
  }

  function sectionCount(one: RepositoryDetail, pane: 'behavior' | 'commands'): number {
    const keys: readonly ConfigKey[] =
      pane === 'behavior'
        ? BOOLEAN_FIELDS.map((field) => field.key)
        : ['command_prefix', 'allowed_commands', 'command_aliases'];

    return keys.filter((key) => Object.hasOwn(one.config_patch, key)).length;
  }

  /* How many of this repository's own settings a pane holds, where the number
     is worth a badge. The file pane counts a broken file rather than settings,
     and sync has nothing to count - what it holds is one switch and a list. */
  function sectionBadge(one: RepositoryDetail, pane: RepositorySection): number | undefined {
    if (pane === 'file') return one.config_file_error === undefined ? undefined : 1;
    if (pane === 'sync') return undefined;

    const count = sectionCount(one, pane);

    return count === 0 ? undefined : count;
  }

  /* What each pane is called. Which panes exist is REPOSITORY_SECTIONS, which
     keys this record, so a fifth one is a compile error here rather than a
     switch quietly missing an option. Both the label under the switch and the
     switch's own options read it. */
  const SECTION_LABELS: Record<RepositorySection, string> = {
    file: 'File',
    behavior: 'Behavior',
    commands: 'Commands',
    sync: 'Sync',
  };

  /** Names the pane for a screen reader, which the switch above it does not. */
  function sectionLabel(pane: RepositorySection): string {
    return SECTION_LABELS[pane];
  }

  function capitalize(value: string): string {
    return value.slice(0, 1).toUpperCase() + value.slice(1);
  }
</script>

<div class="view-frame">
  <section class="repository-page" aria-labelledby={titleId}>
    <PanePath segments={[{ label: 'Repositories', href: backHref, onSelect: onBack }]} />

    <header class="object-head">
      <h2 class="mono-title" id={titleId}>{repository.name}</h2>
      <p class="object-sub">
        Repository settings override workspace defaults and repository-file values
      </p>
    </header>

    {#if detail !== undefined}
      <div class="pane-tools">
        <SegmentedControl
          name="repository-{repository.id}-section"
          label="Settings for {repository.name}"
          options={sections.map((pane) => ({
            value: pane,
            label: SECTION_LABELS[pane],
            badge: sectionBadge(detail, pane),
          }))}
          value={section}
          onSelect={(next) => onSection(next as RepositorySection)}
        />
      </div>
    {/if}

    {#if failure !== null}
      <p class="form-error repository-page-error" role="alert">{failure}</p>
    {/if}

    {#if detail === undefined}
      <p class="detail-loading" role="status">Reading repository settings…</p>
    {:else}
      <section class="card group-card" aria-labelledby="repository-merge-ci">
        <div class="group-head">
          <h3 class="group-name" id="repository-merge-ci">Merge after CI</h3>
          <span class="save-whisper" class:is-on={mergeSavedOn} role="status"
            ><Icon name="check" size={12} /><span class="t">Saved</span></span
          >
          {#if detail.pending_ci_gate !== undefined}
            <span class="pill {GATE_PILLS[detail.pending_ci_gate.readiness]}"
              ><span class="t">{capitalize(detail.pending_ci_gate.readiness)}</span></span
            >
          {/if}
        </div>
        <div class="policy-rows">
          <div class="policy-row">
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
                  >Follows workspace - {detail.pending_ci_mode_inherited}</span
                >
              </span>
              <button
                class="setting-clear"
                title="Override the workspace mode"
                disabled={disabled || savingPendingCI}
                onclick={overrideMode}
              >
                <Icon name="plus" size={10} />
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
                      aria-label="Repository protection"
                      disabled={disabled || savingPendingCI}
                    >
                      {detail.pending_ci_mode_override === 'checks' ? 'Checks' : 'Labels'}
                    </button>
                  {/snippet}
                  <div class="menu-list">
                    {#each PENDING_CI_CHOICES as option (option.value)}
                      <button
                        class="menu-item"
                        role="option"
                        aria-selected={detail.pending_ci_mode_override === option.value}
                        onclick={() => void pushPendingCI({ mode: option.value })}
                      >
                        <span class="menu-check">
                          {#if detail.pending_ci_mode_override === option.value}<Icon
                              name="check"
                              size={16}
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
                disabled={disabled || savingPendingCI}
                onclick={() => void pushPendingCI({ mode: null })}
              >
                <Icon name="close" size={10} />
              </button>
            {/if}
          </div>
          <div
            class="policy-row"
            class:policy-block={detail.pending_ci_branch_patterns_override !== null}
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
                  >Follows workspace - {detail.pending_ci_branch_patterns_inherited.include.join(
                    ', ',
                  )}</span
                >
              </span>
              <button
                class="setting-clear"
                title="Override the protected branch patterns"
                disabled={disabled || savingPendingCI}
                onclick={overridePatterns}
              >
                <Icon name="plus" size={10} />
              </button>
            {:else}
              <span class="policy-value"></span>
              <button
                class="setting-clear"
                title="Stop overriding - follow workspace settings"
                disabled={disabled || savingPendingCI}
                onclick={() => void pushPendingCI({ patterns: null })}
              >
                <Icon name="close" size={10} />
              </button>
              <div class="pattern-line">
                <PatternEntries
                  patterns={detail.pending_ci_branch_patterns_override.include}
                  readOnly={disabled || savingPendingCI}
                  onChange={setIncludes}
                />
              </div>
            {/if}
          </div>
          {#if detail.pending_ci_branch_patterns_override !== null}
            <div class="policy-row policy-block">
              <span class="setting-say">
                <span class="setting-name">Excluded refs</span>
                <span class="setting-why"
                  >Optional patterns that should keep the inherited merge behavior</span
                >
              </span>
              <div class="pattern-line">
                <PatternEntries
                  patterns={detail.pending_ci_branch_patterns_override.exclude}
                  readOnly={disabled || savingPendingCI}
                  onChange={setExcludes}
                />
              </div>
            </div>
          {/if}
          <div class="policy-row">
            <span class="setting-say">
              <label class="setting-name" for="repository-quiet-{repository.id}"
                >Stable passing window</label
              >
              <span class="setting-why">Seconds; leave blank to inherit</span>
            </span>
            <span class="policy-value">
              <input
                id="repository-quiet-{repository.id}"
                class="num-inline"
                inputmode="numeric"
                placeholder={detail.pending_ci_quiet_period_seconds_inherited?.toString() ??
                  'Global default'}
                value={quietShown}
                disabled={readOnly}
                oninput={(event) => typeQuiet(event.currentTarget.value)}
                onblur={saveQuiet}
              />
            </span>
          </div>
        </div>
        {#if detail.pending_ci_gate !== undefined}
          <p class="gate-note" class:gate-problem={detail.pending_ci_gate.readiness === 'blocked'}>
            {detail.pending_ci_gate.reason}
          </p>
        {/if}
      </section>

      <div
        class="repository-detail-content"
        role="group"
        aria-label="{sectionLabel(section)} settings for {repository.name}"
      >
        {#if section === 'file'}
          <section class="card group-card" aria-labelledby="repository-file-head">
            <div class="group-head">
              <h3 class="group-name" id="repository-file-head">Repository file</h3>
              <span class="save-whisper" class:is-on={fileSavedOn} role="status"
                ><Icon name="check" size={12} /><span class="t">Saved</span></span
              >
              <span class="pill {FILE_STATUS_PILLS[detail.repository.config_file_status]}"
                ><span class="t">{capitalize(detail.repository.config_file_status)}</span></span
              >
            </div>
            <p class="group-note">
              Settings Smyklot reads from the repository itself, which override account defaults
            </p>
            <div class={['file-card', detail.config_file_error !== undefined && 'file-problem']}>
              <!-- 14px glyph in an 18px slot, the same pairing every other icon
                 slot in the product uses. -->
              <span class="file-card-icon status-{detail.repository.config_file_status}">
                <Icon name="file" size={14} />
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
                    <button type="button" class="f-again" {disabled} onclick={onResetMigration}>
                      Let it ask
                    </button>
                  </p>
                {/if}
              </div>
            </div>
            <div class="policy-rows">
              <div class="policy-row">
                <span class="setting-say">
                  <span class="setting-name">Bypass file</span>
                  <span class="setting-why"
                    >Repository-file settings are ignored and the exception is recorded in Audit</span
                  >
                </span>
                <span class="policy-value">
                  <SegmentedControl
                    name="repository-bypass-{repository.id}"
                    label="Repository file handling"
                    options={[
                      { value: 'observe', label: 'Observe' },
                      { value: 'bypass', label: 'Bypass' },
                    ]}
                    value={detail.ignore_repository_file ? 'bypass' : 'observe'}
                    {disabled}
                    compact
                    onSelect={(value) => void setBypass(value === 'bypass')}
                  />
                </span>
              </div>
            </div>
            {#if overriddenBehaviorKeys(detail).length > 0}
              <div class="file-overrides">
                <ConfigEditor
                  patch={detail.config_patch}
                  inherited={detail.inherited_config}
                  scope="repository"
                  idPrefix="{repository.id}-file"
                  section="behavior"
                  only={overriddenBehaviorKeys(detail)}
                  {disabled}
                  onSave={onSaveConfig}
                />
              </div>
            {/if}
          </section>
        {:else if section === 'sync'}
          {#if syncOverride === undefined && syncReadProblem !== null}
            <!-- A read that failed is not a read still going, and the two read
                 identically in a dim line saying "Reading…". -->
            <p class="form-error" role="alert">{syncReadProblem}</p>
          {:else if syncOverride === undefined}
            <p class="detail-loading" role="status">Reading what this repository adjusts…</p>
          {:else}
            <RepositorySyncPane
              stored={syncOverride}
              {readOnly}
              {now}
              saving={syncSaving}
              saveProblem={syncSaveProblem}
              onSave={onSaveSync}
            />
          {/if}
        {:else}
          <ConfigEditor
            patch={detail.config_patch}
            inherited={detail.inherited_config}
            scope="repository"
            idPrefix={repository.id}
            {section}
            {disabled}
            onSave={onSaveConfig}
          />
        {/if}
      </div>
    {/if}
  </section>
</div>

<style>
  .view-frame {
    margin-inline: auto;
    max-width: var(--content-max);
  }

  .repository-page {
    display: grid;
    gap: var(--space-4);
    min-width: 0;
  }

  .object-head {
    display: grid;
    gap: var(--space-2);
  }

  .mono-title {
    font-family: var(--mono);
    font-size: 1.375rem;
    letter-spacing: -0.01em;
    margin: 0;
  }

  .object-sub {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    line-height: round(1.5em, 1px);
    margin: 0;
    max-width: 64ch;
  }

  .pane-tools {
    display: flex;
    justify-content: flex-start;
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

  .repository-detail-content {
    display: grid;
    gap: var(--space-4);
    min-width: 0;
  }

  .card {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    padding: var(--space-5);
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

  .group-note {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: 0 0 var(--space-2);
    max-width: 60ch;
  }

  .save-whisper {
    align-items: center;
    background: var(--success-tint);
    block-size: 20px;
    border-radius: var(--radius-chip);
    color: var(--success);
    display: inline-flex;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: 4px;
    margin-inline-start: auto;
    opacity: 0;
    padding: 0 0.5rem;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .save-whisper.is-on {
    opacity: 1;
  }

  .save-whisper .t {
    text-box: trim-both cap alphabetic;
  }

  .pill {
    align-items: center;
    block-size: 20px;
    border-radius: var(--radius-chip);
    display: inline-flex;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: 0.25rem;
    line-height: 1;
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

  .policy-rows {
    display: grid;
  }

  .policy-row {
    align-items: center;
    display: grid;
    gap: var(--space-2) var(--space-4);
    grid-template-columns: 1fr auto auto;
    margin-inline: calc(var(--space-2) * -1);
    min-block-size: 48px;
    padding: 0.5rem var(--space-2);
    position: relative;
  }

  /* Every row owns the drawn hairline under itself; the last one stands
     down, so the card ends on its own padding. */
  .policy-row:not(:last-child)::after {
    background: var(--border-subtle);
    block-size: 1px;
    bottom: 0;
    content: '';
    inset-inline: var(--space-2);
    position: absolute;
  }

  .setting-say {
    display: grid;
    gap: var(--space-3);
  }

  .setting-name {
    font-size: var(--font-size-meta);
    font-weight: 600;
    min-block-size: 10px;
    text-box: trim-both cap alphabetic;
  }

  .setting-why {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }

  .setting-why code {
    font-family: var(--mono);
    font-size: var(--font-size-micro);
  }

  .policy-value {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-self: end;
  }

  .setting-unmanaged {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    font-style: normal;
  }

  .setting-clear {
    align-items: center;
    background: transparent;
    block-size: 26px;
    border: 0;
    border-radius: 50%;
    color: var(--text-muted);
    cursor: pointer;
    display: inline-flex;
    inline-size: 26px;
    justify-content: center;
    padding: 0;
  }

  .setting-clear:hover {
    background: var(--interactive-hover-layer);
    color: var(--text-primary);
  }

  .setting-clear:active {
    background: var(--interactive-pressed);
  }

  .policy-row .setting-clear {
    opacity: 0.45;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .policy-row:hover .setting-clear,
  .policy-row:focus-within .setting-clear {
    opacity: 1;
  }

  .value-select {
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
    font-size: var(--font-size-control);
    min-block-size: 28px;
    padding: 0 1.5rem 0 var(--space-2);
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

  /* A block row keeps the grid for its say and lays its entries on a
     full-width second line. The extra breathing room lives INSIDE the row,
     above the entries - the block padding stays the shared 8px so the air
     around every hairline is the same on both sides. */
  .pattern-line {
    grid-column: 1 / -1;
    margin-block: var(--space-1) 0;
  }

  .num-inline {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font-family: var(--mono);
    font-size: var(--font-size-control);
    min-block-size: 28px;
    padding: 0 var(--space-2);
    text-align: end;
    width: 8.5rem;
  }

  .num-inline::placeholder {
    color: var(--text-muted);
  }

  .num-inline:focus-visible {
    border-color: var(--brand-action);
    outline: 2px solid var(--brand);
  }

  .gate-note {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    margin: var(--space-3) 0 0;
    padding: var(--space-2) var(--space-3);
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
    line-height: 1;
    text-box: trim-both cap alphabetic;
  }

  .f-copy code {
    color: var(--text-muted);
    display: block;
    font-size: var(--font-size-compact);
    line-height: 1;
    margin-top: 0.8rem;
    text-box: trim-both cap alphabetic;
  }

  /* Trimmed like the two lines above it, and for the same reason: the card
     centres its copy block as a BOX, so the block's box has to equal its ink or
     the centring is of something the reader cannot see. */
  .f-copy p {
    color: var(--danger);
    font-size: var(--font-size-compact);
    line-height: 1;
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
     bypass row, separated by the same drawn hairline the rows use. */
  .file-overrides {
    border-top: 1px solid var(--border-subtle);
  }

  .file-problem strong,
  .form-error {
    color: var(--stop);
  }
</style>
