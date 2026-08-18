<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import NightSky from '#lib/components/NightSky.svelte';
  import NightRocket from '#lib/components/NightRocket.svelte';

  const { Story } = defineMeta({
    title: 'Decor/NightRocket',
    component: NightRocket,
    parameters: { bleed: true, layout: 'fullscreen' },
  });
</script>

<!--
  Drawn inside a sky, because it needs one to cross. "Barrels" here are tight turns,
  not rolls. A theme switch gates whether it is *active*, not whether it exists, so
  changing the theme does not delete it mid-flight.

  The astronaut and the meteors are off by name. All three default to on, so `rocket`
  named the one already flying and the story showed the whole sky's cast at once -
  and with `SkySlots` holding two of them at a time, which two you got was a race.
-->
<Story name="In flight">
  {#snippet template()}
    <NightSky height="28rem" rocket astronaut={false} meteors={false} />
  {/snippet}
</Story>

<Story name="Faster">
  {#snippet template()}
    <NightSky height="28rem" rocket rocketSpeed={1.8} astronaut={false} meteors={false} />
  {/snippet}
</Story>
