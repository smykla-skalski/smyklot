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
  import { foldPaths, matchPaths, type PathMatch } from '#lib/fuzzy.js';

  let {
    paths,
    repositories,
    partial = false,
    value = $bindable(''),
    label,
    onChoose,
  }: {
    paths: readonly KnownPath[];
    /** How many repositories the installation syncs, which the counts are of. */
    repositories: number;
    /**
     * Whether GitHub declined to list one of those repositories whole. Nothing
     * here drops a path on purpose, so this is the only way the list can be
     * short - and a short list that looks complete is what makes somebody
     * believe a file they can see is not there.
     */
    partial?: boolean;
    /** The path being written. Bindable, because the field is the value. */
    value?: string;
    /** Names the field - "Path in each repository". */
    label: string;
    onChoose?: (path: string) => void;
  } = $props();

  let open = $state(false);
  let active = $state(0);
  let input = $state<HTMLInputElement | null>(null);

  const counts = $derived(new Map(paths.map((known) => [known.path, known.repositories])));

  /* Held beside the list rather than rebuilt per keystroke: `names` allocated a
     fifty-thousand-element array inside the matching `$derived`, and `folded`
     is the lowercased copy `matchPath` would otherwise redo for every path on
     every key. Both change when the installation's paths do, which is about
     once a day. */
  const names = $derived(paths.map((known) => known.path));
  const folded = $derived(foldPaths(names));

  const matches = $derived(matchPaths(names, value, 50, folded));

  /** True once the query is a path that already exists, spelled exactly. */
  const exact = $derived(counts.has(value));

  const clamped = $derived(Math.min(active, Math.max(matches.length - 1, 0)));

  /** The path split into what a reader scans - the name - and where it lives. */
  const parts = (match: PathMatch) => {
    const cut = match.path.lastIndexOf('/') + 1;
    const marked = new Set(match.positions);
    /* Whole characters out, code units counted.
     *
     * `positions` are code-unit offsets, because `matchPath` walks the path
     * with `indexOf`. Spreading a string iterates by CODE POINT, so mapping the
     * spread's own index onto those positions drifts by one for every astral
     * character before it - a path holding an emoji painted the wrong letters
     * as matched. Splitting by code unit instead would cut a surrogate pair in
     * half and render two replacement characters, so the walk yields whole
     * characters and advances the offset by what each one really occupies. */
    const piece = (from: number, to: number) => {
      const walked: { character: string; matched: boolean }[] = [];
      let at = from;
      for (const character of match.path.slice(from, to)) {
        walked.push({ character, matched: marked.has(at) });
        at += character.length;
      }

      return walked;
    };

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
      /* The list caps at 19rem and scrolls, and focus never leaves the field -
         so arrowing past the fold moved `aria-activedescendant` onto an option
         nobody could see. A pointer user's selection is always in view; the
         keyboard user's has to be put there. */
      requestAnimationFrame(() => {
        document.getElementById(`finder-option-${active}`)?.scrollIntoView({ block: 'nearest' });
      });

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
    class="text-input finder-input"
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

      {#if value === ''}
        <!-- What is offered before anything is typed. The list arrives held by
             most repositories first, so the head of it is the most useful thing
             to show - it used to be a `recents` prop no caller ever passed, so
             this said "No path here matches that" over an empty query. -->
        <p class="finder-band">Most repositories hold</p>
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
          <!-- The second line is the one rule that makes a path missing rather
               than merely unmatched: the index holds ordinary files, because a
               template has to be one - sync refuses to write an executable, a
               symlink or a submodule. Said here because it is the only place
               somebody would notice their script absent and have nothing to go
               on. -->
          <li class="finder-empty" role="presentation">
            <span>No path here matches that</span>
            <span class="finder-why">Executables, symlinks and submodules are never offered</span>
          </li>
        {/each}
      </ul>

      {#if partial}
        <p class="finder-partial">
          GitHub would not list one of these repositories whole, so a path it holds may be missing
          from this list. Typing one still works
        </p>
      {/if}

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
  /* The popover is `inset-inline: 0` against this, so this element's width IS
     the menu's width. Without an inline size of its own it shrank to its
     content as a flex item, and the menu came out narrower than the paths it
     was listing. */
  .finder {
    inline-size: 100%;
    max-inline-size: 34rem;
    position: relative;
  }

  /* `.text-input` and only what a path finder adds to one. It used to restate
     all ten declarations, and got the focus ring wrong doing it: an `outline`
     where every other input in the panel draws an inset box-shadow, so this was
     the one control in the panel whose focus looked different. PatternList sits
     two components away and already does it this way. */
  .finder-input {
    font-family: var(--mono);
    width: 100%;
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
    display: grid;
    font-size: var(--font-size-compact);
    gap: 0.2rem;
    padding: 0.55rem;
  }

  /* Quieter than the line above it: the reason is read second, by somebody who
     has already taken in that nothing matched. */
  .finder-why {
    font-size: var(--font-size-micro);
    opacity: 0.85;
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

  /* A limit rather than a fault: nothing here drops a path on purpose, and this
     is GitHub declining to list a very large repository whole even after the
     listing was divided around it. */
  .finder-partial {
    border-top: 1px solid var(--border-subtle);
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    margin: 0;
    padding: 0.55rem 0.75rem;
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
