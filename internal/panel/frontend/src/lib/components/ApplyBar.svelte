<script lang="ts">
  /**
   * The sticky decision bar and its melt. At rest, in its own flow slot,
   * this is a plain card - the elevation story belongs ONLY to the moment
   * it is glued to the viewport riding over rows. Pure CSS: a sticky
   * element's view() timeline FREEZES while it is pinned, and advances the
   * moment it seats and rides the flow - so "stuck" is progress held below
   * the range, and the 16px of scroll past the dock is the melt. No JS, no
   * sentinel.
   *
   * The page hosting one must carry `timeline-scope: --bar-slot` on the
   * scrolling frame above the bar - the slot marker after the bar declares
   * the named timeline, and the scope is what hands it back up.
   */
  import type { Snippet } from 'svelte';

  const { children }: { children: Snippet } = $props();
</script>

<div class="apply-bar">
  {@render children()}
</div>
<span class="apply-bar-slot" aria-hidden="true"></span>

<style>
  .apply-bar {
    align-items: center;
    animation: apply-bar-seat linear both;
    /* NOT view() on the bar itself: Chromium folds a sticky subject's whole
       displacement into its view-timeline, so px offsets against `entry`
       land nowhere. The slot marker after the bar is plain flow - its
       geometry is the seat's truth. */
    animation-timeline: --bar-slot;
    /* The 1px slot reaches the 3.2rem dock line 51px into its entry; the bar
       unglues exactly there, and the next 16px of travel are the melt. */
    animation-range: entry calc(0% + 51px) entry calc(0% + 67px);
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    bottom: 3.2rem;
    box-shadow: var(--shadow-plate);
    display: flex;
    gap: var(--space-4);
    margin-top: var(--space-4);
    padding: var(--space-3) var(--space-4);
    position: sticky;
    z-index: 5;
  }

  /* The bar's seat, written in flow where sticky cannot smear it. */
  .apply-bar-slot {
    block-size: 1px;
    display: block;
    margin-block-start: -1px;
    view-timeline: --bar-slot block;
  }

  /* Glued (the from-state the frozen timeline holds): glass over the rows
     sliding under. Hairline only - the halo lives in ::after, which paints
     OVER ::before and would erase a bar shadow under itself. */
  @keyframes apply-bar-seat {
    from {
      --melt: 1;
      backdrop-filter: blur(10px);
      background: color-mix(in srgb, var(--surface-base) 86%, transparent);
      box-shadow: 0 -1px 0 var(--shadow-color);
    }

    to {
      --melt: 0;
      backdrop-filter: blur(0);
      background: var(--surface-base);
      box-shadow: var(--shadow-plate);
    }
  }

  /* Registered so the veil rides the same melt as the shadow, interpolated
     rather than flipped. Inherits: the keyframes run on the BAR and the
     pseudos read the value the way any descendant would. NOT named --scrim:
     that token is a colour at :root, and registering it as <number> voids
     the drawer overlay and dialog backdrop. */
  @property --melt {
    syntax: '<number>';
    inherits: true;
    initial-value: 0;
  }

  /* The OUTSIDE of the effect, in two layers with one job each.
     ::before is the DISSOLVE and nothing else: rows melt into their own
     surface before they pass under the glued edge, on a ramp tall enough
     (72px, solid for the last 16) to grade a row across two or three
     positions instead of biting one in half beside a crisp one. */
  .apply-bar::before {
    background: linear-gradient(to top, var(--surface-base) 16px, transparent);
    block-size: 72px;
    content: '';
    inset-block-end: calc(100% + 1px);
    /* Overhang the bar and melt the ends: a band cut exactly bar-wide dies
       in a vertical seam on each side, which no real shadow does. */
    inset-inline: -24px;
    mask-image: linear-gradient(
      to right,
      transparent,
      #000 24px,
      #000 calc(100% - 24px),
      transparent
    );
    opacity: var(--melt, 0);
    pointer-events: none;
    position: absolute;
  }

  /* ::after is the LIGHT: one halo drawn around the whole rounded box, so
     it wraps the corners and flanks instead of stopping at the top band.
     Upward-biased (the overlap is above), with one even layer so the sides
     and bottom carry their share. */
  .apply-bar::after {
    border-radius: inherit;
    box-shadow:
      0 -8px 24px -6px var(--shadow-color),
      0 -16px 32px -12px var(--shadow-color),
      0 2px 12px -3px var(--shadow-color);
    content: '';
    inset: 0;
    opacity: var(--melt, 0);
    pointer-events: none;
    position: absolute;
  }

  @media (max-width: 36rem) {
    .apply-bar {
      align-items: stretch;
      bottom: var(--space-2);
      display: grid;
      gap: var(--space-3);
      grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
      padding: var(--space-3);
    }

    .apply-bar :global(.apply-counts),
    .apply-bar :global(.apply-note) {
      grid-column: 1 / -1;
      min-inline-size: 0;
    }

    .apply-bar :global(.btn) {
      inline-size: 100%;
      justify-content: center;
      min-inline-size: 0;
      white-space: normal;
    }
  }
</style>
