<script lang="ts">
  import type { Snippet } from 'svelte';

  const {
    dot = false,
    live = false,
    state,
    icon,
    children,
  }: {
    /** For a state that is current rather than permanent. */
    dot?: boolean;
    /** A state that is arriving as it happens; pulses the dot. */
    live?: boolean;
    /** Colours the pill from the thing it reports on - the database's health, today. */
    state?: string;
    /** A symbol the words alone cannot carry, drawn before the label. */
    icon?: Snippet;
    children: Snippet;
  } = $props();
</script>

<!--
@component
A fact about the thing beside it, not a control. The distinction from `Chip` is what
each is for rather than how it looks: a chip carries a row's value and takes a tone
from the vocabulary its column draws, and a pill says what is true of the surface it
sits on - the scope of a page, the health of a dependency, whether changes are
arriving live. It is uppercase and quieter for that same reason: it is a caption, not
a value.

`dot` marks a state that is true right now rather than a fixed attribute, and `live`
sets it pulsing. Both were spelled by hand at seven call sites, along with the
`.cap-trim` the label needs to sit on its cap height.
-->

<!--
  The label is trimmed to its cap height because the pill centres its children's
  boxes, and a text box carries the leading above the capitals and the room under the
  baseline, which are never equal. Every call site remembered this; the component
  means none of them has to again.
-->
<span class="status-pill" data-state={state}>
  {#if icon !== undefined}
    {@render icon()}
  {/if}
  {#if dot}
    <span class="status-pill-dot" class:live aria-hidden="true"></span>
  {/if}
  <span class="cap-trim">{@render children()}</span>
</span>
