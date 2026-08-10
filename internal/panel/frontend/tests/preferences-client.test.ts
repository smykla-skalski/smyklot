import { describe, expect, it, vi } from 'vitest';

import {
  EMPTY_DOC_SUM,
  createPrefsSync,
  emptyPrefsDoc,
  prefsChecksum,
  readPrefsDoc,
  writePrefsDoc,
  type PrefsDoc,
  type PrefsSync,
} from '../src/lib/preferences-sync';

class MemoryStorage {
  private items = new Map<string, string>();

  getItem(key: string): string | null {
    return this.items.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.items.set(key, value);
  }

  removeItem(key: string): void {
    this.items.delete(key);
  }
}

interface ClientFixture {
  storage: MemoryStorage;
  client: PrefsSync;
  sent: string[];
  timers: Array<{ handler: () => void; delay: number }>;
  runTimers: () => void;
  lastPatch: () => Record<string, unknown>;
}

function clientFixture(sendResult = true): ClientFixture {
  const storage = new MemoryStorage();
  const timers: Array<{ handler: () => void; delay: number }> = [];
  const sent: string[] = [];
  const client = createPrefsSync({
    storage,
    clock: {
      setTimeout: (handler, delay) => {
        const timer = { handler, delay };
        timers.push(timer);
        return timer;
      },
      clearTimeout: (handle) => {
        const index = timers.indexOf(handle as (typeof timers)[number]);
        if (index >= 0) timers.splice(index, 1);
      },
    },
  });
  client.attach((frame) => {
    sent.push(frame);
    return sendResult;
  });

  return {
    storage,
    client,
    sent,
    timers,
    runTimers: () => {
      while (timers.length > 0) timers.shift()?.handler();
    },
    lastPatch: () => {
      const frame = sent.at(-1);
      if (frame === undefined) throw new Error('nothing was sent');
      return (JSON.parse(frame) as { changes: Record<string, unknown> }).changes;
    },
  };
}

describe('createPrefsSync writes', () => {
  it('coalesces rapid changes into one debounced patch', () => {
    const fixture = clientFixture();

    fixture.client.set('theme', 'light');
    fixture.client.set('theme', 'dark');
    fixture.client.set('sidebar', 'collapsed');
    expect(fixture.sent).toHaveLength(0);
    expect(fixture.timers.length).toBeGreaterThan(0);

    fixture.runTimers();

    expect(fixture.sent).toHaveLength(1);
    expect(fixture.lastPatch()).toEqual({ theme: 'dark', sidebar: 'collapsed' });
  });

  it('syncs values equal to their default as deletions', () => {
    const fixture = clientFixture();
    const doc = emptyPrefsDoc();
    doc.shadow = { theme: 'dark' };
    doc.rev = 1;
    writePrefsDoc(doc, fixture.storage);

    fixture.client.set('theme', 'system');
    fixture.runTimers();

    expect(fixture.lastPatch()).toEqual({ theme: null });
    expect(fixture.client.get('theme')).toBe('system');
  });

  it('skips writes the shadow already satisfies and clears stale pending', () => {
    const fixture = clientFixture();
    const doc = emptyPrefsDoc();
    doc.shadow = { theme: 'dark' };
    doc.pending = { theme: 'light' };
    doc.rev = 1;
    writePrefsDoc(doc, fixture.storage);

    fixture.client.set('theme', 'dark');

    expect(readPrefsDoc(fixture.storage).pending).toEqual({});
    fixture.runTimers();
    expect(fixture.sent).toHaveLength(0);
  });

  it('sanitizes free-text values before storing them', () => {
    const fixture = clientFixture();

    fixture.client.set('table.users.search', ' spaced\u0000 out\u2028 ');
    fixture.runTimers();

    expect(fixture.lastPatch()).toEqual({ 'table.users.search': ' spaced out ' });
  });

  it('keeps pending while detached and resumes after the next handshake', () => {
    const fixture = clientFixture();
    fixture.client.detach();

    fixture.client.set('theme', 'dark');
    expect(fixture.timers).toHaveLength(0);
    expect(readPrefsDoc(fixture.storage).pending).toEqual({ theme: 'dark' });

    fixture.client.attach((frame) => {
      fixture.sent.push(frame);
      return true;
    });
    fixture.client.onPrefsReady({ rev: 0, sum: EMPTY_DOC_SUM, values: {} });

    expect(fixture.sent).toHaveLength(1);
    expect(fixture.lastPatch()).toEqual({ theme: 'dark' });
  });
});

describe('createPrefsSync handshake', () => {
  it('builds the dial query from the stored revision and checksum', () => {
    const fixture = clientFixture();
    expect(fixture.client.dialQuery()).toBe(`prefs_rev=0&prefs_sum=${EMPTY_DOC_SUM}`);

    const doc = emptyPrefsDoc();
    doc.rev = 5;
    doc.sum = 'abcdef0123456789';
    writePrefsDoc(doc, fixture.storage);

    expect(fixture.client.dialQuery()).toBe('prefs_rev=5&prefs_sum=abcdef0123456789');
  });

  it('applies a snapshot but keeps pending as the latest intent', () => {
    const fixture = clientFixture();
    const doc = emptyPrefsDoc();
    doc.pending = { theme: 'dark', sidebar: 'collapsed' };
    writePrefsDoc(doc, fixture.storage);
    const changed: string[][] = [];
    fixture.client.subscribe((keys) => changed.push([...keys].sort()));

    fixture.client.onPrefsReady({
      rev: 4,
      sum: 'feedfacefeedface',
      values: { theme: 'light', sidebar: 'collapsed', 'history.time_display': 'absolute' },
    });

    const stored = readPrefsDoc(fixture.storage);
    expect(stored.rev).toBe(4);
    expect(stored.sum).toBe('feedfacefeedface');
    expect(stored.shadow).toEqual({
      theme: 'light',
      sidebar: 'collapsed',
      'history.time_display': 'absolute',
    });
    // The sidebar pending entry matched the snapshot and was acknowledged;
    // the theme entry survived and went straight back to the server.
    expect(stored.pending).toEqual({ theme: 'dark' });
    expect(fixture.lastPatch()).toEqual({ theme: 'dark' });
    expect(fixture.client.get('theme')).toBe('dark');
    expect(changed).toEqual([['history.time_display']]);
  });

  it('confirms revision and checksum without values on a matching handshake', () => {
    const fixture = clientFixture();

    fixture.client.onPrefsReady({ rev: 0, sum: EMPTY_DOC_SUM });

    expect(readPrefsDoc(fixture.storage)).toEqual(emptyPrefsDoc());
    expect(fixture.sent).toHaveLength(0);
  });

  it('drops malformed snapshot values instead of storing them', () => {
    const fixture = clientFixture();

    fixture.client.onPrefsReady({
      rev: 2,
      sum: 'feedfacefeedface',
      values: { theme: 'dark', broken: 7 },
    });

    expect(readPrefsDoc(fixture.storage).shadow).toEqual({ theme: 'dark' });
  });
});

describe('createPrefsSync change stream', () => {
  it('applies newer changes, acknowledges pending, and recomputes the checksum', async () => {
    const fixture = clientFixture();
    const doc = emptyPrefsDoc();
    doc.pending = { theme: 'dark' };
    writePrefsDoc(doc, fixture.storage);

    fixture.client.onPrefsChanged({ rev: 1, changes: { theme: 'dark' } });

    const stored = readPrefsDoc(fixture.storage);
    expect(stored.rev).toBe(1);
    expect(stored.shadow).toEqual({ theme: 'dark' });
    expect(stored.pending).toEqual({});

    const expected = await prefsChecksum({ theme: 'dark' });
    await vi.waitFor(() => {
      expect(readPrefsDoc(fixture.storage).sum).toBe(expected);
    });
  });

  it('ignores stale revisions', () => {
    const fixture = clientFixture();
    const doc = emptyPrefsDoc();
    doc.rev = 3;
    doc.shadow = { theme: 'dark' };
    doc.sum = 'abcdef0123456789';
    writePrefsDoc(doc, fixture.storage);

    fixture.client.onPrefsChanged({ rev: 3, changes: { theme: 'light' } });

    expect(readPrefsDoc(fixture.storage).shadow).toEqual({ theme: 'dark' });
  });

  it('blanks the checksum on a revision gap to force the next snapshot', () => {
    const fixture = clientFixture();
    const doc = emptyPrefsDoc();
    doc.rev = 1;
    writePrefsDoc(doc, fixture.storage);

    fixture.client.onPrefsChanged({ rev: 5, changes: { theme: 'dark' } });

    const stored = readPrefsDoc(fixture.storage);
    expect(stored.rev).toBe(5);
    expect(stored.sum).toBe('');
    expect(fixture.client.dialQuery()).toBe('prefs_rev=5&prefs_sum=');
  });

  it('notifies subscribers about remotely changed keys', () => {
    const fixture = clientFixture();
    const changed: string[][] = [];
    fixture.client.subscribe((keys) => changed.push([...keys].sort()));

    fixture.client.onPrefsChanged({
      rev: 1,
      changes: { theme: 'dark', 'table.users.roles': ['admin'] },
    });

    expect(changed).toEqual([['table.users.roles', 'theme']]);
    expect(fixture.client.get('table.users.roles')).toEqual(['admin']);
  });

  it('drops rejected keys from pending', () => {
    const fixture = clientFixture();
    const doc = emptyPrefsDoc();
    doc.pending = { theme: 'dark', sidebar: 'collapsed' };
    writePrefsDoc(doc, fixture.storage);

    fixture.client.onPrefsRejected(['theme', 'unknown']);

    expect(readPrefsDoc(fixture.storage).pending).toEqual({ sidebar: 'collapsed' });
  });
});

describe('createPrefsSync accounts', () => {
  it('claims an unowned document and keeps it for the same account', () => {
    const fixture = clientFixture();
    const doc = emptyPrefsDoc();
    doc.pending = { theme: 'dark' };
    writePrefsDoc(doc, fixture.storage);

    fixture.client.adoptAccount('github:test:user:1');
    expect(readPrefsDoc(fixture.storage).account).toBe('github:test:user:1');
    expect(readPrefsDoc(fixture.storage).pending).toEqual({ theme: 'dark' });

    fixture.client.adoptAccount('github:test:user:1');
    expect(readPrefsDoc(fixture.storage).pending).toEqual({ theme: 'dark' });
  });

  it('resets the document when a different account signs in', () => {
    const fixture = clientFixture();
    const doc: PrefsDoc = {
      account: 'github:test:user:1',
      rev: 6,
      sum: 'abcdef0123456789',
      shadow: { theme: 'dark' },
      pending: { sidebar: 'collapsed' },
    };
    writePrefsDoc(doc, fixture.storage);
    const changed: string[][] = [];
    fixture.client.subscribe((keys) => changed.push([...keys].sort()));

    fixture.client.adoptAccount('github:test:user:2');

    expect(readPrefsDoc(fixture.storage)).toEqual(emptyPrefsDoc('github:test:user:2'));
    expect(changed).toEqual([['sidebar', 'theme']]);
  });
});
