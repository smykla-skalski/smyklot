<script lang="ts">
  import type { QueueActionType, QueueItem } from '#lib/types.js';
  import Button from './Button.svelte';
  import DataTable from './DataTable.svelte';

  const {
    items,
    clock = Date.now,
    onOpen,
    onAction,
  }: {
    items: readonly QueueItem[];
    clock?: () => number;
    onOpen: (item: QueueItem) => void;
    onAction: (item: QueueItem, action: QueueActionType) => void;
  } = $props();

  function words(value: string): string {
    return value.replaceAll('_', ' ').replace(/^./, (letter) => letter.toUpperCase());
  }

  function absolute(value: string, timeZone?: string): string {
    return new Intl.DateTimeFormat(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
      timeZoneName: 'short',
      ...(timeZone === undefined ? {} : { timeZone }),
    }).format(new Date(value));
  }

  function countdown(value: string): string {
    const seconds = Math.round((new Date(value).getTime() - clock()) / 1000);
    if (seconds <= 0) return 'eligible now';
    if (seconds < 60) return `in ${seconds}s`;
    if (seconds < 3600) return `in ${Math.ceil(seconds / 60)}m`;
    if (seconds < 86_400) return `in ${Math.ceil(seconds / 3600)}h`;
    return `in ${Math.ceil(seconds / 86_400)}d`;
  }
</script>

{#snippet cells(item: QueueItem)}
  <th scope="row" data-label="Work">
    <div class="queue-cell band-trim-stack">
      <span class="queue-title">{item.title}</span>
      <span class="queue-summary">{item.summary ?? words(item.kind)}</span>
    </div>
  </th>
  <td data-label="State">
    <div class="queue-cell band-trim-stack">
      <span class="state-chip state-{item.state}">{words(item.state)}</span>
      {#if item.blocked_reason}
        <span class="queue-reason">{item.blocked_reason}</span>
      {:else if item.state === 'running' && item.progress_total > 0}
        <span class="queue-reason">{item.progress_current} of {item.progress_total}</span>
      {:else if item.attempt > 0}
        <span class="queue-reason">Attempt {item.attempt}</span>
      {/if}
    </div>
  </td>
  <td data-label="Schedule">
    <div class="queue-cell band-trim-stack">
      <time datetime={item.eligible_at}>Viewer: {absolute(item.eligible_at)}</time>
      {#if item.profile_timezone}
        <span class="queue-reason"
          >Window: {absolute(item.eligible_at, item.profile_timezone)} · {item.profile_name ??
            words(item.profile_id ?? 'Window')}</span
        >
      {/if}
      <span class="queue-reason">{countdown(item.eligible_at)}</span>
    </div>
  </td>
  <td data-label="Estimate">
    <div class="queue-cell band-trim-stack">
      <span>{item.estimated_start_at ? absolute(item.estimated_start_at) : 'Not estimated'}</span>
      <span class="queue-reason">
        {item.work_ahead === 0 ? 'Next in lane' : `${item.work_ahead} ahead`} · estimate
      </span>
    </div>
  </td>
  <td data-label="Priority">
    <div class="queue-cell band-trim-stack">
      <span class="priority priority-{item.priority}">{words(item.priority)}</span>
      {#if item.priority_overridden}<span class="queue-reason">One-off override</span>{/if}
    </div>
  </td>
  <td data-label="Actions">
    <div class="queue-actions">
      <Button row onclick={() => onOpen(item)}>Details</Button>
      {#each item.actions ?? [] as action (action)}
        <Button
          row
          tone={action === 'run_now' ? 'signal' : action === 'cancel' ? 'stop-quiet' : 'default'}
          onclick={() => onAction(item, action)}
        >
          {action === 'run_now'
            ? 'Run now'
            : action === 'next_window'
              ? 'Next window'
              : action === 'schedule_at'
                ? 'Schedule'
                : action === 'set_priority'
                  ? 'Priority'
                  : 'Cancel'}
        </Button>
      {/each}
    </div>
  </td>
{/snippet}

{#snippet empty()}
  <div class="queue-empty">
    <strong>Nothing in this view</strong>
    <span>Queued work appears here as soon as the service accepts it.</span>
  </div>
{/snippet}

<DataTable
  rows={items}
  rowKey={(item) => item.id}
  caption="Background work queue"
  regionLabel="Background work queue"
  columns={[
    { label: 'Work' },
    { label: 'State' },
    { label: 'Schedule' },
    { label: 'Estimated start' },
    { label: 'Priority' },
    { label: 'Actions' },
  ]}
  {cells}
  {empty}
  class="general-queue-table"
  pinned
  stacked
/>

<style>
  :global(.general-queue-table) {
    --table-cell-pad-block: 0.72rem;
    --table-cell-pad-inline: var(--space-3);
    min-height: 12rem;
  }
  th,
  td {
    vertical-align: middle;
  }
  .queue-title,
  .queue-summary,
  .queue-reason,
  time {
    display: block;
  }
  .queue-cell > span {
    display: block;
  }
  .queue-cell > :is(.state-chip, .priority) {
    display: flex;
    width: fit-content;
  }
  .queue-title {
    font-size: 0.84rem;
    font-weight: 750;
  }
  .queue-summary,
  .queue-reason {
    color: var(--dim);
    font-size: 0.72rem;
    margin-top: var(--space-1);
  }
  time,
  .priority {
    font-size: 0.78rem;
    font-variant-numeric: tabular-nums;
  }
  .state-chip,
  .priority {
    border: 1px solid var(--border-subtle);
    border-radius: 999px;
    display: inline-flex;
    font-size: 0.68rem;
    font-weight: 760;
    letter-spacing: 0.035em;
    padding: 0.22rem 0.46rem;
    text-transform: uppercase;
  }
  .state-running,
  .state-ready,
  .priority-urgent {
    border-color: color-mix(in srgb, var(--accent) 42%, var(--border-subtle));
    color: var(--accent);
  }
  .state-failed,
  .state-blocked {
    border-color: color-mix(in srgb, var(--danger) 45%, var(--border-subtle));
    color: var(--danger);
  }
  .state-succeeded {
    border-color: color-mix(in srgb, var(--success) 42%, var(--border-subtle));
    color: var(--success);
  }
  .queue-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .queue-empty {
    display: grid;
    gap: var(--space-1);
    padding: var(--space-6);
    text-align: center;
  }
  .queue-empty span {
    color: var(--dim);
  }
</style>
