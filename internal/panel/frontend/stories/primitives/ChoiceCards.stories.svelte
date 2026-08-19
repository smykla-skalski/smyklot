<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import ChoiceCards from '#lib/components/ChoiceCards.svelte';

  const ENFORCEMENT = [
    { value: 'active', title: 'Active', why: 'The rules hold. A push that breaks one is refused.' },
    {
      value: 'evaluate',
      title: 'Evaluate',
      why: 'A dry run. Violations are recorded and shown here, nothing is blocked.',
    },
    {
      value: 'disabled',
      title: 'Disabled',
      why: 'Written to every repository but switched off there.',
    },
  ];

  const STRATEGY = [
    { value: 'deep-merge', title: 'Deep merge', why: 'Key by key, all the way down' },
    {
      value: 'shallow-merge',
      title: 'Shallow merge',
      why: 'Top-level keys only; nested objects replace whole',
    },
    { value: 'replace', title: 'Replace', why: 'The template is the file, byte for byte' },
  ];

  const { Story } = defineMeta({
    title: 'Primitives/ChoiceCards',
    component: ChoiceCards,
    args: { options: ENFORCEMENT, value: 'active', name: 'enforcement', label: 'Enforcement' },
  });
</script>

<script lang="ts">
  let strategy = $state('deep-merge');
</script>

<!--
  A saved choice whose options each need a sentence - enforcement modes, merge
  strategies, list rules. Where the words on a segment would carry the whole
  difference, this is the control instead; a wrong pick here is expensive.
-->
<Story name="Enforcement">
  {#snippet template()}
    <ChoiceCards
      options={ENFORCEMENT}
      value="active"
      name="enforce-demo"
      label="Enforcement"
      onSelect={() => {}}
    />
  {/snippet}
</Story>

<Story name="Merge strategy, live">
  {#snippet template()}
    <ChoiceCards
      options={STRATEGY}
      value={strategy}
      name="strategy-demo"
      label="Merge strategy"
      onSelect={(next) => (strategy = next)}
    />
  {/snippet}
</Story>

<Story name="Read only">
  {#snippet template()}
    <ChoiceCards
      options={STRATEGY}
      value="replace"
      name="strategy-ro"
      label="Merge strategy"
      disabled
      onSelect={() => {}}
    />
  {/snippet}
</Story>
