<script lang="ts">
  /**
   * One kind of thing this installation keeps in step, on the overview.
   *
   * The strip under the summary is the board again - the same repositories in
   * the same order, one slot each - so a repository that is out of step lines
   * up vertically across the board and all four cards, and "which kind is `af`
   * failing" is answered by looking down a column rather than by opening
   * anything. That is the whole reason the order is the board's and not this
   * card's own.
   *
   * The switch gates planning, and only planning: turning a kind off leaves
   * every repository exactly as it is, which is why it can be instant. The card
   * stays where it is when it goes off and says so in muted ink - a kind that
   * vanished when switched off would be a kind nobody could switch back on.
   *
   * The title's link covers the whole card, so a press anywhere opens the kind;
   * the switch and the chevron stand above it and stay their own controls.
   */
  import Icon from '#lib/components/Icon.svelte';
  import Switch from '#lib/components/Switch.svelte';
  import type { SyncState } from '#lib/components/StateMark.svelte';

  const {
    name,
    href,
    summary,
    states,
    when,
    enabled = true,
    onToggle,
  }: {
    name: string;
    href: string;
    /** What is configured, in one line - "12 labels · removal off". */
    summary: string;
    /**
     * One per repository, in the board's order. Absent where the per-repository
     * state has not been read yet - the card says what it knows and draws no
     * strip, rather than drawing twenty-five settled slots it cannot vouch for.
     */
    states?: readonly SyncState[];
    /** Who last changed it and when. */
    when?: string;
    enabled?: boolean;
    onToggle?: (next: boolean) => void;
  } = $props();
</script>

<div class="kind-card" class:is-off={!enabled}>
  <div class="kind-card-head">
    <a class="kind-name band-trim" {href}>{name}</a>
    <!-- Disabled where nothing is listening, so the control cannot say it did
         something it did not. `onToggle?.(next)` on a live switch is a thumb
         that moves - a checkbox flips itself - over a save that never happens,
         which is the most confident way to be wrong on this page. -->
    <Switch
      checked={enabled}
      ariaLabel="{name} sync"
      disabled={onToggle === undefined}
      onChange={(next) => onToggle?.(next)}
    />
  </div>

  <span class="kind-sum">{summary}</span>

  {#if states !== undefined}
    <div class="kind-strip" aria-hidden="true">
      {#each states as state, at (at)}
        <span class="is-{state}"></span>
      {/each}
    </div>
  {/if}

  <span class="kind-foot">
    <!-- Trimmed so the flex row centres the words rather than the box that
         carries their leading - untrimmed they sat 0.34px above the chevron. -->
    <span class="kind-when band-trim">{when ?? ''}</span>
    <a class="kind-open" {href} aria-label="Open {name}">
      <Icon name="chevron-right" size={12} />
    </a>
  </span>
</div>

<style>
  /* Four rows, fixed: head, summary that takes the slack, strip, foot. The
     summary is the only one that can be two lines, so it is the only one
     allowed to grow - which is what puts every card's strip on one line. */
  .kind-card {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    display: grid;
    gap: var(--space-3);
    grid-template-rows: auto 1fr auto auto;
    padding: var(--space-4);
    position: relative;
    transition: background var(--duration-fast) var(--ease-standard);
  }

  /* The same layer every other surface in the panel hovers with, rather than a
     swap to another named ground: `--surface-raised` sits a hair off
     `--surface-base` on three of the four palettes, which is a hover nobody
     can see. */
  .kind-card:hover {
    background-image: linear-gradient(
      var(--interactive-hover-layer),
      var(--interactive-hover-layer)
    );
  }

  /* A card is a wide surface, so it takes the smallest press step - the same
     one a table row takes. */
  .kind-card:active {
    background-image: linear-gradient(var(--press), var(--press));
    transform: scale(var(--press-scale-surface));
  }

  /* It leans the way it would take the reader, and darkens as it goes - the
     same two pixels the attention rows lean. */
  .kind-card:hover .kind-open {
    color: var(--text-primary);
    translate: 2px 0;
  }

  .kind-card-head {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    justify-content: space-between;
  }

  .kind-name {
    color: var(--text-primary);
    font-size: var(--font-size-title);
    font-weight: 600;
    text-decoration: none;
  }

  /* The card is the target; the controls on it are not. */
  .kind-name::after {
    content: '';
    inset: 0;
    position: absolute;
  }

  .kind-card :global(.switch),
  .kind-open {
    position: relative;
    z-index: 1;
  }

  /*
   * Two lines are reserved whether or not two are used.
   *
   * The summary is the only row that can wrap, and the strips have to line up
   * across all four cards - so the slack from a one-line summary had nowhere to
   * go but under that summary: 31.5px there against 12px on the card beside it,
   * which reads as two correct cards and two broken ones. Reserving the second
   * line takes the slack out of the row entirely, and the strips still line up
   * because every card's summary is now the same height.
   *
   * `lh` rather than a measured number, so it follows the type rather than
   * being re-tuned whenever the meta size moves.
   */
  .kind-sum {
    align-self: start;
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    min-block-size: 2lh;
  }

  /* The board's slots, at a quarter of the size: material, not decoration. */
  .kind-strip {
    display: flex;
    gap: 2.5px;
  }

  .kind-strip span {
    background: color-mix(in srgb, var(--text-primary) 6%, var(--surface-base));
    block-size: 9px;
    border: 0.5px solid var(--border-control);
    border-radius: 2.5px;
    flex: 1;
  }

  .kind-strip .is-change {
    background: color-mix(in srgb, var(--cell-pending) 45%, var(--surface-base));
    border-color: transparent;
  }

  .kind-strip .is-refused {
    background: color-mix(in srgb, var(--cell-refused) 50%, var(--surface-base));
    border-color: transparent;
  }

  .kind-strip .is-off {
    background: none;
    border: 1px dashed color-mix(in srgb, var(--cell-off) 90%, transparent);
  }

  /* The foot keeps a full line box even though its contents are shorter than
     one: the attribution is micro text and the chevron is 12px, and without it
     four cards whose summaries wrap differently end at four different heights. */
  .kind-foot {
    align-items: center;
    align-self: end;
    display: flex;
    gap: var(--space-2);
    justify-content: space-between;
    min-block-size: 1.5em;
  }

  .kind-when {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
  }

  .kind-open {
    color: var(--text-muted);
    display: inline-flex;
    transition:
      color var(--duration-fast) var(--ease-standard),
      translate var(--duration-fast) var(--ease-standard);
  }

  /* Off keeps its place and says so. */
  .kind-card.is-off .kind-name,
  .kind-card.is-off .kind-sum {
    color: var(--text-muted);
  }

  /* The colour still changes; only the lean goes. */
  @media (prefers-reduced-motion: reduce) {
    .kind-card,
    .kind-open {
      transition: none;
    }

    .kind-card:hover .kind-open {
      translate: none;
    }
  }
</style>
