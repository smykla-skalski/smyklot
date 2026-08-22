<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import CodeBlock from '#lib/components/CodeBlock.svelte';

  const JSON_TEXT = [
    '{',
    '  "$schema": "https://docs.renovatebot.com/renovate-schema.json",',
    '  "extends": ["config:recommended"],',
    '  // Weekend runs keep review noise out of the working week',
    '  "schedule": ["* 4 * * 6"],',
    '  "timezone": "UTC",',
    '  "automerge": false',
    '}',
  ].join('\n');

  const { Story } = defineMeta({
    title: 'Primitives/CodeBlock',
    component: CodeBlock,
    args: {
      text: JSON_TEXT,
      lang: 'json',
      overridden: null,
    },
  });
</script>

<!--
  A read-only code window: numbered lines, tokenized by the file's own
  language, and - where a set of lines is handed in - the managed gutter
  bar on the ones an adjustment rewrote.
-->
<Story name="JSON" />

<!-- The blue gutter bars mark what an adjustment rewrote. -->
<Story name="With overridden lines" args={{ overridden: new Set([5, 6]) }} />

<Story
  name="YAML"
  args={{
    text: ['name: test', 'on:', '  push:', '    branches: [main]', '# pinned by digest'].join('\n'),
    lang: 'yaml',
  }}
/>

<Story
  name="Markdown"
  args={{
    text: [
      '# Contributing',
      '',
      'Open a pull request against `main`.',
      '',
      '- Sign-off: `-sS`',
    ].join('\n'),
    lang: 'markdown',
  }}
/>
