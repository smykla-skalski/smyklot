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
  import InheritControl from './InheritControl.svelte';

  const {
    stored,
    enabled,
    unreadable,
    readOnly,
    saving,
    onSave,
  }: {
    stored: Record<string, unknown>;
    enabled: boolean;
    unreadable: boolean;
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
  const GROUPS: readonly { title: string; note: string; fields: readonly Field[] }[] = [
    {
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
  const changed = $derived(wanted !== enabled || canonical(draft) !== canonical(stored));

  /** Two documents that would be saved the same way, compared the same way. */
  function canonical(document: Record<string, unknown>): string {
    const keys = Object.keys(document).sort();

    return JSON.stringify(keys.map((key) => [key, document[key]]));
  }

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

  {#if unreadable}
    <p class="settings-error" role="alert">
      These settings are stored in a form this version of Smyklot cannot read, so they are not shown
      and nothing here can be changed. Nothing has been lost.
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
    <fieldset class="settings-group">
      <legend>{group.title}</legend>
      <p class="settings-note">{group.note}</p>

      {#each group.fields as field (field.key)}
        <div class="settings-row">
          <span class="settings-label">{field.label}</span>
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
    </fieldset>
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

  .settings-error {
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    color: var(--text-strong);
    margin: 0;
    padding: var(--space-2) var(--space-3);
  }

  .settings-group {
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-control);
    display: grid;
    gap: var(--space-2);
    margin: 0;
    padding: var(--space-3);
  }

  .settings-group legend {
    color: var(--text-strong);
    font-weight: 600;
    padding-inline: var(--space-1);
  }

  /* The label and its control sit on one line where there is room and stack
     where there is not, so a narrow window scrolls down rather than across. */
  .settings-row {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    justify-content: space-between;
  }

  .settings-actions {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
</style>
