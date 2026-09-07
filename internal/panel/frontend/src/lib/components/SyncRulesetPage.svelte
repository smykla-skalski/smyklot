<script module lang="ts">
  /**
   * The rules this panel can express, in the order the page lists them.
   * Everything the service's ruleset model carries and nothing more - a rule
   * the form could name but the endpoint cannot write would be a promise.
   */
  export interface RuleDef {
    key: keyof SyncRulesetRules;
    label: string;
    why: string;
    /** Whether the rule carries parameters worth an editor. */
    parameterized: boolean;
  }

  export const RULE_CATALOGUE: readonly RuleDef[] = [
    {
      key: 'pull_request',
      label: 'Require a pull request',
      why: 'Require changes to go through a pull request',
      parameterized: true,
    },
    {
      key: 'required_status_checks',
      label: 'Require status checks',
      why: 'Named checks must pass before merging',
      parameterized: true,
    },
    { key: 'non_fast_forward', label: 'Block force pushes', why: '', parameterized: false },
    { key: 'deletion', label: 'Restrict deletions', why: '', parameterized: false },
    { key: 'creation', label: 'Restrict creations', why: '', parameterized: false },
    {
      key: 'required_linear_history',
      label: 'Require linear history',
      why: '',
      parameterized: false,
    },
    { key: 'required_signatures', label: 'Require signed commits', why: '', parameterized: false },
    {
      key: 'update',
      label: 'Restrict updates',
      why: 'Only bypass actors can update matching branches',
      parameterized: true,
    },
    {
      key: 'code_scanning',
      label: 'Require code scanning',
      why: 'Block merges for missing scans or alerts above the selected thresholds',
      parameterized: true,
    },
  ];
</script>

<!--
@component
One ruleset's own page. Every card speaks the settings row grammar: say
on the left, value on the right, ghost clear at the end. A rule's
parameters ARE its value, so they sit in the value column as chips
beside Edit. Parameterized rules open a shared inspector; Done stages the
complete rule while Cancel leaves the document unchanged.
-->

<script lang="ts">
  import { tick } from 'svelte';
  import { globRuns } from '../glob-runs';
  import { numericValue } from '../merge';
  import { receipts } from '../receipts.svelte';
  import { sameRulesetFields } from '../ruleset-equality';
  import type { SyncConfig, SyncRuleset, BypassActorLookup, SyncRulesetRules } from '../types';
  import { SYNC_SECTION_LABELS, type SyncSection } from '../routes';

  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import Card from './Card.svelte';
  import BypassActorEditor from './BypassActorEditor.svelte';
  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import Popover from './Popover.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import RulesetRuleEditor from './RulesetRuleEditor.svelte';
  import type { EditableRuleKey } from '../ruleset-rule-editor';

  const {
    config,
    savedDocument = {},
    name,
    readOnly,
    organizationActors = true,
    problem = null,
    sectionHref,
    onOpenSection,
    onChangeDocument,
    dirtyDocument = false,
    lookupBypassActors,
  }: {
    config: SyncConfig | null;
    savedDocument?: Record<string, unknown>;
    /** Which ruleset the address names. */
    name: string;
    readOnly: boolean;
    organizationActors?: boolean;
    problem?: string | null;
    sectionHref: (section: SyncSection) => string;
    onOpenSection: (section: SyncSection) => void;
    onChangeDocument: (
      document: Record<string, unknown>,
      expectedDocument?: Record<string, unknown>,
    ) => boolean | void;
    dirtyDocument?: boolean;
    lookupBypassActors?: BypassActorLookup;
  } = $props();

  const stored = $derived(config?.document ?? {});
  const frozen = $derived(readOnly || config?.unreadable === true || config === null);

  const rulesets = $derived(
    Array.isArray(stored.rulesets) ? (stored.rulesets as SyncRuleset[]) : [],
  );
  const ruleset = $derived(rulesets.find((held) => held.name === name) ?? null);
  const savedRulesets = $derived(
    Array.isArray(savedDocument.rulesets) ? (savedDocument.rulesets as SyncRuleset[]) : [],
  );
  const savedRuleset = $derived(savedRulesets.find((held) => held.name === name) ?? null);

  function partDirty(part: 'enforcement' | 'conditions' | 'rules' | 'bypass_actors'): boolean {
    const current = ruleset?.[part];
    const saved = savedRuleset?.[part];
    return (
      dirtyDocument &&
      !sameRulesetFields(
        current === undefined ? {} : { [part]: current },
        saved === undefined ? {} : { [part]: saved },
      )
    );
  }

  function conditionDirty(side: 'include' | 'exclude'): boolean {
    return (
      dirtyDocument &&
      !sameRulesetFields(
        { conditions: { [side]: ruleset?.conditions[side] ?? [] } },
        { conditions: { [side]: savedRuleset?.conditions[side] ?? [] } },
      )
    );
  }

  function ruleDirty(key: keyof SyncRulesetRules): boolean {
    const current = ruleset?.rules?.[key];
    const saved = savedRuleset?.rules?.[key];
    return (
      dirtyDocument &&
      !sameRulesetFields(
        { rules: current === undefined ? {} : { [key]: current } },
        { rules: saved === undefined ? {} : { [key]: saved } },
      )
    );
  }

  let actorEditor: { toggleAdd: (trigger?: HTMLElement) => void } | undefined = $state();
  let addingActor = $state(false);

  /** Writes one changed ruleset back into the whole document. */
  function patch(
    change: Partial<SyncRuleset>,
    expectedDocument?: Record<string, unknown>,
  ): boolean | void {
    if (frozen || ruleset === null) return false;
    return onChangeDocument(
      {
        ...stored,
        rulesets: rulesets.map((held) => (held.name === name ? { ...held, ...change } : held)),
      },
      expectedDocument,
    );
  }

  function patchRules(
    change: Partial<SyncRulesetRules>,
    expectedDocument?: Record<string, unknown>,
  ): boolean | void {
    if (ruleset === null) return false;
    const rules = { ...ruleset.rules };
    for (const [key, value] of Object.entries(change)) {
      if (value === undefined) delete rules[key as keyof SyncRulesetRules];
      else (rules as Record<string, unknown>)[key] = value;
    }
    return patch({ rules }, expectedDocument);
  }

  /* What was deleted and where it stood, so the page can put it back. The document is
     the only record of a ruleset, so a delete that left for the list took the way back
     with it - and nothing has happened on GitHub yet, which is exactly the moment an
     undo is worth offering. */
  let removed = $state<{ ruleset: SyncRuleset; at: number } | null>(null);

  function deleteRuleset(): void {
    if (frozen || ruleset === null) return;
    removed = { ruleset, at: rulesets.findIndex((held) => held.name === name) };
    onChangeDocument({
      ...stored,
      rulesets: rulesets.filter((held) => held.name !== name),
    });
    receipts.say(`${name} will be removed from every syncing repository by the next sync`, {
      undo: restoreRuleset,
    });
  }

  function restoreRuleset(): void {
    const held = removed;
    if (held === null || frozen) return;
    const next = [...rulesets];
    next.splice(Math.max(0, held.at), 0, held.ruleset);
    removed = null;
    onChangeDocument({ ...stored, rulesets: next });
    receipts.say(`Put back - ${held.ruleset.name} stays`);
  }

  /* ---------- Where it applies ---------- */

  const include = $derived(ruleset?.conditions.include ?? []);
  const exclude = $derived(ruleset?.conditions.exclude ?? []);

  const coverage = $derived(
    ruleset === null && removed !== null
      ? 'Pending removal - nothing has changed on GitHub yet'
      : ruleset === null
        ? 'No ruleset by this name - it may have been renamed or removed'
        : include.length === 0
          ? 'Covering no branches yet - add a pattern below'
          : include.length === 1 && include[0] === '~DEFAULT_BRANCH'
            ? 'Targets the default branch of every syncing repository'
            : `Targets ${include.join(', ')} in every syncing repository`,
  );

  let includeOpen = $state(false);
  let excludeOpen = $state(false);
  let addValue = $state('');

  function addPattern(side: 'include' | 'exclude'): void {
    const value = addValue.trim();
    if (value === '' || ruleset === null) return;
    addValue = '';
    includeOpen = false;
    excludeOpen = false;
    const held = side === 'include' ? include : exclude;
    if (held.includes(value)) return;
    patch({ conditions: { ...ruleset.conditions, [side]: [...held, value] } });
  }

  function removePattern(side: 'include' | 'exclude', value: string): void {
    if (ruleset === null) return;
    const held = side === 'include' ? include : exclude;
    patch({
      conditions: { ...ruleset.conditions, [side]: held.filter((one) => one !== value) },
    });
  }

  /* ---------- What it enforces ---------- */

  const onRules = $derived(
    RULE_CATALOGUE.filter((rule) => ruleset?.rules?.[rule.key] !== undefined),
  );
  const offRules = $derived(
    RULE_CATALOGUE.filter((rule) => ruleset?.rules?.[rule.key] === undefined),
  );

  let pickingRule = $state(false);

  let addRuleButton = $state<HTMLButtonElement | null>(null);

  async function ruleOn(key: keyof SyncRulesetRules): Promise<void> {
    const owner = editorScope;
    pickingRule = false;
    if (isEditableRule(key)) {
      await tick();
      if (editorScope === owner && !frozen) ruleEditor?.show(key, addRuleButton ?? undefined);
    } else patchRules({ [key]: true } as Partial<SyncRulesetRules>);
  }

  function ruleOff(key: keyof SyncRulesetRules): void {
    patchRules({ [key]: undefined } as Partial<SyncRulesetRules>);
  }

  function paramChips(key: keyof SyncRulesetRules): Array<{ strong?: string; text: string }> {
    const rules = ruleset?.rules;
    if (rules === undefined) return [];
    if (key === 'pull_request' && rules.pull_request !== undefined) {
      const rule = rules.pull_request;
      const chips: Array<{ strong?: string; text: string }> = [];
      const approvals = numericValue(rule.required_approving_review_count) ?? 0;
      chips.push({
        strong: String(approvals),
        text: approvals === 1 ? 'approval' : 'approvals',
      });
      if (rule.dismiss_stale_reviews_on_push === true)
        chips.push({ text: 'stale approvals dismissed' });
      if (rule.require_code_owner_review === true) chips.push({ text: 'code owners' });
      if (rule.require_last_push_approval === true) chips.push({ text: 'last push approved' });
      if (rule.required_review_thread_resolution === true) chips.push({ text: 'threads resolved' });
      return chips;
    }
    if (key === 'required_status_checks' && rules.required_status_checks !== undefined) {
      return rules.required_status_checks.required_status_checks.map((check) => ({
        text: check.context,
      }));
    }
    if (key === 'update' && rules.update !== undefined) {
      return rules.update.update_allows_fetch_and_merge === true
        ? [{ text: 'fetch and merge allowed' }]
        : [];
    }
    if (key === 'code_scanning' && rules.code_scanning !== undefined) {
      return rules.code_scanning.code_scanning_tools.map((tool) => ({ text: tool.tool }));
    }
    return [];
  }

  let ruleEditor: { show: (key: EditableRuleKey, trigger?: HTMLElement) => void } | undefined =
    $state();
  const editorScope = $derived(`${sectionHref('rulesets')}/${name}`);

  function isEditableRule(key: keyof SyncRulesetRules): key is EditableRuleKey {
    return (
      key === 'pull_request' ||
      key === 'required_status_checks' ||
      key === 'update' ||
      key === 'code_scanning'
    );
  }
  function openEditor(key: keyof SyncRulesetRules, trigger: HTMLElement): void {
    if (!frozen && isEditableRule(key)) ruleEditor?.show(key, trigger);
  }

  /* ---------- The bypass list ---------- */

  const actors = $derived(ruleset?.bypass_actors ?? []);
</script>

<RulesetRuleEditor
  bind:this={ruleEditor}
  scope={editorScope}
  rulesetName={name}
  rules={ruleset?.rules}
  disabled={frozen || ruleset === null}
  lookup={lookupBypassActors}
  onApply={(key, parameters) =>
    patchRules({ [key]: parameters } as Partial<SyncRulesetRules>, stored)}
/>

<div class="view-frame">
  <!-- One crumb, to the row this page sits under. Sync is where that row lives,
       not a second place to go back to. -->

  <PageHeader
    ancestors={[
      {
        label: SYNC_SECTION_LABELS.rulesets,
        href: sectionHref('rulesets'),
        onSelect: () => onOpenSection('rulesets'),
      },
    ]}
    id="sync-ruleset-heading"
    section="Ruleset"
    title={name}
    mono
    description={coverage}
  />

  {#if problem !== null}
    <FormError message={problem} />
  {/if}

  {#if ruleset !== null}
    <Card unsaved={partDirty('enforcement')}>
      <div class="policy-rows">
        <div
          class="policy-row"
          class:is-unsaved={partDirty('enforcement')}
          data-unsaved={partDirty('enforcement') || undefined}
        >
          <span class="setting-say">
            <span class="setting-name">Enforcement</span>
            <span class="setting-why">
              {#if ruleset.enforcement === 'active'}The rules hold. A push that breaks one is
                refused{:else if ruleset.enforcement === 'evaluate'}The rules are watched. Breaking
                one is recorded, never refused{:else}The rules sleep. Nothing is checked or recorded{/if}
            </span>
          </span>
          <span class="policy-value">
            <SegmentedControl
              name="ruleset-enforcement"
              label="Enforcement"
              options={[
                { value: 'active', label: 'Active' },
                { value: 'evaluate', label: 'Evaluate' },
                { value: 'disabled', label: 'Disabled' },
              ]}
              value={ruleset.enforcement}
              disabled={frozen}
              onSelect={(value) => patch({ enforcement: value })}
            />
          </span>
        </div>
      </div>
    </Card>

    <Card unsaved={partDirty('conditions')}>
      <div class="card-head">
        <h2 class="card-title">Where it applies</h2>
      </div>
      <div class="policy-rows">
        <div
          class="policy-row"
          class:is-unsaved={conditionDirty('include')}
          data-unsaved={conditionDirty('include') || undefined}
        >
          <span class="setting-say"
            ><span class="setting-name">Included branches</span>
            <span class="setting-why"
              ><span class="glob-meta">*</span> matches one segment,
              <span class="glob-meta">**</span> crosses them</span
            ></span
          >
          <span class="policy-value">
            {#each include as pattern (pattern)}
              <span class="cond-chip"
                ><span class="t"
                  >{#each globRuns(pattern) as run, at (at)}{#if run.meta}<span class="glob-meta"
                        >{run.text}</span
                      >{:else}{run.text}{/if}{/each}</span
                >
                <button
                  aria-label="Remove {pattern}"
                  disabled={frozen}
                  onclick={() => removePattern('include', pattern)}
                  ><Icon name="close" size="nano" /></button
                ></span
              >
            {/each}
            <Popover
              role="dialog"
              label="Pattern to add"
              align="end"
              bind:open={includeOpen}
              onopen={() => (addValue = '')}
            >
              {#snippet trigger(attributes)}
                <Button {...attributes} tone="add" disabled={frozen}>
                  {#snippet icon()}<Icon name="plus" size="sm" />{/snippet}Add a pattern
                </Button>
              {/snippet}
              <div class="name-menu">
                <div class="menu-search">
                  <Icon name="search" size="xs" />
                  <input
                    placeholder="releases/* or a branch name"
                    aria-label="Pattern to add"
                    spellcheck="false"
                    bind:value={addValue}
                    onkeydown={(event) => {
                      if (event.key === 'Enter') {
                        event.preventDefault();
                        addPattern('include');
                      }
                    }}
                  />
                </div>
                <div class="menu-hint">Enter adds it · Esc closes · * spans any run</div>
              </div>
            </Popover>
          </span>
        </div>
        <div
          class="policy-row"
          class:is-unsaved={conditionDirty('exclude')}
          data-unsaved={conditionDirty('exclude') || undefined}
        >
          <span class="setting-say"><span class="setting-name">Excluded branches</span></span>
          <span class="policy-value">
            {#if exclude.length === 0}
              <span class="setting-unmanaged">None</span>
            {:else}
              {#each exclude as pattern (pattern)}
                <span class="cond-chip"
                  ><span class="t"
                    >{#each globRuns(pattern) as run, at (at)}{#if run.meta}<span class="glob-meta"
                          >{run.text}</span
                        >{:else}{run.text}{/if}{/each}</span
                  >
                  <button
                    aria-label="Remove {pattern}"
                    disabled={frozen}
                    onclick={() => removePattern('exclude', pattern)}
                    ><Icon name="close" size="nano" /></button
                  ></span
                >
              {/each}
            {/if}
            <Popover
              role="dialog"
              label="Pattern to add"
              align="end"
              bind:open={excludeOpen}
              onopen={() => (addValue = '')}
            >
              {#snippet trigger(attributes)}
                <Button {...attributes} tone="add" disabled={frozen}>
                  {#snippet icon()}<Icon name="plus" size="sm" />{/snippet}Add a pattern
                </Button>
              {/snippet}
              <div class="name-menu">
                <div class="menu-search">
                  <Icon name="search" size="xs" />
                  <input
                    placeholder="releases/* or a branch name"
                    aria-label="Pattern to add"
                    spellcheck="false"
                    bind:value={addValue}
                    onkeydown={(event) => {
                      if (event.key === 'Enter') {
                        event.preventDefault();
                        addPattern('exclude');
                      }
                    }}
                  />
                </div>
                <div class="menu-hint">Enter adds it · Esc closes · * spans any run</div>
              </div>
            </Popover>
          </span>
        </div>
      </div>
    </Card>

    <Card unsaved={partDirty('rules')}>
      <div class="card-head">
        <h2 class="card-title">What it enforces</h2>
        <span class="card-meta">{onRules.length} of {RULE_CATALOGUE.length} rules on</span>
      </div>
      <div class="policy-rows">
        {#each onRules as rule (rule.key)}
          <div
            class="policy-row rule-row"
            class:rule-simple={!rule.parameterized}
            class:is-unsaved={ruleDirty(rule.key)}
            data-unsaved={ruleDirty(rule.key) || undefined}
          >
            <span class="setting-say">
              <span class="setting-name">{rule.label}</span>
              {#if rule.why !== ''}
                <span class="setting-why">{rule.why}</span>
              {/if}
            </span>
            <span class="policy-value rule-value">
              {#if rule.parameterized}
                <span class="rule-summary">
                  {#each paramChips(rule.key) as chip, at (at)}
                    <span class="param-chip"
                      >{#if chip.strong !== undefined}<strong>{chip.strong}</strong>{/if}<span
                        class="t">{chip.text}</span
                      ></span
                    >
                  {/each}
                </span>
              {/if}
              <span class="rule-actions">
                {#if rule.parameterized}
                  <Button
                    tone="quiet"
                    disabled={frozen}
                    onclick={(event) => openEditor(rule.key, event.currentTarget)}
                  >
                    Edit
                  </Button>
                {/if}
                <IconButton
                  icon="close"
                  toolbar
                  label="Switch the rule off"
                  disabled={frozen}
                  onclick={() => ruleOff(rule.key)}
                />
              </span>
            </span>
          </div>
        {/each}
      </div>
      {#if offRules.length > 0}
        <div class="group-rest rule-remainder">
          <span class="rest-say"
            ><span class="rest-count"
              >{offRules.length}
              {offRules.length === 1 ? 'rule is' : 'rules are'} off</span
            >
            - {offRules.map((rule) => rule.label).join(', ')}</span
          >
          <Popover
            bind:open={pickingRule}
            role="dialog"
            label="Rule choices"
            align="end"
            itemSelector=".btn"
          >
            {#snippet trigger(attributes)}
              <Button {...attributes} tone="add" bind:element={addRuleButton} disabled={frozen}>
                {#snippet icon()}<Icon name="plus" size="sm" />{/snippet}Add a rule
              </Button>
            {/snippet}
            <div class="addition-picker addition-menu">
              <span class="form-help">Choose a rule to add</span>
              <div class="addition-choices">
                {#each offRules as rule (rule.key)}
                  <Button tone="add" disabled={frozen} onclick={() => ruleOn(rule.key)}>
                    {#snippet icon()}<Icon name="plus" size="sm" />{/snippet}{rule.label}
                  </Button>
                {/each}
                <Button onclick={() => (pickingRule = false)}>Cancel</Button>
              </div>
            </div>
          </Popover>
        </div>
      {/if}
    </Card>

    <Card unsaved={partDirty('bypass_actors')}>
      <div class="card-head">
        <h2 class="card-title">Bypass list</h2>
        <Button
          tone="add"
          disabled={frozen}
          aria-expanded={addingActor}
          onclick={(event) => actorEditor?.toggleAdd(event.currentTarget)}
        >
          {#snippet icon()}<Icon name="plus" size="sm" />{/snippet}
          Add an actor
        </Button>
      </div>
      <BypassActorEditor
        bind:this={actorEditor}
        bind:adding={addingActor}
        showAddButton={false}
        savedActors={dirtyDocument ? (savedRuleset?.bypass_actors ?? []) : actors}
        {organizationActors}
        {actors}
        lookup={lookupBypassActors}
        readOnly={frozen}
        onChange={(next) => patch({ bypass_actors: next })}
      />
    </Card>

    <!-- The one destructive act on the page, in the row grammar every other setting
         here is written in: what it does on the left, the act on the right. -->
    {#if !readOnly}
      <Card class="danger-zone">
        <div class="card-head"><h2 class="card-title">Danger zone</h2></div>
        <div class="setting-rows">
          <div class="setting-row">
            <span class="setting-say">
              <span class="setting-name">Delete this ruleset</span>
              <span class="setting-why"
                >Removes {name} from every syncing repository on the next sync</span
              >
            </span>
            <span class="setting-value">
              <Button tone="stop-quiet" disabled={frozen} onclick={deleteRuleset}>
                Delete this ruleset
              </Button>
            </span>
          </div>
        </div>
      </Card>
    {/if}
  {:else if removed !== null}
    <!-- Deleted here, not yet on GitHub: the configuration has stopped carrying it and
         the next applied plan is what removes it, so the way back is offered until
         then rather than on the list this used to leave for. -->
    <div class="state-panel is-warn">
      <span
        ><strong>Pending removal</strong> The next sync removes {name} after this change is saved · Existing
        GitHub protection stays in place until then</span
      >
      <Button disabled={frozen} onclick={restoreRuleset}>Undo - keep this ruleset</Button>
    </div>
  {/if}
</div>

<style>
  /* The reading column is the sheet's; what is this page's own is the apply bar's seat,
     measured by the slot after it. */
  .view-frame {
    timeline-scope: --bar-slot;
  }

  /* The remainder is a summary line and not a row, so the list still seams into it. */
  .policy-rows:has(+ .group-rest) > .policy-row:last-child::after {
    content: '';
    inset-inline: var(--space-2);
  }

  /* A rule has two sides. Its Edit and off controls stay one action group
     while summaries wrap independently inside the value side. */
  .rule-value {
    flex-wrap: nowrap;
    margin-inline-start: auto;
    min-inline-size: 0;
  }

  .rule-summary,
  .rule-actions {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }

  .rule-summary {
    flex-wrap: wrap;
    min-inline-size: 0;
  }

  .rule-actions {
    flex: none;
    margin-inline-start: auto;
  }

  .rule-simple > .setting-say {
    flex: 1;
    min-inline-size: 0;
  }

  .rule-simple > .rule-value {
    flex: none;
  }

  .rule-remainder > :global(.btn) {
    margin-inline-start: auto;
  }

  @container (max-width: 32rem) {
    .rule-row:not(.rule-simple) > .rule-value {
      flex-basis: 100%;
      max-inline-size: 100%;
    }
  }

  /* ---------- Chips: a value, and a parameter said in a word ---------- */

  /* Not `.chip`: the app's own status chip owns that name globally, and a
     scoped twin still inherits its weight. */
  .cond-chip {
    align-items: center;
    background: var(--surface-inset);
    block-size: var(--tier-mark);
    border-radius: var(--r-chip);
    color: var(--text-secondary);
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    gap: 0.25rem;
    line-height: var(--leading-flat);
    padding: 0 var(--space-2);
  }

  .cond-chip .t {
    display: block;
    text-box: trim-both cap alphabetic;
  }

  /* A 20px disc folded around an 8px glyph - exactly the chip's height, so
     the hover fill never pokes past the pill. */
  .cond-chip button {
    align-items: center;
    background: none;
    border: 0;
    border-radius: 50%;
    color: inherit;
    cursor: pointer;
    display: inline-flex;
    margin: -0.375rem;
    opacity: 0.65;
    padding: 0.375rem;
  }

  .cond-chip button:hover {
    background: var(--interactive-hover-layer);
    opacity: 1;
  }

  .cond-chip button:active {
    background: var(--interactive-pressed);
  }

  .param-chip {
    align-items: center;
    background: var(--surface-inset);
    min-block-size: var(--tier-mark);
    max-inline-size: 100%;
    border-radius: var(--r-chip);
    color: var(--text-secondary);
    display: inline-flex;
    font-size: var(--font-size-micro);
    gap: 0.25rem;
    line-height: var(--leading-flat);
    padding: var(--space-1) var(--space-2);
  }

  .param-chip .t {
    display: block;
    line-height: var(--leading-tight);
    min-inline-size: 0;
    overflow-wrap: anywhere;
    text-box: trim-both cap alphabetic;
  }

  .param-chip strong {
    color: var(--text-primary);
    font-variant-numeric: tabular-nums;
    text-box: trim-both cap alphabetic;
  }

  /* ---------- The unmanaged remainder ---------- */

  .group-rest {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    margin-inline: calc(var(--space-2) * -1);
    /* Its separator is the last row's own bottom hairline, so the gaps
       around that line keep the row rhythm. */
    padding: var(--space-2) var(--space-2) 0;
    position: relative;
  }

  .rest-say {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    text-box: trim-both cap alphabetic;
  }

  .rest-count {
    color: var(--text-secondary);
    font-weight: 600;
  }

  /* ---------- The one-field popover every Add-a-... chip gets ---------- */

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

  @media (max-width: 36rem) {
    .view-frame {
      overflow-x: hidden;
    }

    .group-rest {
      align-items: stretch;
      flex-direction: column;
    }
  }
</style>
