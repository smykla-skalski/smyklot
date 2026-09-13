import { describe, expect, it } from 'vitest';
import { SyncCheckIntentStore } from '../src/lib/sync-check-intent';

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
  return { values, storage, store: new SyncCheckIntentStore(storage, 'account', 'workspace') };
}

describe('pending check intent [Unit]', () => {
  it('recovers the exact command after reload even when the reason changes', () => {
    const { storage, store } = fixture();
    const first = store.begin(' Original reason ');
    expect(first.reason).toBe('Original reason');
    expect(first.request_key).toBeTruthy();
    const reloaded = new SyncCheckIntentStore(storage, 'account', 'workspace');
    expect(reloaded.read()).toEqual(first);
    expect(reloaded.begin('Different reason')).toEqual(first);
  });
  it('isolates accounts and workspaces', () => {
    const { storage, store } = fixture();
    store.begin('Original');
    expect(new SyncCheckIntentStore(storage, 'other', 'workspace').read()).toBeNull();
    expect(new SyncCheckIntentStore(storage, 'account', 'other').read()).toBeNull();
  });
  it('keeps a newer request when an old completion arrives', () => {
    const { store } = fixture();
    const first = store.begin('First');
    store.clear(first.request_key);
    const second = store.begin('Second');
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
    store.begin('Original');
    const address = [...values.keys()][0]!;
    values.set(address, raw);
    expect(() => store.begin('Replacement')).toThrow();
    expect(values.get(address)).toBe(raw);
  });
  it('does not return an unsaved command when storage fails', () => {
    const { storage } = fixture();
    const store = new SyncCheckIntentStore(
      {
        ...storage,
        setItem: () => {
          throw new Error('quota');
        },
      },
      'a',
      'w',
    );
    expect(() => store.begin('Reason')).toThrow('quota');
    expect(store.read()).toBeNull();
  });
  it('retains a receipt when clearing fails so it can be recovered again', () => {
    const { storage, store } = fixture();
    const first = store.begin('Reason');
    const failing = new SyncCheckIntentStore(
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
});
