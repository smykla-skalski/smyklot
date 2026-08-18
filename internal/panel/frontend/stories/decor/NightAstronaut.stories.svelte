<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import NightSky from '#lib/components/NightSky.svelte';
  import NightAstronaut from '#lib/components/NightAstronaut.svelte';

  const { Story } = defineMeta({ title: 'Decor/NightAstronaut', component: NightAstronaut });
</script>

<!-- Needs a sky to drift across, and a free slot to be allowed into. -->
<Story name="Adrift">
  {#snippet template()}
    <NightSky height="28rem" astronaut />
  {/snippet}
</Story>
