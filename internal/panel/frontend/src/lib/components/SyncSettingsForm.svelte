<script lang="ts">
  /**
   * What an installation expects its repositories to be set to.
   *
   * Every setting has three states rather than two, and the third one is the
   * important one: a setting nobody configured is left exactly as each
   * repository has it. That is why the controls are the same linked ones the
   * configuration editor uses - the chain says "following each repository", and
   * breaking it is what makes a value a policy.
   *
   * Nothing here is sent until Save. A settings change is one request per
   * repository and the whole of it succeeds or fails together, so a control
   * that saved on every click would send a dozen half-formed policies.
   */
  import { canonicalStringify } from '$lib/preferences-sync';

  import InheritControl from './InheritControl.svelte';

  const {
    stored,
    enabled,
    unreadable,
    unavailable = '',
    problem = null,
    readOnly,
    saving,
    onSave,
  }: {
    stored: Record<string, unknown>;
    enabled: boolean;
    unreadable: boolean;
    /**
     * What this kind needs and the installation has not granted, or empty.
     * Saving is still allowed - configuring before granting is the ordinary
     * order - but a switch that is on while this is set changes nothing, and
     * the plan list below says the same thing it says while waiting for a
     * sweep. This is the only place the difference is visible.
     */
    unavailable?: string;
    /**
     * What went wrong saving these settings, which belongs beside them. The
     * labels form on the same page saves separately and neither waits for the
     * other, so one shared message is one form's failure wiped by the other's
     * next click.
     */
    problem?: string | null;
    readOnly: boolean;
    saving: boolean;
    onSave: (enabled: boolean, document: Record<string, unknown>) => void;
  } = $props();

  type Choice = { value: string; label: string };

  type Field = {
    key: string;
    label: string;
    /** A switch is a boolean in the document; a choice is one of GitHub's words. */
    kind: 'switch' | 'choice';
    options: readonly Choice[];
  };

  const ON = 'on';
  const OFF = 'off';
  const SWITCH: readonly Choice[] = [
    { value: ON, label: 'On' },
    { value: OFF, label: 'Off' },
  ];

  function toggle(key: string, label: string): Field {
    return { key, label, kind: 'switch', options: SWITCH };
  }

  function choice(key: string, label: string, options: readonly Choice[]): Field {
    return { key, label, kind: 'choice', options };
  }

  /**
   * The settings, grouped the way somebody thinks about them rather than the
   * way the endpoint spells them. The keys are GitHub's own, because they are
   * what the stored document holds and what a plan names.
   */
  const GROUPS: readonly {
    id: string;
    title: string;
    note: string;
    fields: readonly Field[];
  }[] = [
    {
      id: 'merging',
      title: 'Merging',
      note: 'How a pull request may be merged, and what happens afterwards.',
      fields: [
        toggle('allow_merge_commit', 'Merge commits'),
        toggle('allow_squash_merge', 'Squash merging'),
        toggle('allow_rebase_merge', 'Rebase merging'),
        toggle('allow_auto_merge', 'Auto-merge'),
        toggle('delete_branch_on_merge', 'Delete the branch on merge'),
        toggle('allow_update_branch', 'Offer to update the branch'),
      ],
    },
    {
      id: 'wording',
      title: 'Commit wording',
      note: 'What a merge or squash commit is called. A repository that does not allow the strategy keeps its own.',
      fields: [
        choice('squash_merge_commit_title', 'Squash commit title', [
          { value: 'PR_TITLE', label: 'PR title' },
          { value: 'COMMIT_OR_PR_TITLE', label: 'Commit or PR' },
        ]),
        choice('squash_merge_commit_message', 'Squash commit message', [
          { value: 'PR_BODY', label: 'PR body' },
          { value: 'COMMIT_MESSAGES', label: 'Commits' },
          { value: 'BLANK', label: 'Blank' },
        ]),
        choice('merge_commit_title', 'Merge commit title', [
          { value: 'PR_TITLE', label: 'PR title' },
          { value: 'MERGE_MESSAGE', label: 'Merge message' },
        ]),
        choice('merge_commit_message', 'Merge commit message', [
          { value: 'PR_BODY', label: 'PR body' },
          { value: 'PR_TITLE', label: 'PR title' },
          { value: 'BLANK', label: 'Blank' },
        ]),
      ],
    },
    {
      id: 'features',
      title: 'Features',
      note: 'Which tabs a repository offers.',
      fields: [
        toggle('has_issues', 'Issues'),
        toggle('has_projects', 'Projects'),
        toggle('has_wiki', 'Wiki'),
        toggle('has_discussions', 'Discussions'),
      ],
    },
    {
      id: 'security',
      title: 'Security',
      note: 'A repository that does not have one of these is left alone rather than asked, and the plan says which.',
      fields: [
        toggle('advanced_security', 'Advanced security'),
        toggle('secret_scanning', 'Secret scanning'),
        toggle('secret_scanning_push_protection', 'Push protection'),
      ],
    },
  ];

  /* The draft is derived from what is saved and then written over as somebody
     edits, so a save landing from anywhere - this form, another tab - reseeds
     it rather than leaving the screen describing a document that is gone. */
  let draft = $derived<Record<string, unknown>>({ ...stored });
  let wanted = $derived(enabled);

  const disabled = $derived(saving || readOnly || unreadable);

  /* Two documents that would be saved the same way compare the same way, which
     is what the preferences sync already needed and already spells. The saved
     side is rendered once per save rather than once per keystroke. */
  const saved = $derived(canonicalStringify(stored));
  const changed = $derived(wanted !== enabled || canonicalStringify(draft) !== saved);

  /** What a control shows: null where nothing is configured. */
  function valueOf(field: Field): string | null {
    const value = draft[field.key];

    if (field.kind === 'switch') {
      return typeof value === 'boolean' ? (value ? ON : OFF) : null;
    }

    return typeof value === 'string' ? value : null;
  }

  function select(field: Field, selection: string): void {
    draft = {
      ...draft,
      [field.key]: field.kind === 'switch' ? selection === ON : selection,
    };
  }

  /** Following each repository again, which is the absence of a key. */
  function restore(field: Field): void {
    const rest = { ...draft };
    delete rest[field.key];
    draft = rest;
  }
</script>

<section class="settings" aria-labelledby="sync-settings-heading">
  <header class="settings-header">
    <h2 id="sync-settings-heading">Repository settings</h2>
    <p class="settings-lead">
      What every repository in this installation should be set to. Anything left following its
      repository is not touched at all, which is not the same as setting it off.
    </p>
  </header>

  {#if problem !== null}
    <p class="settings-error" role="alert">{problem}</p>
  {/if}

  {#if unreadable}
    <p class="settings-error" role="alert">
      These settings are stored in a form this version of Smyklot cannot read, so they are not shown
      and nothing here can be changed. Nothing has been lost.
    </p>
  {/if}

  <!-- Only while the switch is on, because that is when the difference shows:
       a kind nobody asked for is not waiting on anything. Bound to the switch
       rather than to what was saved, so somebody turning it on is told before
       they press save rather than after. -->
  {#if unavailable !== '' && wanted}
    <p class="settings-notice" role="status">
      {unavailable}. Nothing here will be planned or changed until an owner grants it on the
      installation's page on GitHub. The settings below can be saved in the meantime.
    </p>
  {/if}

  <div class="settings-switch">
    <label>
      <input
        type="checkbox"
        checked={wanted}
        {disabled}
        onchange={(event) => (wanted = event.currentTarget.checked)}
      />
      Keep these settings in step across every repository
    </label>
  </div>

  {#each GROUPS as group (group.title)}
    <section class="settings-group" aria-labelledby="sync-group-{group.id}">
      <header class="settings-group-heading">
        <h3 id="sync-group-{group.id}">{group.title}</h3>
        <p class="settings-note">{group.note}</p>
      </header>

      <div class="settings-rows">
        {#each group.fields as field (field.key)}
          <div class="settings-row">
            <span class="settings-label">{field.label}</span>
            <span class="settings-spacer"></span>
            <InheritControl
              label={field.label}
              source="each repository"
              sourcePronoun="them"
              inheritedLabel="whatever it has now"
              value={valueOf(field)}
              options={field.options}
              {disabled}
              onSelect={(selection) => select(field, selection)}
              onRestore={() => restore(field)}
            />
          </div>
        {/each}
      </div>
    </section>
  {/each}

  {#if !readOnly}
    <div class="settings-actions">
      <button
        class="btn btn-signal"
        type="button"
        disabled={disabled || !changed}
        onclick={() => onSave(wanted, draft)}
      >
        {saving ? 'Saving' : 'Save settings'}
      </button>
      {#if changed}
        <p class="settings-note">Nothing is changed on GitHub until a plan is approved.</p>
      {/if}
    </div>
  {/if}
</section>

<style>
  .settings {
    display: grid;
    gap: var(--space-3);
  }

  .settings-header {
    display: grid;
    gap: var(--space-1);
  }

  .settings-lead,
  .settings-note {
    color: var(--text-muted);
    margin: 0;
  }

  .settings-error,
  .settings-notice {
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    color: var(--text-strong);
    margin: 0;
    padding: var(--space-2) var(--space-3);
  }

  .settings-group {
    display: grid;
    gap: var(--space-2);
  }

  .settings-group-heading {
    display: grid;
    gap: var(--space-1);
  }

  .settings-group-heading h3 {
    margin: 0;
  }

  /* The rows the configuration editor uses, because they are the same kind of
     row: a name, and one control that answers followed-or-overridden. */
  .settings-rows {
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
  }

  .settings-row + .settings-row {
    border-top: 1px solid var(--rule);
  }

  /* One line where there is room and two where there is not, so a narrow
     window scrolls down rather than across. */
  .settings-row {
    align-items: center;
    background: var(--strip);
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    min-height: 3.25rem;
    padding: var(--space-2) 0.875rem;
  }

  .settings-row:first-child {
    border-radius: calc(var(--r-ctl) - 1px) calc(var(--r-ctl) - 1px) 0 0;
  }

  .settings-row:last-child {
    border-radius: 0 0 calc(var(--r-ctl) - 1px) calc(var(--r-ctl) - 1px);
  }

  .settings-label {
    font-size: 0.875rem;
    font-weight: 600;
  }

  /* The control sits at the end of its row rather than at the end of the page:
     the spacer collapses when the row wraps, which is what puts the control
     under its own name at a narrow width instead of far off to the right. */
  .settings-spacer {
    flex: 1;
  }

  .settings-actions {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
</style>
