<script lang="ts">
  import type { Snippet } from 'svelte';

  const {
    id,
    title,
    description,
    eyebrow,
    kicker,
    actions,
  }: {
    /** The id the page's `aria-labelledby` points at. */
    id: string;
    title: string;
    /** One line saying what the page is for. */
    description: string;
    /** Plain section context above the title, such as Access or History. */
    eyebrow?: string;
    /** Above the title - on the Root console, whose authority this page is under. */
    kicker?: Snippet;
    /** Live status and the controls that act on the page. Never identity. */
    actions?: Snippet;
  } = $props();
</script>

<!--
@component
The top of a page: a title, what it is for, and the controls that act on it.

There were two of these - `PanelHeader` and `RootPageHeader` - and 72 of their
~81 CSS lines were identical, comments included. `RootPageHeader`'s own comment
said so: "one header anatomy shared with PanelHeader". `HistoryPanel` rendered
one or the other for the *same page*, depending on which console it was in.

What genuinely differed was a kicker line above the title, which the Root console
uses to say whose authority the page is under. That is a snippet now, and it is
the only thing that changes the layout: with a kicker the three rows become four,
and the title, the slot and the description each move down one.

Two smaller differences were drift and are gone: the panel's description balanced
its line breaks and the Root one did not, and the panel's mobile action slot took
the full width while the Root one stretched without asking for it. Both now do
what the panel did.
-->

<!--
  A grid rather than a flex row, so the action slot shares the TITLE's row and
  centres on it. Centring against the whole heading block hung the button below the
  title as soon as a description wrapped underneath it.
-->
<header class="page-header" class:has-kicker={kicker !== undefined || eyebrow !== undefined}>
  {#if kicker !== undefined}
    <p class="page-kicker">{@render kicker()}</p>
  {:else if eyebrow !== undefined}
    <p class="page-kicker">{eyebrow}</p>
  {/if}
  <!-- The title's box IS its band, which is what lets the slot beside it centre
       natively. There used to be a measured `translateY(round(0.0382em, 1px))`
       on that slot instead, correcting for the room a line box keeps under the
       baseline that the capitals never use. Trimmed, there is nothing to
       correct. -->
  <h2 class="band-trim" {id}>{title}</h2>
  {#if actions !== undefined}
    <div class="page-header-actions">
      {@render actions()}
    </div>
  {/if}
  <p class="page-header-description band-trim">{description}</p>
</header>

<style>
  .page-header {
    /* A flat token, not clamp(…, 2vw, …): the viewport-relative step resolved to
       27.56px at this width, which is both a size the mock never uses and a fraction
       that every metric under it inherited. */
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
    /* The header's exit is a rhythm decision, not a spacing one: 24px to the
       first surface below it, and the pane's own top edge above. */
    padding: var(--space-2) 0 var(--rhythm-head-surface-wide);
  }

  .page-header.has-kicker {
    grid-template-rows: auto minmax(var(--control-height-compact), auto) auto;
  }

  .page-kicker {
    align-items: center;
    color: var(--brand-action-text);
    display: flex;
    font: 700 var(--font-size-micro) / var(--leading-flat) var(--sans);
    gap: 0.6rem;
    grid-column: 1;
    grid-row: 1;
    letter-spacing: 0.08em;
    /* The approved design's own step. With the title trimmed and its row a control
       tall, the ink-to-cap gap this produces is 29.05px, which is what the mock
       measures. */
    margin: 0 0 var(--space-3);
    text-transform: uppercase;
  }

  h2 {
    font-size: var(--title-size);
    font-weight: 700;
    grid-column: 1;
    grid-row: 1;
    letter-spacing: -0.03em;
    /* 34px, the scale's page tier and exactly what `round(1.2em, 1px)` computed here:
       1.2 of a 28px title is 33.6, and the half pixel put the description - and
       everything under it - on a fractional device row. The em keying existed for a
       22px detail heading that overrode `--title-size`; nothing overrides it now, so
       the size and its leading are one decision again. */
    line-height: var(--leading-page);
    margin: 0;
  }

  /* The meta size, as the approved design draws it: a description explains the
     title, and at body size it competes with the page's own content for weight. */
  .page-header-description {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    grid-column: 1;
    grid-row: 2;
    line-height: var(--leading-meta);
    /* Baseline to cap line, now that both boxes are their bands and the title's row
       is a control tall: 18.57px, the gap the approved design measures. The number
       written here is the whole gap, rather than the number plus whatever leading the
       two fonts happened to carry. */
    margin: var(--space-3) 0 0;
    max-width: 52rem;
    text-wrap: balance;
  }

  .page-header-actions {
    /* Header-slot controls share the toolbar height (34px) so the CTA reads as part
       of the control system, not a taller outlier. */
    --local-control-height: var(--control-height-compact);

    align-items: center;
    display: flex;
    flex: none;
    gap: var(--space-2);
    grid-column: 2;
    grid-row: 1;
    justify-self: end;
  }

  /* With a kicker the title, the slot and the description each move down one row. */
  .page-header.has-kicker h2,
  .page-header.has-kicker .page-header-actions {
    grid-row: 2;
  }

  .page-header.has-kicker .page-header-description {
    grid-row: 3;
  }

  @media (max-width: 36rem) {
    .page-header {
      grid-template-columns: minmax(0, 1fr);
      padding-bottom: var(--rhythm-head-surface-compact);
    }

    .page-header-actions {
      grid-column: 1;
      grid-row: 3;
      justify-self: stretch;
      /* The stacked distance is its own decision: the slot has left the title's
         row, so what separates them is no longer the head's exit. */
      margin-top: var(--rhythm-head-actions-stacked);
      width: 100%;
    }

    .page-header.has-kicker .page-header-actions {
      grid-row: 4;
    }
  }
</style>
