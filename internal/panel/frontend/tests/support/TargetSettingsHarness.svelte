<script lang="ts">
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
    readOnly = false,
  }: { drafts: SettingsDraftRegistry; target: PanelTarget; readOnly?: boolean } = $props();

  setSettingsDraftRegistry(untrack(() => drafts));
</script>

<TargetSettings {target} {readOnly} />
