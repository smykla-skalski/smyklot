<script lang="ts">
  import Button from './Button.svelte';

  const {
    title,
    description,
    actionLabel,
    onAction,
  }: {
    /** The state, named. Written without its full stop - this adds it. */
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

This is a `.state-panel`, which is the ONE recipe for every non-happy state a page can
be in. It used to be a second one: a centred column under a tinted disc holding a
magnifying glass, sized and spaced unlike anything else that answers the same question.
The glass was the tell - a search icon over a list that was never searched says a query
came up dry when nothing was ever asked, and the design the panel is drawn from refuses
it by name.

The full stop after the title is added here rather than written by the caller. Every
panel in the drawing ends its opener with one, and a rule nine call sites have to
remember separately is a rule that ends up spelled eight ways.
-->

<div class="state-panel">
  <span><strong>{title}.</strong> {description}</span>
  {#if actionLabel !== undefined && onAction !== undefined}
    <!-- `Button` wraps the label so `app.css` can trim it: a button centres its
         label BOX, and the box carries the leading above the capitals and the room
         under the baseline, which are never equal. Bare, this word sat 0.47px above
         the middle of its own surface. -->
    <Button onclick={onAction}>{actionLabel}</Button>
  {/if}
</div>
