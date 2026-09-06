import { describe, expect, it } from 'vitest';

import {
  adoptSyncOverrideSettings,
  buildSyncOverrideEditorEnvelope,
  cloneSyncOverrideEditorEnvelope,
  parseSyncOverrideEditorEnvelope,
  serializeSyncOverrideDocument,
  stageSyncOverrideControl,
  syncOverrideBatchInput,
  syncOverrideCommittedResource,
  syncOverrideControls,
  syncOverrideDraftEnvelope,
  syncOverrideResource,
  syncOverrideSavedControls,
  syncOverrideFormattingEntries,
  withSyncOverrideFormatting,
  type SyncOverrideEditorEnvelope,
} from '../src/lib/repository-sync-override-settings';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import { parseJson } from '../src/lib/merge';
import type { SettingsDraftStorage, SettingsJson } from '../src/lib/settings-draft-storage';
import type { WorkspaceSyncOverrideSettingsState, SyncOverride } from '../src/lib/types';

function override(over: Partial<SyncOverride> = {}): SyncOverride {
  return {
    kind: 'files',
    enabled: null,
    document: {},
    revision: 0,
    unreadable: false,
    ...over,
  };
}

class MemoryStorage implements SettingsDraftStorage {
  readonly values = new Map<string, string>();

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }
}

describe('repository sync override settings adapter [Unit]', () => {
  it('normalizes signed zero only in typed Markdown metadata through draft reload and save', () => {
    const stored = override({
      document: parseJson(
        '{"merges":[{"path":"README.md","strategy":"markdown","sections":[{"action":"delete","heading":"# Old","occurrence":-0}]},{"path":"renovate.json","overrides":{"id":-0}}]}',
      ) as Record<string, unknown>,
    });
    const storage = new MemoryStorage();
    const first = new SettingsDraftRegistry({ storage, writerId: 'first' });
    first.hydrate('viewer');
    adoptSyncOverrideSettings(first, 'target', 'repo', stored);
    const envelope = buildSyncOverrideEditorEnvelope(stored);
    expect(
      stageSyncOverrideControl(
        first,
        'target',
        'repo',
        stored,
        { ...envelope, enabled: false },
        'repositories.repo.sync.files.enabled',
      ),
    ).toBe(true);
    const second = new SettingsDraftRegistry({ storage, writerId: 'second' });
    second.hydrate('viewer');
    const reopened = syncOverrideDraftEnvelope(second, 'target', 'repo', stored);
    const batch = syncOverrideBatchInput('repo', stored.revision, reopened);
    expect(batch.ok).toBe(true);
    if (batch.ok) {
      expect(JSON.stringify(batch.input.document)).toContain('"occurrence":0');
      expect(JSON.stringify(batch.input.document)).toContain('"id":-0');
    }
  });

  it('keeps typed formatting and Markdown metadata primitive beside lossless file content', () => {
    const source = {
      formats: [
        { path: 'renovate.json', formatting: { common: { indent_width: 4, line_width: 80 } } },
      ],
      merges: [
        {
          path: 'renovate.json',
          overrides: { id: JSON.rawJSON('1e400'), data: { rawJSON: '1e400' } },
        },
        {
          path: 'README.md',
          strategy: 'markdown',
          sections: [{ action: 'delete', heading: '# Old', occurrence: 1 }],
        },
      ],
    };
    const document = parseJson(JSON.stringify(source)) as Record<string, unknown>;
    const envelope = buildSyncOverrideEditorEnvelope(override({ document }));
    expect(syncOverrideFormattingEntries(envelope)).toEqual(source.formats);
    expect((envelope.document.merges as Array<Record<string, unknown>>)[1].sections).toEqual(
      source.merges[1].sections,
    );
    const batch = syncOverrideBatchInput('repo', 3, envelope);
    expect(batch.ok).toBe(true);
    if (batch.ok) expect(JSON.stringify(batch.input.document)).toContain('"id":1e400');
  });

  it.each(['1e400', '-1e400', '1e-400', '-0', '1.50', '9007199254740993', '42'])(
    'loads, persists, reopens and saves the exact numeric literal %s',
    (literal) => {
      const document = {
        merges: [
          {
            path: 'renovate.json',
            overrides: { id: JSON.rawJSON(literal), data: { rawJSON: literal } },
          },
        ],
      };
      const stored = override({ document, revision: 3 });
      const storage = new MemoryStorage();
      const first = new SettingsDraftRegistry({ storage, now: () => 10, writerId: 'first' });
      first.hydrate('viewer');
      expect(adoptSyncOverrideSettings(first, 'target', 'repo', stored)).toBe(true);
      const saved = buildSyncOverrideEditorEnvelope(stored);
      expect(JSON.stringify(saved.document)).toBe(JSON.stringify(document));
      expect(
        stageSyncOverrideControl(
          first,
          'target',
          'repo',
          stored,
          {
            ...saved,
            enabled: false,
          },
          'repositories.repo.sync.files.enabled',
        ),
      ).toBe(true);
      const second = new SettingsDraftRegistry({ storage, now: () => 20, writerId: 'second' });
      second.hydrate('viewer');
      const reopened = syncOverrideDraftEnvelope(second, 'target', 'repo', stored);
      expect(reopened.enabled).toBe(false);
      expect(second.resource(syncOverrideResource('target', 'repo'))?.expectedRevision).toBe(3);
      expect(JSON.stringify(reopened.document)).toBe(JSON.stringify(document));
      const batch = syncOverrideBatchInput('repo', 3, reopened);
      expect(batch.ok).toBe(true);
      if (!batch.ok) return;
      expect(JSON.stringify(batch.input.document)).toContain(`"id":${literal}`);
      expect(JSON.stringify(batch.input.document)).toContain(`"data":{"rawJSON":"${literal}"}`);
      expect(
        stageSyncOverrideControl(
          second,
          'target',
          'repo',
          stored,
          saved,
          'repositories.repo.sync.files.enabled',
        ),
      ).toBe(true);
      expect(second.dirty).toBe(false);
    },
  );

  it('clears a strategy override when the picker returns to the saved default', () => {
    const stored = override({
      document: { merges: [{ path: 'renovate.json', overrides: { enabled: true } }] },
    });
    const drafts = new SettingsDraftRegistry({ storage: null, writerId: 'test' });
    drafts.hydrate('viewer');
    adoptSyncOverrideSettings(drafts, 'target', 'repo', stored);
    const saved = buildSyncOverrideEditorEnvelope(stored);
    for (const strategy of ['deep-merge', '']) {
      const next = cloneSyncOverrideEditorEnvelope(saved);
      next.document.merges = [{ path: 'renovate.json', overrides: { enabled: true }, strategy }];
      expect(
        stageSyncOverrideControl(
          drafts,
          'target',
          'repo',
          stored,
          next,
          'repositories.repo.sync.files.document',
        ),
      ).toBe(true);
      expect(drafts.dirtyControls()).toHaveLength(strategy === '' ? 0 : 1);
    }
    expect(syncOverrideDraftEnvelope(drafts, 'target', 'repo', stored)).toEqual(saved);
    expect(drafts.beginSave({ type: 'workspace', targetId: 'target' })).toBeNull();
  });

  it.each(['config.json', 'config.JSONC', 'config.yaml', 'config.yml', 'config.TOML'])(
    'preserves content and list rules when saving a supported structured file: %s',
    (path) => {
      const merge = {
        path,
        strategy: 'deep-merge',
        overrides: { tools: ['local'] },
        arrays: [{ path: '$.tools', strategy: 'append' }],
        deduplicate: true,
      };
      const envelope = buildSyncOverrideEditorEnvelope(override({ document: { merges: [merge] } }));
      const result = syncOverrideBatchInput('repo', 3, envelope);
      expect(result).toEqual({
        ok: true,
        input: {
          repository_id: 'repo',
          kind: 'files',
          enabled: null,
          document: { merges: [merge] },
          expected_revision: 3,
        },
      });
      envelope.override_texts = ['{"tools":'];
      expect(syncOverrideBatchInput('repo', 3, envelope).ok).toBe(false);
    },
  );

  it('clears restored JSON content independently of editor whitespace and key order', () => {
    const stored = override({
      document: {
        merges: [{ path: 'renovate.json', overrides: { timezone: 'UTC', enabled: true } }],
      },
    });
    const drafts = new SettingsDraftRegistry({ storage: null, writerId: 'test' });
    drafts.hydrate('viewer');
    adoptSyncOverrideSettings(drafts, 'target', 'repo', stored);
    const saved = buildSyncOverrideEditorEnvelope(stored);
    for (const text of [
      '{"timezone":"CET"}',
      '{"timezone":',
      '{"timezone":"UTC","enabled":true}',
    ]) {
      expect(
        stageSyncOverrideControl(
          drafts,
          'target',
          'repo',
          stored,
          {
            ...saved,
            override_texts: [text],
          },
          'repositories.repo.sync.files.document',
        ),
      ).toBe(true);
      expect(drafts.dirtyControls()).toHaveLength(text.includes('UTC') ? 0 : 1);
    }
    expect(syncOverrideDraftEnvelope(drafts, 'target', 'repo', stored)).toEqual(saved);
    expect(drafts.beginSave({ type: 'workspace', targetId: 'target' })).toBeNull();
  });

  it('defines stable file-sync control IDs and repository locations', () => {
    expect(syncOverrideControls('repo-1')).toEqual([
      {
        id: 'repositories.repo-1.sync.files.enabled',
        location: {
          section: 'repositories',
          path: ['repo-1', 'sync', 'files', 'enabled'],
        },
      },
      {
        id: 'repositories.repo-1.sync.files.document',
        location: {
          section: 'repositories',
          path: ['repo-1', 'sync', 'files', 'document'],
        },
      },
    ]);
  });

  it('builds a complete canonical envelope for an absent revision-zero override', () => {
    const envelope = buildSyncOverrideEditorEnvelope(override());
    expect(envelope).toEqual({ enabled: null, document: {}, override_texts: [] });
    expect(parseSyncOverrideEditorEnvelope(envelope)).toEqual(envelope);

    const drafts = new SettingsDraftRegistry({ storage: null, writerId: 'test' });
    drafts.hydrate('viewer-1');
    expect(adoptSyncOverrideSettings(drafts, 'target-1', 'repo-1', override())).toBe(true);
    expect(drafts.resource(syncOverrideResource('target-1', 'repo-1'))?.expectedRevision).toBe(0);
  });

  it('deep-clones unknown document keys and keeps raw merge text independent', () => {
    const source = override({
      document: {
        future: { nested: ['keep-me'] },
        merges: [
          {
            path: 'renovate.json',
            overrides: { timezone: 'UTC' },
            future_merge_key: { untouched: true },
          },
        ],
      },
    });
    const envelope = buildSyncOverrideEditorEnvelope(source);
    const cloned = cloneSyncOverrideEditorEnvelope(envelope);
    (cloned.document.future as { nested: string[] }).nested.push('changed');
    cloned.override_texts[0] = '{"timezone": ';

    expect((envelope.document.future as { nested: string[] }).nested).toEqual(['keep-me']);
    expect(
      (envelope.document.merges as Array<Record<string, unknown>>)[0].future_merge_key,
    ).toEqual({ untouched: true });
    expect(envelope.override_texts[0]).toContain('"UTC"');
    expect(source.document.future).toEqual({ nested: ['keep-me'] });
  });

  it('preserves an unknown key and malformed raw text through a browser restart', () => {
    const storage = new MemoryStorage();
    const stored = override({
      document: { merges: [{ path: 'renovate.json', overrides: { timezone: 'UTC' } }] },
    });
    const first = new SettingsDraftRegistry({ storage, now: () => 10, writerId: 'first' });
    first.hydrate('viewer-1');
    adoptSyncOverrideSettings(first, 'target-1', 'repo-1', stored);

    const next: SyncOverrideEditorEnvelope = {
      ...syncOverrideDraftEnvelope(first, 'target-1', 'repo-1', stored),
      document: {
        ...syncOverrideDraftEnvelope(first, 'target-1', 'repo-1', stored).document,
        future: { untouched: true },
      },
      override_texts: ['{"timezone": '],
    };
    expect(
      stageSyncOverrideControl(
        first,
        'target-1',
        'repo-1',
        stored,
        next,
        'repositories.repo-1.sync.files.document',
      ),
    ).toBe(true);

    const restarted = new SettingsDraftRegistry({ storage, now: () => 20, writerId: 'second' });
    expect(restarted.hydrate('viewer-1').restoredResources).toBe(1);
    expect(syncOverrideDraftEnvelope(restarted, 'target-1', 'repo-1', stored)).toEqual(next);
  });

  it('serializes structured JSON, YAML, and Markdown adjustments without mutating the draft', () => {
    const envelope: SyncOverrideEditorEnvelope = {
      enabled: false,
      document: {
        excludes: ['vendor/*'],
        merges: [
          {
            path: 'renovate.json',
            strategy: 'deep-merge',
            overrides: { stale: true },
            arrays: [{ path: '$.packageRules', strategy: 'append' }],
          },
          { path: 'catalog.yaml', strategy: 'shallow-merge' },
          {
            path: 'CONTRIBUTING.md',
            strategy: 'markdown',
            sections: [{ action: 'after', heading: '## Build', content: 'Run `mise run ci`' }],
          },
        ],
      },
      override_texts: ['{"packageRules":[{"matchManagers":["npm"]}]}', '{"team":"platform"}', ''],
    };
    const before = cloneSyncOverrideEditorEnvelope(envelope);
    const serialized = syncOverrideBatchInput('repo-1', 4, envelope);

    expect(serialized.ok).toBe(true);
    if (!serialized.ok) return;
    expect(serialized.input).toMatchObject({
      repository_id: 'repo-1',
      kind: 'files',
      enabled: false,
      expected_revision: 4,
      document: {
        excludes: ['vendor/*'],
        merges: [
          {
            path: 'renovate.json',
            strategy: 'deep-merge',
            arrays: [{ path: '$.packageRules', strategy: 'append' }],
            overrides: { packageRules: [{ matchManagers: ['npm'] }] },
          },
          {
            path: 'catalog.yaml',
            strategy: 'shallow-merge',
            overrides: { team: 'platform' },
          },
          {
            path: 'CONTRIBUTING.md',
            strategy: 'markdown',
            sections: [{ action: 'after', heading: '## Build', content: 'Run `mise run ci`' }],
          },
        ],
      },
    });
    expect(envelope).toEqual(before);
  });

  it('round-trips exact-path formatting through the typed document adapter', () => {
    const envelope = buildSyncOverrideEditorEnvelope(
      override({
        document: {
          excludes: ['vendor/*'],
          formats: [{ path: 'renovate.json', formatting: { json: { arrays: 'compact' } } }],
        },
      }),
    );

    expect(syncOverrideFormattingEntries(envelope)).toEqual([
      { path: 'renovate.json', formatting: { json: { arrays: 'compact' } } },
    ]);
    const changed = withSyncOverrideFormatting(envelope, 'renovate.json', {
      common: { final_newline: 'insert' },
    });
    expect(serializeSyncOverrideDocument(changed)).toEqual({
      ok: true,
      document: {
        excludes: ['vendor/*'],
        formats: [
          {
            path: 'renovate.json',
            formatting: { common: { final_newline: 'insert' } },
          },
        ],
      },
    });
    expect(envelope.document.formats).toEqual([
      { path: 'renovate.json', formatting: { json: { arrays: 'compact' } } },
    ]);
  });

  it('preserves future path-formatting fields so the strict serializer can refuse them', () => {
    const envelope = buildSyncOverrideEditorEnvelope(
      override({
        document: {
          formats: [
            {
              path: 'config.toml',
              formatting: { common: { final_newline: 'insert' } },
              future_option: { keep: true },
            },
          ],
        },
      }),
    );

    const changed = withSyncOverrideFormatting(envelope, 'README.md', {
      markdown: { tables: 'align' },
    });
    expect((changed.document.formats as SettingsJson[])[0]).toMatchObject({
      path: 'config.toml',
      future_option: { keep: true },
    });
    expect(serializeSyncOverrideDocument(changed)).toEqual({
      ok: false,
      problem: 'File formatting override 1 is invalid',
    });
  });

  it('refuses duplicate, unsupported, and empty path-formatting rows', () => {
    const document = (formats: SettingsJson[]): SyncOverrideEditorEnvelope => ({
      enabled: null,
      document: { formats },
      override_texts: [],
    });
    expect(
      serializeSyncOverrideDocument(
        document([
          { path: 'README.md', formatting: { markdown: { tables: 'align' } } },
          { path: 'readme.md', formatting: { common: { final_newline: 'insert' } } },
        ]),
      ),
    ).toMatchObject({ ok: false, problem: 'README.md has formatting configured twice' });
    expect(
      serializeSyncOverrideDocument(
        document([{ path: 'Makefile', formatting: { common: { final_newline: 'insert' } } }]),
      ),
    ).toMatchObject({ ok: false, problem: 'Makefile has no supported formatter' });
    expect(
      serializeSyncOverrideDocument(document([{ path: 'README.md', formatting: {} }])),
    ).toMatchObject({
      ok: false,
      problem: 'README.md has an invalid or empty formatting override',
    });
  });

  it('refuses malformed text, duplicate paths, unknown keys, and invalid merge rules', () => {
    const malformed: SyncOverrideEditorEnvelope = {
      enabled: null,
      document: { merges: [{ path: 'renovate.json' }] },
      override_texts: ['{"timezone": '],
    };
    expect(serializeSyncOverrideDocument(malformed)).toEqual({
      ok: false,
      problem: 'What renovate.json sets is not a JSON object',
    });

    const duplicate: SyncOverrideEditorEnvelope = {
      enabled: null,
      document: {
        merges: [
          { path: 'README.md', sections: [{ action: 'delete', heading: '# Old' }] },
          { path: 'readme.md', sections: [{ action: 'delete', heading: '# Old' }] },
        ],
      },
      override_texts: ['', ''],
    };
    expect(serializeSyncOverrideDocument(duplicate)).toMatchObject({
      ok: false,
      problem: 'readme.md is adjusted twice',
    });

    const unknown: SyncOverrideEditorEnvelope = {
      enabled: null,
      document: { future: { keep: true } },
      override_texts: [],
    };
    expect(parseSyncOverrideEditorEnvelope(unknown)).toEqual(unknown);
    expect(serializeSyncOverrideDocument(unknown)).toEqual({
      ok: false,
      problem: 'This version cannot safely save document key future',
    });

    const badRule: SyncOverrideEditorEnvelope = {
      enabled: null,
      document: {
        merges: [
          {
            path: 'renovate.json',
            arrays: [{ path: '$.packageRules', strategy: 'append' }],
          },
        ],
      },
      override_texts: ['{"timezone":"UTC"}'],
    };
    expect(serializeSyncOverrideDocument(badRule)).toMatchObject({ ok: false });
    expect((serializeSyncOverrideDocument(badRule) as { problem: string }).problem).toContain(
      'No adjustment sets $.packageRules',
    );
  });

  it('drops inert structured text after a row becomes a Markdown adjustment', () => {
    const switched: SyncOverrideEditorEnvelope = {
      enabled: null,
      document: {
        merges: [
          {
            path: 'README.md',
            overrides: { stale: true },
            arrays: [{ path: '$.stale', strategy: 'append' }],
            sections: [{ action: 'delete', heading: '# Retired' }],
          },
        ],
      },
      override_texts: ['{"half-written": '],
    };

    expect(serializeSyncOverrideDocument(switched)).toEqual({
      ok: true,
      document: {
        merges: [
          {
            path: 'README.md',
            sections: [{ action: 'delete', heading: '# Retired' }],
          },
        ],
      },
    });
  });

  it('projects raw document text and stages enablement as separate controls', () => {
    const stored = override({
      document: { merges: [{ path: 'renovate.json', overrides: { timezone: 'UTC' } }] },
    });
    const envelope = buildSyncOverrideEditorEnvelope(stored);
    const controls = syncOverrideSavedControls('repo-1', envelope);
    expect(controls['repositories.repo-1.sync.files.enabled']).toBeNull();
    expect(controls['repositories.repo-1.sync.files.document']).toEqual({
      document: envelope.document,
      override_texts: envelope.override_texts,
    });

    const drafts = new SettingsDraftRegistry({ storage: null, now: () => 1, writerId: 'test' });
    drafts.hydrate('viewer-1');
    adoptSyncOverrideSettings(drafts, 'target-1', 'repo-1', stored);
    expect(
      stageSyncOverrideControl(
        drafts,
        'target-1',
        'repo-1',
        stored,
        { ...envelope, enabled: false },
        'repositories.repo-1.sync.files.enabled',
      ),
    ).toBe(true);
    expect(drafts.dirtyControls()).toMatchObject([
      { id: 'repositories.repo-1.sync.files.enabled', saved: null, value: false },
    ]);
  });

  it('commits the returned canonical state and refuses unreadable canonical input', () => {
    const state: WorkspaceSyncOverrideSettingsState = {
      target_id: 'target-1',
      repository_id: 'repo-1',
      kind: 'files',
      enabled: true,
      document: { excludes: ['LICENSE'] },
      revision: 9,
    };
    const committed = syncOverrideCommittedResource(state);
    expect(committed.resource).toEqual({
      type: 'sync-override',
      targetId: 'target-1',
      repositoryId: 'repo-1',
      kind: 'files',
    });
    expect(committed.revision).toBe(9);
    expect(committed.value).toEqual({
      enabled: true,
      document: { excludes: ['LICENSE'] },
      override_texts: [],
    });

    expect(() => buildSyncOverrideEditorEnvelope(override({ unreadable: true }))).toThrow(
      /unreadable/,
    );
    const drafts = new SettingsDraftRegistry({ storage: null });
    drafts.hydrate('viewer-1');
    expect(() =>
      adoptSyncOverrideSettings(drafts, 'target-1', 'repo-1', override({ unreadable: true })),
    ).toThrow(/unreadable/);
  });
});
