<script module lang="ts">
  import type { Language } from '#lib/syntax.js';

  export interface CodeLine {
    text: string;
    /** A unified diff's channel. Omitted on a context line and on plain files. */
    op?: '+' | '-';
    /** Half-open character ranges the diff found changed inside this line. */
    marks?: readonly (readonly [number, number])[];
    /** This installation overrides this line: it wears the managed bar. */
    overridden?: boolean;
    /** The line's number in the file. A diff leaves it off what it adds. */
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
   * A file, or a change to one, coloured and read rather than edited.
   *
   * Three things are said at once here and each gets its own channel, because
   * colour alone is one channel and this surface is read by people who cannot
   * separate all of it. The language is said in ink; the direction of a change
   * is said in a ground, a glyph and the gutter; and the words that actually
   * changed are said in a deeper ground of the same hue. The diff grounds are
   * drawn for code rather than borrowed from the notice tints, which are tuned
   * to sit under a sentence and leave syntax with nowhere to go.
   *
   * The file's own colouring is worked out first and the change marks are laid
   * over it, so a changed value keeps being a string. Doing it the other way
   * round loses the colouring of the one word most worth reading.
   *
   * Nothing here is `innerHTML`: a token is an element and its text is text, so
   * a value containing a `<` is a value.
   */
  import Icon from '#lib/components/Icon.svelte';
  import { tokenizeMarked } from '#lib/syntax.js';

  const {
    lines,
    language,
    label,
    onClearOverride,
  }: {
    lines: readonly CodeLine[];
    language: Language;
    /** Names the block for assistive tech - the path, usually. */
    label: string;
    /**
     * Offered on an overridden line. It removes the override so the template's
     * own value returns; it never writes a value of its own.
     */
    onClearOverride?: (line: CodeLine, at: number) => void;
  } = $props();

  const pieces = (line: CodeLine) => tokenizeMarked(line.text, language, line.marks ?? []);
</script>

<div class="code" role="figure" aria-label={label}>
  <pre>{#each lines as line, at (at)}<div
        class="ln"
        class:is-add={line.op === '+'}
        class:is-del={line.op === '-'}
        class:is-overridden={line.overridden === true}><span class="no">{line.number ?? ''}</span
        ><span class="src"
          >{#if line.op !== undefined}<span class="op">{line.op}</span
            >{/if}{#each pieces(line) as piece, index (index)}<span
              class="tok tok-{piece.kind}"
              class:is-word={piece.marked}>{piece.text}</span
            >{/each}</span
        >{#if line.overridden === true && onClearOverride !== undefined}<button
            type="button"
            class="line-clear"
            aria-label="Stop overriding line {line.number ?? at + 1}"
            title="Stop overriding this line - the template's value returns"
            onclick={() => onClearOverride(line, at)}><Icon name="close" size={12} /></button
          >{/if}</div>{/each}</pre>
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
    grid-template-columns: 3rem 1fr auto;
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

  .line-clear {
    align-self: center;
    appearance: none;
    background: none;
    border: 0;
    border-radius: 4px;
    color: var(--text-muted);
    cursor: pointer;
    display: none;
    font: inherit;
    margin-inline-end: 0.4rem;
    padding: 0.1rem 0.3rem;
  }

  /* Shown on hover, and on focus - a control only a mouse can reveal is a
     control only a mouse has. */
  .ln.is-overridden:hover .line-clear,
  .line-clear:focus-visible {
    display: inline-flex;
  }

  .line-clear:hover {
    background-image: linear-gradient(
      var(--interactive-hover-layer),
      var(--interactive-hover-layer)
    );
    color: var(--text-primary);
  }

  .ln.is-add {
    background: var(--diff-add-bg);
  }

  .ln.is-del {
    background: var(--diff-del-bg);
  }

  .ln.is-add > .no,
  .ln.is-del > .no {
    opacity: 1;
  }

  .ln.is-add > .no {
    color: var(--diff-add-ink);
  }

  .ln.is-del > .no {
    color: var(--diff-del-ink);
  }

  /* The glyph carries the direction beside the colour, in its own column so
     the code below it still lines up. */
  .op {
    display: inline-block;
    min-width: 1.1em;
  }

  .ln.is-add .op {
    color: var(--diff-add-ink);
  }

  .ln.is-del .op {
    color: var(--diff-del-ink);
  }

  .ln.is-add .tok.is-word {
    background: var(--diff-add-word);
    border-radius: 3px;
  }

  .ln.is-del .tok.is-word {
    background: var(--diff-del-word);
    border-radius: 3px;
  }
</style>
