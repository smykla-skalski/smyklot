<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import BuildInfo from '#lib/components/BuildInfo.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/BuildInfo',
    component: BuildInfo,
    args: { build: { version: '1.56.0', serviceHost: 'smyklot.com' } },
  });
</script>

<!-- Muted navigation metadata, with exact long values retained in accessible text. -->
<Story name="Default" />
<Story name="Version only" args={{ build: { version: '1.56.0', serviceHost: null } }} />
<Story
  name="Nothing to report"
  args={{ build: { version: null, serviceHost: null } }}
  tags={['blank']}
/>
<Story
  name="Long values"
  args={{
    build: {
      version: '1.56.0-development-build-with-a-long-revision',
      serviceHost: 'smyklot.service.production.example.com',
    },
  }}
/>
