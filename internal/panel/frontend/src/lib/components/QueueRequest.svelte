<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { useInterval } from 'runed';

  import type { PanelApi } from '#lib/api.js';
  import { formatTimestamp } from '#lib/format.js';
  import { cleanupState, outcomeState, queueNext, queueState, sinceLabel } from '#lib/queue.js';
  import type { PendingCIEvent } from '#lib/types.js';
  import BackLink from './BackLink.svelte';
  import Chip from './Chip.svelte';
  import Icon, { type IconName } from './Icon.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import RootPageHeader from './RootPageHeader.svelte';

  const {
    api,
    rootRole,
    requestId,
    queueHref,
    onBack,
  }: {
    api: PanelApi;
    rootRole: string;
    requestId: string;
    queueHref: string;
    onBack: () => void;
  } = $props();

  /* The reconciler's `PassingQuiet`, which is the whole span the ring draws. */
  const QUIET_SECONDS = 30;

  /* Keyed by the request, so walking from one to another swaps the record rather
     than showing the previous one until the new read lands. */
  const detailQuery = createQuery(() => ({
    queryKey: ['root-pending-ci', requestId],
    queryFn: () => api.fetchRootPendingCI(requestId),
  }));
  const detail = $derived(detailQuery.data ?? null);
  const loading = $derived(detailQuery.isFetching);
  let actionProblem = $state<string | null>(null);
  const problem = $derived(
    actionProblem ??
      (detailQuery.error === null
        ? null
        : detailQuery.error instanceof Error
          ? detailQuery.error.message
          : String(detailQuery.error)),
  );
  let acting = $state<string | null>(null);
  let now = $state(Date.now());

  const request = $derived(detail?.request ?? null);
  const armed = $derived(request?.lifecycle === 'armed');
  /* Not `state`: that name shadows the `$state` rune for the type checker, which
     then reads every rune in the file as a block-scoped variable used before its
     declaration. */
  const stateChip = $derived(
    request === null ? null : armed ? queueState(request) : outcomeState(request),
  );
  const next = $derived(request === null ? null : queueNext(request, now));

  /* Every second, because the countdown beside a waiting request is drawn in them.
     The tables the panel already had tick at thirty - see `RepositoryList` - which is
     the right rate for "4 minutes ago" and the wrong one for a clock running out. */
  useInterval(1000, { callback: () => (now = Date.now()) });

  async function load(): Promise<void> {
    actionProblem = null;
    await detailQuery.refetch();
  }

  async function act(action: 'check' | 'cancel'): Promise<void> {
    if (request === null) return;
    acting = action;
    actionProblem = null;
    try {
      if (action === 'check') await api.checkRootPendingCI(request.id, request.revision);
      else await api.cancelRootPendingCI(request.id, request.revision);
      await load();
    } catch (error) {
      actionProblem = error instanceof Error ? error.message : String(error);
    } finally {
      if (acting === action) acting = null;
    }
  }

  /* One mark per kind of event, and the tone says whether it was a step forward,
     an outcome, or a problem. The shape is what carries it - the rail is a
     column of identical circles otherwise. */
  function eventMark(event: PendingCIEvent): { icon: IconName; tone: string } {
    switch (event.kind) {
      case 'armed':
        return { icon: 'plus', tone: 'act' };
      case 'wake_received':
        return { icon: 'mail', tone: '' };
      case 'reconciliation_started':
        return { icon: 'refresh', tone: '' };
      case 'checks_observed':
        return event.state === 'passing'
          ? { icon: 'success', tone: 'pass' }
          : event.state === 'failing'
            ? { icon: 'failure', tone: 'fail' }
            : { icon: 'pending', tone: '' };
      case 'merge_started':
        return { icon: 'branch', tone: 'act' };
      case 'finished':
        return { icon: 'success', tone: 'pass' };
      case 'superseded':
        return { icon: 'alert', tone: 'warn' };
      case 'cleanup_retry':
        return { icon: 'refresh', tone: 'warn' };
      case 'cleanup_completed':
        return { icon: 'check', tone: 'pass' };
    }
  }

  function eventTitle(event: PendingCIEvent): string {
    switch (event.kind) {
      case 'armed':
        return 'Armed';
      case 'wake_received':
        return 'Delivery received';
      case 'reconciliation_started':
        return 'Reconciliation started';
      case 'checks_observed':
        return 'Checks observed';
      case 'merge_started':
        return 'Merge started';
      case 'finished':
        return 'Finished';
      case 'superseded':
        return 'Superseded';
      case 'cleanup_retry':
        return 'Cleanup retried';
      case 'cleanup_completed':
        return 'Cleanup finished';
    }
  }

  function triggerLabel(event: PendingCIEvent): string {
    return event.trigger.replaceAll('_', ' ').replace(/^./u, (first) => first.toUpperCase());
  }

  function clockOf(value: string): string {
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime())
      ? value
      : parsed.toLocaleTimeString(undefined, {
          hour: '2-digit',
          minute: '2-digit',
          second: '2-digit',
        });
  }
</script>

<BackLink href={queueHref} label="Queue" onNavigate={onBack} />

{#if request === null}
  <RootPageHeader role={rootRole} title="Request" subtitle="Reading the record" />
  {#if problem !== null}
    <ResultProblem
      title="The request could not be read"
      {problem}
      onRetry={() => void load()}
      busy={loading}
    />
  {/if}
{:else}
  <RootPageHeader
    role={rootRole}
    title={`${request.repository_full_name} #${request.pull_request}`}
    subtitle={`Armed ${sinceLabel(request.requested_at, now)} by @${request.requester}, bound to commit ${request.head_sha.slice(0, 8)}`}
  >
    <a
      class="btn"
      href={`https://github.com/${request.repository_full_name}/pull/${request.pull_request}`}
      rel="noreferrer"
      target="_blank"
    >
      <Icon name="github" size={16} />
      <span class="button-label">Open on GitHub</span>
    </a>
  </RootPageHeader>

  {#if problem !== null}
    <ResultProblem
      title="The request could not be read"
      {problem}
      onRetry={() => void load()}
      busy={loading}
      overContent
    />
  {/if}

  <section class="plate summary-plate" aria-label="What this request is doing">
    <!-- The card is bands, not things floating in one padded box. The state and
         what can be done about it share the top band; the facts below are a grid
         of real cells, and the rule between them sits in the middle of its own
         space because both sides give it the plate's own padding. -->
    <div class="card-band">
      <div class="card-state">
        {#if stateChip !== null}
          <Chip tone={stateChip.tone} icon={stateChip.icon}>{stateChip.label}</Chip>
        {/if}
        {#if next !== null && armed}
          <span
            class="next-lead"
            class:due={next.merging}
            class:idle={!next.merging}
            class:imminent={next.merging && next.seconds !== null && next.seconds <= 10}
            class:final={next.merging && next.seconds !== null && next.seconds <= 5}
          >
            {#if next.merging && next.seconds !== null}
              <svg class="ring" viewBox="0 0 14 14" fill="none" aria-hidden="true">
                <circle
                  cx="7"
                  cy="7"
                  r="5.6"
                  stroke="currentColor"
                  stroke-opacity="0.25"
                  stroke-width="1.8"
                />
                <circle
                  cx="7"
                  cy="7"
                  r="5.6"
                  stroke="currentColor"
                  stroke-width="1.8"
                  stroke-linecap="round"
                  stroke-dasharray="35.2"
                  stroke-dashoffset={35.2 * (1 - Math.min(1, next.seconds / QUIET_SECONDS))}
                  transform="rotate(-90 7 7)"
                />
              </svg>
            {/if}
            <span class="band-trim">{next.lead}</span>
          </span>
        {/if}
      </div>
      {#if armed}
        <div class="card-actions">
          <button
            class="btn"
            type="button"
            disabled={acting !== null}
            onclick={() => void act('check')}
          >
            <Icon name="refresh" size={14} strokeWidth={2} />
            <span class="button-label">{acting === 'check' ? 'Checking…' : 'Check now'}</span>
          </button>
          <!-- Bordered rather than filled: the one filled danger control in this
               flow is the confirmation, so a page cannot be left holding two. -->
          <button
            class="btn btn-stop-quiet"
            type="button"
            disabled={acting !== null}
            onclick={() => void act('cancel')}
          >
            <Icon name="close" size={14} strokeWidth={2} />
            <span class="button-label">{acting === 'cancel' ? 'Cancelling…' : 'Cancel'}</span>
          </button>
        </div>
      {/if}
    </div>

    <dl class="facts">
      <div class="fact">
        <dt>Method</dt>
        <dd>
          <span class="cap-trim"
            >{request.merge_method.slice(0, 1).toUpperCase()}{request.merge_method.slice(1)}</span
          >
        </dd>
      </div>
      <div class="fact">
        <dt>Checks</dt>
        <dd>
          <span class="cap-trim"
            >{request.required_checks_only ? 'Required only' : 'All checks'}</span
          >
        </dd>
      </div>
      <div class="fact">
        <dt>Commit</dt>
        <dd class="mono"><span class="cap-trim">{request.head_sha.slice(0, 8)}</span></dd>
      </div>
      <div class="fact">
        <dt>Armed</dt>
        <dd>
          <span class="cap-trim" title={formatTimestamp(request.requested_at)}
            >{sinceLabel(request.requested_at, now)}</span
          >
        </dd>
      </div>
      <div class="fact">
        <dt>Schedule</dt>
        <dd>
          <Chip tone={request.schedule === 'active' ? 'neutral' : 'absent'} small
            >{request.schedule === 'active' ? 'Active' : 'Deferred'}</Chip
          >
        </dd>
      </div>
      <div class="fact">
        <dt>Cleanup</dt>
        <dd>
          {#if armed}
            <span class="cap-trim">Not yet</span>
          {:else}
            {@const cleanup = cleanupState(request)}
            <Chip tone={cleanup.tone} icon={cleanup.icon} small>{cleanup.label}</Chip>
          {/if}
        </dd>
      </div>
    </dl>
  </section>

  <h3 class="timeline-heading">Timeline</h3>
  <p class="timeline-lede">Every durable event, newest last, with the delivery that caused it</p>

  <section class="plate timeline-plate" aria-label="Timeline">
    <ol class="timeline">
      {#each detail?.events ?? [] as event (event.id)}
        {@const mark = eventMark(event)}
        <li>
          <!-- The rail runs from the middle of one mark to the middle of the
               next, which is the only place a connector between two beads can
               start and stop. Both figures are the tokens the entry is already
               built from: half a mark down to reach its own centre, and back out
               by this entry's bottom padding, the next one's top padding and half
               of ITS mark. -->
          <span class="rail" aria-hidden="true"></span>
          <span class="mark {mark.tone}" aria-hidden="true">
            <Icon name={mark.icon} size={14} strokeWidth={2} />
          </span>
          <div class="head">
            <strong>{eventTitle(event)}</strong>
            <Chip tone="neutral" small>{triggerLabel(event)}</Chip>
          </div>
          <time datetime={event.created_at} title={formatTimestamp(event.created_at)}
            >{clockOf(event.created_at)}</time
          >
          <div class="body">
            <p>{event.summary}</p>
            {#if event.event_key !== undefined || event.delivery_id !== undefined}
              <div class="facts-line">
                {#if event.event_key !== undefined}
                  <span class="key">{event.event_key}</span>
                {/if}
                {#if event.delivery_id !== undefined}
                  <span class="key">delivery {event.delivery_id}</span>
                {/if}
              </div>
            {/if}
          </div>
        </li>
      {:else}
        <li class="timeline-empty">
          {loading ? 'Reading the record…' : 'Nothing has happened to this request yet'}
        </li>
      {/each}
    </ol>
  </section>
{/if}

<style>
  /* No bottom margin: `.plate` carries one for a stack of cards, and the heading
     under this one states its own gap. The two were adding up to 40px where the
     approved design has 24. */
  .summary-plate {
    margin-bottom: 0;
    padding: var(--space-5);
  }

  /* One measure for the whole card: the plate's own padding. The gap above the
     buttons, the gap from them down to the rule, the gap from the rule to the
     labels, and the gap under the last value are then the same number, and the
     rule sits in the middle of its own space rather than nearer one side. */
  .card-band {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3) var(--space-4);
    justify-content: space-between;
    padding-bottom: var(--space-5);
  }

  .card-state {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
  }

  .card-actions {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  /* Bordered danger: the tone is in the ink and the edge, and the one filled
     danger control in this flow is the confirmation. */
  .btn-stop-quiet {
    border-color: color-mix(in srgb, var(--danger) 45%, var(--control-border));
    color: var(--danger);
  }

  .btn-stop-quiet:hover:not(:disabled) {
    background: var(--danger-tint);
    border-color: var(--danger);
  }

  .next-lead {
    align-items: baseline;
    display: flex;
    font-size: var(--font-size-meta);
    font-weight: 600;
    gap: 0.4rem;
  }

  .ring {
    block-size: 1cap;
    flex: none;
    inline-size: 1cap;
    overflow: visible;
  }

  .next-lead.due {
    color: var(--clear);
    transition: color var(--duration-normal) var(--ease-out);
  }

  .next-lead.idle {
    color: var(--text-soft);
  }

  .next-lead.due.imminent {
    color: var(--warning);
  }

  .next-lead.due.final {
    animation: countdown-pulse 700ms var(--ease-out) infinite alternate;
    color: var(--stop);
  }

  @keyframes countdown-pulse {
    from {
      opacity: 1;
    }

    to {
      opacity: 0.35;
    }
  }

  /* A `dl` carries a 1em margin nobody asked for. It was 15px of it above the
     rule and another 15 below the last value, which is what made the card read
     as two blocks with a hole between them rather than one card. */
  .facts {
    border-top: 1px solid var(--rule);
    display: grid;
    grid-template-columns: repeat(6, minmax(0, 1fr));
    margin: 0;
    padding-top: var(--space-5);
  }

  .fact {
    padding-inline: var(--space-4);
  }

  /* The outer cells sit flush with the plate's content edge, so the padding
     lands around the rules between the cells rather than inside the card's own
     frame - the separators are inset, never full bleed. */
  .fact:first-child {
    padding-inline-start: 0;
  }

  .fact:last-child {
    padding-inline-end: 0;
  }

  .fact + .fact {
    border-inline-start: 1px solid var(--rule);
  }

  .fact dt {
    color: var(--dim);
    font: 700 var(--font-size-micro) / 1 var(--sans);
    letter-spacing: 0.08em;
    text-box: trim-both cap alphabetic;
    text-transform: uppercase;
  }

  .fact dd {
    align-items: center;
    display: flex;
    font-size: var(--font-size-meta);
    margin: var(--space-2) 0 0;
    min-width: 0;
  }

  .fact dd.mono {
    font-family: var(--mono);
  }

  /* Six facts want one row, and only stack when the room genuinely runs out.
     Each width restates the whole cell recipe rather than layering onto the one
     above it: a rule keyed to `+ .fact` alone draws a left edge on the first
     cell of every wrapped row.

     A range, not an open-ended `max-width`: two stacked queries both match at
     the narrow end, and the wider one's `nth-child(3n)` then decides padding for
     a two-column grid it knows nothing about. */
  @media (min-width: 40.0625rem) and (max-width: 64rem) {
    .facts {
      gap: var(--space-4) 0;
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }

    .fact:nth-child(3n + 1) {
      border-inline-start: 0;
      padding-inline-start: 0;
    }

    .fact:nth-child(3n) {
      padding-inline-end: 0;
    }
  }

  @media (max-width: 40rem) {
    .facts {
      gap: var(--space-4) 0;
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    .fact:nth-child(odd) {
      border-inline-start: 0;
      padding-inline-start: 0;
    }

    .fact:nth-child(even) {
      padding-inline-end: 0;
    }
  }

  .timeline-heading {
    font: 700 var(--font-size-card-title) / 1.3 var(--sans);
    letter-spacing: -0.02em;
    margin: var(--space-6) 0 var(--space-2);
    text-box: trim-both cap alphabetic;
  }

  .timeline-lede {
    color: var(--text-soft);
    font-size: var(--font-size-meta);
    margin: 0 0 var(--space-4);
    text-box: trim-both cap alphabetic;
  }

  /* `.plate` carries no padding of its own - each caller states what its content
     needs - and this one needs the same as the card above it, so the two read as
     one column rather than as a padded card over an unpadded one. */
  .timeline-plate {
    padding: var(--space-5);
  }

  /* The entry carries the padding its hover wash needs and the list gives back
     exactly that much, so the content stays where it was and the highlight has
     room on every side instead of ending on the letters. */
  /* One measure on both axes, so the hover band is inset from the card by the
     same amount all the way round: the plate's padding less the entry's own.
     They used to differ - 8 vertical against 12 horizontal - which left the wash
     4px nearer the card's side edges than its top and bottom.

     The negative margin equals the entry padding exactly, which is what keeps
     the content where it would be with no wash at all: the band grows outward
     into the plate's padding rather than pushing the words inward. */
  .timeline {
    --tl-mark: 1.75rem;
    --tl-inset: var(--space-3);

    display: grid;
    gap: 0;
    list-style: none;
    margin: calc(-1 * var(--tl-inset));
    padding: 0;
  }

  /* Three columns, because the timestamp deserves one: pushed to the right of a
     flex row it moved with the title beside it, and a column of times that does
     not line up is worse than no column at all. */
  .timeline li {
    border-radius: var(--r-ctl);
    column-gap: var(--space-3);
    display: grid;
    grid-template-columns: var(--tl-mark) minmax(0, 1fr) auto;
    /* Explicit, so the rail's `1 / -1` spans the entry instead of collapsing. */
    grid-template-rows: auto auto;
    padding: var(--tl-inset);
    transition: background-color var(--duration-fast) var(--ease-out);
  }

  /* The time sits a long way from the event it belongs to, so the entry lights
     up as one band under the pointer. It is a reading aid, not a control. */
  .timeline li:hover {
    background: var(--table-row-hover);
  }

  .timeline li:hover > time {
    color: var(--text);
  }

  .timeline .rail {
    background: var(--rule);
    grid-area: 1 / 1 / -1 / 2;
    justify-self: center;
    margin-block: calc(var(--tl-mark) / 2) calc(-1 * (2 * var(--tl-inset) + var(--tl-mark) / 2));
    width: 1px;
  }

  .timeline li:last-child .rail {
    display: none;
  }

  /* The mark's opaque fill is what interrupts the rail - no offset decides where
     the line stops. */
  .timeline .mark {
    align-items: center;
    align-self: center;
    background: var(--strip);
    block-size: var(--tl-mark);
    border-radius: 50%;
    box-shadow: inset 0 0 0 1px color-mix(in srgb, currentcolor 30%, transparent);
    color: var(--dim);
    display: flex;
    grid-area: 1 / 1 / 2 / 2;
    inline-size: var(--tl-mark);
    justify-content: center;
  }

  .timeline .mark.pass {
    background: var(--clear-tint);
    color: var(--clear);
  }

  .timeline .mark.fail {
    background: var(--stop-tint);
    color: var(--stop);
  }

  .timeline .mark.warn {
    background: var(--warning-tint);
    color: var(--warning);
  }

  .timeline .mark.act {
    background: var(--accent-tint);
    color: var(--accent);
  }

  .timeline .head {
    align-items: center;
    align-self: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    grid-area: 1 / 2 / 2 / 3;
  }

  .timeline .head strong {
    font-size: var(--font-size-meta);
    font-weight: 700;
    line-height: 1;
    text-box: trim-both cap alphabetic;
  }

  /* The same size as the title beside it: at 11px the two runs share a line but
     their cap bands no longer share a centre, and the column is a real datum
     rather than a footnote. */
  .timeline > li > time {
    align-self: center;
    color: var(--dim);
    font: 400 var(--font-size-meta) / 1 var(--mono);
    grid-area: 1 / 3 / 2 / 4;
    text-box: trim-both cap alphabetic;
  }

  /* The entry's own padding carries the space between entries, so the body does
     not also hold a trailing block of it - the two were stacking, and an event
     with one line of summary sat in twice the room it needed. */
  .timeline .body {
    grid-area: 2 / 2 / 3 / -1;
    padding-block: var(--space-2) 0;
  }

  .timeline .body p {
    color: var(--text-soft);
    font-size: var(--font-size-compact);
    margin: 0;
    text-box: trim-both cap alphabetic;
  }

  .timeline .facts-line {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    margin-top: var(--space-2);
  }

  /* Trimmed to its own band, so the equal padding above and below is the whole
     of what centres the key on its ground. Untrimmed, the mono face put the
     characters 0.52px above the middle of the chip they sit in - a device row at
     2x, on every key in the record. */
  .timeline .key {
    background: var(--well);
    border-radius: var(--r-chip);
    color: var(--text-soft);
    font: 500 var(--font-size-micro) / 1 var(--mono);
    max-width: 100%;
    overflow: clip;
    overflow-clip-margin: 0.4em;
    padding: 0.3rem 0.4rem;
    text-box: trim-both cap alphabetic;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .timeline-empty {
    color: var(--dim);
    display: block;
    font-size: var(--font-size-meta);
    padding: var(--space-4) var(--space-3);
  }
</style>
