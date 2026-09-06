// @vitest-environment jsdom
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import FormattingEditor from '../src/lib/components/FormattingEditor.svelte';
import { defaultFormattingPolicy, formattingSources } from '../src/lib/formatting';
import type { SyncFileFormattingResolution } from '../src/lib/sync-file-render.generated';

describe('FormattingEditor [Component]', () => {
  beforeEach(() => {
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  it('uses the shared content-width file type control', () => {
    render(FormattingEditor, {
      patch: {},
      inherited: defaultFormattingPolicy(),
      scope: 'target',
      idPrefix: 'width',
      onChange: vi.fn(),
    });
    const types = screen.getByRole('group', { name: 'Formatting file type' });
    expect(types.classList.contains('fluid')).toBe(false);
    expect(within(types).getAllByRole('radio')).toHaveLength(6);
  });

  it('explains priority without inventing saved states on ordinary settings pages', () => {
    const { container } = render(FormattingEditor, {
      patch: {},
      inherited: defaultFormattingPolicy(),
      scope: 'repository',
      idPrefix: 'sources',
      onChange: vi.fn(),
    });
    const disclosure = container.querySelector('details')!;
    expect(disclosure.open).toBe(false);
    expect(disclosure.querySelector('summary')?.textContent).toContain('How formatting is chosen');
    expect(disclosure.textContent).toContain('Repository config');
    expect(disclosure.textContent).not.toMatch(/Inherited|Saved|Active|In use/u);
    expect(disclosure.querySelector('.origin-current')?.textContent).toContain(
      'Repository settings',
    );
  });

  it('keeps unsaved removals and skipped sources distinct from the current editor', () => {
    const policy = defaultFormattingPolicy();
    const resolution: SyncFileFormattingResolution = {
      current_layer: 'repository_path',
      inherited_policy: policy,
      effective_policy: policy,
      provenance: formattingSources('process'),
      layers: [
        { source: 'process', state: 'baseline' },
        { source: 'target', state: 'absent' },
        { source: 'repository_file', state: 'bypassed', config_path: '.github/smyklot.yaml' },
        { source: 'repository_panel', state: 'draft' },
        { source: 'template', state: 'stored' },
        { source: 'repository_path', state: 'absent' },
      ],
    };
    const { container } = render(FormattingEditor, {
      patch: {},
      inherited: policy,
      scope: 'path',
      idPrefix: 'sources',
      path: 'renovate.json',
      resolution,
      onChange: vi.fn(),
    });
    const disclosure = container.querySelector('details')!;
    expect(disclosure.querySelectorAll('li')).toHaveLength(5);
    expect(disclosure.textContent).not.toContain('Workspace defaults');
    const rows = [...disclosure.querySelectorAll('li')];
    expect(rows[1]?.textContent).toContain('Not used');
    expect(rows[1]?.textContent).toContain('.github/smyklot.yaml');
    expect(rows[2]?.textContent).toContain('Unsaved changes');
    expect(rows[4]?.textContent).toContain('Editing here');
    expect(rows[4]?.textContent).toContain('No overrides');
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

  it.each([{}, { common: { indent_width: 2 } }])(
    'restores the saved numeric leaf presence after a draft roundtrip: %j',
    async (savedPatch) => {
      const onChange = vi.fn();
      render(FormattingEditor, {
        patch: { common: { indent_width: 4 } },
        savedPatch,
        inherited: defaultFormattingPolicy(),
        scope: 'runtime',
        idPrefix: 'saved-number',
        onChange,
      });
      await fireEvent.input(screen.getByLabelText('Indent Width'), { target: { value: '2' } });
      expect(onChange).toHaveBeenLastCalledWith(savedPatch, 'formatting.common.indent_width');
    },
  );

  it('keeps a numeric override when restoring inheritance would change its displayed value', async () => {
    const onChange = vi.fn();
    const inherited = defaultFormattingPolicy();
    inherited.common.indent_width = 4;
    render(FormattingEditor, {
      patch: { preset: 'conventional' },
      savedPatch: {},
      inherited,
      scope: 'runtime',
      idPrefix: 'preset-number',
      onChange,
    });
    await fireEvent.input(screen.getByLabelText('Indent Width'), { target: { value: '4' } });
    expect(onChange).toHaveBeenLastCalledWith(
      { preset: 'conventional', common: { indent_width: 4 } },
      'formatting.common.indent_width',
    );
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
