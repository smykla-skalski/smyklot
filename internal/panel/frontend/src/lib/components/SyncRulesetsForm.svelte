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
  import { canonicalStringify } from '#lib/preferences-sync.js';
  import type {
    SyncRuleset,
    SyncRulesetBypassActor,
    SyncRulesetCodeScanningTool,
    SyncRulesetPullRequestRule,
    SyncRulesetRules,
    SyncRulesetStatusCheck,
  } from '#lib/types.js';

  import Button from './Button.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import SyncDocumentForm from './SyncDocumentForm.svelte';

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
    unavailable?: string;
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

  const REVIEW_FLAGS: readonly {
    key: keyof SyncRulesetPullRequestRule;
    label: string;
  }[] = [
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
  let excludes = $derived<string[]>(storedExcludes(stored));
  let wanted = $derived(enabled);

  const disabled = $derived(saving || readOnly || unreadable);

  /* The whole document rather than the parts with controls. Anything a later
     version adds is stored here too, and a form that rebuilt the document from
     its own controls would drop every key it has no control for - which is the
     failure this chunk indicts the old tool for. */
  const payload = $derived(asDocument(drafts, removal, excludes));

  /* What a save would send if nobody touched anything, rather than the stored
     document itself. The two differ on a kind nobody has configured, where the
     document is empty and this is three keys with their defaults, and comparing
     against the wrong one offers a save the moment the page loads. */
  const untouched = $derived(
    canonicalStringify(
      asDocument(storedRulesets(stored), stored.allow_removal === true, storedExcludes(stored)),
    ),
  );

  const changed = $derived(wanted !== enabled || canonicalStringify(payload) !== untouched);

  /** Named asDocument rather than document, which is a global this would hide. */
  function asDocument(
    rulesets: SyncRuleset[],
    allowRemoval: boolean,
    excluded: string[],
  ): Record<string, unknown> {
    return { ...stored, rulesets, allow_removal: allowRemoval, excludes: excluded };
  }

  function storedRulesets(from: Record<string, unknown>): SyncRuleset[] {
    return Array.isArray(from.rulesets) ? (from.rulesets as SyncRuleset[]) : [];
  }

  function storedExcludes(from: Record<string, unknown>): string[] {
    return Array.isArray(from.excludes) ? (from.excludes as string[]) : [];
  }

  /**
   * A list with one entry changed, and a list without one, both as new lists.
   *
   * Every edit on this page is one of the two, at three depths: the rulesets,
   * a ruleset's bypass actors, a code-scanning rule's tools. Written out each
   * time they were six copies of the same index arithmetic.
   *
   * New lists rather than edits in place, because the draft is compared against
   * what was saved to decide whether Save is offered, and a list mutated where
   * it stands compares equal to itself.
   */
  function patchedAt<T>(items: T[], at: number, change: Partial<T>): T[] {
    return items.map((item, index) => (index === at ? { ...item, ...change } : item));
  }

  function withoutAt<T>(items: T[], at: number): T[] {
    return items.filter((_, index) => index !== at);
  }

  function patch(index: number, change: Partial<SyncRuleset>): void {
    drafts = patchedAt(drafts, index, change);
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
    drafts = withoutAt(drafts, index);
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

  /**
   * Typed on the rule it reads rather than on a bag of unknowns, so a field
   * name that stops existing is a compile error rather than a control that
   * quietly shows Off for ever.
   */
  function flagOn<T>(rule: T | undefined, key: keyof T): string {
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

  /**
   * Restricting updates is a rule of its own rather than one of the plain
   * switches above, because it carries a parameter: whether a push that only
   * fast-forwards the ref to its own base is still allowed.
   */
  function toggleUpdate(index: number, on: boolean): void {
    patchRules(index, {
      update: on ? { update_allows_fetch_and_merge: true } : undefined,
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
    patch(index, { bypass_actors: patchedAt(actors(drafts[index]), at, change) });
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
    patch(index, { bypass_actors: withoutAt(actors(drafts[index]), at) });
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
      code_scanning: { code_scanning_tools: patchedAt(tools(drafts[index]), at, change) },
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
      code_scanning: { code_scanning_tools: withoutAt(tools(drafts[index]), at) },
    });
  }

  /** A stable handle for a row nobody has named yet, so typing does not remount it. */
  function rowKey(index: number): string {
    return `ruleset-${index}`;
  }
</script>

<!--
  Every rule below is one row asking one on-or-off question, and the words
  beside the control are the same words the control answers to. Written out nine
  times they were typed twice each and kept in step by hand, which is how a row
  comes to read one thing and announce another.

  Declared out here rather than inside the form: a snippet that is a component's
  direct child is one of its props, and this one belongs to nobody but the rows.
-->
{#snippet toggleRow(label: string, name: string, value: string, onSelect: (chosen: string) => void)}
  <div class="ruleset-row">
    <span class="ruleset-label">{label}</span>
    <span class="ruleset-spacer"></span>
    <SegmentedControl {name} {label} options={SWITCH} {value} {disabled} {onSelect} />
  </div>
{/snippet}

<SyncDocumentForm
  heading="Rulesets"
  noun="rulesets"
  lead="What every repository in this installation should enforce on its branches and tags. A
        ruleset named here is owned whole: what it does not say stops being enforced, and the plan
        says so before anything changes."
  enabled={wanted}
  {unreadable}
  {unavailable}
  {problem}
  {readOnly}
  {saving}
  {changed}
  {disabled}
  onToggle={(value) => (wanted = value)}
  onSave={() => onSave(wanted, payload)}
>
  <div class="ruleset-rows">
    <!-- The one control here that destroys something. A ruleset removed from
         this list goes on enforcing for ever unless this is on, and turning it
         on removes anything a repository has that is not named below. -->
    {@render toggleRow(
      'Remove rulesets this list does not name',
      'rulesets-removal',
      removal ? ON : OFF,
      (chosen: string) => (removal = chosen === ON),
    )}
  </div>

  <!-- The safety valve beside the switch above, and the reason it is here
       rather than only in the API: a person who can turn removal on from this
       page has to be able to protect something from this page too. -->
  <label class="ruleset-field">
    <span class="ruleset-field-label">Rulesets to leave alone</span>
    <!-- The note is a sibling rather than a child, because everything inside a
         label becomes part of the control's name and a reader would hear the
         whole paragraph before reaching the box. -->
    <textarea
      rows="2"
      {disabled}
      aria-describedby="rulesets-excludes-note"
      value={lines(excludes)}
      placeholder="hand-made-*"
      onchange={(event) => (excludes = asList(event.currentTarget.value))}></textarea>
  </label>
  <p class="ruleset-note" id="rulesets-excludes-note">
    One name or pattern per line, where <code>*</code> stands for any run of characters. These are neither
    written nor removed, whatever the list above says.
  </p>

  {#if drafts.length === 0}
    <p class="rulesets-empty">No rulesets yet.</p>
  {/if}

  {#each drafts as ruleset, index (rowKey(index))}
    <article class="ruleset">
      <div class="ruleset-row">
        <label class="ruleset-name">
          <span class="ruleset-field-label">Name</span>
          <input
            type="text"
            value={ruleset.name}
            {disabled}
            placeholder="main-branch-protection"
            onchange={(event) => patch(index, { name: event.currentTarget.value })}
          />
        </label>

        {#if !readOnly}
          <!-- Every bare word inside a button is wrapped, here and below: a button is
               a flex container, so its text sits in an anonymous box no selector can
               reach, and `text-box` on the button itself never touches it. See
               `.button-label` in `app.css`. Unwrapped, each of these sat 0.47px high. -->
          <Button tone="quiet" {disabled} onclick={() => remove(index)}>Remove</Button>
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
        <span class="ruleset-field-label">Refs it covers</span>
        <textarea
          rows="2"
          {disabled}
          aria-describedby="{rowKey(index)}-include-note"
          value={lines(ruleset.conditions?.include)}
          placeholder="refs/heads/main"
          onchange={(event) =>
            patch(index, {
              conditions: {
                ...ruleset.conditions,
                include: asList(event.currentTarget.value),
              },
            })}></textarea>
      </label>
      <p class="ruleset-note" id="{rowKey(index)}-include-note">
        One pattern per line. <code>~DEFAULT_BRANCH</code> covers whatever each repository calls its
        default branch, and <code>~ALL</code> covers every ref.
      </p>

      <label class="ruleset-field">
        <span class="ruleset-field-label">Refs it leaves out</span>
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
          {@render toggleRow(
            rule.label,
            `${rowKey(index)}-${rule.key}`,
            flagOn(ruleset.rules, rule.key),
            (chosen: string) => patchRules(index, { [rule.key]: chosen === ON }),
          )}
        {/each}

        {@render toggleRow(
          'Restrict updates',
          `${rowKey(index)}-update`,
          ruleset.rules?.update === undefined ? OFF : ON,
          (chosen: string) => toggleUpdate(index, chosen === ON),
        )}

        {#if ruleset.rules?.update !== undefined}
          {@render toggleRow(
            'Still allow fetch and merge',
            `${rowKey(index)}-fetch-and-merge`,
            flagOn(ruleset.rules.update, 'update_allows_fetch_and_merge'),
            (chosen: string) =>
              patchRules(index, { update: { update_allows_fetch_and_merge: chosen === ON } }),
          )}
        {/if}

        {@render toggleRow(
          'Require a pull request',
          `${rowKey(index)}-pull-request`,
          ruleset.rules?.pull_request === undefined ? OFF : ON,
          (chosen: string) => togglePullRequest(index, chosen === ON),
        )}
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
            {@render toggleRow(
              flag.label,
              `${rowKey(index)}-${flag.key}`,
              flagOn(ruleset.rules.pull_request, flag.key),
              (chosen: string) =>
                patchRules(index, {
                  pull_request: { ...ruleset.rules.pull_request!, [flag.key]: chosen === ON },
                }),
            )}
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
        {@render toggleRow(
          'Require status checks',
          `${rowKey(index)}-checks`,
          ruleset.rules?.required_status_checks === undefined ? OFF : ON,
          (chosen: string) => toggleChecks(index, chosen === ON),
        )}
      </div>

      {#if ruleset.rules?.required_status_checks !== undefined}
        <div class="ruleset-nested">
          <label class="ruleset-field">
            <span class="ruleset-field-label">Checks that must pass</span>
            <textarea
              rows="2"
              {disabled}
              value={lines(
                ruleset.rules.required_status_checks.required_status_checks.map(
                  (check) => check.context,
                ),
              )}
              placeholder="build"
              aria-describedby="{rowKey(index)}-checks-note"
              onchange={(event) => setChecks(index, asList(event.currentTarget.value))}></textarea>
          </label>
          <p class="ruleset-note" id="{rowKey(index)}-checks-note">
            One per line. GitHub refuses a rule that names none, so a save with this empty comes
            back refused rather than quietly dropping the rule.
          </p>

          <div class="ruleset-rows">
            {@render toggleRow(
              'Branch up to date',
              `${rowKey(index)}-strict`,
              flagOn(ruleset.rules.required_status_checks, 'strict_required_status_checks_policy'),
              (chosen: string) =>
                patchRules(index, {
                  required_status_checks: {
                    ...ruleset.rules.required_status_checks!,
                    strict_required_status_checks_policy: chosen === ON,
                  },
                }),
            )}

            {@render toggleRow(
              'Not on a new branch',
              `${rowKey(index)}-on-create`,
              flagOn(ruleset.rules.required_status_checks, 'do_not_enforce_on_create'),
              (chosen: string) =>
                patchRules(index, {
                  required_status_checks: {
                    ...ruleset.rules.required_status_checks!,
                    do_not_enforce_on_create: chosen === ON,
                  },
                }),
            )}
          </div>
        </div>
      {/if}

      <div class="ruleset-rows">
        {@render toggleRow(
          'Require code scanning',
          `${rowKey(index)}-scanning`,
          ruleset.rules?.code_scanning === undefined ? OFF : ON,
          (chosen: string) => toggleScanning(index, chosen === ON),
        )}
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
                <Button tone="quiet" {disabled} onclick={() => removeTool(index, at)}>
                  Remove
                </Button>
              {/if}
            </div>
          {/each}

          {#if !readOnly}
            <Button tone="quiet" {disabled} onclick={() => addTool(index)}>Add a tool</Button>
          {/if}
        </div>
      {/if}

      <!-- A property of the ruleset rather than of the rule above it, so it is
           set off with a heading of its own. Run on, it read as one more
           option of whatever rule happened to come last. -->
      <div class="ruleset-section">
        <h4 class="ruleset-eyebrow">Who may step around it</h4>

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
              <Button tone="quiet" {disabled} onclick={() => removeActor(index, at)}>Remove</Button>
            {/if}
          </div>
        {/each}

        {#if !readOnly}
          <Button tone="quiet" {disabled} onclick={() => addActor(index)}>Add an actor</Button>
        {/if}
      </div>
    </article>
  {/each}

  {#snippet actions()}
    <Button tone="quiet" {disabled} onclick={add}>Add a ruleset</Button>
  {/snippet}
</SyncDocumentForm>

<style>
  /* The settings form's group eyebrow, one level down: a ruleset is several
     things at once, and without a name over each block they run together into
     one long list of switches. `ConfigEditor` is where these numbers live. */
  .ruleset-eyebrow {
    color: var(--brand-action);
    font-size: var(--font-size-micro);
    font-weight: 700;
    letter-spacing: 0.1em;
    margin: 0 0 0.375rem;
    text-transform: uppercase;
  }

  .ruleset-section {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }

  /* A button is a grid child here and would otherwise stretch the width of the
     card. Only the buttons: the rows beside them are meant to run the full
     width, which is what puts every control at the same right edge. */
  .ruleset-section > :global(.btn),
  .ruleset-nested > :global(.btn) {
    justify-self: start;
  }

  .ruleset-field-label {
    font-size: 0.875rem;
    font-weight: 600;
  }

  .rulesets-empty,
  .ruleset-note {
    color: var(--dim);
    font-size: var(--font-size-meta);
    margin: 0;
    max-width: 60ch;
  }

  /* One box per ruleset and none inside it, which is the settings form's rule
     one level down: there the plate is the box, and here the ruleset is. A
     list that is genuinely several things does need them told apart. */
  .ruleset {
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-3);
    padding: var(--space-3);
  }

  .ruleset-rows > .ruleset-row + .ruleset-row {
    border-top: 1px solid var(--rule);
  }

  /* One line where there is room and two where there is not, so a narrow
     window scrolls down rather than across. Written to the settings form's
     numbers, because the two sit on the same page. */
  .ruleset-row {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    padding-block: 0.7rem;
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

  /* Set in from its rule, and ruled down the side, so a nested block reads as
     belonging to the row above it rather than as another rule of its own.
     Indentation alone was not enough to see at this row height. */
  .ruleset-nested {
    border-inline-start: 1px solid var(--rule);
    display: grid;
    gap: var(--space-2);
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
</style>
