<script module lang="ts">
  import type { SyncFileMergeEntry } from '../types';

  /** The pill's word for how a template lands where repositories adjust it. */
  export function strategyWord(merges: readonly SyncFileMergeEntry[]): string {
    if (merges.length === 0) return 'replaces';
    const strategy = merges[0]?.merge?.strategy;
    if (strategy === 'markdown') return 'merges · sections';
    if (strategy === 'shallow-merge') return 'merges · shallow';
    return 'merges · deep';
  }
</script>

<script lang="ts">
  /**
   * The shared files list: what every repository should carry. Each template
   * is a named object one press from its own page; the add flow is a path
   * field with fuzzy suggestions from what the organization's repositories
   * already hold - the index ships once, matching costs no requests.
   */
  import { Command } from 'bits-ui';

  import { formatRelative } from '../format';
  import { rankPaths, type PathMatch } from '../pathfinder';
  import type { SyncConfig, SyncFile, SyncFilesContext, SyncPlan, SyncStatus } from '../types';
  import type { SyncSection } from '../routes';

  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import PanePath from './PanePath.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import Popover from './Popover.svelte';
  import Switch from './Switch.svelte';

  const {
    config,
    savedDocument = {},
    context,
    plan,
    status,
    nowMs,
    readOnly,
    problem = null,
    sectionHref,
    onOpenSection,
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
    status: SyncStatus | null;
    nowMs: number;
    readOnly: boolean;
    problem?: string | null;
    sectionHref: (section: SyncSection) => string;
    onOpenSection: (section: SyncSection) => void;
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
    const rows = status?.repositories ?? [];
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
  <PanePath
    segments={[
      { label: 'Sync', href: sectionHref('overview'), onSelect: () => onOpenSection('overview') },
    ]}
  />

  <div class="kind-head" class:is-unsaved={dirtyEnabled} data-unsaved={dirtyEnabled || undefined}>
    <div class="kind-head-say">
      <h2 class="card-title">Shared files</h2>
      <p class="kind-head-sub">
        What every repository should carry, and what it should say. A file that differs arrives as a
        pull request the repository can merge or close
      </p>
    </div>
    <Switch
      checked={enabled}
      label="File sync"
      word="Syncing"
      disabled={frozen}
      onToggle={onToggleEnabled}
    />
  </div>

  {#if problem !== null}
    <FormError message={problem} />
  {/if}

  {#if unreadable}
    <p class="sync-notice" role="alert">
      This installation's files are stored in a form this version of Smyklot cannot read, so they
      are not shown and nothing here can be changed. Nothing has been lost.
    </p>
  {/if}

  {#if unavailable !== '' && enabled}
    <p class="sync-notice" role="status">
      {unavailable}. Nothing here will be planned or changed until an owner grants it on the
      installation's page on GitHub.
    </p>
  {/if}

  <div class="card" class:is-unsaved={dirtyDocument} data-unsaved={dirtyDocument || undefined}>
    <div class="card-head">
      <h3 class="card-title">{files.length} {files.length === 1 ? 'template' : 'templates'}</h3>
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
            <Icon name="plus" size={13} />
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
            <Icon name="search" size={12} />
            <Command.Input
              bind:value={query}
              placeholder="renovate.json, or a path no repository has yet"
              spellcheck="false"
              autocomplete="off"
            />
          </div>
          <div class="finder-scope">
            <span>Paths across this installation</span>
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
                  <Icon name="plus" size={12} />
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
    </div>

    {#if files.length > 0}
      <div class="object-list">
        {#each files as file (file.path)}
          {@const merges = mergesOf(file.path)}
          {@const pending = differs(file.path)}
          {@const refused = refusals(file.path)}
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
                <span class="pill pill-neutral"><span class="t">{strategyWord(merges)}</span></span>
              </span>
              <span class="object-sum">{adjustersWord(file.path)}{updatedWord(file)}</span>
            </span>
            <span class="object-side">
              {#if refused > 0}
                <span class="mx-mark mx-refused"
                  ><Icon name="failure" size={12} /><span class="t">{refused} refused</span></span
                >
              {:else if pending > 0}
                <span class="mx-mark mx-pending"
                  ><span class="t">{pending} {pending === 1 ? 'differs' : 'differ'}</span></span
                >
              {:else}
                <span class="mx-mark mx-instep"><Icon name="check" size={14} /></span>
              {/if}
              <Icon name="chevron-right" size={12} />
            </span>
          </a>
        {/each}
      </div>
    {:else if !unreadable}
      <p class="sync-empty">No templates yet</p>
    {/if}
  </div>

  <div class="card">
    <div class="setting-rows">
      <div
        class="setting-row"
        class:is-unsaved={dirtyDocument && !same(stored.retired, savedDocument.retired)}
        data-unsaved={(dirtyDocument && !same(stored.retired, savedDocument.retired)) || undefined}
      >
        <span class="setting-say">
          <span class="setting-name">Paths to remove</span>
          <span class="setting-why"
            >Deleted wherever a repository still has them - the only thing here that deletes
            anything</span
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
          <span class="setting-name">Paths to leave alone</span>
          <span class="setting-why"
            >Patterns. Neither written nor removed, whatever the lists above say</span
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
  </div>
</div>

<style>
  .view-frame {
    margin-inline: auto;
    max-width: var(--content-max);
  }

  .kind-head {
    align-items: start;
    display: flex;
    gap: var(--space-4);
    justify-content: space-between;
    margin-bottom: var(--space-4);
  }

  .kind-head-say {
    display: grid;
    gap: var(--space-2);
  }

  .kind-head-sub {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    line-height: round(1.5em, 1px);
    margin: 0;
  }

  .kind-head :global(.switch) {
    min-block-size: auto;
  }

  .kind-head.is-unsaved,
  .object-row.is-unsaved,
  .setting-row.is-unsaved {
    background: color-mix(in srgb, var(--brand-action-tint) 45%, transparent);
    box-shadow: inset 2px 0 var(--brand-action);
  }

  .kind-head.is-unsaved {
    margin-inline: calc(var(--space-2) * -1);
    padding: var(--space-2);
  }

  .card {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    padding: var(--space-5);
  }

  .card.is-unsaved {
    border-color: color-mix(in srgb, var(--brand-action) 55%, var(--border-subtle));
  }

  .card + .card {
    margin-top: var(--space-4);
  }

  .card-title {
    font-size: var(--font-size-card-title);
    font-weight: 600;
    margin: 0;
    min-block-size: 13px;
    text-box: trim-both cap alphabetic;
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

  .sync-empty {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    margin: 0;
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

  .object-list {
    display: grid;
    margin-block-end: -12px;
  }

  .object-row {
    align-items: center;
    border-radius: var(--r-ctl);
    color: inherit;
    display: grid;
    gap: var(--space-4);
    grid-template-columns: 1fr auto;
    margin-inline: calc(var(--space-3) * -1);
    padding: 0.75rem var(--space-3);
    position: relative;
    text-decoration: none;
  }

  .object-row:hover {
    background: var(--table-row-hover);
  }

  .object-row:active {
    background: var(--table-row-pressed);
    box-shadow: var(--pressed-inset);
    translate: 0 1px;
  }

  .object-row:not(:last-child)::after {
    background: var(--border-subtle);
    block-size: 1px;
    bottom: 0;
    content: '';
    inset-inline: var(--space-3);
    position: absolute;
  }

  /* The hover pill has rounded corners; a hairline crossing its edge reads
     as a crack in it. The hovered row hides its own separator and the one
     its neighbour would draw over it. */
  .object-row:hover::after,
  .object-row:has(+ .object-row:hover)::after {
    background: transparent;
  }

  .object-main {
    display: grid;
    gap: var(--space-1);
  }

  .object-name-row {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    min-block-size: 20px;
  }

  .file-path {
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-weight: 500;
    text-box: trim-both cap alphabetic;
  }

  .object-sum {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .object-side {
    align-items: center;
    color: var(--text-muted);
    display: flex;
    gap: var(--space-3);
  }

  .pill {
    align-items: center;
    block-size: 20px;
    border-radius: var(--radius-chip);
    display: inline-flex;
    font-size: var(--font-size-micro);
    font-weight: 600;
    gap: 0.25rem;
    line-height: 1;
    padding: 0 0.5rem;
  }

  .pill .t {
    display: block;
    text-box: trim-both cap alphabetic;
  }

  .pill-neutral {
    background: var(--surface-inset);
    color: var(--text-secondary);
  }

  .mx-mark {
    align-items: center;
    block-size: 20px;
    border-radius: var(--r-chip);
    box-sizing: border-box;
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    gap: 0.25rem;
    line-height: 1;
    padding: 0 0.5rem;
  }

  .mx-mark .t {
    display: block;
    text-box: trim-both cap alphabetic;
  }

  .mx-instep {
    color: var(--success);
  }

  .mx-pending {
    background: var(--cell-pending-bg);
    border: 1px solid color-mix(in srgb, var(--cell-pending) 38%, transparent);
    color: var(--cell-pending);
    font-weight: 500;
  }

  .mx-refused {
    background: var(--cell-refused-bg);
    border: 1px solid color-mix(in srgb, var(--cell-refused) 38%, transparent);
    color: var(--cell-refused);
    font-weight: 500;
  }

  /* ---------- The bottom card ---------- */

  .setting-rows {
    display: grid;
  }

  .card > .setting-rows:only-child {
    margin-block: calc(var(--space-5) * -1);
  }

  .card > .setting-rows:only-child > .setting-row {
    align-items: start;
    padding-block: var(--space-5);
  }

  .setting-row {
    align-items: center;
    border-radius: var(--r-ctl);
    display: grid;
    gap: var(--space-2) var(--space-4);
    grid-auto-columns: auto;
    grid-auto-flow: column;
    grid-template-columns: 1fr;
    margin-inline: calc(var(--space-2) * -1);
    min-block-size: var(--touch-target);
    padding: var(--space-3) var(--space-2);
    position: relative;
  }

  .setting-row:not(:last-child)::after {
    background: var(--border-subtle);
    block-size: 1px;
    bottom: 0;
    content: '';
    inset-inline: var(--space-2);
    position: absolute;
  }

  .setting-say {
    display: grid;
    gap: var(--space-3);
  }

  .setting-name {
    font-size: var(--font-size-meta);
    font-weight: 600;
    min-block-size: 10px;
    text-box: trim-both cap alphabetic;
  }

  .setting-why {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    min-block-size: 9px;
    text-box: trim-both cap alphabetic;
  }

  .setting-value {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    justify-content: end;
    justify-self: end;
    min-inline-size: 0;
  }

  @media (max-width: 36rem) {
    .card {
      padding: var(--space-4);
    }

    .object-row {
      gap: var(--space-2);
      grid-template-columns: minmax(0, 1fr) auto;
    }

    .object-main,
    .object-name-row,
    .file-path,
    .object-sum {
      min-inline-size: 0;
    }

    .object-name-row {
      align-items: start;
      flex-direction: column;
    }

    .file-path,
    .object-sum {
      overflow-wrap: anywhere;
    }

    .object-side {
      gap: var(--space-1);
    }

    .setting-row {
      grid-auto-flow: row;
      grid-template-columns: minmax(0, 1fr);
    }

    .setting-value {
      justify-content: start;
      justify-self: stretch;
    }
  }
</style>
