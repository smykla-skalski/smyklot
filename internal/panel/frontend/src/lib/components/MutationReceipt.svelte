<script lang="ts">
  import { receipts } from '#lib/receipts.svelte.js';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';

  /** How long a timed receipt keeps the floor, and what a hover holds. */
  const LINGER_MS = 6_000;

  const current = $derived(receipts.current);
  let lift = $state(0);
  let held = $state(false);

  /* A receipt reads as one line, so a new one replaces the old rather than sliding a
     second box in beside it - and the timer restarts with the words. */
  $effect(() => {
    const receipt = current;
    if (receipt === undefined || receipt === null || receipt.sticky === true) return;
    if (held) return;
    const timer = window.setTimeout(() => receipts.dismiss(), LINGER_MS);

    return () => window.clearTimeout(timer);
  });

  /* THE RECEIPT NEVER SITS ON AN APPLY BUTTON. It is drawn at the foot of the window
     and so is every sticky decision bar in the product, so it measures the highest one
     on screen and stands above it. Measured rather than declared: which bar is up is a
     fact about the page being read, not about this component. */
  function measure(): void {
    let above = 0;
    for (const bar of document.querySelectorAll('.apply-bar, .settings-composer')) {
      const box = bar.getBoundingClientRect();
      if (box.width === 0 && box.height === 0) continue;
      if (box.top < window.innerHeight && box.bottom > 0) {
        above = Math.max(above, window.innerHeight - box.top);
      }
    }
    lift = above === 0 ? 0 : Math.round(above) + 12;
  }

  $effect(() => {
    if (current === null) {
      lift = 0;
      return;
    }
    measure();
    const onScroll = (): void => measure();
    window.addEventListener('scroll', onScroll, { passive: true });
    window.addEventListener('resize', onScroll);

    return () => {
      window.removeEventListener('scroll', onScroll);
      window.removeEventListener('resize', onScroll);
    };
  });

  function hold(): void {
    held = true;
  }

  function release(event: FocusEvent | PointerEvent): void {
    const next = (event as FocusEvent).relatedTarget;
    if (next instanceof Node && event.currentTarget instanceof Node) {
      if (event.currentTarget.contains(next)) return;
    }
    held = false;
  }
</script>

<!--
@component
The receipt for a change: what happened, and the way back where there is one.

Mounted once by the shell, because a change made in a dialog is reported after the
dialog has closed and a receipt owned by the page underneath would go with it.
-->

{#if current !== null}
  <div
    class="toast"
    role="status"
    style:--toast-lift="{lift}px"
    onpointerenter={hold}
    onpointerleave={release}
    onfocusin={hold}
    onfocusout={release}
  >
    <span class="toast-say">{current.say}</span>
    {#if current.undo !== undefined}
      <Button onclick={() => receipts.undo()}>Undo</Button>
    {/if}
    <Button tone="quiet" onclick={() => receipts.dismiss()}>
      {#snippet icon()}<Icon name="close" size="sm" />{/snippet}
      <span class="visually-hidden">Dismiss</span>
    </Button>
  </div>
{/if}

<style>
  .toast {
    align-items: center;
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--r-strip);
    box-shadow: var(--shadow-popover);
    color: var(--text-primary);
    display: flex;
    font-size: var(--font-size-meta);
    gap: var(--space-3);
    /* One overlay stack: the base offset, plus whatever sticky action bar is on
       screen, plus the phone's safe area. */
    inset-block-end: calc(
      max(var(--space-4), env(safe-area-inset-bottom)) + var(--toast-lift, 0px)
    );
    inset-inline-start: 50%;
    max-inline-size: min(34rem, calc(100vw - 2rem));
    padding: var(--space-2) var(--space-2) var(--space-2) var(--space-4);
    position: fixed;
    translate: -50% 0;
    z-index: var(--layer-toast);
  }

  .toast-say {
    line-height: var(--leading-meta);
    /* The words wrap; the buttons beside them never shrink. */
    min-inline-size: 0;
    text-box: trim-both cap alphabetic;
  }
</style>
