<script lang="ts">
  import { Tooltip } from 'bits-ui';
  import type { Snippet } from 'svelte';

  const {
    id,
    text,
    align = 'center',
    side = 'top',
    mono = false,
    disabled = false,
    children,
  }: {
    id?: string;
    text: string;
    align?: 'start' | 'center' | 'end';
    side?: 'top' | 'right' | 'bottom' | 'left';
    /**
     * For a tooltip whose whole text is one identifier - a repository name, a
     * ref, a digest. The panel writes those in the mono face everywhere it
     * writes them, and a name that changes face on its way into a tooltip
     * reads as a different kind of thing.
     */
    mono?: boolean;
    /** A tip that has nothing to add right now stays quiet without unmounting its trigger. */
    disabled?: boolean;
    children: Snippet<[Record<string, unknown>]>;
  } = $props();
</script>

<!--
@component
The panel's one tooltip, and there is deliberately only one. It says what a control is
for when the control cannot say it itself - an icon button, a label that has been cut
off - and never anything the reader needs: a tip is not shown on a phone, it cannot be
reached by every input, and anything that must be read belongs on the page.

It takes a snippet rather than wrapping its child, so the trigger stays the caller's
own element and keeps its own markup. That is what lets `ClippedLabel` be a plain span
that becomes a trigger only once its text is actually clipped, and `HelpTip` a real
button.

`mono` is for a tip whose whole text is one identifier - a repository name, a ref, a
digest. The panel writes those in the mono face everywhere else, and a name that
changes face on its way into a tooltip reads as a different kind of thing.
-->

<Tooltip.Provider delayDuration={250}>
  <Tooltip.Root {disabled}>
    <Tooltip.Trigger>
      {#snippet child({ props })}
        {@render children(props)}
      {/snippet}
    </Tooltip.Trigger>
    <Tooltip.Portal to=".app-shell">
      <Tooltip.Content
        {id}
        class="app-tooltip-content{mono ? ' is-mono' : ''}"
        {side}
        {align}
        sideOffset={6}
        collisionPadding={8}
      >
        {text}
      </Tooltip.Content>
    </Tooltip.Portal>
  </Tooltip.Root>
</Tooltip.Provider>

<style>
  :global(.app-tooltip-content) {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    color: var(--text-secondary);
    font: 400 var(--font-size-meta) / var(--leading-meta) var(--sans);
    letter-spacing: normal;
    max-width: min(17rem, calc(100vw - 3rem));
    padding: 0.625rem 0.75rem;
    pointer-events: none;
    text-align: left;
    text-transform: none;
    white-space: normal;
    width: max-content;
    z-index: var(--layer-tooltip);
  }

  /* An identifier breaks anywhere, because a repository name has no words in
     it to break between and the alternative is a tooltip as wide as the page. */
  :global(.app-tooltip-content.is-mono) {
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    overflow-wrap: anywhere;
  }
</style>
