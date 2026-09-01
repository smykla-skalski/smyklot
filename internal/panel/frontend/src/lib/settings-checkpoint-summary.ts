import type {
  SettingsCheckpoint,
  SettingsCheckpointItem,
  SettingsCheckpointState,
  SyncKind,
} from './types';

/**
 * THE ONE PLACE THE OLD WORD STAYS, and it is not vocabulary - it is data.
 *
 * These strings are what `internal/storage` wrote into the audit table, row by row, on a
 * running service. They are keys to history rather than words anybody reads: this
 * function is the thing that turns them into words. Renaming them would orphan every
 * audit row already written, which is a migration and not a rename, so the audit keeps
 * its spelling and the reader still sees "Restored".
 */
export function settingsCheckpointActionLabel(action: SettingsCheckpoint['action']): string {
  if (action === 'installation.settings.restored' || action === 'runtime.settings.restored') {
    return 'Restored';
  }
  if (action === 'installation.settings.baseline' || action === 'runtime.settings.baseline') {
    return 'Initial snapshot';
  }
  return 'Saved';
}

export function settingsCheckpointItemLabel(item: SettingsCheckpointItem): string {
  const syncKind =
    item.sync_kind === undefined
      ? 'Sync'
      : item.sync_kind[0]?.toLocaleUpperCase() + item.sync_kind.slice(1);
  switch (item.kind) {
    case 'target':
      return 'Workspace settings';
    case 'repository':
      return item.repository_full_name ?? 'Repository settings';
    case 'sync_config':
      return `${syncKind} Sync`;
    case 'sync_override':
      return `${item.repository_full_name ?? 'Repository'} · ${syncKind} override`;
    case 'runtime':
      return 'Service settings';
  }
}

function countKeys(value: unknown): number {
  return isRecord(value) ? Object.keys(value).length : 0;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function nestedDocument(document: Record<string, unknown>): Record<string, unknown> {
  const value = document.document;
  if (isRecord(value)) return value;
  if (typeof value !== 'string') return {};
  try {
    const parsed: unknown = JSON.parse(value);
    return isRecord(parsed) ? parsed : {};
  } catch {
    return {};
  }
}

function targetSummary(document: Record<string, unknown>): string {
  const repositoryDefault =
    document.repository_default_enabled === true
      ? 'Repositories on by default'
      : 'Repositories off by default';
  const pendingCI =
    document.pending_ci_mode_default === 'labels' ? 'Pending CI labels' : 'Pending CI checks';
  const patches = countKeys(document.config_patch);
  return `${repositoryDefault} · ${pendingCI} · ${patches} ${patches === 1 ? 'policy override' : 'policy overrides'}`;
}

function repositorySummary(document: Record<string, unknown>): string {
  const enabled =
    document.enabled_override === true
      ? 'On'
      : document.enabled_override === false
        ? 'Off'
        : 'From the workspace';
  const repositoryFile =
    document.ignore_repository_file === true ? 'Repository file ignored' : 'Repository file read';
  const patches = countKeys(document.config_patch);
  return `${enabled} · ${repositoryFile} · ${patches} ${patches === 1 ? 'policy override' : 'policy overrides'}`;
}

function syncSummary(kind: SyncKind, enabled: boolean, document: Record<string, unknown>): string {
  const prefix = enabled ? 'On' : 'Off';
  switch (kind) {
    case 'labels': {
      const labels = Array.isArray(document.labels) ? document.labels.length : 0;
      const excludes = Array.isArray(document.excludes) ? document.excludes.length : 0;
      return `${prefix} · ${labels} ${labels === 1 ? 'label' : 'labels'} · ${document.allow_removal === true ? 'removal allowed' : 'removal blocked'} · ${excludes} ${excludes === 1 ? 'exclusion' : 'exclusions'}`;
    }
    case 'settings': {
      const settings = Object.keys(document).length;
      return `${prefix} · ${settings} managed ${settings === 1 ? 'setting' : 'settings'}`;
    }
    case 'rulesets': {
      const rulesets = Array.isArray(document.rulesets) ? document.rulesets.length : 0;
      return `${prefix} · ${rulesets} ${rulesets === 1 ? 'ruleset' : 'rulesets'} · ${document.allow_removal === true ? 'removal allowed' : 'removal blocked'}`;
    }
    case 'files': {
      const files = Array.isArray(document.files) ? document.files.length : 0;
      const retired = Array.isArray(document.retired) ? document.retired.length : 0;
      return `${prefix} · ${files} shared ${files === 1 ? 'file' : 'files'} · ${retired} retired`;
    }
  }
}

function syncConfigSummary(
  item: SettingsCheckpointItem,
  document: Record<string, unknown>,
): string {
  if (item.sync_kind === undefined) return 'Stored Sync configuration';
  return syncSummary(item.sync_kind, document.enabled === true, nestedDocument(document));
}

function syncOverrideSummary(document: Record<string, unknown>): string {
  const enabled =
    document.enabled === true ? 'On' : document.enabled === false ? 'Off' : 'From the workspace';
  const fields = countKeys(nestedDocument(document));
  return `${enabled} · ${fields} ${fields === 1 ? 'stored field' : 'stored fields'}`;
}

function runtimeSummary(document: Record<string, unknown>): string {
  const overrides = [
    document.bot_config,
    document.log_level,
    document.poll_interval,
    document.pending_ci_quiet_period,
    document.path_index_interval,
    document.session_ttl,
  ].filter((value) => value !== null).length;
  return `${overrides} ${overrides === 1 ? 'override' : 'overrides'} · Current deployment fills the rest`;
}

export function settingsCheckpointSummary(
  item: SettingsCheckpointItem,
  state: SettingsCheckpointState | null,
): string {
  if (state === null) return 'Not configured';
  switch (item.kind) {
    case 'target':
      return targetSummary(state.document);
    case 'repository':
      return repositorySummary(state.document);
    case 'sync_config':
      return syncConfigSummary(item, state.document);
    case 'sync_override':
      return syncOverrideSummary(state.document);
    case 'runtime':
      return runtimeSummary(state.document);
  }
}
