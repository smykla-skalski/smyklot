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
  lifecycle: 'armed' | 'merged' | 'cancelled' | 'superseded';
  schedule: 'active' | 'deferred';
  next_check_at: string;
  next_check_trigger: PendingCITrigger;
  last_observed_state: string;
  reason: string;
  requested_at: string;
  updated_at: string;
  finished_at?: string;
  cleanup_pending: boolean;
  cleanup_error?: string;
  revision: number;
}

export type PendingCITrigger =
  'command' | 'webhook' | 'fallback' | 'quiet_period' | 'manual' | 'cleanup';

export interface PendingCIEvent {
  id: string;
  kind:
    | 'armed'
    | 'superseded'
    | 'wake_received'
    | 'reconciliation_started'
    | 'checks_observed'
    | 'merge_started'
    | 'finished'
    | 'cleanup_retry'
    | 'cleanup_completed';
  trigger: PendingCITrigger;
  event_name?: string;
  event_key?: string;
  delivery_id?: string;
  state?: string;
  summary: string;
  created_at: string;
}

export interface PendingCIDetail {
  request: PendingCIRequest;
  events: PendingCIEvent[];
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
    recent: PendingCIRequest[];
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
  merge_after_ci_quiet_period: {
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
  merge_after_ci_quiet_period_seconds: number | null;
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

/** How far Smyklot has got with moving a repository's file to TOML. */
export type ConfigMigrationState = 'none' | 'proposed' | 'declined' | 'blocked';
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
  config_file_path?: string;
  config_file_superseded?: string[];
  config_migration: ConfigMigrationState;
  config_migration_pr?: number;
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

/** One label an installation expects its repositories to carry. */
export interface SyncLabel {
  name: string;
  color: string;
  /**
   * Absent means "leave whatever the repository wrote". Present and empty means
   * "clear it". They are different requests, which is why this is optional
   * rather than a string that is sometimes blank.
   */
  description?: string;
}

/**
 * One ruleset an installation expects its repositories to enforce.
 *
 * Values rather than optionals wherever GitHub has no third state, because a
 * ruleset is written by replacement: the request defines the whole object and
 * what it does not carry is not enforced. There is no request meaning "leave
 * this rule as it is", so a form that could express one would be promising
 * something the endpoint cannot do.
 */
export interface SyncRuleset {
  name: string;
  /** branch or tag. */
  target: string;
  /** active, evaluate or disabled. */
  enforcement: string;
  conditions: SyncRulesetConditions;
  bypass_actors?: SyncRulesetBypassActor[];
  rules: SyncRulesetRules;
}

/** Which refs a ruleset applies to, as patterns rather than branch names. */
export interface SyncRulesetConditions {
  include?: string[];
  exclude?: string[];
}

/** Somebody who may step around a ruleset. */
export interface SyncRulesetBypassActor {
  actor_id: number;
  /** Integration, OrganizationAdmin, RepositoryRole, Team or DeployKey. */
  actor_type: string;
  /** always, pull_request or exempt. */
  bypass_mode: string;
}

/** What a ruleset enforces. */
export interface SyncRulesetRules {
  creation?: boolean;
  deletion?: boolean;
  non_fast_forward?: boolean;
  required_linear_history?: boolean;
  required_signatures?: boolean;
  update?: SyncRulesetUpdateRule;
  pull_request?: SyncRulesetPullRequestRule;
  required_status_checks?: SyncRulesetStatusChecksRule;
  code_scanning?: SyncRulesetCodeScanningRule;
}

export interface SyncRulesetUpdateRule {
  update_allows_fetch_and_merge?: boolean;
}

export interface SyncRulesetPullRequestRule {
  required_approving_review_count?: number;
  dismiss_stale_reviews_on_push?: boolean;
  require_code_owner_review?: boolean;
  require_last_push_approval?: boolean;
  required_review_thread_resolution?: boolean;
  /** merge, squash or rebase. GitHub needs at least one. */
  allowed_merge_methods: string[];
}

export interface SyncRulesetStatusChecksRule {
  /** GitHub refuses an empty list, so a rule with no check is no rule. */
  required_status_checks: SyncRulesetStatusCheck[];
  strict_required_status_checks_policy?: boolean;
  do_not_enforce_on_create?: boolean;
}

export interface SyncRulesetStatusCheck {
  context: string;
  /** Pins the check to the App reporting it. Absent leaves it unpinned. */
  integration_id?: number;
}

export interface SyncRulesetCodeScanningRule {
  code_scanning_tools: SyncRulesetCodeScanningTool[];
}

export interface SyncRulesetCodeScanningTool {
  tool: string;
  alerts_threshold: string;
  security_alerts_threshold: string;
}

/** An installation's label sync configuration, as saved. */
export interface SyncConfig {
  kind: string;
  enabled: boolean;
  labels: SyncLabel[];
  allow_removal: boolean;
  excludes: string[];
  revision: number;
  updated_by: string;
  updated_at: string;
  digest: string;
  /**
   * The stored configuration as it is, whatever kind it belongs to. The typed
   * fields above describe labels, which is the kind this panel has a form for;
   * every other kind travels here untouched.
   */
  document: Record<string, unknown>;
  /**
   * The stored document could not be read, so the lists above are empty because
   * nothing came out of them - not because nothing is configured. Saving over
   * this would send back the emptiness rather than the labels the row holds, so
   * the view refuses.
   */
  unreadable: boolean;
  /**
   * What this kind needs and the installation has not granted, or empty. A
   * switch that is on means nothing without the permission behind it: the sweep
   * leaves the kind out, nothing is planned and nothing fails, and an empty
   * plan list looks exactly like a sweep that has not come round yet.
   */
  unavailable: string;
}

/** What a save sends. The revision is what it believes it is replacing. */
export interface SyncConfigInput {
  enabled: boolean;
  expected_revision: number;
  /**
   * The label set and what may be done to it, for the labels kind. Optional
   * because they describe that kind alone: a settings save that had to send
   * empty ones would be sending three values nothing reads.
   */
  labels?: SyncLabel[];
  allow_removal?: boolean;
  excludes?: string[];
  /**
   * The kind's own document, for every kind but labels. Labels travel in the
   * typed fields above because the panel has a form built out of them; anything
   * else is sent as it is, so a kind is configurable before it has one.
   */
  document?: Record<string, unknown>;
}

/** One change a plan would make. */
export interface SyncAction {
  repository: string;
  kind: string;
  operation: 'create' | 'update' | 'delete';
  subject: string;
  before?: string;
  after?: string;
  state: 'pending' | 'applied' | 'failed' | 'skipped';
  error?: string;
  blocker?: string;
}

/** A computed answer to "what would change", and the unit somebody approves. */
export interface SyncPlan {
  id: string;
  trigger: string;
  state: 'computed' | 'approved' | 'applying' | 'applied' | 'failed' | 'stale' | 'expired';
  digest: string;
  counts: { create: number; update: number; delete: number };
  actions: SyncAction[];
  computed_at: string;
  expires_at: string;
  approved_at?: string;
  finished_at?: string;
}
