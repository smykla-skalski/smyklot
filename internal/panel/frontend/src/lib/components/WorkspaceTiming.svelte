<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';

  import type { PanelApi } from '#lib/api.js';
  import type {
    QueuePolicy,
    QueuePriority,
    QueueWorkload,
    ScheduleProfile,
    ScheduleRequest,
  } from '#lib/types.js';
  import { receipts } from '#lib/receipts.svelte.js';
  /* One name per job, wherever it is read. A workspace used to keep a second table
     of its own, so the row a member asked about and the row an operator answered
     named the same job two ways. */
  import { workloadTitle } from '#lib/workloads.js';

  import Button from './Button.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import Modal from './Modal.svelte';
  import ScheduleWindowsEditor, { type EditableWindow } from './ScheduleWindowsEditor.svelte';
  import Select from './Select.svelte';

  const {
    api,
    targetId,
    canRequest = false,
  }: {
    api: PanelApi;
    targetId: string;
    /** Whether this reader may ask for a change at all. A member still reads the answer. */
    canRequest?: boolean;
  } = $props();

  /** The workloads a workspace may ask about; the rest are the service's own. */
  const REQUESTABLE = new Set<QueueWorkload>([
    'pending_ci',
    'pending_ci_gate',
    'reaction_scan',
    'config_migration',
    'sync_scan',
    'path_refresh',
  ]);

  const DAY = 24 * 60;

  const schedules = createQuery(() => ({
    queryKey: ['schedules', 'target', targetId],
    queryFn: async () => {
      const [loaded, requests] = await Promise.all([
        api.fetchTargetSchedules(targetId),
        api.fetchTargetScheduleRequests(targetId),
      ]);
      return {
        policies: loaded.policies.effective,
        profiles: loaded.profiles,
        requests,
      };
    },
  }));

  const policies = $derived<QueuePolicy[]>(
    (schedules.data?.policies ?? []).filter((policy) => REQUESTABLE.has(policy.kind)),
  );
  const profiles = $derived<ScheduleProfile[]>(schedules.data?.profiles ?? []);
  const pending = $derived<ScheduleRequest[]>(
    (schedules.data?.requests ?? []).filter((request) => request.state === 'pending'),
  );

  /**
   * WHEN SMYKLOT ACTS, IN A SENTENCE.
   *
   * Read from the windows rather than from a profile's name: a profile called anything at
   * all whose seven days each run midnight to midnight IS around the clock, and the name
   * is the operators' word for it rather than an answer to the question asked here.
   */
  const fact = $derived.by(() => {
    if (schedules.isPending) return 'Reading the service…';
    const named = [...new Set(policies.map((policy) => policy.profile_id))]
      .map((id) => profiles.find((profile) => profile.id === id))
      .filter((profile): profile is ScheduleProfile => profile !== undefined);
    if (named.length === 0) return 'Around the clock (UTC)';
    if (named.length > 1) return `${named.length} named windows`;
    const only = named[0] as ScheduleProfile;
    return alwaysOpen(only)
      ? `Around the clock (${only.timezone})`
      : `${only.name} (${only.timezone})`;
  });

  function alwaysOpen(profile: ScheduleProfile): boolean {
    return (
      profile.windows.length === 7 &&
      profile.windows.every((window) => window.start_minute === 0 && window.end_minute >= DAY)
    );
  }

  function requestedWindow(request: ScheduleRequest): string {
    if (request.custom_profile !== undefined) return request.custom_profile.name;
    return profiles.find((profile) => profile.id === request.profile_id)?.name ?? 'a named window';
  }

  /* ---------- Asking for a change ---------- */

  let open = $state(false);
  let opener = $state<HTMLButtonElement | null>(null);
  let busy = $state(false);
  let problem = $state('');

  let kind = $state<QueueWorkload>('sync_scan');
  let windowMode = $state<'existing' | 'custom'>('existing');
  let chosenProfile = $state<string | null>(null);
  let customName = $state('Workspace hours');
  let timezone = $state(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC');
  let windows = $state.raw<EditableWindow[]>([
    { id: 'request-1', weekday: 1, start: '09:00', end: '17:00' },
    { id: 'request-2', weekday: 2, start: '09:00', end: '17:00' },
    { id: 'request-3', weekday: 3, start: '09:00', end: '17:00' },
    { id: 'request-4', weekday: 4, start: '09:00', end: '17:00' },
    { id: 'request-5', weekday: 5, start: '09:00', end: '17:00' },
  ]);
  let exceptions = $state('');
  let cadence = $state<number | null | undefined>(undefined);
  let priority = $state<QueuePriority | null>(null);
  let reason = $state('');

  const chosen = $derived(policies.find((policy) => policy.kind === kind));
  const cadenceShown = $derived(
    cadence !== undefined ? cadence : Math.round((chosen?.cadence ?? 0) / 1_000_000_000),
  );
  const priorityShown = $derived(priority ?? chosen?.default_priority ?? 'normal');
  const profileShown = $derived(chosenProfile ?? chosen?.profile_id ?? profiles[0]?.id ?? '');
  const cadenceInvalid = $derived(
    cadenceShown === null ||
      !Number.isFinite(cadenceShown) ||
      cadenceShown < 0 ||
      (kind !== 'pending_ci' && cadenceShown <= 0),
  );

  function pickKind(next: QueueWorkload): void {
    kind = next;
    cadence = undefined;
    priority = null;
    chosenProfile = null;
  }

  function minute(value: string): number {
    const [hour = '0', rest = '0'] = value.split(':');
    return Number(hour) * 60 + Number(rest);
  }

  function parseExceptions(): ScheduleProfile['exceptions'] {
    return exceptions
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)
      .map((line) => {
        const [date = '', span = 'closed'] = line.split(/\s+/u, 2);
        if (span === 'closed') return { date, closed: true };
        const [from = '00:00', to = '00:00'] = span.split('-', 2);
        return { date, closed: false, start_minute: minute(from), end_minute: minute(to) };
      });
  }

  async function send(): Promise<void> {
    const current = chosen;
    if (current === undefined || reason.trim() === '' || cadenceInvalid) return;
    busy = true;
    problem = '';
    try {
      const custom: ScheduleProfile = {
        id: '',
        name: customName.trim(),
        timezone: timezone.trim(),
        system: false,
        revision: 0,
        windows: windows.map((window) => ({
          weekday: window.weekday,
          start_minute: minute(window.start),
          end_minute: minute(window.end),
        })),
        exceptions: parseExceptions(),
      };
      await api.createTargetScheduleRequest(targetId, {
        kind,
        base_revision: current.revision,
        ...(windowMode === 'existing' ? { profile_id: profileShown } : { custom_profile: custom }),
        cadence_seconds: cadenceShown as number,
        default_priority: priorityShown,
        configuration: current.configuration,
        reason: reason.trim(),
      });
      reason = '';
      open = false;
      receipts.say('Sent to the operators for a decision');
      await schedules.refetch();
    } catch (cause) {
      problem = cause instanceof Error ? cause.message : String(cause);
    } finally {
      busy = false;
    }
  }

  async function withdraw(request: ScheduleRequest): Promise<void> {
    busy = true;
    problem = '';
    try {
      await api.withdrawTargetScheduleRequest(targetId, request.id, request.revision);
      receipts.say('Request withdrawn - the operators will not see it');
      await schedules.refetch();
    } catch (cause) {
      problem = cause instanceof Error ? cause.message : String(cause);
    } finally {
      busy = false;
    }
  }

  const pendingTone: ChipTone = 'warning';
</script>

<!--
@component
When Smyklot acts here, and the way to ask for that to change.

The timing itself belongs to the service: a workspace cannot set it, so this states the
answer in a sentence and carries the request that reaches the operators. What it replaced
was a page of the operators' own tables - policies, profiles, a request log - which
answered a question a workspace never asks and hid the one it does.
-->

<div class="policy-rows">
  <div class="policy-row">
    <span class="setting-say">
      <span class="setting-name">When Smyklot acts</span>
      <span class="setting-why"
        >Timing is run by the service. A request goes to its operators with your reason attached</span
      >
    </span>
    <span class="policy-value">
      <span class="setting-fact">{fact}</span>
      {#if canRequest}
        <Button tone="quiet" bind:element={opener} onclick={() => (open = true)}
          >Request a change</Button
        >
      {/if}
    </span>
  </div>

  {#each pending as request (request.id)}
    <div class="policy-row">
      <!-- NAMED AS THE ASK, not as the setting. Titled with the workload alone it read as
           a second copy of the row above it - two rows in one card saying "Path index". -->
      <span class="setting-say">
        <span class="setting-name">A change to {workloadTitle(request.kind)}</span>
        <span class="setting-why">{requestedWindow(request)} · {request.reason}</span>
      </span>
      <span class="policy-value">
        <Chip tone={pendingTone}>Waiting on the operators</Chip>
        <Button tone="quiet" disabled={busy} onclick={() => void withdraw(request)}>Withdraw</Button
        >
      </span>
    </div>
  {/each}
</div>

{#if problem !== ''}
  <div class="state-panel is-error" role="alert">
    <span><strong>The request did not go through.</strong> {problem}</span>
  </div>
{/if}

<Modal
  id="workspace-timing-request"
  {open}
  title="Request a change to when Smyklot acts"
  description="The operators decide. Say what you need and why."
  returnFocus={opener}
  onClose={() => (open = false)}
>
  <div class="request-form">
    <label>
      <span>Job</span>
      <Select
        value={kind}
        onchange={(event) =>
          pickKind((event.currentTarget as HTMLSelectElement).value as QueueWorkload)}
      >
        {#each policies as policy (policy.kind)}
          <option value={policy.kind}>{workloadTitle(policy.kind)}</option>
        {/each}
      </Select>
    </label>

    <label>
      <span>Hours</span>
      <Select bind:value={windowMode}>
        <option value="existing">A named set of hours</option>
        <option value="custom">Hours of your own</option>
      </Select>
    </label>

    {#if windowMode === 'existing'}
      <label>
        <span>Which hours</span>
        <Select
          aria-label="Which hours"
          value={profileShown}
          onchange={(event) => (chosenProfile = (event.currentTarget as HTMLSelectElement).value)}
        >
          {#each profiles as profile (profile.id)}
            <option value={profile.id}>{profile.name}</option>
          {/each}
        </Select>
      </label>
    {:else}
      <label>
        <span>Name</span>
        <input class="text-input" bind:value={customName} />
      </label>
      <label>
        <span>Timezone</span>
        <input class="text-input" bind:value={timezone} placeholder="Europe/Warsaw" />
      </label>
      <div class="request-windows">
        <ScheduleWindowsEditor
          idPrefix="timing-window"
          {windows}
          onChange={(next) => (windows = next)}
        />
      </div>
      <label>
        <span>Date exceptions</span>
        <textarea
          class="text-input"
          rows="4"
          bind:value={exceptions}
          placeholder="2026-12-25 closed&#10;2026-12-31 09:00-13:00"></textarea>
      </label>
      <p class="request-helper">
        One local date per line: <code>YYYY-MM-DD closed</code> or
        <code>YYYY-MM-DD HH:MM-HH:MM</code>.
      </p>
    {/if}

    <label>
      <span>How often, in seconds</span>
      <input
        class="text-input"
        type="number"
        min={kind === 'pending_ci' ? 0 : 1}
        step="60"
        value={cadenceShown ?? ''}
        oninput={(event) => {
          const typed = (event.currentTarget as HTMLInputElement).valueAsNumber;
          cadence = Number.isFinite(typed) ? typed : null;
        }}
      />
    </label>

    <label>
      <span>Priority</span>
      <Select
        value={priorityShown}
        onchange={(event) =>
          (priority = (event.currentTarget as HTMLSelectElement).value as QueuePriority)}
      >
        <option value="low">Low</option>
        <option value="normal">Normal</option>
        <option value="high">High</option>
        <option value="urgent">Urgent</option>
      </Select>
    </label>

    <label class="request-reason">
      <span>Reason</span>
      <textarea
        class="text-input"
        rows="3"
        bind:value={reason}
        placeholder="What this timing is getting in the way of"></textarea>
    </label>
  </div>

  {#snippet footer()}
    <Button tone="ghost" onclick={() => (open = false)}>Cancel</Button>
    <Button
      tone="signal"
      disabled={busy || reason.trim() === '' || cadenceInvalid}
      onclick={() => void send()}>{busy ? 'Sending…' : 'Send request'}</Button
    >
  {/snippet}
</Modal>

<style>
  .request-form {
    display: grid;
    gap: var(--space-4);
  }

  .request-form label {
    display: grid;
    gap: var(--space-2);
    font-size: var(--font-size-meta);
  }

  .request-form label > span {
    color: var(--text-secondary);
    font-weight: 600;
    text-box: trim-both cap alphabetic;
  }

  .request-helper {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    line-height: var(--leading-compact);
    margin: 0;
    text-box: trim-both cap alphabetic;
  }

  .request-helper code {
    font-family: var(--mono);
  }

  .text-input {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font: inherit;
    min-block-size: var(--tier-quiet);
    padding: var(--space-2);
    width: 100%;
  }

  .text-input:focus-visible {
    border-color: var(--brand-action);
    outline: 2px solid var(--focus);
  }

  /* A refusal stands under the rows on the card's own text edge, not inside one. What
     SUCCEEDED is a receipt at the foot of the window, so only this is left here. */
  .state-panel {
    margin-block-start: var(--space-3);
  }
</style>
