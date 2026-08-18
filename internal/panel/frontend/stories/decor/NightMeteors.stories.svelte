<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import NightSky from '#lib/components/NightSky.svelte';
  import NightMeteors from '#lib/components/NightMeteors.svelte';

  const { Story } = defineMeta({
    title: 'Decor/NightMeteors',
    component: NightMeteors,
    parameters: { bleed: true, layout: 'fullscreen' },
  });
</script>

<!--
  Only the meteors: the other two default to on, so naming this one changed nothing
  and the story drew all three.
-->
<Story name="Falling">
  {#snippet template()}
    <NightSky
      height="28rem"
      meteors
      rocket={false}
      astronaut={false}
      firstShower={{ after: 1, within: 2 }}
    />
  {/snippet}
</Story>
