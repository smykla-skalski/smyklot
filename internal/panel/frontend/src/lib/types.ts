import type { ArrayStrategy } from '#lib/merge.js';

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
  affected_installations?: number;
  affected_items?: number;
  affected_policies?: number;
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
  pending_ci_mode_default: PendingCIMode;
  pending_ci_branch_patterns_default: PendingCIBranchPatterns;
  pending_ci_quiet_period_seconds_override: number | null;
  /**
   * What this installation would use if it set nothing: what the running
   * service resolved. Never null, so the panel prefills the deployment's own
   * answer rather than a number typed into a component.
   */
  pending_ci_quiet_period_seconds_inherited: number;
  /** How often this installation's repositories have their file lists checked. */
  path_index_interval_seconds_override: number | null;
  path_index_interval_seconds_inherited: number;
  pending_ci_permissions: PendingCIPermissions;
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

export type QueueWorkload =
  | 'webhook_delivery'
  | 'pending_ci'
  | 'pending_ci_gate'
  | 'catalog_refresh'
  | 'reaction_scan'
  | 'config_migration'
  | 'sync_scan'
  | 'sync_apply'
  | 'path_refresh'
  | 'delivery_cleanup'
  | 'auth_cleanup'
  | 'schedule_change';
export type QueueState =
  | 'awaiting_approval'
  | 'scheduled'
  | 'blocked'
  | 'ready'
  | 'running'
  | 'retrying'
  | 'succeeded'
  | 'failed'
  | 'cancelled'
  | 'superseded';
export type QueuePriority = 'low' | 'normal' | 'high' | 'urgent';
export type QueueActionType = 'run_now' | 'next_window' | 'schedule_at' | 'set_priority' | 'cancel';

interface QueueItemBase {
  id: string;
  lane: 'webhook' | 'pending_ci' | 'maintenance';
  target_id?: string;
  repository_id?: string;
  source_kind?: string;
  source_id?: string;
  title: string;
  summary?: string;
  state: QueueState;
  priority: QueuePriority;
  priority_overridden: boolean;
  window_mode: 'respect' | 'bypass';
  immediate: boolean;
  profile_id?: string;
  profile_name?: string;
  profile_timezone?: string;
  not_before: string;
  cadence_anchor_at?: string;
  eligible_at: string;
  estimated_start_at?: string;
  work_ahead: number;
  blocked_reason?: string;
  progress_current: number;
  progress_total: number;
  attempt: number;
  lease_expires_at?: string;
  requested_by?: string;
  reason?: string;
  revision: number;
  created_at: string;
  updated_at: string;
  started_at?: string;
  finished_at?: string;
  actions?: QueueActionType[];
}

export type QueueItem =
  | (QueueItemBase & {
      kind: 'webhook_delivery';
      details?: { delivery_id?: number; event?: string };
    })
  | (QueueItemBase & {
      kind: 'pending_ci';
      details?: { pull_request?: number; head_sha?: string };
    })
  | (QueueItemBase & {
      kind: 'sync_apply';
      details?: { create?: number; update?: number; delete?: number };
    })
  | (QueueItemBase & {
      kind: 'schedule_change';
      details?: { policy_kind?: QueueWorkload };
    })
  | (QueueItemBase & {
      kind: Exclude<
        QueueWorkload,
        'webhook_delivery' | 'pending_ci' | 'sync_apply' | 'schedule_change'
      >;
      details?: Record<string, unknown>;
    });

export interface QueueEvent {
  id: number;
  item_id: string;
  actor_id?: string;
  actor: string;
  kind: string;
  state: QueueState;
  summary: string;
  details?: Record<string, unknown>;
  created_at: string;
}

export interface QueueDetail {
  item: QueueItem;
  events: QueueEvent[];
}

export interface QueuePage {
  items: QueueItem[];
  next_offset: number;
  total: number;
  facets: {
    targets: string[];
    repositories: string[];
    profiles: string[];
    states: QueueState[];
    workloads: QueueWorkload[];
    priorities: QueuePriority[];
  };
}

export interface QueueActionInput {
  type: QueueActionType;
  expected_revision: number;
  reason?: string;
  at?: string;
  outside_window?: boolean;
  priority?: QueuePriority;
}

export interface QueueSchedulePreview {
  item_revision: number;
  requested_at: string;
  eligible_at: string;
  outside_window: boolean;
  profile_id?: string;
  profile_name?: string;
  profile_timezone?: string;
}

export interface ScheduleProfile {
  id: string;
  target_id?: string;
  name: string;
  timezone: string;
  system: boolean;
  archived_at?: string;
  revision: number;
  affected_installations?: number;
  affected_items?: number;
  affected_policies?: number;
  windows: Array<{ weekday: number; start_minute: number; end_minute: number }>;
  exceptions: Array<{
    date: string;
    closed: boolean;
    start_minute?: number;
    end_minute?: number;
  }>;
}

export interface QueuePolicy {
  kind: QueueWorkload;
  target_id?: string;
  enabled: boolean;
  cadence: number;
  profile_id: string;
  default_priority: QueuePriority;
  retry_delay: number;
  retention?: number;
  approval_ttl?: number;
  configuration?: Record<string, unknown>;
  revision: number;
  updated_at: string;
}

export interface SchedulePolicySet {
  current: QueuePolicy[];
  deployment_defaults: QueuePolicy[];
  overrides: QueuePolicy[];
  effective: QueuePolicy[];
}

export interface QueuePolicyStatus {
  kind: QueueWorkload;
  target_id?: string;
  last_run_at?: string;
  last_state?: QueueState;
  next_eligibility_at?: string;
  estimated_start_at?: string;
  work_ahead: number;
  current_state?: QueueState;
  current_queue_item_id?: string;
}

export interface RootJobPolicies {
  policies: QueuePolicy[];
  policy_set: SchedulePolicySet;
  statuses: QueuePolicyStatus[];
}

export interface ScheduleRequest {
  id: string;
  target_id: string;
  kind: QueueWorkload;
  state: 'pending' | 'approved' | 'rejected' | 'withdrawn' | 'stale';
  base_revision: number;
  base_target_id?: string;
  profile_id?: string;
  custom_profile?: ScheduleProfile;
  cadence: number;
  default_priority: QueuePriority;
  configuration?: Record<string, unknown>;
  reason: string;
  requested_by: string;
  revision: number;
  created_at: string;
  updated_at: string;
}

export interface TargetSchedules {
  policies: SchedulePolicySet;
  profiles: ScheduleProfile[];
  statuses: QueuePolicyStatus[];
}

export interface ScheduleProfileInput {
  name: string;
  timezone: string;
  windows: ScheduleProfile['windows'];
  exceptions: ScheduleProfile['exceptions'];
  expected_revision: number;
}

export interface QueuePolicyInput {
  enabled: boolean;
  cadence_seconds: number;
  profile_id: string;
  default_priority: QueuePriority;
  retry_delay_seconds: number;
  retention_seconds?: number;
  approval_lifetime_seconds?: number;
  configuration?: Record<string, unknown>;
  expected_revision: number;
}

export interface ScheduleRequestInput {
  kind: QueueWorkload;
  base_revision: number;
  profile_id?: string;
  custom_profile?: ScheduleProfile;
  cadence_seconds: number;
  default_priority: QueuePriority;
  configuration?: Record<string, unknown>;
  reason: string;
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
  /** How often a repository's file list is checked for changes. */
  path_index_interval: {
    deployment_seconds: number;
    override_seconds: number | null;
    effective_seconds: number;
    /**
     * The largest value this setting accepts. Sent rather than known here: the
     * bound is enforced by the service and by a CHECK constraint, and a third
     * copy typed into a component is the one that goes stale silently.
     */
    max_seconds: number;
  };
  session_lifetime: {
    deployment_seconds: number;
    override_seconds: number | null;
    effective_seconds: number;
  };
  revision: number;
  checkpoint_id?: string;
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
  path_index_interval_seconds: number | null;
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

export type PendingCIMode = 'labels' | 'checks';
export type PendingCIReadiness = 'ready' | 'provisioning' | 'draining' | 'blocked';

export interface PendingCIBranchPatterns {
  include: string[];
  exclude: string[];
}

export interface PendingCIPermissions {
  checks_write: boolean;
  administration_write: boolean;
  merge_queues_read: boolean;
  commit_statuses_read: boolean;
}

export interface PendingCIGate {
  desired_mode: PendingCIMode;
  effective_mode: 'none' | PendingCIMode;
  readiness: PendingCIReadiness;
  reason: string;
  app_id?: number;
  ruleset_id?: number;
}

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
  pending_ci_mode: PendingCIMode;
  pending_ci_mode_source: 'target' | 'repository';
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
  pending_ci_mode_override: PendingCIMode | null;
  pending_ci_mode_inherited: PendingCIMode;
  pending_ci_branch_patterns_override: PendingCIBranchPatterns | null;
  pending_ci_branch_patterns_inherited: PendingCIBranchPatterns;
  pending_ci_quiet_period_seconds_override: number | null;
  /**
   * What this repository would use if it set nothing, resolved through every
   * level above it. Never null: a page that had to invent a prefill invented
   * the same one whatever the deployment ran with.
   */
  pending_ci_quiet_period_seconds_inherited: number;
  /** How often this repository's file list is checked; null inherits. */
  path_index_interval_seconds_override: number | null;
  path_index_interval_seconds_inherited: number;
  pending_ci_gate?: PendingCIGate;
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

/** A complete installation-defaults document in one atomic settings save. */
export interface InstallationTargetSettingsInput {
  repository_default_enabled: boolean;
  pending_ci_mode_default: PendingCIMode;
  pending_ci_branch_patterns_default: PendingCIBranchPatterns;
  pending_ci_quiet_period_seconds_override: number | null;
  path_index_interval_seconds_override: number | null;
  config_patch: ConfigPatch;
  expected_revision: number;
}

/** A complete repository-settings document in one atomic settings save. */
export interface InstallationRepositorySettingsInput {
  repository_id: string;
  enabled_override: boolean | null;
  pending_ci_mode_override: PendingCIMode | null;
  pending_ci_branch_patterns_override: PendingCIBranchPatterns | null;
  pending_ci_quiet_period_seconds_override: number | null;
  path_index_interval_seconds_override: number | null;
  config_patch: ConfigPatch;
  ignore_repository_file: boolean;
  expected_revision: number;
}

export type InstallationSyncConfigSettingsInput =
  | {
      kind: 'labels';
      enabled: boolean;
      labels: SyncLabel[];
      allow_removal: boolean;
      excludes: string[];
      expected_revision: number;
    }
  | {
      kind: Exclude<SyncKind, 'labels'>;
      enabled: boolean;
      document: Record<string, unknown>;
      expected_revision: number;
    };

export interface InstallationSyncOverrideSettingsInput {
  repository_id: string;
  kind: SyncKind;
  enabled: boolean | null;
  document: Record<string, unknown>;
  expected_revision: number;
}

export interface InstallationSettingsBatchInput {
  target?: InstallationTargetSettingsInput;
  repositories?: InstallationRepositorySettingsInput[];
  sync_configs?: InstallationSyncConfigSettingsInput[];
  sync_overrides?: InstallationSyncOverrideSettingsInput[];
}

export interface InstallationTargetSettingsState extends Omit<
  InstallationTargetSettingsInput,
  'expected_revision'
> {
  target_id: string;
  revision: number;
}

export interface InstallationRepositorySettingsState extends Omit<
  InstallationRepositorySettingsInput,
  'expected_revision'
> {
  revision: number;
}

export interface InstallationSyncConfigSettingsState {
  target_id: string;
  kind: SyncKind;
  enabled: boolean;
  document: Record<string, unknown>;
  revision: number;
}

export interface InstallationSyncOverrideSettingsState {
  target_id: string;
  repository_id: string;
  kind: SyncKind;
  enabled: boolean | null;
  document: Record<string, unknown>;
  revision: number;
}

export interface InstallationSettingsBatchResponse {
  checkpoint_id?: string;
  target?: InstallationTargetSettingsState;
  repositories?: InstallationRepositorySettingsState[];
  sync_configs?: InstallationSyncConfigSettingsState[];
  sync_overrides?: InstallationSyncOverrideSettingsState[];
}

export type SettingsCheckpointItemKind =
  'target' | 'repository' | 'sync_config' | 'sync_override' | 'runtime';

export interface SettingsCheckpointState {
  document: Record<string, unknown>;
  digest: string;
  revision: number;
}

export interface SettingsCheckpointIncompatibility {
  code: string;
  reason: string;
}

export type SettingsCheckpointRestoreSide = 'before' | 'after';

export interface SettingsCheckpointSide {
  /** False only when this checkpoint never captured this side, as with a baseline's Before. */
  available: boolean;
  /** Null with available=true is a captured absence for an optional Sync resource. */
  state: SettingsCheckpointState | null;
  differs: boolean;
  restorable: boolean;
  incompatibility?: SettingsCheckpointIncompatibility;
}

export interface SettingsCheckpointItem {
  kind: SettingsCheckpointItemKind;
  repository_id?: string;
  repository_full_name?: string;
  sync_kind?: SyncKind;
  document_version: number;
  before: SettingsCheckpointSide;
  after: SettingsCheckpointSide;
  current: SettingsCheckpointState | null;
  changed: boolean;
}

export interface SettingsCheckpoint {
  id: string;
  action:
    | 'installation.settings.saved'
    | 'installation.settings.restored'
    | 'installation.settings.baseline'
    | 'runtime.settings.saved'
    | 'runtime.settings.restored'
    | 'runtime.settings.baseline';
  actor: PanelAccount;
  restored_from_id?: string;
  restored_side?: SettingsCheckpointRestoreSide;
  created_at: string;
  affected_kinds: SettingsCheckpointItemKind[];
  items: SettingsCheckpointItem[];
}

export interface SettingsRestoreSelection {
  kind: SettingsCheckpointItemKind;
  repository_id?: string;
  sync_kind?: SyncKind;
  expected_revision: number;
}

export interface SettingsRestoreInput {
  state: SettingsCheckpointRestoreSide;
  selections: SettingsRestoreSelection[];
}

export type InstallationSettingsConflict =
  | {
      resource: 'target';
      target_id: string;
      expected_revision: number;
      actual_revision: number;
      latest?: InstallationTargetSettingsState;
    }
  | {
      resource: 'repository';
      target_id: string;
      repository_id: string;
      expected_revision: number;
      actual_revision: number;
      latest?: InstallationRepositorySettingsState;
    }
  | {
      resource: 'sync_config';
      target_id: string;
      kind: SyncKind;
      expected_revision: number;
      actual_revision: number;
      latest?: InstallationSyncConfigSettingsState;
    }
  | {
      resource: 'sync_override';
      target_id: string;
      repository_id: string;
      kind: SyncKind;
      expected_revision: number;
      actual_revision: number;
      latest?: InstallationSyncOverrideSettingsState;
    };

export interface AuditEntry {
  id: string;
  category?: AuditCategory;
  target_id?: string;
  installation?: PanelAccount;
  actor: PanelAccount;
  subject?: PanelAccount;
  elevation_id?: string;
  action: string;
  summary: string;
  settings_checkpoint_id?: string;
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
export type AuditChange = 'all' | 'repository' | 'account' | 'sync';
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
    kind?: SyncKind;
    conflicts?: InstallationSettingsConflict[];
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

/**
 * One file every repository is expected to carry, and what it should say.
 *
 * The content is configuration rather than a file in another repository, which
 * is what stops a template going missing between the place it is kept and the
 * repository it is written to.
 */
export interface SyncFile {
  path: string;
  content: string;
}

/**
 * How one repository composes its copy of one template.
 *
 * There is deliberately no type for the whole adjustment document. The pane
 * reads it as an open record and writes it back the same way, so a key a later
 * version of the service adds survives a save by somebody running this one.
 */
export interface SyncFileMerge {
  path: string;
  /** deep-merge, shallow-merge or markdown. Empty lets the extension decide. */
  strategy?: string;
  overrides?: Record<string, unknown>;
  /**
   * What happens to the lists the template and the overrides both have.
   *
   * Ordered rather than keyed by path, because ranging a map gave two rules on
   * one document no order and the file a repository ended up with depended on
   * nothing anybody could see.
   */
  arrays?: SyncArrayRule[];
  /** Only meaningful beside a list rule: a list with no rule is replaced whole. */
  deduplicate?: boolean;
  /** How a Markdown template is edited. Exclusive with the three above. */
  sections?: SyncSection[];
}

/** What to do with the list at one path. */
export interface SyncArrayRule {
  path: string;
  /* The vocabulary rather than `string`: `filemerge` refuses a rule spelling
     anything else, and the panel used to hand a Select's raw value straight
     through - so the one place a typo could arrive is also the one place
     nothing checked. */
  strategy: ArrayStrategy;
}

/** A literal substitution inside a section. */
export interface SyncPatch {
  find: string;
  /** Empty removes the text found. */
  replace: string;
}

/** One operation on a Markdown document. */
export interface SyncSection {
  /** before, after, replace, delete, patch, append or prepend. */
  action: string;
  /**
   * Written the way the document writes it, marks and all: `## Usage`. The
   * marks are how `## Usage` is told from `### Usage`.
   */
  heading?: string;
  /** Which one, counting from one, where a heading appears more than once. */
  occurrence?: number;
  content?: string;
  patches?: SyncPatch[];
}

/** What one repository says about one kind of sync. */
export interface SyncOverride {
  kind: string;
  /** null where the repository inherits the installation's answer. */
  enabled: boolean | null;
  document: Record<string, unknown>;
  revision: number;
  updated_by?: string;
  updated_at?: string;
  /** A stored document this version cannot read, so nothing here was shown. */
  unreadable: boolean;
  /**
   * Why this kind is not being synced here, and when the planner last found
   * that. Absent where nothing is wrong.
   */
  problem?: string;
  problem_at?: string;
}

/** What a repository's answer is saved as. */
/**
 * Every path this installation's repositories are known to hold, and how many
 * hold each.
 *
 * A picture rather than a fact - whatever each default branch held when it was
 * last looked at. A path it does not know is still a path the finder accepts.
 */
export interface SyncPathIndex {
  paths: { path: string; repositories: number }[];
  /** How many repositories contributed to it. */
  repositories: number;
  /**
   * When the STALEST of those was read. Absent before anything has been.
   *
   * The union is only as current as its oldest member, which is the same
   * reading `partial` takes: one repository nobody has looked at in a week is
   * a week-old answer, whatever the others say.
   */
  observed_at?: string;
  /**
   * Whether GitHub declined to list one of those repositories whole, even
   * after the listing was divided around its refusal. Nothing drops a path on
   * purpose, so this is the only way the list can be short.
   */
  partial?: boolean;
}

/**
 * One repository's answer, in a list of every repository's.
 *
 * The name travels with it because the page reading this is about a file rather
 * than about a repository: "three repositories adjust renovate.json" is what
 * this list answers, and ids would mean a request per row to turn each one back
 * into a word.
 */
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

/** The kinds sync manages, in the order every surface lists them. */
export const SYNC_KINDS = ['labels', 'settings', 'rulesets', 'files'] as const;
export type SyncKind = (typeof SYNC_KINDS)[number];

/**
 * One repository's answer for one kind: quiet when in step, a count when a
 * plan would change it, a refusal with its reason on the repository, or
 * switched off there.
 */
export interface SyncCell {
  state: 'in_step' | 'pending' | 'refused' | 'off';
  /** Pending only: how many of the plan's changes land here for this kind. */
  changes?: number;
}

/** One repository on the board, with its per-kind cells. */
export interface SyncRepositoryStatus {
  repository: string;
  cells: Record<SyncKind, SyncCell>;
  /** Pending only: how many of the changes are removals. */
  removals?: number;
  /** Refused only: the repository's own word about why, in words. */
  reason?: string;
}

/**
 * The fleet: every repository sync covers and where each one stands. What the
 * overview's board, legend, out-of-step list and kind strips are drawn from.
 */
export interface SyncStatus {
  checked_at: string;
  repositories: SyncRepositoryStatus[];
}

/**
 * What the files pages need beyond the document: the path index the finder
 * matches over, and every repository adjustment of every template, so the
 * list can count adjusters and the file page can show them.
 */
export interface SyncFilesContext {
  /** How many repositories the installation covers. */
  repositories: number;
  /** How many of them file sync reaches - the rest switched it off. */
  covered: number;
  /** Every path any repository holds, deduped, with how many hold it. */
  known_paths: Array<{ path: string; repositories: number }>;
  merges: SyncFileMergeEntry[];
}

/** One repository's adjustment of one template. */
export interface SyncFileMergeEntry {
  repository: string;
  repository_id: string;
  path: string;
  /** The stored merge, whole - strategy, overrides, arrays, sections. */
  merge: Record<string, unknown>;
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
  execution_stage: string;
  queue_item?: QueueItem;
}

export interface SyncRunNowResponse {
  status: 'scan_queued' | 'approval_required' | 'already_running' | 'plan_dispatched';
  plan?: SyncPlan;
  queue_item?: QueueItem;
}
