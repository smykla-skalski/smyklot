<script lang="ts">
  import { scheduleMinute } from '#lib/schedule-input.js';
  import { scheduleExceptionProblems, scheduleWindowProblems } from '#lib/schedule-validation.js';
  import {
    editableExceptions,
    exceptionDraft,
    exceptionInputs,
    type EditableException,
  } from '#lib/schedule-exceptions.js';
  import ScheduleExceptionsEditor from './ScheduleExceptionsEditor.svelte';
  import { onMount } from 'svelte';
  import type { ScheduleProfile, ScheduleProfileInput } from '#lib/types.js';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import Callout from './Callout.svelte';
  import FormError from './FormError.svelte';
  import ScheduleWindowsEditor, { type EditableWindow } from './ScheduleWindowsEditor.svelte';

  const {
    profile,
    open,
    busy,
    error,
    onClose,
    onSubmit,
  }: {
    profile: ScheduleProfile | null;
    open: boolean;
    busy: boolean;
    error: string;
    onClose: () => void;
    onSubmit: (input: ScheduleProfileInput) => void;
  } = $props();

  let inputProblem = $state('');
  let name = $state('');
  let timezone = $state(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC');
  let windows = $state.raw<EditableWindow[]>([
    { id: 'default-1', weekday: 1, start: '09:00', end: '17:00' },
    { id: 'default-2', weekday: 2, start: '09:00', end: '17:00' },
    { id: 'default-3', weekday: 3, start: '09:00', end: '17:00' },
    { id: 'default-4', weekday: 4, start: '09:00', end: '17:00' },
    { id: 'default-5', weekday: 5, start: '09:00', end: '17:00' },
  ]);
  let exceptions = $state.raw<EditableException[]>([]);
  let showExceptionProblems = $state(false);
  let baseline = $state('');
  let confirmingDiscard = $state(false);
  let editingControl = $state<HTMLElement | null>(null);
  const changed = $derived(baseline !== '' && snapshot() !== baseline);

  onMount(() => {
    name = profile?.name ?? '';
    timezone = profile?.timezone ?? (Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC');
    const storedWindows = profile?.windows ?? [];
    // Defaults belong only to a new profile. Empty saved hours mean exceptions only.
    if (profile !== null) {
      windows = storedWindows.map((window, index) => ({
        id: `stored-${index}`,
        weekday: window.weekday,
        start: minuteTime(window.start_minute),
        end: minuteTime(window.end_minute),
      }));
    }
    exceptions = editableExceptions(profile?.exceptions ?? []);
    baseline = snapshot();
  });

  /** Window ids are editing handles, not settings. Restoring the same hours is clean. */
  function snapshot(): string {
    return JSON.stringify({
      name: name.trim(),
      timezone: timezone.trim(),
      windows: windows
        .map(({ weekday, start, end }) => ({ weekday, start, end }))
        .sort(
          (a, b) =>
            a.weekday - b.weekday || a.start.localeCompare(b.start) || a.end.localeCompare(b.end),
        ),
      exceptions: exceptionDraft(exceptions),
    });
  }

  function beforeClose(): boolean {
    if (busy) return false;
    if (!changed) return true;
    if (!confirmingDiscard) {
      const active = document.activeElement;
      editingControl =
        active instanceof HTMLElement && active.closest('#profile-editor')
          ? active
          : document.getElementById('profile-editor');
      confirmingDiscard = true;
    }
    return false;
  }

  function minuteTime(minutes: number): string {
    return `${String(Math.floor(minutes / 60)).padStart(2, '0')}:${String(minutes % 60).padStart(2, '0')}`;
  }

  function windowsValid(): boolean {
    return (
      scheduleWindowProblems(
        windows.map((window) => ({
          weekday: window.weekday,
          start_minute: scheduleMinute(window.start),
          end_minute: scheduleMinute(window.end),
        })),
      ).length === 0 &&
      (windows.length > 0 || exceptions.length > 0)
    );
  }

  function submit(): void {
    inputProblem = '';
    showExceptionProblems = true;
    const parsedExceptions = exceptionInputs(exceptions);
    if (scheduleExceptionProblems(parsedExceptions).length > 0) return;
    onSubmit({
      name: name.trim(),
      timezone: timezone.trim(),
      windows: windows.map((window) => ({
        weekday: window.weekday,
        start_minute: scheduleMinute(window.start),
        end_minute: scheduleMinute(window.end),
      })),
      exceptions: parsedExceptions,
      expected_revision: profile?.revision ?? 0,
    });
  }
</script>

<!--
@component
A named set of windows during which work may run. One dialog for both making and
editing, told apart by whether it was given a profile - the fields are identical, and
two dialogs would be two places to add the next one.

A profile is referred to by policies, so this edits the definition and never the uses:
changing a window here changes when every policy that names it runs.
-->

<ConfirmDialog
  id="profile-editor"
  {open}
  title={profile === null ? 'New hours profile' : 'Edit hours profile'}
  description="Scheduled jobs use these hours and this timezone"
  {busy}
  busyLabel="Saving…"
  confirmLabel="Save profile"
  confirmTone="signal"
  confirmDisabled={name.trim() === '' || timezone.trim() === '' || !windowsValid()}
  {onClose}
  {beforeClose}
  onConfirm={submit}
>
  <div class="form-stack">
    {#if profile !== null}
      <Callout>
        <p>
          Saving updates {profile.affected_items ?? 0} queued
          {profile.affected_items === 1 ? ' item' : ' items'} in
          {profile.affected_workspaces ?? 0}
          {profile.affected_workspaces === 1 ? ' workspace' : ' workspaces'} and affects
          {profile.affected_policies ?? 0}
          {profile.affected_policies === 1 ? ' policy' : ' policies'}
        </p>
      </Callout>
    {/if}
    <label class="form-field" for="profile-name">
      <span class="form-label">Profile name</span>
      <input
        class="text-input"
        id="profile-name"
        bind:value={name}
        placeholder="Europe business hours"
      />
    </label>
    <label class="form-field" for="profile-timezone">
      <span class="form-label">Timezone</span>
      <input
        class="text-input"
        id="profile-timezone"
        bind:value={timezone}
        placeholder="Europe/Warsaw"
      />
    </label>
    <ScheduleWindowsEditor
      idPrefix="profile-window"
      {windows}
      onChange={(next) => (windows = next)}
    />
    <ScheduleExceptionsEditor
      idPrefix="profile-exception"
      entries={exceptions}
      onChange={(next) => (exceptions = next)}
      showProblems={showExceptionProblems}
    />
    <FormError message={inputProblem || error} />
  </div>
  <ConfirmDialog
    id="discard-hours-changes"
    open={confirmingDiscard}
    title="Discard hours changes?"
    returnFocus={editingControl}
    onClose={() => (confirmingDiscard = false)}
    onConfirm={onClose}
    confirmLabel="Discard changes"
    confirmTone="stop"
    cancelLabel="Keep editing"
  >
    <p class="form-help">Your changes to this hours profile will be lost</p>
  </ConfirmDialog>
</ConfirmDialog>
