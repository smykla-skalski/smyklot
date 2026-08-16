<script lang="ts">
  import type { Snippet } from 'svelte';

  const {
    id,
    title,
    description,
    actions,
  }: {
    id: string;
    title: string;
    description: string;
    actions?: Snippet;
  } = $props();
</script>

<header class="panel-header">
  <h2 {id}>{title}</h2>
  {#if actions !== undefined}
    <div class="panel-header-actions">
      {@render actions()}
    </div>
  {/if}
  <p>{description}</p>
</header>

<style>
  /* A grid, not a flex row, so the action shares the TITLE's row and centres on
     it. Centring against the whole heading block hung the button below the
     title once a description wrapped underneath. */
  .panel-header {
    /* A flat token, not clamp(…, 2vw, …): the viewport-relative step resolved
       to 27.56px at this width, which is both a size the mock never uses and a
       fraction that every metric under it inherited. */
    --title-size: var(--font-size-page-title);

    align-content: center;
    align-items: center;
    column-gap: var(--space-6);
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    /* The title's row is a control tall whether or not there is a control in it.
       Trimmed, the title is 20.86px, so a page with actions would otherwise sit
       6.57px further from its description than a page without them - the rhythm
       would depend on what the slot happens to hold. `minmax` rather than a fixed
       height: a taller slot still grows the row, and everything in it still
       centres. */
    grid-template-rows: minmax(var(--control-height-compact), auto) auto;
    min-height: 5.25rem;
    padding: var(--space-2) 0 var(--space-6);
  }

  h2 {
    font-size: var(--title-size);
    font-weight: 700;
    grid-column: 1;
    grid-row: 1;
    letter-spacing: -0.03em;
    /* Rounded to whole pixels: 1.2 of a 28px title is 33.6, and a fractional
       line box puts the subtitle - and everything under it - on a half device
       pixel, which reads as a soft edge at 2x. Keyed to 1.2em, the element's
       OWN size: keying it to --title-size gave a 22px detail heading the 28px
       title's 34px line box. */
    line-height: round(1.2em, 1px);
    margin: 0;
    /* The title's box IS its band, which is what lets the slot beside it centre
       natively. There used to be a measured `translateY(round(0.0382em, 1px))`
       on that slot instead, correcting for the room a line box keeps under the
       baseline that the capitals never use. Trimmed, there is nothing to
       correct. */
    text-box: trim-both cap alphabetic;
  }

  /* The meta size, as the approved design draws it, and the same as the Root
     header's subtitle: the two headers are one anatomy and read alike. */
  p {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    grid-column: 1;
    grid-row: 2;
    /* Whole pixels, same rule as the title: 1.5 of a 15px body is 22.5, and
       the half pixel pushed the toolbar row under this line off the device
       grid. */
    line-height: round(1.5em, 1px);
    /* Baseline to cap line, now that both boxes are their bands and the title's
       row is a control tall: 18.57px, the gap the approved design measures. The
       number written here is the whole gap, rather than the number plus whatever
       leading the two fonts happened to carry. */
    margin: var(--space-3) 0 0;
    max-width: 52rem;
    text-box: trim-both cap alphabetic;
    text-wrap: balance;
  }

  .panel-header-actions {
    /* Header-slot controls share the toolbar height (34px) so the CTA reads
       as part of the control system, not a taller outlier. */
    --local-control-height: var(--control-height-compact);

    align-items: center;
    display: flex;
    flex: none;
    gap: var(--space-2);
    grid-column: 2;
    grid-row: 1;
    justify-self: end;
  }

  @media (max-width: 36rem) {
    .panel-header {
      grid-template-columns: minmax(0, 1fr);
      padding-bottom: var(--space-4);
    }

    .panel-header-actions {
      grid-column: 1;
      grid-row: 3;
      justify-self: stretch;
      margin-top: var(--space-3);
      width: 100%;
    }
  }
</style>
