<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import PatternEntries from '#lib/components/PatternEntries.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/PatternEntries',
    component: PatternEntries,
    args: {
      patterns: ['hand-made-*', 'legacy/*'],
      readOnly: false,
      onChange: fn(),
    },
  });
</script>

<!--
  A leave-alone list edited in place: each entry is an input wearing field
  material - press its text to edit it where it stands, the x detaches it,
  Add opens an empty one. An entry left empty is dropped rather than kept
  blank.
-->
<Story name="Two patterns" />

<!-- Nothing yet: just the Add. -->
<Story name="Empty" args={{ patterns: [] }} />

<!-- A reader without write: the entries read, nothing answers. -->
<Story name="Read only" args={{ readOnly: true }} />
