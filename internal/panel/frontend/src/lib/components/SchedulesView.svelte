<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import type { PanelApi } from '#lib/api.js';
  import { receipts } from '#lib/receipts.svelte.js';
  import { hoursPhrase, windowsSentence } from '#lib/schedule-words.js';
  import type {
    QueuePolicy,
    QueuePolicyInput,
    QueueWorkload,
    QueuePolicyStatus,
    SchedulePolicySet,
    ScheduleProfile,
    ScheduleProfileInput,
    ScheduleRequest,
  } from '#lib/types.js';
  import { cadenceWords, workloadDescription, workloadTitle } from '#lib/workloads.js';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import Icon from './Icon.svelte';
  import Pill from './Pill.svelte';
  import Plate from './Plate.svelte';
  import PolicyEditorDialog from './PolicyEditorDialog.svelte';
  import ProfileEditorDialog from './ProfileEditorDialog.svelte';
  import RelativeTime from './RelativeTime.svelte';
  import RootPageHeader from './RootPageHeader.svelte';

  const {
    api,
  }: {
    api: PanelApi;
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
    queryKey: ['schedules', 'root'],
    queryFn: fetchSchedules,
  }));

  /* Requests and overrides are asked for by a workspace, and the wire says which by
     id. The catalog is what turns one into a name, and the console has it cached. */
  const catalogQuery = createQuery(() => ({
    queryKey: ['root-workspaces'],
    queryFn: () => api.fetchRootWorkspaces(),
  }));
  const catalog = $derived(
    new Map((catalogQuery.data ?? []).map((row) => [row.id, row.account.display_name])),
  );

  const scheduleData = $derived<ScheduleViewData | null>(schedulesQuery.data ?? null);
  const policies = $derived<QueuePolicy[]>(scheduleData?.policies ?? []);
  const policySet = $derived<SchedulePolicySet | null>(scheduleData?.policySet ?? null);
  const statuses = $derived<QueuePolicyStatus[]>(scheduleData?.statuses ?? []);
  const profiles = $derived<ScheduleProfile[]>(scheduleData?.profiles ?? []);
  const requests = $derived<ScheduleRequest[]>(scheduleData?.requests ?? []);
  const loading = $derived(schedulesQuery.isFetching);
  const queryError = $derived(errorMessage(schedulesQuery.error));
  const error = $derived(operationError || queryError);
  let editingPolicy = $state<QueuePolicy | null>(null);
  let revertingPolicy = $state<QueuePolicy | null>(null);
  let editingProfile = $state<ScheduleProfile | null>(null);
  let profileOpen = $state(false);
  let dialogBusy = $state(false);
  let dialogError = $state('');
  let deciding = $state<ScheduleRequest | null>(null);
  let archivingProfile = $state<ScheduleProfile | null>(null);
  let decision = $state<'approve' | 'reject'>('approve');
  let decisionReason = $state('');
  let promoteProfile = $state(false);

  /* One clock for the page, floored to the minute: every row reads a time against it. */
  const nowMs = Math.floor(Date.now() / 60_000) * 60_000;

  /* Four, because a console opens on what is happening rather than on the whole
     deployment: the jobs that ran most recently are the ones somebody came to look
     at, and the rest are one press away. */
  const JOBS_SHOWN = 4;
  let allJobs = $state(false);

  const policyOverrides = $derived(policySet?.overrides ?? []);
  const waiting = $derived(requests.filter((request) => request.state === 'pending'));
  const decided = $derived(requests.filter((request) => request.state !== 'pending').slice(0, 5));

  /* Most recently run first, and a job that has never run sorts last: it has nothing
     to be recent about. */
  const jobs = $derived([...policies].sort((left, right) => ran(right.kind) - ran(left.kind)));
  const shownJobs = $derived(allJobs ? jobs : jobs.slice(0, JOBS_SHOWN));

  function ran(kind: QueueWorkload): number {
    const at = policyStatus(kind)?.last_run_at;

    return at === undefined ? 0 : Date.parse(at);
  }

  function errorMessage(cause: unknown): string {
    if (cause === null || cause === undefined) return '';
    return cause instanceof Error ? cause.message : String(cause);
  }

  async function fetchSchedules(): Promise<ScheduleViewData> {
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

  async function load(): Promise<void> {
    operationError = '';
    await schedulesQuery.refetch();
  }

  function profileById(id: string): ScheduleProfile | undefined {
    return profiles.find((profile) => profile.id === id);
  }

  function workspaceName(id: string | undefined): string {
    if (id === undefined) return 'this deployment';

    return catalog.get(id) ?? id;
  }

  function policyStatus(kind: QueueWorkload): QueuePolicyStatus | undefined {
    return statuses.find((status) => status.kind === kind);
  }

  function overridesFor(kind: QueueWorkload): QueuePolicy[] {
    return policyOverrides.filter((policy) => policy.kind === kind);
  }

  /**
   * A job as one sentence: what it does, how often, in whose hours.
   *
   * The four columns this replaced said the same thing in fragments - `6h`,
   * `Always Open`, `Est. 31 Aug, 20:36 · next` - and left a reader to assemble it.
   */
  function jobSentence(policy: QueuePolicy): string {
    const said = [workloadDescription(policy.kind)];
    said.push(
      policy.enabled
        ? `${cadenceWords(policy.cadence)} ${hoursPhrase(profileById(policy.profile_id), policy.profile_id)}`
        : 'paused - it does not run at all',
    );

    const overrides = overridesFor(policy.kind);
    if (overrides.length > 0) {
      said.push(
        `overridden for ${overrides.map((override) => workspaceName(override.target_id)).join(', ')}`,
      );
    }

    return said.join(' · ');
  }

  /** An hours profile as one sentence: where it is, when it is open, who runs on it. */
  function hoursSentence(profile: ScheduleProfile): string {
    const said = [`${profile.timezone} · ${windowsSentence(profile)}`];
    const on = profile.affected_workspaces;
    if (on !== undefined) {
      said.push(`${on} ${on === 1 ? 'workspace runs' : 'workspaces run'} on it`);
    }

    return said.join(' · ');
  }

  /** Who asked, by name where the account is still readable. */
  function asker(request: ScheduleRequest): string {
    return request.requester?.display_name ?? request.requester?.login ?? request.requested_by;
  }

  /** What a workspace is asking for, in the words it asked in. */
  function requestSentence(request: ScheduleRequest): string {
    return `${workspaceName(request.target_id)} asks: ${workloadTitle(request.kind)} ${cadenceWords(request.cadence)}`;
  }

  async function savePolicy(input: QueuePolicyInput): Promise<void> {
    if (editingPolicy === null) return;
    dialogBusy = true;
    try {
      const saved =
        editingPolicy.target_id === undefined
          ? await api.updateRootJobPolicy(editingPolicy.kind, input)
          : await api.updateRootWorkspaceJobPolicy(
              editingPolicy.target_id,
              editingPolicy.kind,
              input,
            );
      receipts.say(`Saved - ${workloadTitle(saved.kind)} runs on its new schedule`);
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
      await api.deleteRootWorkspaceJobPolicy(
        revertingPolicy.target_id,
        revertingPolicy.kind,
        revertingPolicy.revision,
      );
      receipts.say(`${workloadTitle(revertingPolicy.kind)} now runs on the deployment schedule`);
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
      receipts.say(`Saved - ${saved.name}`);
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
      receipts.say(`${archivingProfile.name} archived - nothing runs on it any more`);
      archivingProfile = null;
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
      receipts.say(
        saved.state === 'approved'
          ? `Approved - ${workspaceName(saved.target_id)} runs ${workloadTitle(saved.kind).toLowerCase()} on its own schedule`
          : `Declined - ${workspaceName(saved.target_id)} keeps the deployment schedule`,
      );
      deciding = null;
      await load();
    } catch (cause) {
      dialogError = cause instanceof Error ? cause.message : String(cause);
    } finally {
      dialogBusy = false;
    }
  }
</script>

<!--
@component
How often background jobs run, and when.

A job is one sentence - what it does, how often, in whose hours - rather than six
duration fragments across four columns, and cadence is said the way a person says it:
"every 30 minutes", never 1800000000000. What a workspace has asked for leads the page,
because it is the only thing here waiting on somebody.

This page is the operators'. A workspace's members see the one row of it that is theirs,
in their own settings.
-->

<div class="view-frame" aria-busy={loading}>
  <RootPageHeader title="Schedules" subtitle="How often background jobs run and when">
    <Button
      onclick={() => {
        editingProfile = null;
        profileOpen = true;
      }}
    >
      {#snippet icon()}<Icon name="plus" size="sm" strokeWidth={2} />{/snippet}
      New hours profile
    </Button>
  </RootPageHeader>

  {#if loading && policies.length === 0 && profiles.length === 0}
    <Plate label="Loading…"><p class="dim" role="status">Reading the schedules…</p></Plate>
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

    {#if waiting.length > 0}
      <Card>
        <div class="card-head"><h2 class="card-title">Needs a decision</h2></div>
        <div class="object-list">
          {#each waiting as request (request.id)}
            <div class="object-row">
              <span class="object-main">
                <span class="object-name-row">
                  <span class="object-name">{requestSentence(request)}</span>
                </span>
                <!-- The reason as it was written, in quotation marks: it is somebody's
                     sentence and the page is asking an operator to weigh it. -->
                <!-- The space stands outside the block, never at the end of one:
                     Svelte trims a block's trailing whitespace, and the reason arrived
                     welded to the word after it. -->
                <span class="object-sum"
                  >{#if request.reason.trim() !== ''}“{request.reason}” ·{/if} asked by
                  {asker(request)},
                  <RelativeTime value={request.created_at} {nowMs} /></span
                >
              </span>
              <span class="object-side">
                <Button tone="signal" onclick={() => openDecision(request, 'approve')}>
                  Approve
                </Button>
                <Button tone="quiet" onclick={() => openDecision(request, 'reject')}>
                  Decline
                </Button>
              </span>
            </div>
          {/each}
        </div>
      </Card>
    {/if}

    <Card>
      <div class="card-head">
        <h2 class="card-title">Jobs</h2>
        <span class="card-meta"
          >Showing {shownJobs.length} of {jobs.length}
          {jobs.length === 1 ? 'job' : 'jobs'}{#if policyOverrides.length > 0}&nbsp;· {policyOverrides.length}
            overridden{/if}</span
        >
      </div>
      <div class="object-list">
        {#each shownJobs as policy (policy.kind)}
          {@const status = policyStatus(policy.kind)}
          {@const nextAt = policy.enabled ? status?.next_eligibility_at : undefined}
          <div class="object-row">
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name">{workloadTitle(policy.kind)}</span>
                {#if !policy.enabled}
                  <Pill tone="neutral">Paused</Pill>
                {/if}
                {#if policy.default_priority !== 'normal'}
                  <Pill tone={policy.default_priority === 'urgent' ? 'danger' : 'warning'}>
                    {policy.default_priority === 'urgent' ? 'Urgent' : 'High'} priority
                  </Pill>
                {/if}
              </span>
              <!-- The separator rides the words rather than standing in the block:
                   Svelte trims a block's leading whitespace, so "· next" arrived stuck
                   to the last letter of the sentence before it. -->
              <span class="object-sum"
                >{nextAt === undefined
                  ? jobSentence(policy)
                  : `${jobSentence(policy)} · next `}{#if nextAt !== undefined}<RelativeTime
                    value={nextAt}
                    {nowMs}
                    future
                  />{/if}</span
              >
            </span>
            <span class="object-side">
              <Button
                tone="quiet"
                aria-label="Edit schedule - {workloadTitle(policy.kind)}"
                onclick={() => {
                  editingPolicy = policy;
                  dialogError = '';
                }}
              >
                Edit schedule
              </Button>
            </span>
          </div>
        {/each}
      </div>
      {#if jobs.length > JOBS_SHOWN}
        <div class="list-foot">
          <span
            >{allJobs
              ? `All ${jobs.length} jobs`
              : `Showing the ${JOBS_SHOWN} most recently run`}</span
          >
          <Button tone="quiet" onclick={() => (allJobs = !allJobs)}>
            {allJobs ? 'Show the recent ones' : `Show all ${jobs.length} jobs`}
          </Button>
        </div>
      {/if}
    </Card>

    {#if policyOverrides.length > 0}
      <!-- What a workspace was granted, and the way back off it. The job rows above
           say a workspace has its own hours; this is where that is read and undone. -->
      <Card>
        <div class="card-head"><h2 class="card-title">Workspace overrides</h2></div>
        <div class="object-list">
          {#each policyOverrides as policy (`${policy.target_id}:${policy.kind}`)}
            <div class="object-row">
              <span class="object-main">
                <span class="object-name-row">
                  <span class="object-name"
                    >{workspaceName(policy.target_id)} · {workloadTitle(policy.kind)}</span
                  >
                </span>
                <span class="object-sum"
                  >{policy.enabled
                    ? `${cadenceWords(policy.cadence)} ${hoursPhrase(profileById(policy.profile_id), policy.profile_id)}`
                    : 'paused - it does not run at all'} · {policy.default_priority} priority</span
                >
              </span>
              <span class="object-side">
                <Button tone="quiet" onclick={() => (editingPolicy = policy)}>Edit schedule</Button>
                <Button tone="quiet" onclick={() => (revertingPolicy = policy)}>
                  Use the deployment schedule
                </Button>
              </span>
            </div>
          {/each}
        </div>
      </Card>
    {/if}

    <Card>
      <div class="card-head"><h2 class="card-title">Hours</h2></div>
      <div class="object-list">
        {#each profiles as profile (profile.id)}
          <div class="object-row">
            <span class="object-main">
              <span class="object-name-row">
                <span class="object-name">{profile.name}</span>
                {#if profile.system}
                  <Pill>built in</Pill>
                {/if}
              </span>
              <span class="object-sum">{hoursSentence(profile)}</span>
            </span>
            <span class="object-side">
              {#if !profile.system}
                <Button
                  tone="quiet"
                  aria-label="Edit - the {profile.name} profile"
                  onclick={() => {
                    editingProfile = profile;
                    profileOpen = true;
                    dialogError = '';
                  }}
                >
                  Edit
                </Button>
                <Button tone="quiet" onclick={() => (archivingProfile = profile)}>Archive</Button>
              {/if}
            </span>
          </div>
        {/each}
      </div>
    </Card>

    {#if decided.length > 0}
      <Card>
        <div class="card-head"><h2 class="card-title">Recently decided</h2></div>
        <div class="object-list">
          {#each decided as request (request.id)}
            <div class="object-row">
              <span class="object-main">
                <span class="object-name-row">
                  <span class="object-name">{requestSentence(request)}</span>
                  <Pill tone={request.state === 'approved' ? 'success' : 'neutral'}>
                    {request.state}
                  </Pill>
                </span>
                <span class="object-sum"
                  >Asked by {asker(request)},
                  <RelativeTime value={request.created_at} {nowMs} /> · decided
                  <RelativeTime value={request.updated_at} {nowMs} /></span
                >
              </span>
            </div>
          {/each}
        </div>
      </Card>
    {/if}
  {/if}
</div>

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
  id="revert-workspace-policy"
  open={revertingPolicy !== null}
  title="Use the deployment schedule?"
  description={revertingPolicy === null
    ? undefined
    : `${workspaceName(revertingPolicy.target_id)} will stop overriding ${workloadTitle(revertingPolicy.kind)}.`}
  busy={dialogBusy}
  confirmLabel="Use the deployment schedule"
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
  title={decision === 'approve' ? 'Approve schedule request' : 'Decline schedule request'}
  description={deciding?.reason}
  busy={dialogBusy}
  confirmLabel={decision === 'approve' ? 'Approve' : 'Decline'}
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
  title="Archive hours profile"
  description={archivingProfile === null
    ? undefined
    : `${archivingProfile.name} can only be archived after every job is reassigned.`}
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

<style>
  .decision-reason,
  .promote-profile {
    color: var(--text-secondary);
    display: grid;
    font-size: var(--font-size-compact);
    gap: var(--space-2);
  }

  .promote-profile {
    align-items: center;
    grid-auto-flow: column;
    justify-content: start;
  }

  .decision-reason textarea {
    background: var(--surface-base);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font: inherit;
    padding: var(--space-2);
    resize: vertical;
    width: 100%;
  }

  .form-error {
    color: var(--danger);
    font-size: var(--font-size-compact);
    margin: 0;
  }

  .dim {
    color: var(--text-secondary);
    margin: 0;
  }
</style>
