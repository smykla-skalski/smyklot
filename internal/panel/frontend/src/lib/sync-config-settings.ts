import { formatJson, parseJson, type JsonValue } from './merge';
import type {
  InstallationSyncConfigSettingsInput,
  InstallationSyncConfigSettingsState,
  SyncConfig,
  SyncKind,
  SyncLabel,
} from './types';
import { SYNC_KINDS } from './types';
import type { SettingsCommittedResource, SettingsDraftRegistry } from './settings-drafts.svelte';
import type { SettingsJson, SettingsLocation, SettingsResource } from './settings-draft-storage';

const LABEL_KEYS = ['kind', 'enabled', 'labels', 'allow_removal', 'excludes'] as const;
const DOCUMENT_KEYS = ['kind', 'enabled', 'document_text'] as const;
const syncKinds = new Set<string>(SYNC_KINDS);

type SyncLabelValue = Record<string, SettingsJson> & {
  name: string;
  color: string;
  description?: string;
};

export type SyncLabelsEditorEnvelope = Record<string, SettingsJson> & {
  kind: 'labels';
  enabled: boolean;
  labels: SyncLabelValue[];
  allow_removal: boolean;
  excludes: string[];
};

export type SyncDocumentEditorEnvelope = Record<string, SettingsJson> & {
  kind: Exclude<SyncKind, 'labels'>;
  enabled: boolean;
  /** Canonical JSON text keeps raw number literals safe across browser restarts. */
  document_text: string;
};

export type SyncConfigEditorEnvelope = SyncLabelsEditorEnvelope | SyncDocumentEditorEnvelope;

export type SyncConfigControlId =
  | `sync.${SyncKind}.enabled`
  | 'sync.labels.labels'
  | 'sync.labels.allow_removal'
  | 'sync.labels.excludes'
  | `sync.${Exclude<SyncKind, 'labels'>}.document`;

export interface SyncConfigControlDefinition {
  id: SyncConfigControlId;
  location: SettingsLocation;
}

export type SyncConfigSerializationResult =
  { ok: true; input: InstallationSyncConfigSettingsInput } | { ok: false; problem: string };

export function syncConfigResource(targetId: string, kind: SyncKind): SettingsResource {
  return { type: 'sync-config', targetId, kind };
}

export function syncConfigControls(kind: SyncKind): readonly SyncConfigControlDefinition[] {
  const base: SyncConfigControlDefinition = {
    id: `sync.${kind}.enabled`,
    location: { section: 'sync', path: [kind, 'enabled'] },
  };
  if (kind === 'labels') {
    return [
      base,
      { id: 'sync.labels.labels', location: { section: 'sync', path: ['labels', 'labels'] } },
      {
        id: 'sync.labels.allow_removal',
        location: { section: 'sync', path: ['labels', 'allow_removal'] },
      },
      {
        id: 'sync.labels.excludes',
        location: { section: 'sync', path: ['labels', 'excludes'] },
      },
    ];
  }
  return [
    base,
    {
      id: `sync.${kind}.document`,
      location: { section: 'sync', path: [kind, 'document'] },
    },
  ];
}

export function buildSyncConfigEditorEnvelope(config: SyncConfig): SyncConfigEditorEnvelope {
  const kind = requireEditableConfig(config);
  if (kind === 'labels') {
    return {
      kind,
      enabled: config.enabled,
      labels: config.labels.map(cloneLabel),
      allow_removal: config.allow_removal,
      excludes: [...config.excludes],
    };
  }
  return {
    kind,
    enabled: config.enabled,
    document_text: formatDocument(config.document),
  };
}

export function parseSyncConfigEditorEnvelope(
  value: unknown,
  expectedKind?: SyncKind,
): SyncConfigEditorEnvelope | null {
  if (!isRecord(value) || !isSyncKind(value.kind)) return null;
  if (expectedKind !== undefined && value.kind !== expectedKind) return null;
  if (typeof value.enabled !== 'boolean') return null;
  if (value.kind === 'labels') {
    if (!hasExactKeys(value, LABEL_KEYS)) return null;
    if (!Array.isArray(value.labels) || !isStringArray(value.excludes)) return null;
    if (typeof value.allow_removal !== 'boolean') return null;
    const labels = value.labels.map(parseLabel);
    if (labels.some((label) => label === null)) return null;
    return {
      kind: value.kind,
      enabled: value.enabled,
      labels: labels as SyncLabelValue[],
      allow_removal: value.allow_removal,
      excludes: [...value.excludes],
    };
  }
  if (!hasExactKeys(value, DOCUMENT_KEYS) || typeof value.document_text !== 'string') return null;
  return { kind: value.kind, enabled: value.enabled, document_text: value.document_text };
}

export function adoptSyncConfigSettings(
  registry: SettingsDraftRegistry,
  targetId: string,
  config: SyncConfig,
): boolean {
  const envelope = buildSyncConfigEditorEnvelope(config);
  return registry.adoptBase(syncConfigResource(targetId, envelope.kind), config.revision, envelope);
}

export function syncConfigDraftEnvelope(
  registry: SettingsDraftRegistry,
  targetId: string,
  config: SyncConfig,
): SyncConfigEditorEnvelope {
  const kind = requireEditableConfig(config);
  const stored = registry.value(syncConfigResource(targetId, kind));
  if (stored === null) return buildSyncConfigEditorEnvelope(config);
  const envelope = parseSyncConfigEditorEnvelope(stored, kind);
  if (envelope === null) throw new TypeError('stored Sync configuration draft is invalid');
  return envelope;
}

export function stageSyncConfigControl(
  registry: SettingsDraftRegistry,
  targetId: string,
  config: SyncConfig,
  nextValue: SyncConfigEditorEnvelope,
  controlId: SyncConfigControlId,
): boolean {
  const kind = requireEditableConfig(config);
  const definition = syncConfigControls(kind).find(({ id }) => id === controlId);
  const next = parseSyncConfigEditorEnvelope(nextValue, kind);
  if (definition === undefined || next === null) return false;
  const resource = syncConfigResource(targetId, kind);
  const snapshot = registry.resource(resource);
  const base = parseSyncConfigEditorEnvelope(
    snapshot?.base ?? buildSyncConfigEditorEnvelope(config),
    kind,
  );
  if (base === null) return false;
  const saved = syncConfigSavedControls(base);
  const current = syncConfigSavedControls(next);
  return registry.stage(resource, next, {
    id: controlId,
    location: definition.location,
    saved: saved[controlId]!,
    value: current[controlId]!,
  });
}

export function syncConfigSavedControls(
  envelope: SyncConfigEditorEnvelope,
): Readonly<Record<string, SettingsJson>> {
  if (envelope.kind === 'labels') {
    return {
      'sync.labels.enabled': envelope.enabled,
      'sync.labels.labels': envelope.labels.map((label) => ({ ...label })),
      'sync.labels.allow_removal': envelope.allow_removal,
      'sync.labels.excludes': [...envelope.excludes],
    };
  }
  return {
    [`sync.${envelope.kind}.enabled`]: envelope.enabled,
    [`sync.${envelope.kind}.document`]: envelope.document_text,
  };
}

export function syncConfigForEditor(
  config: SyncConfig,
  envelope: SyncConfigEditorEnvelope,
): SyncConfig {
  const parsed = parseSyncConfigEditorEnvelope(envelope, requireEditableConfig(config));
  if (parsed === null) throw new TypeError('Sync configuration draft is invalid');
  if (parsed.kind === 'labels') {
    return {
      ...config,
      enabled: parsed.enabled,
      labels: parsed.labels.map(cloneLabel),
      allow_removal: parsed.allow_removal,
      excludes: [...parsed.excludes],
    };
  }
  const document = parseDocument(parsed.document_text);
  if (document === null) throw new TypeError('Sync configuration document is invalid');
  return { ...config, enabled: parsed.enabled, document };
}

export function syncConfigBatchInput(
  expectedRevision: number,
  envelope: SyncConfigEditorEnvelope,
): SyncConfigSerializationResult {
  if (!Number.isSafeInteger(expectedRevision) || expectedRevision < 0) {
    return { ok: false, problem: 'Sync configuration revision is invalid' };
  }
  const parsed = parseSyncConfigEditorEnvelope(envelope);
  if (parsed === null) return { ok: false, problem: 'Sync configuration draft is invalid' };
  if (parsed.kind === 'labels') {
    return {
      ok: true,
      input: {
        kind: parsed.kind,
        enabled: parsed.enabled,
        labels: parsed.labels.map(cloneLabel),
        allow_removal: parsed.allow_removal,
        excludes: [...parsed.excludes],
        expected_revision: expectedRevision,
      },
    };
  }
  const document = parseDocument(parsed.document_text);
  if (document === null) {
    return { ok: false, problem: `${kindWord(parsed.kind)} configuration is not a JSON object` };
  }
  return {
    ok: true,
    input: {
      kind: parsed.kind,
      enabled: parsed.enabled,
      document,
      expected_revision: expectedRevision,
    },
  };
}

export function syncConfigCommittedResource(
  state: InstallationSyncConfigSettingsState,
): SettingsCommittedResource {
  if (!isSyncKind(state.kind)) throw new TypeError('saved Sync configuration kind is unknown');
  if (!isJsonRecord(state.document)) {
    throw new TypeError('saved Sync configuration document is not an object');
  }
  const labels = state.kind === 'labels' ? parseLabelDocument(state.document) : null;
  if (state.kind === 'labels' && labels === null) {
    throw new TypeError('saved labels configuration is invalid');
  }
  const envelope = buildSyncConfigEditorEnvelope({
    kind: state.kind,
    enabled: state.enabled,
    labels: labels?.labels ?? [],
    allow_removal: labels?.allowRemoval ?? false,
    excludes: labels?.excludes ?? [],
    revision: state.revision,
    updated_by: '',
    updated_at: '',
    digest: '',
    document: state.document,
    unreadable: false,
    unavailable: '',
  });
  return {
    resource: syncConfigResource(state.target_id, state.kind),
    revision: state.revision,
    value: envelope,
    savedControls: syncConfigSavedControls(envelope),
  };
}

function requireEditableConfig(config: SyncConfig): SyncKind {
  if (!isSyncKind(config.kind)) throw new TypeError('Sync configuration kind is unknown');
  if (config.unreadable) throw new TypeError('unreadable Sync configuration cannot be edited');
  if (!Number.isSafeInteger(config.revision) || config.revision < 0) {
    throw new TypeError('Sync configuration revision is invalid');
  }
  return config.kind;
}

function formatDocument(value: Record<string, unknown>): string {
  try {
    return formatJson(value as JsonValue).trimEnd();
  } catch {
    throw new TypeError('Sync configuration document is not finite JSON');
  }
}

function parseDocument(text: string): Record<string, unknown> | null {
  const parsed = parseJson(text);
  return isJsonRecord(parsed) ? parsed : null;
}

function parseLabel(value: unknown): SyncLabelValue | null {
  if (!isRecord(value)) return null;
  const keys =
    value.description === undefined ? ['name', 'color'] : ['name', 'color', 'description'];
  if (!hasExactKeys(value, keys)) return null;
  if (typeof value.name !== 'string' || typeof value.color !== 'string') return null;
  if (value.description !== undefined && typeof value.description !== 'string') return null;
  return {
    name: value.name,
    color: value.color,
    ...(value.description === undefined ? {} : { description: value.description }),
  };
}

function cloneLabel(label: SyncLabel): SyncLabelValue {
  const parsed = parseLabel(label);
  if (parsed === null) throw new TypeError('Sync label is invalid');
  return parsed;
}

function parseLabelDocument(document: Record<string, unknown>): {
  labels: SyncLabelValue[];
  allowRemoval: boolean;
  excludes: string[];
} | null {
  const keys = Object.keys(document);
  if (keys.some((key) => !['labels', 'allow_removal', 'excludes'].includes(key))) return null;
  if (!Array.isArray(document.labels) || typeof document.allow_removal !== 'boolean') return null;
  if (document.excludes !== undefined && !isStringArray(document.excludes)) return null;
  const labels = document.labels.map(parseLabel);
  if (labels.some((label) => label === null)) return null;
  return {
    labels: labels as SyncLabelValue[],
    allowRemoval: document.allow_removal,
    excludes: document.excludes === undefined ? [] : [...document.excludes],
  };
}

function kindWord(kind: Exclude<SyncKind, 'labels'>): string {
  return kind.charAt(0).toUpperCase() + kind.slice(1);
}

function isSyncKind(value: unknown): value is SyncKind {
  return typeof value === 'string' && syncKinds.has(value);
}

function isJsonRecord(value: unknown): value is Record<string, unknown> {
  return (
    typeof value === 'object' &&
    value !== null &&
    !Array.isArray(value) &&
    !(typeof JSON.isRawJSON === 'function' && JSON.isRawJSON(value))
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((entry) => typeof entry === 'string');
}

function hasExactKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}
