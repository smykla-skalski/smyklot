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
  import Chip, { type ChipTone } from './Chip.svelte';
  import Button from './Button.svelte';
  import ConfigEditor from './ConfigEditor.svelte';
  import HelpTip from './HelpTip.svelte';
  import Icon from './Icon.svelte';
  import InheritControl from './InheritControl.svelte';
  import BackLink from './BackLink.svelte';
  import PageHeader from './PageHeader.svelte';
  import Plate from './Plate.svelte';
  import RepositorySyncPane from './RepositorySyncPane.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  /**
   * One repository's own page.
   *
   * This was a dialog until it was three panes with a save bar in each, which is
   * a screen someone works in rather than something standing over the list for a
   * moment. It reads as any other page of the panel does - a way back, a header
   * with the switch beside it, then the pane - and it is addressable, so a link
   * points at the pane a colleague was asked to look at.
   */

  const FILE_MODE_OPTIONS = [
    { value: 'observe', label: 'Observe' },
    { value: 'bypass', label: 'Bypass' },
  ] as const;
  const FILE_STATUS_TONES = {
    valid: 'clear',
    missing: 'neutral',
    invalid: 'stop',
    bypassed: 'warning',
  } as const satisfies Record<RepositoryFileStatus, ChipTone>;

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
    onBypass: (bypass: boolean) => void;
    /* The editor awaits this to know when its save bar can settle, so the
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
  const PENDING_CI_MODE_OPTIONS = [
    { value: 'checks', label: 'Checks' },
    { value: 'labels', label: 'Labels' },
  ] as const;
  const GATE_TONES = {
    ready: 'clear',
    provisioning: 'neutral',
    draining: 'warning',
    blocked: 'stop',
  } as const satisfies Record<'ready' | 'provisioning' | 'draining' | 'blocked', ChipTone>;
  let savingPendingCI = $state(false);
  let pendingCIMode = $state<PendingCIMode | null>(null);
  let overridePendingCIPatterns = $state(false);
  let pendingCIIncludes = $state('');
  let pendingCIExcludes = $state('');
  let pendingCIQuiet = $state('');

  $effect(() => {
    if (detail === undefined || savingPendingCI) return;
    pendingCIMode = detail.pending_ci_mode_override;
    overridePendingCIPatterns = detail.pending_ci_branch_patterns_override !== null;
    const patterns =
      detail.pending_ci_branch_patterns_override ?? detail.pending_ci_branch_patterns_inherited;
    pendingCIIncludes = patterns.include.join('\n');
    pendingCIExcludes = patterns.exclude.join('\n');
    pendingCIQuiet = detail.pending_ci_quiet_period_seconds_override?.toString() ?? '';
  });

  function pendingCILines(value: string): string[] {
    return value
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line !== '');
  }

  async function savePendingCI(): Promise<void> {
    if (detail === undefined || savingPendingCI) return;
    const includes = pendingCILines(pendingCIIncludes);
    if (overridePendingCIPatterns && includes.length === 0) return;
    const quiet = pendingCIQuiet.trim() === '' ? null : Number(pendingCIQuiet);
    if (quiet !== null && (!Number.isInteger(quiet) || quiet < 0 || quiet > 86_400)) return;
    savingPendingCI = true;
    try {
      await onSavePendingCI(
        pendingCIMode,
        overridePendingCIPatterns
          ? { include: includes, exclude: pendingCILines(pendingCIExcludes) }
          : null,
        quiet,
      );
    } finally {
      savingPendingCI = false;
    }
  }

  /* The repository-file pane lists the behavior settings this repository
     actually overrides, the way the approved design draws it: the file card, the
     bypass control, then whatever this repo has changed, with its own save bar.
     Someone reading the file pane is asking "what does this repository do
     differently", and the answer belongs on the same screen as the file. */
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
</script>

<section class="repository-page" aria-labelledby={titleId}>
  <BackLink href={backHref} label="Repositories" onNavigate={onBack} />

  <PageHeader
    id={titleId}
    title={repository.name}
    description="Repository settings override workspace defaults and repository-file values"
  >
    {#snippet actions()}
      {#if detail !== undefined}
        <SegmentedControl
          name="repository-{repository.id}-section"
          label="Settings for {repository.name}"
          compact
          options={sections.map((pane) => ({
            value: pane,
            label: SECTION_LABELS[pane],
            badge: sectionBadge(detail, pane),
          }))}
          value={section}
          onSelect={(next) => onSection(next as RepositorySection)}
        />
      {/if}
    {/snippet}
  </PageHeader>

  {#if failure !== null}
    <p class="form-error repository-page-error" role="alert">{failure}</p>
  {/if}

  {#if detail === undefined}
    <p class="detail-loading dim" role="status">Reading repository settings…</p>
  {:else}
    <Plate label="Merge after CI">
      {#snippet status()}
        {#if detail.pending_ci_gate !== undefined}
          <Chip small tone={GATE_TONES[detail.pending_ci_gate.readiness]} dot>
            {detail.pending_ci_gate.readiness.slice(0, 1).toUpperCase() +
              detail.pending_ci_gate.readiness.slice(1)}
          </Chip>
        {/if}
      {/snippet}
      <form
        class="pending-ci-form"
        onsubmit={(event) => {
          event.preventDefault();
          void savePendingCI();
        }}
      >
        <div class="pending-ci-row">
          <div>
            <strong>Repository protection</strong>
            <p>
              {pendingCIMode === null
                ? `Inherited ${detail.pending_ci_mode_inherited} mode`
                : 'This repository overrides the workspace mode'}
            </p>
          </div>
          <InheritControl
            label="Merge after CI representation"
            source="workspace settings"
            inheritedValue={detail.pending_ci_mode_inherited}
            inheritedLabel={detail.pending_ci_mode_inherited}
            value={pendingCIMode}
            options={PENDING_CI_MODE_OPTIONS}
            disabled={disabled || savingPendingCI}
            onSelect={(value) => (pendingCIMode = value as PendingCIMode)}
            onRestore={() => (pendingCIMode = null)}
          />
        </div>
        {#if detail.pending_ci_gate !== undefined}
          <p class:gate-problem={detail.pending_ci_gate.readiness === 'blocked'} class="gate-note">
            {detail.pending_ci_gate.reason}
          </p>
        {/if}
        <label class="override-check">
          <input
            type="checkbox"
            bind:checked={overridePendingCIPatterns}
            disabled={disabled || savingPendingCI}
          />
          Override protected branch patterns
        </label>
        <div class="pending-ci-grid">
          <label>
            <span>Protected refs</span>
            <textarea
              rows="3"
              bind:value={pendingCIIncludes}
              disabled={disabled || savingPendingCI || !overridePendingCIPatterns}></textarea>
            <small>One raw GitHub ruleset pattern per line.</small>
          </label>
          <label>
            <span>Excluded refs</span>
            <textarea
              rows="3"
              bind:value={pendingCIExcludes}
              disabled={disabled || savingPendingCI || !overridePendingCIPatterns}></textarea>
          </label>
          <label>
            <span>Stable passing window</span>
            <input
              type="number"
              min="0"
              max="86400"
              step="1"
              bind:value={pendingCIQuiet}
              disabled={disabled || savingPendingCI}
              placeholder={detail.pending_ci_quiet_period_seconds_inherited?.toString() ??
                'Global default'}
            />
            <small>Seconds; leave blank to inherit.</small>
          </label>
        </div>
        <div class="pending-ci-actions">
          <Button type="submit" tone="brand" disabled={disabled || savingPendingCI}>
            {savingPendingCI ? 'Saving…' : 'Save merge settings'}
          </Button>
        </div>
      </form>
    </Plate>
    <div
      class="repository-detail-content"
      role="group"
      aria-label="{sectionLabel(section)} settings for {repository.name}"
    >
      {#if section === 'file'}
        <Plate label="Repository file">
          {#snippet status()}
            <div class="pane-status">
              <Chip small tone={FILE_STATUS_TONES[detail.repository.config_file_status]} dot>
                {detail.repository.config_file_status.slice(0, 1).toUpperCase() +
                  detail.repository.config_file_status.slice(1)}
              </Chip>
              <HelpTip
                id="repository-file-help-{repository.id}"
                label="About the repository file"
                text="Settings Smyklot reads from the repository itself, which override account defaults"
              />
            </div>
          {/snippet}
          <div class="file-pane">
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
            <div class="override-row">
              <span class="o-label">
                <!-- Trimmed, so the words centre against the 18px help slot on
                   their caps rather than on a taller line box. -->
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
                {disabled}
                compact
                onSelect={(value) => onBypass(value === 'bypass')}
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
                {disabled}
                onSave={onSaveConfig}
              />
            {/if}
          </div>
        </Plate>
      {:else if section === 'sync'}
        {#if syncOverride === undefined && syncReadProblem !== null}
          <!-- A read that failed is not a read still going, and the two read
               identically in a dim line saying "Reading…". -->
          <p class="form-error" role="alert">{syncReadProblem}</p>
        {:else if syncOverride === undefined}
          <p class="detail-loading dim" role="status">Reading what this repository adjusts…</p>
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
        {@const count = sectionCount(detail, section)}
        <Plate label={section === 'behavior' ? 'Behavior overrides' : 'Command overrides'}>
          {#snippet status()}
            <div class="pane-status">
              {#if count > 0}
                <Chip small>{count} {count === 1 ? 'override' : 'overrides'}</Chip>
              {/if}
              <HelpTip
                id="repository-overrides-{repository.id}-{section}"
                label="About repository overrides"
                text="Only settings changed here override configuration defaults from Settings and repository-file settings"
              />
            </div>
          {/snippet}
          <ConfigEditor
            patch={detail.config_patch}
            inherited={detail.inherited_config}
            scope="repository"
            idPrefix={repository.id}
            {section}
            {disabled}
            onSave={onSaveConfig}
          />
        </Plate>
      {/if}
    </div>
  {/if}
</section>

<style>
  .repository-page {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  /* The same way back the console's installation page draws, so the two detail
     pages read as one anatomy: a chevron, a word, and the list it returns to. */
  /* The page is titled by the repository name, which is code, so it sets in
     mono. `PageHeader` stamps the id on the heading itself. */
  :global(#repository-page-title) {
    font-family: var(--mono);
  }

  .repository-page-error {
    margin: 0 0 var(--space-3);
  }

  .detail-loading {
    margin: 0;
    padding: var(--space-4);
  }

  .repository-detail-content {
    min-width: 0;
  }

  .pending-ci-form,
  .pending-ci-grid,
  .pending-ci-grid label {
    display: grid;
    gap: var(--space-3);
  }

  .pending-ci-form {
    padding: var(--space-4);
  }

  .pending-ci-row {
    align-items: center;
    display: flex;
    gap: var(--space-4);
    justify-content: space-between;
  }

  .pending-ci-row p,
  .gate-note,
  .pending-ci-grid small {
    color: var(--dim);
    font-size: var(--font-size-meta);
    margin: 0.25rem 0 0;
  }

  .gate-note {
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    padding: var(--space-3);
    text-box: trim-both cap alphabetic;
  }

  .gate-note.gate-problem {
    color: var(--danger);
  }

  .override-check {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }

  .pending-ci-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .pending-ci-grid label:last-child {
    grid-column: 1 / -1;
    max-width: 20rem;
  }

  .pending-ci-grid :is(textarea, input[type='number']) {
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    color: var(--text);
    font: var(--font-size-body) / 1.4 var(--mono);
    padding: 0.625rem 0.75rem;
  }

  .pending-ci-actions {
    display: flex;
    justify-content: flex-end;
  }

  @media (max-width: 40rem) {
    .pending-ci-row {
      align-items: start;
      flex-direction: column;
    }

    .pending-ci-grid {
      grid-template-columns: 1fr;
    }

    .pending-ci-grid label:last-child {
      grid-column: auto;
    }
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
    color: var(--dim);
    display: block;
    font-size: var(--font-size-compact);
    line-height: 1;
    margin-top: 0.8rem;
    text-box: trim-both cap alphabetic;
  }

  /* Trimmed like the two lines above it, and for the same reason: the card
     centres its copy block as a BOX, so the block's box has to equal its ink or
     the centring is of something the reader cannot see. Untrimmed, this third
     line carried its own leading and descender room below the words and pulled
     the block 2.49px off the icon beside it - measured by the alignment sweep
     once the repository page was added to the routes it walks. */
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
    color: var(--dim);
  }

  /* An inline continuation of the sentence above it, not a control in its own
     right: it sits on the same line, at the same size, and is underlined the way
     a link in prose is. Giving it a button's chrome would make refusing a
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
     above them, not the flush list style the Behavior pane uses - on this pane
     they are cards in a stack, not rows in a table. 0.875rem is the pane's stack
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

  /* The status corner of a pane's plate: whatever the pane has to report, then
     its help. The same shape the account's own Settings plate uses, because
     these are the same settings one level down. */
  .pane-status {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }

  .file-problem strong,
  .form-error {
    color: var(--stop);
  }
</style>
