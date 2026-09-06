<script lang="ts">
  import {
    defaultKeymap,
    history,
    historyKeymap,
    invertedEffects,
    isolateHistory,
    undo,
    undoDepth,
  } from '@codemirror/commands';
  import { json } from '@codemirror/lang-json';
  import { markdown } from '@codemirror/lang-markdown';
  import { yaml } from '@codemirror/lang-yaml';
  import { toml } from '@codemirror/legacy-modes/mode/toml';
  import { HighlightStyle, StreamLanguage, syntaxHighlighting } from '@codemirror/language';
  import {
    Annotation,
    Compartment,
    EditorState,
    StateEffect,
    StateField,
    Text,
    RangeSetBuilder,
    type Extension,
  } from '@codemirror/state';
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
  import {
    storeTemplateBody,
    templateBody,
    templateLineEnding,
    terminateTemplate,
  } from '../template-content';
  import type { Attachment } from 'svelte/attachments';

  const {
    value,
    lang = 'json',
    readOnly = false,
    overridden = null,
    onChange,
    onHistory,
    onFormat,
    terminalNewline = false,
    label = 'Code editor',
  }: {
    value: string;
    lang?: CodeLang;
    readOnly?: boolean;
    /** 1-indexed lines to mark as overridden - the blue gutter bar. */
    overridden?: ReadonlySet<number> | null;
    onChange: (text: string, context?: unknown) => void;
    /** How many steps the editor's own history can take back. */
    onHistory?: (depth: number) => void;
    /** Applies the current backend preview. Bound to Option/Alt+Shift+F. */
    onFormat?: () => void;
    /** Shared files store the required final newline outside the visible document. */
    terminalNewline?: boolean;
    label?: string;
  } = $props();

  /** The visible twin of Ctrl/Cmd+Z - a page button steps the same history. */
  export function undoEdit(): void {
    if (view !== null) undo(view);
  }

  /** Optional immutable caller context travels with the document through Undo/Redo. */
  export function replaceValue(
    text: string,
    context: unknown = view?.state.field(editContext),
  ): void {
    const ending = templateLineEnding(text);
    text = displayValue(text);
    if (view === null) return;
    const textChanged = text !== view.state.sliceDoc();
    const endingChanged = ending !== view.state.lineBreak;
    const contextChanged = !Object.is(context, view.state.field(editContext));
    if (!textChanged && !endingChanged && !contextChanged) return;
    view.dispatch({
      ...(textChanged
        ? { changes: { from: 0, to: view.state.doc.length, insert: editorText(text) } }
        : {}),
      effects: [
        ...(endingChanged ? [lineEnding.reconfigure(EditorState.lineSeparator.of(ending))] : []),
        ...(contextChanged ? [setEditContext.of(context)] : []),
      ],
      annotations: isolateHistory.of('full'),
    });
  }

  const editorText = (text: string): Text => Text.of(text.split(/\r\n?|\n/u));

  function displayValue(text: string): string {
    return terminalNewline ? templateBody(terminateTemplate(text)) : text;
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
      lineHeight: 'var(--leading-meta)',
      overflowX: 'auto',
      padding: 'var(--space-3) 0',
    },
    '.cm-content': {
      caretColor: 'var(--text-primary)',
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
    if (lang === 'toml') return StreamLanguage.define(toml);
    if (lang === 'markdown') return markdown();
    if (lang === 'text') return [];
    return [json(), commentPlugin];
  }

  const formatKey = {
    key: 'Shift-Alt-f',
    preventDefault: true,
    run: (): boolean => {
      if (readOnly || onFormat === undefined) return false;
      onFormat();
      return true;
    },
  };

  const holds = new Compartment();
  const accessibleName = new Compartment();
  const lineEnding = new Compartment();
  const marks = new Compartment();
  const externalValue = Annotation.define<boolean>();
  const setEditContext = StateEffect.define<unknown>();
  const editContext = StateField.define<unknown>({
    create: () => undefined,
    update(context, transaction) {
      for (const effect of transaction.effects) {
        if (effect.is(setEditContext)) context = effect.value;
      }
      return context;
    },
  });

  let view: EditorView | null = null;

  const editor: Attachment = (host) => {
    /* Re-runs of the attachment reuse the root - a host can attach one only
       once in its lifetime. */
    const shadow = host.shadowRoot ?? (host as HTMLElement).attachShadow({ mode: 'open' });
    const state = EditorState.create({
      doc: untrack(() => editorText(displayValue(value))),
      extensions: [
        lineNumbers(),
        history(),
        editContext,
        invertedEffects.of((transaction) => [
          ...(transaction.startState.lineBreak === transaction.state.lineBreak
            ? []
            : [
                lineEnding.reconfigure(
                  EditorState.lineSeparator.of(transaction.startState.lineBreak),
                ),
              ]),
          ...(Object.is(
            transaction.startState.field(editContext),
            transaction.state.field(editContext),
          )
            ? []
            : [setEditContext.of(transaction.startState.field(editContext))]),
        ]),
        keymap.of([formatKey, ...defaultKeymap, ...historyKeymap]),
        untrack(() => language()),
        syntaxHighlighting(inks),
        holds.of(frozen(untrack(() => readOnly))),
        accessibleName.of(EditorView.contentAttributes.of({ 'aria-label': untrack(() => label) })),
        lineEnding.of(EditorState.lineSeparator.of(untrack(() => templateLineEnding(value)))),
        marks.of([]),
        EditorView.updateListener.of((update) => {
          if (
            (update.docChanged ||
              update.startState.lineBreak !== update.state.lineBreak ||
              !Object.is(update.startState.field(editContext), update.state.field(editContext))) &&
            !update.transactions.some((transaction) => transaction.annotation(externalValue))
          ) {
            const text = terminalNewline
              ? storeTemplateBody(update.state.sliceDoc(), update.state.lineBreak as '\n' | '\r\n')
              : update.state.sliceDoc();
            const context = update.state.field(editContext);
            if (context === undefined) onChange(text);
            else onChange(text, context);
          }
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
    const next = displayValue(value);
    const ending = templateLineEnding(value);
    const held = view;
    if (held !== null && (next !== held.state.sliceDoc() || ending !== held.state.lineBreak)) {
      held.dispatch({
        changes: { from: 0, to: held.state.doc.length, insert: editorText(next) },
        effects: lineEnding.reconfigure(EditorState.lineSeparator.of(ending)),
        annotations: externalValue.of(true),
      });
    }
  });

  $effect(() => {
    const held = frozen(readOnly);
    view?.dispatch({ effects: holds.reconfigure(held) });
  });

  $effect(() => {
    view?.dispatch({
      effects: accessibleName.reconfigure(EditorView.contentAttributes.of({ 'aria-label': label })),
    });
  });

  $effect(() => {
    const set = overridden;
    const held = view;
    if (held !== null) held.dispatch({ effects: marks.reconfigure(markLines(held.state, set)) });
  });
</script>

<!--
@component
The composed copy as an editable surface: CodeMirror, dressed to sit
where a CodeBlock sits - same font, same line grid, same managed gutter
bar on the lines an adjustment rewrote. JSON only, because that is the
one language the merge can be derived back from.

The editor mounts inside a shadow root. That is not decoration: the
panel serves `style-src 'self'`, under which the style element
CodeMirror injects into a document head is parsed and thrown away -
silently, like every CSP style refusal. In a shadow root its style
module rides `adoptedStyleSheets`, which is script writing to the
CSSOM, and CSP does not govern that. (Spelled "style element" here
because svelte2tsx scans script comments for tags.)
-->

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
    line-height: var(--leading-meta);
  }

  .code-editor:focus-within {
    border-color: var(--focus);
    outline: var(--focus-ring-width) solid var(--focus);
    outline-offset: var(--focus-ring-inset);
  }
</style>
