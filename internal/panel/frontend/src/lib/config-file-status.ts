import { SYNC_KINDS } from './types';
import type { SettingsDraftRegistry } from './settings-drafts.svelte';
import type { ConfigFileSyncStatus } from './config-file-sync';
import type { PillTone } from './components/Pill.svelte';

export interface ConfigFileStatusConnection {
  data?: ConfigFileSyncStatus;
  isPending: boolean;
  isFetching: boolean;
  error: Error | null;
  refetch: () => unknown;
}
export type ConfigFileStatusReader = (
  targetId: string,
  repositoryId?: string,
) => Promise<ConfigFileSyncStatus>;

/** Own cache family: repository mutation updaters only accept RepositoryDetail values. */
export function configFileStatusQuery(
  targetId: string,
  surface: 'panel' | 'root',
  repositoryId: string | undefined,
  revision: number | string,
  read: ConfigFileStatusReader,
  enabled: boolean,
) {
  return {
    queryKey: ['config-file-status', targetId, surface, repositoryId ?? '', revision],
    queryFn: () => read(targetId, repositoryId),
    enabled,
  };
}

/** Include every saved resource the backend compares, without using draft text or policy. */
export function configFileStatusRevision(
  registry: Pick<SettingsDraftRegistry, 'resource'>,
  targetId: string,
  fallbackOwnerRevision: number,
  repositoryId?: string,
): string {
  const owner = registry.resource(
    repositoryId === undefined
      ? { type: 'target-defaults', targetId }
      : { type: 'repository-settings', targetId, repositoryId },
  );
  const revisions = SYNC_KINDS.map(
    (kind) =>
      registry.resource(
        repositoryId === undefined
          ? { type: 'sync-config', targetId, kind }
          : { type: 'sync-override', targetId, repositoryId, kind },
      )?.expectedRevision ?? 'unread',
  );
  return [owner?.expectedRevision ?? fallbackOwnerRevision, ...revisions].join('/');
}

interface StatusView {
  label: string;
  tone: PillTone;
  description: string;
  proposal?: { number: number; url: string };
}
const waiting: StatusView = {
  label: 'Waiting for check',
  tone: 'neutral',
  description: 'The saved settings have not been checked against the file yet',
};

/** Only saved policy and current observations determine operational status. */
export function configFileStatusView(
  status: ConfigFileSyncStatus,
  savedEnabled: boolean,
  savedFileIgnored = false,
): StatusView {
  if (!status.available)
    return {
      label: 'Unavailable',
      tone: 'warning',
      description: 'Smyklot cannot currently access this workspace or repository',
    };
  if (!savedEnabled)
    return { label: 'Off', tone: 'neutral', description: 'Settings are not synced with the file' };
  if (savedFileIgnored)
    return {
      label: 'File settings off',
      tone: 'warning',
      description: 'Turn on Use file settings to allow imports and pull requests',
    };
  if (status.enabled !== savedEnabled || !status.last_check || !status.last_check.settings_current)
    return waiting;
  const check = status.last_check;
  if (status.status === 'pending')
    return {
      label: 'Sync pending',
      tone: 'neutral',
      description: 'Changes have not finished syncing',
      proposal: check.proposal,
    };
  if (status.status === 'ready')
    return {
      label: 'In sync',
      tone: 'success',
      description: 'The saved settings matched the file at the last check',
    };
  if (status.status === 'proposed')
    return {
      label: 'Pull request open',
      tone: 'info',
      description: 'Panel changes are awaiting a merge on GitHub',
      proposal: check.proposal,
    };
  if (status.status === 'blocked') {
    if (check.problem === 'proposal_outstanding')
      return {
        label: 'Open pull request',
        tone: 'warning',
        description:
          'Settings match the file, but an earlier configuration pull request is still open',
        proposal: check.proposal,
      };
    if (check.problem === 'conflicting_edits')
      return {
        label: 'Conflicting changes',
        tone: 'warning',
        description: 'Some settings changed in both the panel and the file',
        proposal: check.proposal,
      };
    if (check.problem === 'file_removed')
      return {
        label: 'File removed',
        tone: 'warning',
        description: 'Sync is paused because the connected file was removed',
      };
    if (check.problem === 'file_disabled')
      return {
        label: 'File settings off',
        tone: 'warning',
        description: 'Turn on Use file settings to allow imports and pull requests',
      };
    return {
      label: 'Needs attention',
      tone: 'warning',
      description:
        check.message?.replace(/[.]+$/u, '') || 'The last check could not sync the saved settings',
      proposal: check.proposal,
    };
  }
  return waiting;
}

/** Proposal links must name the supplied repository and actual pull request. */
export function configFileProposalHref(
  proposal: { number: number; url: string } | undefined,
  repository: string,
): string | null {
  if (!proposal || !Number.isSafeInteger(proposal.number) || proposal.number < 1) return null;
  if (
    !/^[\w.-]+\/[\w.-]+$/u.test(repository) ||
    repository.split('/').some((part) => part === '.' || part === '..')
  )
    return null;
  try {
    const url = new URL(proposal.url);
    return url.protocol === 'https:' &&
      url.host === 'github.com' &&
      !url.username &&
      !url.password &&
      !url.search &&
      !url.hash &&
      url.pathname === `/${repository}/pull/${proposal.number}`
      ? url.href
      : null;
  } catch {
    return null;
  }
}
