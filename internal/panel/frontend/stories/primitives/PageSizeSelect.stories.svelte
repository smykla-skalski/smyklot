<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import PageSizeSelect from '#lib/components/PageSizeSelect.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/PageSizeSelect',
    component: PageSizeSelect,
    args: { value: 20, label: 'Rows per page', onSelect: fn() },
  });
</script>

<!-- The one select that sets its own width, so the number does not swim in the box. -->
<Story name="Default" />

<Story name="A larger page" args={{ value: 100 }} />
