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

<!--
@component
The rulesets list: named objects, two levels deep and no deeper - press
a row for the ruleset's own page. Enforcement is worn as a pill on the
row, so Evaluate mode is visible from the list. Below, the same two
decisions every kind carries: removal, and the names left alone.
-->

<script lang="ts">
  import { receipts } from '../receipts.svelte';
  import type { SyncConfig, SyncPlan, SyncStatus } from '../types';

  import Card from './Card.svelte';
  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import Popover from './Popover.svelte';
  import Switch from './Switch.svelte';
  import SyncKindFacts, { syncSwitchLabel, syncSwitchWord } from './SyncKindFacts.svelte';

  const {
    config,
    savedDocument = {},
    plan,
    readOnly,
    problem = null,
    syncStatus = null,
    nowMs,
    rulesetHref,
    onOpenRuleset,
    onToggleEnabled,
    onChangeDocument,
    dirtyEnabled = false,
    dirtyDocument = false,
  }: {
    config: SyncConfig | null;
    savedDocument?: Record<string, unknown>;
    plan: SyncPlan | null;
    readOnly: boolean;
    problem?: string | null;
    /** The fleet, for how far this kind reaches. */
    syncStatus?: SyncStatus | null;
    nowMs: number;
    rulesetHref: (name: string) => string;
    onOpenRuleset: (name: string) => void;
    onToggleEnabled: (enabled: boolean) => void;
    onChangeDocument: (document: Record<string, unknown>) => void;
    dirtyEnabled?: boolean;
    dirtyDocument?: boolean;
  } = $props();

  const stored = $derived(config?.document ?? {});
  const enabled = $derived(config?.enabled ?? false);
  const unreadable = $derived(config?.unreadable === true);
  const unavailable = $derived(config?.unavailable ?? '');
  const frozen = $derived(readOnly || unreadable || config === null);

  const rulesets = $derived(
    Array.isArray(stored.rulesets) ? (stored.rulesets as SyncRuleset[]) : [],
  );
  const allowRemoval = $derived(stored.allow_removal === true);
  const excludes = $derived(Array.isArray(stored.excludes) ? (stored.excludes as string[]) : []);
  const savedRulesets = $derived(
    Array.isArray(savedDocument.rulesets) ? (savedDocument.rulesets as SyncRuleset[]) : [],
  );

  function stage(change: Partial<Record<string, unknown>>): void {
    if (frozen) return;
    onChangeDocument({ ...stored, ...change });
  }

  function same(left: unknown, right: unknown): boolean {
    try {
      return JSON.stringify(left) === JSON.stringify(right);
    } catch {
      return false;
    }
  }

  function rulesetDirty(ruleset: SyncRuleset): boolean {
    return (
      dirtyDocument &&
      !same(
        ruleset,
        savedRulesets.find((saved) => saved.name === ruleset.name),
      )
    );
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
    onChangeDocument({ ...stored, rulesets: [...rulesets, born] });
    receipts.say(`${name} added, enforcing nothing yet - its rules are chosen on this page`);
    onOpenRuleset(name);
  }
</script>

<div class="view-frame">
  <PageHeader
    id="sync-rulesets-heading"
    section="Sync"
    title="Rulesets"
    description="Saved rulesets are kept in step automatically across syncing repositories"
    statusUnsaved={dirtyEnabled}
  >
    {#snippet actions()}
      <Popover role="dialog" label="Name the ruleset" align="end" bind:open={adding}>
        {#snippet trigger(attributes)}
          <button {...attributes} class="btn" disabled={frozen}>
            <Icon name="plus" size="sm" />
            <span class="button-label">Add a ruleset</span>
          </button>
        {/snippet}
        <div class="name-menu">
          <div class="menu-search">
            <Icon name="search" size="xs" />
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
    {/snippet}
    {#snippet status()}
      <SyncKindFacts
        kind="rulesets"
        {enabled}
        status={syncStatus}
        updatedBy={config?.updated_by ?? ''}
        updatedAt={config?.updated_at ?? ''}
        {nowMs}
      />
      <Switch
        checked={enabled}
        label={syncSwitchLabel('rulesets', enabled)}
        word={syncSwitchWord(enabled)}
        disabled={frozen}
        onToggle={onToggleEnabled}
      />
    {/snippet}
  </PageHeader>

  {#if problem !== null}
    <FormError message={problem} />
  {/if}

  {#if unreadable}
    <p class="sync-notice" role="alert">
      This workspace's rulesets are stored in a form this version of Smyklot cannot read, so they
      are not shown and nothing here can be changed. Nothing has been lost.
    </p>
  {/if}

  {#if unavailable !== '' && enabled}
    <p class="sync-notice" role="status">
      {unavailable}. Nothing here will be planned or changed until an owner grants it on the
      workspace's App page on GitHub.
    </p>
  {/if}

  <Card unsaved={dirtyDocument}>
    <div class="card-head">
      <h2 class="card-title">{rulesets.length} {rulesets.length === 1 ? 'ruleset' : 'rulesets'}</h2>
    </div>
    {#if rulesets.length > 0}
      <div class="object-list">
        {#each rulesets as ruleset (ruleset.name)}
          {@const pill = enforcementPill(ruleset)}
          {@const pending = differs(ruleset.name)}
          <a
            class="object-row"
            class:is-unsaved={rulesetDirty(ruleset)}
            data-unsaved={rulesetDirty(ruleset) || undefined}
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
                    <span class="scope-word">{pending === 1 ? 'repository' : 'repositories'} </span>
                    {pending === 1 ? 'differs' : 'differ'}</span
                  ></span
                >
              {:else}
                <span class="mx-mark mx-instep"><Icon name="check" size="sm" /></span>
              {/if}
              <Icon name="chevron-right" size="xs" />
            </span>
          </a>
        {/each}
      </div>
    {:else if !unreadable}
      <div class="state-panel">
        <span
          ><strong>No rulesets are synced here.</strong> Every repository keeps whatever protections it
          sets itself - add one and the next plan previews it everywhere</span
        >
      </div>
    {/if}
  </Card>

  <Card>
    <div class="setting-rows">
      <div
        class="setting-row"
        class:is-unsaved={dirtyDocument && !same(stored.allow_removal, savedDocument.allow_removal)}
        data-unsaved={(dirtyDocument && !same(stored.allow_removal, savedDocument.allow_removal)) ||
          undefined}
      >
        <span class="setting-say">
          <span class="setting-name">Delete unlisted rulesets</span>
          <span class="setting-why"
            >Off, a repository may keep rulesets of its own. On, unnamed rulesets are deleted from
            every syncing repository, except ignored matches</span
          >
        </span>
        <Switch
          checked={allowRemoval}
          label="Delete unlisted rulesets"
          disabled={frozen}
          onToggle={(next) => stage({ allow_removal: next })}
        />
      </div>
      <div
        class="setting-row"
        class:is-unsaved={dirtyDocument && !same(stored.excludes, savedDocument.excludes)}
        data-unsaved={(dirtyDocument && !same(stored.excludes, savedDocument.excludes)) ||
          undefined}
      >
        <span class="setting-say">
          <span class="setting-name">Ignored rulesets</span>
          <span class="setting-why"
            >Names or globs Smyklot never creates, updates, or deletes. Ignoring overrides every
            list above</span
          >
        </span>
        <span class="setting-value">
          <PatternEntries
            patterns={excludes}
            readOnly={frozen}
            onChange={(next) => stage({ excludes: next })}
          />
        </span>
      </div>
    </div>
  </Card>
</div>

<style>
  .object-row.is-unsaved {
    background: color-mix(in srgb, var(--brand-action-tint) 45%, transparent);
    box-shadow: inset 2px 0 var(--brand-action);
  }

  .card.is-unsaved {
    border-color: color-mix(in srgb, var(--brand-action) 55%, var(--border-subtle));
  }

  .card + .card {
    /* THE DISTANCE BETWEEN TWO CARDS ON A PAGE, and there is only one. */
    margin-block-start: var(--rhythm-card-gap);
  }

  /* THE HEAD'S LINE IS ITS TITLE'S CAP, so the title-to-first-row ink never
     depends on which adornments the card happens to carry. A control in the
     head gives its own slack back rather than growing the line. */
  .card-head {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    margin-bottom: var(--rhythm-card-head-body);
    min-block-size: var(--card-head-line);
  }

  .card-head :global(.btn) {
    margin-block: calc((var(--card-head-line) - var(--control-height-compact)) / 2);
  }

  .sync-notice {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    font-size: var(--font-size-meta);
    margin: 0 0 var(--space-4);
    padding: var(--space-2) var(--space-3);
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
    padding: var(--row-pad-default) var(--space-3);
    position: relative;
    text-decoration: none;
  }

  .object-row:active {
    background: var(--row-pressed);
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
    min-block-size: var(--object-name-line);
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
    block-size: var(--tier-mark);
    border-radius: var(--radius-chip);
    display: inline-flex;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: 0.25rem;
    line-height: var(--leading-flat);
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
    line-height: var(--leading-tight);
    padding: var(--space-1) var(--space-3) var(--space-2);
  }

  /* ---------- The bottom card ---------- */

  @media (max-width: 36rem) {
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
  }
</style>
