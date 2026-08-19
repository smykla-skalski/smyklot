<script lang="ts">
  /**
   * One named thing in a list of them, and the way into it.
   *
   * A ruleset and a shared file are the same shape to read: a name somebody
   * recognises, one line saying what it is, and what it would do to the fleet.
   * Two levels deep and no deeper - the row opens the thing's own page rather
   * than expanding into it, because a list that expands is a list whose rows
   * stop being comparable the moment one is opened.
   *
   * The name is mono, always: every one of these is a string somebody has to
   * match character for character against something on GitHub.
   */
  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';

  const {
    name,
    href,
    summary,
    pill,
    mark,
    action,
  }: {
    name: string;
    /** Where the thing's own page is. Without one the row is a statement, not a way in. */
    href?: string;
    /** What it is, in one line - "default branch · 6 rules · 2 bypass actors". */
    summary?: string;
    /** A fact worn beside the name - enforcement, merge strategy. */
    pill?: Snippet;
    /** What it would do to the fleet, at the end of the row. */
    mark?: Snippet;
    /** A control in the mark's place, for a row that is not a link. */
    action?: Snippet;
  } = $props();
</script>

<svelte:element
  this={href === undefined ? 'div' : 'a'}
  class="object-row"
  class:is-link={href !== undefined}
  {href}
>
  <span class="object-main">
    <span class="object-name-row">
      <span class="object-name band-trim">{name}</span>
      {#if pill !== undefined}
        {@render pill()}
      {/if}
    </span>
    {#if summary !== undefined}
      <span class="object-sum band-trim">{summary}</span>
    {/if}
  </span>
  <span class="object-side">
    {#if mark !== undefined}
      {@render mark()}
    {/if}
    {#if action !== undefined}
      {@render action()}
    {/if}
    {#if href !== undefined}
      <Icon name="chevron-right" size={13} />
    {/if}
  </span>
</svelte:element>

<style>
  .object-row {
    align-items: center;
    border-radius: var(--r-ctl);
    color: inherit;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    padding: 0.6rem var(--space-2);
    text-decoration: none;
  }

  .is-link {
    cursor: pointer;
  }

  .is-link:hover {
    background-image: linear-gradient(
      var(--interactive-hover-layer),
      var(--interactive-hover-layer)
    );
  }

  .is-link:active {
    background-image: linear-gradient(var(--press), var(--press));
    transform: scale(var(--press-scale-surface));
  }

  .is-link:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: -1px;
  }

  .object-main {
    display: grid;
    gap: 0.25rem;
    min-width: 0;
  }

  .object-name-row {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  /* Both lines trimmed, through the shared class. The name was and the summary
     under it was not, so the stack kept the summary's descender space below its
     baseline and sat 0.30px off the column beside it. */
  .object-name {
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-weight: 500;
    overflow-wrap: anywhere;
  }

  .object-sum {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .object-side {
    align-items: center;
    color: var(--text-muted);
    display: flex;
    flex: none;
    gap: var(--space-2);
  }
</style>
