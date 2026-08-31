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
    /** One line saying what the page is for. Not every page owes one. */
    description?: string;
    /** Where this page is: the workspace it belongs to, and the group inside it. */
    eyebrow?: string;
    /** Above the title - on the Root console, whose authority this page is under. */
    kicker?: Snippet;
    /** Live status and the controls that act on the page. Never identity. */
    actions?: Snippet;
  } = $props();
</script>

<!--
@component
The top of a page: where it is, what it is called, what it is for, and the controls
that act on it.

The copy is a REAL vertical stack - eyebrow, title, description - and the action group
is the second column, centred against the whole stack rather than against whichever
line it happens to share a row with. The distance between the title and its sentence is
one line of the SENTENCE's leading, not a container gap: a gap cannot read `1cap`,
because `1cap` resolves against the element that writes it and the container's font is
not the sentence's.

The title is the page's `<h1>`. There is one page title per page, and it is this.
-->

<header class="page-head">
  <div class="page-head-say">
    {#if kicker !== undefined}
      <p class="page-eyebrow">{@render kicker()}</p>
    {:else if eyebrow !== undefined}
      <p class="page-eyebrow">{eyebrow}</p>
    {/if}
    <h1 class="page-title" {id}>{title}</h1>
    {#if description !== undefined}
      <p class="page-sub">{description}</p>
    {/if}
  </div>
  {#if actions !== undefined}
    <div class="page-actions">
      {@render actions()}
    </div>
  {/if}
</header>

<style>
  .page-head {
    align-items: center;
    column-gap: var(--space-3);
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    /* The header's exit is a rhythm decision: 24px to the first surface below it,
       and the pane's own top edge above. */
    margin-block-end: var(--rhythm-head-surface-wide);
    row-gap: var(--rhythm-head-actions-stacked);
  }

  .page-head-say {
    display: grid;
    gap: 0;
    min-inline-size: 0;
  }

  .page-head-say > :global(* + *) {
    margin-block-start: calc(var(--leading-meta) - 1cap);
  }

  .page-head-say > .page-title {
    margin-block-start: var(--rhythm-head-copy);
  }

  .page-eyebrow {
    align-items: center;
    color: var(--text-muted);
    display: flex;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: var(--space-2);
    letter-spacing: 0.07em;
    /* THE RAMP'S OWN LEADING FOR ITS SIZE, not the flat control line: the micro size
       steps up on a phone, and a kicker that wraps there would be leading itself on a
       line the scale never owed it. Nothing moves at any width where the eyebrow is
       one line, because a single line's box is trimmed to its own cap. */
    line-height: var(--leading-micro);
    margin: 0;
    min-block-size: 9px;
    text-transform: uppercase;
  }

  /* The console's own pages say so in its own ink. */
  :global(.app-shell.root-mode) .page-eyebrow {
    color: var(--brand-action-text);
  }

  .page-title {
    font-size: var(--font-size-page-title);
    font-weight: 650;
    letter-spacing: -0.015em;
    line-height: var(--leading-page);
    margin: 0;
    min-block-size: 20px;
    text-box: trim-both cap alphabetic;
    text-wrap: balance;
  }

  /* Focused on arrival, never ringed for a pointer - see the shell's route focus. */
  .page-title:focus {
    outline: none;
  }

  .page-sub {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    line-height: var(--leading-meta);
    margin: 0;
    /* No measure cap: a line uses the container it is in. A ch-width on prose left
       a broken column wherever the pane was wide. */
    text-wrap: pretty;
  }

  .page-actions {
    /* Header-slot controls share the toolbar height (34px) so the CTA reads as part
       of the control system, not a taller outlier. */
    --local-control-height: var(--control-height-compact);

    align-items: center;
    align-self: center;
    display: flex;
    flex: none;
    flex-wrap: wrap;
    gap: var(--space-2);
    justify-content: flex-end;
  }

  @media (max-width: 36rem) {
    .page-head {
      grid-template-columns: minmax(0, 1fr);
      margin-block-end: var(--rhythm-head-surface-compact);
    }

    .page-actions {
      justify-self: stretch;
      width: 100%;
    }
  }
</style>
