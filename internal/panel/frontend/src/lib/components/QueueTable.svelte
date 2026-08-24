<script lang="ts">
  import type { QueueActionType, QueueItem } from '#lib/types.js';
  import ActionMenu, { type ActionMenuItem } from './ActionMenu.svelte';
  import Button from './Button.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
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

  function stateTone(state: QueueItem['state']): ChipTone {
    if (state === 'running' || state === 'ready') return 'signal';
    if (state === 'succeeded') return 'clear';
    if (state === 'failed') return 'stop';
    if (state === 'blocked' || state === 'retrying') return 'warning';
    if (state === 'awaiting_approval') return 'accent';
    if (state === 'cancelled' || state === 'superseded') return 'absent';
    return 'neutral';
  }

  function priorityTone(priority: QueueItem['priority']): ChipTone {
    if (priority === 'urgent') return 'stop';
    if (priority === 'high') return 'accent';
    if (priority === 'low') return 'absent';
    return 'neutral';
  }

  function actionLabel(action: QueueActionType): string {
    if (action === 'next_window') return 'Next window';
    if (action === 'schedule_at') return 'Schedule exact time';
    if (action === 'set_priority') return 'Change priority';
    if (action === 'cancel') return 'Cancel work';
    return 'Run now';
  }

  function actionItems(item: QueueItem): ActionMenuItem[] {
    return (item.actions ?? [])
      .filter((action) => action !== 'run_now')
      .map((action) => ({
        id: action,
        label: actionLabel(action),
        description:
          action === 'next_window'
            ? 'Keep the assigned execution window'
            : action === 'schedule_at'
              ? 'Choose the earliest acceptable time'
              : action === 'set_priority'
                ? 'Move this item to another priority band'
                : 'Keep the item in audited history',
        icon:
          action === 'next_window'
            ? 'pending'
            : action === 'schedule_at'
              ? 'history'
              : action === 'set_priority'
                ? 'sliders'
                : 'trash',
        tone: action === 'cancel' ? 'danger' : 'default',
      }));
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
    <div class="queue-cell state-cell">
      <div class="state-line">
        <Chip tone={stateTone(item.state)} dot={item.state === 'running'}>{words(item.state)}</Chip>
        <span class="priority-{item.priority}">
          <Chip tone={priorityTone(item.priority)} small>{words(item.priority)}</Chip>
        </span>
      </div>
      {#if item.blocked_reason}
        <span class="queue-reason">{item.blocked_reason}</span>
      {:else if item.state === 'running' && item.progress_total > 0}
        <span class="queue-reason">{item.progress_current} of {item.progress_total}</span>
      {:else if item.attempt > 0}
        <span class="queue-reason">Attempt {item.attempt}</span>
      {/if}
    </div>
  </td>
  <td data-label="Timing">
    <dl class="timing-facts">
      <div>
        <dt>Earliest</dt>
        <dd><time datetime={item.eligible_at}>{absolute(item.eligible_at)}</time></dd>
      </div>
      {#if item.profile_timezone}
        <div>
          <dt>Window</dt>
          <dd>
            {item.profile_name ?? words(item.profile_id ?? 'Window')} · {absolute(
              item.eligible_at,
              item.profile_timezone,
            )}
          </dd>
        </div>
      {/if}
      <div>
        <dt>Estimate</dt>
        <dd>
          {item.estimated_start_at ? absolute(item.estimated_start_at) : 'Not estimated'} ·
          {item.work_ahead === 0 ? 'next in lane' : `${item.work_ahead} ahead`}
        </dd>
      </div>
    </dl>
    <span class="eligibility">{countdown(item.eligible_at)}</span>
  </td>
  <td data-label="Actions">
    <div class="queue-actions">
      <Button row onclick={() => onOpen(item)}>Details</Button>
      {#if item.actions?.includes('run_now')}
        <Button row tone="signal" onclick={() => onAction(item, 'run_now')}>Run now</Button>
      {/if}
      {#if actionItems(item).length > 0}
        <ActionMenu
          label={`More actions for ${item.title}`}
          items={actionItems(item)}
          onSelect={(action) => onAction(item, action as QueueActionType)}
        />
      {/if}
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
  columns={[{ label: 'Work' }, { label: 'Status' }, { label: 'Timing' }, { label: 'Actions' }]}
  columnWidths={['27%', '21%', '32%', '20%']}
  {cells}
  {empty}
  class="general-queue-table"
  scrollable={false}
  stacked
/>

<style>
  :global(.general-queue-table) {
    --table-cell-font-size: var(--font-size-meta);
    --table-cell-pad-block: var(--space-3);
    --table-cell-pad-inline: var(--space-4);
    --table-layout: fixed;
    --table-min-width: 0;
    min-height: 12rem;
  }
  th,
  td {
    vertical-align: middle;
  }
  .queue-title,
  .queue-summary,
  .queue-reason {
    display: block;
  }
  .queue-title {
    color: var(--text-primary);
    font-size: var(--font-size-body);
    font-weight: 700;
    line-height: 1.25;
  }
  .queue-summary,
  .queue-reason {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    line-height: 1.35;
    margin-top: 0.35rem;
  }
  .state-cell {
    display: grid;
    gap: var(--space-1);
  }
  .state-line {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }
  .timing-facts {
    display: grid;
    gap: 0.35rem;
    margin: 0;
  }
  .timing-facts div {
    align-items: baseline;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: 3.5rem minmax(0, 1fr);
  }
  .timing-facts dt {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    font-weight: 650;
    letter-spacing: 0.045em;
    text-transform: uppercase;
  }
  .timing-facts dd {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    line-height: 1.35;
    margin: 0;
  }
  .timing-facts time {
    font-variant-numeric: tabular-nums;
  }
  .eligibility {
    color: var(--brand-action-text);
    display: block;
    font-size: var(--font-size-compact);
    font-weight: 650;
    margin-top: var(--space-2);
  }
  .queue-actions {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
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
  @media (max-width: 64rem) {
    .queue-actions {
      justify-content: flex-start;
    }
  }
</style>
