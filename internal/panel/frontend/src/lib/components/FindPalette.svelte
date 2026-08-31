<script module lang="ts">
  /** One thing a reader can reach, as the palette lists it. */
  export interface FindEntry {
    /** The heading it files under: Pages, Repositories, People, Workspaces. */
    group: string;
    title: string;
    /** What it is, in the palette's own voice - never the address. */
    say: string;
    href: string;
    select: () => void;
    /**
     * The console this belongs to, when that is not the one open. A result that
     * leaves the console the reader is in is never silent about it.
     */
    cross?: string;
  }

  /**
   * The shortest query worth answering.
   *
   * One letter matches most of the panel: every page, every workspace and whatever the
   * service returns for it, which is a list nobody reads and a request nobody wanted.
   * Two is where a query starts naming something. Exported because the palette and the
   * results page are one search and must agree on where it begins.
   */
  export const SEARCH_MINIMUM = 2;

  const RECENTS_KEY = 'smyklot-panel-recent-searches';
  const RECENTS_KEPT = 5;
  /** Rows shown before the palette stops listing and says how many are left. */
  const ROWS_SHOWN = 12;

  function readRecents(): string[] {
    try {
      const raw = localStorage.getItem(RECENTS_KEY);
      if (raw === null) return [];
      const parsed: unknown = JSON.parse(raw);
      return Array.isArray(parsed) ? parsed.filter((item) => typeof item === 'string') : [];
    } catch {
      return [];
    }
  }

  function writeRecents(entries: readonly string[]): void {
    try {
      localStorage.setItem(RECENTS_KEY, JSON.stringify(entries.slice(0, RECENTS_KEPT)));
    } catch {
      /* A reader with site data blocked still gets a palette; only the list is lost. */
    }
  }

  /** Every term has to appear somewhere in the row - the words, not their order. */
  export function findMatches(entry: FindEntry, terms: readonly string[]): boolean {
    const haystack = `${entry.title} ${entry.say}`.toLowerCase();
    return terms.every((term) => haystack.includes(term));
  }

  /** The run of a row's text, split into the parts a term matched and the rest. */
  export function findMarks(
    text: string,
    terms: readonly string[],
  ): { text: string; hit: boolean }[] {
    const hits: [number, number][] = [];
    const lower = text.toLowerCase();
    for (const term of terms) {
      if (term === '') continue;
      let from = lower.indexOf(term);
      while (from !== -1) {
        hits.push([from, from + term.length]);
        from = lower.indexOf(term, from + term.length);
      }
    }
    if (hits.length === 0) return [{ text, hit: false }];
    hits.sort((left, right) => left[0] - right[0]);
    const parts: { text: string; hit: boolean }[] = [];
    let at = 0;
    for (const [start, end] of hits) {
      if (end <= at) continue;
      const from = Math.max(at, start);
      if (from > at) parts.push({ text: text.slice(at, from), hit: false });
      parts.push({ text: text.slice(from, end), hit: true });
      at = end;
    }
    if (at < text.length) parts.push({ text: text.slice(at), hit: false });
    return parts;
  }
</script>

<script lang="ts">
  import { untrack } from 'svelte';

  import { searchAddress } from '../addresses.ts';

  import Icon from './Icon.svelte';

  let {
    open = $bindable(false),
    placeholder,
    entries,
    lookup,
    crossLabel,
  }: {
    open?: boolean;
    /** What this palette searches: "Search this workspace" / "Search the console". */
    placeholder: string;
    /** Everything already known - the pages, and the workspaces. */
    entries: readonly FindEntry[];
    /** What has to be asked for: repositories and people, by what was typed. */
    lookup?: (query: string) => Promise<FindEntry[]>;
    /** The other console's name, for the row that widens the search. */
    crossLabel?: string;
  } = $props();

  let dialog = $state<HTMLDialogElement | null>(null);
  let field = $state<HTMLInputElement | null>(null);
  let query = $state('');
  let at = $state(0);
  let allScopes = $state(false);
  let recents = $state<string[]>([]);
  let lookedUp = $state<FindEntry[]>([]);
  let looking = $state(false);
  let asked = 0;

  /* Under the minimum there is no query yet: the field is being typed into, not asked
     with. `asking` is what everything else reads, so the rows, the count and the
     lookup all begin at the same character. */
  const asking = $derived(query.trim().length >= SEARCH_MINIMUM);
  const terms = $derived(
    !asking
      ? []
      : query
          .trim()
          .toLowerCase()
          .split(/\s+/u)
          .filter((term) => term !== ''),
  );

  /* Scope first, then the words: a result that leaves the console the reader is in
     is offered only once they ask for it. */
  const hits = $derived.by((): FindEntry[] => {
    if (terms.length === 0) return [];
    const all = [...entries, ...lookedUp].filter((entry) => findMatches(entry, terms));
    return allScopes ? all : all.filter((entry) => entry.cross === undefined);
  });
  const shown = $derived(hits.slice(0, ROWS_SHOWN));
  const groups = $derived.by(() => {
    const order: { name: string; cross?: string; rows: FindEntry[] }[] = [];
    for (const entry of shown) {
      const key = `${entry.cross ?? ''}:${entry.group}`;
      const held = order.find((group) => `${group.cross ?? ''}:${group.name}` === key);
      if (held === undefined) order.push({ name: entry.group, cross: entry.cross, rows: [entry] });
      else held.rows.push(entry);
    }
    return order;
  });
  /* One flat run of everything the arrows walk, rows and the tail together. */
  const walk = $derived([
    ...shown.map((entry) => ({ kind: 'entry' as const, entry })),
    ...(crossLabel !== undefined && !allScopes && terms.length > 0
      ? [{ kind: 'scope' as const, entry: null }]
      : []),
  ]);
  const recentWalk = $derived(terms.length === 0 ? recents : []);
  const steps = $derived(terms.length === 0 ? recentWalk.length : walk.length);

  $effect(() => {
    if (!open) return;
    untrack(() => {
      query = '';
      at = 0;
      allScopes = false;
      recents = readRecents();
      lookedUp = [];
      dialog?.showModal();
      field?.focus();
    });
  });

  $effect(() => {
    if (open || dialog === null) return;
    untrack(() => dialog?.close());
  });

  /* What has to be asked for, asked once the typing settles. The answer is thrown
     away if a later question has already been asked - a slow reply must never
     replace a newer one. */
  $effect(() => {
    const text = query.trim();
    if (lookup === undefined || !asking) {
      untrack(() => {
        lookedUp = [];
        looking = false;
      });
      return;
    }
    const mine = ++asked;
    untrack(() => (looking = true));
    const timer = setTimeout(() => {
      void lookup(text).then((rows) => {
        if (mine !== asked) return;
        lookedUp = rows;
        looking = false;
      });
    }, 160);
    return () => clearTimeout(timer);
  });

  $effect(() => {
    void query;
    void allScopes;
    untrack(() => (at = 0));
  });

  function remember(text: string): void {
    const kept = text.trim();
    if (kept === '') return;
    const next = [kept, ...recents.filter((item) => item.toLowerCase() !== kept.toLowerCase())];
    recents = next.slice(0, RECENTS_KEPT);
    writeRecents(recents);
  }

  function choose(entry: FindEntry): void {
    remember(query);
    open = false;
    entry.select();
  }

  function keys(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      open = false;
      return;
    }
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      if (steps === 0) return;
      event.preventDefault();
      at = (at + (event.key === 'ArrowDown' ? 1 : steps - 1)) % steps;
      return;
    }
    if (event.key !== 'Enter') return;
    event.preventDefault();
    if (terms.length === 0) {
      const recent = recentWalk[at];
      if (recent !== undefined) {
        query = recent;
        field?.focus();
      }
      return;
    }
    const step = walk[at];
    if (step === undefined) return;
    if (step.kind === 'scope') allScopes = true;
    else choose(step.entry);
  }
</script>

<!--
@component
The one field that reaches everything: pages, the workspaces, the repositories in
this one, and the people in it.

Scoped to the console the reader is in, because a result that quietly changes
console is a result that loses them. What the other console holds is one row away -
and when it is offered, it is offered under the shield, named.
-->

<dialog
  class="find-panel"
  bind:this={dialog}
  aria-label="Search"
  onkeydown={keys}
  onclose={() => (open = false)}
  onclick={(event) => {
    if (event.target === dialog) open = false;
  }}
>
  <div class="find-card">
    <div class="find-field">
      <span class="gi"><Icon name="search" size="base" /></span>
      <input
        class="find-input"
        bind:this={field}
        bind:value={query}
        type="text"
        role="combobox"
        spellcheck="false"
        autocomplete="off"
        {placeholder}
        aria-label="Search"
        aria-expanded={steps > 0}
        aria-controls="find-menu"
        aria-autocomplete="list"
      />
      <button
        class="find-close"
        type="button"
        aria-label="Close search"
        onclick={() => (open = false)}
      >
        <Icon name="close" size="sm" />
      </button>
    </div>
    <div class="find-menu" id="find-menu" role="listbox" aria-label="Results">
      {#if terms.length === 0}
        {#if query.trim() !== ''}
          <!-- One letter is not a question: it matches most of the panel, so saying so
               is better than answering it with everything. -->
          <p class="find-note">Keep typing - a search starts at {SEARCH_MINIMUM} letters</p>
        {:else if recents.length === 0}
          <p class="find-note">Type to search - pages, repositories, people</p>
        {:else}
          <div class="find-group" role="group" aria-labelledby="find-g-recent">
            <span class="find-group-head">
              <span class="find-group-name" id="find-g-recent">Recent</span>
              <button
                class="find-clear"
                type="button"
                aria-label="Clear recent searches - the list only, never your data"
                onclick={() => {
                  recents = [];
                  writeRecents(recents);
                }}
              >
                Clear
              </button>
            </span>
            {#each recents as recent, index (recent)}
              <button
                class="find-row is-recent"
                class:is-at={at === index}
                type="button"
                role="option"
                tabindex="-1"
                aria-selected={at === index}
                onclick={() => {
                  query = recent;
                  field?.focus();
                }}
              >
                <span class="gi"><Icon name="history" size="sm" /></span>
                <span class="t">{recent}</span>
              </button>
            {/each}
          </div>
        {/if}
      {:else if shown.length === 0}
        <!-- Never "nothing matches" while the answer is still on its way: the
             repositories and the people are asked for, and a claim made before
             they arrive is a claim that is wrong every time. -->
        <p class="find-note" role="status">
          {looking ? 'Looking…' : `Nothing here matches "${query.trim()}"`}
        </p>
      {:else}
        {#each groups as group (`${group.cross ?? ''}:${group.name}`)}
          <div class="find-group" role="group">
            {#if group.cross !== undefined}
              <span class="find-group-name is-cross">
                <Icon name="shield" size="xs" />{group.cross} · {group.name}
              </span>
            {:else}
              <span class="find-group-name">{group.name}</span>
            {/if}
            {#each group.rows as row (row.href + row.title)}
              <a
                class="find-row"
                class:is-at={walk[at]?.entry === row}
                href={row.href}
                role="option"
                tabindex="-1"
                aria-selected={walk[at]?.entry === row}
                onclick={(event) => {
                  if (event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0)
                    return;
                  event.preventDefault();
                  choose(row);
                }}
              >
                <span class="find-row-head">
                  {#each findMarks(row.title, terms) as part, index (index)}
                    {#if part.hit}<mark>{part.text}</mark>{:else}{part.text}{/if}
                  {/each}
                </span>
                <span class="find-row-text">
                  {#each findMarks(row.say, terms) as part, index (index)}
                    {#if part.hit}<mark>{part.text}</mark>{:else}{part.text}{/if}
                  {/each}
                </span>
              </a>
            {/each}
          </div>
        {/each}
        {#if hits.length > shown.length}
          <!-- The count used to be the end of it: a reader was told twelve of their
               matches were on screen and left with no way to the rest.

               No number on the link. This palette counts what it is scoped to and the
               page counts both consoles, so a count here is a promise the page does not
               keep - it said twenty and showed thirty-one. -->
          <a class="find-note find-all" href={searchAddress(query)} onclick={() => (open = false)}>
            See all results for “{query.trim()}”
          </a>
        {/if}
      {/if}
      {#if crossLabel !== undefined && !allScopes && terms.length > 0}
        <button
          class="find-scope"
          class:is-at={walk[at]?.kind === 'scope'}
          type="button"
          onclick={() => (allScopes = true)}
        >
          <Icon name="shield" size="sm" />
          Search {crossLabel} as well
        </button>
      {/if}
    </div>
    <footer class="find-foot">
      <span><kbd>↑</kbd><kbd>↓</kbd> to move</span>
      <span><kbd>↵</kbd> to open</span>
      <span><kbd>esc</kbd> to close</span>
    </footer>
  </div>
</dialog>

<style>
  .find-panel {
    align-content: start;
    background: transparent;
    block-size: 100%;
    border: 0;
    color: var(--text-primary);
    display: none;
    inline-size: 100%;
    inset: 0;
    justify-items: center;
    max-block-size: none;
    max-inline-size: none;
    opacity: 0;
    overflow: auto;
    overscroll-behavior: contain;
    padding: 0;
    transition:
      display var(--duration-fast) allow-discrete,
      overlay var(--duration-fast) allow-discrete,
      opacity var(--duration-fast) var(--ease-standard),
      translate var(--duration-fast) var(--ease-standard);
    translate: 0 -8px;
  }

  .find-panel[open] {
    display: grid;
    opacity: 1;
    translate: 0 0;
  }

  @starting-style {
    .find-panel[open] {
      opacity: 0;
      translate: 0 -8px;
    }
  }

  .find-panel::backdrop {
    background: var(--scrim);
    opacity: 0;
    transition:
      display var(--duration-fast) allow-discrete,
      overlay var(--duration-fast) allow-discrete,
      opacity var(--duration-fast) var(--ease-standard);
  }

  .find-panel[open]::backdrop {
    opacity: 1;
  }

  @starting-style {
    .find-panel[open]::backdrop {
      opacity: 0;
    }
  }

  .find-card {
    background: var(--surface-base);
    border: 1px solid var(--popover-border);
    border-radius: var(--r-strip);
    box-shadow: var(--shadow-dialog);
    display: grid;
    grid-template-rows: auto minmax(0, 1fr) auto;
    inline-size: min(40rem, calc(100vw - 2 * var(--space-4)));
    margin-block: min(12vh, 4rem);
    max-block-size: min(34rem, 76vh);
    overflow: hidden;
  }

  .find-field {
    align-items: center;
    border-block-end: 1px solid var(--border-subtle);
    /* The card's radius MINUS its 1px border, not inherit: the row sits 1px inside
       the card, so only r-1 makes the ring's arc concentric with the border's inner
       arc - inheriting r left a 0.4px crescent of card ground at each corner. */
    border-start-start-radius: calc(var(--r-strip) - 1px);
    border-start-end-radius: calc(var(--r-strip) - 1px);
    color: var(--text-muted);
    display: grid;
    gap: var(--space-3);
    grid-template-columns: auto minmax(0, 1fr) auto;
    /* 20, not 16: the menu's rows ink at 8 (menu pad) + 12 (row pad), and the field
       glyph and the foot's first hint stand on that same line. */
    padding-inline: calc(var(--space-2) + var(--space-3));
  }

  /* An OUTLINE, not an inset shadow: a shadow is painted by the element and clipped
     by any ancestor hiding its overflow, and this field lives inside a rounded,
     clipping card. Inset because the field fills the card's top edge and has nothing
     outside to offset into. */
  .find-field:focus-within {
    outline: var(--focus-ring-width) solid var(--focus);
    outline-offset: var(--focus-ring-inset);
  }

  .find-input {
    background: none;
    border: 0;
    color: var(--text-primary);
    font: inherit;
    font-size: var(--font-size-body);
    min-inline-size: 0;
    outline: none;
    padding-block: var(--space-4);
  }

  .find-input::placeholder {
    color: var(--text-secondary);
  }

  /* The setting-clear circle - the app's one shape for a quiet dismiss. */
  .find-close {
    align-items: center;
    background: none;
    block-size: var(--tier-quiet);
    border: 0;
    border-radius: 50%;
    color: var(--text-muted);
    cursor: pointer;
    display: inline-flex;
    inline-size: var(--tier-quiet);
    justify-content: center;
    padding: 0;
  }

  .find-close:hover {
    background: var(--interactive-hover-layer);
    color: var(--text-primary);
  }

  .find-close:active {
    background: var(--interactive-pressed);
  }

  .find-menu {
    overflow-y: auto;
    overscroll-behavior: contain;
    padding: var(--space-2);
  }

  .find-group + .find-group {
    border-block-start: 1px solid var(--border-subtle);
    margin-block-start: var(--space-2);
    padding-block-start: var(--space-2);
  }

  .find-group-name {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    font-weight: 600;
    letter-spacing: 0.07em;
    line-height: var(--leading-tight);
    padding: var(--space-2) var(--space-3) var(--space-1);
    text-transform: uppercase;
  }

  .find-group-head {
    align-items: center;
    display: flex;
    justify-content: space-between;
  }

  .find-clear {
    background: none;
    border: 0;
    border-radius: 6px;
    color: var(--text-muted);
    cursor: pointer;
    font-size: var(--font-size-micro);
    margin-inline-end: var(--space-1);
    padding: 2px var(--space-2);
  }

  .find-clear:hover {
    background: var(--interactive-hover-layer);
    color: var(--text-primary);
  }

  .find-clear:active {
    background: var(--interactive-pressed);
    box-shadow: var(--pressed-inset);
  }

  /* Menu-item states on menu-item radius - the palette is a big menu. */
  .find-row {
    background: none;
    border: 0;
    border-radius: 6px;
    color: inherit;
    cursor: pointer;
    display: grid;
    gap: 2px;
    inline-size: 100%;
    padding: var(--space-2) var(--space-3);
    text-align: start;
    text-decoration: none;
  }

  .find-row.is-at {
    background: var(--interactive-hover-layer);
  }

  .find-row:active {
    background: var(--interactive-pressed);
    box-shadow: var(--pressed-inset);
  }

  .find-row-head {
    color: var(--text-primary);
    font-size: var(--font-size-compact);
    font-weight: 600;
  }

  .find-row-text {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
  }

  .find-row mark {
    background: none;
    color: var(--match-ink);
    font-weight: 650;
  }

  .find-row.is-recent {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    min-block-size: var(--control-height-compact);
  }

  .find-row.is-recent .gi {
    color: var(--text-muted);
    display: inline-flex;
  }

  .find-row.is-recent .t {
    color: var(--text-primary);
    font-size: var(--font-size-compact);
  }

  .find-note {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    margin: 0;
    padding: var(--space-4);
  }

  /* The way to the rest, which is a link and reads like one. */
  .find-all {
    color: var(--brand-action-text);
    display: block;
    text-decoration: none;
  }

  .find-all:hover {
    text-decoration: underline;
    text-underline-offset: 0.15em;
  }

  .find-foot {
    border-block-start: 1px solid var(--border-subtle);
    color: var(--text-muted);
    display: flex;
    font-size: var(--font-size-micro);
    gap: var(--space-4);
    padding: var(--space-3) calc(var(--space-2) + var(--space-3));
  }

  .find-foot kbd {
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: 4px;
    font-family: inherit;
    margin-inline-end: var(--space-1);
    padding: 1px var(--space-1);
  }

  /* Cross-console results carry the shield and the console's name - a violet group
     inside a workspace search is never silent about where it goes. */
  .find-group-name.is-cross {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }

  .find-scope {
    align-items: center;
    background: none;
    border: 0;
    border-block-start: 1px solid var(--border-subtle);
    border-radius: 0;
    color: var(--text-secondary);
    cursor: pointer;
    display: flex;
    font-size: var(--font-size-compact);
    gap: var(--space-2);
    inline-size: 100%;
    margin-block-start: var(--space-2);
    padding: var(--space-3);
    text-align: start;
  }

  .find-scope:hover,
  .find-scope.is-at {
    background: var(--interactive-hover-layer);
    color: var(--text-primary);
  }
</style>
