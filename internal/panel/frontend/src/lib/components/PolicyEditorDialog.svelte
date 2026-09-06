<script lang="ts">
  import { onMount } from 'svelte';
  import type {
    QueuePolicy,
    QueuePolicyInput,
    QueuePriority,
    ScheduleProfile,
  } from '#lib/types.js';
  import { workloadCadenceDescription, workloadTitle } from '#lib/workloads.js';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import DurationInput from './DurationInput.svelte';
  import Select from './Select.svelte';
  import Switch from './Switch.svelte';
  import FormError from './FormError.svelte';

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

  let durationProblems = $state<Record<string, string | null>>({});
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
    'config_file_sync',
    'sync_scan',
    'path_refresh',
    'delivery_cleanup',
    'auth_cleanup',
  ]);
  const cadenceDescription = $derived(
    policy === null ? undefined : workloadCadenceDescription(policy.kind),
  );

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
    if (Object.values(durationProblems).some((problem) => problem !== null)) return true;
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
      return (
        webhookMaxDelay <= 0 ||
        !Number.isInteger(webhookMaxAttempts) ||
        webhookMaxAttempts < 1 ||
        webhookMaxAttempts > 100
      );

    return false;
  }

  function submit(): void {
    if (policy === null || invalid()) return;
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
  <div class="form-stack policy-form">
    <div class="form-row">
      <span class="form-label">Run this job</span>
      <span class="policy-value"
        ><Switch
          bare
          checked={enabled}
          label="Run this job"
          disabled={busy}
          onToggle={(next) => (enabled = next)}
        /></span
      >
    </div>
    <div
      class="form-field"
      role="group"
      aria-labelledby="policy-cadence-label"
      aria-describedby={cadenceDescription === undefined ? undefined : 'policy-cadence-help'}
    >
      <label class="form-label" id="policy-cadence-label" for="policy-cadence">How often</label>
      <DurationInput
        id="policy-cadence"
        label="How often"
        amountLabel="How often"
        value={cadence}
        units={['seconds', 'minutes', 'hours', 'days']}
        minimum={enabled && policy !== null && recurringKinds.has(policy.kind) ? 1 : 0}
        disabled={busy}
        onChange={(seconds) => {
          if (seconds !== null) cadence = seconds;
        }}
        onValidityChange={(problem) => (durationProblems.cadence = problem)}
      />
      {#if cadenceDescription !== undefined}
        <p id="policy-cadence-help" class="form-help">{cadenceDescription}</p>
      {/if}
    </div>
    <div class="form-field">
      <label class="form-label" for="policy-window">Hours</label>
      <Select
        disabled={busy}
        id="policy-window"
        bind:value={profileId}
        options={profiles.map((profile) => ({
          value: profile.id,
          label: `${profile.name} · ${profile.timezone}`,
        }))}
      />
    </div>
    <div class="form-field">
      <label class="form-label" for="policy-priority">Default priority</label>
      <Select
        disabled={busy}
        id="policy-priority"
        bind:value={priority}
        options={[
          { value: 'low', label: 'Low' },
          { value: 'normal', label: 'Normal' },
          { value: 'high', label: 'High' },
          { value: 'urgent', label: 'Urgent' },
        ]}
      />
    </div>
    <div class="form-field">
      <label class="form-label" for="policy-retry">Wait before retrying</label>
      <DurationInput
        id="policy-retry"
        label="Wait before retrying"
        amountLabel="Wait before retrying"
        value={retryDelay}
        units={['seconds', 'minutes', 'hours', 'days']}
        minimum={0}
        disabled={busy}
        onChange={(seconds) => {
          if (seconds !== null) retryDelay = seconds;
        }}
        onValidityChange={(problem) => (durationProblems.retryDelay = problem)}
      />
    </div>
    <div class="form-row">
      <span class="form-label">Delete finished records after a while</span>
      <span class="policy-value"
        ><Switch
          bare
          checked={retentionEnabled}
          label="Delete finished records after a while"
          disabled={busy}
          onToggle={(next) => (retentionEnabled = next)}
        /></span
      >
    </div>
    {#if retentionEnabled}
      <div class="form-field">
        <label class="form-label" for="policy-retention">Keep finished records for</label>
        <DurationInput
          id="policy-retention"
          label="Keep finished records for"
          amountLabel="Keep finished records for"
          value={retention}
          units={['seconds', 'minutes', 'hours', 'days']}
          minimum={0}
          disabled={busy}
          onChange={(seconds) => {
            if (seconds !== null) retention = seconds;
          }}
          onValidityChange={(problem) => (durationProblems.retention = problem)}
        />
      </div>
    {/if}
    {#if policy?.kind === 'sync_scan'}
      <section class="form-section">
        <h3 class="card-title">Sync plan safety</h3>
        <div class="form-field">
          <label class="form-label" for="policy-approval">An approval expires after</label>
          <DurationInput
            id="policy-approval"
            label="An approval expires after"
            amountLabel="An approval expires after"
            value={approvalLifetime}
            units={['seconds', 'minutes', 'hours', 'days']}
            minimum={1}
            disabled={busy}
            onChange={(seconds) => {
              if (seconds !== null) approvalLifetime = seconds;
            }}
            onValidityChange={(problem) => (durationProblems.approvalLifetime = problem)}
          />
        </div>
      </section>
    {:else if policy?.kind === 'pending_ci'}
      <section class="form-section">
        <h3 class="card-title">Pending CI timing</h3>
        <div class="form-field">
          <label class="form-label" for="policy-active-check">Look again every</label>
          <DurationInput
            id="policy-active-check"
            label="Look again every"
            amountLabel="Look again every"
            value={activeCheck}
            units={['seconds', 'minutes', 'hours', 'days']}
            minimum={1}
            disabled={busy}
            onChange={(seconds) => {
              if (seconds !== null) activeCheck = seconds;
            }}
            onValidityChange={(problem) => (durationProblems.activeCheck = problem)}
          />
        </div>
        <div class="form-field">
          <label class="form-label" for="policy-no-check">Wait for checks to appear</label>
          <DurationInput
            id="policy-no-check"
            label="Wait for checks to appear"
            amountLabel="Wait for checks to appear"
            value={noCheckGrace}
            units={['seconds', 'minutes', 'hours', 'days']}
            minimum={1}
            disabled={busy}
            onChange={(seconds) => {
              if (seconds !== null) noCheckGrace = seconds;
            }}
            onValidityChange={(problem) => (durationProblems.noCheckGrace = problem)}
          />
        </div>
        <div class="form-field">
          <label class="form-label" for="policy-defer-after">Slow down after no progress for</label>
          <DurationInput
            id="policy-defer-after"
            label="Slow down after no progress for"
            amountLabel="Slow down after no progress for"
            value={deferAfter}
            units={['seconds', 'minutes', 'hours', 'days']}
            minimum={1}
            disabled={busy}
            onChange={(seconds) => {
              if (seconds !== null) deferAfter = seconds;
            }}
            onValidityChange={(problem) => (durationProblems.deferAfter = problem)}
          />
        </div>
        <div class="form-field">
          <label class="form-label" for="policy-deferred-check">Then look every</label>
          <DurationInput
            id="policy-deferred-check"
            label="Then look every"
            amountLabel="Then look every"
            value={deferredCheck}
            units={['seconds', 'minutes', 'hours', 'days']}
            minimum={1}
            disabled={busy}
            onChange={(seconds) => {
              if (seconds !== null) deferredCheck = seconds;
            }}
            onValidityChange={(problem) => (durationProblems.deferredCheck = problem)}
          />
        </div>
        <div class="form-field">
          <label class="form-label" for="policy-quiet">Quiet period after checks pass</label>
          <DurationInput
            id="policy-quiet"
            label="Quiet period after checks pass"
            amountLabel="Quiet period after checks pass"
            value={passingQuiet}
            units={['seconds', 'minutes', 'hours', 'days']}
            minimum={0}
            disabled={busy}
            onChange={(seconds) => {
              if (seconds !== null) passingQuiet = seconds;
            }}
            onValidityChange={(problem) => (durationProblems.passingQuiet = problem)}
          />
        </div>
      </section>
    {:else if policy?.kind === 'webhook_delivery'}
      <section class="form-section">
        <h3 class="card-title">Webhook retry budget</h3>
        <div class="form-field">
          <label class="form-label" for="policy-webhook-max-delay"
            >Longest wait between attempts</label
          >
          <DurationInput
            id="policy-webhook-max-delay"
            label="Longest wait between attempts"
            amountLabel="Longest wait between attempts"
            value={webhookMaxDelay}
            units={['seconds', 'minutes', 'hours', 'days']}
            minimum={1}
            disabled={busy}
            onChange={(seconds) => {
              if (seconds !== null) webhookMaxDelay = seconds;
            }}
            onValidityChange={(problem) => (durationProblems.webhookMaxDelay = problem)}
          />
        </div>
        <div class="form-field">
          <label class="form-label" for="policy-webhook-attempts">Attempts before giving up</label>
          <input
            id="policy-webhook-attempts"
            class="text-input mono attempts-count"
            disabled={busy}
            type="number"
            min="1"
            max="100"
            bind:value={webhookMaxAttempts}
          />
        </div>
      </section>
    {/if}
    <FormError message={error} />
  </div>
</ConfirmDialog>

<style>
  .attempts-count {
    inline-size: 8ch;
  }
</style>
