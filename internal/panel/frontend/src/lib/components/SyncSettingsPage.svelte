<script module lang="ts">
  /**
   * The catalogue: every setting the panel can manage, grouped the way
   * somebody thinks about them rather than the way the endpoint spells
   * them. The keys are GitHub's own, because they are what the stored
   * document holds and what a plan names.
   */
  export interface SettingChoice {
    value: string;
    label: string;
  }

  export interface SettingDef {
    key: string;
    label: string;
    /** What managing it holds every repository to - empty where the name says it all. */
    why: string;
    kind: 'switch' | 'choice';
    choices: readonly SettingChoice[];
  }

  export interface SettingGroup {
    id: string;
    title: string;
    note: string;
    fields: readonly SettingDef[];
  }

  const toggle = (key: string, label: string, why = ''): SettingDef => ({
    key,
    label,
    why,
    kind: 'switch',
    choices: [],
  });

  const choice = (key: string, label: string, choices: readonly SettingChoice[]): SettingDef => ({
    key,
    label,
    why: '',
    kind: 'choice',
    choices,
  });

  export const SETTING_GROUPS: readonly SettingGroup[] = [
    {
      id: 'merging',
      title: 'Merging',
      note: '',
      fields: [
        toggle(
          'allow_squash_merge',
          'Squash merging',
          'Allow pull requests to merge as one commit',
        ),
        toggle(
          'allow_merge_commit',
          'Merge commits',
          'Allow pull requests to merge with a merge commit',
        ),
        toggle(
          'allow_auto_merge',
          'Auto-merge',
          'Let pull requests merge automatically once requirements pass',
        ),
        toggle(
          'delete_branch_on_merge',
          'Delete the branch on merge',
          'Delete pull request branches after merging',
        ),
        toggle(
          'allow_rebase_merge',
          'Rebase merging',
          'Allow pull request commits to be rebased onto the base branch',
        ),
        toggle(
          'allow_update_branch',
          'Offer to update the branch',
          'Show the option to update pull request branches',
        ),
      ],
    },
    {
      id: 'wording',
      title: 'Commit wording',
      note: 'These options apply only when their merge method is enabled',
      fields: [
        choice('squash_merge_commit_title', 'Squash commit title', [
          { value: 'PR_TITLE', label: 'Pull request title' },
          {
            value: 'COMMIT_OR_PR_TITLE',
            label: 'Commit or pull request title',
          },
        ]),
        choice('squash_merge_commit_message', 'Squash commit message', [
          { value: 'PR_BODY', label: 'Pull request description' },
          { value: 'COMMIT_MESSAGES', label: 'Commit messages' },
          { value: 'BLANK', label: 'Blank' },
        ]),
        choice('merge_commit_title', 'Merge commit title', [
          { value: 'PR_TITLE', label: 'Pull request title' },
          { value: 'MERGE_MESSAGE', label: 'Default merge message' },
        ]),
        choice('merge_commit_message', 'Merge commit message', [
          { value: 'PR_BODY', label: 'Pull request description' },
          { value: 'PR_TITLE', label: 'Pull request title' },
          { value: 'BLANK', label: 'Blank' },
        ]),
      ],
    },
    {
      id: 'features',
      title: 'Features',
      note: '',
      fields: [
        toggle('has_issues', 'Issues'),
        toggle('has_wiki', 'Wiki'),
        toggle('has_projects', 'Projects'),
        toggle('has_discussions', 'Discussions'),
      ],
    },
    {
      id: 'security',
      title: 'Security',
      note: 'Unavailable features are skipped and reported in Sync status',
      fields: [
        toggle('secret_scanning', 'Secret scanning'),
        toggle('secret_scanning_push_protection', 'Push protection'),
        toggle('advanced_security', 'Advanced security'),
      ],
    },
  ];

  /**
   * Every key the catalogue knows, exported for the overview's
   * "9 of 17 managed" summary.
   */
  export const SETTINGS_FIELD_KEYS: readonly string[] = SETTING_GROUPS.flatMap((group) =>
    group.fields.map((field) => field.key),
  );
  export const SETTINGS_FIELD_TOTAL = SETTINGS_FIELD_KEYS.length;

  const SETTING_NAMES: ReadonlyMap<string, string> = new Map(
    SETTING_GROUPS.flatMap((group) => group.fields.map((field) => [field.key, field.label])),
  );

  /**
   * What to call a setting, in the words this panel already calls it.
   *
   * A plan names GitHub's keys - the comment at the top of this catalogue says
   * so - and `allow_squash_merge` is not what the form above calls that switch.
   * One vocabulary, so the row a plan draws and the switch somebody flips say
   * the same thing.
   *
   * The key itself for a setting this version has no name for, spaced rather
   * than left in snake case: a key the panel cannot name is still a key the
   * reader can match against GitHub's own documentation.
   */
  export function settingName(key: string): string {
    return SETTING_NAMES.get(key) ?? key.replaceAll('_', ' ');
  }
</script>

<!--
@component
The settings page is the policy: only managed settings render as rows,
and each group lists the options still owned by individual repositories.
A shared picker adds one without expanding the page. The x removes management,
never "writes the default". "All" in the segmented control
turns the unmanaged names into rows of their own.
-->

<script lang="ts">
  import { tick } from 'svelte';
  import { revealControl } from '../reveal-control';
  import type { SyncConfig, SyncStatus } from '../types';

  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import FormError from './FormError.svelte';
  import IconButton from './IconButton.svelte';
  import Select from './Select.svelte';
  import Callout from './Callout.svelte';
  import PageHeader from './PageHeader.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import Switch from './Switch.svelte';
  import SyncKindFacts, { syncSwitchLabel, syncSwitchWord } from './SyncKindFacts.svelte';

  const {
    config,
    savedDocument = {},
    readOnly,
    problem = null,
    syncStatus = null,
    nowMs,
    onToggleEnabled,
    onChangeDocument,
    dirtyEnabled = false,
    dirtyDocument = false,
  }: {
    config: SyncConfig | null;
    savedDocument?: Record<string, unknown>;
    readOnly: boolean;
    problem?: string | null;
    /** The fleet, for how far this kind reaches. */
    syncStatus?: SyncStatus | null;
    nowMs: number;
    onToggleEnabled: (enabled: boolean) => void;
    onChangeDocument: (document: Record<string, unknown>) => void;
    dirtyEnabled?: boolean;
    dirtyDocument?: boolean;
  } = $props();

  const stored = $derived(config?.document ?? {});
  const enabled = $derived(config?.enabled ?? false);
  const unreadable = $derived(config?.unreadable === true);
  const unavailable = $derived(config?.unavailable ?? '');
  const frozen = $derived(readOnly || unreadable || config === null);

  const managedCount = $derived(SETTINGS_FIELD_KEYS.filter((key) => key in stored).length);

  /* ---------- The tools: search narrows, the seg widens ---------- */

  let query = $state('');
  let show = $state<'managed' | 'everything'>('managed');

  const matches = (field: SettingDef): boolean =>
    query.trim() === '' || field.label.toLowerCase().includes(query.trim().toLowerCase());

  let frame = $state<HTMLDivElement | null>(null);

  async function manage(field: SettingDef): Promise<void> {
    if (frozen) return;
    /* A newly managed setting arrives holding something rather than a hole:
       on for a switch, the first word for a choice - the row is right there
       to say otherwise. */
    const value = field.kind === 'switch' ? true : (field.choices[0]?.value ?? '');
    onChangeDocument({ ...stored, [field.key]: value });
    await tick();
    if (frozen) return;
    const row = frame?.querySelector<HTMLElement>(`[data-option="${field.key}"]`);
    const control = row?.querySelector<HTMLElement>('input:not([type="hidden"]), button');
    if (control) {
      control.focus({ preventScroll: true });
      revealControl(row ?? control);
    }
  }

  function unmanage(field: SettingDef): void {
    if (frozen) return;
    const next = { ...stored };
    delete next[field.key];
    onChangeDocument(next);
  }

  function setValue(field: SettingDef, value: unknown): void {
    if (frozen) return;
    onChangeDocument({ ...stored, [field.key]: value });
  }

  function fieldDirty(key: string): boolean {
    return dirtyDocument && canonical(stored[key]) !== canonical(savedDocument[key]);
  }

  function groupDirty(group: SettingGroup): boolean {
    return group.fields.some(({ key }) => fieldDirty(key));
  }

  function remainingChanged(group: SettingGroup): boolean {
    return dirtyDocument && group.fields.some(({ key }) => key in stored !== key in savedDocument);
  }

  function canonical(value: unknown): string {
    try {
      return JSON.stringify(value) ?? 'undefined';
    } catch {
      return String(value);
    }
  }

  function groupRows(group: SettingGroup): SettingDef[] {
    return group.fields.filter(
      (field) => matches(field) && (show === 'everything' || field.key in stored),
    );
  }

  function groupRest(group: SettingGroup): SettingDef[] {
    return group.fields.filter((field) => !(field.key in stored));
  }

  /* A searched-away group stands down whole; an empty rest line does too. */
  const visibleGroups = $derived(
    SETTING_GROUPS.filter((group) => groupRows(group).length > 0 || query.trim() === ''),
  );
</script>

<div class="view-frame" bind:this={frame}>
  <!-- OPTIONS, not settings. The tree already has a Workspace settings row and a
       Sync status one, and no two rows in it may share a word a reader navigates by -
       which is also what this page is: GitHub's own repository options, held in step. -->
  <PageHeader
    id="sync-settings-heading"
    section="Sync"
    title="Repository options"
    description="Managed options apply to all syncing repositories · Unmanaged options keep each repository’s value"
    statusUnsaved={dirtyEnabled}
  >
    {#snippet status()}
      <SyncKindFacts
        kind="settings"
        {enabled}
        status={syncStatus}
        updatedBy={config?.updated_by ?? ''}
        updatedAt={config?.updated_at ?? ''}
        {nowMs}
      />
      <Switch
        checked={enabled}
        label={syncSwitchLabel('settings', enabled)}
        word={syncSwitchWord(enabled)}
        disabled={frozen}
        onToggle={onToggleEnabled}
      />
    {/snippet}
  </PageHeader>

  {#if problem !== null}
    <FormError message={problem} />
  {/if}

  {#if unreadable}
    <Callout role="alert"
      ><p>
        This version of Smyklot cannot read the saved repository options · Editing is unavailable
      </p></Callout
    >
  {/if}

  {#if unavailable !== '' && enabled}
    <Callout role="status"
      ><p>
        {unavailable.replace(/[.]+$/u, '')} · A workspace owner must grant the required permissions in
        Smyklot's GitHub installation settings before these options can sync
      </p></Callout
    >
  {/if}

  <div class="matrix-tools">
    <SearchField
      label="Search options"
      placeholder="Search options"
      value={query}
      onInput={(value) => (query = value)}
    />
    <SegmentedControl
      name="settings-show"
      label="Show"
      options={[
        { value: 'managed', label: 'Managed', badge: managedCount },
        { value: 'everything', label: 'All', badge: SETTINGS_FIELD_TOTAL },
      ]}
      value={show}
      onSelect={(value) => (show = value as 'managed' | 'everything')}
    />
  </div>

  {#if visibleGroups.length === 0}
    <!-- The one honest answer to a search that matches nothing: the groups
         stand down whole, so without this the page went silently blank. It is a
         `.state-panel` like every other nothing-to-show, and it carries the way
         back - it used to be a muted line offering none. -->
    <div class="state-panel">
      <span><strong>No matching options</strong> · Try another name or clear the search</span>
      <Button onclick={() => (query = '')}>Clear the search</Button>
    </div>
  {:else if managedCount === 0 && show === 'managed'}
    <!-- Nothing is managed, so every group below is a heading over "0 of 6" and
         nothing else. The page said that four times and never once said what it
         meant. The groups stay, because their "Manage an option" picker is
         the way out of this state - the panel names it and points at them. -->
    <div class="state-panel">
      <span
        ><strong>No managed settings yet</strong> · Choose which GitHub settings Smyklot should manage
        across syncing repositories</span
      >
      <Button onclick={() => (show = 'everything')}>Show every option</Button>
    </div>
  {/if}

  <div class="setting-groups card-stack">
    {#each visibleGroups as group (group.id)}
      {@const rows = groupRows(group)}
      {@const rest = groupRest(group)}
      <Card unsaved={groupDirty(group)} labelledby="settings-group-{group.id}">
        <div class="card-head">
          <h2 class="card-title" id="settings-group-{group.id}">{group.title}</h2>
          <span class="card-meta"
            >{group.fields.length - rest.length} of {group.fields.length} managed</span
          >
        </div>
        {#if group.note !== ''}
          <p class="group-note">{group.note}</p>
        {/if}
        <div class="policy-rows">
          {#each rows as field (field.key)}
            {@const managed = field.key in stored}
            <div
              class="policy-row"
              data-option={field.key}
              class:is-unsaved={fieldDirty(field.key)}
              data-unsaved={fieldDirty(field.key) || undefined}
            >
              <span class="setting-say">
                <span class="setting-name">{field.label}</span>
                {#if field.why !== ''}<span class="setting-why">{field.why}</span>{/if}
              </span>
              <span class="policy-value option-controls">
                {#if !managed}
                  <span class="setting-unmanaged">From each repository</span>
                  <IconButton
                    toolbar
                    icon="plus"
                    label="Manage {field.label}"
                    disabled={frozen}
                    onclick={() => manage(field)}
                  />
                {:else}
                  {#if field.kind === 'switch'}
                    <Switch
                      checked={stored[field.key] === true}
                      label={field.label}
                      disabled={frozen}
                      onToggle={(next) => setValue(field, next)}
                    />
                  {:else}
                    <Select
                      value={String(stored[field.key] ?? '')}
                      options={field.choices}
                      aria-label={field.label}
                      disabled={frozen}
                      onValueChange={(value) => setValue(field, value)}
                    />
                  {/if}
                  <IconButton
                    toolbar
                    icon="close"
                    label="Stop managing {field.label}"
                    disabled={frozen}
                    onclick={() => unmanage(field)}
                  />
                {/if}
              </span>
            </div>
          {/each}
          {#if rest.length > 0 && show === 'managed' && query.trim() === ''}
            <div
              class="policy-row group-rest"
              class:is-unsaved={remainingChanged(group)}
              data-unsaved={remainingChanged(group) || undefined}
            >
              <span class="setting-say rest-say">
                <span class="setting-name"
                  >{rest.length}
                  {rest.length === 1 ? 'option follows' : 'options follow'} each repository</span
                >
                <span class="setting-why">{rest.map((field) => field.label).join(', ')}</span>
              </span>
              <span class="policy-value">
                {#key rest.map((field) => field.key).join(',')}
                  <Select
                    value={undefined}
                    placeholder="Manage an option"
                    disabled={frozen}
                    aria-label="Manage an option in {group.title}"
                    options={rest.map((field) => ({ value: field.key, label: field.label }))}
                    onValueChange={(key) => {
                      const field = rest.find((candidate) => candidate.key === key);
                      if (field) manage(field);
                    }}
                  />
                {/key}
              </span>
            </div>
          {/if}
        </div>
      </Card>
    {/each}
  </div>
</div>

<style>
  /* ---------- The tools row ---------- */

  /* No margin below: this bar is always a child of the frame, whose gap is the
     distance to what it acts on. A margin here as well made 32px where the
     drawing has 16 - and stating it and standing it down in `app.css` does not
     work from here, because a scoped rule TIES with the shared one and wins on
     source order. */
  .matrix-tools {
    --search-field-width: 16rem;
    --search-field-flex: 0 1 16rem;
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
  }

  /* ---------- Policy groups: the page is the policy ---------- */

  /* The no-match answer is a `.state-panel` from the shared sheet now, so the
     only thing left to say here is where it sits in the page's rhythm. */
  .view-frame > .state-panel {
    margin-block-end: var(--space-4);
  }

  /* The head, the title and the tally beside it are the sheet's now - `card-head`,
     `card-title`, `card-meta` - so what is left here is the one thing only this page
     has: a card holding a change nobody has saved. */

  /* The value and its remove action are one unit, including when the row wraps. */
  .option-controls {
    flex-wrap: nowrap;
    min-inline-size: 0;
    /* An indivisible picker/action pair can use the full line after wrapping. */
    max-inline-size: 100%;
  }

  /* This fixed switch/action pair leaves the rest of the row for its copy. */
  .policy-row:has(> .option-controls > :global(.switch)) > .setting-say {
    flex-basis: 0;
    min-inline-size: 0;
  }

  @media (max-width: 36rem) {
    .matrix-tools {
      --search-field-width: 100%;
      align-items: stretch;
      display: grid;
    }
  }
</style>
