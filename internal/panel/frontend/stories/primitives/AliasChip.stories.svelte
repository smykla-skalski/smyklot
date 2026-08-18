<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import AliasChip from '#lib/components/AliasChip.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/AliasChip',
    component: AliasChip,
    args: { from: 'ship', to: 'merge', onRemove: fn() },
  });
</script>

<!--
  One alias: the word a comment can write, an arrow, and the command it stands for.
  The arrow is `aria-hidden` - it is punctuation between two values rather than a
  word, and read aloud it says "right arrow" in the middle of a pair a reader is
  trying to hear as one thing.
-->
<Story name="Saved">
  {#snippet template(args)}<AliasChip {...args} />{/snippet}
</Story>

<!--
  Unsaved rather than new. An alias whose command has CHANGED is drawn this way too,
  because what would be written differs from what is stored - which is the question
  the chip is answering, and it is not "is this row recent".
-->
<Story name="Unsaved">
  {#snippet template(args)}<AliasChip {...args} added />{/snippet}
</Story>

<!-- Read-only: the word stays, the way to remove it goes. -->
<Story name="Disabled">
  {#snippet template(args)}<AliasChip {...args} disabled />{/snippet}
</Story>
