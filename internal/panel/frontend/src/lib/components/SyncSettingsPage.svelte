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
          'A pull request may land squashed to one commit',
        ),
        toggle('allow_merge_commit', 'Merge commits', 'A pull request may land as a merge commit'),
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
          'allow_rebase_merge',
          'Rebase merging',
          'A pull request may land rebased, commit by commit',
        ),
        toggle(
          'allow_update_branch',
          'Offer to update the branch',
          'GitHub offers to bring a behind branch up to date',
        ),
      ],
    },
    {
      id: 'wording',
      title: 'Commit wording',
      note: 'A wording is only sent where its merge strategy is on - anything else is withheld and the plan says so',
      fields: [
        choice('squash_merge_commit_title', 'Squash commit title', [
          { value: 'PR_TITLE', label: 'Pull request title' },
          {
            value: 'COMMIT_OR_PR_TITLE',
            label: "Commit title, or the pull request's when squashing many",
          },
        ]),
        choice('squash_merge_commit_message', 'Squash commit message', [
          { value: 'PR_BODY', label: "The pull request's body" },
          { value: 'COMMIT_MESSAGES', label: 'The commits, listed' },
          { value: 'BLANK', label: 'Blank' },
        ]),
        choice('merge_commit_title', 'Merge commit title', [
          { value: 'PR_TITLE', label: 'Pull request title' },
          { value: 'MERGE_MESSAGE', label: "GitHub's own merge message" },
        ]),
        choice('merge_commit_message', 'Merge commit message', [
          { value: 'PR_BODY', label: "The pull request's body" },
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
      note: 'A repository that does not offer one of these is left alone rather than asked, and the plan names the ones it skipped',
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
and everything unmanaged is one sentence per group with the names as
scent, one press from being managed. The x removes the management,
never "writes the default". "Everything" in the segmented control
turns the unmanaged names into rows of their own.
-->

<script lang="ts">
  import type { SyncConfig, SyncStatus } from '../types';

  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import ClippedLabel from './ClippedLabel.svelte';
  import Popover from './Popover.svelte';
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

  /* ---------- One picker open at a time ---------- */

  let picking = $state<string | null>(null);

  function manage(field: SettingDef): void {
    if (frozen) return;
    picking = null;
    /* A newly managed setting arrives holding something rather than a hole:
       on for a switch, the first word for a choice - the row is right there
       to say otherwise. */
    const value = field.kind === 'switch' ? true : (field.choices[0]?.value ?? '');
    onChangeDocument({ ...stored, [field.key]: value });
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

  function canonical(value: unknown): string {
    try {
      return JSON.stringify(value) ?? 'undefined';
    } catch {
      return String(value);
    }
  }

  function choiceWord(field: SettingDef): string {
    const value = stored[field.key];
    return field.choices.find((held) => held.value === value)?.label ?? String(value ?? '');
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

<div class="view-frame">
  <!-- OPTIONS, not settings. The tree already has a Workspace settings row and a
       Sync status one, and no two rows in it may share a word a reader navigates by -
       which is also what this page is: GitHub's own repository options, held in step. -->
  <PageHeader
    id="sync-settings-heading"
    section="Sync"
    title="Repository options"
    description="Managed options are enforced everywhere; anything unmanaged is left exactly as each repository has it"
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
    <p class="sync-notice" role="alert">
      This workspace's settings are stored in a form this version of Smyklot cannot read, so they
      are not shown and nothing here can be changed. Nothing has been lost.
    </p>
  {/if}

  {#if unavailable !== '' && enabled}
    <p class="sync-notice" role="status">
      {unavailable}. Nothing here will be planned or changed until an owner grants it on the App's
      installation page on GitHub.
    </p>
  {/if}

  <div class="matrix-tools">
    <input
      class="matrix-search"
      type="search"
      placeholder="Search options"
      aria-label="Search options"
      bind:value={query}
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
      <span
        ><strong>Nothing matches</strong> "{query.trim()}" here. Check the spelling, or clear the
        search to see every setting again</span
      >
      <Button onclick={() => (query = '')}>Clear the search</Button>
    </div>
  {:else if managedCount === 0 && show === 'managed'}
    <!-- Nothing is managed, so every group below is a heading over "0 of 6" and
         nothing else. The page said that four times and never once said what it
         meant. The groups stay, because their "Manage another setting" line is
         the way out of this state - the panel names it and points at them. -->
    <div class="state-panel">
      <span
        ><strong>No options are managed here.</strong> Every repository keeps its own GitHub settings
        until one is managed below, and then every syncing repository is held to it</span
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
        {#if rows.length > 0}
          <div class="policy-rows">
            {#each rows as field (field.key)}
              {@const managed = field.key in stored}
              <div
                class="policy-row"
                class:is-unsaved={fieldDirty(field.key)}
                data-unsaved={fieldDirty(field.key) || undefined}
              >
                <span class="setting-say">
                  <span class="setting-name">{field.label}</span>
                  {#if field.why !== ''}
                    <span class="setting-why">{field.why}</span>
                  {/if}
                </span>
                {#if !managed}
                  <span class="policy-value">
                    <span class="setting-unmanaged">From each repository</span>
                  </span>
                  <button
                    class="setting-clear"
                    title="Manage this setting"
                    disabled={frozen}
                    onclick={() => manage(field)}
                  >
                    <Icon name="plus" size="micro" />
                  </button>
                {:else if field.kind === 'switch'}
                  <span class="policy-value">
                    <span class="value-word" class:is-on={stored[field.key] === true}
                      >{stored[field.key] === true ? 'On' : 'Off'}</span
                    >
                    <Switch
                      checked={stored[field.key] === true}
                      label={field.label}
                      disabled={frozen}
                      onToggle={(next) => setValue(field, next)}
                    />
                  </span>
                  <button
                    class="setting-clear"
                    title="Stop managing - repositories keep their own value"
                    disabled={frozen}
                    onclick={() => unmanage(field)}
                  >
                    <Icon name="close" size="micro" />
                  </button>
                {:else}
                  <span class="policy-value">
                    <Popover
                      role="listbox"
                      label="{field.label} choices"
                      align="end"
                      itemSelector=".menu-item"
                    >
                      {#snippet trigger(attributes)}
                        <button
                          {...attributes}
                          class="value-select"
                          type="button"
                          aria-label={`${choiceWord(field)} - ${field.label}`}
                          disabled={frozen}
                        >
                          <span class="t">{choiceWord(field)}</span>
                        </button>
                      {/snippet}
                      <div class="menu-list">
                        {#each field.choices as option (option.value)}
                          <button
                            class="menu-item"
                            role="option"
                            aria-selected={stored[field.key] === option.value}
                            onclick={() => setValue(field, option.value)}
                          >
                            <span class="menu-check">
                              {#if stored[field.key] === option.value}<Icon
                                  name="check"
                                  size="base"
                                />{/if}
                            </span>
                            <ClippedLabel class="mi-label" text={option.label} />
                          </button>
                        {/each}
                      </div>
                    </Popover>
                  </span>
                  <button
                    class="setting-clear"
                    title="Stop managing"
                    disabled={frozen}
                    onclick={() => unmanage(field)}
                  >
                    <Icon name="close" size="micro" />
                  </button>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
        {#if rest.length > 0 && show === 'managed' && query.trim() === ''}
          {@const names = rest.map((field) => field.label)}
          <div
            class="group-rest"
            class:is-open={picking === group.id}
            class:is-unsaved={groupDirty(group)}
            data-unsaved={groupDirty(group) || undefined}
          >
            {#if picking === group.id}
              <span class="rest-say"
                ><span class="rest-count">{rest.length} follow each repository</span> - pick one to manage:</span
              >
              <span class="rest-picks">
                {#each rest as field (field.key)}
                  <button class="add-chip" onclick={() => manage(field)}>
                    <Icon name="plus" size="xs" />
                    <span class="t">{field.label}</span>
                  </button>
                {/each}
                <Button tone="quiet" onclick={() => (picking = null)}>Cancel</Button>
              </span>
            {:else}
              <span class="rest-say"
                ><span class="rest-count">{rest.length} follow each repository</span> - {names.join(
                  ', ',
                )}</span
              >
              <Button tone="quiet" disabled={frozen} onclick={() => (picking = group.id)}>
                {#snippet icon()}<Icon name="plus" size="sm" />{/snippet}
                Manage another setting
              </Button>
            {/if}
          </div>
        {/if}
      </Card>
    {/each}
  </div>
</div>

<svelte:document
  onkeydown={(event) => {
    if (event.key === 'Escape' && picking !== null) picking = null;
  }}
/>

<style>
  .sync-notice {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    font-size: var(--font-size-meta);
    margin: 0 0 var(--space-4);
    padding: var(--space-2) var(--space-3);
  }

  /* ---------- The tools row ---------- */

  /* No margin below: this bar is always a child of the frame, whose gap is the
     distance to what it acts on. A margin here as well made 32px where the
     drawing has 16 - and stating it and standing it down in `app.css` does not
     work from here, because a scoped rule TIES with the shared one and wins on
     source order. */
  .matrix-tools {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
  }

  .matrix-search {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font-size: var(--font-size-control);
    min-block-size: var(--control-height-compact);
    padding-inline: var(--space-3);
    width: 16rem;
  }

  .matrix-search::placeholder {
    color: var(--text-muted);
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
  .card.is-unsaved {
    border-color: color-mix(in srgb, var(--brand-action) 55%, var(--border-subtle));
  }

  .group-rest.is-unsaved {
    background: color-mix(in srgb, var(--brand-action-tint) 45%, transparent);
    box-shadow: inset 2px 0 var(--brand-action);
  }

  /* The unmanaged remainder is a summary line and not a row, so the list still seams into
     it - that line IS the remainder's separator. */
  .policy-rows:has(+ .group-rest) > .policy-row:last-child::after {
    content: '';
    inset-inline: var(--space-2);
  }

  /* Value select: a compact control for the 3+-choice settings. The arrow
     is drawn, not a glyph - two gradient strokes meeting at the chevron. */
  .value-select {
    align-items: center;
    appearance: none;
    background:
      linear-gradient(45deg, transparent 49%, var(--text-secondary) 51%) calc(100% - 14px) 55% / 5px
        5px no-repeat,
      linear-gradient(135deg, var(--text-secondary) 49%, transparent 51%) calc(100% - 9px) 55% / 5px
        5px no-repeat,
      var(--control-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    cursor: pointer;
    display: inline-flex;
    font-size: var(--font-size-control);
    min-block-size: var(--tier-quiet);
    padding: 0 1.5rem 0 var(--space-2);
  }

  .value-select .t {
    text-box: trim-both cap alphabetic;
  }

  /* Open, the trigger wears the pressed ground for as long as its menu
     stands - the same read as every other open trigger. */
  .value-select[data-state='open'] {
    background:
      linear-gradient(45deg, transparent 49%, var(--text-secondary) 51%) calc(100% - 14px) 55% / 5px
        5px no-repeat,
      linear-gradient(135deg, var(--text-secondary) 49%, transparent 51%) calc(100% - 9px) 55% / 5px
        5px no-repeat,
      var(--control-bg-pressed);
  }

  /* ---------- The menu the select opens ---------- */

  .menu-item {
    align-items: center;
    background: none;
    border: 0;
    border-radius: 6px;
    block-size: 32px;
    color: var(--text-primary);
    cursor: pointer;
    display: flex;
    font-size: var(--font-size-control);
    gap: var(--space-2);
    inline-size: 100%;
    padding-inline: var(--space-3);
    text-align: start;
  }

  .menu-item:hover {
    background: var(--interactive-hover-layer);
  }

  /* Keyboard walks the rows the way the pointer does: the row lights, no ring. */
  .menu-item:focus-visible {
    background: var(--interactive-hover-layer);
    outline: none;
  }

  .menu-item:active {
    background: var(--interactive-pressed);
  }

  .menu-check {
    display: inline-flex;
    flex: none;
    inline-size: 16px;
    justify-content: center;
  }

  /* No cap trim here: a menu label is a sentence with descenders, and the
     trim cut them off. The 32px flex row centres it on its own. Anchored
     through the row because the span is ClippedLabel's markup, outside this
     component's scope. */
  .menu-item :global(.mi-label) {
    min-inline-size: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* ---------- The unmanaged remainder ---------- */

  .group-rest {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    /* Bleeds like the rows above it. Its separator is the last row's own
       bottom hairline, so the gaps around that line stay the row rhythm -
       and a card with nothing managed shows no line under its title. */
    margin-inline: calc(var(--space-2) * -1);
    padding: var(--space-2) var(--space-2) 0;
    position: relative;
  }

  .rest-say {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    text-box: trim-both cap alphabetic;
  }

  .rest-count {
    color: var(--text-secondary);
    font-weight: 600;
  }

  .rest-picks {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .add-chip {
    align-items: center;
    background: var(--control-bg);
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-chip);
    color: var(--text-secondary);
    cursor: pointer;
    display: inline-flex;
    font-size: var(--font-size-compact);
    font-weight: 500;
    gap: 0.35rem;
    min-block-size: 30px;
    padding-block: 0;
    padding-inline: 0.7rem;
  }

  .add-chip:hover {
    background: var(--control-bg-hover);
    border-style: solid;
    color: var(--text-primary);
  }

  .add-chip:active {
    background: var(--control-bg-pressed);
  }

  .add-chip .t {
    text-box: trim-both cap alphabetic;
  }

  @media (max-width: 36rem) {
    .matrix-tools {
      align-items: stretch;
      display: grid;
    }

    .matrix-search {
      width: 100%;
    }

    .value-select {
      max-inline-size: 100%;
    }

    .value-select .t {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .group-rest {
      align-items: stretch;
      flex-direction: column;
    }

    .group-rest :global(.btn) {
      align-self: start;
    }
  }
</style>
