<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import type { PanelApi } from '#lib/api.js';
  import type {
    QueuePolicy,
    QueuePolicyInput,
    QueuePriority,
    QueueWorkload,
    QueuePolicyStatus,
    SchedulePolicySet,
    ScheduleProfile,
    ScheduleProfileInput,
    ScheduleRequest,
  } from '#lib/types.js';
  import Button from './Button.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import PageHeader from './PageHeader.svelte';
  import Plate from './Plate.svelte';
  import PolicyEditorDialog from './PolicyEditorDialog.svelte';
  import ProfileEditorDialog from './ProfileEditorDialog.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import ScheduleWindowsEditor, { type EditableWindow } from './ScheduleWindowsEditor.svelte';

  const {
    api,
    targetId,
    rootRole = '',
    canRequest = false,
    actorAccountId = '',
    refreshRevision = 0,
  }: {
    api: PanelApi;
    targetId?: string;
    rootRole?: string;
    canRequest?: boolean;
    actorAccountId?: string;
    refreshRevision?: number;
  } = $props();

  let policies = $state.raw<QueuePolicy[]>([]);
  let policySet = $state.raw<SchedulePolicySet | null>(null);
  let statuses = $state.raw<QueuePolicyStatus[]>([]);
  let profiles = $state.raw<ScheduleProfile[]>([]);
  let requests = $state.raw<ScheduleRequest[]>([]);
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');
  let editingPolicy = $state<QueuePolicy | null>(null);
  let revertingPolicy = $state<QueuePolicy | null>(null);
  let editingProfile = $state<ScheduleProfile | null>(null);
  let profileOpen = $state(false);
  let dialogBusy = $state(false);
  let dialogError = $state('');
  let deciding = $state<ScheduleRequest | null>(null);
  let archivingProfile = $state<ScheduleProfile | null>(null);
  let withdrawingRequest = $state<ScheduleRequest | null>(null);
  let decision = $state<'approve' | 'reject'>('approve');
  let decisionReason = $state('');
  let promoteProfile = $state(false);

  let requestKind = $state<QueueWorkload>('sync_scan');
  let requestProfile = $state('always-open');
  let requestWindowMode = $state<'existing' | 'custom'>('existing');
  let requestCustomName = $state('Installation hours');
  let requestTimezone = $state(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC');
  let requestWindows = $state.raw<EditableWindow[]>([
    { id: 'request-1', weekday: 1, start: '09:00', end: '17:00' },
    { id: 'request-2', weekday: 2, start: '09:00', end: '17:00' },
    { id: 'request-3', weekday: 3, start: '09:00', end: '17:00' },
    { id: 'request-4', weekday: 4, start: '09:00', end: '17:00' },
    { id: 'request-5', weekday: 5, start: '09:00', end: '17:00' },
  ]);
  let requestExceptions = $state('');
  let requestCadence = $state(21_600);
  let requestPriority = $state<QueuePriority>('normal');
  let requestReason = $state('');
  let requestBusy = $state(false);

  const installationKinds = new Set<QueueWorkload>([
    'pending_ci',
    'pending_ci_gate',
    'reaction_scan',
    'config_migration',
    'sync_scan',
    'path_refresh',
  ]);
  const requestablePolicies = $derived(
    policies.filter((policy) => installationKinds.has(policy.kind)),
  );

  onMount(() => void load());

  $effect(() => {
    if (refreshRevision > 0) untrack(() => void load(false));
  });

  async function load(showLoading = true): Promise<void> {
    if (showLoading) loading = true;
    try {
      if (targetId === undefined) {
        const [loadedProfiles, policyDocument, loadedRequests] = await Promise.all([
          api.fetchRootScheduleProfiles(),
          api.fetchRootJobPolicies(),
          api.fetchRootScheduleRequests(),
        ]);
        profiles = loadedProfiles;
        policies = policyDocument.policies;
        policySet = policyDocument.policy_set;
        statuses = policyDocument.statuses;
        requests = loadedRequests;
      } else {
        const [schedules, loadedRequests] = await Promise.all([
          api.fetchTargetSchedules(targetId),
          api.fetchTargetScheduleRequests(targetId),
        ]);
        profiles = schedules.profiles;
        policies = schedules.policies.effective;
        policySet = schedules.policies;
        statuses = schedules.statuses;
        requests = loadedRequests;
      }
      error = '';
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause);
    } finally {
      loading = false;
    }
  }

  function duration(value: number): string {
    const seconds = Math.round(value / 1_000_000_000);
    if (seconds === 0) return 'Immediate';
    if (seconds % 86_400 === 0) return `${seconds / 86_400}d`;
    if (seconds % 3_600 === 0) return `${seconds / 3_600}h`;
    if (seconds % 60 === 0) return `${seconds / 60}m`;
    return `${seconds}s`;
  }

  function profileName(id: string): string {
    return profiles.find((profile) => profile.id === id)?.name ?? id;
  }

  function policyStatus(kind: QueueWorkload): QueuePolicyStatus | undefined {
    return statuses.find((status) => status.kind === kind);
  }

  function instant(value?: string): string {
    if (value === undefined) return 'None scheduled';
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: 'medium',
      timeStyle: 'short',
    }).format(new Date(value));
  }

  function policySource(kind: QueueWorkload): string {
    if (policySet?.overrides.some((policy) => policy.kind === kind)) return 'Installation override';
    return 'Deployment default';
  }

  function numberSetting(policy: QueuePolicy, key: string): number | undefined {
    const value = policy.configuration?.[key];
    return typeof value === 'number' ? value : undefined;
  }

  function jobDetails(policy: QueuePolicy): string[] {
    const details: string[] = [];
    if (policy.approval_ttl !== undefined)
      details.push(`Approval lifetime ${duration(policy.approval_ttl)}`);
    if (policy.retention !== undefined) details.push(`Retention ${duration(policy.retention)}`);
    if (policy.kind === 'pending_ci') {
      const timing: Array<[string, string]> = [
        ['Active', 'active_check_seconds'],
        ['Discovery grace', 'no_check_grace_seconds'],
        ['Defer after', 'defer_after_seconds'],
        ['Deferred', 'deferred_check_seconds'],
        ['Quiet', 'passing_quiet_seconds'],
      ];
      for (const [label, key] of timing) {
        const seconds = numberSetting(policy, key);
        if (seconds !== undefined) details.push(`${label} ${duration(seconds * 1_000_000_000)}`);
      }
    }
    if (policy.kind === 'webhook_delivery') {
      const maxDelay = numberSetting(policy, 'max_delay_seconds');
      const maxAttempts = numberSetting(policy, 'max_attempts');
      if (maxDelay !== undefined) details.push(`Retry cap ${duration(maxDelay * 1_000_000_000)}`);
      if (maxAttempts !== undefined) details.push(`${maxAttempts} attempts`);
    }

    return details;
  }

  async function savePolicy(input: QueuePolicyInput): Promise<void> {
    if (editingPolicy === null) return;
    dialogBusy = true;
    try {
      const saved =
        editingPolicy.target_id === undefined
          ? await api.updateRootJobPolicy(editingPolicy.kind, input)
          : await api.updateRootInstallationJobPolicy(
              editingPolicy.target_id,
              editingPolicy.kind,
              input,
            );
      policies = policies.map((policy) => (policy.kind === saved.kind ? saved : policy));
      notice = `${saved.kind.replaceAll('_', ' ')} policy saved`;
      editingPolicy = null;
      dialogError = '';
      await load();
    } catch (cause) {
      dialogError = cause instanceof Error ? cause.message : String(cause);
    } finally {
      dialogBusy = false;
    }
  }

  async function revertPolicy(): Promise<void> {
    if (revertingPolicy?.target_id === undefined) return;
    dialogBusy = true;
    try {
      await api.deleteRootInstallationJobPolicy(
        revertingPolicy.target_id,
        revertingPolicy.kind,
        revertingPolicy.revision,
      );
      notice = `${revertingPolicy.kind.replaceAll('_', ' ')} now inherits the deployment default`;
      revertingPolicy = null;
      dialogError = '';
      await load();
    } catch (cause) {
      dialogError = cause instanceof Error ? cause.message : String(cause);
    } finally {
      dialogBusy = false;
    }
  }

  async function saveProfile(input: ScheduleProfileInput): Promise<void> {
    dialogBusy = true;
    try {
      const saved =
        editingProfile === null
          ? await api.createRootScheduleProfile(input)
          : await api.updateRootScheduleProfile(editingProfile.id, input);
      profiles = [...profiles.filter((profile) => profile.id !== saved.id), saved].sort((a, b) =>
        a.name.localeCompare(b.name),
      );
      notice = `${saved.name} saved`;
      profileOpen = false;
      editingProfile = null;
      dialogError = '';
      await load();
    } catch (cause) {
      dialogError = cause instanceof Error ? cause.message : String(cause);
    } finally {
      dialogBusy = false;
    }
  }

  async function archiveProfile(): Promise<void> {
    if (archivingProfile === null) return;
    dialogBusy = true;
    try {
      await api.archiveRootScheduleProfile(archivingProfile.id, archivingProfile.revision);
      profiles = profiles.filter((profile) => profile.id !== archivingProfile?.id);
      notice = `${archivingProfile.name} archived`;
      archivingProfile = null;
      dialogError = '';
    } catch (cause) {
      dialogError = cause instanceof Error ? cause.message : String(cause);
    } finally {
      dialogBusy = false;
    }
  }

  async function withdrawRequest(): Promise<void> {
    if (targetId === undefined || withdrawingRequest === null) return;
    dialogBusy = true;
    try {
      const saved = await api.withdrawTargetScheduleRequest(
        targetId,
        withdrawingRequest.id,
        withdrawingRequest.revision,
      );
      requests = requests.map((request) => (request.id === saved.id ? saved : request));
      notice = 'Schedule request withdrawn';
      withdrawingRequest = null;
      dialogError = '';
    } catch (cause) {
      dialogError = cause instanceof Error ? cause.message : String(cause);
    } finally {
      dialogBusy = false;
    }
  }

  function openDecision(request: ScheduleRequest, choice: 'approve' | 'reject'): void {
    deciding = request;
    decision = choice;
    decisionReason = '';
    promoteProfile = false;
    dialogError = '';
  }

  async function submitDecision(): Promise<void> {
    if (deciding === null || decisionReason.trim() === '') return;
    dialogBusy = true;
    try {
      const saved = await api.decideRootScheduleRequest(deciding.id, {
        approve: decision === 'approve',
        promote_profile: promoteProfile,
        reason: decisionReason.trim(),
        expected_revision: deciding.revision,
      });
      requests = requests.map((request) => (request.id === saved.id ? saved : request));
      notice = `Schedule request ${saved.state}`;
      deciding = null;
      await load();
    } catch (cause) {
      dialogError = cause instanceof Error ? cause.message : String(cause);
    } finally {
      dialogBusy = false;
    }
  }

  async function submitRequest(): Promise<void> {
    if (targetId === undefined || requestReason.trim() === '') return;
    const current = policies.find((policy) => policy.kind === requestKind);
    if (current === undefined) return;
    requestBusy = true;
    try {
      const customProfile: ScheduleProfile = {
        id: '',
        name: requestCustomName.trim(),
        timezone: requestTimezone.trim(),
        system: false,
        revision: 0,
        windows: requestWindows.map((window) => ({
          weekday: window.weekday,
          start_minute: timeMinute(window.start),
          end_minute: timeMinute(window.end),
        })),
        exceptions: parseRequestExceptions(),
      };
      const saved = await api.createTargetScheduleRequest(targetId, {
        kind: requestKind,
        base_revision: current.revision,
        ...(requestWindowMode === 'existing'
          ? { profile_id: requestProfile }
          : { custom_profile: customProfile }),
        cadence_seconds: requestCadence,
        default_priority: requestPriority,
        configuration: current.configuration,
        reason: requestReason.trim(),
      });
      requests = [saved, ...requests];
      requestReason = '';
      notice = 'Schedule change sent to Root for approval';
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause);
    } finally {
      requestBusy = false;
    }
  }

  function timeMinute(value: string): number {
    const [hour = '0', minute = '0'] = value.split(':');
    return Number(hour) * 60 + Number(minute);
  }

  function parseRequestExceptions(): ScheduleProfile['exceptions'] {
    return requestExceptions
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)
      .map((line) => {
        const [date = '', span = 'closed'] = line.split(/\s+/, 2);
        if (span === 'closed') return { date, closed: true };
        const [from = '00:00', to = '00:00'] = span.split('-', 2);
        return {
          date,
          closed: false,
          start_minute: timeMinute(from),
          end_minute: timeMinute(to),
        };
      });
  }
</script>

<section
  class="schedules-view"
  aria-labelledby={targetId === undefined ? 'root-page-heading' : 'schedules-heading'}
>
  {#if targetId === undefined}
    <RootPageHeader
      role={rootRole}
      title="Schedules"
      subtitle="Execution windows, workload cadence, retries, and installation requests"
    >
      <Button
        tone="signal"
        onclick={() => {
          editingProfile = null;
          profileOpen = true;
        }}>New profile</Button
      >
    </RootPageHeader>
  {:else}
    <PageHeader
      id="schedules-heading"
      title="Schedules"
      description="Effective background-work policy for this installation"
    />
  {/if}

  <p class="sr-only" aria-live="polite">{notice}</p>
  {#if loading && policies.length === 0 && profiles.length === 0}
    <Plate label="Loading"><p class="dim" role="status">Reading schedule policy…</p></Plate>
  {:else if error !== ''}
    <Plate label="Schedules unavailable" tone="alarm"
      ><p>{error}</p>
      <Button onclick={() => void load()}>Try again</Button></Plate
    >
  {:else}
    <section class="schedule-section" aria-labelledby="policy-heading">
      <div class="section-heading">
        <div>
          <span class="eyebrow">Effective settings</span>
          <h2 id="policy-heading">Workload policies</h2>
        </div>
        <span class="dim">{policies.length} workloads</span>
      </div>
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div class="policy-table-wrap" tabindex="0" role="region" aria-label="Workload policies">
        <table>
          <thead
            ><tr
              ><th>Workload</th><th>Cadence</th><th>Window</th><th>Priority</th><th>Retry</th><th
                >Runtime</th
              ><th>Job details</th>{#if targetId === undefined}<th>Action</th>{/if}</tr
            ></thead
          >
          <tbody>
            {#each policies as policy (policy.kind)}
              {@const status = policyStatus(policy.kind)}
              {@const details = jobDetails(policy)}
              <tr>
                <th scope="row"
                  ><strong>{policy.kind.replaceAll('_', ' ')}</strong><span
                    >{policy.enabled ? 'Enabled' : 'Disabled'} · {targetId === undefined
                      ? 'deployment default'
                      : policySource(policy.kind)} · revision {policy.revision}</span
                  ></th
                >
                <td>{duration(policy.cadence)}</td>
                <td>{profileName(policy.profile_id)}</td>
                <td>{policy.default_priority}</td>
                <td>{duration(policy.retry_delay)}</td>
                <td class="runtime-cell">
                  <span>{status?.current_state?.replaceAll('_', ' ') ?? 'Idle'}</span>
                  <small
                    >Last {instant(status?.last_run_at)}{#if status?.last_state}
                      · {status.last_state}{/if}</small
                  >
                  <small>Next {instant(status?.next_eligibility_at)}</small>
                  {#if status?.estimated_start_at}<small
                      >Estimate {instant(status.estimated_start_at)} · {status.work_ahead} ahead</small
                    >{/if}
                </td>
                <td class="details-cell">
                  {#if details.length === 0}<span class="dim">Standard policy</span>{/if}
                  {#each details as detail (detail)}<small>{detail}</small>{/each}
                </td>
                {#if targetId === undefined}<td
                    ><Button
                      row
                      onclick={() => {
                        editingPolicy = policy;
                        dialogError = '';
                      }}>Configure</Button
                    ></td
                  >{/if}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </section>

    {#if targetId === undefined && (policySet?.overrides.length ?? 0) > 0}
      <section class="schedule-section" aria-labelledby="overrides-heading">
        <div class="section-heading">
          <div>
            <span class="eyebrow">Approved installation settings</span>
            <h2 id="overrides-heading">Overrides</h2>
          </div>
          <span class="dim">{policySet?.overrides.length ?? 0} active</span>
        </div>
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <div
          class="policy-table-wrap"
          tabindex="0"
          role="region"
          aria-label="Installation overrides"
        >
          <table class="overrides-table">
            <thead
              ><tr
                ><th>Installation</th><th>Workload</th><th>Cadence</th><th>Window</th><th
                  >Priority</th
                ><th>Revision</th><th>Action</th></tr
              ></thead
            >
            <tbody>
              {#each policySet?.overrides ?? [] as policy (`${policy.target_id}:${policy.kind}`)}
                <tr>
                  <td><code>{policy.target_id}</code></td>
                  <th scope="row">{policy.kind.replaceAll('_', ' ')}</th>
                  <td>{duration(policy.cadence)}</td>
                  <td>{profileName(policy.profile_id)}</td>
                  <td>{policy.default_priority}</td>
                  <td>{policy.revision}</td>
                  <td
                    ><div class="request-buttons">
                      <Button row onclick={() => (editingPolicy = policy)}>Configure</Button>
                      <Button row tone="stop-quiet" onclick={() => (revertingPolicy = policy)}
                        >Use default</Button
                      >
                    </div></td
                  >
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </section>
    {/if}

    <section class="schedule-section" aria-labelledby="profile-heading">
      <div class="section-heading">
        <div>
          <span class="eyebrow">Named windows</span>
          <h2 id="profile-heading">Profiles</h2>
        </div>
      </div>
      <div class="profile-grid">
        {#each profiles as profile (profile.id)}
          <article class="profile-card">
            <div>
              <strong>{profile.name}</strong><span
                >{profile.timezone} · revision {profile.revision}</span
              >
            </div>
            <span
              >{profile.windows.length} weekly windows · {profile.exceptions.length} exceptions</span
            >
            {#if targetId === undefined && !profile.system}
              <div class="profile-actions">
                <Button
                  row
                  onclick={() => {
                    editingProfile = profile;
                    profileOpen = true;
                    dialogError = '';
                  }}>Edit</Button
                >
                <Button
                  row
                  tone="stop-quiet"
                  onclick={() => {
                    archivingProfile = profile;
                    dialogError = '';
                  }}>Archive</Button
                >
              </div>
            {/if}
          </article>
        {/each}
      </div>
    </section>

    {#if targetId !== undefined && canRequest}
      <section class="schedule-section request-form" aria-labelledby="request-heading">
        <div class="section-heading">
          <div>
            <span class="eyebrow">Installation override</span>
            <h2 id="request-heading">Request a recurring change</h2>
          </div>
        </div>
        <label
          >Workload<select bind:value={requestKind}
            >{#each requestablePolicies as policy (policy.kind)}<option value={policy.kind}
                >{policy.kind.replaceAll('_', ' ')}</option
              >{/each}</select
          ></label
        >
        <label
          >Window source<select bind:value={requestWindowMode}
            ><option value="existing">Named profile</option><option value="custom"
              >Custom hours</option
            ></select
          ></label
        >
        {#if requestWindowMode === 'existing'}
          <label
            >Window<select bind:value={requestProfile}
              >{#each profiles as profile (profile.id)}<option value={profile.id}
                  >{profile.name}</option
                >{/each}</select
            ></label
          >
        {:else}
          <label>Profile name<input bind:value={requestCustomName} /></label>
          <label>Timezone<input bind:value={requestTimezone} placeholder="Europe/Warsaw" /></label>
          <div class="custom-window">
            <ScheduleWindowsEditor
              idPrefix="request-window"
              windows={requestWindows}
              onChange={(next) => (requestWindows = next)}
            />
          </div>
          <label class="request-exceptions"
            >Date exceptions<textarea
              rows="4"
              bind:value={requestExceptions}
              placeholder="2026-12-25 closed&#10;2026-12-31 09:00-13:00"></textarea></label
          >
          <p class="request-helper">
            One local date per line: <code>YYYY-MM-DD closed</code> or
            <code>YYYY-MM-DD HH:MM-HH:MM</code>.
          </p>
        {/if}
        <label
          >Cadence seconds<input
            type="number"
            min="0"
            step="60"
            bind:value={requestCadence}
          /></label
        >
        <label
          >Priority<select bind:value={requestPriority}
            ><option value="low">Low</option><option value="normal">Normal</option><option
              value="high">High</option
            ><option value="urgent">Urgent</option></select
          ></label
        >
        <label class="reason-field"
          >Reason<textarea
            rows="3"
            bind:value={requestReason}
            placeholder="Explain the operational need"></textarea></label
        >
        <div class="request-action">
          <Button
            tone="signal"
            disabled={requestBusy || requestReason.trim() === ''}
            onclick={() => void submitRequest()}>{requestBusy ? 'Sending…' : 'Send request'}</Button
          >
        </div>
      </section>
    {/if}

    <section class="schedule-section" aria-labelledby="requests-heading">
      <div class="section-heading">
        <div>
          <span class="eyebrow">Approval trail</span>
          <h2 id="requests-heading">Schedule requests</h2>
        </div>
      </div>
      {#if requests.length === 0}<p class="dim">No schedule requests yet.</p>{/if}
      {#each requests as request (request.id)}
        <article class="request-row">
          <div>
            <strong>{request.kind.replaceAll('_', ' ')}</strong><span>{request.reason}</span>
            {#if request.custom_profile !== undefined}<span
                >{request.custom_profile.name} · {request.custom_profile.timezone}</span
              >{/if}
            <span
              >{duration(request.cadence)} cadence · {request.default_priority} priority · base revision
              {request.base_revision} ({request.base_target_id === undefined
                ? 'global policy'
                : 'installation override'})</span
            >
          </div>
          <span class="request-state">{request.state}</span>
          {#if targetId === undefined && request.state === 'pending'}
            <div class="request-buttons">
              <Button row tone="signal" onclick={() => openDecision(request, 'approve')}
                >Approve</Button
              ><Button row tone="stop-quiet" onclick={() => openDecision(request, 'reject')}
                >Reject</Button
              >
            </div>
          {:else if targetId !== undefined && request.state === 'pending' && request.requested_by === actorAccountId}
            <Button
              row
              tone="stop-quiet"
              onclick={() => {
                withdrawingRequest = request;
                dialogError = '';
              }}>Withdraw</Button
            >
          {/if}
        </article>
      {/each}
    </section>
  {/if}
</section>

{#key editingPolicy?.kind ?? ''}
  <PolicyEditorDialog
    policy={editingPolicy}
    {profiles}
    busy={dialogBusy}
    error={dialogError}
    onClose={() => (editingPolicy = null)}
    onSubmit={(input) => void savePolicy(input)}
  />
{/key}
{#key `${editingProfile?.id ?? 'new'}:${profileOpen}`}
  <ProfileEditorDialog
    profile={editingProfile}
    open={profileOpen}
    busy={dialogBusy}
    error={dialogError}
    onClose={() => {
      if (!dialogBusy) profileOpen = false;
    }}
    onSubmit={(input) => void saveProfile(input)}
  />
{/key}
<ConfirmDialog
  id="revert-installation-policy"
  open={revertingPolicy !== null}
  title="Use deployment default?"
  description={revertingPolicy === null
    ? undefined
    : `${revertingPolicy.target_id} will stop overriding ${revertingPolicy.kind.replaceAll('_', ' ')}.`}
  busy={dialogBusy}
  confirmLabel="Use default"
  confirmTone="stop"
  onClose={() => {
    if (!dialogBusy) revertingPolicy = null;
  }}
  onConfirm={() => void revertPolicy()}
>
  {#if dialogError !== ''}<p class="form-error" role="alert">{dialogError}</p>{/if}
</ConfirmDialog>
<ConfirmDialog
  id="schedule-decision"
  open={deciding !== null}
  title={decision === 'approve' ? 'Approve schedule request' : 'Reject schedule request'}
  description={deciding?.reason}
  busy={dialogBusy}
  confirmLabel={decision === 'approve' ? 'Approve' : 'Reject'}
  confirmTone={decision === 'approve' ? 'signal' : 'stop'}
  confirmDisabled={decisionReason.trim() === ''}
  onClose={() => {
    if (!dialogBusy) deciding = null;
  }}
  onConfirm={() => void submitDecision()}
>
  <label class="decision-reason"
    >Decision reason<textarea rows="3" bind:value={decisionReason}></textarea></label
  >
  {#if deciding?.custom_profile !== undefined && decision === 'approve'}
    <label class="promote-profile"
      ><input type="checkbox" bind:checked={promoteProfile} />Promote these custom hours to a
      reusable global profile</label
    >
  {/if}
  {#if dialogError !== ''}<p class="form-error" role="alert">{dialogError}</p>{/if}
</ConfirmDialog>
<ConfirmDialog
  id="archive-profile"
  open={archivingProfile !== null}
  title="Archive window profile"
  description={archivingProfile === null
    ? undefined
    : `${archivingProfile.name} can only be archived after every policy is reassigned.`}
  busy={dialogBusy}
  confirmLabel="Archive profile"
  confirmTone="stop"
  onClose={() => {
    if (!dialogBusy) archivingProfile = null;
  }}
  onConfirm={() => void archiveProfile()}
>
  {#if dialogError !== ''}<p class="form-error" role="alert">{dialogError}</p>{/if}
</ConfirmDialog>
<ConfirmDialog
  id="withdraw-schedule-request"
  open={withdrawingRequest !== null}
  title="Withdraw schedule request"
  description="Root will no longer be able to approve this request. The audit record remains."
  busy={dialogBusy}
  confirmLabel="Withdraw request"
  confirmTone="stop"
  onClose={() => {
    if (!dialogBusy) withdrawingRequest = null;
  }}
  onConfirm={() => void withdrawRequest()}
>
  {#if dialogError !== ''}<p class="form-error" role="alert">{dialogError}</p>{/if}
</ConfirmDialog>

<style>
  .schedules-view {
    display: grid;
    gap: var(--space-5);
  }
  .schedule-section {
    display: grid;
    gap: var(--space-3);
  }
  .section-heading {
    align-items: end;
    display: flex;
    justify-content: space-between;
  }
  .section-heading h2 {
    font-size: 1rem;
    margin: var(--space-1) 0 0;
  }
  .eyebrow {
    color: var(--dim);
    font-size: 0.66rem;
    font-weight: 760;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }
  .policy-table-wrap {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    overflow: auto;
  }
  table {
    border-collapse: collapse;
    min-width: 48rem;
    width: 100%;
  }
  .overrides-table {
    min-width: 42rem;
  }
  th,
  td {
    border-bottom: 1px solid var(--border-subtle);
    font-size: 0.78rem;
    padding: 0.72rem 0.85rem;
    text-align: left;
  }
  thead th {
    background: var(--table-header-bg);
    color: var(--dim);
    font-size: 0.68rem;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }
  tbody tr:last-child > * {
    border-bottom: 0;
  }
  tbody th span,
  .profile-card span,
  .request-row span {
    color: var(--dim);
    display: block;
    font-size: 0.7rem;
    margin-top: var(--space-1);
  }
  .runtime-cell,
  .details-cell {
    min-width: 12rem;
  }
  .runtime-cell > span,
  .runtime-cell small,
  .details-cell small {
    display: block;
  }
  .runtime-cell > span {
    text-transform: capitalize;
  }
  .runtime-cell small,
  .details-cell small {
    color: var(--dim);
    margin-top: var(--space-1);
  }
  .profile-grid {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: repeat(auto-fit, minmax(15rem, 1fr));
  }
  .profile-card,
  .request-row {
    align-items: center;
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    display: grid;
    gap: var(--space-3);
    padding: var(--space-4);
  }
  .profile-card {
    grid-template-columns: 1fr auto auto;
  }
  .request-row {
    grid-template-columns: 1fr auto auto;
  }
  .request-state {
    border: 1px solid var(--control-border);
    border-radius: 999px;
    color: var(--text) !important;
    margin: 0 !important;
    padding: 0.22rem 0.48rem;
    text-transform: uppercase;
  }
  .request-buttons {
    display: flex;
    gap: var(--space-2);
  }
  .profile-actions {
    display: flex;
    gap: var(--space-2);
  }
  .request-form {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    grid-template-columns: repeat(4, minmax(0, 1fr));
    padding: var(--space-4);
  }
  .request-form .section-heading,
  .reason-field,
  .custom-window,
  .request-exceptions,
  .request-helper {
    grid-column: 1 / -1;
  }
  .request-form label,
  .decision-reason {
    display: grid;
    font-size: 0.72rem;
    font-weight: 720;
    gap: var(--space-1);
  }
  .custom-window {
    border: 1px solid var(--border-subtle);
  }
  .request-helper {
    color: var(--dim);
    font-size: 0.72rem;
    margin: calc(var(--space-2) * -1) 0 0;
  }
  .promote-profile {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }
  :is(select, input, textarea) {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--radius-control);
    color: var(--text);
    font: inherit;
    min-height: 2.5rem;
    padding: var(--space-2) var(--space-3);
  }
  textarea {
    line-height: 1.45;
    resize: vertical;
  }
  .request-action {
    display: flex;
    grid-column: 1 / -1;
    justify-content: flex-end;
  }
  .form-error {
    color: var(--danger);
  }
  @media (max-width: 54rem) {
    .request-form {
      grid-template-columns: 1fr 1fr;
    }
    .profile-card,
    .request-row {
      grid-template-columns: 1fr auto;
    }
    .request-buttons {
      grid-column: 1 / -1;
    }
  }
  @media (max-width: 34rem) {
    .request-form {
      grid-template-columns: 1fr;
    }
    .profile-card,
    .request-row {
      grid-template-columns: 1fr;
    }
  }
</style>
