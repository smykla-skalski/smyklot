import { describe, expect, it } from 'vitest';
import { SyncRequestIntentStore } from '../src/lib/sync-request-intent';

function fixture() {
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
  return { values, storage, store: new SyncRequestIntentStore(storage, 'account', 'workspace') };
}

describe('pending check intent [Unit]', () => {
  it('recovers the exact command after reload even when the reason changes', () => {
    const { storage, store } = fixture();
    const first = store.begin({ action: 'check', reason: ' Original reason ' });
    expect(first.reason).toBe('Original reason');
    expect(first.request_key).toBeTruthy();
    const reloaded = new SyncRequestIntentStore(storage, 'account', 'workspace');
    expect(reloaded.read()).toEqual(first);
    expect(reloaded.begin({ action: 'check', reason: 'Different reason' })).toEqual(first);
  });
  it('isolates accounts and workspaces', () => {
    const { storage, store } = fixture();
    store.begin({ action: 'check', reason: 'Original' });
    expect(new SyncRequestIntentStore(storage, 'other', 'workspace').read()).toBeNull();
    expect(new SyncRequestIntentStore(storage, 'account', 'other').read()).toBeNull();
  });
  it('keeps a newer request when an old completion arrives', () => {
    const { store } = fixture();
    const first = store.begin({ action: 'check', reason: 'First' });
    store.clear(first.request_key);
    const second = store.begin({ action: 'check', reason: 'Second' });
    expect(second.request_key).not.toBe(first.request_key);
    store.clear(first.request_key);
    expect(store.read()).toEqual(second);
  });
  it.each([
    '{',
    '{}',
    JSON.stringify({ action: 'check', request_key: 'é'.repeat(101), reason: 'Reason' }),
  ])('preserves unreadable storage rather than overwriting it: %s', (raw) => {
    const { values, store } = fixture();
    store.begin({ action: 'check', reason: 'Original' });
    const address = [...values.keys()][0]!;
    values.set(address, raw);
    expect(() => store.begin({ action: 'check', reason: 'Replacement' })).toThrow();
    expect(values.get(address)).toBe(raw);
  });
  it('does not return an unsaved command when storage fails', () => {
    const { storage } = fixture();
    const store = new SyncRequestIntentStore(
      {
        ...storage,
        setItem: () => {
          throw new Error('quota');
        },
      },
      'a',
      'w',
    );
    expect(() => store.begin({ action: 'check', reason: 'Reason' })).toThrow('quota');
    expect(store.read()).toBeNull();
  });
  it('retains a receipt when clearing fails so it can be recovered again', () => {
    const { storage, store } = fixture();
    const first = store.begin({ action: 'check', reason: 'Reason' });
    const failing = new SyncRequestIntentStore(
      {
        ...storage,
        removeItem: () => {
          throw new Error('blocked');
        },
      },
      'account',
      'workspace',
    );
    expect(() => failing.clear(first.request_key)).toThrow('blocked');
    expect(store.read()).toEqual(first);
  });
  it('retains an exact dispatch across reload and rejects replacing it with a check', () => {
    const { store, storage } = fixture();
    const input = {
      action: 'dispatch' as const,
      plan_id: 'selected',
      expected_revision: 7,
      reason: 'Run reviewed changes',
    };
    const first = store.begin(input);
    const reloaded = new SyncRequestIntentStore(storage, 'account', 'workspace');
    expect(reloaded.read()).toEqual(first);
    expect(reloaded.begin({ action: 'check', reason: 'different' })).toEqual(first);
    expect(reloaded.begin({ ...input, plan_id: 'newer', expected_revision: 9 })).toEqual(first);
  });
  it.each([
    { plan_id: '' },
    { expected_revision: 0 },
    { expected_revision: 1.5 },
    { expected_revision: '1' },
  ])('rejects malformed dispatch %j before writing', (change) => {
    const { values, store } = fixture();
    expect(() =>
      store.begin({
        action: 'dispatch',
        plan_id: 'selected',
        expected_revision: 1,
        reason: 'Reason',
        ...change,
      } as never),
    ).toThrow();
    expect(values.size).toBe(0);
  });
});
