/**
 * Fixtures for the catalogue.
 *
 * A deliberate fixed instant rather than `Date.now()`: half of what these components
 * draw is relative - "4 minutes ago", a countdown running out - and a story showing a
 * duration has to say the same thing twice or it cannot be compared against anything.
 *
 * These are the shapes the dev mock seeds, kept small enough to read. The mock's own
 * seed data is still private to `dev/mock-server.ts`; when it is lifted into
 * `dev/fixtures.ts` these should import from there rather than restate it.
 */
import type {
  ConfigSources,
  ConfigValues,
  PanelAccount,
  PanelTarget,
  NotificationPage,
  PendingCIRequest,
  RootInstallation,
  RootOverview,
  RepositoryDetail,
  RepositorySummary,
  RootRuntimeSettings,
  SecurityNotification,
} from '#lib/types.js';

/** 2026-08-18T00:00:00Z, so every relative label in a story is stable. */
export const NOW = Date.UTC(2026, 7, 18);

const at = (offsetMs: number): string => new Date(NOW + offsetMs).toISOString();

function request(over: Partial<PendingCIRequest> & { id: string }): PendingCIRequest {
  return {
    repository_full_name: 'smykla-skalski/smyklot',
    pull_request: 240,
    head_sha: 'b2894a7c9f1e4d3a8c6b5f2e1d0a9c8b7e6f5d4c',
    merge_method: 'squash',
    required_checks_only: false,
    requester: 'bart',
    lifecycle: 'armed',
    schedule: 'active',
    next_check_at: at(25_000),
    next_check_trigger: 'poll',
    last_observed_state: 'pending',
    reason: '',
    requested_at: at(-8 * 60_000),
    updated_at: at(-40_000),
    cleanup_pending: false,
    revision: 3,
    ...over,
  } as PendingCIRequest;
}

export const QUEUE: RootOverview['pending_ci'] = {
  active: [
    request({ id: 'q-1', last_observed_state: 'passing', next_check_at: at(9_000) }),
    request({
      id: 'q-2',
      pull_request: 241,
      repository_full_name: 'smykla-skalski/platform-infra',
      last_observed_state: 'failing',
      requester: 'marta-w',
    }),
  ],
  deferred: [
    request({
      id: 'q-3',
      pull_request: 198,
      repository_full_name: 'smykla-skalski/legacy-service',
      schedule: 'deferred',
      last_observed_state: 'no_checks',
      requested_at: at(-3 * 60 * 60_000),
    }),
  ],
  recent: [
    request({
      id: 'q-4',
      pull_request: 237,
      lifecycle: 'merged',
      finished_at: at(-4 * 60_000),
      last_observed_state: 'passing',
    }),
    request({
      id: 'q-5',
      pull_request: 236,
      lifecycle: 'cancelled',
      finished_at: at(-19 * 60_000),
      reason: 'Cancelled by @bart',
    }),
  ],
};

export const ACCOUNT: PanelAccount = {
  id: '2001',
  provider: 'github:https://api.github.com',
  subject_id: '2001',
  login: 'smykla-skalski',
  display_name: 'Smykla Skalski',
  avatar_url: null,
};

export const CONFIG: ConfigValues = {
  quiet_success: false,
  quiet_reactions: false,
  quiet_pending: false,
  allowed_commands: ['approve', 'merge', 'squash'],
  command_aliases: { ship: 'merge' },
  command_prefix: '/smyklot ',
  disable_mentions: false,
  disable_bare_commands: false,
  disable_unapprove: false,
  disable_reactions: false,
  disable_deleted_comments: false,
  allow_self_approval: false,
};

/** Every key resolves from the deployment unless a story says otherwise. */
const SOURCES = Object.fromEntries(
  (Object.keys(CONFIG) as (keyof ConfigValues)[]).map((key) => [key, 'process' as const]),
) as ConfigSources;

export const TARGET: PanelTarget = {
  id: '2001',
  installation_id: '3001',
  type: 'Organization',
  account: ACCOUNT,
  repository_default_enabled: false,
  config_patch: {},
  inherited_config: CONFIG,
  effective_config: CONFIG,
  config_sources: SOURCES,
  revision: 4,
  repository_counts: { total: 5, enabled: 3, disabled: 2 },
  effective_role: 'owner',
  access_source: 'owner',
  capabilities: { read: true, write: true, manage_target_users: true },
};

export const OVERVIEW: RootOverview = {
  service: {
    status: 'healthy',
    version: '1.37.0',
    service_host: 'smyklot.com',
    uptime_seconds: 6 * 24 * 60 * 60 + 4 * 60 * 60,
    storage: 'healthy',
    database: {
      state: 'healthy',
      engine: 'postgres',
      version: '16.4',
      schema_version: 24,
      size_bytes: 84 * 1024 * 1024,
      latency_ms: 3.2,
      connections: { open: 4, in_use: 1, idle: 3, max: 16, wait_count: 0, wait_ms: 0 },
    },
  },
  catalog: { installations: 3, repositories: 41, enabled_repositories: 28 },
  ownership: { fresh: 2, stale: 1, permission_pending: 0, error: 0 },
  active_elevations: 0,
  unread_security_events: 2,
  recent_failures: [],
  pending_ci: QUEUE,
};

const notification = (
  over: Partial<SecurityNotification> & { id: string },
): SecurityNotification => ({
  installation: ACCOUNT,
  actor: { ...ACCOUNT, id: '1001', login: 'bart', display_name: 'Bart Smykla' },
  elevation_id: 'elev-8f2c1d9e',
  audit_event_id: 'audit-4a7b',
  action: 'target.settings.updated',
  reason: 'Investigating a stuck delivery',
  created_at: at(-2 * 60 * 60_000),
  ...over,
});

export const NOTIFICATIONS: NotificationPage = {
  items: [
    notification({ id: 'n-1' }),
    notification({
      id: 'n-2',
      action: 'repository.settings.updated',
      created_at: at(-26 * 60 * 60_000),
      read_at: at(-25 * 60 * 60_000),
    }),
  ],
  next_cursor: null,
  total: 2,
  unread: 1,
};

function installation(
  over: Partial<RootInstallation> & { id: string; login: string },
): RootInstallation {
  const { login, ...rest } = over;
  return {
    installation_id: `3${over.id}`,
    type: 'Organization',
    account: { ...ACCOUNT, id: over.id, subject_id: over.id, login, display_name: login },
    available: true,
    owned_by_viewer: true,
    repository_counts: { total: 12, enabled: 9, disabled: 3 },
    delivery_health: { failed: 0 },
    ownership: {
      source: 'organization_admin',
      status: 'fresh',
      synced_at: at(-20 * 60_000),
      owner_count: 3,
      stale: false,
    },
    ...rest,
  } as RootInstallation;
}

export const INSTALLATIONS: RootInstallation[] = [
  installation({ id: '2001', login: 'smykla-skalski' }),
  installation({
    id: '2002',
    login: 'platform-co',
    owned_by_viewer: false,
    delivery_health: { failed: 4, last_failure_at: at(-90 * 60_000) },
    ownership: {
      source: 'organization_admin',
      status: 'permission_pending',
      synced_at: at(-6 * 60 * 60_000),
      owner_count: 0,
      stale: true,
    },
  }),
];

export const RUNTIME: RootRuntimeSettings = {
  behavior_defaults: { deployment: CONFIG, override: null, effective: CONFIG },
  log_level: { deployment: 'info', override: null, effective: 'info' },
  reaction_poll_interval: {
    deployment_seconds: 30,
    override_seconds: null,
    effective_seconds: 30,
  },
  merge_after_ci_quiet_period: {
    deployment_seconds: 30,
    override_seconds: null,
    effective_seconds: 30,
  },
  session_lifetime: {
    deployment_seconds: 12 * 60 * 60,
    override_seconds: null,
    effective_seconds: 12 * 60 * 60,
  },
  revision: 7,
  service: {
    version: OVERVIEW.service.version,
    uptime_seconds: OVERVIEW.service.uptime_seconds,
    storage: OVERVIEW.service.storage,
    database: OVERVIEW.service.database,
    listeners: { public: ':8080', admin: '127.0.0.1:9090' },
    public_paths: { panel: '/panel', webhook: '/webhook' },
    provider_endpoints: {
      api: 'https://api.github.com',
      authorize: 'https://github.com/login/oauth/authorize',
      token: 'https://github.com/login/oauth/access_token',
    },
    credential_presence: { webhook: true, app: true, oauth: true },
  },
};

export const REPOSITORY: RepositorySummary = {
  id: '4001',
  name: 'smyklot',
  full_name: 'smykla-skalski/smyklot',
  private: false,
  default_branch: 'main',
  available: true,
  enabled_override: true,
  effective_enabled: true,
  enabled_source: 'repository',
  config_override_count: 2,
  config_file_status: 'valid',
  updated_at: at(-12 * 60_000),
};

export const REPOSITORY_DETAIL: RepositoryDetail = {
  repository: REPOSITORY,
  config_patch: { quiet_success: false, allow_self_approval: true },
  inherited_config: CONFIG,
  effective_config: { ...CONFIG, allow_self_approval: true },
  config_sources: { ...SOURCES, allow_self_approval: 'repository_panel' },
  config_file_patch: { command_prefix: '/smyklot ' },
  config_file_path: '.smyklot.toml',
  config_migration: 'none',
  ignore_repository_file: false,
  revision: 3,
};
