import { SYNC_KINDS, type SyncKind } from './types';

export const SETTINGS_DRAFT_SCHEMA = 1;
const SETTINGS_DRAFT_KEY_PREFIX = `smyklot.panel.settings-drafts.v${SETTINGS_DRAFT_SCHEMA}`;

export type SettingsJson =
  null | boolean | number | string | SettingsJson[] | { [key: string]: SettingsJson };

export type SettingsResource =
  | { type: 'target-defaults'; targetId: string }
  | { type: 'repository-settings'; targetId: string; repositoryId: string }
  | { type: 'sync-config'; targetId: string; kind: SyncKind }
  | { type: 'sync-override'; targetId: string; repositoryId: string; kind: SyncKind }
  | { type: 'runtime' };

export type SettingsScope = { type: 'workspace'; targetId: string } | { type: 'root' };
export type SettingsSection = 'defaults' | 'repositories' | 'sync' | 'runtime';

export interface SettingsLocation {
  section: SettingsSection;
  path?: readonly string[];
}

export interface StoredSettingsControl {
  id: string;
  location: { section: SettingsSection; path: string[] };
  saved: SettingsJson;
  value: SettingsJson;
  changedAt: number;
}

export type StoredSettingsConflict =
  | {
      type: 'revision';
      detectedAt: number;
      actualRevision: number;
      latestBase?: SettingsJson;
    }
  | {
      type: 'external-draft';
      detectedAt: number;
      writerId: string;
      incomingDraft: SettingsJson;
      incomingControls: StoredSettingsControl[];
    };

export interface StoredSettingsVersion {
  writerId: string;
  clock: Record<string, number>;
}

interface StoredSettingsRecordBase {
  key: string;
  resource: SettingsResource;
  version: StoredSettingsVersion;
}

export interface StoredSettingsResource extends StoredSettingsRecordBase {
  deleted: false;
  expectedRevision: number;
  base: SettingsJson;
  draft: SettingsJson;
  controls: StoredSettingsControl[];
  firstChangedAt: number;
  lastChangedAt: number;
  attentionAt: number | null;
  conflict: StoredSettingsConflict | null;
  editToken: string;
}

export interface StoredSettingsTombstone extends StoredSettingsRecordBase {
  deleted: true;
  expectedRevision: number;
  base: SettingsJson;
}

export type StoredSettingsRecord = StoredSettingsResource | StoredSettingsTombstone;

export interface StoredSettingsDraftDocument {
  schema: typeof SETTINGS_DRAFT_SCHEMA;
  accountId: string;
  sequence: number;
  writerId: string;
  records: StoredSettingsRecord[];
}

export type SettingsDraftParseResult =
  | { status: 'empty' }
  | { status: 'valid'; document: StoredSettingsDraftDocument }
  | { status: 'corrupt' };

export type SettingsDraftStorage = Pick<Storage, 'getItem' | 'setItem'>;

const syncKinds = new Set<string>(SYNC_KINDS);
const sections = new Set<string>(['defaults', 'repositories', 'sync', 'runtime']);

export function settingsDraftStorageKey(accountId: string): string {
  return `${SETTINGS_DRAFT_KEY_PREFIX}:${encodeURIComponent(accountId)}`;
}

export function settingsResourceKey(resource: SettingsResource): string {
  switch (resource.type) {
    case 'target-defaults':
      return JSON.stringify([resource.type, resource.targetId]);
    case 'repository-settings':
      return JSON.stringify([resource.type, resource.targetId, resource.repositoryId]);
    case 'sync-config':
      return JSON.stringify([resource.type, resource.targetId, resource.kind]);
    case 'sync-override':
      return JSON.stringify([
        resource.type,
        resource.targetId,
        resource.repositoryId,
        resource.kind,
      ]);
    case 'runtime':
      return JSON.stringify([resource.type]);
  }
}

export function settingsScopeOf(resource: SettingsResource): SettingsScope {
  return resource.type === 'runtime'
    ? { type: 'root' }
    : { type: 'workspace', targetId: resource.targetId };
}

export function settingsScopeKey(scope: SettingsScope): string {
  return scope.type === 'root'
    ? JSON.stringify([scope.type])
    : JSON.stringify([scope.type, scope.targetId]);
}

export function sameSettingsScope(left: SettingsScope, right: SettingsScope): boolean {
  return settingsScopeKey(left) === settingsScopeKey(right);
}

export function normalizeSettingsLocation(
  location: SettingsLocation,
): StoredSettingsControl['location'] {
  if (!sections.has(location.section)) throw new TypeError('unknown settings section');
  const path = [...(location.path ?? [])];
  if (path.some((segment) => segment.length === 0)) {
    throw new TypeError('settings location segments must not be empty');
  }
  return { section: location.section, path };
}

export function settingsLocationStartsWith(
  location: StoredSettingsControl['location'],
  prefix: SettingsLocation,
): boolean {
  if (location.section !== prefix.section) return false;
  const path = prefix.path ?? [];
  return path.every((segment, index) => location.path[index] === segment);
}

export function cloneSettingsJson<T extends SettingsJson>(value: T): T {
  if (!isSettingsJson(value)) throw new TypeError('settings values must be finite JSON values');
  return cloneJson(value) as T;
}

export function sameSettingsJson(left: SettingsJson, right: SettingsJson): boolean {
  return canonicalSettingsJson(left) === canonicalSettingsJson(right);
}

export function parseSettingsDraftDocument(
  serialized: string | null,
  accountId: string,
): SettingsDraftParseResult {
  if (serialized === null) return { status: 'empty' };

  try {
    const parsed: unknown = JSON.parse(serialized);
    if (!isRecord(parsed)) return { status: 'corrupt' };
    if (parsed.schema !== SETTINGS_DRAFT_SCHEMA || parsed.accountId !== accountId) {
      return { status: 'corrupt' };
    }
    if (!isCounter(parsed.sequence) || !isNonEmptyString(parsed.writerId)) {
      return { status: 'corrupt' };
    }
    if (!Array.isArray(parsed.records)) return { status: 'corrupt' };

    const records: StoredSettingsRecord[] = [];
    const keys = new Set<string>();
    for (const candidate of parsed.records) {
      const record = parseStoredRecord(candidate);
      if (record === null || keys.has(record.key)) return { status: 'corrupt' };
      if (Math.max(...Object.values(record.version.clock)) > parsed.sequence) {
        return { status: 'corrupt' };
      }
      keys.add(record.key);
      records.push(record);
    }

    return {
      status: 'valid',
      document: {
        schema: SETTINGS_DRAFT_SCHEMA,
        accountId,
        sequence: parsed.sequence,
        writerId: parsed.writerId,
        records,
      },
    };
  } catch {
    return { status: 'corrupt' };
  }
}

function parseStoredRecord(value: unknown): StoredSettingsRecord | null {
  if (!isRecord(value)) return null;
  const resource = parseResource(value.resource);
  const version = parseVersion(value.version);
  if (resource === null || version === null || value.key !== settingsResourceKey(resource))
    return null;
  if (!isCounter(value.expectedRevision) || !isSettingsJson(value.base)) return null;

  if (value.deleted === true) {
    return {
      deleted: true,
      key: value.key,
      resource,
      version,
      expectedRevision: value.expectedRevision,
      base: cloneJson(value.base),
    };
  }
  if (value.deleted !== false || !isSettingsJson(value.draft) || !Array.isArray(value.controls)) {
    return null;
  }
  if (!isTimestamp(value.firstChangedAt) || !isTimestamp(value.lastChangedAt)) return null;
  if (value.lastChangedAt < value.firstChangedAt) return null;
  if (value.attentionAt !== null && !isTimestamp(value.attentionAt)) return null;
  if (!isNonEmptyString(value.editToken)) return null;

  const controls: StoredSettingsControl[] = [];
  const ids = new Set<string>();
  for (const candidate of value.controls) {
    const control = parseStoredControl(candidate);
    if (control === null || ids.has(control.id)) return null;
    ids.add(control.id);
    controls.push(control);
  }
  const aggregateDirty = !sameSettingsJson(value.base, value.draft);
  const controlsDirty = controls.some((control) => !sameSettingsJson(control.saved, control.value));
  if (aggregateDirty !== controlsDirty) return null;

  const conflict = parseConflict(value.conflict);
  if (value.conflict !== null && conflict === null) return null;

  return {
    deleted: false,
    key: value.key,
    resource,
    version,
    expectedRevision: value.expectedRevision,
    base: cloneJson(value.base),
    draft: cloneJson(value.draft),
    controls,
    firstChangedAt: value.firstChangedAt,
    lastChangedAt: value.lastChangedAt,
    attentionAt: value.attentionAt,
    conflict,
    editToken: value.editToken,
  };
}

function parseVersion(value: unknown): StoredSettingsVersion | null {
  if (!isRecord(value) || !isNonEmptyString(value.writerId) || !isRecord(value.clock)) {
    return null;
  }
  const entries = Object.entries(value.clock);
  if (
    entries.length === 0 ||
    entries.some(([writerId, counter]) => !isNonEmptyString(writerId) || !isCounter(counter)) ||
    !Object.prototype.hasOwnProperty.call(value.clock, value.writerId) ||
    value.clock[value.writerId] === 0
  ) {
    return null;
  }
  const clock: Record<string, number> = {};
  for (const [writerId, counter] of entries.sort(([left], [right]) => left.localeCompare(right))) {
    clock[writerId] = counter as number;
  }
  return { writerId: value.writerId, clock };
}

function parseStoredControl(value: unknown): StoredSettingsControl | null {
  if (!isRecord(value) || !isNonEmptyString(value.id) || !isTimestamp(value.changedAt)) return null;
  if (!isSettingsJson(value.saved) || !isSettingsJson(value.value)) return null;
  if (!isRecord(value.location) || !sections.has(String(value.location.section))) return null;
  if (!Array.isArray(value.location.path) || !value.location.path.every(isNonEmptyString))
    return null;
  return {
    id: value.id,
    location: {
      section: value.location.section as SettingsSection,
      path: [...value.location.path],
    },
    saved: cloneJson(value.saved),
    value: cloneJson(value.value),
    changedAt: value.changedAt,
  };
}

function parseConflict(value: unknown): StoredSettingsConflict | null {
  if (value === null) return null;
  if (!isRecord(value) || !isTimestamp(value.detectedAt)) return null;
  if (value.type === 'revision' && isCounter(value.actualRevision)) {
    if (value.latestBase !== undefined && !isSettingsJson(value.latestBase)) return null;
    return {
      type: 'revision',
      detectedAt: value.detectedAt,
      actualRevision: value.actualRevision,
      ...(value.latestBase === undefined ? {} : { latestBase: cloneJson(value.latestBase) }),
    };
  }
  if (
    value.type === 'external-draft' &&
    isNonEmptyString(value.writerId) &&
    isSettingsJson(value.incomingDraft) &&
    Array.isArray(value.incomingControls)
  ) {
    const incomingControls: StoredSettingsControl[] = [];
    const ids = new Set<string>();
    for (const candidate of value.incomingControls) {
      const control = parseStoredControl(candidate);
      if (control === null || ids.has(control.id)) return null;
      ids.add(control.id);
      incomingControls.push(control);
    }
    return {
      type: 'external-draft',
      detectedAt: value.detectedAt,
      writerId: value.writerId,
      incomingDraft: cloneJson(value.incomingDraft),
      incomingControls,
    };
  }
  return null;
}

function parseResource(value: unknown): SettingsResource | null {
  if (!isRecord(value) || !isNonEmptyString(value.type)) return null;
  if (value.type === 'runtime') return { type: 'runtime' };
  if (!isNonEmptyString(value.targetId)) return null;
  if (value.type === 'target-defaults') return { type: value.type, targetId: value.targetId };
  if (value.type === 'repository-settings' && isNonEmptyString(value.repositoryId)) {
    return { type: value.type, targetId: value.targetId, repositoryId: value.repositoryId };
  }
  if (value.type === 'sync-config' && isSyncKind(value.kind)) {
    return { type: value.type, targetId: value.targetId, kind: value.kind };
  }
  if (
    value.type === 'sync-override' &&
    isNonEmptyString(value.repositoryId) &&
    isSyncKind(value.kind)
  ) {
    return {
      type: value.type,
      targetId: value.targetId,
      repositoryId: value.repositoryId,
      kind: value.kind,
    };
  }
  return null;
}

function isSyncKind(value: unknown): value is SyncKind {
  return typeof value === 'string' && syncKinds.has(value);
}

function isSettingsJson(value: unknown, ancestors = new WeakSet<object>()): value is SettingsJson {
  if (value === null || typeof value === 'string' || typeof value === 'boolean') return true;
  if (typeof value === 'number') return Number.isFinite(value);
  if (typeof value !== 'object' || ancestors.has(value)) return false;
  ancestors.add(value);
  const valid = Array.isArray(value)
    ? value.every((entry) => isSettingsJson(entry, ancestors))
    : (Object.getPrototypeOf(value) === Object.prototype ||
        Object.getPrototypeOf(value) === null) &&
      Object.values(value).every((entry) => isSettingsJson(entry, ancestors));
  ancestors.delete(value);
  return valid;
}

function cloneJson(value: SettingsJson): SettingsJson {
  if (Array.isArray(value)) return value.map(cloneJson);
  if (value !== null && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value).map(([key, entry]) => [key, cloneJson(entry)]));
  }
  return value;
}

function canonicalSettingsJson(value: SettingsJson): string {
  if (Array.isArray(value)) return `[${value.map(canonicalSettingsJson).join(',')}]`;
  if (value !== null && typeof value === 'object') {
    return `{${Object.keys(value)
      .sort()
      .map((key) => `${JSON.stringify(key)}:${canonicalSettingsJson(value[key]!)}`)
      .join(',')}}`;
  }
  return JSON.stringify(value);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0;
}

function isCounter(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0;
}

function isTimestamp(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0;
}
