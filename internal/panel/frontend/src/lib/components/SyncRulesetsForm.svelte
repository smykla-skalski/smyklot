<script lang="ts">
  /**
   * What an installation expects its repositories to enforce.
   *
   * Unlike the settings beside it, a ruleset has no third state. GitHub writes
   * one by replacement - the request defines the whole object and what it does
   * not carry is not enforced - so every control here is a plain on or off, and
   * a ruleset this configuration names is owned whole.
   *
   * Nothing is sent until Save, and nothing reaches GitHub until a plan is
   * approved. What the replacement would drop is in that plan, which is the
   * point of there being one.
   */
  import { canonicalStringify } from '$lib/preferences-sync';
  import type {
    SyncRuleset,
    SyncRulesetBypassActor,
    SyncRulesetCodeScanningTool,
    SyncRulesetRules,
    SyncRulesetStatusCheck,
  } from '$lib/types';

  import SegmentedControl from './SegmentedControl.svelte';

  const {
    stored,
    enabled,
    unreadable,
    unavailable = '',
    problem = null,
    readOnly,
    saving,
    onSave,
  }: {
    stored: Record<string, unknown>;
    enabled: boolean;
    unreadable: boolean;
    /**
     * What this kind needs and the installation has not granted, or empty.
     * Saving is still allowed - configuring before granting is the ordinary
     * order - but a switch that is on while this is set changes nothing.
     */
    unavailable?: string;
    /** What went wrong saving these rulesets, which belongs beside them. */
    problem?: string | null;
    readOnly: boolean;
    saving: boolean;
    onSave: (enabled: boolean, document: Record<string, unknown>) => void;
  } = $props();

  const ON = 'on';
  const OFF = 'off';
  const SWITCH = [
    { value: ON, label: 'On' },
    { value: OFF, label: 'Off' },
  ] as const;

  const TARGETS = [
    { value: 'branch', label: 'Branches' },
    { value: 'tag', label: 'Tags' },
  ] as const;

  const ENFORCEMENTS = [
    { value: 'active', label: 'Enforced' },
    { value: 'evaluate', label: 'Reported' },
    { value: 'disabled', label: 'Off' },
  ] as const;

  const MERGE_METHODS = ['merge', 'squash', 'rebase'] as const;

  const ACTOR_TYPES = [
    'Integration',
    'OrganizationAdmin',
    'RepositoryRole',
    'Team',
    'DeployKey',
  ] as const;

  const BYPASS_MODES = ['always', 'pull_request', 'exempt'] as const;

  const ALERT_THRESHOLDS = ['none', 'errors', 'errors_and_warnings', 'all'] as const;
  const SECURITY_THRESHOLDS = [
    'none',
    'critical',
    'high_or_higher',
    'medium_or_higher',
    'all',
  ] as const;

  /** The rules that are on or off and carry nothing else. */
  const SIMPLE_RULES: readonly { key: keyof SyncRulesetRules; label: string }[] = [
    { key: 'creation', label: 'Block creation' },
    { key: 'deletion', label: 'Block deletion' },
    { key: 'non_fast_forward', label: 'Block force pushes' },
    { key: 'required_linear_history', label: 'Require linear history' },
    { key: 'required_signatures', label: 'Require signed commits' },
  ];

  const REVIEW_FLAGS: readonly { key: string; label: string }[] = [
    { key: 'require_code_owner_review', label: 'From code owners' },
    { key: 'dismiss_stale_reviews_on_push', label: 'Dismissed on push' },
    { key: 'require_last_push_approval', label: 'Covering the last push' },
    { key: 'required_review_thread_resolution', label: 'Threads resolved' },
  ];

  /* Derived from what is saved and written over as somebody edits, so a save
     landing from anywhere reseeds it rather than leaving the screen describing
     a document that is gone. */
  let drafts = $derived<SyncRuleset[]>(storedRulesets(stored));
  let removal = $derived(stored.allow_removal === true);
  let wanted = $derived(enabled);

  const disabled = $derived(saving || readOnly || unreadable);

  /* The whole document rather than the rulesets alone. Exclusions and anything
     a later version adds are stored here too, and a form that rebuilt the
     document from its own controls would drop every key it has no control for
     - which is the failure this chunk indicts the old tool for. */
  const payload = $derived({ ...stored, rulesets: drafts, allow_removal: removal });

  const saved = $derived(canonicalStringify(stored));
  const changed = $derived(wanted !== enabled || canonicalStringify(payload) !== saved);

  function storedRulesets(from: Record<string, unknown>): SyncRuleset[] {
    return Array.isArray(from.rulesets) ? (from.rulesets as SyncRuleset[]) : [];
  }

  function patch(index: number, change: Partial<SyncRuleset>): void {
    drafts = drafts.map((ruleset, at) => (at === index ? { ...ruleset, ...change } : ruleset));
  }

  function patchRules(index: number, change: Partial<SyncRulesetRules>): void {
    patch(index, { rules: { ...drafts[index].rules, ...change } });
  }

  function add(): void {
    drafts = [
      ...drafts,
      {
        name: '',
        target: 'branch',
        // The pattern that means the same thing on every repository. Naming
        // refs/heads/main protects nothing on the ones still calling it master,
        // which is the whole problem an organization-wide tool has.
        enforcement: 'active',
        conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
        rules: {},
      },
    ];
  }

  function remove(index: number): void {
    drafts = drafts.filter((_, at) => at !== index);
  }

  /** One ref or context per line, which is how somebody writes a list. */
  function lines(values: readonly string[] | undefined): string {
    return (values ?? []).join('\n');
  }

  function asList(text: string): string[] {
    return text
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line !== '');
  }

  function ruleOn(ruleset: SyncRuleset, key: keyof SyncRulesetRules): string {
    return ruleset.rules?.[key] === true ? ON : OFF;
  }

  function flagOn(rule: Record<string, unknown> | undefined, key: string): string {
    return rule?.[key] === true ? ON : OFF;
  }

  function reviewCount(ruleset: SyncRuleset): number {
    return ruleset.rules?.pull_request?.required_approving_review_count ?? 0;
  }

  function toggleMergeMethod(index: number, method: string, on: boolean): void {
    const pull = drafts[index].rules?.pull_request;
    if (pull === undefined) return;

    const kept = pull.allowed_merge_methods.filter((allowed) => allowed !== method);
    patchRules(index, {
      pull_request: {
        ...pull,
        allowed_merge_methods: on ? [...kept, method] : kept,
      },
    });
  }

  /** Turning a rule that carries parameters on gives it the smallest legal shape. */
  function togglePullRequest(index: number, on: boolean): void {
    patchRules(index, {
      pull_request: on
        ? { required_approving_review_count: 1, allowed_merge_methods: ['squash'] }
        : undefined,
    });
  }

  function toggleChecks(index: number, on: boolean): void {
    patchRules(index, {
      required_status_checks: on ? { required_status_checks: [] } : undefined,
    });
  }

  function toggleScanning(index: number, on: boolean): void {
    patchRules(index, {
      code_scanning: on ? { code_scanning_tools: [] } : undefined,
    });
  }

  function setChecks(index: number, contexts: string[]): void {
    const rule = drafts[index].rules?.required_status_checks;
    if (rule === undefined) return;

    // Matched back to what is already there so an integration somebody pinned
    // through the API survives being retyped in this box.
    const pinned = new Map(rule.required_status_checks.map((check) => [check.context, check]));
    const checks: SyncRulesetStatusCheck[] = contexts.map(
      (context) => pinned.get(context) ?? { context },
    );

    patchRules(index, { required_status_checks: { ...rule, required_status_checks: checks } });
  }

  function actors(ruleset: SyncRuleset): SyncRulesetBypassActor[] {
    return ruleset.bypass_actors ?? [];
  }

  function patchActor(index: number, at: number, change: Partial<SyncRulesetBypassActor>): void {
    patch(index, {
      bypass_actors: actors(drafts[index]).map((actor, position) =>
        position === at ? { ...actor, ...change } : actor,
      ),
    });
  }

  function addActor(index: number): void {
    patch(index, {
      bypass_actors: [
        ...actors(drafts[index]),
        { actor_id: 0, actor_type: 'OrganizationAdmin', bypass_mode: 'always' },
      ],
    });
  }

  function removeActor(index: number, at: number): void {
    patch(index, {
      bypass_actors: actors(drafts[index]).filter((_, position) => position !== at),
    });
  }

  function tools(ruleset: SyncRuleset): SyncRulesetCodeScanningTool[] {
    return ruleset.rules?.code_scanning?.code_scanning_tools ?? [];
  }

  function patchTool(
    index: number,
    at: number,
    change: Partial<SyncRulesetCodeScanningTool>,
  ): void {
    patchRules(index, {
      code_scanning: {
        code_scanning_tools: tools(drafts[index]).map((tool, position) =>
          position === at ? { ...tool, ...change } : tool,
        ),
      },
    });
  }

  function addTool(index: number): void {
    patchRules(index, {
      code_scanning: {
        code_scanning_tools: [
          ...tools(drafts[index]),
          {
            tool: 'CodeQL',
            alerts_threshold: 'errors',
            security_alerts_threshold: 'high_or_higher',
          },
        ],
      },
    });
  }

  function removeTool(index: number, at: number): void {
    patchRules(index, {
      code_scanning: {
        code_scanning_tools: tools(drafts[index]).filter((_, position) => position !== at),
      },
    });
  }

  /** A stable handle for a row nobody has named yet, so typing does not remount it. */
  function rowKey(index: number): string {
    return `ruleset-${index}`;
  }
</script>

<section class="rulesets" aria-labelledby="sync-rulesets-heading">
  <header class="rulesets-header">
    <h2 id="sync-rulesets-heading">Rulesets</h2>
    <p class="rulesets-lead">
      What every repository in this installation should enforce on its branches and tags. A ruleset
      named here is owned whole: what it does not say stops being enforced, and the plan says so
      before anything changes.
    </p>
  </header>

  {#if problem !== null}
    <p class="rulesets-error" role="alert">{problem}</p>
  {/if}

  {#if unreadable}
    <p class="rulesets-error" role="alert">
      These rulesets are stored in a form this version of Smyklot cannot read, so they are not shown
      and nothing here can be changed. Nothing has been lost.
    </p>
  {/if}

  <!-- Only while the switch is on: a kind nobody asked for is not waiting on
       anything. Bound to the switch rather than to what was saved, so somebody
       turning it on is told before they press save rather than after. -->
  {#if unavailable !== '' && wanted}
    <p class="rulesets-notice" role="status">
      {unavailable}. Nothing here will be planned or changed until an owner grants it on the
      installation's page on GitHub. The rulesets below can be saved in the meantime.
    </p>
  {/if}

  <div class="rulesets-switches">
    <label>
      <input
        type="checkbox"
        checked={wanted}
        {disabled}
        onchange={(event) => (wanted = event.currentTarget.checked)}
      />
      Keep these rulesets in step across every repository
    </label>

    <!-- The one control here that destroys something. A ruleset removed from
         this list goes on enforcing for ever unless this is on, and turning it
         on removes anything a repository has that is not named above. -->
    <label>
      <input
        type="checkbox"
        checked={removal}
        {disabled}
        onchange={(event) => (removal = event.currentTarget.checked)}
      />
      Remove rulesets this list does not name
    </label>
  </div>

  {#if drafts.length === 0}
    <p class="rulesets-empty">No rulesets yet.</p>
  {/if}

  {#each drafts as ruleset, index (rowKey(index))}
    <article class="ruleset">
      <div class="ruleset-row">
        <label class="ruleset-name">
          <span>Name</span>
          <input
            type="text"
            value={ruleset.name}
            {disabled}
            placeholder="main-branch-protection"
            onchange={(event) => patch(index, { name: event.currentTarget.value })}
          />
        </label>

        {#if !readOnly}
          <button class="btn btn-quiet" type="button" {disabled} onclick={() => remove(index)}>
            Remove
          </button>
        {/if}
      </div>

      <div class="ruleset-row">
        <span class="ruleset-label">Applies to</span>
        <span class="ruleset-spacer"></span>
        <SegmentedControl
          name="{rowKey(index)}-target"
          label="What {ruleset.name || 'this ruleset'} applies to"
          options={TARGETS}
          value={ruleset.target}
          {disabled}
          onSelect={(value) => patch(index, { target: value })}
        />
      </div>

      <div class="ruleset-row">
        <span class="ruleset-label">Enforcement</span>
        <span class="ruleset-spacer"></span>
        <SegmentedControl
          name="{rowKey(index)}-enforcement"
          label="How {ruleset.name || 'this ruleset'} is enforced"
          options={ENFORCEMENTS}
          value={ruleset.enforcement}
          {disabled}
          onSelect={(value) => patch(index, { enforcement: value })}
        />
      </div>

      <label class="ruleset-field">
        <span>Refs it covers</span>
        <textarea
          rows="2"
          {disabled}
          value={lines(ruleset.conditions?.include)}
          placeholder="refs/heads/main"
          onchange={(event) =>
            patch(index, {
              conditions: {
                ...ruleset.conditions,
                include: asList(event.currentTarget.value),
              },
            })}></textarea>
        <span class="ruleset-note">
          One pattern per line. <code>~DEFAULT_BRANCH</code> covers whatever each repository calls
          its default branch, and <code>~ALL</code> covers every ref.
        </span>
      </label>

      <label class="ruleset-field">
        <span>Refs it leaves out</span>
        <textarea
          rows="2"
          {disabled}
          value={lines(ruleset.conditions?.exclude)}
          onchange={(event) =>
            patch(index, {
              conditions: {
                ...ruleset.conditions,
                exclude: asList(event.currentTarget.value),
              },
            })}></textarea>
      </label>

      <div class="ruleset-rows">
        {#each SIMPLE_RULES as rule (rule.key)}
          <div class="ruleset-row">
            <span class="ruleset-label">{rule.label}</span>
            <span class="ruleset-spacer"></span>
            <SegmentedControl
              name="{rowKey(index)}-{rule.key}"
              label={rule.label}
              options={SWITCH}
              value={ruleOn(ruleset, rule.key)}
              {disabled}
              onSelect={(value) => patchRules(index, { [rule.key]: value === ON })}
            />
          </div>
        {/each}

        <div class="ruleset-row">
          <span class="ruleset-label">Require a pull request</span>
          <span class="ruleset-spacer"></span>
          <SegmentedControl
            name="{rowKey(index)}-pull-request"
            label="Require a pull request"
            options={SWITCH}
            value={ruleset.rules?.pull_request === undefined ? OFF : ON}
            {disabled}
            onSelect={(value) => togglePullRequest(index, value === ON)}
          />
        </div>
      </div>

      {#if ruleset.rules?.pull_request !== undefined}
        <div class="ruleset-rows ruleset-nested">
          <div class="ruleset-row">
            <span class="ruleset-label">Approving reviews</span>
            <span class="ruleset-spacer"></span>
            <input
              class="ruleset-count"
              type="number"
              min="0"
              value={reviewCount(ruleset)}
              {disabled}
              onchange={(event) =>
                patchRules(index, {
                  pull_request: {
                    ...ruleset.rules.pull_request!,
                    required_approving_review_count: Number(event.currentTarget.value),
                  },
                })}
            />
          </div>

          {#each REVIEW_FLAGS as flag (flag.key)}
            <div class="ruleset-row">
              <span class="ruleset-label">{flag.label}</span>
              <span class="ruleset-spacer"></span>
              <SegmentedControl
                name="{rowKey(index)}-{flag.key}"
                label={flag.label}
                options={SWITCH}
                value={flagOn(
                  ruleset.rules.pull_request as unknown as Record<string, unknown>,
                  flag.key,
                )}
                {disabled}
                onSelect={(value) =>
                  patchRules(index, {
                    pull_request: {
                      ...ruleset.rules.pull_request!,
                      [flag.key]: value === ON,
                    },
                  })}
              />
            </div>
          {/each}

          <!-- GitHub needs at least one, so the plan would be refused with
               none. Said here rather than only in the error a save returns. -->
          <div class="ruleset-row">
            <span class="ruleset-label">Merged by</span>
            <span class="ruleset-spacer"></span>
            <div class="ruleset-methods">
              {#each MERGE_METHODS as method (method)}
                <label>
                  <input
                    type="checkbox"
                    checked={ruleset.rules.pull_request.allowed_merge_methods.includes(method)}
                    {disabled}
                    onchange={(event) =>
                      toggleMergeMethod(index, method, event.currentTarget.checked)}
                  />
                  {method}
                </label>
              {/each}
            </div>
          </div>
        </div>
      {/if}

      <div class="ruleset-rows">
        <div class="ruleset-row">
          <span class="ruleset-label">Require status checks</span>
          <span class="ruleset-spacer"></span>
          <SegmentedControl
            name="{rowKey(index)}-checks"
            label="Require status checks"
            options={SWITCH}
            value={ruleset.rules?.required_status_checks === undefined ? OFF : ON}
            {disabled}
            onSelect={(value) => toggleChecks(index, value === ON)}
          />
        </div>
      </div>

      {#if ruleset.rules?.required_status_checks !== undefined}
        <div class="ruleset-nested">
          <label class="ruleset-field">
            <span>Checks that must pass</span>
            <textarea
              rows="2"
              {disabled}
              value={lines(
                ruleset.rules.required_status_checks.required_status_checks.map(
                  (check) => check.context,
                ),
              )}
              placeholder="build"
              onchange={(event) => setChecks(index, asList(event.currentTarget.value))}></textarea>
            <span class="ruleset-note">
              One per line. GitHub refuses a rule that names none, so a save with this empty comes
              back refused rather than quietly dropping the rule.
            </span>
          </label>

          <div class="ruleset-rows">
            <div class="ruleset-row">
              <span class="ruleset-label">Branch up to date</span>
              <span class="ruleset-spacer"></span>
              <SegmentedControl
                name="{rowKey(index)}-strict"
                label="Branch up to date"
                options={SWITCH}
                value={flagOn(
                  ruleset.rules.required_status_checks as unknown as Record<string, unknown>,
                  'strict_required_status_checks_policy',
                )}
                {disabled}
                onSelect={(value) =>
                  patchRules(index, {
                    required_status_checks: {
                      ...ruleset.rules.required_status_checks!,
                      strict_required_status_checks_policy: value === ON,
                    },
                  })}
              />
            </div>

            <div class="ruleset-row">
              <span class="ruleset-label">Not on a new branch</span>
              <span class="ruleset-spacer"></span>
              <SegmentedControl
                name="{rowKey(index)}-on-create"
                label="Not on a new branch"
                options={SWITCH}
                value={flagOn(
                  ruleset.rules.required_status_checks as unknown as Record<string, unknown>,
                  'do_not_enforce_on_create',
                )}
                {disabled}
                onSelect={(value) =>
                  patchRules(index, {
                    required_status_checks: {
                      ...ruleset.rules.required_status_checks!,
                      do_not_enforce_on_create: value === ON,
                    },
                  })}
              />
            </div>
          </div>
        </div>
      {/if}

      <div class="ruleset-rows">
        <div class="ruleset-row">
          <span class="ruleset-label">Require code scanning</span>
          <span class="ruleset-spacer"></span>
          <SegmentedControl
            name="{rowKey(index)}-scanning"
            label="Require code scanning"
            options={SWITCH}
            value={ruleset.rules?.code_scanning === undefined ? OFF : ON}
            {disabled}
            onSelect={(value) => toggleScanning(index, value === ON)}
          />
        </div>
      </div>

      {#if ruleset.rules?.code_scanning !== undefined}
        <div class="ruleset-nested">
          {#each tools(ruleset) as tool, at (at)}
            <div class="ruleset-row">
              <input
                type="text"
                value={tool.tool}
                {disabled}
                aria-label="Code scanning tool"
                onchange={(event) => patchTool(index, at, { tool: event.currentTarget.value })}
              />
              <select
                {disabled}
                aria-label="Alert threshold for {tool.tool}"
                value={tool.alerts_threshold}
                onchange={(event) =>
                  patchTool(index, at, { alerts_threshold: event.currentTarget.value })}
              >
                {#each ALERT_THRESHOLDS as threshold (threshold)}
                  <option value={threshold}>{threshold}</option>
                {/each}
              </select>
              <select
                {disabled}
                aria-label="Security alert threshold for {tool.tool}"
                value={tool.security_alerts_threshold}
                onchange={(event) =>
                  patchTool(index, at, { security_alerts_threshold: event.currentTarget.value })}
              >
                {#each SECURITY_THRESHOLDS as threshold (threshold)}
                  <option value={threshold}>{threshold}</option>
                {/each}
              </select>
              {#if !readOnly}
                <button
                  class="btn btn-quiet"
                  type="button"
                  {disabled}
                  onclick={() => removeTool(index, at)}
                >
                  Remove
                </button>
              {/if}
            </div>
          {/each}

          {#if !readOnly}
            <button class="btn btn-quiet" type="button" {disabled} onclick={() => addTool(index)}>
              Add a tool
            </button>
          {/if}
        </div>
      {/if}

      <div class="ruleset-nested">
        <span class="ruleset-label">Who may step around it</span>

        {#each actors(ruleset) as actor, at (at)}
          <div class="ruleset-row">
            <select
              {disabled}
              aria-label="Bypass actor type"
              value={actor.actor_type}
              onchange={(event) => patchActor(index, at, { actor_type: event.currentTarget.value })}
            >
              {#each ACTOR_TYPES as type (type)}
                <option value={type}>{type}</option>
              {/each}
            </select>
            <input
              class="ruleset-count"
              type="number"
              min="1"
              {disabled}
              aria-label="Bypass actor id"
              value={actor.actor_id}
              onchange={(event) =>
                patchActor(index, at, { actor_id: Number(event.currentTarget.value) })}
            />
            <select
              {disabled}
              aria-label="Bypass mode"
              value={actor.bypass_mode}
              onchange={(event) =>
                patchActor(index, at, { bypass_mode: event.currentTarget.value })}
            >
              {#each BYPASS_MODES as mode (mode)}
                <option value={mode}>{mode}</option>
              {/each}
            </select>
            {#if !readOnly}
              <button
                class="btn btn-quiet"
                type="button"
                {disabled}
                onclick={() => removeActor(index, at)}
              >
                Remove
              </button>
            {/if}
          </div>
        {/each}

        {#if !readOnly}
          <button class="btn btn-quiet" type="button" {disabled} onclick={() => addActor(index)}>
            Add an actor
          </button>
        {/if}
      </div>
    </article>
  {/each}

  {#if !readOnly}
    <div class="rulesets-actions">
      <button class="btn btn-quiet" type="button" {disabled} onclick={add}>Add a ruleset</button>
      <button
        class="btn btn-signal"
        type="button"
        disabled={disabled || !changed}
        onclick={() => onSave(wanted, payload)}
      >
        {saving ? 'Saving' : 'Save rulesets'}
      </button>
      {#if changed}
        <p class="rulesets-note">Nothing is changed on GitHub until a plan is approved.</p>
      {/if}
    </div>
  {/if}
</section>

<style>
  .rulesets {
    display: grid;
    gap: var(--space-3);
  }

  .rulesets-header {
    display: grid;
    gap: var(--space-1);
  }

  .rulesets-lead,
  .rulesets-note,
  .rulesets-empty,
  .ruleset-note {
    color: var(--text-muted);
    margin: 0;
  }

  .rulesets-error,
  .rulesets-notice {
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    color: var(--text-strong);
    margin: 0;
    padding: var(--space-2) var(--space-3);
  }

  .rulesets-switches {
    display: grid;
    gap: var(--space-1);
  }

  .ruleset {
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
  }

  .ruleset-rows {
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
  }

  .ruleset-rows > .ruleset-row + .ruleset-row {
    border-top: 1px solid var(--rule);
  }

  /* One line where there is room and two where there is not, so a narrow
     window scrolls down rather than across. */
  .ruleset-row {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    min-height: 3.25rem;
    padding: var(--space-2) 0.875rem;
  }

  .ruleset-rows > .ruleset-row {
    background: var(--strip);
  }

  .ruleset-label {
    font-size: 0.875rem;
    font-weight: 600;
  }

  .ruleset-spacer {
    flex: 1;
  }

  .ruleset-field,
  .ruleset-name {
    display: grid;
    gap: var(--space-1);
  }

  .ruleset-name {
    flex: 1;
  }

  .ruleset-nested {
    display: grid;
    gap: var(--space-2);
    /* Set in from its rule so a nested block reads as belonging to the row
       above it rather than as another rule of its own. */
    padding-inline-start: var(--space-3);
  }

  .ruleset-methods {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .ruleset-count {
    inline-size: 5rem;
  }

  .rulesets-actions {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
</style>
