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
    SyncConfigInput,
    SyncFilesContext,
    SyncKind,
    SyncOverride,
    SyncOverrideInput,
    SyncPlan,
    SyncStatus,
  } from '#lib/types.js';
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
    editorLogin = '',
    readOnly,
    fetchConfig,
    saveConfig,
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
  }: {
    targetId: string;
    /** Which of the view's sections the address names; see `routes.ts`. */
    section: SyncSection;
    /** One ruleset's own page, when the address names one. */
    rulesetName?: string | null;
    /** One template's own page, when the address names one. */
    fileName?: string | null;
    /** Who is signed in, stamped onto a template's freshness on save. */
    editorLogin?: string;
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
    saveConfig: (targetId: string, kind: string, input: SyncConfigInput) => Promise<SyncConfig>;
    fetchPlan: (targetId: string) => Promise<{ plan: SyncPlan | null }>;
    approvePlan: (targetId: string, planId: string, digest: string) => Promise<{ plan: SyncPlan }>;
    discardPlan: (targetId: string, planId: string) => Promise<void>;
    fetchStatus: (targetId: string) => Promise<SyncStatus>;
    sectionHref: (section: SyncSection) => string;
    onOpenSection: (section: SyncSection) => void;
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

  let config = $state<SyncConfig | null>(null);
  let plan = $state<SyncPlan | null>(null);
  let syncStatus = $state<SyncStatus | null>(null);
  let filesContext = $state<SyncFilesContext | null>(null);
  /* Read once per load rather than live: the overview's relative times move
     with the data they describe, not with a ticking clock. */
  let nowMs = $state(Date.now());
  let approving = $state(false);
  let discarding = $state(false);

  /* Kept per kind rather than as a field each, because every kind after labels
     has the same three: what is saved, whether a save is in flight, and what
     went wrong. Three fields per kind is how the third one comes to reuse the
     second one's by accident. */
  let documents = $state<Record<DocumentKind, SyncConfig | null>>({
    settings: null,
    rulesets: null,
    files: null,
  });
  let savingDocument = $state<Record<DocumentKind, boolean>>({
    settings: false,
    rulesets: false,
    files: false,
  });

  /* One failure per thing that can fail, because the forms are saved
     independently and none disables the others. A single field let a settings
     save clear a labels failure the moment it started - the label switch had
     already sprung back and nothing on the page said why. */
  let error = $state<string | null>(null);
  let documentError = $state<Record<DocumentKind, string | null>>({
    settings: null,
    rulesets: null,
    files: null,
  });

  /* Every document, because each is only meaningful beside the plan: a plan
     says what would change, and what it would change to is what the
     configurations list. untrack keeps the writes below from feeding back into
     the read that caused them. */
  $effect(() => {
    const id = targetId;
    untrack(() => void load(id));
  });

  async function load(id: string): Promise<void> {
    error = null;
    documentError = { settings: null, rulesets: null, files: null };
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
      config = loadedConfig;
      documents = { settings: loadedSettings, rulesets: loadedRulesets, files: loadedFiles };
      plan = loadedPlan.plan;
      syncStatus = loadedStatus;
      filesContext = loadedContext;
      nowMs = Date.now();
    } catch (cause) {
      error = messageOf(cause);
    }
  }

  /** Saves the whole labels configuration; the page whispers on true. */
  async function saveLabels(input: {
    enabled: boolean;
    labels: SyncConfig['labels'];
    allow_removal: boolean;
    excludes: string[];
  }): Promise<boolean> {
    const current = config;
    if (current === null) return false;

    error = null;
    try {
      config = await saveConfig(targetId, LABELS, {
        ...input,
        expected_revision: current.revision,
      });
      // Saving invalidates any plan computed from the old configuration, so the
      // one on screen is re-read rather than left describing something that is
      // no longer true.
      plan = (await fetchPlan(targetId)).plan;
      return true;
    } catch (cause) {
      error = messageOf(cause);
      return false;
    }
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
   * A document kind is saved whole, unlike the labels switch beside it: a
   * repository's settings are one request that succeeds or fails together, and
   * a ruleset is written by replacement, so a control that saved on every click
   * would send a dozen half-formed policies and compute a plan for each.
   */
  async function onSaveDocument(
    kind: DocumentKind,
    wanted: boolean,
    document: Record<string, unknown>,
  ): Promise<boolean> {
    const current = documents[kind];
    if (current === null) return false;

    savingDocument = { ...savingDocument, [kind]: true };
    documentError = { ...documentError, [kind]: null };
    try {
      const saved = await saveConfig(targetId, kind, {
        enabled: wanted,
        document,
        expected_revision: current.revision,
      });
      documents = { ...documents, [kind]: saved };
      plan = (await fetchPlan(targetId)).plan;
      return true;
    } catch (cause) {
      documentError = { ...documentError, [kind]: messageOf(cause) };
      return false;
    } finally {
      savingDocument = { ...savingDocument, [kind]: false };
    }
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
  <SyncLabelsPage
    {config}
    {readOnly}
    problem={error}
    {sectionHref}
    {onOpenSection}
    onSave={saveLabels}
  />
{:else if section === 'rulesets'}
  {#if rulesetName !== null}
    <SyncRulesetPage
      config={documents.rulesets}
      name={rulesetName}
      {readOnly}
      problem={documentError.rulesets}
      saving={savingDocument.rulesets}
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
      saving={savingDocument.rulesets}
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
      saving={savingDocument.files}
      {editorLogin}
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
      saving={savingDocument.files}
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
    saving={savingDocument.settings}
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
