<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import InheritControl from '#lib/components/InheritControl.svelte';

  const OPTIONS = [
    { value: 'enabled', label: 'Enabled' },
    { value: 'disabled', label: 'Disabled' },
  ];

  const { Story } = defineMeta({
    title: 'Primitives/InheritControl',
    component: InheritControl,
    argTypes: { disabled: { control: 'boolean' } },
    args: {
      label: 'Quiet success',
      source: 'the account',
      inheritedValue: 'enabled',
      inheritedLabel: 'Enabled',
      value: null,
      options: OPTIONS,
      disabled: false,
      onSelect: fn(),
      onRestore: fn(),
    },
  });
</script>

<!--
  Inheriting: no choice has been made here, so the control shows where the value
  comes from and what it lands on.
-->
<Story name="Inheriting" />

<!-- Overridden: the value is this level's own, and it can be given back. -->
<Story name="Overridden" args={{ value: 'disabled' }} />

<Story name="Read only" args={{ disabled: true }} />

<!-- A source that is plural reads differently in the sentence. -->
<Story name="Plural source" args={{ source: 'the account defaults', sourcePronoun: 'them' }} />
