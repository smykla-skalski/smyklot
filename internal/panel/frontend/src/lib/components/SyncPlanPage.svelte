<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';

  import { formatDateTime, formatRelative, formatUntil } from '../format';
  import { SYNC_KINDS, type SyncAction, type SyncPlan, type SyncRulesetDetail } from '../types';
  import { SYNC_SECTION_LABELS } from '../routes';

  import ApplyBar from './ApplyBar.svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import DiffBlock from './DiffBlock.svelte';
  import Icon from './Icon.svelte';
  import LabelBadge from './LabelBadge.svelte';
  import PageHeader from './PageHeader.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import { settingName } from './SyncSettingsPage.svelte';

  const {
    plan,
    nowMs,
    readOnly,
    canControl,
    approving,
    discarding,
    runNowBusy,
    onApprove,
    onDiscard,
    onRunNow,
  }: {
    plan: SyncPlan | null;
    /** The clock, passed in so a story renders the same minute every time. */
    nowMs: number;
    readOnly: boolean;
    canControl: boolean;
    approving: boolean;
    discarding: boolean;
    runNowBusy: boolean;
    onApprove: (planId: string, digest: string) => void;
    onDiscard: (planId: string) => void;
    onRunNow: (reason: string) => void;
  } = $props();

  const actions = $derived(plan?.actions ?? []);
  const total = $derived(actions.length);

  /* ---------- The kind filter ---------- */

  /**
   * A FILTER IS NOT A DESTINATION, and the two want different words. The tree
   * has to say "Repository options" because it navigates and no two rows in it
   * may share a word; a segment beside four others, over a list that has just
   * said what it is, only has to say which of the five. The long names took the
   * control to 510px against the drawing's 412 and pushed the whole filter past
   * half the content column.
   *
   * Nothing that navigates reads this - `SYNC_SECTION_LABELS` is still the one
   * name for a section, and this is a shorter reading of two of them, held
   * beside the control that wants it.
   */
  const FILTER_LABEL: Partial<Record<string, string>> = {
    settings: 'Options',
    files: 'Files',
  };

  const KIND_LABEL: Record<string, string> = SYNC_SECTION_LABELS;

  let filter = $state('all');

  const kindOptions = $derived([
    { value: 'all', label: 'All', badge: total },
    ...SYNC_KINDS.filter((kind) => actions.some((action) => action.kind === kind)).map((kind) => ({
      value: kind,
      label: FILTER_LABEL[kind] ?? KIND_LABEL[kind] ?? kind,
      badge: actions.filter((action) => action.kind === kind).length,
    })),
  ]);

  /* ---------- The groups ---------- */

  interface Group {
    repository: string;
    /** Every action, kind-sorted - the header counts read the whole group. */
    actions: SyncAction[];
  }

  const kindRank = (kind: string): number => {
    const at = (SYNC_KINDS as readonly string[]).indexOf(kind);
    return at === -1 ? SYNC_KINDS.length : at;
  };

  const groups = $derived.by((): Group[] => {
    const order: string[] = [];
    const byRepo: Record<string, SyncAction[]> = {};
    for (const action of actions) {
      const held = byRepo[action.repository];
      if (held === undefined) {
        order.push(action.repository);
        byRepo[action.repository] = [action];
      } else {
        held.push(action);
      }
    }
    return order.map((repository) => ({
      repository,
      actions: [...(byRepo[repository] ?? [])].sort(
        (left, right) => kindRank(left.kind) - kindRank(right.kind),
      ),
    }));
  });

  const visibleOf = (group: Group): SyncAction[] =>
    filter === 'all' ? group.actions : group.actions.filter((action) => action.kind === filter);

  function groupCounts(group: Group): { add: number; chg: number; del: number } {
    return {
      add: group.actions.filter((action) => action.operation === 'create').length,
      chg: group.actions.filter((action) => action.operation === 'update').length,
      del: group.actions.filter((action) => action.operation === 'delete').length,
    };
  }

  const failedOf = (group: Group): number =>
    group.actions.filter((action) => action.state === 'failed').length;

  /* ---------- One row's words ---------- */

  function opWord(action: SyncAction): string {
    if (action.operation === 'create') return '+ add';
    if (action.operation === 'update') return '~ change';
    return '− remove';
  }

  function opClass(action: SyncAction): string {
    if (action.operation === 'create') return 'is-add';
    if (action.operation === 'update') return 'is-chg';
    return 'is-del';
  }

  /**
   * THE FALLBACK, not the reading.
   *
   * The service sends `detail` - the same facts with their shape intact - and a
   * row draws that. This is what is left for an action whose payload the service
   * could not decode, and for a kind that has no typed detail yet: a sentence
   * somebody else formatted, printed as one.
   *
   * A file's before and after are the file itself, so its rows say how the
   * change arrives instead of quoting it - the diff below the row is the quote.
   */
  function fromTo(action: SyncAction): string {
    if (action.kind === 'files') {
      return action.operation === 'delete' ? '- marked for removal above' : '- as a pull request';
    }
    if (action.operation === 'update') {
      return `${action.before ?? ''} → ${action.after ?? ''}`;
    }
    if (action.operation === 'delete') {
      return action.before === undefined ? '' : `- ${action.before}`;
    }
    if (action.kind === 'settings') {
      return action.after === undefined ? '' : `- now managed, ${action.after}`;
    }
    return action.after === undefined ? '' : `- ${action.after}`;
  }

  /**
   * What a row draws, decided once so the markup asks a name rather than a
   * chain of conditions.
   *
   * `settings` is the only one that is a LIST: a settings change is one action,
   * because GitHub replaces a repository's settings in one request and they
   * succeed or fail together, and it is several facts. One action, one row, a
   * line per field - so the counts still say what would be applied while the
   * page says what would happen.
   */
  type RowShape = 'label' | 'settings' | 'ruleset' | 'file' | 'sentence';

  function rowShape(action: SyncAction): RowShape {
    const detail = action.detail;
    if (detail === undefined) return 'sentence';
    if (detail.label !== undefined) return 'label';
    if (detail.ruleset !== undefined) return 'ruleset';
    if (detail.file !== undefined) return 'file';
    if (detail.settings !== undefined && detail.settings.length > 0) return 'settings';
    return 'sentence';
  }

  /** How a file change arrives. Files land as a pull request; nothing else does. */
  const fileArrival = (action: SyncAction): string =>
    action.operation === 'delete' ? 'marked for removal above' : 'as a pull request';

  /** What a ruleset enforces, as one line. Empty is worth saying out loud. */
  function rulesetSummary(detail: SyncRulesetDetail): string {
    const parts = [detail.target, detail.enforcement];
    parts.push(
      detail.rules === undefined || detail.rules.length === 0
        ? 'enforcing nothing'
        : detail.rules.join(', '),
    );
    if (detail.bypass > 0) {
      parts.push(`${detail.bypass} ${detail.bypass === 1 ? 'bypass' : 'bypasses'}`);
    }

    return parts.join(' · ');
  }

  /* ---------- The expanded file diff ---------- */

  const keyOf = (action: SyncAction): string =>
    `${action.repository}\u0000${action.kind}\u0000${action.subject}`;

  const expandable = (action: SyncAction): boolean =>
    action.kind === 'files' &&
    action.operation !== 'delete' &&
    (action.before !== undefined || action.after !== undefined);

  const expanded = new SvelteSet<string>();

  function toggle(action: SyncAction): void {
    const key = keyOf(action);
    if (expanded.has(key)) expanded.delete(key);
    else expanded.add(key);
  }

  /* Which groups stand open. Kept as flips against the default - first group
     open, the rest closed - so a fresh plan needs no setup pass. */
  const flipped = new SvelteSet<string>();

  const isOpen = (repository: string, index: number): boolean =>
    index === 0 ? !flipped.has(repository) : flipped.has(repository);

  function toggleGroup(repository: string): void {
    if (flipped.has(repository)) flipped.delete(repository);
    else flipped.add(repository);
  }

  /* ---------- The apply bar and the confirmation ---------- */

  const approvable = $derived(plan !== null && plan.state === 'computed' && total > 0);

  let confirming = $state(false);
  let runConfirming = $state(false);
  let runReason = $state('');

  const removals = $derived(actions.filter((action) => action.operation === 'delete'));

  function confirmLine(group: Group): string {
    const dels = groupCounts(group).del;
    const changes = `${group.actions.length} ${group.actions.length === 1 ? 'change' : 'changes'}`;
    if (dels === 0) return `- ${changes}`;
    return `- ${changes}, ${dels} ${dels === 1 ? 'removal' : 'removals'}`;
  }

  /* ---------- The hero ---------- */

  const expiresWording = $derived.by(() => {
    if (plan === null) return null;
    const until = formatUntil(plan.expires_at, nowMs);
    return until.startsWith('in ')
      ? { lead: 'Expires in ', strong: until.slice(3) }
      : { lead: 'Expires ', strong: until };
  });

  const landed = $derived(actions.filter((action) => action.state === 'applied').length);
  const failed = $derived(actions.filter((action) => action.state === 'failed').length);

  const profileTime = (value: string, timezone?: string): string =>
    formatDateTime(value, { timeZone: timezone });
</script>

<!--
@component
The plan: Terraform's grammar under the overview's register. The verdict
is the hero, the state is the sentence rather than a badge, and every
fact lives once - scale in the hero, freshness and expiry on its
baseline, operation counts and the promise on the apply bar, scope on
the button.
-->

<div class="view-frame">
  <PageHeader id="sync-plan-heading" section="Sync" title="Plan" />

  {#if plan === null || total === 0}
    <!-- Having no plan is a state, not a verdict: the page-tier heading and the
         paragraph under it were the loaded page's shape worn by the empty one.
         `Check drift now` is the panel's one act rather than a button beside a
         headline about nothing. -->
    <Card>
      <div class="state-panel">
        <span
          ><strong>No plan is open.</strong> Every repository matches the configuration - a reconcile
          runs on a timer and writes a plan here the moment something drifts</span
        >
        {#if canControl}
          <Button tone="signal" disabled={runNowBusy} onclick={() => (runConfirming = true)}
            >{runNowBusy ? 'Queuing scan…' : 'Check drift now'}</Button
          >
        {/if}
      </div>
    </Card>
  {:else}
    <div class="hero">
      <h2>
        {#if plan.state === 'computed'}<span class="is-drift"
            >{total}
            {total === 1 ? 'change' : 'changes'}</span
          >
          {total === 1 ? 'waits' : 'wait'} for you{:else if plan.state === 'approved'}<span
            class="is-drift">{total} {total === 1 ? 'change' : 'changes'}</span
          > approved{:else if plan.state === 'applying'}Applying - {landed} of {total} landed{:else if plan.state === 'applied'}All
          {total} landed{:else if plan.state === 'failed'}<span class="is-failed"
            >{failed} of {total} failed</span
          >{:else if plan.state === 'stale'}This plan is <span class="is-stale">stale</span
          >{:else}This plan
          <span class="is-expired">expired</span>{/if}
      </h2>
      <span class="hero-meta hero-meta-lines">
        <span>Computed <strong>{formatRelative(plan.computed_at, nowMs)}</strong></span>
        {#if plan.state === 'computed' && expiresWording !== null}
          <span>{expiresWording.lead}<strong>{expiresWording.strong}</strong></span>
        {/if}
      </span>
    </div>

    {#if plan.queue_item !== undefined}
      {@const queued = plan.queue_item}
      <section class="schedule-card" aria-labelledby="plan-schedule-title">
        <div class="schedule-card-head">
          <h3 id="plan-schedule-title">Execution schedule</h3>
          <div class="schedule-card-actions">
            <span class="schedule-state">{queued.state.replaceAll('_', ' ')}</span>
            {#if canControl && plan.state === 'approved'}
              <Button row tone="signal" disabled={runNowBusy} onclick={() => (runConfirming = true)}
                >{runNowBusy ? 'Dispatching…' : 'Run now'}</Button
              >
            {/if}
          </div>
        </div>
        <dl class="schedule-facts">
          <div>
            <dt>Runs no earlier than</dt>
            <dd>
              <time datetime={queued.eligible_at}>{formatDateTime(queued.eligible_at)}</time>
              <small>{formatUntil(queued.eligible_at, nowMs)} in your timezone</small>
            </dd>
          </div>
          <div>
            <dt>Hours</dt>
            <dd>
              {queued.profile_name ?? queued.profile_id ?? 'One-time bypass'}
              <small>{profileTime(queued.eligible_at, queued.profile_timezone)}</small>
            </dd>
          </div>
          <div>
            <dt>Estimated start</dt>
            <dd>
              {queued.estimated_start_at
                ? formatDateTime(queued.estimated_start_at)
                : 'Not estimated'}
              <small
                >{queued.work_ahead === 0
                  ? 'Nothing ahead of it'
                  : `${queued.work_ahead} items ahead`} · estimate</small
              >
            </dd>
          </div>
          <div>
            <dt>Current status</dt>
            <dd>
              {queued.blocked_reason || queued.summary || queued.state.replaceAll('_', ' ')}
              <small
                >{queued.state === 'running'
                  ? `${plan.execution_stage} · attempt ${queued.attempt} · ${queued.progress_current} of ${queued.progress_total}`
                  : queued.state === 'retrying'
                    ? `Retry attempt ${queued.attempt}`
                    : `${plan.execution_stage} · priority ${queued.priority}`}</small
              >
            </dd>
          </div>
        </dl>
      </section>
    {/if}

    <div class="plan-tools">
      <SegmentedControl
        name="plan-kind"
        label="Kind"
        options={kindOptions}
        value={filter}
        onSelect={(value) => (filter = value)}
      />
    </div>

    <!-- ONE CARD, and a repository is an OBJECT ROW in it. Each repository used
         to be a card of its own, so three repositories were three frames and a
         row grammar this app uses nowhere else - the rest of the panel is one
         card holding an `.object-list`, and the plan had no reason to be the
         exception. The disclosure body is a sibling of the row inside the same
         `<li>`, which `display: contents` lets the list lay out as its own
         band. -->
    <!-- No head. `Plan`, `14 changes wait for you` and `3 repositories` were
         three headings in the page's first 250px, and the third one counted
         what the rows under it show and the Apply button already scopes. A list
         card without one is grammar this app already has - `RootWorkspaces`
         opens the same way. -->
    <Card>
      <ul class="object-list plan-repositories">
        {#each groups as group, index (group.repository)}
          {@const visible = visibleOf(group)}
          {@const counts = groupCounts(group)}
          {@const groupFailed = failedOf(group)}
          {@const open = isOpen(group.repository, index)}
          {@const rowsId = `plan-group-${index}`}
          <li>
            <button
              type="button"
              class="object-row repo-row"
              class:is-open={open}
              aria-expanded={open}
              aria-controls={rowsId}
              onclick={() => toggleGroup(group.repository)}
            >
              <span class="object-main">
                <span class="object-name-row">
                  <span class="object-name mono-name">{group.repository}</span>
                </span>
              </span>
              <span class="object-side">
                {#if groupFailed > 0}
                  <span class="pill pill-danger"><span class="t">{groupFailed} failed</span></span>
                {:else}
                  <span class="repo-group-counts">
                    {#if counts.add > 0}<span class="count-add">+{counts.add}</span>{/if}
                    {#if counts.chg > 0}<span class="count-chg">~{counts.chg}</span>{/if}
                    {#if counts.del > 0}<span class="count-del">−{counts.del}</span>{/if}
                  </span>
                {/if}
                <span class="repo-caret"><Icon name="chevron-right" size="xs" /></span>
              </span>
            </button>
            {#if open}
              <div class="action-rows" id={rowsId}>
                {#each visible as action, at (keyOf(action))}
                  {@const firstOfRun = at === 0 || visible[at - 1]?.kind !== action.kind}
                  {@const opens = expandable(action)}
                  {@const showing = opens && expanded.has(keyOf(action))}
                  {@const shape = rowShape(action)}
                  <!-- ONE ROW, whatever it can do. A row that opens a diff used to be
                   a second component - a 24px button beside a 40px div, holding
                   the same three spans - so a list of six rows kept two rhythms
                   and the only pressable one was the shortest. What it can do is
                   the tag and a mark at its end, never a different row. -->
                  <div
                    class="action-row"
                    class:has-diff={opens}
                    class:is-expanded={showing}
                    data-kind={action.kind}
                  >
                    <!-- The role is the tag's own, spelled out because the checker
                     reads `onclick` on a `<svelte:element>` without being able
                     to see that the tag beside it is `button` whenever the
                     handler is there. -->
                    <svelte:element
                      this={opens ? 'button' : 'div'}
                      class="action-row-line"
                      type={opens ? 'button' : undefined}
                      role={opens ? 'button' : undefined}
                      aria-expanded={opens ? showing : undefined}
                      onclick={opens ? () => toggle(action) : undefined}
                    >
                      <span class="action-op {opClass(action)}">{opWord(action)}</span>
                      <span class="action-kind">{firstOfRun ? action.kind : ''}</span>
                      {#if shape === 'label' && action.detail?.label !== undefined}
                        {@const label = action.detail.label}
                        {@const was = action.detail.previous_label}
                        <span class="action-what"
                          >{#if was !== undefined}<!-- A CHANGE SHOWS WHAT MOVED.
                              Both badges, the arrow between them, and only the
                              description that differs - a colour drift read as
                              the same label printed twice without this.
                            --><LabelBadge
                              label={was}
                              size="compact"
                            /><span class="from-to label-arrow">→</span>{/if}<LabelBadge
                            {label}
                            size="compact"
                          />{#if label.description && label.description !== was?.description}<span
                              class="from-to label-description">{label.description}</span
                            >{/if}</span
                        >
                      {:else if shape === 'settings' && action.detail?.settings !== undefined}
                        <!-- ONE ACTION, a line per setting. GitHub replaces a
                             repository's settings in one request, so they apply
                             or fail together and stay one action - and they are
                             still several facts, where one sentence naming every
                             field at once was one. -->
                        <span class="action-what action-settings">
                          {#each action.detail.settings as setting (setting.field)}
                            <span class="setting-line"
                              >{settingName(setting.field)}<span class="from-to"
                                >{setting.from} → {setting.to}</span
                              ></span
                            >
                          {/each}
                          {#each action.detail.follows ?? [] as follows (follows)}
                            <span class="setting-line"
                              >{settingName(follows)}<span class="from-to"
                                >GitHub switches this off too</span
                              ></span
                            >
                          {/each}
                          {#each action.detail.withheld ?? [] as withheld (withheld.field)}
                            <span class="setting-line"
                              >{settingName(withheld.field)}<span class="from-to"
                                >left alone: {withheld.reason}</span
                              ></span
                            >
                          {/each}
                        </span>
                      {:else if shape === 'ruleset' && action.detail?.ruleset !== undefined}
                        {@const ruleset = action.detail.ruleset}
                        <span class="action-what"
                          >{ruleset.name}<span class="from-to">{rulesetSummary(ruleset)}</span
                          ></span
                        >
                      {:else if shape === 'file' && action.detail?.file !== undefined}
                        <span class="action-what"
                          >{action.detail.file.path}<span class="from-to"
                            >{fileArrival(action)}</span
                          ></span
                        >
                      {:else}
                        <span class="action-what"
                          >{action.subject}
                          {#if fromTo(action) !== ''}<span class="from-to">{fromTo(action)}</span
                            >{/if}</span
                        >
                      {/if}
                    </svelte:element>
                    {#if showing}
                      <div class="action-diff">
                        <DiffBlock before={action.before ?? ''} after={action.after ?? ''} />
                      </div>
                    {/if}
                    {#if action.error !== undefined}
                      <span class="action-fail">{action.error}</span>
                    {:else if action.blocker !== undefined}
                      <span class="action-fail">not tried: {action.blocker} failed first</span>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </li>
        {/each}
      </ul>
    </Card>

    {#if approvable && !readOnly}
      <ApplyBar>
        <span class="apply-counts">
          {#if plan.counts.create > 0}<span class="count-add">+{plan.counts.create}</span>{/if}
          {#if plan.counts.update > 0}<span class="count-chg">~{plan.counts.update}</span>{/if}
          {#if plan.counts.delete > 0}<span class="count-del">−{plan.counts.delete}</span>{/if}
        </span>
        <!-- `&nbsp;` twice, written as the entity rather than as the character:
             a bar this wide reflows at every sidebar width, and the two places
             it can break are the one that starts a line with a dash and the one
             that leaves `3` stranded from what it counts. A literal U+00A0 in
             the source does the same thing and is invisible to whoever edits
             the sentence next. -->
        <span class="apply-note"
          >Nothing reaches GitHub until you apply the plan&nbsp;- files open pull requests and other
          changes apply directly</span
        >
        <Button tone="quiet" disabled={discarding || approving} onclick={() => onDiscard(plan.id)}>
          {discarding ? 'Discarding' : 'Discard'}
        </Button>
        <Button
          tone="signal"
          disabled={approving || discarding}
          onclick={() => (confirming = true)}
        >
          Apply to {groups.length}&nbsp;{groups.length === 1 ? 'repository' : 'repositories'}
        </Button>
      </ApplyBar>
    {/if}

    <ConfirmDialog
      id="apply-plan-dialog"
      open={confirming}
      title="Apply this plan?"
      description="Smyklot will change {groups.length} {groups.length === 1
        ? 'repository'
        : 'repositories'} now. Settings, labels and rulesets change directly; each file change opens a pull request"
      confirmLabel="Apply the plan"
      busyLabel="Applying…"
      confirmTone="signal"
      busy={approving}
      onClose={() => (confirming = false)}
      onConfirm={() => {
        confirming = false;
        onApprove(plan.id, plan.digest);
      }}
    >
      <div class="confirm-list">
        {#each groups as group (group.repository)}
          <span>{group.repository} <span class="confirm-muted">{confirmLine(group)}</span></span>
        {/each}
      </div>
      {#if removals.length > 0}
        <p class="confirm-consequence">
          <span class="confirm-danger"
            >{removals.length === 1 ? 'One removal' : `${removals.length} removals`}:</span
          >
          {removals
            .map((action) => `${action.subject} is deleted from ${action.repository}`)
            .join('; ')}. This cannot be undone from here
        </p>
      {/if}
    </ConfirmDialog>
  {/if}

  <ConfirmDialog
    id="sync-run-now-dialog"
    open={runConfirming}
    title={plan?.state === 'approved' ? 'Run approved plan now?' : 'Check organization drift now?'}
    description={plan?.state === 'approved'
      ? 'This bypasses the assigned window once and immediately dispatches the approved plan.'
      : 'This queues an immediate drift scan. Any changes still require human approval.'}
    confirmLabel={plan?.state === 'approved' ? 'Run now' : 'Queue drift scan'}
    busyLabel="Queuing…"
    confirmTone="signal"
    busy={runNowBusy}
    confirmDisabled={runReason.trim() === ''}
    onClose={() => {
      if (!runNowBusy) runConfirming = false;
    }}
    onConfirm={() => {
      const reason = runReason.trim();
      if (reason === '') return;
      runConfirming = false;
      runReason = '';
      onRunNow(reason);
    }}
  >
    <label class="run-reason" for="sync-run-reason"
      >Reason<textarea
        id="sync-run-reason"
        rows="3"
        bind:value={runReason}
        placeholder="Why should this run outside its normal schedule?"></textarea></label
    >
  </ConfirmDialog>
</div>

<style>
  /* The reading column is the sheet's; what is this page's own is the apply bar's seat -
     the marker's named view timeline is declared in the slot after it and handed back up
     here. */
  .view-frame {
    timeline-scope: --bar-slot;
  }

  /* ---------- The hero, in the overview's own register ---------- */

  /* No margin above: the frame's gap is what stands this off the head, and the 8px
     here was added to it - the one page whose head exited at 32 where every other
     exits at 24. */
  /* ONE COLUMN, and the meta reads under the verdict rather than off in the
     opposite corner. Freshness and expiry are the sentence that says how long
     the count above them is true for; parked at the far right of the same line
     they were a caption on nothing, and expiry - the fact that voids the whole
     page - was the smallest type on it. */
  .hero {
    display: grid;
    gap: var(--space-2);
    grid-template-columns: minmax(0, 1fr);
    margin-block-end: var(--space-4);
  }

  /* The verdict is CONTENT UNDER the page's h1, not a second page title. At the
     page tier it measured the same 28px as `Plan` above it, and two headings of
     one size in one corner leave the reading order to guesswork. 22px whole is
     one full step down, which `--leading-title` is already the leading for. */
  .hero h2 {
    font-size: 1.375rem;
    font-weight: 650;
    letter-spacing: -0.02em;
    line-height: var(--leading-title);
    margin: 0;
    min-block-size: 19px;
    text-box: trim-both cap alphabetic;
  }

  .hero h2 .is-drift {
    color: var(--diff-chg-ink);
  }

  .hero h2 .is-failed {
    color: var(--danger);
  }

  .hero h2 .is-stale {
    color: var(--warning);
  }

  .hero h2 .is-expired {
    color: var(--text-muted);
  }

  .hero-meta {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }

  .hero-meta strong {
    color: var(--text-secondary);
    font-weight: 600;
  }

  .hero-meta-lines {
    display: grid;
    gap: var(--space-1);
    justify-items: start;
  }

  .hero-meta-lines > span {
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }

  /* The kind filter on its own line - a control never shares a line with prose.
     No margin below: this bar is always a child of the frame, whose gap is the
     distance to what it acts on, and a margin here as well made 32px where the
     drawing has 16. */
  .plan-tools {
    display: flex;
  }

  .schedule-card {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    margin-block-end: var(--space-4);
    padding: var(--space-4);
  }

  .schedule-card-head {
    align-items: center;
    display: flex;
    justify-content: space-between;
    margin-block-end: var(--space-4);
  }

  .schedule-card-head h3 {
    font-size: var(--font-size-card-title);
    margin: 0;
  }

  .schedule-card-actions {
    align-items: center;
    display: flex;
    gap: var(--space-3);
  }

  .schedule-state {
    color: var(--text-secondary);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    text-transform: uppercase;
  }

  .schedule-facts {
    display: grid;
    gap: var(--space-4);
    grid-template-columns: repeat(4, minmax(0, 1fr));
    margin: 0;
  }

  .schedule-facts div {
    min-width: 0;
  }

  .schedule-facts dt,
  .schedule-facts small {
    color: var(--text-muted);
    display: block;
    font-size: var(--font-size-micro);
  }

  .schedule-facts dd {
    color: var(--text-primary);
    font-size: var(--font-size-meta);
    margin: var(--space-1) 0 0;
  }

  .schedule-facts small {
    margin-block-start: var(--space-1);
  }

  /* ---------- The repositories ---------- */

  /* A repository is an object row, so its material is the list's and nothing is
     declared here but what a plan row has that other rows do not: a name in the
     identifier face, and a caret that turns. */
  /* No `width: 100%`. An object row bleeds 12px into the card's padding on each
     side so its hover paints to the frame, and it does that by stretching in
     the list's track and then wearing a negative inline margin. A declared
     100% pins it to the track instead, so the row came out 24px narrower than
     the well underneath it and the two disagreed down their whole length. A
     grid item stretches on its own. */
  /* THE LIST SHEDS ITS PADDING AT BOTH ENDS. `.object-list` already hands back
     the last row's, so the last ink closes on the card's own frame; with no card
     head above it the first row needs the same, or the card opens with 20px of
     its own padding and 12px of the row's stacked - 34.7px of nothing before the
     first name. The row keeps its padding, which is its hit area; what goes is
     the card's, which the row is already standing in. */
  .plan-repositories {
    margin-block-start: calc(var(--row-pad-default) * -1);
  }

  .repo-row {
    cursor: pointer;
    font: inherit;
    text-align: start;
  }

  .mono-name {
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-weight: 500;
  }

  .repo-caret {
    color: var(--text-muted);
    display: inline-flex;
    transition: rotate var(--duration-fast) var(--ease-standard);
  }

  .repo-row.is-open .repo-caret {
    rotate: 90deg;
  }

  /* An open row keeps `.object-row`'s hairline, hover and press - suppressing
     them to stop it competing with the well left a header that answered nothing
     at all. What it adds is REACH.

     A hover paints the row's own rounded box, and the well below opens with
     rounded top corners of its own, so the paint stopped at the row's bottom
     curve and left two lit notches of card white in the corners between them.
     The row squares the edge it shares with the well and carries the paint one
     corner-radius further down, behind it - so the wash runs under the well's
     curve and the corners come out solid. */
  .repo-row.is-open {
    border-end-end-radius: 0;
    border-end-start-radius: 0;
  }

  .repo-row.is-open::before {
    block-size: var(--r-ctl);
    content: '';
    inset-block-start: 100%;
    inset-inline: 0;
    position: absolute;
  }

  :is(a, button).repo-row.is-open:hover::before {
    background: var(--row-hover);
  }

  :is(a, button).repo-row.is-open:active::before {
    background: var(--row-pressed);
  }

  /* THREE SLOTS, ALWAYS, and each operation keeps its own. Laid out as a flex
     run the group was right-aligned, so a repository with no removals put its
     `+3` where the repository above it put `~2`: scanning the rail, a green
     number and a blue one shared a column and the sign was the only thing
     saying which was which. A missing count leaves its slot empty rather than
     closing the rank up.

     The slot is a fixed 4ch and not content-derived, because each row is its
     own grid: `auto` would size every row to ITS widest count and the rows
     would disagree again. Mono and tabular make 4ch exact - a sign and three
     digits - and identical on every row. */
  .repo-group-counts {
    --count-slot: 4ch;

    color: var(--text-muted);
    display: grid;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    gap: var(--space-3);
    grid-template-columns: repeat(3, var(--count-slot));
  }

  .repo-group-counts > * {
    justify-self: end;
    text-box: trim-both cap alphabetic;
  }

  .repo-group-counts .count-add {
    grid-column: 1;
  }

  .repo-group-counts .count-chg {
    grid-column: 2;
  }

  .repo-group-counts .count-del {
    grid-column: 3;
  }

  .count-add {
    color: var(--diff-add-ink);
  }

  .count-chg {
    color: var(--diff-chg-ink);
  }

  .count-del {
    color: var(--diff-del-ink);
  }

  /* The disclosure body, inset under the row it belongs to. A well rather than a
     second card: the ladder is canvas, card, opened repository, opened action,
     code - and giving this its own frame inside a card would put two frames
     around one list. The negative inline margin is the object row's own, so the
     well runs the full width of the card's content column. */
  .action-rows {
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-ctl);
    display: grid;
    margin-block: 0 var(--space-3);
    margin-inline: calc(var(--space-3) * -1);
    padding: var(--space-2);
    /* POSITIONED, so it paints over the header's reach strip by DOM order.
       A negative z-index on the strip put it behind the card's own ground and
       it never showed at all; two positioned siblings settle it by which comes
       second, and the well does. */
    position: relative;
  }

  /* The row owns the tracks and the padding; the line inside is a subgrid, so
     the columns are declared ONCE and `.action-fail` on column 3 lands under
     the subject without a calc that has to be kept in step with them. */
  .action-row {
    border-radius: var(--r-ctl);
    display: grid;
    gap: var(--space-3);
    font-size: var(--font-size-compact);
    /* MEASURED, not estimated. The verb track holds `− remove` at 66.06px in
       67.2 and is right. The kind track held 83.2 for a vocabulary of four
       words - `labels`, `settings`, `rulesets`, `files` - whose widest is
       `settings` at 46.48, so 36.7px of every row was a column that could
       never be filled, sitting between the two halves of the sentence. 3rem
       leaves the same 1.5px the verb track leaves, and hands the rest to the
       subject. Declared rather than `max-content`: each row is its own grid,
       and content sizing would let two rows disagree about where the sentence
       starts - the same fault the counts had. */
    grid-template-columns: 4.2rem 3rem 1fr;
    padding: var(--space-3) var(--space-2);
  }

  /* Hover answers the POINTER, so only a row the pointer can do something with
     wears it. Every row lighting up said all of them were pressable, and one in
     six was. */
  .action-row.has-diff:hover {
    background: var(--row-hover);
  }

  /* An opened action gets its own ground, like a rule opened for editing -
     the diff was floating on the card with no parent to belong to. */
  .action-row.is-expanded {
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-ctl);
    margin-block: var(--space-1);
  }

  /* The open row keeps its raised ground; re-tinting the whole card on hover
     would flash under the diff. The edge answers the pointer instead. */
  .action-row.is-expanded:hover {
    background: var(--surface-raised);
    border-color: var(--control-border);
  }

  /* DECLARED, so the row is 42px whatever it says - the drawing's height, and
     the same reasoning as the group head's declared 40.

     The three cells were each voting on it with their own font, and
     `align-items: baseline` let them: the kind is sans against two mono
     neighbours and its box sat a pixel higher, and an empty kind cell on a
     continuation row had no box at all. Rows came out 39, 40 and 41 by content,
     and the run read as a limp. `min-block-size` is the line the drawing has,
     and centring puts the three cells on it whatever each of them measures. */
  .action-row-line {
    align-items: center;
    background: none;
    border: 0;
    color: inherit;
    display: grid;
    font: inherit;
    grid-column: 1 / -1;
    grid-template-columns: subgrid;
    line-height: var(--leading-compact);
    min-block-size: var(--leading-compact);
    padding: 0;
    /* Inert on a desktop row, which is one band. A phone stacks the subject
       under the verb, and only the LINE can space those two - a subgrid takes
       its parent's gap for the axis it subgrids, and that is the columns. */
    row-gap: var(--space-2);
    /* Carries the hit layer below. */
    position: relative;
    text-align: start;
    width: 100%;
  }

  /* EVERY CELL IS ITS CAP BAND. Without this the verb and the kind are their
     whole 18px line boxes while a stacked setting line is its 8.76px band, so
     the row's ink sat half a pixel off the middle of its own surface - which is
     the fault the vertical sweep exists to catch. `min-block-size` above is
     what keeps a one-line row 42px once the cells no longer fill it. */
  .action-row-line > * {
    text-box: trim-both cap alphabetic;
  }

  /* A STACKED SUBJECT LEADS, it does not float. Centred against two or five
     settings the verb sat in the middle of the block with nothing beside it,
     reading as a label for the gap rather than for the first line. Start, so
     `~ change` and `settings` sit on the first field's own band and the rest of
     the fields hang under them.

     Only where the subject stacks: a one-line row is centred in its declared
     42px, and start-aligning that would push its band to the top of the line. */
  .action-row-line:has(.action-settings) {
    align-items: start;
  }

  button.action-row-line {
    cursor: pointer;
  }

  /* THE PRESS REACHES THE WHOLE ROW. The row's padding is on the row, so the
     button is only the line inside it - 18px of a 42px band that hovers, and
     under the 24px floor a target has to clear. The hit is a layer rather than
     a wrapper, the way `.row-hit` does it elsewhere: moving the padding onto
     the button instead would take the tracks off the row, and the subgrid and
     the failure line under it both read their columns from there.

     Only as far as the line, never the diff: an opened row's file is content
     to read, not a second place to press for closing it. */
  button.action-row-line::before {
    content: '';
    inset-block: calc(var(--space-3) * -1);
    inset-inline: calc(var(--space-2) * -1);
    position: absolute;
  }

  /* Inside the opened row's ground the diff runs full width - the ground
     already says which row it belongs to. It is a child of the row's own grid
     now, so it has to be told to cross it: left to flow it took column one and
     came out 67px wide, the width of the verb. */
  .action-diff {
    grid-column: 1 / -1;
    margin: var(--space-2) 0 0;
    min-inline-size: 0;
  }

  .action-op {
    font-family: var(--mono);
    font-variant-numeric: tabular-nums;
  }

  .action-op.is-add {
    color: var(--diff-add-ink);
  }

  .action-op.is-chg {
    color: var(--diff-chg-ink);
  }

  .action-op.is-del {
    color: var(--diff-del-ink);
    font-weight: 600;
  }

  .action-kind {
    color: var(--text-muted);
  }

  .action-what {
    font-family: var(--mono);
  }

  .action-what .from-to {
    color: var(--text-muted);
    /* The detail follows the subject with one space of its own, so the two do
       not run together when the subject is drawn rather than written - a badge
       is an element, and an element has no trailing space. */
    margin-inline-start: 0.5ch;
  }

  /* A LABEL'S DESCRIPTION IS PROSE, and it was set in the same mono as the
     label's own name beside it - two greys of the same face running together,
     with only a space saying where the name stopped. The reading face is what
     separates them, and it is what the description is: a sentence about the
     label, not a second identifier.

     The face alone, at the row's own size: a description set larger also raises
     the line's ink band above the box it sits in, and the row came out 0.47px
     off its own middle. */
  .action-what .label-description {
    font-family: var(--sans);
    margin-inline-start: 1ch;
  }

  /* A settings action's several facts, stacked. The row's own line stays the
     first of them, so a one-setting change is the same 42px as every other row
     and only a change that really says more is taller. */
  .action-settings {
    display: grid;
    gap: var(--space-2);
    justify-items: start;
  }

  .setting-line {
    text-box: trim-both cap alphabetic;
  }

  /* The badge sets no line height: it is 9px of disc in a 12px line, and a row
     whose subject is drawn must measure the same as a row whose subject is
     written. */
  .action-what :global(.label-badge) {
    vertical-align: baseline;
  }

  /* Between two badges, so it needs the space on BOTH sides. `.from-to` leads
     with half a character and nothing after it, which is right where a detail
     follows a subject and wrong where an arrow stands between two things. */
  /* Selected through `.action-what` so it OUTRANKS the rule above rather than
     tying with it: `.action-what .from-to` sets the start margin, and a bare
     `.label-arrow` lost that half of the pair on source order - the arrow came
     out 0.5ch from the badge before it and 1ch from the one after. */
  .action-what .label-arrow {
    margin-inline: 1ch;
  }

  /* The error belongs to ITS row: pulled to 4px under its own line, so the
     next row (16px of paddings away) can never claim it. */
  .action-fail {
    color: var(--danger);
    font-size: var(--font-size-micro);
    grid-column: 3;
    line-height: var(--leading-micro);
    margin-block-start: calc(var(--space-1) - var(--space-3));
  }

  /* ---------- The apply bar: material lives in ApplyBar.svelte ---------- */

  .apply-counts {
    display: flex;
    flex: none;
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-variant-numeric: tabular-nums;
    gap: var(--space-3);
  }

  .apply-counts > * {
    text-box: trim-both cap alphabetic;
  }

  .apply-note {
    color: var(--text-secondary);
    flex: 1;
    font-size: var(--font-size-meta);
    line-height: var(--leading-meta);
    text-box: trim-both cap alphabetic;
  }

  .card-head {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    margin-bottom: var(--space-4);
  }

  .plan-rule {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    line-height: var(--leading-meta);
    margin: 0;
    text-wrap: pretty;
  }

  .pill {
    align-items: center;
    /* A chip's height is a decision: 20px, the app's chip-small. */
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

  .pill-danger {
    background: var(--danger-tint);
    color: var(--danger);
  }

  /* ---------- The confirmation's own lines ---------- */

  .confirm-list {
    display: grid;
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-variant-numeric: tabular-nums;
    gap: var(--space-1);
  }

  .confirm-muted {
    color: var(--text-muted);
  }

  .confirm-consequence {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    margin: 0;
  }

  .confirm-danger {
    color: var(--danger);
    font-weight: 600;
  }

  .run-reason {
    color: var(--text-secondary);
    display: grid;
    font-size: var(--font-size-meta);
    gap: var(--space-2);
  }

  .run-reason textarea {
    background: var(--control-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font: inherit;
    min-block-size: 5rem;
    padding: var(--space-3);
    resize: vertical;
  }

  .run-reason textarea:focus-visible {
    outline: var(--focus-ring-width) solid var(--focus);
    outline-offset: var(--focus-ring-offset);
  }

  @media (max-width: 36rem) {
    .view-frame {
      overflow-x: hidden;
    }

    .hero h2 {
      font-size: 2rem;
      overflow-wrap: anywhere;
    }

    .schedule-facts {
      grid-template-columns: minmax(0, 1fr);
    }

    .schedule-card-head,
    .schedule-card-actions {
      align-items: flex-start;
    }

    .schedule-card-head {
      gap: var(--space-3);
    }

    .schedule-card-actions {
      flex-direction: column;
    }

    .plan-tools {
      max-inline-size: 100%;
      min-inline-size: 0;
      overflow-x: auto;
    }

    /* The row still owns the tracks; the line is still the subgrid that reads
       them, so only one of the two is restated here. */
    .action-row {
      gap: var(--space-2);
      grid-template-columns: minmax(0, 1fr) auto;
    }

    .action-op {
      grid-column: 1;
    }

    .action-kind {
      grid-column: 2;
      text-align: end;
    }

    .action-what,
    .action-fail {
      grid-column: 1 / -1;
      margin-block-start: 0;
      min-inline-size: 0;
      overflow-wrap: anywhere;
    }
  }
</style>
