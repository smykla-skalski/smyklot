// @vitest-environment jsdom
import { EditorState } from '@codemirror/state';
import { EditorView } from '@codemirror/view';
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import RepositoryControl from '../src/lib/components/RepositoryControl.svelte';
import type { RepositoryDetail, RepositorySummary } from '../src/lib/types';
import { REPOSITORY, REPOSITORY_DETAIL } from '../stories/support/fixtures';

class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

const SEARCH_PATHS = [
  '.smyklot.toml',
  '.smyklot/config.toml',
  '.github/.smyklot.toml',
  '.github/smyklot.yaml',
];
const NOW = Date.parse('2026-09-06T12:00:00Z');

function detail(over: Partial<RepositoryDetail> = {}): RepositoryDetail {
  return {
    ...REPOSITORY_DETAIL,
    config_file_observation: {
      status: 'valid',
      search_paths: SEARCH_PATHS,
      observed_at: new Date(NOW - 5 * 60_000).toISOString(),
    },
    ...over,
  };
}

function props(over: Record<string, unknown> = {}) {
  return {
    repository: REPOSITORY,
    detail: detail(),
    now: NOW,
    onEnablement: vi.fn(),
    onUseFile: vi.fn(),
    onResetMigration: vi.fn(),
    ...over,
  };
}

async function inspect(): Promise<HTMLElement> {
  await fireEvent.click(screen.getByRole('button', { name: 'Inspect file' }));
  return screen.getByRole('dialog', { name: 'Configuration file' });
}

function parsedEditor(): EditorView | null {
  for (const host of document.querySelectorAll('.code-editor')) {
    const content = host.shadowRoot?.querySelector<HTMLElement>(
      '[aria-label="Parsed file settings"]',
    );
    if (content) return EditorView.findFromDOM(content);
  }
  return null;
}

describe('RepositoryControl observed file and editable policy [Component]', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it.each([false, true])(
    'keeps valid-empty observation independent of file use %s',
    async (ignored) => {
      render(
        RepositoryControl,
        props({
          detail: detail({ config_file_patch: {}, ignore_repository_file: ignored }),
        }),
      );

      expect(screen.getByText('Valid')).toBeTruthy();
      expect(
        (screen.getByRole('checkbox', { name: 'Use file settings' }) as HTMLInputElement).checked,
      ).toBe(!ignored);
      const dialog = await inspect();
      expect(
        within(dialog).getByText('Original syntax and comments are not shown', { exact: false }),
      ).toBeTruthy();
      expect(parsedEditor()?.state.doc.toString()).toBe('{}');
      expect(parsedEditor()?.state.facet(EditorState.readOnly)).toBe(true);
    },
  );

  it('does not infer an observation from the old combined status or a saved patch', async () => {
    render(RepositoryControl, props({ detail: detail({ config_file_observation: undefined }) }));

    expect(screen.getByText('Not checked')).toBeTruthy();
    expect(screen.queryByText('Valid')).toBeNull();
    const dialog = await inspect();
    expect(within(dialog).getByRole('heading', { name: 'File status' })).toBeTruthy();
    expect(within(dialog).getByText('No recorded file check')).toBeTruthy();
    expect(within(dialog).queryByText('Path', { exact: true })).toBeNull();
    expect(within(dialog).queryByRole('link', { name: 'Open on GitHub' })).toBeNull();
    expect(within(dialog).getByText('The service has not reported its search paths')).toBeTruthy();
    expect(parsedEditor()).toBeNull();
  });

  it('stages the enabled file-use value and the workspace/on/off choice', async () => {
    const callbacks = props({
      detail: detail({ ignore_repository_file: true }),
      dirtyUseFile: true,
    });
    render(RepositoryControl, callbacks);

    const toggle = screen.getByRole('checkbox', { name: 'Use file settings' });
    await fireEvent.click(toggle);
    expect(callbacks.onUseFile).toHaveBeenCalledWith(true);
    expect(toggle.closest('[data-unsaved]')?.getAttribute('data-unsaved')).toBe('true');
    await fireEvent.click(screen.getByRole('radio', { name: 'Off' }));
    expect(callbacks.onEnablement).toHaveBeenCalledWith('disabled');
  });

  it('reports invalid files even when their settings are ignored, with the reason in inspection', async () => {
    render(
      RepositoryControl,
      props({
        detail: detail({
          config_file_observation: { status: 'invalid', search_paths: SEARCH_PATHS },
          config_file_patch: {},
          config_file_error: 'line 4: command_aliases must be a mapping',
          ignore_repository_file: true,
        }),
      }),
    );

    expect(screen.getByText('Invalid')).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
    const dialog = await inspect();
    expect(within(dialog).getByRole('alert').textContent).toContain(
      'command_aliases must be a mapping',
    );
    expect(parsedEditor()).toBeNull();
  });

  it('explains that an active invalid file blocks commands, including the off-state consequence', async () => {
    const component = render(
      RepositoryControl,
      props({
        detail: detail({
          config_file_observation: { status: 'invalid', search_paths: SEARCH_PATHS },
        }),
      }),
    );
    expect(
      screen.getByText(
        'Commands are blocked until the file is fixed or file settings are turned off',
      ),
    ).toBeTruthy();
    await component.rerender({ enablement: 'disabled' });
    expect(
      screen.getByText('Fix the file or turn file settings off before enabling Smyklot'),
    ).toBeTruthy();
  });

  it('renders the backend search priority and distinguishes other detected files', async () => {
    render(
      RepositoryControl,
      props({
        detail: detail({
          config_file_superseded: ['.smyklot/config.toml', '.github/smyklot.yaml'],
        }),
      }),
    );
    const dialog = await inspect();
    const paths = within(dialog)
      .getAllByRole('listitem')
      .map((row) => row.textContent);
    expect(paths).toHaveLength(SEARCH_PATHS.length);
    SEARCH_PATHS.forEach((path, index) => expect(paths[index]).toContain(path));
    expect(paths[0]).toContain('Selected');
    expect(paths[1]).toContain('Ignored');
    expect(paths[2]).not.toContain('Ignored');
    expect(paths[3]).toContain('Ignored');
    expect(within(dialog).getByText('The first file found is used')).toBeTruthy();
  });

  it('does not label unobserved candidate paths as present or absent', async () => {
    render(
      RepositoryControl,
      props({
        detail: detail({
          config_file_observation: { status: 'unknown', search_paths: SEARCH_PATHS },
          config_file_superseded: ['.github/smyklot.yaml'],
        }),
      }),
    );
    const dialog = await inspect();
    const rows = within(dialog).getAllByRole('listitem');
    expect(rows).toHaveLength(SEARCH_PATHS.length);
    rows.forEach((row) => {
      expect(row.textContent).not.toMatch(/Selected|Ignored|Missing|Not found/u);
    });
  });

  it('encodes a default branch and file path in the GitHub source link', async () => {
    render(
      RepositoryControl,
      props({
        repository: { ...REPOSITORY, default_branch: 'release/next' },
        detail: detail({ config_file_path: '.github/config #1.toml' }),
      }),
    );
    const dialog = await inspect();
    expect(within(dialog).getByRole('link', { name: 'Open on GitHub' }).getAttribute('href')).toBe(
      'https://github.com/smykla-skalski/smyklot/blob/release%2Fnext/.github/config%20%231.toml',
    );
  });

  it.each([
    { full_name: 'https://evil.example/repo', path: '.smyklot.toml' },
    { full_name: 'owner/repo', path: '../.smyklot.toml' },
    { full_name: 'owner/repo', path: '/.smyklot.toml' },
  ])('does not invent a source link from unsafe metadata %s', async ({ full_name, path }) => {
    render(
      RepositoryControl,
      props({
        repository: { ...REPOSITORY, full_name },
        detail: detail({ config_file_path: path }),
      }),
    );
    const dialog = await inspect();
    expect(within(dialog).queryByRole('link', { name: 'Open on GitHub' })).toBeNull();
  });

  it('shows missing independently of stale selected-path metadata', async () => {
    render(
      RepositoryControl,
      props({
        detail: detail({
          config_file_observation: { status: 'missing', search_paths: SEARCH_PATHS },
        }),
      }),
    );
    expect(screen.getByText('Missing')).toBeTruthy();
    const dialog = await inspect();
    expect(within(dialog).getByRole('heading', { name: 'File status' })).toBeTruthy();
    expect(within(dialog).getByText('No configuration file found')).toBeTruthy();
    expect(within(dialog).queryByText('Path', { exact: true })).toBeNull();
    expect(within(dialog).queryByRole('link', { name: 'Open on GitHub' })).toBeNull();
    expect(within(dialog).queryByText('Selected', { exact: true })).toBeNull();
  });

  it('links the proposed migration and offers retry only for stopped proposals', async () => {
    const callbacks = props({
      detail: detail({ config_migration: 'proposed', config_migration_pr: 42 }),
    });
    const component = render(RepositoryControl, callbacks);
    let dialog = await inspect();
    expect(
      within(dialog).getByRole('link', { name: 'Pull request #42' }).getAttribute('href'),
    ).toBe('https://github.com/smykla-skalski/smyklot/pull/42');
    expect(within(dialog).queryByRole('button', { name: 'Retry proposal' })).toBeNull();

    await component.rerender({ detail: detail({ config_migration: 'declined' }) });
    dialog = screen.getByRole('dialog', { name: 'Configuration file' });
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Retry proposal' }));
    expect(callbacks.onResetMigration).toHaveBeenCalledOnce();
  });

  it.each([
    { readOnly: true, busy: false },
    { readOnly: false, busy: true },
  ])('keeps retry unavailable when %s without blocking inspection', async ({ readOnly, busy }) => {
    render(
      RepositoryControl,
      props({ readOnly, busy, detail: detail({ config_migration: 'blocked' }) }),
    );
    expect(
      (screen.getByRole('checkbox', { name: 'Use file settings' }) as HTMLInputElement).disabled,
    ).toBe(readOnly);
    const dialog = await inspect();
    expect(
      (
        within(dialog).getByRole('button', {
          name: busy ? 'Requesting proposal…' : 'Retry proposal',
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
  });

  it('closes without staging edits and never reopens for a previously inspected repository', async () => {
    const callbacks = props();
    const component = render(RepositoryControl, callbacks);
    await inspect();
    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(callbacks.onEnablement).not.toHaveBeenCalled();
    expect(callbacks.onUseFile).not.toHaveBeenCalled();
    await inspect();
    await vi.advanceTimersByTimeAsync(10);
    await fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).toBeNull();
    await inspect();

    const nextRepository: RepositorySummary = {
      ...REPOSITORY,
      id: 'different',
      name: 'other',
      full_name: 'owner/other',
    };
    await component.rerender({ repository: nextRepository });
    expect(screen.queryByRole('dialog')).toBeNull();
    await component.rerender({ repository: REPOSITORY });
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});
