<script lang="ts">
  import Chip from './Chip.svelte';

  /**
   * A receipt for something already on the clipboard.
   *
   * It is not a status the dialog carries, it is an answer to a press, so it leaves once it has
   * been read - a confirmation that never goes away stops being read as an answer and starts being
   * read as part of the dialog. Failures are the other kind of message and do not belong here: a
   * clipboard that refused is an instruction to copy the field by hand, and an instruction has to
   * stay next to the field it is about.
   *
   * Meant for the slot beside a dialog title. The chip is out of flow, so arriving and leaving
   * cannot reflow the heading beside it; it hangs to the left of wherever the slot sits, which is
   * the right edge of a modal header.
   */

  const {
    shown,
    pulse,
    message = 'Copied to your clipboard',
    onDone,
  }: {
    shown: boolean;
    /** Bumped by the caller on every copy, so a press during a receipt gets its own. */
    pulse: number;
    message?: string;
    /** Called when the receipt has played out and the caller should forget it. */
    onDone: () => void;
  } = $props();
</script>

<!-- The region is here whether or not it holds anything: a live region inserted with its text
     already in it is announced by some readers and not others, one already in the document by all
     of them. `animationend` rather than a timer, so the lifetime is stated once, in the animation
     that plays it out. -->
<span class="copy-receipt" role="status" onanimationend={onDone}>
  {#if shown}
    {#key pulse}
      <Chip tone="clear" icon="check">{message}</Chip>
    {/key}
  {/if}
</span>

<style>
  .copy-receipt {
    /* How long a receipt lives, stated once: long enough to be read twice. The animation is what
       ends it, so this is the message's lifetime rather than a motion setting. */
    --receipt-life: 2.6s;

    display: block;
    position: relative;
  }

  .copy-receipt :global(.chip) {
    animation: copy-receipt var(--receipt-life) var(--ease-out) both;
    position: absolute;
    right: 0;
    top: 0;
  }

  @keyframes copy-receipt {
    0% {
      opacity: 0;
      scale: 0.94;
      translate: 0 -0.3rem;
    }

    6%,
    88% {
      opacity: 1;
      scale: 1;
      translate: 0 0;
    }

    100% {
      opacity: 0;
      scale: 0.98;
      translate: 0 -0.2rem;
    }
  }

  /* Same lifetime, no travel.

     The app-wide reduced-motion rule squashes every animation to 0.01ms, which is right for motion
     and wrong here: this animation is not movement, it is how long the message stays. Left
     squashed, the receipt was removed on the frame it appeared and a reduced-motion reader never
     saw it at all. The duration is re-asserted at the same weight and higher specificity; what
     goes is the travel, in the keyframes below. */
  @media (prefers-reduced-motion: reduce) {
    .copy-receipt :global(.chip) {
      animation-duration: var(--receipt-life) !important;
    }

    @keyframes copy-receipt {
      0%,
      100% {
        opacity: 0;
      }

      6%,
      88% {
        opacity: 1;
      }
    }
  }
</style>
