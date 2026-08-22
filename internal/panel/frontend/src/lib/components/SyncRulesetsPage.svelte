<script module lang="ts">
  import type { SyncRuleset } from '../types';

  /** How many of a ruleset's rules are switched on. */
  export function ruleCount(ruleset: SyncRuleset): number {
    return Object.values(ruleset.rules ?? {}).filter((value) => value !== undefined).length;
  }

  /** The refs a ruleset covers, said the way a person says them. */
  export function coverageWord(ruleset: SyncRuleset): string {
    const include = ruleset.conditions.include ?? [];
    if (include.length === 1 && include[0] === '~DEFAULT_BRANCH') return 'default branch';
    if (include.length === 0) return 'no branches yet';
    return include.join(', ');
  }
</script>

<script lang="ts">
  /**
   * The rulesets list: named objects, two levels deep and no deeper - press
   * a row for the ruleset's own page. Enforcement is worn as a pill on the
   * row, so Evaluate mode is visible from the list. Below, the same two
   * decisions every kind carries: removal, and the names left alone.
   */
  import type { SyncConfig, SyncPlan } from '../types';
  import type { SyncSection } from '../routes';

  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import PanePath from './PanePath.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import Popover from './Popover.svelte';
  import Switch from './Switch.svelte';

  const {
    config,
    plan,
    readOnly,
    problem = null,
    saving,
    sectionHref,
    onOpenSection,
    rulesetHref,
    onOpenRuleset,
    onSave,
  }: {
    config: SyncConfig | null;
    plan: SyncPlan | null;
    readOnly: boolean;
    problem?: string | null;
    saving: boolean;
    sectionHref: (section: SyncSection) => string;
    onOpenSection: (section: SyncSection) => void;
    rulesetHref: (name: string) => string;
    onOpenRuleset: (name: string) => void;
    onSave: (enabled: boolean, document: Record<string, unknown>) => void;
  } = $props();

  const stored = $derived(config?.document ?? {});
  const enabled = $derived(config?.enabled ?? false);
  const unreadable = $derived(config?.unreadable === true);
  const unavailable = $derived(config?.unavailable ?? '');
  const frozen = $derived(readOnly || unreadable || saving || config === null);

  const rulesets = $derived(
    Array.isArray(stored.rulesets) ? (stored.rulesets as SyncRuleset[]) : [],
  );
  const allowRemoval = $derived(stored.allow_removal === true);
  const excludes = $derived(Array.isArray(stored.excludes) ? (stored.excludes as string[]) : []);

  function save(change: Partial<Record<string, unknown>>): void {
    if (frozen) return;
    onSave(enabled, { ...stored, ...change });
  }

  /* ---------- What the plan says about each ruleset ---------- */

  function differs(name: string): number {
    const actions = plan?.actions ?? [];
    return new Set(
      actions
        .filter((action) => action.kind === 'rulesets' && action.subject === name)
        .map((action) => action.repository),
    ).size;
  }

  function bypassWord(ruleset: SyncRuleset): string {
    const count = ruleset.bypass_actors?.length ?? 0;
    if (count === 0) return 'no bypass';
    return `${count} bypass actor${count === 1 ? '' : 's'}`;
  }

  function enforcementPill(ruleset: SyncRuleset): { word: string; tone: string } {
    if (ruleset.enforcement === 'active') return { word: 'Active', tone: 'pill-success' };
    if (ruleset.enforcement === 'evaluate') return { word: 'Evaluate', tone: 'pill-warning' };
    return { word: 'Disabled', tone: 'pill-muted' };
  }

  function open(event: MouseEvent, name: string): void {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
      return;
    }
    event.preventDefault();
    onOpenRuleset(name);
  }

  /* ---------- Adding one: a name, then its own page ---------- */

  let adding = $state(false);
  let newName = $state('');

  const nameTaken = $derived(rulesets.some((held) => held.name === newName.trim()));

  function addRuleset(): void {
    const name = newName.trim();
    if (name === '' || nameTaken || frozen) return;
    adding = false;
    newName = '';
    /* Born disabled with the default branch covered and nothing enforced
       yet: its own page is where the rules are chosen, and a ruleset that
       arrived active with zero rules would be a policy nobody wrote. */
    const born: SyncRuleset = {
      name,
      target: 'branch',
      enforcement: 'disabled',
      conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
      rules: {},
    };
    onSave(enabled, { ...stored, rulesets: [...rulesets, born] });
    onOpenRuleset(name);
  }
</script>

<div class="view-frame">
  <PanePath
    segments={[
      { label: 'Sync', href: sectionHref('overview'), onSelect: () => onOpenSection('overview') },
    ]}
  />

  <div class="kind-head">
    <div class="kind-head-say">
      <h2 class="card-title">Rulesets</h2>
      <p class="kind-head-sub">
        A ruleset named here is owned whole: what it does not say stops being enforced, and the plan
        shows exactly that before anything changes
      </p>
    </div>
    <Switch
      checked={enabled}
      label="Ruleset sync"
      word="Syncing"
      disabled={frozen}
      onToggle={(next) => onSave(next, stored)}
    />
  </div>

  {#if problem !== null}
    <FormError message={problem} />
  {/if}

  {#if unreadable}
    <p class="sync-notice" role="alert">
      This installation's rulesets are stored in a form this version of Smyklot cannot read, so they
      are not shown and nothing here can be changed. Nothing has been lost.
    </p>
  {/if}

  {#if unavailable !== '' && enabled}
    <p class="sync-notice" role="status">
      {unavailable}. Nothing here will be planned or changed until an owner grants it on the
      installation's page on GitHub.
    </p>
  {/if}

  <div class="card">
    <div class="card-head">
      <h3 class="card-title">{rulesets.length} {rulesets.length === 1 ? 'ruleset' : 'rulesets'}</h3>
      <Popover role="dialog" label="Name the ruleset" align="end" bind:open={adding}>
        {#snippet trigger(attributes)}
          <button {...attributes} class="btn" disabled={frozen}>
            <Icon name="plus" size={13} />
            <span class="button-label">Add a ruleset</span>
          </button>
        {/snippet}
        <div class="name-menu">
          <div class="menu-search">
            <Icon name="search" size={12} />
            <input
              placeholder="main-protection"
              aria-label="Name for the new ruleset"
              spellcheck="false"
              bind:value={newName}
              onkeydown={(event) => {
                if (event.key === 'Enter') addRuleset();
              }}
            />
          </div>
          <div class="menu-hint">
            {nameTaken ? 'That name is taken' : 'Enter creates it · Esc closes'}
          </div>
        </div>
      </Popover>
    </div>
    {#if rulesets.length > 0}
      <div class="object-list">
        {#each rulesets as ruleset (ruleset.name)}
          {@const pill = enforcementPill(ruleset)}
          {@const pending = differs(ruleset.name)}
          <a
            class="object-row"
            href={rulesetHref(ruleset.name)}
            onclick={(event) => open(event, ruleset.name)}
          >
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name">{ruleset.name}</span>
                <span class="pill {pill.tone}"><span class="t">{pill.word}</span></span>
              </span>
              <span class="object-sum"
                >{coverageWord(ruleset)} · {ruleCount(ruleset)}
                {ruleCount(ruleset) === 1 ? 'rule' : 'rules'} · {bypassWord(ruleset)}</span
              >
            </span>
            <span class="object-side">
              {#if pending > 0}
                <span class="mx-mark mx-pending"
                  ><span class="t"
                    >{pending}
                    <span class="scope-word"
                      >{pending === 1 ? 'repository' : 'repositories'}
                    </span>{pending === 1 ? 'differs' : 'differ'}</span
                  ></span
                >
              {:else}
                <span class="mx-mark mx-instep"><Icon name="check" size={14} /></span>
              {/if}
              <Icon name="chevron-right" size={12} />
            </span>
          </a>
        {/each}
      </div>
    {:else if !unreadable}
      <p class="sync-empty">No rulesets yet</p>
    {/if}
  </div>

  <div class="card">
    <div class="setting-rows">
      <div class="setting-row">
        <span class="setting-say">
          <span class="setting-name">Remove rulesets this list does not name</span>
          <span class="setting-why"
            >Off, a repository may keep rulesets of its own. On, everything unnamed is deleted</span
          >
        </span>
        <Switch
          checked={allowRemoval}
          label="Remove rulesets this list does not name"
          disabled={frozen}
          onToggle={(next) => save({ allow_removal: next })}
        />
      </div>
      <div class="setting-row">
        <span class="setting-say">
          <span class="setting-name">Rulesets to leave alone</span>
          <span class="setting-why"
            >Name or pattern. Neither written nor removed, whatever the list above says</span
          >
        </span>
        <span class="setting-value">
          <PatternEntries
            patterns={excludes}
            readOnly={frozen}
            onChange={(next) => save({ excludes: next })}
          />
        </span>
      </div>
    </div>
  </div>
</div>

<style>
  .view-frame {
    margin-inline: auto;
    max-width: var(--content-max);
  }

  .kind-head {
    align-items: start;
    display: flex;
    gap: var(--space-4);
    justify-content: space-between;
    margin-bottom: var(--space-4);
  }

  .kind-head-say {
    display: grid;
    gap: var(--space-2);
  }

  .kind-head-sub {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    line-height: round(1.5em, 1px);
    margin: 0;
  }

  .kind-head :global(.switch) {
    min-block-size: auto;
  }

  .card {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    padding: var(--space-5);
  }

  .card + .card {
    margin-top: var(--space-4);
  }

  .card-title {
    font-size: var(--font-size-card-title);
    font-weight: 600;
    margin: 0;
    min-block-size: 13px;
    text-box: trim-both cap alphabetic;
  }

  .card-head {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }

  .sync-notice {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    font-size: var(--font-size-meta);
    margin: 0 0 var(--space-4);
    padding: var(--space-2) var(--space-3);
  }

  .sync-empty {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    margin: 0;
  }

  /* ---------- The list: one pressable row per named object ---------- */

  .object-list {
    display: grid;
    margin-block-end: -12px;
  }

  .object-row {
    align-items: center;
    border-radius: var(--r-ctl);
    color: inherit;
    display: grid;
    gap: var(--space-4);
    grid-template-columns: 1fr auto;
    margin-inline: calc(var(--space-3) * -1);
    padding: 0.75rem var(--space-3);
    position: relative;
    text-decoration: none;
  }

  .object-row:hover {
    background: var(--table-row-hover);
  }

  .object-row:active {
    background: var(--table-row-pressed);
    box-shadow: var(--pressed-inset);
    translate: 0 1px;
  }

  .object-row:not(:last-child)::after {
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
  .object-row:hover::after,
  .object-row:has(+ .object-row:hover)::after {
    background: transparent;
  }

  .object-main {
    display: grid;
    gap: var(--space-1);
  }

  /* 20px whether or not a pill sits in the line - the row's height is not
     allowed to depend on which badges the object happens to carry. */
  .object-name-row {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    min-block-size: 20px;
  }

  .object-name {
    font-size: var(--font-size-meta);
    font-weight: 600;
    text-box: trim-both cap alphabetic;
    text-box: trim-both cap alphabetic;
  }

  .object-sum {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .object-side {
    align-items: center;
    color: var(--text-muted);
    display: flex;
    gap: var(--space-3);
  }

  .pill {
    align-items: center;
    block-size: 20px;
    border-radius: var(--radius-chip);
    display: inline-flex;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: 0.25rem;
    line-height: 1;
    padding: 0 0.5rem;
  }

  .pill .t {
    display: block;
    text-box: trim-both cap alphabetic;
  }

  .pill-success {
    background: var(--success-tint);
    color: var(--success);
  }

  .pill-warning {
    background: var(--warning-tint);
    color: var(--warning);
  }

  .pill-muted {
    background: var(--surface-inset);
    color: var(--text-muted);
  }

  .mx-mark {
    align-items: center;
    block-size: 20px;
    border-radius: var(--r-chip);
    box-sizing: border-box;
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    gap: 0.25rem;
    line-height: 1;
    padding: 0 0.5rem;
  }

  .mx-mark .t {
    display: block;
    text-box: trim-both cap alphabetic;
  }

  /* On the board an in-step cell is the quiet norm; on a list row the same
     mark is the row's whole verdict, standing beside a worded pending pill -
     here it earns the success ink. */
  .mx-instep {
    color: var(--success);
  }

  .mx-pending {
    background: var(--cell-pending-bg);
    border: 1px solid color-mix(in srgb, var(--cell-pending) 38%, transparent);
    color: var(--cell-pending);
    font-weight: 500;
  }

  /* ---------- The name popover ---------- */

  /* The menu's 4px mat - `.menu-search` bleeds to the edges with negative
     margins that assume exactly this pad. */
  .name-menu {
    display: grid;
    inline-size: 16rem;
    padding: var(--space-1);
  }

  .menu-search {
    align-items: center;
    block-size: 36px;
    box-shadow: 0 1px 0 var(--border-subtle);
    color: var(--text-muted);
    display: flex;
    gap: var(--space-2);
    margin: calc(var(--space-1) * -1) calc(var(--space-1) * -1) var(--space-1);
    padding: 0 var(--space-3);
  }

  .menu-search input {
    background: none;
    block-size: 100%;
    border: 0;
    color: var(--text-primary);
    flex: 1;
    font-size: var(--font-size-control);
    outline: none;
    padding: 0;
  }

  .menu-search input::placeholder {
    color: var(--text-muted);
  }

  .menu-hint {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    line-height: 16px;
    padding: var(--space-1) var(--space-3) var(--space-2);
  }

  /* ---------- The bottom card ---------- */

  .setting-rows {
    display: grid;
  }

  .card > .setting-rows:only-child {
    margin-block: calc(var(--space-5) * -1);
  }

  .card > .setting-rows:only-child > .setting-row {
    align-items: start;
    padding-block: var(--space-5);
  }

  .setting-row :global(.switch) {
    align-self: center;
    margin-block: calc((20px - var(--touch-target)) / 2);
  }

  .setting-row {
    align-items: center;
    border-radius: var(--r-ctl);
    display: grid;
    gap: var(--space-2) var(--space-4);
    grid-auto-columns: auto;
    grid-auto-flow: column;
    grid-template-columns: 1fr;
    margin-inline: calc(var(--space-2) * -1);
    min-block-size: var(--touch-target);
    padding: var(--space-3) var(--space-2);
    position: relative;
  }

  .setting-row:not(:last-child)::after {
    background: var(--border-subtle);
    block-size: 1px;
    bottom: 0;
    content: '';
    inset-inline: var(--space-2);
    position: absolute;
  }

  .setting-say {
    display: grid;
    gap: var(--space-3);
  }

  .setting-name {
    font-size: var(--font-size-meta);
    font-weight: 600;
    min-block-size: 10px;
    text-box: trim-both cap alphabetic;
  }

  .setting-why {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }

  .setting-value {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    justify-content: end;
    justify-self: end;
    min-inline-size: 0;
  }

  @media (max-width: 36rem) {
    .card {
      padding: var(--space-4);
    }

    .object-row {
      gap: var(--space-2);
      grid-template-columns: minmax(0, 1fr) auto;
    }

    .object-main,
    .object-name-row,
    .object-name,
    .object-sum {
      min-inline-size: 0;
    }

    .object-name-row {
      align-items: start;
      flex-direction: column;
    }

    .object-name,
    .object-sum {
      overflow-wrap: anywhere;
    }

    .object-side {
      gap: var(--space-1);
    }

    .mx-pending .scope-word {
      display: none;
    }

    .setting-row {
      grid-auto-flow: row;
      grid-template-columns: minmax(0, 1fr);
    }

    .setting-value {
      justify-content: start;
      justify-self: stretch;
    }
  }
</style>
