<script lang="ts">
  import { untrack } from 'svelte';

  import { CONFIG_KEYS } from '../config';
  import {
    FORMATTING_FIELDS,
    formattingOverrideCount,
    type FormattingFieldKey,
    type FormattingPatch,
  } from '../formatting';
  import { durationParts, formatDuration, type DurationUnit } from '../duration';
  import { getSettingsDraftRegistry, type SettingsScope } from '../settings-drafts.svelte';
  import {
    buildTargetDefaultsDocument,
    overlayTargetDefaultsDocument,
    parseTargetDefaultsDocument,
    stageTargetDefaultsControl,
    targetDefaultsDraftDocument,
    targetDefaultsResource,
    type TargetDefaultsControlId,
  } from '../target-defaults-settings';
  import type { PanelApi } from '../api';
  import type { ConfigKey, ConfigPatch, PanelTarget, PendingCIMode } from '../types';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import ClippedLabel from './ClippedLabel.svelte';
  import ConfigEditor from './ConfigEditor.svelte';
  import FormattingEditor from './FormattingEditor.svelte';
  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import PageHeader from './PageHeader.svelte';
  import PageToc, { type TocEntry } from './PageToc.svelte';
  import Popover from './Popover.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import WorkspaceTiming from './WorkspaceTiming.svelte';

  const PENDING_CI_CHOICES = [
    { value: 'checks', label: 'Checks' },
    { value: 'labels', label: 'Labels' },
  ] as const;

  const ARRIVAL_CHOICES = [
    { value: 'off', label: 'Start off' },
    { value: 'on', label: 'Start on' },
  ] as const;

  /* The page's own sections, in the order they are written. The formatting card is the
     one the design does not model - the rules it holds are real and reachable nowhere
     else, so it is indexed like the rest rather than left off the list. */
  const TOC: readonly TocEntry[] = [
    { id: 'ws-newrepos', label: 'New repositories' },
    { id: 'ws-merging', label: 'Merging' },
    { id: 'ws-behavior', label: 'Behavior' },
    { id: 'ws-commands', label: 'Commands' },
    { id: 'ws-formatting', label: 'Formatting' },
    { id: 'ws-timing', label: 'Timing' },
  ];

  const {
    target: canonicalTarget,
    readOnly = false,
    timing,
  }: {
    target: PanelTarget;
    readOnly?: boolean;
    /**
     * What the Timing card needs to say when Smyklot acts and to carry a request to the
     * operators. Absent where the page is rendered outside the shell - a story, a
     * component spec - and the card then holds the settings this page owns and no more.
     */
    timing?: { api: PanelApi; canRequest: boolean };
  } = $props();

  const drafts = getSettingsDraftRegistry();
  const resource = $derived(targetDefaultsResource(canonicalTarget.id));
  const settingsScope = $derived({
    type: 'workspace',
    targetId: canonicalTarget.id,
  } as const satisfies SettingsScope);
  const document = $derived(targetDefaultsDraftDocument(drafts, canonicalTarget));
  const target = $derived(overlayTargetDefaultsDocument(canonicalTarget, document));
  let failure = $state<string | null>(null);
  const frozen = $derived(readOnly);
  const dirtyConfigKeys = $derived(
    CONFIG_KEYS.filter((key) => controlDirty(`defaults.config_patch.${key}`)),
  );
  const dirtyFormattingKeys = $derived(
    FORMATTING_FIELDS.filter((field) => controlDirty(`defaults.config_patch.${field.key}`)).map(
      (field) => field.key,
    ),
  );
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

  $effect(() => {
    const revision = canonicalTarget.revision;
    const base = buildTargetDefaultsDocument(canonicalTarget);
    untrack(() => drafts.adoptBase(resource, revision, base));
  });

  function controlDirty(controlId: TargetDefaultsControlId): boolean {
    return drafts.isControlDirty(settingsScope, controlId);
  }

  function stage(nextValue: unknown, controlId: TargetDefaultsControlId): boolean {
    const next = parseTargetDefaultsDocument(nextValue);
    if (next === null || !stageTargetDefaultsControl(drafts, canonicalTarget, next, controlId)) {
      failure = 'This setting is not valid';
      return false;
    }
    failure = null;
    return true;
  }

  function updateConfig(configPatch: ConfigPatch, changedKey: ConfigKey): void {
    stage({ ...document, config_patch: configPatch }, `defaults.config_patch.${changedKey}`);
  }

  function updateFormatting(formatting: FormattingPatch, changedKey: FormattingFieldKey): void {
    const configPatch: ConfigPatch = { ...target.config_patch };
    if (formattingOverrideCount(formatting) === 0) delete configPatch.formatting;
    else configPatch.formatting = formatting;
    stage({ ...document, config_patch: configPatch }, `defaults.config_patch.${changedKey}`);
  }

  function setFormattingValidity(valid: boolean): void {
    drafts.setValidationProblem(
      settingsScope,
      'defaults.config_patch.formatting',
      valid ? null : 'Formatting widths must be whole numbers within their documented bounds',
    );
  }

  function setMode(mode: PendingCIMode): void {
    if (mode === target.pending_ci_mode_default) return;
    stage({ ...document, pending_ci_mode_default: mode }, 'defaults.pending_ci_mode_default');
  }

  function setIncludes(next: string[]): void {
    if (next.length === 0) {
      failure = 'At least one protected ref is required';
      return;
    }
    stage(
      {
        ...document,
        pending_ci_branch_patterns_default: {
          include: next,
          exclude: target.pending_ci_branch_patterns_default.exclude,
        },
      },
      'defaults.pending_ci_branch_patterns_default.include',
    );
  }

  function setExcludes(next: string[]): void {
    stage(
      {
        ...document,
        pending_ci_branch_patterns_default: {
          include: target.pending_ci_branch_patterns_default.include,
          exclude: next,
        },
      },
      'defaults.pending_ci_branch_patterns_default.exclude',
    );
  }

  let quietDraft = $state<string | null>(null);
  const quietShown = $derived(
    quietDraft ?? target.pending_ci_quiet_period_seconds_override?.toString() ?? '',
  );

  function typeQuiet(value: string): void {
    quietDraft = value;
    const trimmed = value.trim();
    const quiet = trimmed === '' ? null : Number(trimmed);
    if (quiet !== null && (!Number.isInteger(quiet) || quiet < 0 || quiet > 86_400)) {
      failure = 'Quiet period must be whole seconds from 0 to 86400';
      return;
    }
    stage(
      { ...document, pending_ci_quiet_period_seconds_override: quiet },
      'defaults.pending_ci_quiet_period_seconds_override',
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
      target.path_index_interval_seconds_override ?? target.path_index_interval_seconds_inherited;
    return durationParts(seconds, PATH_INDEX_UNITS);
  }

  const indexAmountShown = $derived(indexAmountDraft ?? indexParts().amount.toString());
  const indexUnitShown = $derived(indexUnitDraft ?? indexParts().unit);

  function typeIndexAmount(value: string): void {
    indexAmountDraft = value;
    const unit = indexUnitShown;
    indexUnitDraft = unit;
    saveIndexDraft(value, unit);
  }

  function pickIndexUnit(unit: DurationUnit): void {
    const amount = indexAmountShown;
    indexAmountDraft = amount;
    indexUnitDraft = unit;
    if (saveIndexDraft(amount, unit)) {
      indexAmountDraft = null;
      indexUnitDraft = null;
    }
  }

  function saveIndexDraft(amount: string, unit: DurationUnit): boolean {
    const seconds = Math.round(Number(amount) * UNIT_SECONDS[unit]);
    if (!Number.isFinite(seconds) || seconds < 60 || seconds > 604_800) {
      failure = 'File index interval must be from 1 minute to 7 days';
      return false;
    }
    return stage(
      { ...document, path_index_interval_seconds_override: seconds },
      'defaults.path_index_interval_seconds_override',
    );
  }

  function finishIndexDraft(): void {
    if (indexAmountDraft === null || indexUnitDraft === null) return;
    if (saveIndexDraft(indexAmountDraft, indexUnitDraft)) {
      indexAmountDraft = null;
      indexUnitDraft = null;
    }
  }
</script>

<!--
@component
What a workspace does by default, which every repository inside it inherits until it
says otherwise. This is the top of the settings chain the panel exposes, so a value set
here is the one a repository's editor shows as inherited.

`readOnly` keeps the page whole and closes the controls. A member who can see how their
workspace is configured without being able to change it is a real reader, and hiding the
settings from them answers a different question than the one they asked.
-->

<div class="view-frame">
  <div class="page-main">
    <section class="settings-page card-stack" aria-labelledby="defaults-heading">
      <PageHeader
        id="defaults-heading"
        title="Workspace settings"
        description="What every repository here inherits, unless one overrides it for itself"
      />

      {#if failure !== null}
        <FormError message={failure} />
      {/if}

      <Card id="ws-newrepos" labelledby="settings-repositories">
        <div class="card-head">
          <h2 class="card-title" id="settings-repositories">New repositories</h2>
        </div>
        <div class="policy-rows">
          <div
            class={[
              'policy-row',
              { 'is-unsaved': controlDirty('defaults.repository_default_enabled') },
            ]}
            data-unsaved={controlDirty('defaults.repository_default_enabled') || undefined}
          >
            <span class="setting-say">
              <span class="setting-name">When a repository appears</span>
              <span class="setting-why"
                >A repository with no setting of its own starts here, and stays there until somebody
                turns it on</span
              >
            </span>
            <span class="policy-value">
              <!-- TWO WORDS, NOT A TOGGLE. The question is what a repository Smyklot has never
                 seen should do on arrival, and "off" and "on" are the two answers to it -
                 a switch would ask instead whether the policy itself is enabled. -->
              <SegmentedControl
                name="repository-default-{canonicalTarget.id}"
                label="When a repository appears"
                options={ARRIVAL_CHOICES}
                value={target.repository_default_enabled ? 'on' : 'off'}
                disabled={frozen}
                compact
                onSelect={(next) =>
                  stage(
                    { ...document, repository_default_enabled: next === 'on' },
                    'defaults.repository_default_enabled',
                  )}
              />
            </span>
          </div>
        </div>
      </Card>

      {#snippet timingCard()}
        <details class="card fold" id="ws-timing">
          <summary>
            <Icon name="chevron-right" size="xs" />
            <h2 class="card-title">Timing</h2>
            <span class="fold-scent">When Smyklot acts, and how often - rarely changed</span>
          </summary>
          {#if timing !== undefined}
            <WorkspaceTiming
              api={timing.api}
              targetId={canonicalTarget.id}
              canRequest={timing.canRequest}
            />
          {/if}
          <div class="policy-rows">
            <div
              class={[
                'policy-row',
                { 'is-unsaved': controlDirty('defaults.path_index_interval_seconds_override') },
              ]}
              data-unsaved={controlDirty('defaults.path_index_interval_seconds_override') ||
                undefined}
            >
              <span class="setting-say">
                <span class="setting-name">File index</span>
                <span class="setting-why">How often each repository's file list is read again</span>
              </span>
              {#if target.path_index_interval_seconds_override === null}
                <span class="policy-value">
                  <span class="setting-unmanaged"
                    >From the service: every {formatDuration(
                      durationParts(target.path_index_interval_seconds_inherited, PATH_INDEX_UNITS),
                    )}</span
                  >
                  <Button
                    tone="quiet"
                    disabled={frozen}
                    onclick={() =>
                      stage(
                        {
                          ...document,
                          path_index_interval_seconds_override:
                            target.path_index_interval_seconds_inherited,
                        },
                        'defaults.path_index_interval_seconds_override',
                      )}
                  >
                    {#snippet icon()}<Icon name="plus" size="sm" />{/snippet}
                    Answer here
                  </Button>
                </span>
              {:else}
                <span class="policy-value">
                  <input
                    class="num-inline num-short"
                    inputmode="numeric"
                    aria-label="File index interval amount"
                    value={indexAmountShown}
                    disabled={frozen}
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
                        aria-label="File index interval unit"
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
                            {#if indexUnitShown === unit}<Icon name="check" size="base" />{/if}
                          </span>
                          <ClippedLabel class="mi-label" text={unit} />
                        </button>
                      {/each}
                    </div>
                  </Popover>
                  <!-- A WORD, NOT A GLYPH. The bare x asked a reader to know that this one
                     crossed out an answer rather than deleting the setting. -->
                  <Button
                    tone="quiet"
                    disabled={frozen}
                    onclick={() =>
                      stage(
                        { ...document, path_index_interval_seconds_override: null },
                        'defaults.path_index_interval_seconds_override',
                      )}>Reset</Button
                  >
                </span>
              {/if}
            </div>
          </div>
        </details>
      {/snippet}

      <Card id="ws-merging" labelledby="settings-merge-ci">
        <div class="card-head">
          <h2 class="card-title" id="settings-merge-ci">Merging</h2>
          <span class="pill {mergePill.tone}"><span class="t">{mergePill.word}</span></span>
        </div>
        <div class="policy-rows">
          <div
            class={[
              'policy-row',
              { 'is-unsaved': controlDirty('defaults.pending_ci_mode_default') },
            ]}
            data-unsaved={controlDirty('defaults.pending_ci_mode_default') || undefined}
          >
            <span class="setting-say">
              <span class="setting-name">Repository protection</span>
              <span class="setting-why"
                >Checks mode creates an app-bound required check and merges the exact authorized
                head</span
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
                    aria-label="{target.pending_ci_mode_default === 'checks'
                      ? 'Checks'
                      : 'Labels'} - repository protection"
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
                            size="base"
                          />{/if}
                      </span>
                      <ClippedLabel class="mi-label" text={option.label} />
                    </button>
                  {/each}
                </div>
              </Popover>
            </span>
          </div>
          <div
            class={[
              'policy-row',
              {
                'is-unsaved': controlDirty('defaults.pending_ci_branch_patterns_default.include'),
              },
            ]}
            data-unsaved={controlDirty('defaults.pending_ci_branch_patterns_default.include') ||
              undefined}
          >
            <span class="setting-say">
              <span class="setting-name">Protected branches</span>
              <span class="setting-why"
                >Where the protection applies - branches matching any pattern here. Raw GitHub
                ruleset patterns, such as <code>~DEFAULT_BRANCH</code>, and at least one is required</span
              >
            </span>
            <span class="policy-value setting-value-wrap">
              <PatternEntries
                patterns={target.pending_ci_branch_patterns_default.include}
                readOnly={frozen}
                onChange={setIncludes}
              />
            </span>
          </div>
          <div
            class={[
              'policy-row',
              {
                'is-unsaved': controlDirty('defaults.pending_ci_branch_patterns_default.exclude'),
              },
            ]}
            data-unsaved={controlDirty('defaults.pending_ci_branch_patterns_default.exclude') ||
              undefined}
          >
            <span class="setting-say">
              <span class="setting-name">Excluded refs</span>
              <span class="setting-why"
                >Optional patterns that should keep the inherited merge behavior</span
              >
            </span>
            <span class="policy-value setting-value-wrap">
              <PatternEntries
                patterns={target.pending_ci_branch_patterns_default.exclude}
                readOnly={frozen}
                onChange={setExcludes}
              />
            </span>
          </div>
          <div
            class={[
              'policy-row',
              {
                'is-unsaved': controlDirty('defaults.pending_ci_quiet_period_seconds_override'),
              },
            ]}
            data-unsaved={controlDirty('defaults.pending_ci_quiet_period_seconds_override') ||
              undefined}
          >
            <span class="setting-say">
              <label class="setting-name" for="settings-quiet-period"
                >Quiet period after checks pass</label
              >
              <span class="setting-why"
                >Checks must pass and stay green this long before Smyklot merges. At zero seconds
                Smyklot merges as soon as a second look agrees</span
              >
            </span>
            <!-- THE UNIT LIVES BESIDE THE NUMBER, never buried in the sentence: a reader
               typing 30 into a box should not have to read a line of prose to learn
               whether the field is asking for seconds or minutes. -->
            <span class="policy-value entry-suffix">
              <input
                id="settings-quiet-period"
                class="num-inline"
                inputmode="numeric"
                placeholder={target.pending_ci_quiet_period_seconds_inherited.toString()}
                value={quietShown}
                disabled={frozen}
                oninput={(event) => typeQuiet(event.currentTarget.value)}
                onblur={finishQuiet}
              />
              <span class="entry-unit">seconds</span>
            </span>
          </div>
        </div>
        {#if target.pending_ci_mode_default === 'checks' && !pendingCIPermissionsReady}
          <p class="perm-note" role="status">
            Grant Checks write and Administration write to activate checks mode. Repositories remain
            blocked until GitHub approves both permissions.
          </p>
        {/if}
      </Card>

      <ConfigEditor
        patch={target.config_patch}
        inherited={target.inherited_config}
        scope="target"
        idPrefix={target.id}
        anchorPrefix="ws"
        disabled={frozen}
        dirtyKeys={dirtyConfigKeys}
        onChange={updateConfig}
      />
      <FormattingEditor
        patch={target.config_patch.formatting ?? {}}
        inherited={target.inherited_config.formatting}
        sources={target.formatting_sources}
        scope="target"
        idPrefix={target.id}
        anchor="ws-formatting"
        disabled={frozen}
        dirtyKeys={dirtyFormattingKeys}
        onChange={updateFormatting}
        onValidity={setFormattingValidity}
      />
      <!-- LAST, BECAUSE IT IS RARELY WANTED. Written beside the setting it holds and
           rendered at the foot of the page, where a shut card costs a reader one line. -->
      {@render timingCard()}
    </section>
  </div>
  <PageToc entries={TOC} />
</div>

<style>
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

  .pill-muted {
    background: var(--surface-inset);
    color: var(--text-muted);
  }

  .value-word.is-on {
    color: var(--text-secondary);
    font-weight: 600;
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

  .perm-note {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    color: var(--warning);
    font-size: var(--font-size-meta);
    /* Ink-true with even padding, so the words sit on the note's centre. */
    line-height: var(--leading-meta);
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
  }
</style>
