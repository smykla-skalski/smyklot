import { parseJson, type JsonValue } from '../src/lib/merge.js';
import type { ConfigFilePreview } from '../src/lib/config-file-sync.js';
import { CONFIG_FILE_STATUS_FIXTURES, type ConfigFileStatusVariant } from './config-file-status.js';

export type ConfigFileReviewVariant = ConfigFileStatusVariant | 'missing' | 'independent';

/** A semantic FileDocument, matching the backend's sparse Patch plus panel section. */
export function mockChoiceDocument(
  side: 'panel' | 'file',
  workspace: boolean,
  removed = false,
): JsonValue {
  const exact = '{"id":9007199254740993,"tiny":1e-400,"signed_zero":-0}';
  const syncDocument = workspace
    ? { files: [{ path: 'renovate.json', content: `${exact}\n` }] }
    : parseJson(`{"merges":[{"path":"renovate.json","overrides":${exact}}]}`)!;
  return {
    command_prefix: side === 'panel' ? '/panel ' : '/file ',
    quiet_success: false,
    ...(removed ? {} : { allow_self_approval: true }),
    panel: {
      version: 1,
      scope: workspace ? 'workspace' : 'repository',
      sync: { files: { document: syncDocument } },
    },
  };
}

/** Reads do not change state. The caller binds the token to current owner/input revisions. */
export function mockConfigFilePreview(
  variant: ConfigFileReviewVariant,
  repository: string,
  token: string,
  workspace = false,
  enabled = true,
  fileIgnored = false,
): ConfigFilePreview {
  const base = {
    checked_at: new Date().toISOString(),
    path: workspace ? '.smyklot/workspace.toml' : '.smyklot.toml',
    head: 'c'.repeat(40),
  };
  if (!enabled)
    return {
      ...base,
      status: 'off',
      problem: 'sync_off',
      message: 'Enable configuration file sync to review changes',
    };
  if (fileIgnored)
    return {
      ...base,
      status: 'blocked',
      problem: 'file_disabled',
      message: 'Turn on file settings before syncing changes in both directions',
    };
  if (variant === 'conflict' || variant === 'stale' || variant === 'removed' || variant === 'off') {
    const removed = variant === 'removed';
    return {
      ...base,
      status: 'blocked',
      problem: removed ? 'file_removed' : 'conflicting_edits',
      ...(removed ? {} : { conflict_count: 1, conflict_paths: [['command_prefix']] }),
      review_token: token,
      choices: (removed ? (['panel'] as const) : (['panel', 'file'] as const)).map((side) => ({
        side,
        available: true,
        document: mockChoiceDocument(side, workspace, removed),
        import_panel: !removed,
        publish_file: true,
      })),
    };
  }
  if (
    variant === 'missing' ||
    variant === 'independent' ||
    variant === 'pending' ||
    variant === 'syncing' ||
    variant === 'proposed'
  )
    return { ...base, status: 'pending' };
  if (variant === 'ready') return { ...base, status: 'ready' };
  if (variant === 'unavailable')
    return {
      ...base,
      status: 'blocked',
      problem: 'repository_unavailable',
      message: 'Smyklot cannot currently access this repository',
    };
  const check = CONFIG_FILE_STATUS_FIXTURES[variant].check;
  return {
    ...base,
    status: 'blocked',
    problem: check.problem,
    message:
      'message' in check
        ? check.message
        : 'Settings match the file, but an earlier configuration pull request is still open',
    ...('proposal' in check
      ? {
          proposal: {
            number: check.proposal.number,
            url: `https://github.com/${repository}/pull/${check.proposal.number}`,
          },
        }
      : {}),
  };
}
