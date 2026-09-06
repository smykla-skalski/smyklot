<script lang="ts">
  import { onMount } from 'svelte';
  import { formatDateTime } from '#lib/format.js';
  import type {
    QueueActionInput,
    QueueActionType,
    QueueItem,
    QueuePriority,
    QueueSchedulePreview,
  } from '#lib/types.js';
  import Button from './Button.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import Select from './Select.svelte';
  import Switch from './Switch.svelte';
  import FormError from './FormError.svelte';

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

  /* Was `dateStyle: 'medium', timeStyle: 'long'`, which is this to the second and
     the zone - said in the two words that cannot carry a `timeZoneName` beside
     them. One spelling for an instant, so a preview and the row it previews read
     the same. */
  const previewTime = (value: string, timeZone?: string): string =>
    formatDateTime(value, { timeZone, named: true, seconds: true });
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
  <div class="form-stack queue-action-form">
    {#if action === 'run_now'}
      <p>
        Run this job once now, outside its normal schedule · Work already running is not interrupted
      </p>
    {:else if action === 'next_window'}
      <p>Clear the delay and use the next available time within the job's hours</p>
    {:else if action === 'schedule_at'}
      <div class="form-field">
        <label class="form-label" for="queue-action-time">Not before</label>
        <input
          class="text-input"
          disabled={busy}
          id="queue-action-time"
          type="datetime-local"
          bind:value={at}
        />
      </div>
      <div class="form-row">
        <span class="form-label">Allow this run outside the job's hours</span>
        <span class="policy-value">
          <Switch
            bare
            label="Allow this run outside the job's hours"
            checked={outsideWindow}
            disabled={busy}
            onToggle={(next) => {
              outsideWindow = next;
              preview = null;
              previewKey = '';
            }}
          />
        </span>
      </div>
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
        <FormError message={previewError} />
      {/if}
    {:else if action === 'set_priority'}
      <div class="form-field">
        <label class="form-label" for="queue-action-priority">Priority</label>
        <Select
          id="queue-action-priority"
          disabled={busy}
          bind:value={priority}
          options={[
            { value: 'low', label: 'Low' },
            { value: 'normal', label: 'Normal' },
            { value: 'high', label: 'High' },
            { value: 'urgent', label: 'Urgent' },
          ]}
        />
      </div>
    {:else if action === 'cancel'}
      <p>The cancellation and who requested it remain in Queue history</p>
    {/if}

    {#if needsReason}
      <div class="form-field">
        <label class="form-label" for="queue-action-reason">Reason</label>
        <textarea
          id="queue-action-reason"
          class="text-input"
          disabled={busy}
          rows="3"
          bind:value={reason}
          placeholder="Why is this exception needed?"></textarea>
      </div>
    {/if}
    {#if error !== ''}
      <FormError message={error} />
    {/if}
  </div>
</ConfirmDialog>

<style>
  .queue-action-form > p {
    color: var(--text-secondary);
    line-height: var(--row-copy-leading);
    margin: 0;
    text-box: trim-both cap alphabetic;
  }
  .schedule-preview {
    display: grid;
    font-size: var(--font-size-compact);
    gap: var(--row-copy-gap);
    line-height: var(--row-copy-leading);
    text-box: trim-both cap alphabetic;
  }
  .schedule-preview span {
    color: var(--text-secondary);
  }
</style>
