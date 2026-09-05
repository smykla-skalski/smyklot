/**
 * Fixtures for the catalogue.
 *
 * A deliberate fixed instant rather than `Date.now()`: half of what these components
 * draw is relative - "4 minutes ago", a countdown running out - and a story showing a
 * duration has to say the same thing twice or it cannot be compared against anything.
 *
 * Everything the mock itself seeds comes FROM the mock, through `dev/fixtures.ts` -
 * the account, the config defaults, the users, the notifications, the pending-CI
 * queue and the sync documents are the same objects the dev server hands the panel,
 * built here at the fixed instant above. A story that restated them would agree with
 * the service on the day it was written and drift from it after, which is how a
 * catalogue comes to show a shape nothing sends any more.
 *
 * What is still declared below is what the mock has no seed for, because it is
 * assembled per request rather than stored: `RootOverview` is counted from the state,
 * and `NotificationPage`, `RepositorySummary` and `RepositoryDetail` are pages the
 * handlers build. Those stay here, small enough to read, until there is a builder to
 * call instead.
 */
import { DEFAULT_CONFIG, KNOWN_PATHS, rootPanelUsers, seed, VIEWER } from '../../dev/fixtures.ts';
import { CONFIG_KEYS } from '#lib/config.js';
import { formattingSources, parseFormattingPatch } from '#lib/formatting.js';

import type {
  ConfigSources,
  ConfigValues,
  PanelAccount,
  PanelTarget,
  NotificationPage,
  PendingCIRequest,
  RootWorkspace,
  RootOverview,
  RepositoryDetail,
  RepositorySummary,
  RootRuntimeSettings,
  SecurityNotification,
  SyncConfig,
  SyncFilesContext,
  SyncFileMergeEntry,
  SyncOverride,
  SyncPlan,
  SyncStatus,
} from '#lib/types.js';

/** 2026-08-18T00:00:00Z, so every relative label in a story is stable. */
export const NOW = Date.UTC(2026, 7, 18);

const at = (offsetMs: number): string => new Date(NOW + offsetMs).toISOString();

/**
 * The mock's whole state, at the fixed instant.
 *
 * No issued invitations and no stored preferences: both are things the dev server
 * persists to disk between runs, and a story has neither and must not depend on one.
 */
const MOCK = seed(undefined, NOW);

/** Durable background work, shared with the development server. */
export const GENERAL_QUEUE = MOCK.queue;

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

/* Assembled per request by the handler rather than stored, so there is no seed to
   call: `pending_ci` is a live split of the queue into active and recent. */
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

/** The signed-in account, exactly as the mock serves it. */
export const ACCOUNT: PanelAccount = VIEWER;

/** The defaults the service ships, not a copy of them. */
export const CONFIG: ConfigValues = DEFAULT_CONFIG;

/** Every key resolves from the deployment unless a story says otherwise. */
const SOURCES = Object.fromEntries(
  CONFIG_KEYS.map((key) => [key, 'process' as const]),
) as ConfigSources;

/** The panel accounts the mock seeds, across every system role and status. */
export const USERS = MOCK.users;

/** Every repository the organisation workspace reaches, as the mock seeds them. */
export const REPOSITORIES = MOCK.targets[0]!.repositories.map((entry) => entry.detail.repository);

/** The configuration changes the organisation workspace has recorded. */
export const AUDIT = MOCK.targets[0]!.audit;

/** The delivery failures the organisation workspace has recorded. */
export const FAILURES = MOCK.targets[0]!.failures;

/** The same people as the Root console sees them, counts and all. */
export const ROOT_USERS = rootPanelUsers(MOCK);

/** The Root invitations the mock seeds, in every state one can be in. */
export const INVITATIONS = MOCK.invitations;

/** The organisation workspace the mock seeds, not a second description of it. */
export const TARGET: PanelTarget = MOCK.targets[0]!.value;

/** The same workspace as a Root sees it before requesting temporary write access. */
export const ROOT_TARGET: PanelTarget = {
  ...TARGET,
  effective_role: 'none',
  access_source: 'root',
  capabilities: { read: true, write: false, manage_target_users: false },
};

/**
 * What each sync kind has configured, keyed the way the mock keys it.
 *
 * Three of the four are seeded; `settings` is not, and the mock invents an empty
 * document for it on first ask. That invention stamps `new Date()`, so it is rebuilt
 * here at the fixed instant instead - a catalogue that showed a different "saved just
 * now" on every reload would be comparing two of its own screenshots and finding a
 * difference nothing put there.
 */
export const SYNC_CONFIGS: ReadonlyMap<string, SyncConfig> = MOCK.sync;

/** The shape a kind nobody has configured comes back as. */
export function emptySyncConfig(kind: string): SyncConfig {
  return {
    kind,
    enabled: false,
    labels: [],
    allow_removal: false,
    excludes: [],
    revision: 0,
    updated_by: '',
    updated_at: at(0),
    digest: '',
    document: {},
    unreadable: false,
    unavailable: '',
  };
}

/** What the mock would change to bring the organisation's repositories into step. */
export const SYNC_PLAN: SyncPlan | null = MOCK.syncPlans.get(TARGET.id) ?? null;

/** The fleet state the mock serves beside the plan. */
export const SYNC_STATUS: SyncStatus = MOCK.syncStatus.get(TARGET.id) ?? {
  checked_at: at(0),
  repositories: [],
};

/** The same fleet after a sweep found no drift, for settled-state stories. */
export const SYNC_STATUS_IN_STEP: SyncStatus = {
  checked_at: SYNC_STATUS.checked_at,
  repositories: SYNC_STATUS.repositories.map((row) => ({
    repository: row.repository,
    cells: {
      labels: { state: 'in_step' },
      settings: { state: 'in_step' },
      rulesets: { state: 'in_step' },
      files: { state: 'in_step' },
    },
  })),
};

/** One repository's own answer about the files the organisation keeps in step. */
export const SYNC_OVERRIDES: ReadonlyMap<string, SyncOverride> = MOCK.syncOverrides;

function storyFileAdjustments(): SyncFileMergeEntry[] {
  const adjustments = new Map<string, SyncFileMergeEntry>();
  for (const [key, override] of SYNC_OVERRIDES) {
    const [repositoryId, kind] = key.split('/');
    if (kind !== 'files' || repositoryId === undefined) continue;
    const repository =
      MOCK.targets
        .flatMap((target) => target.repositories)
        .find((candidate) => candidate.detail.repository.id === repositoryId)?.detail.repository
        .name ?? repositoryId;
    const merges = override.document.merges;
    if (Array.isArray(merges)) {
      for (const merge of merges as Array<Record<string, unknown>>) {
        if (typeof merge.path !== 'string') continue;
        adjustments.set(`${repositoryId}\u0000${merge.path}`, {
          repository,
          repository_id: repositoryId,
          path: merge.path,
          merge,
        });
      }
    }
    const formats = override.document.formats;
    if (!Array.isArray(formats)) continue;
    for (const row of formats) {
      if (
        typeof row !== 'object' ||
        row === null ||
        !('path' in row) ||
        typeof row.path !== 'string' ||
        !('formatting' in row)
      ) {
        continue;
      }
      const formatting = parseFormattingPatch(row.formatting);
      if (formatting === null) continue;
      const adjustmentKey = `${repositoryId}\u0000${row.path}`;
      adjustments.set(adjustmentKey, {
        repository,
        repository_id: repositoryId,
        path: row.path,
        ...(adjustments.get(adjustmentKey)?.merge === undefined
          ? {}
          : { merge: adjustments.get(adjustmentKey)?.merge }),
        formatting,
      });
    }
  }
  return [...adjustments.values()];
}

/** The file index and repository adjustments the mock derives for the files view. */
export const SYNC_FILES_CONTEXT: SyncFilesContext = {
  repositories: SYNC_STATUS.repositories.length,
  covered: SYNC_STATUS.repositories.filter((row) => row.cells.files.state !== 'off').length,
  known_paths: KNOWN_PATHS,
  repository_policies: [...SYNC_STATUS.repositories].map((row) => {
    const found = MOCK.targets
      .flatMap((target) => target.repositories)
      .find((candidate) => candidate.detail.repository.name === row.repository)?.detail.repository;
    return {
      repository: row.repository,
      repository_id: found?.id ?? `mock:${row.repository}`,
    };
  }),
  merges: storyFileAdjustments(),
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
  catalog: { workspaces: 3, repositories: 41, enabled_repositories: 28 },
  ownership: { fresh: 2, stale: 1, permission_pending: 0, error: 0 },
  active_elevations: 0,
  unread_security_events: 2,
  recent_failures: [],
  pending_ci: QUEUE,
};

const notification = (
  over: Partial<SecurityNotification> & { id: string },
): SecurityNotification => ({
  workspace: ACCOUNT,
  actor: { ...ACCOUNT, id: '1001', login: 'bart', display_name: 'Bart Smykla' },
  elevation_id: 'elev-8f2c1d9e',
  audit_event_id: 'audit-4a7b',
  action: 'workspace.settings.saved',
  reason: 'Investigating a stuck delivery',
  created_at: at(-2 * 60 * 60_000),
  ...over,
});

export const NOTIFICATIONS: NotificationPage = {
  items: [
    notification({ id: 'n-1' }),
    notification({
      id: 'n-2',
      action: 'workspace.settings.restored',
      created_at: at(-26 * 60 * 60_000),
      read_at: at(-25 * 60 * 60_000),
    }),
  ],
  next_cursor: null,
  total: 2,
  unread: 1,
};

function workspace(over: Partial<RootWorkspace> & { id: string; login: string }): RootWorkspace {
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
  } as RootWorkspace;
}

export const WORKSPACES: RootWorkspace[] = [
  workspace({ id: '2001', login: 'smykla-skalski' }),
  workspace({
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

/** The mock's seeded organisation in the Root workspace list. */
const SEEDED_ROOT_WORKSPACE: RootWorkspace = {
  id: ROOT_TARGET.id,
  installation_id: ROOT_TARGET.installation_id,
  type: ROOT_TARGET.type,
  account: ROOT_TARGET.account,
  available: true,
  owned_by_viewer: true,
  repository_counts: ROOT_TARGET.repository_counts,
  delivery_health: { failed: 1, last_failure_at: at(-18 * 60_000) },
  ownership: {
    source: 'organization_admin',
    status: 'fresh',
    synced_at: at(-3 * 60_000),
    owner_count: 2,
    stale: false,
  },
};

/** The same seeded organisation as a Root reads it before temporary elevation. */
export const ROOT_WORKSPACE: RootWorkspace = {
  ...SEEDED_ROOT_WORKSPACE,
  owned_by_viewer: false,
};

export const RUNTIME: RootRuntimeSettings = {
  background_work_paused: false,
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
  path_index_interval: {
    deployment_seconds: 60 * 60,
    override_seconds: null,
    effective_seconds: 60 * 60,
    max_seconds: 7 * 24 * 60 * 60,
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
  pending_ci_mode: 'checks',
  pending_ci_mode_source: 'target',
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
  formatting_sources: formattingSources('process'),
  config_file_patch: { command_prefix: '/smyklot ' },
  config_file_path: '.smyklot.toml',
  config_migration: 'none',
  ignore_repository_file: false,
  pending_ci_mode_override: null,
  pending_ci_mode_inherited: 'checks',
  pending_ci_branch_patterns_override: null,
  pending_ci_branch_patterns_inherited: { include: ['~DEFAULT_BRANCH'], exclude: [] },
  pending_ci_quiet_period_seconds_override: null,
  pending_ci_quiet_period_seconds_inherited: 30,
  path_index_interval_seconds_override: null,
  path_index_interval_seconds_inherited: 60 * 60,
  pending_ci_gate: {
    desired_mode: 'checks',
    effective_mode: 'checks',
    readiness: 'ready',
    reason: 'Checks and required context are ready',
  },
  revision: 3,
};
