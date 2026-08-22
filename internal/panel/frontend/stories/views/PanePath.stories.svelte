<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import PanePath from '#lib/components/PanePath.svelte';

  const { Story } = defineMeta({
    title: 'Views/PanePath',
    component: PanePath,
    args: {
      segments: [{ label: 'Sync', href: '#/sync' }],
    },
  });
</script>

<!--
  Every page below its console's root opens with its ancestors - each one
  pressable, in the eyebrow voice. Root pages carry none: the shell already
  says the console, and a path pointing at the page it sits on is a self-link.
-->
<Story name="Kid page" />

<!-- A drill-down names the list it came from. -->
<Story
  name="Object page"
  args={{
    segments: [
      { label: 'Sync', href: '#/sync' },
      { label: 'Rulesets', href: '#/sync/rulesets' },
    ],
  }}
/>

<!-- No ancestors, no path: the component draws nothing rather than a self-link.
     Tagged blank so the catalogue check holds it to exactly that. -->
<Story name="Console root" args={{ segments: [] }} tags={['blank']} />
