<script lang="ts">
  import { durationParts, formatDuration, type DurationUnit } from '../duration';
  import type { ConfigPatch, PanelTarget, PendingCIMode, TargetSettingsInput } from '../types';
  import ClippedLabel from './ClippedLabel.svelte';
  import ConfigEditor from './ConfigEditor.svelte';
  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import Popover from './Popover.svelte';
  import Switch from './Switch.svelte';

  const PENDING_CI_CHOICES = [
    { value: 'checks', label: 'Checks' },
    { value: 'labels', label: 'Labels' },
  ] as const;

  const {
    target,
    readOnly = false,
    onUpdate,
  }: {
    target: PanelTarget;
    readOnly?: boolean;
    onUpdate: (input: TargetSettingsInput) => Promise<void>;
  } = $props();

  let saving = $state(false);
  let failure = $state<string | null>(null);
  const frozen = $derived(readOnly || saving);
  const pendingCIPermissionsReady = $derived(
    target.pending_ci_permissions.checks_write &&
      target.pending_ci_permissions.administration_write,
  );
  const mergePill = $derived(
    target.pending_ci_mode_default === 'checks'
      ? pendingCIPermissionsReady
        ? { tone: 'pill-success', word: 'App permissions ready' }
        : { tone: 'pill-warning', word: 'Approval required' }
      : { tone: 'pill-muted', word: 'Compatibility mode' },
  );

  /* ---------- The saved receipts, one per card ---------- */

  let repoSavedOn = $state(false);
  let mergeSavedOn = $state(false);
  let repoTimer: ReturnType<typeof setTimeout> | undefined;
  let mergeTimer: ReturnType<typeof setTimeout> | undefined;

  function whisper(card: 'repo' | 'merge'): void {
    if (card === 'repo') {
      repoSavedOn = true;
      clearTimeout(repoTimer);
      repoTimer = setTimeout(() => (repoSavedOn = false), 1400);
    } else {
      mergeSavedOn = true;
      clearTimeout(mergeTimer);
      mergeTimer = setTimeout(() => (mergeSavedOn = false), 1400);
    }
  }

  function settingsInput(overrides: Partial<TargetSettingsInput>): TargetSettingsInput {
    return {
      repository_default_enabled: target.repository_default_enabled,
      pending_ci_mode_default: target.pending_ci_mode_default,
      pending_ci_branch_patterns_default: target.pending_ci_branch_patterns_default,
      pending_ci_quiet_period_seconds_override: target.pending_ci_quiet_period_seconds_override,
      path_index_interval_seconds_override: target.path_index_interval_seconds_override,
      config_patch: target.config_patch,
      expected_revision: target.revision,
      ...overrides,
    };
  }

  async function push(
    card: 'repo' | 'merge',
    overrides: Partial<TargetSettingsInput>,
  ): Promise<void> {
    if (saving) return;
    saving = true;
    failure = null;
    try {
      await onUpdate(settingsInput(overrides));
      whisper(card);
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    } finally {
      saving = false;
    }
  }

  /* The behavior and command cards report through their own receipts; this
     card's part is only to carry the change and surface a refusal. */
  async function updateConfig(configPatch: ConfigPatch): Promise<void> {
    failure = null;
    try {
      await onUpdate(settingsInput({ config_patch: configPatch }));
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
      throw error;
    }
  }

  function setMode(mode: PendingCIMode): void {
    if (mode === target.pending_ci_mode_default) return;
    void push('merge', { pending_ci_mode_default: mode });
  }

  function setIncludes(next: string[]): void {
    if (next.length === 0) {
      failure = 'At least one protected ref is required';
      return;
    }
    void push('merge', {
      pending_ci_branch_patterns_default: {
        include: next,
        exclude: target.pending_ci_branch_patterns_default.exclude,
      },
    });
  }

  function setExcludes(next: string[]): void {
    void push('merge', {
      pending_ci_branch_patterns_default: {
        include: target.pending_ci_branch_patterns_default.include,
        exclude: next,
      },
    });
  }

  /* ---------- The quiet-period seconds, saved after a typing rest ---------- */

  const SAVE_REST_MS = 900;
  let quietDraft = $state<string | null>(null);
  let quietTimer: ReturnType<typeof setTimeout> | undefined;
  const quietShown = $derived(
    quietDraft ?? target.pending_ci_quiet_period_seconds_override?.toString() ?? '',
  );

  function typeQuiet(value: string): void {
    quietDraft = value;
    clearTimeout(quietTimer);
    quietTimer = setTimeout(saveQuiet, SAVE_REST_MS);
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
  let indexTimer: ReturnType<typeof setTimeout> | undefined;

  function indexParts(): { amount: number; unit: DurationUnit } {
    const seconds =
      target.path_index_interval_seconds_override ?? target.path_index_interval_seconds_inherited;
    return durationParts(seconds, PATH_INDEX_UNITS);
  }

  const indexAmountShown = $derived(indexAmountDraft ?? indexParts().amount.toString());
  const indexUnitShown = $derived(indexUnitDraft ?? indexParts().unit);

  function typeIndexAmount(value: string): void {
    indexAmountDraft = value;
    indexUnitDraft = indexUnitShown;
    clearTimeout(indexTimer);
    indexTimer = setTimeout(saveIndexDraft, SAVE_REST_MS);
  }

  function pickIndexUnit(unit: DurationUnit): void {
    indexAmountDraft = indexAmountShown;
    indexUnitDraft = unit;
    saveIndexDraft();
  }

  function saveIndexDraft(): void {
    clearTimeout(indexTimer);
    indexTimer = undefined;
    if (indexAmountDraft === null || indexUnitDraft === null) return;
    const seconds = Math.round(Number(indexAmountDraft) * UNIT_SECONDS[indexUnitDraft]);
    if (!Number.isFinite(seconds) || seconds < 60) return;
    indexAmountDraft = null;
    indexUnitDraft = null;
    void push('repo', { path_index_interval_seconds_override: seconds });
  }

  function saveQuiet(): void {
    clearTimeout(quietTimer);
    quietTimer = undefined;
    if (quietDraft === null) return;
    const trimmed = quietDraft.trim();
    const quiet = trimmed === '' ? null : Number(trimmed);
    if (quiet !== null && (!Number.isInteger(quiet) || quiet < 0 || quiet > 86_400)) {
      failure = 'Quiet period must be whole seconds from 0 to 86400';
      return;
    }
    if (quiet === target.pending_ci_quiet_period_seconds_override) {
      quietDraft = null;
      return;
    }
    quietDraft = null;
    void push('merge', { pending_ci_quiet_period_seconds_override: quiet });
  }
</script>

<div class="view-frame">
  <section class="settings-page" aria-labelledby="settings-heading">
    <div class="kind-head">
      <div class="kind-head-say">
        <h2 class="card-title" id="settings-heading">Settings</h2>
        <p class="kind-head-sub">
          Defaults every repository inherits unless a repository overrides them
        </p>
      </div>
    </div>

    {#if failure !== null}
      <FormError message={failure} />
    {/if}

    <section class="card group-card" aria-labelledby="settings-repositories">
      <div class="group-head">
        <h3 class="group-name" id="settings-repositories">Repositories</h3>
        <span class="save-whisper" class:is-on={repoSavedOn} role="status"
          ><Icon name="check" size={12} /><span class="t">Saved</span></span
        >
      </div>
      <div class="policy-rows">
        <div class="policy-row">
          <span class="setting-say">
            <span class="setting-name">Unconfigured repositories</span>
            <span class="setting-why"
              >How Smyklot treats repositories that don't have their own setting yet. New
              installations start disabled, so nothing runs before you decide</span
            >
          </span>
          <span class="policy-value">
            <span class="value-word" class:is-on={target.repository_default_enabled}
              >{target.repository_default_enabled ? 'On' : 'Off'}</span
            >
            <Switch
              checked={target.repository_default_enabled}
              label="Unconfigured repositories"
              disabled={frozen}
              onToggle={(next) => void push('repo', { repository_default_enabled: next })}
            />
          </span>
        </div>
        <div class="policy-row">
          <span class="setting-say">
            <span class="setting-name">Path index</span>
            <span class="setting-why"
              >How often each repository's file list is read again for the finder and the plans</span
            >
          </span>
          {#if target.path_index_interval_seconds_override === null}
            <span class="policy-value">
              <span class="setting-unmanaged"
                >Follows the deployment - every {formatDuration(
                  durationParts(target.path_index_interval_seconds_inherited, PATH_INDEX_UNITS),
                )}</span
              >
            </span>
            <button
              class="setting-clear"
              title="Answer for this workspace"
              disabled={frozen}
              onclick={() =>
                void push('repo', {
                  path_index_interval_seconds_override:
                    target.path_index_interval_seconds_inherited,
                })}
            >
              <Icon name="plus" size={10} />
            </button>
          {:else}
            <span class="policy-value">
              <input
                class="num-inline num-short"
                inputmode="numeric"
                aria-label="Path index interval amount"
                value={indexAmountShown}
                disabled={readOnly}
                oninput={(event) => typeIndexAmount(event.currentTarget.value)}
                onblur={saveIndexDraft}
              />
              <Popover
                role="listbox"
                label="Path index interval unit"
                align="end"
                itemSelector=".menu-item"
              >
                {#snippet trigger(attributes)}
                  <button
                    {...attributes}
                    class="value-select"
                    type="button"
                    aria-label="Path index interval unit"
                    disabled={frozen}
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
                        {#if indexUnitShown === unit}<Icon name="check" size={16} />{/if}
                      </span>
                      <ClippedLabel class="mi-label" text={unit} />
                    </button>
                  {/each}
                </div>
              </Popover>
            </span>
            <button
              class="setting-clear"
              title="Stop answering - follow the deployment"
              disabled={frozen}
              onclick={() => void push('repo', { path_index_interval_seconds_override: null })}
            >
              <Icon name="close" size={10} />
            </button>
          {/if}
        </div>
      </div>
    </section>

    <section class="card group-card" aria-labelledby="settings-merge-ci">
      <div class="group-head">
        <h3 class="group-name" id="settings-merge-ci">Merge after CI</h3>
        <span class="save-whisper" class:is-on={mergeSavedOn} role="status"
          ><Icon name="check" size={12} /><span class="t">Saved</span></span
        >
        <span class="pill {mergePill.tone}"><span class="t">{mergePill.word}</span></span>
      </div>
      <div class="policy-rows">
        <div class="policy-row">
          <span class="setting-say">
            <span class="setting-name">Repository protection</span>
            <span class="setting-why"
              >Checks mode creates an app-bound required check and merges the exact authorized head</span
            >
          </span>
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
                  disabled={frozen}
                >
                  <span class="t"
                    >{target.pending_ci_mode_default === 'checks' ? 'Checks' : 'Labels'}</span
                  >
                </button>
              {/snippet}
              <div class="menu-list">
                {#each PENDING_CI_CHOICES as option (option.value)}
                  <button
                    class="menu-item"
                    role="option"
                    aria-selected={target.pending_ci_mode_default === option.value}
                    onclick={() => setMode(option.value)}
                  >
                    <span class="menu-check">
                      {#if target.pending_ci_mode_default === option.value}<Icon
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
        </div>
        <div class="policy-row policy-block">
          <span class="setting-say">
            <span class="setting-name">Protected refs</span>
            <span class="setting-why"
              >Raw GitHub ruleset patterns, such as <code>~DEFAULT_BRANCH</code>. At least one is
              required</span
            >
          </span>
          <div class="pattern-line">
            <PatternEntries
              patterns={target.pending_ci_branch_patterns_default.include}
              readOnly={frozen}
              onChange={setIncludes}
            />
          </div>
        </div>
        <div class="policy-row policy-block">
          <span class="setting-say">
            <span class="setting-name">Excluded refs</span>
            <span class="setting-why"
              >Optional patterns that should keep the inherited merge behavior</span
            >
          </span>
          <div class="pattern-line">
            <PatternEntries
              patterns={target.pending_ci_branch_patterns_default.exclude}
              readOnly={frozen}
              onChange={setExcludes}
            />
          </div>
        </div>
        <div class="policy-row">
          <span class="setting-say">
            <label class="setting-name" for="settings-quiet-period">Stable passing window</label>
            <span class="setting-why"
              >Seconds. Zero still requires two matching passing observations</span
            >
          </span>
          <span class="policy-value">
            <input
              id="settings-quiet-period"
              class="num-inline"
              inputmode="numeric"
              placeholder={target.pending_ci_quiet_period_seconds_inherited.toString()}
              value={quietShown}
              disabled={readOnly}
              oninput={(event) => typeQuiet(event.currentTarget.value)}
              onblur={saveQuiet}
            />
          </span>
        </div>
      </div>
      {#if target.pending_ci_mode_default === 'checks' && !pendingCIPermissionsReady}
        <p class="perm-note" role="status">
          Grant Checks write and Administration write to activate checks mode. Repositories remain
          blocked until GitHub approves both permissions.
        </p>
      {/if}
    </section>

    <ConfigEditor
      patch={target.config_patch}
      inherited={target.inherited_config}
      scope="target"
      idPrefix={target.id}
      disabled={readOnly}
      onSave={updateConfig}
    />
  </section>
</div>

<style>
  .view-frame {
    margin-inline: auto;
    max-width: var(--content-max);
  }

  .settings-page {
    display: grid;
    gap: var(--space-4);
  }

  .kind-head {
    align-items: start;
    display: flex;
    gap: var(--space-4);
    justify-content: space-between;
  }

  .kind-head-say {
    display: grid;
    gap: var(--space-2);
  }

  .card-title {
    font-size: var(--font-size-card-title);
    font-weight: 600;
    margin: 0;
    min-block-size: 13px;
    text-box: trim-both cap alphabetic;
  }

  .kind-head-sub {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    margin: 0;
    max-width: 72ch;
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
    /* The air around a drawn hairline is the card's own padding, on both
       sides; the edge rows shed it where no line follows, since the card
       edge already carries that inset. */
    padding: var(--space-5) var(--space-2);
    position: relative;
  }

  .policy-row:first-child {
    padding-block-start: var(--space-2);
  }

  .policy-row:last-child {
    padding-block-end: var(--space-2);
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

  .value-word {
    color: var(--text-muted);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    min-inline-size: 1.9rem;
    text-align: end;
    text-box: trim-both cap alphabetic;
  }

  .value-word.is-on {
    color: var(--text-secondary);
    font-weight: 600;
  }

  .setting-unmanaged {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    font-style: normal;
    /* Ink-true, so the padding around the hairlines measures to the glyphs
       rather than to the line box's leading. */
    text-box: trim-both cap alphabetic;
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
    min-block-size: 28px;
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

  .num-inline.num-short {
    width: 5rem;
  }

  .num-inline::placeholder {
    color: var(--text-muted);
  }

  .num-inline:focus-visible {
    border-color: var(--brand-action);
    outline: 2px solid var(--brand);
  }

  .perm-note {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    color: var(--warning);
    font-size: var(--font-size-meta);
    /* Ink-true with even padding, so the words sit on the note's centre. */
    line-height: round(1.5em, 1px);
    margin: var(--space-3) 0 0;
    padding: var(--space-3);
    text-box: trim-both cap alphabetic;
  }

  /* On a phone the head's three parts cannot share one line - the tally or
     pill drops under the title instead of holding the card wide. */
  @media (max-width: 30rem) {
    .group-head {
      flex-wrap: wrap;
    }

    /* The say keeps the line and the control moves under it - beside it,
       the copy was down to a word a line while the control still ran off
       the screen and took the layout viewport with it. */
    .policy-row {
      grid-template-columns: minmax(0, 1fr) auto;
    }

    .policy-row .setting-say {
      grid-column: 1;
      grid-row: 1;
    }

    .policy-row .setting-clear {
      grid-column: 2;
      grid-row: 1;
      opacity: 1;
    }

    .policy-row .policy-value {
      flex-wrap: wrap;
      grid-column: 1 / -1;
      justify-self: start;
    }
  }
</style>
