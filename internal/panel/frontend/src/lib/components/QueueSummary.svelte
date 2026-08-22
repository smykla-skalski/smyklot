<script lang="ts">
  import { plainClick } from '#lib/follow.js';
  import { bySoonest, queueNext, queueState } from '#lib/queue.js';
  import type { PendingCIRequest, RootOverview } from '#lib/types.js';
  import Chip from './Chip.svelte';
  import Icon from './Icon.svelte';

  const {
    queue,
    now,
    queueHref,
    onOpenQueue,
    requestHref,
    onOpenRequest,
  }: {
    queue: RootOverview['pending_ci'];
    now: number;
    queueHref: string;
    onOpenQueue: () => void;
    requestHref: (requestId: string) => string;
    onOpenRequest: (requestId: string) => void;
  } = $props();

  /* The reconciler's `PassingQuiet`, which is the whole span the ring draws. */
  const QUIET_SECONDS = 30;
  /* Three, because that is what says "what lands next" without turning the card
     into the table it links to. */
  const SHOWN = 3;

  const waiting = $derived([...queue.active, ...queue.deferred].sort(bySoonest));
  const next = $derived(waiting.slice(0, SHOWN));
  /* The one thing a reader takes at a glance: Review when something needs a
     person, Clear otherwise. */
  const needsReview = $derived(
    waiting.some((request) => ['failing', 'indeterminate'].includes(request.last_observed_state)),
  );
  const summary = $derived.by(() => {
    if (waiting.length === 0) return 'Nothing waiting on CI';
    const failing = waiting.filter((request) => request.last_observed_state === 'failing').length;
    const count = `${waiting.length} waiting`;
    if (failing === 0) return count;

    return `${count}, ${failing === 1 ? 'one of them failing' : `${failing} of them failing`}`;
  });

  function ownerOf(fullName: string): string {
    const slash = fullName.lastIndexOf('/');
    return slash === -1 ? '' : `${fullName.slice(0, slash)}/`;
  }

  function repositoryOf(fullName: string): string {
    const slash = fullName.lastIndexOf('/');
    return slash === -1 ? fullName : fullName.slice(slash + 1);
  }

  function open(event: MouseEvent): void {
    if (!plainClick(event)) return;
    event.preventDefault();
    onOpenQueue();
  }

  function openRequest(event: MouseEvent, requestId: string): void {
    if (!plainClick(event)) return;
    event.preventDefault();
    onOpenRequest(requestId);
  }

  function leadOf(request: PendingCIRequest): ReturnType<typeof queueNext> {
    return queueNext(request, now);
  }
</script>

<!-- Full width and first, above the two health panels: everything else on the
     Overview is a standing state, and this is the only thing on the page that is
     about to happen by itself. -->
<article class="plate queue-panel" class:is-empty={waiting.length === 0}>
  <header class="panel-head">
    <div class="band-trim-kids">
      <h3>Queue</h3>
      <p>{summary}</p>
    </div>
    <!-- The state and the way out share the header's right, in that order: the
         chip is what a reader takes at a glance and the link is what they do
         about it. Same quiet link the health panel below uses for "View all" - a
         card's escape hatch is a link, not a button, and the two cards should not
         spell it two ways. -->
    <span class="panel-head-end">
      {#if needsReview}
        <Chip tone="warning" icon="alert">Review</Chip>
      {:else}
        <Chip tone="clear" icon="check">Clear</Chip>
      {/if}
      <!-- "View all", because the card beside it already says that for the same
           gesture. The card is headed Queue, so the words do not have to name it
           again - and two cards on one page spelling their escape hatch two ways
           is the drift this whole pass exists to end. -->
      <a class="panel-link" href={queueHref} onclick={open} aria-label="View the whole queue">
        <span class="cap-trim">View all</span>
        <Icon name="chevron-right" size={14} />
      </a>
    </span>
  </header>

  {#if next.length === 0}
    <p class="panel-empty band-trim">
      Nothing is armed. New <span class="mono">after ci</span> commands appear here.
    </p>
  {:else}
    <!-- Three rows, not a proportional bar. The ownership panel beside this one
         already spends its body on a track, and a track is the right shape for a
         distribution that is always there. A queue is not a distribution: it is
         usually short, often empty, and the useful facts are what lands next and
         whether anything is stuck. -->
    <ol class="panel-next">
      {#each next as request (request.id)}
        {@const state = queueState(request)}
        {@const lead = leadOf(request)}
        <li>
          <!-- A real link, so the row can be opened in a tab like the metric
               cards above it, and so the chevron at its end is a promise the
               browser keeps rather than one a click handler makes. -->
          <a
            class="panel-row"
            href={requestHref(request.id)}
            onclick={(event) => openRequest(event, request.id)}
          >
            <Chip tone={state.tone} icon={state.icon}>{state.label}</Chip>
            <span class="pr-name band-trim-kids">
              <span class="pr-owner">{ownerOf(request.repository_full_name)}</span>
              <span class="pr-repo">{repositoryOf(request.repository_full_name)}</span>
              <span class="pr-num">#{request.pull_request}</span>
            </span>
            <span
              class="next-lead"
              class:due={lead.merging}
              class:idle={!lead.merging}
              class:imminent={lead.merging && lead.seconds !== null && lead.seconds <= 10}
              class:final={lead.merging && lead.seconds !== null && lead.seconds <= 5}
            >
              {#if lead.merging && lead.seconds !== null}
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
                    stroke-dashoffset={35.2 * (1 - Math.min(1, lead.seconds / QUIET_SECONDS))}
                    transform="rotate(-90 7 7)"
                  />
                </svg>
              {/if}
              <span class="band-trim">{lead.lead}</span>
            </span>
            <span class="row-chevron" aria-hidden="true">
              <Icon name="chevron-right" size={14} />
            </span>
          </a>
        </li>
      {/each}
    </ol>
  {/if}
</article>

<style>
  .queue-panel {
    --line-gap: 0.5rem;

    display: grid;
    gap: var(--space-3);
    padding: var(--space-5);
  }

  /* The chip centres on the TITLE, not on the title-and-subtitle block - the
     same rule the page header follows, and the reason both are grids: a flex row
     would centre it on whatever the left column happens to be. */
  .panel-head {
    align-items: center;
    column-gap: var(--space-3);
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .panel-head > div {
    display: contents;
  }

  .panel-head h3 {
    align-self: center;
    font: 700 var(--font-size-card-title) / 1 var(--sans);
    grid-area: 1 / 1;
    letter-spacing: -0.02em;
    margin: 0;
  }

  .panel-head p {
    color: var(--text-soft);
    font-size: var(--font-size-meta);
    grid-area: 2 / 1;
    margin: var(--line-gap) 0 0;
  }

  .panel-head-end {
    align-items: center;
    align-self: center;
    display: flex;
    gap: var(--space-3);
    grid-area: 1 / 2;
  }

  /* The health panel's "View all", spelled the same way: accent ink, 650 at the
     compact size, and a chevron a fifth of a space away. */
  .panel-link {
    align-items: center;
    border-radius: var(--r-ctl);
    color: var(--accent);
    display: inline-flex;
    font: 650 var(--font-size-compact) / 1 var(--sans);
    gap: 0.2rem;
    text-decoration: none;
    white-space: nowrap;
  }

  .panel-link:hover {
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .panel-link:active {
    color: var(--brand-action-hover);
    transform: scale(var(--press-scale-compact));
  }

  /* Same as the timeline: the row takes the padding its highlight needs and the
     list gives back exactly that much, so the wash reaches past the content
     instead of stopping on it. */
  .panel-next {
    display: grid;
    list-style: none;
    margin: 0 calc(-1 * var(--space-3));
    padding: 0;
  }

  .panel-next li {
    position: relative;
  }

  /* The chevron takes a column of its own rather than sitting absolutely at the
     row's edge the way the metric cards' does: those cards have one block of
     content and room to spare, and here the row already ends in a right-aligned
     time that the mark would otherwise print on top of. */
  .panel-row {
    align-items: center;
    border-radius: var(--r-ctl);
    color: inherit;
    display: grid;
    gap: var(--space-3);
    grid-template-columns: 9.5rem minmax(0, 1fr) auto auto;
    min-height: 2.75rem;
    padding-inline: var(--space-3);
    text-decoration: none;
    transition: background-color var(--duration-fast) var(--ease-out);
  }

  .row-chevron {
    color: var(--text-muted);
    display: inline-flex;
  }

  .panel-row:hover .row-chevron {
    color: var(--accent);
  }

  .panel-row:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: -2px;
  }

  .panel-row:active {
    background: var(--table-row-pressed);
  }

  /* The rule ends where the content does - on the chip's left edge and on the
     last word's right - so it is drawn inset to the row's own padding rather
     than as the row's border. As a border it ran the full width of the box,
     which reaches 12px further out on each side to give the hover wash its room,
     and it took the wash's 8px corner radius with it: a separator with rounded
     ends and a different length from everything above it. */
  .panel-next li + li::before {
    background: var(--rule);
    block-size: 1px;
    content: '';
    inset-block-start: 0;
    inset-inline: var(--space-3);
    position: absolute;
  }

  /* A grid item stretches by default, and a stretched chip is a bar. */
  .panel-next :global(.chip),
  .panel-next .pr-name {
    justify-self: start;
  }

  .panel-next .next-lead {
    justify-self: end;
  }

  .panel-row:hover {
    background: var(--table-row-hover);
  }

  .pr-name {
    align-items: baseline;
    display: flex;
    font-size: var(--font-size-meta);
    gap: 0.15rem;
    min-width: 0;
  }

  .pr-name > :global(*) {
    line-height: 1;
  }

  .pr-owner {
    color: var(--dim);
    flex: none;
  }

  .pr-repo {
    color: var(--text);
    font-weight: 700;
    min-width: 0;
    overflow: clip;
    overflow-clip-margin: 0.4em;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .pr-num {
    color: var(--text-soft);
    flex: none;
    font-weight: 600;
    margin-left: 0.15rem;
  }

  /* The row's first column is a stated 9.5rem, which a phone does not have to
     spare once the name and the countdown have had theirs - the four of them
     came to 337px on a 320px screen and took the whole overview down to 95%
     with them. Wrapped instead: the chip, the name and the chevron keep the
     line they are read on, and the countdown takes the one below. Ordered
     rather than reordered in the markup, so the chevron stays last in the tab
     and reading order it has on a wide screen. */
  @media (max-width: 48rem) {
    .panel-row {
      display: flex;
      flex-wrap: wrap;
      padding-block: var(--space-2);
    }

    .pr-name {
      flex: 1 1 6rem;
    }

    /* The owner gives before the page does: on the narrowest phones the
       unshrinkable owner + number pair ran 4px past the screen. */
    .pr-owner {
      flex: 0 1 auto;
      min-width: 0;
      overflow: clip;
      overflow-clip-margin: 0.4em;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .row-chevron {
      order: 1;
    }

    .next-lead {
      flex: 1 0 100%;
      order: 2;
    }
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

  /* The empty panel keeps the roomier rhythm: it holds one sentence, and the
     tight spacing that suits three rows of data makes a single line look
     stranded against the card's own padding. */
  .queue-panel.is-empty {
    gap: var(--space-4);
  }

  .panel-empty {
    color: var(--text-soft);
    font-size: var(--font-size-meta);
    margin: 0;
  }

  .mono {
    font-family: var(--mono);
  }
</style>
