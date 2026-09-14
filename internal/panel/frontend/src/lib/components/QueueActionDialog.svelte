<script lang="ts">
  import { queueActionLabel } from '#lib/queue-words.js';
  import { timezoneLocalLabel } from '#lib/schedule-timezone.js';
  import type { LocalTimeResolution } from '#lib/schedule-local-time.js';
  import { onMount, onDestroy } from 'svelte';
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
    onResolveTime,
    onSubmit,
  }: {
    item: QueueItem | null;
    action: QueueActionType | null;
    busy: boolean;
    error: string;
    onClose: () => void;
    onPreview: (input: QueueActionInput) => Promise<QueueSchedulePreview>;
    onResolveTime: (
      timezone: string,
      localTime: string,
      signal?: AbortSignal,
    ) => Promise<LocalTimeResolution>;
    onSubmit: (input: QueueActionInput) => void;
  } = $props();

  let reason = $state('');
  let at = $state('');
  let timezone = $state('UTC');
  let resolution = $state.raw<LocalTimeResolution | null>(null);
  let instant = $state('');
  let generation = 0;
  let controller: AbortController | undefined;
  let outsideWindow = $state(false);
  let priority = $state<QueuePriority>('normal');
  let preview = $state<QueueSchedulePreview | null>(null);
  let previewBusy = $state(false);
  let previewError = $state('');
  let previewKey = $state('');

  onMount(() => {
    timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
    at = item === null ? '' : localDateTime(item.not_before);
    priority = item?.priority ?? 'normal';
  });

  onDestroy(() => invalidatePreview(true));

  const needsReason = $derived(action === 'run_now' || (action === 'schedule_at' && outsideWindow));
  const invalid = $derived(
    previewBusy ||
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

  function submit(): void {
    if (item === null || action === null || invalid) return;
    const input: QueueActionInput = {
      type: action,
      expected_revision: item.revision,
    };
    if (reason.trim() !== '') input.reason = reason.trim();
    if (action === 'schedule_at') {
      input.at = instant;
      input.outside_window = outsideWindow;
    }
    if (action === 'set_priority') input.priority = priority;
    onSubmit(input);
  }

  function scheduleKey(): string {
    return `${timezone}:${at}:${instant}:${outsideWindow}`;
  }

  function invalidatePreview(resetTime = false): void {
    generation++;
    controller?.abort();
    previewBusy = false;
    preview = null;
    previewKey = '';
    previewError = '';
    if (resetTime) {
      resolution = null;
      instant = '';
    }
  }

  async function refreshPreview(): Promise<void> {
    if (item === null || action !== 'schedule_at' || at === '' || previewBusy) return;
    const current = ++generation;
    controller = new AbortController();
    previewBusy = true;
    previewError = '';
    try {
      if (resolution === null) {
        const resolved = await onResolveTime(timezone, at, controller.signal);
        if (current !== generation) return;
        resolution = resolved;
        instant = resolved.options.length === 1 ? resolved.options[0]!.at : '';
      }
      if (resolution.options.length === 0 || instant === '') return;
      const key = scheduleKey();
      const result = await onPreview({
        type: 'schedule_at',
        expected_revision: item.revision,
        at: instant,
        outside_window: outsideWindow,
      });
      if (current !== generation || key !== scheduleKey()) return;
      preview = result;
      previewKey = key;
    } catch (cause) {
      if (current !== generation) return;
      preview = null;
      previewKey = '';
      previewError = cause instanceof Error ? cause.message : String(cause);
    } finally {
      if (current === generation) previewBusy = false;
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
  title={action === null ? 'Queue action' : queueActionLabel(action, item)}
  description={item === null ? undefined : item.title}
  {busy}
  busyLabel="Applying…"
  confirmLabel={action === 'run_now' || action === 'cancel'
    ? queueActionLabel(action, item)
    : 'Apply'}
  confirmTone={action === 'cancel' ? 'stop' : action === 'run_now' ? 'signal' : 'default'}
  confirmDisabled={invalid}
  {onClose}
  onConfirm={submit}
>
  <div class="form-stack queue-action-form">
    {#if action === 'run_now'}
      <p>
        Allow this queued occurrence to start now, bypassing its delay and allowed hours. A worker
        starts it when capacity is available. This does not create another occurrence or interrupt
        work already running.
      </p>
    {:else if action === 'next_window'}
      <p>
        Remove this occurrence's delay and wait for its next allowed window and an available worker.
      </p>
    {:else if action === 'schedule_at'}
      <div class="form-field">
        <label class="form-label" for="queue-action-time">Not before</label>
        <input
          class="text-input"
          disabled={busy}
          id="queue-action-time"
          type="datetime-local"
          required
          aria-describedby="queue-action-time-help"
          aria-invalid={resolution?.options.length === 0}
          oninput={() => invalidatePreview(true)}
          bind:value={at}
        />
        <p id="queue-action-time-help" class="form-help">
          Required. Enter a time in {timezone}, your browser's timezone.
        </p>
        {#if resolution?.options.length === 0}
          <p class="form-error" role="alert">
            This time does not occur in {timezone} because the clocks change. Choose another time.
          </p>
        {:else if resolution && resolution.options.length > 1}
          <p class="form-help" role="status">
            This time occurs more than once because the clocks change. Choose which occurrence you
            mean.
          </p>
          <label class="form-label" for="queue-action-occurrence">Occurrence</label>
          <Select
            id="queue-action-occurrence"
            required
            disabled={busy}
            value={instant || undefined}
            placeholder="Choose an occurrence"
            options={resolution.options.map((option) => ({
              value: option.at,
              label: timezoneLocalLabel(option),
            }))}
            onValueChange={(value) => {
              invalidatePreview();
              instant = value ?? '';
            }}
          />
        {/if}
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
              invalidatePreview();
            }}
          />
        </span>
      </div>
      <Button
        row
        disabled={at === '' || busy}
        aria-disabled={previewBusy}
        onclick={() => void refreshPreview()}
        >{previewBusy ? 'Calculating…' : 'Preview earliest start'}</Button
      >
      {#if preview !== null && previewKey === scheduleKey()}
        <div class="schedule-preview" role="status">
          <strong>Can start from</strong>
          <span>Actual start depends on worker availability.</span>
          <time datetime={preview.eligible_at}>{previewTime(preview.eligible_at)}</time>
          <span>UTC: {previewTime(preview.eligible_at, 'UTC')}</span>
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
      <p>
        This cancels the selected occurrence. The cancellation and requester remain in Queue
        history.
      </p>
      {#if item?.source_kind === 'recurring'}
        <p>The recurring schedule stays enabled. Future occurrences can still be queued.</p>
      {/if}
    {/if}

    {#if needsReason}
      <div class="form-field">
        <label class="form-label" for="queue-action-reason">Reason</label>
        <textarea
          required
          aria-describedby="queue-action-reason-help"
          id="queue-action-reason"
          class="text-input"
          disabled={busy}
          rows="3"
          bind:value={reason}
          placeholder="Why is this exception needed?"></textarea>
        <p id="queue-action-reason-help" class="form-help">
          Required. Explain why this occurrence should bypass its usual timing.
        </p>
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
