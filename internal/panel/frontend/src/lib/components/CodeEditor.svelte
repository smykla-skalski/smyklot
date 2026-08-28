<script lang="ts">
  /**
   * The composed copy as an editable surface: CodeMirror, dressed to sit
   * where a CodeBlock sits - same font, same line grid, same managed gutter
   * bar on the lines an adjustment rewrote. JSON only, because that is the
   * one language the merge can be derived back from.
   *
   * The editor mounts inside a shadow root. That is not decoration: the
   * panel serves `style-src 'self'`, under which the style element
   * CodeMirror injects into a document head is parsed and thrown away -
   * silently, like every CSP style refusal. In a shadow root its style
   * module rides `adoptedStyleSheets`, which is script writing to the
   * CSSOM, and CSP does not govern that. (Spelled "style element" here
   * because svelte2tsx scans script comments for tags.)
   */
  import { defaultKeymap, history, historyKeymap, undo, undoDepth } from '@codemirror/commands';
  import { json } from '@codemirror/lang-json';
  import { markdown } from '@codemirror/lang-markdown';
  import { yaml } from '@codemirror/lang-yaml';
  import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
  import { Compartment, EditorState, RangeSetBuilder, type Extension } from '@codemirror/state';
  import {
    Decoration,
    EditorView,
    GutterMarker,
    MatchDecorator,
    ViewPlugin,
    gutterLineClass,
    keymap,
    lineNumbers,
    type DecorationSet,
    type ViewUpdate,
  } from '@codemirror/view';
  import { tags } from '@lezer/highlight';
  import { untrack } from 'svelte';
  import type { CodeLang } from '../code-tokens';
  import type { Attachment } from 'svelte/attachments';

  const {
    value,
    lang = 'json',
    readOnly = false,
    overridden = null,
    onChange,
    onHistory,
  }: {
    value: string;
    lang?: CodeLang;
    readOnly?: boolean;
    /** 1-indexed lines to mark as overridden - the blue gutter bar. */
    overridden?: ReadonlySet<number> | null;
    onChange: (text: string) => void;
    /** How many steps the editor's own history can take back. */
    onHistory?: (depth: number) => void;
  } = $props();

  /** The visible twin of Ctrl/Cmd+Z - a page button steps the same history. */
  export function undoEdit(): void {
    if (view !== null) undo(view);
  }

  /** Replace the document in one CodeMirror transaction so one Undo restores it. */
  export function replaceValue(text: string): void {
    if (view === null || text === view.state.doc.toString()) return;
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } });
  }

  /* The same inks CodeBlock's tokenizer classes wear, on lezer's tags. */
  const inks = HighlightStyle.define([
    { tag: tags.propertyName, color: 'var(--code-key)' },
    { tag: tags.string, color: 'var(--code-string)' },
    { tag: [tags.number, tags.bool, tags.null], color: 'var(--code-const)' },
    { tag: [tags.punctuation, tags.separator, tags.bracket], color: 'var(--code-punct)' },
    { tag: tags.comment, color: 'var(--code-comment)', fontStyle: 'italic' },
    { tag: tags.atom, color: 'var(--code-const)' },
    { tag: tags.heading, color: 'var(--code-key)', fontWeight: '600' },
    { tag: tags.emphasis, fontStyle: 'italic' },
    { tag: tags.strong, fontWeight: '600' },
  ]);

  /* lang-json is strict JSON, so the comments the templates carry parse as
     errors and would sit unstyled. The service reads them as comments; so
     does this ink. Strings are matched first so a // inside one stays a
     string. */
  const commentInk = new MatchDecorator({
    regexp: /("(?:[^"\\]|\\.)*")|(\/\/.*)/g,
    decorate: (add, from, to, match) => {
      if (match[2] !== undefined) {
        add(from + (match[1]?.length ?? 0), to, Decoration.mark({ class: 'cm-jsonc-comment' }));
      }
    },
  });

  const commentPlugin = ViewPlugin.fromClass(
    class {
      decorations: DecorationSet;
      constructor(view: EditorView) {
        this.decorations = commentInk.createDeco(view);
      }
      update(update: ViewUpdate) {
        this.decorations = commentInk.updateDeco(update, this.decorations);
      }
    },
    { decorations: (plugin) => plugin.decorations },
  );

  const overriddenLine = Decoration.line({ class: 'cm-overridden' });

  class OverriddenNumber extends GutterMarker {
    elementClass = 'cm-overridden-no';
  }

  const overriddenNumber = new OverriddenNumber();

  function markLines(state: EditorState, set: ReadonlySet<number> | null): Extension {
    const lines = [...(set ?? [])]
      .filter((at) => at >= 1 && at <= state.doc.lines)
      .sort((a, b) => a - b);
    const decos = new RangeSetBuilder<Decoration>();
    const numbers = new RangeSetBuilder<GutterMarker>();
    for (const at of lines) {
      const line = state.doc.line(at);
      decos.add(line.from, line.from, overriddenLine);
      numbers.add(line.from, line.from, overriddenNumber);
    }
    return [EditorView.decorations.of(decos.finish()), gutterLineClass.of(numbers.finish())];
  }

  function frozen(held: boolean): Extension {
    return [EditorState.readOnly.of(held), EditorView.editable.of(!held)];
  }

  const surface = EditorView.theme({
    '&': {
      fontFamily: 'var(--mono)',
      fontSize: 'var(--font-size-compact)',
    },
    '.cm-scroller': {
      fontFamily: 'inherit',
      lineHeight: 'round(1.65em, 1px)',
      overflowX: 'auto',
      padding: 'var(--space-3) 0',
    },
    '.cm-content': {
      caretColor: 'var(--text)',
      padding: '0',
    },
    '.cm-line': {
      padding: '0 var(--space-3) 0 0',
    },
    '.cm-gutters': {
      background: 'transparent',
      border: 'none',
      color: 'var(--text-muted)',
    },
    '.cm-lineNumbers .cm-gutterElement': {
      boxSizing: 'border-box',
      fontVariantNumeric: 'tabular-nums',
      minWidth: '3rem',
      opacity: '0.6',
      padding: '0 0.75rem',
      userSelect: 'none',
    },
    '&.cm-focused': {
      outline: 'none',
    },
    '.cm-jsonc-comment': {
      color: 'var(--code-comment)',
      fontStyle: 'italic',
    },
    '.cm-overridden': {
      background: 'color-mix(in srgb, var(--brand-action) 5%, transparent)',
    },
    '.cm-lineNumbers .cm-gutterElement.cm-overridden-no': {
      borderInlineStart: '3px solid var(--managed-bar)',
      color: 'var(--brand-action-text)',
      opacity: '1',
      paddingInlineStart: 'calc(0.75rem - 3px)',
    },
  });

  /* The language never changes for a mounted surface - a path keeps its
     extension - so it is not a compartment. lang-json is strict, which is
     why the json surface also carries the comment ink above. */
  function language(): Extension {
    if (lang === 'yaml') return yaml();
    if (lang === 'markdown') return markdown();
    return [json(), commentPlugin];
  }

  const holds = new Compartment();
  const marks = new Compartment();

  let view: EditorView | null = null;

  const editor: Attachment = (host) => {
    /* Re-runs of the attachment reuse the root - a host can attach one only
       once in its lifetime. */
    const shadow = host.shadowRoot ?? (host as HTMLElement).attachShadow({ mode: 'open' });
    const state = EditorState.create({
      doc: untrack(() => value),
      extensions: [
        lineNumbers(),
        history(),
        keymap.of([...defaultKeymap, ...historyKeymap]),
        untrack(() => language()),
        syntaxHighlighting(inks),
        holds.of(frozen(untrack(() => readOnly))),
        marks.of([]),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) onChange(update.state.doc.toString());
          onHistory?.(undoDepth(update.state));
        }),
        surface,
      ],
    });
    const created = new EditorView({ state, parent: shadow, root: shadow });
    created.dispatch({
      effects: marks.reconfigure(
        markLines(
          created.state,
          untrack(() => overridden),
        ),
      ),
    });
    view = created;
    return () => {
      created.destroy();
      view = null;
    };
  };

  /* The skill-book pattern: the instance is created once, and each piece of
     state it mirrors gets its own effect that dispatches rather than
     recreating the editor. */
  $effect(() => {
    const next = value;
    const held = view;
    if (held !== null && next !== held.state.doc.toString()) {
      held.dispatch({ changes: { from: 0, to: held.state.doc.length, insert: next } });
    }
  });

  $effect(() => {
    const held = frozen(readOnly);
    view?.dispatch({ effects: holds.reconfigure(held) });
  });

  $effect(() => {
    const set = overridden;
    const held = view;
    if (held !== null) held.dispatch({ effects: marks.reconfigure(markLines(held.state, set)) });
  });
</script>

<div class="code-editor" {@attach editor}></div>

<style>
  /* The CodeBlock's shell, worn by the host; the editor inside inherits the
     font through the shadow boundary via the custom properties. */
  .code-editor {
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-ctl);
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    line-height: round(1.65em, 1px);
  }

  .code-editor:focus-within {
    border-color: var(--focus);
    outline: 2px solid var(--focus);
    outline-offset: -1px;
  }
</style>
