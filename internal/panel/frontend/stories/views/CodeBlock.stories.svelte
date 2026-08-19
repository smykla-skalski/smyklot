<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import CodeBlock, { type CodeLine } from '#lib/components/CodeBlock.svelte';

  const { Story } = defineMeta({
    title: 'Views/CodeBlock',
    component: CodeBlock,
  });

  /** The range a word occupies in its line, which is what a real diff sends. */
  const at = (text: string, word: string): [number, number] => {
    const from = text.indexOf(word);

    return [from, from + word.length];
  };

  const numbered = (text: string, extra: Partial<CodeLine> = {}): CodeLine[] =>
    text.split('\n').map((line, index) => ({ text: line, number: index + 1, ...extra }));

  const TEMPLATE = numbered(`{
  "$schema": "https://docs.renovatebot.com/renovate-schema.json",
  "extends": ["config:recommended"],
  "labels": ["dependencies"],
  "schedule": ["before 6am on monday"],
  "prConcurrentLimit": 3,
  "rebaseWhen": "behind-base-branch",
  "automerge": false
}`);

  // Two lines this installation decides for every repository. The bar in the
  // gutter is the only mark; the value itself is written like any other.
  const OVERRIDDEN = TEMPLATE.map((line) => ({
    ...line,
    overridden: line.number === 5 || line.number === 6,
  }));

  const CHANGE: CodeLine[] = [
    { text: '  "extends": ["config:recommended"],', number: 3 },
    {
      text: '  "labels": ["dependencies"],',
      op: '-',
      marks: [at('  "labels": ["dependencies"],', '["dependencies"]')],
    },
    {
      text: '  "labels": ["dependencies", "renovate"],',
      op: '+',
      marks: [at('  "labels": ["dependencies", "renovate"],', '["dependencies", "renovate"]')],
    },
    { text: '  "schedule": ["before 6am on monday"],', number: 5 },
    {
      text: '  "prConcurrentLimit": 3,',
      op: '-',
      marks: [at('  "prConcurrentLimit": 3,', '3')],
    },
    {
      text: '  "prConcurrentLimit": 5,',
      op: '+',
      marks: [at('  "prConcurrentLimit": 5,', '5')],
    },
    { text: '  "rebaseWhen": "behind-base-branch",', number: 7 },
    { text: '  "automerge": false', op: '-', marks: [at('  "automerge": false', 'false')] },
    { text: '  "automerge": true', op: '+', marks: [at('  "automerge": true', 'true')] },
  ];

  const WORKFLOW = numbered(`name: Test
on:
  push:
    branches: [main]
jobs:
  test:
    # Pinned by digest, never by tag.
    runs-on: ubuntu-latest
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@v4
      - run: mise run ci`);

  const README = numbered(`# smyklot

Approvals and merges, decided by \`CODEOWNERS\`.

## Running it

- \`mise run build\` builds the binary
- \`mise run test\` runs every suite`);

  const CONFIG = numbered(`# Written by the installation, not by hand.
[merge]
method = "squash"
delete_branch = true

[approvals]
required = 1
dismiss_stale = true`);
</script>

<!--
  A change, said in four channels at once: the ground says which direction, the
  glyph in the gutter says it again for a reader who cannot separate the hues,
  the deeper ground marks the words that actually changed, and the file's own
  colouring survives all three - a changed value is still a string.
-->
<Story name="A change to a file">
  {#snippet template()}
    <CodeBlock lines={CHANGE} language="json" label="renovate.json - what would change" />
  {/snippet}
</Story>

<!--
  The template as it stands, with the two lines this installation decides.
  The gutter bar is the editor's own vocabulary for "managed here", and the
  clear at the row's end removes the override - it never writes a value.
-->
<Story name="A template with overridden lines">
  {#snippet template()}
    <CodeBlock
      lines={OVERRIDDEN}
      language="json"
      label="renovate.json"
      onClearOverride={() => {}}
    />
  {/snippet}
</Story>

<Story name="YAML">
  {#snippet template()}
    <CodeBlock lines={WORKFLOW} language="yaml" label=".github/workflows/test.yaml" />
  {/snippet}
</Story>

<Story name="TOML">
  {#snippet template()}
    <CodeBlock lines={CONFIG} language="toml" label=".smyklot.toml" />
  {/snippet}
</Story>

<Story name="Markdown">
  {#snippet template()}
    <CodeBlock lines={README} language="markdown" label="README.md" />
  {/snippet}
</Story>
