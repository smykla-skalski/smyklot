<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';
  import CursorPaginationBar from '#lib/components/CursorPaginationBar.svelte';
  const { Story } = defineMeta({
    title: 'Primitives/CursorPaginationBar',
    component: CursorPaginationBar,
    args: {
      label: 'Sync history',
      count: 20,
      total: 45,
      pageSize: 20,
      canPrevious: false,
      canNext: true,
      onPrevious: fn(),
      onNext: fn(),
      onPageSizeSelect: fn(),
    },
  });
</script>

<Story name="First page" />
<Story name="Middle page" args={{ canPrevious: true }} />
<Story name="Last page" args={{ canPrevious: true, canNext: false, count: 5 }} />
<Story name="Loading" args={{ busy: true }} />
<Story name="Empty" args={{ count: 0, total: 0, canNext: false }} />
