<script module lang="ts">
  import type { Language } from '#lib/syntax.js';

  export interface CodeLine {
    text: string;
    /** This installation overrides this line: it wears the managed bar. */
    overridden?: boolean;
    /** The line's number in the file. */
    number?: number;
  }

  /**
   * A whole file's text, cut into the lines this draws.
   *
   * Here rather than in each caller because the trailing-newline rule is easy
   * to write twice and get right once: a file ends with one, and splitting on
   * it leaves a final empty piece that would be drawn as a line which is there
   * and is not. The editor and the file page had a copy each.
   */
  export function codeLines(text: string, overridden: readonly number[] = []): CodeLine[] {
    const marked = new Set(overridden);

    return text
      .split('\n')
      .slice(0, text.endsWith('\n') ? -1 : undefined)
      .map((line, at) => ({ text: line, number: at + 1, overridden: marked.has(at + 1) }));
  }
</script>

<script lang="ts">
  /**
   * A file, coloured and read rather than edited.
   *
   * Two things are said at once here and each gets its own channel, because
   * colour alone is one channel and this surface is read by people who cannot
   * separate all of it. The language is said in ink; a line this installation
   * decides is said in a ground and in a bar down the gutter, so an override is
   * legible with the ink turned off.
   *
   * Nothing here is `innerHTML`: a token is an element and its text is text, so
   * a value containing a `<` is a value.
   */
  import { tokenize } from '#lib/syntax.js';

  const {
    lines,
    language,
    label,
  }: {
    lines: readonly CodeLine[];
    language: Language;
    /** Names the block for assistive tech - the path, usually. */
    label: string;
  } = $props();
</script>

<div class="code" role="figure" aria-label={label}>
  <pre>{#each lines as line, at (at)}<div
        class="ln"
        class:is-overridden={line.overridden === true}><span class="no">{line.number ?? ''}</span
        ><span class="src"
          >{#each tokenize(line.text, language) as piece, index (index)}<span
              class="tok-{piece.kind}">{piece.text}</span
            >{/each}</span
        ></div>{/each}</pre>
</div>

<style>
  .code {
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-ctl);
    font-size: var(--font-size-compact);
    line-height: 1.65;
    overflow-x: auto;
    padding: var(--space-3) 0;
  }

  pre {
    font-family: var(--mono);
    margin: 0;
  }

  .ln {
    display: grid;
    grid-template-columns: 3rem 1fr;
  }

  .no {
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
    opacity: 0.6;
    padding-inline: 0.75rem;
    text-align: end;
    user-select: none;
  }

  .src {
    padding-inline-end: var(--space-3);
    white-space: pre;
  }

  .tok-key {
    color: var(--code-key);
  }

  .tok-const {
    color: var(--code-const);
  }

  .tok-string {
    color: var(--code-string);
  }

  .tok-comment {
    color: var(--code-comment);
    font-style: italic;
  }

  .tok-punct {
    color: var(--code-punct);
  }

  .tok-heading {
    color: var(--code-key);
    font-weight: 600;
  }

  /* A line this installation decides, wearing the same gutter bar an editor
     puts beside a setting it has overridden. */
  .ln.is-overridden {
    background: color-mix(in srgb, var(--managed-bar) 5%, transparent);
  }

  .ln.is-overridden > .no {
    border-inline-start: 3px solid var(--managed-bar);
    color: var(--brand-action-text);
    opacity: 1;
    padding-inline-start: calc(0.75rem - 3px);
  }
</style>
