<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import PageNavigation from '#lib/components/PageNavigation.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/PageNavigation',
    component: PageNavigation,
    argTypes: { disabled: { control: 'boolean' } },
    args: { pageIndex: 0, pageCount: 5, disabled: false, onSelect: fn() },
  });
</script>

<Story name="First page" />

<Story name="In the middle" args={{ pageIndex: 2 }} />

<Story name="Last page" args={{ pageIndex: 4 }} />

<!-- Many pages, so the numbers have to elide somewhere. -->
<Story name="Many pages" args={{ pageIndex: 8, pageCount: 24 }} />

<Story name="Disabled" args={{ disabled: true }} />
