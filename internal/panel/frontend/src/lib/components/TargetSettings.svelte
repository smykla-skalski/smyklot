<script lang="ts">
  import { page } from '$app/state';
  import type {
    ConfigurationReviewClient,
    ConfigurationReviewSource,
  } from '../config-file-review.svelte';
  import { configFileStatusRevision, type ConfigFileStatusConnection } from '../config-file-status';
  import ConfigurationFileSync from './ConfigurationFileSync.svelte';
  import type { BypassActorLookup } from '../types';
  import BypassPolicyEditor from './BypassPolicyEditor.svelte';
  import { untrack } from 'svelte';
  import { useInterval } from 'runed';

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
  import DurationInput from './DurationInput.svelte';
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
  import PickerTrigger from './PickerTrigger.svelte';
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
    { id: 'ws-config-file', label: 'Configuration file sync' },
    { id: 'ws-newrepos', label: 'New repositories' },
    { id: 'ws-merging', label: 'Merging' },
    { id: 'ws-exceptions', label: 'Merge exceptions' },
    { id: 'ws-behavior', label: 'Behavior' },
    { id: 'ws-commands', label: 'Commands' },
    { id: 'ws-formatting', label: 'Formatting' },
    { id: 'ws-timing', label: 'Timing' },
  ];

  const {
    target: canonicalTarget,
    readOnly = false,
    timing,
    lookupBypassActors,
    configFileConnection,
    configFileReview,
  }: {
    target: PanelTarget;
    configFileConnection?: ConfigFileStatusConnection;
    configFileReview?: ConfigurationReviewClient;
    lookupBypassActors?: BypassActorLookup;
    readOnly?: boolean;
    /**
     * What the Timing card needs to say when Smyklot acts and to carry a request to the
     * operators. Absent where the page is rendered outside the shell - a story, a
     * component spec - and the card then holds the settings this page owns and no more.
     */
    timing?: { api: PanelApi; canRequest: boolean };
  } = $props();

  const drafts = getSettingsDraftRegistry();
  // This page owns its observation clock in both normal and Root workspace views.
  // The Root wrapper's separate elevation clock intentionally pauses without a visit.
  let now = $state(Date.now());
  useInterval(30_000, { callback: () => (now = Date.now()) });
  const resource = $derived(targetDefaultsResource(canonicalTarget.id));
  const settingsScope = $derived({
    type: 'workspace',
    targetId: canonicalTarget.id,
  } as const satisfies SettingsScope);
  const reviewSource = $derived.by((): ConfigurationReviewSource | undefined => {
    const client = configFileReview;
    if (!client) return undefined;
    const targetId = canonicalTarget.id;
    const saved = parseTargetDefaultsDocument(drafts.resource(resource)?.base);
    return {
      identity: JSON.stringify([
        drafts.accountId,
        targetId,
        canonicalTarget.account.login,
        readOnly,
        page.url.pathname,
        configFileStatusRevision(drafts, targetId, canonicalTarget.revision),
      ]),
      prepare: () => drafts.refreshFromStorage(),
      hasDrafts: drafts.dirtyControls(settingsScope).length > 0,
      canWrite: !readOnly,
      enabled: saved?.config_file_sync_enabled ?? canonicalTarget.config_file_sync_enabled ?? false,
      fileIgnored: false,
      preview: () => client.preview(targetId),
      resolve: (input) => client.resolve(targetId, input),
      onResolved: () => {
        void configFileConnection?.refetch();
      },
    };
  });
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

  const PATH_INDEX_UNITS: readonly DurationUnit[] = ['seconds', 'minutes', 'hours', 'days'];

  function durationValidity(control: TargetDefaultsControlId, problem: string | null): void {
    drafts.setValidationProblem(settingsScope, control, problem);
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
        description="Settings and defaults for this workspace"
      />

      {#if failure !== null}
        <FormError message={failure} />
      {/if}

      <ConfigurationFileSync
        id="ws-config-file"
        {now}
        scope="workspace"
        repository={`${canonicalTarget.account.login}/.github`}
        enabled={target.config_file_sync_enabled ?? false}
        savedEnabled={parseTargetDefaultsDocument(drafts.resource(resource)?.base)
          ?.config_file_sync_enabled ??
          canonicalTarget.config_file_sync_enabled ??
          false}
        dirty={controlDirty('defaults.config_file_sync_enabled')}
        {readOnly}
        connection={configFileConnection}
        {reviewSource}
        onChange={(enabled) =>
          stage(
            { ...document, config_file_sync_enabled: enabled },
            'defaults.config_file_sync_enabled',
          )}
      />

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
                    tone="add"
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
                  <DurationInput
                    label="File index interval"
                    value={target.path_index_interval_seconds_override}
                    units={PATH_INDEX_UNITS}
                    minimum={60}
                    maximum={604_800}
                    disabled={frozen}
                    onChange={(seconds) =>
                      stage(
                        { ...document, path_index_interval_seconds_override: seconds },
                        'defaults.path_index_interval_seconds_override',
                      )}
                    onValidityChange={(problem) =>
                      durationValidity('defaults.path_index_interval_seconds_override', problem)}
                  />
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
                  <PickerTrigger
                    {...attributes}
                    type="button"
                    aria-label="{target.pending_ci_mode_default === 'checks'
                      ? 'Checks'
                      : 'Labels'} - repository protection"
                    disabled={frozen}
                  >
                    {target.pending_ci_mode_default === 'checks' ? 'Checks' : 'Labels'}
                  </PickerTrigger>
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
            <span class="policy-value">
              <DurationInput
                id="settings-quiet-period"
                label="Quiet period after checks pass"
                amountLabel="Quiet period after checks pass"
                value={target.pending_ci_quiet_period_seconds_override}
                inherited={target.pending_ci_quiet_period_seconds_inherited}
                maximum={86_400}
                allowEmpty
                disabled={frozen}
                onChange={(seconds) =>
                  stage(
                    { ...document, pending_ci_quiet_period_seconds_override: seconds },
                    'defaults.pending_ci_quiet_period_seconds_override',
                  )}
                onValidityChange={(problem) =>
                  durationValidity('defaults.pending_ci_quiet_period_seconds_override', problem)}
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
      </Card>
      <BypassPolicyEditor
        id="ws-exceptions"
        unsaved={controlDirty('defaults.pending_ci_bypass_policy_default')}
        organizationActors={target.type === 'Organization'}
        value={target.pending_ci_bypass_policy_default ?? null}
        lookup={lookupBypassActors}
        readOnly={frozen}
        onChange={(value) =>
          stage(
            { ...document, pending_ci_bypass_policy_default: value },
            'defaults.pending_ci_bypass_policy_default',
          )}
      />

      <ConfigEditor
        patch={target.config_patch}
        inherited={target.inherited_config}
        scope="target"
        idPrefix={target.id}
        anchorPrefix="ws"
        disabled={frozen}
        dirtyKeys={dirtyConfigKeys}
        onChange={updateConfig}
        onValidity={(problem) =>
          drafts.setValidationProblem(
            settingsScope,
            'defaults.config_patch.command_aliases',
            problem,
          )}
      />
      <FormattingEditor
        patch={target.config_patch.formatting ?? {}}
        savedPatch={canonicalTarget.config_patch.formatting ?? {}}
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
