<script lang="ts">
  import { tick } from 'svelte';

  import { globRuns } from '#lib/glob-runs.js';

  import Button from './Button.svelte';
  import Icon from './Icon.svelte';

  const {
    patterns,
    readOnly = false,
    onChange,
  }: {
    patterns: readonly string[];
    readOnly?: boolean;
    /** The whole list after any commit - empty entries already dropped. */
    onChange: (next: string[]) => void;
  } = $props();

  let editing = $state<number | null>(null);
  let value = $state('');
  let input: HTMLInputElement | null = $state(null);
  let timer: ReturnType<typeof setTimeout> | undefined;

  function commit(): void {
    clearTimeout(timer);
    timer = undefined;
    const open = editing;
    if (open === null) return;
    const trimmed = value.trim();
    /* An emptied entry is not written mid-thought - clearing to retype must
       not delete it under the keyboard. Closing empty is what drops it. */
    if (trimmed === '' || (patterns[open] ?? '') === trimmed) return;
    const next = [...patterns];
    next[open] = trimmed;
    onChange(next.filter((held) => held !== ''));
  }

  function close(): void {
    commit();
    const open = editing;
    editing = null;
    if (open !== null && value.trim() === '' && (patterns[open] ?? '') !== '') {
      onChange(patterns.filter((_, at) => at !== open));
    }
  }

  async function add(): Promise<void> {
    if (readOnly) return;
    /* An empty entry already open IS the new entry - Add returns the
       keyboard to it rather than minting a second blank. */
    if (editing !== null && value.trim() === '') {
      input?.focus();
      return;
    }
    commit();
    editing = patterns.length;
    value = '';
    await tick();
    input?.focus();
  }

  async function open(index: number, event?: MouseEvent): Promise<void> {
    if (readOnly || editing === index) return;
    let offset: number | null = null;
    if (event !== undefined) {
      const target = event.currentTarget as HTMLElement;
      const read =
        typeof document.caretPositionFromPoint === 'function'
          ? document.caretPositionFromPoint(event.clientX, event.clientY)
          : document.caretRangeFromPoint?.(event.clientX, event.clientY);
      if (read) {
        const node = 'offsetNode' in read ? read.offsetNode : read.startContainer;
        const at = 'offset' in read ? read.offset : read.startOffset;
        const host = node.nodeType === Node.TEXT_NODE ? node.parentElement : (node as HTMLElement);
        if (host === target || target.contains(host)) offset = at;
      }
    }
    commit();
    editing = index;
    value = patterns[index] ?? '';
    await tick();
    if (input !== null) {
      input.focus();
      const pos = Math.min(offset ?? input.value.length, input.value.length);
      input.setSelectionRange(pos, pos);
    }
  }

  function remove(index: number): void {
    if (readOnly) return;
    if (editing === index) editing = null;
    else if (editing !== null && editing > index) editing -= 1;
    onChange(patterns.filter((_, at) => at !== index));
  }

  function typed(event: Event): void {
    clearTimeout(timer);
    const type = (event as InputEvent).inputType;
    if (type === 'insertFromPaste' || type === 'insertFromDrop') commit();
    else timer = setTimeout(commit, 700);
  }

  function keys(event: KeyboardEvent): void {
    if (event.key === 'Enter' || event.key === 'Escape') close();
  }

  function outside(event: MouseEvent): void {
    const target = event.target as HTMLElement;
    /* A press whose target the open editor's own re-render detached was
       somebody's press, never "outside". */
    if (!target.isConnected) return;
    if (editing !== null && !target.closest('.pattern-entry, .pattern-add')) close();
  }

  /* The row draws the phantom entry while a fresh one is being typed. */
  const shown = $derived(
    editing !== null && editing >= patterns.length ? [...patterns, ''] : [...patterns],
  );
</script>

<!--
@component
A list of patterns edited in place: an entry is an INPUT wearing field
material, press its text to edit it where it stands, the x detaches it,
Add opens an empty one. Commits ride a 700ms debounce; pasting, Enter,
Escape and leaving the field commit at once, and an entry left empty is
dropped rather than kept blank.

Three pages share this row: labels, rulesets and files all carry a
leave-alone list, and each used to grow its own copy of the same
fifteen behaviours.
-->

<svelte:document onclick={outside} />

<span class="pattern-entries">
  {#each shown as pattern, index (index)}
    {#if editing === index}
      <span class="pattern-entry is-editing">
        <input
          class="pattern-input"
          style:inline-size="calc({Math.max(value.length, 6)}ch + 2px)"
          bind:this={input}
          bind:value
          aria-label="Pattern"
          spellcheck="false"
          oninput={typed}
          onkeydown={keys}
          onfocusout={commit}
        />
        <button
          class="pattern-del"
          aria-label="Remove {pattern === '' ? 'pattern' : pattern}"
          onclick={() => remove(index)}
        >
          <Icon name="close" size="xs" />
        </button>
      </span>
    {:else}
      <span class="pattern-entry">
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
        <!-- The metacharacters inked apart from the path they sit in: a pattern is
             read for what makes it a pattern. -->
        <span class="t" onclick={(event) => void open(index, event)}
          >{#each globRuns(pattern) as run, at (at)}{#if run.meta}<span class="glob-meta"
                >{run.text}</span
              >{:else}{run.text}{/if}{/each}</span
        >
        <button
          class="pattern-del"
          aria-label="Remove {pattern}"
          disabled={readOnly}
          onclick={() => remove(index)}
        >
          <Icon name="close" size="xs" />
        </button>
      </span>
    {/if}
  {/each}
</span>
<Button tone="quiet" class="pattern-add" disabled={readOnly} onclick={() => void add()}>
  {#snippet icon()}<Icon name="plus" size="sm" />{/snippet}
  Add
</Button>

<style>
  .pattern-entries {
    align-items: center;
    display: inline-flex;
    flex-wrap: wrap;
    /* As wide as the entries it holds - see the note above `.chip` in app.css. */
    inline-size: fit-content;
    gap: var(--space-2);
  }

  /* A pattern entry is an INPUT, not a chip: field material at the Add
     button's 34px, press the text to edit in place. */
  .pattern-entry {
    align-items: center;
    background: var(--input-bg);
    block-size: var(--control-height-compact);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    box-sizing: border-box;
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    gap: 4px;
    padding: 0 4px 0 var(--space-3);
  }

  .pattern-entry .t {
    cursor: text;
    /* Ink-true, so the words share the row's centre with the x beside them. */
    text-box: trim-both cap alphabetic;
  }

  .pattern-entry:focus-within {
    border-color: var(--focus);
    outline: var(--focus-ring-width) solid var(--focus);
    outline-offset: var(--focus-ring-inset);
  }

  .pattern-entry .pattern-input {
    background: transparent;
    border: 0;
    color: var(--text-primary);
    font: inherit;
    min-inline-size: 6ch;
    outline: none;
    padding: 0;
  }

  /* An x, not a trash, and neutral, not danger: this button DETACHES a
     value from a list - light, instantly re-typeable, nothing cascades.
     Its square keeps the space; the glyph waits for the pointer. 24px
     inside the 32px content box, so the ring around it is exactly 4 on
     every side, and its 3px corner is the field's 8 minus the 5 between
     their edges. */
  .pattern-del {
    align-items: center;
    background: transparent;
    block-size: var(--field-target-min);
    border: 0;
    border-radius: 3px;
    color: var(--text-muted);
    cursor: pointer;
    display: inline-flex;
    flex: none;
    inline-size: var(--field-target-min);
    justify-content: center;
    opacity: 0;
    padding: 0;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      color var(--duration-fast) var(--ease-standard),
      opacity var(--duration-fast) var(--ease-standard),
      translate var(--duration-press) var(--ease-standard),
      box-shadow var(--duration-press) var(--ease-standard);
  }

  .pattern-entry:hover .pattern-del,
  .pattern-entry:focus-within .pattern-del {
    opacity: 1;
  }

  .pattern-del:hover {
    background: var(--interactive-hover-layer);
    color: var(--text-primary);
  }

  .pattern-del:active {
    background: var(--interactive-pressed);
    box-shadow: var(--pressed-inset);
    translate: 0 1px;
  }
</style>
