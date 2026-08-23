import { describe, expect, it } from 'vitest';

import {
  SettingsDraftRegistry,
  settingsDraftStorageKey,
  settingsResourceKey,
  type SettingsJson,
  type SettingsResource,
  type SettingsScope,
} from '../src/lib/settings-drafts.svelte';

class MemoryStorage {
  readonly values = new Map<string, string>();

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }

  removeItem(key: string): void {
    this.values.delete(key);
  }
}

const targetOne: SettingsScope = { type: 'installation', targetId: 'target-1' };
const targetTwo: SettingsScope = { type: 'installation', targetId: 'target-2' };
const root: SettingsScope = { type: 'root' };

const defaults: SettingsResource = { type: 'target-defaults', targetId: 'target-1' };
const repository: SettingsResource = {
  type: 'repository-settings',
  targetId: 'target-1',
  repositoryId: 'repository-1',
};
const syncConfig: SettingsResource = {
  type: 'sync-config',
  targetId: 'target-1',
  kind: 'labels',
};
const syncOverride: SettingsResource = {
  type: 'sync-override',
  targetId: 'target-2',
  repositoryId: 'repository-2',
  kind: 'files',
};
const runtime: SettingsResource = { type: 'runtime' };

function registry(
  storage: MemoryStorage | null = new MemoryStorage(),
  clock: { value: number } = { value: 1_000 },
  writerId = 'writer-1',
): SettingsDraftRegistry {
  return new SettingsDraftRegistry({ storage, now: () => clock.value, writerId });
}

function stageBoolean(
  drafts: SettingsDraftRegistry,
  resource: SettingsResource,
  controlId: string,
  section: 'defaults' | 'repositories' | 'sync' | 'runtime',
  path: string[],
  saved = false,
  value = true,
): void {
  expect(drafts.adoptBase(resource, 1, { enabled: saved })).toBe(true);
  expect(
    drafts.stage(
      resource,
      { enabled: value },
      {
        id: controlId,
        location: { section, path },
        saved,
        value,
      },
    ),
  ).toBe(true);
}

describe('SettingsDraftRegistry scopes and locations [Unit]', () => {
  it('holds every resource kind and bubbles dirty controls through semantic locations', () => {
    const drafts = registry();
    drafts.hydrate('account-1');

    stageBoolean(drafts, defaults, 'defaults.repository-default-enabled', 'defaults', [
      'repositories',
    ]);
    stageBoolean(drafts, repository, 'repositories.repository-1.pending-ci', 'repositories', [
      'repository-1',
      'merge',
    ]);
    stageBoolean(drafts, syncConfig, 'sync.labels.enabled', 'sync', ['labels']);
    stageBoolean(
      drafts,
      syncOverride,
      'repositories.repository-2.sync.files.enabled',
      'repositories',
      ['repository-2', 'sync'],
    );
    stageBoolean(drafts, runtime, 'runtime.reaction-sweep', 'runtime', ['process']);

    expect(drafts.dirtyResourceCount).toBe(5);
    expect(drafts.dirtyControlCount).toBe(5);
    expect(drafts.dirtyTargetIds).toEqual(['target-1', 'target-2']);
    expect(drafts.dirtyResources(targetOne)).toHaveLength(3);
    expect(drafts.dirtyResources(targetTwo)).toHaveLength(1);
    expect(drafts.dirtyResources(root)).toHaveLength(1);
    expect(drafts.dirtyAt(targetOne, { section: 'sync' })).toBe(true);
    expect(drafts.dirtyAt(targetOne, { section: 'sync', path: ['labels'] })).toBe(true);
    expect(drafts.dirtyAt(targetOne, { section: 'sync', path: ['rulesets'] })).toBe(false);
    expect(drafts.dirtyAt(targetOne, { section: 'defaults' })).toBe(true);
    expect(drafts.dirtyAt(root, { section: 'runtime' })).toBe(true);
    expect(drafts.isControlDirty(targetOne, 'sync.labels.enabled')).toBe(true);
  });

  it('returns detached values so callers cannot mutate registry state without staging', () => {
    const drafts = registry();
    drafts.hydrate('account-1');
    drafts.adoptBase(defaults, 1, { nested: { enabled: false } });

    const value = drafts.value(defaults) as { nested: { enabled: boolean } };
    value.nested.enabled = true;

    expect(drafts.value(defaults)).toEqual({ nested: { enabled: false } });
    expect(drafts.dirty).toBe(false);
  });
});

describe('SettingsDraftRegistry durability [Unit]', () => {
  it('persists a versioned full base and draft for only the owning account', () => {
    const storage = new MemoryStorage();
    const clock = { value: 1_000 };
    const first = registry(storage, clock, 'first-tab');
    first.hydrate('account-1');
    stageBoolean(first, defaults, 'defaults.enabled', 'defaults', ['repositories']);
    clock.value = 1_200;
    expect(first.markAttention(targetOne)).toBe(true);

    const stored = JSON.parse(storage.getItem(settingsDraftStorageKey('account-1'))!) as {
      schema: number;
      accountId: string;
      records: Array<{
        deleted: boolean;
        expectedRevision: number;
        base: SettingsJson;
        draft: SettingsJson;
        firstChangedAt: number;
        lastChangedAt: number;
        attentionAt: number;
      }>;
    };
    expect(stored).toMatchObject({ schema: 1, accountId: 'account-1' });
    expect(stored.records[0]).toMatchObject({
      deleted: false,
      expectedRevision: 1,
      base: { enabled: false },
      draft: { enabled: true },
      firstChangedAt: 1_000,
      lastChangedAt: 1_000,
      attentionAt: 1_200,
    });

    const restored = registry(storage, clock, 'second-tab');
    expect(restored.hydrate('account-2')).toEqual({
      restoredResources: 0,
      firstChangedAt: null,
    });
    expect(restored.dirty).toBe(false);
    expect(restored.hydrate('account-1')).toEqual({
      restoredResources: 1,
      firstChangedAt: 1_000,
    });
    expect(restored.value(defaults)).toEqual({ enabled: true });
    expect(restored.timestamps(targetOne)).toEqual({
      firstChangedAt: 1_000,
      lastChangedAt: 1_000,
      attentionAt: 1_200,
    });
  });

  it('ignores malformed documents and continues when storage refuses writes', () => {
    const malformed = new MemoryStorage();
    malformed.setItem(settingsDraftStorageKey('account-1'), '{not-json');
    const drafts = registry(malformed);
    expect(drafts.hydrate('account-1').restoredResources).toBe(0);
    expect(drafts.storageProblem).toContain('corrupted');

    const refusing = {
      getItem: () => null,
      setItem: () => {
        throw new DOMException('full', 'QuotaExceededError');
      },
      removeItem: () => {},
    };
    const volatile = new SettingsDraftRegistry({ storage: refusing, now: () => 1, writerId: 'x' });
    volatile.hydrate('account-1');
    stageBoolean(volatile, defaults, 'defaults.enabled', 'defaults', []);

    expect(volatile.dirty).toBe(true);
    expect(volatile.storageProblem).toContain('will not survive');
    expect(() => registry(null).hydrate('account-1')).not.toThrow();
  });

  it('leaves a clean tombstone once the account has no dirty resources', () => {
    const storage = new MemoryStorage();
    const drafts = registry(storage);
    drafts.hydrate('account-1');
    stageBoolean(drafts, defaults, 'defaults.enabled', 'defaults', []);
    expect(storage.getItem(settingsDraftStorageKey('account-1'))).not.toBeNull();

    expect(drafts.discardScope(targetOne)).toBe(1);

    expect(drafts.dirty).toBe(false);
    expect(JSON.parse(storage.getItem(settingsDraftStorageKey('account-1'))!)).toMatchObject({
      records: [{ deleted: true, base: { enabled: false }, expectedRevision: 1 }],
    });
    expect(drafts.value(defaults)).toEqual({ enabled: false });
  });

  it('rejects the whole corrupted document without overwriting its recoverable bytes', () => {
    const storage = new MemoryStorage();
    const first = registry(storage);
    first.hydrate('account-1');
    stageBoolean(first, defaults, 'defaults.enabled', 'defaults', []);
    const document = JSON.parse(storage.getItem(settingsDraftStorageKey('account-1'))!) as {
      records: unknown[];
    };
    document.records.push(document.records[0]);
    const corrupted = JSON.stringify(document);
    storage.setItem(settingsDraftStorageKey('account-1'), corrupted);

    const restored = registry(storage, { value: 2_000 }, 'restored');
    expect(restored.hydrate('account-1')).toEqual({
      restoredResources: 0,
      firstChangedAt: null,
    });
    expect(restored.storageProblem).toContain('corrupted');

    restored.adoptBase(defaults, 1, { enabled: false });
    expect(
      restored.stage(
        defaults,
        { enabled: true },
        {
          id: 'defaults.enabled',
          location: { section: 'defaults' },
          saved: false,
          value: true,
        },
      ),
    ).toBe(true);
    expect(storage.getItem(settingsDraftStorageKey('account-1'))).toBe(corrupted);
    expect(restored.storageProblem).toContain('corrupted');
  });

  it('keeps timestamps valid when the wall clock moves backwards', () => {
    const storage = new MemoryStorage();
    const clock = { value: 1_000 };
    const drafts = registry(storage, clock);
    drafts.hydrate('account-1');
    drafts.adoptBase(defaults, 1, { first: false, second: false });
    drafts.stage(
      defaults,
      { first: true, second: false },
      {
        id: 'defaults.first',
        location: { section: 'defaults' },
        saved: false,
        value: true,
      },
    );

    clock.value = 900;
    expect(
      drafts.stage(
        defaults,
        { first: true, second: true },
        {
          id: 'defaults.second',
          location: { section: 'defaults' },
          saved: false,
          value: true,
        },
      ),
    ).toBe(true);

    const restored = registry(storage, clock, 'restored');
    expect(restored.hydrate('account-1').restoredResources).toBe(1);
    expect(restored.value(defaults)).toEqual({ first: true, second: true });
    expect(restored.timestamps(targetOne)).toEqual({
      firstChangedAt: 1_000,
      lastChangedAt: 1_001,
      attentionAt: null,
    });
  });
});

describe('SettingsDraftRegistry save transitions [Unit]', () => {
  it('snapshots one scope and preserves later edits when that snapshot commits', () => {
    const clock = { value: 100 };
    const drafts = registry(new MemoryStorage(), clock);
    drafts.hydrate('account-1');
    drafts.adoptBase(defaults, 4, { mode: 'off' });
    drafts.stage(
      defaults,
      { mode: 'submitted' },
      {
        id: 'defaults.mode',
        location: { section: 'defaults' },
        saved: 'off',
        value: 'submitted',
      },
    );

    const attempt = drafts.beginSave(targetOne)!;
    expect(attempt.entries).toMatchObject([
      {
        resourceKey: settingsResourceKey(defaults),
        expectedRevision: 4,
        value: { mode: 'submitted' },
      },
    ]);
    expect(drafts.operation(targetOne).saving).toBe(true);

    clock.value = 120;
    drafts.stage(
      defaults,
      { mode: 'edited-later' },
      {
        id: 'defaults.mode',
        location: { section: 'defaults' },
        saved: 'off',
        value: 'edited-later',
      },
    );
    expect(
      drafts.commitSave(
        attempt,
        [
          {
            resource: defaults,
            revision: 5,
            value: { mode: 'submitted' },
            savedControls: { 'defaults.mode': 'submitted' },
          },
        ],
        'Saved',
      ),
    ).toBe(true);

    expect(drafts.resource(defaults)).toMatchObject({
      expectedRevision: 5,
      base: { mode: 'submitted' },
      value: { mode: 'edited-later' },
      dirty: true,
      controls: [{ id: 'defaults.mode', saved: 'submitted', value: 'edited-later' }],
    });
    expect(drafts.operation(targetOne)).toEqual({
      saving: false,
      problem: null,
      notice: 'Saved',
    });

    const retry = drafts.beginSave(targetOne)!;
    expect(retry.entries[0]?.expectedRevision).toBe(5);
    expect(
      drafts.commitSave(retry, [
        {
          resource: defaults,
          revision: 6,
          value: { mode: 'edited-later' },
          savedControls: { 'defaults.mode': 'edited-later' },
        },
      ]),
    ).toBe(true);
    expect(drafts.dirty).toBe(false);
  });

  it('preserves a revert made while the submitted value is saving', () => {
    const clock = { value: 100 };
    const drafts = registry(new MemoryStorage(), clock);
    drafts.hydrate('account-1');
    drafts.adoptBase(defaults, 1, { enabled: false });
    drafts.stage(
      defaults,
      { enabled: true },
      {
        id: 'defaults.enabled',
        location: { section: 'defaults' },
        saved: false,
        value: true,
      },
    );
    const attempt = drafts.beginSave(targetOne)!;

    clock.value = 110;
    drafts.stage(
      defaults,
      { enabled: false },
      {
        id: 'defaults.enabled',
        location: { section: 'defaults' },
        saved: false,
        value: false,
      },
    );
    expect(drafts.hasDirty(targetOne)).toBe(false);

    drafts.commitSave(attempt, [
      {
        resource: defaults,
        revision: 2,
        value: { enabled: true },
        savedControls: { 'defaults.enabled': true },
      },
    ]);

    expect(drafts.resource(defaults)).toMatchObject({
      base: { enabled: true },
      value: { enabled: false },
      dirty: true,
      controls: [{ id: 'defaults.enabled', saved: true, value: false }],
    });
  });

  it('keeps a resource first staged after save began out of that attempt', () => {
    const drafts = registry();
    drafts.hydrate('account-1');
    stageBoolean(drafts, defaults, 'defaults.enabled', 'defaults', []);
    const attempt = drafts.beginSave(targetOne)!;

    stageBoolean(drafts, repository, 'repositories.enabled', 'repositories', ['repository-1']);
    drafts.commitSave(attempt, [
      {
        resource: defaults,
        revision: 2,
        value: { enabled: true },
        savedControls: { 'defaults.enabled': true },
      },
    ]);

    expect(drafts.dirtyResources(targetOne)).toMatchObject([{ resource: repository, dirty: true }]);
  });

  it('marks save conflicts, blocks another attempt, and rebases without dropping the draft', () => {
    const drafts = registry();
    drafts.hydrate('account-1');
    drafts.adoptBase(defaults, 1, { mode: 'public' });
    drafts.stage(
      defaults,
      { mode: 'private' },
      {
        id: 'defaults.mode',
        location: { section: 'defaults' },
        saved: 'public',
        value: 'private',
      },
    );
    const attempt = drafts.beginSave(targetOne)!;

    expect(
      drafts.failSave(attempt, 'changed elsewhere', [
        { resource: defaults, actualRevision: 2, latestBase: { mode: 'internal' } },
      ]),
    ).toBe(true);
    expect(drafts.hasConflicts(targetOne)).toBe(true);
    expect(drafts.beginSave(targetOne)).toBeNull();
    expect(drafts.rebase(defaults, 2, { mode: 'internal' }, {})).toBe(false);
    expect(drafts.rebase(defaults, 2, { mode: 'internal' }, { 'defaults.mode': 'internal' })).toBe(
      true,
    );

    expect(drafts.resource(defaults)).toMatchObject({
      expectedRevision: 2,
      base: { mode: 'internal' },
      value: { mode: 'private' },
      conflict: null,
    });
    expect(drafts.operation(targetOne).problem).toBe('changed elsewhere');
    drafts.dismissProblem(targetOne);
    expect(drafts.operation(targetOne).problem).toBeNull();
    expect(drafts.beginSave(targetOne)?.entries[0]?.expectedRevision).toBe(2);
  });

  it('detects a fresh server base under a dirty resource', () => {
    const drafts = registry();
    drafts.hydrate('account-1');
    stageBoolean(drafts, defaults, 'defaults.enabled', 'defaults', []);

    expect(drafts.adoptBase(defaults, 2, { enabled: 'elsewhere' })).toBe(false);
    expect(drafts.resource(defaults)).toMatchObject({
      expectedRevision: 1,
      value: { enabled: true },
      conflict: { type: 'revision', actualRevision: 2, latestBase: { enabled: 'elsewhere' } },
    });
  });

  it('discards to the known current server revision and refuses an unknown conflict base', () => {
    const drafts = registry();
    drafts.hydrate('account-1');
    stageBoolean(drafts, defaults, 'defaults.enabled', 'defaults', []);
    const attempt = drafts.beginSave(targetOne)!;
    drafts.failSave(attempt, 'changed elsewhere', [
      { resource: defaults, actualRevision: 2, latestBase: { enabled: 'server' } },
    ]);

    expect(drafts.discardResource(defaults)).toBe(true);
    expect(drafts.resource(defaults)).toMatchObject({
      expectedRevision: 2,
      base: { enabled: 'server' },
      value: { enabled: 'server' },
      dirty: false,
    });

    drafts.stage(
      defaults,
      { enabled: true },
      {
        id: 'defaults.enabled',
        location: { section: 'defaults' },
        saved: 'server',
        value: true,
      },
    );
    const retry = drafts.beginSave(targetOne)!;
    drafts.failSave(retry, 'changed without a body', [{ resource: defaults, actualRevision: 3 }]);
    expect(drafts.discardResource(defaults)).toBe(false);
    expect(drafts.hasDirty(targetOne)).toBe(true);
  });

  it('uses canonical committed control values when rebasing edits made during save', () => {
    const drafts = registry();
    drafts.hydrate('account-1');
    drafts.adoptBase(defaults, 1, { mode: 'off' });
    drafts.stage(
      defaults,
      { mode: ' submitted ' },
      {
        id: 'defaults.mode',
        location: { section: 'defaults' },
        saved: 'off',
        value: ' submitted ',
      },
    );
    const attempt = drafts.beginSave(targetOne)!;
    drafts.stage(
      defaults,
      { mode: 'later' },
      {
        id: 'defaults.mode',
        location: { section: 'defaults' },
        saved: 'off',
        value: 'later',
      },
    );

    expect(
      drafts.commitSave(attempt, [
        {
          resource: defaults,
          revision: 2,
          value: { mode: 'submitted' },
          savedControls: { 'defaults.mode': 'submitted' },
        },
      ]),
    ).toBe(true);
    expect(drafts.resource(defaults)).toMatchObject({
      base: { mode: 'submitted' },
      value: { mode: 'later' },
      controls: [{ saved: 'submitted', value: 'later' }],
    });

    expect(
      drafts.stage(
        defaults,
        { mode: 'submitted' },
        {
          id: 'defaults.mode',
          location: { section: 'defaults' },
          saved: 'off',
          value: 'submitted',
        },
      ),
    ).toBe(true);
    expect(drafts.hasDirty(targetOne)).toBe(false);
  });

  it('rejects mismatched aggregate and control dirtiness', () => {
    const drafts = registry();
    drafts.hydrate('account-1');
    drafts.adoptBase(defaults, 1, { enabled: false });

    expect(
      drafts.stage(
        defaults,
        { enabled: false },
        {
          id: 'defaults.enabled',
          location: { section: 'defaults' },
          saved: false,
          value: true,
        },
      ),
    ).toBe(false);
    expect(
      drafts.stage(
        defaults,
        { enabled: true },
        {
          id: 'defaults.enabled',
          location: { section: 'defaults' },
          saved: false,
          value: false,
        },
      ),
    ).toBe(false);
    expect(drafts.dirty).toBe(false);
  });
});

describe('SettingsDraftRegistry cross-tab reconciliation [Unit]', () => {
  it('treats unequal local counters as concurrent and preserves both drafts', () => {
    const leftStorage = new MemoryStorage();
    const rightStorage = new MemoryStorage();
    const left = registry(leftStorage, { value: 100 }, 'left');
    const right = registry(rightStorage, { value: 110 }, 'right');
    left.hydrate('account-1');
    right.hydrate('account-1');
    left.adoptBase(defaults, 1, { mode: 'saved' });
    right.adoptBase(defaults, 1, { mode: 'saved' });

    left.stage(
      defaults,
      { mode: 'left-first' },
      {
        id: 'defaults.mode',
        location: { section: 'defaults' },
        saved: 'saved',
        value: 'left-first',
      },
    );
    left.stage(
      defaults,
      { mode: 'left-second' },
      {
        id: 'defaults.mode',
        location: { section: 'defaults' },
        saved: 'saved',
        value: 'left-second',
      },
    );
    expect(left.markAttention(targetOne)).toBe(true);
    right.stage(
      defaults,
      { mode: 'right' },
      {
        id: 'defaults.mode',
        location: { section: 'defaults' },
        saved: 'saved',
        value: 'right',
      },
    );

    left.reconcile(rightStorage.getItem(settingsDraftStorageKey('account-1')));
    expect(left.resource(defaults)).toMatchObject({
      value: { mode: 'right' },
      conflict: {
        type: 'external-draft',
        writerId: 'left',
        incomingDraft: { mode: 'left-second' },
      },
    });

    right.reconcile(leftStorage.getItem(settingsDraftStorageKey('account-1')));
    expect(right.resource(defaults)).toMatchObject({
      value: { mode: 'right' },
      conflict: {
        type: 'external-draft',
        writerId: 'left',
        incomingDraft: { mode: 'left-second' },
      },
    });
  });

  it('lets a causal follow-up supersede every draft it observed', () => {
    const leftStorage = new MemoryStorage();
    const rightStorage = new MemoryStorage();
    const left = registry(leftStorage, { value: 100 }, 'left');
    const right = registry(rightStorage, { value: 110 }, 'right');
    left.hydrate('account-1');
    right.hydrate('account-1');
    left.adoptBase(defaults, 1, { mode: 'saved' });
    right.adoptBase(defaults, 1, { mode: 'saved' });
    left.stage(
      defaults,
      { mode: 'left' },
      {
        id: 'defaults.mode',
        location: { section: 'defaults' },
        saved: 'saved',
        value: 'left',
      },
    );
    right.stage(
      defaults,
      { mode: 'right' },
      {
        id: 'defaults.mode',
        location: { section: 'defaults' },
        saved: 'saved',
        value: 'right',
      },
    );
    right.reconcile(leftStorage.getItem(settingsDraftStorageKey('account-1')));
    expect(right.hasConflicts(targetOne)).toBe(true);

    expect(right.rebase(defaults, 1, { mode: 'saved' }, { 'defaults.mode': 'saved' })).toBe(true);
    expect(
      right.stage(
        defaults,
        { mode: 'resolved' },
        {
          id: 'defaults.mode',
          location: { section: 'defaults' },
          saved: 'saved',
          value: 'resolved',
        },
      ),
    ).toBe(true);

    left.reconcile(rightStorage.getItem(settingsDraftStorageKey('account-1')));
    expect(left.resource(defaults)).toMatchObject({
      value: { mode: 'resolved' },
      conflict: null,
    });
  });

  it('preserves an edit concurrent with a discard tombstone', () => {
    const seedStorage = new MemoryStorage();
    const seed = registry(seedStorage, { value: 90 }, 'seed');
    seed.hydrate('account-1');
    seed.adoptBase(defaults, 1, { mode: 'saved' });
    seed.stage(
      defaults,
      { mode: 'seed-draft' },
      {
        id: 'defaults.mode',
        location: { section: 'defaults' },
        saved: 'saved',
        value: 'seed-draft',
      },
    );
    const seeded = seedStorage.getItem(settingsDraftStorageKey('account-1'))!;
    const leftStorage = new MemoryStorage();
    const rightStorage = new MemoryStorage();
    leftStorage.setItem(settingsDraftStorageKey('account-1'), seeded);
    rightStorage.setItem(settingsDraftStorageKey('account-1'), seeded);
    const left = registry(leftStorage, { value: 100 }, 'left');
    const right = registry(rightStorage, { value: 110 }, 'right');
    left.hydrate('account-1');
    right.hydrate('account-1');

    expect(left.discardResource(defaults)).toBe(true);
    expect(
      right.stage(
        defaults,
        { mode: 'right-edit' },
        {
          id: 'defaults.mode',
          location: { section: 'defaults' },
          saved: 'saved',
          value: 'right-edit',
        },
      ),
    ).toBe(true);

    left.reconcile(rightStorage.getItem(settingsDraftStorageKey('account-1')));
    expect(left.resource(defaults)).toMatchObject({
      dirty: true,
      value: { mode: 'right-edit' },
    });
    right.reconcile(leftStorage.getItem(settingsDraftStorageKey('account-1')));
    expect(right.resource(defaults)).toMatchObject({
      dirty: true,
      value: { mode: 'right-edit' },
    });

    const restored = registry(leftStorage, { value: 120 }, 'restored');
    expect(restored.hydrate('account-1').restoredResources).toBe(1);
    expect(restored.value(defaults)).toEqual({ mode: 'right-edit' });
  });

  it('converges shared-storage writes and tombstones without losing or resurrecting resources', () => {
    const storage = new MemoryStorage();
    const left = registry(storage, { value: 100 }, 'left');
    const right = registry(storage, { value: 110 }, 'right');
    left.hydrate('account-1');
    right.hydrate('account-1');

    stageBoolean(left, defaults, 'defaults.enabled', 'defaults', []);
    stageBoolean(right, syncConfig, 'sync.labels.enabled', 'sync', ['labels']);

    const firstReload = registry(storage, { value: 120 }, 'first-reload');
    expect(firstReload.hydrate('account-1').restoredResources).toBe(2);
    expect(firstReload.value(defaults)).toEqual({ enabled: true });
    expect(firstReload.value(syncConfig)).toEqual({ enabled: true });

    expect(right.discardResource(defaults)).toBe(true);
    left.reconcile(storage.getItem(settingsDraftStorageKey('account-1')));
    expect(left.resource(defaults)).toMatchObject({ dirty: false, value: { enabled: false } });
    expect(left.dirtyResources()).toMatchObject([{ resource: syncConfig, dirty: true }]);

    const secondReload = registry(storage, { value: 130 }, 'second-reload');
    expect(secondReload.hydrate('account-1').restoredResources).toBe(1);
    expect(secondReload.resource(defaults)).toMatchObject({
      dirty: false,
      value: { enabled: false },
    });
    expect(secondReload.value(syncConfig)).toEqual({ enabled: true });
  });

  it('merges disjoint resources from another localStorage writer', () => {
    const leftStorage = new MemoryStorage();
    const rightStorage = new MemoryStorage();
    const left = registry(leftStorage, { value: 100 }, 'left');
    const right = registry(rightStorage, { value: 110 }, 'right');
    left.hydrate('account-1');
    right.hydrate('account-1');
    stageBoolean(left, defaults, 'defaults.enabled', 'defaults', []);
    stageBoolean(right, syncOverride, 'override.enabled', 'repositories', ['repository-2', 'sync']);

    expect(left.reconcile(rightStorage.getItem(settingsDraftStorageKey('account-1')))).toBe(1);

    expect(left.dirtyResourceCount).toBe(2);
    expect(left.dirtyTargetIds).toEqual(['target-1', 'target-2']);
  });

  it('converges on one value and preserves the other overlapping draft as a conflict', () => {
    const leftStorage = new MemoryStorage();
    const rightStorage = new MemoryStorage();
    const left = registry(leftStorage, { value: 100 }, 'left');
    const right = registry(rightStorage, { value: 110 }, 'right');
    left.hydrate('account-1');
    right.hydrate('account-1');
    left.adoptBase(defaults, 1, { mode: 'saved' });
    right.adoptBase(defaults, 1, { mode: 'saved' });
    left.stage(
      defaults,
      { mode: 'left' },
      {
        id: 'defaults.mode',
        location: { section: 'defaults' },
        saved: 'saved',
        value: 'left',
      },
    );
    right.stage(
      defaults,
      { mode: 'right' },
      {
        id: 'defaults.mode',
        location: { section: 'defaults' },
        saved: 'saved',
        value: 'right',
      },
    );

    left.reconcile(rightStorage.getItem(settingsDraftStorageKey('account-1')));

    expect(left.resource(defaults)).toMatchObject({
      value: { mode: 'right' },
      conflict: { type: 'external-draft', writerId: 'left', incomingDraft: { mode: 'left' } },
    });
    expect(left.beginSave(targetOne)).toBeNull();

    expect(left.resolveExternalConflicts(targetOne)).toBe(1);
    expect(left.resource(defaults)).toMatchObject({
      value: { mode: 'right' },
      conflict: null,
    });
    expect(left.beginSave(targetOne)).not.toBeNull();

    right.reconcile(leftStorage.getItem(settingsDraftStorageKey('account-1')));
    expect(right.resource(defaults)).toMatchObject({
      value: { mode: 'right' },
      conflict: null,
    });
  });
});
