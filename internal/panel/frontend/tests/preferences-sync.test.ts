import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  EMPTY_DOC_SUM,
  PREF_DEFAULTS,
  browserStorage,
  canonicalStringify,
  effectivePref,
  effectivePrefs,
  emptyPrefsDoc,
  migrateLegacyPreferences,
  prefsChecksum,
  readPrefsDoc,
  samePrefValue,
  sanitizePrefString,
  writePrefsDoc,
  type PrefValues,
  type PrefsDoc,
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

// Shared golden vectors — keep in sync with internal/panel/preferences_test.go.
const checksumVectors: { name: string; values: PrefValues; canonical: string; checksum: string }[] =
  [
    {
      name: 'empty document',
      values: {},
      canonical: '{}',
      checksum: '44136fa355b3678a',
    },
    {
      name: 'single string',
      values: { theme: 'dark' },
      canonical: '{"theme":"dark"}',
      checksum: '0f4f87db4567232a',
    },
    {
      name: 'sorted keys with every value shape',
      values: {
        theme: 'system',
        'table.users.roles': ['viewer', 'admin'],
        last_installation: 'smykla-skalski',
      },
      canonical:
        '{"last_installation":"smykla-skalski","table.users.roles":["viewer","admin"],"theme":"system"}',
      checksum: '7f918baa90c14181',
    },
    {
      name: 'JSON.stringify escaping',
      values: {
        'table.history.search': 'he said "hi" \\ <&> \t tab \u0001 low',
        'table.users.search': 'π🙂 emoji',
      },
      canonical:
        '{"table.history.search":"he said \\"hi\\" \\\\ <&> \\t tab \\u0001 low","table.users.search":"π🙂 emoji"}',
      checksum: '44161ee8b69d82a4',
    },
  ];

describe('preference checksum', () => {
  it.each(checksumVectors)('digests the $name golden vector', async (vector) => {
    expect(canonicalStringify(vector.values)).toBe(vector.canonical);
    await expect(prefsChecksum(vector.values)).resolves.toBe(vector.checksum);
  });

  it('pins the empty-document checksum constant', async () => {
    await expect(prefsChecksum({})).resolves.toBe(EMPTY_DOC_SUM);
  });
});

describe('preference document storage', () => {
  it('falls back to an empty document when nothing is stored', () => {
    expect(readPrefsDoc({ getItem: () => null })).toEqual(emptyPrefsDoc());
  });

  it('round-trips a document', () => {
    const storage = new MemoryStorage();
    const doc: PrefsDoc = {
      account: 'github:test:user:1',
      rev: 4,
      sum: 'abcdef0123456789',
      shadow: { theme: 'dark', 'table.users.roles': ['admin'] },
      pending: { sidebar: 'collapsed', theme: null },
    };

    writePrefsDoc(doc, storage);

    expect(readPrefsDoc(storage)).toEqual(doc);
  });

  it('discards malformed or invalid stored documents', () => {
    expect(readPrefsDoc({ getItem: () => 'not json' })).toEqual(emptyPrefsDoc());
    expect(readPrefsDoc({ getItem: () => '{"rev":-1}' })).toEqual(emptyPrefsDoc());
    expect(
      readPrefsDoc({
        getItem: () =>
          JSON.stringify({ account: null, rev: 1, sum: 'x', shadow: { theme: 5 }, pending: {} }),
      }),
    ).toEqual(emptyPrefsDoc());
  });

  it('continues when browser storage is unavailable', () => {
    expect(
      readPrefsDoc({
        getItem: () => {
          throw new DOMException('Storage is unavailable', 'SecurityError');
        },
      }),
    ).toEqual(emptyPrefsDoc());

    expect(() =>
      writePrefsDoc(emptyPrefsDoc(), {
        setItem: () => {
          throw new DOMException('Storage is full', 'QuotaExceededError');
        },
      }),
    ).not.toThrow();
  });
});

describe('effective preferences', () => {
  const doc: PrefsDoc = {
    account: null,
    rev: 2,
    sum: 'abcdef0123456789',
    shadow: { theme: 'dark', sidebar: 'collapsed' },
    pending: { theme: null, 'history.time_display': 'absolute' },
  };

  it('overlays pending on the shadow and applies deletions', () => {
    expect(effectivePrefs(doc)).toEqual({
      sidebar: 'collapsed',
      'history.time_display': 'absolute',
    });
  });

  it('resolves single keys through pending, shadow, then defaults', () => {
    expect(effectivePref(doc, 'history.time_display')).toBe('absolute');
    expect(effectivePref(doc, 'sidebar')).toBe('collapsed');
    expect(effectivePref(doc, 'theme')).toBe(PREF_DEFAULTS.theme);
    expect(effectivePref(doc, 'table.users.sort')).toBe('name_asc');
    expect(effectivePref(doc, 'last_installation')).toBeNull();
  });

  it('compares values structurally', () => {
    expect(samePrefValue(['a', 'b'], ['a', 'b'])).toBe(true);
    expect(samePrefValue(['a', 'b'], ['b', 'a'])).toBe(false);
    expect(samePrefValue('dark', 'dark')).toBe(true);
    expect(samePrefValue(null, undefined)).toBe(true);
    expect(samePrefValue('dark', null)).toBe(false);
  });
});

describe('preference string sanitizer', () => {
  it('strips control characters and separators', () => {
    expect(sanitizePrefString('a\u0000b\u2028c\u2029d\u007fe\tf')).toBe('abcdef');
  });

  it('caps the length', () => {
    expect(sanitizePrefString('x'.repeat(300))).toHaveLength(256);
  });

  it('caps by code point so the cut never splits a surrogate pair', () => {
    expect(sanitizePrefString('🙂'.repeat(300))).toBe('🙂'.repeat(256));
    expect(sanitizePrefString('a'.repeat(255) + '🙂' + 'tail')).toBe('a'.repeat(255) + '🙂');
  });

  it('keeps ordinary unicode', () => {
    expect(sanitizePrefString('π🙂 emoji')).toBe('π🙂 emoji');
  });
});

describe('legacy preference migration', () => {
  it('moves non-default legacy values into pending and removes the old keys', () => {
    const storage = new MemoryStorage();
    storage.setItem('smyklot.panel.theme', 'dark');
    storage.setItem('smyklot.panel.sidebar.display', 'collapsed');
    storage.setItem('smyklot.panel.history.time-display', 'relative');
    storage.setItem('smyklot.panel.last-installation', 'smykla-skalski');

    migrateLegacyPreferences(storage);

    const doc = readPrefsDoc(storage);
    expect(doc.pending).toEqual({
      theme: 'dark',
      sidebar: 'collapsed',
      last_installation: 'smykla-skalski',
    });
    expect(doc.rev).toBe(0);
    expect(storage.getItem('smyklot.panel.theme')).toBeNull();
    expect(storage.getItem('smyklot.panel.sidebar.display')).toBeNull();
    expect(storage.getItem('smyklot.panel.history.time-display')).toBeNull();
    expect(storage.getItem('smyklot.panel.last-installation')).toBeNull();
  });

  it('does nothing once a synced document exists', () => {
    const storage = new MemoryStorage();
    const existing: PrefsDoc = { ...emptyPrefsDoc('github:test:user:1'), rev: 3 };
    writePrefsDoc(existing, storage);
    storage.setItem('smyklot.panel.theme', 'dark');

    migrateLegacyPreferences(storage);

    expect(readPrefsDoc(storage)).toEqual(existing);
    expect(storage.getItem('smyklot.panel.theme')).toBe('dark');
  });

  it('writes an empty document when legacy values match the defaults', () => {
    const storage = new MemoryStorage();
    storage.setItem('smyklot.panel.theme', 'system');

    migrateLegacyPreferences(storage);

    expect(readPrefsDoc(storage)).toEqual(emptyPrefsDoc());
    expect(storage.getItem('smyklot.panel.theme')).toBeNull();
  });

  it('continues when browser storage is unavailable', () => {
    const getItem = vi.fn(() => {
      throw new DOMException('Storage is unavailable', 'SecurityError');
    });

    expect(() =>
      migrateLegacyPreferences({ getItem, setItem: vi.fn(), removeItem: vi.fn() }),
    ).not.toThrow();
  });
});

/**
 * Storage that is present and unusable, which is not the same as storage that throws.
 *
 * `window.localStorage` is typed non-optional, so nothing here is visible to the type checker: a
 * host can leave the accessor answering undefined, and Node's own Web Storage does exactly that
 * unless the process was given `--localstorage-file`. This module writes and removes as well as
 * reads, so a partial object is no more usable to it than a missing one.
 */
describe('storage the browser declines to provide', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  /* Asserted on `browserStorage` rather than through a reader, because a reader cannot tell the
     two apart: it catches the TypeError from calling a missing method and returns the same empty
     document it would have returned from the guard. The rule under test is which path it took. */
  it.each([
    ['undefined', undefined],
    ['null', null],
    ['read-only', { getItem: (): string | null => null }],
    ['unable to remove', { getItem: (): string | null => null, setItem: (): void => {} }],
  ])('reads as no storage at all when localStorage is %s', (_case, localStorage) => {
    vi.stubGlobal('window', { localStorage });

    expect(browserStorage()).toBeNull();
  });

  it('reads as itself when the storage works', () => {
    const storage = new MemoryStorage();
    writePrefsDoc(emptyPrefsDoc('acme'), storage);
    vi.stubGlobal('window', { localStorage: storage });

    expect(browserStorage()).toBe(storage);
    expect(readPrefsDoc()).toEqual(emptyPrefsDoc('acme'));
  });

  it('leaves every entry point inert when there is no storage', () => {
    vi.stubGlobal('window', { localStorage: undefined });

    expect(readPrefsDoc()).toEqual(emptyPrefsDoc());
    expect(() => {
      writePrefsDoc(emptyPrefsDoc());
    }).not.toThrow();
    expect(() => {
      migrateLegacyPreferences();
    }).not.toThrow();
  });
});
