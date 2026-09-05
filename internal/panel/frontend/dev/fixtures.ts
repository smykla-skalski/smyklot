/**
 * The panel's fixture data, and the only copy of it.
 *
 * This was 900 lines inside `mock-server.ts`, reachable by nothing: the module exports
 * `mockServer()` and a default, and every constant and builder behind them was private.
 * The Storybook catalogue needs exactly this data, and a story that restated it would
 * be a second set of fixtures to keep in step with the first - agreeing with the mock
 * on the day it was written and drifting from it thereafter, which is how a story comes
 * to show a shape the service no longer sends.
 *
 * **Nothing here may import from `node:`.** `mock-server.ts` pulls in `node:crypto`,
 * `node:fs`, `node:http` and `node:stream`; a browser-side story importing any of them
 * transitively would fail to build. The split is clean because every `node:` call was
 * already in the plugin half - `seed()` had only two impurities, and both are now
 * parameters:
 *
 * - the issued invitations it read off disk, which a story passes empty
 * - `Date.now()`, which a story passes as a fixed instant. That is the only way a story
 *   showing "3 minutes ago" can be looked at twice and agree with itself, and it makes
 *   the browser suite's mock deterministic for free.
 *
 * Types come from `src/lib/types.ts`, the same ones the service is written against, so
 * these fixtures are checked against the real API contract rather than a copy of it.
 */

import type {
  AuditEntry,
  ConfigKey,
  ConfigPatch,
  ConfigSources,
  ConfigValues,
  DeliveryFailure,
  PanelAccount,
  PanelInvitation,
  PanelTarget,
  PanelUser,
  RootPanelUser,
  PendingCIRequest,
  QueueItem,
  WorkspaceRole,
  TargetUserAccess,
  RepositoryDetail,
  RepositorySummary,
  RootElevation,
  SettingsCheckpoint,
  SyncCell,
  SyncConfig,
  SyncOverride,
  SyncPlan,
  SyncStatus,
  SecurityNotification,
  InvitationStatus,
} from '../src/lib/types.ts';
import { CONFIG_KEYS } from '../src/lib/config.ts';
import {
  applyFormattingPatch,
  applyFormattingSources,
  defaultFormattingPolicy,
  formattingSources,
} from '../src/lib/formatting.ts';

export const DEFAULT_CONFIG: ConfigValues = {
  formatting: defaultFormattingPolicy(),
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
  allow_draft_merges: false,
};

/**
 * Where the mock keeps a fixture account's profile picture. Production
 * accounts carry GitHub avatar URLs; the signed-in user and the workspaces
 * carry one here too, so the panel's image path renders in dev rather than
 * only ever the monogram fallback. Roster and seeded users stay bare - the
 * fallback needs exercising somewhere.
 */
export function devAvatarUrl(login: string): string {
  return `/__avatar/${login}.svg`;
}

export const VIEWER: PanelAccount = {
  id: '1001',
  provider: 'github:https://api.github.com',
  subject_id: '1001',
  login: 'bart',
  display_name: 'Bart Smykla',
  avatar_url: devAvatarUrl('bart'),
};

/**
 * Stands in for the organization roster the service reads from GitHub.
 *
 * Deliberately holds names that overlap on different parts - a login starting
 * with the query, a display name starting with it, and one that only contains it
 * - so the ordering the panel applies is visible while working on it.
 */
export const MOCK_ORGANIZATION_ROSTER: PanelAccount[] = [
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

export const OWNER_CAPABILITIES = {
  read: true,
  write: true,
  manage_target_users: true,
};

export const ROOT_READ_CAPABILITIES = {
  read: true,
  write: false,
  manage_target_users: false,
};

export interface MockRepository {
  detail: RepositoryDetail;
  filePatch: ConfigPatch;
}

export interface MockTarget {
  value: PanelTarget;
  repositories: MockRepository[];
  audit: AuditEntry[];
  failures: DeliveryFailure[];
}

export interface MockInvitation extends PanelInvitation {
  token: string;
}

/**
 * Everything the mock serves, and nothing it serves it WITH.
 *
 * The open websockets and the shell fetcher used to sit in here beside the data. They
 * are the plugin's plumbing, they are typed against `node:stream`, and a fixture module
 * that named them could not be read from a browser. `mock-server.ts` extends this with
 * both, which is the honest division: this is the state, that is the server.
 */
export interface MockState {
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
  queue: QueueItem[];
  /**
   * The work this process has watched all the way through, and may therefore arm again.
   *
   * The reconciler recycles what it finishes, so the loop can be watched more than once. What it did
   * not finish is the past: the three seeded outcomes are what the Recent table exists to show, and
   * a past that arms itself again is not a past. Keyed off the trigger instead, one of them matched
   * - `pending-ci-3` merged two hours before the process started - and Recent quietly lost a row.
   *
   * Both tables are in here. The two id spaces do not meet, and the rule is the same one.
   */
  queueLoop: Set<string>;
  /**
   * What each queue row rests as, so the reconciler can put it back.
   *
   * The seeds are the shape worth showing - one row running, one waiting on checks, one
   * retrying after a rate limit - and the reconciler walks each of them to done. Without
   * somewhere to read the resting shape from, re-arming would have to guess which
   * waiting state a finished row came from and what it was blocked on.
   */
  queueRest: Map<string, QueueItem>;
  runtime: {
    backgroundWorkPaused: boolean;
    behaviorOverride: ConfigValues | null;
    logLevelOverride: string | null;
    pollIntervalOverride: number | null;
    pendingCIQuietPeriodOverride: number | null;
    pathIndexIntervalOverride: number | null;
    sessionTTLOverride: number | null;
    revision: number;
    checkpointCounter: number;
    checkpoints: Map<string, SettingsCheckpoint>;
    audit: AuditEntry[];
    updatedAt?: string;
    updatedBy?: PanelAccount;
    startedAt: number;
  };
  workspaceSettings: {
    checkpointCounter: number;
    checkpoints: Map<string, SettingsCheckpoint>;
  };
  prefs: { values: Record<string, unknown>; rev: number };
  /** Label sync, per workspace: what is configured and what is in flight. */
  sync: Map<string, SyncConfig>;
  /** What each repository adjusts, keyed by repository and kind together. */
  syncOverrides: Map<string, SyncOverride>;
  syncPlans: Map<string, SyncPlan>;
  /** The fleet: where every covered repository stands, per workspace. */
  syncStatus: Map<string, SyncStatus>;
}

/**
 * mockSyncConfig answers what a workspace has configured, inventing an
 * empty answer the first time. Never configured is not an error and not the
 * same as configured and switched off, which is what the server says too.
 */
/**
 * Every fixture, as one state object.
 *
 * `issued` is the invitations the dev mock persisted to disk between runs; a story
 * passes none. `now` is the instant every timestamp is derived from - pass a fixed one
 * and the data is reproducible, which is what a relative time like "3 minutes ago"
 * needs to mean the same thing twice.
 */
export function seed(
  issued: { invitations: MockInvitation[]; counter: number } = { invitations: [], counter: 1 },
  now: number = Date.UTC(2026, 7, 18),
  prefs: MockState['prefs'] = { values: {}, rev: 0 },
): MockState {
  const iso = (offsetMs: number): string => new Date(now + offsetMs).toISOString();
  const organization = targetSeed({
    id: '2001',
    workspaceId: '3001',
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
  /* The widest a repository name is allowed to be. GitHub caps a name at 100
     characters, so this is not an unlikely value - it is the last one, and a
     column sized for the names that happen to be in a demo is a column that has
     never met the one that matters. `tests/browser/table-columns.test.ts` reads
     this row: with the name column laid out in bare `fr` this pushed every other
     column off the end of the row. */
  organization.repositories.push(
    repositorySeed(organization.value, {
      id: '4099',
      name: 'a'.repeat(46) + '-the-longest-name-github-will-accept-' + 'z'.repeat(18),
      enabledOverride: null,
      filePatch: {},
      panelPatch: {},
      bypass: false,
      updatedAt: iso(-9 * 86_400_000),
    }),
  );
  organization.audit = [
    {
      ...auditSeed(
        'audit-1',
        'ownership.synced',
        "refreshed the workspace's owner list",
        undefined,
        iso(-12 * 60_000),
      ),
    },
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
    ['repository.config_migration.reset', 'reset migrated repository configuration for'],
    ['target.access.updated', 'updated who may act in the workspace'],
    ['invitation.created', 'invited a workspace member'],
  ] as const;
  for (let index = 0; index < 34; index += 1) {
    const [action, summary] = cycled(auditActions, index);
    const repository = action.startsWith('repository.')
      ? cycled(organization.repositories, index).detail.repository.full_name
      : undefined;
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
    'Smyklot no longer has access to this repository',
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
    workspaceId: '3002',
    login: 'bart',
    displayName: 'Bart Smykla',
    type: 'User',
    repositoryDefaultEnabled: true,
    targetPatch: { disable_bare_commands: true },
    /* Deliberately bare: one rail tile keeps showing the generated mark. */
    avatar: false,
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
  const queue = queueSeeds(iso).filter((item) => item.id !== 'queue-sync-scheduled');
  const automaticSync = queue.find((item) => item.id === 'queue-sync-apply');
  if (automaticSync !== undefined) {
    delete automaticSync.repository_id;
    delete automaticSync.repository_name;
    delete automaticSync.started_at;
    automaticSync.title = 'Sync shared configuration';
    automaticSync.summary = '14 changes queued automatically';
    automaticSync.state = 'scheduled';
    automaticSync.progress_current = 0;
    automaticSync.progress_total = 14;
    automaticSync.attempt = 0;
    automaticSync.details = syncPlanSeed(iso).actions.reduce(
      (counts, action) => ({ ...counts, [action.operation]: counts[action.operation] + 1 }),
      { create: 0, update: 0, delete: 0 },
    );
  }
  const sync = new Map([
    [`${organization.value.id}/labels`, syncLabelsSeed(iso)],
    [`${organization.value.id}/settings`, syncSettingsSeed(iso)],
    [`${organization.value.id}/rulesets`, syncRulesetsSeed(iso)],
    [`${organization.value.id}/files`, syncFilesSeed(iso)],
  ]);
  return {
    signedIn: true,
    forceFailure: false,
    hideTargets: false,
    targets: [
      organization,
      personal,
      /* 32 team orgs + the organization + the personal target = 34 workspaces,
         which is what the approved overview demo's ownership legend adds up to:
         24 fresh (team-11..32 plus those two) + 8 stale + 1 approval + 1 error. */
      ...Array.from({ length: 32 }, (_, index) =>
        targetSeed({
          id: `mock-organization-${index + 1}`,
          workspaceId: String(3010 + index),
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
    queue,
    queueLoop: new Set(),
    // The sync fixture shares a stable pending plan. The generic queue animation
    // must not finish its row while Sync status still reports those same changes queued.
    queueRest: new Map(
      queue.filter((item) => item.kind !== 'sync_apply').map((item) => [item.id, item]),
    ),
    runtime: {
      backgroundWorkPaused: false,
      behaviorOverride: null,
      logLevelOverride: null,
      pollIntervalOverride: null,
      pendingCIQuietPeriodOverride: null,
      pathIndexIntervalOverride: null,
      sessionTTLOverride: null,
      revision: 0,
      checkpointCounter: 1,
      checkpoints: new Map(),
      audit: [],
      startedAt: now,
    },
    workspaceSettings: {
      checkpointCounter: 1,
      checkpoints: new Map(),
    },
    prefs,
    /* Configured and waiting, because empty was the only state this page could
       be looked at in: `mockSyncConfig` invents an empty document the first time
       it is asked, and no plan was ever computed, so the label list and the plan
       list rendered nowhere and drifted out of the design unseen. */
    sync,
    /* One repository that adjusts a template, because the pane that shows one
       has a card per adjustment and a form nobody can look at except empty is
       a form that drifts out of the design unseen. */
    syncOverrides: new Map([
      [
        '4001/files',
        {
          kind: 'files',
          enabled: null,
          document: {
            merges: [
              {
                path: 'renovate.json',
                strategy: 'deep-merge',
                overrides: {
                  timezone: 'Europe/Warsaw',
                  schedule: ['* 4 * * 6'],
                  ignorePaths: ['crates/harness-codex-acp/**'],
                },
                arrays: [{ path: '$.ignorePaths', strategy: 'append' }],
                deduplicate: true,
              },
              {
                path: 'CONTRIBUTING.md',
                strategy: 'markdown',
                sections: [
                  { action: 'after', heading: '## Commits', content: '- Squash on merge' },
                  {
                    action: 'patch',
                    heading: '### Making Changes',
                    patches: [{ find: 'make check', replace: 'mise run check' }],
                  },
                ],
              },
            ],
            formats: [
              {
                path: 'renovate.json',
                formatting: {
                  common: { line_ending: 'crlf' },
                  json: { arrays: 'compact' },
                },
              },
            ],
          },
          revision: 1,
          updated_by: 'bart',
          updated_at: iso(-2 * 60 * 60_000),
          unreadable: false,
        },
      ],
      /* The design's three renovate adjusters, on the three repositories the
         plan already changes - real ids, so the file page's adjuster list and
         the board name the same repositories. */
      [
        '4002/files',
        {
          kind: 'files',
          enabled: null,
          document: {
            merges: [
              {
                path: 'renovate.json',
                strategy: 'deep-merge',
                overrides: {
                  schedule: ['* 4 * * 1-5'],
                  timezone: 'Europe/Warsaw',
                  packageRules: [{ matchManagers: ['npm'], groupName: 'frontend packages' }],
                },
                arrays: [{ path: '$.packageRules', strategy: 'append' }],
              },
            ],
          },
          revision: 2,
          updated_by: 'bart',
          updated_at: iso(-3 * 60 * 60_000),
          unreadable: false,
        },
      ],
      [
        '4005/files',
        {
          kind: 'files',
          enabled: null,
          document: {
            merges: [
              {
                path: 'renovate.json',
                strategy: 'deep-merge',
                overrides: {
                  packageRules: [{ matchManagers: ['dockerfile'], groupName: 'images' }],
                },
                arrays: [{ path: '$.packageRules', strategy: 'append' }],
              },
            ],
          },
          revision: 1,
          updated_by: 'kasia',
          updated_at: iso(-26 * 60 * 60_000),
          unreadable: false,
        },
      ],
      [
        '4006/files',
        {
          kind: 'files',
          enabled: null,
          document: {
            merges: [
              {
                path: 'renovate.json',
                strategy: 'deep-merge',
                overrides: { automerge: null },
              },
            ],
          },
          revision: 1,
          updated_by: 'tomasz',
          updated_at: iso(-4 * 24 * 60 * 60_000),
          unreadable: false,
        },
      ],
      /* And one the planner refuses, because a repository receiving none of the
         organization's files reads here exactly like one receiving all of them
         unless the notice that says so is on a screen somebody looks at. */
      [
        '4003/files',
        {
          kind: 'files',
          enabled: null,
          document: {},
          revision: 0,
          unreadable: false,
          problem:
            'these files cannot be composed: docs/guide.md cannot be written ' +
            'because docs is not a directory in this repository',
          problem_at: iso(-4 * 60_000),
        },
      ],
    ]),
    syncPlans: new Map([
      [
        organization.value.id,
        {
          ...syncPlanSeed(iso),
          state: 'approved',
          execution_stage: 'Queued for automatic sync',
          approved_at: iso(-12 * 60_000),
        },
      ],
    ]),
    syncStatus: new Map([[organization.value.id, syncStatusSeed(iso)]]),
    // Replaced by install() with the running server's own page.
  };
}

/**
 * The fleet, over the repositories this workspace actually holds.
 *
 * IT HAS TO BE THE SAME REPOSITORIES. The sync side of this mock named a
 * different set of twenty-five - `af`, `afi`, `harness` and so on - and only
 * `smyklot` was in both, so anything joining a repository to its sync state was
 * invisible here for twenty-seven of twenty-eight. The repository page's own
 * sentence says "syncing - 4 changes in the open plan" and could never say it.
 *
 * The distribution the design draws is what matters and is kept whole: three
 * repositories carrying the plan's changes (platform-infra 2/2/0/2, api-gateway
 * 0/4/1/0, auth-service 2/1/0/0 - kind totals labels 4 / settings 7 /
 * rulesets 1 / files 2, fourteen in all), one refusal with its reason on the
 * row, and two repositories with kinds switched off.
 */
export function syncStatusSeed(iso: (offsetMs: number) => string): SyncStatus {
  type Mark = number | 'off' | 'ref';
  const fleet: [string, Mark, Mark, Mark, Mark][] = [
    ['smyklot', 0, 0, 0, 0],
    ['platform-infra', 2, 2, 0, 2],
    ['legacy-service', 0, 0, 0, 'ref'],
    ['migration-demo', 'off', 0, 0, 'off'],
    ['api-gateway', 0, 4, 1, 0],
    ['auth-service', 2, 1, 0, 0],
    ['billing-worker', 0, 0, 0, 0],
    ['cli-tools', 0, 0, 0, 0],
    ['customer-portal', 0, 0, 0, 0],
    ['data-pipeline', 0, 0, 0, 0],
    ['deployment-config', 'off', 'off', 'off', 'off'],
    ['design-system', 0, 0, 0, 0],
    ['docs-site', 0, 0, 0, 0],
    ['edge-proxy', 0, 0, 0, 0],
    ['event-consumer', 0, 0, 0, 0],
    ['feature-flags', 0, 0, 0, 0],
    ['identity-provider', 0, 0, 0, 0],
    ['internal-tools', 0, 0, 0, 0],
    ['mobile-api', 0, 0, 0, 0],
    ['notification-service', 0, 0, 0, 0],
    ['observability', 0, 0, 0, 0],
    ['payments-api', 0, 0, 0, 0],
    ['release-automation', 0, 0, 0, 0],
    ['runtime-images', 0, 0, 0, 0],
    ['search-indexer', 0, 0, 0, 0],
    ['security-policies', 0, 0, 0, 0],
    ['support-tools', 0, 0, 0, 0],
    ['web-frontend', 0, 0, 0, 0],
  ];
  const cell = (mark: Mark): SyncCell => {
    if (mark === 'off') return { state: 'off' };
    if (mark === 'ref') return { state: 'refused' };
    return mark > 0 ? { state: 'pending', changes: mark } : { state: 'in_step' };
  };
  return {
    checked_at: iso(-5 * 60_000),
    repositories: fleet.map(([repository, labels, settings, rulesets, files]) => ({
      repository,
      cells: {
        labels: cell(labels),
        settings: cell(settings),
        rulesets: cell(rulesets),
        files: cell(files),
      },
      ...(repository === 'platform-infra' ? { removals: 1 } : {}),
      ...(repository === 'legacy-service'
        ? {
            reason:
              'docs/guide.md cannot be written because docs is not a directory in this repository',
          }
        : {}),
    })),
  };
}

/** A label set an organization has configured, switched on and being enforced. */
export function syncLabelsSeed(iso: (offsetMs: number) => string): SyncConfig {
  return {
    kind: 'labels',
    enabled: true,
    /* The design's own five, word for word - the labels page is compared
       against the mock screen by screen. */
    labels: [
      { name: 'bug', color: 'd73a4a', description: 'Something is broken' },
      { name: 'enhancement', color: 'a2eeef', description: 'New behaviour somebody asked for' },
      {
        name: 'dependencies',
        color: '0e8a16',
        description: "Dependency updates, mostly Renovate's",
      },
      {
        name: 'good first issue',
        color: '7057ff',
        description: 'Small, self-contained, documented',
      },
      // No description at all, which is not the same as an empty one: the row
      // has to read without the second column.
      { name: 'chore', color: '6b7280' },
    ],
    /* Off, like the design's overview says: removal is the sharp end of label
       sync and the demo keeps it sheathed. */
    allow_removal: false,
    excludes: ['hand-made-*'],
    revision: 3,
    updated_by: 'bart',
    updated_at: iso(-2 * 60 * 60_000),
    digest: 'sha256:labels',
    document: {},
    unreadable: false,
    unavailable: '',
  };
}

/**
 * The settings an organization manages, seeded flat the way the form stores
 * them - GitHub's own keys at the top level. Nine of the catalogue's
 * seventeen, which is the overview's "9 of 17 managed".
 */
export function syncSettingsSeed(iso: (offsetMs: number) => string): SyncConfig {
  return {
    kind: 'settings',
    enabled: true,
    labels: [],
    allow_removal: false,
    excludes: [],
    revision: 5,
    updated_by: 'bart',
    updated_at: iso(-26 * 60 * 60_000),
    digest: 'sha256:settings',
    /* The design page's nine, group for group: Merging 4 of 6, wording 2 of
       4, features 2 of 4, security 1 of 3. */
    document: {
      allow_squash_merge: true,
      allow_merge_commit: false,
      allow_auto_merge: true,
      delete_branch_on_merge: true,
      squash_merge_commit_title: 'PR_TITLE',
      squash_merge_commit_message: 'COMMIT_MESSAGES',
      has_issues: true,
      has_wiki: false,
      secret_scanning: true,
    },
    unreadable: false,
    unavailable: '',
  };
}

/**
 * The ruleset an organization actually runs, seeded for the same reason the
 * labels above are: a form nobody can look at except empty is a form that
 * drifts out of the design unseen, and this one has nested rows that only
 * appear once a rule carrying parameters is switched on.
 */
export function syncRulesetsSeed(iso: (offsetMs: number) => string): SyncConfig {
  return {
    kind: 'rulesets',
    enabled: true,
    labels: [],
    allow_removal: false,
    excludes: [],
    revision: 2,
    updated_by: 'bart',
    updated_at: iso(-3 * 24 * 60 * 60_000),
    digest: 'sha256:rulesets',
    document: {
      rulesets: [
        /* The design's list row: "main-protection · Active · default branch ·
           6 rules · 2 bypass actors". */
        {
          name: 'main-protection',
          target: 'branch',
          enforcement: 'active',
          conditions: { include: ['~DEFAULT_BRANCH'], exclude: [] },
          bypass_actors: [
            { actor_id: 5, actor_type: 'RepositoryRole', bypass_mode: 'always' },
            { actor_id: 1216238, actor_type: 'Integration', bypass_mode: 'pull_request' },
          ],
          rules: {
            deletion: true,
            non_fast_forward: true,
            required_linear_history: true,
            required_signatures: true,
            pull_request: {
              required_approving_review_count: 1,
              dismiss_stale_reviews_on_push: true,
              allowed_merge_methods: ['squash'],
            },
            required_status_checks: {
              required_status_checks: [{ context: 'test' }, { context: 'lint' }],
              strict_required_status_checks_policy: true,
            },
          },
        },
        /* A second ruleset still evaluating, so the overview can say
           "2 rulesets · 1 evaluating" the way the design does. */
        {
          name: 'release-tags',
          target: 'tag',
          enforcement: 'evaluate',
          conditions: { include: ['refs/tags/v*'], exclude: [] },
          rules: {
            deletion: true,
            non_fast_forward: true,
          },
        },
      ],
      allow_removal: false,
      excludes: ['hand-made-*'],
    },
    unreadable: false,
    unavailable: '',
  };
}

/**
 * The files an organization keeps in step, seeded for the reason the two above
 * are: a form nobody can look at except empty drifts out of the design unseen,
 * and this one draws a card per file whose height is the template's.
 */
export function syncFilesSeed(iso: (offsetMs: number) => string): SyncConfig {
  return {
    kind: 'files',
    enabled: true,
    labels: [],
    allow_removal: false,
    excludes: [],
    revision: 4,
    updated_by: 'bart',
    updated_at: iso(-20 * 60_000),
    digest: 'sha256:files',
    /* The design's five templates, content for content - the file pages are
       compared against the mock screen by screen. Freshness belongs to the
       strict configuration envelope, not inside the file document. */
    document: {
      files: [
        {
          path: 'renovate.json',
          formatting: { common: { final_newline: 'insert' } },
          content: [
            '{',
            '  "$schema": "https://docs.renovatebot.com/renovate-schema.json",',
            '  "extends": ["config:recommended"],',
            '  "schedule": ["* 4 * * 6"],',
            '  "timezone": "UTC",',
            '  "packageRules": [',
            '    { "matchManagers": ["gomod"], "groupName": "go modules" }',
            '  ],',
            '  "automerge": false',
            '}',
          ].join('\n'),
        },
        {
          path: '.github/workflows/ci.yaml',
          content: [
            'name: test',
            'on:',
            '  push:',
            '    branches: [main]',
            'jobs:',
            '  test:',
            '    runs-on: ubuntu-latest',
            '    steps:',
            '      # Pinned by digest, never by tag',
            '      - uses: actions/checkout@8edcb1b',
            '      - run: mise run ci',
          ].join('\n'),
        },
        {
          path: 'CONTRIBUTING.md',
          content: [
            '# Contributing',
            '',
            'Open a pull request against `main`.',
            '',
            '## Commits',
            '',
            '- Conventional commits: `feat:`, `fix:`, `docs:`',
            '- Sign-off and GPG sign: `-sS`',
          ].join('\n'),
        },
        {
          path: '.github/CODEOWNERS',
          content: '* @smykla-skalski/maintainers\n',
        },
        {
          path: 'LICENSE',
          content: 'Apache License 2.0\n',
        },
      ],
      retired: ['.github/stale.yml'],
      excludes: ['LICENSE-*'],
    },
    unreadable: false,
    unavailable: '',
  };
}

/**
 * A plan waiting for somebody, at the design's scale: fourteen changes -
 * eight additions, five changes, one removal - across the three drifted
 * repositories, agreeing row for row with what the board's cells count.
 */
export function syncPlanSeed(iso: (offsetMs: number) => string): SyncPlan {
  /* THE SHAPES THE SERVICE SENDS, which is not what this used to hold.
     A label arrived here pre-stripped of its name, and settings arrived one
     tidy action per field - neither of which the planner can produce. So the
     plan page looked right on a laptop and wrong in production, where a label
     row printed its name twice and every changed setting in a repository was
     one sentence. Every action below carries the `detail` the service now
     sends, and a settings change is one action with a line per field.

     The counts are the drawing's: eight additions, five changes, one removal,
     split +3 ~2 -1, +3 ~2, +2 ~1 across the three repositories. */
  const label = (name: string, color: string, description?: string) => ({
    name,
    color,
    ...(description === undefined ? {} : { description }),
  });
  /* Excerpt-sized on purpose: what a plan carries is the window worth
     reading, and the diff the page draws from this pair is the design's own
     five lines. */
  const renovateBefore = [
    '"extends": ["config:recommended"],',
    '"schedule": ["* 4 * * 0"],',
    '"packageRules": [',
  ].join('\n');
  const renovateAfter = [
    '"extends": ["config:recommended"],',
    '"schedule": ["* 4 * * 1-5"],',
    '"timezone": "Europe/Warsaw",',
    '"packageRules": [',
  ].join('\n');
  return {
    id: 'plan-1',
    trigger: 'reconcile',
    state: 'computed',
    execution_stage: 'Waiting for approval',
    digest: 'sha256:plan',
    counts: { create: 8, update: 5, delete: 1 },
    actions: [
      {
        repository: 'platform-infra',
        kind: 'labels',
        operation: 'create',
        subject: 'dependencies',
        detail: { label: label('dependencies', '0e8a16', "Dependency updates, mostly Renovate's") },
        state: 'pending',
      },
      {
        repository: 'platform-infra',
        kind: 'labels',
        operation: 'create',
        subject: 'good first issue',
        detail: { label: label('good first issue', '7057ff') },
        state: 'pending',
      },
      {
        repository: 'platform-infra',
        kind: 'files',
        operation: 'create',
        subject: 'renovate.json',
        before: renovateBefore,
        after: renovateAfter,
        detail: { file: { path: 'renovate.json', proposal: 'smyklot/sync', bytes: 412 } },
        state: 'pending',
      },
      {
        repository: 'platform-infra',
        kind: 'settings',
        operation: 'update',
        subject: 'repository',
        detail: {
          settings: [
            { field: 'allow_squash_merge', from: 'off', to: 'on' },
            { field: 'has_wiki', from: 'on', to: 'off' },
          ],
        },
        state: 'pending',
      },
      {
        repository: 'platform-infra',
        kind: 'labels',
        operation: 'update',
        /* A change, so both sides: the colour has drifted and the description
           has not, which is the row that has to show what moved. */
        subject: 'bug',
        detail: {
          label: label('bug', 'd73a4a', 'Something is broken'),
          previous_label: label('bug', 'ff8800', 'Something is broken'),
        },
        state: 'pending',
      },
      {
        repository: 'platform-infra',
        kind: 'files',
        operation: 'delete',
        subject: '.github/stale.yml',
        detail: { file: { path: '.github/stale.yml', proposal: 'smyklot/sync', bytes: 0 } },
        state: 'pending',
      },
      {
        repository: 'api-gateway',
        kind: 'labels',
        operation: 'create',
        subject: 'enhancement',
        detail: { label: label('enhancement', 'a2eeef', 'New behaviour somebody asked for') },
        state: 'pending',
      },
      {
        repository: 'api-gateway',
        kind: 'labels',
        operation: 'create',
        /* No description, which is a label the configuration allows and a shape
           the row has to draw: the badge alone, with nothing after it. */
        subject: 'chore',
        detail: { label: label('chore', '6b7280') },
        state: 'pending',
      },
      {
        repository: 'api-gateway',
        kind: 'rulesets',
        operation: 'create',
        subject: 'main-protection',
        detail: {
          ruleset: {
            name: 'main-protection',
            target: 'branch',
            enforcement: 'active',
            rules: ['no deletion', 'no force pushes', 'linear history'],
            bypass: 1,
          },
        },
        state: 'pending',
      },
      {
        /* Three fields, one that GitHub switches off alongside them, and one
           this repository will not be given. The whole reason a settings action
           needs more than a sentence. */
        repository: 'api-gateway',
        kind: 'settings',
        operation: 'update',
        subject: 'repository',
        detail: {
          settings: [
            { field: 'allow_auto_merge', from: 'off', to: 'on' },
            { field: 'delete_branch_on_merge', from: 'off', to: 'on' },
            { field: 'has_projects', from: 'on', to: 'off' },
          ],
          follows: ['allow_merge_commit'],
          withheld: [
            { field: 'advanced_security', reason: 'not available on this repository plan' },
          ],
        },
        state: 'pending',
      },
      {
        repository: 'api-gateway',
        kind: 'rulesets',
        operation: 'update',
        subject: 'release-protection',
        detail: {
          ruleset: {
            name: 'release-protection',
            target: 'tag',
            enforcement: 'evaluate',
            rules: ['no deletion'],
            bypass: 0,
          },
        },
        state: 'pending',
      },
      {
        repository: 'auth-service',
        kind: 'labels',
        operation: 'create',
        subject: 'dependencies',
        detail: { label: label('dependencies', '0e8a16', "Dependency updates, mostly Renovate's") },
        state: 'pending',
      },
      {
        repository: 'auth-service',
        kind: 'labels',
        operation: 'create',
        subject: 'good first issue',
        detail: { label: label('good first issue', '7057ff') },
        state: 'pending',
      },
      {
        repository: 'auth-service',
        kind: 'settings',
        operation: 'update',
        subject: 'repository',
        detail: { settings: [{ field: 'has_projects', from: 'on', to: 'off' }] },
        state: 'pending',
      },
    ],
    computed_at: iso(-12 * 60_000),
    expires_at: iso(6 * 60 * 60_000 + 5 * 60_000),
  };
}

export function invitationSeeds(
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
  const roles: Array<Exclude<WorkspaceRole, 'none'>> = ['viewer', 'editor', 'admin'];
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
export function securityNotificationSeeds(
  iso: (offsetMs: number) => string,
  workspace: PanelAccount,
  actor: PanelAccount,
): SecurityNotification[] {
  return [
    {
      id: 'security-3',
      workspace,
      actor,
      elevation_id: 'R7mQ2xKfLp0Zc4Vn8sTdWb1yHgJ3aEuN6iOqXr5vBkM',
      audit_event_id: '203',
      action: 'installation.settings.restored',
      reason: 'Restore command handling during production incident',
      created_at: iso(-18 * 60_000),
    },
    {
      id: 'security-2',
      workspace,
      actor,
      elevation_id: 'R7mQ2xKfLp0Zc4Vn8sTdWb1yHgJ3aEuN6iOqXr5vBkM',
      audit_event_id: '202',
      action: 'installation.settings.saved',
      reason: 'Restore command handling during production incident',
      created_at: iso(-24 * 60_000),
    },
    {
      id: 'security-1',
      workspace,
      actor,
      elevation_id: 'hT4wYs9dRfB2nKmXpQ7vLc0jZgA5eU8iRoW1yNbD3xE',
      audit_event_id: '188',
      action: 'installation.settings.restored',
      reason: 'Owner-approved support investigation',
      created_at: iso(-2 * 86_400_000),
      read_at: iso(-47 * 3_600_000),
    },
  ];
}

/** What a role may do. The one answer, so a scoped user and a seeded one agree. */
export function capabilitiesFor(role: WorkspaceRole) {
  return {
    read: role !== 'none',
    write: role === 'owner' || role === 'admin' || role === 'editor',
    manage_target_users: role === 'owner' || role === 'admin',
  };
}

export function userSeeds(iso: (offsetMs: number) => string): PanelUser[] {
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
    role: WorkspaceRole,
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
  const roles: WorkspaceRole[] = ['viewer', 'editor', 'admin', 'none'];
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

export function targetSeed(input: {
  id: string;
  workspaceId: string;
  login: string;
  displayName: string;
  type: 'Organization' | 'User';
  repositoryDefaultEnabled: boolean;
  targetPatch: ConfigPatch;
  /** Off for a workspace that should exercise the generated-mark fallback. */
  avatar?: boolean;
}): MockTarget {
  const account: PanelAccount = {
    id: input.id,
    provider: 'github:https://api.github.com',
    subject_id: input.id,
    login: input.login,
    display_name: input.displayName,
    avatar_url: input.avatar === false ? null : devAvatarUrl(input.login),
  };
  const resolved = resolveConfig(input.targetPatch, {}, {}, false);
  return {
    value: {
      id: input.id,
      installation_id: input.workspaceId,
      type: input.type,
      account,
      repository_default_enabled: input.repositoryDefaultEnabled,
      pending_ci_mode_default: 'checks',
      pending_ci_branch_patterns_default: { include: ['~DEFAULT_BRANCH'], exclude: [] },
      pending_ci_quiet_period_seconds_override: null,
      pending_ci_quiet_period_seconds_inherited: DEV_PENDING_CI_QUIET_SECONDS,
      path_index_interval_seconds_override: null,
      path_index_interval_seconds_inherited: DEV_PATH_INDEX_SECONDS,
      pending_ci_permissions: {
        checks_write: true,
        administration_write: true,
        merge_queues_read: true,
        commit_statuses_read: true,
      },
      config_patch: input.targetPatch,
      inherited_config: structuredClone(DEFAULT_CONFIG),
      effective_config: resolved.values,
      config_sources: resolved.sources,
      formatting_sources: resolved.formattingSources,
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

export function repositorySeed(
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
    pending_ci_mode: target.pending_ci_mode_default,
    pending_ci_mode_source: 'target',
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
      formatting_sources: resolved.formattingSources,
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
      pending_ci_mode_override: null,
      pending_ci_mode_inherited: target.pending_ci_mode_default,
      pending_ci_branch_patterns_override: null,
      pending_ci_branch_patterns_inherited: target.pending_ci_branch_patterns_default,
      pending_ci_quiet_period_seconds_override: null,
      pending_ci_quiet_period_seconds_inherited:
        target.pending_ci_quiet_period_seconds_override ?? DEV_PENDING_CI_QUIET_SECONDS,
      path_index_interval_seconds_override: null,
      path_index_interval_seconds_inherited:
        target.path_index_interval_seconds_override ?? DEV_PATH_INDEX_SECONDS,
      pending_ci_gate: {
        desired_mode: target.pending_ci_mode_default,
        effective_mode: target.pending_ci_mode_default,
        readiness: 'ready',
        reason: 'Ready in the development fixture',
      },
      revision: 1,
    },
  };
}

export function auditSeed(
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
): {
  values: ConfigValues;
  sources: ConfigSources;
  formattingSources: PanelTarget['formatting_sources'];
} {
  const values = structuredClone(DEFAULT_CONFIG);
  const sources = Object.fromEntries(CONFIG_KEYS.map((key) => [key, 'process'])) as ConfigSources;
  let resolvedFormattingSources = formattingSources<ConfigSources[ConfigKey]>('process');
  resolvedFormattingSources = applyPatch(
    values,
    sources,
    resolvedFormattingSources,
    targetPatch,
    'target',
  );
  if (!bypass) {
    resolvedFormattingSources = applyPatch(
      values,
      sources,
      resolvedFormattingSources,
      filePatch,
      'repository_file',
    );
  }
  resolvedFormattingSources = applyPatch(
    values,
    sources,
    resolvedFormattingSources,
    panelPatch,
    'repository_panel',
  );
  return { values, sources, formattingSources: resolvedFormattingSources };
}

function applyPatch(
  values: ConfigValues,
  sources: ConfigSources,
  currentFormattingSources: PanelTarget['formatting_sources'],
  patch: ConfigPatch,
  source: ConfigSources[ConfigKey],
): PanelTarget['formatting_sources'] {
  for (const key of CONFIG_KEYS) {
    const value = patch[key];
    if (value === undefined) continue;
    Object.assign(values, { [key]: structuredClone(value) });
    sources[key] = source;
  }
  if (patch.formatting === undefined) return currentFormattingSources;
  values.formatting = applyFormattingPatch(values.formatting, patch.formatting);
  return applyFormattingSources(currentFormattingSources, patch.formatting, source);
}

function recomputeTarget(target: MockTarget): void {
  const targetResolved = resolveConfig(target.value.config_patch, {}, {}, false);
  target.value.inherited_config = structuredClone(DEFAULT_CONFIG);
  target.value.effective_config = targetResolved.values;
  target.value.config_sources = targetResolved.sources;
  target.value.formatting_sources = targetResolved.formattingSources;
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
  detail.formatting_sources = resolved.formattingSources;
  detail.repository.effective_enabled =
    detail.repository.enabled_override ?? target.value.repository_default_enabled;
  detail.repository.enabled_source =
    detail.repository.enabled_override === null ? 'target' : 'repository';
  detail.repository.config_override_count = Object.keys(detail.config_patch).length;
  if (detail.ignore_repository_file) detail.repository.config_file_status = 'bypassed';
}

export function queueSeeds(iso: (offsetMs: number) => string): QueueItem[] {
  const common = {
    target_id: '2001',
    profile_id: 'always-open',
    profile_name: 'Always Open',
    profile_timezone: 'UTC',
    window_mode: 'respect' as const,
    immediate: false,
    work_ahead: 0,
    priority_overridden: false,
    progress_current: 0,
    progress_total: 0,
    attempt: 0,
    revision: 1,
  };
  return [
    {
      ...common,
      id: 'queue-sync-apply',
      kind: 'sync_apply',
      lane: 'maintenance',
      repository_id: '4001',
      repository_name: 'smykla-skalski/smyklot',
      title: 'Apply organization sync plan',
      summary: 'smykla-skalski · 12 changes',
      state: 'running',
      priority: 'high',
      not_before: iso(-8 * 60_000),
      eligible_at: iso(-8 * 60_000),
      estimated_start_at: iso(-8 * 60_000),
      progress_current: 4,
      progress_total: 12,
      details: { create: 3, update: 7, delete: 2 },
      attempt: 1,
      created_at: iso(-18 * 60_000),
      updated_at: iso(-2 * 60_000),
      started_at: iso(-8 * 60_000),
      actions: ['set_priority'],
    },
    {
      ...common,
      id: 'queue-pending-ci',
      kind: 'pending_ci',
      lane: 'pending_ci',
      repository_id: '4002',
      repository_name: 'smykla-skalski/platform-infra',
      title: 'Merge after CI',
      summary: 'Waiting for required checks',
      state: 'blocked',
      priority: 'normal',
      not_before: iso(-25 * 60_000),
      eligible_at: iso(5 * 60_000),
      estimated_start_at: iso(6 * 60_000),
      work_ahead: 1,
      blocked_reason: 'Waiting on required checks',
      details: {
        pull_request: 184,
        pull_request_title: 'Update rate limits for the edge tier',
        head_sha: '2bb2221374c1a9ee4f8b0d3c6a5e9017cc41ab8e',
      },
      created_at: iso(-30 * 60_000),
      updated_at: iso(-3 * 60_000),
      actions: ['run_now', 'next_window', 'schedule_at', 'set_priority', 'cancel'],
    },
    {
      ...common,
      id: 'queue-sync-scheduled',
      kind: 'sync_apply',
      lane: 'maintenance',
      title: 'Sync shared configuration',
      summary: '14 changes queued automatically',
      state: 'scheduled',
      priority: 'normal',
      not_before: iso(-12 * 60_000),
      eligible_at: iso(-12 * 60_000),
      created_at: iso(-12 * 60_000),
      updated_at: iso(-12 * 60_000),
      actions: [],
    },
    {
      ...common,
      id: 'queue-reaction-retry',
      kind: 'reaction_scan',
      lane: 'maintenance',
      repository_id: '4003',
      repository_name: 'smykla-skalski/legacy-service',
      title: 'Scan for new commands',
      summary: 'smykla-skalski/agent-platform',
      state: 'retrying',
      priority: 'low',
      not_before: iso(-10 * 60_000),
      eligible_at: iso(2 * 60_000),
      estimated_start_at: iso(4 * 60_000),
      work_ahead: 2,
      blocked_reason: 'GitHub rate limit; retry scheduled',
      attempt: 2,
      created_at: iso(-40 * 60_000),
      updated_at: iso(-1 * 60_000),
      actions: ['run_now', 'next_window', 'schedule_at', 'set_priority', 'cancel'],
    },
    {
      ...common,
      id: 'queue-catalog-complete',
      kind: 'catalog_refresh',
      lane: 'maintenance',
      title: 'Refresh the list of repositories',
      state: 'succeeded',
      priority: 'normal',
      not_before: iso(-90 * 60_000),
      eligible_at: iso(-90 * 60_000),
      created_at: iso(-90 * 60_000),
      updated_at: iso(-88 * 60_000),
      finished_at: iso(-88 * 60_000),
      actions: [],
    },
  ];
}

export function pendingCISeeds(iso: (offsetMs: number) => string): PendingCIRequest[] {
  return [
    /* Passing and inside its quiet period, which is the one row whose next event
       is the merge itself rather than another look at the checks. Without one of
       these seeded, the countdown and its ring - the whole point of the Next
       column - never appear in development. */
    {
      id: 'pending-ci-0',
      repository_full_name: 'smykla-skalski/panel',
      pull_request: 204,
      head_sha: '2bb2221374c1a9ee4f8b0d3c6a5e9017cc41ab8e',
      merge_method: 'squash',
      required_checks_only: false,
      requester: 'lin',
      lifecycle: 'armed',
      schedule: 'active',
      next_check_at: iso(24_000),
      next_check_trigger: 'quiet_period',
      last_observed_state: 'passing',
      reason: '',
      requested_at: iso(-6 * 60_000),
      updated_at: iso(-6_000),
      cleanup_pending: false,
      revision: 2,
    },
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
    /* The three states the first three do not cover, so that every value the
       Checks column can draw is on the screen at once. Without them the column
       was measured, and looked at, against three of its six. */
    {
      id: 'pending-ci-6',
      repository_full_name: 'smykla-skalski/docs',
      pull_request: 311,
      head_sha: 'd41d8cd98f00b204e9800998ecf8427e6a1b3f5c',
      merge_method: 'merge',
      required_checks_only: false,
      requester: 'operator',
      lifecycle: 'armed',
      schedule: 'active',
      next_check_at: iso(9 * 60_000),
      next_check_trigger: 'fallback',
      last_observed_state: 'indeterminate',
      reason: '',
      requested_at: iso(-42 * 60_000),
      updated_at: iso(-9 * 60_000),
      cleanup_pending: false,
      revision: 2,
    },
    {
      id: 'pending-ci-7',
      repository_full_name: 'smykla-skalski/charts',
      pull_request: 18,
      head_sha: '7c9f0a1e5b3d68427ac0f19e34d5b8027fa6cd11',
      merge_method: 'squash',
      required_checks_only: false,
      requester: 'lin',
      lifecycle: 'armed',
      schedule: 'active',
      next_check_at: iso(2 * 60_000),
      next_check_trigger: 'fallback',
      last_observed_state: 'no_checks',
      reason: '',
      requested_at: iso(-3 * 60_000),
      updated_at: iso(-3 * 60_000),
      cleanup_pending: false,
      revision: 1,
    },
    /* Armed a moment ago and never yet reconciled, which is what an empty
       `last_observed_state` means - the column reads it as "Scheduled". The
       service writes no state at arm time (`last_observed_state` is
       `NOT NULL DEFAULT ''` and `sqlstore.Arm` leaves it), so every request
       passes through this and the mock has to be able to show it. */
    {
      id: 'pending-ci-8',
      repository_full_name: 'smykla-skalski/actions',
      pull_request: 7,
      head_sha: 'b52e7d3016fa94c8e0d271b6a4f8c3915de027ab',
      merge_method: 'rebase',
      required_checks_only: false,
      requester: 'bart',
      lifecycle: 'armed',
      schedule: 'active',
      next_check_at: iso(30_000),
      next_check_trigger: 'command',
      last_observed_state: '',
      reason: '',
      requested_at: iso(-20_000),
      updated_at: iso(-20_000),
      cleanup_pending: false,
      revision: 1,
    },
    /* Three that have finished, so the queue's Done card has its own rows: one of
       each way a request can end, and one with cleanup still outstanding so the
       line that reports it has something to report. */
    {
      id: 'pending-ci-3',
      repository_full_name: 'smykla-skalski/smyklot',
      pull_request: 196,
      head_sha: '2bb22213f0a94c7e1d8b6e5f3a20c7419de88b03',
      merge_method: 'squash',
      required_checks_only: false,
      requester: 'bart',
      lifecycle: 'merged',
      schedule: 'active',
      next_check_at: iso(-2 * 3_600_000),
      next_check_trigger: 'cleanup',
      last_observed_state: 'passing',
      reason: 'Checks passed and stayed quiet for 30 s',
      requested_at: iso(-3 * 3_600_000),
      updated_at: iso(-2 * 3_600_000),
      finished_at: iso(-2 * 3_600_000),
      cleanup_pending: false,
      revision: 5,
    },
    {
      id: 'pending-ci-4',
      repository_full_name: 'smykla-skalski/infra',
      pull_request: 70,
      head_sha: '91ee4c0287d3a5b1f6c0e94a72d5183be6f0c7a9',
      merge_method: 'rebase',
      required_checks_only: false,
      requester: 'operator',
      lifecycle: 'cancelled',
      schedule: 'active',
      next_check_at: iso(-4 * 3_600_000),
      next_check_trigger: 'manual',
      last_observed_state: 'pending',
      reason: 'Head commit changed after the command',
      requested_at: iso(-5 * 3_600_000),
      updated_at: iso(-4 * 3_600_000),
      finished_at: iso(-4 * 3_600_000),
      cleanup_pending: true,
      revision: 4,
    },
    {
      id: 'pending-ci-5',
      repository_full_name: 'smykla-skalski/panel',
      pull_request: 41,
      head_sha: 'a1c9e004b7f2153ce8a09d4b6172fe3d05c8a71b',
      merge_method: 'squash',
      required_checks_only: true,
      requester: 'lin',
      lifecycle: 'superseded',
      schedule: 'active',
      next_check_at: iso(-6 * 3_600_000),
      next_check_trigger: 'command',
      last_observed_state: 'passing',
      reason: 'Replaced by a later /merge after ci',
      requested_at: iso(-7 * 3_600_000),
      updated_at: iso(-6 * 3_600_000),
      finished_at: iso(-6 * 3_600_000),
      cleanup_pending: false,
      cleanup_error: 'the head branch was already gone',
      revision: 9,
    },
  ];
}

/** One item from a list, by an index that may run past its end. */
export function cycled<T>(items: readonly T[], index: number): T {
  const item = items[index % items.length];
  if (item === undefined) throw new Error('cannot cycle through an empty collection');

  return item;
}

/** What one account may do in one workspace, and where that answer came from. */
export function targetAccess(
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

/**
 * What a workspace has configured for one kind of sync, inventing an empty document
 * the first time it is asked so a page has a shape to render before anything is set.
 *
 * `new Date()` and not the seeded `now`: this answers a request rather than seeding a
 * fixture, so the timestamp it stamps is the moment it was asked.
 */
export function mockSyncConfig(state: MockState, key: string, kind: string): SyncConfig {
  const existing = state.sync.get(key);
  if (existing) {
    return existing;
  }

  const fresh: SyncConfig = {
    kind,
    enabled: false,
    labels: [],
    allow_removal: false,
    excludes: [],
    revision: 0,
    updated_by: '',
    updated_at: new Date().toISOString(),
    digest: '',
    document: {},
    unreadable: false,
    unavailable: '',
  };
  state.sync.set(key, fresh);

  return fresh;
}

/** The two workspaces the mock's Root actually owns. */
export function mockRootOwns(target: MockTarget): boolean {
  return target.value.id === '2001' || target.value.id === '1001';
}

/**
 * The Root console's view of an account: the same person as a `PanelUser`, plus the two
 * counts only the console asks for. Derived rather than seeded, which is why it is a
 * function here and not a constant - and why a story calls it instead of describing its
 * result, so the console's shape stays one thing.
 */
export function rootPanelUsers(state: MockState): RootPanelUser[] {
  return state.users.map((user) => ({
    account: user.account,
    system_role: user.system_role,
    status: user.status,
    ...(user.ban_reason === undefined ? {} : { ban_reason: user.ban_reason }),
    ...(user.banned_at === undefined ? {} : { banned_at: user.banned_at }),
    ...(user.last_login_at === undefined ? {} : { last_login_at: user.last_login_at }),
    revision: user.revision,
    owned_workspaces:
      user.account.id === VIEWER.id
        ? state.targets.filter((target) => mockRootOwns(target)).length
        : 0,
    assigned_workspaces: state.targets.filter((target) =>
      state.targetAccess.get(target.value.id)?.has(user.account.id),
    ).length,
    manageable: user.account.id !== VIEWER.id && user.system_role === 'none',
    can_manage_system_role: user.account.id !== VIEWER.id && user.system_role !== 'super_root',
  }));
}

/**
 * The path index the finder matches over: every path any repository in the
 * workspace holds, deduped, with how many hold it.
 */
export const KNOWN_PATHS: Array<{ path: string; repositories: number }> = [
  { path: '.github/workflows/ci.yaml', repositories: 25 },
  { path: '.github/workflows/release.yaml', repositories: 18 },
  { path: '.github/workflows/pages.yaml', repositories: 3 },
  { path: '.github/workflows/pr-commands.yaml', repositories: 6 },
  { path: '.github/CODEOWNERS', repositories: 25 },
  { path: '.github/dependabot.yml', repositories: 9 },
  { path: '.github/stale.yml', repositories: 2 },
  { path: '.github/renovate.json5', repositories: 4 },
  { path: 'renovate.json', repositories: 21 },
  { path: 'CONTRIBUTING.md', repositories: 24 },
  { path: 'README.md', repositories: 25 },
  { path: 'LICENSE', repositories: 25 },
  { path: 'Makefile', repositories: 14 },
  { path: 'mise.toml', repositories: 11 },
  { path: '.golangci.yml', repositories: 8 },
  { path: 'Dockerfile', repositories: 12 },
  { path: 'docs/getting-started.md', repositories: 7 },
  { path: 'cmd/main.go', repositories: 9 },
  { path: 'pkg/config/config.go', repositories: 6 },
  { path: 'internal/server/server.go', repositories: 4 },
  { path: 'charts/app/values.yaml', repositories: 5 },
  { path: 'charts/app/Chart.yaml', repositories: 5 },
  { path: 'scripts/release.sh', repositories: 8 },
  { path: 'scripts/test.sh', repositories: 10 },
  { path: '.editorconfig', repositories: 17 },
  { path: '.gitignore', repositories: 25 },
  { path: 'SECURITY.md', repositories: 13 },
  { path: 'CODE_OF_CONDUCT.md', repositories: 12 },
];

/**
 * What the development deployment resolves for the durations that cascade.
 *
 * Named here rather than typed at each of the six places that need them: the
 * mock has to agree with itself across the runtime settings, a workspace
 * and a repository, or the panel prefills one number and saves against another.
 */
export const DEV_PATH_INDEX_SECONDS = 3_600;
export const DEV_PENDING_CI_QUIET_SECONDS = 30;

/** The ceiling the service enforces - `panel.MaxPathIndexInterval`, in seconds. */
export const DEV_MAX_PATH_INDEX_SECONDS = 604_800;

/**
 * What one repository holds, for the path finder to offer.
 *
 * Derived from the name rather than listed per repository, because the finder
 * is worth looking at when the same path is in most of them and a few are not -
 * which is the shape it has to rank, and the shape a hand-written fixture never
 * quite has.
 */
export function mockRepositoryPaths(name: string): string[] {
  const everywhere = [
    'README.md',
    'LICENSE',
    '.github/CODEOWNERS',
    '.github/workflows/test.yaml',
    '.gitignore',
  ];
  const some = [
    'renovate.json',
    'CONTRIBUTING.md',
    '.github/workflows/release.yaml',
    'docs/guide.md',
    'internal/storage/sqlstore/store.go',
    'Makefile',
  ];

  // A stable spread: the same repository always holds the same paths, so a
  // reader comparing two visits is comparing the same list.
  const seed = [...name].reduce((total, letter) => total + letter.charCodeAt(0), 0);

  return [...everywhere, ...some.filter((_, index) => (seed + index) % 3 !== 0)];
}

/**
 * When this repository's tree was last read, as an offset in days.
 *
 * Not all the same, and one of them old: the panel's answer takes its STALEST
 * row, so a fixture where every reading is fresh makes the notice above the
 * finder unreachable in development - which is where a developer would
 * otherwise see it.
 */
export function mockRepositoryScanAge(name: string): number {
  const seed = [...name].reduce((total, letter) => total + letter.charCodeAt(0), 0);

  return seed % 7 === 0 ? 9 : seed % 3;
}
