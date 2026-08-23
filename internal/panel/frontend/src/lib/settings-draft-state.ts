import {
  cloneSettingsJson,
  sameSettingsJson,
  settingsResourceKey,
  type SettingsJson,
  type SettingsLocation,
  type SettingsResource,
  type StoredSettingsConflict,
  type StoredSettingsControl,
  type StoredSettingsRecord,
  type StoredSettingsResource,
  type StoredSettingsTombstone,
  type StoredSettingsVersion,
} from './settings-draft-storage';

export interface ResourceState {
  resource: SettingsResource;
  expectedRevision: number;
  base: SettingsJson;
  draft: SettingsJson;
  controls: Record<string, StoredSettingsControl>;
  firstChangedAt: number | null;
  lastChangedAt: number | null;
  attentionAt: number | null;
  conflict: StoredSettingsConflict | null;
  editToken: string;
}

export interface SettingsDirtyControl {
  resource: SettingsResource;
  resourceKey: string;
  id: string;
  location: { section: SettingsLocation['section']; path: string[] };
  saved: SettingsJson;
  value: SettingsJson;
  changedAt: number;
}

export interface SettingsDraftResourceSnapshot {
  resource: SettingsResource;
  resourceKey: string;
  expectedRevision: number;
  base: SettingsJson;
  value: SettingsJson;
  dirty: boolean;
  controls: SettingsDirtyControl[];
  firstChangedAt: number | null;
  lastChangedAt: number | null;
  attentionAt: number | null;
  conflict: StoredSettingsConflict | null;
}

export type SettingsRecordMap = Record<string, StoredSettingsRecord>;

export function cleanResource(
  resource: SettingsResource,
  expectedRevision: number,
  base: SettingsJson,
): ResourceState {
  const value = cloneSettingsJson(base);
  return {
    resource: cloneResource(resource),
    expectedRevision,
    base: value,
    draft: cloneSettingsJson(value),
    controls: {},
    firstChangedAt: null,
    lastChangedAt: null,
    attentionAt: null,
    conflict: null,
    editToken: '',
  };
}

export function stateFromRecord(record: StoredSettingsRecord): ResourceState {
  if (record.deleted) return cleanResource(record.resource, record.expectedRevision, record.base);
  return {
    resource: cloneResource(record.resource),
    expectedRevision: record.expectedRevision,
    base: cloneSettingsJson(record.base),
    draft: cloneSettingsJson(record.draft),
    controls: Object.fromEntries(
      record.controls.map((control) => [control.id, cloneControl(control)]),
    ),
    firstChangedAt: record.firstChangedAt,
    lastChangedAt: record.lastChangedAt,
    attentionAt: record.attentionAt,
    conflict: cloneConflict(record.conflict),
    editToken: record.editToken,
  };
}

export function resourcesFromRecords(records: SettingsRecordMap): Record<string, ResourceState> {
  return Object.fromEntries(
    Object.entries(records).map(([key, record]) => [key, stateFromRecord(record)]),
  );
}

export function recordMap(records: readonly StoredSettingsRecord[]): SettingsRecordMap {
  return Object.fromEntries(records.map((record) => [record.key, cloneRecord(record)]));
}

export function activeRecord(
  key: string,
  state: ResourceState,
  version: StoredSettingsVersion,
): StoredSettingsResource {
  if (!stateInvariant(state) || !isDirty(state)) {
    throw new TypeError('active settings records must contain a consistent dirty resource');
  }
  return {
    deleted: false,
    key,
    resource: cloneResource(state.resource),
    version: cloneVersion(version),
    expectedRevision: state.expectedRevision,
    base: cloneSettingsJson(state.base),
    draft: cloneSettingsJson(state.draft),
    controls: Object.values(state.controls).map(cloneControl),
    firstChangedAt: state.firstChangedAt ?? 0,
    lastChangedAt: state.lastChangedAt ?? state.firstChangedAt ?? 0,
    attentionAt: state.attentionAt,
    conflict: cloneConflict(state.conflict),
    editToken: state.editToken,
  };
}

export function tombstoneRecord(
  resource: SettingsResource,
  expectedRevision: number,
  base: SettingsJson,
  version: StoredSettingsVersion,
): StoredSettingsTombstone {
  return {
    deleted: true,
    key: settingsResourceKey(resource),
    resource: cloneResource(resource),
    version: cloneVersion(version),
    expectedRevision,
    base: cloneSettingsJson(base),
  };
}

export function mergeRecordMaps(
  left: SettingsRecordMap,
  right: SettingsRecordMap,
  detectedAt: number,
): SettingsRecordMap {
  const merged: SettingsRecordMap = {};
  for (const key of new Set([...Object.keys(left), ...Object.keys(right)])) {
    const local = left[key];
    const incoming = right[key];
    merged[key] =
      local === undefined
        ? cloneRecord(incoming!)
        : incoming === undefined
          ? cloneRecord(local)
          : mergeRecord(local, incoming, detectedAt);
  }
  return merged;
}

export function sameRecordMaps(left: SettingsRecordMap, right: SettingsRecordMap): boolean {
  return canonicalRecordMap(left) === canonicalRecordMap(right);
}

export function highestRecordSequence(records: SettingsRecordMap): number {
  return Object.values(records).reduce((highest, record) => {
    return Math.max(highest, ...Object.values(record.version.clock));
  }, 0);
}

export function latestRecordTimestamp(records: SettingsRecordMap): number {
  let latest = 0;
  for (const record of Object.values(records)) {
    if (record.deleted) continue;
    latest = Math.max(latest, record.firstChangedAt, record.lastChangedAt, record.attentionAt ?? 0);
    if (record.conflict !== null) latest = Math.max(latest, record.conflict.detectedAt);
    for (const control of record.controls) latest = Math.max(latest, control.changedAt);
  }
  return latest;
}

export function snapshotResource(key: string, state: ResourceState): SettingsDraftResourceSnapshot {
  return {
    resource: cloneResource(state.resource),
    resourceKey: key,
    expectedRevision: state.expectedRevision,
    base: cloneSettingsJson(state.base),
    value: cloneSettingsJson(isDirty(state) ? state.draft : state.base),
    dirty: isDirty(state),
    controls: dirtyControlsFor(key, state),
    firstChangedAt: state.firstChangedAt,
    lastChangedAt: state.lastChangedAt,
    attentionAt: state.attentionAt,
    conflict: cloneConflict(state.conflict),
  };
}

export function dirtyControlsFor(key: string, state: ResourceState): SettingsDirtyControl[] {
  return Object.values(state.controls)
    .filter((control) => !sameSettingsJson(control.saved, control.value))
    .map((control) => ({
      resource: cloneResource(state.resource),
      resourceKey: key,
      id: control.id,
      location: { section: control.location.section, path: [...control.location.path] },
      saved: cloneSettingsJson(control.saved),
      value: cloneSettingsJson(control.value),
      changedAt: control.changedAt,
    }));
}

export function isDirty(state: ResourceState): boolean {
  return !sameSettingsJson(state.base, state.draft);
}

export function stateInvariant(state: ResourceState): boolean {
  const controlsDirty = Object.values(state.controls).some(
    (control) => !sameSettingsJson(control.saved, control.value),
  );
  return isDirty(state) === controlsDirty;
}

export function firstDirtyControlAt(state: ResourceState): number | null {
  return minimum(
    Object.values(state.controls)
      .filter((control) => !sameSettingsJson(control.saved, control.value))
      .map((control) => control.changedAt),
  );
}

export function completeSavedProjection(
  state: ResourceState,
  projection: Readonly<Record<string, SettingsJson>>,
): Record<string, StoredSettingsControl> | null {
  const controls: Record<string, StoredSettingsControl> = {};
  for (const [id, control] of Object.entries(state.controls)) {
    if (!Object.prototype.hasOwnProperty.call(projection, id)) return null;
    controls[id] = { ...cloneControl(control), saved: cloneSettingsJson(projection[id]!) };
  }
  return controls;
}

export function discardBase(
  state: ResourceState,
): { expectedRevision: number; base: SettingsJson } | null {
  if (state.conflict?.type !== 'revision') {
    return { expectedRevision: state.expectedRevision, base: cloneSettingsJson(state.base) };
  }
  if (state.conflict.latestBase === undefined) return null;
  return {
    expectedRevision: state.conflict.actualRevision,
    base: cloneSettingsJson(state.conflict.latestBase),
  };
}

export function cloneControl(control: StoredSettingsControl): StoredSettingsControl {
  return {
    id: control.id,
    location: { section: control.location.section, path: [...control.location.path] },
    saved: cloneSettingsJson(control.saved),
    value: cloneSettingsJson(control.value),
    changedAt: control.changedAt,
  };
}

export function cloneConflict(
  conflict: StoredSettingsConflict | null,
): StoredSettingsConflict | null {
  if (conflict === null) return null;
  if (conflict.type === 'revision') {
    return {
      ...conflict,
      ...(conflict.latestBase === undefined
        ? {}
        : { latestBase: cloneSettingsJson(conflict.latestBase) }),
    };
  }
  return {
    ...conflict,
    incomingDraft: cloneSettingsJson(conflict.incomingDraft),
    incomingControls: conflict.incomingControls.map(cloneControl),
  };
}

export function cloneResource(resource: SettingsResource): SettingsResource {
  return { ...resource };
}

function mergeRecord(
  left: StoredSettingsRecord,
  right: StoredSettingsRecord,
  detectedAt: number,
): StoredSettingsRecord {
  if (canonicalRecord(left) === canonicalRecord(right)) return cloneRecord(left);
  const relation = causalRelation(left.version, right.version);

  if (left.deleted && right.deleted) {
    if (relation === 'left-descends') return cloneRecord(left);
    if (relation === 'right-descends') return cloneRecord(right);
    const preferred = preferredRecord(left, right) as StoredSettingsTombstone;
    return tombstoneRecord(
      preferred.resource,
      preferred.expectedRevision,
      preferred.base,
      joinVersion(left.version, right.version, preferred.version.writerId),
    );
  }
  if (!left.deleted && !right.deleted) {
    if (relation === 'left-descends') return cloneRecord(left);
    if (relation === 'right-descends') return cloneRecord(right);
    if (
      left.expectedRevision === right.expectedRevision &&
      sameSettingsJson(left.base, right.base) &&
      sameSettingsJson(left.draft, right.draft)
    ) {
      return mergeMatchingActiveRecords(left, right);
    }
    const winner = preferredRecord(left, right) as StoredSettingsResource;
    const loser = winner === left ? right : left;
    const state = stateFromRecord(winner);
    state.conflict = {
      type: 'external-draft',
      detectedAt,
      writerId: loser.version.writerId,
      incomingDraft: cloneSettingsJson(loser.draft),
      incomingControls: loser.controls.map(cloneControl),
    };
    return activeRecord(
      winner.key,
      state,
      joinVersion(left.version, right.version, winner.version.writerId),
    );
  }

  const active = (left.deleted ? right : left) as StoredSettingsResource;
  const deleted = (left.deleted ? left : right) as StoredSettingsTombstone;
  const deletedRelation = causalRelation(deleted.version, active.version);
  if (deletedRelation === 'left-descends') return cloneRecord(deleted);
  if (deletedRelation === 'right-descends') return cloneRecord(active);

  const state = stateFromRecord(active);
  if (
    state.expectedRevision !== deleted.expectedRevision ||
    !sameSettingsJson(state.base, deleted.base)
  ) {
    state.conflict = {
      type: 'revision',
      detectedAt,
      actualRevision: deleted.expectedRevision,
      latestBase: cloneSettingsJson(deleted.base),
    };
  }
  return activeRecord(
    active.key,
    state,
    joinVersion(active.version, deleted.version, active.version.writerId),
  );
}

function mergeMatchingActiveRecords(
  left: StoredSettingsResource,
  right: StoredSettingsResource,
): StoredSettingsResource {
  const preferred = preferredRecord(left, right) as StoredSettingsResource;
  const state = stateFromRecord(preferred);
  const controls = { ...state.controls };
  for (const incoming of (preferred === left ? right : left).controls) {
    const current = controls[incoming.id];
    if (
      current === undefined ||
      incoming.changedAt > current.changedAt ||
      (incoming.changedAt === current.changedAt &&
        canonicalControl(incoming) > canonicalControl(current))
    ) {
      controls[incoming.id] = cloneControl(incoming);
    }
  }
  state.controls = controls;
  state.firstChangedAt = minimum([left.firstChangedAt, right.firstChangedAt]);
  state.lastChangedAt = Math.max(left.lastChangedAt, right.lastChangedAt);
  state.attentionAt = maximum([left.attentionAt, right.attentionAt]);
  state.conflict = cloneConflict(left.conflict ?? right.conflict);
  return activeRecord(
    preferred.key,
    state,
    joinVersion(left.version, right.version, preferred.version.writerId),
  );
}

function preferredRecord<T extends StoredSettingsRecord>(left: T, right: T): T {
  if (left.deleted !== right.deleted) return left.deleted ? right : left;
  if (left.version.writerId !== right.version.writerId) {
    return left.version.writerId > right.version.writerId ? left : right;
  }
  return canonicalRecord(left) >= canonicalRecord(right) ? left : right;
}

function cloneRecord(record: StoredSettingsRecord): StoredSettingsRecord {
  if (record.deleted) {
    return tombstoneRecord(record.resource, record.expectedRevision, record.base, record.version);
  }
  return activeRecord(record.key, stateFromRecord(record), record.version);
}

function cloneVersion(version: StoredSettingsVersion): StoredSettingsVersion {
  return {
    writerId: version.writerId,
    clock: Object.fromEntries(
      Object.entries(version.clock).sort(([left], [right]) => left.localeCompare(right)),
    ),
  };
}

type CausalRelation = 'equal' | 'left-descends' | 'right-descends' | 'concurrent';

function causalRelation(left: StoredSettingsVersion, right: StoredSettingsVersion): CausalRelation {
  const leftDominates = dominates(left.clock, right.clock);
  const rightDominates = dominates(right.clock, left.clock);
  if (leftDominates && rightDominates) return 'equal';
  if (leftDominates) return 'left-descends';
  if (rightDominates) return 'right-descends';
  return 'concurrent';
}

function dominates(left: Record<string, number>, right: Record<string, number>): boolean {
  return Object.entries(right).every(([writerId, counter]) => (left[writerId] ?? 0) >= counter);
}

function joinVersion(
  left: StoredSettingsVersion,
  right: StoredSettingsVersion,
  writerId: string,
): StoredSettingsVersion {
  const clock: Record<string, number> = {};
  for (const id of new Set([...Object.keys(left.clock), ...Object.keys(right.clock)])) {
    clock[id] = Math.max(left.clock[id] ?? 0, right.clock[id] ?? 0);
  }
  return cloneVersion({ writerId, clock });
}

function canonicalRecordMap(records: SettingsRecordMap): string {
  return JSON.stringify(
    Object.keys(records)
      .sort()
      .map((key) => records[key]),
  );
}

function canonicalRecord(record: StoredSettingsRecord): string {
  return JSON.stringify(record);
}

function canonicalControl(control: StoredSettingsControl): string {
  return JSON.stringify(control);
}

function minimum(values: (number | null)[]): number | null {
  const present = values.filter((value): value is number => value !== null);
  return present.length === 0 ? null : Math.min(...present);
}

function maximum(values: (number | null)[]): number | null {
  const present = values.filter((value): value is number => value !== null);
  return present.length === 0 ? null : Math.max(...present);
}
