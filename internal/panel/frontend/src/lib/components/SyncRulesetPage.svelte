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
      why: 'Nothing lands on the branch except through one',
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
      why: 'Only this ruleset decides how the ref may move',
      parameterized: true,
    },
    {
      key: 'code_scanning',
      label: 'Require code scanning',
      why: 'Named tools must have reported before merging',
      parameterized: true,
    },
  ];
</script>

<script lang="ts">
  /**
   * One ruleset's own page. Every card speaks the settings row grammar: say
   * on the left, value on the right, ghost clear at the end. A rule's
   * parameters ARE its value, so they sit in the value column as chips
   * beside Edit, and the row opens into a small staged form - fields
   * stacked left, Cancel and Done on a hairline foot.
   */
  import { numericValue } from '../merge';
  import type { SyncConfig, SyncRuleset, SyncRulesetBypassActor, SyncRulesetRules } from '../types';
  import type { SyncSection } from '../routes';

  import ApplyBar from './ApplyBar.svelte';
  import Button from './Button.svelte';
  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import PanePath from './PanePath.svelte';
  import Popover from './Popover.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import Switch from './Switch.svelte';

  const {
    config,
    name,
    readOnly,
    problem = null,
    saving,
    sectionHref,
    onOpenSection,
    onSave,
  }: {
    config: SyncConfig | null;
    /** Which ruleset the address names. */
    name: string;
    readOnly: boolean;
    problem?: string | null;
    saving: boolean;
    sectionHref: (section: SyncSection) => string;
    onOpenSection: (section: SyncSection) => void;
    onSave: (enabled: boolean, document: Record<string, unknown>) => void;
  } = $props();

  const stored = $derived(config?.document ?? {});
  const enabled = $derived(config?.enabled ?? false);
  const frozen = $derived(readOnly || config?.unreadable === true || saving || config === null);

  const rulesets = $derived(
    Array.isArray(stored.rulesets) ? (stored.rulesets as SyncRuleset[]) : [],
  );
  const ruleset = $derived(rulesets.find((held) => held.name === name) ?? null);

  /** Writes one changed ruleset back into the whole document. */
  function patch(change: Partial<SyncRuleset>): void {
    if (frozen || ruleset === null) return;
    onSave(enabled, {
      ...stored,
      rulesets: rulesets.map((held) => (held.name === name ? { ...held, ...change } : held)),
    });
  }

  function patchRules(change: Partial<SyncRulesetRules>): void {
    if (ruleset === null) return;
    const rules = { ...ruleset.rules };
    for (const [key, value] of Object.entries(change)) {
      if (value === undefined) delete rules[key as keyof SyncRulesetRules];
      else (rules as Record<string, unknown>)[key] = value;
    }
    patch({ rules });
  }

  function deleteRuleset(): void {
    if (frozen) return;
    onSave(enabled, {
      ...stored,
      rulesets: rulesets.filter((held) => held.name !== name),
    });
    onOpenSection('rulesets');
  }

  /* ---------- Where it applies ---------- */

  const include = $derived(ruleset?.conditions.include ?? []);
  const exclude = $derived(ruleset?.conditions.exclude ?? []);

  let includeOpen = $state(false);
  let excludeOpen = $state(false);
  let checksOpen = $state(false);
  let toolsOpen = $state(false);
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

  /** A rule arrives holding a usable value; its editor is right there. */
  function ruleOn(key: keyof SyncRulesetRules): void {
    pickingRule = false;
    if (key === 'pull_request') {
      patchRules({
        pull_request: {
          required_approving_review_count: 1,
          allowed_merge_methods: ['merge', 'squash', 'rebase'],
        },
      });
    } else if (key === 'required_status_checks') {
      patchRules({
        required_status_checks: {
          required_status_checks: [],
          strict_required_status_checks_policy: true,
        },
      });
      editing = 'required_status_checks';
      seedDrafts('required_status_checks');
    } else if (key === 'update') {
      patchRules({ update: {} });
    } else if (key === 'code_scanning') {
      patchRules({ code_scanning: { code_scanning_tools: [] } });
    } else {
      patchRules({ [key]: true } as Partial<SyncRulesetRules>);
    }
  }

  function ruleOff(key: keyof SyncRulesetRules): void {
    if (editing === key) editing = null;
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

  /* ---------- The staged rule editors ---------- */

  let editing = $state<keyof SyncRulesetRules | null>(null);

  let prApprovals = $state(1);
  let prStale = $state(false);
  let prOwners = $state(false);
  let prLastPush = $state(false);
  let prThreads = $state(false);
  let prMethods = $state<string[]>([]);

  let checksList = $state<string[]>([]);
  let checksStrict = $state(false);
  let checksSkipCreate = $state(false);

  let updateFetchMerge = $state(false);

  let scanTools = $state<string[]>([]);

  function seedDrafts(key: keyof SyncRulesetRules): void {
    const rules = ruleset?.rules;
    if (key === 'pull_request') {
      const rule = rules?.pull_request;
      prApprovals = numericValue(rule?.required_approving_review_count) ?? 1;
      prStale = rule?.dismiss_stale_reviews_on_push === true;
      prOwners = rule?.require_code_owner_review === true;
      prLastPush = rule?.require_last_push_approval === true;
      prThreads = rule?.required_review_thread_resolution === true;
      prMethods = [...(rule?.allowed_merge_methods ?? ['merge', 'squash', 'rebase'])];
    } else if (key === 'required_status_checks') {
      const rule = rules?.required_status_checks;
      checksList = (rule?.required_status_checks ?? []).map((check) => check.context);
      checksStrict = rule?.strict_required_status_checks_policy === true;
      checksSkipCreate = rule?.do_not_enforce_on_create === true;
    } else if (key === 'update') {
      updateFetchMerge = rules?.update?.update_allows_fetch_and_merge === true;
    } else if (key === 'code_scanning') {
      scanTools = (rules?.code_scanning?.code_scanning_tools ?? []).map((tool) => tool.tool);
    }
  }

  function openEditor(key: keyof SyncRulesetRules): void {
    if (frozen) return;
    seedDrafts(key);
    editing = key;
  }

  function saveEditor(): void {
    const key = editing;
    editing = null;
    if (key === 'pull_request') {
      patchRules({
        pull_request: {
          required_approving_review_count: Math.max(0, Math.min(10, prApprovals)),
          ...(prStale ? { dismiss_stale_reviews_on_push: true } : {}),
          ...(prOwners ? { require_code_owner_review: true } : {}),
          ...(prLastPush ? { require_last_push_approval: true } : {}),
          ...(prThreads ? { required_review_thread_resolution: true } : {}),
          allowed_merge_methods: prMethods.length > 0 ? prMethods : ['merge'],
        },
      });
    } else if (key === 'required_status_checks') {
      const kept = ruleset?.rules?.required_status_checks?.required_status_checks ?? [];
      patchRules({
        required_status_checks: {
          /* Names survive with their pinned apps: a context that was pinned
             to the App reporting it stays pinned through a rename-free edit. */
          required_status_checks: checksList.map(
            (context) => kept.find((check) => check.context === context) ?? { context },
          ),
          ...(checksStrict ? { strict_required_status_checks_policy: true } : {}),
          ...(checksSkipCreate ? { do_not_enforce_on_create: true } : {}),
        },
      });
    } else if (key === 'update') {
      patchRules({
        update: updateFetchMerge ? { update_allows_fetch_and_merge: true } : {},
      });
    } else if (key === 'code_scanning') {
      const kept = ruleset?.rules?.code_scanning?.code_scanning_tools ?? [];
      patchRules({
        code_scanning: {
          /* ponytail: thresholds default on a fresh tool; editing them waits
             for somebody to need it. */
          code_scanning_tools: scanTools.map(
            (tool) =>
              kept.find((held) => held.tool === tool) ?? {
                tool,
                alerts_threshold: 'errors',
                security_alerts_threshold: 'high_or_higher',
              },
          ),
        },
      });
    }
  }

  function toggleMethod(method: string): void {
    prMethods = prMethods.includes(method)
      ? prMethods.filter((held) => held !== method)
      : [...prMethods, method];
  }

  function addListValue(list: 'checks' | 'tools'): void {
    const value = addValue.trim();
    addValue = '';
    checksOpen = false;
    toolsOpen = false;
    if (value === '') return;
    if (list === 'checks' && !checksList.includes(value)) checksList = [...checksList, value];
    if (list === 'tools' && !scanTools.includes(value)) scanTools = [...scanTools, value];
  }

  /* ---------- Who may step around it ---------- */

  const actors = $derived(ruleset?.bypass_actors ?? []);

  function actorName(actor: SyncRulesetBypassActor): string {
    if (actor.actor_type === 'OrganizationAdmin') return 'Organization admin';
    const id = numericValue(actor.actor_id) ?? 0;
    if (actor.actor_type === 'RepositoryRole') {
      const roles: Record<number, string> = {
        5: 'Repository admin',
        4: 'Maintainers',
        2: 'Writers',
      };
      return roles[id] ?? `Repository role ${id}`;
    }
    if (actor.actor_type === 'Integration') return `App ${id}`;
    if (actor.actor_type === 'Team') return `Team ${id}`;
    return `Deploy keys`;
  }

  function actorWhy(actor: SyncRulesetBypassActor): string {
    if (actor.bypass_mode === 'pull_request') return 'Pull requests only';
    return 'Always - pushes and pull requests both';
  }

  function removeActor(at: number): void {
    patch({ bypass_actors: actors.filter((_, index) => index !== at) });
  }

  let addingActor = $state(false);
  let actorType = $state('RepositoryRole');
  let actorId = $state('5');
  let actorMode = $state('always');

  function addActor(): void {
    addingActor = false;
    const parsed = Number.parseInt(actorId, 10);
    patch({
      bypass_actors: [
        ...actors,
        {
          actor_type: actorType,
          actor_id: actorType === 'OrganizationAdmin' ? 0 : Number.isNaN(parsed) ? 0 : parsed,
          bypass_mode: actorMode,
        },
      ],
    });
  }

  const ACTOR_TYPES = [
    { value: 'RepositoryRole', label: 'Repository role' },
    { value: 'OrganizationAdmin', label: 'Organization admin' },
    { value: 'Integration', label: 'App' },
    { value: 'Team', label: 'Team' },
    { value: 'DeployKey', label: 'Deploy keys' },
  ];
</script>

<div class="view-frame">
  <PanePath
    segments={[
      { label: 'Sync', href: sectionHref('overview'), onSelect: () => onOpenSection('overview') },
      {
        label: 'Rulesets',
        href: sectionHref('rulesets'),
        onSelect: () => onOpenSection('rulesets'),
      },
    ]}
  />

  <header class="object-head">
    <h2 class="mono-title">{name}</h2>
    <p class="object-sub">
      {#if ruleset === null}
        No ruleset by this name - it may have been renamed or removed
      {:else if include.length === 1 && include[0] === '~DEFAULT_BRANCH'}
        Enforced on the default branch of every syncing repository
      {:else if include.length === 0}
        Covering no branches yet - add a pattern below
      {:else}
        Enforced on {include.join(', ')} in every syncing repository
      {/if}
    </p>
  </header>

  {#if problem !== null}
    <FormError message={problem} />
  {/if}

  {#if ruleset !== null}
    <div class="card group-card">
      <div class="policy-rows">
        <div class="policy-row">
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
    </div>

    <div class="card group-card">
      <div class="group-head">
        <h3 class="group-name">Where it applies</h3>
      </div>
      <div class="policy-rows">
        <div class="policy-row">
          <span class="setting-say"><span class="setting-name">Branches it covers</span></span>
          <span class="policy-value">
            {#each include as pattern (pattern)}
              <span class="cond-chip"
                ><span class="t">{pattern}</span>
                <button
                  aria-label="Remove {pattern}"
                  disabled={frozen}
                  onclick={() => removePattern('include', pattern)}
                  ><Icon name="close" size={8} /></button
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
                <button {...attributes} class="add-chip" disabled={frozen}>
                  <Icon name="plus" size={12} />
                  <span class="t">Add a pattern</span>
                </button>
              {/snippet}
              <div class="name-menu">
                <div class="menu-search">
                  <Icon name="search" size={12} />
                  <input
                    placeholder="releases/* or a branch name"
                    aria-label="Pattern to add"
                    spellcheck="false"
                    bind:value={addValue}
                    onkeydown={(event) => {
                      if (event.key === 'Enter') addPattern('include');
                    }}
                  />
                </div>
                <div class="menu-hint">Enter adds it · Esc closes · * spans any run</div>
              </div>
            </Popover>
          </span>
        </div>
        <div class="policy-row">
          <span class="setting-say"><span class="setting-name">Branches it leaves out</span></span>
          <span class="policy-value">
            {#if exclude.length === 0}
              <span class="setting-unmanaged">None</span>
            {:else}
              {#each exclude as pattern (pattern)}
                <span class="cond-chip"
                  ><span class="t">{pattern}</span>
                  <button
                    aria-label="Remove {pattern}"
                    disabled={frozen}
                    onclick={() => removePattern('exclude', pattern)}
                    ><Icon name="close" size={8} /></button
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
                <button {...attributes} class="add-chip" disabled={frozen}>
                  <Icon name="plus" size={12} />
                  <span class="t">Add a pattern</span>
                </button>
              {/snippet}
              <div class="name-menu">
                <div class="menu-search">
                  <Icon name="search" size={12} />
                  <input
                    placeholder="releases/* or a branch name"
                    aria-label="Pattern to add"
                    spellcheck="false"
                    bind:value={addValue}
                    onkeydown={(event) => {
                      if (event.key === 'Enter') addPattern('exclude');
                    }}
                  />
                </div>
                <div class="menu-hint">Enter adds it · Esc closes · * spans any run</div>
              </div>
            </Popover>
          </span>
        </div>
      </div>
    </div>

    <div class="card group-card">
      <div class="group-head">
        <h3 class="group-name">What it enforces</h3>
        <span class="group-tally">{onRules.length} of {RULE_CATALOGUE.length} rules on</span>
      </div>
      <div class="policy-rows">
        {#each onRules as rule (rule.key)}
          <div class="policy-row">
            <span class="setting-say">
              <span class="setting-name">{rule.label}</span>
              {#if rule.why !== ''}
                <span class="setting-why">{rule.why}</span>
              {/if}
            </span>
            <span class="policy-value">
              {#each paramChips(rule.key) as chip, at (at)}
                <span class="param-chip"
                  >{#if chip.strong !== undefined}<strong>{chip.strong}</strong>{/if}<span class="t"
                    >{chip.text}</span
                  ></span
                >
              {/each}
              {#if rule.parameterized}
                <Button tone="quiet" disabled={frozen} onclick={() => openEditor(rule.key)}>
                  Edit
                </Button>
              {/if}
            </span>
            <button
              class="setting-clear"
              title="Switch the rule off"
              disabled={frozen}
              onclick={() => ruleOff(rule.key)}
            >
              <Icon name="close" size={10} />
            </button>
            {#if editing === rule.key}
              <div class="rule-edit">
                {#if rule.key === 'pull_request'}
                  <div class="entry-field">
                    <span class="entry-label">Approvals required</span>
                    <input
                      class="text-inline num-input"
                      type="number"
                      min="0"
                      max="10"
                      bind:value={prApprovals}
                      aria-label="Approvals required"
                    />
                  </div>
                  <div class="rule-flag">
                    <span>Dismiss stale approvals when new commits arrive</span>
                    <Switch
                      checked={prStale}
                      bare
                      label="Dismiss stale approvals"
                      onToggle={(next) => (prStale = next)}
                    />
                  </div>
                  <div class="rule-flag">
                    <span>Require a code owner's review</span>
                    <Switch
                      checked={prOwners}
                      bare
                      label="Require a code owner's review"
                      onToggle={(next) => (prOwners = next)}
                    />
                  </div>
                  <div class="rule-flag">
                    <span>Require the last push to be approved by somebody else</span>
                    <Switch
                      checked={prLastPush}
                      bare
                      label="Require last push approval"
                      onToggle={(next) => (prLastPush = next)}
                    />
                  </div>
                  <div class="rule-flag">
                    <span>Require every review thread resolved</span>
                    <Switch
                      checked={prThreads}
                      bare
                      label="Require review threads resolved"
                      onToggle={(next) => (prThreads = next)}
                    />
                  </div>
                  <div class="entry-field">
                    <span class="entry-label">Ways a pull request may land</span>
                    <span class="chip-line">
                      {#each ['merge', 'squash', 'rebase'] as method (method)}
                        <button
                          class="add-chip"
                          class:is-held={prMethods.includes(method)}
                          onclick={() => toggleMethod(method)}
                        >
                          {#if prMethods.includes(method)}<Icon
                              name="check"
                              size={12}
                            />{:else}<Icon name="plus" size={12} />{/if}
                          <span class="t">{method}</span>
                        </button>
                      {/each}
                    </span>
                  </div>
                {:else if rule.key === 'required_status_checks'}
                  <div class="entry-field">
                    <span class="entry-label">Checks that must pass</span>
                    <span class="chip-line">
                      {#each checksList as context (context)}
                        <span class="cond-chip"
                          ><span class="t">{context}</span>
                          <button
                            aria-label="Remove {context}"
                            onclick={() =>
                              (checksList = checksList.filter((held) => held !== context))}
                            ><Icon name="close" size={8} /></button
                          ></span
                        >
                      {/each}
                      <Popover
                        role="dialog"
                        label="Check to add"
                        align="start"
                        bind:open={checksOpen}
                        onopen={() => (addValue = '')}
                      >
                        {#snippet trigger(attributes)}
                          <button {...attributes} class="add-chip">
                            <Icon name="plus" size={12} />
                            <span class="t">Add a check</span>
                          </button>
                        {/snippet}
                        <div class="name-menu">
                          <div class="menu-search">
                            <Icon name="search" size={12} />
                            <input
                              placeholder="test"
                              aria-label="Check to add"
                              spellcheck="false"
                              bind:value={addValue}
                              onkeydown={(event) => {
                                if (event.key === 'Enter') addListValue('checks');
                              }}
                            />
                          </div>
                          <div class="menu-hint">Enter adds it · Esc closes</div>
                        </div>
                      </Popover>
                    </span>
                  </div>
                  <div class="rule-flag">
                    <span>Count a check only from its latest run</span>
                    <Switch
                      checked={checksStrict}
                      bare
                      label="Count a check only from its latest run"
                      onToggle={(next) => (checksStrict = next)}
                    />
                  </div>
                  <div class="rule-flag">
                    <span>Skip these checks when the branch is first created</span>
                    <Switch
                      checked={checksSkipCreate}
                      bare
                      label="Skip on branch creation"
                      onToggle={(next) => (checksSkipCreate = next)}
                    />
                  </div>
                {:else if rule.key === 'update'}
                  <div class="rule-flag">
                    <span>Still allow updating by fetch and merge</span>
                    <Switch
                      checked={updateFetchMerge}
                      bare
                      label="Allow fetch and merge"
                      onToggle={(next) => (updateFetchMerge = next)}
                    />
                  </div>
                {:else if rule.key === 'code_scanning'}
                  <div class="entry-field">
                    <span class="entry-label">Tools that must have reported</span>
                    <span class="chip-line">
                      {#each scanTools as tool (tool)}
                        <span class="cond-chip"
                          ><span class="t">{tool}</span>
                          <button
                            aria-label="Remove {tool}"
                            onclick={() => (scanTools = scanTools.filter((held) => held !== tool))}
                            ><Icon name="close" size={8} /></button
                          ></span
                        >
                      {/each}
                      <Popover
                        role="dialog"
                        label="Tool to add"
                        align="start"
                        bind:open={toolsOpen}
                        onopen={() => (addValue = '')}
                      >
                        {#snippet trigger(attributes)}
                          <button {...attributes} class="add-chip">
                            <Icon name="plus" size={12} />
                            <span class="t">Add a tool</span>
                          </button>
                        {/snippet}
                        <div class="name-menu">
                          <div class="menu-search">
                            <Icon name="search" size={12} />
                            <input
                              placeholder="CodeQL"
                              aria-label="Tool to add"
                              spellcheck="false"
                              bind:value={addValue}
                              onkeydown={(event) => {
                                if (event.key === 'Enter') addListValue('tools');
                              }}
                            />
                          </div>
                          <div class="menu-hint">Enter adds it · Esc closes</div>
                        </div>
                      </Popover>
                    </span>
                  </div>
                {/if}
                <div class="rule-edit-foot">
                  <Button tone="quiet" onclick={() => (editing = null)}>Cancel</Button>
                  <Button tone="signal" onclick={saveEditor}>Done</Button>
                </div>
              </div>
            {/if}
          </div>
        {/each}
      </div>
      {#if offRules.length > 0}
        <div class="group-rest" class:is-open={pickingRule}>
          {#if pickingRule}
            <span class="rest-say"
              ><span class="rest-count">{offRules.length} rules are off</span> - pick one to switch on:</span
            >
            <span class="rest-picks">
              {#each offRules as rule (rule.key)}
                <button class="add-chip" onclick={() => ruleOn(rule.key)}>
                  <Icon name="plus" size={12} />
                  <span class="t">{rule.label}</span>
                </button>
              {/each}
              <Button tone="quiet" onclick={() => (pickingRule = false)}>Cancel</Button>
            </span>
          {:else}
            <span class="rest-say"
              ><span class="rest-count"
                >{offRules.length}
                {offRules.length === 1 ? 'rule is' : 'rules are'} off</span
              >
              - {offRules.map((rule) => rule.label).join(', ')}</span
            >
            <Button tone="quiet" disabled={frozen} onclick={() => (pickingRule = true)}>
              {#snippet icon()}<Icon name="plus" size={13} />{/snippet}
              Add a rule
            </Button>
          {/if}
        </div>
      {/if}
    </div>

    <div class="card group-card">
      <div class="group-head">
        <h3 class="group-name">Who may step around it</h3>
        <span class="group-tally">{actors.length} {actors.length === 1 ? 'actor' : 'actors'}</span>
      </div>
      {#if actors.length > 0}
        <div class="policy-rows">
          {#each actors as actor, at (at)}
            <div class="policy-row">
              <span class="setting-say">
                <span class="setting-name">{actorName(actor)}</span>
                <span class="setting-why">{actorWhy(actor)}</span>
              </span>
              <span class="policy-value"></span>
              <button
                class="setting-clear"
                title="Remove this actor"
                disabled={frozen}
                onclick={() => removeActor(at)}
              >
                <Icon name="close" size={10} />
              </button>
            </div>
          {/each}
        </div>
      {/if}
      <div class="group-rest" class:is-open={addingActor}>
        {#if addingActor}
          <div class="rule-edit actor-edit">
            <div class="entry-field">
              <span class="entry-label">Who</span>
              <span class="chip-line">
                {#each ACTOR_TYPES as kind (kind.value)}
                  <button
                    class="add-chip"
                    class:is-held={actorType === kind.value}
                    onclick={() => (actorType = kind.value)}
                  >
                    {#if actorType === kind.value}<Icon name="check" size={12} />{:else}<Icon
                        name="plus"
                        size={12}
                      />{/if}
                    <span class="t">{kind.label}</span>
                  </button>
                {/each}
              </span>
            </div>
            {#if actorType !== 'OrganizationAdmin'}
              <div class="entry-field">
                <span class="entry-label"
                  >{actorType === 'RepositoryRole'
                    ? 'Role id - 5 admin, 4 maintain, 2 write'
                    : 'Its id on GitHub'}</span
                >
                <input
                  class="text-inline num-input"
                  bind:value={actorId}
                  aria-label="Actor id"
                  spellcheck="false"
                />
              </div>
            {/if}
            <div class="rule-flag">
              <span>Only through pull requests</span>
              <Switch
                checked={actorMode === 'pull_request'}
                bare
                label="Only through pull requests"
                onToggle={(next) => (actorMode = next ? 'pull_request' : 'always')}
              />
            </div>
            <div class="rule-edit-foot">
              <Button tone="quiet" onclick={() => (addingActor = false)}>Cancel</Button>
              <Button tone="signal" onclick={addActor}>Add</Button>
            </div>
          </div>
        {:else}
          <span class="rest-say"
            >An actor here steps around every rule above, everywhere this ruleset applies</span
          >
          <Button tone="quiet" disabled={frozen} onclick={() => (addingActor = true)}>
            {#snippet icon()}<Icon name="plus" size={13} />{/snippet}
            Add an actor
          </Button>
        {/if}
      </div>
    </div>

    {#if !readOnly}
      <ApplyBar>
        <span class="apply-note"
          >Deleting removes this ruleset from every syncing repository on the next plan</span
        >
        <Button tone="stop-quiet" disabled={frozen} onclick={deleteRuleset}>
          Delete this ruleset
        </Button>
      </ApplyBar>
    {/if}
  {/if}
</div>

<style>
  .view-frame {
    box-sizing: border-box;
    inline-size: 100%;
    margin-inline: auto;
    max-width: var(--content-max);
    min-inline-size: 0;
    timeline-scope: --bar-slot;
  }

  .object-head {
    display: grid;
    gap: var(--space-2);
    margin-bottom: var(--space-4);
  }

  .mono-title {
    font-family: var(--mono);
    font-size: 1.375rem;
    letter-spacing: -0.01em;
    margin: 0;
  }

  .object-sub {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    line-height: round(1.5em, 1px);
    margin: 0;
    max-width: 64ch;
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

  .group-head {
    align-items: end;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    margin-bottom: var(--space-2);
  }

  .group-name {
    font-size: var(--font-size-title);
    font-weight: 600;
    margin: 0;
    min-block-size: 12px;
    text-box: trim-both cap alphabetic;
  }

  .group-tally {
    color: var(--text-muted);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }

  .policy-rows {
    display: grid;
  }

  .policy-row {
    align-items: center;
    display: grid;
    gap: var(--space-2) var(--space-4);
    grid-template-columns: 1fr auto auto;
    margin-inline: calc(var(--space-2) * -1);
    min-block-size: 48px;
    padding: 0.5rem var(--space-2);
    position: relative;
  }

  /* Every row owns the line under itself; the last one keeps it only when
     the off-rules remainder follows, where it doubles as that separator. */
  .policy-row::after {
    background: var(--border-subtle);
    block-size: 1px;
    bottom: 0;
    content: '';
    inset-inline: var(--space-2);
    position: absolute;
  }

  .policy-row:last-child::after {
    content: none;
  }

  .policy-rows:has(+ .group-rest) > .policy-row:last-child::after {
    content: '';
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

  .setting-unmanaged {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .policy-value {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    /* The settings page's 12px, which is the design's - the 8px here made
       every value column measure 4px per seam off the mock. */
    gap: var(--space-3);
    justify-content: end;
    justify-self: end;
  }

  .setting-clear {
    align-items: center;
    background: transparent;
    block-size: 26px;
    border: 0;
    border-radius: 50%;
    color: var(--text-muted);
    cursor: pointer;
    display: inline-flex;
    inline-size: 26px;
    justify-content: center;
    padding: 0;
  }

  .setting-clear:hover {
    background: var(--interactive-hover-layer);
    color: var(--text-primary);
  }

  .setting-clear:active {
    background: var(--interactive-pressed);
  }

  .policy-row .setting-clear {
    opacity: 0.45;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .policy-row:hover .setting-clear,
  .policy-row:focus-within .setting-clear {
    opacity: 1;
  }

  /* ---------- Chips: a value, and a parameter said in a word ---------- */

  /* Not `.chip`: the app's own status chip owns that name globally, and a
     scoped twin still inherits its weight. */
  .cond-chip {
    align-items: center;
    background: var(--surface-inset);
    block-size: 20px;
    border-radius: var(--r-chip);
    color: var(--text-secondary);
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    gap: 0.25rem;
    line-height: 1;
    padding: 0 var(--space-2);
  }

  .cond-chip .t {
    display: block;
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
    block-size: 20px;
    border-radius: var(--r-chip);
    color: var(--text-secondary);
    display: inline-flex;
    font-size: var(--font-size-micro);
    gap: 0.25rem;
    line-height: 1;
    padding: 0 var(--space-2);
  }

  .param-chip .t {
    display: block;
  }

  .param-chip strong {
    color: var(--text-primary);
    font-variant-numeric: tabular-nums;
  }

  .add-chip {
    align-items: center;
    background: var(--control-bg);
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-chip);
    color: var(--text-secondary);
    cursor: pointer;
    display: inline-flex;
    font-size: var(--font-size-compact);
    font-weight: 500;
    gap: 0.35rem;
    min-block-size: 30px;
    padding-block: 0;
    padding-inline: 0.7rem;
  }

  .add-chip:hover {
    background: var(--control-bg-hover);
    border-style: solid;
    color: var(--text-primary);
  }

  .add-chip:active {
    background: var(--control-bg-pressed);
  }

  .add-chip.is-held {
    background: var(--control-bg-pressed);
    border-style: solid;
    color: var(--text-primary);
  }

  .add-chip .t {
    text-box: trim-both cap alphabetic;
  }

  /* ---------- The row opened for editing ---------- */

  .rule-edit {
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-ctl);
    display: grid;
    gap: var(--space-3);
    grid-column: 1 / -1;
    /* Back inside the halo's overhang, aligned with the card's text edge. */
    margin-block: var(--space-1) var(--space-2);
    margin-inline: var(--space-2);
    padding: var(--space-4);
  }

  .actor-edit {
    flex: 1;
  }

  .entry-field {
    display: grid;
    gap: 0.5rem;
  }

  .entry-label {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    font-weight: 600;
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }

  .chip-line {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .rule-flag {
    align-items: center;
    color: var(--text-secondary);
    display: flex;
    font-size: var(--font-size-compact);
    gap: var(--space-3);
    justify-content: space-between;
  }

  .rule-edit-foot {
    border-top: 1px solid var(--border-subtle);
    display: flex;
    gap: var(--space-2);
    justify-content: flex-end;
    padding-top: var(--space-3);
  }

  .text-inline {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font-size: var(--font-size-control);
    min-block-size: 30px;
    padding-inline: 0.55rem;
  }

  .text-inline:focus {
    border-color: var(--focus);
    outline: 2px solid var(--focus);
    outline-offset: -1px;
  }

  .num-input {
    padding-inline-end: 0.3rem;
    text-align: center;
    width: 4.2rem;
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

  .rest-picks {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
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
    line-height: 16px;
    padding: var(--space-1) var(--space-3) var(--space-2);
  }

  .apply-note {
    color: var(--text-secondary);
    flex: 1;
    font-size: var(--font-size-meta);
    line-height: round(1.5em, 1px);
    text-box: trim-both cap alphabetic;
  }

  @media (max-width: 36rem) {
    .view-frame {
      overflow-x: hidden;
    }

    .card {
      padding: var(--space-4);
    }

    .policy-row {
      grid-template-columns: minmax(0, 1fr) auto;
    }

    .policy-row .setting-say {
      grid-column: 1;
      min-inline-size: 0;
    }

    .policy-row .policy-value {
      grid-column: 1 / -1;
      grid-row: 2;
      justify-content: start;
      justify-self: stretch;
      min-inline-size: 0;
    }

    .policy-row .setting-clear {
      grid-column: 2;
      grid-row: 1;
      opacity: 1;
    }

    .group-rest {
      align-items: stretch;
      flex-direction: column;
    }

    .rule-edit,
    .entry-field,
    .text-inline {
      max-inline-size: 100%;
      min-inline-size: 0;
    }

    .rule-edit-foot {
      flex-wrap: wrap;
    }
  }
</style>
