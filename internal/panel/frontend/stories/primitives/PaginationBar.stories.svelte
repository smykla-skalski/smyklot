<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import PaginationBar from '#lib/components/PaginationBar.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/PaginationBar',
    component: PaginationBar,
    argTypes: { disabled: { control: 'boolean' } },
    args: {
      label: 'Repositories',
      pageIndex: 0,
      pageCount: 5,
      pageSize: 20,
      itemCount: 20,
      total: 94,
      disabled: false,
      onPageSelect: fn(),
      onPageSizeSelect: fn(),
    },
  });
</script>

<Story name="First page" />

<Story name="In the middle" args={{ pageIndex: 2 }} />

<Story name="Last page" args={{ pageIndex: 4, itemCount: 14 }} />

<!-- One page of results: there is nowhere to go, and the control says so quietly. -->
<Story name="A single page" args={{ pageCount: 1, itemCount: 7, total: 7 }} />

<Story name="Disabled" args={{ disabled: true }} />
