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

  /**
   * The surface is what the service composes, list rules and all.
   *
   * It used to be RFC 7396 and nothing else, so a repository with an append
   * rule was shown its own list replacing the template's - under a picker that
   * moved nothing on screen - and a save then derived from that wrong baseline.
   */
  it('composes an append rule the way the service does', async () => {
    render(SyncFileDetail, {
      ...base,
      adjustments: [
        adjustment({
          document: {
            merges: [
              {
                path: 'renovate.json',
                overrides: { extends: ['local:house-style'] },
                arrays: [{ path: '$.extends', strategy: 'append' }],
              },
            ],
          },
        }),
      ],
    });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));

    const editor = screen.getByRole('textbox', { name: 'What af ends up with' });
    expect((editor as HTMLTextAreaElement).value).toContain('"config:recommended"');
    expect((editor as HTMLTextAreaElement).value).toContain('"local:house-style"');
  });

  /**
   * Rules are an ordered list because two of them on one document have to
   * resolve the same way twice, and answering the list question rewrote that
   * order: the old rule was filtered out and the new one pushed on the end, so
   * changing a strategy moved its rule behind every rule it used to run before.
   * Nothing on screen said so - the control shows the word, not the position.
   */
  it('answers the list question in place, leaving the rules in their order', async () => {
    const onSaveAdjustment = vi.fn();
    render(SyncFileDetail, {
      ...base,
      onSaveAdjustment,
      adjustments: [
        adjustment({
          document: {
            merges: [
              {
                path: 'renovate.json',
                overrides: { extends: ['local:house-style'], reviewers: ['af'] },
                arrays: [
                  { path: '$.extends', strategy: 'append' },
                  { path: '$.reviewers', strategy: 'prepend' },
                ],
              },
            ],
          },
        }),
      ],
    });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    const asked = screen.getByRole('radiogroup', { name: 'How the two lists combine' });
    await fireEvent.click(within(asked).getByRole('radio', { name: /Prepend/u }));

    const document_ = onSaveAdjustment.mock.calls[0]?.[1] as {
      merges: { arrays: { path: string; strategy: string }[] }[];
    };
    expect(document_.merges[0]?.arrays).toEqual([
      { path: '$.extends', strategy: 'prepend' },
      { path: '$.reviewers', strategy: 'prepend' },
    ]);
  });

  /** And what it stores is the repository's share of that list, not the whole of it. */
  it('stores what a repository contributes to an appended list', async () => {
    const onSaveAdjustment = vi.fn();
    render(SyncFileDetail, {
      ...base,
      onSaveAdjustment,
      adjustments: [
        adjustment({
          document: {
            merges: [
              {
                path: 'renovate.json',
                overrides: { extends: ['local:house-style'] },
                arrays: [{ path: '$.extends', strategy: 'append' }],
              },
            ],
          },
        }),
      ],
    });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    const editor = screen.getByRole('textbox', { name: 'What af ends up with' });
    await fireEvent.input(editor, {
      target: {
        value:
          '{\n  "automerge": false,\n  "extends": [\n    "config:recommended",\n' +
          '    "local:house-style",\n    "local:extra"\n  ],\n  "timezone": "UTC"\n}\n',
      },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    const document_ = onSaveAdjustment.mock.calls[0]?.[1] as {
      merges: { overrides: Record<string, unknown> }[];
    };
    expect(document_.merges[0]?.overrides).toEqual({
      extends: ['local:house-style', 'local:extra'],
    });
  });

  /** A list that no longer holds the template's own entries cannot be an append. */
  it('refuses an edit that takes the template out of an appended list', async () => {
    const onSaveAdjustment = vi.fn();
    render(SyncFileDetail, {
      ...base,
      onSaveAdjustment,
      adjustments: [
        adjustment({
          document: {
            merges: [
              {
                path: 'renovate.json',
                overrides: { extends: ['local:house-style'] },
                arrays: [{ path: '$.extends', strategy: 'append' }],
              },
            ],
          },
        }),
      ],
    });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    const editor = screen.getByRole('textbox', { name: 'What af ends up with' });
    await fireEvent.input(editor, {
      target: {
        value:
          '{\n  "automerge": false,\n  "extends": [\n    "local:house-style"\n  ],\n' +
          '  "timezone": "UTC"\n}\n',
      },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(onSaveAdjustment).not.toHaveBeenCalled();
    expect(screen.getByRole('alert').textContent).toContain('appends to that list');
  });

  /**
   * A rejected write leaves the editor open over what was typed.
   *
   * It used to close first, so a 409 from somebody else's edit left the page
   * saying why beside a surface that no longer held the words - and reopening
   * reads the server's copy, so there was no way back to them at all.
   */
  it('holds the typed document when the write is refused', async () => {
    const onSaveAdjustment = vi.fn().mockResolvedValue(false);
    render(SyncFileDetail, { ...base, onSaveAdjustment });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    const editor = screen.getByRole('textbox', { name: 'What af ends up with' });
    const typed =
      '{\n  "extends": ["config:recommended"],\n  "timezone": "UTC",\n  "automerge": true\n}\n';
    await fireEvent.input(editor, { target: { value: typed } });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(onSaveAdjustment).toHaveBeenCalledTimes(1);
    const still = screen.getByRole('textbox', { name: 'What af ends up with' });
    expect((still as HTMLTextAreaElement).value).toBe(typed);
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
