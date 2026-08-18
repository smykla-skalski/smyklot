<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import NightSky from '#lib/components/NightSky.svelte';
  import NightMeteors from '#lib/components/NightMeteors.svelte';

  const { Story } = defineMeta({ title: 'Decor/NightMeteors', component: NightMeteors });
</script>

<Story name="Falling">
  {#snippet template()}
    <NightSky height="28rem" meteors />
  {/snippet}
</Story>
