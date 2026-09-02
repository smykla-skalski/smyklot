<script module lang="ts">
  import { SYNC_KINDS, type SyncKind } from '../types';

  export type TileState = 'settled' | 'change' | 'refused' | 'off';

  /** Where one repository stands, read off its cells the way the board colours it. */
  export function tileState(
    cells: Record<SyncKind, { state: string; changes?: number }>,
  ): TileState {
    const all = SYNC_KINDS.map((kind) => cells[kind]);
    if (all.some((cell) => cell.state === 'refused')) return 'refused';
    if (all.some((cell) => (cell.changes ?? 0) > 0)) return 'change';
    if (all.every((cell) => cell.state === 'off')) return 'off';
    return 'settled';
  }
</script>

<script lang="ts">
  import { formatRelative, formatUntil } from '../format';
  import type { SyncConfig, SyncPlan, SyncRepositoryStatus, SyncStatus } from '../types';
  import { SYNC_SECTION_LABELS, type SyncSection } from '../routes';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import Switch from './Switch.svelte';
  import { SETTINGS_FIELD_KEYS, SETTINGS_FIELD_TOTAL } from './SyncSettingsPage.svelte';

  const {
    status,
    plan,
    configs,
    nowMs,
    repositories = null,
    sectionHref,
    onOpenSection,
    onToggleKind,
    dirtyControls = [],
    readOnly = false,
  }: {
    status: SyncStatus;
    plan: SyncPlan | null;
    configs: Partial<Record<SyncKind, SyncConfig>>;
    /** The clock, passed in so a story renders the same minute every time. */
    nowMs: number;
    /**
     * How many repositories the workspace holds, which is not how many sync.
     * The freshness line says both - "checked X · N of M repositories sync" -
     * and reach is the half a count of syncing repositories cannot supply.
     */
    repositories?: number | null;
    sectionHref: (section: SyncSection) => string;
    onOpenSection: (section: SyncSection) => void;
    onToggleKind: (kind: SyncKind, enabled: boolean) => void;
    dirtyControls?: readonly string[];
    readOnly?: boolean;
  } = $props();

  const rows = $derived(status.repositories);
  const states = $derived(new Map(rows.map((row) => [row.repository, tileState(row.cells)])));

  const changesOf = (row: SyncRepositoryStatus): number =>
    SYNC_KINDS.reduce((total, kind) => total + (row.cells[kind].changes ?? 0), 0);

  const outOfStep = $derived(
    rows.filter((row) => {
      const state = states.get(row.repository);
      return state === 'change' || state === 'refused';
    }).length,
  );
  const legendCounts = $derived({
    settled: rows.filter((row) => states.get(row.repository) === 'settled').length,
    change: rows.filter((row) => states.get(row.repository) === 'change').length,
    refused: rows.filter((row) => states.get(row.repository) === 'refused').length,
    off: rows.filter((row) => states.get(row.repository) === 'off').length,
  });

  /* The legend is key AND filter: press a row and the board dims the rest. */
  let filter = $state<TileState | null>(null);

  /* With no rows there are two different silences, and they want different words:
     the workspace syncs nothing at all, or it syncs and every repository has opted
     out. The switches below are the way out of the first and not of the second. */
  const anyKindOn = $derived(SYNC_KINDS.some((kind) => configs[kind]?.enabled === true));

  /* How far sync reaches, said beside the freshness the way the drawing has it.
     Silent where the total is unknown rather than guessing it from the rows,
     which would make "2 of 2 sync" true of a workspace holding twenty. */
  const reachWord = $derived.by(() => {
    /* Nothing to report a share of: "0 of 0 repositories sync" is the freshness
       line carrying a fact with no content in it. */
    if (repositories === null || repositories === 0) return '';
    const others = repositories - rows.length;
    const said = `${rows.length} of ${repositories} ${repositories === 1 ? 'repository' : 'repositories'} sync`;

    return others > 0 ? `${said}; the other ${others} answer commands only` : said;
  });

  /* Carried WITH its separator rather than behind an `{#if}`. Svelte drops the
     whitespace at a block's edges, so a leading " · " inside one arrives welded
     to the word before it: "Checked 5 minutes ago· 28 of 28 repositories sync". */
  const reachSuffix = $derived(reachWord === '' ? '' : ` · ${reachWord}`);

  const totalChanges = $derived(rows.reduce((total, row) => total + changesOf(row), 0));
  const changedRepos = $derived(rows.filter((row) => changesOf(row) > 0).length);
  const removals = $derived(rows.reduce((total, row) => total + (row.removals ?? 0), 0));

  const attention = $derived(
    rows.filter((row) => {
      const state = states.get(row.repository);
      return state === 'change' || state === 'refused';
    }),
  );

  function attnWhy(row: SyncRepositoryStatus): string {
    const kinds = SYNC_KINDS.filter((kind) => (row.cells[kind].changes ?? 0) > 0);
    const words = kinds.join(' · ');
    if ((row.removals ?? 0) > 0) {
      return `${words} - ${row.removals} removal${row.removals === 1 ? '' : 's'} among them`;
    }
    return words;
  }

  const KIND_LABEL = SYNC_SECTION_LABELS;
  const KIND_SECTION: Record<SyncKind, SyncSection> = {
    labels: 'labels',
    settings: 'settings',
    rulesets: 'rulesets',
    files: 'files',
  };

  function kindSum(kind: SyncKind): string {
    const config = configs[kind];
    if (config === undefined) return '';
    if (kind === 'labels') {
      const count = config.labels.length;
      return `${count} label${count === 1 ? '' : 's'} · removal ${config.allow_removal ? 'on' : 'off'}`;
    }
    if (kind === 'settings') {
      /* The settings document is flat - GitHub's own keys at the top level -
         so managed is the catalogue keys the document sets. */
      const managed = SETTINGS_FIELD_KEYS.filter((key) => key in config.document).length;
      return `${managed} of ${SETTINGS_FIELD_TOTAL} managed, the rest follow each repository`;
    }
    if (kind === 'rulesets') {
      const rulesets = listOf(config, 'rulesets');
      const evaluating = rulesets.filter(
        (entry) => (entry as { enforcement?: string }).enforcement === 'evaluate',
      ).length;
      const base = `${rulesets.length} ruleset${rulesets.length === 1 ? '' : 's'}`;
      return evaluating > 0 ? `${base} · ${evaluating} evaluating` : base;
    }
    const files = listOf(config, 'files');
    const retired = listOf(config, 'retired');
    const parts = [`${files.length} template${files.length === 1 ? '' : 's'}`];
    if (retired.length > 0) {
      parts.push(`${retired.length} retired path${retired.length === 1 ? '' : 's'}`);
    }
    parts.push('changes arrive as pull requests');
    return parts.join(' · ');
  }

  function listOf(config: SyncConfig, key: string): unknown[] {
    const value = config.document[key];
    return Array.isArray(value) ? value : [];
  }

  function kindWhen(kind: SyncKind): string {
    const config = configs[kind];
    if (config === undefined || config.updated_by === '') return '';
    return `${config.updated_by}, ${formatRelative(config.updated_at, nowMs)}`;
  }

  function open(event: MouseEvent, section: SyncSection): void {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
      return;
    }
    event.preventDefault();
    onOpenSection(section);
  }

  function kindDirty(kind: SyncKind): boolean {
    return dirtyControls.some((control) => control.startsWith(`sync.${kind}.`));
  }
</script>

<!--
@component
What the organisation expects of its repositories, and how far they are from it. The
overview is the register the four sync pages are read in - each kind says whether it is
enabled, what it would change, and nothing more; the detail is on the page for that
kind.

Its clock is passed in so a story renders the same minute every time. A page that says
"4 minutes ago" from the wall clock cannot be photographed.

Enabling a kind here only makes it eligible for planning. Nothing is written until a
plan is applied, which is why these are switches and not a form.
-->

<section class="view-frame" aria-labelledby="sync-overview-heading">
  <PageHeader id="sync-overview-heading" section="Sync" title="Sync status" />

  <div class="board">
    <!-- The verdict HEADS the board rather than floating above it: a free-standing
         statement over a card is a second page title, and the thing it is a verdict
         about is the board underneath. Its freshness sits directly under it, not
         parked at the card's far edge. -->
    <div class="card-head verdict-head">
      <h2 class="card-title">
        {#if rows.length === 0}{anyKindOn
            ? 'No repository syncs yet'
            : 'Sync is off here'}{:else if outOfStep > 0}<span class="is-drift"
            >{outOfStep} of {rows.length}</span
          >
          syncing repositories are out of step{:else if legendCounts.off === rows.length}All
          {rows.length} are switched off here{:else if legendCounts.off > 0}{rows.length -
            legendCounts.off} active in step ·
          {legendCounts.off} switched off{:else}All {rows.length} are in step{/if}
      </h2>
      <span class="card-note"
        >Checked <strong>{formatRelative(status.checked_at, nowMs)}</strong>{reachSuffix}</span
      >
    </div>
    {#if rows.length === 0}
      <!-- A board of nothing is not a board. Drawn anyway it was an empty grey
           trough over a legend of four zeros, which reads as a fleet that failed
           to load rather than as a workspace with nothing to check - so the card
           says what it has instead, the way every other page with nothing does. -->
      <!-- The head says WHICH state this is; the panel says what follows from it.
           They said the same fact twice, in two voices, one above the other. -->
      <div class="state-panel">
        <span>
          {#if anyKindOn}
            <strong>Every repository has turned this off for itself.</strong> The workspace still syncs,
            so switching one back on in a repository puts it on the board at the next sweep
          {:else}
            <strong>Nothing is being kept in step.</strong> Open a kind and switch it on - the next sweep
            compares every repository against it, and nothing reaches GitHub until a plan is applied
          {/if}
        </span>
        {#if !anyKindOn}
          <!-- The one next step, named rather than pointed at. The kind cards used
               to carry the switches and this said "below"; with the cards gone that
               word pointed at empty canvas, which is the dead end a state panel is
               not allowed to be. Each kind's own page carries the same switch. -->
          <a class="btn" href={sectionHref('labels')} onclick={(event) => open(event, 'labels')}>
            <span class="button-label">Open Labels</span>
          </a>
        {/if}
      </div>
    {:else}
      <div class="board-lay">
        <!-- The fleet: one raised tile per repository, a numeral where changes
           wait, dashed sockets where sync is off. -->
        <div class="board-well">
          {#each rows as row (row.repository)}
            {@const state = states.get(row.repository) ?? 'settled'}
            <button
              type="button"
              class={`tile is-${state}`}
              class:is-dim={filter !== null && state !== filter}
              aria-label={`${row.repository} - ${state}`}
              data-name={row.repository}
              onclick={() => onOpenSection(state === 'refused' ? 'files' : 'plan')}
            >
              {#if state === 'change'}<span class="t">{changesOf(row)}</span
                >{:else if state === 'refused'}<Icon
                  name="failure"
                  size="sm"
                />{:else if state === 'settled'}<Icon name="check" size="xs" />{/if}
            </button>
          {/each}
        </div>
        <div class="legend">
          {#each [['settled', 'In step', legendCounts.settled], ['change', 'Would change', legendCounts.change], ['refused', 'Refused', legendCounts.refused], ['off', 'Switched off here', legendCounts.off]] as [state, word, count] (state)}
            <button
              type="button"
              class="legend-row"
              aria-pressed={filter === state}
              onclick={() => (filter = filter === state ? null : (state as TileState))}
            >
              <span class={`legend-swatch is-${state}`}></span>
              <span class="legend-word">{word}</span>
              <span class="legend-count">{count}</span>
            </button>
          {/each}
        </div>
      </div>
    {/if}
    {#if totalChanges > 0}
      <div class="board-foot">
        <div class="board-foot-say">
          <span class="board-foot-line"
            >{totalChanges} changes across {changedRepos} repositories{removals > 0
              ? `, including ${removals} removal${removals === 1 ? '' : 's'}`
              : ''}</span
          >
          <span class="board-foot-when"
            >{plan !== null ? `Expires ${formatUntil(plan.expires_at, nowMs)} · ` : ''}nothing
            happens until you apply it</span
          >
        </div>
        <Button tone="signal" href={sectionHref('plan')} onclick={(event) => open(event, 'plan')}
          >Review the plan</Button
        >
      </div>
    {/if}
  </div>

  <!-- The out-of-step list: every repository the board colours, with its reason on
       the row. The board shows the shape; this says the words - in the same row
       grammar every other list on the panel uses. -->
  {#if attention.length > 0}
    <Card>
      <div class="card-head">
        <h2 class="card-title" id="sync-attention-label">Needs attention</h2>
      </div>
      <ul class="object-list" aria-labelledby="sync-attention-label">
        {#each attention as row (row.repository)}
          {@const refused = states.get(row.repository) === 'refused'}
          <li>
            <a
              class="object-row"
              href={sectionHref(refused ? 'files' : 'plan')}
              onclick={(event) => open(event, refused ? 'files' : 'plan')}
            >
              <span class="object-main">
                <span class="object-name-row">
                  <span class="object-name"><span class="file-path">{row.repository}</span></span>
                  {#if refused}
                    <span class="mx-mark mx-refused" role="img" aria-label="refused"
                      ><span class="t">refused</span></span
                    >
                  {:else}
                    <span
                      class="mx-mark mx-pending"
                      role="img"
                      aria-label="would change, {changesOf(row)} changes"
                      ><span class="t">{changesOf(row)} changes</span></span
                    >
                  {/if}
                </span>
                <span class="object-sum" class:is-refused={refused}
                  >{refused ? (row.reason ?? '') : attnWhy(row)}</span
                >
              </span>
              <span class="object-side">
                <span class="gi"><Icon name="chevron-right" size="xs" /></span>
              </span>
            </a>
          </li>
        {/each}
      </ul>
    </Card>
  {/if}

  <!-- Kind cards: the strip repeats the board's slots in the board's order - so
       with no slots to repeat there is no card to draw. Four of them saying
       "0 labels", "0 of 17 managed", "0 rulesets", "0 templates" under a panel
       that has just said the workspace syncs nothing is the same nothing, four
       more times, and the drawing has none of them here. The switches move onto
       each kind's own page, which the tree still reaches. -->
  {#if rows.length > 0}
    <div class="kind-grid">
      {#each SYNC_KINDS as kind (kind)}
        {@const config = configs[kind]}
        <div
          class="kind-card"
          class:is-off={config?.enabled === false}
          class:is-unsaved={kindDirty(kind)}
          data-unsaved={kindDirty(kind) || undefined}
        >
          <!-- The sheet's own head, not a private copy of it: `card-head` carries
               the fixed head line that puts a title's cap the same distance from
               every card's top edge whatever stands beside it, and `card-title`
               carries the tier. Written out here, this head had neither - so its
               name sat 4px lower than the two cards above it and one tier down. -->
          <div class="card-head kind-card-head">
            <a
              class="card-title kind-name"
              href={sectionHref(KIND_SECTION[kind])}
              onclick={(event) => open(event, KIND_SECTION[kind])}>{KIND_LABEL[kind]}</a
            >
            <Switch
              checked={config?.enabled === true}
              label={`${KIND_LABEL[kind]} sync`}
              bare
              disabled={readOnly || config === undefined}
              onToggle={(next) => onToggleKind(kind, next)}
            />
          </div>
          <span class="kind-sum">{kindSum(kind)}</span>
          <div class="kind-strip-row">
            <div class="kind-strip">
              {#each rows as row (row.repository)}
                {@const cell = row.cells[kind]}
                <span
                  class:is-change={(cell.changes ?? 0) > 0}
                  class:is-refused={cell.state === 'refused'}
                  class:is-off={cell.state === 'off'}
                  title={row.repository}
                ></span>
              {/each}
            </div>
          </div>
          <span class="kind-foot">
            <span class="kind-when">{kindWhen(kind)}</span>
            <a
              class="kind-open"
              href={sectionHref(KIND_SECTION[kind])}
              aria-label={`Open ${KIND_LABEL[kind].toLowerCase()}`}
              onclick={(event) => open(event, KIND_SECTION[kind])}
            >
              <Icon name="chevron-right" size="micro" />
            </a>
          </span>
        </div>
      {/each}
    </div>
  {/if}
</section>

<style>
  /* `.verdict-head` and everything it carries are in `app.css`. Three cards wear the
     class and only this one had these rules, so the other two were laid out by the shared
     copy alone - and when that copy stopped applying, this one went on looking right and
     hid it. */

  /* A card in everything but its class - it wears the sheet's own surface
     tokens, and its padding is the surface padding rather than a number of
     its own. It held its content 4px closer than every card beside it. */
  .board {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    padding: var(--rhythm-surface-pad);
  }

  /* start, not the stretch default: stretched, the well grew to the legend
     column's height wherever the tiles wrapped to fewer rows. */
  .board-lay {
    align-items: start;
    display: grid;
    gap: var(--space-5);
    grid-template-columns: 1fr auto;
  }

  .board-well {
    align-content: start;
    background: var(--tile-well);
    border-radius: 10px;
    box-shadow: var(--well-shadow);
    display: grid;
    gap: 7px;
    /* auto-fit, not auto-fill: fill keeps its empty ghost tracks and leaves
       a dead band inside the well's right edge. */
    grid-template-columns: repeat(auto-fit, minmax(2.75rem, 1fr));
    padding: 10px;
  }

  .tile {
    align-items: center;
    appearance: none;
    /* A declared 44px, not aspect-ratio: fluid tracks made the keycap height
       fractional, so every row edge rasterised soft. */
    block-size: 2.75rem;
    background: var(--tile-face);
    border: 1px solid var(--tile-border);
    border-radius: 9px;
    box-shadow: var(--tile-shadow);
    cursor: pointer;
    display: flex;
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-variant-numeric: tabular-nums;
    justify-content: center;
    padding: 0;
    position: relative;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      border-color var(--duration-fast) var(--ease-standard),
      color var(--duration-fast) var(--ease-standard),
      opacity var(--duration-fast) var(--ease-standard),
      translate var(--duration-press) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard);
  }

  /* The veil is the ::before layer, so every cap answers - coloured, off and
     settled alike - and the answer FADES. */
  .tile .t {
    text-box: trim-both cap alphabetic;
  }

  .tile::before {
    background: var(--row-pressed);
    border-radius: inherit;
    content: '';
    inset: 0;
    opacity: 0;
    pointer-events: none;
    position: absolute;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .tile:hover {
    translate: 0 -1px;
  }

  .tile:hover::before {
    opacity: 0.5;
  }

  .tile:active {
    box-shadow: var(--pressed-inset);
    translate: 0 1px;
  }

  .tile:active::before {
    opacity: 1;
  }

  .tile:hover::after,
  .tile:focus-visible::after {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: 6px;
    bottom: calc(100% + 5px);
    box-shadow: var(--shadow-popover);
    color: var(--text-primary);
    content: attr(data-name);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    left: 50%;
    padding: 0.3rem 0.5rem;
    position: absolute;
    translate: -50% 0;
    white-space: nowrap;
    z-index: 2;
  }

  .tile.is-settled {
    color: color-mix(in srgb, var(--text-muted) 75%, transparent);
  }

  .tile.is-change {
    background: color-mix(in srgb, var(--diff-chg-ink) 14%, var(--tile-face));
    border-color: color-mix(in srgb, var(--diff-chg-ink) 55%, transparent);
    color: var(--diff-chg-ink);
    font-weight: 600;
  }

  .tile.is-refused {
    background: color-mix(in srgb, var(--diff-del-ink) 14%, var(--tile-face));
    border-color: color-mix(in srgb, var(--diff-del-ink) 55%, transparent);
    color: var(--diff-del-ink);
  }

  .tile.is-off {
    background: none;
    /* A whole stroke: 1.5px rasterised soft on both edges. */
    border: 1px dashed color-mix(in srgb, var(--text-muted) 90%, transparent);
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
    appearance: none;
    background: none;
    border: 0;
    /* A pressable row wears a declared height: 28px whole. */
    block-size: var(--tier-quiet);
    border-radius: var(--r-ctl);
    box-sizing: border-box;
    color: inherit;
    cursor: pointer;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: auto 1fr auto;
    padding: 0 0.5rem;
    position: relative;
    text-align: start;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      translate var(--duration-press) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard);
  }

  .legend-row::before {
    background: var(--row-pressed);
    border-radius: inherit;
    content: '';
    inset: 0;
    opacity: 0;
    pointer-events: none;
    position: absolute;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .legend-row:hover {
    background: var(--row-hover);
  }

  .legend-row:active {
    background: var(--row-pressed);
    box-shadow: var(--pressed-inset);
    translate: 0 1px;
  }

  .legend-row[aria-pressed='true'] {
    background: var(--interactive-pressed-bg);
  }

  .legend-row[aria-pressed='true']:hover::before {
    opacity: 0.5;
  }

  .legend-row[aria-pressed='true']:active::before {
    opacity: 1;
  }

  /* 14px, not 0.85rem: 13.6 rasterised every swatch edge soft. */
  .legend-swatch {
    border: 1px solid var(--tile-border);
    border-radius: 4px;
    block-size: 14px;
    inline-size: 14px;
  }

  .legend-swatch.is-settled {
    background: var(--tile-face);
    box-shadow: var(--tile-shadow);
  }

  .legend-swatch.is-change {
    background: color-mix(in srgb, var(--diff-chg-ink) 14%, var(--tile-face));
    border-color: color-mix(in srgb, var(--diff-chg-ink) 55%, transparent);
  }

  .legend-swatch.is-refused {
    background: color-mix(in srgb, var(--diff-del-ink) 14%, var(--tile-face));
    border-color: color-mix(in srgb, var(--diff-del-ink) 55%, transparent);
  }

  .legend-swatch.is-off {
    background: none;
    border: 1px dashed var(--text-muted);
  }

  .legend-word {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    text-box: trim-both cap alphabetic;
  }

  .legend-count {
    color: var(--text-primary);
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-variant-numeric: tabular-nums;
    text-box: trim-both cap alphabetic;
  }

  .board-foot {
    align-items: center;
    border-top: 1px solid var(--border-subtle);
    display: flex;
    gap: var(--space-3);
    margin-top: var(--space-4);
    padding-top: var(--space-3);
  }

  .board-foot-say {
    display: grid;
    flex: 1;
    /* 8, not 4: with both lines ink-trimmed the gap IS the ink distance. */
    gap: var(--space-2);
  }

  .board-foot-line {
    font-size: var(--font-size-meta);
    font-weight: 600;
    min-block-size: 10px;
    text-box: trim-both cap alphabetic;
  }

  .board-foot-when {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    /* Ink-true, so the two-line say centres against the Apply button by
       what the eye reads rather than by the second line's leading. */
    min-block-size: 8px;
    text-box: trim-both cap alphabetic;
  }

  /* A refused row's account steps up one ink - louder than routine metadata,
     calmer than the mark that already carries the state. */
  .object-sum.is-refused {
    color: var(--text-secondary);
  }

  /* A repository name in a row keeps the mono voice the product gives one. */
  .file-path {
    font-family: var(--mono);
    font-weight: 500;
  }

  /* Two across by default: four only where each card genuinely gets ~16rem. */
  /* No margin: the frame's gap is what separates this from the card above it,
     and a margin on top of it made the one distance on the page that was 40px
     where every other pair is 16. */
  .kind-grid {
    display: grid;
    gap: var(--space-4);
    grid-template-columns: repeat(2, 1fr);
  }

  @media (min-width: 85rem) {
    .kind-grid {
      grid-template-columns: repeat(4, 1fr);
    }
  }

  .kind-card {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    display: grid;
    gap: var(--space-3);
    grid-template-rows: auto 1fr auto auto;
    /* The 1fr column must be allowed to BE a quarter: left at min-width auto,
       a fleet's worth of strip slots sets the track's floor and all four
       cards march past the frame together. */
    min-width: 0;
    padding: var(--rhythm-surface-pad);
    position: relative;
    text-decoration: none;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      border-color var(--duration-fast) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard),
      translate var(--duration-press) var(--ease-standard);
  }

  .kind-card.is-unsaved {
    border-color: color-mix(in srgb, var(--brand-action) 55%, var(--border-subtle));
    box-shadow: inset 2px 0 var(--brand-action);
  }

  /* A raised fill alone measured 1.2 dE00 on the light card - invisible. The
     card answers with its edge and a lift instead. */
  .kind-card:hover {
    background: var(--surface-raised);
    border-color: var(--control-border);
    box-shadow: 0 2px 8px var(--shadow-color);
  }

  .kind-card:hover :global(.kind-open) {
    color: var(--text-primary);
  }

  .kind-card:active {
    box-shadow: var(--pressed-inset);
    translate: 0 1px;
  }

  /* The title's link covers the card, so the hover keeps its promise; the
     switch and chevron sit above it and stay their own controls. */
  .kind-card-head a::after {
    content: '';
    inset: 0;
    position: absolute;
  }

  .kind-card :global(.switch),
  .kind-card .kind-open {
    position: relative;
    z-index: 1;
  }

  /* `.card-head` gives the tier, the trim and the head line - the title's cap,
     which is what puts it the same distance below every card's edge. The margin
     below belongs to the grid's gap, not to the head. */
  /* The head keeps the sheet's line - the title's cap - so the name opens the
     same 20px below the card's edge that the stamp closes above it and the words
     hold from the sides. The switch centres on that line and overflows it, which
     is what the line is for: a control is placed by it, never sized by it. */
  .kind-card-head {
    justify-content: space-between;
    margin-block-end: 0;
  }

  /* `.card-title` gives the tier and the trim; this only says it is a link. */
  .kind-name {
    color: inherit;
    text-decoration: none;
  }

  .kind-name:hover {
    color: var(--brand-action-text);
  }

  /* THE COPY-RHYTHM LAW, on the kind-card family. The distance from the name's box to
     this one is 8px like every other name-and-sentence pair, and here it is reached by
     PULLING rather than by setting: the card's own 12px grid gap plus the 4px of slack
     the head row carries over the trimmed name already spend 16, so the family's
     declared value is what takes it back to 8. Measured rendered, not declared -
     re-measure if the head's type or the card's tracks move. */
  .kind-sum {
    align-self: start;
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    line-height: var(--leading-meta);
    margin-block-start: var(--row-copy-gap-kind);
  }

  /* One slot per repository, the board's order, the board's material. */
  .kind-strip-row {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }

  /* A track per slot and wrapping rows, so the strip carries any fleet: one
     row of stretched slots for a handful of repositories, more rows of
     uniform ones for dozens. Flexed slots shrank with the fleet instead -
     at 80 repositories they were 2px of border and the cards overflowed the
     frame, because the un-wrappable run set the column's min-content. */
  .kind-strip {
    display: grid;
    flex: 1;
    gap: 3px;
    grid-template-columns: repeat(auto-fit, minmax(12px, 1fr));
  }

  /* 12px whole: at tablet widths 9px slots read as slivers, and half-pixel
     borders rasterised soft. */
  .kind-strip span {
    background: color-mix(in srgb, var(--text-primary) 6%, var(--surface-base));
    border: 1px solid color-mix(in srgb, var(--border-control) 55%, transparent);
    border-radius: 3px;
    block-size: 12px;
    box-sizing: border-box;
  }

  .kind-strip .is-change {
    background: color-mix(in srgb, var(--diff-chg-ink) 45%, var(--surface-base));
    border-color: transparent;
  }

  .kind-strip .is-refused {
    background: color-mix(in srgb, var(--diff-del-ink) 50%, var(--surface-base));
    border-color: transparent;
  }

  .kind-strip .is-off {
    background: none;
    border: 1px dashed color-mix(in srgb, var(--text-muted) 70%, transparent);
  }

  .kind-foot {
    align-items: center;
    align-self: end;
    display: flex;
    gap: var(--space-2);
    justify-content: space-between;
  }

  .kind-when {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    /* Ink-true, so the stamp shares the foot's centre with the chevron. */
    min-block-size: 8px;
    text-box: trim-both cap alphabetic;
  }

  .kind-card :global(.switch) {
    min-block-size: auto;
  }

  /* THE GROUND AND THE TARGET COME BACK AS PAINT, NOT AS LAYOUT - the same
     trade the bare switch makes. Laid out at 24px this was the tallest thing in
     the foot, so the card's last line of ink floated 28.9px above an edge whose
     sides were 20: the reader sees the stamp, not the round ground that appears
     under a pointer. Sized to its glyph, the foot's box IS its ink and the card
     closes 20px below the words. The pseudo keeps the 24px WCAG 2.5.8 asks for
     and the ground a hover paints, both centred on the glyph. */
  .kind-open {
    align-items: center;
    color: var(--text-muted);
    display: inline-flex;
    justify-content: center;
    padding: 0;
    position: relative;
    transition:
      color var(--duration-fast) var(--ease-standard),
      translate var(--duration-press) var(--ease-standard);
  }

  .kind-open::before {
    block-size: var(--field-target-min);
    border-radius: 50%;
    content: '';
    inline-size: var(--field-target-min);
    inset-block-start: 50%;
    inset-inline-start: 50%;
    position: absolute;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard);
    translate: -50% -50%;
  }

  .kind-open:hover {
    color: var(--text-primary);
  }

  .kind-open:hover::before {
    background: var(--interactive-hover-layer);
  }

  .kind-open:active {
    translate: 0 1px;
  }

  .kind-open:active::before {
    background: var(--interactive-pressed);
    box-shadow: var(--pressed-inset);
  }

  .kind-card.is-off .kind-sum,
  .kind-card.is-off .kind-name {
    color: var(--text-muted);
  }

  @media (max-width: 52rem) {
    .hero {
      gap: var(--space-3);
      grid-template-columns: 1fr;
    }

    .board-lay {
      grid-template-columns: 1fr;
    }

    .legend {
      display: grid;
      gap: var(--space-1) var(--space-2);
      grid-template-columns: 1fr 1fr;
      min-width: 0;
    }

    .kind-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
