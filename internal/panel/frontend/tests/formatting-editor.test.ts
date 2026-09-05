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
    await fireEvent.click(screen.getByRole('radio', { name: 'JSON' }));
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

  it('shows only common and matching file rules for a template path', () => {
    render(FormattingEditor, {
      patch: {},
      inherited: defaultFormattingPolicy(),
      scope: 'template',
      idPrefix: 'json-file',
      path: 'renovate.json',
      onChange: vi.fn(),
    });

    expect(screen.getByRole('region', { name: 'Common' })).toBeDefined();
    expect(screen.getByRole('region', { name: 'JSON' })).toBeDefined();
    expect(screen.queryByRole('region', { name: 'YAML' })).toBeNull();
    expect(screen.queryByRole('region', { name: 'TOML' })).toBeNull();
    expect(screen.queryByRole('navigation', { name: 'Formatting file type' })).toBeNull();
  });

  it('keeps JSON rules editable for JSONC output', async () => {
    const onChange = vi.fn();
    render(FormattingEditor, {
      patch: { json: { arrays: 'compact' } },
      inherited: defaultFormattingPolicy(),
      scope: 'template',
      idPrefix: 'jsonc-file',
      path: 'settings.jsonc',
      onChange,
    });
    const json = screen.getByRole('region', { name: 'JSON' });
    expect(screen.getByRole('region', { name: 'JSONC' })).toBeDefined();
    await fireEvent.click(
      within(within(json).getByRole('group', { name: 'Arrays' })).getByRole('radio', {
        name: 'Expanded',
      }),
    );
    expect(onChange).toHaveBeenLastCalledWith(
      { json: { arrays: 'expanded' } },
      'formatting.json.arrays',
    );
    expect(screen.queryByRole('region', { name: 'YAML' })).toBeNull();
  });

  it('shows no formatting controls for an unsupported file path', () => {
    render(FormattingEditor, {
      patch: {},
      inherited: defaultFormattingPolicy(),
      scope: 'template',
      idPrefix: 'plain-file',
      path: 'NOTICE',
      onChange: vi.fn(),
    });

    expect(screen.queryByText('Formatting')).toBeNull();
  });
});
