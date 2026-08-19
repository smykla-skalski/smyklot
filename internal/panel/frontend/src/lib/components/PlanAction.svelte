<script module lang="ts">
  /** What would happen to one thing, in the three words a plan ever needs. */
  export type PlanOp = 'add' | 'change' | 'remove';
</script>

<script lang="ts">
  /**
   * One line of a plan: what would happen, to what kind of thing, to which one.
   *
   * The verb is a word and a glyph, in three fixed columns, so a plan is read
   * down its first column - twelve adds and one removal is a shape, not a
   * sentence to be assembled from thirteen rows. `remove` is the only one that
   * carries weight in ink and in stroke, because it is the only one that takes
   * something away, and a plan is approved by someone scanning for exactly that.
   *
   * `from → to` is the whole reason a change row exists. A row that says only
   * "squash merging" leaves the reader to open the repository to find out which
   * way it would go.
   */
  const {
    op,
    kind,
    what,
    detail,
    failure,
  }: {
    op: PlanOp;
    /** Which kind of thing - labels, settings, rulesets, files. */
    kind: string;
    /** The thing itself, in the name a person gave it. */
    what: string;
    /** What would change about it: `off → on`, or how it would arrive. */
    detail?: string;
    /** Why it did not happen, said on the row rather than in a drill-down. */
    failure?: string;
  } = $props();

  const WORD: Record<PlanOp, string> = { add: '+ add', change: '~ change', remove: '− remove' };
</script>

<div class="action-row">
  <div class="action-row-line">
    <span class="action-op is-{op}">{WORD[op]}</span>
    <span class="action-kind">{kind}</span>
    <span class="action-what"
      >{what}{#if detail !== undefined}<span class="from-to">{detail}</span>{/if}</span
    >
  </div>
  {#if failure !== undefined}
    <p class="action-fail">{failure}</p>
  {/if}
</div>

<style>
  .action-row {
    border-radius: var(--r-ctl);
    display: grid;
    font-size: var(--font-size-compact);
    gap: var(--space-2);
    padding: 0.45rem var(--space-2);
  }

  .action-row:hover {
    background: var(--table-row-hover);
  }

  .action-row-line {
    align-items: baseline;
    display: grid;
    gap: var(--space-3);
    grid-template-columns: 4.2rem 5.2rem 1fr;
  }

  .action-op {
    font-family: var(--mono);
    font-variant-numeric: tabular-nums;
  }

  .action-op.is-add {
    color: var(--diff-add-ink);
  }

  .action-op.is-change {
    color: var(--diff-chg-ink);
  }

  /* The one that takes something away, said in weight as well as in ink. */
  .action-op.is-remove {
    color: var(--diff-del-ink);
    font-weight: 600;
  }

  .action-kind {
    color: var(--text-muted);
  }

  .action-what {
    font-family: var(--mono);
    min-width: 0;
    overflow-wrap: anywhere;
  }

  /* The gap is drawn rather than typed: Svelte trims whitespace at the edge of
     an element, so a space written in the markup here is one that disappears
     and joins the name to its own change. */
  .from-to {
    color: var(--text-muted);
    margin-inline-start: 0.45em;
  }

  /* On the row, never only in a drill-down: a reader approving a plan has to
     be able to see what did not work without opening anything. */
  .action-fail {
    color: var(--danger);
    font-size: var(--font-size-micro);
    margin: 0;
    padding-inline-start: calc(4.2rem + 5.2rem + var(--space-3) * 2);
  }

  .action-diff {
    padding-inline-start: calc(4.2rem + var(--space-3));
  }

  @media (max-width: 40rem) {
    .action-row-line {
      grid-template-columns: 4.2rem 1fr;
    }

    .action-kind {
      grid-row: 2;
    }

    .action-fail,
    .action-diff {
      padding-inline-start: 0;
    }
  }
</style>
