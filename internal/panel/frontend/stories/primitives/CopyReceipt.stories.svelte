<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import CopyReceipt from '#lib/components/CopyReceipt.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/CopyReceipt',
    component: CopyReceipt,
    argTypes: { shown: { control: 'boolean' } },
    args: { shown: true, pulse: 1, onDone: fn() },
  });
</script>

<!--
  `pulse` is bumped by the caller on every copy, so pressing again while a receipt is
  still on screen gets its own rather than extending the first.
-->
<Story name="Shown" />

<Story name="Its own words" args={{ message: 'Invitation link copied' }} />

<!-- Between copies, which is where this component spends its life. -->
<Story name="Hidden" args={{ shown: false }} tags={['blank']} />
