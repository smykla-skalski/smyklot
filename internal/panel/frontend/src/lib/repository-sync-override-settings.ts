import { composeFile, formatJson, parseJson, validateSpec } from './merge';
import type { JsonValue, MergeSpec } from './merge';
import type {
  InstallationSyncOverrideSettingsInput,
  InstallationSyncOverrideSettingsState,
  SyncOverride,
} from './types';
import type { SettingsCommittedResource, SettingsDraftRegistry } from './settings-drafts.svelte';
import {
  cloneSettingsJson,
  type SettingsJson,
  type SettingsLocation,
  type SettingsResource,
} from './settings-draft-storage';

const ENVELOPE_KEYS = ['enabled', 'document', 'override_texts'] as const;
const DOCUMENT_KEYS = ['merges', 'excludes'] as const;
const MERGE_KEYS = ['path', 'strategy', 'overrides', 'arrays', 'deduplicate', 'sections'] as const;
const ARRAY_RULE_KEYS = ['path', 'strategy'] as const;
const SECTION_KEYS = ['action', 'heading', 'occurrence', 'content', 'patches'] as const;
const PATCH_KEYS = ['find', 'replace'] as const;
const STRUCTURED_PATH = /\.(?:json|ya?ml)$/i;
const MARKDOWN_PATH = /\.(?:md|markdown)$/i;
const STRUCTURED_STRATEGIES = new Set(['', 'deep-merge', 'shallow-merge']);
const ARRAY_STRATEGIES = new Set(['replace', 'append', 'prepend']);
const SECTION_ACTIONS = new Set([
  'before',
  'after',
  'replace',
  'delete',
  'patch',
  'append',
  'prepend',
]);

export type SyncOverrideSettingsDocument = Record<string, SettingsJson>;

/**
 * The controlled editor state persisted by the draft registry.
 *
 * Text is separate from the parsed document so an unfinished JSON edit, and
 * literal number spellings such as `1.50`, survive navigation and restart.
 */
export type SyncOverrideEditorEnvelope = Record<string, SettingsJson> & {
  enabled: boolean | null;
  document: SyncOverrideSettingsDocument;
  override_texts: string[];
};

export type SyncOverrideControlId =
  `repositories.${string}.sync.files.enabled` | `repositories.${string}.sync.files.document`;

export interface SyncOverrideControlDefinition {
  id: SyncOverrideControlId;
  location: SettingsLocation;
}

export type SyncOverrideSerializationResult =
  { ok: true; input: InstallationSyncOverrideSettingsInput } | { ok: false; problem: string };

interface DocumentSerializationSuccess {
  ok: true;
  document: Record<string, unknown>;
}

type DocumentSerializationResult = DocumentSerializationSuccess | { ok: false; problem: string };

/** The two stable controls exposed by a repository's file-sync pane. */
export function syncOverrideControls(
  repositoryId: string,
): readonly SyncOverrideControlDefinition[] {
  const prefix = `repositories.${repositoryId}.sync.files` as const;
  return [
    {
      id: `${prefix}.enabled`,
      location: { section: 'repositories', path: [repositoryId, 'sync', 'files', 'enabled'] },
    },
    {
      id: `${prefix}.document`,
      location: { section: 'repositories', path: [repositoryId, 'sync', 'files', 'document'] },
    },
  ];
}

export function syncOverrideResource(targetId: string, repositoryId: string): SettingsResource {
  return { type: 'sync-override', targetId, repositoryId, kind: 'files' };
}

/** Build a restart-safe base from a readable canonical response, including revision zero. */
export function buildSyncOverrideEditorEnvelope(stored: SyncOverride): SyncOverrideEditorEnvelope {
  assertReadableFilesOverride(stored);
  const document = plainSettingsRecord(stored.document);
  if (document === null) throw new TypeError('sync override document is not finite JSON');

  return {
    enabled: stored.enabled,
    document,
    override_texts: overrideTexts(stored.document),
  };
}

/** Parse persisted editor state without interpreting or discarding unknown document keys. */
export function parseSyncOverrideEditorEnvelope(value: unknown): SyncOverrideEditorEnvelope | null {
  if (!isRecord(value) || !hasExactKeys(value, ENVELOPE_KEYS)) return null;
  if (value.enabled !== null && typeof value.enabled !== 'boolean') return null;
  if (!isRecord(value.document) || !isStringArray(value.override_texts)) return null;

  try {
    const document = cloneSettingsJson(value.document as SettingsJson);
    if (!isRecord(document)) return null;
    return {
      enabled: value.enabled,
      document: document as SyncOverrideSettingsDocument,
      override_texts: [...value.override_texts],
    };
  } catch {
    return null;
  }
}

export function cloneSyncOverrideEditorEnvelope(
  envelope: SyncOverrideEditorEnvelope,
): SyncOverrideEditorEnvelope {
  const parsed = parseSyncOverrideEditorEnvelope(envelope);
  if (parsed === null) throw new TypeError('sync override editor state is invalid');
  return parsed;
}

/** Adopt the canonical response while preserving a locally edited envelope. */
export function adoptSyncOverrideSettings(
  registry: SettingsDraftRegistry,
  targetId: string,
  repositoryId: string,
  stored: SyncOverride,
): boolean {
  assertReadableFilesOverride(stored);
  return registry.adoptBase(
    syncOverrideResource(targetId, repositoryId),
    stored.revision,
    buildSyncOverrideEditorEnvelope(stored),
  );
}

/** Return the draft envelope, or a fresh base when the resource has no local edit. */
export function syncOverrideDraftEnvelope(
  registry: SettingsDraftRegistry,
  targetId: string,
  repositoryId: string,
  stored: SyncOverride,
): SyncOverrideEditorEnvelope {
  assertReadableFilesOverride(stored);
  const value = registry.value(syncOverrideResource(targetId, repositoryId));
  if (value === null) return buildSyncOverrideEditorEnvelope(stored);
  const parsed = parseSyncOverrideEditorEnvelope(value);
  if (parsed === null) throw new TypeError('stored sync override draft is invalid');
  return parsed;
}

/** Stage either the enablement or the complete controlled document editor. */
export function stageSyncOverrideControl(
  registry: SettingsDraftRegistry,
  targetId: string,
  repositoryId: string,
  stored: SyncOverride,
  nextValue: SyncOverrideEditorEnvelope,
  controlId: SyncOverrideControlId,
): boolean {
  assertReadableFilesOverride(stored);
  const definition = syncOverrideControls(repositoryId).find(({ id }) => id === controlId);
  if (definition === undefined) return false;
  const next = parseSyncOverrideEditorEnvelope(nextValue);
  if (next === null) return false;
  const resource = syncOverrideResource(targetId, repositoryId);
  const snapshot = registry.resource(resource);
  const base = parseSyncOverrideEditorEnvelope(
    snapshot?.base ?? buildSyncOverrideEditorEnvelope(stored),
  );
  if (base === null) return false;
  const saved = syncOverrideSavedControls(repositoryId, base);
  const current = syncOverrideSavedControls(repositoryId, next);

  return registry.stage(resource, next, {
    id: controlId,
    location: definition.location,
    saved: saved[controlId]!,
    value: current[controlId]!,
  });
}

/** Saved projections include raw text so malformed edits remain visibly dirty. */
export function syncOverrideSavedControls(
  repositoryId: string,
  envelope: SyncOverrideEditorEnvelope,
): Readonly<Record<SyncOverrideControlId, SettingsJson>> {
  const prefix = `repositories.${repositoryId}.sync.files`;
  return {
    [`${prefix}.enabled`]: envelope.enabled,
    [`${prefix}.document`]: {
      document: cloneSettingsJson(envelope.document),
      override_texts: [...envelope.override_texts],
    },
  } as Record<SyncOverrideControlId, SettingsJson>;
}

/** Validate and serialize the controlled state for one atomic installation save. */
export function syncOverrideBatchInput(
  repositoryId: string,
  expectedRevision: number,
  envelope: SyncOverrideEditorEnvelope,
): SyncOverrideSerializationResult {
  if (repositoryId.length === 0) return { ok: false, problem: 'Repository ID is missing' };
  if (!Number.isSafeInteger(expectedRevision) || expectedRevision < 0) {
    return { ok: false, problem: 'Repository file sync revision is invalid' };
  }
  const parsed = parseSyncOverrideEditorEnvelope(envelope);
  if (parsed === null) return { ok: false, problem: 'Repository file sync draft is invalid' };
  const serialized = serializeSyncOverrideDocument(parsed);
  if (!serialized.ok) return serialized;

  return {
    ok: true,
    input: {
      repository_id: repositoryId,
      kind: 'files',
      enabled: parsed.enabled,
      document: serialized.document,
      expected_revision: expectedRevision,
    },
  };
}

/** Validate the document and its raw merge texts without mutating the envelope. */
export function serializeSyncOverrideDocument(
  envelope: SyncOverrideEditorEnvelope,
): DocumentSerializationResult {
  const document = plainSettingsRecord(envelope.document);
  if (document === null) return { ok: false, problem: 'Repository file sync document is invalid' };
  const unknown = firstUnknownKey(document, DOCUMENT_KEYS);
  if (unknown !== null) {
    return { ok: false, problem: `This version cannot safely save document key ${unknown}` };
  }

  const excludes = serializeExcludes(document.excludes);
  if (!excludes.ok) return excludes;
  const merges = document.merges;
  if (merges !== undefined && !Array.isArray(merges)) {
    return { ok: false, problem: 'File adjustments must be a list' };
  }
  const rows = merges ?? [];
  if (rows.length !== envelope.override_texts.length) {
    return { ok: false, problem: 'File adjustment text no longer matches its row' };
  }

  const serialized: Record<string, unknown> = {};
  if (excludes.value.length > 0) serialized.excludes = excludes.value;
  const savedMerges: Record<string, unknown>[] = [];
  const seen = new Set<string>();
  for (const [index, row] of rows.entries()) {
    const saved = serializeMerge(row, envelope.override_texts[index]!, index);
    if (!saved.ok) return saved;
    const folded = saved.path.toLocaleLowerCase();
    if (seen.has(folded)) {
      return { ok: false, problem: `${saved.path} is adjusted twice` };
    }
    seen.add(folded);
    savedMerges.push(saved.merge);
  }
  if (savedMerges.length > 0) serialized.merges = savedMerges;
  return { ok: true, document: serialized };
}

/** Convert a canonical compact batch response into a registry commit result. */
export function syncOverrideCommittedResource(
  state: InstallationSyncOverrideSettingsState,
): SettingsCommittedResource {
  if (state.kind !== 'files') throw new TypeError('only file sync overrides have an editor');
  const envelope = buildSyncOverrideEditorEnvelope({
    kind: state.kind,
    enabled: state.enabled,
    document: state.document,
    revision: state.revision,
    unreadable: false,
  });
  return {
    resource: syncOverrideResource(state.target_id, state.repository_id),
    revision: state.revision,
    value: envelope,
    savedControls: syncOverrideSavedControls(state.repository_id, envelope),
  };
}

type MergeSerialization =
  { ok: true; path: string; merge: Record<string, unknown> } | { ok: false; problem: string };

function serializeMerge(row: SettingsJson, text: string, index: number): MergeSerialization {
  const named = `File adjustment ${index + 1}`;
  if (!isRecord(row)) return { ok: false, problem: `${named} is not an object` };
  const unknown = firstUnknownKey(row, MERGE_KEYS);
  if (unknown !== null) {
    return {
      ok: false,
      problem: `${named} has a key this version cannot safely save: ${unknown}`,
    };
  }
  if (typeof row.path !== 'string' || row.path.length === 0) {
    return { ok: false, problem: `${named} names no file` };
  }
  if (row.strategy !== undefined && typeof row.strategy !== 'string') {
    return { ok: false, problem: `${row.path} has an invalid merge strategy` };
  }
  const merge = cloneUnknownRecord(row);
  if (MARKDOWN_PATH.test(row.path)) {
    delete merge.overrides;
    delete merge.arrays;
    delete merge.deduplicate;
    const problem = validateMarkdownMerge(merge, row.path);
    return problem === null ? { ok: true, path: row.path, merge } : { ok: false, problem };
  }
  if (!STRUCTURED_PATH.test(row.path)) {
    return {
      ok: false,
      problem: `${row.path} has no extension this can merge; JSON, YAML and Markdown can`,
    };
  }
  const overrides = parseOverrideText(text, row.path);
  if (!overrides.ok) return overrides;
  if (overrides.value === null || Object.keys(overrides.value).length === 0) {
    delete merge.overrides;
  } else {
    merge.overrides = overrides.value;
  }
  delete merge.sections;
  const shapeProblem = validateStructuredShape(merge, row.path);
  if (shapeProblem !== null) return { ok: false, problem: shapeProblem };
  const validationPath = row.path.toLowerCase().endsWith('.json') ? row.path : 'override.json';
  const problem = validateSpec(validationPath, merge as MergeSpec);
  if (problem !== undefined) return { ok: false, problem: `${row.path}: ${problem}` };
  const composed = composeFile(validationPath, {}, merge as MergeSpec);
  return composed.ok
    ? { ok: true, path: row.path, merge }
    : { ok: false, problem: `${row.path}: ${composed.reason}` };
}

type OverrideTextResult =
  { ok: true; value: Record<string, JsonValue> | null } | { ok: false; problem: string };

function parseOverrideText(text: string, path: string): OverrideTextResult {
  if (text.trim() === '') return { ok: true, value: null };
  const parsed = parseJson(text);
  if (parsed === null) return { ok: true, value: null };
  if (parsed === undefined || !isJsonRecord(parsed)) {
    return { ok: false, problem: `What ${path} sets is not a JSON object` };
  }
  return { ok: true, value: parsed };
}

function validateStructuredShape(merge: Record<string, unknown>, path: string): string | null {
  const strategy = merge.strategy ?? '';
  if (typeof strategy !== 'string' || !STRUCTURED_STRATEGIES.has(strategy)) {
    return `${path} has an unknown structured merge strategy`;
  }
  if (merge.deduplicate !== undefined && typeof merge.deduplicate !== 'boolean') {
    return `${path} has an invalid deduplication setting`;
  }
  if (merge.arrays !== undefined && !Array.isArray(merge.arrays)) {
    return `${path} list rules must be a list`;
  }
  for (const [index, rule] of (merge.arrays ?? []).entries()) {
    if (!isRecord(rule) || firstUnknownKey(rule, ARRAY_RULE_KEYS) !== null) {
      return `List rule ${index + 1} of ${path} is invalid`;
    }
    if (typeof rule.path !== 'string' || typeof rule.strategy !== 'string') {
      return `List rule ${index + 1} of ${path} is incomplete`;
    }
    if (!ARRAY_STRATEGIES.has(rule.strategy)) {
      return `List rule ${index + 1} of ${path} has an unknown strategy`;
    }
  }
  return null;
}

function validateMarkdownMerge(merge: Record<string, unknown>, path: string): string | null {
  const strategy = merge.strategy ?? '';
  if (strategy !== '' && strategy !== 'markdown') {
    return `${path} is Markdown, which has no keys to merge; use "markdown"`;
  }
  if (!Array.isArray(merge.sections) || merge.sections.length === 0) {
    return `${path} is edited by its headings, and no section says how`;
  }
  for (const [index, section] of merge.sections.entries()) {
    const problem = validateMarkdownSection(section, index, path);
    if (problem !== null) return problem;
  }
  return null;
}

function validateMarkdownSection(section: unknown, index: number, path: string): string | null {
  const named = `Section ${index + 1} of ${path}`;
  if (!isRecord(section) || firstUnknownKey(section, SECTION_KEYS) !== null) {
    return `${named} is invalid`;
  }
  if (typeof section.action !== 'string' || !SECTION_ACTIONS.has(section.action)) {
    return `${named} has an unknown action`;
  }
  if (!optionalString(section.heading) || !optionalString(section.content)) {
    return `${named} has invalid text`;
  }
  if (
    section.occurrence !== undefined &&
    (!Number.isSafeInteger(section.occurrence) || Number(section.occurrence) < 0)
  ) {
    return `${named} has an invalid occurrence`;
  }
  if (section.patches !== undefined && !Array.isArray(section.patches)) {
    return `${named} substitutions must be a list`;
  }
  const action = section.action;
  if (action === 'append' || action === 'prepend') {
    if ((section.heading ?? '') !== '' || Number(section.occurrence ?? 0) !== 0) {
      return `${named} addresses no heading`;
    }
    return nonEmptyText(section.content) ? null : `${named} needs the content it writes`;
  }
  if (!isMarkdownHeading(section.heading)) {
    return `${named} needs the heading it addresses, written with its # marks`;
  }
  if (action === 'delete') return null;
  if (action === 'patch') return validatePatches(section.patches, named);
  return nonEmptyText(section.content) ? null : `${named} needs the content it writes`;
}

function validatePatches(value: unknown, named: string): string | null {
  if (!Array.isArray(value) || value.length === 0) return `${named} substitutes nothing`;
  for (const [index, patch] of value.entries()) {
    if (!isRecord(patch) || !hasExactKeys(patch, PATCH_KEYS)) {
      return `Substitution ${index + 1} of ${named} is invalid`;
    }
    if (typeof patch.find !== 'string' || typeof patch.replace !== 'string') {
      return `Substitution ${index + 1} of ${named} has invalid text`;
    }
    if (patch.find === '') return `${named} has a substitution that finds nothing`;
  }
  return null;
}

type ExcludesResult = { ok: true; value: string[] } | { ok: false; problem: string };

function serializeExcludes(value: SettingsJson | undefined): ExcludesResult {
  if (value === undefined) return { ok: true, value: [] };
  if (!isStringArray(value)) return { ok: false, problem: 'File exclusions must be a list' };
  const empty = value.findIndex((pattern) => pattern.trim() === '');
  if (empty >= 0) return { ok: false, problem: `File exclusion ${empty + 1} is empty` };
  return { ok: true, value: [...value] };
}

function overrideTexts(document: Record<string, unknown>): string[] {
  if (!Array.isArray(document.merges)) return [];
  return document.merges.map((row) => {
    if (!isRecord(row) || !Object.hasOwn(row, 'overrides')) return '';
    try {
      return formatJson(row.overrides as JsonValue).trimEnd();
    } catch {
      throw new TypeError('sync override contains an unreadable merge value');
    }
  });
}

function plainSettingsRecord(value: unknown): SyncOverrideSettingsDocument | null {
  if (!isRecord(value) || !validJsonWithRawNumbers(value)) return null;
  try {
    const serialized = JSON.stringify(value);
    if (serialized === undefined) return null;
    const parsed: unknown = JSON.parse(serialized);
    if (!isRecord(parsed)) return null;
    return cloneSettingsJson(parsed as SettingsJson) as SyncOverrideSettingsDocument;
  } catch {
    return null;
  }
}

function validJsonWithRawNumbers(value: unknown, ancestors = new WeakSet<object>()): boolean {
  if (value === null || typeof value === 'string' || typeof value === 'boolean') return true;
  if (typeof value === 'number') return Number.isFinite(value);
  if (typeof value !== 'object' || ancestors.has(value)) return false;
  if (isRawJson(value)) return true;
  if (Array.isArray(value)) {
    ancestors.add(value);
    const valid = value.every((entry) => validJsonWithRawNumbers(entry, ancestors));
    ancestors.delete(value);
    return valid;
  }
  if (Object.getPrototypeOf(value) !== Object.prototype && Object.getPrototypeOf(value) !== null) {
    return false;
  }
  ancestors.add(value);
  const valid = Object.values(value).every((entry) => validJsonWithRawNumbers(entry, ancestors));
  ancestors.delete(value);
  return valid;
}

function cloneUnknownRecord(value: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(value).map(([key, entry]) => [key, cloneUnknown(entry)]),
  );
}

function cloneUnknown(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(cloneUnknown);
  if (isRecord(value)) {
    return Object.fromEntries(
      Object.entries(value).map(([key, entry]) => [key, cloneUnknown(entry)]),
    );
  }
  return value;
}

function assertReadableFilesOverride(stored: SyncOverride): void {
  if (stored.unreadable) {
    throw new TypeError('unreadable sync overrides cannot be edited or replaced');
  }
  if (stored.kind !== 'files') throw new TypeError('only file sync overrides have an editor');
  if (!Number.isSafeInteger(stored.revision) || stored.revision < 0) {
    throw new TypeError('sync override revision is invalid');
  }
}

function isJsonRecord(value: JsonValue): value is Record<string, JsonValue> {
  return typeof value === 'object' && value !== null && !Array.isArray(value) && !isRawJson(value);
}

function isRawJson(value: unknown): boolean {
  return typeof JSON.isRawJSON === 'function' && JSON.isRawJSON(value);
}

function isMarkdownHeading(value: unknown): boolean {
  return typeof value === 'string' && /^ {0,3}#{1,6}(?:[ \t]|$)/.test(value);
}

function nonEmptyText(value: unknown): boolean {
  return typeof value === 'string' && value.trim() !== '';
}

function optionalString(value: unknown): boolean {
  return value === undefined || typeof value === 'string';
}

function firstUnknownKey(value: Record<string, unknown>, keys: readonly string[]): string | null {
  return Object.keys(value).find((key) => !keys.includes(key)) ?? null;
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((entry) => typeof entry === 'string');
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value) && !isRawJson(value);
}

function hasExactKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}
