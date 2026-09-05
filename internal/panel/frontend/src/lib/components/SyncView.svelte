<script lang="ts">
  import { untrack } from 'svelte';
  import { useInterval } from 'runed';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';

  import { formatJson, type JsonValue } from '#lib/merge.js';
  import {
    adoptSyncOverrideSettings,
    stageSyncOverrideControl,
    syncOverrideDraftEnvelope,
    type SyncOverrideControlId,
    type SyncOverrideEditorEnvelope,
  } from '#lib/repository-sync-override-settings.js';
  import { getSettingsDraftRegistry, type SettingsScope } from '#lib/settings-drafts.svelte.js';
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
    SyncConfig,
    SyncFilesContext,
    SyncKind,
    SyncOverride,
    SyncPlan,
    SyncRunNowResponse,
    SyncStatus,
  } from '#lib/types.js';
  import type {
    SyncFileRenderInput,
    SyncFileRenderResponse,
  } from '#lib/sync-file-render.generated.js';
  import type { SyncSection } from '#lib/routes.js';

  import FormError from './FormError.svelte';
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
    section,
    rulesetName = null,
    fileName = null,
    readOnly,
    canControl = false,
    fetchConfig,
    fetchPlan,
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
  }: {
    permissionsHref?: string | null;
    queueHref?: string | null;
    repositoryHref?: ((repository: string) => string) | null;
    targetId: string;
    /** Which of the view's sections the address names; see `routes.ts`. */
    section: SyncSection;
    /** One ruleset's own page, when the address names one. */
    rulesetName?: string | null;
    /** One template's own page, when the address names one. */
    fileName?: string | null;
    readOnly: boolean;
    canControl?: boolean;
    rulesetHref: (name: string) => string;
    onOpenRuleset: (name: string) => void;
    fileHref: (path: string) => string;
    onOpenFile: (path: string) => void;
    fetchFilesContext: (targetId: string) => Promise<SyncFilesContext>;
    renderFile: (targetId: string, input: SyncFileRenderInput) => Promise<SyncFileRenderResponse>;
    fetchOverride: (targetId: string, repositoryId: string, kind: string) => Promise<SyncOverride>;
    fetchConfig: (targetId: string, kind: string) => Promise<SyncConfig>;
    fetchPlan: (targetId: string) => Promise<{ plan: SyncPlan | null }>;
    approvePlan: (targetId: string, planId: string, digest: string) => Promise<{ plan: SyncPlan }>;
    discardPlan: (targetId: string, planId: string) => Promise<void>;
    runSyncNow?: (
      targetId: string,
      input: { expected_revision: number; reason: string },
    ) => Promise<SyncRunNowResponse>;
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
    notifyOnChangeProps: ['data', 'error'],
    queryFn: () => fetchPlan(targetId),
  }));
  const statusQuery = createQuery(() => ({
    queryKey: ['sync-status', targetId],
    queryFn: () => fetchStatus(targetId),
  }));
  const plan = $derived(planQuery.data?.plan ?? null);
  const syncStatus = $derived(statusQuery.data ?? null);
  let detailsOpen = $state(false);
  let detailsTrigger = $state<HTMLElement | null>(null);
  function closeDetails(): void {
    detailsOpen = false;
    if (section === 'plan') onOpenSection('overview');
  }
  function openDetails(trigger: HTMLElement): void {
    detailsTrigger = trigger;
    detailsOpen = true;
  }
  let filesContext = $state<SyncFilesContext | null>(null);
  /* The injected clock keeps catalogue examples deterministic while live views age. */
  let nowMs = $state(untrack(() => clock()));
  useInterval(30_000, { callback: () => (nowMs = clock()) });
  let approving = $state(false);
  let discarding = $state(false);
  let runningNow = $state(false);
  let runNotice = $state('');

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
  ): boolean {
    const canonical = canonicalConfigs[kind];
    if (
      canonical === undefined ||
      canonical.unreadable ||
      !stageSyncConfigControl(drafts, targetId, canonical, envelope, controlId)
    ) {
      setStageProblem(kind, 'This Sync configuration change is not valid');
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

  function stageDocument(kind: DocumentKind, document: Record<string, unknown>): boolean {
    const current = currentEnvelope(kind);
    if (current === null || current.kind === LABELS) return false;
    try {
      return stageEnvelope(
        kind,
        { ...current, document_text: formatJson(document as JsonValue).trimEnd() },
        `sync.${kind}.document`,
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

  async function onApprove(planId: string, digest: string): Promise<void> {
    approving = true;
    error = null;
    try {
      queryClient.setQueryData(
        ['sync-plan', targetId],
        await approvePlan(targetId, planId, digest),
      );
      await statusQuery.refetch();
    } catch (cause) {
      error = messageOf(cause);
    } finally {
      approving = false;
    }
  }

  /** Throwing a plan away asks nothing on GitHub - the next sweep recomputes. */
  async function onDiscard(planId: string): Promise<void> {
    discarding = true;
    error = null;
    try {
      await discardPlan(targetId, planId);
      await Promise.all([planQuery.refetch(), statusQuery.refetch()]);
    } catch (cause) {
      error = messageOf(cause);
    } finally {
      discarding = false;
    }
  }

  async function onRunNow(reason: string): Promise<void> {
    runningNow = true;
    error = null;
    runNotice = '';
    try {
      const response = await runSyncNow(targetId, {
        expected_revision: plan?.queue_item?.revision ?? 0,
        reason,
      });
      if (response.plan !== undefined)
        queryClient.setQueryData(['sync-plan', targetId], { plan: response.plan });
      if (response.status === 'scan_queued')
        runNotice = 'Checking repositories · changes will sync automatically';
      if (response.status === 'plan_dispatched') runNotice = 'Sync queued for immediate dispatch';
      if (response.status === 'approval_required')
        runNotice = 'An earlier sync needs a one-time decision · open Review changes';
      if (response.status === 'already_running') runNotice = 'Sync is already running';
    } catch (cause) {
      error = messageOf(cause);
    } finally {
      runningNow = false;
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

{#if section === 'overview' || section === 'plan'}
  {#if error !== null}
    <FormError message={error} />
  {/if}
  {#if planQuery.error || statusQuery.error}<FormError
      message={messageOf(planQuery.error ?? statusQuery.error)}
    />{/if}
  {#if runNotice !== ''}<p class="sync-run-notice" role="status">{runNotice}</p>{/if}
  {#if syncStatus !== null}
    <SyncOverview
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
      onCheck={() => void onRunNow('Check sync from the status view')}
      onDetails={openDetails}
      repositories={filesContext?.repositories ?? null}
      {sectionHref}
      {onOpenSection}
      onToggleKind={toggleKind}
      {dirtyControls}
      {readOnly}
    />
  {:else if !statusQuery.error}
    <p role="status">Loading sync status…</p>
  {/if}
  <Modal
    id="sync-details"
    open={detailsOpen || section === 'plan'}
    title="Sync details"
    variant="inspector"
    returnFocus={detailsTrigger}
    onClose={closeDetails}
  >
    {#snippet headerExtra()}<Button
        tone="quiet"
        aria-label="Close sync details"
        onclick={closeDetails}>Close</Button
      >{/snippet}
    <SyncPlanPage
      embedded
      {plan}
      {nowMs}
      {readOnly}
      {canControl}
      {approving}
      {discarding}
      runNowBusy={runningNow}
      onApprove={(planId, digest) => void onApprove(planId, digest)}
      onDiscard={(planId) => void onDiscard(planId)}
      onRunNow={(reason) => void onRunNow(reason)}
    />
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
      config={documents.rulesets}
      savedDocument={canonicalConfigs.rulesets?.document}
      name={rulesetName}
      {readOnly}
      problem={documentError.rulesets}
      {sectionHref}
      {onOpenSection}
      onChangeDocument={(document) => void stageDocument(RULESETS, document)}
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
      onChangeDocument={(document) => void stageDocument(RULESETS, document)}
      dirtyEnabled={dirtyControls.includes('sync.rulesets.enabled')}
      dirtyDocument={dirtyControls.includes('sync.rulesets.document')}
    />
  {/if}
{:else if section === 'files'}
  {#if fileName !== null}
    <SyncFilePage
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
      renderFile={(input) => renderFile(targetId, input)}
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
      onChangeDocument={(document) => void stageDocument(FILES, document)}
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

<style>
  /* The settings page's plates, on the settings page's ground. */
  .sync-page :global(.plate) {
    background: var(--surface-base);
  }

  :global(.form-error) {
    margin: var(--space-3) 0 0;
  }

  .sync-run-notice {
    border-inline-start: 2px solid var(--info);
    color: var(--text-secondary);
    margin: var(--space-3) 0;
    padding: var(--space-2) var(--space-3);
  }
</style>
