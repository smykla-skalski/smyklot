<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import NightSky from '#lib/components/NightSky.svelte';
  import NightAstronaut from '#lib/components/NightAstronaut.svelte';

  const { Story } = defineMeta({
    title: 'Decor/NightAstronaut',
    component: NightAstronaut,
    parameters: { bleed: true, layout: 'fullscreen' },
  });
</script>

<!--
  Needs a sky to drift across, and a free slot to be allowed into. The rocket and the
  meteors are turned off by name: all three default to on, so `astronaut` on its own
  named the one already running and left the other two running beside it.

  The first crossing is brought forward too. On a page it is eight to thirty-five
  seconds out, which is the whole point of it there and useless here - a story that
  shows nothing for its first eight seconds is not showing what it describes.
-->
<Story name="Adrift">
  {#snippet template()}
    <NightSky
      height="28rem"
      astronaut
      rocket={false}
      meteors={false}
      firstSighting={{ after: 1, within: 2 }}
    />
  {/snippet}
</Story>
