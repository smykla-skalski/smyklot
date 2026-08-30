<script lang="ts">
  import { tokenizeLine, type CodeLang } from '../code-tokens';

  const {
    text,
    lang = 'json',
    overridden = null,
  }: {
    text: string;
    lang?: CodeLang;
    /** 1-indexed lines to mark as overridden - the blue gutter bar. */
    overridden?: ReadonlySet<number> | null;
  } = $props();

  const lines = $derived(text.replace(/\n$/, '').split('\n'));
</script>

<!--
@component
A read-only code window: the template a repository is held to, or the
copy it ends up with. Every line numbered, tokenized by the file's own
language, and - where a set of line numbers is handed in - wearing the
managed gutter bar on the lines an adjustment rewrote.
-->

<div class="code">
  <pre>{#each lines as line, index (index)}<div
        class="ln"
        class:is-overridden={overridden?.has(index + 1) === true}><span class="no">{index + 1}</span
        ><span class="src"
          >{#each tokenizeLine(lang, line) as piece, at (at)}<span class={piece.cls ?? ''}
              >{piece.text}</span
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
    /* 20px, the meta tier. Code is set at the compact size and takes the tier above
       its own, because a line of code is denser than a line of prose and needs the
       room back - and 20 is a step the scale already has rather than a figure this
       one surface invented. */
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

  /* Overridden lines wear the managed bar in the gutter. */
  .ln.is-overridden {
    background: color-mix(in srgb, var(--brand-action) 5%, transparent);
  }

  .ln.is-overridden > .no {
    border-inline-start: 3px solid var(--managed-bar);
    color: var(--brand-action-text);
    opacity: 1;
    padding-inline-start: calc(0.75rem - 3px);
  }
</style>
