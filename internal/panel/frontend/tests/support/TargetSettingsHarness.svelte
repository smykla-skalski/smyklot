<script lang="ts">
  import type { ConfigFileStatusConnection } from '../../src/lib/config-file-status';
  import { untrack } from 'svelte';

  import TargetSettings from '../../src/lib/components/TargetSettings.svelte';
  import {
    setSettingsDraftRegistry,
    type SettingsDraftRegistry,
  } from '../../src/lib/settings-drafts.svelte';
  import type { PanelTarget } from '../../src/lib/types';

  const {
    drafts,
    target,
    configFileConnection,
    readOnly = false,
  }: {
    drafts: SettingsDraftRegistry;
    target: PanelTarget;
    configFileConnection?: ConfigFileStatusConnection;
    readOnly?: boolean;
  } = $props();

  setSettingsDraftRegistry(untrack(() => drafts));
</script>

<TargetSettings {target} {readOnly} {configFileConnection} />
