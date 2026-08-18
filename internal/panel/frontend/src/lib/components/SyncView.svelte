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

  import type { SyncAction, SyncConfig, SyncConfigInput, SyncLabel, SyncPlan } from '#lib/types.js';

  import Button from './Button.svelte';
  import FormError from './FormError.svelte';
  import PageHeader from './PageHeader.svelte';
  import Plate from './Plate.svelte';
  import SyncFilesForm from './SyncFilesForm.svelte';
  import SyncLabelsForm from './SyncLabelsForm.svelte';
  import SyncRulesetsForm from './SyncRulesetsForm.svelte';
  import SyncSettingsForm from './SyncSettingsForm.svelte';

  const {
    targetId,
    readOnly,
    fetchConfig,
    saveConfig,
    fetchPlan,
    approvePlan,
  }: {
    targetId: string;
    readOnly: boolean;
    fetchConfig: (targetId: string, kind: string) => Promise<SyncConfig>;
    saveConfig: (targetId: string, kind: string, input: SyncConfigInput) => Promise<SyncConfig>;
    fetchPlan: (targetId: string) => Promise<{ plan: SyncPlan | null }>;
    approvePlan: (targetId: string, planId: string, digest: string) => Promise<{ plan: SyncPlan }>;
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
  let saving = $state(false);
  let approving = $state(false);

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
      const [loadedConfig, loadedSettings, loadedRulesets, loadedFiles, loadedPlan] =
        await Promise.all([
          fetchConfig(id, LABELS),
          fetchConfig(id, SETTINGS),
          fetchConfig(id, RULESETS),
          fetchConfig(id, FILES),
          fetchPlan(id),
        ]);
      config = loadedConfig;
      documents = { settings: loadedSettings, rulesets: loadedRulesets, files: loadedFiles };
      plan = loadedPlan.plan;
    } catch (cause) {
      error = messageOf(cause);
    }
  }

  async function onSave(
    enabled: boolean,
    labels: SyncLabel[],
    allowRemoval: boolean,
    excludes: string[],
  ): Promise<void> {
    const current = config;
    if (current === null) return;

    saving = true;
    error = null;
    try {
      config = await saveConfig(targetId, LABELS, {
        enabled,
        labels,
        allow_removal: allowRemoval,
        excludes,
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

  /**
   * A plan is only worth approving while it is waiting for somebody. The other
   * states are reported rather than acted on, which is why the button is bound
   * to this and not merely to a plan existing.
   */
  const approvable = $derived(plan !== null && plan.state === 'computed');

  const planNote = $derived(planExplanation(plan));

  /**
   * What a plan's state means, in the words somebody needs to decide what to do
   * next. "Stale" and "expired" are the pair worth spelling out: one says
   * somebody changed the configuration and the other says nobody came back in
   * time, and a reader who cannot tell them apart cannot tell whose turn it is.
   */
  function planExplanation(current: SyncPlan | null): string | null {
    if (current === null) {
      return null;
    }

    switch (current.state) {
      case 'computed':
        return 'Waiting for you. Nothing has been changed on GitHub yet.';
      case 'approved':
        return 'Approved, and waiting for the service to pick it up.';
      case 'applying':
        return 'Being applied now.';
      case 'applied':
        return 'Applied.';
      case 'failed':
        return 'Some of this did not apply. The rows below say which.';
      case 'stale':
        return 'The labels changed while this was on screen, so it no longer describes what would happen.';
      case 'expired':
        return 'Nobody acted on this in time. The next sweep will work it out again.';
      default:
        return null;
    }
  }

  function operationLabel(action: SyncAction): string {
    switch (action.operation) {
      case 'create':
        return 'add';
      case 'update':
        return 'change';
      default:
        return 'remove';
    }
  }
</script>

<section class="sync-page" aria-labelledby="sync-heading">
  <PageHeader
    id="sync-heading"
    title="Sync"
    description="What every repository in this installation should look like, and what Smyklot would change to make that true"
  />

  {#if config !== null}
    <SyncLabelsForm
      {labels}
      allowRemoval={config.allow_removal}
      excludes={config.excludes}
      {enabled}
      {unreadable}
      {unavailable}
      problem={error}
      {readOnly}
      {saving}
      onSave={(wanted, edited, allowRemoval, excludes) =>
        void onSave(wanted, edited, allowRemoval, excludes)}
    />
  {:else if error !== null}
    <!-- Nothing loaded, so there is no form to hang this on. It used to hang on
         the labels plate, which was the one part of this page drawn whether or
         not anything had been read - and a failure with nowhere to go is a page
         that comes up blank and says why nowhere. -->
    <FormError message={error} />
  {/if}

  {#if documents.settings !== null}
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

  {#if documents.rulesets !== null}
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

  {#if documents.files !== null}
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

  <Plate label="What would change">
    {#if plan === null}
      <!-- Deliberately not "nothing to do". Nothing is waiting, which is also
           what it looks like a moment after saving, before any reconcile has
           read a repository - and telling somebody their new configuration
           needs no changes would be a claim nothing here has checked. -->
      <p class="sync-lead">
        Nothing waiting. A reconcile runs on a timer and proposes whatever differs
      </p>
    {:else}
      <p class="sync-lead">{planNote}</p>
      <p class="sync-counts">
        {plan.counts.create} to add, {plan.counts.update} to change, {plan.counts.delete} to remove
      </p>

      <ul class="sync-rows">
        {#each plan.actions as action (action.repository + action.kind + action.subject)}
          <li class="sync-row" class:sync-removal={action.operation === 'delete'}>
            <span class="sync-operation">{operationLabel(action)}</span>
            <!-- Which of the sections above this row came from. One list covers
                 them all, and "change repository" says nothing on its own. -->
            <span class="sync-kind">{action.kind}</span>
            <span class="sync-name">{action.subject}</span>
            {#if action.after}
              <span class="sync-description">{action.after}</span>
            {:else if action.before}
              <span class="sync-description">{action.before}</span>
            {/if}
            {#if action.error}
              <span class="sync-failure">{action.error}</span>
            {:else if action.blocker}
              <span class="sync-failure">not tried: {action.blocker} failed first</span>
            {/if}
          </li>
        {/each}
      </ul>

      {#if approvable && !readOnly}
        {@const approved = plan}
        <div class="sync-actions">
          <Button
            tone="signal"
            disabled={approving}
            onclick={() => onApprove(approved.id, approved.digest)}
          >
            {approving ? 'Approving' : 'Apply these changes'}
          </Button>
        </div>
      {/if}
    {/if}
  </Plate>
</section>

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
  .sync-counts {
    color: var(--dim);
    font-size: var(--font-size-meta);
    margin: var(--space-3) 0 0;
    max-width: 60ch;
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

  .sync-description,
  .sync-kind {
    color: var(--dim);
    font-size: var(--font-size-meta);
  }

  .sync-operation {
    color: var(--dim);
    font-size: var(--font-size-meta);
    font-variant-numeric: tabular-nums;
    min-width: 4.5rem;
  }

  /* A removal is the one row worth finding without reading. It destroys
     something somebody may have made by hand, and it is off unless an operator
     switched it on. */
  .sync-removal .sync-operation {
    color: var(--text-primary);
    font-weight: 600;
  }

  .sync-failure {
    color: var(--stop);
    flex-basis: 100%;
    font-size: var(--font-size-meta);
  }

  .sync-actions {
    margin-top: var(--space-4);
  }
</style>
