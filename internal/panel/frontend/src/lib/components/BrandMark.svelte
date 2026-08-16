<script lang="ts">
  import haloUrl from '../../assets/smyklot-halo.svg';

  const {
    part,
    heading = false,
    stacked = false,
    interior = 'solid',
    size = 36,
  }: {
    /**
     * The line under the wordmark, naming the console. Written as it renders.
     * Omit it where the page says the same thing elsewhere - the invitation page
     * has it in the footer, and twice on one screen is once too many.
     */
    part?: string;
    /** Whether this mark is the page's own heading, as it is in the sidebar. */
    heading?: boolean;
    /** Icon over wordmark rather than beside it, for a mark standing on its own. */
    stacked?: boolean;
    /**
     * The ground inside the ring. `clear` leaves it open, so whatever the mark
     * stands on shows through and the ring reads as a window onto it - which is
     * what the night pages want, and what the sky carries through. `solid` paints
     * the halo's own dark interior behind the robot.
     *
     * It defaults to `solid` because the robot is drawn in white with fine dark
     * outlines: on a light ground with nothing behind it, it very nearly
     * disappears. A new caller that forgets this gets a mark that reads on any
     * background, and the worst a wrong default can do is hide a backdrop.
     */
    interior?: 'solid' | 'clear';
    /**
     * The icon's edge in pixels, glow included. The halo fills 89.25% of it - the
     * rest is the canvas the ring's bloom spills onto - so a mark that has to
     * measure the ring rather than the box multiplies by that. The wordmark
     * scales with this when stacked.
     */
    size?: number;
  } = $props();
</script>

<svelte:element this={heading ? 'h1' : 'p'} class={['mark', stacked && 'stacked']}>
  <!-- The disc is a sibling behind the image rather than a setting in the file:
       the mark is an `<img>`, so nothing in the host document can reach the SVG's
       own `#halo-interior-background` toggle, and the sidebar and the night pages
       want opposite answers from the same asset. It paints what the file's solid
       setting paints, at the geometry the file draws it at - see the style. -->
  <span class={['mark-well', interior === 'solid' && 'grounded']}>
    <img class="mark-icon" src={haloUrl} alt="" width={size} height={size} decoding="async" />
  </span>
  <span class="mark-copy">
    <span class="mark-name">Smyklot</span>
    {#if part !== undefined}
      <span class="mark-part">{part}</span>
    {/if}
  </span>
</svelte:element>

<style>
  .mark {
    align-items: center;
    display: flex;
    gap: 0.625rem;
    margin: 0;
    min-width: 0;
  }

  .mark-well {
    display: inline-flex;
    flex: none;
    position: relative;
  }

  /* The halo's interior, reproduced behind the image.
     ------------------------------------------------
     `smyklot-halo.svg` draws the ring in a 1340 box as a circle of r=556 stroked
     at 84, so the ring's inner edge - and the interior circle the file fills, at
     r=514 - is 1028/1340 of the box across, or 76.72%. `#09152B` is the colour
     the file's own `#halo-interior-solid-color` uses, so this is the file's solid
     setting rather than an invention.
     Behind the image, which is the order the file paints in too: interior first,
     then the robot, then the ring and its bloom over everything, so the glow
     still spills inward across this. */
  .mark-well.grounded::before {
    background: #09152b;
    border-radius: 50%;
    content: '';
    height: 76.72%;
    left: 50%;
    position: absolute;
    top: 50%;
    translate: -50% -50%;
    width: 76.72%;
  }

  .mark-icon {
    flex: none;
    object-fit: contain;
    position: relative;
  }

  .mark-copy {
    display: grid;
    gap: 0.3rem;
    min-width: 0;
  }

  /* The sidebar tokens are the mark's own: they are declared per theme at the
     document root, so a page with no sidebar - the invitation page - paints the
     mark in the same ink. They are not interchangeable with `--text-primary`,
     which stays dark under the Root light theme while the mark does not. */
  .mark-name {
    color: var(--sidebar-text);
    font: 700 0.8125rem / 1 var(--sans);
    letter-spacing: 0.11em;
    text-box: trim-both cap alphabetic;
    text-transform: uppercase;
  }

  .mark-part {
    color: var(--sidebar-text-muted);
    font: 700 0.65625rem / 1 var(--sans);
    letter-spacing: 0.12em;
    text-box: trim-both cap alphabetic;
  }

  /* Standing on its own rather than heading a rail: the icon carries the mark and
     the words sit under it, so the type steps up to match the larger disc. */
  .mark.stacked {
    flex-direction: column;
    gap: var(--space-3);
    text-align: center;
  }

  .mark.stacked .mark-copy {
    gap: 0.45rem;
    justify-items: center;
  }

  .mark.stacked .mark-name {
    font-size: 1rem;
    letter-spacing: 0.14em;
  }

  .mark.stacked .mark-part {
    font-size: 0.75rem;
    letter-spacing: 0.15em;
  }
</style>
