<script lang="ts">
  import { countOverrides } from '../config';
  import type { ConfigPatch, PanelTarget, TargetSettingsInput } from '../types';
  import Chip from './Chip.svelte';
  import ConfigEditor from './ConfigEditor.svelte';
  import HelpTip from './HelpTip.svelte';
  import PanelHeader from './PanelHeader.svelte';
  import Plate from './Plate.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  const REPOSITORY_DEFAULT_OPTIONS = [
    { value: 'enabled', label: 'Enabled' },
    { value: 'disabled', label: 'Disabled' },
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
  const defaultEnabled = $derived(pendingDefault ?? target.repository_default_enabled);
  const overrides = $derived(countOverrides(target.config_patch));

  async function updateDefault(enabled: boolean): Promise<void> {
    if (savingDefault || enabled === defaultEnabled) return;
    pendingDefault = enabled;
    savingDefault = true;
    failure = null;
    try {
      await onUpdate({
        repository_default_enabled: enabled,
        config_patch: target.config_patch,
        expected_revision: target.revision,
      });
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
      await onUpdate({
        repository_default_enabled: target.repository_default_enabled,
        config_patch: configPatch,
        expected_revision: target.revision,
      });
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    }
  }
</script>

<section class="settings-page" aria-labelledby="settings-heading">
  <PanelHeader
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
    <p class="form-error" role="alert">{failure}</p>
  {/if}

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

  .form-error {
    color: var(--stop);
    font-size: 0.8125rem;
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
  }
</style>
