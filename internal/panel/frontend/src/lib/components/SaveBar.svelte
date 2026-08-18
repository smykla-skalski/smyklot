<script lang="ts">
  import Button from './Button.svelte';

  /**
   * The bar that appears when a form has unsaved changes, and says how many.
   *
   * Two shapes, one component. Floating, it is a slab fixed to the bottom of the
   * viewport that rises when it appears - that is the account editor, where the rows
   * being edited can be scrolled away from. Inline, it sits under the rows it belongs
   * to and drops the animation, the dark ground and the status dot, because a bar that
   * cannot be scrolled away from does not need to announce its own arrival, and the
   * row above it already carries its unsaved marker.
   *
   * `role="status"` rather than `alert`: the count changing is worth hearing, but it
   * changes on every keystroke in a text field and an assertive live region would
   * interrupt the typing that caused it.
   *
   * Discard is a bare `<button>` on purpose and not a `Button` tone. On the floating
   * bar it stands on the dark slab and inherits its ink, which no tone in the set
   * does - `quiet` is built against the page's ground. Giving it one would mean a
   * tone that exists for one control on one surface.
   */
  const {
    count,
    saving = false,
    disabled = false,
    inline = false,
    onSave,
    onDiscard,
  }: {
    /** How many settings differ from what is stored. */
    count: number;
    /** A save is in flight; the button says so rather than going quiet. */
    saving?: boolean;
    disabled?: boolean;
    /** Under the rows rather than fixed to the viewport. */
    inline?: boolean;
    onSave?: () => void;
    onDiscard?: () => void;
  } = $props();
</script>

<div class="save-bar" class:save-bar-inline={inline} role="status">
  <span class="save-dot" aria-hidden="true"></span>
  <span class="save-count">
    {count} unsaved {count === 1 ? 'change' : 'changes'}
  </span>
  <button class="bar-ghost" type="button" {disabled} onclick={() => onDiscard?.()}>
    Discard
  </button>
  <Button tone="signal" {disabled} onclick={() => onSave?.()}>
    {saving ? 'Saving…' : 'Save'}
  </Button>
</div>

<style>
  .save-bar {
    align-items: center;
    animation: save-bar-rise 240ms var(--ease-standard);
    background: var(--text-primary);
    border-radius: 12px;
    bottom: 1.25rem;
    box-shadow: 0 12px 32px rgb(0 0 0 / 30%);
    color: var(--canvas);
    display: flex;
    font: 600 var(--font-size-control) / 1 var(--sans);
    gap: 0.875rem;
    left: 50%;
    padding: 0.625rem 0.75rem 0.625rem 1rem;
    position: fixed;
    transform: translateX(-50%);
    z-index: var(--layer-sticky);
  }

  .save-bar-inline {
    animation: none;
    bottom: auto;
    font-size: var(--font-size-compact);
    gap: var(--space-3);
    justify-content: flex-end;
    left: auto;
    margin-top: 1.125rem;
    padding: 0 calc(0.875rem + 1px) 0 0;
    position: static;
    transform: none;
  }

  @keyframes save-bar-rise {
    from {
      opacity: 0;
      transform: translate(-50%, 1rem);
    }

    to {
      opacity: 1;
      transform: translate(-50%, 0);
    }
  }

  .save-dot {
    animation: save-dot-pulse 1.6s ease-in-out infinite;
    background: var(--pending-inverse);
    border-radius: 50%;
    flex: none;
    height: 8px;
    width: 8px;
  }

  @keyframes save-dot-pulse {
    0%,
    100% {
      opacity: 1;
    }

    50% {
      opacity: 0.35;
    }
  }

  .bar-ghost {
    background: none;
    border: 0;
    border-radius: var(--r-ctl);
    color: inherit;
    cursor: pointer;
    font: 600 var(--font-size-control) / 1 var(--sans);
    opacity: 0.75;
    padding: 0.5rem 0.625rem;
  }

  .save-bar-inline .save-dot {
    display: none;
  }

  .save-bar-inline .save-count {
    color: var(--text-secondary);
    font-weight: 400;
  }

  .save-bar-inline .bar-ghost {
    /* The transparent border is load-bearing, same as the segmented control:
       the button recipe beside it is 1px border + 0.9rem, so padding alone
       leaves this one 2px narrower than the Save it sits next to. */
    border: 1px solid transparent;
    color: var(--text-primary);
    font: 600 var(--font-size-compact) / 1 var(--sans);
    height: var(--control-height-compact);
    opacity: 1;
    padding: 0 0.9rem;
  }

  .save-bar-inline {
    background: transparent;
    box-shadow: none;
    color: var(--text-secondary);
  }

  .bar-ghost:hover:not(:disabled) {
    background: rgb(255 255 255 / 12%);
    opacity: 1;
  }

  .save-bar-inline .bar-ghost:hover:not(:disabled) {
    background: var(--well);
  }

  /* The count and Discard take the trim every label in the product takes. The Save's
     own label is not here: `Button` wraps it in `.button-label`, which `app.css` trims
     the same way. One copy, not two. */
  .save-count,
  .bar-ghost {
    text-box: trim-both cap alphabetic;
  }

  @media (prefers-reduced-motion: reduce) {
    .save-bar {
      animation: none;
    }

    .save-dot {
      animation: none;
    }
  }
</style>
