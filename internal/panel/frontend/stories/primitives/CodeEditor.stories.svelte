<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import CodeEditor from '#lib/components/CodeEditor.svelte';

  const JSON_TEXT = [
    '{',
    '  "$schema": "https://docs.renovatebot.com/renovate-schema.json",',
    '  "extends": ["config:recommended"],',
    '  // Weekend runs keep review noise out of the working week',
    '  "schedule": ["* 4 * * 6"],',
    '  "timezone": "Europe/Warsaw",',
    '  "automerge": false',
    '}',
  ].join('\n');

  const { Story } = defineMeta({
    title: 'Primitives/CodeEditor',
    component: CodeEditor,
    args: {
      value: JSON_TEXT,
      readOnly: false,
      overridden: null,
      onChange: () => {},
    },
  });
</script>

<!--
  The composed copy as an editable surface: CodeMirror wearing CodeBlock's
  clothes. Type into it - the page derives the override from the edit.
  Comments survive, because the surface is the template's own bytes.
-->
<Story name="Editable" />

<!-- The managed gutter bars mark the lines an adjustment rewrote. -->
<Story name="With overridden lines" args={{ overridden: new Set([6]) }} />

<!-- Frozen: a reader who may not write sees the same surface, inert. -->
<Story name="Read only" args={{ readOnly: true, overridden: new Set([6]) }} />

<Story
  name="YAML"
  args={{
    lang: 'yaml',
    value: ['name: test', 'on:', '  push:', '    branches: [main]', '# pinned by digest'].join(
      '\n',
    ),
  }}
/>
