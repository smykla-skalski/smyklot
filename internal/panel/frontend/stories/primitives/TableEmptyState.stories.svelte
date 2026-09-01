<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import TableEmptyState from '#lib/components/TableEmptyState.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/TableEmptyState',
    component: TableEmptyState,
    args: {
      title: 'No repositories match this search',
      description: 'Clear the search or widen the filters to see the rest',
    },
  });
</script>

<!-- The shape is `.state-panel` in app.css - the one recipe for every non-happy state a
     page can be in, shared with the failure that stands in the same place and with the
     no-match answer on every searchable list. All this decides is the words. -->
<Story name="Nothing to show" />

<Story name="With a way out" args={{ actionLabel: 'Clear filters', onAction: fn() }} />
