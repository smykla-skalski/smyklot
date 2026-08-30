<script lang="ts">
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';

  const {
    title,
    description,
    actionLabel,
    onAction,
  }: {
    title: string;
    description: string;
    actionLabel?: string;
    onAction?: () => void;
  } = $props();
</script>

<!--
@component
What a collection says when it has nothing to show. It names what happened and offers
the one next step, so it is never a dead end - an empty table that only says "no
results" leaves the reader to guess whether that is the filter, the permissions or the
data.

The difference from `Callout` is who is speaking. A callout says something about the
work a reader is looking at and sits beside it; this stands in place of the work,
because there is none. `actionLabel` is optional for the one case where the next step
is somewhere else entirely and the sentence has to carry it.

An empty list is not an error and is not drawn as one. A request that actually failed
is `ResultProblem`, which says so and offers the retry.
-->

<!-- The shape is `.table-notice` in app.css, shared with the failure that stands
     in the same place. All this decides is the glyph and the words. -->
<div class="table-notice">
  <span class="table-notice-mark" aria-hidden="true"><Icon name="search" size={22} /></span>
  <strong>{title}</strong>
  <span>{description}</span>
  {#if actionLabel !== undefined && onAction !== undefined}
    <!-- `Button` wraps the label so `app.css` can trim it: a button centres its
         label BOX, and the box carries the leading above the capitals and the room
         under the baseline, which are never equal. Bare, this word sat 0.47px above
         the middle of its own surface. -->
    <Button onclick={onAction}>{actionLabel}</Button>
  {/if}
</div>
