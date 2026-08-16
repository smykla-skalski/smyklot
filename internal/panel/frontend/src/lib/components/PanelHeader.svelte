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
    /* A control centred on the title's line box lands slightly high against the
       letters, because the line box reserves room below the baseline that the
       caps never use. The nudge drops it onto the cap-to-baseline centre - the
       band the eye reads as "the word".

       0.0382em is measured, not derived from nominal font metrics: TextMetrics
       puts this face's cap height at 0.75em and its baseline 0.6382em under the
       line box top at line-height 1.2. Deriving it from the 0.347/0.108 ascent
       split overshot by more than 2px, which is visible. Keyed to the TITLE's
       size - an em on the action would resolve against its own font.

       Descenders are deliberately out of it: centring on the glyph ink would
       move the control whenever a title happened to contain a "y". */
    /* A flat token, not clamp(…, 2vw, …): the viewport-relative step resolved
       to 27.56px at this width, which is both a size the mock never uses and a
       fraction that every metric under it inherited. */
    --title-size: var(--font-size-page-title);
    --title-ink-offset: round(calc(var(--title-size) * 0.0382), 1px);

    align-content: center;
    align-items: center;
    column-gap: var(--space-6);
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    min-height: 5.25rem;
    padding: var(--space-2) 0 var(--space-6);
  }

  h2 {
    font-size: var(--title-size);
    font-weight: 700;
    grid-column: 1;
    grid-row: 1;
    letter-spacing: -0.035em;
    /* Rounded to whole pixels: 1.2 of a 28px title is 33.6, and a fractional
       line box puts the subtitle - and everything under it - on a half device
       pixel, which reads as a soft edge at 2x. Keyed to 1.2em, the element's
       OWN size: keying it to --title-size gave a 22px detail heading the 28px
       title's 34px line box. */
    line-height: round(1.2em, 1px);
    margin: 0;
  }

  p {
    color: var(--text-secondary);
    font-size: var(--font-size-body);
    grid-column: 1;
    grid-row: 2;
    /* Whole pixels, same rule as the title: 1.5 of a 15px body is 22.5, and
       the half pixel pushed the toolbar row under this line off the device
       grid. */
    line-height: round(1.5em, 1px);
    margin: var(--space-2) 0 0;
    max-width: 52rem;
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
    /* Down onto the letters' optical centre. A transform, not a margin: a
       margin would grow the title's row whenever the control is taller than the
       title line (a 34px segmented control does, a 24px pill does not), which
       would drag the title itself and every gap with it. */
    transform: translateY(var(--title-ink-offset));
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
