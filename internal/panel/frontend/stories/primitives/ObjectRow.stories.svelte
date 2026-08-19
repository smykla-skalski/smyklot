<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Button from '#lib/components/Button.svelte';
  import Chip from '#lib/components/Chip.svelte';
  import ObjectRow from '#lib/components/ObjectRow.svelte';
  import StateMark from '#lib/components/StateMark.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/ObjectRow',
    component: ObjectRow,
  });
</script>

<!--
  A ruleset and a shared file are the same shape to read: a name somebody has to
  match character for character against GitHub, one line saying what it is, and
  what it would do to the fleet.
-->
<Story name="A ruleset">
  {#snippet template()}
    <ObjectRow
      name="main-branch-protection"
      href="#ruleset"
      summary="default branch · 6 rules · 2 bypass actors"
    >
      {#snippet pill()}
        <Chip tone="signal" small>Active</Chip>
      {/snippet}
      {#snippet mark()}
        <StateMark state="change" label="1 repository differs" />
      {/snippet}
    </ObjectRow>
  {/snippet}
</Story>

<!-- Evaluate is a ruleset that looks enforced and enforces nothing, so it is
     worn on the row rather than waiting behind a press. -->
<Story name="Evaluating, and in step">
  {#snippet template()}
    <ObjectRow name="release-tags" href="#ruleset" summary="refs/tags/v* · 2 rules · no bypass">
      {#snippet pill()}
        <Chip tone="warning" small>Evaluate</Chip>
      {/snippet}
      {#snippet mark()}
        <StateMark state="settled" />
      {/snippet}
    </ObjectRow>
  {/snippet}
</Story>

<Story name="A file the plan refused">
  {#snippet template()}
    <ObjectRow
      name=".github/workflows/ci.yaml"
      href="#file"
      summary="no adjustments · updated 5 days ago"
    >
      {#snippet pill()}
        <Chip tone="neutral" small>replaces</Chip>
      {/snippet}
      {#snippet mark()}
        <StateMark state="refused" label="1 refused" />
      {/snippet}
    </ObjectRow>
  {/snippet}
</Story>

<!-- Not every row is a way in. A repository's adjustment is a statement with a
     control on it, and it carries no chevron, because there is nowhere to go. -->
<Story name="A statement rather than a link">
  {#snippet template()}
    <ObjectRow name="af" summary="changes 2 keys — schedule, timezone">
      {#snippet action()}
        <Button tone="quiet">Edit</Button>
      {/snippet}
    </ObjectRow>
  {/snippet}
</Story>
