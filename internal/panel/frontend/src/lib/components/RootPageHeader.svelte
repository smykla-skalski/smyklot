<script lang="ts">
  import type { Snippet } from 'svelte';

  const {
    role,
    title,
    subtitle,
    headingId = 'root-page-heading',
    showScope = true,
    children,
  }: {
    role: string;
    title: string;
    subtitle: string;
    headingId?: string;
    showScope?: boolean;
    children?: Snippet;
  } = $props();
</script>

<header class="root-page-header">
  <p class="root-kicker">
    <span class="cap-trim">Root mode · {role}</span>
    {#if showScope}
      <span class="status-pill"><span class="cap-trim">Application scope</span></span>
    {/if}
  </p>
  <h2 id={headingId}>{title}</h2>
  {#if children}
    <div class="header-slot">
      {@render children()}
    </div>
  {/if}
  <p class="root-subtitle">{subtitle}</p>
</header>

<style>
  /* One header anatomy shared with PanelHeader: kicker → title → subtitle → right
     slot. Scope is identity, so its pill lives on the kicker line — the slot holds
     only live status and real controls. */
  /* A grid, not a flex row: the slot shares the TITLE's row (row 2) and centres
     on it, rather than on the kicker + title + subtitle block. */
  .root-page-header {
    --title-size: var(--font-size-page-title);

    align-content: center;
    align-items: center;
    column-gap: var(--space-6);
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    /* The title's row is a control tall whether or not there is a control in it.
       Trimmed, the title is 20.86px, so a page with a segmented control in the
       slot would otherwise sit 6.57px further from its kicker and its subtitle
       than a page without one - the rhythm would depend on what the slot happens
       to hold. `minmax` rather than a fixed height: a taller slot still grows the
       row, and everything in it still centres. */
    grid-template-rows: auto minmax(var(--control-height-compact), auto) auto;
    min-height: 5.25rem;
    padding: var(--space-2) 0 var(--space-6);
  }

  .root-kicker {
    align-items: center;
    color: var(--brand-action-text);
    display: flex;
    font: 700 var(--font-size-micro) / 1 var(--sans);
    gap: 0.6rem;
    grid-column: 1;
    grid-row: 1;
    letter-spacing: 0.08em;
    /* The approved design's own step. With the title trimmed and its row a
       control tall, the ink-to-cap gap this produces is 29.05px, which is what
       the mock measures. */
    margin: 0 0 var(--space-3);
    text-transform: uppercase;
  }

  h2 {
    font-size: var(--title-size);
    font-weight: 700;
    grid-column: 1;
    grid-row: 2;
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
       on that slot instead. */
    text-box: trim-both cap alphabetic;
  }

  /* The meta size, as the approved design draws it: a subtitle explains the
     title, and at body size it competes with the page's own content for weight. */
  .root-subtitle {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    grid-column: 1;
    grid-row: 3;
    /* Whole pixels, same rule as the title: 1.5 of a 15px body is 22.5, and
       the half pixel pushed the toolbar row under this line off the device
       grid. */
    line-height: round(1.5em, 1px);
    /* Baseline to cap line, both boxes being their bands and the title's row a
       control tall: 18.57px, the gap the approved design measures. */
    margin: var(--space-3) 0 0;
    max-width: 52rem;
    text-box: trim-both cap alphabetic;
  }

  .header-slot {
    /* Header-slot controls share the toolbar height (34px), same rule as
       PanelHeader's actions slot. */
    --local-control-height: var(--control-height-compact);

    align-items: center;
    display: flex;
    flex: none;
    gap: var(--space-2);
    grid-column: 2;
    grid-row: 2;
    justify-self: end;
  }

  @media (max-width: 36rem) {
    .root-page-header {
      grid-template-columns: minmax(0, 1fr);
      padding-bottom: var(--space-4);
    }

    .header-slot {
      grid-column: 1;
      grid-row: 4;
      justify-self: stretch;
      margin-top: var(--space-3);
    }
  }
</style>
