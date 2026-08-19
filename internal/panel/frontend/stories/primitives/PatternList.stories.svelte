<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import PatternList from '#lib/components/PatternList.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/PatternList',
    component: PatternList,
  });
</script>

<script lang="ts">
  // Live, because the whole point is that adding one is a press and removing
  // one is a press.
  let excludes = $state(['hand-made-*', 'LICENSE-*']);
  let checks = $state<string[]>([]);
</script>

<!--
  Six places on the sync pages hold a list of strings somebody types one at a
  time. Every one of them used to be a textarea - a control that asks a reader
  to know that lines are the separator, hides how many entries there are behind
  however many happen to fit, and answers a typo with nothing at all.
-->
<Story name="A few patterns">
  {#snippet template()}
    <PatternList
      values={excludes}
      label="Rulesets to leave alone"
      addLabel="Add a pattern"
      placeholder="hand-made-*"
      onChange={(next) => (excludes = next)}
    />
  {/snippet}
</Story>

<!-- Empty says what empty means here, rather than leaving the row blank. -->
<Story name="Nothing named">
  {#snippet template()}
    <PatternList
      values={checks}
      label="Checks that must pass"
      addLabel="Add a check"
      placeholder="test"
      empty="None — GitHub refuses a rule that names none"
      onChange={(next) => (checks = next)}
    />
  {/snippet}
</Story>

<!-- Read only: the entries stay legible and nothing offers to change them. -->
<Story name="Read only">
  {#snippet template()}
    <PatternList
      values={['.github/stale.yml']}
      label="Paths to remove"
      disabled
      onChange={() => {}}
    />
  {/snippet}
</Story>
