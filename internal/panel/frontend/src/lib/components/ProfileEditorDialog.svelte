<script lang="ts">
  import { onMount } from 'svelte';
  import type { ScheduleProfile, ScheduleProfileInput } from '#lib/types.js';
  import ConfirmDialog from './ConfirmDialog.svelte';
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

  let name = $state('');
  let timezone = $state(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC');
  let windows = $state.raw<EditableWindow[]>([
    { id: 'default-1', weekday: 1, start: '09:00', end: '17:00' },
    { id: 'default-2', weekday: 2, start: '09:00', end: '17:00' },
    { id: 'default-3', weekday: 3, start: '09:00', end: '17:00' },
    { id: 'default-4', weekday: 4, start: '09:00', end: '17:00' },
    { id: 'default-5', weekday: 5, start: '09:00', end: '17:00' },
  ]);
  let exceptions = $state('');
  const exceptionExample = '2026-12-25 closed\n2026-12-31 09:00-13:00';

  onMount(() => {
    name = profile?.name ?? '';
    timezone = profile?.timezone ?? (Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC');
    const storedWindows = profile?.windows ?? [];
    if (storedWindows.length > 0) {
      windows = storedWindows.map((window, index) => ({
        id: `stored-${index}`,
        weekday: window.weekday,
        start: minuteTime(window.start_minute),
        end: minuteTime(window.end_minute),
      }));
    }
    exceptions = (profile?.exceptions ?? [])
      .map((entry) =>
        entry.closed
          ? `${entry.date} closed`
          : `${entry.date} ${minuteTime(entry.start_minute ?? 0)}-${minuteTime(entry.end_minute ?? 0)}`,
      )
      .join('\n');
  });

  function minuteTime(minutes: number): string {
    return `${String(Math.floor(minutes / 60)).padStart(2, '0')}:${String(minutes % 60).padStart(2, '0')}`;
  }

  function timeMinute(value: string): number {
    const [hour = '0', minute = '0'] = value.split(':');
    return Number(hour) * 60 + Number(minute);
  }

  function windowsValid(): boolean {
    const byDay: Array<Array<{ start: number; end: number }>> = Array.from({ length: 7 }, () => []);
    for (const window of windows) {
      const start = timeMinute(window.start);
      const end = timeMinute(window.end);
      if (start >= end) return false;
      byDay[window.weekday]?.push({ start, end });
    }
    for (const day of byDay) {
      day.sort((left, right) => left.start - right.start);
      if (day.some((entry, index) => index > 0 && entry.start < (day[index - 1]?.end ?? 0)))
        return false;
    }

    return windows.length > 0;
  }

  function parseExceptions(): ScheduleProfileInput['exceptions'] {
    return exceptions
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)
      .map((line) => {
        const [date = '', span = 'closed'] = line.split(/\s+/, 2);
        if (span === 'closed') return { date, closed: true };
        const [from = '00:00', to = '00:00'] = span.split('-', 2);
        return { date, closed: false, start_minute: timeMinute(from), end_minute: timeMinute(to) };
      });
  }

  function submit(): void {
    onSubmit({
      name: name.trim(),
      timezone: timezone.trim(),
      windows: windows.map((window) => ({
        weekday: window.weekday,
        start_minute: timeMinute(window.start),
        end_minute: timeMinute(window.end),
      })),
      exceptions: parseExceptions(),
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
  title={profile === null ? 'New window profile' : 'Edit window profile'}
  description="Times are interpreted in the profile timezone. Existing scheduled work is recomputed."
  {busy}
  busyLabel="Saving…"
  confirmLabel="Save profile"
  confirmTone="signal"
  confirmDisabled={name.trim() === '' || timezone.trim() === '' || !windowsValid()}
  {onClose}
  onConfirm={submit}
>
  <div class="profile-form">
    {#if profile !== null}
      <p class="impact" role="status">
        Saving recomputes {profile.affected_items ?? 0} future queue
        {profile.affected_items === 1 ? ' item' : ' items'} across
        {profile.affected_workspaces ?? 0}
        {profile.affected_workspaces === 1 ? ' workspace' : ' workspaces'}.
        {profile.affected_policies ?? 0}
        {profile.affected_policies === 1 ? ' policy uses' : ' policies use'} this profile.
      </p>
    {/if}
    <label for="profile-name">Profile name</label>
    <input id="profile-name" bind:value={name} placeholder="Europe business hours" />
    <label for="profile-timezone">IANA timezone</label>
    <input id="profile-timezone" bind:value={timezone} placeholder="Europe/Warsaw" />
    <ScheduleWindowsEditor
      idPrefix="profile-window"
      {windows}
      onChange={(next) => (windows = next)}
    />
    <label for="profile-exceptions">Date exceptions</label>
    <textarea
      id="profile-exceptions"
      rows="4"
      bind:value={exceptions}
      placeholder={exceptionExample}></textarea>
    <p class="helper">
      One local date per line: <code>YYYY-MM-DD closed</code> or
      <code>YYYY-MM-DD HH:MM-HH:MM</code>.
    </p>
    {#if error !== ''}<p class="form-error" role="alert">{error}</p>{/if}
  </div>
</ConfirmDialog>

<style>
  .profile-form {
    display: grid;
    gap: var(--space-3);
  }
  .profile-form > label {
    font-size: 0.75rem;
    font-weight: 720;
  }
  .profile-form > label {
    margin-bottom: calc(var(--space-2) * -1);
  }
  input:not([type='checkbox']),
  textarea {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--radius-control);
    color: var(--text-primary);
    font: inherit;
    min-height: 2.75rem;
    padding: var(--space-2) var(--space-3);
  }
  .helper {
    color: var(--text-muted);
    font-size: 0.72rem;
    margin: calc(var(--space-2) * -1) 0 0;
  }
  .impact {
    background: var(--surface-raised);
    border-inline-start: 2px solid var(--info);
    color: var(--text-secondary);
    font-size: 0.75rem;
    line-height: var(--leading-compact);
    margin: 0;
    padding: var(--space-3);
  }
  .form-error {
    color: var(--danger);
    margin: 0;
  }
</style>
