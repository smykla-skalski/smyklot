import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { FileDraftValidation, fileDraftValidationSnapshot } from '../src/lib/file-draft-validation';
import { formatJson, parseJson, type JsonValue } from '../src/lib/merge';
import {
  SettingsDraftRegistry,
  type SettingsDraftStorage,
} from '../src/lib/settings-drafts.svelte';
import { saveWorkspaceDrafts } from '../src/lib/workspace-settings-save';
import {
  adoptSyncConfigSettings,
  buildSyncConfigEditorEnvelope,
  stageSyncConfigControl,
} from '../src/lib/sync-config-settings';
import {
  adoptSyncOverrideSettings,
  buildSyncOverrideEditorEnvelope,
  stageSyncOverrideControl,
} from '../src/lib/repository-sync-override-settings';
import { emptySyncConfig } from '../stories/support/fixtures';
import type { SyncConfig, SyncOverride } from '../src/lib/types';
import type {
  SyncFileRenderInput,
  SyncFileRenderResponse,
} from '../src/lib/sync-file-render.generated';

const scope = { type: 'workspace', targetId: '2001' } as const;
const path = '.config/quality.yaml';
const valid: SyncFileRenderResponse = {
  valid: true,
  final_content: '',
  matches_formatting: true,
  diagnostics: [],
};
const invalid: SyncFileRenderResponse = {
  ...valid,
  valid: false,
  diagnostics: [{ stage: 'parse', code: 'invalid_template', message: 'Invalid YAML' }],
};
const config: SyncConfig = {
  ...emptySyncConfig('files'),
  revision: 1,
  document: { files: [{ path, content: 'enabled: true\n' }] },
};
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe('application-owned file validation [Unit]', () => {
  const coordinators: FileDraftValidation[] = [];
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    for (const owner of coordinators) owner.dispose();
    coordinators.length = 0;
    vi.useRealTimers();
  });

  function setup(options: { files?: boolean; storage?: SettingsDraftStorage | null } = {}) {
    const registry = new SettingsDraftRegistry({
      storage: options.storage ?? null,
      writerId: 'test',
    });
    registry.hydrate('viewer');
    if (options.files !== false) adoptSyncConfigSettings(registry, scope.targetId, config);
    const api = {
      renderSyncFile: vi.fn<
        (targetId: string, input: SyncFileRenderInput) => Promise<SyncFileRenderResponse>
      >(async () => valid),
      fetchSyncConfig: vi.fn(async () => config),
    };
    const owner = new FileDraftValidation(registry, api);
    coordinators.push(owner);
    const update = () => owner.update(fileDraftValidationSnapshot(registry));
    const edit = (content: string) => {
      expect(
        stageSyncConfigControl(
          registry,
          scope.targetId,
          config,
          {
            kind: 'files',
            enabled: true,
            document_text: formatJson({ files: [{ path, content }] }),
          },
          'sync.files.document',
        ),
      ).toBe(true);
      update();
    };
    return { registry, api, owner, update, edit };
  }

  it('validates another tab’s imported file draft before the atomic save begins', async () => {
    const values = new Map<string, string>();
    const storage = {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => {
        values.set(key, value);
      },
    };
    const { registry, api, update } = setup({ storage });
    const settings = emptySyncConfig('settings');
    adoptSyncConfigSettings(registry, scope.targetId, settings);
    stageSyncConfigControl(
      registry,
      scope.targetId,
      settings,
      { ...buildSyncConfigEditorEnvelope(settings), enabled: true },
      'sync.settings.enabled',
    );
    update();
    expect(registry.validationProblem(scope)).toBeNull();

    const otherTab = new SettingsDraftRegistry({ storage, writerId: 'other-tab' });
    otherTab.hydrate('viewer');
    adoptSyncConfigSettings(otherTab, scope.targetId, config);
    stageSyncConfigControl(
      otherTab,
      scope.targetId,
      config,
      {
        kind: 'files',
        enabled: true,
        document_text: formatJson({ files: [{ path, content: 'unfinished: [\n' }] }),
      },
      'sync.files.document',
    );
    // No storage event or reactive layout update is delivered to the saving tab.
    expect(
      registry
        .dirtyResources()
        .some(({ resource }) => resource.type === 'sync-config' && resource.kind === 'files'),
    ).toBe(false);
    const save = vi.fn(async () => {
      throw new Error('Unexpected transport call');
    });
    expect((await saveWorkspaceDrafts(registry, scope.targetId, save)).saved).toBe(false);
    expect(save).not.toHaveBeenCalled();
    expect(registry.validationProblem(scope)).toContain('Checking file content');
    api.renderSyncFile.mockResolvedValue(invalid);
    await vi.advanceTimersByTimeAsync(120);
    expect(registry.validationProblem(scope)).toContain('Invalid YAML');
    expect((await saveWorkspaceDrafts(registry, scope.targetId, save)).saved).toBe(false);
    expect(save).not.toHaveBeenCalled();
    otherTab.dispose();
  });

  it('debounces an immutable dirty snapshot independently of any editor', async () => {
    const { registry, api, edit } = setup();
    edit('enabled: false\n');
    edit('enabled: false\nretries: 2\n');
    expect(registry.beginSave(scope)).toBeNull();
    await vi.advanceTimersByTimeAsync(119);
    expect(api.renderSyncFile).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(api.renderSyncFile).toHaveBeenCalledTimes(1);
    expect(api.renderSyncFile).toHaveBeenCalledWith('2001', {
      path,
      draft_content: 'enabled: false\nretries: 2\n',
      template_formatting: {},
    });
    expect(registry.validationProblem(scope)).toBeNull();
    expect(registry.beginSave(scope)).not.toBeNull();
  });

  it('keeps the first validation message paired with its file across scopes and discard', async () => {
    const { registry, api, owner, edit, update } = setup();
    api.renderSyncFile.mockResolvedValue(invalid);
    edit('unfinished: [');
    await vi.advanceTimersByTimeAsync(120);
    const issue = registry.validationIssue(scope)!;
    expect(issue.problem).toContain('Invalid YAML');
    expect(owner.problemFile(scope, issue.controlId)).toEqual({ path, repositoryId: '' });
    expect(owner.problemFile({ type: 'workspace', targetId: 'other' }, issue.controlId)).toBeNull();
    expect(owner.problemFile(scope, 'unrelated')).toBeNull();
    registry.discardScope(scope);
    update();
    expect(owner.problemFile(scope, issue.controlId)).toBeNull();
  });

  it('keeps an invalid result blocking after its requesting editor is gone', async () => {
    const { registry, api, edit, update } = setup();
    api.renderSyncFile.mockResolvedValue(invalid);
    edit('unfinished: [\n');
    await vi.advanceTimersByTimeAsync(120);
    update();
    expect(registry.validationProblem(scope)).toContain('Invalid YAML');
    expect(registry.beginSave(scope)).toBeNull();
    expect(api.renderSyncFile).toHaveBeenCalledTimes(1);
  });

  it('settles a failed check when a fresh same-input preview succeeds', async () => {
    const { registry, api, owner, edit } = setup();
    api.renderSyncFile
      .mockRejectedValueOnce(new Error('Connection interrupted'))
      .mockResolvedValue(valid);
    edit('enabled: false\n');
    await vi.advanceTimersByTimeAsync(120);
    expect(registry.validationProblem(scope)).toContain('Connection interrupted');
    await owner.render(scope.targetId, {
      path,
      draft_content: 'enabled: false\n',
      template_formatting: {},
    });
    expect(api.renderSyncFile).toHaveBeenCalledTimes(2);
    expect(registry.validationProblem(scope)).toBeNull();
    expect(registry.beginSave(scope)).not.toBeNull();
  });

  it.each(['same template', 'another template'] as const)(
    'validates invalid content with numeric formatting on %s',
    async (location) => {
      const { registry, api, update } = setup();
      api.renderSyncFile.mockResolvedValue(invalid);
      const files: JsonValue[] =
        location === 'same template'
          ? [{ path, content: 'unfinished: [\n', formatting: { common: { indent_width: 2 } } }]
          : [
              { path, content: 'unfinished: [\n' },
              {
                path: 'other.yaml',
                content: 'valid: true\n',
                formatting: { common: { indent_width: 2 } },
              },
            ];
      expect(
        stageSyncConfigControl(
          registry,
          scope.targetId,
          config,
          {
            kind: 'files',
            enabled: true,
            document_text: formatJson({ files }),
          },
          'sync.files.document',
        ),
      ).toBe(true);
      update();
      expect(registry.beginSave(scope)).toBeNull();
      await vi.advanceTimersByTimeAsync(120);
      expect(api.renderSyncFile.mock.calls.some(([, input]) => input.path === path)).toBe(true);
      expect(registry.validationProblem(scope)).toContain('Invalid YAML');
      expect(registry.beginSave(scope)).toBeNull();
    },
  );

  it.each(['discard', 'account', 'remove'] as const)(
    'ignores a late invalid result after %s',
    async (action) => {
      const { registry, api, edit, update } = setup();
      const pending = deferred<SyncFileRenderResponse>();
      api.renderSyncFile.mockReturnValue(pending.promise);
      edit('enabled: false\n');
      await vi.advanceTimersByTimeAsync(120);
      if (action === 'discard') registry.discardScope(scope);
      else if (action === 'account') registry.hydrate('another-viewer');
      else
        stageSyncConfigControl(
          registry,
          scope.targetId,
          config,
          { kind: 'files', enabled: true, document_text: '{"files":[]}' },
          'sync.files.document',
        );
      update();
      pending.resolve(invalid);
      await vi.advanceTimersByTimeAsync(0);
      expect(registry.validationProblem(scope)).toBeNull();
    },
  );

  it('compares the live draft again before accepting a response', async () => {
    const { registry, api, edit } = setup();
    const pending = deferred<SyncFileRenderResponse>();
    api.renderSyncFile.mockReturnValueOnce(pending.promise).mockResolvedValue(valid);
    edit('unfinished: [\n');
    await vi.advanceTimersByTimeAsync(120);
    // Deliberately do not call the layout update before the old response arrives.
    stageSyncConfigControl(
      registry,
      scope.targetId,
      config,
      {
        kind: 'files',
        enabled: true,
        document_text: formatJson({ files: [{ path, content: 'fixed: true\n' }] }),
      },
      'sync.files.document',
    );
    pending.resolve(invalid);
    await vi.advanceTimersByTimeAsync(120);
    expect(registry.validationProblem(scope)).toBeNull();
    expect(api.renderSyncFile).toHaveBeenCalledTimes(2);
  });

  it('shares an in-flight preview request with the save check', async () => {
    const { registry, api, owner, edit } = setup();
    const pending = deferred<SyncFileRenderResponse>();
    api.renderSyncFile.mockReturnValue(pending.promise);
    edit('enabled: false\n');
    const preview = owner.render(scope.targetId, {
      path,
      draft_content: 'enabled: false\n',
      template_formatting: {},
    });
    await vi.advanceTimersByTimeAsync(120);
    expect(api.renderSyncFile).toHaveBeenCalledTimes(1);
    pending.resolve(valid);
    await preview;
    await vi.advanceTimersByTimeAsync(0);
    expect(registry.validationProblem(scope)).toBeNull();
  });

  it('does not combine requests whose exact numeric literals differ', async () => {
    const { api, owner } = setup();
    const pending = deferred<SyncFileRenderResponse>();
    api.renderSyncFile.mockReturnValue(pending.promise);
    const render = (literal: string) =>
      owner.render(scope.targetId, {
        path,
        draft_content: 'id: 0\n',
        template_formatting: {},
        repository: {
          id: '4001',
          path_formatting: {},
          merge: {
            overrides: parseJson(`{"id":${literal}}`) as Record<string, unknown>,
          },
        },
      });
    const first = render('9007199254740992');
    const second = render('9007199254740993');
    expect(api.renderSyncFile).toHaveBeenCalledTimes(2);
    pending.resolve(valid);
    await Promise.all([first, second]);
  });

  it('loads a template for a restored repository draft without a mounted Sync view', async () => {
    const { registry, api, owner, update } = setup({ files: false });
    const stored: SyncOverride = {
      kind: 'files',
      enabled: null,
      document: {},
      revision: 1,
      unreadable: false,
    };
    adoptSyncOverrideSettings(registry, scope.targetId, '4001', stored);
    const envelope = buildSyncOverrideEditorEnvelope(stored);
    envelope.document = {
      merges: [
        {
          path,
          overrides: parseJson('{"id":1e400,"other":-0,"small":1e-400,"large":9007199254740993}')!,
        },
      ],
    };
    envelope.override_texts = ['{"id":1e400,"other":-0,"small":1e-400,"large":9007199254740993}'];
    stageSyncOverrideControl(
      registry,
      scope.targetId,
      '4001',
      stored,
      envelope,
      'repositories.4001.sync.files.document',
    );
    update();
    expect(registry.validationProblem(scope)).toContain('Loading shared files');
    expect(owner.problemFile(scope, registry.validationIssue(scope)?.controlId)).toBeNull();
    api.renderSyncFile.mockResolvedValue(invalid);
    await vi.advanceTimersByTimeAsync(120);
    expect(api.fetchSyncConfig).toHaveBeenCalledTimes(1);
    expect(api.renderSyncFile).toHaveBeenCalledTimes(1);
    const wire = JSON.stringify(api.renderSyncFile.mock.calls[0]);
    expect(wire).toContain('1e400');
    expect(wire).toContain('"other":-0');
    expect(wire).toContain('1e-400');
    expect(wire).toContain('9007199254740993');
    expect(owner.problemFile(scope, registry.validationIssue(scope)?.controlId)).toEqual({
      path,
      repositoryId: '4001',
    });
  });

  it('does not render stale valid content behind an unfinished raw override', async () => {
    const { registry, api, update } = setup();
    const stored: SyncOverride = {
      kind: 'files',
      enabled: null,
      document: {},
      revision: 1,
      unreadable: false,
    };
    adoptSyncOverrideSettings(registry, scope.targetId, '4001', stored);
    const envelope = buildSyncOverrideEditorEnvelope(stored);
    envelope.document = { merges: [{ path, overrides: {} }] };
    envelope.override_texts = ['{"unfinished":'];
    stageSyncOverrideControl(
      registry,
      scope.targetId,
      '4001',
      stored,
      envelope,
      'repositories.4001.sync.files.document',
    );
    update();
    await vi.advanceTimersByTimeAsync(120);
    expect(api.renderSyncFile).not.toHaveBeenCalled();
    expect(registry.validationProblem(scope)).toBeNull();
  });
});
