<!--
@component
A card: the panel's one surface for a group of related things.

There is nothing to remember. The distance to whatever is above is the card's
own top margin, carried by `.card` itself rather than by a rule about the card
before it - so it cannot be forgotten and it needs no sibling to be true. It
collapses against a page header's larger exit, and a container that lays cards
out with a gap of its own turns it off.

That is what this replaces: a `block-gap-top` invented at 24px where the
distance is 16, the same rule copied into three pages' scoped styles, and that
invented class carried on a fourth page where it styled nothing.
-->

<script lang="ts">
  import type { Snippet } from 'svelte';

  const {
    class: className = '',
    unsaved = false,
    id,
    label,
    labelledby,
    children,
  }: {
    /** What this card is, beyond being one - never its padding or its frame. */
    class?: string;
    /** Holds a control whose change is staged and not yet saved. */
    unsaved?: boolean;
    id?: string;
    /** What names it, where the name is not a heading on the page. */
    label?: string;
    /** The heading that names it, for a card that is a labelled region. */
    labelledby?: string;
    children: Snippet;
  } = $props();
</script>

<div
  class="card {className}"
  class:is-unsaved={unsaved}
  data-unsaved={unsaved || undefined}
  {id}
  aria-label={label}
  aria-labelledby={labelledby}
  role={label === undefined && labelledby === undefined ? undefined : 'region'}
>
  {@render children()}
</div>

<style>
  /* The card's own look - the frame, the padding and the top margin - is `.card`
     in `app.css`, because it has to reach the two cards this component cannot
     render: a `<details>` that is a card, and any element a future one needs to
     be. What this component owns is the CONTRACT: the class is always there,
     the staged marker is a boolean rather than a class anybody spells, and
     there is nowhere left to invent a second gap. */
  .card.is-unsaved {
    border-color: color-mix(in srgb, var(--brand-action) 55%, var(--border-subtle));
  }
</style>
