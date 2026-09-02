<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';

  import { formatDateTime, formatRelative, formatUntil } from '../format';
  import { SYNC_KINDS, type SyncAction, type SyncPlan } from '../types';
  import { SYNC_SECTION_LABELS } from '../routes';

  import ApplyBar from './ApplyBar.svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import DiffBlock from './DiffBlock.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

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
   * The detail beside the subject. A file's before and after are the file
   * itself, so its rows say how the change arrives instead of quoting it -
   * the diff below the row is the quote.
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
    <Card>
      <div class="card-head">
        <h2 class="card-title">
          {groups.length}
          {groups.length === 1 ? 'repository' : 'repositories'}
        </h2>
      </div>
      <ul class="object-list">
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
                      <span class="action-what"
                        >{action.subject}
                        {#if fromTo(action) !== ''}<span class="from-to">{fromTo(action)}</span
                          >{/if}</span
                      >
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
  }

  /* The row owns the tracks and the padding; the line inside is a subgrid, so
     the columns are declared ONCE and `.action-fail` on column 3 lands under
     the subject without a calc that has to be kept in step with them. */
  .action-row {
    border-radius: var(--r-ctl);
    display: grid;
    gap: var(--space-3);
    font-size: var(--font-size-compact);
    grid-template-columns: 4.2rem 5.2rem 1fr;
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
