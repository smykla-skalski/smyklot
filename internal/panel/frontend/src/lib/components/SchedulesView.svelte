<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
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
  import Chip, { type ChipTone } from './Chip.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import DataTable from './DataTable.svelte';
  import Icon, { type IconName } from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import Plate from './Plate.svelte';
  import PolicyEditorDialog from './PolicyEditorDialog.svelte';
  import ProfileEditorDialog from './ProfileEditorDialog.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import ScheduleWindowsEditor, { type EditableWindow } from './ScheduleWindowsEditor.svelte';
  import Select from './Select.svelte';

  const {
    api,
    targetId,
    rootRole = '',
    canRequest = false,
    actorAccountId = '',
  }: {
    api: PanelApi;
    targetId?: string;
    rootRole?: string;
    canRequest?: boolean;
    actorAccountId?: string;
  } = $props();

  interface ScheduleViewData {
    policies: QueuePolicy[];
    policySet: SchedulePolicySet;
    statuses: QueuePolicyStatus[];
    profiles: ScheduleProfile[];
    requests: ScheduleRequest[];
  }

  let operationError = $state('');
  const schedulesQuery = createQuery(() => ({
    queryKey: targetId === undefined ? ['schedules', 'root'] : ['schedules', 'target', targetId],
    queryFn: fetchSchedules,
  }));
  const scheduleData = $derived<ScheduleViewData | null>(schedulesQuery.data ?? null);
  const policies = $derived<QueuePolicy[]>(scheduleData?.policies ?? []);
  const policySet = $derived<SchedulePolicySet | null>(scheduleData?.policySet ?? null);
  const statuses = $derived<QueuePolicyStatus[]>(scheduleData?.statuses ?? []);
  const profiles = $derived<ScheduleProfile[]>(scheduleData?.profiles ?? []);
  const requests = $derived<ScheduleRequest[]>(scheduleData?.requests ?? []);
  const loading = $derived(schedulesQuery.isFetching);
  const queryError = $derived(errorMessage(schedulesQuery.error));
  const error = $derived(operationError || queryError);
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
  let requestProfile = $state<string | null>(null);
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
  let requestCadence = $state<number | null | undefined>(undefined);
  let requestPriority = $state<QueuePriority | null>(null);
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
  const displayedPolicies = $derived(targetId === undefined ? policies : requestablePolicies);
  const policyOverrides = $derived(policySet?.overrides ?? []);
  const activeWorkloads = $derived(
    displayedPolicies.filter((policy) => policyStatus(policy.kind)?.current_state !== undefined)
      .length,
  );
  const pendingRequests = $derived(
    requests.filter((request) => request.state === 'pending').length,
  );

  const workloadCopy: Record<QueueWorkload, { title: string; description: string }> = {
    webhook_delivery: {
      title: 'Webhook delivery',
      description: 'Accept and deliver GitHub events',
    },
    pending_ci: {
      title: 'Pending CI checks',
      description: 'Recheck merge requests waiting on CI',
    },
    pending_ci_gate: {
      title: 'Deferred CI gate',
      description: 'Wake deferred checks after their quiet period',
    },
    catalog_refresh: {
      title: 'Catalog refresh',
      description: 'Discover installations and repositories',
    },
    reaction_scan: {
      title: 'Reaction discovery',
      description: 'Find pull request approval reactions',
    },
    config_migration: {
      title: 'Configuration migration',
      description: 'Move repositories to the current configuration',
    },
    sync_scan: {
      title: 'Organization sync scan',
      description: 'Compute drift and prepare an approval plan',
    },
    sync_apply: {
      title: 'Sync plan execution',
      description: 'Apply a previously approved organization plan',
    },
    path_refresh: {
      title: 'Path indexing',
      description: 'Refresh repository configuration paths',
    },
    delivery_cleanup: {
      title: 'Delivery retention',
      description: 'Remove expired delivery history',
    },
    auth_cleanup: {
      title: 'Authentication cleanup',
      description: 'Remove expired sessions and credentials',
    },
    schedule_change: {
      title: 'Schedule change',
      description: 'Apply an approved recurring policy request',
    },
  };

  function errorMessage(cause: unknown): string {
    if (cause === null || cause === undefined) return '';
    return cause instanceof Error ? cause.message : String(cause);
  }

  async function fetchSchedules(): Promise<ScheduleViewData> {
    if (targetId === undefined) {
      const [loadedProfiles, policyDocument, loadedRequests] = await Promise.all([
        api.fetchRootScheduleProfiles(),
        api.fetchRootJobPolicies(),
        api.fetchRootScheduleRequests(),
      ]);
      return {
        profiles: loadedProfiles,
        policies: policyDocument.policies,
        policySet: policyDocument.policy_set,
        statuses: policyDocument.statuses,
        requests: loadedRequests,
      };
    }

    const [schedules, loadedRequests] = await Promise.all([
      api.fetchTargetSchedules(targetId),
      api.fetchTargetScheduleRequests(targetId),
    ]);
    return {
      profiles: schedules.profiles,
      policies: schedules.policies.effective,
      policySet: schedules.policies,
      statuses: schedules.statuses,
      requests: loadedRequests,
    };
  }

  async function load(): Promise<void> {
    operationError = '';
    await schedulesQuery.refetch();
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

  function workloadTitle(kind: QueueWorkload): string {
    return workloadCopy[kind].title;
  }

  function workloadDescription(kind: QueueWorkload): string {
    return workloadCopy[kind].description;
  }

  function policyStatus(kind: QueueWorkload): QueuePolicyStatus | undefined {
    return statuses.find((status) => status.kind === kind);
  }

  function runtimeTone(state?: string): ChipTone {
    if (state === 'running' || state === 'ready') return 'signal';
    if (state === 'failed') return 'stop';
    if (state === 'blocked' || state === 'retrying') return 'warning';
    if (state === 'succeeded') return 'clear';
    if (state === undefined) return 'absent';
    return 'neutral';
  }

  function requestTone(state: ScheduleRequest['state']): ChipTone {
    if (state === 'approved') return 'clear';
    if (state === 'rejected' || state === 'stale') return 'stop';
    if (state === 'pending') return 'warning';
    return 'absent';
  }

  function summaryIcon(index: number): IconName {
    return (['sliders', 'refresh', 'history', 'pending'] as const)[index] ?? 'sliders';
  }

  function compactInstant(value?: string): string {
    if (value === undefined) return 'not scheduled';
    return new Intl.DateTimeFormat(undefined, {
      day: 'numeric',
      month: 'short',
      hour: 'numeric',
      minute: '2-digit',
    }).format(new Date(value));
  }

  function policySource(kind: QueueWorkload): string {
    if (policyOverrides.some((policy) => policy.kind === kind)) return 'Installation override';
    return 'Global policy';
  }

  function deploymentDefault(kind: QueueWorkload): QueuePolicy | undefined {
    return policySet?.deployment_defaults.find((policy) => policy.kind === kind);
  }

  function deploymentSummary(policy: QueuePolicy): string {
    const cadence = policy.enabled ? duration(policy.cadence) : 'disabled';
    return `Deployment ${cadence} · ${profileName(policy.profile_id)} · ${policy.default_priority}`;
  }

  function numberSetting(policy: QueuePolicy, key: string): number | undefined {
    const value = policy.configuration?.[key];
    return typeof value === 'number' ? value : undefined;
  }

  function selectRequestKind(kind: QueueWorkload): void {
    requestKind = kind;
    requestCadence = undefined;
    requestPriority = null;
    requestProfile = null;
  }

  function selectedRequestPolicy(): QueuePolicy | undefined {
    return policies.find((candidate) => candidate.kind === requestKind);
  }

  function requestCadenceValue(): number | null {
    if (requestCadence !== undefined) return requestCadence;
    return Math.round((selectedRequestPolicy()?.cadence ?? 0) / 1_000_000_000);
  }

  function requestPriorityValue(): QueuePriority {
    return requestPriority ?? selectedRequestPolicy()?.default_priority ?? 'normal';
  }

  function requestProfileValue(): string {
    return requestProfile ?? selectedRequestPolicy()?.profile_id ?? profiles[0]?.id ?? '';
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
      notice = `${revertingPolicy.kind.replaceAll('_', ' ')} now inherits the global policy`;
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
      notice = `${archivingProfile.name} archived`;
      archivingProfile = null;
      dialogError = '';
      await load();
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
      await api.withdrawTargetScheduleRequest(
        targetId,
        withdrawingRequest.id,
        withdrawingRequest.revision,
      );
      notice = 'Schedule request withdrawn';
      withdrawingRequest = null;
      dialogError = '';
      await load();
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
    if (targetId === undefined || requestReason.trim() === '' || requestCadenceInvalid()) return;
    const current = policies.find((policy) => policy.kind === requestKind);
    if (current === undefined) return;
    const cadence = requestCadenceValue();
    if (cadence === null) return;
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
      await api.createTargetScheduleRequest(targetId, {
        kind: requestKind,
        base_revision: current.revision,
        ...(requestWindowMode === 'existing'
          ? { profile_id: requestProfileValue() }
          : { custom_profile: customProfile }),
        cadence_seconds: cadence,
        default_priority: requestPriorityValue(),
        configuration: current.configuration,
        reason: requestReason.trim(),
      });
      requestReason = '';
      notice = 'Schedule change sent to Root for approval';
      await load();
    } catch (cause) {
      operationError = cause instanceof Error ? cause.message : String(cause);
    } finally {
      requestBusy = false;
    }
  }

  function requestCadenceInvalid(): boolean {
    const cadence = requestCadenceValue();
    if (cadence === null || !Number.isFinite(cadence)) return true;
    return cadence < 0 || (requestKind !== 'pending_ci' && cadence <= 0);
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

{#snippet policyCells(policy: QueuePolicy)}
  {@const status = policyStatus(policy.kind)}
  {@const details = jobDetails(policy)}
  {@const baseline = deploymentDefault(policy.kind)}
  {@const statusTone = runtimeTone(status?.current_state)}
  <th scope="row" data-label="Workload">
    <div class="policy-work band-trim-stack">
      <div class="policy-title-line">
        <strong>{workloadTitle(policy.kind)}</strong>
        <Chip tone={policy.enabled ? 'accent' : 'absent'} small>
          {policy.enabled ? 'Enabled' : 'Disabled'}
        </Chip>
      </div>
      <span class="policy-description">{workloadDescription(policy.kind)}</span>
      <span class="policy-source">
        {targetId === undefined ? 'Global policy' : policySource(policy.kind)} · revision
        {policy.revision}
      </span>
    </div>
  </th>
  <td data-label="Schedule">
    <dl class="policy-facts">
      <div>
        <dt>Cadence</dt>
        <dd>{duration(policy.cadence)}</dd>
      </div>
      <div>
        <dt>Window</dt>
        <dd>{profileName(policy.profile_id)}</dd>
      </div>
    </dl>
  </td>
  <td data-label="Runtime">
    <div class="runtime-summary band-trim-stack">
      <span class="runtime-state runtime-state-{statusTone}">
        <span class="runtime-dot" aria-hidden="true"></span>
        <strong>{status?.current_state?.replaceAll('_', ' ') ?? 'Idle'}</strong>
      </span>
      <span>Next {compactInstant(status?.next_eligibility_at)}</span>
      <span>Last {compactInstant(status?.last_run_at)}</span>
      {#if status?.estimated_start_at}
        <span
          >Est. {compactInstant(status.estimated_start_at)} · {status.work_ahead === 0
            ? 'next'
            : `${status.work_ahead} ahead`}</span
        >
      {/if}
    </div>
  </td>
  <td data-label="Policy">
    <div class="policy-detail band-trim-stack">
      <div class="policy-chip-line">
        <Chip tone={policy.default_priority === 'urgent' ? 'stop' : 'neutral'} small>
          {policy.default_priority} priority
        </Chip>
        <span>Retry {duration(policy.retry_delay)}</span>
      </div>
      {#each details as detail (detail)}<span>{detail}</span>{/each}
      {#if details.length === 0}<span>Standard retry and retention</span>{/if}
      {#if targetId === undefined && baseline !== undefined}
        <span>{deploymentSummary(baseline)}</span>
      {/if}
    </div>
  </td>
  {#if targetId === undefined}
    <td data-label="Action">
      <div class="policy-action">
        <Button
          row
          onclick={() => {
            editingPolicy = policy;
            dialogError = '';
          }}>Configure</Button
        >
      </div>
    </td>
  {/if}
{/snippet}

{#snippet overrideCells(policy: QueuePolicy)}
  <th class="override-installation" scope="row" data-label="Installation">
    <div class="override-stack band-trim-stack">
      <code>{policy.target_id}</code>
      <span>{workloadTitle(policy.kind)}</span>
    </div>
  </th>
  <td class="override-value" data-label="Schedule">
    <div class="override-stack band-trim-stack">
      <strong>{duration(policy.cadence)}</strong>
      <span>{profileName(policy.profile_id)}</span>
    </div>
  </td>
  <td class="override-value" data-label="Policy">
    <div class="override-stack band-trim-stack">
      <Chip tone="neutral" small>{policy.default_priority} priority</Chip>
      <span>Revision {policy.revision}</span>
    </div>
  </td>
  <td data-label="Actions">
    <div class="request-buttons">
      <Button row onclick={() => (editingPolicy = policy)}>Configure</Button>
      <Button row tone="stop-quiet" onclick={() => (revertingPolicy = policy)}>Use global</Button>
    </div>
  </td>
{/snippet}

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

  <p class="visually-hidden" aria-live="polite">{notice}</p>
  {#if loading && policies.length === 0 && profiles.length === 0}
    <Plate label="Loading"><p class="dim" role="status">Reading schedule policy…</p></Plate>
  {:else if error !== '' && scheduleData === null}
    <Plate label="Schedules unavailable" tone="alarm"
      ><p>{error}</p>
      <Button onclick={() => void load()}>Try again</Button></Plate
    >
  {:else}
    {#if error !== ''}
      <Plate label="Schedule update delayed" tone="alarm">
        <p>{error}</p>
        <Button onclick={() => void load()}>Try again</Button>
      </Plate>
    {/if}
    <div class="schedule-summary" aria-label="Schedule overview">
      {#each [{ label: 'Workloads', value: displayedPolicies.length, detail: targetId === undefined ? 'global policies' : 'installation policies' }, { label: 'Active now', value: activeWorkloads, detail: 'visible Queue items' }, { label: 'Profiles', value: profiles.length, detail: 'named execution windows' }, { label: 'Requests', value: pendingRequests, detail: 'awaiting a decision' }] as metric, index (metric.label)}
        <article>
          <span class="summary-mark"><Icon name={summaryIcon(index)} size={16} /></span>
          <div>
            <span>{metric.label}</span>
            <strong>{metric.value}</strong>
            <small>{metric.detail}</small>
          </div>
        </article>
      {/each}
    </div>

    <section class="schedule-section" aria-labelledby="policy-heading">
      <div class="section-heading">
        <div>
          <span class="eyebrow">Effective settings</span>
          <h2 id="policy-heading">Workload policies</h2>
        </div>
        <span class="dim">{displayedPolicies.length} workloads</span>
      </div>
      <DataTable
        rows={displayedPolicies}
        rowKey={(policy) => policy.kind}
        caption="Workload policies"
        regionLabel="Workload policies"
        columns={targetId === undefined
          ? [
              { label: 'Workload' },
              { label: 'Schedule' },
              { label: 'Runtime' },
              { label: 'Policy' },
              { label: 'Action' },
            ]
          : [
              { label: 'Workload' },
              { label: 'Schedule' },
              { label: 'Runtime' },
              { label: 'Policy' },
            ]}
        columnWidths={targetId === undefined
          ? ['27%', '16%', '25%', '20%', '12%']
          : ['30%', '18%', '28%', '24%']}
        cells={policyCells}
        class="policy-table-wrap"
        scrollable={false}
        stacked
      />
    </section>

    {#if targetId === undefined && policyOverrides.length > 0}
      <section class="schedule-section" aria-labelledby="overrides-heading">
        <div class="section-heading">
          <div>
            <span class="eyebrow">Approved installation settings</span>
            <h2 id="overrides-heading">Overrides</h2>
          </div>
          <span class="dim">{policyOverrides.length} active</span>
        </div>
        <DataTable
          rows={policyOverrides}
          rowKey={(policy) => `${policy.target_id}:${policy.kind}`}
          caption="Installation overrides"
          regionLabel="Installation overrides"
          columns={[
            { label: 'Installation' },
            { label: 'Schedule' },
            { label: 'Policy' },
            { label: 'Actions' },
          ]}
          columnWidths={['30%', '24%', '20%', '26%']}
          cells={overrideCells}
          class="policy-table-wrap overrides-table"
          scrollable={false}
          stacked
        />
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
            <div class="profile-heading">
              <span class="profile-mark"><Icon name="history" size={15} /></span>
              <div>
                <strong>{profile.name}</strong>
                <span>Revision {profile.revision}</span>
              </div>
              <Chip tone={profile.system ? 'accent' : 'neutral'} small>
                {profile.system ? 'System' : 'Custom'}
              </Chip>
            </div>
            <dl class="profile-facts">
              <div>
                <dt>Timezone</dt>
                <dd>{profile.timezone}</dd>
              </div>
              <div>
                <dt>Weekly</dt>
                <dd>{profile.windows.length} windows</dd>
              </div>
              <div>
                <dt>Exceptions</dt>
                <dd>{profile.exceptions.length}</dd>
              </div>
            </dl>
            {#if targetId === undefined}
              <p class="profile-impact">
                {profile.affected_installations ?? 0} installations · {profile.affected_items ?? 0}
                queued items
              </p>
            {/if}
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
        <label>
          <span>Workload</span>
          <Select
            value={requestKind}
            onchange={(event) =>
              selectRequestKind((event.currentTarget as HTMLSelectElement).value as QueueWorkload)}
          >
            {#each requestablePolicies as policy (policy.kind)}
              <option value={policy.kind}>{workloadTitle(policy.kind)}</option>
            {/each}
          </Select>
        </label>
        <label>
          <span>Window source</span>
          <Select bind:value={requestWindowMode}>
            <option value="existing">Named profile</option>
            <option value="custom">Custom hours</option>
          </Select>
        </label>
        {#if requestWindowMode === 'existing'}
          <label>
            <span>Window</span>
            <Select
              aria-label="Window profile"
              value={requestProfileValue()}
              onchange={(event) =>
                (requestProfile = (event.currentTarget as HTMLSelectElement).value)}
            >
              {#each profiles as profile (profile.id)}
                <option value={profile.id}>{profile.name}</option>
              {/each}
            </Select>
          </label>
        {:else}
          <label>
            <span>Profile name</span>
            <input class="text-input" bind:value={requestCustomName} />
          </label>
          <label>
            <span>Timezone</span>
            <input class="text-input" bind:value={requestTimezone} placeholder="Europe/Warsaw" />
          </label>
          <div class="custom-window">
            <ScheduleWindowsEditor
              idPrefix="request-window"
              windows={requestWindows}
              onChange={(next) => (requestWindows = next)}
            />
          </div>
          <label class="request-exceptions">
            <span>Date exceptions</span>
            <textarea
              class="text-input"
              rows="4"
              bind:value={requestExceptions}
              placeholder="2026-12-25 closed&#10;2026-12-31 09:00-13:00"></textarea>
          </label>
          <p class="request-helper">
            One local date per line: <code>YYYY-MM-DD closed</code> or
            <code>YYYY-MM-DD HH:MM-HH:MM</code>.
          </p>
        {/if}
        <label>
          <span>Cadence seconds</span>
          <input
            class="text-input"
            type="number"
            min={requestKind === 'pending_ci' ? 0 : 1}
            step="60"
            value={requestCadenceValue() ?? ''}
            oninput={(event) => {
              const cadence = (event.currentTarget as HTMLInputElement).valueAsNumber;
              requestCadence = Number.isFinite(cadence) ? cadence : null;
            }}
          />
        </label>
        <label>
          <span>Priority</span>
          <Select
            value={requestPriorityValue()}
            onchange={(event) =>
              (requestPriority = (event.currentTarget as HTMLSelectElement).value as QueuePriority)}
          >
            <option value="low">Low</option>
            <option value="normal">Normal</option>
            <option value="high">High</option>
            <option value="urgent">Urgent</option>
          </Select>
        </label>
        <label class="reason-field">
          <span>Reason</span>
          <textarea
            class="text-input"
            rows="3"
            bind:value={requestReason}
            placeholder="Explain the operational need"></textarea>
        </label>
        <div class="request-action">
          <Button
            tone="signal"
            disabled={requestBusy || requestReason.trim() === '' || requestCadenceInvalid()}
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
          <div class="request-copy">
            <div class="request-title">
              <strong>{workloadTitle(request.kind)}</strong>
              <Chip tone={requestTone(request.state)} small>{request.state}</Chip>
            </div>
            <span class="request-detail">{request.reason}</span>
            {#if request.custom_profile !== undefined}<span class="request-detail"
                >{request.custom_profile.name} · {request.custom_profile.timezone}</span
              >{/if}
            <span class="request-detail"
              >{duration(request.cadence)} cadence · {request.default_priority} priority · base revision
              {request.base_revision} ({request.base_target_id === undefined
                ? 'global policy'
                : 'installation override'})</span
            >
          </div>
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
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    min-width: 0;
  }
  .schedule-section {
    display: grid;
    gap: var(--space-3);
  }
  .schedule-summary {
    display: grid;
    gap: var(--space-2);
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
  .schedule-summary article {
    align-items: center;
    background: var(--surface-base);
    border: 1px solid color-mix(in srgb, var(--brand-action) 13%, var(--border-subtle));
    border-radius: var(--radius-surface);
    box-shadow: var(--shadow-plate);
    display: flex;
    gap: var(--space-3);
    min-width: 0;
    padding: var(--space-3) var(--space-4);
  }
  .summary-mark,
  .profile-mark {
    align-items: center;
    background: color-mix(in srgb, var(--brand-action) 12%, transparent);
    border-radius: var(--radius-control);
    color: var(--brand-action-text);
    display: inline-flex;
    flex: none;
    justify-content: center;
  }
  .summary-mark {
    height: 2rem;
    width: 2rem;
  }
  .schedule-summary article > div {
    display: grid;
    gap: 0.12rem;
    min-width: 0;
  }
  .schedule-summary article > div > :first-child {
    text-box: trim-start cap alphabetic;
  }
  .schedule-summary article > div > :last-child {
    text-box: trim-end cap alphabetic;
  }
  .schedule-summary article div > span,
  .schedule-summary small {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
  }
  .schedule-summary article div > span {
    font-weight: 650;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }
  .schedule-summary strong {
    color: var(--text-primary);
    font-size: 1.25rem;
    line-height: 1;
  }
  .section-heading {
    align-items: flex-end;
    display: flex;
    justify-content: space-between;
  }
  .section-heading h2 {
    font-size: var(--font-size-title);
    letter-spacing: -0.015em;
    margin: var(--space-1) 0 0;
  }
  .eyebrow {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  :global(.policy-table-wrap) {
    --table-cell-font-size: var(--font-size-meta);
    --table-cell-pad-block: var(--space-3);
    --table-cell-pad-inline: var(--space-4);
    --table-layout: fixed;
    --table-min-width: 0;
  }
  th,
  td {
    vertical-align: middle;
  }
  .policy-description,
  .policy-source,
  .request-detail {
    color: var(--text-muted);
    display: block;
    font-size: var(--font-size-compact);
    line-height: 1.35;
  }
  .policy-title-line {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .policy-title-line > strong {
    color: var(--text-primary);
    font-size: var(--font-size-body);
    text-box: trim-both cap alphabetic;
  }
  .policy-work,
  .override-stack {
    display: grid;
  }
  .policy-work {
    gap: var(--space-1);
  }
  .policy-description {
    margin-top: 0;
  }
  .policy-source,
  .request-detail {
    margin-top: 0;
  }
  .policy-source {
    font-size: var(--font-size-micro);
  }
  .policy-facts,
  .profile-facts {
    display: grid;
    gap: var(--space-1);
    margin: 0;
  }
  .policy-facts > div {
    display: grid;
    gap: 0.1rem;
  }
  .policy-facts > :first-child > dt {
    text-box: trim-start cap alphabetic;
  }
  .policy-facts > :last-child > dd {
    text-box: trim-end cap alphabetic;
  }
  .policy-facts dt,
  .profile-facts dt {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    font-weight: 650;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }
  .policy-facts dd,
  .profile-facts dd {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    margin: 0;
  }
  .runtime-summary,
  .policy-detail {
    display: grid;
    gap: var(--space-1);
  }
  .runtime-summary {
    justify-items: start;
  }
  .runtime-state {
    align-items: center;
    color: var(--text-muted);
    display: flex;
    gap: var(--space-1);
  }
  .runtime-state strong {
    font-size: var(--font-size-compact);
    font-weight: 650;
    text-box: trim-both cap alphabetic;
    text-transform: capitalize;
  }
  .runtime-dot {
    background: currentcolor;
    border-radius: 50%;
    height: 0.4rem;
    width: 0.4rem;
  }
  .runtime-state-signal,
  .runtime-state-accent {
    color: var(--brand-action-text);
  }
  .runtime-state-warning {
    color: var(--warning);
  }
  .runtime-state-stop {
    color: var(--stop);
  }
  .runtime-state-clear {
    color: var(--clear);
  }
  .runtime-summary > span,
  .policy-detail > span,
  .policy-chip-line > span,
  .override-stack > span {
    color: var(--text-muted);
    display: block;
    font-size: var(--font-size-compact);
    line-height: 1.35;
  }
  .policy-chip-line {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .policy-chip-line > span {
    text-box: trim-both cap alphabetic;
  }
  .policy-action {
    align-items: center;
    display: flex;
    justify-content: flex-start;
  }
  .override-stack {
    gap: var(--space-1);
  }
  .profile-grid {
    display: grid;
    gap: var(--space-2);
    grid-template-columns: repeat(auto-fit, minmax(18rem, 1fr));
  }
  .profile-card,
  .request-row {
    background: var(--surface-base);
    border: 1px solid color-mix(in srgb, var(--brand-action) 10%, var(--border-subtle));
    border-radius: var(--radius-surface);
    box-shadow: var(--shadow-plate);
    padding: var(--space-4);
  }
  .profile-card {
    display: grid;
    gap: var(--space-3);
  }
  .profile-heading {
    align-items: center;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: auto minmax(0, 1fr) auto;
  }
  .profile-mark {
    height: 1.75rem;
    width: 1.75rem;
  }
  .profile-heading > div {
    display: block;
    min-width: 0;
  }
  .profile-heading > div > :first-child {
    text-box: trim-start cap alphabetic;
  }
  .profile-heading > div > :last-child {
    text-box: trim-end cap alphabetic;
  }
  .profile-heading strong,
  .profile-heading div > span {
    display: block;
  }
  .profile-heading strong {
    color: var(--text-primary);
    font-size: var(--font-size-meta);
  }
  .profile-heading div > span,
  .profile-impact {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: var(--space-1) 0 0;
  }
  .profile-facts {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  .profile-facts > div {
    display: grid;
    gap: var(--space-1);
  }
  .profile-actions,
  .request-buttons {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .profile-actions {
    border-top: 1px solid var(--rule);
    padding-top: var(--space-3);
  }
  .request-row {
    align-items: center;
    display: flex;
    gap: var(--space-4);
    justify-content: space-between;
  }
  .request-copy {
    min-width: 0;
  }
  .request-title {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .request-title strong {
    color: var(--text-primary);
    font-size: var(--font-size-meta);
  }
  .request-form {
    background: var(--surface-base);
    border: 1px solid color-mix(in srgb, var(--brand-action) 13%, var(--border-subtle));
    border-radius: var(--radius-surface);
    box-shadow: var(--shadow-plate);
    grid-template-columns: repeat(3, minmax(0, 1fr));
    padding: var(--space-5);
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
    gap: var(--space-1);
  }
  .request-form label > span,
  .decision-reason {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    font-weight: 700;
    letter-spacing: 0.05em;
    text-transform: uppercase;
  }
  .custom-window {
    border: 1px solid var(--rule);
    border-radius: var(--radius-control);
    overflow: hidden;
  }
  .request-helper {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    margin: calc(var(--space-2) * -1) 0 0;
  }
  .promote-profile {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }
  :is(input, textarea) {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--radius-control);
    color: var(--text);
    font: inherit;
    min-height: var(--control-height);
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
  @media (max-width: 64rem) {
    .schedule-summary {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .request-form {
      grid-template-columns: 1fr 1fr;
    }
    .request-row {
      align-items: flex-start;
      flex-direction: column;
    }
  }
  @media (max-width: 34rem) {
    .schedule-summary,
    .request-form {
      grid-template-columns: 1fr;
    }
    .schedule-summary article {
      padding: var(--space-3);
    }
    .profile-facts {
      grid-template-columns: 1fr;
    }
    .section-heading {
      align-items: flex-start;
      flex-direction: column;
      gap: var(--space-1);
    }
  }
</style>
