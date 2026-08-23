<script lang="ts">
  /**
   * Org-wide label sync: what an installation expects its repositories to carry,
   * and what the service would change to make that true.
   *
   * Two halves, in the order the questions arrive. What is configured, which is
   * the thing somebody edits. Then what that would do, which is the thing
   * somebody approves - and those are deliberately not the same act. A sync that
   * applied on save would give nobody the chance to read the deletions first.
   */
  import { untrack } from 'svelte';

  import type {
    SyncConfig,
    SyncFilesContext,
    SyncKind,
    SyncOverride,
    SyncOverrideInput,
    SyncPlan,
    SyncStatus,
  } from '#lib/types.js';
  import type { SyncDraftSet } from '#lib/sync-drafts.svelte.js';
  import type { SyncSection } from '#lib/routes.js';

  import FormError from './FormError.svelte';
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
    fetchConfig,
    drafts,
    fetchPlan,
    approvePlan,
    discardPlan,
    fetchStatus,
    sectionHref,
    onOpenSection,
    rulesetHref,
    onOpenRuleset,
    fileHref,
    onOpenFile,
    fetchFilesContext,
    fetchOverride,
    saveOverride,
    clock = Date.now,
  }: {
    targetId: string;
    /** Which of the view's sections the address names; see `routes.ts`. */
    section: SyncSection;
    /** One ruleset's own page, when the address names one. */
    rulesetName?: string | null;
    /** One template's own page, when the address names one. */
    fileName?: string | null;
    readOnly: boolean;
    rulesetHref: (name: string) => string;
    onOpenRuleset: (name: string) => void;
    fileHref: (path: string) => string;
    onOpenFile: (path: string) => void;
    fetchFilesContext: (targetId: string) => Promise<SyncFilesContext>;
    fetchOverride: (targetId: string, repositoryId: string, kind: string) => Promise<SyncOverride>;
    saveOverride: (
      targetId: string,
      repositoryId: string,
      kind: string,
      input: SyncOverrideInput,
    ) => Promise<SyncOverride>;
    fetchConfig: (targetId: string, kind: string) => Promise<SyncConfig>;
    drafts: SyncDraftSet;
    fetchPlan: (targetId: string) => Promise<{ plan: SyncPlan | null }>;
    approvePlan: (targetId: string, planId: string, digest: string) => Promise<{ plan: SyncPlan }>;
    discardPlan: (targetId: string, planId: string) => Promise<void>;
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

  const config = $derived(drafts.config(LABELS));
  const documents = $derived<Record<DocumentKind, SyncConfig | null>>({
    settings: drafts.config(SETTINGS),
    rulesets: drafts.config(RULESETS),
    files: drafts.config(FILES),
  });
  let plan = $state<SyncPlan | null>(null);
  let syncStatus = $state<SyncStatus | null>(null);
  let filesContext = $state<SyncFilesContext | null>(null);
  /* Read once per load rather than live: the overview's relative times move
     with the data they describe, not with a ticking clock. */
  let nowMs = $state(0);
  let approving = $state(false);
  let discarding = $state(false);

  let error = $state<string | null>(null);
  const labelsError = $derived(drafts.invalidKind === LABELS ? drafts.problem : error);
  const documentError = $derived<Record<DocumentKind, string | null>>({
    settings: drafts.invalidKind === SETTINGS ? drafts.problem : null,
    rulesets: drafts.invalidKind === RULESETS ? drafts.problem : null,
    files: drafts.invalidKind === FILES ? drafts.problem : null,
  });

  /* Every document, because each is only meaningful beside the plan: a plan
     says what would change, and what it would change to is what the
     configurations list. untrack keeps the writes below from feeding back into
     the read that caused them. */
  $effect(() => {
    const id = targetId;
    void drafts.refresh;
    untrack(() => void load(id));
  });

  async function load(id: string): Promise<void> {
    error = null;
    try {
      const [
        loadedConfig,
        loadedSettings,
        loadedRulesets,
        loadedFiles,
        loadedPlan,
        loadedStatus,
        loadedContext,
      ] = await Promise.all([
        fetchConfig(id, LABELS),
        fetchConfig(id, SETTINGS),
        fetchConfig(id, RULESETS),
        fetchConfig(id, FILES),
        fetchPlan(id),
        fetchStatus(id),
        fetchFilesContext(id),
      ]);
      drafts.adopt([loadedConfig, loadedSettings, loadedRulesets, loadedFiles]);
      plan = loadedPlan.plan;
      syncStatus = loadedStatus;
      filesContext = loadedContext;
      nowMs = clock();
    } catch (cause) {
      error = messageOf(cause);
    }
  }

  /** Stages the whole labels configuration in the installation-wide draft. */
  async function saveLabels(input: {
    enabled: boolean;
    labels: SyncConfig['labels'];
    allow_removal: boolean;
    excludes: string[];
  }): Promise<boolean> {
    return drafts.stage(LABELS, input);
  }

  async function onSave(enabled: boolean): Promise<void> {
    const current = config;
    if (current === null) return;
    await saveLabels({
      enabled,
      labels: current.labels,
      allow_removal: current.allow_removal,
      excludes: current.excludes,
    });
  }

  /**
   * A document kind is staged whole. The bottom composer sends every dirty kind
   * in one request, so moving between settings, rulesets and files never exposes
   * a half-saved installation.
   */
  async function onSaveDocument(
    kind: DocumentKind,
    wanted: boolean,
    document: Record<string, unknown>,
  ): Promise<boolean> {
    return drafts.stage(kind, { enabled: wanted, document });
  }

  async function onApprove(planId: string, digest: string): Promise<void> {
    approving = true;
    error = null;
    try {
      plan = (await approvePlan(targetId, planId, digest)).plan;
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
      plan = (await fetchPlan(targetId)).plan;
    } catch (cause) {
      error = messageOf(cause);
    } finally {
      discarding = false;
    }
  }

  function messageOf(cause: unknown): string {
    return cause instanceof Error ? cause.message : String(cause);
  }

  /* The overview's kind switches are the same acts the forms perform: labels
     save through the typed fields, every other kind saves its document back
     with only the switch changed. */
  function toggleKind(kind: SyncKind, next: boolean): void {
    if (kind === 'labels') {
      void onSave(next);
      return;
    }
    const current = documents[kind];
    if (current !== null) void onSaveDocument(kind, next, current.document);
  }
</script>

{#if section === 'overview'}
  {#if error !== null}
    <FormError message={error} />
  {/if}
  {#if syncStatus !== null}
    <SyncOverview
      status={syncStatus}
      {plan}
      configs={{
        labels: config ?? undefined,
        settings: documents.settings ?? undefined,
        rulesets: documents.rulesets ?? undefined,
        files: documents.files ?? undefined,
      }}
      {nowMs}
      {sectionHref}
      {onOpenSection}
      onToggleKind={toggleKind}
      {readOnly}
    />
  {/if}
{:else if section === 'plan'}
  {#if error !== null}
    <FormError message={error} />
  {/if}
  <SyncPlanPage
    {plan}
    {nowMs}
    {readOnly}
    {approving}
    {discarding}
    {sectionHref}
    {onOpenSection}
    onApprove={(planId, digest) => void onApprove(planId, digest)}
    onDiscard={(planId) => void onDiscard(planId)}
  />
{:else if section === 'labels'}
  {#key config === null}
    <SyncLabelsPage
      {config}
      {readOnly}
      problem={labelsError}
      {sectionHref}
      {onOpenSection}
      onSave={saveLabels}
    />
  {/key}
{:else if section === 'rulesets'}
  {#if rulesetName !== null}
    <SyncRulesetPage
      config={documents.rulesets}
      name={rulesetName}
      {readOnly}
      problem={documentError.rulesets}
      saving={drafts.saving}
      {sectionHref}
      {onOpenSection}
      onSave={(wanted, document) => void onSaveDocument(RULESETS, wanted, document)}
    />
  {:else}
    <SyncRulesetsPage
      config={documents.rulesets}
      {plan}
      {readOnly}
      problem={documentError.rulesets}
      saving={drafts.saving}
      {sectionHref}
      {onOpenSection}
      {rulesetHref}
      {onOpenRuleset}
      onSave={(wanted, document) => void onSaveDocument(RULESETS, wanted, document)}
    />
  {/if}
{:else if section === 'files'}
  {#if fileName !== null}
    <SyncFilePage
      config={documents.files}
      context={filesContext}
      path={fileName}
      {nowMs}
      {readOnly}
      problem={documentError.files}
      saving={drafts.saving}
      {sectionHref}
      {onOpenSection}
      onSave={(wanted, document) => onSaveDocument(FILES, wanted, document)}
      fetchOverride={(repositoryId) => fetchOverride(targetId, repositoryId, FILES)}
      saveOverride={(repositoryId, input) => saveOverride(targetId, repositoryId, FILES, input)}
    />
  {:else}
    <SyncFilesPage
      config={documents.files}
      context={filesContext}
      {plan}
      status={syncStatus}
      {nowMs}
      {readOnly}
      problem={documentError.files}
      saving={drafts.saving}
      {sectionHref}
      {onOpenSection}
      {fileHref}
      {onOpenFile}
      onSave={(wanted, document) => void onSaveDocument(FILES, wanted, document)}
    />
  {/if}
{:else if section === 'settings'}
  <SyncSettingsPage
    config={documents.settings}
    {readOnly}
    problem={documentError.settings}
    saving={drafts.saving}
    {sectionHref}
    {onOpenSection}
    onSave={(wanted, document) => void onSaveDocument(SETTINGS, wanted, document)}
  />
{:else}
  <section class="sync-page" aria-labelledby="sync-heading">
    <PageHeader
      id="sync-heading"
      title="Sync"
      description="What every repository in this installation should look like, and what Smyklot would change to make that true"
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
</style>
