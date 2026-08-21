<script module lang="ts">
  import type { SyncFileMergeEntry } from '../types';

  /** The pill's word for how a template lands where repositories adjust it. */
  export function strategyWord(merges: readonly SyncFileMergeEntry[]): string {
    if (merges.length === 0) return 'replaces';
    const strategy = merges[0]?.merge.strategy;
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
  import { tick } from 'svelte';

  import { formatRelative } from '../format';
  import { rankPaths, type PathMatch } from '../pathfinder';
  import type { SyncConfig, SyncFile, SyncFilesContext, SyncPlan, SyncStatus } from '../types';
  import type { SyncSection } from '../routes';

  import Button from './Button.svelte';
  import FormError from './FormError.svelte';
  import Icon from './Icon.svelte';
  import PanePath from './PanePath.svelte';
  import PatternEntries from './PatternEntries.svelte';
  import Switch from './Switch.svelte';

  const {
    config,
    context,
    plan,
    status,
    nowMs,
    readOnly,
    problem = null,
    saving,
    sectionHref,
    onOpenSection,
    fileHref,
    onOpenFile,
    onSave,
  }: {
    config: SyncConfig | null;
    context: SyncFilesContext | null;
    plan: SyncPlan | null;
    status: SyncStatus | null;
    nowMs: number;
    readOnly: boolean;
    problem?: string | null;
    saving: boolean;
    sectionHref: (section: SyncSection) => string;
    onOpenSection: (section: SyncSection) => void;
    fileHref: (path: string) => string;
    onOpenFile: (path: string) => void;
    onSave: (enabled: boolean, document: Record<string, unknown>) => void;
  } = $props();

  const stored = $derived(config?.document ?? {});
  const enabled = $derived(config?.enabled ?? false);
  const unreadable = $derived(config?.unreadable === true);
  const unavailable = $derived(config?.unavailable ?? '');
  const frozen = $derived(readOnly || unreadable || saving || config === null);

  const files = $derived(Array.isArray(stored.files) ? (stored.files as SyncFile[]) : []);
  const retired = $derived(Array.isArray(stored.retired) ? (stored.retired as string[]) : []);
  const excludes = $derived(Array.isArray(stored.excludes) ? (stored.excludes as string[]) : []);

  function save(change: Partial<Record<string, unknown>>): void {
    if (frozen) return;
    onSave(enabled, { ...stored, ...change });
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

  /* ---------- The finder ---------- */

  let addOpen = $state(false);
  let query = $state('');
  let selected = $state(0);
  let finderInput: HTMLInputElement | null = $state(null);

  const ranked: PathMatch[] = $derived(rankPaths(context?.known_paths ?? [], query.trim()));

  async function openFinder(): Promise<void> {
    addOpen = true;
    query = '';
    selected = 0;
    await tick();
    finderInput?.focus();
  }

  /** Choosing a path manages it: an existing template opens, a new one is born empty. */
  function choose(path: string): void {
    const clean = path.trim();
    if (clean === '') return;
    addOpen = false;
    if (!files.some((file) => file.path === clean)) {
      save({ files: [...files, { path: clean, content: '' }] });
    }
    onOpenFile(clean);
  }

  function finderKeys(event: KeyboardEvent): void {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      const last = Math.max(ranked.length - 1, 0);
      selected =
        event.key === 'ArrowDown' ? Math.min(selected + 1, last) : Math.max(selected - 1, 0);
    } else if (event.key === 'Enter') {
      choose(ranked[selected]?.path ?? query);
    } else if (event.key === 'Escape') {
      addOpen = false;
    }
  }

  function finderOutside(event: MouseEvent): void {
    const target = event.target as HTMLElement;
    if (!target.isConnected) return;
    if (addOpen && !target.closest('.finder, .add-file')) addOpen = false;
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

<svelte:document onclick={finderOutside} />

<div class="view-frame">
  <PanePath
    segments={[
      { label: 'Sync', href: sectionHref('overview'), onSelect: () => onOpenSection('overview') },
    ]}
  />

  <div class="kind-head">
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
      onToggle={(next) => onSave(next, stored)}
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

  <div class="card">
    <div class="card-head">
      <h3 class="card-title">{files.length} {files.length === 1 ? 'template' : 'templates'}</h3>
      <Button class="add-file" disabled={frozen} onclick={() => void openFinder()}>
        {#snippet icon()}<Icon name="plus" size={13} />{/snippet}
        Add a file
      </Button>
    </div>

    {#if addOpen}
      <div class="finder">
        <input
          class="finder-input"
          type="text"
          spellcheck="false"
          role="combobox"
          aria-expanded="true"
          aria-controls="finder-list"
          aria-autocomplete="list"
          aria-label="Path of the file to manage"
          bind:this={finderInput}
          bind:value={query}
          oninput={() => (selected = 0)}
          onkeydown={finderKeys}
        />
        <div class="finder-pop">
          <div class="finder-scope">
            <span>Paths across this installation</span>
            <span
              >{(context?.known_paths ?? []).length.toLocaleString('en-US')} known · {context?.repositories ??
                0} repositories</span
            >
          </div>
          <ul class="finder-list" id="finder-list" role="listbox">
            {#each ranked as match, at (match.path)}
              <!-- svelte-ignore a11y_click_events_have_key_events -->
              <li
                class="finder-opt"
                role="option"
                aria-selected={at === selected}
                onclick={() => choose(match.path)}
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
              </li>
            {:else}
              <li class="finder-opt is-empty">No repository has a matching path</li>
            {/each}
          </ul>
          {#if query.trim() !== ''}
            <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
            <div class="finder-new" onclick={() => choose(query)}>
              <Icon name="plus" size={12} />
              <span
                >None of these? <span class="file-path">{query.trim()}</span> starts a new path no repository
                has yet</span
              >
            </div>
          {/if}
          <div class="finder-keys">
            <span><kbd>↑</kbd><kbd>↓</kbd> move</span><span><kbd>↵</kbd> choose</span><span
              ><kbd>esc</kbd> close</span
            >
          </div>
        </div>
      </div>
    {/if}

    {#if files.length > 0}
      <div class="object-list" class:block-gap-top={addOpen}>
        {#each files as file (file.path)}
          {@const merges = mergesOf(file.path)}
          {@const pending = differs(file.path)}
          {@const refused = refusals(file.path)}
          <a
            class="object-row"
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
              <Icon name="chevron-right" size={10} />
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
      <div class="setting-row">
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
            onChange={(next) => save({ retired: next })}
          />
        </span>
      </div>
      <div class="setting-row">
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
            onChange={(next) => save({ excludes: next })}
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

  .card {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    padding: var(--space-5);
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

  .card-head .card-title {
    flex: 1;
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

  .finder {
    max-width: 34rem;
    position: relative;
  }

  .finder-input {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font-family: var(--mono);
    font-size: var(--font-size-control);
    min-block-size: var(--control-height-compact);
    padding-inline: 0.7rem;
    width: 100%;
  }

  .finder-input:focus-visible {
    border-color: var(--focus);
    outline: 2px solid var(--focus);
    outline-offset: -1px;
  }

  .finder-pop {
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-popover);
    box-shadow: var(--shadow-popover);
    inset-inline: 0;
    margin-top: 6px;
    overflow: hidden;
    position: absolute;
    z-index: var(--layer-popover);
  }

  .finder-scope {
    border-bottom: 1px solid var(--border-subtle);
    color: var(--text-muted);
    display: flex;
    font-size: var(--font-size-micro);
    justify-content: space-between;
    padding: 0.5rem 0.75rem;
  }

  .finder-list {
    list-style: none;
    margin: 0;
    max-height: 19rem;
    overflow-y: auto;
    padding: 0.3rem;
  }

  .finder-opt {
    align-items: center;
    border-radius: var(--r-ctl);
    cursor: pointer;
    display: flex;
    gap: var(--space-3);
    padding: 0.45rem 0.55rem;
  }

  .finder-opt:hover {
    background: var(--interactive-hover-layer);
  }

  .finder-opt[aria-selected='true'] {
    background: var(--brand-action-tint);
  }

  .finder-opt.is-empty {
    color: var(--text-muted);
    cursor: default;
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

  .finder-new {
    align-items: center;
    border-top: 1px solid var(--border-subtle);
    color: var(--text-secondary);
    cursor: pointer;
    display: flex;
    font-size: var(--font-size-compact);
    gap: var(--space-2);
    padding: 0.55rem 0.75rem;
  }

  .finder-new .file-path {
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

  .block-gap-top {
    margin-top: var(--space-6);
  }

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

  .object-row + .object-row::before {
    background: var(--border-subtle);
    block-size: 1px;
    content: '';
    inset-inline: var(--space-3);
    position: absolute;
    top: 0;
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

  .setting-row + .setting-row::before {
    background: var(--border-subtle);
    block-size: 1px;
    content: '';
    inset-inline: var(--space-2);
    position: absolute;
    top: 0;
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
</style>
