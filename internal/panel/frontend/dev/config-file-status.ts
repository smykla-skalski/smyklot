import { SYNC_KINDS } from '../src/lib/types.js';
import type { MockState } from './fixtures.js';
import type { ConfigFileSyncStatus } from '../src/lib/config-file-sync.js';

type Check = NonNullable<ConfigFileSyncStatus['last_check']>;
export interface ConfigFileStatusFixture {
  enabled: boolean;
  available?: boolean;
  check?: Partial<Check> & Pick<Check, 'status'>;
}

/** Named states are shared by the mock API and the component catalogue. */
export const CONFIG_FILE_STATUS_FIXTURES = {
  off: { enabled: false },
  pending: { enabled: true },
  syncing: { enabled: true, check: { status: 'pending' } },
  ready: { enabled: true, check: { status: 'ready' } },
  proposed: { enabled: true, check: { status: 'proposed', proposal: { number: 84, url: '' } } },
  conflict: {
    enabled: true,
    check: {
      status: 'blocked',
      problem: 'conflicting_edits',
      conflict_count: 2,
      conflict_paths: [
        ['config', 'command_prefix'],
        ['config', 'quiet_success'],
      ],
    },
  },
  removed: { enabled: true, check: { status: 'blocked', problem: 'file_removed' } },
  invalid: {
    enabled: true,
    check: {
      status: 'blocked',
      problem: 'invalid_file',
      message: 'The configuration file contains invalid TOML',
    },
  },
  schema: {
    enabled: true,
    check: {
      status: 'blocked',
      problem: 'invalid_file',
      message: 'This file uses an unsupported configuration version',
    },
  },
  scope: {
    enabled: true,
    check: {
      status: 'blocked',
      problem: 'invalid_file',
      message: 'This file contains settings for a different scope',
    },
  },
  fileOff: { enabled: true, check: { status: 'blocked', problem: 'file_disabled' } },
  missingAccess: {
    enabled: true,
    check: {
      status: 'blocked',
      problem: 'workspace_repository_missing',
      message: 'Give Smyklot access to the .github repository to sync workspace settings',
    },
  },
  unavailable: { enabled: true, available: false },
  stale: { enabled: true, check: { status: 'ready', settings_current: false } },
  outstanding: {
    enabled: true,
    check: {
      status: 'blocked',
      problem: 'proposal_outstanding',
      proposal: { number: 78, url: '' },
    },
  },
} as const satisfies Record<string, ConfigFileStatusFixture>;
export type ConfigFileStatusVariant = keyof typeof CONFIG_FILE_STATUS_FIXTURES;

export const REPOSITORY_CONFIG_FILE_STATES: Readonly<Record<string, ConfigFileStatusVariant>> = {
  'auth-service': 'ready',
  'billing-worker': 'pending',
  'cli-tools': 'invalid',
  'customer-portal': 'fileOff',
  'data-pipeline': 'pending',
  'deployment-config': 'proposed',
  'design-system': 'outstanding',
  'docs-site': 'scope',
  'edge-proxy': 'conflict',
  'event-consumer': 'removed',
  'feature-flags': 'stale',
  'identity-provider': 'unavailable',
  'internal-tools': 'schema',
};

export function workspaceConfigFileVariant(login: string): ConfigFileStatusVariant {
  if (login === 'bart') return 'missingAccess';
  if (login === 'team-01') return 'ready';
  return 'off';
}

/** A read never advances the coordinator. A save makes the old check stale. */
export function mockConfigFileStatus(
  variant: ConfigFileStatusVariant,
  repository: string,
  enabled = CONFIG_FILE_STATUS_FIXTURES[variant].enabled,
  revision = 1,
  fileIgnored = false,
  workspace = false,
  checkedAt = '2026-09-07T12:00:00Z',
  syncCurrent = true,
): ConfigFileSyncStatus {
  const fixture: ConfigFileStatusFixture = CONFIG_FILE_STATUS_FIXTURES[variant];
  const available = fixture.available !== false;
  if (!enabled || !available) return { enabled, available, status: 'off' };
  if (!fixture.check) return { enabled, available, status: 'pending' };
  const current = revision === 1 && syncCurrent && fixture.check.settings_current !== false;
  const check: Check = {
    checked_at: checkedAt,
    path: workspace ? '.smyklot/workspace.toml' : '.smyklot.toml',
    settings_current: current,
    ...fixture.check,
  };
  // Preserve the revision freshness rule even for explicitly stale fixture checks.
  check.settings_current = current;
  if (fileIgnored && current) {
    check.status = 'blocked';
    check.problem = 'file_disabled';
    delete check.proposal;
  }
  if (check.proposal)
    check.proposal = {
      ...check.proposal,
      url: `https://github.com/${repository}/pull/${check.proposal.number}`,
    };
  return { enabled, available, status: current ? check.status : 'pending', last_check: check };
}

/** Owner and every applicable Sync kind participate, including previously absent kinds. */
export function mockConfigFileInputs(
  state: Pick<MockState, 'sync' | 'syncOverrides'>,
  targetId: string,
  repositoryId: string | undefined,
  revision: number,
): string {
  const values = repositoryId === undefined ? state.sync : state.syncOverrides;
  const prefix = repositoryId ?? targetId;
  return [
    revision,
    ...SYNC_KINDS.map((kind) => values.get(`${prefix}/${kind}`)?.revision ?? 0),
  ].join('/');
}
