<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import FormError from '#lib/components/FormError.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/FormError',
    component: FormError,
    args: { message: 'That login is already a Root on this installation' },
  });
</script>

<!--
  Twelve call sites wrote this line, and the part that matters is the part easiest to
  leave off: `role="alert"`. Without it the message paints and nothing announces it,
  so a reader who cannot see the form is told the submission simply did not happen.
-->
<Story name="Message" />

<!-- Nothing renders when there is nothing to say, so callers stop writing the guard. -->
<!-- No message, so nothing at all: the box is the message. -->
<Story name="Empty" args={{ message: null }} tags={['blank']} />
