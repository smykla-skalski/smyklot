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

  **It takes a while.** The first crossing is eight to thirty-five seconds out, and
  that wait is the whole point of the thing - a crossing that arrived on cue would be
  a feature rather than a surprise. The timing is not a prop: an easter egg that a
  caller can schedule is one somebody will schedule, and the component would then be
  keeping a promise it exists to break. Leave the story open.
-->
<Story name="Adrift">
  {#snippet template()}
    <NightSky height="28rem" astronaut rocket={false} meteors={false} />
  {/snippet}
</Story>
