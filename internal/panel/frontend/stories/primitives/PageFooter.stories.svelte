<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import PageFooter from '#lib/components/PageFooter.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/PageFooter',
    component: PageFooter,
    args: { build: { version: '1.37.0', serviceHost: 'smyklot.com' } },
  });
</script>

<!--
  The items centre against each other by their trimmed boxes rather than by
  per-item nudges - see the note in the component.
-->
<Story name="Default" />

<Story name="Version only" args={{ build: { version: '1.37.0', serviceHost: null } }} />

<!-- Nothing to say, so nothing is drawn. -->
<!-- A build that named neither a version nor a host leaves the footer with nothing to
     stand on, so it stands down rather than drawing an empty rule. -->
<Story
  name="Nothing to report"
  args={{ build: { version: null, serviceHost: null } }}
  tags={['blank']}
/>
