<script lang="ts">
  /**
   * The rows a table draws while it is waiting for its first answer.
   *
   * Six tables wrote this by hand, and the copies had drifted: two of them had lost
   * the content bars entirely, the other four disagreed about how tall a row is,
   * where the bars start and how wide they run. None of that is accident - a
   * skeleton hints at the columns underneath it, so the geometry is per table in the
   * same way `grid-template-columns` is.
   *
   * So the shape is a prop and the measurements are custom properties, and every
   * caller keeps exactly what it had. What is shared is what should never have been
   * copied: the markup, the pulse, and the line that tells a screen reader what is
   * being waited for.
   *
   * A placeholder never replaces content that has already loaded - it stands in
   * before the first answer and not after it, which is the caller's decision to make.
   */
  const {
    rows = 6,
    bars = true,
    label,
  }: {
    rows?: number;
    /** Two bars per row, hinting at the columns. Off for a table whose rows are plain. */
    bars?: boolean;
    /** What is being waited for, announced rather than drawn. */
    label?: string;
  } = $props();
</script>

<div class="skeleton" class:bars aria-hidden="true">
  {#each { length: rows }, index (index)}
    <span></span>
  {/each}
</div>
{#if label !== undefined}
  <p class="visually-hidden" role="status">{label}</p>
{/if}

<style>
  .skeleton {
    display: grid;
    min-height: var(--skeleton-min-height, auto);
  }

  /* `skeleton-pulse` is declared in `app.css` so the two placeholders that keep
     their own markup animate with the same one. Reduced motion is answered there
     too, by the blanket rule that shortens every animation. */
  .skeleton span {
    animation: skeleton-pulse 1.35s ease-in-out infinite alternate;
    border-bottom: 1px solid var(--rule);
    display: block;
    height: var(--skeleton-row-height, 3.5rem);
    position: relative;
  }

  .skeleton.bars span::before,
  .skeleton.bars span::after {
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    content: '';
    height: 0.75rem;
    left: var(--space-4);
    position: absolute;
    top: var(--skeleton-bar-top, 1rem);
    width: var(--skeleton-bar-a-width, min(13rem, 28%));
  }

  .skeleton.bars span::after {
    left: var(--skeleton-bar-b-left, 46%);
    width: var(--skeleton-bar-b-width, min(8rem, 18%));
  }
</style>
