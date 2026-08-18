<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SearchField from '#lib/components/SearchField.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/SearchField',
    component: SearchField,
    args: {
      label: 'Search repositories',
      placeholder: 'Search repositories',
      value: '',
      onInput: fn(),
    },
  });
</script>

<Story name="Empty" />

<Story name="With a query" args={{ value: 'platform' }} />
