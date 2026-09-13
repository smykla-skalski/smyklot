// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ComponentProps } from 'svelte';
import SyncFilesPage from '../src/lib/components/SyncFilesPage.svelte';

const receipt = vi.hoisted(() => vi.fn());
vi.mock('../src/lib/receipts.svelte', () => ({ receipts: { say: receipt } }));

function props(
  over: Partial<ComponentProps<typeof SyncFilesPage>> = {},
): ComponentProps<typeof SyncFilesPage> {
  return {
    config: {
      kind: 'files',
      labels: [],
      allow_removal: false,
      enabled: true,
      document: { files: [] },
      excludes: [],
      revision: 1,
      updated_at: new Date(0).toISOString(),
      updated_by: 'bart',
      digest: '',
      unreadable: false,
      unavailable: '',
    },
    savedDocument: { files: [] },
    context: null,
    plan: null,
    syncStatus: null,
    nowMs: 0,
    readOnly: false,
    fileHref: (path) => `#${path}`,
    onOpenFile: vi.fn(),
    onToggleEnabled: vi.fn(),
    onChangeDocument: vi.fn(() => true),
    ...over,
  };
}

describe('shared-file draft lifecycle', () => {
  const originalScroll = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollIntoView');
  afterEach(() => {
    vi.unstubAllGlobals();
    if (originalScroll)
      Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', originalScroll);
    else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
  });
  beforeEach(() => {
    receipt.mockClear();
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
      configurable: true,
      value: vi.fn(),
    });
    document.body.innerHTML = '<main class="app-shell"></main>';
    vi.stubGlobal(
      'ResizeObserver',
      class {
        observe() {}
        disconnect() {}
        unobserve() {}
      },
    );
  });
  it.each([false, true])('reports creation only when staging succeeds (%s)', async (accepted) => {
    const input = props({ onChangeDocument: vi.fn(() => accepted) });
    render(SyncFilesPage, { props: input });
    await fireEvent.click(screen.getByRole('button', { name: 'Add a file' }));
    await fireEvent.input(
      screen.getByPlaceholderText('renovate.json, or a path no repository has yet'),
      { target: { value: 'draft.json' } },
    );
    await fireEvent.click(await screen.findByRole('option', { name: /Start draft.json/ }));
    expect(input.onChangeDocument).toHaveBeenCalledWith({
      files: [{ path: 'draft.json', content: '' }],
    });
    expect(input.onOpenFile).toHaveBeenCalledTimes(accepted ? 1 : 0);
    expect(receipt).toHaveBeenCalledTimes(accepted ? 1 : 0);
    if (accepted)
      expect(receipt).toHaveBeenCalledWith(
        'Draft template created for draft.json. Add content, then save.',
      );
  });
  it('identifies a saved template without claiming a repository outcome', () => {
    const input = props();
    input.config!.document = { files: [{ path: 'saved.json', content: '{}' }] };
    render(SyncFilesPage, { props: input });
    const row = screen.getByRole('link', { name: /saved.json/ });
    expect(row.textContent).toContain('Saved');
    expect(row.querySelector('.mx-instep')).toBeNull();
  });
  it('does not borrow saved provenance or claim sync success for a new row', () => {
    const input = props();
    input.config!.document = { files: [{ path: 'draft.json', content: '' }] };
    render(SyncFilesPage, { props: { ...input, dirtyDocument: true } });
    const row = screen.getByRole('link', { name: /draft.json/ });
    expect(row.textContent).toContain('Unsaved new file');
    expect(row.textContent).toContain('Not saved');
    expect(row.textContent).not.toContain('updated');
    expect(row.querySelector('.mx-instep')).toBeNull();
  });
});
