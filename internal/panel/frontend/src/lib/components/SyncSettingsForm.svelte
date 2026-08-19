<script module lang="ts">
  /**
   * The settings this installation can decide, and how each is said.
   *
   * At module scope because the count is a fact about the page rather than
   * about one instance of it: the overview's card says "9 of 17 managed", and a
   * second copy of the list to count is the copy that goes stale.
   */
  type Choice = { value: string; label: string };

  /**
   * A switch carries no vocabulary of its own. It is a boolean in the document
   * and renders as a Switch, so the two words beside it are the control's, not
   * the setting's - a list here would be a copy nothing reads.
   */
  export type SettingField = {
    key: string;
    label: string;
    /** What it means, in the reader's words rather than GitHub's. */
    why?: string;
  } & ({ kind: 'switch' } | { kind: 'choice'; options: readonly Choice[] });

  function toggle(key: string, label: string, why?: string): SettingField {
    return { key, label, why, kind: 'switch' };
  }

  function choice(key: string, label: string, options: readonly Choice[]): SettingField {
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
    /** Only where the rows cannot say it themselves; each carries its own why. */
    note?: string;
    fields: readonly SettingField[];
  }[] = [
    {
      id: 'merging',
      title: 'Merging',
      fields: [
        toggle('allow_merge_commit', 'Merge commits', 'A pull request may land as a merge commit'),
        toggle(
          'allow_squash_merge',
          'Squash merging',
          'A pull request may land squashed to one commit',
        ),
        toggle(
          'allow_rebase_merge',
          'Rebase merging',
          'A pull request may land rebased onto the base',
        ),
        toggle(
          'allow_auto_merge',
          'Auto-merge',
          'A pull request may be set to merge itself once checks pass',
        ),
        toggle(
          'delete_branch_on_merge',
          'Delete the branch on merge',
          'The head branch goes as soon as its pull request lands',
        ),
        toggle(
          'allow_update_branch',
          'Offer to update the branch',
          'A pull request behind its base offers a button to catch up',
        ),
      ],
    },
    {
      id: 'wording',
      title: 'Commit wording',
      note: 'A wording is only sent where its merge strategy is on — anything else is withheld and the plan says so.',
      fields: [
        choice('squash_merge_commit_title', 'Squash commit title', [
          { value: 'PR_TITLE', label: 'Pull request title' },
          { value: 'COMMIT_OR_PR_TITLE', label: "Commit title, or the pull request's" },
        ]),
        choice('squash_merge_commit_message', 'Squash commit message', [
          { value: 'PR_BODY', label: 'Pull request body' },
          { value: 'COMMIT_MESSAGES', label: 'The commits, listed' },
          { value: 'BLANK', label: 'Blank' },
        ]),
        choice('merge_commit_title', 'Merge commit title', [
          { value: 'PR_TITLE', label: 'Pull request title' },
          { value: 'MERGE_MESSAGE', label: 'Merge message' },
        ]),
        choice('merge_commit_message', 'Merge commit message', [
          { value: 'PR_BODY', label: 'Pull request body' },
          { value: 'PR_TITLE', label: 'Pull request title' },
          { value: 'BLANK', label: 'Blank' },
        ]),
      ],
    },
    {
      id: 'features',
      title: 'Features',
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
      note: 'A repository that does not offer one of these is left alone rather than asked, and the plan names which.',
      fields: [
        toggle('advanced_security', 'Advanced security'),
        toggle('secret_scanning', 'Secret scanning'),
        toggle('secret_scanning_push_protection', 'Push protection'),
      ],
    },
  ];

  /** Every key a repository setting can be managed under. */
  export const SETTING_KEYS: readonly string[] = GROUPS.flatMap((group) =>
    group.fields.map((field) => field.key),
  );
</script>

<script lang="ts">
  /**
   * What an installation expects its repositories to be set to.
   *
   * The page is the policy. A settings page listing every switch GitHub has
   * makes a reader work out which of seventeen rows this installation actually
   * decides, and that answer - usually nine - is the only thing on the page
   * worth knowing. So a managed setting is a row and the rest is one sentence
   * per group naming them: enough to answer "is X managed?" without turning the
   * page back into a form.
   *
   * A setting has three states and the third one is the point: one nobody
   * managed is left exactly as each repository has it, which is not the same as
   * setting it off. The × on a row removes the management and never writes a
   * value.
   *
   * Every control here lands at once. The document is the panel's own record of
   * what should be true, and nothing reaches GitHub until a plan is approved -
   * so there is nothing for a Save button to hold back, and a page of pending
   * edits is a page whose switches disagree with the plan beside them.
   */
  import Button from './Button.svelte';
  import PolicyGroup from './PolicyGroup.svelte';
  import PolicyRow from './PolicyRow.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import Select from './Select.svelte';
  import Switch from './Switch.svelte';
  import SyncKindHead from './SyncKindHead.svelte';

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

  const disabled = $derived(saving || readOnly || unreadable);

  /** Which group's picker is open, or null. One at a time, like a menu. */
  let picking = $state<string | null>(null);
  let query = $state('');
  let show = $state<'managed' | 'all'>('managed');

  const managed = $derived(SETTING_KEYS.filter((key) => stored[key] !== undefined).length);

  function isManaged(field: SettingField): boolean {
    return stored[field.key] !== undefined;
  }

  /** What a control shows, as the word a reader checks the column against. */
  function wordOf(field: SettingField): string | undefined {
    const value = stored[field.key];
    if (field.kind === 'switch') return value === true ? 'On' : 'Off';

    return field.options.find((option) => option.value === value)?.label;
  }

  function chosen(field: Extract<SettingField, { kind: 'choice' }>): string {
    const value = stored[field.key];

    return typeof value === 'string' ? value : (field.options[0]?.value ?? '');
  }

  /* The whole stored document rather than the keys with controls: anything a
     newer version of the service wrote travels back untouched, rather than
     being dropped by a browser running an older build of this page. */
  function write(key: string, value: unknown): void {
    onSave(enabled, { ...stored, [key]: value });
  }

  /** Following each repository again, which is the absence of a key. */
  function stopManaging(key: string): void {
    const rest = { ...stored };
    delete rest[key];
    onSave(enabled, rest);
  }

  /** Managing one starts it at the value GitHub itself starts a repository on. */
  function manage(field: SettingField): void {
    picking = null;
    write(field.key, field.kind === 'switch' ? true : (field.options[0]?.value ?? ''));
  }

  /* A search reaches settings that are not managed, because finding one is how
     somebody comes to manage it - so it searches the whole vocabulary and the
     Managed/Everything filter decides what an empty query shows. */
  function matches(field: SettingField): boolean {
    if (query === '') return show === 'all' || isManaged(field);

    return field.label.toLowerCase().includes(query.trim().toLowerCase());
  }

  const groups = $derived(
    GROUPS.map((group) => {
      const shown = group.fields.filter(matches);

      return {
        ...group,
        shown,
        managed: group.fields.filter(isManaged).length,
        unmanaged: group.fields.filter((field) => !isManaged(field)),
      };
    }).filter(
      (group) => group.shown.length > 0 || (query === '' && group.managed < group.fields.length),
    ),
  );
</script>

<SyncKindHead
  title="Repository settings"
  lead="Manage a setting and every repository is held to its value. Anything unmanaged is left exactly as each repository has it, which is not the same as setting it off"
  noun="settings"
  {enabled}
  {unreadable}
  {unavailable}
  {problem}
  {readOnly}
  {saving}
  onToggle={(next) => onSave(next, stored)}
/>

<div class="settings-tools">
  <SearchField
    label="Search settings"
    placeholder="Search settings"
    value={query}
    onInput={(next) => (query = next)}
  />
  <!-- An instant filter over what is already on screen, which is the one thing
       a segmented control is for: it saves nothing and the page never asks. -->
  <SegmentedControl
    name="sync-settings-show"
    label="Show"
    compact
    options={[
      { value: 'managed', label: 'Managed', badge: managed },
      { value: 'all', label: 'Everything', badge: SETTING_KEYS.length },
    ]}
    value={show}
    onSelect={(next) => (show = next as 'managed' | 'all')}
  />
</div>

<div class="setting-groups">
  {#each groups as group (group.id)}
    <PolicyGroup
      name={group.title}
      note={group.note}
      managed={group.managed}
      total={group.fields.length}
      unmanaged={group.unmanaged}
      picking={picking === group.id}
      {disabled}
      onManage={disabled || readOnly ? undefined : () => (picking = group.id)}
      onPick={(key) => {
        const field = group.unmanaged.find((one) => one.key === key);
        if (field !== undefined) manage(field);
      }}
      onCancel={() => (picking = null)}
    >
      {#each group.shown as field (field.key)}
        {#if isManaged(field)}
          <PolicyRow
            name={field.label}
            why={field.why}
            value={field.kind === 'switch' ? wordOf(field) : undefined}
            onStopManaging={readOnly ? undefined : () => stopManaging(field.key)}
          >
            {#snippet control()}
              {#if field.kind === 'switch'}
                <Switch
                  checked={stored[field.key] === true}
                  ariaLabel={field.label}
                  {disabled}
                  onChange={(next) => write(field.key, next)}
                />
              {:else}
                <Select
                  aria-label={field.label}
                  options={field.options}
                  value={chosen(field)}
                  {disabled}
                  onchange={(event) => write(field.key, event.currentTarget.value)}
                />
              {/if}
            {/snippet}
          </PolicyRow>
        {:else}
          <!-- Only reachable through Everything or a search: an unmanaged
                 setting is a thing to start managing, not a value to read. -->
          <PolicyRow name={field.label} why={field.why} value="Follows">
            {#snippet control()}
              <Button tone="quiet" {disabled} onclick={() => manage(field)}>Manage</Button>
            {/snippet}
          </PolicyRow>
        {/if}
      {/each}
    </PolicyGroup>
  {/each}

  {#if groups.length === 0}
    <p class="empty-note settings-empty">Nothing here matches that</p>
  {/if}
</div>

<style>
  /* The mock's own toolbar: the search at 16rem on one side and the filter on
     the other, rather than a search stretched across the row. */
  .settings-tools {
    --search-field-flex: 0 1 16rem;
    --search-field-width: 16rem;

    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    justify-content: space-between;
    margin-bottom: var(--space-4);
  }

  .setting-groups {
    display: grid;
    gap: var(--space-4);
  }
</style>
