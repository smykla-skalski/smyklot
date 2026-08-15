export const COMMANDS = [
  'approve',
  'merge',
  'squash',
  'rebase',
  'unapprove',
  'cleanup',
  'help',
] as const;

export type CommandName = (typeof COMMANDS)[number];

export interface ConfigValues {
  quiet_success: boolean;
  quiet_reactions: boolean;
  quiet_pending: boolean;
  allowed_commands: string[];
  command_aliases: Record<string, string>;
  command_prefix: string;
  disable_mentions: boolean;
  disable_bare_commands: boolean;
  disable_unapprove: boolean;
  disable_reactions: boolean;
  disable_deleted_comments: boolean;
  allow_self_approval: boolean;
}

/** Omitted values inherit from the next lower-precedence source. */
export type ConfigPatch = Partial<ConfigValues>;

export type ConfigKey = keyof ConfigValues;
export type ConfigSource = 'process' | 'target' | 'repository_file' | 'repository_panel';
export type ConfigSources = Record<ConfigKey, ConfigSource>;

export interface PanelAccount {
  id: string;
  provider: string;
  subject_id: string;
  login: string;
  display_name: string;
  avatar_url: string | null;
}

export type InstallationRole = 'none' | 'viewer' | 'editor' | 'admin' | 'owner';
export type SystemRole = 'none' | 'root' | 'super_root';
export type PanelUserStatus = 'active' | 'banned' | 'removed';
export type AccessSource = 'owner' | 'target' | 'suspended' | 'root' | 'elevation' | 'denied';

export interface PanelCapabilities {
  read: boolean;
  write: boolean;
  manage_target_users: boolean;
}

export interface PanelViewer {
  account: PanelAccount;
  system_role: SystemRole;
  status: PanelUserStatus;
  target_count: number;
}

export interface TargetUserAccess {
  role: Exclude<InstallationRole, 'owner'> | null;
  suspended: boolean;
  suspension_reason?: string;
  revision: number;
  updated_at?: string;
  effective_role: InstallationRole;
  source: AccessSource;
  capabilities: PanelCapabilities;
}

export interface PanelUser {
  account: PanelAccount;
  system_role: SystemRole;
  status: PanelUserStatus;
  ban_reason?: string;
  banned_at?: string;
  last_login_at?: string;
  revision: number;
  created_at: string;
  updated_at: string;
  manageable: boolean;
  target_access?: TargetUserAccess;
}

export type PanelUserSort =
  | 'name_asc'
  | 'name_desc'
  | 'role_asc'
  | 'role_desc'
  | 'updated_newest'
  | 'updated_oldest'
  | 'login_newest'
  | 'login_oldest';
export type PanelUserListStatus = 'active' | 'banned' | 'suspended';

export interface PanelUserPageRequest {
  cursor?: string;
  query: string;
  sort: PanelUserSort;
  limit: number;
  roles: InstallationRole[];
  statuses: PanelUserListStatus[];
}

export interface RootPanelUser {
  account: PanelAccount;
  system_role: SystemRole;
  status: PanelUserStatus;
  ban_reason?: string;
  banned_at?: string;
  removed_at?: string;
  last_login_at?: string;
  revision: number;
  owned_installations: number;
  assigned_installations: number;
  manageable: boolean;
  can_manage_system_role: boolean;
}

export type RootPanelUserSort =
  'name_asc' | 'name_desc' | 'role_asc' | 'role_desc' | 'login_newest' | 'login_oldest';

export interface RootPanelUserPageRequest {
  cursor?: string;
  query: string;
  sort: RootPanelUserSort;
  limit: number;
  systemRoles: SystemRole[];
  statuses: PanelUserStatus[];
}

export type UpdateRootUserInput =
  | {
      system_role: Exclude<SystemRole, 'super_root'>;
      expected_revision: number;
    }
  | {
      status: PanelUserStatus;
      reason?: string;
      expected_revision: number;
    };

export interface AddTargetUserInput {
  login: string;
  role: Exclude<InstallationRole, 'none' | 'owner'>;
}

export interface UpdateTargetUserInput {
  role: Exclude<InstallationRole, 'owner'> | null;
  suspended: boolean;
  suspension_reason?: string;
  expected_revision: number;
}

export type InvitationStatus = 'pending' | 'accepted' | 'declined' | 'revoked' | 'expired';
export type InvitationDays = 1 | 7 | 30;

export interface PanelInvitation {
  id: string;
  account: PanelAccount;
  target_id?: string;
  target_name?: string;
  /** The scope's GitHub login, which is what identifies it there. */
  target_login?: string;
  target_kind?: 'Organization' | 'User';
  role?: Exclude<InstallationRole, 'none'>;
  system_role?: Exclude<SystemRole, 'none' | 'super_root'>;
  status: InvitationStatus;
  expires_at: string;
  created_by: PanelAccount;
  created_at: string;
  responded_at?: string;
  invite_url?: string;
}

export interface AddRootInvitationInput {
  login: string;
  expires_in_days: InvitationDays;
  /** Set only on the second, deliberate attempt after the invited identity declined. */
  acknowledge_declined?: boolean;
}

export type InvitationSort =
  | 'created_newest'
  | 'created_oldest'
  | 'expiry_soonest'
  | 'expiry_latest'
  | 'name_asc'
  | 'name_desc'
  | 'role_asc'
  | 'role_desc';

export interface InvitationPageRequest {
  cursor?: string;
  query: string;
  sort: InvitationSort;
  limit: number;
  roles: Exclude<InstallationRole, 'none'>[];
  statuses: InvitationStatus[];
}

export interface AccessDecision {
  id: string;
  actor: PanelAccount;
  action: string;
  summary: string;
  created_at: string;
}

export interface AddTargetInvitationInput {
  login: string;
  role: Exclude<InstallationRole, 'none' | 'owner'>;
  expires_in_days: InvitationDays;
  /** Set only on the second, deliberate attempt after the invited identity declined. */
  acknowledge_declined?: boolean;
}

export interface InvitationSignIn {
  token: string;
  action: 'accept' | 'decline';
}

export interface RepositoryCounts {
  total: number;
  enabled: number;
  disabled: number;
}

export interface PanelTarget {
  id: string;
  installation_id: string;
  type: 'Organization' | 'User';
  account: PanelAccount;
  repository_default_enabled: boolean;
  config_patch: ConfigPatch;
  inherited_config: ConfigValues;
  effective_config: ConfigValues;
  config_sources: ConfigSources;
  revision: number;
  repository_counts: RepositoryCounts;
  effective_role: InstallationRole;
  access_source: AccessSource;
  capabilities: PanelCapabilities;
  suspension_reason?: string;
}

export type OwnershipSource = 'personal' | 'organization_admin';
export type OwnershipStatus = 'fresh' | 'permission_pending' | 'error';

export interface OwnershipState {
  source: OwnershipSource;
  status: OwnershipStatus;
  detail?: string;
  synced_at: string;
  owner_count: number;
  stale: boolean;
}

export interface RootInstallation {
  id: string;
  installation_id: string;
  type: 'Organization' | 'User';
  account: PanelAccount;
  available: boolean;
  owned_by_viewer: boolean;
  repository_counts: RepositoryCounts;
  delivery_health: {
    failed: number;
    last_failure_at?: string;
  };
  ownership: OwnershipState;
}

export interface RootOverviewFailure {
  installation: PanelAccount;
  failure: DeliveryFailure;
}

export interface PendingCIRequest {
  id: string;
  repository_full_name: string;
  pull_request: number;
  head_sha: string;
  merge_method: 'merge' | 'squash' | 'rebase';
  required_checks_only: boolean;
  requester: string;
  schedule: 'active' | 'deferred';
  next_check_at: string;
  last_observed_state: string;
  requested_at: string;
  updated_at: string;
  revision: number;
}

/** What the storage subsystem reports about itself. */
export interface DatabaseStatus {
  state: DependencyState;
  /** The engine's own name, printed and never matched on. */
  engine: string;
  version: string;
  schema_version: number;
  size_bytes: number;
  latency_ms: number;
  /** Why the description is incomplete, absent when it is not. */
  detail?: string;
  connections: {
    open: number;
    in_use: number;
    idle: number;
    max: number;
    /** Callers that have waited for a free connection since the service started. */
    wait_count: number;
    wait_ms: number;
  };
}

export type DependencyState = 'healthy' | 'degraded' | 'unavailable';

export interface RootOverview {
  service: {
    status: 'healthy';
    version: string;
    service_host: string;
    uptime_seconds: number;
    storage: DependencyState;
    database: DatabaseStatus;
  };
  catalog: {
    installations: number;
    repositories: number;
    enabled_repositories: number;
  };
  ownership: {
    fresh: number;
    stale: number;
    permission_pending: number;
    error: number;
  };
  active_elevations: number;
  unread_security_events: number;
  recent_failures: RootOverviewFailure[];
  pending_ci: {
    active: PendingCIRequest[];
    deferred: PendingCIRequest[];
  };
}

export interface RootElevation {
  id: string;
  target_id: string;
  reason?: string;
  started_at: string;
  expires_at: string;
  ended_at?: string;
}

export interface RootElevationInput {
  acknowledged: true;
  reason?: string;
}

export interface RootRuntimeSettings {
  behavior_defaults: {
    deployment: ConfigValues;
    override: ConfigValues | null;
    effective: ConfigValues;
  };
  log_level: {
    deployment: string;
    override: string | null;
    effective: string;
  };
  reaction_poll_interval: {
    deployment_seconds: number;
    override_seconds: number | null;
    effective_seconds: number;
  };
  session_lifetime: {
    deployment_seconds: number;
    override_seconds: number | null;
    effective_seconds: number;
  };
  revision: number;
  updated_at?: string;
  updated_by?: PanelAccount;
  service: {
    version: string;
    uptime_seconds: number;
    storage: DependencyState;
    database: DatabaseStatus;
    listeners: { public: string; admin: string };
    public_paths: { panel: string; webhook: string };
    provider_endpoints: { api: string; authorize: string; token: string };
    credential_presence: { webhook: boolean; app: boolean; oauth: boolean };
  };
}

export interface RootRuntimeSettingsInput {
  bot_config: ConfigValues | null;
  log_level: string | null;
  reaction_poll_interval_seconds: number | null;
  session_ttl_seconds: number | null;
  expected_revision: number;
}

export interface SecurityNotification {
  id: string;
  installation: PanelAccount;
  actor: PanelAccount;
  elevation_id: string;
  audit_event_id: string;
  action: string;
  reason?: string;
  created_at: string;
  read_at?: string;
}

export interface NotificationPage {
  items: SecurityNotification[];
  next_cursor: string | null;
  total: number;
  unread: number;
}

export interface NotificationPageRequest {
  cursor?: string;
  limit: number;
}

export type RepositoryFileStatus = 'missing' | 'valid' | 'invalid' | 'bypassed';
export type RepositoryEnabledSource = 'target' | 'repository';

export interface RepositorySummary {
  id: string;
  name: string;
  full_name: string;
  private: boolean;
  default_branch: string;
  available: boolean;
  enabled_override: boolean | null;
  effective_enabled: boolean;
  enabled_source: RepositoryEnabledSource;
  config_override_count: number;
  config_file_status: RepositoryFileStatus;
  updated_at: string;
}

export interface RepositoryDetail {
  repository: RepositorySummary;
  config_patch: ConfigPatch;
  inherited_config: ConfigValues;
  effective_config: ConfigValues;
  config_sources: ConfigSources;
  config_file_patch: ConfigPatch;
  config_file_error?: string;
  ignore_repository_file: boolean;
  revision: number;
}

export type RepositorySort =
  | 'name_asc'
  | 'name_desc'
  | 'file_asc'
  | 'file_desc'
  | 'overrides_asc'
  | 'overrides_desc'
  | 'newest'
  | 'oldest';
export type RepositoryStateFilter = 'all' | 'enabled' | 'disabled';
export type RepositorySettingFilter =
  { mode: 'all' | 'custom' | 'none' } | { mode: 'keys'; keys: ConfigKey[] };

export interface RepositoryPageRequest {
  cursor?: string;
  query: string;
  sort: RepositorySort;
  limit: number;
  state: RepositoryStateFilter;
  files: RepositoryFileStatus[];
  setting: RepositorySettingFilter;
}

export interface TargetSettingsInput {
  repository_default_enabled: boolean;
  config_patch: ConfigPatch;
  expected_revision: number;
}

export interface RepositorySettingsInput {
  enabled_override: boolean | null;
  config_patch: ConfigPatch;
  ignore_repository_file: boolean;
  expected_revision: number;
}

export interface AuditEntry {
  id: string;
  category?: AuditCategory;
  installation?: PanelAccount;
  actor: PanelAccount;
  subject?: PanelAccount;
  elevation_id?: string;
  action: string;
  summary: string;
  repository_full_name?: string;
  created_at: string;
}

export type AuditCategory =
  'configuration' | 'access' | 'ownership' | 'elevation' | 'notification' | 'runtime';

export interface DeliveryFailure {
  id: string;
  installation?: PanelAccount;
  delivery_id: string;
  repository_full_name: string;
  event: string;
  stage: string;
  reason: string;
  retryable: boolean;
  occurred_at: string;
}

export type HistorySort =
  | 'newest'
  | 'oldest'
  | 'actor_asc'
  | 'actor_desc'
  | 'target_asc'
  | 'target_desc'
  | 'change_asc'
  | 'change_desc'
  | 'status_asc'
  | 'status_desc'
  | 'repository_asc'
  | 'repository_desc';
export type AuditScope = 'all' | 'account' | 'repositories';
export type AuditChange = 'all' | 'enablement' | 'repository' | 'account';
export type FailureKind = 'all' | 'retryable' | 'permanent';

export interface HistoryRequest {
  cursor?: string;
  query: string;
  sort: HistorySort;
  limit: number;
}

export interface AuditHistoryRequest extends HistoryRequest {
  scope?: AuditScope;
  change?: AuditChange;
  categories?: AuditCategory[];
}

export interface FailureHistoryRequest extends HistoryRequest {
  kind: FailureKind;
}

export interface Page<T> {
  items: T[];
  next_cursor: string | null;
  total: number;
}

export interface PanelErrorBody {
  error: {
    code: string;
    message: string;
  };
}
