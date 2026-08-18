<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import FilterMenu from '#lib/components/FilterMenu.svelte';
  import type { FilterSection } from '#lib/filter-menu.js';

  const STATES: readonly FilterSection[] = [
    {
      options: [
        { value: 'passing', label: 'Passing', tone: 'valid', icon: 'success' },
        { value: 'pending', label: 'Running', tone: 'neutral', icon: 'pending' },
        { value: 'failing', label: 'Failing', tone: 'invalid', icon: 'failure' },
        { value: 'no_checks', label: 'No checks', tone: 'missing', icon: 'minus-circle' },
      ],
    },
  ];

  const SCOPED: readonly FilterSection[] = [
    { options: [{ value: 'all', label: 'All settings', exclusive: true }] },
    {
      label: 'Behaviour',
      options: [
        { value: 'quiet_success', label: 'Quiet success' },
        { value: 'allow_self_approval', label: 'Allow self approval' },
      ],
    },
  ];

  const { Story } = defineMeta({
    title: 'Primitives/FilterMenu',
    component: FilterMenu,
    args: {
      label: 'State',
      summary: 'All states',
      hint: 'Filter by what the checks last said',
      sections: STATES,
      selected: [],
      onChange: fn(),
    },
  });
</script>

<!--
  No state here is told apart by hue alone - each option carries its own glyph -
  because several of these pairs cannot survive one dichromacy or another. The menu
  offers exactly the states its column can draw; `tests/queue-vocabulary.test.ts`
  compares the two lists so they cannot part.
-->
<Story name="Nothing selected">
  {#snippet template(args)}
    <div class="frame"><FilterMenu {...args} /></div>
  {/snippet}
</Story>

<Story name="Some selected" args={{ selected: ['passing', 'failing'], summary: '2 states' }}>
  {#snippet template(args)}
    <div class="frame"><FilterMenu {...args} /></div>
  {/snippet}
</Story>

<!-- Sections group the options, and an exclusive option clears the rest. -->
<Story
  name="Grouped"
  args={{
    label: 'Settings',
    summary: 'All settings',
    sections: SCOPED,
    multiple: true,
    fallbackValue: 'all',
  }}
>
  {#snippet template(args)}
    <div class="frame"><FilterMenu {...args} /></div>
  {/snippet}
</Story>

<style>
  .frame {
    padding-bottom: 18rem;
  }
</style>
