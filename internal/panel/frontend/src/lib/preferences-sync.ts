import {
  readLastInstallation,
  readSidebarDisplay,
  readThemeDisplay,
  readTimeDisplay,
} from './preferences';

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
  last_installation: null,
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

function browserStorage(): Storage | null {
  if (typeof window === 'undefined') return null;

  try {
    return window.localStorage;
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

  return wellFormed.replace(PREF_STRING_STRIP, '').slice(0, MAX_PREF_STRING_LENGTH);
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
    const lastInstallation = readLastInstallation(storage);
    if (lastInstallation !== null) {
      doc.pending.last_installation = sanitizePrefString(lastInstallation);
    }

    writePrefsDoc(doc, storage);
    storage.removeItem('smyklot.panel.theme');
    storage.removeItem('smyklot.panel.sidebar.display');
    storage.removeItem('smyklot.panel.history.time-display');
    storage.removeItem('smyklot.panel.last-installation');
  } catch {
    // Browser preferences are best-effort and must never block the panel
  }
}
