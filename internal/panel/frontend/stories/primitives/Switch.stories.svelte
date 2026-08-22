<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import Switch from '#lib/components/Switch.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Switch',
    component: Switch,
    argTypes: {
      checked: { control: 'boolean' },
      disabled: { control: 'boolean' },
      word: { control: 'text' },
    },
    args: {
      checked: true,
      label: 'Label sync',
      disabled: false,
      onToggle: fn(),
    },
  });
</script>

<!--
  A setting that has already taken effect: flipping it IS the change. On wears
  the brand because the product is acting; off is a quiet inset track.
-->
<Story name="On" />

<Story name="Off" args={{ checked: false }} />

<!-- The visible word names the act, where a page-level switch needs one. -->
<Story name="Worded" args={{ word: 'Syncing' }} />

<Story name="Disabled" args={{ disabled: true }} />
