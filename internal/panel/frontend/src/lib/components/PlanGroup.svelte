<script lang="ts">
  /**
   * One repository's share of a plan, folded until it is wanted.
   *
   * A plan is grouped by repository rather than by kind because that is the
   * unit a person is answerable for: "what would happen to `af`" is a question
   * with an owner, and "every label change everywhere" is not. The counts on
   * the closed summary are what make folding honest - a group can be left shut
   * and still read, which a chevron alone never allows.
   *
   * `<details>` rather than a hand-rolled disclosure: it is open before the
   * script runs, it is searchable by the browser's own find, and its open state
   * survives a print.
   */
  import Icon from '#lib/components/Icon.svelte';

  const {
    repository,
    added = 0,
    changed = 0,
    removed = 0,
    open = false,
    children,
  }: {
    repository: string;
    added?: number;
    changed?: number;
    removed?: number;
    /** The first group opens; a reader who wants the rest asks for them. */
    open?: boolean;
    children: import('svelte').Snippet;
  } = $props();
</script>

<details class="repo-group" {open}>
  <summary>
    <Icon name="chevron-right" size={11} />
    <span class="repo-group-name">{repository}</span>
    <span class="repo-group-counts">
      {#if added > 0}<span class="count-add">+{added}</span>{/if}
      {#if changed > 0}<span class="count-change">~{changed}</span>{/if}
      {#if removed > 0}<span class="count-remove">−{removed}</span>{/if}
    </span>
  </summary>
  <div class="action-rows">
    {@render children()}
  </div>
</details>

<style>
  /* Clipped, so the summary's hover fill stops at the rounded corner instead of
     painting its own square one over it. `clip` rather than `hidden`: this is
     not a scroll container and should not become one. */
  .repo-group {
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    overflow: clip;
  }

  .repo-group + :global(.repo-group) {
    margin-top: var(--space-3);
  }

  summary {
    align-items: center;
    cursor: pointer;
    display: flex;
    gap: var(--space-3);
    list-style: none;
    padding: 0.7rem var(--space-4);
  }

  summary::-webkit-details-marker {
    display: none;
  }

  summary:hover {
    background: var(--table-row-hover);
  }

  /* The same press every other row in the panel takes. */
  summary:active {
    background: var(--table-row-pressed);
    transform: scale(var(--press-scale-surface));
  }

  summary :global(svg) {
    color: var(--text-muted);
    transition: rotate var(--duration-fast) var(--ease-standard);
  }

  .repo-group[open] summary :global(svg) {
    rotate: 90deg;
  }

  .repo-group-name {
    flex: 1;
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    font-weight: 500;
  }

  .repo-group-counts {
    color: var(--text-muted);
    display: flex;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    gap: var(--space-3);
  }

  .count-add {
    color: var(--diff-add-ink);
  }

  .count-change {
    color: var(--diff-chg-ink);
  }

  .count-remove {
    color: var(--diff-del-ink);
    font-weight: 600;
  }

  .action-rows {
    border-top: 1px solid var(--border-subtle);
    display: grid;
    padding: var(--space-2);
  }

  @media (prefers-reduced-motion: reduce) {
    summary :global(svg) {
      transition: none;
    }
  }
</style>
