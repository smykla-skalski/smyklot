<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import CodeEditor from '#lib/components/CodeEditor.svelte';

  const { Story } = defineMeta({
    title: 'Views/CodeEditor',
    component: CodeEditor,
  });

  const RENOVATE = `{
  "$schema": "https://docs.renovatebot.com/renovate-schema.json",
  "extends": ["config:recommended"],
  "schedule": ["* 4 * * 1-5"],
  "timezone": "Europe/Warsaw",
  "automerge": false
}
`;

  const WORKFLOW = `name: test
on:
  push:
    branches: [main]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # Pinned by digest, never by tag
      - uses: actions/checkout@8edcb1b
      - run: mise run ci
`;
</script>

<script lang="ts">
  let renovate = $state(RENOVATE);
  let workflow = $state(WORKFLOW);
</script>

<!--
  A coloured `CodeBlock` underneath and a transparent textarea over it. The
  editor is the same picture as the reader because it IS the reader: a change to
  how a string is coloured lands on both at once, and there is no second
  highlighter to disagree with the first.
-->
<Story name="JSON">
  {#snippet template()}
    <CodeEditor bind:value={renovate} language="json" label="renovate.json" rows={8} />
  {/snippet}
</Story>

<!-- The lines this installation decides wear the managed bar, the same one an
     editor puts beside an overridden setting. -->
<Story name="With overridden lines">
  {#snippet template()}
    <CodeEditor
      bind:value={renovate}
      language="json"
      label="renovate.json"
      overridden={[4, 5]}
      rows={8}
    />
  {/snippet}
</Story>

<Story name="YAML">
  {#snippet template()}
    <CodeEditor bind:value={workflow} language="yaml" label="ci.yaml" rows={12} />
  {/snippet}
</Story>

<!-- Read only: the colours stay, the caret does not. -->
<Story name="Disabled">
  {#snippet template()}
    <CodeEditor value={RENOVATE} language="json" label="renovate.json" disabled rows={8} />
  {/snippet}
</Story>
