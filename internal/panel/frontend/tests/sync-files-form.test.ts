// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncFilesForm from '../src/lib/components/SyncFilesForm.svelte';

/** The segmented control measures itself to place a thumb; jsdom does not. */
class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

/**
 * The template a repository ends up carrying is whatever this form sends, so
 * anything it drops on the way to a save is a file that stops being
 * synchronized - reported by the plan as an ordinary change.
 */
describe('SyncFilesForm [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  afterEach(() => vi.unstubAllGlobals());

  const base = {
    stored: {},
    enabled: false,
    unreadable: false,
    readOnly: false,
    saving: false,
    onSave: () => {},
  };

  function contributing(over: Record<string, unknown> = {}) {
    return { path: 'CONTRIBUTING.md', content: '# Contributing\n', ...over };
  }

  function saved() {
    const sent: { enabled: boolean; document: Record<string, unknown> }[] = [];

    return {
      sent,
      onSave: (enabled: boolean, document: Record<string, unknown>) =>
        sent.push({ enabled, document }),
    };
  }

  async function save(): Promise<void> {
    await fireEvent.click(screen.getByRole('button', { name: 'Save files' }));
  }

  it('shows the files the installation configured', () => {
    render(SyncFilesForm, { ...base, stored: { files: [contributing()] } });

    expect(screen.getByDisplayValue('CONTRIBUTING.md')).toBeTruthy();

    // Read off the element rather than matched, because the matcher trims and
    // a template's trailing newline is part of the file.
    const content = screen.getByPlaceholderText('# Contributing') as HTMLTextAreaElement;
    expect(content.value).toBe('# Contributing\n');
  });

  it('says so where nothing is configured rather than showing an empty row', () => {
    render(SyncFilesForm, base);

    expect(screen.getByText('No files yet.')).toBeTruthy();
  });

  /**
   * A kind nobody has configured must not offer a save on load. The stored
   * document is empty and what a save would send is three keys with their
   * defaults, so comparing against the wrong one puts Save live before anybody
   * has touched anything.
   */
  it('offers no save until something changes', () => {
    render(SyncFilesForm, base);

    expect(screen.getByRole('button', { name: 'Save files' }).hasAttribute('disabled')).toBe(true);
  });

  it('sends a file somebody added', async () => {
    const { sent, onSave } = saved();
    render(SyncFilesForm, { ...base, onSave });

    await fireEvent.click(screen.getByRole('button', { name: 'Add a file' }));
    await fireEvent.change(screen.getByPlaceholderText('CONTRIBUTING.md'), {
      target: { value: 'SECURITY.md' },
    });
    await fireEvent.change(screen.getByPlaceholderText('# Contributing'), {
      target: { value: '# Security' },
    });
    await save();

    expect(sent).toHaveLength(1);
    expect(sent[0].document.files).toEqual([{ path: 'SECURITY.md', content: '# Security' }]);
  });

  it('sends a file somebody removed', async () => {
    const { sent, onSave } = saved();
    render(SyncFilesForm, { ...base, stored: { files: [contributing()] }, onSave });

    await fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
    await save();

    expect(sent[0].document.files).toEqual([]);
  });

  /**
   * The only thing on this page that deletes anything, and the reason there is
   * no switch beside it: the tool this replaces published one promising to
   * remove every file the configuration does not name, which is every file in
   * the repository.
   */
  it('sends the paths to remove, one per line', async () => {
    const { sent, onSave } = saved();
    render(SyncFilesForm, { ...base, stored: { files: [contributing()] }, onSave });

    await fireEvent.change(screen.getByPlaceholderText('.github/workflows/sync-trigger.yml'), {
      target: { value: '.renovaterc\n\n  .github/workflows/sync-trigger.yml  \n' },
    });
    await save();

    expect(sent[0].document.retired).toEqual(['.renovaterc', '.github/workflows/sync-trigger.yml']);
  });

  it('sends the paths to leave alone', async () => {
    const { sent, onSave } = saved();
    render(SyncFilesForm, { ...base, stored: { files: [contributing()] }, onSave });

    await fireEvent.change(screen.getByPlaceholderText('LICENSE'), {
      target: { value: 'LICENSE\n*.md' },
    });
    await save();

    expect(sent[0].document.excludes).toEqual(['LICENSE', '*.md']);
  });

  /**
   * Anything a later version adds is stored in the same document, and a form
   * that rebuilt it from its own controls would drop every key it has no
   * control for.
   */
  it('carries through a key it has no control for', async () => {
    const { sent, onSave } = saved();
    render(SyncFilesForm, {
      ...base,
      stored: { files: [contributing()], something_later: { deep: true } },
      onSave,
    });

    await fireEvent.click(screen.getByRole('button', { name: 'Add a file' }));
    await save();

    expect(sent[0].document.something_later).toEqual({ deep: true });
  });

  /**
   * A document this version cannot read renders as no files at all, which is
   * exactly what a repository configuring none looks like. Saving over it would
   * send the emptiness the panel invented rather than the templates the row
   * still holds.
   */
  it('changes nothing while the stored document cannot be read', () => {
    render(SyncFilesForm, { ...base, unreadable: true });

    expect(screen.getByRole('alert').textContent).toContain('cannot read');
    expect(screen.getByRole('button', { name: 'Save files' }).hasAttribute('disabled')).toBe(true);
  });

  it('offers no save at all to somebody who may only read', () => {
    render(SyncFilesForm, { ...base, readOnly: true, stored: { files: [contributing()] } });

    expect(screen.queryByRole('button', { name: 'Save files' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Remove' })).toBeNull();
  });

  it('says which permission is missing while the switch is on', () => {
    render(SyncFilesForm, {
      ...base,
      enabled: true,
      unavailable: 'Smyklot has not been granted contents access, which files sync needs',
    });

    expect(screen.getByRole('status').textContent).toContain('contents');
  });
});
