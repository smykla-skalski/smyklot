<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Card from '#lib/components/Card.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Card',
    component: Card,
    argTypes: {
      unsaved: { control: 'boolean' },
    },
    args: { unsaved: false },
  });
</script>

<!--
  The panel's one surface for a group of related things. How far it sits from
  what is above it is the stack's gap, not the card's own margin - so a page
  that lays cards out has nothing to remember and nowhere to invent a second
  distance.
-->
<Story name="One card">
  {#snippet template(args)}
    <Card unsaved={args.unsaved}>
      <div class="card-head"><h2 class="card-title">Merging</h2></div>
      <p>What a card holds is the caller's; the frame and its rhythm are not.</p>
    </Card>
  {/snippet}
</Story>

<!-- Two in a stack, one card-gap apart, said by the stack. -->
<Story name="A stack">
  {#snippet template(args)}
    <div class="card-stack">
      <Card unsaved={args.unsaved}>
        <div class="card-head"><h2 class="card-title">Behavior</h2></div>
        <p>The first.</p>
      </Card>
      <Card unsaved={args.unsaved}>
        <div class="card-head"><h2 class="card-title">Commands</h2></div>
        <p>The second, sixteen pixels below it.</p>
      </Card>
    </div>
  {/snippet}
</Story>

<!-- Holding a control whose change is staged and not yet saved. -->
<Story name="Unsaved">
  {#snippet template()}
    <Card unsaved>
      <div class="card-head"><h2 class="card-title">Labels</h2></div>
      <p>The frame carries the marker, so the whole group reads as changed.</p>
    </Card>
  {/snippet}
</Story>
