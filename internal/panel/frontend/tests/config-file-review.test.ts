import { describe, expect, it, vi } from 'vitest';
import { PanelApiError } from '../src/lib/api';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import { parseJson, formatJson } from '../src/lib/merge';
import {
  ConfigurationReview,
  type ConfigurationReviewSource,
} from '../src/lib/config-file-review.svelte';
import type { ConfigFilePreview } from '../src/lib/config-file-sync';

const preview = (): ConfigFilePreview => ({
  status: 'blocked',
  checked_at: '2026-09-07T12:00:00Z',
  problem: 'conflicting_edits',
  review_token: 'a'.repeat(64),
  conflict_count: 1,
  conflict_paths: [['command_prefix']],
  choices: ['panel', 'file'].map((side) => ({
    side: side as 'panel' | 'file',
    available: true,
    import_panel: true,
    publish_file: true,
    document: parseJson(
      `{"command_prefix":"/${side}","quiet_success":false,"allow_self_approval":true,"panel":{"version":1,"scope":"repository","sync":{"files":{"document":{"merges":[{"path":"renovate.json","overrides":{"id":9007199254740993,"tiny":1e-400}}]}}}}}`,
    ),
  })),
});
function setup() {
  let source: ConfigurationReviewSource = {
    identity: 'account/workspace/repository',
    hasDrafts: false,
    canWrite: true,
    enabled: true,
    fileIgnored: false,
    preview: vi.fn(async () => preview()),
    resolve: vi.fn(async () => ({ status: 'pending' as const })),
    onResolved: vi.fn(),
  };
  const model = new ConfigurationReview(() => source);
  return {
    model,
    get source() {
      return source;
    },
    replace(next: Partial<ConfigurationReviewSource>) {
      source = { ...source, ...next };
    },
  };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe('fresh configuration-file review [Unit]', () => {
  it.each(['comparison', 'resolution'] as const)(
    'checks storage before %s when another tab has not delivered its event',
    async (operation) => {
      const values = new Map<string, string>();
      const storage = {
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => {
          values.set(key, value);
        },
        removeItem: (key: string) => {
          values.delete(key);
        },
      };
      const first = new SettingsDraftRegistry({ storage, writerId: 'first' });
      const second = new SettingsDraftRegistry({ storage, writerId: 'second' });
      first.hydrate('viewer');
      second.hydrate('viewer');
      const scope = { type: 'workspace', targetId: 'workspace' } as const;
      const context = setup();
      const model = new ConfigurationReview(() => ({
        ...context.source,
        prepare: () => first.refreshFromStorage(),
        hasDrafts: first.dirtyControls(scope).length > 0,
      }));
      if (operation === 'resolution') {
        await model.show();
        model.select('panel');
      }
      const resource = { type: 'target-defaults', targetId: 'workspace' } as const;
      second.adoptBase(resource, 1, { enabled: false });
      second.stage(
        resource,
        { enabled: true },
        {
          id: 'defaults.enabled',
          location: { section: 'defaults', path: ['enabled'] },
          saved: false,
          value: true,
        },
      );
      expect(first.dirtyControls(scope)).toHaveLength(0);
      if (operation === 'comparison') await model.show();
      else await model.submit();
      expect(first.dirtyControls(scope)).toHaveLength(1);
      expect(context.source.resolve).not.toHaveBeenCalled();
      expect(context.source.preview).toHaveBeenCalledTimes(operation === 'comparison' ? 0 : 1);
      expect(model.canSubmit).toBe(false);
      first.dispose();
      second.dispose();
    },
  );
  it.each(['panel', 'file'] as const)(
    'previews %s without writing and submits only the explicit current token/side',
    async (side) => {
      const { model, source } = setup();
      await model.show();
      expect(model.selected).toBeNull();
      expect(source.resolve).not.toHaveBeenCalled();
      model.select(side);
      const text = formatJson(model.choice!.document!);
      expect(text).toContain('9007199254740993');
      expect(text).toContain('1e-400');
      expect(text).toContain('"quiet_success": false');
      expect(text).toContain('"allow_self_approval": true');
      expect(source.resolve).not.toHaveBeenCalled();
      await model.submit();
      expect(source.resolve).toHaveBeenCalledExactlyOnceWith({
        review_token: 'a'.repeat(64),
        side,
      });
      expect(model.pending).toBe(true);
      expect(model.canSubmit).toBe(false);
      await model.submit();
      expect(source.resolve).toHaveBeenCalledTimes(1);
      expect(source.onResolved).toHaveBeenCalledTimes(1);
    },
  );
  it('clears a stale choice and requires a new selection after a 409 comparison refresh', async () => {
    const context = setup();
    const read = vi
      .fn()
      .mockResolvedValueOnce(preview())
      .mockResolvedValueOnce({ ...preview(), review_token: 'b'.repeat(64) });
    context.replace({
      preview: read,
      resolve: vi
        .fn()
        .mockRejectedValue(new PanelApiError(409, 'config_file_changed', 'Settings changed')),
    });
    await context.model.show();
    context.model.select('file');
    await context.model.submit();
    expect(read).toHaveBeenCalledTimes(2);
    expect(context.model.selected).toBeNull();
    expect(context.model.canSubmit).toBe(false);
    expect(context.model.preview?.review_token).toBe('b'.repeat(64));
  });
  it.each([410, 422])(
    'invalidates the token after HTTP %s instead of repeating a write',
    async (status) => {
      const context = setup();
      context.replace({
        resolve: vi.fn().mockRejectedValue(new PanelApiError(status, 'blocked', 'Cannot continue')),
      });
      await context.model.show();
      context.model.select('panel');
      await context.model.submit();
      await context.model.submit();
      expect(context.source.resolve).toHaveBeenCalledTimes(1);
      expect(context.model.preview).toBeNull();
      expect(context.model.problem).toContain('Cannot continue');
    },
  );
  it.each(['draft', 'off', 'ignored'] as const)(
    'does not compare or resolve when %s blocks saved settings',
    async (reason) => {
      const context = setup();
      context.replace(
        reason === 'draft'
          ? { hasDrafts: true }
          : reason === 'off'
            ? { enabled: false }
            : { fileIgnored: true },
      );
      await context.model.show();
      expect(context.source.preview).not.toHaveBeenCalled();
      expect(context.model.canSubmit).toBe(false);
    },
  );
  it('allows read-only comparison and selection but never writes', async () => {
    const context = setup();
    context.replace({ canWrite: false });
    await context.model.show();
    context.model.select('panel');
    await context.model.submit();
    expect(context.model.choice).not.toBeNull();
    expect(context.source.resolve).not.toHaveBeenCalled();
  });
  it.each(['close', 'owner', 'draft'] as const)(
    'ignores a delayed preview after %s changes',
    async (change) => {
      const context = setup();
      const pending = deferred<ConfigFilePreview>();
      context.replace({ preview: () => pending.promise });
      const loading = context.model.show();
      if (change === 'close') context.model.close();
      else
        context.replace(change === 'owner' ? { identity: 'another/account' } : { hasDrafts: true });
      pending.resolve(preview());
      await loading;
      expect(context.model.preview).toBeNull();
      expect(context.model.canSubmit).toBe(false);
    },
  );
  it('does not publish old resolution completion into another owner', async () => {
    const context = setup();
    const pending = deferred<{ status: 'pending' }>();
    context.replace({ resolve: () => pending.promise });
    await context.model.show();
    context.model.select('panel');
    const saving = context.model.submit();
    context.replace({ identity: 'other' });
    pending.resolve({ status: 'pending' });
    await saving;
    expect(context.model.pending).toBe(false);
    expect(context.source.onResolved).not.toHaveBeenCalled();
  });
  it('does not invent a file-side choice when a deleted file only supports recreation', async () => {
    const context = setup();
    context.replace({
      preview: async () => ({
        ...preview(),
        problem: 'file_removed',
        choices: [preview().choices![0]!],
      }),
    });
    await context.model.show();
    context.model.select('file');
    expect(context.model.selected).toBe('panel');
    expect(context.model.canSubmit).toBe(true);
  });
  it.each(['invalid_file', 'proposal_outstanding', 'workspace_repository_missing'])(
    'has no resolution for %s without a backend choice',
    async (problem) => {
      const context = setup();
      context.replace({ preview: async () => ({ status: 'blocked', checked_at: '', problem }) });
      await context.model.show();
      context.model.select('panel');
      expect(context.model.choice).toBeNull();
      expect(context.model.canSubmit).toBe(false);
    },
  );
});
