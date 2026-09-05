<script lang="ts">
  import type { Snippet } from 'svelte';
  import PanePath, { type PanePathSegment } from './PanePath.svelte';

  import { panelSessionOrNull } from '../session.svelte';

  const {
    id,
    title,
    mono = false,
    description,
    section,
    ancestors,
    eyebrow,
    kicker,
    actions,
    status,
    statusUnsaved = false,
  }: {
    /** The id the page's `aria-labelledby` points at. */
    id: string;
    title: string;
    /**
     * The title is a name the product did not choose - a ruleset, a path - so it
     * keeps its mono voice at the header's own scale.
     */
    mono?: boolean;
    /** One line saying what the page is for. Not every page owes one. */
    description?: string;
    /** The group this page sits in - Sync, Access, Activity. Not every page is in one. */
    section?: string;
    /** The way up replaces the scope eyebrow on a detail page. */
    ancestors?: readonly PanePathSegment[];
    /** The whole eyebrow, for a page whose scope is not the open workspace. */
    eyebrow?: string;
    /** Above the title - on the Root console, whose authority this page is under. */
    kicker?: Snippet;
    /** Live status and the controls that act on the page. Never identity. */
    actions?: Snippet;
    /**
     * A band under the copy, spanning the whole head: the page's own state and
     * the one switch that changes it. Its own row rather than a third thing in
     * the action group, because a consequence is prose and reads across the
     * width - the sync kind pages state what pausing costs before the switch.
     */
    status?: Snippet;
    /** The band holds a control whose change is staged and not yet saved. */
    statusUnsaved?: boolean;
  } = $props();

  /**
   * WHERE this page is, said by the shell rather than by the page.
   *
   * Every workspace page's eyebrow opens with the workspace it belongs to, and every
   * console page's says the console. A page knows the group it is in and nothing
   * about which workspace is open, so it names the group and this composes the rest -
   * which is also what stops thirty pages spelling one word thirty ways.
   *
   * Nullable, because the same page rendered by a story or a component test stands
   * outside the shell: there the eyebrow is the group alone rather than nothing at all.
   */
  const session = panelSessionOrNull();
  const scope = $derived(
    session === null
      ? ''
      : session.isRootMode
        ? 'Operations console'
        : (session.selectedTarget?.account.display_name ??
          session.selectedTarget?.account.login ??
          ''),
  );
  const eyebrowText = $derived(
    eyebrow ?? [scope, section].filter((part) => part !== undefined && part !== '').join(' · '),
  );
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
    {#if ancestors !== undefined}
      <PanePath segments={ancestors} />
    {:else if kicker !== undefined}
      <p class="page-eyebrow">{@render kicker()}</p>
    {:else if eyebrowText !== ''}
      <p class="page-eyebrow">{eyebrowText}</p>
    {/if}
    <h1 class="page-title" class:mono-title={mono} {id}>{title}</h1>
    {#if description !== undefined}
      <p class="page-sub">{description}</p>
    {/if}
  </div>
  {#if actions !== undefined}
    <div class="page-actions">
      {@render actions()}
    </div>
  {/if}
  {#if status !== undefined}
    <div
      class="page-status"
      class:is-unsaved={statusUnsaved}
      data-unsaved={statusUnsaved || undefined}
    >
      {@render status()}
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
       and the pane's own top edge above. Whatever the container already gives is
       subtracted - `--head-exit-gap`, declared by a container that spaces its
       children and absent on one that does not, so this is the whole distance in
       a plate and the difference in a stack. Written as one term rather than as
       two numbers, because two numbers for one distance is how this drifts. */
    margin-block-end: calc(var(--rhythm-head-surface-wide) - var(--head-exit-gap, 0px));
    row-gap: var(--rhythm-head-actions-stacked);
  }

  /* A head that exits onto a TOOLBAR closes tighter than one exiting onto a surface.
     `app.css` says this too, and says it at the same specificity as the rule above -
     so on source order the component won and every page with a filter bar under its
     head stood 4px too far off it. Said here, where the losing rule is.

     All three toolbars, not just the filter bar: the sync pages draw `.matrix-tools`
     and `.plan-tools`, which are the same thing under different names, and named
     alone the filter bar left those two heads exiting as if onto a surface. */
  .page-head:has(+ :global(.filter-bar)),
  .page-head:has(+ :global(.matrix-tools)),
  .page-head:has(+ :global(.plan-tools)) {
    margin-block-end: calc(var(--rhythm-head-toolbar) - var(--head-exit-gap, 0px));
  }

  /* A head carrying a status band is three areas rather than two columns, and
     the band spans both - keyed on the band being there rather than on a
     modifier, because its presence is the whole condition. */
  .page-head:has(> .page-status) {
    align-items: center;
    grid-template-areas:
      'copy actions'
      'status status';
    row-gap: var(--space-3);
  }

  .page-head:has(> .page-status):not(:has(> .page-actions)) {
    grid-template-areas:
      'copy copy'
      'status status';
  }

  .page-head:has(> .page-status) > .page-head-say {
    grid-area: copy;
  }

  .page-head:has(> .page-status) > .page-actions {
    grid-area: actions;
  }

  .page-status {
    align-items: center;
    display: grid;
    gap: var(--space-4);
    grid-area: status;
    grid-template-columns: minmax(0, 1fr) auto;
    min-block-size: 34px;
  }

  /* The switch's tap box must not set the band's height - the hit area survives
     on the input itself, which is what the band's own 34px already covers. */
  .page-status :global(.switch) {
    min-block-size: 32px;
  }

  /* The panel's one unsaved marker, on the band that holds the staged switch
     rather than on the whole head - the title is not what changed. */
  .page-status.is-unsaved {
    background: color-mix(in srgb, var(--brand-action-tint) 45%, transparent);
    box-shadow: inset 2px 0 var(--brand-action);
    margin-inline: calc(var(--space-2) * -1);
    padding: var(--space-2);
  }

  .page-head-say {
    display: grid;
    gap: 0;
    min-inline-size: 0;
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

  /* A name the product did not choose keeps its mono voice, at the header's own
     scale rather than the row scale the class carries inside a list. */
  .page-title.mono-title {
    font-family: var(--mono);
    font-size: 1.625rem;
    font-weight: 600;
    letter-spacing: -0.01em;
    min-block-size: 19px;
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

  /* AFTER the three resets above, not before them. The title and its sentence are a
     copy pair like any other, so the distance is one line of the SENTENCE'S leading -
     a gap cannot read `1cap`, because `1cap` resolves against whatever writes it and
     the container's font is not the sentence's. Written first, these tied with
     `.page-sub`'s own `margin: 0` on specificity and lost on source order, so every
     page in the panel closed its head 10px tighter than the design's. */
  .page-head-say > :global(* + *) {
    margin-block-start: calc(var(--leading-meta) - 1cap);
  }

  .page-head-say > .page-title {
    margin-block-start: var(--rhythm-head-copy);
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

  /* A PHONE IS NARROWER THAN A TITLE BESIDE A CONTROL. The head becomes one column and
     the actions stack under the copy, packed left - kept at `flex-end` they read as a
     stray button floating at the far edge with nothing to align to. */
  @media (max-width: 47.9375rem) {
    .page-head {
      grid-template-columns: minmax(0, 1fr);
      margin-block-end: calc(var(--rhythm-head-surface-compact) - var(--head-exit-gap, 0px));
    }

    .page-head:has(+ :global(.filter-bar)),
    .page-head:has(+ :global(.matrix-tools)),
    .page-head:has(+ :global(.plan-tools)) {
      margin-block-end: calc(var(--rhythm-head-toolbar) - var(--head-exit-gap, 0px));
    }

    /* One column: the complete action group stacks AFTER the state it acts on. */
    .page-head:has(> .page-status) {
      grid-template-areas: 'copy' 'status' 'actions';
    }

    .page-head:has(> .page-status):not(:has(> .page-actions)) {
      grid-template-areas: 'copy' 'status';
    }

    .page-status {
      align-items: start;
    }

    .page-actions {
      justify-content: flex-start;
    }
  }
</style>
