<!--
@component
The shared files list: what every repository should carry. Each template
is a named object one press from its own page; the add flow is a path
field with fuzzy suggestions from what the organization's repositories
already hold - the index ships once, matching costs no requests.
-->

<script lang="ts">
  import { Command } from 'bits-ui';

  import { formatRelative } from '../format';
  import { rankPaths, type PathMatch } from '../pathfinder';
  import { receipts } from '../receipts.svelte';
  import type {
    SyncConfig,
    SyncFile,
    SyncFileMergeEntry,
    SyncFilesContext,
    SyncPlan,
    SyncStatus,
  } from '../types';

  import Card from './Card.svelte';
  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import Popover from './Popover.svelte';
  import Switch from './Switch.svelte';
  import SyncKindFacts, { syncSwitchLabel, syncSwitchWord } from './SyncKindFacts.svelte';

  const {
    config,
    savedDocument = {},
    context,
    plan,
    syncStatus,
    nowMs,
    readOnly,
    problem = null,
    fileHref,
    onOpenFile,
    onToggleEnabled,
    onChangeDocument,
    dirtyEnabled = false,
    dirtyDocument = false,
  }: {
    config: SyncConfig | null;
    savedDocument?: Record<string, unknown>;
    context: SyncFilesContext | null;
    plan: SyncPlan | null;
    /** The fleet, for how far this kind reaches and which repositories refused. */
    syncStatus: SyncStatus | null;
    nowMs: number;
    readOnly: boolean;
    problem?: string | null;
    fileHref: (path: string) => string;
    onOpenFile: (path: string) => void;
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

  const files = $derived(Array.isArray(stored.files) ? (stored.files as SyncFile[]) : []);
  const retired = $derived(Array.isArray(stored.retired) ? (stored.retired as string[]) : []);
  const excludes = $derived(Array.isArray(stored.excludes) ? (stored.excludes as string[]) : []);
  const savedFiles = $derived(
    Array.isArray(savedDocument.files) ? (savedDocument.files as SyncFile[]) : [],
  );

  function stage(change: Partial<Record<string, unknown>>): void {
    if (frozen) return;
    onChangeDocument({ ...stored, ...change });
  }

  function same(left: unknown, right: unknown): boolean {
    try {
      return JSON.stringify(left) === JSON.stringify(right);
    } catch {
      return false;
    }
  }

  function fileDirty(file: SyncFile): boolean {
    return (
      dirtyDocument &&
      !same(
        file,
        savedFiles.find((saved) => saved.path === file.path),
      )
    );
  }

  /* ---------- One row's words ---------- */

  const mergesOf = (path: string): SyncFileMergeEntry[] =>
    (context?.merges ?? []).filter((entry) => entry.path === path);

  function adjustersWord(path: string): string {
    const count = mergesOf(path).length;
    if (count === 0) return 'no adjustments';
    return `${count} ${count === 1 ? 'repository adjusts' : 'repositories adjust'} it`;
  }

  function updatedWord(file: SyncFile & { updated_at?: string }): string {
    const at = file.updated_at ?? config?.updated_at;
    return at === undefined ? '' : ` · updated ${formatRelative(at, nowMs)}`;
  }

  function differs(path: string): number {
    const actions = plan?.actions ?? [];
    return new Set(
      actions
        .filter((action) => action.kind === 'files' && action.subject === path)
        .map((action) => action.repository),
    ).size;
  }

  function refusals(path: string): number {
    const rows = syncStatus?.repositories ?? [];
    return rows.filter(
      (row) => row.cells.files.state === 'refused' && (row.reason ?? '').includes(path),
    ).length;
  }

  /* ---------- The finder ----------
     The palette shape every fuzzy picker converged on - an ARIA combobox in
     a popover, input on top, ranked list, key legend below - built on
     bits-ui's Command (the cmdk pattern) so focus, aria-activedescendant
     and the arrow/Enter contract come from the library rather than being
     hand-rolled here. Ranking stays ours: `rankPaths` is path-aware in a
     way a generic command score is not, so filtering is turned off and the
     list is rendered from it directly. */

  let addOpen = $state(false);
  let query = $state('');

  const ranked: PathMatch[] = $derived(rankPaths(context?.known_paths ?? [], query.trim()));
  const cleanQuery = $derived(query.trim());
  /** A path no repository holds is still a legal ask - the explicit create
      row is how a combobox says so out loud. */
  const startable = $derived(
    cleanQuery !== '' && !ranked.some((match) => match.path === cleanQuery),
  );

  /** Choosing a path manages it: an existing template opens, a new one is born empty. */
  function choose(path: string): void {
    const clean = path.trim();
    if (clean === '') return;
    addOpen = false;
    if (!files.some((file) => file.path === clean)) {
      stage({ files: [...files, { path: clean, content: '' }] });
      receipts.say(
        `${clean} is shared now - the next sync opens a pull request in every syncing repository`,
      );
    }
    onOpenFile(clean);
  }

  /** A path split for the marks: directory dimmed, basename full ink. */
  function markedParts(match: PathMatch): Array<{ text: string; mark: boolean; base: boolean }> {
    const cut = match.path.lastIndexOf('/') + 1;
    return [...match.path].map((ch, at) => ({
      text: ch,
      mark: match.positions.includes(at),
      base: at >= cut,
    }));
  }

  function open(event: MouseEvent, path: string): void {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
      return;
    }
    event.preventDefault();
    onOpenFile(path);
  }
</script>

<div class="view-frame">
  <PageHeader
    id="sync-files-heading"
    section="Sync"
    title="Shared files"
    description="Shared templates reach repositories through pull requests they can merge or close"
    statusUnsaved={dirtyEnabled}
  >
    {#snippet actions()}
      {@render addFile()}
    {/snippet}
    {#snippet status()}
      <SyncKindFacts
        kind="files"
        {enabled}
        status={syncStatus}
        updatedBy={config?.updated_by ?? ''}
        updatedAt={config?.updated_at ?? ''}
        {nowMs}
      />
      <Switch
        checked={enabled}
        label={syncSwitchLabel('files', enabled)}
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
      This workspace's files are stored in a form this version of Smyklot cannot read, so they are
      not shown and nothing here can be changed. Nothing has been lost.
    </p>
  {/if}

  {#if unavailable !== '' && enabled}
    <p class="sync-notice" role="status">
      {unavailable}. Nothing here will be planned or changed until an owner grants it on the App's
      installation page on GitHub.
    </p>
  {/if}

  {#snippet addFile()}
    <Popover
      bind:open={addOpen}
      role="dialog"
      label="Add a file"
      align="start"
      focusSelector=".finder-search input"
      onopen={() => (query = '')}
    >
      {#snippet trigger(attributes)}
        <!-- A raw .btn: Button's own props collide with the trigger's
               spread attributes, the way every other Popover trigger here
               already found. -->
        <button {...attributes} type="button" class="btn add-file" disabled={frozen}>
          <Icon name="plus" size="sm" />
          <span class="button-label">Add a file</span>
        </button>
      {/snippet}
      <Command.Root
        class="finder-palette"
        label="Path of the file to manage"
        shouldFilter={false}
        loop
      >
        <div class="menu-search finder-search">
          <Icon name="search" size="xs" />
          <Command.Input
            bind:value={query}
            placeholder="renovate.json, or a path no repository has yet"
            spellcheck="false"
            autocomplete="off"
          />
        </div>
        <div class="finder-scope">
          <span>Paths across this workspace</span>
          <span
            >{(context?.known_paths ?? []).length.toLocaleString('en-US')} known · {context?.repositories ??
              0} repositories</span
          >
        </div>
        <Command.List class="finder-list">
          <Command.Viewport>
            {#each ranked as match (match.path)}
              <Command.Item
                class="finder-opt"
                value={match.path}
                onSelect={() => choose(match.path)}
              >
                <span class="finder-path">
                  {#each markedParts(match) as part, index (index)}<span
                      class:dir={!part.base}
                      class:base={part.base}
                      class:is-mark={part.mark}>{part.text}</span
                    >{/each}
                </span>
                <span class="finder-count"
                  >in {match.repositories}
                  {match.repositories === 1 ? 'repo' : 'repos'}</span
                >
              </Command.Item>
            {/each}
            {#if startable}
              <Command.Item
                class="finder-opt finder-new"
                value={'start: ' + cleanQuery}
                onSelect={() => choose(cleanQuery)}
              >
                <Icon name="plus" size="xs" />
                <span
                  >Start <span class="file-path">{cleanQuery}</span> - no repository has it yet</span
                >
              </Command.Item>
            {/if}
            {#if ranked.length === 0 && !startable}
              <div class="finder-empty">Type a path - matches appear as you go</div>
            {/if}
          </Command.Viewport>
        </Command.List>
        <div class="finder-keys">
          <span><kbd>↑</kbd><kbd>↓</kbd> move</span><span><kbd>↵</kbd> choose</span><span
            ><kbd>esc</kbd> close</span
          >
        </div>
      </Command.Root>
    </Popover>
  {/snippet}

  <Card unsaved={dirtyDocument}>
    <div class="card-head">
      <h2 class="card-title">{files.length} {files.length === 1 ? 'template' : 'templates'}</h2>
    </div>

    {#if files.length > 0}
      <ul class="object-list">
        {#each files as file (file.path)}
          {@const pending = differs(file.path)}
          {@const refused = refusals(file.path)}
          <li>
            <a
              class="object-row"
              class:is-unsaved={fileDirty(file)}
              data-unsaved={fileDirty(file) || undefined}
              href={fileHref(file.path)}
              onclick={(event) => open(event, file.path)}
            >
              <span class="object-main">
                <span class="object-name-row">
                  <span class="file-path">{file.path}</span>
                </span>
                <span class="object-sum">{adjustersWord(file.path)}{updatedWord(file)}</span>
              </span>
              <span class="object-side">
                {#if refused > 0}
                  <span class="mx-mark mx-refused"
                    ><Icon name="failure" size="xs" /><span class="t">{refused} refused</span></span
                  >
                {:else if pending > 0}
                  <span class="mx-mark mx-pending"
                    ><span class="t">{pending} {pending === 1 ? 'differs' : 'differ'}</span></span
                  >
                {:else}
                  <span class="mx-mark mx-instep"><Icon name="check" size="sm" /></span>
                {/if}
                <Icon name="chevron-right" size="xs" />
              </span>
            </a>
          </li>
        {/each}
      </ul>
    {:else if !unreadable}
      <div class="state-panel">
        <span
          ><strong>No shared files yet.</strong> A template added here is copied into every syncing repository
          as a pull request it can merge or close</span
        >
      </div>
    {/if}
  </Card>

  <Card>
    <div class="setting-rows">
      <div
        class="setting-row"
        class:is-unsaved={dirtyDocument && !same(stored.retired, savedDocument.retired)}
        data-unsaved={(dirtyDocument && !same(stored.retired, savedDocument.retired)) || undefined}
      >
        <span class="setting-say">
          <span class="setting-name">Paths to remove</span>
          <span class="setting-why"
            >Deleted from every syncing repository that still has them, except ignored matches - the
            only thing here that deletes anything</span
          >
        </span>
        <span class="setting-value">
          <PatternEntries
            patterns={retired}
            readOnly={frozen}
            onChange={(next) => stage({ retired: next })}
          />
        </span>
      </div>
      <div
        class="setting-row"
        class:is-unsaved={dirtyDocument && !same(stored.excludes, savedDocument.excludes)}
        data-unsaved={(dirtyDocument && !same(stored.excludes, savedDocument.excludes)) ||
          undefined}
      >
        <span class="setting-say">
          <span class="setting-name">Ignored paths</span>
          <span class="setting-why"
            >Patterns. Neither written nor removed - ignoring wins over both lists above, removal
            included</span
          >
        </span>
        <span class="setting-value">
          <PatternEntries
            patterns={excludes}
            readOnly={frozen}
            onChange={(next) => stage({ excludes: next })}
          />
        </span>
      </div>
    </div>
  </Card>
</div>

<style>
  .object-row.is-unsaved {
    background: color-mix(in srgb, var(--brand-action-tint) 45%, transparent);
    box-shadow: inset 2px 0 var(--brand-action);
  }

  .card.is-unsaved {
    border-color: color-mix(in srgb, var(--brand-action) 55%, var(--border-subtle));
  }

  .card-head {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }

  .sync-notice {
    background: var(--surface-inset);
    border-radius: var(--r-ctl);
    font-size: var(--font-size-meta);
    margin: 0 0 var(--space-4);
    padding: var(--space-2) var(--space-3);
  }

  /* ---------- The finder ---------- */

  /* The palette: input on top, ranked list, key legend - the shape every
     fuzzy picker converged on. The popover owns the layer; this owns the
     grid inside it. */
  :global(.finder-palette) {
    display: grid;
    inline-size: min(30rem, calc(100vw - 24px));
  }

  .finder-search {
    align-items: center;
    block-size: 36px;
    box-shadow: 0 1px 0 var(--border-subtle);
    color: var(--text-muted);
    display: flex;
    gap: var(--space-2);
    padding: 0 var(--space-3);
  }

  .finder-search :global(input) {
    background: none;
    block-size: 100%;
    border: 0;
    color: var(--text-primary);
    flex: 1;
    font-family: var(--mono);
    font-size: var(--font-size-control);
    outline: none;
    padding: 0;
  }

  .finder-search :global(input)::placeholder {
    color: var(--text-muted);
    font-family: var(--sans);
  }

  .finder-scope {
    border-bottom: 1px solid var(--border-subtle);
    color: var(--text-muted);
    display: flex;
    font-size: var(--font-size-micro);
    justify-content: space-between;
    padding: 0.5rem 0.75rem;
  }

  :global(.finder-list) {
    max-height: 19rem;
    overflow-y: auto;
    padding: var(--space-1);
  }

  /* Rows breathe 2px apart, like every other menu's. */
  :global(.finder-list [data-command-viewport]) {
    display: grid;
    row-gap: 2px;
  }

  :global(.finder-opt) {
    align-items: center;
    border-radius: 6px;
    cursor: pointer;
    display: flex;
    gap: var(--space-3);
    padding: 0.45rem 0.55rem;
  }

  /* The menu-row voice, in the order the app's menus speak it: pointer and
     keyboard share one highlight - Command moves data-selected with the
     arrows and under the pointer both - and a press answers one step
     darker. The brand tint stays out of it: transient focus is not a
     chosen value. */
  :global(.finder-opt[data-selected]) {
    background: var(--interactive-hover-layer);
  }

  :global(.finder-opt:active) {
    background: var(--interactive-pressed);
  }

  .finder-empty {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    padding: 0.45rem 0.55rem;
  }

  .finder-path {
    flex: 1;
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .finder-path .dir {
    color: var(--text-muted);
  }

  .finder-path .base {
    color: var(--text-primary);
    font-weight: 500;
  }

  .finder-path .is-mark {
    color: var(--match-ink);
    font-weight: 700;
  }

  .finder-count {
    color: var(--text-muted);
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  :global(.finder-opt.finder-new) {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    gap: var(--space-2);
  }

  :global(.finder-opt.finder-new) .file-path {
    color: var(--text-primary);
  }

  .finder-keys {
    background: var(--surface-raised);
    border-top: 1px solid var(--border-subtle);
    color: var(--text-muted);
    display: flex;
    font-size: var(--font-size-micro);
    gap: var(--space-4);
    padding: 0.45rem 0.75rem;
  }

  .finder-keys kbd {
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: 4px;
    font-size: var(--font-size-nano);
    padding: 0.1rem 0.3rem;
  }

  /* ---------- The list ---------- */

  .file-path {
    min-inline-size: 0;
    overflow-wrap: anywhere;
  }
</style>
