<script lang="ts">
  import { onMount } from 'svelte';
  import type {
    QueueActionInput,
    QueueActionType,
    QueueItem,
    QueuePriority,
    QueueSchedulePreview,
  } from '#lib/types.js';
  import Button from './Button.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';

  const {
    item,
    action,
    busy,
    error,
    onClose,
    onPreview,
    onSubmit,
  }: {
    item: QueueItem | null;
    action: QueueActionType | null;
    busy: boolean;
    error: string;
    onClose: () => void;
    onPreview: (input: QueueActionInput) => Promise<QueueSchedulePreview>;
    onSubmit: (input: QueueActionInput) => void;
  } = $props();

  let reason = $state('');
  let at = $state('');
  let outsideWindow = $state(false);
  let priority = $state<QueuePriority>('normal');
  let preview = $state<QueueSchedulePreview | null>(null);
  let previewBusy = $state(false);
  let previewError = $state('');
  let previewKey = $state('');

  onMount(() => {
    at = item === null ? '' : localDateTime(item.not_before);
    priority = item?.priority ?? 'normal';
  });

  const needsReason = $derived(action === 'run_now' || (action === 'schedule_at' && outsideWindow));
  const invalid = $derived(
    item === null ||
      action === null ||
      (needsReason && reason.trim() === '') ||
      (action === 'schedule_at' && (at === '' || preview === null || previewKey !== scheduleKey())),
  );

  function localDateTime(value: string): string {
    const date = new Date(value);
    const offset = date.getTimezoneOffset() * 60_000;
    return new Date(date.getTime() - offset).toISOString().slice(0, 16);
  }

  function titleFor(value: QueueActionType | null): string {
    switch (value) {
      case 'run_now':
        return 'Run now';
      case 'next_window':
        return 'Move to next window';
      case 'schedule_at':
        return 'Schedule exact time';
      case 'set_priority':
        return 'Change priority';
      case 'cancel':
        return 'Cancel queued work';
      default:
        return 'Queue action';
    }
  }

  function submit(): void {
    if (item === null || action === null || invalid) return;
    const input: QueueActionInput = {
      type: action,
      expected_revision: item.revision,
    };
    if (reason.trim() !== '') input.reason = reason.trim();
    if (action === 'schedule_at') {
      input.at = new Date(at).toISOString();
      input.outside_window = outsideWindow;
    }
    if (action === 'set_priority') input.priority = priority;
    onSubmit(input);
  }

  function scheduleKey(): string {
    return `${at}:${outsideWindow}`;
  }

  async function refreshPreview(): Promise<void> {
    if (item === null || action !== 'schedule_at' || at === '') return;
    const key = scheduleKey();
    previewBusy = true;
    previewError = '';
    try {
      preview = await onPreview({
        type: 'schedule_at',
        expected_revision: item.revision,
        at: new Date(at).toISOString(),
        outside_window: outsideWindow,
      });
      previewKey = key;
    } catch (cause) {
      preview = null;
      previewKey = '';
      previewError = cause instanceof Error ? cause.message : String(cause);
    } finally {
      previewBusy = false;
    }
  }

  function previewTime(value: string, timeZone?: string): string {
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: 'medium',
      timeStyle: 'long',
      ...(timeZone === undefined ? {} : { timeZone }),
    }).format(new Date(value));
  }
</script>

<!--
@component
Confirming one act on one piece of queued work, with what that act would actually do
shown before it is taken. `onPreview` is why this is a dialog rather than a button:
rescheduling asks the service when the work would then run, and a reader agreeing to a
new time should see the time.

One dialog for every queue action rather than one per verb - the shape is the same and
only the sentence differs, and four dialogs asking the same question is how they come
to ask it four different ways.
-->

<ConfirmDialog
  id="queue-action"
  open={item !== null && action !== null}
  title={titleFor(action)}
  description={item === null ? undefined : item.title}
  {busy}
  busyLabel="Applying…"
  confirmLabel={action === 'run_now' ? 'Run now' : action === 'cancel' ? 'Cancel work' : 'Apply'}
  confirmTone={action === 'cancel' ? 'stop' : action === 'run_now' ? 'signal' : 'default'}
  confirmDisabled={invalid}
  {onClose}
  onConfirm={submit}
>
  <div class="queue-action-form">
    {#if action === 'run_now'}
      <p>
        Smyklot runs this once now, ignoring how often the job runs and the hours it keeps. Work
        already running is never interrupted.
      </p>
    {:else if action === 'next_window'}
      <p>The delay is cleared, but the job's hours still apply.</p>
    {:else if action === 'schedule_at'}
      <label for="queue-action-time">Not before</label>
      <input id="queue-action-time" type="datetime-local" bind:value={at} />
      <label class="check-line">
        <input
          type="checkbox"
          bind:checked={outsideWindow}
          onchange={() => {
            preview = null;
            previewKey = '';
          }}
        />
        <span>Allow this run outside the job's hours</span>
      </label>
      <Button row disabled={at === '' || previewBusy} onclick={() => void refreshPreview()}
        >{previewBusy ? 'Calculating…' : 'Preview when it runs'}</Button
      >
      {#if preview !== null && previewKey === scheduleKey()}
        <div class="schedule-preview" role="status">
          <strong>It would first run</strong>
          <time datetime={preview.eligible_at}>{previewTime(preview.eligible_at)}</time>
          {#if preview.profile_timezone !== undefined}
            <span
              >{preview.profile_name} ·
              {previewTime(preview.eligible_at, preview.profile_timezone)}</span
            >
          {:else}
            <span>This once, outside the job's hours</span>
          {/if}
        </div>
      {:else if previewError !== ''}
        <p class="form-error" role="alert">{previewError}</p>
      {/if}
    {:else if action === 'set_priority'}
      <label for="queue-action-priority">Priority</label>
      <select id="queue-action-priority" bind:value={priority}>
        <option value="low">Low</option>
        <option value="normal">Normal</option>
        <option value="high">High</option>
        <option value="urgent">Urgent</option>
      </select>
    {:else if action === 'cancel'}
      <p>The item remains in Queue history with the action and actor recorded.</p>
    {/if}

    {#if needsReason}
      <label for="queue-action-reason">Reason</label>
      <textarea
        id="queue-action-reason"
        rows="3"
        bind:value={reason}
        placeholder="Why is this exception needed?"></textarea>
    {/if}
    {#if error !== ''}
      <p class="form-error" role="alert">{error}</p>
    {/if}
  </div>
</ConfirmDialog>

<style>
  .queue-action-form {
    display: grid;
    gap: var(--space-3);
  }
  .queue-action-form p {
    color: var(--text-muted);
    margin: 0;
  }
  label:not(.check-line) {
    font-size: 0.78rem;
    font-weight: 700;
    margin-bottom: calc(var(--space-2) * -1);
  }
  input[type='datetime-local'],
  select,
  textarea {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--radius-control);
    color: var(--text-primary);
    font: inherit;
    min-height: 2.75rem;
    padding: var(--space-2) var(--space-3);
  }
  textarea {
    line-height: var(--leading-body);
    resize: vertical;
  }
  .check-line {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    min-height: 2.75rem;
  }
  .form-error {
    color: var(--danger);
  }
  .schedule-preview {
    background: var(--surface-raised);
    border-inline-start: 2px solid var(--info);
    display: grid;
    font-size: 0.76rem;
    gap: var(--space-1);
    padding: var(--space-3);
  }
  .schedule-preview span {
    color: var(--text-muted);
  }
</style>
