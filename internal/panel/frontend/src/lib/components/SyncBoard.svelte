<script module lang="ts">
  /** What one repository is, seen from the installation that configures it. */
  import type { SyncState } from './StateMark.svelte';

  export interface BoardRepository {
    name: string;
    state: SyncState;
    /** Only read for `change`: the numeral the tile carries. */
    changes?: number;
    /** Only read for `refused`: said in the tile's own words, not a code. */
    reason?: string;
  }

  /** In the order a reader meets them: settled first, then what wants them. */
  const LEGEND: readonly { state: SyncState; word: string }[] = [
    { state: 'settled', word: 'In step' },
    { state: 'change', word: 'Would change' },
    { state: 'refused', word: 'Refused' },
    { state: 'off', word: 'Not watched here' },
  ];
</script>

<script lang="ts">
  /**
   * The fleet at a glance: one raised tile per repository, in a sunken well.
   *
   * A table of repositories answers "what is wrong with THIS one"; this answers
   * "how much of the fleet is out of step", which is the question the page is
   * opened with. Twenty-five tiles are read as a shape long before any row of a
   * table is read as a sentence - so the tile carries state as material (raised,
   * tinted, dashed socket) and the numeral only where changes actually wait.
   *
   * The legend is a key AND a filter: pressing a row dims every tile that is not
   * in that state, which is how a reader finds the four that matter among the
   * twenty-one that do not. It dims rather than removes, so the shape of the
   * fleet never moves under the hand.
   *
   * Colour is never the only channel. Settled carries a tick, refused a cross,
   * change a numeral, and off is a dashed socket with no fill at all.
   */
  import AppTooltip from '#lib/components/AppTooltip.svelte';
  import Icon from '#lib/components/Icon.svelte';

  const {
    repositories,
    label,
    footLine,
    footWhen,
    hrefOf,
    children,
  }: {
    repositories: readonly BoardRepository[];
    /** Names the board for assistive tech - "Repositories in this installation". */
    label: string;
    /** The plan in one sentence, under the board. Omit when there is no plan. */
    footLine?: string;
    /** When it was worked out, and what it is waiting on. */
    footWhen?: string;
    /**
     * Where one repository's own page is.
     *
     * A href rather than a callback, because a tile is an address: the panel
     * navigated it with `window.location.assign`, which throws the whole
     * application away and loads it again - a white flash and every query
     * refetched, to reach a page the client router could have drawn. A real
     * link also copies, opens in a tab and answers the middle button.
     */
    hrefOf?: (repository: BoardRepository) => string;
    /** The foot's action - a link to the plan, usually. */
    children?: import('svelte').Snippet;
  } = $props();

  let filter = $state<SyncState | null>(null);

  const counts = $derived(
    repositories.reduce<Record<SyncState, number>>(
      (tally, repository) => {
        tally[repository.state] += 1;

        return tally;
      },
      { settled: 0, change: 0, refused: 0, off: 0 },
    ),
  );

  const said = (repository: BoardRepository): string => {
    if (repository.state === 'refused') {
      return repository.reason === undefined
        ? `${repository.name} - refused`
        : `${repository.name} - refused: ${repository.reason}`;
    }
    if (repository.state === 'change') {
      const changes = repository.changes ?? 0;

      return `${repository.name} - ${changes} ${changes === 1 ? 'change' : 'changes'} waiting`;
    }

    return `${repository.name} - ${repository.state === 'off' ? 'not watched here' : 'in step'}`;
  };
</script>

<div class="board">
  <div class="board-lay">
    <div class="board-well" role="group" aria-label={label}>
      {#each repositories as repository (repository.name)}
        <AppTooltip text={repository.name} mono>
          {#snippet children(props)}
            <svelte:element
              this={hrefOf === undefined ? 'span' : 'a'}
              {...props}
              role={hrefOf === undefined ? 'img' : undefined}
              href={hrefOf?.(repository)}
              class="tile is-{repository.state}"
              class:is-link={hrefOf !== undefined}
              class:is-dim={filter !== null && filter !== repository.state}
              aria-label={said(repository)}
            >
              {#if repository.state === 'change'}
                <!-- Wrapped, because a bare figure in a flex container is an
                     anonymous box no selector reaches and the trim never lands
                     on it - which put the count 0.39px above the middle of its
                     tile. -->
                <span class="cap-trim">{repository.changes ?? 0}</span>
              {:else if repository.state === 'refused'}
                <Icon name="failure" size={13} />
              {:else if repository.state === 'settled'}
                <Icon name="check" size={11} />
              {/if}
            </svelte:element>
          {/snippet}
        </AppTooltip>
      {/each}
    </div>

    <div class="legend">
      {#each LEGEND as row (row.state)}
        <button
          type="button"
          class="legend-row"
          aria-pressed={filter === row.state}
          onclick={() => {
            filter = filter === row.state ? null : row.state;
          }}
        >
          <span class="legend-swatch is-{row.state}"></span>
          <span class="legend-word band-trim">{row.word}</span>
          <span class="legend-count band-trim">{counts[row.state]}</span>
        </button>
      {/each}
    </div>
  </div>

  {#if footLine !== undefined}
    <div class="board-foot">
      <div class="board-foot-say">
        <!-- Both lines trimmed or neither: one untrimmed line leaves its
             leading and descender inside the block, and the pair then reads
             2.25px above the button it sits beside. -->
        <span class="board-foot-line band-trim">{footLine}</span>
        {#if footWhen !== undefined}
          <span class="board-foot-when band-trim">{footWhen}</span>
        {/if}
      </div>
      {@render children?.()}
    </div>
  {/if}
</div>

<style>
  .board {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    padding: var(--space-4);
  }

  .board-lay {
    display: grid;
    gap: var(--space-5);
    grid-template-columns: 1fr auto;
  }

  /* The well is below the page, so the tiles read as sitting IN something. */
  .board-well {
    align-content: start;
    background: var(--tile-well);
    border-radius: 10px;
    box-shadow: inset 0 1px 2px rgb(0 0 0 / 12%);
    display: grid;
    gap: 7px;
    grid-template-columns: repeat(auto-fill, minmax(2.75rem, 1fr));
    padding: 10px;
  }

  /* The legend row is not a `.btn`, and `app.css` resets buttons by class rather
     than by element - so a bare one keeps the UA's own face, which under
     `color-scheme: dark` is a mid grey. */
  .tile,
  .legend-row {
    appearance: none;
    background: none;
    border: 0;
    color: inherit;
    font: inherit;
    padding: 0;
    text-decoration: none;
  }

  /* Only where it goes somewhere: a board drawn without a destination is a
     picture, and a picture that answers the hand is a promise it cannot keep. */
  .tile.is-link,
  .legend-row {
    cursor: pointer;
  }

  .tile {
    align-items: center;
    aspect-ratio: 1;
    background: var(--tile-face);
    border: 1px solid var(--tile-border);
    border-radius: 9px;
    box-shadow: var(--tile-shadow);
    display: flex;
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-variant-numeric: tabular-nums;
    justify-content: center;
    padding: 0;
    position: relative;
    transition:
      background-image var(--duration-fast) var(--ease-standard),
      transform var(--duration-fast) var(--ease-standard),
      opacity var(--duration-fast) var(--ease-out);
  }

  /* The app's own two states, not a lift. A tile used to answer a hover only by
     rising a pixel, which is nothing at all beside a repository name and is a
     move no other control here makes - every interactive surface in the panel
     answers in colour, over whatever ground it already has, and shrinks toward
     its own centre when pressed. The layer goes on `background-image` because
     `background` is what carries each tile's state colour. */
  .tile.is-link:hover {
    background-image: linear-gradient(
      var(--interactive-hover-layer),
      var(--interactive-hover-layer)
    );
  }

  /* Compact rather than the default scale: a tile is a small square, and the
     larger step reads as the whole board twitching. */
  .tile.is-link:active {
    background-image: linear-gradient(var(--press), var(--press));
    transform: scale(var(--press-scale-compact));
  }

  /* The name is `AppTooltip`'s now, not a `::after` on the tile.
     A pseudo-element cannot leave the box that draws it in any useful way: it
     was centred on its tile and never wrapped, so a repository with a long name
     on the left of the board ran under the sidebar and was cut off there - the
     one place a name is worth reading is the one place it could not be. The
     primitive portals to `.app-shell`, wraps at 17rem, and keeps itself 8px
     inside the viewport, which is the panel's rule for every other overlay. */

  .tile.is-settled {
    color: color-mix(in srgb, var(--text-muted) 42%, transparent);
  }

  .tile.is-change {
    background: color-mix(in srgb, var(--cell-pending) 14%, var(--tile-face));
    border-color: color-mix(in srgb, var(--cell-pending) 55%, transparent);
    color: var(--cell-pending);
    font-weight: 600;
  }

  .tile.is-refused {
    background: color-mix(in srgb, var(--cell-refused) 14%, var(--tile-face));
    border-color: color-mix(in srgb, var(--cell-refused) 55%, transparent);
    color: var(--cell-refused);
  }

  /* Not a tile with a state - an empty socket where a tile would be. */
  .tile.is-off {
    background: none;
    border: 1.5px dashed color-mix(in srgb, var(--cell-off) 90%, transparent);
    box-shadow: none;
    color: var(--text-muted);
  }

  .tile.is-dim {
    opacity: 0.28;
  }

  .legend {
    align-content: start;
    display: grid;
    gap: var(--space-1);
    min-width: 13rem;
  }

  .legend-row {
    align-items: center;
    border-radius: var(--r-ctl);
    display: grid;
    gap: var(--space-2);
    grid-template-columns: auto 1fr auto;
    padding: 0.4rem 0.55rem;
    text-align: start;
  }

  .legend-row:hover {
    background-image: linear-gradient(
      var(--interactive-hover-layer),
      var(--interactive-hover-layer)
    );
  }

  .legend-row:active {
    background-image: linear-gradient(var(--press), var(--press));
    transform: scale(var(--press-scale-surface));
  }

  .legend-row[aria-pressed='true'] {
    background: var(--interactive-pressed-bg);
  }

  .legend-swatch {
    block-size: 0.85rem;
    border: 1px solid var(--tile-border);
    border-radius: 4px;
    inline-size: 0.85rem;
  }

  .legend-swatch.is-settled {
    background: var(--tile-face);
    box-shadow: var(--tile-shadow);
  }

  .legend-swatch.is-change {
    background: color-mix(in srgb, var(--cell-pending) 14%, var(--tile-face));
    border-color: color-mix(in srgb, var(--cell-pending) 55%, transparent);
  }

  .legend-swatch.is-refused {
    background: color-mix(in srgb, var(--cell-refused) 14%, var(--tile-face));
    border-color: color-mix(in srgb, var(--cell-refused) 55%, transparent);
  }

  .legend-swatch.is-off {
    background: none;
    border: 1.5px dashed var(--cell-off);
  }

  .legend-word {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
  }

  .legend-count {
    color: var(--text-primary);
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-variant-numeric: tabular-nums;
  }

  .board-foot {
    align-items: center;
    border-top: 1px solid var(--border-subtle);
    display: flex;
    gap: var(--space-3);
    margin-top: var(--space-4);
    padding-top: var(--space-3);
  }

  /* The gap the panel puts between a line and the line that explains it - the
     same one `PageHeader` puts under a title.

     The mock declares 4px here and the app took the number rather than the
     distance. Both of these lines are trimmed to their cap band, so 4px IS the
     gap between the letters; in the mock it is a gap between two line boxes,
     each carrying its own half-leading, a descender under the first and the
     ascender above the second's caps - about nine pixels of air the trim takes
     away. A number copied across that difference draws a tighter pair than the
     one it was copied from. */
  .board-foot-say {
    display: grid;
    flex: 1;
    gap: var(--space-3);
  }

  .board-foot-line {
    font-size: var(--font-size-meta);
    font-weight: 600;
  }

  .board-foot-when {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
  }

  /* The legend goes under the well rather than beside it: a 13rem column
     beside a board leaves the board too narrow to read as a fleet. */
  @media (max-width: 52rem) {
    .board-lay {
      grid-template-columns: 1fr;
    }

    .legend {
      grid-template-columns: 1fr 1fr;
    }

    .board-foot {
      align-items: start;
      flex-direction: column;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .tile {
      transition: none;
    }

    /* The colour still changes - what a reader needs is the state, and only the
       movement is what motion sensitivity is about. */
    .tile.is-link:active {
      transform: none;
    }
  }
</style>
