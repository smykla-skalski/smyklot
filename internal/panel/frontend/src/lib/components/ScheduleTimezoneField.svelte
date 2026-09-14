<script lang="ts">
  import { Combobox } from 'bits-ui';
  import { untrack } from 'svelte';
  import { PanelApiError, type PanelApi } from '#lib/api.js';
  import {
    timezoneSuggestions,
    timezoneLocalLabel,
    type ScheduleTimezonePreview,
  } from '#lib/schedule-timezone.js';
  import Button from './Button.svelte';
  import FormError from './FormError.svelte';

  let {
    id,
    value = $bindable(),
    api,
    onValidityChange,
  }: {
    id: string;
    value: string;
    api: Pick<PanelApi, 'previewScheduleTimezone'>;
    onValidityChange: (valid: boolean) => void;
  } = $props();
  let open = $state(false);
  let selected = $state('');
  let preview = $state.raw<ScheduleTimezonePreview | null>(null);
  let problem = $state('');
  let retryable = $state(false);
  let retry = $state(0);
  let pending = $state(true);
  const zones = timezoneSuggestions(untrack(() => value));
  const matches = $derived(
    zones.filter((zone) =>
      zone
        .replaceAll('_', ' ')
        .toLowerCase()
        .includes(value.trim().replaceAll('_', ' ').toLowerCase()),
    ),
  );
  const visibleProblem = $derived(open ? '' : problem);
  const helpId = $derived(`${id}-help`);
  const errorId = $derived(`${id}-error`);

  // Preview requests are external IO. Cleanup cancels both the debounce and stale responses.
  $effect(() => {
    const zone = value.trim();
    const attempt = retry;
    const controller = new AbortController();
    untrack(() => {
      void attempt;
      onValidityChange(false);
      preview = null;
      problem = zone ? '' : 'Choose a timezone';
      retryable = false;
      pending = zone !== '';
    });
    const timer = setTimeout(async () => {
      if (!zone) return;
      try {
        const result = await api.previewScheduleTimezone(
          zone,
          new Date().toISOString(),
          controller.signal,
        );
        if (controller.signal.aborted) return;
        preview = result;
        onValidityChange(true);
      } catch (cause) {
        if (controller.signal.aborted) return;
        retryable = !(cause instanceof PanelApiError && cause.code === 'invalid_timezone');
        problem = retryable
          ? 'Could not check this timezone. Try again.'
          : 'Choose a timezone supported by the scheduler';
      } finally {
        if (!controller.signal.aborted) pending = false;
      }
    }, 250);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  });

  function choose(zone: string): void {
    if (!zone) return;
    value = zone;
    selected = zone;
    open = false;
  }
</script>

<!--
@component
A searchable timezone with an authoritative scheduler preview. Browser names are
suggestions, not proof of support. Each edit invalidates the previous answer;
requests are debounced and aborted on change or dismissal. Callers may submit only
after onValidityChange confirms the current value. Failed reads retain the input
and provide retry independently of changing the schedule.
-->
<div class="form-field">
  <label class="form-label" for={id}>Timezone</label>
  <Combobox.Root
    type="single"
    items={matches.map((zone) => ({ value: zone, label: zone }))}
    bind:open
    bind:value={selected}
    inputValue={value}
    onValueChange={choose}
  >
    <Combobox.Input
      {id}
      class="text-input"
      autocomplete="off"
      spellcheck="false"
      autocapitalize="none"
      aria-invalid={visibleProblem && !retryable ? true : undefined}
      aria-describedby={visibleProblem ? `${helpId} ${errorId}` : helpId}
      oninput={(event) => {
        value = event.currentTarget.value;
        selected = '';
      }}
      onfocus={() => (open = true)}
    />
    <Combobox.Portal to=".app-shell">
      <Combobox.Content
        class="select-menu select-menu-in-dialog"
        sideOffset={4}
        collisionPadding={8}
      >
        <Combobox.Viewport class="menu-list select-options" aria-label="Timezones">
          {#each matches as zone (zone)}<Combobox.Item class="menu-option" value={zone} label={zone}
              >{zone.replaceAll('_', ' ')}</Combobox.Item
            >{/each}
          {#if matches.length === 0}<p class="form-help">
              No matching suggestions. You can enter a timezone name.
            </p>{/if}
        </Combobox.Viewport>
      </Combobox.Content>
    </Combobox.Portal>
  </Combobox.Root>
  <p class="form-help" id={helpId}>
    Search by city or timezone name. Scheduled hours use this timezone.
  </p>
  <div aria-live="polite">
    {#if pending}<p class="form-help">Checking timezone…</p>
    {:else if preview}<p class="form-help">Local time: {timezoneLocalLabel(preview)}</p>{/if}
  </div>
  {#if problem}<div id={errorId}><FormError message={visibleProblem} /></div>{/if}
  {#if retryable && !open}<Button tone="quiet" onclick={() => (retry += 1)}
      >Retry timezone check</Button
    >{/if}
</div>
