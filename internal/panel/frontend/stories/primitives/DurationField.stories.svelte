<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import DurationField from '#lib/components/DurationField.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/DurationField',
    component: DurationField,
    argTypes: { disabled: { control: 'boolean' } },
    args: {
      label: 'Reaction sweep interval',
      amount: 30,
      unit: 'seconds',
      units: ['seconds', 'minutes', 'hours'],
      disabled: false,
      onApply: fn(),
    },
  });
</script>

<!--
  The Root settings page asks for three of these and had written the form three times
  over. The labels are visually hidden: the plate above has already named the setting,
  and saying it again over two controls in a row would say it three times - but a
  screen reader arrives at the fields with no plate heading in between, so it is still
  said.
-->
<Story name="Default" />

<Story name="Disabled" args={{ disabled: true }} />

<!-- A session is measured in longer units than a sweep. -->
<Story
  name="Longer units"
  args={{
    label: 'Session lifetime',
    amount: 12,
    unit: 'hours',
    units: ['minutes', 'hours', 'days'],
  }}
/>
