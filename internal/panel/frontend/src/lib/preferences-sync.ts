import { CONFIG_KEYS } from './config.ts';
import {
  readLastWorkspace,
  readSidebarDisplay,
  readThemeDisplay,
  readTimeDisplay,
} from './preferences.ts';
import type { ConfigKey, RepositorySettingFilter } from './types.ts';

export type PrefValue = string | string[];
export type PrefValues = Record<string, PrefValue>;
export type PendingValues = Record<string, PrefValue | null>;

// PrefsDoc is the single localStorage document behind synced preferences.
// `shadow` mirrors the last server-confirmed state at `rev`/`sum`; `pending`
// overlays local changes not yet acknowledged (null = reset to default).
export interface PrefsDoc {
  account: string | null;
  rev: number;
  sum: string;
  shadow: PrefValues;
  pending: PendingValues;
}

// PREF_DEFAULTS lists every syncable key with the value the panel uses when
// nothing is stored. Values equal to their default are synced as deletions so
// documents stay small. Keys must match the server registry in
// internal/panel/preferences.go.
export const PREF_DEFAULTS = {
  theme: 'system',
  sidebar: 'expanded',
  'history.time_display': 'relative',
  last_workspace: null,
  'table.repositories.sort': 'name_asc',
  'table.repositories.state': 'all',
  'table.repositories.files': [],
  'table.repositories.settings': ['all'],
  'table.repositories.search': '',
  'table.history.type': 'audit',
  'table.history.sort': 'newest',
  'table.history.scope': 'all',
  'table.history.change': 'all',
  'table.history.failure_kind': 'all',
  'table.history.search': '',
  'table.users.sort': 'name_asc',
  'table.users.roles': [],
  'table.users.statuses': [],
  'table.users.search': '',
  'table.invitations.sort': 'name_asc',
  'table.invitations.roles': [],
  'table.invitations.statuses': [],
  'table.invitations.search': '',
} satisfies Record<string, PrefValue | null>;

export type PrefKey = keyof typeof PREF_DEFAULTS;

const PREFS_DOC_KEY = 'smyklot.panel.prefs';
const MAX_PREF_STRING_LENGTH = 256;

// Checksum of the canonical empty document; must equal the Go side's digest
// of "{}" (see the shared golden vectors).
export const EMPTY_DOC_SUM = '44136fa355b3678a';

type PreferenceReader = Pick<Storage, 'getItem'>;
type PreferenceWriter = Pick<Storage, 'setItem'>;
type PreferenceStore = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

// Exported for the specs: the rule it states - unusable storage reads as null - is invisible from
// the readers, which catch the resulting failure either way.
export function browserStorage(): Storage | null {
  if (typeof window === 'undefined') return null;

  try {
    // See the same guard in `preferences.ts`: `window.localStorage` is typed non-optional but can
    // answer undefined on a host whose Web Storage is present and disabled. This module reads,
    // writes and removes, so all three have to be there before it counts as storage.
    const storage = window.localStorage as Storage | null | undefined;
    if (storage === null || storage === undefined) return null;

    const usable =
      typeof storage.getItem === 'function' &&
      typeof storage.setItem === 'function' &&
      typeof storage.removeItem === 'function';

    return usable ? storage : null;
  } catch {
    return null;
  }
}

export function emptyPrefsDoc(account: string | null = null): PrefsDoc {
  return { account, rev: 0, sum: EMPTY_DOC_SUM, shadow: {}, pending: {} };
}

function isPrefValue(value: unknown): value is PrefValue {
  if (typeof value === 'string') return true;

  return Array.isArray(value) && value.every((element) => typeof element === 'string');
}

function isPrefValues(value: unknown): value is PrefValues {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return false;

  return Object.values(value).every(isPrefValue);
}

function isPendingValues(value: unknown): value is PendingValues {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return false;

  return Object.values(value).every((element) => element === null || isPrefValue(element));
}

function isPrefsDoc(value: unknown): value is PrefsDoc {
  if (value === null || typeof value !== 'object') return false;
  const doc = value as Record<string, unknown>;

  return (
    (doc.account === null || typeof doc.account === 'string') &&
    typeof doc.rev === 'number' &&
    Number.isInteger(doc.rev) &&
    doc.rev >= 0 &&
    typeof doc.sum === 'string' &&
    isPrefValues(doc.shadow) &&
    isPendingValues(doc.pending)
  );
}

export function readPrefsDoc(storage: PreferenceReader | null = browserStorage()): PrefsDoc {
  if (storage === null) return emptyPrefsDoc();

  try {
    const stored = storage.getItem(PREFS_DOC_KEY);
    if (stored === null) return emptyPrefsDoc();
    const parsed: unknown = JSON.parse(stored);

    return isPrefsDoc(parsed) ? parsed : emptyPrefsDoc();
  } catch {
    return emptyPrefsDoc();
  }
}

export function writePrefsDoc(
  doc: PrefsDoc,
  storage: PreferenceWriter | null = browserStorage(),
): void {
  if (storage === null) return;

  try {
    storage.setItem(PREFS_DOC_KEY, JSON.stringify(doc));
  } catch {
    // Browser preferences are best-effort and must never block the panel
  }
}

// effectivePrefs is the state the panel renders: the server shadow with the
// pending overlay applied (a pending null removes the key).
export function effectivePrefs(doc: PrefsDoc): PrefValues {
  const values: PrefValues = { ...doc.shadow };
  for (const [key, value] of Object.entries(doc.pending)) {
    if (value === null) {
      delete values[key];
    } else {
      values[key] = value;
    }
  }

  return values;
}

export function effectivePref(doc: PrefsDoc, key: PrefKey): PrefValue | null {
  const pending = doc.pending[key];
  if (pending !== undefined) return pending ?? PREF_DEFAULTS[key];

  return doc.shadow[key] ?? PREF_DEFAULTS[key];
}

export function samePrefValue(
  first: PrefValue | null | undefined,
  second: PrefValue | null | undefined,
): boolean {
  return canonicalStringify(first ?? null) === canonicalStringify(second ?? null);
}

// canonicalStringify serializes to the exact bytes the server digests: object
// keys sorted, compact separators, JSON.stringify string escaping. Must stay
// byte-identical with canonicalPrefs in internal/panel/preferences.go.
export function canonicalStringify(value: unknown): string {
  if (Array.isArray(value)) {
    return '[' + value.map((element) => canonicalStringify(element)).join(',') + ']';
  }
  if (value !== null && typeof value === 'object') {
    const entries = Object.keys(value)
      .sort()
      .map(
        (key) =>
          JSON.stringify(key) + ':' + canonicalStringify((value as Record<string, unknown>)[key]),
      );

    return '{' + entries.join(',') + '}';
  }

  return JSON.stringify(value);
}

// prefsChecksum digests a values object for the connect handshake: the first
// 16 hex characters of SHA-256 over the canonical serialization.
export async function prefsChecksum(values: PrefValues): Promise<string> {
  const canonical = new TextEncoder().encode(canonicalStringify(values));
  const digest = await crypto.subtle.digest('SHA-256', canonical);

  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0'))
    .join('')
    .slice(0, 16);
}

// sanitizePrefString mirrors the server registry's free-text rules so every
// value the client produces is accepted: well-formed, capped, and free of
// control characters and the U+2028/U+2029 separators.
// eslint-disable-next-line no-control-regex
const PREF_STRING_STRIP = /[\u0000-\u001f\u007f\u2028\u2029]/g;

export function sanitizePrefString(value: string): string {
  // toWellFormed is ES2024; fall back to the raw value where it is missing.
  const candidate = value as string & { toWellFormed?: () => string };
  const wellFormed = candidate.toWellFormed?.() ?? value;
  const stripped = wellFormed.replace(PREF_STRING_STRIP, '');

  // Cap by code point, not UTF-16 units: the server counts runes, and a
  // unit-based slice could land inside a surrogate pair.
  return Array.from(stripped).slice(0, MAX_PREF_STRING_LENGTH).join('');
}

// migrateLegacyPreferences moves the four standalone localStorage keys into
// the synced document as pending changes, then removes them. Runs once: an
// existing document means migration already happened.
export function migrateLegacyPreferences(storage: PreferenceStore | null = browserStorage()): void {
  if (storage === null) return;

  try {
    if (storage.getItem(PREFS_DOC_KEY) !== null) return;

    const doc = emptyPrefsDoc();
    const theme = readThemeDisplay(storage);
    if (theme !== PREF_DEFAULTS.theme) doc.pending.theme = theme;
    const sidebar = readSidebarDisplay(storage);
    if (sidebar !== PREF_DEFAULTS.sidebar) doc.pending.sidebar = sidebar;
    const timeDisplay = readTimeDisplay(storage);
    if (timeDisplay !== PREF_DEFAULTS['history.time_display']) {
      doc.pending['history.time_display'] = timeDisplay;
    }
    const lastWorkspace = readLastWorkspace(storage);
    if (lastWorkspace !== null) {
      doc.pending.last_workspace = sanitizePrefString(lastWorkspace);
    }

    writePrefsDoc(doc, storage);
    storage.removeItem('smyklot.panel.theme');
    storage.removeItem('smyklot.panel.sidebar.display');
    storage.removeItem('smyklot.panel.history.time-display');
    storage.removeItem('smyklot.panel.last-workspace');
  } catch {
    // Browser preferences are best-effort and must never block the panel
  }
}

export interface PrefsSyncClock {
  setTimeout(handler: () => void, delay: number): unknown;
  clearTimeout(handle: unknown): void;
}

// PrefsSnapshot and PrefsChange mirror the stream payload shapes in
// events.ts without importing them, keeping this module transport-free.
export interface PrefsSnapshot {
  rev: number;
  sum: string;
  values?: Record<string, unknown>;
}

export interface PrefsChange {
  rev: number;
  changes: Record<string, unknown>;
}

export interface PrefsSync {
  get(key: PrefKey): PrefValue | null;
  set(key: PrefKey, value: PrefValue | null): void;
  subscribe(listener: (keys: string[]) => void): () => void;
  adoptAccount(accountId: string): void;
  dialQuery(): string;
  attach(send: (frame: string) => boolean): void;
  detach(): void;
  onPrefsReady(prefs: PrefsSnapshot): void;
  onPrefsChanged(change: PrefsChange): void;
  onPrefsRejected(keys: string[]): void;
}

export interface PrefsSyncOptions {
  storage?: PreferenceStore | null;
  clock?: PrefsSyncClock;
  debounceMs?: number;
}

const DEFAULT_FLUSH_DEBOUNCE_MS = 300;

const globalPrefsClock: PrefsSyncClock = {
  setTimeout: (handler, delay) => setTimeout(handler, delay),
  clearTimeout: (handle) => clearTimeout(handle as number),
};

// createPrefsSync owns the synced preference document: reads resolve through
// the pending overlay, writes coalesce into debounced prefs.patch frames, and
// the three stream callbacks reconcile server state. Every mutation re-reads
// the document so concurrent tabs converge through the server fan-out.
export function createPrefsSync(options: PrefsSyncOptions = {}): PrefsSync {
  const storage = options.storage === undefined ? browserStorage() : options.storage;
  const clock = options.clock ?? globalPrefsClock;
  const debounceMs = options.debounceMs ?? DEFAULT_FLUSH_DEBOUNCE_MS;
  const listeners = new Set<(keys: string[]) => void>();
  let sendFrame: ((frame: string) => boolean) | null = null;
  let flushHandle: unknown;
  let hashChain: Promise<void> = Promise.resolve();

  const notifyChanged = (before: PrefValues, after: PrefValues): void => {
    const keys = new Set([...Object.keys(before), ...Object.keys(after)]);
    const changed = [...keys].filter((key) => !samePrefValue(before[key], after[key]));
    if (changed.length === 0) return;
    for (const listener of [...listeners]) listener(changed);
  };

  const flushNow = (): void => {
    if (flushHandle !== undefined) {
      clock.clearTimeout(flushHandle);
      flushHandle = undefined;
    }
    if (sendFrame === null) return;
    const doc = readPrefsDoc(storage);
    if (Object.keys(doc.pending).length === 0) return;
    // Pending survives until its echo confirms it; a dropped frame is
    // retried by the next flush or the next connect handshake.
    sendFrame(JSON.stringify({ version: 1, type: 'prefs.patch', changes: doc.pending }));
  };

  const scheduleFlush = (): void => {
    if (sendFrame === null) return;
    if (flushHandle !== undefined) clock.clearTimeout(flushHandle);
    flushHandle = clock.setTimeout(() => {
      flushHandle = undefined;
      flushNow();
    }, debounceMs);
  };

  // dropAcknowledgedPending removes pending entries the shadow now satisfies
  // — the echo of our own patch, or an equal change from another session.
  const dropAcknowledgedPending = (doc: PrefsDoc): void => {
    for (const [key, value] of Object.entries(doc.pending)) {
      const shadow = doc.shadow[key];
      const satisfied = value === null ? shadow === undefined : samePrefValue(shadow, value);
      if (satisfied) delete doc.pending[key];
    }
  };

  // recomputeSum refreshes the stored checksum after an incremental shadow
  // change. Chained so writes stay ordered; abandoned when the revision moved
  // on, leaving the blank checksum to force a snapshot on the next connect.
  const recomputeSum = (rev: number): void => {
    hashChain = hashChain.then(async () => {
      const doc = readPrefsDoc(storage);
      if (doc.rev !== rev) return;
      const sum = await prefsChecksum(doc.shadow);
      const current = readPrefsDoc(storage);
      if (current.rev !== rev) return;
      current.sum = sum;
      writePrefsDoc(current, storage);
    });
  };

  const sanitizeValue = (value: PrefValue): PrefValue =>
    typeof value === 'string' ? sanitizePrefString(value) : value.map(sanitizePrefString);

  return {
    get: (key) => effectivePref(readPrefsDoc(storage), key),

    set: (key, value) => {
      const sanitized = value === null ? null : sanitizeValue(value);
      // Values equal to their default sync as deletions to keep docs small.
      const target = samePrefValue(sanitized, PREF_DEFAULTS[key]) ? null : sanitized;
      const doc = readPrefsDoc(storage);
      const shadowSatisfied =
        target === null ? doc.shadow[key] === undefined : samePrefValue(doc.shadow[key], target);
      if (shadowSatisfied) {
        if (key in doc.pending) {
          delete doc.pending[key];
          writePrefsDoc(doc, storage);
        }
        return;
      }
      if (key in doc.pending && samePrefValue(doc.pending[key], target)) return;
      doc.pending[key] = target;
      writePrefsDoc(doc, storage);
      scheduleFlush();
    },

    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },

    adoptAccount: (accountId) => {
      const doc = readPrefsDoc(storage);
      if (doc.account === accountId) return;
      if (doc.account === null) {
        doc.account = accountId;
        writePrefsDoc(doc, storage);
        return;
      }
      // A different account signed in on this browser: their preferences
      // must not leak across, so the local document starts over.
      const before = effectivePrefs(doc);
      const fresh = emptyPrefsDoc(accountId);
      writePrefsDoc(fresh, storage);
      notifyChanged(before, effectivePrefs(fresh));
    },

    dialQuery: () => {
      const doc = readPrefsDoc(storage);
      return `prefs_rev=${String(doc.rev)}&prefs_sum=${encodeURIComponent(doc.sum)}`;
    },

    attach: (send) => {
      sendFrame = send;
    },

    detach: () => {
      sendFrame = null;
      if (flushHandle !== undefined) {
        clock.clearTimeout(flushHandle);
        flushHandle = undefined;
      }
    },

    onPrefsReady: (prefs) => {
      const doc = readPrefsDoc(storage);
      const before = effectivePrefs(doc);
      if (prefs.values !== undefined) {
        const shadow: PrefValues = {};
        for (const [key, value] of Object.entries(prefs.values)) {
          if (isPrefValue(value)) shadow[key] = value;
        }
        doc.shadow = shadow;
      }
      doc.rev = prefs.rev;
      doc.sum = prefs.sum;
      dropAcknowledgedPending(doc);
      writePrefsDoc(doc, storage);
      notifyChanged(before, effectivePrefs(doc));
      // Surviving pending entries are the user's latest intent: they win
      // over the snapshot and go straight back to the server.
      flushNow();
    },

    onPrefsChanged: ({ rev, changes }) => {
      const doc = readPrefsDoc(storage);
      if (rev <= doc.rev) return;
      const before = effectivePrefs(doc);
      let applied = true;
      for (const [key, value] of Object.entries(changes)) {
        if (value === null) {
          delete doc.shadow[key];
        } else if (isPrefValue(value)) {
          doc.shadow[key] = value;
        } else {
          applied = false;
        }
      }
      const gap = rev > doc.rev + 1;
      doc.rev = rev;
      // Blank until recomputed; if this connection drops first, the blank
      // checksum forces a snapshot on the next connect.
      doc.sum = '';
      dropAcknowledgedPending(doc);
      writePrefsDoc(doc, storage);
      notifyChanged(before, effectivePrefs(doc));
      if (!gap && applied) recomputeSum(rev);
    },

    onPrefsRejected: (keys) => {
      const doc = readPrefsDoc(storage);
      let changed = false;
      for (const key of keys) {
        if (key in doc.pending) {
          delete doc.pending[key];
          changed = true;
        }
      }
      if (changed) writePrefsDoc(doc, storage);
    },
  };
}

// PrefsAccessor is the narrow read/write surface components receive as a
// prop; the stream lifecycle stays with the owner in App.svelte.
export type PrefsAccessor = Pick<PrefsSync, 'get' | 'set'>;

// EPHEMERAL_PREFS backs table components rendered outside the synced panel
// shell (Root administration): reads fall through to defaults and writes are
// dropped, so that state stays local to the mount.
export const EPHEMERAL_PREFS: PrefsAccessor = {
  get: () => null,
  set: () => {},
};

// prefOption narrows a stored value to one of a component's known options,
// falling back when the value is missing or from a newer panel build.
export function prefOption<T extends string>(
  value: PrefValue | null,
  options: readonly T[],
  fallback: T,
): T {
  return typeof value === 'string' && (options as readonly string[]).includes(value)
    ? (value as T)
    : fallback;
}

// prefList narrows a stored array to a component's known options, dropping
// anything unknown.
export function prefList<T extends string>(value: PrefValue | null, options: readonly T[]): T[] {
  if (!Array.isArray(value)) return [];

  return value.filter((element): element is T => (options as readonly string[]).includes(element));
}

export function prefText(value: PrefValue | null): string {
  return typeof value === 'string' ? value : '';
}

// The repository setting filter is an object in the UI but syncs as a flat
// string array — mode first, config keys after it when the mode is "keys" —
// because preference values are limited to strings and string arrays.
export function encodeRepositorySettingFilter(filter: RepositorySettingFilter): string[] {
  return filter.mode === 'keys' ? ['keys', ...filter.keys] : [filter.mode];
}

export function decodeRepositorySettingFilter(value: PrefValue | null): RepositorySettingFilter {
  if (!Array.isArray(value)) return { mode: 'all' };
  const [mode, ...rest] = value;
  if (mode === 'custom' || mode === 'none') return { mode };
  if (mode === 'keys') {
    const keys = rest.filter((key): key is ConfigKey =>
      (CONFIG_KEYS as readonly string[]).includes(key),
    );
    if (keys.length > 0) return { mode: 'keys', keys };
  }

  return { mode: 'all' };
}
