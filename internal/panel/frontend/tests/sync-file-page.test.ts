// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import type { ComponentProps } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncFilePage, {
  templateDocumentWithContent,
} from '../src/lib/components/SyncFilePage.svelte';
import { defaultFormattingPolicy, formattingSources } from '../src/lib/formatting';
import {
  buildSyncOverrideEditorEnvelope,
  type SyncOverrideControlId,
  type SyncOverrideEditorEnvelope,
} from '../src/lib/repository-sync-override-settings';
import type {
  SyncConfig,
  SyncFileRenderInput,
  SyncFileRenderResponse,
  SyncFilesContext,
  SyncOverride,
} from '../src/lib/types';

const POLICY = defaultFormattingPolicy();

function repositoryPolicy(repository: string, repositoryId: string) {
  return {
    repository,
    repository_id: repositoryId,
  };
}

function validRender(input: SyncFileRenderInput): SyncFileRenderResponse {
  const repository = input.repository !== undefined;
  const current = repository ? 'repository_path' : 'template';
  return {
    valid: true,
    final_content: input.draft_content,
    matches_formatting: true,
    diagnostics: [],
    formatting: {
      current_layer: current,
      inherited_policy: POLICY,
      effective_policy: POLICY,
      provenance: formattingSources(current),
      layers: [
        { source: 'process', state: 'baseline' },
        { source: 'target', state: 'absent' },
        { source: 'template', state: 'stored' },
        ...(repository ? [{ source: 'repository_path' as const, state: 'absent' as const }] : []),
      ],
    },
  };
}

const renderFile = async (input: SyncFileRenderInput) => validRender(input);

function configWithTemplate(content = '{}'): SyncConfig {
  return {
    kind: 'files',
    enabled: true,
    labels: [],
    allow_removal: false,
    excludes: [],
    revision: 1,
    updated_by: 'bart',
    updated_at: new Date(0).toISOString(),
    digest: '',
    document: { files: [{ path: 'renovate.json', content }] },
    unreadable: false,
    unavailable: '',
  };
}

type SyncFilePageProps = ComponentProps<typeof SyncFilePage>;

function renderProps(over: Partial<SyncFilePageProps> = {}): SyncFilePageProps {
  return {
    config: configWithTemplate(),
    context: {
      repositories: 0,
      covered: 0,
      known_paths: [],
      repository_policies: [],
      merges: [],
    },
    path: 'renovate.json',
    nowMs: 0,
    readOnly: false,
    sectionHref: () => '#',
    onOpenSection: vi.fn(),
    onChangeDocument: () => true,
    fetchOverride: vi.fn(),
    renderFile,
    onFormattingValidity: vi.fn(),
    onChangeOverride: vi.fn(() => true),
    ...over,
  };
}

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

  it('changes content without discarding formatting or future file fields', () => {
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
      files: [
        {
          path: 'renovate.json',
          content: '{ "timezone": "UTC" }\n',
          updated_at: 'not part of orgsync.File',
          updated_by: 'also not part of orgsync.File',
        },
      ],
      retired: [],
    });
  });

  it('blocks saving while the template render check is still pending', async () => {
    const pending = deferred<SyncFileRenderResponse>();
    const onFormattingValidity = vi.fn();
    const renderFile = vi.fn(() => pending.promise);

    render(SyncFilePage, {
      props: renderProps({ renderFile, onFormattingValidity }),
    });

    await vi.waitFor(() =>
      expect(onFormattingValidity).toHaveBeenCalledWith(
        'sync.files.template-render::renovate.json',
        false,
        'The template formatting check has not finished',
      ),
    );
    expect(renderFile).not.toHaveBeenCalled();

    pending.resolve(
      validRender({ path: 'renovate.json', draft_content: '{}', template_formatting: {} }),
    );
  });

  it('keeps an invalid dirty template blocked after the page unmounts', async () => {
    const onFormattingValidity = vi.fn();
    const invalid: SyncFileRenderResponse = {
      valid: false,
      final_content: '',
      matches_formatting: false,
      diagnostics: [{ stage: 'format', code: 'unsafe_format', message: 'Formatting is unsafe' }],
    };
    const rendered = render(SyncFilePage, {
      props: renderProps({
        config: configWithTemplate('{"changed":true}'),
        savedDocument: { files: [{ path: 'renovate.json', content: '{}' }] },
        dirtyDocument: true,
        renderFile: vi.fn().mockResolvedValue(invalid),
        onFormattingValidity,
      }),
    });

    await vi.waitFor(() =>
      expect(onFormattingValidity).toHaveBeenCalledWith(
        'sync.files.template-render::renovate.json',
        false,
        'Formatting is unsafe',
      ),
    );
    const callsBeforeUnmount = onFormattingValidity.mock.calls.length;
    rendered.unmount();

    expect(
      onFormattingValidity.mock.calls
        .slice(callsBeforeUnmount)
        .some(
          ([control, valid]) =>
            control === 'sync.files.template-render::renovate.json' && valid === true,
        ),
    ).toBe(false);
  });

  it('keeps an invalid dirty repository output blocked after its row collapses', async () => {
    const onFormattingValidity = vi.fn();
    const stored: SyncOverride = {
      kind: 'files',
      enabled: null,
      document: {},
      revision: 1,
      updated_by: 'bart',
      updated_at: new Date(0).toISOString(),
      unreadable: false,
    };
    const renderFile = vi.fn(async (input: SyncFileRenderInput): Promise<SyncFileRenderResponse> =>
      input.repository === undefined
        ? validRender(input)
        : {
            valid: false,
            final_content: '',
            matches_formatting: false,
            diagnostics: [
              {
                stage: 'format',
                code: 'unsafe_format',
                message: 'Repository formatting is unsafe',
              },
            ],
          },
    );
    render(SyncFilePage, {
      props: renderProps({
        context: {
          repositories: 1,
          covered: 1,
          known_paths: [],
          repository_policies: [repositoryPolicy('repo-a', 'repo-1')],
          merges: [],
        },
        dirtyControls: ['repositories.repo-1.sync.files.document'],
        fetchOverride: vi.fn().mockResolvedValue({
          stored,
          envelope: buildSyncOverrideEditorEnvelope(stored),
        }),
        renderFile,
        onFormattingValidity,
      }),
    });

    const row = screen.getByRole('button', { name: /repo-a/ });
    await fireEvent.click(row);
    await vi.waitFor(() =>
      expect(onFormattingValidity).toHaveBeenCalledWith(
        'sync.files.repository-render:repo-1:renovate.json',
        false,
        'Repository formatting is unsafe',
      ),
    );
    const callsBeforeCollapse = onFormattingValidity.mock.calls.length;
    await fireEvent.click(row);

    expect(
      onFormattingValidity.mock.calls
        .slice(callsBeforeCollapse)
        .some(
          ([control, valid]) =>
            control === 'sync.files.repository-render:repo-1:renovate.json' && valid === true,
        ),
    ).toBe(false);
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
      repository_policies: [repositoryPolicy('repo-a', 'a'), repositoryPolicy('repo-b', 'b')],
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
        renderFile,
        onFormattingValidity: vi.fn(),
        onChangeOverride: vi.fn(() => true),
      },
    });

    await fireEvent.click(screen.getByRole('button', { name: /repo-a/ }));
    await fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    await fireEvent.click(screen.getByRole('button', { name: /repo-b/ }));
    first.reject(new Error('repo-a response crossed into repo-b'));
    await Promise.resolve();
    await Promise.resolve();

    expect(screen.queryByText('repo-a response crossed into repo-b')).toBeNull();
    expect(screen.getByRole('dialog', { name: 'repo-b' })).toBeDefined();

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
          repository_policies: [repositoryPolicy('repo-a', 'repo-1')],
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
        renderFile,
        onFormattingValidity: vi.fn(),
        onChangeOverride,
      },
    });

    await fireEvent.click(screen.getByRole('button', { name: /repo-a/ }));
    await fireEvent.click(screen.getByRole('radio', { name: 'Content adjustment' }));
    const remove = await screen.findByRole('button', { name: 'Stop changing timezone' });
    await vi.waitFor(() => expect((remove as HTMLButtonElement).disabled).toBe(false));
    await fireEvent.click(remove);

    expect(staged).toHaveLength(1);
    expect(staged[0]?.controlId).toBe('repositories.repo-1.sync.files.document');
    expect(staged[0]?.next.override_texts).toEqual(['{\n  "amount": 1.50\n}']);
  });
});
