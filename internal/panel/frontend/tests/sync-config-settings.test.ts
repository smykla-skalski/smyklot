import { describe, expect, it } from 'vitest';

import { parseJson, type JsonValue } from '../src/lib/merge';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import type { SettingsDraftStorage } from '../src/lib/settings-draft-storage';
import {
  adoptSyncConfigSettings,
  buildSyncConfigEditorEnvelope,
  parseSyncConfigEditorEnvelope,
  stageSyncConfigControl,
  syncConfigBatchInput,
  syncConfigCommittedResource,
  syncConfigControls,
  syncConfigDraftEnvelope,
  syncConfigForEditor,
  syncConfigResource,
  syncConfigSavedControls,
  type SyncDocumentEditorEnvelope,
} from '../src/lib/sync-config-settings';
import type { SyncConfig } from '../src/lib/types';
import { emptySyncConfig } from '../stories/support/fixtures';

class MemoryStorage implements SettingsDraftStorage {
  readonly values = new Map<string, string>();

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }
}

function documentConfig(kind: 'settings' | 'rulesets' | 'files', text: string): SyncConfig {
  const document = parseJson(text);
  if (!isJsonRecord(document)) {
    throw new TypeError('test document must be an object');
  }
  return { ...emptySyncConfig(kind), kind, enabled: true, revision: 4, document };
}

function isJsonRecord(value: JsonValue | undefined): value is { [key: string]: JsonValue } {
  return (
    typeof value === 'object' &&
    value !== null &&
    !Array.isArray(value) &&
    !(typeof JSON.isRawJSON === 'function' && JSON.isRawJSON(value))
  );
}

describe('Sync configuration settings adapter [Unit]', () => {
  it('defines stable controls and stages each labels decision independently', () => {
    const config: SyncConfig = {
      ...emptySyncConfig('labels'),
      enabled: true,
      revision: 2,
      labels: [{ name: 'ci/run', color: 'abcdef', description: 'Run CI' }],
      allow_removal: false,
      excludes: ['local/*'],
    };
    expect(syncConfigControls('labels').map(({ id }) => id)).toEqual([
      'sync.labels.enabled',
      'sync.labels.labels',
      'sync.labels.allow_removal',
      'sync.labels.excludes',
    ]);
    const drafts = new SettingsDraftRegistry({ storage: null, now: () => 1, writerId: 'test' });
    drafts.hydrate('viewer-1');
    expect(adoptSyncConfigSettings(drafts, 'target-1', config)).toBe(true);
    const envelope = syncConfigDraftEnvelope(drafts, 'target-1', config);
    expect(
      stageSyncConfigControl(
        drafts,
        'target-1',
        config,
        { ...envelope, allow_removal: true },
        'sync.labels.allow_removal',
      ),
    ).toBe(true);

    expect(drafts.dirtyControls()).toMatchObject([
      { id: 'sync.labels.allow_removal', saved: false, value: true },
    ]);
    expect(
      syncConfigForEditor(config, syncConfigDraftEnvelope(drafts, 'target-1', config)),
    ).toMatchObject({ allow_removal: true, labels: config.labels, excludes: config.excludes });
  });

  it('preserves raw number literals and unknown document keys across restart and save', () => {
    const config = documentConfig(
      'settings',
      '{"future":{"huge":900719925474099312345,"ratio":1.50},"merge_method":"squash"}',
    );
    const envelope = buildSyncConfigEditorEnvelope(config);
    expect(envelope.kind).toBe('settings');
    if (envelope.kind === 'labels') return;
    expect(envelope.document_text).toContain('900719925474099312345');
    expect(envelope.document_text).toContain('1.50');

    const storage = new MemoryStorage();
    const first = new SettingsDraftRegistry({ storage, now: () => 10, writerId: 'first' });
    first.hydrate('viewer-1');
    adoptSyncConfigSettings(first, 'target-1', config);
    stageSyncConfigControl(
      first,
      'target-1',
      config,
      { ...envelope, enabled: false },
      'sync.settings.enabled',
    );

    const restarted = new SettingsDraftRegistry({ storage, now: () => 20, writerId: 'second' });
    expect(restarted.hydrate('viewer-1').restoredResources).toBe(1);
    const restored = syncConfigDraftEnvelope(restarted, 'target-1', config);
    expect(restored).toEqual({ ...envelope, enabled: false });
    const serialized = syncConfigBatchInput(4, restored);
    expect(serialized.ok).toBe(true);
    if (!serialized.ok || serialized.input.kind === 'labels') return;
    expect(JSON.stringify(serialized.input.document)).toBe(
      '{"future":{"huge":900719925474099312345,"ratio":1.50},"merge_method":"squash"}',
    );
  });

  it('persists malformed document text but blocks it before the API request', () => {
    const malformed: SyncDocumentEditorEnvelope = {
      kind: 'files',
      enabled: true,
      document_text: '{"files": [',
    };
    expect(parseSyncConfigEditorEnvelope(malformed, 'files')).toEqual(malformed);
    expect(syncConfigBatchInput(1, malformed)).toEqual({
      ok: false,
      problem: 'Files configuration is not a JSON object',
    });
  });

  it('commits compact document and labels responses with complete saved projections', () => {
    const documentState = {
      target_id: 'target-1',
      kind: 'rulesets' as const,
      enabled: false,
      document: { rulesets: [{ name: 'main' }] },
      revision: 8,
    };
    const document = syncConfigCommittedResource(documentState);
    expect(document.resource).toEqual(syncConfigResource('target-1', 'rulesets'));
    expect(document.revision).toBe(8);
    expect(document.savedControls['sync.rulesets.document']).toContain('"rulesets"');

    const labelState = {
      target_id: 'target-1',
      kind: 'labels' as const,
      enabled: true,
      document: {
        labels: [{ name: 'ci/run', color: 'abcdef', description: 'Run CI' }],
        allow_removal: true,
        excludes: ['local/*'],
      },
      revision: 9,
    };
    const labels = syncConfigCommittedResource(labelState);
    expect(labels.savedControls).toEqual(
      syncConfigSavedControls({
        kind: 'labels',
        enabled: true,
        labels: [{ name: 'ci/run', color: 'abcdef', description: 'Run CI' }],
        allow_removal: true,
        excludes: ['local/*'],
      }),
    );

    expect(() =>
      syncConfigCommittedResource({
        ...labelState,
        document: { labels: [{ name: 'ci/run' }], allow_removal: false },
      }),
    ).toThrow(/labels configuration is invalid/);
    expect(() =>
      syncConfigCommittedResource({
        ...documentState,
        document: [] as unknown as Record<string, unknown>,
      }),
    ).toThrow(/document is not an object/);
  });

  it('refuses unknown kinds, unreadable bases, and partial envelopes', () => {
    expect(() =>
      buildSyncConfigEditorEnvelope({ ...emptySyncConfig('labels'), unreadable: true }),
    ).toThrow(/unreadable/);
    expect(() => buildSyncConfigEditorEnvelope(emptySyncConfig('future'))).toThrow(/unknown/);
    expect(parseSyncConfigEditorEnvelope({ kind: 'settings', enabled: true })).toBeNull();
    expect(
      parseSyncConfigEditorEnvelope({
        kind: 'labels',
        enabled: true,
        labels: [{ name: 'ci', color: 'ffffff', extra: true }],
        allow_removal: false,
        excludes: [],
      }),
    ).toBeNull();
  });
});
