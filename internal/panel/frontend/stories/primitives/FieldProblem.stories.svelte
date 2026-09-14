<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import FieldProblem from '#lib/components/FieldProblem.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/FieldProblem',
    component: FieldProblem,
    args: { id: 'setting-problem', message: 'Choose a supported value for this setting' },
  });
</script>

<Story name="Rejected field" />
<Story name="Cleared" args={{ message: null }} tags={['blank']} />
