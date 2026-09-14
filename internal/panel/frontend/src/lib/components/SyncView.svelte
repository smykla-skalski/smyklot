<script lang="ts">
  import { untrack, onMount, tick } from 'svelte';
  import { PanelApiError } from '#lib/api.js';
  import { SyncRequestIntentStore } from '#lib/sync-request-intent.js';
  import Callout from './Callout.svelte';
  import { receipts } from '#lib/receipts.svelte.js';
  import { useInterval } from 'runed';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';

  import { formatJson, parseJson, type JsonValue } from '#lib/merge.js';
  import { sameSettingsJson } from '#lib/settings-draft-storage.js';
  import {
    adoptSyncOverrideSettings,
    stageSyncOverrideControl,
    syncOverrideDraftEnvelope,
    type SyncOverrideControlId,
    type SyncOverrideEditorEnvelope,
  } from '#lib/repository-sync-override-settings.js';
  import {
    getSettingsDraftRegistry,
    type SettingsScope,
    type SettingsJson,
  } from '#lib/settings-drafts.svelte.js';
  import { getFileDraftValidation } from '#lib/file-draft-validation.js';
  import {
    adoptSyncConfigSettings,
    stageSyncConfigControl,
    syncConfigDraftEnvelope,
    syncConfigForEditor,
    type SyncConfigControlId,
    type SyncConfigEditorEnvelope,
    type SyncLabelsEditorEnvelope,
  } from '#lib/sync-config-settings.js';
  import type {
    BypassActorLookup,
    SyncConfig,
    SyncFilesContext,
    SyncKind,
    SyncOverride,
    SyncPlan,
    SyncRunNowResponse,
    SyncRunNowInput,
    SyncRunNowIntent,
    SyncStatus,
  } from '#lib/types.js';
  import type {
    SyncFileRenderInput,
    SyncFileRenderResponse,
  } from '#lib/sync-file-render.generated.js';
  import type { SyncSection } from '#lib/routes.js';

  import SyncHistory from './SyncHistory.svelte';
  import SyncRequests from './SyncRequests.svelte';
  import SyncCheckInspector from './SyncCheckInspector.svelte';
  import Link from './Link.svelte';
  import type { SyncCheckResponse } from '../types';
  import type { Page, SyncPlanSummary } from '../types';
  import FormError from './FormError.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import { syncResultProblem } from '../sync-result-error';
  import Modal from './Modal.svelte';
  import Button from './Button.svelte';
  import PageHeader from './PageHeader.svelte';
  import SyncFilePage from './SyncFilePage.svelte';
  import SyncFilesPage from './SyncFilesPage.svelte';
  import SyncLabelsPage from './SyncLabelsPage.svelte';
  import SyncOverview from './SyncOverview.svelte';
  import SyncPlanPage from './SyncPlanPage.svelte';
  import SyncRulesetPage from './SyncRulesetPage.svelte';
  import SyncRulesetsPage from './SyncRulesetsPage.svelte';
  import SyncSettingsPage from './SyncSettingsPage.svelte';

  const {
    targetId,
    actorId = '',
    section,
    rulesetName = null,
    fileName = null,
    readOnly,
    organizationActors = true,
    canControl = false,
    fetchConfig,
    fetchPlan,
    fetchHistory,
    fetchRequests,
    historyResultHref,
    checkResultHref,
    onOpenCheck,
    onOpenHistoryResult,
    selectedPlanId = null,
    selectedCheckId = null,
    checkHref,
    fetchCheck,
    checkEvidenceApi,
    onOpenPlan,
    approvePlan,
    discardPlan,
    runSyncNow = async () => {
      throw new Error('Sync queue control is unavailable');
    },
    fetchStatus,
    sectionHref,
    onOpenSection,
    rulesetHref,
    onOpenRuleset,
    fileHref,
    onOpenFile,
    fetchFilesContext,
    renderFile,
    fetchOverride,
    repositoryHref = null,
    permissionsHref = null,
    queueHref = null,
    clock = Date.now,
    lookupBypassActors,
  }: {
    checkEvidenceApi?: Pick<import('../api').PanelApi, 'fetchSyncCheckObservations'>;
    selectedCheckId?: string | null;
    checkHref: (id: string) => string;
    fetchCheck: (id: string) => Promise<SyncCheckResponse>;
    permissionsHref?: string | null;
    queueHref?: string | null;
    repositoryHref?: ((repository: string) => string) | null;
    targetId: string;
    actorId?: string;
    lookupBypassActors?: BypassActorLookup;
    /** Which of the view's sections the address names; see `routes.ts`. */
    section: SyncSection;
    /** One ruleset's own page, when the address names one. */
    rulesetName?: string | null;
    /** One template's own page, when the address names one. */
    fileName?: string | null;
    readOnly: boolean;
    organizationActors?: boolean;
    canControl?: boolean;
    rulesetHref: (name: string) => string;
    onOpenRuleset: (name: string) => void;
    fileHref: (path: string) => string;
    onOpenFile: (path: string) => void;
    fetchFilesContext: (targetId: string) => Promise<SyncFilesContext>;
    renderFile: (targetId: string, input: SyncFileRenderInput) => Promise<SyncFileRenderResponse>;
    fetchOverride: (targetId: string, repositoryId: string, kind: string) => Promise<SyncOverride>;
    fetchConfig: (targetId: string, kind: string) => Promise<SyncConfig>;
    fetchRequests?: import('../api').PanelApi['fetchSyncRequests'];
    fetchHistory: (
      targetId: string,
      request: { limit: number; cursor?: string },
    ) => Promise<Page<SyncPlanSummary>>;
    historyResultHref: (id: string) => string;
    checkResultHref: (checkId: string, planId: string) => string;
    onOpenCheck: (id: string) => void;
    onOpenHistoryResult: (id: string) => void;
    selectedPlanId?: string | null;
    onOpenPlan?: (planId: string) => void;
    fetchPlan: (
      targetId: string,
      planId?: string,
    ) => Promise<{ plan: SyncPlan | null; check?: import('../types').SyncCheckCapability }>;
    approvePlan: (targetId: string, planId: string, digest: string) => Promise<{ plan: SyncPlan }>;
    discardPlan: (targetId: string, planId: string) => Promise<void>;
    runSyncNow?: (targetId: string, input: SyncRunNowInput) => Promise<SyncRunNowResponse>;
    fetchStatus: (targetId: string) => Promise<SyncStatus>;
    sectionHref: (section: SyncSection) => string;
    onOpenSection: (section: SyncSection) => void;
    /** Injectable only so deterministic catalogue states do not age against the wall clock. */
    clock?: () => number;
  } = $props();

  // The kinds this view has a form for, named rather than taken as a parameter
  // nothing varies.
  const LABELS = 'labels';
  const SETTINGS = 'settings';
  const RULESETS = 'rulesets';
  const FILES = 'files';

  /**
   * The kinds whose whole configuration is one document. Labels are not one of
   * them: their form was built out of typed fields before there was a second
   * kind, and everything since travels as a document.
   */
  type DocumentKind = typeof SETTINGS | typeof RULESETS | typeof FILES;

  type EditorState = { config: SyncConfig | null; problem: string | null };

  const drafts = getSettingsDraftRegistry();
  const fileValidation = getFileDraftValidation();
  const settingsScope = $derived({
    type: 'workspace',
    targetId,
  } as const satisfies SettingsScope);
  let canonicalConfigs = $state.raw<Partial<Record<SyncKind, SyncConfig>>>({});
  let stageProblems = $state.raw<Partial<Record<SyncKind, string>>>({});

  const editorStates = $derived.by(() => {
    const states: Partial<Record<SyncKind, EditorState>> = {};
    for (const kind of [LABELS, SETTINGS, RULESETS, FILES] as const) {
      states[kind] = editorState(kind);
    }
    return states;
  });
  const config = $derived(editorStates.labels?.config ?? null);
  const documents = $derived<Record<DocumentKind, SyncConfig | null>>({
    settings: editorStates.settings?.config ?? null,
    rulesets: editorStates.rulesets?.config ?? null,
    files: editorStates.files?.config ?? null,
  });
  const dirtyControls = $derived(drafts.dirtyControls(settingsScope).map(({ id }) => id));
  const queryClient = useQueryClient();
  const planQuery = createQuery(() => ({
    queryKey: ['sync-plan', targetId],
    // Details can finish before the status request mounts their first consumer.
    notifyOnChangeProps: ['data', 'error', 'isPending', 'isFetching'],
    retry: false,
    queryFn: () => fetchPlan(targetId),
  }));
  const selectedPlanQuery = createQuery(() => ({
    queryKey: ['sync-plan', targetId, selectedPlanId],
    enabled: selectedPlanId !== null,
    retry: false,
    queryFn: () => fetchPlan(targetId, selectedPlanId ?? undefined),
  }));
  const inspectedPlanQuery = $derived(selectedPlanId === null ? planQuery : selectedPlanQuery);
  let refreshingPlan = $state(false);
  async function refreshInspectedPlan(): Promise<void> {
    if (refreshingPlan) return;
    refreshingPlan = true;
    try {
      await inspectedPlanQuery.refetch();
    } finally {
      refreshingPlan = false;
    }
  }
  let inspectedPage: { focusStatus: () => void } | undefined = $state();
  async function retryInspectedPlan(): Promise<void> {
    const trigger = document.activeElement;
    const requestedTarget = targetId;
    const requestedPlan = selectedPlanId;
    let movedFocus = false;
    const trackFocus = (event: FocusEvent) => {
      if (trigger?.isConnected && event.target !== trigger && event.target !== document.body)
        movedFocus = true;
    };
    document.addEventListener('focusin', trackFocus);
    try {
      await refreshInspectedPlan();
      await tick();
    } finally {
      document.removeEventListener('focusin', trackFocus);
    }
    if (
      detailsVisible &&
      targetId === requestedTarget &&
      selectedPlanId === requestedPlan &&
      trigger instanceof HTMLElement &&
      !trigger.isConnected &&
      !movedFocus
    )
      inspectedPage?.focusStatus();
  }
  const inspectedPlanProblem = $derived(syncResultProblem(inspectedPlanQuery.error));
  const inspectedPlan = $derived(
    selectedPlanId === null
      ? (planQuery.data?.plan ?? null)
      : (selectedPlanQuery.data?.plan ?? null),
  );
  const statusQuery = createQuery(() => ({
    queryKey: ['sync-status', targetId],
    queryFn: () => fetchStatus(targetId),
  }));
  const plan = $derived(planQuery.data?.plan ?? null);
  const syncStatus = $derived(statusQuery.data ?? null);
  let detailsOpen = $state(false);
  const detailsVisible = $derived(
    detailsOpen || section === 'plan' || (section === 'history' && selectedPlanId !== null),
  );
  const feedbackReadError = $derived(
    (!detailsVisible ? planQuery.error : null) ?? statusQuery.error,
  );
  let detailsTrigger = $state<HTMLElement | null>(null);
  function closeDetails(): void {
    detailsOpen = false;
    if (section === 'plan') {
      if (selectedCheckId !== null) onOpenCheck(selectedCheckId);
      else onOpenSection('overview');
    }
    if (section === 'history') onOpenSection('history');
  }
  function openDetails(trigger: HTMLElement): void {
    detailsTrigger = trigger;
    if (plan !== null && onOpenPlan !== undefined) onOpenPlan(plan.id);
    else detailsOpen = true;
  }
  let filesContext = $state<SyncFilesContext | null>(null);
  /* The injected clock keeps catalogue examples deterministic while live views age. */
  let nowMs = $state(untrack(() => clock()));
  useInterval(30_000, { callback: () => (nowMs = clock()) });
  let approving = $state(false);
  let discarding = $state(false);
  let runningNow = $state(false);
  let runNotice = $state('');
  let requestedCheckId = $state<string | null>(null);
  let requestedDispatchId = $state<string | null>(null);
  let pendingRequest = $state.raw<SyncRunNowInput | null>(null);
  let requestStorageProblem = $state<string | null>(null);
  let requestUncertain = $state(false);
  let requestCleanupPending = $state(false);
  let requestStore: SyncRequestIntentStore | null = null;
  function readRequestStorage(): void {
    if (runningNow || pendingRequest) return;
    if (!actorId) {
      requestStorageProblem = 'Your account must be loaded before requesting sync.';
      return;
    }
    try {
      requestStore = new SyncRequestIntentStore(window.sessionStorage, actorId, targetId);
      pendingRequest = requestStore.read();
      requestUncertain = pendingRequest !== null;
      requestStorageProblem = null;
    } catch {
      requestStorageProblem =
        'Browser storage could not be read. Retry access to recover any saved request. New requests remain paused.';
    }
  }
  onMount(readRequestStorage);

  let error = $state<string | null>(null);
  const labelsError = $derived(stageProblems.labels ?? editorStates.labels?.problem ?? error);
  const documentError = $derived<Record<DocumentKind, string | null>>({
    settings: stageProblems.settings ?? editorStates.settings?.problem ?? null,
    rulesets: stageProblems.rulesets ?? editorStates.rulesets?.problem ?? null,
    files: stageProblems.files ?? editorStates.files?.problem ?? null,
  });

  /* Every document, because each is only meaningful beside the plan: a plan
     says what would change, and what it would change to is what the
     configurations list. untrack keeps the writes below from feeding back into
     the read that caused them. */
  $effect(() => {
    const id = targetId;
    untrack(() => void load(id));
  });

  /* A successful application-wide save leaves its notice in the registry. The
     committed configuration is already available there; the read refreshes the
     plan, status and files context that are deliberately not part of a draft. */
  let handledSaveNotice = untrack(() => drafts.operation(settingsScope).notice);
  $effect(() => {
    const notice = drafts.operation(settingsScope).notice;
    if (notice === null) {
      handledSaveNotice = null;
      return;
    }
    if (notice === handledSaveNotice) return;
    handledSaveNotice = notice;
    const id = targetId;
    untrack(() => void load(id));
  });

  async function load(id: string): Promise<void> {
    error = null;
    try {
      const [loadedConfig, loadedSettings, loadedRulesets, loadedFiles, loadedContext] =
        await Promise.all([
          fetchConfig(id, LABELS),
          fetchConfig(id, SETTINGS),
          fetchConfig(id, RULESETS),
          fetchConfig(id, FILES),
          fetchFilesContext(id),
        ]);
      const loaded = [loadedConfig, loadedSettings, loadedRulesets, loadedFiles];
      canonicalConfigs = Object.fromEntries(loaded.map((config) => [config.kind, config]));
      for (const config of loaded) {
        if (!config.unreadable) adoptSyncConfigSettings(drafts, id, config);
      }
      filesContext = loadedContext;
      nowMs = clock();
    } catch (cause) {
      error = messageOf(cause);
    }
  }

  function editorState(kind: SyncKind): EditorState {
    const canonical = canonicalConfigs[kind];
    if (canonical === undefined) return { config: null, problem: null };
    if (canonical.unreadable) return { config: canonical, problem: null };
    try {
      return {
        config: syncConfigForEditor(
          canonical,
          syncConfigDraftEnvelope(drafts, targetId, canonical),
        ),
        problem: null,
      };
    } catch (cause) {
      return { config: null, problem: messageOf(cause) };
    }
  }

  function currentEnvelope(kind: SyncKind): SyncConfigEditorEnvelope | null {
    const canonical = canonicalConfigs[kind];
    if (canonical === undefined || canonical.unreadable) return null;
    try {
      return syncConfigDraftEnvelope(drafts, targetId, canonical);
    } catch (cause) {
      setStageProblem(kind, messageOf(cause));
      return null;
    }
  }

  function stageEnvelope(
    kind: SyncKind,
    envelope: SyncConfigEditorEnvelope,
    controlId: SyncConfigControlId,
    expectedValue?: SyncConfigEditorEnvelope,
  ): boolean {
    const canonical = canonicalConfigs[kind];
    if (
      canonical === undefined ||
      canonical.unreadable ||
      !stageSyncConfigControl(drafts, targetId, canonical, envelope, controlId, expectedValue)
    ) {
      setStageProblem(
        kind,
        expectedValue === undefined
          ? 'This Sync configuration change is not valid'
          : 'Settings changed before this edit could be applied · review the current values',
      );
      return false;
    }
    setStageProblem(kind, null);
    return true;
  }

  function stageLabels(
    next: {
      enabled: boolean;
      labels: SyncConfig['labels'];
      allow_removal: boolean;
      excludes: string[];
    },
    controlId: Extract<SyncConfigControlId, `sync.labels.${string}`>,
  ): boolean {
    const labels: SyncLabelsEditorEnvelope['labels'] = next.labels.map((label) => ({
      name: label.name,
      color: label.color,
      ...(label.description === undefined ? {} : { description: label.description }),
    }));
    return stageEnvelope(LABELS, { kind: LABELS, ...next, labels }, controlId);
  }

  function stageDocument(
    kind: DocumentKind,
    document: Record<string, unknown>,
    expectedDocument?: Record<string, unknown>,
  ): boolean {
    const current = currentEnvelope(kind);
    if (current === null || current.kind === LABELS) return false;
    try {
      if (
        expectedDocument !== undefined &&
        !sameSettingsJson(
          parseJson(current.document_text) as SettingsJson,
          expectedDocument as SettingsJson,
        )
      ) {
        setStageProblem(
          kind,
          'Settings changed before this edit could be applied · review the current values',
        );
        return false;
      }
      return stageEnvelope(
        kind,
        { ...current, document_text: formatJson(document as JsonValue).trimEnd() },
        `sync.${kind}.document`,
        current,
      );
    } catch (cause) {
      setStageProblem(kind, messageOf(cause));
      return false;
    }
  }

  function setStageProblem(kind: SyncKind, problem: string | null): void {
    const next = { ...stageProblems };
    if (problem === null) delete next[kind];
    else next[kind] = problem;
    stageProblems = next;
  }

  async function loadFilesOverride(repositoryId: string): Promise<{
    stored: SyncOverride;
    envelope: SyncOverrideEditorEnvelope | null;
  }> {
    const id = targetId;
    const stored = await fetchOverride(id, repositoryId, FILES);
    if (stored.unreadable) return { stored, envelope: null };
    adoptSyncOverrideSettings(drafts, id, repositoryId, stored);
    return {
      stored,
      envelope: syncOverrideDraftEnvelope(drafts, id, repositoryId, stored),
    };
  }

  function stageFilesOverride(
    repositoryId: string,
    stored: SyncOverride,
    next: SyncOverrideEditorEnvelope,
    controlId: SyncOverrideControlId,
  ): boolean {
    if (stored.unreadable) return false;
    try {
      return stageSyncOverrideControl(drafts, targetId, repositoryId, stored, next, controlId);
    } catch {
      return false;
    }
  }

  async function refreshSyncQueries(requestTargetId: string): Promise<void> {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['sync-plan', requestTargetId] }),
      queryClient.invalidateQueries({ queryKey: ['sync-status', requestTargetId] }),
    ]);
  }

  async function onApprove(planId: string, digest: string): Promise<void> {
    const requestTargetId = targetId;
    approving = true;
    error = null;
    try {
      await approvePlan(requestTargetId, planId, digest);
      await refreshSyncQueries(requestTargetId);
    } catch (cause) {
      error = messageOf(cause);
    } finally {
      approving = false;
    }
  }

  /** Throwing a plan away asks nothing on GitHub - the next sweep recomputes. */
  async function onDiscard(planId: string): Promise<void> {
    const requestTargetId = targetId;
    discarding = true;
    error = null;
    try {
      await discardPlan(requestTargetId, planId);
      await refreshSyncQueries(requestTargetId);
    } catch (cause) {
      error = messageOf(cause);
    } finally {
      discarding = false;
    }
  }

  async function onRunNow(input: SyncRunNowIntent): Promise<void> {
    if (runningNow) return;
    if (pendingRequest && input.request_key !== pendingRequest.request_key) {
      error = 'Recover the previous sync request before starting another.';
      return;
    }
    const requestTargetId = targetId;
    const recovering = requestUncertain;
    runningNow = true;
    error = null;
    runNotice = '';
    requestedCheckId = null;
    requestedDispatchId = null;
    try {
      if (!pendingRequest) {
        try {
          if (!requestStore || requestStorageProblem) throw new Error('Storage unavailable');
          pendingRequest = requestStore.begin(input);
        } catch {
          requestStorageProblem =
            'The request could not be saved in browser storage and has not been sent. Retry browser storage before starting again.';
          return;
        }
      }
      // The exact pending command is already known. Storage failure must not prevent
      // its idempotent recovery or replace its identity with a new request.
      const request = pendingRequest;
      const response = await runSyncNow(requestTargetId, request);
      const valid =
        request.action === 'check'
          ? response.status === 'changes_pending' ||
            (response.status === 'check_accepted' && !!response.check_id)
          : response.status === 'approval_required' ||
            response.status === 'already_running' ||
            (response.status === 'dispatch_accepted' &&
              response.plan_id === request.plan_id &&
              !!response.queue_id);
      if (!valid)
        throw new Error(
          'The sync response could not be confirmed. Recover the request before starting another.',
        );
      requestCleanupPending = true;
      requestUncertain = false;
      finishRequestRecovery();
      if (response.status === 'check_accepted') {
        requestedCheckId = response.check_id!;
        runNotice = 'Your check request was accepted. Open the check to see its current result.';
      }
      if (response.status === 'dispatch_accepted') {
        if (recovering) {
          requestedDispatchId = response.plan_id!;
          runNotice =
            'Your request to run these changes was accepted. Open the changes to see their current result.';
        } else receipts.say('Your request to run these changes was accepted');
      }
      if (response.status === 'changes_pending')
        runNotice =
          'Earlier changes are still pending. Review them before requesting another check.';
      if (response.status === 'approval_required')
        runNotice = 'These changes need approval before they can run.';
      if (response.status === 'already_running') runNotice = 'These changes are already running.';
      await refreshSyncQueries(requestTargetId);
    } catch (cause) {
      if (
        !recovering &&
        pendingRequest &&
        cause instanceof PanelApiError &&
        [400, 404, 409].includes(cause.status) &&
        [
          'invalid_request',
          'not_found',
          'stale_revision',
          'unsupported_plan_state',
          'conflict',
        ].includes(cause.code)
      ) {
        try {
          requestStore!.clear(pendingRequest.request_key);
          pendingRequest = null;
          void queryClient.invalidateQueries({ queryKey: ['sync-plan', requestTargetId] });
        } catch {
          /* Preserve the request if storage cannot confirm its removal. */
        }
      }
      if (pendingRequest) requestUncertain = true;
      error = messageOf(cause);
    } finally {
      runningNow = false;
    }
  }

  function finishRequestRecovery(): void {
    if (!pendingRequest || !requestCleanupPending) return;
    try {
      if (!requestStore) throw new Error('Storage unavailable');
      requestStore.clear(pendingRequest.request_key);
      pendingRequest = null;
      requestCleanupPending = false;
      requestUncertain = false;
      requestStorageProblem = null;
    } catch {
      requestStorageProblem =
        'The response was received, but its saved request could not be cleared. New requests are paused.';
    }
  }

  function messageOf(cause: unknown): string {
    return cause instanceof Error ? cause.message : String(cause);
  }

  /* The overview and each editor stage the same enablement control. Spreading
     the envelope preserves malformed document text for the generic save to
     reject without losing what somebody typed. */
  function toggleKind(kind: SyncKind, next: boolean): void {
    const current = currentEnvelope(kind);
    if (current !== null)
      stageEnvelope(kind, { ...current, enabled: next }, `sync.${kind}.enabled`);
  }
</script>

<!--
@component
Automatic reconciliation and shared configuration editing. Drafts remain explicit:
Save publishes the desired state, and the service reconciles it automatically.
Live plan and status queries share the shell's event invalidation and polling fallback.
-->

{#snippet requestFeedback()}
  {#if requestStorageProblem || (pendingRequest && requestUncertain) || error !== null || feedbackReadError || runNotice !== ''}
    <div class="sync-feedback">
      {#if requestStorageProblem}
        <FormError message={requestStorageProblem} />
        {#if !pendingRequest}<Button tone="quiet" onclick={readRequestStorage}
            >Retry browser storage</Button
          >{/if}
      {/if}
      {#if pendingRequest && (requestUncertain || requestCleanupPending)}
        <Callout role="status">
          <div class="callout-copy">
            <strong
              >{requestCleanupPending
                ? 'Finish local request recovery'
                : pendingRequest.action === 'check'
                  ? 'Check request needs confirmation'
                  : 'Run request needs confirmation'}</strong
            >
            <p>
              {requestCleanupPending
                ? 'The server response is confirmed. Finish recovery to clear the saved browser record. This does not send the request again.'
                : 'The previous request may already have been accepted. Recover it before starting another sync request.'}
            </p>
          </div>
          {#snippet actions()}
            {#if requestCleanupPending}<Button onclick={finishRequestRecovery}
                >Finish recovery</Button
              >
            {:else if canControl}<Button
                disabled={runningNow}
                onclick={() => pendingRequest && void onRunNow(pendingRequest)}
                >{runningNow
                  ? 'Recovering…'
                  : pendingRequest?.action === 'check'
                    ? 'Recover check'
                    : 'Recover run request'}</Button
              >
            {:else}<span>You need Admin or Owner access to recover this request.</span>{/if}
          {/snippet}
        </Callout>
      {/if}
      {#if error !== null}
        <FormError message={error} />
      {/if}
      {#if feedbackReadError}<FormError message={messageOf(feedbackReadError)} />{/if}
      {#if runNotice !== ''}<div class="sync-run-notice" role="status">
          <p>{runNotice}</p>
          {#if requestedCheckId}<Link href={checkHref(requestedCheckId)}>View check</Link>{/if}
          {#if requestedDispatchId}<Link href={historyResultHref(requestedDispatchId)}
              >View accepted changes</Link
            >{/if}
        </div>{/if}
    </div>
  {/if}
{/snippet}

{#snippet pageFeedback()}
  {#if !detailsVisible}{@render requestFeedback()}{/if}
  {#if fetchRequests && actorId}
    {#key JSON.stringify([actorId, targetId])}
      <SyncRequests
        {actorId}
        {targetId}
        {nowMs}
        {fetchRequests}
        {checkHref}
        resultHref={historyResultHref}
        {onOpenCheck}
        onOpenResult={onOpenHistoryResult}
      />
    {/key}
  {/if}
{/snippet}

{#if section === 'overview' || section === 'plan' || section === 'history'}
  {#if section === 'history'}
    {@render pageFeedback()}
    <SyncHistory
      {targetId}
      {nowMs}
      {fetchHistory}
      resultHref={historyResultHref}
      onOpenResult={onOpenHistoryResult}
      onStatus={() => onOpenSection('overview')}
    />
  {:else if syncStatus !== null}
    <SyncOverview
      feedback={pageFeedback}
      status={syncStatus}
      savedConfigs={canonicalConfigs}
      {permissionsHref}
      {queueHref}
      {plan}
      configs={{
        labels: config ?? undefined,
        settings: documents.settings ?? undefined,
        rulesets: documents.rulesets ?? undefined,
        files: documents.files ?? undefined,
      }}
      {nowMs}
      {repositoryHref}
      {canControl}
      busy={runningNow}
      checkPending={pendingRequest !== null || requestStorageProblem !== null}
      onCheck={() => void onRunNow({ action: 'check', reason: 'Check sync from the status view' })}
      onDetails={openDetails}
      {sectionHref}
      {onOpenSection}
      onToggleKind={toggleKind}
      {dirtyControls}
      {readOnly}
    />
  {:else}
    {@render pageFeedback()}
    {#if !statusQuery.error}<p role="status">Loading sync status…</p>{/if}
  {/if}
  <Modal
    id="sync-details"
    open={detailsVisible}
    title="Sync details"
    variant="inspector"
    returnFocus={detailsTrigger}
    onClose={closeDetails}
  >
    {#snippet headerExtra()}<Button
        tone="quiet"
        aria-label={selectedCheckId !== null ? 'Back to check' : 'Close sync details'}
        onclick={closeDetails}>{selectedCheckId !== null ? 'Back to check' : 'Close'}</Button
      >{/snippet}
    {#if detailsVisible}{@render requestFeedback()}{/if}
    {#if inspectedPlanQuery.error || (refreshingPlan && inspectedPlanQuery.data === undefined)}
      <ResultProblem
        title={inspectedPlanProblem.title}
        problem={inspectedPlanProblem.description}
        onRetry={inspectedPlanProblem.retry ? () => void retryInspectedPlan() : undefined}
        busy={inspectedPlanQuery.isFetching}
      />
    {:else if inspectedPlanQuery.isPending}
      <p role="status">Loading sync result…</p>
    {:else}
      <SyncPlanPage
        bind:this={inspectedPage}
        embedded
        plan={inspectedPlan}
        {targetId}
        checkCapability={selectedPlanId === null
          ? planQuery.data?.check
          : selectedPlanQuery.data?.check}
        onOpenBlockingPlan={onOpenHistoryResult}
        onOpenRunningCheck={onOpenCheck}
        refreshing={refreshingPlan}
        onRefresh={refreshInspectedPlan}
        {nowMs}
        {readOnly}
        {canControl}
        {approving}
        {discarding}
        runNowBlocked={pendingRequest !== null || requestStorageProblem !== null}
        runNowBusy={runningNow}
        onApprove={(planId, digest) => void onApprove(planId, digest)}
        onDiscard={(planId) => void onDiscard(planId)}
        onRunNow={(input) => void onRunNow(input)}
      />
    {/if}
  </Modal>
{:else if section === 'labels'}
  {#key config === null}
    <SyncLabelsPage
      {config}
      {readOnly}
      problem={labelsError}
      {syncStatus}
      {nowMs}
      onChange={stageLabels}
      {dirtyControls}
    />
  {/key}
{:else if section === 'rulesets'}
  {#if rulesetName !== null}
    <SyncRulesetPage
      {organizationActors}
      {lookupBypassActors}
      config={documents.rulesets}
      savedDocument={canonicalConfigs.rulesets?.document}
      name={rulesetName}
      {readOnly}
      problem={documentError.rulesets}
      {sectionHref}
      {onOpenSection}
      onChangeDocument={(document, expected) => stageDocument(RULESETS, document, expected)}
      dirtyDocument={dirtyControls.includes('sync.rulesets.document')}
    />
  {:else}
    <SyncRulesetsPage
      config={documents.rulesets}
      savedDocument={canonicalConfigs.rulesets?.document}
      {plan}
      {readOnly}
      problem={documentError.rulesets}
      {syncStatus}
      {nowMs}
      {rulesetHref}
      {onOpenRuleset}
      onToggleEnabled={(wanted) => toggleKind(RULESETS, wanted)}
      onChangeDocument={(document) => stageDocument(RULESETS, document)}
      dirtyEnabled={dirtyControls.includes('sync.rulesets.enabled')}
      dirtyDocument={dirtyControls.includes('sync.rulesets.document')}
    />
  {/if}
{:else if section === 'files'}
  {#if fileName !== null}
    <SyncFilePage
      {repositoryHref}
      config={documents.files}
      savedDocument={canonicalConfigs.files?.document}
      context={filesContext}
      path={fileName}
      {nowMs}
      {readOnly}
      problem={documentError.files}
      {sectionHref}
      {onOpenSection}
      onChangeDocument={(document) => stageDocument(FILES, document)}
      dirtyDocument={dirtyControls.includes('sync.files.document')}
      {dirtyControls}
      fetchOverride={loadFilesOverride}
      renderFile={(input) => fileValidation?.render(targetId, input) ?? renderFile(targetId, input)}
      onFormattingValidity={(control, valid, message) =>
        drafts.setValidationProblem(settingsScope, control, valid ? null : message)}
      onChangeOverride={stageFilesOverride}
    />
  {:else}
    <SyncFilesPage
      config={documents.files}
      savedDocument={canonicalConfigs.files?.document}
      context={filesContext}
      {plan}
      {syncStatus}
      {nowMs}
      {readOnly}
      problem={documentError.files}
      {fileHref}
      {onOpenFile}
      onToggleEnabled={(wanted) => toggleKind(FILES, wanted)}
      onChangeDocument={(document) => stageDocument(FILES, document)}
      dirtyEnabled={dirtyControls.includes('sync.files.enabled')}
      dirtyDocument={dirtyControls.includes('sync.files.document')}
    />
  {/if}
{:else if section === 'settings'}
  <SyncSettingsPage
    config={documents.settings}
    savedDocument={canonicalConfigs.settings?.document}
    {readOnly}
    problem={documentError.settings}
    {syncStatus}
    {nowMs}
    onToggleEnabled={(wanted) => toggleKind(SETTINGS, wanted)}
    onChangeDocument={(document) => void stageDocument(SETTINGS, document)}
    dirtyEnabled={dirtyControls.includes('sync.settings.enabled')}
    dirtyDocument={dirtyControls.includes('sync.settings.document')}
  />
{:else}
  <section class="sync-page" aria-labelledby="sync-heading">
    <PageHeader
      id="sync-heading"
      section="Sync"
      title="Sync"
      description="What every repository in this workspace should look like, and what Smyklot would change to make that true"
    />
  </section>
{/if}

<SyncCheckInspector
  {checkHref}
  {actorId}
  {checkEvidenceApi}
  itemId={section === 'overview' ? selectedCheckId : null}
  syncResultHref={(id) =>
    selectedCheckId === null ? historyResultHref(id) : checkResultHref(selectedCheckId, id)}
  {targetId}
  {fetchCheck}
  onClose={() => onOpenSection('overview')}
/>

<style>
  /* The settings page's plates, on the settings page's ground. */
  .sync-page :global(.plate) {
    background: var(--surface-base);
  }

  .sync-feedback {
    display: grid;
    gap: var(--space-3);
  }

  .sync-run-notice {
    color: var(--text-secondary);
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }
</style>
