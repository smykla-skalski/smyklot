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

  import type { SyncConfig, SyncConfigInput, SyncKind, SyncPlan, SyncStatus } from '#lib/types.js';
  import type { SyncSection } from '#lib/routes.js';

  import FormError from './FormError.svelte';
  import PageHeader from './PageHeader.svelte';
  import Plate from './Plate.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import SyncFilesForm from './SyncFilesForm.svelte';
  import SyncOverview from './SyncOverview.svelte';
  import SyncPlanPage from './SyncPlanPage.svelte';
  import SyncRulesetsForm from './SyncRulesetsForm.svelte';
  import SyncSettingsForm from './SyncSettingsForm.svelte';

  /** The same two words the settings page puts on the same decision. */
  const SYNC_OPTIONS = [
    { value: 'enabled', label: 'Enabled' },
    { value: 'disabled', label: 'Disabled' },
  ] as const;

  const {
    targetId,
    section,
    readOnly,
    fetchConfig,
    saveConfig,
    fetchPlan,
    approvePlan,
    discardPlan,
    fetchStatus,
    sectionHref,
    onOpenSection,
  }: {
    targetId: string;
    /** Which of the view's sections the address names; see `routes.ts`. */
    section: SyncSection;
    readOnly: boolean;
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
  /* Read once per load rather than live: the overview's relative times move
     with the data they describe, not with a ticking clock. */
  let nowMs = $state(Date.now());
  let saving = $state(false);
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
      const [loadedConfig, loadedSettings, loadedRulesets, loadedFiles, loadedPlan, loadedStatus] =
        await Promise.all([
          fetchConfig(id, LABELS),
          fetchConfig(id, SETTINGS),
          fetchConfig(id, RULESETS),
          fetchConfig(id, FILES),
          fetchPlan(id),
          fetchStatus(id),
        ]);
      config = loadedConfig;
      documents = { settings: loadedSettings, rulesets: loadedRulesets, files: loadedFiles };
      plan = loadedPlan.plan;
      syncStatus = loadedStatus;
      nowMs = Date.now();
    } catch (cause) {
      error = messageOf(cause);
    }
  }

  async function onSave(enabled: boolean): Promise<void> {
    const current = config;
    if (current === null) return;

    saving = true;
    error = null;
    try {
      config = await saveConfig(targetId, LABELS, {
        enabled,
        labels: current.labels,
        allow_removal: current.allow_removal,
        excludes: current.excludes,
        expected_revision: current.revision,
      });
      // Saving invalidates any plan computed from the old configuration, so the
      // one on screen is re-read rather than left describing something that is
      // no longer true.
      plan = (await fetchPlan(targetId)).plan;
    } catch (cause) {
      error = messageOf(cause);
    } finally {
      saving = false;
    }
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
  ): Promise<void> {
    const current = documents[kind];
    if (current === null) return;

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
    } catch (cause) {
      documentError = { ...documentError, [kind]: messageOf(cause) };
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

  const labels = $derived(config?.labels ?? []);
  const enabled = $derived(config?.enabled ?? false);

  /**
   * A configuration this version cannot read shows an empty list because
   * nothing came out of the stored document, not because nothing is
   * configured. Saving from that form would send the emptiness back and wipe a
   * label set nobody was ever shown, so nothing here is editable.
   */
  const unreadable = $derived(config?.unreadable === true);

  /**
   * What labels sync needs and this installation has not granted. Empty for
   * nearly every installation - labelling is what the bot was let in to do -
   * but the answer carries it for every kind, and a page that read it for one
   * kind and not the other would go quiet on whichever one was missed next.
   */
  const unavailable = $derived(config?.unavailable ?? '');

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
{:else}
  <section class="sync-page" aria-labelledby="sync-heading">
    <PageHeader
      id="sync-heading"
      title="Sync"
      description="What every repository in this installation should look like, and what Smyklot would change to make that true"
    />

    {#if section === 'labels'}
      <Plate label="Labels">
        {#snippet status()}
          <SegmentedControl
            name="sync-labels-{targetId}"
            label="Label sync"
            descriptionId="sync-labels-help"
            options={SYNC_OPTIONS}
            value={enabled ? 'enabled' : 'disabled'}
            compact
            disabled={saving || readOnly || unreadable || config === null}
            onSelect={(selection) => void onSave(selection === 'enabled')}
          />
        {/snippet}

        <p class="sync-lead" id="sync-labels-help">
          The labels every repository in this installation should carry. Smyklot works out what
          would change and asks before changing anything
        </p>

        {#if error !== null}
          <FormError message={error} />
        {/if}

        {#if unreadable}
          <p class="sync-notice" role="alert">
            This installation's labels are stored in a form this version of Smyklot cannot read, so
            they are not shown and nothing here can be changed. Nothing has been lost.
          </p>
        {/if}

        <!-- Only while the switch is on: a kind nobody asked for is not waiting on
         anything, and the permission is somebody else's to grant. -->
        {#if unavailable !== '' && enabled}
          <p class="sync-notice" role="status">
            {unavailable}. Nothing here will be planned or changed until an owner grants it on the
            installation's page on GitHub.
          </p>
        {/if}

        {#if unreadable}
          <!-- Deliberately not "no labels yet". An empty list here would be the panel
           inventing an answer it does not have. -->
        {:else if labels.length === 0}
          <p class="sync-empty">No labels yet</p>
        {:else}
          <ul class="sync-rows">
            {#each labels as label (label.name)}
              <li class="sync-row">
                <!-- The colour is the label's own, so it is set as a custom property
                 rather than an inline style: the panel serves style-src 'self',
                 under which a style attribute is parsed and then discarded. -->
                <span class="sync-swatch" style:--swatch="#{label.color}" aria-hidden="true"></span>
                <span class="sync-name">{label.name}</span>
                {#if label.description}
                  <span class="sync-description">{label.description}</span>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </Plate>
    {/if}

    {#if section === 'settings' && documents.settings !== null}
      <SyncSettingsForm
        stored={documents.settings.document}
        enabled={documents.settings.enabled}
        unreadable={documents.settings.unreadable}
        unavailable={documents.settings.unavailable}
        problem={documentError.settings}
        {readOnly}
        saving={savingDocument.settings}
        onSave={(wanted, document) => onSaveDocument(SETTINGS, wanted, document)}
      />
    {/if}

    {#if section === 'rulesets' && documents.rulesets !== null}
      <SyncRulesetsForm
        stored={documents.rulesets.document}
        enabled={documents.rulesets.enabled}
        unreadable={documents.rulesets.unreadable}
        unavailable={documents.rulesets.unavailable}
        problem={documentError.rulesets}
        {readOnly}
        saving={savingDocument.rulesets}
        onSave={(wanted, document) => onSaveDocument(RULESETS, wanted, document)}
      />
    {/if}

    {#if section === 'files' && documents.files !== null}
      <SyncFilesForm
        stored={documents.files.document}
        enabled={documents.files.enabled}
        unreadable={documents.files.unreadable}
        unavailable={documents.files.unavailable}
        problem={documentError.files}
        {readOnly}
        saving={savingDocument.files}
        onSave={(wanted, document) => onSaveDocument(FILES, wanted, document)}
      />
    {/if}
  </section>
{/if}

<style>
  /* The settings page's plates, on the settings page's ground. */
  .sync-page :global(.plate) {
    background: var(--surface-base);
  }

  /* A plate's opening line, which the body's own padding already places. */
  .sync-lead {
    color: var(--dim);
    font-size: var(--font-size-meta);
    margin: 0;
    max-width: 60ch;
  }

  /* The same line, further down a plate, so it carries the gap itself. */
  .sync-empty {
    color: var(--dim);
    font-size: var(--font-size-meta);
    margin: var(--space-3) 0 0;
    max-width: 60ch;
  }

  .sync-notice {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    font-size: var(--font-size-meta);
    margin: var(--space-3) 0 0;
    padding: var(--space-2) var(--space-3);
  }

  :global(.form-error) {
    margin: var(--space-3) 0 0;
  }

  /* The rows the configuration editor draws: hairlines between, no box around,
     because the plate is already the box. A bordered list inside a bordered card
     reads as two cards, which is what this page used to look like. */
  .sync-rows {
    list-style: none;
    margin: var(--space-3) 0 0;
    padding: 0;
  }

  .sync-row {
    align-items: baseline;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    padding-block: 0.7rem;
  }

  .sync-row + .sync-row {
    border-top: 1px solid var(--rule);
  }

  /* The plate's own padding closes the list; the row's would double it. */
  .sync-rows > .sync-row:last-child {
    padding-bottom: 0.15rem;
  }

  .sync-name {
    font-size: 0.875rem;
    font-weight: 600;
  }

  /* The swatch sits on the text baseline rather than centred on the line box, so
     a row whose description wraps does not leave it floating beside the gap. */
  .sync-swatch {
    background: var(--swatch);
    border-radius: 50%;
    display: inline-block;
    height: 0.75em;
    transform: translateY(0.05em);
    width: 0.75em;
  }

  .sync-description {
    color: var(--dim);
    font-size: var(--font-size-meta);
  }
</style>
