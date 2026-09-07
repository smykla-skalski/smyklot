<script lang="ts">
  import {
    emphasizeRuns,
    tokenizeLine,
    unifiedDiff,
    type CodeLang,
    type TokenRun,
  } from '../code-tokens';
  import { diffContext, type NumberedDiffLine } from '../diff-context';
  import Button from './Button.svelte';

  const {
    before,
    after,
    lang = 'json',
    contextLines,
    label,
    labels,
  }: {
    before: string;
    after: string;
    /** The file's own language - the diff colours ride on top of its tokens. */
    lang?: CodeLang;
    /** Keep this many unchanged lines beside changes; omitted shows the full diff. */
    contextLines?: number;
    label?: string;
    /** Names the sources beside the non-color minus/plus markers. */
    labels?: { before: string; after: string };
  } = $props();

  const lines = $derived(unifiedDiff(before, after));
  const context = $derived(diffContext(lines, contextLines));
  const instanceId = $props.id();
  let expanded = $state.raw<{ source: typeof lines; starts: readonly number[] } | null>(null);

  function toggleContext(start: number): void {
    const current = expanded?.source === lines ? [...expanded.starts] : [];
    const starts = current.includes(start)
      ? current.filter((held) => held !== start)
      : [...current, start];
    expanded = { source: lines, starts };
  }

  function runsOf(line: (typeof lines)[number]): TokenRun[] {
    return emphasizeRuns(tokenizeLine(lang, line.text), line.emphasis ?? []);
  }
</script>

<!--
@component
A small unified diff, drawn from the two texts themselves. The plan page
expands a file action into one of these; nothing here scrolls a whole
file - the texts a plan carries are already the window worth reading.

Line numbers count the window, not the file, and only context lines wear
one: an added or removed line has no single number to claim, and the
glyph in the source column already says which side it is on.
-->

{#snippet sourceLine(row: NumberedDiffLine)}
  {@const line = row.line}
  <div
    class="ln"
    class:has-source-lines={contextLines !== undefined}
    class:is-add={line.op === '+'}
    class:is-del={line.op === '-'}
  >
    {#if contextLines !== undefined}<span
        class="no"
        aria-label={row.before === null
          ? undefined
          : `${labels?.before ?? 'Before'} line ${row.before}`}>{row.before ?? ''}</span
      ><span
        class="no"
        aria-label={row.after === null
          ? undefined
          : `${labels?.after ?? 'After'} line ${row.after}`}>{row.after ?? ''}</span
      >{:else}<span class="no">{line.op === ' ' ? row.index + 1 : ''}</span>{/if}<span class="src"
      ><span class="op">{line.op === ' ' ? '' : line.op}</span
      >{#each runsOf(line) as piece, at (at)}<span
          class="{piece.cls ?? ''}{piece.word === true ? ' word' : ''}">{piece.text}</span
        >{/each}</span
    >
  </div>
{/snippet}

{#if labels}
  <div class="diff-legend" aria-label="Comparison sources">
    <span><span class="diff-before" aria-hidden="true">−</span> {labels.before}</span>
    <span><span class="diff-after" aria-hidden="true">+</span> {labels.after}</span>
  </div>
{/if}
<div class="code" aria-label={label} role={label ? 'region' : undefined}>
  <pre>{#each context as row (row.kind === 'line' ? `line-${row.index}` : `gap-${row.start}`)}{#if row.kind === 'line'}{@render sourceLine(
          row,
        )}{:else}{@const open =
          expanded?.source === lines && expanded.starts.includes(row.start)}<div
          class="diff-gap"><Button
            tone="quiet"
            aria-expanded={open}
            aria-controls={`${instanceId}-context-${row.start}`}
            onclick={() => toggleContext(row.start)}
            >{open ? 'Hide' : 'Show'} {row.lines.length} unchanged {row.lines.length === 1
              ? 'line'
              : 'lines'}</Button
          ></div><div
          id={`${instanceId}-context-${row.start}`}
          hidden={!open}>{#if open}{#each row.lines as line (line.index)}{@render sourceLine(
                line,
              )}{/each}{/if}</div>{/if}{/each}</pre>
</div>

<style>
  .code {
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-ctl);
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    /* Whole per line, or N lines compound the fraction. */
    line-height: var(--leading-meta);
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

  .ln.has-source-lines {
    grid-template-columns: 2.5rem 2.5rem minmax(0, 1fr);
  }

  .ln.has-source-lines > .no {
    padding-inline: var(--space-2);
  }

  .diff-legend {
    color: var(--text-muted);
    display: flex;
    flex-wrap: wrap;
    font-size: var(--font-size-compact);
    gap: var(--space-2) var(--space-4);
    line-height: var(--leading-compact);
    margin-block-end: var(--space-2);
  }

  .diff-before {
    color: var(--diff-del-ink);
  }

  .diff-after {
    color: var(--diff-add-ink);
  }

  .diff-gap {
    display: flex;
    justify-content: center;
    min-inline-size: 0;
    white-space: normal;
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
