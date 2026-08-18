<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import ResultProblem from '#lib/components/ResultProblem.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/ResultProblem',
    component: ResultProblem,
    argTypes: {
      busy: { control: 'boolean' },
      overContent: { control: 'boolean' },
    },
    args: {
      title: 'Repositories could not be loaded',
      problem: 'The service did not answer in time',
      busy: false,
      overContent: false,
      onRetry: fn(),
    },
  });
</script>

<!-- Nothing has loaded yet, so the problem stands in place of the content. -->
<Story name="Instead of content" />

<!--
  A refresh that failed over a list already read has not made the list wrong, so the
  notice sits over the content rather than replacing it.
-->
<Story name="Over content" args={{ overContent: true }} />

<Story name="Trying again" args={{ busy: true }} />
