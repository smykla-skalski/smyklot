// @vitest-environment jsdom
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncFileDetail from '../src/lib/components/SyncFileDetail.svelte';
import SyncFilesForm from '../src/lib/components/SyncFilesForm.svelte';
import type { SyncOverrideRow } from '../src/lib/types.js';

/** The segmented controls measure themselves to place a thumb; jsdom does not. */
class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

const TEMPLATE = `{
  "extends": ["config:recommended"],
  "timezone": "UTC",
  "automerge": false
}
`;

const STORED = {
  files: [
    { path: 'renovate.json', content: TEMPLATE },
    { path: 'CONTRIBUTING.md', content: '# Contributing\n' },
  ],
  retired: ['.github/stale.yml'],
  excludes: ['LICENSE-*'],
};

function adjustment(over: Partial<SyncOverrideRow> = {}): SyncOverrideRow {
  return {
    repository_id: '4001',
    repository_name: 'af',
    kind: 'files',
    enabled: null,
    document: {
      merges: [
        {
          path: 'renovate.json',
          strategy: 'deep-merge',
          overrides: { timezone: 'Europe/Warsaw' },
        },
      ],
    },
    revision: 1,
    unreadable: false,
    ...over,
  } as SyncOverrideRow;
}

/**
 * Deletion here is a named list of retired paths and nothing else. The tool
 * this replaces published a switch promising to delete every file not in the
 * central configuration - which is every file in the repository - documented it
 * as dangerous, and never implemented it.
 */
describe('SyncFilesForm [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  afterEach(() => vi.unstubAllGlobals());

  const base = {
    stored: STORED,
    enabled: true,
    unreadable: false,
    readOnly: false,
    saving: false,
    fileHref: (path: string) => `/sync/files/${path}`,
    onSave: () => {},
  };

  it('draws a row per template, with the way into it', () => {
    const { container } = render(SyncFilesForm, base);

    const rows = [...container.querySelectorAll<HTMLAnchorElement>('a.object-row')];
    expect(rows.map((row) => row.getAttribute('href'))).toEqual([
      '/sync/files/renovate.json',
      '/sync/files/CONTRIBUTING.md',
    ]);
  });

  /**
   * How a file arrives is decided by the repositories, not by the template: the
   * installation says what the file should say, and a repository says how its
   * own differs.
   */
  it('says a file replaces until a repository adjusts it', () => {
    const { container } = render(SyncFilesForm, base);
    expect(container.querySelector('a.object-row')?.textContent).toContain('replaces');

    const adjusted = render(SyncFilesForm, { ...base, adjustments: [adjustment()] });
    const row = adjusted.container.querySelector('a.object-row');
    expect(row?.textContent).toContain('merges · deep-merge');
    expect(row?.textContent).toContain('1 repository adjusts it');
  });

  /** Every write carries the whole document, keys this version knows nothing of included. */
  it('adds a path from the finder without dropping the rest', async () => {
    const onSave = vi.fn();
    render(SyncFilesForm, {
      ...base,
      stored: { ...STORED, some_future_key: 'kept' },
      paths: [{ path: '.github/CODEOWNERS', repositories: 20 }],
      repositories: 25,
      onSave,
    });

    await fireEvent.click(screen.getByRole('button', { name: 'Add a file' }));
    const field = screen.getByRole('combobox', { name: 'Path of the file to manage' });
    await fireEvent.focus(field);
    await fireEvent.input(field, { target: { value: 'CODEOWNERS' } });
    await fireEvent.click(screen.getByRole('option', { name: /CODEOWNERS/ }));

    const document_ = onSave.mock.calls[0]?.[1] as { files: { path: string }[] };
    expect(document_.files.map((file) => file.path)).toEqual([
      'renovate.json',
      'CONTRIBUTING.md',
      '.github/CODEOWNERS',
    ]);
    expect(document_).toMatchObject({ some_future_key: 'kept' });
  });

  it('retires a path, which is the only thing here that deletes anything', async () => {
    const onSave = vi.fn();
    render(SyncFilesForm, { ...base, onSave });

    await fireEvent.click(screen.getByRole('button', { name: 'Add a path' }));
    const field = screen.getByRole('textbox', { name: 'Add a path' });
    await fireEvent.input(field, { target: { value: '.github/old.yml' } });
    await fireEvent.keyDown(field, { key: 'Enter' });

    expect(onSave.mock.calls[0]?.[1]).toMatchObject({
      retired: ['.github/stale.yml', '.github/old.yml'],
    });
  });

  it('offers no way to add one while read only', () => {
    render(SyncFilesForm, { ...base, readOnly: true });

    expect(screen.queryByRole('button', { name: 'Add a file' })).toBeNull();
  });
});

/**
 * The RESULT is the editable surface, never the adjustment. Everything here is
 * about that round trip holding: what somebody types is what the service will
 * compose, or the page says it cannot be stored.
 */
describe('SyncFileDetail [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  afterEach(() => vi.unstubAllGlobals());

  const base = {
    stored: STORED,
    path: 'renovate.json',
    listHref: '/sync/files',
    adjustments: [adjustment()],
    repositories: 25,
    readOnly: false,
    saving: false,
    unreadable: false,
    onSave: () => {},
  };

  it('names what each repository changes', () => {
    const { container } = render(SyncFileDetail, base);

    expect(container.querySelector('.object-sum')?.textContent).toBe('changes 1 key — timezone');
  });

  /** The composed file, not the patch: that is the thing somebody came to read. */
  it('opens the composed file for editing', async () => {
    render(SyncFileDetail, base);

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));

    const editor = screen.getByRole('textbox', { name: 'What af ends up with' });
    expect((editor as HTMLTextAreaElement).value).toContain('"timezone": "Europe/Warsaw"');
    expect((editor as HTMLTextAreaElement).value).toContain('"extends"');
  });

  it('stores what was typed as the difference from the template', async () => {
    const onSaveAdjustment = vi.fn();
    render(SyncFileDetail, { ...base, onSaveAdjustment });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    const editor = screen.getByRole('textbox', { name: 'What af ends up with' });
    await fireEvent.input(editor, {
      target: {
        value:
          '{\n  "extends": ["config:recommended"],\n  "timezone": "UTC",\n  "automerge": true\n}\n',
      },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(onSaveAdjustment).toHaveBeenCalledTimes(1);
    const document_ = onSaveAdjustment.mock.calls[0]?.[1] as {
      merges: { path: string; overrides: Record<string, unknown> }[];
    };
    // The timezone went back to the template's, so it stops being adjusted.
    expect(document_.merges[0]?.overrides).toEqual({ automerge: true });
  });

  /**
   * RFC 7396 reads a null in a patch as "remove this key", so storing one would
   * mean something other than what somebody typed.
   */
  it('refuses an edit a merge patch cannot say', async () => {
    const onSaveAdjustment = vi.fn();
    render(SyncFileDetail, { ...base, onSaveAdjustment });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    const editor = screen.getByRole('textbox', { name: 'What af ends up with' });
    await fireEvent.input(editor, {
      target: { value: '{\n  "extends": ["config:recommended"],\n  "timezone": null\n}\n' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(onSaveAdjustment).not.toHaveBeenCalled();
    expect(screen.getByRole('alert').textContent).toContain('cannot set a key to null');
  });

  it('refuses what is not JSON at all', async () => {
    const onSaveAdjustment = vi.fn();
    render(SyncFileDetail, { ...base, onSaveAdjustment });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await fireEvent.input(screen.getByRole('textbox', { name: 'What af ends up with' }), {
      target: { value: '{oops' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(onSaveAdjustment).not.toHaveBeenCalled();
    expect(screen.getByRole('alert').textContent).toContain('not valid JSON');
  });

  /** Clearing an adjustment returns the template's own content, never a value of its own. */
  it('stops adjusting without writing anything in its place', async () => {
    const onSaveAdjustment = vi.fn();
    const { container } = render(SyncFileDetail, { ...base, onSaveAdjustment });

    const row = container.querySelector('.object-row') as HTMLElement;
    await fireEvent.click(within(row).getByRole('button', { name: 'Stop adjusting' }));

    expect(onSaveAdjustment.mock.calls[0]?.[1]).toEqual({ merges: [] });
  });

  /**
   * A Markdown file is composed by rules a browser cannot reproduce, so an
   * adjustment is named rather than drawn as a file that would be a guess.
   */
  it('offers no result surface for a file it cannot compose', () => {
    render(SyncFileDetail, { ...base, path: 'CONTRIBUTING.md', adjustments: [] });

    expect(screen.queryByRole('button', { name: 'Edit the template' })).not.toBeNull();
    expect(document.body.textContent).toContain('cannot reproduce');
  });

  it('says so when no template has that path', () => {
    render(SyncFileDetail, { ...base, path: 'renovate.json5' });

    expect(document.body.textContent).toContain('No template here has that path');
  });
});
