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

  import type { SyncAction, SyncConfig, SyncConfigInput, SyncPlan } from '$lib/types';

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

  // The kinds this view has a form for. Rulesets and files are configurable
  // through the API and have none here yet, so naming the ones this page means
  // is better than a parameter nothing varies.
  const LABELS = 'labels';
  const SETTINGS = 'settings';

  let config = $state<SyncConfig | null>(null);
  let settings = $state<SyncConfig | null>(null);
  let plan = $state<SyncPlan | null>(null);
  let saving = $state(false);
  let savingSettings = $state(false);
  let approving = $state(false);

  /* One failure per thing that can fail, because the two forms are saved
     independently and neither disables the other. A single field let a settings
     save clear a labels failure the moment it started - the label switch had
     already sprung back and nothing on the page said why. */
  let error = $state<string | null>(null);
  let settingsError = $state<string | null>(null);

  /* Both documents, because the second is only meaningful beside the first: a
     plan says what would change, and what it would change to is what the
     configuration lists. untrack keeps the writes below from feeding back into
     the read that caused them. */
  $effect(() => {
    const id = targetId;
    untrack(() => void load(id));
  });

  async function load(id: string): Promise<void> {
    error = null;
    settingsError = null;
    try {
      const [loadedConfig, loadedSettings, loadedPlan] = await Promise.all([
        fetchConfig(id, LABELS),
        fetchConfig(id, SETTINGS),
        fetchPlan(id),
      ]);
      config = loadedConfig;
      settings = loadedSettings;
      plan = loadedPlan.plan;
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
   * The settings are saved whole, unlike the labels switch beside them: a
   * repository's settings are one request that succeeds or fails together, so
   * a control that saved on every click would send a dozen half-formed
   * policies and compute a plan for each.
   */
  async function onSaveSettings(wanted: boolean, document: Record<string, unknown>): Promise<void> {
    const current = settings;
    if (current === null) return;

    savingSettings = true;
    settingsError = null;
    try {
      settings = await saveConfig(targetId, SETTINGS, {
        enabled: wanted,
        document,
        expected_revision: current.revision,
      });
      plan = (await fetchPlan(targetId)).plan;
    } catch (cause) {
      settingsError = messageOf(cause);
    } finally {
      savingSettings = false;
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

<section class="sync" aria-labelledby="sync-heading">
  <header class="sync-header">
    <h2 id="sync-heading">Labels</h2>
    <p class="sync-lead">
      The labels every repository in this installation should carry. Smyklot works out what would
      change and asks before changing anything.
    </p>
  </header>

  {#if error !== null}
    <p class="sync-error" role="alert">{error}</p>
  {/if}

  {#if unreadable}
    <p class="sync-error" role="alert">
      This installation's labels are stored in a form this version of Smyklot cannot read, so they
      are not shown and nothing here can be changed. Nothing has been lost.
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

  <div class="sync-switch">
    <label>
      <input
        type="checkbox"
        checked={enabled}
        disabled={saving || readOnly || unreadable || config === null}
        onchange={(event) => onSave(event.currentTarget.checked)}
      />
      Keep these labels in step across every repository
    </label>
  </div>

  {#if unreadable}
    <!-- Deliberately not "no labels yet". An empty list here would be the panel
         inventing an answer it does not have. -->
  {:else if labels.length === 0}
    <p class="sync-empty">No labels yet.</p>
  {:else}
    <ul class="sync-labels">
      {#each labels as label (label.name)}
        <li class="sync-label">
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
</section>

{#if settings !== null}
  <SyncSettingsForm
    stored={settings.document}
    enabled={settings.enabled}
    unreadable={settings.unreadable}
    unavailable={settings.unavailable}
    problem={settingsError}
    {readOnly}
    saving={savingSettings}
    onSave={onSaveSettings}
  />
{/if}

<section class="sync" aria-labelledby="sync-plan-heading">
  <header class="sync-header">
    <h2 id="sync-plan-heading">What would change</h2>
  </header>

  {#if plan === null}
    <!-- Deliberately not "nothing to do". Nothing is waiting, which is also
         what it looks like a moment after saving, before any reconcile has
         read a repository - and telling somebody their new configuration
         needs no changes would be a claim nothing here has checked. -->
    <p class="sync-empty">
      Nothing waiting. A reconcile runs on a timer and proposes whatever differs.
    </p>
  {:else}
    <p class="sync-note">{planNote}</p>
    <p class="sync-counts">
      {plan.counts.create} to add, {plan.counts.update} to change, {plan.counts.delete} to remove
    </p>

    <ul class="sync-actions">
      {#each plan.actions as action (action.repository + action.kind + action.subject)}
        <li class="sync-action" class:sync-removal={action.operation === 'delete'}>
          <span class="sync-operation">{operationLabel(action)}</span>
          <!-- Which of the sections above this row came from. One list covers
               them all, and "change repository" says nothing on its own. -->
          <span class="sync-kind">{action.kind}</span>
          <span class="sync-subject">{action.subject}</span>
          {#if action.after}
            <span class="sync-after">{action.after}</span>
          {:else if action.before}
            <span class="sync-after">{action.before}</span>
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
      <button
        class="btn btn-signal"
        type="button"
        disabled={approving}
        onclick={() => onApprove(approved.id, approved.digest)}
      >
        {approving ? 'Approving' : 'Apply these changes'}
      </button>
    {/if}
  {/if}
</section>

<style>
  .sync {
    display: grid;
    gap: var(--space-3);
  }

  .sync-header {
    display: grid;
    gap: var(--space-1);
  }

  .sync-lead,
  .sync-note,
  .sync-empty {
    color: var(--text-muted);
    margin: 0;
  }

  .sync-error,
  .sync-notice {
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    color: var(--text-strong);
    margin: 0;
    padding: var(--space-2) var(--space-3);
  }

  .sync-labels,
  .sync-actions {
    display: grid;
    gap: var(--space-1);
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .sync-label,
  .sync-action {
    align-items: baseline;
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
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

  .sync-description,
  .sync-after,
  .sync-counts,
  .sync-kind {
    color: var(--text-muted);
  }

  .sync-counts {
    margin: 0;
  }

  .sync-operation {
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
    min-width: 4.5rem;
  }

  /* A removal is the one row worth finding without reading. It destroys
     something somebody may have made by hand, and it is off unless an operator
     switched it on. */
  .sync-removal .sync-operation {
    color: var(--text-strong);
    font-weight: 600;
  }

  .sync-failure {
    color: var(--text-strong);
    flex-basis: 100%;
  }
</style>
