<script lang="ts">
  import { countOverrides } from '../config';
  import type { ConfigPatch, PanelTarget, PendingCIMode, TargetSettingsInput } from '../types';
  import Button from './Button.svelte';
  import FormError from './FormError.svelte';
  import Chip from './Chip.svelte';
  import ConfigEditor from './ConfigEditor.svelte';
  import HelpTip from './HelpTip.svelte';
  import PageHeader from './PageHeader.svelte';
  import Plate from './Plate.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  const REPOSITORY_DEFAULT_OPTIONS = [
    { value: 'enabled', label: 'Enabled' },
    { value: 'disabled', label: 'Disabled' },
  ] as const;
  const PENDING_CI_MODE_OPTIONS = [
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

  let savingDefault = $state(false);
  let pendingDefault = $state<boolean | null>(null);
  let savedFlash = $state(false);
  let flashTimer: ReturnType<typeof setTimeout> | undefined;
  let failure = $state<string | null>(null);
  let savingPendingCI = $state(false);
  // svelte-ignore state_referenced_locally
  let pendingCIMode = $state<PendingCIMode>(target.pending_ci_mode_default);
  // svelte-ignore state_referenced_locally
  let pendingCIIncludes = $state(target.pending_ci_branch_patterns_default.include.join('\n'));
  // svelte-ignore state_referenced_locally
  let pendingCIExcludes = $state(target.pending_ci_branch_patterns_default.exclude.join('\n'));
  // svelte-ignore state_referenced_locally
  let pendingCIQuiet = $state(target.pending_ci_quiet_period_seconds_override?.toString() ?? '');
  const defaultEnabled = $derived(pendingDefault ?? target.repository_default_enabled);
  const overrides = $derived(countOverrides(target.config_patch));
  const pendingCIPermissionsReady = $derived(
    target.pending_ci_permissions.checks_write &&
      target.pending_ci_permissions.administration_write &&
      target.pending_ci_permissions.merge_queues_read &&
      target.pending_ci_permissions.commit_statuses_read,
  );

  $effect(() => {
    if (savingPendingCI) return;
    pendingCIMode = target.pending_ci_mode_default;
    pendingCIIncludes = target.pending_ci_branch_patterns_default.include.join('\n');
    pendingCIExcludes = target.pending_ci_branch_patterns_default.exclude.join('\n');
    pendingCIQuiet = target.pending_ci_quiet_period_seconds_override?.toString() ?? '';
  });

  function settingsInput(overrides: Partial<TargetSettingsInput>): TargetSettingsInput {
    return {
      repository_default_enabled: target.repository_default_enabled,
      pending_ci_mode_default: target.pending_ci_mode_default,
      pending_ci_branch_patterns_default: target.pending_ci_branch_patterns_default,
      pending_ci_quiet_period_seconds_override: target.pending_ci_quiet_period_seconds_override,
      config_patch: target.config_patch,
      expected_revision: target.revision,
      ...overrides,
    };
  }

  async function updateDefault(enabled: boolean): Promise<void> {
    if (savingDefault || enabled === defaultEnabled) return;
    pendingDefault = enabled;
    savingDefault = true;
    failure = null;
    try {
      await onUpdate(settingsInput({ repository_default_enabled: enabled }));
      savedFlash = true;
      if (flashTimer !== undefined) clearTimeout(flashTimer);
      flashTimer = setTimeout(() => (savedFlash = false), 1600);
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    } finally {
      savingDefault = false;
      pendingDefault = null;
    }
  }

  async function updateConfig(configPatch: ConfigPatch): Promise<void> {
    failure = null;
    try {
      await onUpdate(settingsInput({ config_patch: configPatch }));
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    }
  }

  function lines(value: string): string[] {
    return value
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line !== '');
  }

  async function updatePendingCI(): Promise<void> {
    const include = lines(pendingCIIncludes);
    if (include.length === 0) {
      failure = 'At least one branch include is required';
      return;
    }
    const quiet = pendingCIQuiet.trim() === '' ? null : Number(pendingCIQuiet);
    if (quiet !== null && (!Number.isInteger(quiet) || quiet < 0 || quiet > 86_400)) {
      failure = 'Quiet period must be whole seconds from 0 to 86400';
      return;
    }
    savingPendingCI = true;
    failure = null;
    try {
      await onUpdate(
        settingsInput({
          pending_ci_mode_default: pendingCIMode,
          pending_ci_branch_patterns_default: {
            include,
            exclude: lines(pendingCIExcludes),
          },
          pending_ci_quiet_period_seconds_override: quiet,
        }),
      );
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    } finally {
      savingPendingCI = false;
    }
  }
</script>

<section class="settings-page" aria-labelledby="settings-heading">
  <PageHeader
    id="settings-heading"
    title="Settings"
    description="Defaults every repository inherits unless a repository overrides them"
  />

  <section class="plate policy-plate" aria-labelledby="repository-policy-heading">
    <div class="policy-row">
      <div class="policy-copy">
        <h2 id="repository-policy-heading">Unconfigured repositories</h2>
        <p id="repository-policy-help">
          How Smyklot treats repositories that don't have their own setting yet. New installations
          start disabled, so nothing runs before you decide
        </p>
      </div>
      <span class="saved-flash" class:show={savedFlash} role="status">
        {savedFlash ? 'Saved ✓' : ''}
      </span>
      <SegmentedControl
        name="repository-policy-{target.id}"
        label="Unconfigured repositories"
        descriptionId="repository-policy-help"
        options={REPOSITORY_DEFAULT_OPTIONS}
        value={defaultEnabled ? 'enabled' : 'disabled'}
        compact
        onSelect={(selection) => void updateDefault(selection === 'enabled')}
        disabled={savingDefault || readOnly}
      />
    </div>
  </section>

  {#if failure !== null}
    <FormError message={failure} />
  {/if}

  <Plate label="Merge after CI">
    {#snippet status()}
      <Chip
        small
        tone={pendingCIMode === 'checks' && !pendingCIPermissionsReady ? 'warning' : 'clear'}
      >
        {pendingCIMode === 'checks'
          ? pendingCIPermissionsReady
            ? 'App permissions ready'
            : 'Approval required'
          : 'Compatibility mode'}
      </Chip>
    {/snippet}
    <form
      class="pending-ci-form"
      onsubmit={(event) => {
        event.preventDefault();
        void updatePendingCI();
      }}
    >
      <div class="pending-ci-mode-row">
        <div>
          <strong>Repository protection</strong>
          <p>
            Checks mode creates an app-bound required check and merges the exact authorized head.
          </p>
        </div>
        <SegmentedControl
          name="pending-ci-mode-{target.id}"
          label="Merge after CI representation"
          options={PENDING_CI_MODE_OPTIONS}
          value={pendingCIMode}
          disabled={readOnly || savingPendingCI}
          compact
          onSelect={(value) => (pendingCIMode = value as PendingCIMode)}
        />
      </div>
      {#if pendingCIMode === 'checks' && !pendingCIPermissionsReady}
        <p class="permission-note" role="status">
          Grant Checks write, Commit statuses read, Administration write, and Merge queues read to
          activate checks mode. The setting can be saved now; repositories remain blocked until
          GitHub approves all four permissions.
        </p>
      {/if}
      <div class="pending-ci-grid">
        <label>
          <span>Protected refs</span>
          <textarea rows="3" bind:value={pendingCIIncludes} disabled={readOnly || savingPendingCI}
          ></textarea>
          <small
            >One raw GitHub ruleset pattern per line, such as <code>~DEFAULT_BRANCH</code>.</small
          >
        </label>
        <label>
          <span>Excluded refs</span>
          <textarea rows="3" bind:value={pendingCIExcludes} disabled={readOnly || savingPendingCI}
          ></textarea>
          <small>Optional patterns that should keep the inherited merge behavior.</small>
        </label>
        <label>
          <span>Stable passing window</span>
          <input
            type="number"
            min="0"
            max="86400"
            step="1"
            bind:value={pendingCIQuiet}
            disabled={readOnly || savingPendingCI}
            placeholder="Global default"
          />
          <small>Seconds. Zero still requires two matching passing observations.</small>
        </label>
      </div>
      <div class="pending-ci-actions">
        <Button type="submit" tone="brand" disabled={readOnly || savingPendingCI}>
          {savingPendingCI ? 'Saving…' : 'Save merge settings'}
        </Button>
      </div>
    </form>
  </Plate>

  <Plate label="Configuration defaults">
    {#snippet status()}
      <div class="header-actions">
        <Chip small>{overrides} {overrides === 1 ? 'override' : 'overrides'}</Chip>
        <HelpTip
          id="configuration-defaults-help"
          label="About configuration defaults"
          text="Account-wide command behavior. Repository files and repository settings take precedence"
        />
      </div>
    {/snippet}

    <ConfigEditor
      patch={target.config_patch}
      inherited={target.inherited_config}
      scope="target"
      idPrefix={target.id}
      disabled={readOnly}
      onSave={updateConfig}
    />
  </Plate>
</section>

<style>
  .header-actions {
    align-items: center;
    display: flex;
    gap: 0.25rem;
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

  .pending-ci-mode-row {
    align-items: center;
    display: flex;
    gap: var(--space-4);
    justify-content: space-between;
  }

  .pending-ci-mode-row p,
  .permission-note,
  .pending-ci-grid small {
    color: var(--dim);
    font-size: var(--font-size-meta);
    margin: 0.25rem 0 0;
  }

  .permission-note {
    background: color-mix(in srgb, var(--warning) 9%, transparent);
    border: 1px solid color-mix(in srgb, var(--warning) 28%, transparent);
    border-radius: var(--radius-control);
    padding: var(--space-3);
  }

  .pending-ci-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .pending-ci-grid label:last-child {
    grid-column: 1 / -1;
    max-width: 20rem;
  }

  .pending-ci-grid textarea,
  .pending-ci-grid input {
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

  .settings-page :global(.plate) {
    background: var(--surface-base);
  }

  .policy-row {
    align-items: center;
    display: flex;
    gap: var(--space-4);
    padding: 0.875rem 1.25rem;
  }

  .policy-copy {
    flex: 1;
  }

  .policy-copy h2 {
    font-size: var(--font-size-body);
    font-weight: 650;
    margin: 0;
  }

  .policy-copy p {
    color: var(--dim);
    font-size: var(--font-size-meta);
    margin: 0.125rem 0 0;
    max-width: 52ch;
  }

  .saved-flash {
    color: var(--brand-action);
    flex: none;
    font: 600 var(--font-size-compact) / 1 var(--sans);
    min-width: 3.5rem;
    opacity: 0;
    text-align: end;
    text-box: trim-both cap alphabetic;
    transition: opacity 200ms ease-out;
  }

  .saved-flash.show {
    opacity: 1;
  }

  /* Pulled up against the section above it and given room below; the rest comes
     from `app.css`. `:global` because `FormError` renders the paragraph. */
  :global(.form-error) {
    margin: -0.5rem 0 1rem;
  }

  /* The copy is the only part of this row that can give, and beside a control
     that holds 157px it gave everything: at 320 it was down to a 100px column
     setting one word per line, and the control had still run off the screen.
     Stacked, the copy gets the row and the control sits under it. */
  @media (max-width: 30rem) {
    .policy-row {
      align-items: start;
      flex-direction: column;
      gap: var(--space-3);
      padding-inline: 0.875rem;
      position: relative;
    }

    /* Lifted out of the column rather than left in it: it is empty almost
       always, and an empty flex item still takes a line, so in flow it would
       hold a 27px gap open between the copy and the control to say nothing.
       It stays in the DOM either way - `display: none` would drop a live
       region, and the announcement is the whole point of it. */
    .saved-flash {
      inset-block-start: 0.875rem;
      inset-inline-end: 0.875rem;
      min-width: 0;
      position: absolute;
    }

    .pending-ci-mode-row {
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
</style>
