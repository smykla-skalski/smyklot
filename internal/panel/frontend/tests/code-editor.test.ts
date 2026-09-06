// @vitest-environment jsdom
import { redo } from '@codemirror/commands';
import { EditorView } from '@codemirror/view';
import { render } from '@testing-library/svelte';
import { tick } from 'svelte';
import { describe, expect, it, vi } from 'vitest';

import CodeEditor from '../src/lib/components/CodeEditor.svelte';

describe('code editor line endings [Component]', () => {
  it.each(['first\r\nsecond\r\n', 'single\r\n'])(
    'restores line endings through format, Undo and Redo: %j',
    async (value) => {
      const changed = vi.fn();
      const result = render(CodeEditor, {
        value,
        lang: 'text',
        terminalNewline: true,
        onChange: changed,
      });
      await tick();
      const host = result.container.querySelector('.code-editor')!;
      const view = EditorView.findFromDOM(host.shadowRoot!.querySelector('.cm-content')!)!;
      const formatted = value.replaceAll('\r\n', '\n');
      result.component.replaceValue(formatted);
      expect(changed).toHaveBeenLastCalledWith(formatted);
      result.component.undoEdit();
      expect(changed).toHaveBeenLastCalledWith(value);
      redo(view);
      expect(changed).toHaveBeenLastCalledWith(formatted);
    },
  );
  it.each(['first\r\nsecond\r\n', 'single\r\n', 'first\nsecond\n'])(
    'preserves the original line separator through editing and restoration: %j',
    async (value) => {
      const changed = vi.fn();
      const result = render(CodeEditor, {
        value,
        lang: 'text',
        terminalNewline: true,
        onChange: changed,
      });
      await tick();
      const host = result.container.querySelector('.code-editor')!;
      const view = EditorView.findFromDOM(host.shadowRoot!.querySelector('.cm-content')!)!;
      expect(view.state.doc.lines).toBe(value.includes('second') ? 2 : 1);
      view.dispatch({ changes: { from: 0, insert: '!' } });
      view.dispatch({ changes: { from: 0, to: 1 } });
      expect(changed).toHaveBeenLastCalledWith(value);
    },
  );
});
