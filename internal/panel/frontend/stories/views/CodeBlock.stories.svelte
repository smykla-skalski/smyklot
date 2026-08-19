<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import CodeBlock, { type CodeLine } from '#lib/components/CodeBlock.svelte';

  const { Story } = defineMeta({
    title: 'Views/CodeBlock',
    component: CodeBlock,
  });

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
  The template as it stands, with the two lines this installation decides. The
  gutter bar is the editor's own vocabulary for "managed here", said in a ground
  as well as in ink so it survives a reader who cannot separate the hues - and
  the file's own colouring survives it, because an overridden value is still a
  string.
-->
<Story name="A template with overridden lines">
  {#snippet template()}
    <CodeBlock lines={OVERRIDDEN} language="json" label="renovate.json" />
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
