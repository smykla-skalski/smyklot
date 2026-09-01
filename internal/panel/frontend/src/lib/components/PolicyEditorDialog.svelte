<script lang="ts">
  import { onMount } from 'svelte';
  import type {
    QueuePolicy,
    QueuePolicyInput,
    QueuePriority,
    ScheduleProfile,
  } from '#lib/types.js';
  import { workloadTitle } from '#lib/workloads.js';
  import ConfirmDialog from './ConfirmDialog.svelte';

  const {
    policy,
    profiles,
    busy,
    error,
    onClose,
    onSubmit,
  }: {
    policy: QueuePolicy | null;
    profiles: readonly ScheduleProfile[];
    busy: boolean;
    error: string;
    onClose: () => void;
    onSubmit: (input: QueuePolicyInput) => void;
  } = $props();

  let enabled = $state(true);
  let cadence = $state(300);
  let profileId = $state('always-open');
  let priority = $state<QueuePriority>('normal');
  let retryDelay = $state(30);
  let retentionEnabled = $state(false);
  let retention = $state(2_592_000);
  let approvalLifetime = $state(7_200);
  let activeCheck = $state(300);
  let noCheckGrace = $state(600);
  let deferAfter = $state(3_600);
  let deferredCheck = $state(21_600);
  let passingQuiet = $state(30);
  let webhookMaxDelay = $state(300);
  let webhookMaxAttempts = $state(8);
  const recurringKinds = new Set<QueuePolicy['kind']>([
    'pending_ci_gate',
    'catalog_refresh',
    'reaction_scan',
    'config_migration',
    'sync_scan',
    'path_refresh',
    'delivery_cleanup',
    'auth_cleanup',
  ]);

  onMount(() => {
    if (policy === null) return;
    enabled = policy.enabled;
    cadence = Math.round(policy.cadence / 1_000_000_000);
    profileId = policy.profile_id;
    priority = policy.default_priority;
    retryDelay = Math.round(policy.retry_delay / 1_000_000_000);
    retentionEnabled = policy.retention !== undefined;
    retention = Math.round((policy.retention ?? 2_592_000_000_000_000) / 1_000_000_000);
    approvalLifetime = Math.round((policy.approval_ttl ?? 7_200_000_000_000) / 1_000_000_000);
    const configuration = policy.configuration ?? {};
    activeCheck = numberValue(configuration.active_check_seconds, 300);
    noCheckGrace = numberValue(configuration.no_check_grace_seconds, 600);
    deferAfter = numberValue(configuration.defer_after_seconds, 3_600);
    deferredCheck = numberValue(configuration.deferred_check_seconds, 21_600);
    passingQuiet = numberValue(configuration.passing_quiet_seconds, 30);
    webhookMaxDelay = numberValue(configuration.max_delay_seconds, 300);
    webhookMaxAttempts = numberValue(configuration.max_attempts, 8);
  });

  function numberValue(value: unknown, fallback: number): number {
    return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
  }

  function configuration(): Record<string, unknown> | undefined {
    if (policy?.kind === 'pending_ci') {
      return {
        active_check_seconds: activeCheck,
        no_check_grace_seconds: noCheckGrace,
        defer_after_seconds: deferAfter,
        deferred_check_seconds: deferredCheck,
        passing_quiet_seconds: passingQuiet,
      };
    }
    if (policy?.kind === 'webhook_delivery') {
      return { max_delay_seconds: webhookMaxDelay, max_attempts: webhookMaxAttempts };
    }

    return policy?.configuration;
  }

  function invalid(): boolean {
    if (
      cadence < 0 ||
      (enabled && policy !== null && recurringKinds.has(policy.kind) && cadence <= 0) ||
      retryDelay < 0 ||
      profileId === ''
    )
      return true;
    if (retentionEnabled && retention < 0) return true;
    if (policy?.kind === 'sync_scan' && approvalLifetime <= 0) return true;
    if (policy?.kind === 'pending_ci')
      return (
        activeCheck <= 0 ||
        noCheckGrace <= 0 ||
        deferAfter <= 0 ||
        deferredCheck <= 0 ||
        passingQuiet < 0
      );
    if (policy?.kind === 'webhook_delivery')
      return webhookMaxDelay <= 0 || webhookMaxAttempts < 1 || webhookMaxAttempts > 100;

    return false;
  }

  function submit(): void {
    if (policy === null) return;
    onSubmit({
      enabled,
      cadence_seconds: cadence,
      profile_id: profileId,
      default_priority: priority,
      retry_delay_seconds: retryDelay,
      retention_seconds: retentionEnabled ? retention : undefined,
      approval_lifetime_seconds: policy.kind === 'sync_scan' ? approvalLifetime : undefined,
      configuration: configuration(),
      expected_revision: policy.revision,
    });
  }
</script>

<!--
@component
How much work of one kind the service will run at once, and when it is allowed to.
Editing a policy is editing a rule rather than a record, which is why it confirms: the
next thing the queue does follows from it.

The schedule profiles are passed in rather than fetched, so the dialog cannot offer a
window that no longer exists by the time it opens.
-->

<ConfirmDialog
  id="policy-editor"
  open={policy !== null}
  title="Configure job"
  description={policy === null ? undefined : workloadTitle(policy.kind)}
  {busy}
  busyLabel="Saving…"
  confirmLabel="Save job"
  confirmTone="signal"
  confirmDisabled={invalid()}
  {onClose}
  onConfirm={submit}
>
  <div class="policy-form">
    <label class="check-line"
      ><input type="checkbox" bind:checked={enabled} /><span>Run this job</span></label
    >
    <label for="policy-cadence">How often, in seconds</label>
    <input
      id="policy-cadence"
      type="number"
      min={enabled && policy !== null && recurringKinds.has(policy.kind) ? 1 : 0}
      step="30"
      bind:value={cadence}
    />
    <label for="policy-window">Hours</label>
    <select id="policy-window" bind:value={profileId}>
      {#each profiles as profile (profile.id)}
        <option value={profile.id}>{profile.name} · {profile.timezone}</option>
      {/each}
    </select>
    <label for="policy-priority">Default priority</label>
    <select id="policy-priority" bind:value={priority}>
      <option value="low">Low</option>
      <option value="normal">Normal</option>
      <option value="high">High</option>
      <option value="urgent">Urgent</option>
    </select>
    <label for="policy-retry">Wait before retrying, in seconds</label>
    <input id="policy-retry" type="number" min="0" step="5" bind:value={retryDelay} />
    <label class="check-line"
      ><input type="checkbox" bind:checked={retentionEnabled} /><span
        >Delete finished records after a while</span
      ></label
    >
    {#if retentionEnabled}
      <label for="policy-retention">Keep finished records for, in seconds</label>
      <input id="policy-retention" type="number" min="0" step="3600" bind:value={retention} />
    {/if}
    {#if policy?.kind === 'sync_scan'}
      <fieldset>
        <legend>Sync plan safety</legend>
        <label for="policy-approval">An approval expires after, in seconds</label>
        <input id="policy-approval" type="number" min="1" step="60" bind:value={approvalLifetime} />
      </fieldset>
    {:else if policy?.kind === 'pending_ci'}
      <fieldset class="job-fields">
        <legend>Pending CI timing</legend>
        <label for="policy-active-check">Look again every, in seconds</label>
        <input id="policy-active-check" type="number" min="1" bind:value={activeCheck} />
        <label for="policy-no-check">Wait for checks to appear, in seconds</label>
        <input id="policy-no-check" type="number" min="1" bind:value={noCheckGrace} />
        <label for="policy-defer-after">Slow down after no progress for, in seconds</label>
        <input id="policy-defer-after" type="number" min="1" bind:value={deferAfter} />
        <label for="policy-deferred-check">Then look every, in seconds</label>
        <input id="policy-deferred-check" type="number" min="1" bind:value={deferredCheck} />
        <label for="policy-quiet">Quiet period after checks pass, in seconds</label>
        <input id="policy-quiet" type="number" min="0" bind:value={passingQuiet} />
      </fieldset>
    {:else if policy?.kind === 'webhook_delivery'}
      <fieldset class="job-fields">
        <legend>Webhook retry budget</legend>
        <label for="policy-webhook-max-delay">Longest wait between attempts, in seconds</label>
        <input id="policy-webhook-max-delay" type="number" min="1" bind:value={webhookMaxDelay} />
        <label for="policy-webhook-attempts">Attempts before giving up</label>
        <input
          id="policy-webhook-attempts"
          type="number"
          min="1"
          max="100"
          bind:value={webhookMaxAttempts}
        />
      </fieldset>
    {/if}
    {#if error !== ''}<p class="form-error" role="alert">{error}</p>{/if}
  </div>
</ConfirmDialog>

<style>
  .policy-form {
    display: grid;
    gap: var(--space-3);
  }
  label:not(.check-line) {
    font-size: 0.75rem;
    font-weight: 720;
    margin-bottom: calc(var(--space-2) * -1);
  }
  .check-line {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    min-height: 2.75rem;
  }
  input[type='number'],
  select {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--radius-control);
    color: var(--text-primary);
    font: inherit;
    min-height: 2.75rem;
    padding: 0 var(--space-3);
  }
  fieldset {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    display: grid;
    gap: var(--space-3);
    margin: 0;
    padding: var(--space-3);
  }
  legend {
    font-size: 0.75rem;
    font-weight: 720;
  }
  .form-error {
    color: var(--danger);
    margin: 0;
  }
</style>
