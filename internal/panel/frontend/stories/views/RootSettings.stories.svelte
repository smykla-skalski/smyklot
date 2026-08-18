<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import RootSettings from '#lib/components/RootSettings.svelte';
  import Seeded from '../support/Seeded.svelte';
  import { CONFIG, RUNTIME } from '../support/fixtures.js';

  const KEY = ['root-settings'] as const;

  const base = {
    rootRole: 'Super Root',
    fetchSettings: async () => RUNTIME,
    updateSettings: async () => RUNTIME,
  };

  const { Story } = defineMeta({
    title: 'Views/RootSettings',
    component: RootSettings,
    args: base,
  });
</script>

<!--
  The deployment's own settings, and what each one resolves to. Every value here is
  either the deployment's or an override the console has set on top of it - the
  precedence chain that decides which is written out in CLAUDE.md.
-->
<Story name="All from the deployment">
  {#snippet template(args)}
    <Seeded seed={[[KEY, RUNTIME]]}><RootSettings {...args} /></Seeded>
  {/snippet}
</Story>

<!--
  Overridden: the sweep runs faster, the quiet period is longer, sessions are shorter
  and the log level is louder than the deployment asked for. The chain marker beside
  each says it is no longer inheriting.
-->
<Story name="With overrides">
  {#snippet template(args)}
    <Seeded
      seed={[
        [
          KEY,
          {
            ...RUNTIME,
            log_level: { deployment: 'info', override: 'debug', effective: 'debug' },
            reaction_poll_interval: {
              deployment_seconds: 30,
              override_seconds: 10,
              effective_seconds: 10,
            },
            merge_after_ci_quiet_period: {
              deployment_seconds: 30,
              override_seconds: 300,
              effective_seconds: 300,
            },
            session_lifetime: {
              deployment_seconds: 12 * 60 * 60,
              override_seconds: 60 * 60,
              effective_seconds: 60 * 60,
            },
            behavior_defaults: {
              deployment: CONFIG,
              override: { ...CONFIG, quiet_success: true },
              effective: { ...CONFIG, quiet_success: true },
            },
          },
        ],
      ]}
    >
      <RootSettings {...args} />
    </Seeded>
  {/snippet}
</Story>

<!-- The database answering slowly, which the service card reports rather than hides. -->
<Story name="Database degraded">
  {#snippet template(args)}
    <Seeded
      seed={[
        [
          KEY,
          {
            ...RUNTIME,
            service: {
              ...RUNTIME.service,
              storage: 'degraded',
              database: { ...RUNTIME.service.database, state: 'degraded', latency_ms: 210 },
            },
          },
        ],
      ]}
    >
      <RootSettings {...args} />
    </Seeded>
  {/snippet}
</Story>

<Story name="Loading">
  {#snippet template(args)}
    <Seeded><RootSettings {...args} fetchSettings={() => new Promise(() => {})} /></Seeded>
  {/snippet}
</Story>
