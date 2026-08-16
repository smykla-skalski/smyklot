import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import type { Server as HttpServer, IncomingMessage, ServerResponse } from 'node:http';
import { dirname, resolve } from 'node:path';
import type { Duplex } from 'node:stream';
import { fileURLToPath } from 'node:url';
import type { Connect, Plugin } from 'vite';

import { rewriteMockHtml } from './mock-html.ts';

import type {
  AuditEntry,
  AccessDecision,
  AddRootInvitationInput,
  AddTargetInvitationInput,
  AddTargetUserInput,
  ConfigKey,
  ConfigPatch,
  ConfigSources,
  ConfigValues,
  DatabaseStatus,
  DeliveryFailure,
  Page,
  PanelAccount,
  PanelInvitation,
  PanelTarget,
  PanelUser,
  PendingCIDetail,
  PendingCIRequest,
  InstallationRole,
  TargetUserAccess,
  RepositoryDetail,
  RepositorySettingsInput,
  RepositorySummary,
  RootElevation,
  RootElevationInput,
  RootInstallation,
  RootOverview,
  RootPanelUser,
  RootRuntimeSettings,
  RootRuntimeSettingsInput,
  SecurityNotification,
  TargetSettingsInput,
  UpdateTargetUserInput,
  UpdateRootUserInput,
  InvitationDays,
  InvitationStatus,
} from '../src/lib/types.ts';
import { canonicalStringify, PREF_DEFAULTS } from '../src/lib/preferences-sync.ts';
import { parseInvitationToken, parsePanelRoute } from '../src/lib/routes.ts';

type DevHttpServer = HttpServer;
const BASE = '';
const DEFAULT_PAGE_SIZE = 20;

const DEFAULT_CONFIG: ConfigValues = {
  quiet_success: false,
  quiet_reactions: false,
  quiet_pending: false,
  allowed_commands: [],
  command_aliases: {},
  command_prefix: '/',
  disable_mentions: false,
  disable_bare_commands: false,
  disable_unapprove: false,
  disable_reactions: false,
  disable_deleted_comments: false,
  allow_self_approval: false,
};

const VIEWER: PanelAccount = {
  id: '1001',
  provider: 'github:https://api.github.com',
  subject_id: '1001',
  login: 'bart',
  display_name: 'Bart Smykla',
  avatar_url: null,
};

/**
 * Stands in for the organization roster the service reads from GitHub.
 *
 * Deliberately holds names that overlap on different parts - a login starting
 * with the query, a display name starting with it, and one that only contains it
 * - so the ordering the panel applies is visible while working on it.
 */
const MOCK_ORGANIZATION_ROSTER: PanelAccount[] = [
  { login: 'marta-w', display_name: 'Marta Wisniewska' },
  { login: 'marek', display_name: 'Marek Kowalski' },
  { login: 'kasia', display_name: 'Katarzyna Marcinkowska' },
  { login: 'tomasz', display_name: 'Tomasz Nowak' },
  { login: 'piotr-z', display_name: 'Piotr Zielinski' },
  { login: 'agnieszka', display_name: 'Agnieszka Lewandowska' },
  { login: 'jakub', display_name: 'Jakub Wojcik' },
  { login: 'zofia', display_name: 'Zofia Kaminska' },
  { login: 'michal', display_name: 'Michal Dabrowski' },
  { login: 'ola', display_name: 'Aleksandra Mazur' },
].map((person, index) => ({
  id: `roster-${index + 1}`,
  provider: 'github:https://api.github.com',
  subject_id: `${9000 + index}`,
  login: person.login,
  display_name: person.display_name,
  avatar_url: null,
}));

const OWNER_CAPABILITIES = {
  read: true,
  write: true,
  manage_target_users: true,
};

const ROOT_READ_CAPABILITIES = {
  read: true,
  write: false,
  manage_target_users: false,
};

class MockApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = 'MockApiError';
  }
}

interface MockRepository {
  detail: RepositoryDetail;
  filePatch: ConfigPatch;
}

interface MockTarget {
  value: PanelTarget;
  repositories: MockRepository[];
  audit: AuditEntry[];
  failures: DeliveryFailure[];
}

interface MockInvitation extends PanelInvitation {
  token: string;
}

interface MockState {
  signedIn: boolean;
  forceFailure: boolean;
  /** `?scenario=empty`: an account with nothing installed, without losing the seed. */
  hideTargets: boolean;
  targets: MockTarget[];
  users: PanelUser[];
  targetAccess: Map<string, Map<string, TargetUserAccess>>;
  invitations: MockInvitation[];
  invitationCounter: number;
  elevationCounter: number;
  elevations: Map<string, RootElevation>;
  notifications: SecurityNotification[];
  pendingCI: PendingCIRequest[];
  runtime: {
    behaviorOverride: ConfigValues | null;
    logLevelOverride: string | null;
    pollIntervalOverride: number | null;
    sessionTTLOverride: number | null;
    revision: number;
    updatedAt?: string;
    updatedBy?: PanelAccount;
    startedAt: number;
  };
  streams: Set<Duplex>;
  prefs: { values: Record<string, unknown>; rev: number };
  transformIndex: IndexTransform;
}

/** How a served `index.html` reaches its final form: Vite's own dev transform. */
type IndexTransform = (url: string, html: string) => Promise<string> | string;

function enabled(): boolean {
  return process.env.SMYKLOT_PANEL_DEV_MOCK === '1';
}

/**
 * Whether starting the server should also raise a browser.
 *
 * Off unless asked for. A dev server is restarted far more often than it is
 * started to be looked at - after a config change, after a port clash, from a
 * script - and each restart took a tab whether or not anybody wanted one.
 * `SMYKLOT_PANEL_DEV_OPEN=1`, or Vite's own `--open`, which overrides this
 * either way.
 */
function opensBrowser(): boolean {
  return process.env.SMYKLOT_PANEL_DEV_OPEN === '1';
}

/**
 * Preferences outlive the process they were set in.
 *
 * Everything the panel remembers about you is a synced preference - the theme, whether the sidebar
 * is collapsed, and every table's sort, filters and search. The mock held them in memory, so each
 * Vite restart, and each edit to this file, handed back a factory-fresh panel and you set them all
 * again. Only the preference document is kept: the rest of the mock is fixture data, and a fixture
 * that drifts across restarts is worse than one that resets.
 */
const FRONTEND_DIR = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const PREFERENCES_FILE = resolve(dirname(fileURLToPath(import.meta.url)), '.mock-preferences.json');

interface DevState {
  prefs: MockState['prefs'];
  /** Invitations issued while the mock was running, so a link you generated still opens later. */
  invitations: MockInvitation[];
  invitationCounter: number;
}

function readDevState(): Partial<DevState> {
  try {
    const parsed: unknown = JSON.parse(readFileSync(PREFERENCES_FILE, 'utf8'));
    return parsed !== null && typeof parsed === 'object' ? (parsed as Partial<DevState>) : {};
  } catch {
    // No file yet, or one this build cannot read. Starting clean beats refusing to serve.
    return {};
  }
}

function loadPreferences(): MockState['prefs'] {
  try {
    const parsed: unknown = JSON.parse(readFileSync(PREFERENCES_FILE, 'utf8'));
    if (parsed === null || typeof parsed !== 'object') return { values: {}, rev: 0 };
    const { values, rev } = parsed as { values?: unknown; rev?: unknown };
    if (values === null || typeof values !== 'object') return { values: {}, rev: 0 };
    return {
      values: { ...(values as Record<string, unknown>) },
      rev: typeof rev === 'number' && Number.isFinite(rev) ? rev : 0,
    };
  } catch {
    // No file yet, or one this build cannot read. Starting clean beats refusing to serve.
    return { values: {}, rev: 0 };
  }
}

/**
 * Invitations the mock issued, kept across restarts.
 *
 * Seeded ones are not: they are fixture data, regenerated on every boot, and a fixture that drifts
 * is worse than one that resets. An invitation you created is different - you created it to open
 * the link, and the link outlives the process you made it in.
 */
function loadIssuedInvitations(): { invitations: MockInvitation[]; counter: number } {
  const stored = readDevState();
  const invitations = Array.isArray(stored.invitations) ? stored.invitations : [];
  const highest = invitations.reduce((top, entry) => {
    const counter = Number(entry.token?.replace('mock-', ''));
    return Number.isFinite(counter) && counter > top ? counter : top;
  }, 0);
  return {
    invitations,
    counter: Math.max(
      typeof stored.invitationCounter === 'number' ? stored.invitationCounter : 0,
      highest + 1,
    ),
  };
}

/** `mock-invitation-<counter>`, which is what `createMockInvitation` mints. The seeded fixtures are
 *  `mock-invitation-seed-<n>` and `mock-invitation-target-<state>`, and they are rebuilt each boot. */
const ISSUED_ID = /^mock-invitation-\d+$/u;

function saveDevState(state: MockState): void {
  try {
    writeFileSync(
      PREFERENCES_FILE,
      `${JSON.stringify(
        {
          values: state.prefs.values,
          rev: state.prefs.rev,
          invitations: state.invitations.filter((entry) => ISSUED_ID.test(entry.id)),
          invitationCounter: state.invitationCounter,
        },
        null,
        2,
      )}\n`,
    );
  } catch {
    // Persisting is a convenience, so a read-only checkout still runs the mock.
  }
}

function seed(): MockState {
  const issued = loadIssuedInvitations();
  const now = Date.now();
  const iso = (offsetMs: number): string => new Date(now + offsetMs).toISOString();
  const organization = targetSeed({
    id: '2001',
    installationId: '3001',
    login: 'smykla-skalski',
    displayName: 'Smykla Skalski',
    type: 'Organization',
    repositoryDefaultEnabled: false,
    targetPatch: { quiet_success: true, command_aliases: { ship: 'merge' } },
  });
  organization.repositories = [
    repositorySeed(organization.value, {
      id: '4001',
      name: 'smyklot',
      enabledOverride: true,
      filePatch: { command_prefix: '/smyklot ', allowed_commands: ['approve', 'merge', 'squash'] },
      panelPatch: { quiet_success: false, allow_self_approval: true },
      updatedAt: iso(-12 * 60_000),
    }),
    repositorySeed(organization.value, {
      id: '4002',
      name: 'platform-infra',
      enabledOverride: null,
      filePatch: {},
      panelPatch: {},
      updatedAt: iso(-3 * 3_600_000),
    }),
    repositorySeed(organization.value, {
      id: '4003',
      name: 'legacy-service',
      enabledOverride: true,
      filePatch: {},
      fileError: 'line 7: command_aliases must be a mapping',
      panelPatch: { disable_reactions: true },
      updatedAt: iso(-27 * 3_600_000),
    }),
    repositorySeed(organization.value, {
      id: '4004',
      name: 'migration-demo',
      enabledOverride: null,
      filePatch: { quiet_pending: true },
      panelPatch: {},
      bypass: true,
      updatedAt: iso(-2 * 86_400_000),
    }),
  ];
  const demoNames = [
    'api-gateway',
    'auth-service',
    'billing-worker',
    'cli-tools',
    'customer-portal',
    'data-pipeline',
    'deployment-config',
    'design-system',
    'docs-site',
    'edge-proxy',
    'event-consumer',
    'feature-flags',
    'identity-provider',
    'internal-tools',
    'mobile-api',
    'notification-service',
    'observability',
    'payments-api',
    'release-automation',
    'runtime-images',
    'search-indexer',
    'security-policies',
    'support-tools',
    'web-frontend',
  ] as const;
  for (const [index, name] of demoNames.entries()) {
    organization.repositories.push(
      repositorySeed(organization.value, {
        id: `40${String(index + 5).padStart(2, '0')}`,
        name,
        /* auth-service (index 1) INHERITS, which is what the approved table
           demos in its second row: an unbroken chain and a dashed target on the
           value Settings supplies. An explicit `false` here drew a broken chain
           on both rows and the inherit affordance never appeared. */
        enabledOverride: index % 3 === 0 ? true : index % 3 === 1 ? null : false,
        filePatch: index % 4 === 0 ? { command_prefix: `/${name} ` } : {},
        fileError: index % 7 === 3 ? 'line 4: unknown setting' : undefined,
        /* api-gateway (index 0) keeps Success replies explicitly ENABLED, which is
           the state the approved repository-override demo starts from: the row
           shows a broken link and a saved value, and switching it to Disabled is
           the single unsaved change the demo draws. */
        panelPatch: index % 5 === 0 ? { quiet_success: index % 2 !== 0 } : {},
        bypass: index % 11 === 0,
        private: index % 4 === 1,
        updatedAt: iso(-(index + 3) * 47 * 60_000),
      }),
    );
  }
  organization.audit = [
    auditSeed(
      'audit-1',
      'repository.enabled',
      'enabled repository',
      'smykla-skalski/smyklot',
      iso(-12 * 60_000),
    ),
    auditSeed(
      'audit-2',
      'repository.config.updated',
      'updated two repository settings for',
      'smykla-skalski/smyklot',
      iso(-18 * 60_000),
    ),
    auditSeed(
      'audit-3',
      'repository.file.bypassed',
      'bypassed repository configuration for',
      'smykla-skalski/migration-demo',
      iso(-2 * 86_400_000),
    ),
  ];
  organization.failures = [
    {
      id: 'failure-1',
      delivery_id: 'b63fb9b0-4014-48fc-8108-f4cb6b2674ab',
      repository_full_name: 'smykla-skalski/legacy-service',
      event: 'issue_comment',
      stage: 'config',
      reason: 'repository configuration is invalid',
      retryable: false,
      occurred_at: iso(-49 * 60_000),
    },
    {
      id: 'failure-2',
      delivery_id: 'df36b61f-0ef7-4d39-9529-7ddcad49fbc0',
      repository_full_name: 'smykla-skalski/smyklot',
      event: 'pull_request',
      stage: 'github',
      reason: 'GitHub request timed out after credentials were refreshed',
      retryable: true,
      occurred_at: iso(-4 * 3_600_000),
    },
  ];
  const auditActions = [
    ['repository.enabled', 'enabled repository'],
    ['repository.disabled', 'disabled repository'],
    ['repository.settings.updated', 'updated repository settings for'],
    ['target.settings.updated', 'updated account defaults'],
  ] as const;
  for (let index = 0; index < 34; index += 1) {
    const [action, summary] = cycled(auditActions, index);
    const repository =
      index % 4 === 3
        ? undefined
        : cycled(organization.repositories, index).detail.repository.full_name;
    organization.audit.push(
      auditSeed(
        `audit-seed-${index + 4}`,
        action,
        summary,
        repository,
        iso(-(6 * 60 + index * 37) * 60_000),
      ),
    );
  }
  const failureReasons = [
    'repository configuration is invalid',
    'GitHub request timed out after credentials were refreshed',
    'installation no longer has access to this repository',
    'command could not be applied to the pull request state',
  ] as const;
  for (let index = 0; index < 27; index += 1) {
    const repository = cycled(organization.repositories, index);
    const deliveryPrefix = (index + 3).toString(16).padStart(8, '0');
    organization.failures.push({
      id: `failure-seed-${index + 3}`,
      delivery_id: `${deliveryPrefix}-0000-4000-8000-${String(index + 3).padStart(12, '0')}`,
      repository_full_name: repository.detail.repository.full_name,
      event: index % 2 === 0 ? 'issue_comment' : 'pull_request',
      stage: index % 3 === 0 ? 'config' : 'github',
      reason: cycled(failureReasons, index),
      retryable: index % 3 === 1,
      occurred_at: iso(-(8 * 60 + index * 53) * 60_000),
    });
  }
  recomputeTarget(organization);

  const personal = targetSeed({
    id: '1001',
    installationId: '3002',
    login: 'bart',
    displayName: 'Bart Smykla',
    type: 'User',
    repositoryDefaultEnabled: true,
    targetPatch: { disable_bare_commands: true },
  });
  personal.repositories = [
    repositorySeed(personal.value, {
      id: '5001',
      name: 'playground',
      enabledOverride: null,
      filePatch: {},
      panelPatch: {},
      private: true,
      updatedAt: iso(-20 * 60_000),
    }),
  ];
  recomputeTarget(personal);

  const users = userSeeds(iso);
  const invitations = invitationSeeds(iso, users[0]?.account ?? VIEWER, organization.value);
  const notifications = securityNotificationSeeds(
    iso,
    organization.value.account,
    users[0]?.account ?? VIEWER,
  );
  const organizationAccess = new Map<string, TargetUserAccess>();
  for (const user of users) {
    const role = user.target_access?.role;
    if (role !== undefined && role !== null) {
      organizationAccess.set(user.account.id, targetAccess(role, false, 1));
    }
  }
  organizationAccess.set('1004', {
    role: 'viewer',
    suspended: true,
    suspension_reason: 'On leave',
    revision: 2,
    updated_at: iso(-3 * 86_400_000),
    effective_role: 'none',
    source: 'suspended',
    capabilities: capabilitiesFor('none'),
  });
  return {
    signedIn: true,
    forceFailure: false,
    hideTargets: false,
    targets: [
      organization,
      personal,
      /* 32 team orgs + the organization + the personal target = 34 installations,
         which is what the approved overview demo's ownership legend adds up to:
         24 fresh (team-11..32 plus those two) + 8 stale + 1 approval + 1 error. */
      ...Array.from({ length: 32 }, (_, index) =>
        targetSeed({
          id: `mock-organization-${index + 1}`,
          installationId: String(3010 + index),
          login: `team-${String(index + 1).padStart(2, '0')}`,
          displayName: `Engineering Team ${String(index + 1).padStart(2, '0')}`,
          type: 'Organization',
          repositoryDefaultEnabled: false,
          targetPatch: {},
        }),
      ),
    ],
    users,
    targetAccess: new Map([[organization.value.id, organizationAccess]]),
    // Fixture invitations, then any this mock issued in an earlier run.
    invitations: [...invitations, ...issued.invitations],
    invitationCounter: Math.max(invitations.length + 1, issued.counter),
    elevationCounter: 1,
    elevations: new Map(),
    notifications,
    pendingCI: pendingCISeeds(iso),
    runtime: {
      behaviorOverride: null,
      logLevelOverride: null,
      pollIntervalOverride: null,
      sessionTTLOverride: null,
      revision: 0,
      startedAt: now,
    },
    streams: new Set(),
    prefs: loadPreferences(),
    // Replaced by install() with the running server's own transform.
    transformIndex: (_url, html) => html,
  };
}

function invitationSeeds(
  iso: (offsetMs: number) => string,
  creator: PanelAccount,
  target: PanelTarget,
): MockInvitation[] {
  const invited = (id: string, login: string, displayName: string): PanelAccount => ({
    id,
    provider: VIEWER.provider,
    subject_id: id,
    login,
    display_name: displayName,
    avatar_url: null,
  });
  const invitations: MockInvitation[] = [
    {
      id: 'mock-invitation-target-pending',
      token: 'p'.repeat(43),
      account: invited('1101', 'katherine', 'Katherine Johnson'),
      target_id: target.id,
      target_name: target.account.display_name,
      target_login: target.account.login,
      target_kind: target.type,
      role: 'editor',
      status: 'pending',
      expires_at: iso(7 * 86_400_000),
      created_by: creator,
      created_at: iso(-20 * 60_000),
    },
    {
      id: 'mock-invitation-target-accepted',
      token: 'a'.repeat(43),
      account: invited('1102', 'dorothy', 'Dorothy Vaughan'),
      target_id: target.id,
      target_name: target.account.display_name,
      target_login: target.account.login,
      target_kind: target.type,
      role: 'viewer',
      status: 'accepted',
      expires_at: iso(6 * 86_400_000),
      created_by: creator,
      created_at: iso(-2 * 86_400_000),
      responded_at: iso(-86_400_000),
    },
    {
      id: 'mock-invitation-target-expired',
      token: 'e'.repeat(43),
      account: invited('1103', 'mary', 'Mary Jackson'),
      target_id: target.id,
      target_name: target.account.display_name,
      target_login: target.account.login,
      target_kind: target.type,
      role: 'viewer',
      status: 'expired',
      expires_at: iso(-86_400_000),
      created_by: creator,
      created_at: iso(-8 * 86_400_000),
    },
  ];
  const statuses: InvitationStatus[] = ['pending', 'accepted', 'declined', 'revoked', 'expired'];
  const roles: Array<Exclude<InstallationRole, 'none'>> = ['viewer', 'editor', 'admin'];
  for (let index = 0; index < 25; index += 1) {
    const status = cycled(statuses, index);
    invitations.push({
      id: `mock-invitation-seed-${index + 1}`,
      token: `${String(index + 1).padStart(43, '0')}`,
      account: invited(
        `invitee-${index + 1}`,
        `invitee-${String(index + 1).padStart(2, '0')}`,
        `Invited User ${String(index + 1).padStart(2, '0')}`,
      ),
      target_id: target.id,
      target_name: target.account.display_name,
      target_login: target.account.login,
      target_kind: target.type,
      role: cycled(roles, index),
      status,
      expires_at: iso((index - 8) * 86_400_000),
      created_by: creator,
      created_at: iso(-(index + 3) * 3_600_000),
      ...(status === 'pending' ? {} : { responded_at: iso(-(index + 1) * 3_600_000) }),
    });
  }
  return invitations;
}

/* The ids are token-shaped because a real one is: the server mints an elevation
   id with `randomToken`, and the inbox shows its tail as the correlation key to
   search the audit trail with. Readable slugs here rendered as "Elevation
   d-incident", which reads as a bug in the panel rather than as a fixture that
   does not look like production. */
function securityNotificationSeeds(
  iso: (offsetMs: number) => string,
  installation: PanelAccount,
  actor: PanelAccount,
): SecurityNotification[] {
  return [
    {
      id: 'security-3',
      installation,
      actor,
      elevation_id: 'R7mQ2xKfLp0Zc4Vn8sTdWb1yHgJ3aEuN6iOqXr5vBkM',
      audit_event_id: '203',
      action: 'repository.settings.updated',
      reason: 'Restore command handling during production incident',
      created_at: iso(-18 * 60_000),
    },
    {
      id: 'security-2',
      installation,
      actor,
      elevation_id: 'R7mQ2xKfLp0Zc4Vn8sTdWb1yHgJ3aEuN6iOqXr5vBkM',
      audit_event_id: '202',
      action: 'target.settings.updated',
      reason: 'Restore command handling during production incident',
      created_at: iso(-24 * 60_000),
    },
    {
      id: 'security-1',
      installation,
      actor,
      elevation_id: 'hT4wYs9dRfB2nKmXpQ7vLc0jZgA5eU8iRoW1yNbD3xE',
      audit_event_id: '188',
      action: 'repository.settings.updated',
      reason: 'Owner-approved support investigation',
      created_at: iso(-2 * 86_400_000),
      read_at: iso(-47 * 3_600_000),
    },
  ];
}

function capabilitiesFor(role: InstallationRole) {
  return {
    read: role !== 'none',
    write: role === 'owner' || role === 'admin' || role === 'editor',
    manage_target_users: role === 'owner' || role === 'admin',
  };
}

function targetUsers(state: MockState, targetId: string): PanelUser[] {
  const overrides = state.targetAccess.get(targetId) ?? new Map<string, TargetUserAccess>();
  return state.users
    .filter((user) => user.status !== 'removed')
    .filter((user) => user.account.id === VIEWER.id || overrides.has(user.account.id))
    .map((user) => {
      const override = overrides.get(user.account.id);
      const manageable = user.manageable && user.status === 'active';
      if (override !== undefined) {
        const access = structuredClone(override);
        if (user.status !== 'active') {
          access.effective_role = 'none';
          access.source = 'denied';
          access.capabilities = capabilitiesFor('none');
        }
        return {
          ...structuredClone(user),
          manageable,
          target_access: access,
        };
      }
      const effectiveRole = user.status === 'active' ? 'owner' : 'none';
      const source: TargetUserAccess['source'] = user.status === 'active' ? 'owner' : 'denied';
      return {
        ...structuredClone(user),
        manageable,
        target_access: {
          role: null,
          suspended: false,
          revision: 0,
          effective_role: effectiveRole,
          source,
          capabilities: capabilitiesFor(effectiveRole),
        },
      };
    });
}

function userSeeds(iso: (offsetMs: number) => string): PanelUser[] {
  const account = (id: string, login: string, displayName: string): PanelAccount => ({
    id,
    provider: VIEWER.provider,
    subject_id: id,
    login,
    display_name: displayName,
    avatar_url: null,
  });
  const user = (
    id: string,
    login: string,
    displayName: string,
    role: InstallationRole,
    offsetMs: number,
  ): PanelUser => ({
    account: account(id, login, displayName),
    system_role: 'none',
    status: 'active',
    ...(role === 'none' || role === 'owner' ? {} : { target_access: targetAccess(role, false, 1) }),
    revision: 1,
    created_at: iso(-30 * 86_400_000),
    updated_at: iso(offsetMs),
    last_login_at: iso(offsetMs),
    manageable: true,
  });
  const root: PanelUser = {
    ...user(VIEWER.id, VIEWER.login, VIEWER.display_name, 'owner', -5 * 60_000),
    account: VIEWER,
    system_role: 'super_root',
    manageable: false,
  };
  const banned = user('1005', 'lin', 'Lin Chen', 'viewer', -9 * 86_400_000);
  banned.status = 'banned';
  banned.ban_reason = 'Repeated abuse of merge commands during the release freeze';
  banned.banned_at = iso(-9 * 86_400_000);

  const users = [
    root,
    user('1002', 'ada', 'Ada Lovelace', 'admin', -42 * 60_000),
    user('1003', 'grace', 'Grace Hopper', 'editor', -4 * 3_600_000),
    user('1004', 'margaret', 'Margaret Hamilton', 'viewer', -2 * 86_400_000),
    banned,
  ];
  const roles: InstallationRole[] = ['viewer', 'editor', 'admin', 'none'];
  for (let index = 0; index < 31; index += 1) {
    users.push(
      user(
        `seed-user-${index + 1}`,
        `panel-user-${String(index + 1).padStart(2, '0')}`,
        `Panel User ${String(index + 1).padStart(2, '0')}`,
        cycled(roles, index),
        -(index + 4) * 95 * 60_000,
      ),
    );
  }
  return users;
}

function targetSeed(input: {
  id: string;
  installationId: string;
  login: string;
  displayName: string;
  type: 'Organization' | 'User';
  repositoryDefaultEnabled: boolean;
  targetPatch: ConfigPatch;
}): MockTarget {
  const account: PanelAccount = {
    id: input.id,
    provider: 'github:https://api.github.com',
    subject_id: input.id,
    login: input.login,
    display_name: input.displayName,
    avatar_url: null,
  };
  const resolved = resolveConfig(input.targetPatch, {}, {}, false);
  return {
    value: {
      id: input.id,
      installation_id: input.installationId,
      type: input.type,
      account,
      repository_default_enabled: input.repositoryDefaultEnabled,
      config_patch: input.targetPatch,
      inherited_config: structuredClone(DEFAULT_CONFIG),
      effective_config: resolved.values,
      config_sources: resolved.sources,
      revision: 1,
      repository_counts: { total: 0, enabled: 0, disabled: 0 },
      effective_role: 'owner',
      access_source: 'owner',
      capabilities: OWNER_CAPABILITIES,
    },
    repositories: [],
    audit: [],
    failures: [],
  };
}

function repositorySeed(
  target: PanelTarget,
  input: {
    id: string;
    name: string;
    enabledOverride: boolean | null;
    filePatch: ConfigPatch;
    panelPatch: ConfigPatch;
    fileError?: string;
    bypass?: boolean;
    private?: boolean;
    updatedAt: string;
  },
): MockRepository {
  const bypass = input.bypass ?? false;
  const inherited = resolveConfig(target.config_patch, input.filePatch, {}, bypass);
  const resolved = resolveConfig(target.config_patch, input.filePatch, input.panelPatch, bypass);
  const status = bypass
    ? 'bypassed'
    : input.fileError !== undefined
      ? 'invalid'
      : Object.keys(input.filePatch).length === 0
        ? 'missing'
        : 'valid';
  const summary: RepositorySummary = {
    id: input.id,
    name: input.name,
    full_name: `${target.account.login}/${input.name}`,
    private: input.private ?? false,
    default_branch: Number(input.id.replace(/\D/g, '')) % 5 === 0 ? 'develop' : 'main',
    available: true,
    enabled_override: input.enabledOverride,
    effective_enabled: input.enabledOverride ?? target.repository_default_enabled,
    enabled_source: input.enabledOverride === null ? 'target' : 'repository',
    config_override_count: Object.keys(input.panelPatch).length,
    config_file_status: status,
    updated_at: input.updatedAt,
  };
  return {
    filePatch: input.filePatch,
    detail: {
      repository: summary,
      config_patch: input.panelPatch,
      inherited_config: inherited.values,
      effective_config: resolved.values,
      config_sources: resolved.sources,
      config_file_patch: input.filePatch,
      config_file_error: input.fileError,
      config_file_path: status === 'missing' ? undefined : '.smyklot.toml',
      // Every fifth repository carries the file it was meant to have migrated
      // away from, so the detail pane's "also present" line has something to
      // render against
      config_file_superseded:
        status === 'missing' || Number(input.id.replace(/\D/g, '')) % 5 !== 0
          ? undefined
          : ['.github/smyklot.yaml'],
      // Every seventh repository has already been asked and said no, so the
      // detail pane's refusal line and its way back are both reachable
      config_migration:
        status === 'missing' || Number(input.id.replace(/\D/g, '')) % 7 !== 0 ? 'none' : 'declined',
      config_migration_pr:
        status === 'missing' || Number(input.id.replace(/\D/g, '')) % 7 !== 0 ? undefined : 42,
      ignore_repository_file: bypass,
      revision: 1,
    },
  };
}

function auditSeed(
  id: string,
  action: string,
  summary: string,
  repositoryFullName: string | undefined,
  createdAt: string,
): AuditEntry {
  return {
    id,
    actor: VIEWER,
    action,
    summary,
    repository_full_name: repositoryFullName,
    created_at: createdAt,
  };
}

function resolveConfig(
  targetPatch: ConfigPatch,
  filePatch: ConfigPatch,
  panelPatch: ConfigPatch,
  bypass: boolean,
): { values: ConfigValues; sources: ConfigSources } {
  const values = structuredClone(DEFAULT_CONFIG);
  const sources = Object.fromEntries(
    Object.keys(DEFAULT_CONFIG).map((key) => [key, 'process']),
  ) as ConfigSources;
  applyPatch(values, sources, targetPatch, 'target');
  if (!bypass) applyPatch(values, sources, filePatch, 'repository_file');
  applyPatch(values, sources, panelPatch, 'repository_panel');
  return { values, sources };
}

function applyPatch(
  values: ConfigValues,
  sources: ConfigSources,
  patch: ConfigPatch,
  source: ConfigSources[ConfigKey],
): void {
  for (const key of Object.keys(patch) as ConfigKey[]) {
    const value = patch[key];
    if (value === undefined) continue;
    Object.assign(values, { [key]: structuredClone(value) });
    sources[key] = source;
  }
}

function recomputeTarget(target: MockTarget): void {
  const targetResolved = resolveConfig(target.value.config_patch, {}, {}, false);
  target.value.inherited_config = structuredClone(DEFAULT_CONFIG);
  target.value.effective_config = targetResolved.values;
  target.value.config_sources = targetResolved.sources;
  for (const repository of target.repositories) recomputeRepository(target, repository);
  const enabled = target.repositories.filter(
    (entry) => entry.detail.repository.effective_enabled,
  ).length;
  target.value.repository_counts = {
    total: target.repositories.length,
    enabled,
    disabled: target.repositories.length - enabled,
  };
}

function recomputeRepository(target: MockTarget, repository: MockRepository): void {
  const detail = repository.detail;
  const inherited = resolveConfig(
    target.value.config_patch,
    repository.filePatch,
    {},
    detail.ignore_repository_file,
  );
  const resolved = resolveConfig(
    target.value.config_patch,
    repository.filePatch,
    detail.config_patch,
    detail.ignore_repository_file,
  );
  detail.inherited_config = inherited.values;
  detail.effective_config = resolved.values;
  detail.config_sources = resolved.sources;
  detail.repository.effective_enabled =
    detail.repository.enabled_override ?? target.value.repository_default_enabled;
  detail.repository.enabled_source =
    detail.repository.enabled_override === null ? 'target' : 'repository';
  detail.repository.config_override_count = Object.keys(detail.config_patch).length;
  if (detail.ignore_repository_file) detail.repository.config_file_status = 'bypassed';
}

export function mockServer(): Plugin {
  return {
    name: 'smyklot-panel-mock-server',
    config() {
      if (!enabled()) return;
      return { server: opensBrowser() ? { open: '/' } : {} };
    },
    transformIndexHtml(html) {
      if (!enabled()) return html;
      return rewriteMockHtml(html);
    },
    configureServer(server) {
      if (!enabled()) return;
      install(server.httpServer as DevHttpServer, server.middlewares, (url, html) =>
        server.transformIndexHtml(url, html),
      );
    },
    configurePreviewServer(server) {
      if (enabled()) {
        install(server.httpServer as DevHttpServer, server.middlewares, (_url, html) => html);
      }
    },
  };
}

function install(
  httpServer: DevHttpServer | undefined,
  middlewares: Connect.Server,
  transform: IndexTransform,
): void {
  if (httpServer === undefined) throw new Error('the mock dev server has no HTTP server');
  const state = seed();
  state.transformIndex = transform;
  httpServer.on('upgrade', (request, socket) => handleUpgrade(state, request, socket));
  middlewares.use((req, res, next) => void handle(state, req, res, next));
}

function handleUpgrade(state: MockState, request: IncomingMessage, socket: Duplex): void {
  const path = new URL(request.url ?? '/', 'http://localhost').pathname;
  if (path !== route('/api/v1/events')) return;
  if (!state.signedIn) {
    rejectUpgrade(socket, 401, 'Unauthorized');
    return;
  }
  if (state.forceFailure) {
    rejectUpgrade(socket, 503, 'Service Unavailable');
    return;
  }
  const key = request.headers['sec-websocket-key'];
  if (typeof key !== 'string') {
    rejectUpgrade(socket, 400, 'Bad Request');
    return;
  }
  const accept = createHash('sha1')
    .update(`${key}258EAFA5-E914-47DA-95CA-C5AB0DC85B11`)
    .digest('base64');
  socket.write(
    [
      'HTTP/1.1 101 Switching Protocols',
      'Upgrade: websocket',
      'Connection: Upgrade',
      `Sec-WebSocket-Accept: ${accept}`,
      '',
      '',
    ].join('\r\n'),
  );
  state.streams.add(socket);
  const remove = (): void => {
    state.streams.delete(socket);
  };
  socket.once('close', remove);
  socket.once('error', remove);
  attachWebSocketReader(state, socket);

  const query = new URL(request.url ?? '/', 'http://localhost').searchParams;
  const sum = prefsSum(state);
  const matches =
    query.get('prefs_rev') === String(state.prefs.rev) && query.get('prefs_sum') === sum;
  const prefsInfo: Record<string, unknown> = { rev: state.prefs.rev, sum };
  if (!matches) prefsInfo.values = state.prefs.values;
  writeWebSocket(socket, { version: 1, type: 'ready', prefs: prefsInfo });
}

function prefsSum(state: MockState): string {
  return createHash('sha256')
    .update(canonicalStringify(state.prefs.values))
    .digest('hex')
    .slice(0, 16);
}

// attachWebSocketReader parses masked client frames. Unfragmented frames up
// to 64 KiB only — all the panel client ever sends.
function attachWebSocketReader(state: MockState, socket: Duplex): void {
  let buffered = Buffer.alloc(0);
  socket.on('data', (chunk: Buffer) => {
    buffered = Buffer.concat([buffered, chunk]);
    for (;;) {
      let frame: ClientFrame | null;
      try {
        frame = readClientFrame(buffered);
      } catch {
        socket.end();
        return;
      }
      if (frame === null) return;
      buffered = buffered.subarray(frame.consumed);
      if (frame.opcode === 0x8) {
        writeControlFrame(socket, 0x8, frame.payload);
        socket.end();
        return;
      }
      if (frame.opcode === 0x9) {
        writeControlFrame(socket, 0xa, frame.payload);
        continue;
      }
      if (frame.opcode === 0x1) handleClientFrame(state, socket, frame.payload);
    }
  });
}

interface ClientFrame {
  opcode: number;
  payload: Buffer;
  consumed: number;
}

function readClientFrame(buffered: Buffer): ClientFrame | null {
  if (buffered.length < 2) return null;
  const opcode = (buffered[0] ?? 0) & 0x0f;
  const second = buffered[1] ?? 0;
  if ((second & 0x80) === 0) throw new Error('client frames must be masked');
  let length = second & 0x7f;
  let offset = 2;
  if (length === 127) throw new Error('the mock WebSocket cannot read 64-bit frames');
  if (length === 126) {
    if (buffered.length < 4) return null;
    length = buffered.readUInt16BE(2);
    offset = 4;
  }
  if (buffered.length < offset + 4 + length) return null;
  const mask = buffered.subarray(offset, offset + 4);
  const payload = Buffer.from(buffered.subarray(offset + 4, offset + 4 + length));
  for (let index = 0; index < payload.length; index += 1) {
    payload[index] = (payload[index] ?? 0) ^ (mask[index % 4] ?? 0);
  }

  return { opcode, payload, consumed: offset + 4 + length };
}

function writeControlFrame(socket: Duplex, opcode: number, payload: Buffer): void {
  if (payload.length > 125) throw new Error('the mock WebSocket control payload is too large');
  socket.write(Buffer.concat([Buffer.from([0x80 | opcode, payload.length]), payload]));
}

function handleClientFrame(state: MockState, socket: Duplex, payload: Buffer): void {
  let frame: { version?: number; type?: string; changes?: Record<string, unknown> };
  try {
    frame = JSON.parse(payload.toString('utf8')) as typeof frame;
  } catch {
    return;
  }
  if (frame.version !== 1 || frame.type !== 'prefs.patch') return;

  const accepted: Record<string, unknown> = {};
  const rejected: string[] = [];
  for (const [key, value] of Object.entries(frame.changes ?? {})) {
    const valid =
      value === null ||
      typeof value === 'string' ||
      (Array.isArray(value) && value.every((element) => typeof element === 'string'));
    if (!Object.hasOwn(PREF_DEFAULTS, key) || !valid) {
      rejected.push(key);
      continue;
    }
    accepted[key] = value;
  }

  if (Object.keys(accepted).length > 0) {
    for (const [key, value] of Object.entries(accepted)) {
      if (value === null) {
        delete state.prefs.values[key];
      } else {
        state.prefs.values[key] = value;
      }
    }
    state.prefs.rev += 1;
    saveDevState(state);
    for (const stream of state.streams) {
      writeWebSocket(stream, {
        version: 1,
        type: 'prefs.changed',
        rev: state.prefs.rev,
        changes: accepted,
      });
    }
  }
  if (rejected.length > 0) {
    writeWebSocket(socket, { version: 1, type: 'prefs.rejected', keys: rejected.sort() });
  }
}

function rejectUpgrade(socket: Duplex, status: number, reason: string): void {
  socket.end(`HTTP/1.1 ${status} ${reason}\r\nConnection: close\r\n\r\n`);
}

/** Where this mock is answering, so a generated invitation link opens on the port you are using
 *  rather than the one someone happened to write down. */
let devOrigin = 'http://localhost:5173';

async function handle(
  state: MockState,
  req: IncomingMessage,
  res: ServerResponse,
  next: Connect.NextFunction,
): Promise<void> {
  if (req.headers.host !== undefined) devOrigin = `http://${req.headers.host}`;
  const parsed = new URL(req.url ?? '/', 'http://localhost');
  const path = parsed.pathname.replace(/^\/__smyklot_panel_base__/, '');
  const method = req.method ?? 'GET';

  if (path === '/' && method === 'GET') {
    applyScenario(state, parsed.searchParams.get('scenario'));
    next();
    return;
  }

  /**
   * Every error page, on demand. Dev only, and it exists because almost none of them can be
   * reached here otherwise: they come out of the GitHub sign-in round trip, and the mock has no
   * GitHub to fail against. `/__error/403/forbidden` renders exactly what production would.
   */
  const preview = path.match(/^\/__error\/(?<status>\d{3})(?:\/(?<code>[a-z_]*))?$/u);
  if (preview && method === 'GET') {
    await respondError(
      state,
      req,
      res,
      Number(preview.groups?.status),
      preview.groups?.code ?? '',
      parsed.searchParams.get('message') ?? 'a mock error, for looking at',
    );
    return;
  }

  /**
   * A navigation to a path the panel does not own is a 404, as it is in production.
   *
   * Vite answers every navigation with index.html, so in dev an unknown path quietly rendered the
   * panel and the server's own error response was unreachable - there was no way to look at it
   * without building and running the Go binary. `serveAsset` decides this with
   * `isPanelNavigationPath`; the same decision is `parsePanelRoute` plus the invitation form, so
   * this asks the router the app itself uses rather than keeping a second list in step.
   */
  if (method === 'GET' && (req.headers.accept ?? '').includes('text/html')) {
    const navigable =
      parsePanelRoute('/', path) !== null || parseInvitationToken('/', path) !== null;
    if (!navigable) {
      await respondError(state, req, res, 404, 'not_found', 'panel route not found');
      return;
    }
  }
  const publicInvitation = path.match(/^\/api\/v1\/invites\/(?<token>[^/]+)$/);
  if (publicInvitation && method === 'GET') {
    try {
      const invitation = findInvitationByToken(state, publicInvitation.groups?.token ?? '');
      respond(res, 200, publicInvitationValue(invitation));
    } catch (error) {
      if (error instanceof MockApiError) {
        respond(res, error.status, { error: { code: error.code, message: error.message } });
      } else {
        respond(res, 500, { error: { code: 'internal', message: 'the mock request failed' } });
      }
    }
    return;
  }
  if (path === route('/auth/github/start') && method === 'GET') {
    const token = parsed.searchParams.get('invite');
    const action = parsed.searchParams.get('action');
    if (token !== null && (action === 'accept' || action === 'decline')) {
      const invitation = findInvitationByToken(state, token);
      if (invitation.status !== 'pending') {
        await respondError(
          state,
          req,
          res,
          409,
          'invitation_used',
          'this invitation is no longer pending',
        );
        return;
      }
      invitation.status = action === 'accept' ? 'accepted' : 'declined';
      invitation.responded_at = new Date().toISOString();
      state.signedIn = action === 'accept';
      res.writeHead(302, {
        Location: action === 'accept' ? '/' : `/invite/${encodeURIComponent(token)}?declined=1`,
      });
      res.end();
      return;
    }
    res.writeHead(302, { Location: route('/auth/github/callback') });
    res.end();
    return;
  }
  if (path === route('/auth/github/callback') && method === 'GET') {
    state.signedIn = true;
    res.writeHead(302, { Location: '/' });
    res.end();
    return;
  }
  if (path === route('/api/v1/sign-out') && method === 'POST') {
    broadcast(state, { type: 'session.revoked', code: 'signed_out', reason: 'You signed out' });
    for (const stream of state.streams) stream.end();
    state.streams.clear();
    state.signedIn = false;
    respond(res, 204, null);
    return;
  }
  if (!path.startsWith('/api/') && !path.startsWith('/auth/')) {
    next();
    return;
  }
  if (!state.signedIn) {
    respond(res, 401, { error: { code: 'unauthenticated', message: 'sign in to use the panel' } });
    return;
  }
  if (state.forceFailure && path.startsWith('/api/')) {
    respond(res, 503, { error: { code: 'unavailable', message: 'mock storage is unavailable' } });
    return;
  }

  try {
    if (path === route('/api/v1/session') && method === 'GET') {
      respond(res, 200, {
        account: VIEWER,
        /* A Root with no installations keeps the panel shell, because the console
           is still there to reach. `?scenario=empty` is for the other reader -
           signed in, nothing installed, nothing else to do - so it hands back a
           regular account. */
        system_role: state.hideTargets ? 'none' : 'super_root',
        status: 'active',
        target_count: state.hideTargets
          ? 0
          : state.targets.filter((target) => mockRootOwns(target)).length,
      });
      return;
    }
    if (path === route('/api/v1/targets') && method === 'GET') {
      respond(res, 200, {
        targets: state.hideTargets
          ? []
          : state.targets.filter((target) => mockRootOwns(target)).map((target) => target.value),
      });
      return;
    }
    if (path === route('/api/v1/root/installations') && method === 'GET') {
      const ordered = [...state.targets].sort((left, right) =>
        left.value.type === right.value.type ? 0 : left.value.type === 'Organization' ? -1 : 1,
      );
      respond(res, 200, {
        installations: ordered.map((target) => rootInstallationValue(target)),
      });
      return;
    }
    if (path === route('/api/v1/root/installations/sync') && method === 'POST') {
      broadcast(state, { type: 'resync' });
      respond(res, 200, { target_ids: state.targets.map((target) => target.value.id) });
      return;
    }
    if (path === route('/api/v1/root/overview') && method === 'GET') {
      respond(res, 200, rootOverviewValue(state));
      return;
    }
    if (path === route('/api/v1/root/settings') && method === 'GET') {
      respond(res, 200, rootRuntimeSettingsValue(state));
      return;
    }
    if (path === route('/api/v1/root/settings') && method === 'PUT') {
      const input = await readBody<RootRuntimeSettingsInput>(req);
      if (input.expected_revision !== state.runtime.revision) {
        throw new MockApiError(409, 'conflict', 'runtime settings changed; reload and try again');
      }
      state.runtime.behaviorOverride = copyOptionalConfig(input.bot_config);
      state.runtime.logLevelOverride = input.log_level;
      state.runtime.pollIntervalOverride = input.reaction_poll_interval_seconds;
      state.runtime.sessionTTLOverride = input.session_ttl_seconds;
      state.runtime.revision += 1;
      state.runtime.updatedAt = new Date().toISOString();
      state.runtime.updatedBy = VIEWER;
      const value = rootRuntimeSettingsValue(state);
      broadcast(state, { type: 'resync' });
      respond(res, 200, value);
      return;
    }
    if (path === route('/api/v1/root/access/users') && method === 'GET') {
      const systemRoles = parsed.searchParams.getAll('system_role');
      const statuses = parsed.searchParams.getAll('status');
      respond(
        res,
        200,
        historyPage(
          rootPanelUsers(state),
          parsed.searchParams,
          (user) => user.last_login_at ?? '',
          (user, sort) => {
            if (sort.startsWith('role_')) {
              return ['none', 'root', 'super_root'].indexOf(user.system_role);
            }
            if (sort.startsWith('login_')) return user.last_login_at ?? '';
            return user.account.display_name.toLocaleLowerCase();
          },
          (user, query) =>
            [user.account.display_name, user.account.login].some((value) =>
              value.toLocaleLowerCase().includes(query),
            ),
          (user) =>
            (systemRoles.length === 0 || systemRoles.includes(user.system_role)) &&
            (statuses.length === 0 || statuses.includes(user.status)),
        ),
      );
      return;
    }
    if (path === route('/api/v1/root/access/invitations') && method === 'GET') {
      respond(
        res,
        200,
        invitationPage(
          state.invitations.filter((entry) => entry.system_role === 'root'),
          parsed.searchParams,
        ),
      );
      return;
    }
    if (path === route('/api/v1/root/access/invitations') && method === 'POST') {
      const input = await readBody<AddRootInvitationInput>(req);
      const created = createRootMockInvitation(state, input);
      broadcastInvitation(state, created);
      saveDevState(state);
      respond(res, 201, invitationValue(created));
      return;
    }
    if (path === route('/api/v1/notifications') && method === 'GET') {
      const offset = Math.max(0, Number(parsed.searchParams.get('cursor') ?? '0'));
      const limit = Math.min(100, Math.max(1, Number(parsed.searchParams.get('limit') ?? '20')));
      const items = state.notifications.slice(offset, offset + limit);
      const next = offset + items.length;
      respond(res, 200, {
        items,
        next_cursor: next < state.notifications.length ? String(next) : null,
        total: state.notifications.length,
        unread: state.notifications.filter((notification) => notification.read_at === undefined)
          .length,
      });
      return;
    }
    if (path === route('/api/v1/events') && method === 'GET') {
      respond(res, 426, { error: { code: 'upgrade_required', message: 'WebSocket required' } });
      return;
    }

    const targetSettings = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/settings$/);
    const rootTargetSettings = path.match(
      /^\/api\/v1\/root\/installations\/(?<target>[^/]+)\/settings$/,
    );
    const rootElevation = path.match(
      /^\/api\/v1\/root\/installations\/(?<target>[^/]+)\/elevation$/,
    );
    const rootElevationEnd = path.match(/^\/api\/v1\/root\/elevations\/(?<elevation>[^/]+)$/);
    const notificationRead = path.match(/^\/api\/v1\/notifications\/(?<notification>[^/]+)\/read$/);
    const scopedUsers = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/users$/);
    const rootScopedUsers = path.match(/^\/api\/v1\/root\/installations\/(?<target>[^/]+)\/users$/);
    const userSuggestions = path.match(
      /^\/api\/v1\/(?:targets|root\/installations)\/(?<target>[^/]+)\/user-suggestions$/,
    );
    const rootUser = path.match(/^\/api\/v1\/root\/access\/users\/(?<account>[^/]+)$/);
    const rootInvitationReissue = path.match(
      /^\/api\/v1\/root\/access\/invitations\/(?<invitation>[^/]+)\/reissue$/,
    );
    const rootInvitation = path.match(
      /^\/api\/v1\/root\/access\/invitations\/(?<invitation>[^/]+)$/,
    );
    const scopedUser = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/users\/(?<account>[^/]+)$/,
    );
    const rootScopedUser = path.match(
      /^\/api\/v1\/root\/installations\/(?<target>[^/]+)\/users\/(?<account>[^/]+)$/,
    );
    const scopedUserDecisions = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/users\/(?<account>[^/]+)\/decisions$/,
    );
    const rootScopedUserDecisions = path.match(
      /^\/api\/v1\/root\/installations\/(?<target>[^/]+)\/users\/(?<account>[^/]+)\/decisions$/,
    );
    const scopedInvitations = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/invitations$/);
    const rootScopedInvitations = path.match(
      /^\/api\/v1\/root\/installations\/(?<target>[^/]+)\/invitations$/,
    );
    const reissueInvitation = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/invitations\/(?<invitation>[^/]+)\/reissue$/,
    );
    const rootScopedInvitationReissue = path.match(
      /^\/api\/v1\/root\/installations\/(?<target>[^/]+)\/invitations\/(?<invitation>[^/]+)\/reissue$/,
    );
    const invitation = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/invitations\/(?<invitation>[^/]+)$/,
    );
    const rootScopedInvitation = path.match(
      /^\/api\/v1\/root\/installations\/(?<target>[^/]+)\/invitations\/(?<invitation>[^/]+)$/,
    );
    const repositories = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/repositories$/);
    const rootRepositories = path.match(
      /^\/api\/v1\/root\/installations\/(?<target>[^/]+)\/repositories$/,
    );
    const repository = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/repositories\/(?<repository>[^/]+)$/,
    );
    const rootRepository = path.match(
      /^\/api\/v1\/root\/installations\/(?<target>[^/]+)\/repositories\/(?<repository>[^/]+)$/,
    );
    const repositorySettings = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/repositories\/(?<repository>[^/]+)\/settings$/,
    );
    const rootRepositorySettings = path.match(
      /^\/api\/v1\/root\/installations\/(?<target>[^/]+)\/repositories\/(?<repository>[^/]+)\/settings$/,
    );
    const repositoryConfigMigration = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/repositories\/(?<repository>[^/]+)\/config-migration$/,
    );
    const rootRepositoryConfigMigration = path.match(
      /^\/api\/v1\/root\/installations\/(?<target>[^/]+)\/repositories\/(?<repository>[^/]+)\/config-migration$/,
    );
    const audit = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/audit$/);
    const failures = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/failures$/);
    const rootTargetAudit = path.match(/^\/api\/v1\/root\/installations\/(?<target>[^/]+)\/audit$/);
    const rootTargetFailures = path.match(
      /^\/api\/v1\/root\/installations\/(?<target>[^/]+)\/failures$/,
    );
    const rootHistory = path.match(/^\/api\/v1\/root\/history\/(?<history>audit|failures)$/);
    const rootPendingCICheck = path.match(
      /^\/api\/v1\/root\/pending-ci\/(?<request>[^/]+)\/check$/,
    );
    const rootPendingCI = path.match(/^\/api\/v1\/root\/pending-ci\/(?<request>[^/]+)$/);
    const installationUsers = scopedUsers ?? rootScopedUsers;
    const installationUser = scopedUser ?? rootScopedUser;
    const installationUserDecisions = scopedUserDecisions ?? rootScopedUserDecisions;
    const installationInvitations = scopedInvitations ?? rootScopedInvitations;
    const installationInvitationReissue = reissueInvitation ?? rootScopedInvitationReissue;
    const installationInvitation = invitation ?? rootScopedInvitation;
    const installationAudit = audit ?? rootTargetAudit;
    const installationFailures = failures ?? rootTargetFailures;

    if (rootPendingCICheck && method === 'POST') {
      const request = findPendingCI(state, rootPendingCICheck.groups?.request ?? '');
      const input = await readBody<{ expected_revision: number }>(req);
      requirePendingCIRevision(request, input.expected_revision);
      const now = new Date().toISOString();
      request.schedule = 'active';
      request.next_check_at = now;
      request.next_check_trigger = 'manual';
      request.updated_at = now;
      request.revision += 1;
      broadcast(state, { type: 'resync' });
      respond(res, 200, structuredClone(request));
      return;
    }

    if (rootPendingCI && method === 'GET') {
      const request = findPendingCI(state, rootPendingCI.groups?.request ?? '');
      respond(res, 200, pendingCIDetail(request));
      return;
    }

    if (rootPendingCI && method === 'DELETE') {
      const request = findPendingCI(state, rootPendingCI.groups?.request ?? '');
      const input = await readBody<{ expected_revision: number }>(req);
      requirePendingCIRevision(request, input.expected_revision);
      const now = new Date().toISOString();
      request.lifecycle = 'cancelled';
      request.reason = 'cancelled by panel user @bart';
      request.finished_at = now;
      request.next_check_at = now;
      request.next_check_trigger = 'cleanup';
      request.cleanup_pending = true;
      request.updated_at = now;
      request.revision += 1;
      broadcast(state, { type: 'resync' });
      respond(res, 200, structuredClone(request));
      return;
    }

    if (rootUser && method === 'PUT') {
      const accountID = decodeURIComponent(rootUser.groups?.account ?? '');
      const user = state.users.find((candidate) => candidate.account.id === accountID);
      if (user === undefined) throw new MockApiError(404, 'not_found', 'account not found');
      const input = await readBody<UpdateRootUserInput>(req);
      if (input.expected_revision !== user.revision) {
        throw new MockApiError(409, 'conflict', 'account changed; reload and try again');
      }
      if ('system_role' in input) {
        user.system_role = input.system_role;
      } else {
        user.status = input.status;
        user.ban_reason = input.reason;
        user.banned_at = input.status === 'banned' ? new Date().toISOString() : undefined;
      }
      user.revision += 1;
      user.updated_at = new Date().toISOString();
      respond(res, 204, null);
      return;
    }

    if (rootInvitationReissue && method === 'POST') {
      const current = findInvitation(state, rootInvitationReissue.groups?.invitation ?? '');
      if (current.system_role !== 'root') {
        throw new MockApiError(404, 'not_found', 'Root invitation not found');
      }
      const input = await readBody<{ expires_in_days: InvitationDays }>(req);
      requireReissuable(current);
      refuseUnusableInvitation(state, current.account.login, {}, true);
      current.token = mockInvitationToken(state.invitationCounter++);
      current.status = 'pending';
      current.expires_at = new Date(Date.now() + input.expires_in_days * 86_400_000).toISOString();
      current.created_at = new Date().toISOString();
      current.created_by = VIEWER;
      current.responded_at = undefined;
      broadcastInvitation(state, current);
      saveDevState(state);
      respond(res, 200, invitationValue(current));
      return;
    }

    if (rootInvitation && method === 'DELETE') {
      const current = findInvitation(state, rootInvitation.groups?.invitation ?? '');
      if (current.system_role !== 'root') {
        throw new MockApiError(404, 'not_found', 'Root invitation not found');
      }
      current.status = 'revoked';
      current.responded_at = new Date().toISOString();
      broadcastInvitation(state, current);
      saveDevState(state);
      respond(res, 200, publicInvitationValue(current));
      return;
    }

    if (rootHistory && method === 'GET') {
      if (rootHistory.groups?.history === 'audit') {
        const categories = parsed.searchParams
          .getAll('category')
          .filter((value) => value !== 'all');
        respond(
          res,
          200,
          historyPage(
            rootAuditEntries(state),
            parsed.searchParams,
            (entry) => entry.created_at,
            (entry, sort) => {
              if (sort.startsWith('actor_')) return entry.actor.display_name.toLocaleLowerCase();
              if (sort.startsWith('target_')) {
                return (entry.installation?.display_name ?? 'Smyklot').toLocaleLowerCase();
              }
              if (sort.startsWith('change_')) return entry.summary.toLocaleLowerCase();
              return entry.created_at;
            },
            (entry, query) =>
              [
                entry.category ?? '',
                entry.installation?.display_name ?? 'Smyklot',
                entry.installation?.login ?? '',
                entry.actor.display_name,
                entry.actor.login,
                entry.subject?.display_name ?? '',
                entry.subject?.login ?? '',
                entry.action,
                entry.summary,
              ].some((value) => value.toLocaleLowerCase().includes(query)),
            (entry) => categories.length === 0 || categories.includes(entry.category ?? ''),
          ),
        );
      } else {
        const kind = parsed.searchParams.get('kind') ?? 'all';
        const items = state.targets.flatMap((target) =>
          target.failures.map((failure) => ({
            installation: target.value.account,
            failure,
          })),
        );
        respond(
          res,
          200,
          historyPage(
            items,
            parsed.searchParams,
            (item) => item.failure.occurred_at,
            (item, sort) => {
              if (sort.startsWith('status_')) return item.failure.retryable ? 1 : 0;
              if (sort.startsWith('repository_')) {
                return item.failure.repository_full_name.toLocaleLowerCase();
              }
              return item.failure.occurred_at;
            },
            (item, query) =>
              [
                item.installation.display_name,
                item.installation.login,
                item.failure.delivery_id,
                item.failure.repository_full_name,
                item.failure.event,
                item.failure.stage,
                item.failure.reason,
              ].some((value) => value.toLocaleLowerCase().includes(query)),
            (item) =>
              kind === 'all' ||
              (kind === 'retryable' && item.failure.retryable) ||
              (kind === 'permanent' && !item.failure.retryable),
          ),
        );
      }
      return;
    }

    if (notificationRead && method === 'PUT') {
      const id = decodeURIComponent(notificationRead.groups?.notification ?? '');
      const index = state.notifications.findIndex((notification) => notification.id === id);
      if (index < 0) throw new MockApiError(404, 'not_found', 'security notification not found');
      const current = state.notifications[index];
      if (current === undefined)
        throw new MockApiError(404, 'not_found', 'security notification not found');
      const updated = { ...current, read_at: current.read_at ?? new Date().toISOString() };
      state.notifications[index] = updated;
      respond(res, 200, updated);
      return;
    }

    if (rootElevation && method === 'GET') {
      const target = findTarget(state, rootElevation.groups?.target ?? '');
      const elevation = activeMockElevation(state, target.value.id);
      if (elevation === undefined)
        throw new MockApiError(404, 'not_found', 'elevated installation access was not found');
      respond(res, 200, elevation);
      return;
    }
    if (rootElevation && method === 'POST') {
      const target = findTarget(state, rootElevation.groups?.target ?? '');
      const input = await readBody<RootElevationInput>(req);
      if (input.acknowledged !== true)
        throw new MockApiError(
          400,
          'acknowledgment_required',
          'confirm the elevated access warning',
        );
      if (mockRootOwns(target))
        throw new MockApiError(409, 'conflict', 'you already own this installation');
      if (!rootInstallationValue(target).available)
        throw new MockApiError(409, 'conflict', 'fresh Owners are required');
      const started = new Date();
      const elevation: RootElevation = {
        id: `mock-elevation-${state.elevationCounter++}`,
        target_id: target.value.id,
        ...(input.reason === undefined ? {} : { reason: input.reason }),
        started_at: started.toISOString(),
        expires_at: new Date(started.getTime() + 15 * 60_000).toISOString(),
      };
      state.elevations.set(target.value.id, elevation);
      respond(res, 201, elevation);
      return;
    }
    if (rootElevationEnd && method === 'DELETE') {
      const id = decodeURIComponent(rootElevationEnd.groups?.elevation ?? '');
      const entry = [...state.elevations.entries()].find(([, elevation]) => elevation.id === id);
      if (entry === undefined)
        throw new MockApiError(404, 'not_found', 'elevated installation access was not found');
      const [targetId, elevation] = entry;
      const ended = { ...elevation, ended_at: new Date().toISOString() };
      state.elevations.delete(targetId);
      respond(res, 200, ended);
      return;
    }

    if (rootTargetSettings && method === 'GET') {
      const target = findTarget(state, rootTargetSettings.groups?.target ?? '');
      respond(res, 200, rootTargetValue(state, target));
      return;
    }
    if (rootTargetSettings && method === 'PUT') {
      const target = findTarget(state, rootTargetSettings.groups?.target ?? '');
      requireRootWrite(state, target);
      const input = await readBody<TargetSettingsInput>(req);
      requireRevision(target.value.revision, input.expected_revision);
      target.value.repository_default_enabled = input.repository_default_enabled;
      target.value.config_patch = structuredClone(input.config_patch);
      target.value.revision += 1;
      recomputeTarget(target);
      addAudit(target, 'target.settings.updated', 'updated account defaults');
      respond(res, 200, rootTargetValue(state, target));
      return;
    }
    if (rootRepositories && method === 'GET') {
      const target = findTarget(state, rootRepositories.groups?.target ?? '');
      respond(res, 200, repositoryPage(target.repositories, parsed.searchParams));
      return;
    }
    if (rootRepository && method === 'GET') {
      const target = findTarget(state, rootRepository.groups?.target ?? '');
      respond(res, 200, findRepository(target, rootRepository.groups?.repository ?? '').detail);
      return;
    }
    if (rootRepositorySettings && method === 'PUT') {
      const target = findTarget(state, rootRepositorySettings.groups?.target ?? '');
      requireRootWrite(state, target);
      const stored = findRepository(target, rootRepositorySettings.groups?.repository ?? '');
      const input = await readBody<RepositorySettingsInput>(req);
      requireRevision(stored.detail.revision, input.expected_revision);
      stored.detail.repository.enabled_override = input.enabled_override;
      stored.detail.config_patch = structuredClone(input.config_patch);
      stored.detail.ignore_repository_file = input.ignore_repository_file;
      stored.detail.revision += 1;
      stored.detail.repository.updated_at = new Date().toISOString();
      recomputeTarget(target);
      addAudit(
        target,
        'repository.settings.updated',
        'updated repository settings for',
        stored.detail.repository.full_name,
      );
      respond(res, 200, stored.detail);
      return;
    }
    if (rootRepositoryConfigMigration && method === 'POST') {
      const target = findTarget(state, rootRepositoryConfigMigration.groups?.target ?? '');
      requireRootWrite(state, target);
      const stored = findRepository(target, rootRepositoryConfigMigration.groups?.repository ?? '');
      respond(res, 200, resetMockConfigMigration(state, target, stored));
      return;
    }

    if (installationUserDecisions && method === 'GET') {
      const target = findTarget(state, installationUserDecisions.groups?.target ?? '');
      const accountId = decodeURIComponent(installationUserDecisions.groups?.account ?? '');
      const user = targetUsers(state, target.value.id).find(
        (entry) => entry.account.id === accountId,
      );
      if (user === undefined) throw new MockApiError(404, 'not_found', 'panel user not found');
      respond(res, 200, { decisions: mockDecisions(user, target.value) });
      return;
    }

    if (installationInvitations && method === 'GET') {
      const target = findTarget(state, installationInvitations.groups?.target ?? '');
      respond(
        res,
        200,
        invitationPage(
          state.invitations.filter((entry) => entry.target_id === target.value.id),
          parsed.searchParams,
        ),
      );
      return;
    }
    if (installationInvitations && method === 'POST') {
      const target = findTarget(state, installationInvitations.groups?.target ?? '');
      if (rootScopedInvitations !== null) requireRootWrite(state, target);
      const input = await readBody<AddTargetInvitationInput>(req);
      const created = createMockInvitation(state, input, target.value);
      broadcast(state, { type: 'invitation.changed', target_id: target.value.id });
      saveDevState(state);
      respond(res, 201, invitationValue(created));
      return;
    }
    if (installationInvitationReissue && method === 'POST') {
      const target = findTarget(state, installationInvitationReissue.groups?.target ?? '');
      if (rootScopedInvitationReissue !== null) requireRootWrite(state, target);
      const current = findInvitation(state, installationInvitationReissue.groups?.invitation ?? '');
      if (current.target_id !== target.value.id) {
        throw new MockApiError(404, 'not_found', 'installation invitation not found');
      }
      const input = await readBody<{ expires_in_days: InvitationDays }>(req);
      requireReissuable(current);
      // An outstanding offer can be outgrown while it waits. Renewing it is refused on the same
      // ground as making a new one, exactly as the server does.
      refuseUnusableInvitation(state, current.account.login, { targetId: target.value.id }, true);
      current.token = mockInvitationToken(state.invitationCounter++);
      current.status = 'pending';
      current.expires_at = new Date(Date.now() + input.expires_in_days * 86_400_000).toISOString();
      current.created_at = new Date().toISOString();
      current.responded_at = undefined;
      broadcastInvitation(state, current);
      saveDevState(state);
      respond(res, 200, invitationValue(current));
      return;
    }
    if (installationInvitation && method === 'DELETE') {
      const target = findTarget(state, installationInvitation.groups?.target ?? '');
      if (rootScopedInvitation !== null) requireRootWrite(state, target);
      const current = findInvitation(state, installationInvitation.groups?.invitation ?? '');
      if (current.target_id !== target.value.id) {
        throw new MockApiError(404, 'not_found', 'installation invitation not found');
      }
      current.status = 'revoked';
      current.responded_at = new Date().toISOString();
      broadcastInvitation(state, current);
      saveDevState(state);
      respond(res, 200, publicInvitationValue(current));
      return;
    }
    if (userSuggestions && method === 'GET') {
      const target = findTarget(state, userSuggestions.groups?.target ?? '');
      const query = (parsed.searchParams.get('q') ?? '').trim().toLowerCase();
      if (query.length < 2) {
        respond(res, 200, { items: [] });
        return;
      }
      /* Stands in for the organization roster the service reads from GitHub.
         People already on the installation are dropped, the same as the service
         does - they are not candidates for being added to it. */
      const held = new Set(
        targetUsers(state, target.value.id).map((user) => user.account.login.toLowerCase()),
      );
      /* Ranked the way the service ranks: a login that starts with what was
         typed, then a display name that does, then anything containing it. */
      const rank = (account: PanelAccount): number => {
        const login = account.login.toLowerCase();
        const name = account.display_name.toLowerCase();
        if (login.startsWith(query)) return 0;
        if (name.startsWith(query)) return 1;
        if (login.includes(query) || name.includes(query)) return 2;
        return -1;
      };
      const items = MOCK_ORGANIZATION_ROSTER.filter(
        (account) => !held.has(account.login.toLowerCase()) && rank(account) !== -1,
      )
        .sort((left, right) => rank(left) - rank(right) || left.login.localeCompare(right.login))
        .slice(0, 8);
      respond(res, 200, { items });
      return;
    }

    if (installationUsers && method === 'GET') {
      const target = findTarget(state, installationUsers.groups?.target ?? '');
      respond(res, 200, userPage(targetUsers(state, target.value.id), parsed.searchParams));
      return;
    }
    if (installationUsers && method === 'POST') {
      const target = findTarget(state, installationUsers.groups?.target ?? '');
      if (rootScopedUsers !== null) requireRootWrite(state, target);
      const input = await readBody<AddTargetUserInput>(req);
      let user = state.users.find(
        (entry) => entry.account.login.toLowerCase() === input.login.toLowerCase(),
      );
      if (user === undefined) {
        user = mockUser(input.login);
        state.users.push(user);
      } else if (user.status === 'removed') {
        user.status = 'active';
        user.revision += 1;
        user.updated_at = new Date().toISOString();
      }
      const access = targetAccessFor(state, target.value.id);
      if (access.has(user.account.id)) {
        throw new MockApiError(409, 'conflict', 'this user already has installation access');
      }
      access.set(user.account.id, targetAccess(input.role, false, 1));
      broadcast(state, { type: 'access.changed', target_id: target.value.id });
      respond(res, 201, scopedUserValue(state, target.value.id, user));
      return;
    }
    if (installationUser && method === 'PUT') {
      const target = findTarget(state, installationUser.groups?.target ?? '');
      if (rootScopedUser !== null) requireRootWrite(state, target);
      const user = findUser(state, installationUser.groups?.account ?? '');
      const input = await readBody<UpdateTargetUserInput>(req);
      const access = targetAccessFor(state, target.value.id);
      const current = access.get(user.account.id);
      requireRevision(current?.revision ?? 0, input.expected_revision);
      access.set(
        user.account.id,
        targetAccess(
          input.role,
          input.suspended,
          (current?.revision ?? 0) + 1,
          input.suspension_reason,
        ),
      );
      broadcast(state, { type: 'access.changed', target_id: target.value.id });
      respond(res, 200, scopedUserValue(state, target.value.id, user));
      return;
    }

    if (targetSettings && method === 'PUT') {
      const target = findTarget(state, targetSettings.groups?.target ?? '');
      const input = await readBody<TargetSettingsInput>(req);
      requireRevision(target.value.revision, input.expected_revision);
      target.value.repository_default_enabled = input.repository_default_enabled;
      target.value.config_patch = structuredClone(input.config_patch);
      target.value.revision += 1;
      recomputeTarget(target);
      addAudit(target, 'target.settings.updated', 'updated account defaults');
      broadcast(state, { type: 'target.changed', target_id: target.value.id });
      respond(res, 200, target.value);
      return;
    }
    if (repositories && method === 'GET') {
      const target = findTarget(state, repositories.groups?.target ?? '');
      respond(res, 200, repositoryPage(target.repositories, parsed.searchParams));
      return;
    }
    if (repository && method === 'GET') {
      const target = findTarget(state, repository.groups?.target ?? '');
      respond(res, 200, findRepository(target, repository.groups?.repository ?? '').detail);
      return;
    }
    if (repositorySettings && method === 'PUT') {
      const target = findTarget(state, repositorySettings.groups?.target ?? '');
      const stored = findRepository(target, repositorySettings.groups?.repository ?? '');
      const input = await readBody<RepositorySettingsInput>(req);
      requireRevision(stored.detail.revision, input.expected_revision);
      stored.detail.repository.enabled_override = input.enabled_override;
      stored.detail.config_patch = structuredClone(input.config_patch);
      stored.detail.ignore_repository_file = input.ignore_repository_file;
      stored.detail.revision += 1;
      stored.detail.repository.updated_at = new Date().toISOString();
      recomputeTarget(target);
      addAudit(
        target,
        'repository.settings.updated',
        'updated repository settings for',
        stored.detail.repository.full_name,
      );
      broadcast(state, {
        type: 'repository.changed',
        target_id: target.value.id,
        repository_id: stored.detail.repository.id,
      });
      respond(res, 200, stored.detail);
      return;
    }
    if (repositoryConfigMigration && method === 'POST') {
      const target = findTarget(state, repositoryConfigMigration.groups?.target ?? '');
      const stored = findRepository(target, repositoryConfigMigration.groups?.repository ?? '');
      respond(res, 200, resetMockConfigMigration(state, target, stored));
      return;
    }
    if (installationAudit && method === 'GET') {
      const target = findTarget(state, installationAudit.groups?.target ?? '');
      const scope = parsed.searchParams.get('scope') ?? 'all';
      const change = parsed.searchParams.get('change') ?? 'all';
      respond(
        res,
        200,
        historyPage(
          target.audit,
          parsed.searchParams,
          (entry) => entry.created_at,
          (entry, sort) => {
            if (sort.startsWith('actor_')) return entry.actor.display_name.toLocaleLowerCase();
            if (sort.startsWith('target_')) {
              return (entry.repository_full_name ?? 'Account').toLocaleLowerCase();
            }
            if (sort.startsWith('change_')) return entry.summary.toLocaleLowerCase();
            return entry.created_at;
          },
          (entry, query) =>
            [
              entry.actor.display_name,
              entry.actor.login,
              entry.action,
              entry.summary,
              entry.repository_full_name ?? '',
            ].some((value) => value.toLocaleLowerCase().includes(query)),
          (entry) => {
            const matchesScope =
              scope === 'all' ||
              (scope === 'account' && entry.repository_full_name === undefined) ||
              (scope === 'repositories' && entry.repository_full_name !== undefined);
            const matchesChange =
              change === 'all' ||
              (change === 'enablement' &&
                ['repository.enabled', 'repository.disabled'].includes(entry.action)) ||
              (change === 'repository' &&
                entry.action.startsWith('repository.') &&
                !['repository.enabled', 'repository.disabled'].includes(entry.action)) ||
              (change === 'account' && entry.action.startsWith('target.'));
            return matchesScope && matchesChange;
          },
        ),
      );
      return;
    }
    if (installationFailures && method === 'GET') {
      const target = findTarget(state, installationFailures.groups?.target ?? '');
      const kind = parsed.searchParams.get('kind') ?? 'all';
      respond(
        res,
        200,
        historyPage(
          target.failures,
          parsed.searchParams,
          (failure) => failure.occurred_at,
          (failure, sort) => {
            if (sort.startsWith('status_')) return failure.retryable ? 1 : 0;
            if (sort.startsWith('repository_')) {
              return failure.repository_full_name.toLocaleLowerCase();
            }
            return failure.occurred_at;
          },
          (failure, query) =>
            [
              failure.delivery_id,
              failure.repository_full_name,
              failure.event,
              failure.stage,
              failure.reason,
            ].some((value) => value.toLocaleLowerCase().includes(query)),
          (failure) =>
            kind === 'all' ||
            (kind === 'retryable' && failure.retryable) ||
            (kind === 'permanent' && !failure.retryable),
        ),
      );
      return;
    }
  } catch (error) {
    if (error instanceof MockApiError) {
      respond(res, error.status, { error: { code: error.code, message: error.message } });
      return;
    }
    console.error('Smyklot panel mock failed:', error);
    respond(res, 500, { error: { code: 'internal', message: 'the mock request failed' } });
    return;
  }
  next();
}

function applyScenario(state: MockState, scenario: string | null): void {
  state.forceFailure = scenario === 'error';
  state.signedIn = scenario !== 'signed-out';
  /* Kept apart from the live list rather than emptying it, because a scenario is
     a way of looking at the mock and not a change to it. Emptying it stuck: every
     later request in the same process saw an account with no installations, and
     whatever was being looked at next quietly measured the wrong panel. */
  state.hideTargets = scenario === 'empty';
}

function findTarget(state: MockState, encodedId: string): MockTarget {
  const id = decodeURIComponent(encodedId);
  const target = state.targets.find((entry) => entry.value.id === id);
  if (target === undefined)
    throw new MockApiError(404, 'not_found', 'installation target not found');
  return target;
}

function mockRootOwns(target: MockTarget): boolean {
  return target.value.id === '2001' || target.value.id === '1001';
}

function rootInstallationValue(target: MockTarget): RootInstallation {
  const login = target.value.account.login;
  const permissionPending = login === 'team-01';
  const syncError = login === 'team-02';
  const available = !permissionPending && !syncError;
  const detail = permissionPending
    ? 'Approve the GitHub App Members permission'
    : syncError
      ? 'GitHub owner synchronization failed'
      : undefined;

  return {
    id: target.value.id,
    installation_id: target.value.installation_id,
    type: target.value.type,
    account: target.value.account,
    available,
    owned_by_viewer: mockRootOwns(target),
    repository_counts: target.value.repository_counts,
    delivery_health: {
      failed: login === 'smykla-skalski' ? 1 : 0,
      ...(login === 'smykla-skalski'
        ? { last_failure_at: new Date(Date.now() - 18 * 60_000).toISOString() }
        : {}),
    },
    ownership: {
      source: target.value.type === 'User' ? 'personal' : 'organization_admin',
      status: permissionPending ? 'permission_pending' : syncError ? 'error' : 'fresh',
      ...(detail === undefined ? {} : { detail }),
      synced_at: new Date(Date.now() - (available ? 3 : 19) * 60_000).toISOString(),
      owner_count: available ? (target.value.type === 'User' ? 1 : 2) : 0,
      // Eight stale snapshots (team-03 through team-10), matching the approved
      // overview demo's explicit Stale count.
      stale: available && /^team-(0[3-9]|10)$/.test(login),
    },
  };
}

function rootRuntimeSettingsValue(state: MockState): RootRuntimeSettings {
  const behaviorOverride = copyOptionalConfig(state.runtime.behaviorOverride);
  return {
    behavior_defaults: {
      deployment: copyConfig(DEFAULT_CONFIG),
      override: behaviorOverride,
      effective: behaviorOverride ?? copyConfig(DEFAULT_CONFIG),
    },
    log_level: {
      deployment: 'info',
      override: state.runtime.logLevelOverride,
      effective: state.runtime.logLevelOverride ?? 'info',
    },
    reaction_poll_interval: {
      deployment_seconds: 300,
      override_seconds: state.runtime.pollIntervalOverride,
      effective_seconds: state.runtime.pollIntervalOverride ?? 300,
    },
    session_lifetime: {
      deployment_seconds: 86_400,
      override_seconds: state.runtime.sessionTTLOverride,
      effective_seconds: state.runtime.sessionTTLOverride ?? 86_400,
    },
    revision: state.runtime.revision,
    ...(state.runtime.updatedAt === undefined ? {} : { updated_at: state.runtime.updatedAt }),
    ...(state.runtime.updatedBy === undefined ? {} : { updated_by: state.runtime.updatedBy }),
    service: {
      version: 'dev',
      uptime_seconds: Math.max(0, Math.floor((Date.now() - state.runtime.startedAt) / 1_000)),
      storage: 'healthy',
      database: mockDatabaseStatus(),
      listeners: { public: ':8080', admin: '127.0.0.1:8081' },
      public_paths: { panel: '/', webhook: '/webhook' },
      provider_endpoints: {
        api: 'https://api.github.com',
        authorize: 'https://github.com/login/oauth/authorize',
        token: 'https://github.com/login/oauth/access_token',
      },
      credential_presence: { webhook: true, app: true, oauth: true },
    },
  };
}

/**
 * The database the panel is looking at, mocked as the deployed one: PostgreSQL
 * over a private network, a pool with room in it, and one caller that has
 * queued for a connection at some point, so the note under the card is
 * something a developer sees rather than something only production has.
 */
function mockDatabaseStatus(): DatabaseStatus {
  return {
    state: 'healthy',
    engine: 'PostgreSQL',
    version: '18.6',
    schema_version: 1,
    size_bytes: 84_711_103,
    latency_ms: 1.24,
    connections: { open: 3, in_use: 1, idle: 2, max: 16, wait_count: 2, wait_ms: 41 },
  };
}

function copyOptionalConfig(value: ConfigValues | null): ConfigValues | null {
  return value === null ? null : copyConfig(value);
}

function copyConfig(value: ConfigValues): ConfigValues {
  return {
    ...value,
    allowed_commands: [...value.allowed_commands],
    command_aliases: { ...value.command_aliases },
  };
}

function rootOverviewValue(state: MockState): RootOverview {
  const installations = state.targets.map((target) => rootInstallationValue(target));
  const repositories = state.targets.flatMap((target) => target.repositories);
  const recentFailures = state.targets
    .flatMap((target) =>
      target.failures.map((failure) => ({ installation: target.value.account, failure })),
    )
    .sort(
      (left, right) => Date.parse(right.failure.occurred_at) - Date.parse(left.failure.occurred_at),
    )
    .slice(0, 5);

  return {
    service: {
      status: 'healthy',
      version: 'dev',
      service_host: 'local mock service',
      uptime_seconds: 9_322,
      storage: 'healthy',
      database: mockDatabaseStatus(),
    },
    catalog: {
      installations: state.targets.length,
      repositories: repositories.length,
      enabled_repositories: repositories.filter(
        (repository) => repository.detail.repository.effective_enabled,
      ).length,
    },
    ownership: {
      fresh: installations.filter(
        (installation) =>
          installation.ownership.status === 'fresh' && !installation.ownership.stale,
      ).length,
      stale: installations.filter(
        (installation) => installation.ownership.status === 'fresh' && installation.ownership.stale,
      ).length,
      permission_pending: installations.filter(
        (installation) => installation.ownership.status === 'permission_pending',
      ).length,
      error: installations.filter((installation) => installation.ownership.status === 'error')
        .length,
    },
    active_elevations: [...state.elevations.values()].filter(
      (elevation) => Date.parse(elevation.expires_at) > Date.now(),
    ).length,
    unread_security_events: state.notifications.filter(
      (notification) => notification.read_at === undefined,
    ).length,
    recent_failures: recentFailures,
    pending_ci: {
      active: state.pendingCI.filter(
        (request) => request.lifecycle === 'armed' && request.schedule === 'active',
      ),
      deferred: state.pendingCI.filter(
        (request) => request.lifecycle === 'armed' && request.schedule === 'deferred',
      ),
      recent: state.pendingCI.filter((request) => request.lifecycle !== 'armed'),
    },
  };
}

function pendingCISeeds(iso: (offsetMs: number) => string): PendingCIRequest[] {
  return [
    {
      id: 'pending-ci-1',
      repository_full_name: 'smykla-skalski/smyklot',
      pull_request: 198,
      head_sha: 'fb6ce0370e75410dc5264ba48b279581fd7229ed',
      merge_method: 'squash',
      required_checks_only: false,
      requester: 'bart',
      lifecycle: 'armed',
      schedule: 'active',
      next_check_at: iso(4 * 60_000),
      next_check_trigger: 'webhook',
      last_observed_state: 'pending',
      reason: '',
      requested_at: iso(-18 * 60_000),
      updated_at: iso(-60_000),
      cleanup_pending: false,
      revision: 3,
    },
    {
      id: 'pending-ci-2',
      repository_full_name: 'smykla-skalski/infra',
      pull_request: 72,
      head_sha: 'c5f038bf21cbd097ee9d671f8e76c7a83f6c21d4',
      merge_method: 'rebase',
      required_checks_only: true,
      requester: 'operator',
      lifecycle: 'armed',
      schedule: 'deferred',
      next_check_at: iso(5 * 3_600_000),
      next_check_trigger: 'fallback',
      last_observed_state: 'failing',
      reason: '',
      requested_at: iso(-8 * 3_600_000),
      updated_at: iso(-3 * 3_600_000),
      cleanup_pending: false,
      revision: 7,
    },
  ];
}

function findPendingCI(state: MockState, encodedID: string): PendingCIRequest {
  const id = decodeURIComponent(encodedID);
  const request = state.pendingCI.find((candidate) => candidate.id === id);
  if (request === undefined) {
    throw new MockApiError(404, 'not_found', 'pending CI request not found');
  }
  return request;
}

function requirePendingCIRevision(request: PendingCIRequest, revision: number): void {
  if (revision !== request.revision) {
    throw new MockApiError(409, 'conflict', 'pending CI request changed; reload and try again');
  }
}

function pendingCIDetail(request: PendingCIRequest): PendingCIDetail {
  return {
    request: structuredClone(request),
    events: [
      {
        id: `${request.id}:wake`,
        kind: 'wake_received',
        trigger: 'webhook',
        event_name: 'check_suite',
        event_key: `check_suite:${request.head_sha}:completed:success`,
        delivery_id: 'mock-delivery-id',
        summary: 'Received a CI state webhook and scheduled an immediate reconciliation',
        created_at: request.updated_at,
      },
    ],
  };
}

function rootAuditEntries(state: MockState): AuditEntry[] {
  const installationEvents = state.targets.flatMap((target) =>
    target.audit.map((entry) => ({
      ...entry,
      id: `${target.value.id}-${entry.id}`,
      category: 'configuration' as const,
      installation: target.value.account,
    })),
  );
  const primaryInstallation = state.targets[0]?.value.account;
  const subject = state.users[0]?.account;
  const now = Date.now();
  const systemEvents: AuditEntry[] = [
    {
      id: 'root-runtime-1',
      category: 'runtime',
      actor: VIEWER,
      action: 'runtime.settings.updated',
      summary: 'Updated panel session lifetime',
      created_at: new Date(now - 7 * 60_000).toISOString(),
    },
    {
      id: 'root-elevation-1',
      category: 'elevation',
      installation: primaryInstallation,
      actor: VIEWER,
      elevation_id: 'elevation-204',
      action: 'elevation.started',
      summary: 'Started audited installation access',
      created_at: new Date(now - 22 * 60_000).toISOString(),
    },
    {
      id: 'root-access-1',
      category: 'access',
      installation: primaryInstallation,
      actor: VIEWER,
      subject,
      action: 'target.access.updated',
      summary: 'Updated installation access',
      created_at: new Date(now - 48 * 60_000).toISOString(),
    },
    {
      id: 'root-ownership-1',
      category: 'ownership',
      installation: primaryInstallation,
      actor: VIEWER,
      action: 'ownership.synced',
      summary: 'Synchronized installation owners',
      created_at: new Date(now - 76 * 60_000).toISOString(),
    },
    {
      id: 'root-notification-1',
      category: 'notification',
      installation: primaryInstallation,
      actor: VIEWER,
      action: 'owner.notification.created',
      summary: 'Notified owners about elevated access',
      created_at: new Date(now - 80 * 60_000).toISOString(),
    },
  ];

  return [...installationEvents, ...systemEvents].sort(
    (left, right) => Date.parse(right.created_at) - Date.parse(left.created_at),
  );
}

function rootPanelUsers(state: MockState): RootPanelUser[] {
  return state.users.map((user) => ({
    account: user.account,
    system_role: user.system_role,
    status: user.status,
    ...(user.ban_reason === undefined ? {} : { ban_reason: user.ban_reason }),
    ...(user.banned_at === undefined ? {} : { banned_at: user.banned_at }),
    ...(user.last_login_at === undefined ? {} : { last_login_at: user.last_login_at }),
    revision: user.revision,
    owned_installations:
      user.account.id === VIEWER.id
        ? state.targets.filter((target) => mockRootOwns(target)).length
        : 0,
    assigned_installations: state.targets.filter((target) =>
      state.targetAccess.get(target.value.id)?.has(user.account.id),
    ).length,
    manageable: user.account.id !== VIEWER.id && user.system_role === 'none',
    can_manage_system_role: user.account.id !== VIEWER.id && user.system_role !== 'super_root',
  }));
}

function activeMockElevation(state: MockState, targetId: string): RootElevation | undefined {
  const elevation = state.elevations.get(targetId);
  if (elevation === undefined) return undefined;
  if (Date.parse(elevation.expires_at) > Date.now()) return elevation;
  state.elevations.delete(targetId);
  return undefined;
}

function rootTargetValue(state: MockState, target: MockTarget): PanelTarget {
  if (mockRootOwns(target)) return structuredClone(target.value);
  const elevated = activeMockElevation(state, target.value.id) !== undefined;
  return {
    ...structuredClone(target.value),
    effective_role: 'none',
    access_source: elevated ? 'elevation' : 'root',
    capabilities: { ...ROOT_READ_CAPABILITIES, write: elevated },
  };
}

function requireRootWrite(state: MockState, target: MockTarget): void {
  if (mockRootOwns(target) || activeMockElevation(state, target.value.id) !== undefined) return;
  throw new MockApiError(403, 'elevation_required', 'start elevated access for this installation');
}

function findUser(state: MockState, encodedId: string): PanelUser {
  const id = decodeURIComponent(encodedId);
  const user = state.users.find((entry) => entry.account.id === id);
  if (user === undefined) throw new MockApiError(404, 'not_found', 'panel user not found');
  return user;
}

function findInvitation(state: MockState, encodedId: string): MockInvitation {
  const id = decodeURIComponent(encodedId);
  const invitation = state.invitations.find((entry) => entry.id === id);
  if (invitation === undefined) throw new MockApiError(404, 'not_found', 'invitation not found');
  return invitation;
}

function findInvitationByToken(state: MockState, encodedToken: string): MockInvitation {
  const token = decodeURIComponent(encodedToken);
  const invitation = state.invitations.find((entry) => entry.token === token);
  if (invitation === undefined) throw new MockApiError(404, 'not_found', 'invitation not found');
  return invitation;
}

/**
 * The refusals the panel's server makes, so the dev panel walks into the same walls.
 *
 * Kept beside the two creators rather than inside them: the login is checked against the signed-in
 * viewer before anything is resolved, which is the one refusal the panel can also make for itself.
 */
function refuseUnusableInvitation(
  state: MockState,
  login: string,
  scope: { targetId?: string },
  acknowledgedDecline: boolean,
): void {
  const normalized = login.trim().toLowerCase();
  if (normalized === VIEWER.login.toLowerCase()) {
    throw new MockApiError(403, 'self_invitation', 'you cannot invite yourself');
  }
  const user = state.users.find((entry) => entry.account.login.toLowerCase() === normalized);
  if (scope.targetId === undefined) {
    if (user !== undefined && user.status !== 'removed') {
      throw new MockApiError(
        409,
        'already_has_access',
        'this account is already in Smyklot; change its system role instead of inviting it',
      );
    }
  } else if (user !== undefined && user.status === 'active') {
    const held = state.targetAccess.get(scope.targetId)?.get(user.account.id)?.effective_role;
    if (held !== undefined && held !== 'none') {
      throw new MockApiError(
        409,
        'already_has_access',
        'this user already has access to this installation; change their role instead',
      );
    }
  }
  if (acknowledgedDecline) return;
  const last = state.invitations
    .filter(
      (entry) =>
        entry.account.login.toLowerCase() === normalized &&
        (scope.targetId === undefined
          ? entry.system_role === 'root'
          : entry.target_id === scope.targetId),
    )
    .sort(
      (left, right) =>
        right.created_at.localeCompare(left.created_at) || right.id.localeCompare(left.id),
    )[0];
  if (last?.status === 'declined') {
    throw new MockApiError(
      409,
      'invitation_declined',
      'this user declined the last invitation; confirm to send another',
    );
  }
}

/**
 * Only an offer still in play can be renewed. A declined one is an answer, and the server refuses
 * to reissue it - letting it through here would teach the opposite rule in dev.
 */
function requireReissuable(invitation: MockInvitation): void {
  if (invitation.status === 'pending' || invitation.status === 'expired') return;
  throw new MockApiError(409, 'conflict', 'this invitation is no longer pending');
}

function createMockInvitation(
  state: MockState,
  input: AddTargetInvitationInput,
  target: PanelTarget,
): MockInvitation {
  const now = new Date();
  refuseUnusableInvitation(
    state,
    input.login,
    { targetId: target.id },
    input.acknowledge_declined === true,
  );
  const account = mockUser(input.login).account;
  for (const invitation of state.invitations) {
    if (
      invitation.account.login.toLowerCase() === input.login.toLowerCase() &&
      invitation.target_id === target?.id &&
      invitation.status === 'pending'
    ) {
      invitation.status = 'revoked';
      invitation.responded_at = now.toISOString();
    }
  }
  const counter = state.invitationCounter++;
  const invitation: MockInvitation = {
    id: `mock-invitation-${counter}`,
    token: mockInvitationToken(counter),
    account,
    target_id: target.id,
    target_name: target.account.display_name,
    target_login: target.account.login,
    target_kind: target.type,
    role: input.role,
    status: 'pending',
    expires_at: new Date(now.getTime() + input.expires_in_days * 86_400_000).toISOString(),
    created_by: VIEWER,
    created_at: now.toISOString(),
  };
  state.invitations.unshift(invitation);
  return invitation;
}

function createRootMockInvitation(state: MockState, input: AddRootInvitationInput): MockInvitation {
  const now = new Date();
  refuseUnusableInvitation(state, input.login, {}, input.acknowledge_declined === true);
  const account = mockUser(input.login).account;
  for (const invitation of state.invitations) {
    if (
      invitation.account.login.toLowerCase() === input.login.toLowerCase() &&
      invitation.system_role === 'root' &&
      invitation.status === 'pending'
    ) {
      invitation.status = 'revoked';
      invitation.responded_at = now.toISOString();
    }
  }
  const counter = state.invitationCounter++;
  const invitation: MockInvitation = {
    id: `mock-root-invitation-${counter}`,
    token: mockInvitationToken(counter),
    account,
    system_role: 'root',
    status: 'pending',
    expires_at: new Date(now.getTime() + input.expires_in_days * 86_400_000).toISOString(),
    created_by: VIEWER,
    created_at: now.toISOString(),
  };
  state.invitations.unshift(invitation);
  return invitation;
}

function mockInvitationToken(counter: number): string {
  return `mock-${String(counter).padStart(38, '0')}`;
}

function publicInvitationValue(invitation: MockInvitation): PanelInvitation {
  return structuredClone({
    id: invitation.id,
    account: invitation.account,
    ...(invitation.target_id === undefined ? {} : { target_id: invitation.target_id }),
    ...(invitation.target_name === undefined ? {} : { target_name: invitation.target_name }),
    ...(invitation.target_login === undefined ? {} : { target_login: invitation.target_login }),
    ...(invitation.target_kind === undefined ? {} : { target_kind: invitation.target_kind }),
    role: invitation.role,
    ...(invitation.system_role === undefined ? {} : { system_role: invitation.system_role }),
    status: invitation.status,
    expires_at: invitation.expires_at,
    created_by: invitation.created_by,
    created_at: invitation.created_at,
    ...(invitation.responded_at === undefined ? {} : { responded_at: invitation.responded_at }),
  });
}

function invitationValue(invitation: MockInvitation): PanelInvitation {
  return {
    ...publicInvitationValue(invitation),
    invite_url: `${devOrigin}/invite/${encodeURIComponent(invitation.token)}`,
  };
}

function broadcastInvitation(state: MockState, invitation: MockInvitation): void {
  if (invitation.target_id === undefined) {
    broadcast(state, { type: 'resync' });
  } else {
    broadcast(state, { type: 'invitation.changed', target_id: invitation.target_id });
  }
}

function mockUser(login: string): PanelUser {
  const normalized = login.trim();
  const now = new Date().toISOString();
  return {
    account: {
      id: `github:mock:user:${normalized.toLowerCase()}`,
      provider: VIEWER.provider,
      subject_id: normalized.toLowerCase(),
      login: normalized,
      display_name: normalized,
      avatar_url: null,
    },
    system_role: 'none',
    status: 'active',
    revision: 1,
    created_at: now,
    updated_at: now,
    manageable: true,
  };
}

function targetAccessFor(state: MockState, targetId: string): Map<string, TargetUserAccess> {
  let access = state.targetAccess.get(targetId);
  if (access === undefined) {
    access = new Map();
    state.targetAccess.set(targetId, access);
  }
  return access;
}

function targetAccess(
  role: TargetUserAccess['role'],
  suspended: boolean,
  revision: number,
  reason?: string,
): TargetUserAccess {
  const effectiveRole = suspended ? 'none' : (role ?? 'none');
  return {
    role,
    suspended,
    ...(reason === undefined || reason.trim() === '' ? {} : { suspension_reason: reason.trim() }),
    revision,
    updated_at: new Date().toISOString(),
    effective_role: effectiveRole,
    source: suspended ? 'suspended' : role === null ? 'denied' : 'target',
    capabilities: capabilitiesFor(effectiveRole),
  };
}

function scopedUserValue(state: MockState, targetId: string, user: PanelUser): PanelUser {
  const access = targetAccessFor(state, targetId).get(user.account.id);
  if (access === undefined) throw new MockApiError(404, 'not_found', 'installation role not found');
  return { ...structuredClone(user), target_access: structuredClone(access) };
}

function findRepository(target: MockTarget, encodedSelector: string): MockRepository {
  const selector = decodeURIComponent(encodedSelector);
  const repository = target.repositories.find(
    (entry) => entry.detail.repository.id === selector || entry.detail.repository.name === selector,
  );
  if (repository === undefined) throw new MockApiError(404, 'not_found', 'repository not found');
  return repository;
}

function requireRevision(current: number, expected: number): void {
  if (current !== expected) {
    throw new MockApiError(
      409,
      'conflict',
      'settings changed in another session; latest values were reloaded',
    );
  }
}

function addAudit(target: MockTarget, action: string, summary: string, repository?: string): void {
  target.audit.unshift({
    id: `audit-${Date.now()}`,
    actor: VIEWER,
    action,
    summary,
    repository_full_name: repository,
    created_at: new Date().toISOString(),
  });
}

function resetMockConfigMigration(
  state: MockState,
  target: MockTarget,
  stored: MockRepository,
): RepositoryDetail {
  if (stored.detail.config_migration === 'none') return stored.detail;
  if (
    stored.detail.config_migration !== 'declined' &&
    stored.detail.config_migration !== 'blocked'
  ) {
    throw new MockApiError(409, 'conflict', 'the migration is no longer declined');
  }
  stored.detail.config_migration = 'none';
  stored.detail.config_migration_pr = undefined;
  addAudit(
    target,
    'repository.config_migration.reset',
    'Allowed Smyklot to propose the TOML migration again',
    stored.detail.repository.full_name,
  );
  broadcast(state, {
    type: 'repository.changed',
    target_id: target.value.id,
    repository_id: stored.detail.repository.id,
  });

  return stored.detail;
}

function mockDecisions(user: PanelUser, target: PanelTarget): AccessDecision[] {
  const now = Date.now();
  if (user.status === 'banned') {
    // The approved dialog demo: two decisions with audit references.
    return [
      {
        id: `${user.account.id}-decision-2`,
        actor: VIEWER,
        action: 'target.access.banned',
        summary: 'Sessions revoked immediately \u00b7 Audit #188',
        created_at: new Date(now - 9 * 86_400_000).toISOString(),
      },
      {
        id: `${user.account.id}-decision-1`,
        actor: VIEWER,
        action: 'target.access.suspended',
        summary: 'Warning after repeated force-merge attempts \u00b7 Audit #172',
        created_at: new Date(now - 14 * 86_400_000).toISOString(),
      },
    ];
  }
  const current =
    user.target_access?.suspended === true
      ? {
          action: 'target.access.suspended',
          summary: `suspended installation access${user.target_access.suspension_reason === undefined ? '' : `: ${user.target_access.suspension_reason}`}`,
          created_at: user.target_access.updated_at ?? new Date(now - 3 * 86_400_000).toISOString(),
        }
      : {
          action: 'target.access.updated',
          summary: `updated access to ${target.account.display_name}`,
        };
  return [
    {
      id: `${user.account.id}-decision-3`,
      actor: VIEWER,
      ...current,
      created_at: current.created_at ?? new Date(now - 2 * 3_600_000).toISOString(),
    },
    {
      id: `${user.account.id}-decision-2`,
      actor: VIEWER,
      action: 'target.access.updated',
      summary: 'updated installation access',
      created_at: new Date(now - 18 * 86_400_000).toISOString(),
    },
    {
      id: `${user.account.id}-decision-1`,
      actor: VIEWER,
      action: 'user.created',
      summary: 'added user as viewer',
      created_at: new Date(now - 45 * 86_400_000).toISOString(),
    },
  ];
}

function userPage(users: PanelUser[], parameters: URLSearchParams): Page<PanelUser> {
  const query = (parameters.get('q') ?? '').trim().toLocaleLowerCase();
  const roles = parameters.getAll('role');
  const statuses = parameters.getAll('status');
  const ordered = users
    .filter((user) => {
      const role = user.target_access?.effective_role;
      const status =
        user.status === 'active' && user.target_access?.suspended === true
          ? 'suspended'
          : user.status;
      return (
        (query === '' ||
          user.account.login.toLocaleLowerCase().includes(query) ||
          user.account.display_name.toLocaleLowerCase().includes(query)) &&
        (roles.length === 0 || (role !== undefined && roles.includes(role))) &&
        (statuses.length === 0 || statuses.includes(status))
      );
    })
    .map((user) => structuredClone(user));

  const roleLevel = (user: PanelUser): number => {
    const role = user.target_access?.effective_role ?? 'none';
    return ['none', 'viewer', 'editor', 'admin', 'owner'].indexOf(role);
  };

  switch (parameters.get('sort')) {
    case 'name_desc':
      ordered.sort((left, right) =>
        right.account.display_name.localeCompare(left.account.display_name),
      );
      break;
    case 'role_asc':
      ordered.sort(
        (left, right) =>
          roleLevel(left) - roleLevel(right) ||
          left.account.display_name.localeCompare(right.account.display_name),
      );
      break;
    case 'role_desc':
      ordered.sort(
        (left, right) =>
          roleLevel(right) - roleLevel(left) ||
          left.account.display_name.localeCompare(right.account.display_name),
      );
      break;
    case 'updated_newest':
      ordered.sort((left, right) => right.updated_at.localeCompare(left.updated_at));
      break;
    case 'updated_oldest':
      ordered.sort((left, right) => left.updated_at.localeCompare(right.updated_at));
      break;
    case 'login_newest':
      ordered.sort((left, right) =>
        (right.last_login_at ?? '').localeCompare(left.last_login_at ?? ''),
      );
      break;
    case 'login_oldest':
      ordered.sort((left, right) =>
        (left.last_login_at ?? 'z').localeCompare(right.last_login_at ?? 'z'),
      );
      break;
    default:
      ordered.sort((left, right) =>
        left.account.display_name.localeCompare(right.account.display_name),
      );
  }
  return offsetPage(ordered, parameters);
}

function invitationPage(
  invitations: MockInvitation[],
  parameters: URLSearchParams,
): Page<PanelInvitation> {
  const query = (parameters.get('q') ?? '').trim().toLocaleLowerCase();
  const roles = parameters.getAll('role');
  const statuses = parameters.getAll('status');
  const now = Date.now();
  const roleLevel = (invitation: PanelInvitation): number =>
    invitation.system_role === 'root'
      ? 4
      : ['viewer', 'editor', 'admin'].indexOf(invitation.role ?? '');
  const ordered = invitations
    .map((invitation) => {
      const value = publicInvitationValue(invitation);
      if (value.status === 'pending' && Date.parse(value.expires_at) <= now)
        value.status = 'expired';
      return value;
    })
    .filter(
      (invitation) =>
        (query === '' ||
          invitation.account.login.toLocaleLowerCase().includes(query) ||
          invitation.account.display_name.toLocaleLowerCase().includes(query) ||
          invitation.created_by.login.toLocaleLowerCase().includes(query)) &&
        (roles.length === 0 || roles.includes(invitation.role ?? '')) &&
        (statuses.length === 0 || statuses.includes(invitation.status)),
    );

  switch (parameters.get('sort')) {
    case 'created_oldest':
      ordered.sort((left, right) => left.created_at.localeCompare(right.created_at));
      break;
    case 'expiry_soonest':
      ordered.sort((left, right) => left.expires_at.localeCompare(right.expires_at));
      break;
    case 'expiry_latest':
      ordered.sort((left, right) => right.expires_at.localeCompare(left.expires_at));
      break;
    case 'name_asc':
      ordered.sort((left, right) =>
        left.account.display_name.localeCompare(right.account.display_name),
      );
      break;
    case 'name_desc':
      ordered.sort((left, right) =>
        right.account.display_name.localeCompare(left.account.display_name),
      );
      break;
    case 'role_asc':
      ordered.sort(
        (left, right) =>
          roleLevel(left) - roleLevel(right) ||
          left.account.display_name.localeCompare(right.account.display_name),
      );
      break;
    case 'role_desc':
      ordered.sort(
        (left, right) =>
          roleLevel(right) - roleLevel(left) ||
          left.account.display_name.localeCompare(right.account.display_name),
      );
      break;
    default:
      ordered.sort((left, right) => right.created_at.localeCompare(left.created_at));
  }
  return offsetPage(ordered, parameters);
}

function offsetPage<T>(items: T[], parameters: URLSearchParams): Page<T> {
  const requestedLimit = Number.parseInt(parameters.get('limit') ?? '', 10);
  const limit =
    Number.isFinite(requestedLimit) && requestedLimit > 0
      ? Math.min(requestedLimit, 100)
      : DEFAULT_PAGE_SIZE;
  const requestedOffset = Number.parseInt(parameters.get('cursor') ?? '', 10);
  const offset = Number.isFinite(requestedOffset) && requestedOffset >= 0 ? requestedOffset : 0;
  const next = offset + limit;
  return {
    items: items.slice(offset, next),
    next_cursor: next < items.length ? String(next) : null,
    total: items.length,
  };
}

function historyPage<T>(
  items: T[],
  parameters: URLSearchParams,
  timestamp: (item: T) => string,
  sortValue: (item: T, sort: string) => string | number,
  matches: (item: T, query: string) => boolean,
  visible: (item: T) => boolean,
): Page<T> {
  const requestedLimit = Number.parseInt(parameters.get('limit') ?? '', 10);
  const limit =
    Number.isFinite(requestedLimit) && requestedLimit > 0
      ? Math.min(requestedLimit, 100)
      : DEFAULT_PAGE_SIZE;
  const query = (parameters.get('q') ?? '').trim().toLocaleLowerCase();
  const sort = parameters.get('sort') ?? 'newest';
  const direction = sort === 'newest' || sort.endsWith('_desc') ? -1 : 1;
  const ordered = items
    .filter((item) => visible(item) && (query === '' || matches(item, query)))
    .sort((left, right) => {
      const leftValue = sortValue(left, sort);
      const rightValue = sortValue(right, sort);
      const comparison =
        typeof leftValue === 'number' && typeof rightValue === 'number'
          ? leftValue - rightValue
          : String(leftValue).localeCompare(String(rightValue));
      return comparison * direction || timestamp(right).localeCompare(timestamp(left));
    });

  const offset = Number.parseInt(parameters.get('cursor') ?? '', 10);
  const safeOffset = Number.isFinite(offset) && offset >= 0 ? offset : 0;
  const next = safeOffset + limit;
  return {
    items: ordered.slice(safeOffset, next),
    next_cursor: next < ordered.length ? String(next) : null,
    total: ordered.length,
  };
}

function repositoryPage(
  repositories: MockRepository[],
  parameters: URLSearchParams,
): Page<RepositorySummary> {
  const requestedLimit = Number.parseInt(parameters.get('limit') ?? '', 10);
  const limit =
    Number.isFinite(requestedLimit) && requestedLimit > 0
      ? Math.min(requestedLimit, 100)
      : DEFAULT_PAGE_SIZE;
  const query = (parameters.get('q') ?? '').trim().toLocaleLowerCase();
  const state = parameters.get('state') ?? 'all';
  const files = parameters.getAll('file').filter((value) => value !== 'all');
  const settings = parameters.getAll('setting').filter((value) => value !== 'all');
  const ordered = repositories
    .filter((entry) => {
      const repository = entry.detail.repository;
      const settingKeys = Object.keys(entry.detail.config_patch);
      return (
        (query === '' || repository.full_name.toLocaleLowerCase().includes(query)) &&
        (state === 'all' ||
          (state === 'enabled' && repository.effective_enabled) ||
          (state === 'disabled' && !repository.effective_enabled)) &&
        (files.length === 0 || files.includes(repository.config_file_status)) &&
        (settings.length === 0 ||
          (settings.length === 1 && settings[0] === 'custom' && settingKeys.length > 0) ||
          (settings.length === 1 && settings[0] === 'none' && settingKeys.length === 0) ||
          settings.some((setting) => settingKeys.includes(setting)))
      );
    })
    .map((entry) => entry.detail.repository);

  switch (parameters.get('sort')) {
    case 'name_desc':
      ordered.sort((left, right) => right.full_name.localeCompare(left.full_name));
      break;
    case 'file_asc':
    case 'file_desc': {
      const direction = parameters.get('sort') === 'file_desc' ? -1 : 1;
      ordered.sort(
        (left, right) =>
          left.config_file_status.localeCompare(right.config_file_status) * direction ||
          left.full_name.localeCompare(right.full_name),
      );
      break;
    }
    case 'overrides_asc':
    case 'overrides_desc': {
      const direction = parameters.get('sort') === 'overrides_desc' ? -1 : 1;
      ordered.sort(
        (left, right) =>
          (left.config_override_count - right.config_override_count) * direction ||
          left.full_name.localeCompare(right.full_name),
      );
      break;
    }
    case 'newest':
      ordered.sort((left, right) => right.updated_at.localeCompare(left.updated_at));
      break;
    case 'oldest':
      ordered.sort((left, right) => left.updated_at.localeCompare(right.updated_at));
      break;
    default:
      ordered.sort((left, right) => left.full_name.localeCompare(right.full_name));
  }

  const offset = Number.parseInt(parameters.get('cursor') ?? '', 10);
  const safeOffset = Number.isFinite(offset) && offset >= 0 ? offset : 0;
  const next = safeOffset + limit;
  return {
    items: ordered.slice(safeOffset, next),
    next_cursor: next < ordered.length ? String(next) : null,
    total: ordered.length,
  };
}

function cycled<T>(items: readonly T[], index: number): T {
  const item = items[index % items.length];
  if (item === undefined) throw new Error('cannot cycle through an empty collection');

  return item;
}

function broadcast(state: MockState, event: Record<string, string>): void {
  for (const stream of state.streams) writeWebSocket(stream, { version: 1, ...event });
}

function writeWebSocket(socket: Duplex, event: Record<string, unknown>): void {
  const payload = Buffer.from(JSON.stringify(event));
  let header: Buffer;
  if (payload.length < 126) {
    header = Buffer.from([0x81, payload.length]);
  } else if (payload.length <= 0xffff) {
    header = Buffer.alloc(4);
    header[0] = 0x81;
    header[1] = 126;
    header.writeUInt16BE(payload.length, 2);
  } else {
    throw new Error('mock WebSocket frame is too large');
  }
  socket.write(Buffer.concat([header, payload]));
}

function route(path: string): string {
  return `${BASE}${path}`;
}

async function readBody<T>(req: IncomingMessage): Promise<T> {
  const chunks: Buffer[] = [];
  for await (const chunk of req) chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
  try {
    return JSON.parse(Buffer.concat(chunks).toString('utf8')) as T;
  } catch {
    throw new MockApiError(400, 'invalid_request', 'request body must be valid JSON');
  }
}

function respond(res: ServerResponse, status: number, body: unknown): void {
  res.statusCode = status;
  if (body === null) {
    res.end();
    return;
  }
  res.setHeader('Content-Type', 'application/json');
  res.end(JSON.stringify(body));
}

/**
 * The production answer to an error: the panel's own page for a browser that
 * navigated here, and JSON for everything else.
 *
 * Mirrors `writePageError` in internal/panel/error_page.go, including how it
 * decides which of the two the caller wants. Without it the mock hands a browser
 * a JSON blob where production hands it a page, and the error pages could only be
 * looked at by building the Go binary.
 */
async function respondError(
  state: MockState,
  req: IncomingMessage,
  res: ServerResponse,
  status: number,
  code: string,
  message: string,
): Promise<void> {
  if (!wantsDocument(req)) {
    respond(res, status, { error: { code, message } });
    return;
  }
  const page = await renderErrorDocument(state, req.url ?? '/', status, code, message);
  res.statusCode = status;
  res.setHeader('Content-Type', 'text/html; charset=utf-8');
  res.setHeader('Cache-Control', 'no-store');
  res.end(page);
}

function wantsDocument(req: IncomingMessage): boolean {
  const destination = req.headers['sec-fetch-dest'];
  if (typeof destination === 'string' && destination !== '') return destination === 'document';

  return (req.headers.accept ?? '')
    .split(',')
    .some((entry) => entry.split(';')[0]?.trim() === 'text/html');
}

async function renderErrorDocument(
  state: MockState,
  url: string,
  status: number,
  code: string,
  message: string,
): Promise<string> {
  const source = readFileSync(resolve(FRONTEND_DIR, 'src/app.html'), 'utf8');
  const transformed = await state.transformIndex(url, source);
  const descriptor = escapeHtml(JSON.stringify({ status, code, message }));

  return transformed
    .replace(
      /(<meta name="smyklot-panel-error" content=")[^"]*(")/u,
      (_match, head: string, tail: string) => `${head}${descriptor}${tail}`,
    )
    .replace(
      /(<noscript>)[^<]*(<\/noscript>)/u,
      (_match, head: string, tail: string) =>
        `${head}${escapeHtml(`${status} - ${message}`)}${tail}`,
    );
}

function escapeHtml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&#34;')
    .replaceAll("'", '&#39;');
}

export default mockServer;
