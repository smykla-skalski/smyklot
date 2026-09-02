<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import ListToolsMenu from '#lib/components/ListToolsMenu.svelte';
  import type { FilterSection } from '#lib/filter-menu.js';

  const FILE_STATES: readonly FilterSection[] = [
    {
      options: [
        { value: 'valid', label: 'Valid', tone: 'valid' },
        { value: 'missing', label: 'Missing', tone: 'missing' },
        { value: 'invalid', label: 'Invalid', tone: 'invalid' },
        { value: 'bypassed', label: 'Bypassed', tone: 'bypassed' },
      ],
    },
  ];

  const { Story } = defineMeta({
    title: 'Primitives/ListToolsMenu',
    component: ListToolsMenu,
    args: {
      sorts: [
        { label: 'Repository', direction: 'ascending' as const, onToggle: fn() },
        { label: 'Updated', direction: undefined, onToggle: fn() },
      ],
      filters: [
        {
          label: 'File',
          hint: 'Filter by the configuration file the service found',
          sections: FILE_STATES,
          selected: [],
          multiple: true,
          onChange: fn(),
        },
      ],
    },
  });
</script>

<!--
  A column heading is a fine place for a sort button and a funnel while there are
  columns. On a phone the table is a stack of cards and the headings are gone - on the
  repositories table that meant three sorts and three filters disappeared with them,
  and the page offered a search field and nothing else. This gathers them beside it.

  It owns no state: sorting and filtering already live where the table keeps them, and
  a second copy here would be a second answer to the same question.
-->
<Story name="Sorts and filters">
  {#snippet template(args)}
    <div class="frame"><ListToolsMenu {...args} /></div>
  {/snippet}
</Story>

<Story name="Something selected">
  {#snippet template(args)}
    <div class="frame">
      <ListToolsMenu
        {...args}
        filters={[{ ...args.filters[0], selected: ['missing', 'invalid'] }]}
      />
    </div>
  {/snippet}
</Story>

<style>
  .frame {
    display: flex;
    justify-content: flex-end;
    padding-bottom: 20rem;
  }
</style>
