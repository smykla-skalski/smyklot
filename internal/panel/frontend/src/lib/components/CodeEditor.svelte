<script lang="ts">
  /**
   * The one surface on the sync pages a person types code into.
   *
   * A coloured `CodeBlock` underneath and a transparent `<textarea>` over it,
   * sharing one set of font metrics. The editor is the same picture as the
   * reader because it IS the reader: a change to how a string is coloured, or
   * to the bar an overridden line wears, lands on both at once, and there is no
   * second highlighter to disagree with the first.
   *
   * The alternative was CodeMirror 6, which the proposal named before
   * `CodeBlock` existed. It would have brought its own highlighter, its own
   * theme to keep in step with these tokens, and ~130 KB to a dependency tree
   * that carries five runtime packages - to do what forty lines and the
   * component already here do. The native textarea also brings undo, spell
   * checking off, selection, and the platform's own keyboard for free.
   *
   * The textarea is the scroller and the picture below follows it. Two
   * independent scrollers is how an overlay editor comes to draw one line and
   * hold another.
   */
  import CodeBlock, { codeLines } from './CodeBlock.svelte';
  import type { Language } from '#lib/syntax.js';

  let {
    value = $bindable(''),
    language,
    label,
    overridden = [],
    disabled = false,
    rows = 14,
  }: {
    value?: string;
    language: Language;
    /** Names the field - the path being edited. */
    label: string;
    /** Line numbers, 1-based, this installation's own rather than the template's. */
    overridden?: readonly number[];
    disabled?: boolean;
    /** How tall it stands before it starts scrolling. */
    rows?: number;
  } = $props();

  let frame = $state<HTMLDivElement | null>(null);

  /* `codeLines`, because the picture under the caret is drawn by `CodeBlock`
     and cutting the text differently from it is how the two come to disagree
     about how many lines there are. It owns the trailing-newline rule. */
  const lines = $derived(codeLines(value, overridden));

  /** The picture follows the caret rather than scrolling on its own. */
  function follow(event: Event): void {
    const area = event.currentTarget as HTMLTextAreaElement;
    const code = frame?.querySelector<HTMLElement>('.code');
    if (code === null || code === undefined) return;
    code.scrollLeft = area.scrollLeft;
    code.scrollTop = area.scrollTop;
  }
</script>

<div class="editor" bind:this={frame} style:--editor-rows={rows}>
  <!-- Decoration for the field over it: everything here is said again in the
       textarea's own value, which is what a screen reader reads. -->
  <div class="mirror" aria-hidden="true">
    <CodeBlock {lines} {language} {label} />
  </div>
  <textarea
    class="input"
    aria-label={label}
    spellcheck="false"
    autocomplete="off"
    autocapitalize="off"
    wrap="off"
    {disabled}
    bind:value
    onscroll={follow}></textarea>
</div>

<style>
  .editor {
    /* One number for both layers: the picture and the field are the same box,
       and a height set on either alone is how they come apart. */
    block-size: calc(var(--editor-rows) * 1.65em + 2 * var(--space-3) + 2px);
    font-size: var(--font-size-compact);
    position: relative;
  }

  .mirror,
  .input {
    inset: 0;
    position: absolute;
  }

  /* The textarea scrolls; the picture is moved to match, so it must not have a
     scroller of its own to disagree with. */
  .mirror :global(.code) {
    block-size: 100%;
    overflow: hidden;
  }

  .input {
    background: none;
    border: 1px solid transparent;
    border-radius: var(--r-ctl);
    /* Visible caret, invisible text: the colours below it are the picture. */
    caret-color: var(--text-primary);
    color: transparent;
    font-family: var(--mono);
    font-size: inherit;
    line-height: 1.65;
    overflow: auto;
    /* The gutter is a column of the picture, so the field's text starts where
       that column ends. */
    padding: var(--space-3) var(--space-3) var(--space-3) 3rem;
    resize: none;
    white-space: pre;
  }

  .input::selection {
    /* The text is transparent, so a selection is drawn as a ground alone. */
    background: color-mix(in srgb, var(--brand-action) 30%, transparent);
  }

  .input:focus-visible {
    border-color: var(--focus);
    outline: 2px solid var(--focus);
    outline-offset: -1px;
  }

  .input:disabled {
    cursor: not-allowed;
  }
</style>
