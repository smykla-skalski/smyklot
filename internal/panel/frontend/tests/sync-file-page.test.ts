// @vitest-environment jsdom
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { isolateHistory, redo } from '@codemirror/commands';
import { EditorView } from '@codemirror/view';
import { tick, type ComponentProps } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SyncFilePage, {
  templateDocumentWithContent,
} from '../src/lib/components/SyncFilePage.svelte';
import RepositorySyncPane from '../src/lib/components/RepositorySyncPane.svelte';
import { defaultFormattingPolicy, formattingSources } from '../src/lib/formatting';
import {
  buildSyncOverrideEditorEnvelope,
  adoptSyncOverrideSettings,
  parseSyncOverrideEditorEnvelope,
  stageSyncOverrideControl,
  syncOverrideDraftEnvelope,
  syncOverrideBatchInput,
  type SyncOverrideControlId,
  type SyncOverrideEditorEnvelope,
} from '../src/lib/repository-sync-override-settings';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
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

  it.each(['{"id":2,"id":3}', '{"nested":{"id":2,"id":3}}', '{"id":2,"\\u0069d":3}'])(
    'keeps an invalid raw draft blocked when handed from repository to shared editor: %s',
    async (text) => {
      const merge = { path: 'renovate.json', strategy: 'deep-merge', overrides: { id: 1 } };
      const stored: SyncOverride = {
        kind: 'files',
        enabled: null,
        document: { merges: [merge] },
        revision: 1,
        unreadable: false,
      };
      const values = new Map<string, string>();
      const storage = {
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => {
          values.set(key, value);
        },
      };
      const registry = new SettingsDraftRegistry({ storage, writerId: 'first' });
      registry.hydrate('viewer');
      adoptSyncOverrideSettings(registry, 'target', 'repo-1', stored);
      const repositoryProps = {
        repositoryId: 'repo-1',
        readOnly: false,
        now: 0,
        stored,
        onChange: (envelope: SyncOverrideEditorEnvelope, control: SyncOverrideControlId) => {
          expect(
            stageSyncOverrideControl(registry, 'target', 'repo-1', stored, envelope, control),
          ).toBe(true);
        },
      };
      const rawEditor = render(RepositorySyncPane, repositoryProps);
      const rawHost = document.querySelector('.code-editor')!;
      const rawView = EditorView.findFromDOM(rawHost.shadowRoot!.querySelector('.cm-content')!)!;
      rawView.dispatch({ changes: { from: 0, to: rawView.state.doc.length, insert: text } });
      await tick();
      rawEditor.unmount();
      const restarted = new SettingsDraftRegistry({ storage, writerId: 'second' });
      restarted.hydrate('viewer');
      const envelope = syncOverrideDraftEnvelope(restarted, 'target', 'repo-1', stored);
      expect(envelope.override_texts).toEqual([text]);
      const scope = { type: 'workspace' as const, targetId: 'target' };
      const onChangeOverride = vi.fn(() => true);
      const renderer = vi.fn(renderFile);
      const shared = render(SyncFilePage, {
        props: renderProps({
          config: configWithTemplate('{"id":0,"flag":false}'),
          context: {
            repositories: 1,
            covered: 1,
            known_paths: [],
            repository_policies: [repositoryPolicy('repo-a', 'repo-1')],
            merges: [{ repository: 'repo-a', repository_id: 'repo-1', path: merge.path, merge }],
          },
          fetchOverride: async () => ({ stored, envelope }),
          onChangeOverride,
          dirtyControls: ['repositories.repo-1.sync.files.document'],
          onFormattingValidity: (control, valid, message) =>
            restarted.setValidationProblem(scope, control, valid ? null : message),
          renderFile: renderer,
        }),
      });
      await fireEvent.click(screen.getByRole('button', { name: /repo-a/ }));
      await screen.findByText("Finish this adjustment in the repository's File sync settings");
      await new Promise((resolve) => setTimeout(resolve, 180));
      expect(renderer.mock.calls.filter(([input]) => input.repository !== undefined)).toEqual([]);
      expect(screen.getByRole('dialog').querySelector('.code-editor')).toBeNull();
      expect(onChangeOverride).not.toHaveBeenCalled();
      expect(syncOverrideBatchInput('repo-1', 1, envelope).ok).toBe(false);
      shared.unmount();
      render(RepositorySyncPane, {
        ...repositoryProps,
        envelope,
        onChange: (next, control) => {
          expect(
            stageSyncOverrideControl(restarted, 'target', 'repo-1', stored, next, control),
          ).toBe(true);
        },
      });
      const restoredHost = document.querySelector('.code-editor')!;
      const restored = EditorView.findFromDOM(
        restoredHost.shadowRoot!.querySelector('.cm-content')!,
      )!;
      expect(restored.state.sliceDoc()).toBe(text);
      restored.dispatch({
        changes: { from: 0, to: restored.state.doc.length, insert: '{"id":3}' },
      });
      await tick();
      expect(
        syncOverrideBatchInput(
          'repo-1',
          1,
          syncOverrideDraftEnvelope(restarted, 'target', 'repo-1', stored),
        ).ok,
      ).toBe(true);
      expect(restarted.hasDirty(scope)).toBe(true);
      expect(restarted.validationProblem(scope)).toBeNull();
    },
  );

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
      expect(parseSyncOverrideEditorEnvelope(next)).not.toBeNull();
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

  it('keeps an opening draft unpin when another field returns to its saved value', async () => {
    const merge = {
      path: 'renovate.json',
      strategy: 'deep-merge',
      overrides: { automerge: false, timezone: 'Europe/Warsaw' },
    };
    const stored: SyncOverride = {
      kind: 'files',
      enabled: null,
      document: { merges: [merge] },
      revision: 1,
      updated_by: 'bart',
      updated_at: new Date(0).toISOString(),
      unreadable: false,
    };
    const draft = buildSyncOverrideEditorEnvelope({
      ...stored,
      document: { merges: [{ ...merge, overrides: { timezone: 'Europe/Paris' } }] },
    });
    const staged: SyncOverrideEditorEnvelope[] = [];
    render(SyncFilePage, {
      props: renderProps({
        config: configWithTemplate('{"automerge":false,"timezone":"UTC"}'),
        context: {
          repositories: 1,
          covered: 1,
          known_paths: [],
          repository_policies: [repositoryPolicy('repo-a', 'repo-1')],
          merges: [{ repository: 'repo-a', repository_id: 'repo-1', path: merge.path, merge }],
        },
        fetchOverride: async () => ({ stored, envelope: draft }),
        onChangeOverride: (_id, _stored, next) => {
          staged.push(next);
          return true;
        },
      }),
    });
    await fireEvent.click(screen.getByRole('button', { name: /repo-a/ }));
    await screen.findByRole('button', { name: 'Stop changing timezone' });
    await vi.waitFor(() =>
      expect(
        screen
          .getByRole('dialog')
          .querySelector('.code-editor')
          ?.shadowRoot?.querySelector('.cm-content'),
      ).toBeInstanceOf(HTMLElement),
    );
    const host = screen.getByRole('dialog').querySelector('.code-editor')!;
    const view = EditorView.findFromDOM(host.shadowRoot!.querySelector('.cm-content')!)!;
    await vi.waitFor(() => expect(view.state.sliceDoc()).toContain('Europe/Paris'));
    view.dispatch({
      changes: {
        from: 0,
        to: view.state.doc.length,
        insert: view.state.sliceDoc().replace('Europe/Paris', 'Europe/Warsaw'),
      },
    });
    await tick();
    expect(staged.at(-1)?.document.merges).toEqual([
      { ...merge, overrides: { timezone: 'Europe/Warsaw' } },
    ]);
    await fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
    expect(staged.at(-1)).toEqual(draft);
  });

  it('keeps an unrepresentable null edit visible and unsavable after reopening, then allows correction and Undo', async () => {
    const renderer = vi.fn(renderFile);
    const merge = { path: 'renovate.json', strategy: 'deep-merge', overrides: { id: 1 } };
    const stored: SyncOverride = {
      kind: 'files',
      enabled: null,
      document: { merges: [merge] },
      revision: 1,
      unreadable: false,
    };
    let envelope = buildSyncOverrideEditorEnvelope(stored);
    const props = renderProps({
      renderFile: renderer,
      config: configWithTemplate('{"id":0,"flag":false}'),
      context: {
        repositories: 1,
        covered: 1,
        known_paths: [],
        repository_policies: [repositoryPolicy('repo-a', 'repo-1')],
        merges: [{ repository: 'repo-a', repository_id: 'repo-1', path: merge.path, merge }],
      },
      fetchOverride: async () => ({ stored, envelope }),
      onChangeOverride: (_id, _stored, next) => {
        envelope = next;
        return true;
      },
    });
    let component = render(SyncFilePage, { props });
    const open = async () => {
      await fireEvent.click(screen.getByRole('button', { name: /repo-a/ }));
      await vi.waitFor(() =>
        expect(
          screen
            .getByRole('dialog')
            .querySelector('.code-editor')
            ?.shadowRoot?.querySelector('.cm-content'),
        ).toBeInstanceOf(HTMLElement),
      );
      const host = screen.getByRole('dialog').querySelector('.code-editor')!;
      return EditorView.findFromDOM(host.shadowRoot!.querySelector('.cm-content')!)!;
    };
    let view = await open();
    await vi.waitFor(() =>
      expect(renderer.mock.calls.filter(([input]) => input.repository !== undefined)).toHaveLength(
        1,
      ),
    );
    renderer.mockClear();
    const invalid = '{"id":null,"flag":false}';
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: invalid } });
    await tick();
    await new Promise((resolve) => setTimeout(resolve, 180));
    expect(renderer.mock.calls.filter(([input]) => input.repository !== undefined)).toEqual([]);
    expect(syncOverrideBatchInput('repo-1', 1, envelope).ok).toBe(false);
    expect(screen.getByText('These merge rules cannot store a new null field')).toBeTruthy();
    expect(view.state.sliceDoc()).toBe(invalid);
    component.unmount();
    component = render(SyncFilePage, { props });
    view = await open();
    await new Promise((resolve) => setTimeout(resolve, 180));
    expect(renderer.mock.calls.filter(([input]) => input.repository !== undefined)).toEqual([]);
    expect(view.state.sliceDoc()).toBe(invalid);
    expect(screen.getByText('These merge rules cannot store a new null field')).toBeTruthy();
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: '{"id":2,"flag":false}' },
      annotations: isolateHistory.of('full'),
    });
    await tick();
    const corrected = syncOverrideBatchInput('repo-1', 1, envelope);
    expect(corrected.ok).toBe(true);
    if (corrected.ok) expect(JSON.stringify(corrected.input.document)).toContain('"id":2');
    await vi.waitFor(() =>
      expect(renderer.mock.calls.filter(([input]) => input.repository !== undefined)).toHaveLength(
        1,
      ),
    );
    await fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
    expect(view.state.sliceDoc()).toBe(invalid);
    expect(syncOverrideBatchInput('repo-1', 1, envelope).ok).toBe(false);
    component.unmount();
  });

  it.each([
    { saved: '9007199254740992', wanted: '9007199254740993', template: '0' },
    { saved: '-9007199254740992', wanted: '-9007199254740993', template: '0' },
    { saved: '9.007199254740992e15', wanted: '9.007199254740993e15', template: '0' },
    { saved: '-9.007199254740992e15', wanted: '-9.007199254740993e15', template: '0' },
    { saved: '1.50', wanted: '1.5000', template: '0' },
    { saved: '0', wanted: '9007199254740993', template: '9007199254740992' },
    { saved: '1', wanted: '1e400', template: '0' },
    { saved: '1', wanted: '-1e400', template: '0' },
    { saved: '1e400', wanted: '2e400', template: '0' },
    { saved: '-1e400', wanted: '-2e400', template: '0' },
    { saved: '0', wanted: '1e-400', template: '0' },
    { saved: '1e-400', wanted: '0', template: '1' },
    { saved: '0', wanted: '-0', template: '1' },
  ])(
    'stages the exact numeric wire literal $saved → $wanted',
    async ({ saved, wanted, template }) => {
      const renderer = vi.fn(renderFile);
      const merge = {
        path: 'renovate.json',
        strategy: 'deep-merge',
        overrides: { id: JSON.rawJSON(saved), flag: false },
      };
      const stored: SyncOverride = {
        kind: 'files',
        enabled: null,
        document: { merges: [merge] },
        revision: 1,
        updated_by: 'bart',
        updated_at: new Date(0).toISOString(),
        unreadable: false,
      };
      const staged: SyncOverrideEditorEnvelope[] = [];
      render(SyncFilePage, {
        props: renderProps({
          renderFile: renderer,
          config: configWithTemplate(`{"id":${template},"flag":false}`),
          context: {
            repositories: 1,
            covered: 1,
            known_paths: [],
            repository_policies: [repositoryPolicy('repo-a', 'repo-1')],
            merges: [{ repository: 'repo-a', repository_id: 'repo-1', path: merge.path, merge }],
          },
          fetchOverride: async () => ({
            stored,
            envelope: buildSyncOverrideEditorEnvelope(stored),
          }),
          onChangeOverride: (_id, _stored, next) => {
            expect(parseSyncOverrideEditorEnvelope(next)).not.toBeNull();
            staged.push(next);
            return true;
          },
        }),
      });
      await fireEvent.click(screen.getByRole('button', { name: /repo-a/ }));
      await vi.waitFor(() =>
        expect(
          screen
            .getByRole('dialog')
            .querySelector('.code-editor')
            ?.shadowRoot?.querySelector('.cm-content'),
        ).toBeInstanceOf(HTMLElement),
      );
      const host = screen.getByRole('dialog').querySelector('.code-editor')!;
      const view = EditorView.findFromDOM(host.shadowRoot!.querySelector('.cm-content')!)!;
      expect(view.state.sliceDoc()).toMatch(
        new RegExp(`"id":\\s*${saved.replace(/[.+-]/gu, '\\$&')}`),
      );
      const edit = async (text: string) => {
        view.dispatch({
          changes: { from: 0, to: view.state.doc.length, insert: text },
          annotations: isolateHistory.of('full'),
        });
        await tick();
      };
      const wire = () => {
        const next = staged.at(-1)!;
        const batch = syncOverrideBatchInput('repo-1', stored.revision, next);
        expect(batch.ok).toBe(true);
        return batch.ok ? JSON.stringify(batch.input.document) : '';
      };
      await edit(view.state.sliceDoc().replace(/"id":\s*[-+\d.eE]+/u, `"id":${wanted}`));
      expect(wire()).toContain(`"id":${wanted}`);
      await vi.waitFor(() => {
        const preview = renderer.mock.calls
          .filter(([input]) => input.repository !== undefined)
          .at(-1)?.[0];
        expect(JSON.stringify(preview?.repository?.merge)).toContain(`"id":${wanted}`);
      });
      await edit(view.state.sliceDoc().replace(/"flag":\s*false/u, '"flag":true'));
      expect(wire()).toContain(`"id":${wanted}`);
      await fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
      expect(wire()).toContain(`"id":${wanted}`);
      await fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
      expect(wire()).toContain(`"id":${saved}`);
      redo(view);
      await tick();
      expect(wire()).toContain(`"id":${wanted}`);
    },
  );

  it.each([
    { deduplicate: true, twoLists: false },
    { deduplicate: false, twoLists: false },
    { deduplicate: true, twoLists: true },
    { deduplicate: false, twoLists: true },
  ])(
    'retains pins, deduplicate=$deduplicate and rule order with twoLists=$twoLists',
    async ({ deduplicate, twoLists }) => {
      const otherOverrides = twoLists ? { reviewers: ['reviewer'] } : {};
      const merge = {
        path: 'renovate.json',
        strategy: 'deep-merge',
        overrides: { automerge: false, labels: ['repo'], ...otherOverrides },
        arrays: [
          { path: '$.labels', strategy: 'append' },
          ...(twoLists ? [{ path: '$.reviewers', strategy: 'append' }] : []),
        ],
        deduplicate,
      };
      const stored: SyncOverride = {
        kind: 'files',
        enabled: null,
        document: { merges: [merge] },
        revision: 1,
        updated_by: 'bart',
        updated_at: new Date(0).toISOString(),
        unreadable: false,
      };
      const saved = buildSyncOverrideEditorEnvelope(stored);
      const staged: SyncOverrideEditorEnvelope[] = [];
      render(SyncFilePage, {
        props: renderProps({
          config: configWithTemplate(
            twoLists
              ? '{"automerge":false,"reviewers":["base"],"labels":["base"]}'
              : '{"automerge":false,"labels":["base"]}',
          ),
          context: {
            repositories: 1,
            covered: 1,
            known_paths: [],
            repository_policies: [repositoryPolicy('repo-a', 'repo-1')],
            merges: [{ repository: 'repo-a', repository_id: 'repo-1', path: merge.path, merge }],
          },
          fetchOverride: async () => ({ stored, envelope: saved }),
          onChangeOverride: (_id, _canonical, next) => {
            expect(parseSyncOverrideEditorEnvelope(next)).not.toBeNull();
            staged.push(next);
            return true;
          },
        }),
      });

      await fireEvent.click(screen.getByRole('button', { name: /repo-a/ }));
      const choices = within(
        (await screen.findByText('$.labels')).closest('.list-ask') as HTMLElement,
      );
      await vi.waitFor(() =>
        expect(
          (choices.getByRole('radio', { name: /^Replace/ }) as HTMLInputElement).disabled,
        ).toBe(false),
      );
      await fireEvent.click(choices.getByRole('radio', { name: /^Replace/ }));
      expect(staged).toHaveLength(1);
      expect(staged[0]?.document.merges).toMatchObject([
        {
          path: merge.path,
          strategy: merge.strategy,
          overrides: { automerge: false, labels: ['base', 'repo'], ...otherOverrides },
        },
      ]);
      const replaced = (staged[0]?.document.merges as Array<Record<string, unknown>>)[0];
      expect(replaced?.deduplicate).toBe(twoLists ? deduplicate : undefined);
      expect(replaced?.arrays).toEqual(twoLists ? [merge.arrays[1]] : undefined);
      await fireEvent.click(choices.getByRole('radio', { name: /^Append/ }));
      expect(staged).toHaveLength(2);
      expect(staged[1]).toEqual(saved);
      await fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
      expect(staged.at(-1)).toEqual(staged[0]);
      await fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
      expect(staged.at(-1)).toEqual(saved);

      const host = screen.getByRole('dialog').querySelector('.code-editor')!;
      const view = EditorView.findFromDOM(host.shadowRoot!.querySelector('.cm-content')!)!;
      redo(view);
      await tick();
      expect(staged.at(-1)).toEqual(staged[0]);
      redo(view);
      await tick();
      expect(staged.at(-1)).toEqual(saved);

      view.dispatch({
        changes: {
          from: 0,
          to: view.state.doc.length,
          insert: view.state.sliceDoc().replace(/"automerge":\s*false/u, '"automerge": true'),
        },
      });
      await fireEvent.click(await screen.findByRole('button', { name: 'Stop changing automerge' }));
      const deliberatelyUnpinned = staged.at(-1);
      expect(deliberatelyUnpinned?.document.merges).toEqual([
        { ...merge, overrides: { labels: ['repo'], ...otherOverrides } },
      ]);
      await fireEvent.click(choices.getByRole('radio', { name: /^Replace/ }));
      await fireEvent.click(choices.getByRole('radio', { name: /^Append/ }));
      expect(staged.at(-1)).toEqual(deliberatelyUnpinned);
      view.dispatch({ changes: { from: 1, insert: ' ' } });
      await tick();
      expect(staged.at(-1)).toEqual(deliberatelyUnpinned);
    },
  );
});
