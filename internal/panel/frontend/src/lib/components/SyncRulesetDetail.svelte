<script module lang="ts">
  import type { SyncRulesetRules } from '#lib/types.js';

  /** Every rule this panel can express, in the words a reader recognises. */
  const RULES: readonly {
    key: keyof SyncRulesetRules;
    label: string;
    why?: string;
  }[] = [
    {
      key: 'pull_request',
      label: 'Require a pull request',
      why: 'Nothing lands on the ref except through one',
    },
    {
      key: 'required_status_checks',
      label: 'Require status checks',
      why: 'Named checks must pass before merging',
    },
    { key: 'required_signatures', label: 'Require signed commits' },
    { key: 'required_linear_history', label: 'Require linear history' },
    { key: 'non_fast_forward', label: 'Block force pushes' },
    { key: 'deletion', label: 'Block deletion' },
    { key: 'creation', label: 'Block creation' },
    { key: 'update', label: 'Restrict updates', why: 'A push that only moves the ref forward' },
    {
      key: 'code_scanning',
      label: 'Require code scanning',
      why: 'A tool must have run, and its alerts must be under a threshold',
    },
  ];

  /** What a bypass actor is called, since GitHub answers with a type and a number. */
  function actorName(type: string, id: number): string {
    switch (type) {
      case 'OrganizationAdmin':
        // The one actor with no id at all: GitHub answers this one without one,
        // and a comparison that read the number never settled.
        return 'Organization admin';
      case 'RepositoryRole':
        return `Repository role ${id}`;
      case 'Team':
        return `Team ${id}`;
      case 'Integration':
        return `App ${id}`;
      case 'DeployKey':
        return 'Deploy key';
      default:
        return `${type} ${id}`;
    }
  }

  /** What a bypass mode lets somebody past, said as a sentence. */
  function actorWhy(mode: string): string {
    switch (mode) {
      case 'always':
        return 'Always — pushes and pull requests both';
      case 'pull_request':
        return 'Pull requests only';
      default:
        return 'Exempt from this ruleset';
    }
  }
</script>

<script lang="ts">
  /**
   * One ruleset, and everything it is.
   *
   * The same policy-first language as the settings page: only what is on is a
   * row, each carrying its own parameters as quiet chips, and what is off is
   * one sentence naming it. A ruleset has nine rules this panel can express and
   * most have three; unfolded, all nine were switches with their parameters
   * nested under them, and the page was long enough that nobody read it before
   * pressing Save.
   *
   * Enforcement stays radio cards: three modes, one expensive wrong pick, and
   * the difference between Active and Evaluate cannot be worn on a segment -
   * Evaluate is a ruleset that looks enforced and enforces nothing.
   *
   * Everything lands at once. Nothing reaches GitHub until a plan is approved,
   * so there is nothing for a Save button to hold back.
   */
  import { patchedAt, storedList, withoutAt } from '#lib/form-lists.js';
  // `SyncRulesetRules` comes from the module script above: importing it twice in
  // one file is two declarations of the same name.
  import type {
    SyncRuleset,
    SyncRulesetBypassActor,
    SyncRulesetCodeScanningTool,
    SyncRulesetStatusCheck,
  } from '#lib/types.js';

  import Button from './Button.svelte';
  import Chip from './Chip.svelte';
  import ChoiceCards from './ChoiceCards.svelte';
  import BackLink from './BackLink.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import PatternList from './PatternList.svelte';
  import Plate from './Plate.svelte';
  import PolicyGroup from './PolicyGroup.svelte';
  import PolicyRow from './PolicyRow.svelte';
  import Select from './Select.svelte';
  import Switch from './Switch.svelte';
  import { ENFORCEMENTS } from './SyncRulesetsForm.svelte';

  const {
    stored,
    name,
    listHref,
    readOnly,
    saving,
    unreadable,
    problem = null,
    onSave,
  }: {
    stored: Record<string, unknown>;
    /** Which ruleset this page is, by the only thing that keys one: its name. */
    name: string;
    listHref: string;
    readOnly: boolean;
    saving: boolean;
    unreadable: boolean;
    problem?: string | null;
    onSave: (document: Record<string, unknown>) => void;
  } = $props();

  const disabled = $derived(saving || readOnly || unreadable);

  const rulesets = $derived(storedList<SyncRuleset>(stored, 'rulesets'));
  const at = $derived(rulesets.findIndex((candidate) => candidate.name === name));
  const ruleset = $derived(at === -1 ? undefined : rulesets[at]);
  const rules = $derived<SyncRulesetRules>(ruleset?.rules ?? {});
  const actors = $derived<SyncRulesetBypassActor[]>(ruleset?.bypass_actors ?? []);

  /** Which rule is open for editing, or which actor - one at a time, like a menu. */
  let editing = $state<string | null>(null);
  let adding = $state(false);

  function patch(change: Partial<SyncRuleset>): void {
    if (at === -1) return;
    onSave({ ...stored, rulesets: patchedAt(rulesets, at, change) });
  }

  function patchRules(change: Partial<SyncRulesetRules>): void {
    patch({ rules: { ...rules, ...change } });
  }

  /**
   * Removes one bypass actor, keeping the open editor on the row it was opened
   * on.
   *
   * `editing` holds `actor:${index}`, so removing a row ABOVE an open one left
   * the editor attached to whatever moved up into its place - the reader is
   * then editing a row they did not open, and with two actors it vanishes
   * instead. Nothing is corrupted, because every write goes through
   * `patchActor(index, …)` with the render-time index; the row under the open
   * form is simply not the row they pressed Edit on.
   */
  function removeActor(index: number): void {
    const open = editing?.startsWith('actor:') === true ? Number(editing.slice(6)) : null;
    if (open === index) editing = null;
    else if (open !== null && index < open) editing = `actor:${open - 1}`;
    patch({ bypass_actors: withoutAt(actors, index) });
  }

  function patchPullRequest(change: Record<string, unknown>): void {
    const pull = rules.pull_request;
    if (pull === undefined) return;
    patchRules({ pull_request: { ...pull, ...change } });
  }

  function patchChecks(change: Record<string, unknown>): void {
    const checks = rules.required_status_checks;
    if (checks === undefined) return;
    patchRules({ required_status_checks: { ...checks, ...change } });
  }

  function remove(): void {
    if (at === -1) return;
    onSave({ ...stored, rulesets: withoutAt(rulesets, at) });
  }

  /** A rule carrying parameters is on when it is there at all. */
  function isOn(key: keyof SyncRulesetRules): boolean {
    const rule = rules[key];

    return typeof rule === 'boolean' ? rule : rule !== undefined;
  }

  /** Turning one on gives it the smallest shape GitHub will accept. */
  function switchOn(key: keyof SyncRulesetRules): void {
    // Here rather than at the press: turning a rule on is what closes the
    // picker, wherever the press came from.
    adding = false;
    editing = key;
    switch (key) {
      case 'pull_request':
        patchRules({
          pull_request: { required_approving_review_count: 1, allowed_merge_methods: ['squash'] },
        });
        break;
      case 'required_status_checks':
        patchRules({ required_status_checks: { required_status_checks: [] } });
        break;
      case 'code_scanning':
        patchRules({ code_scanning: { code_scanning_tools: [] } });
        break;
      case 'update':
        patchRules({ update: { update_allows_fetch_and_merge: true } });
        break;
      default:
        patchRules({ [key]: true });
    }
  }

  /**
   * Switching a rule off removes it rather than storing false: GitHub writes a
   * ruleset by replacement, so a rule the document does not carry is a rule
   * that is not enforced, and `false` is a longer way of saying the same thing
   * that a later reader has to work out.
   */
  function switchOff(key: keyof SyncRulesetRules): void {
    if (editing === key) editing = null;
    const rest = { ...rules };
    delete rest[key];
    patch({ rules: rest });
  }

  const MERGE_METHODS = ['merge', 'squash', 'rebase'] as const;
  const ACTOR_TYPES = [
    { value: 'OrganizationAdmin', label: 'Organization admin' },
    { value: 'RepositoryRole', label: 'Repository role' },
    { value: 'Team', label: 'Team' },
    { value: 'Integration', label: 'App' },
    { value: 'DeployKey', label: 'Deploy key' },
  ];
  const BYPASS_MODES = [
    { value: 'always', label: 'Always' },
    { value: 'pull_request', label: 'Pull requests only' },
    { value: 'exempt', label: 'Exempt' },
  ];
  const ALERT_THRESHOLDS = ['none', 'errors', 'errors_and_warnings', 'all'];
  const SECURITY_THRESHOLDS = ['none', 'critical', 'high_or_higher', 'medium_or_higher', 'all'];

  const checks = $derived<SyncRulesetStatusCheck[]>(
    rules.required_status_checks?.required_status_checks ?? [],
  );
  const tools = $derived<SyncRulesetCodeScanningTool[]>(
    rules.code_scanning?.code_scanning_tools ?? [],
  );

  /**
   * Matched back to what is already there, so an integration somebody pinned
   * through the API survives the context being retyped here.
   */
  function setChecks(contexts: string[]): void {
    const pinned = new Map(checks.map((check) => [check.context, check]));
    patchChecks({
      required_status_checks: contexts.map((context) => pinned.get(context) ?? { context }),
    });
  }

  function toggleMergeMethod(method: string, on: boolean): void {
    const kept = (rules.pull_request?.allowed_merge_methods ?? []).filter(
      (allowed) => allowed !== method,
    );
    patchPullRequest({ allowed_merge_methods: on ? [...kept, method] : kept });
  }

  function patchActor(index: number, change: Partial<SyncRulesetBypassActor>): void {
    patch({ bypass_actors: patchedAt(actors, index, change) });
  }

  /**
   * The rule's tool list, written whole. `code_scanning` carries nothing else,
   * so the whole rule is the list and every write here replaces it.
   */
  function setTools(next: SyncRulesetCodeScanningTool[]): void {
    patchRules({ code_scanning: { code_scanning_tools: next } });
  }

  function patchTool(index: number, change: Partial<SyncRulesetCodeScanningTool>): void {
    setTools(patchedAt(tools, index, change));
  }

  function addActor(): void {
    editing = `actor:${actors.length}`;
    patch({
      bypass_actors: [
        ...actors,
        { actor_id: 0, actor_type: 'OrganizationAdmin', bypass_mode: 'always' },
      ],
    });
  }

  const on = $derived(RULES.filter((rule) => isOn(rule.key)));
  const off = $derived(RULES.filter((rule) => !isOn(rule.key)));

  const where = $derived.by(() => {
    const include = ruleset?.conditions?.include ?? [];
    if (include.length === 0) return 'No refs named yet, so this ruleset covers nothing';
    if (include.includes('~ALL')) return 'Enforced on every ref of every syncing repository';
    if (include.includes('~DEFAULT_BRANCH') && include.length === 1) {
      return 'Enforced on the default branch of every syncing repository';
    }

    return `Enforced on ${include.join(', ')} in every syncing repository`;
  });
</script>

<BackLink href={listHref} label="Rulesets" tone="quiet" />

{#if ruleset === undefined}
  <PageHeader
    id="ruleset-heading"
    title={name}
    description="No ruleset here is called that. It may have been renamed or removed since this address was written down"
  />
{:else}
  <PageHeader id="ruleset-heading" title={name} description={where} />

  {#if problem !== null}
    <p class="ruleset-problem" role="alert">{problem}</p>
  {/if}

  <div class="ruleset-page">
    <Plate label="Enforcement">
      {#snippet status()}
        <Chip
          tone={ruleset.enforcement === 'active'
            ? 'signal'
            : ruleset.enforcement === 'evaluate'
              ? 'warning'
              : 'absent'}
          small
        >
          {ENFORCEMENTS.find((option) => option.value === ruleset.enforcement)?.title ??
            ruleset.enforcement}
        </Chip>
      {/snippet}

      <ChoiceCards
        name="ruleset-enforcement"
        label="Enforcement"
        options={ENFORCEMENTS}
        value={ruleset.enforcement}
        {disabled}
        onSelect={(next) => patch({ enforcement: next })}
      />
    </Plate>

    <PolicyGroup name="Where it applies">
      <PolicyRow name="Applies to" value={ruleset.target === 'tag' ? 'Tags' : 'Branches'}>
        {#snippet control()}
          <Select
            aria-label="What this ruleset applies to"
            options={[
              { value: 'branch', label: 'Branches' },
              { value: 'tag', label: 'Tags' },
            ]}
            value={ruleset.target}
            {disabled}
            onchange={(event) => patch({ target: event.currentTarget.value })}
          />
        {/snippet}
      </PolicyRow>

      <PolicyRow
        name="Refs it covers"
        why="~DEFAULT_BRANCH covers whatever each repository calls its default branch, and ~ALL covers every ref"
      >
        {#snippet control()}
          <PatternList
            values={ruleset.conditions?.include ?? []}
            label="Refs it covers"
            addLabel="Add a pattern"
            placeholder="~DEFAULT_BRANCH"
            empty="None — this ruleset covers nothing"
            {disabled}
            onChange={(next) => patch({ conditions: { ...ruleset.conditions, include: next } })}
          />
        {/snippet}
      </PolicyRow>

      <PolicyRow name="Refs it leaves out">
        {#snippet control()}
          <PatternList
            values={ruleset.conditions?.exclude ?? []}
            label="Refs it leaves out"
            addLabel="Add a pattern"
            {disabled}
            onChange={(next) => patch({ conditions: { ...ruleset.conditions, exclude: next } })}
          />
        {/snippet}
      </PolicyRow>
    </PolicyGroup>

    <PolicyGroup
      name="What it enforces"
      managed={on.length}
      total={RULES.length}
      tallyWord="rules on"
      restSay="{off.length} {off.length === 1 ? 'rule is' : 'rules are'} off"
      unmanaged={off}
      picking={adding}
      {disabled}
      onManage={disabled ? undefined : () => (adding = true)}
      onPick={switchOn}
      onCancel={() => (adding = false)}
    >
      {#each on as rule (rule.key)}
        <PolicyRow
          name={rule.label}
          why={rule.why}
          isOpen={editing === rule.key}
          clearLabel="Switch off"
          onStopManaging={readOnly ? undefined : () => switchOff(rule.key)}
        >
          {#snippet params()}
            {#if rule.key === 'pull_request' && rules.pull_request !== undefined}
              {@const pull = rules.pull_request}
              <Chip tone="neutral" small>
                {pull.required_approving_review_count ?? 0}
                {(pull.required_approving_review_count ?? 0) === 1 ? 'approval' : 'approvals'}
              </Chip>
              {#if pull.require_code_owner_review}
                <Chip tone="neutral" small>from code owners</Chip>
              {/if}
              {#if pull.dismiss_stale_reviews_on_push}
                <Chip tone="neutral" small>stale approvals dismissed</Chip>
              {/if}
              {#if pull.require_last_push_approval}
                <Chip tone="neutral" small>covering the last push</Chip>
              {/if}
              {#if pull.required_review_thread_resolution}
                <Chip tone="neutral" small>threads resolved</Chip>
              {/if}
              <Chip tone="neutral" small>merged by {pull.allowed_merge_methods.join(', ')}</Chip>
            {:else if rule.key === 'required_status_checks'}
              {#each checks as check (check.context)}
                <Chip tone="neutral" small>{check.context}</Chip>
              {:else}
                <!-- GitHub refuses a rule that names none, so this is the one
                       parameter whose absence is a refusal waiting to happen. -->
                <Chip tone="stop" small>no check named</Chip>
              {/each}
              {#if rules.required_status_checks?.strict_required_status_checks_policy}
                <Chip tone="neutral" small>branch up to date</Chip>
              {/if}
            {:else if rule.key === 'code_scanning'}
              {#each tools as tool (tool.tool)}
                <Chip tone="neutral" small>{tool.tool} · {tool.alerts_threshold}</Chip>
              {:else}
                <Chip tone="stop" small>no tool named</Chip>
              {/each}
            {:else if rule.key === 'update' && rules.update?.update_allows_fetch_and_merge}
              <Chip tone="neutral" small>fetch and merge still allowed</Chip>
            {/if}
          {/snippet}

          {#snippet control()}
            {#if rule.key === 'pull_request' || rule.key === 'required_status_checks' || rule.key === 'code_scanning' || rule.key === 'update'}
              <Button
                tone="quiet"
                {disabled}
                onclick={() => (editing = editing === rule.key ? null : rule.key)}
              >
                {editing === rule.key ? 'Done' : 'Edit'}
              </Button>
            {/if}
          {/snippet}

          {#snippet open()}
            <!-- No `editing === rule.key` here: `PolicyRow` renders this snippet
                 only while it is open, which is what `isOpen` above says. -->
            {#if rule.key === 'pull_request' && rules.pull_request !== undefined}
              {@const pull = rules.pull_request}
              <label class="rule-field">
                <span>Approving reviews</span>
                <input
                  class="text-input rule-count"
                  type="number"
                  min="0"
                  value={pull.required_approving_review_count ?? 0}
                  {disabled}
                  onchange={(event) =>
                    patchPullRequest({
                      required_approving_review_count: Number(event.currentTarget.value),
                    })}
                />
              </label>
              {#each [{ key: 'require_code_owner_review', label: 'From code owners' }, { key: 'dismiss_stale_reviews_on_push', label: 'Dismissed on push' }, { key: 'require_last_push_approval', label: 'Covering the last push' }, { key: 'required_review_thread_resolution', label: 'Threads resolved' }] as flag (flag.key)}
                <div class="rule-field">
                  <span>{flag.label}</span>
                  <Switch
                    checked={pull[flag.key as keyof typeof pull] === true}
                    ariaLabel={flag.label}
                    {disabled}
                    onChange={(next) => patchPullRequest({ [flag.key]: next })}
                  />
                </div>
              {/each}
              <div class="rule-field">
                <span>Merged by</span>
                <!-- GitHub needs at least one, so a ruleset with none comes
                       back refused rather than quietly losing the rule. -->
                <span class="rule-methods">
                  {#each MERGE_METHODS as method (method)}
                    <label>
                      <input
                        type="checkbox"
                        checked={pull.allowed_merge_methods.includes(method)}
                        {disabled}
                        onchange={(event) => toggleMergeMethod(method, event.currentTarget.checked)}
                      />
                      {method}
                    </label>
                  {/each}
                </span>
              </div>
            {:else if rule.key === 'required_status_checks'}
              <div class="rule-field">
                <span>Checks that must pass</span>
                <PatternList
                  values={checks.map((check) => check.context)}
                  label="Checks that must pass"
                  addLabel="Add a check"
                  placeholder="test"
                  empty="None — GitHub refuses a rule that names none"
                  {disabled}
                  onChange={setChecks}
                />
              </div>
              <div class="rule-field">
                <span>Branch up to date with its base</span>
                <Switch
                  checked={rules.required_status_checks?.strict_required_status_checks_policy ===
                    true}
                  ariaLabel="Branch up to date with its base"
                  {disabled}
                  onChange={(next) => patchChecks({ strict_required_status_checks_policy: next })}
                />
              </div>
              <div class="rule-field">
                <span>Not enforced on a new branch</span>
                <Switch
                  checked={rules.required_status_checks?.do_not_enforce_on_create === true}
                  ariaLabel="Not enforced on a new branch"
                  {disabled}
                  onChange={(next) => patchChecks({ do_not_enforce_on_create: next })}
                />
              </div>
            {:else if rule.key === 'code_scanning'}
              {#each tools as tool, index (index)}
                <div class="rule-tool">
                  <input
                    class="text-input"
                    type="text"
                    value={tool.tool}
                    aria-label="Code scanning tool"
                    {disabled}
                    onchange={(event) => patchTool(index, { tool: event.currentTarget.value })}
                  />
                  <Select
                    aria-label="Alert threshold for {tool.tool}"
                    options={ALERT_THRESHOLDS.map((value) => ({ value, label: value }))}
                    value={tool.alerts_threshold}
                    {disabled}
                    onchange={(event) =>
                      patchTool(index, { alerts_threshold: event.currentTarget.value })}
                  />
                  <Select
                    aria-label="Security alert threshold for {tool.tool}"
                    options={SECURITY_THRESHOLDS.map((value) => ({ value, label: value }))}
                    value={tool.security_alerts_threshold}
                    {disabled}
                    onchange={(event) =>
                      patchTool(index, { security_alerts_threshold: event.currentTarget.value })}
                  />
                  <Button tone="quiet" {disabled} onclick={() => setTools(withoutAt(tools, index))}>
                    Remove
                  </Button>
                </div>
              {/each}
              <Button
                tone="quiet"
                {disabled}
                onclick={() =>
                  setTools([
                    ...tools,
                    {
                      tool: 'CodeQL',
                      alerts_threshold: 'errors',
                      security_alerts_threshold: 'high_or_higher',
                    },
                  ])}
              >
                Add a tool
              </Button>
            {:else if rule.key === 'update'}
              <div class="rule-field">
                <span>Still allow fetch and merge</span>
                <Switch
                  checked={rules.update?.update_allows_fetch_and_merge === true}
                  ariaLabel="Still allow fetch and merge"
                  {disabled}
                  onChange={(next) =>
                    patchRules({ update: { update_allows_fetch_and_merge: next } })}
                />
              </div>
            {/if}
          {/snippet}
        </PolicyRow>
      {/each}
    </PolicyGroup>

    <Plate label="Who may step around it">
      {#snippet status()}
        {#if !readOnly}
          <button type="button" class="add-chip" {disabled} onclick={addActor}>
            <Icon name="plus" size={11} strokeWidth={2} />
            <span class="cap-trim">Add an actor</span>
          </button>
        {/if}
      {/snippet}

      <p class="actors-note">
        An actor here steps around every rule above, everywhere this ruleset applies
      </p>

      <!-- `.ruled-rows` draws the seam inset from the edges and clears it under
           the pointer, which a `border-top` cannot - and these rows have a
           hover ground for the line to be drawn across. -->
      <div class="actor-rows ruled-rows">
        {#each actors as actor, index (index)}
          <PolicyRow
            name={actorName(actor.actor_type, actor.actor_id)}
            why={actorWhy(actor.bypass_mode)}
            isOpen={editing === `actor:${index}`}
            clearLabel="Remove"
            onStopManaging={readOnly ? undefined : () => removeActor(index)}
          >
            {#snippet control()}
              <Button
                tone="quiet"
                {disabled}
                onclick={() => (editing = editing === `actor:${index}` ? null : `actor:${index}`)}
              >
                {editing === `actor:${index}` ? 'Done' : 'Edit'}
              </Button>
            {/snippet}

            {#snippet open()}
              {#if editing === `actor:${index}`}
                <div class="rule-field">
                  <span>Who</span>
                  <Select
                    aria-label="Kind of actor"
                    options={ACTOR_TYPES}
                    value={actor.actor_type}
                    {disabled}
                    onchange={(event) =>
                      patchActor(index, { actor_type: event.currentTarget.value })}
                  />
                </div>
                {#if actor.actor_type !== 'OrganizationAdmin'}
                  <!-- Every actor but this one is keyed by a number. GitHub
                       answers an organization admin without one at all. -->
                  <label class="rule-field">
                    <span>Its id on GitHub</span>
                    <input
                      class="text-input rule-count"
                      type="number"
                      min="1"
                      value={actor.actor_id}
                      {disabled}
                      onchange={(event) =>
                        patchActor(index, { actor_id: Number(event.currentTarget.value) })}
                    />
                  </label>
                {/if}
                <div class="rule-field">
                  <span>Past what</span>
                  <Select
                    aria-label="What this actor may step around"
                    options={BYPASS_MODES}
                    value={actor.bypass_mode}
                    {disabled}
                    onchange={(event) =>
                      patchActor(index, { bypass_mode: event.currentTarget.value })}
                  />
                </div>
              {/if}
            {/snippet}
          </PolicyRow>
        {:else}
          <p class="empty-note actors-empty">Nobody. Every rule above holds for everybody</p>
        {/each}
      </div>
    </Plate>

    {#if !readOnly}
      <div class="ruleset-foot">
        <span class="ruleset-foot-say">
          Deleting removes this ruleset from every syncing repository on the next plan
        </span>
        <Button tone="stop-quiet" {disabled} onclick={remove}>Delete this ruleset</Button>
      </div>
    {/if}
  </div>
{/if}

<style>
  .ruleset-page {
    display: grid;
    gap: var(--space-4);
    margin-top: var(--space-4);
  }

  .ruleset-problem {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    font-size: var(--font-size-meta);
    margin: var(--space-3) 0 0;
    padding: var(--space-2) var(--space-3);
  }

  /* Label on the left, control on the right, at every depth: the edit block's
     rows read as the row they belong to, one step in. */
  .rule-field {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    font-size: var(--font-size-compact);
    gap: var(--space-3);
    justify-content: space-between;
  }

  .rule-count {
    inline-size: 5rem;
  }

  .rule-methods {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    font-size: var(--font-size-compact);
  }

  .rule-tool {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .actors-note,
  .actor-rows {
    display: grid;
  }

  /* The two things that act on the whole ruleset, at the end where a page's own
     acts belong. The sentence sits between them so the destructive one is never
     the thing a hand lands on by accident. */
  .ruleset-foot {
    align-items: center;
    border-top: 1px solid var(--border-subtle);
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    justify-content: space-between;
    padding-top: var(--space-4);
  }

  .ruleset-foot-say {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }
</style>
