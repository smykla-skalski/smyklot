<script lang="ts">
  const {
    checked,
    label,
    word,
    bare = false,
    disabled = false,
    onToggle,
  }: {
    checked: boolean;
    /** The accessible name; required because the track alone says nothing. */
    label: string;
    /** An optional visible word beside the track ("Syncing"). */
    word?: string;
    /**
     * Drops the 44px tap box so the switch cannot inflate the line it sits
     * on - for a head or a card row that sets its own rhythm. The hit area
     * survives on the input itself.
     */
    bare?: boolean;
    disabled?: boolean;
    onToggle: (next: boolean) => void;
  } = $props();
</script>

<!--
@component
A boolean a reader flips directly, rather than a button that opens a form to change
one. Flipping it IS the change, which is why it is an input and not a button - there
is nothing here to confirm.

What the flip reaches is the caller's business, and in this panel it is always a
draft: every call site stages the new value and marks the control dirty, so the change
lands when the page's save composer sends it rather than when the track moves. A
switch that applied immediately would be the same component with a different caller.

The track says nothing on its own, so `label` is required - the visible `word` beside
it is separate and usually absent. `bare` drops the 44px tap box for a head or a card
row that sets its own rhythm; the hit area survives on the input, which is what makes
that safe.
-->

<!-- A setting that has already taken effect: flipping it IS the change, so
     the control is an input rather than a button that opens a form. -->
<label class="switch" class:bare>
  {#if word !== undefined}<span class="switch-word">{word}</span>{/if}
  <input
    type="checkbox"
    {checked}
    {disabled}
    aria-label={label}
    onchange={(event) => onToggle(event.currentTarget.checked)}
  />
  <span class="switch-track"></span>
</label>

<style>
  .switch {
    align-items: center;
    display: inline-flex;
    gap: var(--space-2);
    /* As wide as the track and its word, whatever it is dropped into - see the note
       above `.chip` in app.css for why `inline-flex` does not say that on its own. */
    inline-size: fit-content;
    min-block-size: var(--touch-target, 2.75rem);
  }

  /* Where the switch stands beside content that sets the rhythm (a card
     head), the tap box must not inflate the row. */
  .switch.bare {
    min-block-size: auto;
  }

  /* AND THE TAP BOX COMES BACK AS PAINT RATHER THAN AS LAYOUT. The note here used to say
     the hit area survived on the input - it does not, and could not: that input is
     absolutely positioned, transparent and `pointer-events: none`, so a bare switch's
     whole target was its 34x20 track. WCAG 2.5.8 asks for 24px; this takes the same 44
     the label gives up, which fits because the rows these stand in are 51px apart and
     the box grows only where nothing is laid out. */
  .switch.bare .switch-track::before {
    content: '';
    inset: calc((20px - var(--touch-target)) / 2) 0;
    position: absolute;
  }

  .switch input {
    position: absolute;
    opacity: 0;
    pointer-events: none;
  }

  .switch-track {
    background: var(--switch-track-off);
    border: 1px solid var(--switch-track-off-border);
    border-radius: 999px;
    block-size: 20px;
    cursor: pointer;
    inline-size: 34px;
    padding: 1px;
    position: relative;
    transition:
      background var(--duration-fast) var(--ease-standard),
      border-color var(--duration-fast) var(--ease-standard);
  }

  .switch-track::after {
    background: var(--switch-thumb);
    border-radius: 50%;
    block-size: 16px;
    box-shadow: 0 1px 2px var(--shadow-color);
    content: '';
    inset-block-start: 1px;
    inset-inline-start: 1px;
    position: absolute;
    inline-size: 16px;
    transition: translate var(--duration-fast) var(--ease-standard);
  }

  .switch input:checked + .switch-track {
    background: var(--switch-track-on);
    border-color: var(--switch-track-on);
  }

  .switch input:checked + .switch-track::after {
    translate: 14px 0;
  }

  .switch input:focus-visible + .switch-track {
    outline: var(--focus-ring-width) solid var(--focus);
    outline-offset: var(--focus-ring-offset);
  }

  .switch input:disabled + .switch-track {
    cursor: not-allowed;
    opacity: 0.5;
  }

  .switch-word {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    text-box: trim-both cap alphabetic;
  }
</style>
