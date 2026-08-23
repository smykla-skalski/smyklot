import { createContext } from 'svelte';

import { PanelApiError } from './api';
import {
  SYNC_KINDS,
  type SyncConfig,
  type SyncConfigBatchChange,
  type SyncConfigBatchInput,
  type SyncConfigBatchResponse,
  type SyncConfigInput,
  type SyncKind,
} from './types';

type DraftInput = Omit<SyncConfigInput, 'expected_revision'>;
type SaveSyncConfigs = (
  targetId: string,
  input: SyncConfigBatchInput,
) => Promise<SyncConfigBatchResponse>;
type LoadSyncConfigs = (targetId: string) => Promise<SyncConfig[]>;

const kindSet = new Set<string>(SYNC_KINDS);

export function isSyncKind(value: string | undefined): value is SyncKind {
  return value !== undefined && kindSet.has(value);
}

export function staysInSyncDraftInstallation(
  routeId: string | null | undefined,
  nextAccount: string | undefined,
  currentAccount: string,
): boolean {
  return (
    routeId?.startsWith('/i/[account]') === true &&
    nextAccount?.toLowerCase() === currentAccount.toLowerCase()
  );
}

/**
 * One browser-only draft for an installation's four Sync documents.
 *
 * The saved bases carry the optimistic revisions. Drafts carry only the values
 * somebody changed, so navigating away from Sync cannot turn an old response
 * into the next write's expected revision.
 */
export class SyncDraftSet {
  readonly targetId: string;

  private saved = $state<Partial<Record<SyncKind, SyncConfig>>>({});
  private drafts = $state<Partial<Record<SyncKind, DraftInput>>>({});

  saving = $state(false);
  refreshing = $state(false);
  problem = $state<string | null>(null);
  conflict = $state(false);
  invalidKind = $state<SyncKind | null>(null);
  notice = $state<string | null>(null);
  refresh = $state(0);

  constructor(targetId: string) {
    this.targetId = targetId;
  }

  get dirtyKinds(): SyncKind[] {
    return SYNC_KINDS.filter((kind) => this.drafts[kind] !== undefined);
  }

  get dirtyCount(): number {
    return this.dirtyKinds.length;
  }

  get dirty(): boolean {
    return this.dirtyCount > 0;
  }

  /** Accepts fresh server bases without overwriting a kind already being edited. */
  adopt(configs: SyncConfig[]): void {
    for (const config of configs) {
      if (!isSyncKind(config.kind) || this.drafts[config.kind] !== undefined) continue;
      this.saved[config.kind] = config;
    }
  }

  config(kind: SyncKind): SyncConfig | null {
    const base = this.saved[kind];
    if (base === undefined) return null;
    const draft = this.drafts[kind];
    return draft === undefined ? base : configWithDraft(base, draft);
  }

  stage(kind: SyncKind, input: DraftInput): boolean {
    const base = this.saved[kind];
    if (base === undefined || base.unreadable) return false;

    this.problem = null;
    this.conflict = false;
    this.invalidKind = null;
    this.notice = null;
    if (sameInput(input, inputFor(base))) {
      delete this.drafts[kind];
    } else {
      this.drafts[kind] = input;
    }
    return true;
  }

  discard(): void {
    this.drafts = {};
    this.problem = null;
    this.conflict = false;
    this.invalidKind = null;
    this.notice = null;
  }

  dismissNotice(): void {
    this.notice = null;
  }

  acceptCommitted(configs: SyncConfig[], notice: string): void {
    this.replaceSaved(configs);
    this.drafts = {};
    this.problem = null;
    this.conflict = false;
    this.invalidKind = null;
    this.notice = notice;
    this.refresh += 1;
  }

  async save(saveConfigs: SaveSyncConfigs): Promise<boolean> {
    if (this.saving || this.refreshing || !this.dirty) return false;
    const changes = this.changes();
    if (changes === null) return false;

    this.saving = true;
    this.problem = null;
    this.conflict = false;
    this.invalidKind = null;
    this.notice = null;
    try {
      const result = await saveConfigs(this.targetId, { changes });
      this.acceptSaved(
        result.configs,
        changes,
        'Saved. Reconciliation creates a plan only when repositories need changes.',
      );
      return true;
    } catch (cause) {
      this.problem = cause instanceof Error ? cause.message : String(cause);
      if (cause instanceof PanelApiError) {
        this.conflict = cause.status === 409 || cause.code === 'conflict';
        this.invalidKind = isSyncKind(cause.kind) ? cause.kind : null;
      }
      return false;
    } finally {
      this.saving = false;
    }
  }

  async refreshAfterConflict(loadConfigs: LoadSyncConfigs): Promise<boolean> {
    if (this.saving || this.refreshing || !this.conflict) return false;

    this.refreshing = true;
    try {
      this.replaceSaved(await loadConfigs(this.targetId));
      this.conflict = false;
      this.invalidKind = null;
      this.problem =
        'Latest saved configuration loaded. Review your preserved draft, then save again.';
      return true;
    } catch (cause) {
      this.problem = cause instanceof Error ? cause.message : String(cause);
      return false;
    } finally {
      this.refreshing = false;
    }
  }

  private changes(): SyncConfigBatchChange[] | null {
    const changes: SyncConfigBatchChange[] = [];
    for (const kind of this.dirtyKinds) {
      const base = this.saved[kind];
      const draft = this.drafts[kind];
      if (base === undefined || draft === undefined) return null;
      changes.push({ kind, ...$state.snapshot(draft), expected_revision: base.revision });
    }
    return changes;
  }

  private acceptSaved(
    configs: SyncConfig[],
    committed: SyncConfigBatchChange[],
    notice: string,
  ): void {
    this.replaceSaved(configs);
    for (const change of committed) {
      const current = this.drafts[change.kind];
      if (current !== undefined && sameInput(current, inputForChange(change))) {
        delete this.drafts[change.kind];
      }
    }
    for (const kind of this.dirtyKinds) {
      const base = this.saved[kind];
      const draft = this.drafts[kind];
      if (base !== undefined && draft !== undefined && sameInput(draft, inputFor(base))) {
        delete this.drafts[kind];
      }
    }
    this.problem = null;
    this.conflict = false;
    this.invalidKind = null;
    this.notice = notice;
    this.refresh += 1;
  }

  private replaceSaved(configs: SyncConfig[]): void {
    this.saved = {};
    for (const config of configs) {
      if (isSyncKind(config.kind)) this.saved[config.kind] = config;
    }
  }
}

/** The single draft slot shared by every routed page in the panel shell. */
export class SyncDraftScope {
  current = $state.raw<SyncDraftSet | null>(null);

  forTarget(targetId: string): SyncDraftSet {
    if (this.current?.targetId === targetId) return this.current;
    this.current?.discard();
    this.current = new SyncDraftSet(targetId);
    return this.current;
  }

  discard(): void {
    this.current?.discard();
    this.current = null;
  }
}

export const [getSyncDraftScope, setSyncDraftScope] = createContext<SyncDraftScope>();

function inputFor(config: SyncConfig): DraftInput {
  if (config.kind === 'labels') {
    return {
      enabled: config.enabled,
      labels: config.labels,
      allow_removal: config.allow_removal,
      excludes: config.excludes,
    };
  }
  return { enabled: config.enabled, document: config.document };
}

function configWithDraft(base: SyncConfig, draft: DraftInput): SyncConfig {
  return {
    ...base,
    enabled: draft.enabled,
    ...(base.kind === 'labels'
      ? {
          labels: draft.labels ?? [],
          allow_removal: draft.allow_removal ?? false,
          excludes: draft.excludes ?? [],
        }
      : { document: draft.document ?? {} }),
  };
}

function inputForChange(change: SyncConfigBatchChange): DraftInput {
  if (change.kind === 'labels') {
    return {
      enabled: change.enabled,
      labels: change.labels,
      allow_removal: change.allow_removal,
      excludes: change.excludes,
    };
  }
  return { enabled: change.enabled, document: change.document };
}

function sameInput(left: DraftInput, right: DraftInput): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}
