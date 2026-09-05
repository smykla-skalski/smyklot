import {
  SYNC_KINDS,
  type SyncConfig,
  type PanelTarget,
  type SyncKind,
  type SyncPlan,
  type SyncRepositoryStatus,
  type SyncStatus,
} from './types';

export type SyncHealth = 'blocked' | 'syncing' | 'paused' | 'settled';

export function repositorySyncHealth(row: SyncRepositoryStatus): SyncHealth {
  const cells = SYNC_KINDS.map((kind) => row.cells[kind]);
  if (cells.some((cell) => cell.state === 'refused')) return 'blocked';
  if (cells.every((cell) => cell.state === 'off')) return 'paused';
  if (cells.some((cell) => cell.state === 'pending' || (cell.changes ?? 0) > 0)) return 'syncing';
  return 'settled';
}

export interface SyncIssue {
  id: string;
  title: string;
  detail: string;
  kind?: SyncKind;
  repository?: string;
  permission?: boolean;
  queue?: boolean;
}

/** Only work that needs a person belongs here. Scheduled changes and automatic retries do not. */
export function syncIssues(
  status: SyncStatus | null,
  plan: SyncPlan | null,
  configs: Partial<Record<SyncKind, SyncConfig>> = {},
): SyncIssue[] {
  const issues: SyncIssue[] = [];
  for (const kind of SYNC_KINDS) {
    const config = configs[kind];
    const unavailable =
      status?.unavailable?.[kind] || (config?.enabled ? config.unavailable : undefined);
    const invalid = status?.invalid?.[kind] || (config?.enabled && config.unreadable);
    if (unavailable || invalid) {
      issues.push({
        id: `kind:${kind}`,
        kind,
        permission: !!unavailable,
        title: !unavailable ? 'Configuration needs a fix' : 'GitHub permission needed',
        detail: unavailable || 'This configuration cannot be read; fix it before sync can continue',
      });
    }
  }
  for (const row of status?.repositories ?? []) {
    const blocked = SYNC_KINDS.filter(
      (kind) =>
        row.cells[kind].state === 'refused' &&
        !issues.some((issue) => issue.kind === kind && !issue.repository),
    );
    const kind = blocked[0];
    if (kind === undefined) continue;
    issues.push({
      id: `repository:${row.repository}`,
      repository: row.repository,
      kind,
      title: row.repository,
      detail:
        row.cells[kind].reason ||
        row.reason ||
        'Sync could not continue; inspect this repository’s configuration',
    });
  }
  for (const action of plan?.actions ?? []) {
    if (
      !(action.state === 'failed' || (action.state === 'skipped' && !!action.blocker)) ||
      issues.some(
        (issue) =>
          issue.repository === action.repository ||
          (!issue.repository && issue.kind === action.kind),
      )
    )
      continue;
    issues.push({
      id: `action:${action.repository}`,
      repository: action.repository,
      kind: SYNC_KINDS.find((kind) => kind === action.kind),
      title: action.repository,
      detail: action.error || `${action.subject} could not be synced`,
    });
  }
  if (plan?.queue_item?.state === 'blocked') {
    issues.push({
      id: 'system:queue-blocked',
      queue: true,
      title: 'Automatic sync is paused',
      detail: plan.queue_item.blocked_reason || 'Review the sync policy in Queue to continue',
    });
  }
  if (plan?.state === 'computed' && plan.actions.length > 0) {
    issues.push({
      id: 'system:legacy-approval',
      title: 'An earlier sync needs your decision',
      detail:
        'This change was prepared under manual approval; review it once to clear the pending sync',
    });
  }
  return issues;
}

/** GitHub owns installation permissions; recovery belongs at the installed app. */
export function syncPermissionsHref(
  target: Pick<PanelTarget, 'type' | 'installation_id'> & {
    account: Pick<PanelTarget['account'], 'login'>;
  },
): string {
  const scope =
    target.type === 'Organization'
      ? `organizations/${encodeURIComponent(target.account.login)}/settings`
      : 'settings';
  return `https://github.com/${scope}/installations/${encodeURIComponent(target.installation_id)}`;
}
