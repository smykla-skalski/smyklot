<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';
  import DurationInput from '#lib/components/DurationInput.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/DurationInput',
    component: DurationInput,
    args: { value: 90, label: 'Quiet period', maximum: 86_400, onChange: fn() },
  });
</script>

<Story name="Seconds" />
<Story name="Hours" args={{ value: 3_600 }} />
<Story name="Inherited" args={{ value: null, inherited: 90, allowEmpty: true }} />
<Story name="Disabled" args={{ disabled: true }} />
<Story name="Invalid draft" args={{ editor: { amount: '0.5', unit: 'seconds' } }} />
