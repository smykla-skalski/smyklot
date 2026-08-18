<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import NightSky from '#lib/components/NightSky.svelte';

  const { Story } = defineMeta({
    title: 'Decor/NightSky',
    component: NightSky,
    argTypes: {
      rocket: { control: 'boolean' },
      astronaut: { control: 'boolean' },
      meteors: { control: 'boolean' },
    },
    args: { height: '28rem', rocket: false, astronaut: false, meteors: false },
    parameters: { bleed: true, layout: 'fullscreen' },
  });
</script>

<!--
  The ground the signed-out pages stand on. The easter eggs are off by default here so
  the sky itself can be looked at; turn them on one at a time.

  At most two are ever on screen at once, which `SkySlots` arbitrates - three moving
  things in one sky read as noise rather than as a surprise.
-->
<Story name="Empty sky" />
<Story name="With a rocket" args={{ rocket: true }} />
<Story name="With an astronaut" args={{ astronaut: true }} />
<Story name="With meteors" args={{ meteors: true }} />
