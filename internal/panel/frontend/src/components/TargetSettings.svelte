<script lang="ts">
  import { countOverrides } from '../lib/config';
  import type { ConfigPatch, PanelTarget, TargetSettingsInput } from '../lib/types';
  import Chip from './Chip.svelte';
  import ConfigEditor from './ConfigEditor.svelte';
  import HelpTip from './HelpTip.svelte';
  import PanelHeader from './PanelHeader.svelte';
  import Plate from './Plate.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  const REPOSITORY_DEFAULT_OPTIONS = [
    { value: 'enabled', label: 'On', tone: 'on' },
    { value: 'disabled', label: 'Off', tone: 'off' },
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
  let failure = $state<string | null>(null);
  const defaultEnabled = $derived(pendingDefault ?? target.repository_default_enabled);

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
    description="Set account defaults that repositories inherit unless you override them"
  />

  <Plate label="Enable repositories by default">
    {#snippet status()}
      <div class="header-actions">
        <SegmentedControl
          name="repository-policy-{target.id}"
          label="Enable repositories by default"
          options={REPOSITORY_DEFAULT_OPTIONS}
          value={defaultEnabled ? 'enabled' : 'disabled'}
          onSelect={(selection) => void updateDefault(selection === 'enabled')}
          disabled={savingDefault || readOnly}
        />
        <HelpTip
          id="repository-policy-help"
          label="About enabling repositories by default"
          text="Repositories use this state unless you choose a different state for that repository. New installations start Off so the service only handles repositories you enable deliberately"
        />
      </div>
    {/snippet}
  </Plate>

  {#if failure !== null}
    <p class="form-error" role="alert">{failure}</p>
  {/if}

  <Plate label="Configuration defaults">
    {#snippet status()}
      <div class="header-actions">
        <Chip small>{countOverrides(target.config_patch)} custom settings</Chip>
        <HelpTip
          id="configuration-defaults-help"
          label="About configuration defaults"
          text="Account-wide command behavior. Repository files and repository-specific settings take precedence"
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

  .form-error {
    color: var(--stop);
    font-size: 0.8125rem;
    margin: -0.5rem 0 1rem;
  }
</style>
