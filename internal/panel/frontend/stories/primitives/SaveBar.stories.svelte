<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SaveBar from '#lib/components/SaveBar.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/SaveBar',
    component: SaveBar,
    args: { count: 3, onSave: fn(), onDiscard: fn() },
  });
</script>

<!--
  Floating: a slab fixed to the bottom of the viewport that rises when it appears.
  That is the account editor, where the rows being edited can be scrolled away from,
  so the bar has to follow and has to announce itself.

  `role="status"` and not `alert` - the count is worth hearing, but it changes on every
  keystroke in a text field and an assertive region would interrupt the typing that
  caused it.
-->
<Story name="Floating">
  {#snippet template(args)}<SaveBar {...args} />{/snippet}
</Story>

<!-- The singular, which is a sentence and not a count with an (s) after it. -->
<Story name="One change">
  {#snippet template(args)}<SaveBar {...args} count={1} />{/snippet}
</Story>

<!--
  Inline: under the rows it belongs to. It drops the animation, the dark ground and
  the status dot - a bar that cannot be scrolled away from need not announce its own
  arrival, and the row above already carries its unsaved marker.
-->
<Story name="Inline">
  {#snippet template(args)}<SaveBar {...args} inline />{/snippet}
</Story>

<!-- Mid-save. The button says so rather than going quiet, so the press had an effect. -->
<Story name="Saving">
  {#snippet template(args)}<SaveBar {...args} inline saving disabled />{/snippet}
</Story>
