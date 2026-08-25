<script lang="ts">
  import { plainClick } from '#lib/follow.js';
  import type { QueueItem } from '#lib/types.js';
  import { flip } from 'svelte/animate';
  import { cubicOut } from 'svelte/easing';
  import { MediaQuery } from 'svelte/reactivity';
  import { fade } from 'svelte/transition';
  import Chip, { type ChipTone } from './Chip.svelte';
  import Icon from './Icon.svelte';

  const {
    items,
    total,
    approvals,
    review,
    loading = false,
    error = '',
    now,
    queueHref,
    onOpenQueue,
  }: {
    items: readonly QueueItem[];
    total: number;
    approvals: number;
    review: number;
    loading?: boolean;
    error?: string;
    now: number;
    queueHref: string;
    onOpenQueue: () => void;
  } = $props();

  const reducedMotion = new MediaQuery('prefers-reduced-motion: reduce');
  const visible = $derived(items.slice(0, 3));
  const needsReview = $derived(approvals > 0 || review > 0);
  const summary = $derived.by(() => {
    if (loading && items.length === 0) return 'Reading the durable queue';
    if (error !== '' && items.length === 0) return 'Queue status unavailable';
    if (total === 0 && approvals === 0) return 'No durable work is active';

    const active = `${total} active`;
    return approvals === 0 ? active : `${active} · ${approvals} awaiting approval`;
  });
  const rowMotion = $derived({
    duration: reducedMotion.current ? 0 : 140,
    easing: cubicOut,
  });
  const rowArriving = $derived({ duration: reducedMotion.current ? 0 : 120, delay: 20 });
  const rowLeaving = $derived({ duration: reducedMotion.current ? 0 : 80 });
  const valueMotion = $derived({ duration: reducedMotion.current ? 0 : 90 });

  function open(event: MouseEvent): void {
    if (!plainClick(event)) return;
    event.preventDefault();
    onOpenQueue();
  }

  function words(value: string): string {
    return value.replaceAll('_', ' ').replace(/^./, (letter) => letter.toUpperCase());
  }

  function stateTone(state: QueueItem['state']): ChipTone {
    if (state === 'running' || state === 'ready') return 'signal';
    if (state === 'blocked' || state === 'retrying') return 'warning';
    if (state === 'awaiting_approval') return 'accent';
    return 'neutral';
  }

  function relativeEligibility(item: QueueItem): string {
    if (item.state === 'running') {
      return item.progress_total > 0
        ? `${item.progress_current} of ${item.progress_total}`
        : 'In progress';
    }
    const seconds = Math.round((new Date(item.eligible_at).getTime() - now) / 1_000);
    if (seconds <= 0) return 'Eligible now';
    if (seconds < 60) return `In ${seconds}s`;
    if (seconds < 3_600) return `In ${Math.ceil(seconds / 60)}m`;
    if (seconds < 86_400) return `In ${Math.ceil(seconds / 3_600)}h`;
    return `In ${Math.ceil(seconds / 86_400)}d`;
  }

  function queueNote(item: QueueItem): string {
    if (item.blocked_reason) return item.blocked_reason;
    if (item.state === 'running') return words(item.kind);
    if (item.work_ahead === 0) return 'Next in lane';
    return `${item.work_ahead} ahead`;
  }
</script>

<article class="plate queue-panel" class:is-empty={visible.length === 0}>
  <header class="panel-head">
    <div class="panel-title">
      <h3>Queue</h3>
      <p aria-live="polite">{summary}</p>
    </div>
    <span class="panel-head-end">
      <span class="summary-state">
        {#key `${needsReview}:${approvals}`}
          <span class="summary-state-value" in:fade={valueMotion} out:fade={valueMotion}>
            <Chip tone={needsReview ? 'warning' : 'clear'} icon={needsReview ? 'alert' : 'check'}>
              {needsReview ? 'Review' : 'Live'}
            </Chip>
          </span>
        {/key}
      </span>
      <a class="panel-link" href={queueHref} onclick={open} aria-label="View the whole queue">
        <span class="cap-trim">View all</span>
        <Icon name="chevron-right" size={14} />
      </a>
    </span>
  </header>

  {#if error !== ''}
    <p class="queue-note" role="alert">Live refresh delayed: {error}</p>
  {/if}

  {#if visible.length === 0}
    <p class="panel-empty band-trim">
      {#if loading && visible.length === 0}
        Reading scheduled work…
      {:else if approvals > 0}
        {approvals} {approvals === 1 ? 'plan is' : 'plans are'} waiting for approval in Queue.
      {:else}
        Scheduled and event-driven work will appear here when it becomes active.
      {/if}
    </p>
  {:else}
    <ol class="panel-next">
      {#each visible as item (item.id)}
        <li
          animate:flip={rowMotion}
          in:fade={rowArriving}
          out:fade={rowLeaving}
          data-queue-item={item.id}
        >
          <a
            class="panel-row"
            href={queueHref}
            onclick={open}
            aria-label={`Open ${item.title} in Queue`}
          >
            <span class="state-swap">
              {#key `${item.id}:${item.state}:${item.revision}`}
                <span class="state-value" in:fade={valueMotion} out:fade={valueMotion}>
                  <Chip tone={stateTone(item.state)} dot={item.state === 'running'}>
                    {words(item.state)}
                  </Chip>
                </span>
              {/key}
            </span>
            <span class="work-copy band-trim-kids">
              <strong>{item.title}</strong>
              <small>{item.summary ?? words(item.kind)}</small>
            </span>
            <span class="timing-copy">
              <span class="timing-swap">
                {#key `${item.id}:${item.state}:${item.progress_current}:${item.eligible_at}`}
                  <strong class="timing-value" in:fade={valueMotion} out:fade={valueMotion}>
                    {relativeEligibility(item)}
                  </strong>
                {/key}
              </span>
              <small>{queueNote(item)}</small>
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
    --line-gap: 0.45rem;

    display: grid;
    gap: var(--space-3);
    padding: var(--space-5);
  }
  .panel-head {
    align-items: center;
    column-gap: var(--space-3);
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .panel-title {
    min-width: 0;
  }
  .panel-head h3 {
    font: 700 var(--font-size-card-title) / 1 var(--sans);
    letter-spacing: -0.02em;
    margin: 0;
  }
  .panel-head p {
    color: var(--text-soft);
    font-size: var(--font-size-meta);
    margin: var(--line-gap) 0 0;
  }
  .panel-head-end {
    align-items: center;
    display: flex;
    gap: var(--space-3);
  }
  .summary-state,
  .summary-state-value,
  .state-swap,
  .state-value,
  .timing-swap,
  .timing-value {
    display: grid;
    grid-area: 1 / 1;
  }
  .summary-state-value,
  .state-value {
    justify-self: start;
  }
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
  .panel-link:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 3px;
  }
  .panel-link:active {
    color: var(--brand-action-hover);
    transform: scale(var(--press-scale-compact));
  }
  .queue-note {
    background: color-mix(in srgb, var(--warning) 8%, transparent);
    border-inline-start: 2px solid var(--warning);
    color: var(--text-soft);
    font-size: var(--font-size-compact);
    margin: 0;
    padding: var(--space-2) var(--space-3);
  }
  .panel-next {
    display: grid;
    list-style: none;
    margin: 0 calc(-1 * var(--space-3));
    padding: 0;
  }
  .panel-next li {
    position: relative;
    will-change: transform, opacity;
  }
  .panel-next li + li::before {
    background: var(--rule);
    block-size: 1px;
    content: '';
    inset-block-start: 0;
    inset-inline: var(--space-3);
    position: absolute;
  }
  .panel-row {
    align-items: center;
    border-radius: var(--r-ctl);
    color: inherit;
    display: grid;
    gap: var(--space-3);
    grid-template-columns: 8.75rem minmax(0, 1fr) minmax(7.5rem, auto) auto;
    min-height: 3.25rem;
    padding: var(--space-2) var(--space-3);
    text-decoration: none;
    transition: background-color var(--duration-fast) var(--ease-out);
  }
  .panel-row:hover {
    background: var(--table-row-hover);
  }
  .panel-row:active {
    background: var(--table-row-pressed);
  }
  .panel-row:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: -2px;
  }
  .panel-row:hover .row-chevron {
    color: var(--accent);
  }
  .work-copy,
  .timing-copy {
    display: grid;
    gap: 0.3rem;
    min-width: 0;
  }
  .work-copy strong {
    color: var(--text);
    font-size: var(--font-size-meta);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .work-copy small,
  .timing-copy small {
    color: var(--text-soft);
    font-size: var(--font-size-compact);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .timing-copy {
    justify-items: end;
    text-align: right;
  }
  .timing-value {
    color: var(--brand-action-text);
    font-size: var(--font-size-meta);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  .row-chevron {
    color: var(--text-muted);
    display: inline-flex;
  }
  .queue-panel.is-empty {
    gap: var(--space-4);
  }
  .panel-empty {
    color: var(--text-soft);
    font-size: var(--font-size-meta);
    margin: 0;
  }
  @media (max-width: 48rem) {
    .panel-head {
      align-items: start;
    }
    .panel-head-end {
      gap: var(--space-2);
    }
    .panel-row {
      gap: var(--space-2);
      grid-template-columns: minmax(0, 1fr) auto;
    }
    .state-swap,
    .work-copy {
      grid-column: 1;
    }
    .state-swap {
      grid-row: 1;
    }
    .work-copy {
      grid-row: 2;
    }
    .timing-copy {
      grid-column: 2;
      grid-row: 1 / span 2;
    }
    .row-chevron {
      display: none;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .panel-next li {
      will-change: auto;
    }
  }
</style>
