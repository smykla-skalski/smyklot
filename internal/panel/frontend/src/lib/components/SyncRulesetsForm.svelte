<script module lang="ts">
  import type { SyncRuleset, SyncRulesetRules } from '#lib/types.js';

  /** How a ruleset's enforcement is worn on its row, and said on its page. */
  export const ENFORCEMENTS: readonly { value: string; title: string; why: string }[] = [
    {
      value: 'active',
      title: 'Active',
      why: 'The rules hold. A push that breaks one is refused',
    },
    {
      value: 'evaluate',
      title: 'Evaluate',
      why: 'A dry run: violations are recorded on GitHub, nothing is blocked',
    },
    {
      value: 'disabled',
      title: 'Disabled',
      why: 'Written to every repository but switched off there',
    },
  ];

  /** How many of the rules this panel can express are on. */
  export function ruleCount(rules: SyncRulesetRules | undefined): number {
    if (rules === undefined) return 0;

    return Object.values(rules).filter((rule) => rule !== undefined && rule !== false).length;
  }

  /** What a ruleset is, in the one line a list row carries. */
  export function rulesetSummary(ruleset: SyncRuleset): string {
    const include = ruleset.conditions?.include ?? [];
    const where =
      include.length === 0
        ? 'no refs yet'
        : include.includes('~DEFAULT_BRANCH') && include.length === 1
          ? 'default branch'
          : include.join(', ');
    const rules = ruleCount(ruleset.rules);
    const actors = ruleset.bypass_actors?.length ?? 0;

    return [
      where,
      `${rules} ${rules === 1 ? 'rule' : 'rules'}`,
      actors === 0 ? 'no bypass' : `${actors} bypass ${actors === 1 ? 'actor' : 'actors'}`,
    ].join(' · ');
  }
</script>

<script lang="ts">
  /**
   * What an installation expects its repositories to enforce, as a list of
   * named things.
   *
   * Two levels and no deeper: this page answers "which rulesets, and is each
   * one holding", and a row opens the ruleset's own page for everything else.
   * The whole configuration used to be one page - nine rules, their parameters,
   * their bypass actors and their ref patterns, per ruleset, unfolded - and a
   * page that long is one nobody reads before pressing Save.
   *
   * Enforcement is worn on the row, because Evaluate is a ruleset that looks
   * enforced and enforces nothing, and that is exactly the thing a list should
   * not hide behind a press.
   */
  // The types are imported by the module script above; a second import here is
  // a second declaration in the same file, which TypeScript refuses.
  import { storedList } from '#lib/form-lists.js';

  import Chip from './Chip.svelte';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import ObjectRow from './ObjectRow.svelte';
  import Plate from './Plate.svelte';
  import RemovalPolicy from './RemovalPolicy.svelte';
  import StateMark, { type SyncState } from './StateMark.svelte';
  import SyncKindHead from './SyncKindHead.svelte';

  const {
    stored,
    enabled,
    unreadable,
    unavailable = '',
    problem = null,
    readOnly,
    saving,
    rulesetHref,
    markOf,
    onSave,
  }: {
    stored: Record<string, unknown>;
    enabled: boolean;
    unreadable: boolean;
    unavailable?: string;
    problem?: string | null;
    readOnly: boolean;
    saving: boolean;
    /** Where one ruleset's own page is. */
    rulesetHref: (name: string) => string;
    /** What the plan would do about this ruleset, or nothing while there is no plan. */
    markOf?: (name: string) => { state: SyncState; label?: string } | undefined;
    onSave: (enabled: boolean, document: Record<string, unknown>) => void;
  } = $props();

  const disabled = $derived(saving || readOnly || unreadable);

  const rulesets = $derived(storedList<SyncRuleset>(stored, 'rulesets'));
  const removal = $derived(stored.allow_removal === true);
  const excludes = $derived(storedList<string>(stored, 'excludes'));

  let adding = $state(false);
  let draft = $state('');
  let field = $state<HTMLInputElement | null>(null);

  /* The whole stored document rather than the keys with controls, so a key a
     newer version of the service wrote travels back rather than being dropped
     by a browser running an older build of this page. */
  function write(change: Record<string, unknown>): void {
    onSave(enabled, { ...stored, ...change });
  }

  const TONES = { active: 'signal', evaluate: 'warning', disabled: 'absent' } as const;

  function toneOf(ruleset: SyncRuleset): 'signal' | 'warning' | 'absent' {
    return TONES[ruleset.enforcement as keyof typeof TONES] ?? 'neutral';
  }

  function wordOf(ruleset: SyncRuleset): string {
    return (
      ENFORCEMENTS.find((option) => option.value === ruleset.enforcement)?.title ??
      ruleset.enforcement
    );
  }

  function open(): void {
    adding = true;
    queueMicrotask(() => field?.focus());
  }

  /**
   * A new ruleset starts on the default branch of every repository, because
   * `refs/heads/main` protects nothing on the ones still calling it master -
   * which is the whole problem an organization-wide tool has - and with no
   * rules at all, so that what it enforces is a decision somebody makes on its
   * own page rather than one this press made for them.
   */
  function add(): void {
    const name = draft.trim();
    draft = '';
    adding = false;
    if (name === '' || rulesets.some((ruleset) => ruleset.name === name)) return;

    write({
      rulesets: [
        ...rulesets,
        {
          name,
          target: 'branch',
          enforcement: 'active',
          conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
          rules: {},
        },
      ],
    });
  }
</script>

<SyncKindHead
  title="Rulesets"
  lead="A ruleset named here is owned whole: what it does not say stops being enforced, and the plan shows exactly that before anything changes"
  noun="rulesets"
  {enabled}
  {unreadable}
  {unavailable}
  {problem}
  {readOnly}
  {saving}
  onToggle={(next) => onSave(next, stored)}
/>

<Plate label="{rulesets.length} {rulesets.length === 1 ? 'ruleset' : 'rulesets'}">
  {#snippet status()}
    {#if !readOnly}
      {#if adding}
        <input
          bind:this={field}
          bind:value={draft}
          class="text-input ruleset-name-field"
          type="text"
          spellcheck="false"
          autocomplete="off"
          aria-label="Name of the ruleset to add"
          placeholder="main-branch-protection"
          onblur={add}
          onkeydown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault();
              add();
            }
            if (event.key === 'Escape') {
              draft = '';
              adding = false;
            }
          }}
        />
      {:else}
        <!-- The section's own action, at the height every control in a content
             pane has. See SyncFilesForm for why this is not an `.add-chip`. -->
        <Button {disabled} onclick={open}>
          {#snippet icon()}<Icon name="plus" size={14} strokeWidth={2} />{/snippet}
          Add a ruleset
        </Button>
      {/if}
    {/if}
  {/snippet}

  {#if rulesets.length === 0}
    <p class="empty-note rulesets-empty">
      No rulesets yet. One named here is written to every repository this installation syncs
    </p>
  {:else}
    <div class="object-list ruled-rows">
      {#each rulesets as ruleset (ruleset.name)}
        {@const fleet = markOf?.(ruleset.name)}
        <ObjectRow
          name={ruleset.name}
          href={rulesetHref(ruleset.name)}
          summary={rulesetSummary(ruleset)}
        >
          {#snippet pill()}
            <Chip tone={toneOf(ruleset)} small>{wordOf(ruleset)}</Chip>
          {/snippet}
          {#snippet mark()}
            {#if fleet !== undefined}
              <StateMark state={fleet.state} label={fleet.label} />
            {/if}
          {/snippet}
        </ObjectRow>
      {/each}
    </div>
  {/if}
</Plate>

<RemovalPolicy
  noun="rulesets"
  {removal}
  {excludes}
  {disabled}
  onRemovalChange={(next) => write({ allow_removal: next })}
  onExcludesChange={(next) => write({ excludes: next })}
/>

<style>
  .object-list {
    display: grid;
  }

  /* The rule between two rows, and its manners around a hover, are
     `.ruled-rows` in app.css. An `.object-row` pads by `--space-2`, so that is
     where its rule stops. */
  .object-list {
    --row-rule-inset: var(--space-2);
  }

  /* An input with no ground, no edge and no focus ring at all: it carried a
     size and nothing else, so it was the browser's own control sitting in a
     panel that styles every other one. */
  .ruleset-name-field {
    font-family: var(--mono);
    inline-size: 14rem;
  }
</style>
