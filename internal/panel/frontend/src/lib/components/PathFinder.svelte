<script module lang="ts">
  export interface KnownPath {
    path: string;
    /** How many repositories in this installation already hold it. */
    repositories: number;
  }
</script>

<script lang="ts">
  /**
   * A path field that knows every path the installation has seen.
   *
   * Typing a path into an empty box is guessing: the reader is being asked for
   * a string that has to match, character for character, something they cannot
   * see. So the box answers with what exists - the same file across twenty-five
   * repositories is one row carrying a count, because that is the thing being
   * configured, not twenty-five separate facts.
   *
   * Matching is fzf's: the characters must appear in order, and where they land
   * decides the ranking - a run inside the file name beats the same letters
   * picked out of the directories above it. Until the query carries a `/` the
   * file name is what is being asked about, so those matches sit in their own
   * band above the rest. The characters that matched are painted, from the same
   * walk that produced the ranking; a highlight worked out separately is a
   * highlight that disagrees with the order, which reads as the list being
   * broken.
   *
   * Nothing is fetched per keystroke. The index is a list this installation
   * already holds, and the last row says so: a path no repository has yet is
   * still a path, and the finder offers it rather than refusing the query.
   */
  import Icon from '#lib/components/Icon.svelte';
  import { matchPaths, type PathMatch } from '#lib/fuzzy.js';

  let {
    paths,
    repositories,
    value = $bindable(''),
    label,
    recents = [],
    onChoose,
  }: {
    paths: readonly KnownPath[];
    /** How many repositories the installation syncs, which the counts are of. */
    repositories: number;
    /** The path being written. Bindable, because the field is the value. */
    value?: string;
    /** Names the field - "Path in each repository". */
    label: string;
    /** Shown before anything is typed: what this reader touched last. */
    recents?: readonly string[];
    onChoose?: (path: string) => void;
  } = $props();

  let open = $state(false);
  let active = $state(0);
  let input = $state<HTMLInputElement | null>(null);

  const counts = $derived(new Map(paths.map((known) => [known.path, known.repositories])));

  const matches = $derived.by((): PathMatch[] => {
    if (value === '') {
      return recents.map((path) => ({ path, score: 0, positions: [] }));
    }

    return matchPaths(
      paths.map((known) => known.path),
      value,
      50,
    );
  });

  /** True once the query is a path that already exists, spelled exactly. */
  const exact = $derived(counts.has(value));

  const clamped = $derived(Math.min(active, Math.max(matches.length - 1, 0)));

  /** The path split into what a reader scans - the name - and where it lives. */
  const parts = (match: PathMatch) => {
    const cut = match.path.lastIndexOf('/') + 1;
    const marked = new Set(match.positions);
    const piece = (from: number, to: number) =>
      [...match.path.slice(from, to)].map((character, index) => ({
        character,
        matched: marked.has(from + index),
      }));

    return { directory: piece(0, cut), base: piece(cut, match.path.length) };
  };

  const choose = (path: string) => {
    value = path;
    open = false;
    onChoose?.(path);
    input?.focus();
  };

  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') {
      open = false;

      return;
    }
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      if (!open) {
        open = true;

        return;
      }
      const step = event.key === 'ArrowDown' ? 1 : -1;
      active = (clamped + step + matches.length) % Math.max(matches.length, 1);

      return;
    }
    if (event.key === 'Enter' && open && matches.length > 0) {
      event.preventDefault();
      choose(matches[clamped].path);
    }
  };
</script>

<div class="finder">
  <input
    bind:this={input}
    bind:value
    class="finder-input"
    type="text"
    spellcheck="false"
    autocomplete="off"
    aria-label={label}
    role="combobox"
    aria-expanded={open}
    aria-controls="finder-list"
    aria-autocomplete="list"
    aria-activedescendant={open && matches.length > 0 ? `finder-option-${clamped}` : undefined}
    onfocus={() => {
      open = true;
    }}
    oninput={() => {
      open = true;
      active = 0;
    }}
    onkeydown={onKeyDown}
    onblur={() => {
      open = false;
    }}
  />

  {#if open}
    <!-- Pressing an option must not blur the field: the reader's caret stays
         where they left it, which is what `aria-activedescendant` promises. -->
    <div
      class="finder-pop"
      onmousedown={(event) => {
        event.preventDefault();
      }}
      role="presentation"
    >
      <div class="finder-scope">
        <span>Paths across this installation</span>
        <span>{paths.length.toLocaleString()} known</span>
      </div>

      {#if value === '' && recents.length > 0}
        <p class="finder-band">Recent</p>
      {/if}

      <ul class="finder-list" id="finder-list" role="listbox" aria-label={label}>
        {#each matches as match, index (match.path)}
          {@const shown = parts(match)}
          <!-- The keyboard is on the field, which is the whole point of a
               combobox: focus never leaves the input, and the arrows and Enter
               are handled there. An option that took focus would break the
               `aria-activedescendant` contract this list is built on. -->
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <li
            class="finder-opt"
            id="finder-option-{index}"
            role="option"
            aria-selected={index === clamped}
            onclick={() => choose(match.path)}
          >
            <span class="finder-path">
              <span class="dir"
                >{#each shown.directory as letter, at (at)}<span class:is-match={letter.matched}
                    >{letter.character}</span
                  >{/each}</span
              ><span class="base"
                >{#each shown.base as letter, at (at)}<span class:is-match={letter.matched}
                    >{letter.character}</span
                  >{/each}</span
              >
            </span>
            <span class="finder-count">{counts.get(match.path) ?? 0} of {repositories}</span>
          </li>
        {:else}
          <li class="finder-empty" role="presentation">No path here matches that</li>
        {/each}
      </ul>

      {#if value !== '' && !exact}
        <div class="finder-new">
          <Icon name="plus" size={13} />
          <span>
            None of these? <span class="file-path">{value}</span> starts a path no repository has yet
          </span>
        </div>
      {/if}

      <div class="finder-keys">
        <span><kbd>↑</kbd><kbd>↓</kbd> move</span>
        <span><kbd>↵</kbd> choose</span>
        <span><kbd>esc</kbd> close</span>
      </div>
    </div>
  {/if}
</div>

<style>
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

  .finder-band {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    font-weight: 600;
    letter-spacing: 0.07em;
    margin: 0;
    padding: 0.55rem 0.75rem 0.25rem;
    text-transform: uppercase;
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
    background-image: linear-gradient(
      var(--interactive-hover-layer),
      var(--interactive-hover-layer)
    );
  }

  .finder-opt[aria-selected='true'] {
    background: var(--brand-action-tint);
  }

  .finder-empty {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    padding: 0.55rem;
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

  /* The name is what a reader scans for; the directories say where it lives. */
  .dir {
    color: var(--text-muted);
  }

  .base {
    color: var(--text-primary);
    font-weight: 500;
  }

  .is-match {
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
    display: flex;
    font-size: var(--font-size-compact);
    gap: var(--space-2);
    padding: 0.55rem 0.75rem;
  }

  .file-path {
    color: var(--text-primary);
    font-family: var(--mono);
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

  kbd {
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: 4px;
    font-family: var(--mono);
    font-size: var(--font-size-nano);
    padding: 0.1rem 0.3rem;
  }
</style>
