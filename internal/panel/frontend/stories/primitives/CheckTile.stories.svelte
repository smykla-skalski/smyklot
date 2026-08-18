<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import CheckTile from '#lib/components/CheckTile.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/CheckTile',
    component: CheckTile,
    args: { label: 'merge', onchange: fn() },
  });
</script>

<!--
  A checkbox drawn as a tile. The native input is still the control - it is what a
  keyboard reaches, what a screen reader announces, and what carries every state below
  through a sibling selector. It is visually hidden, never replaced: a div with an
  onclick would mean rebuilding checked, focus and disabled by hand and getting one of
  them wrong.
-->
<Story name="Unchecked">
  {#snippet template(args)}<CheckTile {...args} />{/snippet}
</Story>

<Story name="Checked">
  {#snippet template(args)}<CheckTile {...args} checked />{/snippet}
</Story>

<!--
  Off and staying off. The editor disables a tile for two different reasons - it is
  read-only, or this is the last one checked and unchecking it would leave none - and
  both are the caller's to decide. The tile only draws the answer.
-->
<Story name="Disabled">
  {#snippet template(args)}<CheckTile {...args} disabled />{/snippet}
</Story>

<Story name="Checked and disabled">
  {#snippet template(args)}<CheckTile {...args} checked disabled />{/snippet}
</Story>
