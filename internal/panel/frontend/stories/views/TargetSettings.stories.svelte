<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import TargetSettings from '#lib/components/TargetSettings.svelte';
  import { TARGET } from '../support/fixtures.js';

  const { Story } = defineMeta({
    title: 'Views/TargetSettings',
    component: TargetSettings,
    argTypes: { readOnly: { control: 'boolean' } },
    args: { target: TARGET, readOnly: false },
  });
</script>

<!--
  What an installation's settings look like before any repository has overridden
  anything: every value resolves from the deployment, so the whole page is inherited.
-->
<Story name="All inherited" />

<!-- An account that has answered for itself: the patched keys stop inheriting. -->
<Story
  name="With overrides"
  args={{
    target: {
      ...TARGET,
      config_patch: { quiet_success: true, allow_self_approval: true },
      effective_config: {
        ...TARGET.effective_config,
        quiet_success: true,
        allow_self_approval: true,
      },
      config_sources: {
        ...TARGET.config_sources,
        quiet_success: 'target',
        allow_self_approval: 'target',
      },
    },
  }}
/>

<!-- A viewer can read the settings and change none of them. -->
<Story name="Read only" args={{ readOnly: true }} />

<!-- Repositories are enabled by default here, which flips what every row inherits. -->
<Story
  name="Enabled by default"
  args={{ target: { ...TARGET, repository_default_enabled: true } }}
/>
