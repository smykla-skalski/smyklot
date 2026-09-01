import { createContext } from 'svelte';

import { browserStorage } from './preferences';
import {
  activeRecord,
  cleanResource,
  cloneControl,
  cloneResource,
  completeSavedProjection,
  dirtyControlsFor,
  discardBase,
  firstDirtyControlAt,
  highestRecordSequence,
  isDirty,
  latestRecordTimestamp,
  mergeRecordMaps,
  recordMap,
  resourcesFromRecords,
  sameRecordMaps,
  snapshotResource,
  stateFromRecord,
  stateInvariant,
  tombstoneRecord,
  type ResourceState,
  type SettingsDirtyControl,
  type SettingsDraftResourceSnapshot,
  type SettingsRecordMap,
} from './settings-draft-state';
import {
  SETTINGS_DRAFT_SCHEMA,
  cloneSettingsJson,
  normalizeSettingsLocation,
  parseSettingsDraftDocument,
  sameSettingsJson,
  sameSettingsScope,
  settingsDraftStorageKey,
  settingsLocationStartsWith,
  settingsResourceKey,
  settingsScopeKey,
  settingsScopeOf,
  type SettingsDraftStorage,
  type SettingsJson,
  type SettingsLocation,
  type SettingsResource,
  type SettingsScope,
  type StoredSettingsDraftDocument,
  type StoredSettingsVersion,
} from './settings-draft-storage';

export {
  SETTINGS_DRAFT_SCHEMA,
  settingsDraftStorageKey,
  settingsResourceKey,
  settingsScopeKey,
  settingsScopeOf,
} from './settings-draft-storage';
export type {
  SettingsDraftStorage,
  SettingsJson,
  SettingsLocation,
  SettingsResource,
  SettingsScope,
  SettingsSection,
} from './settings-draft-storage';
export type { SettingsDirtyControl, SettingsDraftResourceSnapshot } from './settings-draft-state';

export interface SettingsControlChange {
  id: string;
  location: SettingsLocation;
  saved: SettingsJson;
  value: SettingsJson;
}

export interface SettingsDraftTimestamps {
  firstChangedAt: number | null;
  lastChangedAt: number | null;
  attentionAt: number | null;
}

export interface SettingsDraftOperationState {
  saving: boolean;
  problem: string | null;
  notice: string | null;
}

export interface SettingsSaveEntry {
  resource: SettingsResource;
  resourceKey: string;
  expectedRevision: number;
  value: SettingsJson;
  controls: SettingsDirtyControl[];
  editToken: string;
}

export interface SettingsSaveAttempt {
  accountId: string;
  scope: SettingsScope;
  token: string;
  startedAt: number;
  entries: SettingsSaveEntry[];
}

export interface SettingsCommittedResource {
  resource: SettingsResource;
  revision: number;
  value: SettingsJson;
  /** Canonical saved value for every control currently known for this resource. */
  savedControls: Readonly<Record<string, SettingsJson>>;
}

export interface SettingsSaveConflict {
  resource: SettingsResource;
  actualRevision: number;
  latestBase?: SettingsJson;
}

export interface SettingsHydrationResult {
  restoredResources: number;
  firstChangedAt: number | null;
}

export interface SettingsDraftRegistryOptions {
  storage?: SettingsDraftStorage | null;
  now?: () => number;
  writerId?: string;
}

interface OperationState {
  token: string | null;
  problem: string | null;
  notice: string | null;
}

const cleanOperation: OperationState = { token: null, problem: null, notice: null };
const unavailableStorage = 'Browser storage is unavailable. Unsaved changes will not survive';
const corruptStorage =
  'Stored settings drafts are corrupted. They were left untouched so they can be recovered';

export class SettingsDraftRegistry {
  accountId = $state<string | null>(null);
  storageProblem = $state<string | null>(null);

  private resources = $state.raw<Record<string, ResourceState>>({});
  private records = $state.raw<SettingsRecordMap>({});
  private operations = $state.raw<Record<string, OperationState>>({});
  private validationProblems = $state.raw<Record<string, Record<string, string>>>({});
  private sequence = 0;
  private editCounter = 0;
  private logicalTime = 0;
  private listening = false;
  private readonly storage: SettingsDraftStorage | null;
  private readonly now: () => number;
  private readonly writerId: string;

  constructor(options: SettingsDraftRegistryOptions = {}) {
    this.storage = options.storage === undefined ? writableBrowserStorage() : options.storage;
    this.now = options.now ?? Date.now;
    this.writerId = options.writerId ?? createWriterId();
  }

  get dirtyResourceCount(): number {
    return this.dirtyStates().length;
  }

  get dirtyControlCount(): number {
    return this.dirtyControls().length;
  }

  get dirty(): boolean {
    return this.dirtyResourceCount > 0;
  }

  get dirtyTargetIds(): string[] {
    const targets: string[] = [];
    for (const state of this.dirtyStates()) {
      const scope = settingsScopeOf(state.resource);
      if (scope.type === 'workspace' && !targets.includes(scope.targetId)) {
        targets.push(scope.targetId);
      }
    }
    return targets;
  }

  hydrate(accountId: string | null): SettingsHydrationResult {
    this.accountId = accountId;
    this.resources = {};
    this.records = {};
    this.operations = {};
    this.validationProblems = {};
    this.sequence = 0;
    this.editCounter = 0;
    this.logicalTime = 0;
    this.storageProblem = null;
    this.listenForStorage();
    if (accountId === null || accountId.length === 0 || this.storage === null) {
      return { restoredResources: 0, firstChangedAt: null };
    }

    try {
      const parsed = parseSettingsDraftDocument(
        this.storage.getItem(settingsDraftStorageKey(accountId)),
        accountId,
      );
      if (parsed.status === 'corrupt') {
        this.storageProblem = corruptStorage;
        return { restoredResources: 0, firstChangedAt: null };
      }
      if (parsed.status === 'valid') this.adoptDocument(parsed.document);
    } catch {
      this.storageProblem = unavailableStorage;
    }

    const timestamps = this.timestamps();
    return {
      restoredResources: this.dirtyResourceCount,
      firstChangedAt: timestamps.firstChangedAt,
    };
  }

  dispose(): void {
    if (!this.listening || typeof window === 'undefined') return;
    window.removeEventListener('storage', this.onStorage);
    this.listening = false;
  }

  adoptBase(resource: SettingsResource, expectedRevision: number, base: SettingsJson): boolean {
    assertRevision(expectedRevision);
    this.syncFromStorage();
    const key = settingsResourceKey(resource);
    const value = cloneSettingsJson(base);
    const current = this.resources[key];
    if (current === undefined) {
      this.setResource(key, cleanResource(resource, expectedRevision, value));
      return true;
    }
    if (expectedRevision < current.expectedRevision) return false;
    if (!isDirty(current)) {
      const clean = cleanResource(resource, expectedRevision, value);
      if (this.records[key] === undefined) this.setResource(key, clean);
      else this.setStateAndPersist(key, clean);
      return true;
    }
    if (sameSettingsJson(current.base, value)) {
      this.setStateAndPersist(key, { ...current, expectedRevision });
      return true;
    }
    if (sameSettingsJson(current.draft, value)) {
      this.setStateAndPersist(key, cleanResource(resource, expectedRevision, value));
      return true;
    }

    this.setStateAndPersist(key, {
      ...current,
      conflict: {
        type: 'revision',
        detectedAt: this.timestamp(),
        actualRevision: expectedRevision,
        latestBase: value,
      },
    });
    return false;
  }

  stage(
    resource: SettingsResource,
    nextValue: SettingsJson,
    change: SettingsControlChange,
  ): boolean {
    this.syncFromStorage();
    const key = settingsResourceKey(resource);
    const current = this.resources[key];
    if (current === undefined || change.id.length === 0) return false;

    const at = this.timestamp();
    const controls = { ...current.controls };
    const previous = controls[change.id];
    controls[change.id] = {
      id: change.id,
      location: normalizeSettingsLocation(change.location),
      saved: cloneSettingsJson(previous?.saved ?? change.saved),
      value: cloneSettingsJson(change.value),
      changedAt: at,
    };
    const candidate: ResourceState = {
      ...current,
      draft: cloneSettingsJson(nextValue),
      controls,
      editToken: this.nextEditToken(at),
    };
    const dirty = isDirty(candidate);
    candidate.firstChangedAt = dirty
      ? (current.firstChangedAt ?? firstDirtyControlAt(candidate) ?? at)
      : null;
    candidate.lastChangedAt = dirty ? at : null;
    candidate.attentionAt = dirty ? current.attentionAt : null;
    if (!dirty) candidate.conflict = null;
    if (!stateInvariant(candidate)) return false;

    this.setStateAndPersist(key, candidate);
    this.clearOperationMessage(settingsScopeOf(resource));
    return true;
  }

  value(resource: SettingsResource): SettingsJson | null {
    const state = this.resources[settingsResourceKey(resource)];
    return state === undefined
      ? null
      : cloneSettingsJson(isDirty(state) ? state.draft : state.base);
  }

  resource(resource: SettingsResource): SettingsDraftResourceSnapshot | null {
    const key = settingsResourceKey(resource);
    const state = this.resources[key];
    return state === undefined ? null : snapshotResource(key, state);
  }

  dirtyResources(scope?: SettingsScope): SettingsDraftResourceSnapshot[] {
    return Object.entries(this.resources)
      .filter(([, state]) => isDirty(state) && inScope(state, scope))
      .map(([key, state]) => snapshotResource(key, state));
  }

  dirtyControls(scope?: SettingsScope): SettingsDirtyControl[] {
    return Object.entries(this.resources).flatMap(([key, state]) =>
      isDirty(state) && inScope(state, scope) ? dirtyControlsFor(key, state) : [],
    );
  }

  isControlDirty(scope: SettingsScope, controlId: string): boolean {
    return this.dirtyControls(scope).some((control) => control.id === controlId);
  }

  dirtyAt(scope: SettingsScope, location: SettingsLocation): boolean {
    return this.dirtyControls(scope).some((control) =>
      settingsLocationStartsWith(control.location, location),
    );
  }

  hasDirty(scope: SettingsScope): boolean {
    return this.dirtyStates(scope).length > 0;
  }

  /**
   * Register an editor-only problem that cannot be represented in the typed
   * draft yet, such as a partially entered bounded integer. These problems are
   * deliberately not persisted: the invalid text belongs to the mounted input,
   * while every stored draft remains valid and round-trippable.
   */
  setValidationProblem(scope: SettingsScope, controlId: string, problem: string | null): void {
    const scopeKey = settingsScopeKey(scope);
    const current = this.validationProblems[scopeKey] ?? {};
    if (
      (problem === null && !Object.hasOwn(current, controlId)) ||
      (problem !== null && current[controlId] === problem)
    ) {
      return;
    }
    const next = { ...current };
    if (problem === null) delete next[controlId];
    else next[controlId] = problem;

    const validationProblems = { ...this.validationProblems };
    if (Object.keys(next).length === 0) delete validationProblems[scopeKey];
    else validationProblems[scopeKey] = next;
    this.validationProblems = validationProblems;
  }

  validationProblem(scope: SettingsScope): string | null {
    const problems = this.validationProblems[settingsScopeKey(scope)];
    if (problems === undefined) return null;
    const first = Object.keys(problems).sort()[0];
    return first === undefined ? null : (problems[first] ?? null);
  }

  hasConflicts(scope: SettingsScope): boolean {
    return this.dirtyStates(scope).some((state) => state.conflict !== null);
  }

  timestamps(scope?: SettingsScope): SettingsDraftTimestamps {
    const states = this.dirtyStates(scope);
    return {
      firstChangedAt: minimum(states.map((state) => state.firstChangedAt)),
      lastChangedAt: maximum(states.map((state) => state.lastChangedAt)),
      attentionAt: maximum(states.map((state) => state.attentionAt)),
    };
  }

  markAttention(scope: SettingsScope): boolean {
    this.syncFromStorage();
    const at = this.timestamp();
    const resources = { ...this.resources };
    const records = { ...this.records };
    let changed = false;
    for (const [key, state] of Object.entries(resources)) {
      if (!isDirty(state) || !sameSettingsScope(settingsScopeOf(state.resource), scope)) continue;
      const next = { ...state, attentionAt: at };
      resources[key] = next;
      records[key] = activeRecord(key, next, this.nextVersion(key));
      changed = true;
    }
    if (!changed) return false;
    this.resources = resources;
    this.records = records;
    this.persist();
    return true;
  }

  discardResource(resource: SettingsResource): boolean {
    const scope = settingsScopeOf(resource);
    if (this.isSaving(scope)) return false;
    this.syncFromStorage();
    const key = settingsResourceKey(resource);
    const current = this.resources[key];
    if (current === undefined || !isDirty(current)) return false;
    const discarded = discardBase(current);
    if (discarded === null) return false;
    this.setStateAndPersist(
      key,
      cleanResource(resource, discarded.expectedRevision, discarded.base),
    );
    this.clearOperationMessage(scope);
    return true;
  }

  discardScope(scope: SettingsScope): number {
    if (this.isSaving(scope)) return 0;
    this.syncFromStorage();
    const selected = Object.entries(this.resources).filter(
      ([, state]) => isDirty(state) && sameSettingsScope(settingsScopeOf(state.resource), scope),
    );
    const discarded = selected.map(([, state]) => discardBase(state));
    if (selected.length === 0 || discarded.some((value) => value === null)) return 0;

    const resources = { ...this.resources };
    const records = { ...this.records };
    selected.forEach(([key, state], index) => {
      const result = discarded[index]!;
      const clean = cleanResource(state.resource, result.expectedRevision, result.base);
      resources[key] = clean;
      records[key] = tombstoneRecord(
        state.resource,
        result.expectedRevision,
        result.base,
        this.nextVersion(key),
      );
    });
    this.resources = resources;
    this.records = records;
    this.operations = { ...this.operations, [settingsScopeKey(scope)]: { ...cleanOperation } };
    const validationProblems = { ...this.validationProblems };
    delete validationProblems[settingsScopeKey(scope)];
    this.validationProblems = validationProblems;
    this.persist();
    return selected.length;
  }

  beginSave(scope: SettingsScope): SettingsSaveAttempt | null {
    this.syncFromStorage();
    const accountId = this.accountId;
    const states = Object.entries(this.resources).filter(
      ([, state]) => isDirty(state) && sameSettingsScope(settingsScopeOf(state.resource), scope),
    );
    if (
      accountId === null ||
      states.length === 0 ||
      states.some(([, state]) => state.conflict !== null) ||
      this.validationProblem(scope) !== null ||
      this.isSaving(scope)
    ) {
      return null;
    }

    const startedAt = this.timestamp();
    const token = this.nextEditToken(startedAt);
    const attempt: SettingsSaveAttempt = {
      accountId,
      scope: cloneScope(scope),
      token,
      startedAt,
      entries: states.map(([key, state]) => ({
        resource: cloneResource(state.resource),
        resourceKey: key,
        expectedRevision: state.expectedRevision,
        value: cloneSettingsJson(state.draft),
        controls: dirtyControlsFor(key, state),
        editToken: state.editToken,
      })),
    };
    this.operations = {
      ...this.operations,
      [settingsScopeKey(scope)]: { token, problem: null, notice: null },
    };
    return attempt;
  }

  commitSave(
    attempt: SettingsSaveAttempt,
    committed: readonly SettingsCommittedResource[],
    notice: string | null = null,
  ): boolean {
    if (!this.ownsAttempt(attempt)) return false;
    this.syncFromStorage();
    const results = Object.fromEntries(
      committed.map((result) => [settingsResourceKey(result.resource), result]),
    );
    if (
      attempt.entries.some((entry) => {
        const result = results[entry.resourceKey];
        return result === undefined || !validRevision(result.revision);
      })
    ) {
      this.finishAttempt(attempt, 'The settings save returned an incomplete result', null);
      return false;
    }

    const resources = { ...this.resources };
    const planned: Array<[string, ResourceState]> = [];
    try {
      for (const submitted of attempt.entries) {
        const current = resources[submitted.resourceKey];
        const result = results[submitted.resourceKey]!;
        const base = cloneSettingsJson(result.value);
        const next = this.committedState(submitted, current, result, base);
        if (next === null || !stateInvariant(next)) {
          this.finishAttempt(attempt, 'The settings save returned incomplete control values', null);
          return false;
        }
        planned.push([submitted.resourceKey, next]);
      }
    } catch {
      this.finishAttempt(attempt, 'The settings save returned invalid values', null);
      return false;
    }

    const records = { ...this.records };
    for (const [key, next] of planned) {
      resources[key] = next;
      records[key] = isDirty(next)
        ? activeRecord(key, next, this.nextVersion(key))
        : tombstoneRecord(next.resource, next.expectedRevision, next.base, this.nextVersion(key));
    }
    this.resources = resources;
    this.records = records;
    this.finishAttempt(attempt, null, notice);
    this.persist();
    return true;
  }

  failSave(
    attempt: SettingsSaveAttempt,
    problem: string,
    conflicts: readonly SettingsSaveConflict[] = [],
  ): boolean {
    if (!this.ownsAttempt(attempt)) return false;
    this.syncFromStorage();
    const at = this.timestamp();
    const resources = { ...this.resources };
    const records = { ...this.records };
    for (const conflict of conflicts) {
      if (!validRevision(conflict.actualRevision)) continue;
      const key = settingsResourceKey(conflict.resource);
      const current = resources[key];
      if (current === undefined || !isDirty(current)) continue;
      const next: ResourceState = {
        ...current,
        conflict: {
          type: 'revision',
          detectedAt: at,
          actualRevision: conflict.actualRevision,
          ...(conflict.latestBase === undefined
            ? {}
            : { latestBase: cloneSettingsJson(conflict.latestBase) }),
        },
      };
      resources[key] = next;
      records[key] = activeRecord(key, next, this.nextVersion(key));
    }
    this.resources = resources;
    this.records = records;
    this.finishAttempt(attempt, problem, null);
    this.persist();
    return true;
  }

  rebase(
    resource: SettingsResource,
    expectedRevision: number,
    latestBase: SettingsJson,
    savedControls: Readonly<Record<string, SettingsJson>>,
    rebasedDraft?: SettingsJson,
  ): boolean {
    assertRevision(expectedRevision);
    this.syncFromStorage();
    const key = settingsResourceKey(resource);
    const current = this.resources[key];
    if (current === undefined || !isDirty(current) || this.isSaving(settingsScopeOf(resource))) {
      return false;
    }
    const controls = completeSavedProjection(current, savedControls);
    if (controls === null) return false;

    const rebased: ResourceState = {
      ...current,
      expectedRevision,
      base: cloneSettingsJson(latestBase),
      draft: cloneSettingsJson(rebasedDraft ?? current.draft),
      controls,
      conflict: null,
      editToken: this.nextEditToken(this.timestamp()),
    };
    if (!stateInvariant(rebased)) return false;
    if (isDirty(rebased)) {
      rebased.firstChangedAt = firstDirtyControlAt(rebased) ?? rebased.firstChangedAt;
      this.setStateAndPersist(key, rebased);
    } else {
      this.setStateAndPersist(key, cleanResource(resource, expectedRevision, latestBase));
    }
    return true;
  }

  /** Keep the draft currently shown after concurrent browser tabs converge. */
  resolveExternalConflicts(scope: SettingsScope): number {
    if (this.isSaving(scope)) return 0;
    this.syncFromStorage();
    const resources = { ...this.resources };
    const records = { ...this.records };
    let resolved = 0;
    for (const [key, state] of Object.entries(resources)) {
      if (
        !isDirty(state) ||
        !sameSettingsScope(settingsScopeOf(state.resource), scope) ||
        state.conflict?.type !== 'external-draft'
      ) {
        continue;
      }
      const next: ResourceState = {
        ...state,
        conflict: null,
        editToken: this.nextEditToken(this.timestamp()),
      };
      resources[key] = next;
      records[key] = activeRecord(key, next, this.nextVersion(key));
      resolved += 1;
    }
    if (resolved === 0) return 0;
    this.resources = resources;
    this.records = records;
    this.persist();
    return resolved;
  }

  operation(scope: SettingsScope): SettingsDraftOperationState {
    const operation = this.operations[settingsScopeKey(scope)] ?? cleanOperation;
    return {
      saving: operation.token !== null,
      problem: operation.problem,
      notice: operation.notice,
    };
  }

  isSaving(scope: SettingsScope): boolean {
    return this.operation(scope).saving;
  }

  dismissNotice(scope: SettingsScope): void {
    const key = settingsScopeKey(scope);
    const operation = this.operations[key];
    if (operation === undefined || operation.notice === null) return;
    this.operations = { ...this.operations, [key]: { ...operation, notice: null } };
  }

  dismissProblem(scope: SettingsScope): void {
    const key = settingsScopeKey(scope);
    const operation = this.operations[key];
    if (operation === undefined || operation.problem === null) return;
    this.operations = { ...this.operations, [key]: { ...operation, problem: null } };
  }

  reconcile(serialized: string | null): number {
    const accountId = this.accountId;
    if (accountId === null) return 0;
    const parsed = parseSettingsDraftDocument(serialized, accountId);
    if (parsed.status === 'corrupt') {
      this.storageProblem = corruptStorage;
      return 0;
    }
    const incoming = parsed.status === 'valid' ? recordMap(parsed.document.records) : {};
    if (parsed.status === 'valid')
      this.sequence = Math.max(this.sequence, parsed.document.sequence);
    const before = this.records;
    const merged = mergeRecordMaps(before, incoming, this.observedTime());
    const changed = changedRecordCount(before, merged);
    this.applyRecords(merged);
    if (!sameRecordMaps(merged, incoming)) this.writeDocument();
    else this.storageProblem = null;
    return changed;
  }

  private committedState(
    submitted: SettingsSaveEntry,
    current: ResourceState | undefined,
    result: SettingsCommittedResource,
    base: SettingsJson,
  ): ResourceState | null {
    if (current === undefined) return cleanResource(submitted.resource, result.revision, base);

    if (current.editToken === submitted.editToken && current.conflict?.type !== 'external-draft') {
      if (
        !hasProjection(
          result.savedControls,
          submitted.controls.map((control) => control.id),
        )
      ) {
        return null;
      }
      return cleanResource(submitted.resource, result.revision, base);
    }

    let draft = current.draft;
    let sourceControls = current.controls;
    if (current.editToken === submitted.editToken && current.conflict?.type === 'external-draft') {
      draft = current.conflict.incomingDraft;
      sourceControls = Object.fromEntries(
        current.conflict.incomingControls.map((control) => [control.id, cloneControl(control)]),
      );
    }
    const projected = completeSavedProjection(
      { ...current, controls: sourceControls },
      result.savedControls,
    );
    if (projected === null) return null;

    const rebased: ResourceState = {
      ...current,
      expectedRevision: result.revision,
      base,
      draft: cloneSettingsJson(draft),
      controls: projected,
      conflict: null,
    };
    if (isDirty(rebased)) {
      rebased.firstChangedAt = firstDirtyControlAt(rebased) ?? current.firstChangedAt;
      rebased.lastChangedAt = maximum(Object.values(projected).map((control) => control.changedAt));
    } else {
      return cleanResource(submitted.resource, result.revision, base);
    }
    return rebased;
  }

  private readonly onStorage = (event: StorageEvent): void => {
    if (this.accountId === null || event.key !== settingsDraftStorageKey(this.accountId)) return;
    this.reconcile(event.newValue);
  };

  private listenForStorage(): void {
    if (this.listening || typeof window === 'undefined') return;
    window.addEventListener('storage', this.onStorage);
    this.listening = true;
  }

  private dirtyStates(scope?: SettingsScope): ResourceState[] {
    return Object.values(this.resources).filter((state) => isDirty(state) && inScope(state, scope));
  }

  private setResource(key: string, state: ResourceState): void {
    this.resources = { ...this.resources, [key]: state };
  }

  private setStateAndPersist(key: string, state: ResourceState): void {
    if (!stateInvariant(state)) throw new TypeError('settings resource and controls disagree');
    this.setResource(key, state);
    this.records = {
      ...this.records,
      [key]: isDirty(state)
        ? activeRecord(key, state, this.nextVersion(key))
        : tombstoneRecord(
            state.resource,
            state.expectedRevision,
            state.base,
            this.nextVersion(key),
          ),
    };
    this.persist();
  }

  private nextEditToken(at: number): string {
    this.editCounter += 1;
    return `${this.writerId}:${at}:${this.editCounter}`;
  }

  private nextVersion(key: string): StoredSettingsVersion {
    const observed = this.records[key]?.version.clock ?? {};
    this.sequence = Math.max(this.sequence, ...Object.values(observed)) + 1;
    return {
      writerId: this.writerId,
      clock: { ...observed, [this.writerId]: this.sequence },
    };
  }

  private timestamp(): number {
    const value = this.now();
    if (!Number.isFinite(value) || value < 0) {
      throw new TypeError('draft clock returned an invalid time');
    }
    this.logicalTime = Math.max(Math.floor(value), this.logicalTime + 1);
    return this.logicalTime;
  }

  private observedTime(): number {
    const value = this.now();
    if (!Number.isFinite(value) || value < 0) {
      throw new TypeError('draft clock returned an invalid time');
    }
    return Math.max(Math.floor(value), this.logicalTime);
  }

  private ownsAttempt(attempt: SettingsSaveAttempt): boolean {
    if (attempt.accountId !== this.accountId) return false;
    return this.operations[settingsScopeKey(attempt.scope)]?.token === attempt.token;
  }

  private finishAttempt(
    attempt: SettingsSaveAttempt,
    problem: string | null,
    notice: string | null,
  ): void {
    this.operations = {
      ...this.operations,
      [settingsScopeKey(attempt.scope)]: { token: null, problem, notice },
    };
  }

  private clearOperationMessage(scope: SettingsScope): void {
    const key = settingsScopeKey(scope);
    const operation = this.operations[key];
    if (operation === undefined || (operation.problem === null && operation.notice === null))
      return;
    this.operations = {
      ...this.operations,
      [key]: { ...operation, problem: null, notice: null },
    };
  }

  private syncFromStorage(): void {
    const accountId = this.accountId;
    if (accountId === null || this.storage === null) return;
    try {
      const parsed = parseSettingsDraftDocument(
        this.storage.getItem(settingsDraftStorageKey(accountId)),
        accountId,
      );
      if (parsed.status === 'corrupt') {
        this.storageProblem = corruptStorage;
        return;
      }
      if (parsed.status === 'empty') return;
      this.sequence = Math.max(this.sequence, parsed.document.sequence);
      const merged = mergeRecordMaps(
        this.records,
        recordMap(parsed.document.records),
        this.observedTime(),
      );
      this.applyRecords(merged);
    } catch {
      this.storageProblem = unavailableStorage;
    }
  }

  private persist(): void {
    const accountId = this.accountId;
    if (accountId === null || this.storage === null) return;
    try {
      const parsed = parseSettingsDraftDocument(
        this.storage.getItem(settingsDraftStorageKey(accountId)),
        accountId,
      );
      if (parsed.status === 'corrupt') {
        this.storageProblem = corruptStorage;
        return;
      }
      const stored = parsed.status === 'valid' ? recordMap(parsed.document.records) : {};
      if (parsed.status === 'valid')
        this.sequence = Math.max(this.sequence, parsed.document.sequence);
      this.applyRecords(mergeRecordMaps(stored, this.records, this.observedTime()));
      this.writeDocument();
    } catch {
      this.storageProblem = unavailableStorage;
    }
  }

  private writeDocument(): void {
    const accountId = this.accountId;
    if (accountId === null || this.storage === null) return;
    this.sequence = Math.max(this.sequence, highestRecordSequence(this.records));
    const document: StoredSettingsDraftDocument = {
      schema: SETTINGS_DRAFT_SCHEMA,
      accountId,
      sequence: this.sequence,
      writerId: this.writerId,
      records: Object.keys(this.records)
        .sort()
        .map((key) => this.records[key]!),
    };
    try {
      this.storage.setItem(settingsDraftStorageKey(accountId), JSON.stringify(document));
      this.storageProblem = null;
    } catch {
      this.storageProblem = unavailableStorage;
    }
  }

  private adoptDocument(document: StoredSettingsDraftDocument): void {
    this.sequence = Math.max(document.sequence, highestRecordSequence(recordMap(document.records)));
    this.editCounter = this.sequence;
    this.records = recordMap(document.records);
    this.resources = resourcesFromRecords(this.records);
    this.logicalTime = latestRecordTimestamp(this.records);
  }

  private applyRecords(records: SettingsRecordMap): void {
    const resources = { ...this.resources };
    for (const [key, record] of Object.entries(records)) {
      const current = resources[key];
      const preservePendingClean =
        record.deleted &&
        current !== undefined &&
        !isDirty(current) &&
        current.editToken !== '' &&
        current.expectedRevision === record.expectedRevision &&
        sameSettingsJson(current.base, record.base);
      if (!preservePendingClean) resources[key] = stateFromRecord(record);
    }
    this.records = records;
    this.resources = resources;
    this.sequence = Math.max(this.sequence, highestRecordSequence(records));
    this.logicalTime = Math.max(this.logicalTime, latestRecordTimestamp(records));
  }
}

export const [getSettingsDraftRegistry, setSettingsDraftRegistry] =
  createContext<SettingsDraftRegistry>();

function writableBrowserStorage(): SettingsDraftStorage | null {
  const storage = browserStorage();
  return storage !== null && typeof storage.setItem === 'function' ? storage : null;
}

function createWriterId(): string {
  try {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      return crypto.randomUUID();
    }
  } catch {
    // A random suffix still keeps two tabs distinct when crypto is unavailable.
  }
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

function changedRecordCount(left: SettingsRecordMap, right: SettingsRecordMap): number {
  let changed = 0;
  for (const key of new Set([...Object.keys(left), ...Object.keys(right)])) {
    const before = left[key] === undefined ? {} : { [key]: left[key] };
    const after = right[key] === undefined ? {} : { [key]: right[key] };
    if (!sameRecordMaps(before, after)) changed += 1;
  }
  return changed;
}

function hasProjection(
  projection: Readonly<Record<string, SettingsJson>>,
  ids: readonly string[],
): boolean {
  return ids.every((id) => Object.prototype.hasOwnProperty.call(projection, id));
}

function inScope(state: ResourceState, scope: SettingsScope | undefined): boolean {
  return scope === undefined || sameSettingsScope(settingsScopeOf(state.resource), scope);
}

function minimum(values: (number | null)[]): number | null {
  const present = values.filter((value): value is number => value !== null);
  return present.length === 0 ? null : Math.min(...present);
}

function maximum(values: (number | null)[]): number | null {
  const present = values.filter((value): value is number => value !== null);
  return present.length === 0 ? null : Math.max(...present);
}

function cloneScope(scope: SettingsScope): SettingsScope {
  return { ...scope };
}

function validRevision(value: number): boolean {
  return Number.isSafeInteger(value) && value >= 0;
}

function assertRevision(value: number): void {
  if (!validRevision(value)) {
    throw new TypeError('settings revisions must be non-negative safe integers');
  }
}
