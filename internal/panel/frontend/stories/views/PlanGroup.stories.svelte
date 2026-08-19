<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import ApplyBar from '#lib/components/ApplyBar.svelte';
  import CodeBlock, { type CodeLine } from '#lib/components/CodeBlock.svelte';
  import PlanAction from '#lib/components/PlanAction.svelte';
  import PlanGroup from '#lib/components/PlanGroup.svelte';

  const { Story } = defineMeta({
    title: 'Views/PlanGroup',
    component: PlanGroup,
  });

  const at = (text: string, word: string): [number, number] => {
    const from = text.indexOf(word);

    return [from, from + word.length];
  };

  /** The change one opened row shows: the file's own colouring, marks over it. */
  const SCHEDULE: CodeLine[] = [
    { text: '  "extends": ["config:recommended"],', number: 2 },
    {
      text: '  "schedule": ["* 4 * * 0"],',
      op: '-',
      marks: [at('  "schedule": ["* 4 * * 0"],', '0')],
    },
    {
      text: '  "schedule": ["* 4 * * 1-5"],',
      op: '+',
      marks: [at('  "schedule": ["* 4 * * 1-5"],', '1-5')],
    },
    {
      text: '  "timezone": "Europe/Warsaw",',
      op: '+',
      marks: [at('  "timezone": "Europe/Warsaw",', '"Europe/Warsaw"')],
    },
    { text: '  "packageRules": [', number: 4 },
  ];
</script>

<!--
  A plan, grouped by the unit somebody is answerable for. The counts on each
  summary make folding honest: a group can be left shut and still read.

  The verb column is what a plan is scanned down - twelve adds and one removal
  is a shape. `remove` is the only verb carrying weight in both ink and stroke,
  because it is the only one that takes something away, and that is what
  approval is being asked for.
-->
<Story name="A plan waiting">
  {#snippet template()}
    <PlanGroup repository="af" added={3} changed={2} removed={1} open>
      <PlanAction
        op="add"
        kind="labels"
        what="dependencies"
        detail="— #0e8a16, dependency updates, mostly Renovate's"
      />
      <PlanAction op="add" kind="labels" what="good first issue" />
      <PlanAction op="change" kind="settings" what="squash merging" detail="off → on" />
      <PlanAction op="change" kind="settings" what="wiki" detail="on → off" />
      <PlanAction op="add" kind="files" what="renovate.json" detail="— as a pull request">
        {#snippet diff()}
          <CodeBlock lines={SCHEDULE} language="json" label="renovate.json - what would change" />
        {/snippet}
      </PlanAction>
      <PlanAction op="remove" kind="files" what=".github/stale.yml" detail="— retired above" />
    </PlanGroup>

    <PlanGroup repository="afi" added={3} changed={2}>
      <PlanAction op="change" kind="settings" what="auto-merge" detail="off → on" />
    </PlanGroup>

    <PlanGroup repository="harness" added={2} changed={1}>
      <PlanAction op="add" kind="files" what=".editorconfig" detail="— as a pull request" />
    </PlanGroup>

    <ApplyBar changes={14} repositories={3} removals={1} asPullRequests />
  {/snippet}
</Story>

<!--
  What a plan looks like when it has been applied and did not entirely work.
  The reason sits on the row: a reader approving the next plan has to be able to
  see what failed without opening anything.
-->
<Story name="After applying, with a refusal">
  {#snippet template()}
    <PlanGroup repository="smyklot-legacy" added={1} changed={1} open>
      <PlanAction op="change" kind="settings" what="delete branch on merge" detail="off → on" />
      <PlanAction
        op="add"
        kind="files"
        what=".github/workflows/ci.yaml"
        detail="— as a pull request"
        failure="Refused: writing .github/workflows needs the workflows permission — grant it on the installation's page"
      />
    </PlanGroup>
  {/snippet}
</Story>
