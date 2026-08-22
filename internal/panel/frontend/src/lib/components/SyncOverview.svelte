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
  import type { SyncSection } from '../routes';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import Switch from './Switch.svelte';
  import { SETTINGS_FIELD_KEYS, SETTINGS_FIELD_TOTAL } from './SyncSettingsPage.svelte';

  const {
    status,
    plan,
    configs,
    nowMs,
    sectionHref,
    onOpenSection,
    onToggleKind,
    readOnly = false,
  }: {
    status: SyncStatus;
    plan: SyncPlan | null;
    configs: Partial<Record<SyncKind, SyncConfig>>;
    /** The clock, passed in so a story renders the same minute every time. */
    nowMs: number;
    sectionHref: (section: SyncSection) => string;
    onOpenSection: (section: SyncSection) => void;
    onToggleKind: (kind: SyncKind, enabled: boolean) => void;
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

  const KIND_LABEL: Record<SyncKind, string> = {
    labels: 'Labels',
    settings: 'Settings',
    rulesets: 'Rulesets',
    files: 'Files',
  };
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
</script>

<div class="view-frame">
  <!-- The verdict: the overview's first CONTENT, not a second header. One
       freshness fact, sharing the verdict's baseline. -->
  <div class="hero">
    <h2>
      {#if outOfStep > 0}<span class="is-drift">{outOfStep} of {rows.length}</span>
        are out of step{:else}All {rows.length} are in step{/if}
    </h2>
    <span class="hero-meta"
      >Checked <strong>{formatRelative(status.checked_at, nowMs)}</strong></span
    >
  </div>

  <div class="board">
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
                size={14}
              />{:else if state === 'settled'}<Icon name="check" size={12} />{/if}
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

  <!-- The out-of-step list: every repository the board colours, with its
       reason on the row. The board shows the shape; this says the words. -->
  {#if attention.length > 0}
    <div class="attn">
      {#each attention as row (row.repository)}
        {@const refused = states.get(row.repository) === 'refused'}
        <a
          class="attn-row"
          href={sectionHref(refused ? 'files' : 'plan')}
          onclick={(event) => open(event, refused ? 'files' : 'plan')}
        >
          <span class="attn-repo">{row.repository}</span>
          <span class="attn-what">
            {#if refused}
              <span class="mx-mark mx-refused"><span class="t">refused</span></span>
            {:else}
              <span class="mx-mark mx-pending"><span class="t">{changesOf(row)} changes</span></span
              >
            {/if}
          </span>
          <span class="attn-why" class:is-refused={refused}
            >{refused ? (row.reason ?? '') : attnWhy(row)}</span
          >
        </a>
      {/each}
    </div>
  {/if}

  <!-- Kind cards: the strip repeats the board's slots in the board's order. -->
  <div class="kind-grid">
    {#each SYNC_KINDS as kind (kind)}
      {@const config = configs[kind]}
      <div class="kind-card" class:is-off={config?.enabled === false}>
        <div class="kind-card-head">
          <a
            class="kind-name"
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
            <Icon name="chevron-right" size={10} />
          </a>
        </span>
      </div>
    {/each}
  </div>
</div>

<style>
  .view-frame {
    margin-inline: auto;
    max-width: var(--content-max);
  }

  /* End-aligned: both sides are trimmed to the alphabetic edge, so end IS the
     shared baseline - and the row stays whole where baseline union drifted. */
  .hero {
    align-items: end;
    display: grid;
    gap: var(--space-4);
    grid-template-columns: 1fr auto;
    margin-block: var(--space-2) var(--space-4);
  }

  .hero h2 {
    /* 38px whole - 2.35rem was 37.6, and the trimmed cap box floors to 29
       so the board below starts on a whole pixel. */
    font-size: 2.375rem;
    font-weight: 700;
    letter-spacing: -0.03em;
    line-height: round(1.1em, 1px);
    margin: 0;
    min-block-size: 29px;
    text-box: trim-both cap alphabetic;
  }

  .hero h2 .is-drift {
    color: var(--diff-chg-ink);
  }

  .hero-meta {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }

  .hero-meta strong {
    color: var(--text-secondary);
    font-weight: 600;
  }

  .board {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    padding: var(--space-4);
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
    background: var(--table-row-pressed);
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
    block-size: 28px;
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
    background: var(--table-row-pressed);
    border-radius: inherit;
    content: '';
    inset: 0;
    opacity: 0;
    pointer-events: none;
    position: absolute;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .legend-row:hover {
    background: var(--table-row-hover);
  }

  .legend-row:active {
    background: var(--table-row-pressed);
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

  .attn {
    display: grid;
    margin-top: var(--space-4);
  }

  .attn-row {
    align-items: baseline;
    border-radius: var(--r-ctl);
    color: inherit;
    cursor: pointer;
    display: grid;
    gap: var(--space-3);
    grid-template-columns: 9.5rem auto 1fr;
    padding: 0.5rem var(--space-3);
    position: relative;
    text-decoration: none;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      translate var(--duration-press) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard);
  }

  .attn-row:not(:last-child)::after {
    background: var(--border-subtle);
    block-size: 1px;
    bottom: 0;
    content: '';
    inset-inline: var(--space-3);
    position: absolute;
  }

  /* The hover pill has rounded corners; a hairline crossing its edge reads
     as a crack in it. The hovered row hides its own separator and the one
     its neighbour would draw over it. */
  .attn-row:hover::after,
  .attn-row:has(+ .attn-row:hover)::after {
    background: transparent;
  }

  .attn-row:hover {
    background: var(--table-row-hover);
  }

  .attn-row:active {
    background: var(--table-row-pressed);
    box-shadow: var(--pressed-inset);
    translate: 0 1px;
  }

  .attn-repo {
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-weight: 500;
  }

  .attn-what {
    display: flex;
    gap: var(--space-2);
  }

  .attn-why {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .attn-why.is-refused {
    color: var(--text-secondary);
  }

  .mx-mark {
    align-items: center;
    border-radius: var(--r-chip);
    /* Declared 20px, the chip-small height. */
    block-size: 20px;
    box-sizing: border-box;
    display: inline-flex;
    gap: 0.25rem;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    line-height: 1;
    padding: 0 0.5rem;
  }

  .mx-mark .t {
    display: block;
    /* Ink-true, so the chip's words sit on the chip's own centre. */
    text-box: trim-both cap alphabetic;
  }

  .mx-pending {
    background: var(--cell-pending-bg);
    border: 1px solid color-mix(in srgb, var(--cell-pending) 38%, transparent);
    color: var(--cell-pending);
    font-weight: 500;
  }

  .mx-refused {
    background: var(--cell-refused-bg);
    border: 1px solid color-mix(in srgb, var(--cell-refused) 38%, transparent);
    color: var(--cell-refused);
    font-weight: 500;
  }

  /* Two across by default: four only where each card genuinely gets ~16rem. */
  .kind-grid {
    display: grid;
    gap: var(--space-4);
    grid-template-columns: repeat(2, 1fr);
    margin-block: var(--space-6);
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
    padding: var(--space-4);
    position: relative;
    text-decoration: none;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      border-color var(--duration-fast) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard),
      translate var(--duration-press) var(--ease-standard);
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

  .kind-card-head {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    justify-content: space-between;
  }

  .kind-name {
    color: inherit;
    font-size: var(--font-size-title);
    font-weight: 600;
    min-block-size: 12px;
    text-box: trim-both cap alphabetic;
    text-decoration: none;
  }

  .kind-name:hover {
    color: var(--brand-action-text);
  }

  .kind-sum {
    align-self: start;
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    line-height: round(1.5em, 1px);
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

  .kind-open {
    align-items: center;
    block-size: 24px;
    border-radius: 50%;
    color: var(--text-muted);
    display: inline-flex;
    inline-size: 24px;
    justify-content: center;
    padding: 0;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      color var(--duration-fast) var(--ease-standard),
      translate var(--duration-press) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard);
  }

  .kind-open:hover {
    background: var(--interactive-hover-layer);
    color: var(--text-primary);
  }

  .kind-open:active {
    background: var(--interactive-pressed);
    box-shadow: var(--pressed-inset);
    translate: 0 1px;
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

  @media (max-width: 30rem) {
    .attn-row {
      grid-template-columns: 1fr;
    }
  }
</style>
