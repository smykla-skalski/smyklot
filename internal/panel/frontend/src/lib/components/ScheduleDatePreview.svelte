<script lang="ts">
  import type { PanelApi } from '#lib/api.js';
  import type { ScheduleDatePreview, SchedulePreviewInput } from '#lib/schedule-preview.js';
  import { scheduleHoursProblems, validScheduleDate } from '#lib/schedule-validation.js';
  import Button from './Button.svelte';
  import FormError from './FormError.svelte';

  const {
    id,
    profile,
    timezoneValid,
    api,
  }: {
    id: string;
    profile: SchedulePreviewInput['profile'];
    timezoneValid: boolean;
    api: Pick<PanelApi, 'previewScheduleDate'>;
  } = $props();
  let date = $state('');
  let answer = $state.raw<{ key: string; preview?: ScheduleDatePreview; error?: string } | null>(
    null,
  );
  let pendingKey = $state('');
  let controller: AbortController | undefined;
  const input = $derived({ date, profile });
  const key = $derived(JSON.stringify(input));
  const valid = $derived(
    timezoneValid && validScheduleDate(date) && scheduleHoursProblems(profile).length === 0,
  );
  const current = $derived(answer?.key === key ? answer : null);
  const busy = $derived(pendingKey === key);
  // The cleanup cancels the external read when its draft changes or this view is removed.
  $effect(() => {
    void key;
    return () => controller?.abort();
  });

  async function preview(): Promise<void> {
    if (!valid) return;
    controller?.abort();
    const request = new AbortController();
    controller = request;
    const requestedKey = key;
    pendingKey = requestedKey;
    answer = null;
    try {
      const result = await api.previewScheduleDate(input, request.signal);
      if (!request.signal.aborted) answer = { key: requestedKey, preview: result };
    } catch {
      if (!request.signal.aborted)
        answer = { key: requestedKey, error: 'Could not preview these hours. Try again.' };
    } finally {
      if (controller === request) pendingKey = '';
    }
  }

  function boundary(value: string): string {
    const offset = value.endsWith('Z') ? '+00:00' : value.slice(-6);
    return `${value.slice(0, 10)} at ${value.slice(11, 16)} (UTC${offset})`;
  }
</script>

<!--
@component
An optional read-only preview of unsaved schedule hours on a local calendar date.
The server owns clock-change rules. Editing the draft cancels pending reads and
hides old answers; previewing never saves or schedules work.
-->
<details>
  <summary class="form-label">Preview a date</summary>
  <div class="form-stack preview-content">
    <p class="form-help">
      See when these hours apply on a date in {profile.timezone || 'the selected timezone'}. This
      does not save changes or start work.
    </p>
    <label class="form-field" for={id}>
      <span class="form-label">Preview date</span>
      <input class="text-input" {id} type="date" bind:value={date} />
    </label>
    {#if !timezoneValid || scheduleHoursProblems(profile).length > 0}<p class="form-help">
        Correct the schedule fields before previewing.
      </p>{/if}
    <Button disabled={!valid || busy} onclick={preview}
      >{busy ? 'Checking hours…' : current?.error ? 'Retry preview' : 'Preview hours'}</Button
    >
    <div class="form-stack" aria-live="polite">
      {#if current?.preview}
        <p class="form-help">Hours for {current.preview.date} in {current.preview.timezone}</p>
        {#if current.preview.windows.length === 0}<p class="form-help">Closed on this date.</p>
        {:else}
          <ul class="form-stack preview-windows">
            {#each current.preview.windows as window, index (index)}
              <li class="form-help">
                {#if window.available}
                  Opens {boundary(window.opens_at!)}<br />Closes {boundary(window.closes_at!)}
                {:else}This interval does not open because the clocks move forward.{/if}
              </li>
            {/each}
          </ul>
          <p class="form-help">
            When clocks move forward, missing times move to the next valid time. When an hour
            repeats, opening uses its first occurrence and closing uses its last.
          </p>
        {/if}
      {/if}
      <FormError message={current?.error ?? ''} />
    </div>
  </div>
</details>

<style>
  .preview-windows {
    margin-block: 0;
  }
  .preview-content {
    margin-block-start: var(--space-4);
  }
</style>
