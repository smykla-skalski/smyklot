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

export type PanelRole = 'none' | 'viewer' | 'editor' | 'admin' | 'owner';
export type PanelUserStatus = 'active' | 'banned' | 'removed';
export type AccessSource = 'root' | 'global' | 'target' | 'suspended' | 'denied';

export interface PanelCapabilities {
  read: boolean;
  write: boolean;
  manage_target_users: boolean;
  manage_global_users: boolean;
  manage_owners: boolean;
}

export interface PanelViewer {
  account: PanelAccount;
  root: boolean;
  status: PanelUserStatus;
  global_role: PanelRole;
  capabilities: PanelCapabilities;
  target_count: number;
}

export interface TargetUserAccess {
  role: Exclude<PanelRole, 'owner'> | null;
  suspended: boolean;
  suspension_reason?: string;
  revision: number;
  updated_at?: string;
  effective_role: PanelRole;
  source: AccessSource;
  capabilities: PanelCapabilities;
}

export interface PanelUser {
  account: PanelAccount;
  root: boolean;
  status: PanelUserStatus;
  global_role: PanelRole;
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
  'name_asc' | 'name_desc' | 'updated_newest' | 'updated_oldest' | 'login_newest' | 'login_oldest';
export type PanelUserListStatus = 'active' | 'banned' | 'suspended';

export interface PanelUserPageRequest {
  cursor?: string;
  query: string;
  sort: PanelUserSort;
  limit: number;
  roles: PanelRole[];
  statuses: PanelUserListStatus[];
}

export interface AddGlobalUserInput {
  login: string;
  role: PanelRole;
  target_id: string;
}

export interface UpdateGlobalUserInput {
  global_role: PanelRole;
  status: PanelUserStatus;
  ban_reason?: string;
  expected_revision: number;
}

export interface AddTargetUserInput {
  login: string;
  role: Exclude<PanelRole, 'none' | 'owner'>;
}

export interface UpdateTargetUserInput {
  role: Exclude<PanelRole, 'owner'> | null;
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
  role: Exclude<PanelRole, 'none'>;
  status: InvitationStatus;
  expires_at: string;
  created_by: PanelAccount;
  created_at: string;
  responded_at?: string;
  invite_url?: string;
}

export type InvitationSort =
  | 'created_newest'
  | 'created_oldest'
  | 'expiry_soonest'
  | 'expiry_latest'
  | 'name_asc'
  | 'name_desc';

export interface InvitationPageRequest {
  cursor?: string;
  query: string;
  sort: InvitationSort;
  limit: number;
  roles: Exclude<PanelRole, 'none'>[];
  statuses: InvitationStatus[];
}

export interface AccessDecision {
  id: string;
  actor: PanelAccount;
  action: string;
  summary: string;
  created_at: string;
}

export interface AddGlobalInvitationInput {
  login: string;
  role: Exclude<PanelRole, 'none'>;
  target_id: string;
  expires_in_days: InvitationDays;
}

export interface AddTargetInvitationInput {
  login: string;
  role: Exclude<PanelRole, 'none' | 'owner'>;
  expires_in_days: InvitationDays;
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
  effective_role: PanelRole;
  access_source: AccessSource;
  capabilities: PanelCapabilities;
  suspension_reason?: string;
}

export type RepositoryFileStatus = 'missing' | 'valid' | 'invalid' | 'bypassed';
export type RepositoryEnabledSource = 'target' | 'repository';

export interface RepositorySummary {
  id: string;
  name: string;
  full_name: string;
  private: boolean;
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

export type RepositorySort = 'name_asc' | 'name_desc' | 'newest' | 'oldest';
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
  actor: PanelAccount;
  action: string;
  summary: string;
  repository_full_name?: string;
  created_at: string;
}

export interface DeliveryFailure {
  id: string;
  delivery_id: string;
  repository_full_name: string;
  event: string;
  stage: string;
  reason: string;
  retryable: boolean;
  occurred_at: string;
}

export type HistorySort = 'newest' | 'oldest';
export type AuditScope = 'all' | 'account' | 'repositories';
export type FailureKind = 'all' | 'retryable' | 'permanent';

export interface HistoryRequest {
  cursor?: string;
  query: string;
  sort: HistorySort;
  limit: number;
}

export interface AuditHistoryRequest extends HistoryRequest {
  scope: AuditScope;
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
