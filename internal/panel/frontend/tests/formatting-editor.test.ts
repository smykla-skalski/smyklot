// @vitest-environment jsdom
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import FormattingEditor from '../src/lib/components/FormattingEditor.svelte';
import { defaultFormattingPolicy } from '../src/lib/formatting';

describe('FormattingEditor [Component]', () => {
  beforeEach(() => {
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  it('stages a preset and explicit preserve as distinct sparse leaves', async () => {
    const onChange = vi.fn();
    const { rerender } = render(FormattingEditor, {
      patch: {},
      inherited: defaultFormattingPolicy(),
      scope: 'target',
      idPrefix: 'editor',
      onChange,
    });

    await fireEvent.click(screen.getByRole('radio', { name: 'Conventional' }));
    expect(onChange).toHaveBeenLastCalledWith({ preset: 'conventional' }, 'formatting.preset');

    await rerender({
      patch: { preset: 'conventional' },
      inherited: defaultFormattingPolicy(),
      scope: 'target',
      idPrefix: 'editor',
      onChange,
    });
    const arrays = screen.getAllByRole('group', { name: 'Arrays' })[0]!;
    await fireEvent.click(within(arrays).getByRole('radio', { name: 'Preserve' }));
    expect(onChange).toHaveBeenLastCalledWith(
      { preset: 'conventional', json: { arrays: 'preserve' } },
      'formatting.json.arrays',
    );
  });

  it('keeps invalid bounded integers out of the typed draft and reports validity', async () => {
    const onChange = vi.fn();
    const onValidity = vi.fn();
    render(FormattingEditor, {
      patch: {},
      inherited: defaultFormattingPolicy(),
      scope: 'target',
      idPrefix: 'numbers',
      onChange,
      onValidity,
    });
    const width = screen.getByLabelText('Indent Width');

    await fireEvent.input(width, { target: { value: '0' } });
    expect(width.getAttribute('aria-invalid')).toBe('true');
    expect(onChange).not.toHaveBeenCalled();
    expect(onValidity).toHaveBeenLastCalledWith(false);

    await fireEvent.input(width, { target: { value: '4' } });
    expect(width.getAttribute('aria-invalid')).toBeNull();
    expect(onChange).toHaveBeenLastCalledWith(
      { common: { indent_width: 4 } },
      'formatting.common.indent_width',
    );
    expect(onValidity).toHaveBeenLastCalledWith(true);
  });

  it('marks only supplied formatting leaves as unsaved', () => {
    render(FormattingEditor, {
      patch: { common: { indent_width: 4, line_width: 120 } },
      inherited: defaultFormattingPolicy(),
      scope: 'target',
      idPrefix: 'dirty',
      dirtyKeys: ['formatting.common.line_width'],
      onChange: vi.fn(),
    });

    expect(screen.getByLabelText('Line Width').closest('[data-unsaved]')).not.toBeNull();
    expect(screen.getByLabelText('Indent Width').closest('[data-unsaved]')).toBeNull();
  });
});
