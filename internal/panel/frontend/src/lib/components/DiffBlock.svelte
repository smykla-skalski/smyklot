<script lang="ts">
  /**
   * A small unified diff, drawn from the two texts themselves. The plan page
   * expands a file action into one of these; nothing here scrolls a whole
   * file - the texts a plan carries are already the window worth reading.
   *
   * Line numbers count the window, not the file, and only context lines wear
   * one: an added or removed line has no single number to claim, and the
   * glyph in the source column already says which side it is on.
   */
  import {
    emphasizeRuns,
    tokenizeLine,
    unifiedDiff,
    type CodeLang,
    type TokenRun,
  } from '../code-tokens';

  const {
    before,
    after,
    lang = 'json',
  }: {
    before: string;
    after: string;
    /** The file's own language - the diff colours ride on top of its tokens. */
    lang?: CodeLang;
  } = $props();

  const lines = $derived(unifiedDiff(before, after));

  function runsOf(line: (typeof lines)[number]): TokenRun[] {
    return emphasizeRuns(tokenizeLine(lang, line.text), line.emphasis ?? []);
  }
</script>

<div class="code">
  <pre>{#each lines as line, index (index)}<div
        class="ln"
        class:is-add={line.op === '+'}
        class:is-del={line.op === '-'}><span class="no">{line.op === ' ' ? index + 1 : ''}</span
        ><span class="src"
          ><span class="op">{line.op === ' ' ? '' : line.op}</span
          >{#each runsOf(line) as piece, at (at)}<span
              class="{piece.cls ?? ''}{piece.word === true ? ' word' : ''}">{piece.text}</span
            >{/each}</span
        ></div>{/each}</pre>
</div>

<style>
  .code {
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-ctl);
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    /* Whole per line, or N lines compound the fraction. */
    line-height: round(1.65em, 1px);
    overflow-x: auto;
    padding: var(--space-3) 0;
  }

  .code pre {
    font: inherit;
    margin: 0;
  }

  .ln {
    display: grid;
    grid-template-columns: 3rem 1fr auto;
  }

  .ln > .no {
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
    opacity: 0.6;
    padding-inline: 0.75rem;
    text-align: end;
    user-select: none;
  }

  .ln > .src {
    padding-inline-end: var(--space-3);
    white-space: pre;
  }

  .tok-key {
    color: var(--code-key);
  }

  .tok-const {
    color: var(--code-const);
  }

  .tok-str {
    color: var(--code-string);
  }

  .tok-com {
    color: var(--code-comment);
    font-style: italic;
  }

  .tok-pun {
    color: var(--code-punct);
  }

  .tok-head {
    color: var(--code-key);
    font-weight: 600;
  }

  /* Diff lines (unified). The glyph carries the channel beside the colour. */
  .ln.is-add {
    background: var(--diff-add-bg);
  }

  .ln.is-del {
    background: var(--diff-del-bg);
  }

  .ln.is-add > .no {
    color: var(--diff-add-ink);
    opacity: 1;
  }

  .ln.is-del > .no {
    color: var(--diff-del-ink);
    opacity: 1;
  }

  .ln .op {
    display: inline-block;
    min-width: 1.1em;
  }

  .ln.is-add .op {
    color: var(--diff-add-ink);
  }

  .ln.is-del .op {
    color: var(--diff-del-ink);
  }

  .ln.is-add .word {
    background: var(--diff-add-word);
    border-radius: 3px;
  }

  .ln.is-del .word {
    background: var(--diff-del-word);
    border-radius: 3px;
  }
</style>
