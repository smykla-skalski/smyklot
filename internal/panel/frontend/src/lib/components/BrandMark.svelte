<script lang="ts">
  import { basePath } from '#lib/paths.js';

  /* Addressed, not imported. Both cuts of the halo live in `static/` and only
     there: the favicon in `app.html` needs a name it can write down, and a
     bundled asset's carries a content hash nobody can predict - so the artwork
     has to be served verbatim anyway, and a second copy under `src/` for the
     importers to reach is one edit away from being a second artwork. Importing
     out of `static/` is the other thing that does not work: Vite's dev server
     refuses to serve it and the mark comes back broken. */
  const haloUrl = `${basePath}/smyklot-halo.svg`;

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

<!--
@component
The panel's own mark: the halo, the wordmark, and optionally the line naming which
console you are in.

`interior` is the one that matters. `solid` paints the halo's own dark ground behind
the robot; `clear` leaves it open so whatever the mark stands on shows through and the
ring reads as a window onto it, which is what the night pages want and what the sky
carries through. The disc is drawn as a sibling behind the image rather than set in the
asset, because the mark is an `<img>` and nothing in the host document can reach the
SVG's own toggle - and the sidebar and the night pages want opposite answers from the
same file.

`heading` makes it the page's `<h1>`, as it is in the sidebar. Only where the mark
really is the page's title; two `<h1>`s on one page is an accessibility fault, not a
style.

`part` names the console under the wordmark, and is omitted where the page says the
same thing elsewhere - the invitation page has it in the footer, and twice on one
screen is once too many.
-->

<svelte:element this={heading ? 'h1' : 'p'} class={['mark', stacked && 'stacked']}>
  <!-- The disc is a sibling behind the image rather than a setting in the file:
       the mark is an `<img>`, so nothing in the host document can reach the SVG's
       own `#halo-interior-background` toggle, and the sidebar and the night pages
       want opposite answers from the same asset. It paints what the file's solid
       setting paints, at the geometry the file draws it at - see the style. -->
  <span class={['mark-well', interior === 'solid' && 'grounded']}>
    <img class="mark-icon" src={haloUrl} alt="" width={size} height={size} decoding="async" />
  </span>
  <span class="mark-copy band-trim-kids">
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
    font: 700 0.8125rem / var(--leading-flat) var(--sans);
    letter-spacing: 0.11em;
    text-transform: uppercase;
  }

  .mark-part {
    color: var(--sidebar-text-muted);
    font: 700 0.65625rem / var(--leading-flat) var(--sans);
    letter-spacing: 0.12em;
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
