// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncFilePage, {
  templateDocumentWithContent,
} from '../src/lib/components/SyncFilePage.svelte';
import {
  buildSyncOverrideEditorEnvelope,
  type SyncOverrideControlId,
  type SyncOverrideEditorEnvelope,
} from '../src/lib/repository-sync-override-settings';
import type { SyncConfig, SyncFilesContext, SyncOverride } from '../src/lib/types';

class TestResizeObserver {
  observe(): void {}
  disconnect(): void {}
}

function deferred<T>(): {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (cause: unknown) => void;
} {
  let resolve!: (value: T) => void;
  let reject!: (cause: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

describe('SyncFilePage [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  afterEach(() => vi.unstubAllGlobals());

  it('writes only the strict file fields the service accepts', () => {
    expect(
      templateDocumentWithContent(
        {
          files: [
            {
              path: 'renovate.json',
              content: '{}',
              updated_at: 'not part of orgsync.File',
              updated_by: 'also not part of orgsync.File',
            },
          ],
          retired: [],
        },
        'renovate.json',
        '{ "timezone": "UTC" }',
      ),
    ).toEqual({
      files: [{ path: 'renovate.json', content: '{ "timezone": "UTC" }' }],
      retired: [],
    });
  });

  it('ignores an override response after another repository opens', async () => {
    const first = deferred<SyncOverride>();
    const second = deferred<SyncOverride>();
    const merge = {
      path: 'renovate.json',
      strategy: 'deep-merge',
      overrides: { timezone: 'UTC' },
    };
    const config: SyncConfig = {
      kind: 'files',
      enabled: true,
      labels: [],
      allow_removal: false,
      excludes: [],
      revision: 1,
      updated_by: 'bart',
      updated_at: new Date(0).toISOString(),
      digest: '',
      document: { files: [{ path: 'renovate.json', content: '{}' }] },
      unreadable: false,
      unavailable: '',
    };
    const context: SyncFilesContext = {
      repositories: 2,
      covered: 2,
      known_paths: [],
      merges: [
        { repository: 'repo-a', repository_id: 'a', path: 'renovate.json', merge },
        { repository: 'repo-b', repository_id: 'b', path: 'renovate.json', merge },
      ],
    };

    render(SyncFilePage, {
      props: {
        config,
        context,
        path: 'renovate.json',
        nowMs: 0,
        readOnly: false,
        problem: null,
        sectionHref: () => '#',
        onOpenSection: vi.fn(),
        onChangeDocument: () => true,
        fetchOverride: async (repositoryId: string) => {
          const stored = await (repositoryId === 'a' ? first.promise : second.promise);
          return { stored, envelope: buildSyncOverrideEditorEnvelope(stored) };
        },
        onChangeOverride: vi.fn(() => true),
      },
    });

    await fireEvent.click(screen.getByRole('button', { name: /repo-a/ }));
    await fireEvent.click(screen.getByRole('button', { name: /repo-b/ }));
    first.reject(new Error('repo-a response crossed into repo-b'));
    await Promise.resolve();
    await Promise.resolve();

    expect(screen.queryByText('repo-a response crossed into repo-b')).toBeNull();
    expect(screen.getByRole('button', { name: /repo-b/ }).getAttribute('aria-expanded')).toBe(
      'true',
    );

    second.resolve({
      kind: 'files',
      enabled: null,
      document: { merges: [merge] },
      revision: 1,
      updated_by: 'bart',
      updated_at: new Date(0).toISOString(),
      unreadable: false,
    });
  });

  it('stages exact control text while preserving untouched numeric literals', async () => {
    const merge = {
      path: 'renovate.json',
      strategy: 'deep-merge',
      overrides: { amount: 1.5, timezone: 'Europe/Warsaw' },
    };
    const rawAmount = typeof JSON.rawJSON === 'function' ? JSON.rawJSON('1.50') : (1.5 as unknown);
    const stored: SyncOverride = {
      kind: 'files',
      enabled: null,
      document: {
        merges: [
          {
            ...merge,
            overrides: { amount: rawAmount, timezone: 'Europe/Warsaw' },
          },
        ],
      },
      revision: 4,
      updated_by: 'bart',
      updated_at: new Date(0).toISOString(),
      unreadable: false,
    };
    const staged: Array<{
      next: SyncOverrideEditorEnvelope;
      controlId: SyncOverrideControlId;
    }> = [];
    const onChangeOverride = (
      _repositoryId: string,
      _canonical: SyncOverride,
      next: SyncOverrideEditorEnvelope,
      controlId: SyncOverrideControlId,
    ): boolean => {
      staged.push({ next, controlId });
      return true;
    };

    render(SyncFilePage, {
      props: {
        config: {
          kind: 'files',
          enabled: true,
          labels: [],
          allow_removal: false,
          excludes: [],
          revision: 1,
          updated_by: 'bart',
          updated_at: new Date(0).toISOString(),
          digest: '',
          document: {
            files: [
              {
                path: 'renovate.json',
                content: '{ "amount": 1, "timezone": "UTC" }',
              },
            ],
          },
          unreadable: false,
          unavailable: '',
        },
        context: {
          repositories: 1,
          covered: 1,
          known_paths: [],
          merges: [
            {
              repository: 'repo-a',
              repository_id: 'repo-1',
              path: 'renovate.json',
              merge,
            },
          ],
        },
        path: 'renovate.json',
        nowMs: 0,
        readOnly: false,
        sectionHref: () => '#',
        onOpenSection: vi.fn(),
        onChangeDocument: () => true,
        fetchOverride: () =>
          Promise.resolve({ stored, envelope: buildSyncOverrideEditorEnvelope(stored) }),
        onChangeOverride,
      },
    });

    await fireEvent.click(screen.getByRole('button', { name: /repo-a/ }));
    const remove = await screen.findByRole('button', { name: 'Stop changing timezone' });
    await vi.waitFor(() => expect((remove as HTMLButtonElement).disabled).toBe(false));
    await fireEvent.click(remove);

    expect(staged).toHaveLength(1);
    expect(staged[0]?.controlId).toBe('repositories.repo-1.sync.files.document');
    expect(staged[0]?.next.override_texts).toEqual(['{\n  "amount": 1.50\n}']);
  });
});
