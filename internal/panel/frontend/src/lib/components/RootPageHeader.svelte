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
    /* Drops the slot onto the title's cap-to-baseline centre; a control centred
       on the line box lands high, since the line box keeps room under the
       baseline the caps never use. See PanelHeader for where 0.0382em comes
       from - it is measured off TextMetrics, not derived from nominal ascent
       and descent, which overshot by more than 2px. Keyed to the TITLE's size -
       an em on the slot would resolve against its own font. */
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

  .root-kicker {
    align-items: center;
    color: var(--brand-action-text);
    display: flex;
    font: 700 var(--font-size-micro) / 1 var(--sans);
    gap: 0.6rem;
    grid-column: 1;
    grid-row: 1;
    letter-spacing: 0.08em;
    margin: 0 0 var(--space-3);
    text-transform: uppercase;
  }

  h2 {
    font-size: var(--title-size);
    font-weight: 700;
    grid-column: 1;
    grid-row: 2;
    letter-spacing: -0.035em;
    /* Rounded to whole pixels: 1.2 of a 28px title is 33.6, and a fractional
       line box puts the subtitle - and everything under it - on a half device
       pixel, which reads as a soft edge at 2x. Keyed to 1.2em, the element's
       OWN size: keying it to --title-size gave a 22px detail heading the 28px
       title's 34px line box. */
    line-height: round(1.2em, 1px);
    margin: 0;
  }

  .root-subtitle {
    color: var(--text-secondary);
    font-size: var(--font-size-body);
    grid-column: 1;
    grid-row: 3;
    /* Whole pixels, same rule as the title: 1.5 of a 15px body is 22.5, and
       the half pixel pushed the toolbar row under this line off the device
       grid. */
    line-height: round(1.5em, 1px);
    margin: var(--space-2) 0 0;
    max-width: 52rem;
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
    /* Down onto the letters' optical centre. A transform, not a margin: a
       margin would grow the title's row whenever the control is taller than the
       title line (a 34px segmented control does, a 24px pill does not), which
       would drag the title itself and every gap with it. */
    transform: translateY(var(--title-ink-offset));
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
