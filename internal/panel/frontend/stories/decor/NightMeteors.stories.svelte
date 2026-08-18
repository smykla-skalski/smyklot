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

  **It takes a while.** The first shower is six to twenty-four seconds out, and that
  wait is the point of it - see the note on the astronaut. Leave the story open.
-->
<Story name="Falling">
  {#snippet template()}
    <NightSky height="28rem" meteors rocket={false} astronaut={false} />
  {/snippet}
</Story>
