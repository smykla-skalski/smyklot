<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import DiffBlock from '#lib/components/DiffBlock.svelte';

  /* The plan page's own window: one changed line with its word emphasised on
     both sides, one inserted line, a context line above and below. */
  const BEFORE = [
    '"extends": ["config:recommended"],',
    '"schedule": ["* 4 * * 0"],',
    '"packageRules": [',
  ].join('\n');
  const AFTER = [
    '"extends": ["config:recommended"],',
    '"schedule": ["* 4 * * 1-5"],',
    '"timezone": "Europe/Warsaw",',
    '"packageRules": [',
  ].join('\n');

  const { Story } = defineMeta({
    title: 'Primitives/DiffBlock',
    component: DiffBlock,
    args: {
      before: BEFORE,
      after: AFTER,
      lang: 'json',
    },
  });
</script>

<!--
  A unified diff drawn from the two texts themselves. Context lines wear the
  window's own numbers; added and removed lines carry their glyph instead,
  and the changed stretch of a paired line is emphasised one state-step
  deeper than its ground.
-->
<Story name="Changed lines" />

<!-- A file arriving whole: everything added, nothing paired to emphasise. -->
<Story
  name="All additions"
  args={{
    before: '',
    after: ['# Contributing', '', '- Fork, branch, open a pull request'].join('\n'),
    lang: 'markdown',
  }}
/>

<!-- YAML, with a removed stretch emphasised on the deletion's own ground. -->
<Story
  name="YAML change"
  args={{
    before: ['name: ci', 'on:', '  schedule:', '    - cron: "0 4 * * 0"'].join('\n'),
    after: ['name: ci', 'on:', '  schedule:', '    - cron: "0 4 * * 1"'].join('\n'),
    lang: 'yaml',
  }}
/>
