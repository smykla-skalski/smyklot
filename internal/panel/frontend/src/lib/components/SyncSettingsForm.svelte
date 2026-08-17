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
  import { canonicalStringify } from '#lib/preferences-sync.js';

  import InheritControl from './InheritControl.svelte';
  import SyncDocumentForm from './SyncDocumentForm.svelte';

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
    unavailable?: string;
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

<SyncDocumentForm
  heading="Repository settings"
  noun="settings"
  lead="What every repository in this installation should be set to. Anything left following its
        repository is not touched at all, which is not the same as setting it off"
  enabled={wanted}
  {unreadable}
  {unavailable}
  {problem}
  {readOnly}
  {saving}
  {changed}
  {disabled}
  onToggle={(value) => (wanted = value)}
  onSave={() => onSave(wanted, draft)}
>
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
</SyncDocumentForm>

<style>
  .settings-note {
    color: var(--dim);
    font-size: var(--font-size-meta);
    margin: 0;
    max-width: 60ch;
  }

  /* The configuration editor's group rhythm: an eyebrow naming the group, a line
     under it saying what it covers, and the rows. Written to the same numbers so
     the two pages read as one - `ConfigEditor` is where they are decided. */
  .settings-group {
    margin-top: 1.375rem;
  }

  .settings-group-heading {
    margin: 0 0.125rem 0.625rem;
  }

  .settings-group-heading h3 {
    color: var(--brand-action);
    font-size: var(--font-size-micro);
    font-weight: 700;
    letter-spacing: 0.1em;
    margin: 0;
    text-transform: uppercase;
  }

  .settings-group-heading p {
    margin: 0.1875rem 0 0;
  }

  /* Hairlines between the rows and no box around them, because the plate is
     already the box. Bordered, this list read as a second card inside the
     first. */
  .settings-row + .settings-row {
    border-top: 1px solid var(--rule);
  }

  /* One line where there is room and two where there is not, so a narrow
     window scrolls down rather than across. */
  .settings-row {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    padding-block: 0.7rem;
  }

  .settings-rows > .settings-row:first-child {
    padding-top: 0.15rem;
  }

  .settings-rows > .settings-row:last-child {
    padding-bottom: 0.15rem;
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
</style>
