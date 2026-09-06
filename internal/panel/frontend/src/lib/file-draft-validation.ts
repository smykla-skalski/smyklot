import { getContext, setContext } from 'svelte';
import { fileFormat } from './file-format';
import { parseFormattingPatch } from './formatting';
import { formatJson, parseJson, type JsonValue } from './merge';
import {
  parseSyncOverrideEditorEnvelope,
  serializeSyncOverrideDocument,
} from './repository-sync-override-settings';
import { cloneSettingsJson, sameSettingsJson, type SettingsJson } from './settings-draft-storage';
import type {
  SettingsDraftRegistry,
  SettingsDraftResourceSnapshot,
  SettingsScope,
} from './settings-drafts.svelte';
import { parseSyncConfigEditorEnvelope, syncConfigResource } from './sync-config-settings';
import type { SyncConfig, SyncFile, SyncFileMerge } from './types';
import type { SyncFileRenderInput, SyncFileRenderResponse } from './sync-file-render.generated';

const contextKey = Symbol('file-draft-validation');
export const setFileDraftValidation = (validation: FileDraftValidation): void => {
  setContext(contextKey, validation);
};
export const getFileDraftValidation = (): FileDraftValidation | undefined => getContext(contextKey);

interface Snapshot {
  accountId: string | null;
  resources: SettingsDraftResourceSnapshot[];
  configurations: Map<string, SettingsJson | null>;
}

/** Read only registry dependencies here, so reporting a result cannot retrigger its own job. */
export function fileDraftValidationSnapshot(registry: SettingsDraftRegistry): Snapshot {
  const resources = registry
    .dirtyResources()
    .filter(
      ({ resource }) =>
        (resource.type === 'sync-config' || resource.type === 'sync-override') &&
        resource.kind === 'files',
    );
  const configurations = new Map<string, SettingsJson | null>();
  for (const { resource } of resources) {
    if (resource.type === 'runtime') continue;
    configurations.set(
      resource.targetId,
      registry.value(syncConfigResource(resource.targetId, 'files')),
    );
  }
  return { accountId: registry.accountId, resources, configurations };
}

interface Check {
  scope: SettingsScope;
  control: string;
  targetId: string;
  input?: SyncFileRenderInput;
  problem?: string;
}
interface Job {
  check: Check;
  signature: string;
  timer?: ReturnType<typeof setTimeout>;
  request?: Promise<SyncFileRenderResponse>;
}
type Loaded = { document: unknown } | { problem: string } | { pending: true };

/** Save validation belongs to the application, while mounted editors own preview and Undo. */
export class FileDraftValidation {
  private snapshot: Snapshot = { accountId: null, resources: [], configurations: new Map() };
  private epoch = 0;
  private disposed = false;
  private readonly removeSaveValidator: () => void;
  private jobs = new Map<string, Job>();
  private loaded = new Map<string, Loaded>();
  private requests = new Map<string, Promise<SyncFileRenderResponse>>();

  constructor(
    private readonly registry: SettingsDraftRegistry,
    private readonly api: {
      renderSyncFile(targetId: string, input: SyncFileRenderInput): Promise<SyncFileRenderResponse>;
      fetchSyncConfig(targetId: string, kind: string): Promise<SyncConfig>;
    },
  ) {
    this.removeSaveValidator = registry.onBeforeSave(() =>
      this.update(fileDraftValidationSnapshot(registry)),
    );
  }

  update(snapshot: Snapshot): void {
    if (this.disposed) return;
    if (snapshot.accountId !== this.snapshot.accountId) {
      this.clear();
      this.epoch += 1;
    }
    this.snapshot = snapshot;
    const checks = new Map<string, Check>();
    if (snapshot.accountId !== null) {
      for (const resource of snapshot.resources) this.plan(resource, checks);
    }
    for (const [key, job] of this.jobs) {
      if (checks.has(key)) continue;
      this.remove(job);
      this.jobs.delete(key);
    }
    for (const [key, check] of checks) {
      const signature = formatJson((check.input ?? check.problem ?? '') as JsonValue);
      if (this.jobs.get(key)?.signature === signature) continue;
      const previous = this.jobs.get(key);
      if (previous) this.remove(previous);
      const job: Job = { check, signature };
      this.jobs.set(key, job);
      this.registry.setValidationProblem(
        check.scope,
        check.control,
        check.problem ?? `${check.input!.path}: Checking file content`,
      );
      if (check.input)
        job.timer = setTimeout(() => {
          void this.render(check.targetId, check.input!).catch(() => {});
        }, 120);
    }
    const targets = new Set(snapshot.configurations.keys());
    for (const targetId of this.loaded.keys())
      if (!targets.has(targetId)) this.loaded.delete(targetId);
  }

  /** Share simultaneous requests; fresh previews also retry the current Save check. */
  render(targetId: string, input: SyncFileRenderInput): Promise<SyncFileRenderResponse> {
    this.update(fileDraftValidationSnapshot(this.registry));
    const epoch = this.epoch;
    const signature = formatJson(input as unknown as JsonValue);
    const key = formatJson([epoch, targetId, input] as JsonValue);
    let request = this.requests.get(key);
    if (!request) {
      request = this.api.renderSyncFile(targetId, input);
      this.requests.set(key, request);
      void request
        .finally(() => {
          if (this.requests.get(key) === request) this.requests.delete(key);
        })
        .catch(() => {});
    }
    for (const [jobKey, job] of this.jobs) {
      if (job.check.targetId !== targetId || job.signature !== signature || job.request === request)
        continue;
      clearTimeout(job.timer);
      job.request = request;
      const currentRequest = request;
      void request.then(
        (result) => {
          const message = result.diagnostics.map(({ message }) => message).join(' · ');
          this.settle(
            epoch,
            jobKey,
            job,
            currentRequest,
            result.valid
              ? null
              : `${input.path}: ${message || 'The file cannot be rendered safely'}`,
          );
        },
        (cause) =>
          this.settle(
            epoch,
            jobKey,
            job,
            currentRequest,
            `${input.path}: ${cause instanceof Error ? cause.message : String(cause)}`,
          ),
      );
    }
    return request;
  }

  dispose(): void {
    this.removeSaveValidator();
    this.disposed = true;
    this.epoch += 1;
    this.clear();
  }

  private clear(): void {
    for (const job of this.jobs.values()) this.remove(job);
    this.jobs.clear();
    this.loaded.clear();
    this.requests.clear();
  }

  private remove(job: Job): void {
    clearTimeout(job.timer);
    this.registry.setValidationProblem(job.check.scope, job.check.control, null);
  }

  private settle(
    epoch: number,
    key: string,
    job: Job,
    request: Promise<SyncFileRenderResponse>,
    problem: string | null,
  ): void {
    // A reply can precede the next layout effect. Compare live state before publishing it.
    this.update(fileDraftValidationSnapshot(this.registry));
    if (epoch !== this.epoch || this.jobs.get(key) !== job || job.request !== request) return;
    this.registry.setValidationProblem(job.check.scope, job.check.control, problem);
  }

  private plan(state: SettingsDraftResourceSnapshot, checks: Map<string, Check>): void {
    const resource = state.resource;
    if (resource.type !== 'sync-config' && resource.type !== 'sync-override') return;
    const targetId = resource.targetId;
    const repositoryId = resource.type === 'sync-override' ? resource.repositoryId : '';
    const scope = { type: 'workspace', targetId } as const;
    const add = (path: string, input?: SyncFileRenderInput, problem?: string) => {
      const control = `sync.files.validation:${encodeURIComponent(repositoryId)}:${encodeURIComponent(path)}`;
      checks.set(`${targetId}:${control}`, { scope, control, targetId, input, problem });
    };
    // Invalid raw envelopes already have a precise, persistent serializer error.
    const changes = resource.type === 'sync-override' ? adjustments(state.value) : null;
    if (resource.type === 'sync-override' && changes === null) return;
    const value = this.snapshot.configurations.get(targetId);
    const envelope =
      value === null || value === undefined ? null : parseSyncConfigEditorEnvelope(value, 'files');
    if (value != null && (envelope === null || envelope.kind === 'labels')) return;
    let document: unknown;
    if (envelope && envelope.kind !== 'labels') {
      try {
        document = parseJson(envelope.document_text);
      } catch {
        return;
      }
    } else {
      const loaded = this.loaded.get(targetId);
      if (!loaded) this.load(targetId);
      if (!loaded || 'pending' in loaded) {
        add('load', undefined, 'Loading shared files for validation');
        return;
      }
      if ('problem' in loaded) {
        add('load', undefined, loaded.problem);
        return;
      }
      document = loaded.document;
    }
    const files = fileMap(document);
    if (files === null) return;
    if (resource.type === 'sync-config') {
      const base = parseSyncConfigEditorEnvelope(state.base, 'files');
      let saved: Map<string, SyncFile> | null = null;
      try {
        saved = base && base.kind !== 'labels' ? fileMap(parseJson(base.document_text)) : null;
      } catch {
        /* malformed base has no trusted comparison */
      }
      for (const file of files.values()) {
        if (
          fileFormat(file.path) === null ||
          sameSettingsJson(
            file as unknown as SettingsJson,
            (saved?.get(file.path) as unknown as SettingsJson) ?? null,
          )
        )
          continue;
        add(file.path, templateInput(file));
      }
      return;
    }
    const saved = adjustments(state.base) ?? new Map();
    for (const path of new Set([...changes!.keys(), ...saved.keys()])) {
      const adjustment = changes!.get(path);
      if (
        sameSettingsJson(
          (adjustment as SettingsJson) ?? null,
          (saved.get(path) as SettingsJson) ?? null,
        )
      )
        continue;
      const file = files.get(path);
      if (!file) {
        if (adjustment) add(path, undefined, `${path}: The shared template is unavailable`);
        continue;
      }
      add(path, {
        ...templateInput(file),
        repository: {
          id: repositoryId,
          path_formatting: adjustment?.formatting ?? {},
          ...(adjustment?.merge === undefined ? {} : { merge: adjustment.merge }),
        },
      });
    }
  }

  private load(targetId: string): void {
    const epoch = this.epoch;
    const loading: Loaded = { pending: true };
    this.loaded.set(targetId, loading);
    void this.api
      .fetchSyncConfig(targetId, 'files')
      .then((config) => {
        if (config.unreadable || config.unavailable)
          throw new Error('Shared files are unavailable for validation');
        return { document: config.document };
      })
      .catch((cause) => ({ problem: cause instanceof Error ? cause.message : String(cause) }))
      .then((loaded) => {
        if (epoch !== this.epoch || this.loaded.get(targetId) !== loading) return;
        this.loaded.set(targetId, loaded);
        this.update(this.snapshot);
      });
  }
}

function templateInput(file: SyncFile): SyncFileRenderInput {
  return {
    path: file.path,
    draft_content: file.content,
    template_formatting: file.formatting ?? {},
  };
}

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function fileMap(value: unknown): Map<string, SyncFile> | null {
  if (!record(value) || !Array.isArray(value.files)) return null;
  const files = new Map<string, SyncFile>();
  for (const item of value.files) {
    if (!record(item) || typeof item.path !== 'string' || typeof item.content !== 'string')
      return null;
    const formatting = parseFormattingPatch(formattingMetadata(item.formatting ?? {}));
    if (formatting === null) return null;
    files.set(item.path, { path: item.path, content: item.content, formatting });
  }
  return files;
}

/** Formatting fields are bounded integer metadata, separate from arbitrary file literals. */
function formattingMetadata(value: unknown): unknown {
  if (typeof JSON.isRawJSON === 'function' && JSON.isRawJSON(value)) {
    const number = Number(value.rawJSON);
    return Number.isSafeInteger(number) && /^-?(?:0|[1-9]\d*)$/u.test(value.rawJSON)
      ? number
      : value;
  }
  if (Array.isArray(value)) return value.map(formattingMetadata);
  if (record(value))
    return Object.fromEntries(
      Object.entries(value).map(([key, entry]) => [key, formattingMetadata(entry)]),
    );
  return value;
}

type Adjustment = { merge?: Omit<SyncFileMerge, 'path'>; formatting?: SyncFile['formatting'] };
function adjustments(value: SettingsJson): Map<string, Adjustment> | null {
  const envelope = parseSyncOverrideEditorEnvelope(value);
  if (!envelope) return null;
  const serialized = serializeSyncOverrideDocument(envelope);
  if (!serialized.ok) return null;
  const document = serialized.document;
  const rows = new Map<string, Adjustment>();
  for (const row of (document.merges ?? []) as SyncFileMerge[]) {
    const merge = cloneSettingsJson(row as unknown as SettingsJson) as unknown as SyncFileMerge;
    const path = merge.path;
    delete (merge as Partial<SyncFileMerge>).path;
    rows.set(path, { merge });
  }
  for (const row of (document.formats ?? []) as {
    path: string;
    formatting: SyncFile['formatting'];
  }[]) {
    rows.set(row.path, { ...rows.get(row.path), formatting: row.formatting });
  }
  return rows;
}
