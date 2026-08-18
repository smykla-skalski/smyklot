<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import WorkspaceTrigger from '#lib/components/WorkspaceTrigger.svelte';
  import { TARGET } from '../support/fixtures.js';

  const { Story } = defineMeta({
    title: 'Primitives/WorkspaceTrigger',
    component: WorkspaceTrigger,
    args: { account: TARGET.account },
  });
</script>

<!--
  The row that opens the workspace list. Sibling to `AccountTrigger` and deliberately
  not the same component: they look alike in the rail but answer different questions.
  This one carries a kicker over the name, because "Workspace" says what the name below
  it IS - the account row needs no such label, since a face and a handle say what they
  are.
-->
<Story name="Workspace row">
  {#snippet template(args)}<WorkspaceTrigger {...args} />{/snippet}
</Story>

<!-- A long name, truncated sideways only: the trim moves the box and not the glyphs,
     so hiding the block axis would take the descenders off the name. -->
<Story name="Long name">
  {#snippet template(args)}
    <WorkspaceTrigger
      {...args}
      account={{ ...TARGET.account, display_name: 'Smykla Skalski Platform Engineering' }}
    />
  {/snippet}
</Story>
