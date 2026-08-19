<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import PlanAction from '#lib/components/PlanAction.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/PlanAction',
    component: PlanAction,
  });
</script>

<!--
  The three verbs, together, which is the only way to judge them: the column is
  read down, and `remove` has to be the one that stops the eye.
-->
<Story name="The three verbs">
  {#snippet template()}
    <div class="rows">
      <PlanAction op="add" kind="labels" what="dependencies" detail="— #0e8a16" />
      <PlanAction op="change" kind="settings" what="squash merging" detail="off → on" />
      <PlanAction op="remove" kind="files" what=".github/stale.yml" detail="— retired above" />
    </div>
  {/snippet}
</Story>

<!-- A row that did not work says so where it stands. -->
<Story name="One that was refused">
  {#snippet template()}
    <div class="rows">
      <PlanAction
        op="add"
        kind="files"
        what=".github/workflows/ci.yaml"
        detail="— as a pull request"
        failure="Refused: writing .github/workflows needs the workflows permission"
      />
    </div>
  {/snippet}
</Story>

<style>
  .rows {
    display: grid;
  }
</style>
