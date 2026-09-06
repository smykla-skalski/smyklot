import type { ConfigPatch, ConfigMigrationState, RepositoryDetail } from '../src/lib/types.js';

// Mirrors config.RepoConfigPaths. Exercise observation independently of bypass.
export const REPOSITORY_FILE_SEARCH_PATHS = [
  '.smyklot.toml',
  '.smyklot/config.toml',
  '.github/.smyklot.toml',
  '.github/smyklot.yaml',
];

export interface RepositoryFileFixture {
  status: NonNullable<RepositoryDetail['config_file_observation']>['status'];
  path?: string;
  patch: ConfigPatch;
  error?: string;
  bypass?: boolean;
  superseded?: string[];
  migration?: ConfigMigrationState;
  migrationPR?: number;
}

export const REPOSITORY_FILE_VARIANTS: Record<string, RepositoryFileFixture> = {
  'migration-demo': {
    status: 'valid',
    path: '.github/smyklot.yaml',
    patch: { quiet_pending: true },
    migration: 'declined',
    migrationPR: 41,
  },
  'search-indexer': {
    status: 'valid',
    path: '.github/smyklot.yaml',
    patch: { command_prefix: '/search-indexer ' },
    migration: 'declined',
    migrationPR: 44,
  },
  'api-gateway': {
    status: 'valid',
    path: '.smyklot.toml',
    patch: { command_prefix: '/api-gateway ' },
    bypass: true,
    superseded: ['.github/smyklot.yaml'],
  },
  'auth-service': { status: 'valid', path: '.smyklot.toml', patch: {} },
  'billing-worker': { status: 'missing', patch: {} },
  'cli-tools': {
    status: 'invalid',
    path: '.github/smyklot.yaml',
    patch: {},
    error: 'line 4: unknown setting "aproved_commands"',
  },
  'customer-portal': {
    status: 'invalid',
    path: '.smyklot.toml',
    patch: {},
    error: 'line 7: command_aliases must be a mapping',
    bypass: true,
  },
  'data-pipeline': { status: 'unknown', patch: {} },
  'deployment-config': {
    status: 'valid',
    path: '.github/smyklot.yaml',
    patch: { quiet_success: true },
    migration: 'proposed',
    migrationPR: 42,
  },
  'design-system': {
    status: 'valid',
    path: '.github/smyklot.yaml',
    patch: { quiet_success: false },
    migration: 'declined',
    migrationPR: 43,
  },
  'docs-site': {
    status: 'valid',
    path: '.github/smyklot.yaml',
    patch: { command_prefix: '/' },
    migration: 'blocked',
  },
  'edge-proxy': {
    status: 'valid',
    path: '.smyklot.toml',
    patch: { quiet_pending: true },
    superseded: ['.smyklot/config.toml', '.github/.smyklot.toml', '.github/smyklot.yaml'],
  },
  'event-consumer': { status: 'missing', patch: {}, bypass: true },
  'feature-flags': {
    status: 'valid',
    path: '.smyklot/config.toml',
    patch: { quiet_reactions: true },
  },
  'identity-provider': {
    status: 'valid',
    path: '.github/.smyklot.toml',
    patch: { disable_mentions: true },
  },
};

export function observedRepositoryFileStatus(detail: RepositoryDetail) {
  if (detail.ignore_repository_file) return 'bypassed' as const;
  const status = detail.config_file_observation?.status;
  return status === undefined || status === 'unknown' ? ('missing' as const) : status;
}
