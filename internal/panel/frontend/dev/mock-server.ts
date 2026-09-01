import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import type { Server as HttpServer, IncomingMessage, ServerResponse } from 'node:http';
import { dirname, resolve } from 'node:path';
import type { Duplex } from 'node:stream';
import { fileURLToPath } from 'node:url';
import type { Connect, Plugin } from 'vite';

import { mockEnabled as enabled } from './mock-html.ts';
import { syncPathIndex } from './path-index.ts';

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
  Page,
  PanelAccount,
  PanelInvitation,
  PanelTarget,
  PanelUser,
  PendingCIDetail,
  PendingCIRequest,
  QueueActionInput,
  QueueDetail,
  QueueItem,
  QueuePage,
  QueueSchedulePreview,
  QueuePolicy,
  QueuePolicyInput,
  QueuePolicyStatus,
  QueueWorkload,
  RootJobPolicies,
  ScheduleProfile,
  ScheduleProfileInput,
  ScheduleRequest,
  ScheduleRequestInput,
  TargetSchedules,
  TargetUserAccess,
  RepositoryDetail,
  RepositorySummary,
  RootElevation,
  RootElevationInput,
  RootWorkspace,
  RootOverview,
  SyncConfig,
  SyncFileMergeEntry,
  SyncFileRepositoryPolicy,
  SyncKind,
  SyncOverride,
  RootRuntimeSettings,
  RootRuntimeSettingsInput,
  SettingsCheckpoint,
  SettingsCheckpointIncompatibility,
  SettingsCheckpointItem,
  SettingsCheckpointItemKind,
  SettingsCheckpointState,
  SettingsRestoreInput,
  UpdateTargetUserInput,
  UpdateRootUserInput,
  InvitationDays,
  WorkspaceRepositorySettingsInput,
  WorkspaceRepositorySettingsState,
  WorkspaceSettingsBatchInput,
  WorkspaceSettingsBatchResponse,
  WorkspaceSettingsConflict,
  WorkspaceSyncConfigSettingsInput,
  WorkspaceSyncConfigSettingsState,
  WorkspaceSyncOverrideSettingsInput,
  WorkspaceSyncOverrideSettingsState,
  WorkspaceTargetSettingsInput,
  WorkspaceTargetSettingsState,
} from '../src/lib/types.ts';
import { SYNC_KINDS } from '../src/lib/types.ts';
import { CONFIG_KEYS } from '../src/lib/config.ts';
import {
  applyFormattingPatch,
  applyFormattingSources,
  formattingSources,
  parseFormattingPatch,
  parseFormattingPolicy,
} from '../src/lib/formatting.ts';
import { canonicalStringify, PREF_DEFAULTS } from '../src/lib/preferences-sync.ts';
/* The fixtures, which used to be nine hundred lines of this file and reachable by
   nothing. They are their own module so the Storybook catalogue can read the same data
   this serves, and so importing them never drags `node:fs` into a browser. */
import {
  KNOWN_PATHS,
  capabilitiesFor,
  cycled,
  DEFAULT_CONFIG,
  DEV_MAX_PATH_INDEX_SECONDS,
  DEV_PATH_INDEX_SECONDS,
  DEV_PENDING_CI_QUIET_SECONDS,
  MOCK_ORGANIZATION_ROSTER,
  mockRepositoryPaths,
  mockRepositoryScanAge,
  mockSyncConfig,
  ROOT_READ_CAPABILITIES,
  mockRootOwns,
  rootPanelUsers,
  seed,
  targetAccess,
  VIEWER,
  type MockInvitation,
  type MockRepository,
  type MockState as Fixtures,
  type MockTarget,
} from './fixtures.ts';
import { parseInvitationToken, parsePanelRoute } from '../src/lib/routes.ts';
import { renderMockSyncFile } from './mock-file-render.ts';

type DevHttpServer = HttpServer;
const BASE = '';
const DEFAULT_PAGE_SIZE = 20;

/**
 * Stands in for the organization roster the service reads from GitHub.
 *
 * Deliberately holds names that overlap on different parts - a login starting
 * with the query, a display name starting with it, and one that only contains it
 * - so the ordering the panel applies is visible while working on it.
 */

class MockApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly details: Record<string, unknown> = {},
  ) {
    super(message);
    this.name = 'MockApiError';
  }
}

/**
 * mockSyncConfig answers what a workspace has configured, inventing an
 * empty answer the first time. Never configured is not an error and not the
 * same as configured and switched off, which is what the server says too.
 */

/** Where the page the panel boots comes from, so the error renderer can patch it. */
type ShellSource = () => Promise<string>;

/* The fixture state plus the two things that belong to the server rather than to the
   data: the sockets a preference change is broadcast over, and the fetcher that answers
   with the built shell. Both are typed against `node:`, which is exactly why they are
   here and not in `fixtures.ts`. */
interface MockState extends Fixtures {
  streams: Set<Duplex>;
  shell: ShellSource;
  scheduleProfiles: ScheduleProfile[];
  scheduleDefaults: QueuePolicy[];
  schedulePolicies: QueuePolicy[];
  scheduleRequests: ScheduleRequest[];
  scheduleCounter: number;
}

/** Marks the error renderer's own request for a shell, so `handle` stands aside. */
const SHELL_REQUEST_HEADER = 'x-smyklot-mock-shell';

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

const NANOSECONDS_PER_SECOND = 1_000_000_000;
const MOCK_SCHEDULE_KINDS: QueueWorkload[] = [
  'webhook_delivery',
  'pending_ci',
  'pending_ci_gate',
  'catalog_refresh',
  'reaction_scan',
  'config_migration',
  'sync_scan',
  'sync_apply',
  'path_refresh',
  'delivery_cleanup',
  'auth_cleanup',
];

function mockScheduleState(
  fixtures: Fixtures,
): Pick<
  MockState,
  | 'scheduleProfiles'
  | 'scheduleDefaults'
  | 'schedulePolicies'
  | 'scheduleRequests'
  | 'scheduleCounter'
> {
  const now = new Date().toISOString();
  const alwaysOpen: ScheduleProfile = {
    id: 'always-open',
    name: 'Always Open',
    timezone: 'UTC',
    system: true,
    revision: 1,
    affected_workspaces: fixtures.targets.length,
    affected_items: fixtures.queue.filter((item) => item.profile_id === 'always-open').length,
    affected_policies: MOCK_SCHEDULE_KINDS.length,
    windows: Array.from({ length: 7 }, (_, weekday) => ({
      weekday,
      start_minute: 0,
      end_minute: 24 * 60,
    })),
    exceptions: [],
  };
  const europeHours: ScheduleProfile = {
    id: 'europe-hours',
    name: 'Europe business hours',
    timezone: 'Europe/Warsaw',
    system: false,
    revision: 3,
    affected_workspaces: 1,
    affected_items: 2,
    affected_policies: 1,
    windows: [1, 2, 3, 4, 5].map((weekday) => ({
      weekday,
      start_minute: 9 * 60,
      end_minute: 17 * 60,
    })),
    exceptions: [{ date: '2026-12-25', closed: true }],
  };
  const cadenceSeconds: Record<QueueWorkload, number> = {
    webhook_delivery: 0,
    pending_ci: 30,
    pending_ci_gate: 60 * 60,
    catalog_refresh: 5 * 60,
    reaction_scan: 5 * 60,
    config_migration: 5 * 60,
    sync_scan: 6 * 60 * 60,
    sync_apply: 0,
    path_refresh: 60 * 60,
    delivery_cleanup: 60 * 60,
    auth_cleanup: 60 * 60,
    schedule_change: 0,
  };
  const policies = MOCK_SCHEDULE_KINDS.map<QueuePolicy>((kind) => ({
    kind,
    enabled: true,
    cadence: cadenceSeconds[kind] * NANOSECONDS_PER_SECOND,
    profile_id: 'always-open',
    default_priority: kind === 'webhook_delivery' ? 'high' : 'normal',
    retry_delay: (kind === 'webhook_delivery' ? 30 : 5 * 60) * NANOSECONDS_PER_SECOND,
    ...(kind === 'sync_scan' ? { approval_ttl: 2 * 60 * 60 * NANOSECONDS_PER_SECOND } : {}),
    ...(kind === 'pending_ci'
      ? {
          configuration: {
            active_check_seconds: 30,
            no_check_grace_seconds: 120,
            defer_after_seconds: 900,
            deferred_check_seconds: 300,
            passing_quiet_seconds: 30,
          },
        }
      : {}),
    ...(kind === 'webhook_delivery'
      ? { configuration: { max_delay_seconds: 3600, max_attempts: 8 } }
      : {}),
    revision: 1,
    updated_at: now,
  }));
  const primaryTarget = fixtures.targets[0]?.value.id;
  if (primaryTarget !== undefined) {
    const base = policies.find((policy) => policy.kind === 'sync_scan');
    if (base !== undefined) {
      policies.push({
        ...base,
        target_id: primaryTarget,
        profile_id: europeHours.id,
        default_priority: 'high',
        revision: 2,
      });
    }
  }

  const requests: ScheduleRequest[] =
    primaryTarget === undefined
      ? []
      : [
          {
            id: 'request:mock-1',
            target_id: primaryTarget,
            kind: 'path_refresh',
            state: 'pending',
            base_revision: 1,
            profile_id: europeHours.id,
            cadence: 30 * 60 * NANOSECONDS_PER_SECOND,
            default_priority: 'normal',
            reason: 'Refresh which paths are watched during the release preparation window',
            requested_by: VIEWER.id,
            requester: VIEWER,
            revision: 1,
            created_at: now,
            updated_at: now,
          },
        ];

  return {
    scheduleProfiles: [alwaysOpen, europeHours],
    scheduleDefaults: structuredClone(policies.filter((policy) => policy.target_id === undefined)),
    schedulePolicies: policies,
    scheduleRequests: requests,
    scheduleCounter: 2,
  };
}

/**
 * Preferences outlive the process they were set in.
 *
 * Everything the panel remembers about you is a synced preference - the theme, whether the sidebar
 * is collapsed, and every table's sort, filters and search. The mock held them in memory, so each
 * Vite restart, and each edit to this file, handed back a factory-fresh panel and you set them all
 * again. Only the preference document is kept: the rest of the mock is fixture data, and a fixture
 * that drifts across restarts is worse than one that resets.
 *
 * Which is why a test run must not read this one. The browser suite drives the same mock, so before
 * this could be pointed elsewhere it measured the panel through whatever the developer had last set
 * here - and one of those preferences is whether the rail is collapsed, which decides whether the
 * rail draws section headers at all. `sidebar-selection` went red on a machine where a dev session
 * had collapsed it and stayed green on CI, where the file does not exist. The theme is in here too,
 * so every colour the suite measures was one dev preference away from being the other palette.
 *
 * Read on each call rather than resolved once: whether this module is imported before or after a
 * caller sets the variable is a detail of Vite's plugin loading, and a value captured at import
 * would make the isolation depend on it.
 */
function preferencesFile(): string {
  return (
    process.env.SMYKLOT_PANEL_DEV_MOCK_PREFERENCES ??
    resolve(dirname(fileURLToPath(import.meta.url)), '.mock-preferences.json')
  );
}

interface DevState {
  prefs: MockState['prefs'];
  /** Invitations issued while the mock was running, so a link you generated still opens later. */
  invitations: MockInvitation[];
  invitationCounter: number;
}

function readDevState(): Partial<DevState> {
  try {
    const parsed: unknown = JSON.parse(readFileSync(preferencesFile(), 'utf8'));
    return parsed !== null && typeof parsed === 'object' ? (parsed as Partial<DevState>) : {};
  } catch {
    // No file yet, or one this build cannot read. Starting clean beats refusing to serve.
    return {};
  }
}

function loadPreferences(): MockState['prefs'] {
  try {
    const parsed: unknown = JSON.parse(readFileSync(preferencesFile(), 'utf8'));
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
      preferencesFile(),
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

/** A label set an organization has configured, switched on and being enforced. */

/**
 * The ruleset an organization actually runs, seeded for the same reason the
 * labels above are: a form nobody can look at except empty is a form that
 * drifts out of the design unseen, and this one has nested rows that only
 * appear once a rule carrying parameters is switched on.
 */

/**
 * A plan waiting for somebody, carrying one of everything the list can draw: an
 * addition, a change, a removal, a row that failed and a row that was never
 * tried because another failed first.
 */

/* The ids are token-shaped because a real one is: the server mints an elevation
   id with `randomToken`, and the inbox shows its tail as the correlation key to
   search the audit trail with. Readable slugs here rendered as "Elevation
   d-incident", which reads as a bug in the panel rather than as a fixture that
   does not look like production. */

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

/**
 * A path the way Go hands it to the panel server, or `null` when it is not a path at all.
 *
 * `net/http` fills `r.URL.Path` with the decoded form, so `%2F` arrives as a separator and
 * `serveAsset` matches on that. Node leaves `URL.pathname` encoded, and a malformed escape
 * throws where Go answers 400, which is the same refusal.
 */
function decodedPath(path: string): string | null {
  try {
    return decodeURIComponent(path);
  } catch {
    return null;
  }
}

export function mockServer(): Plugin {
  return {
    name: 'smyklot-panel-mock-server',
    // Serve only. Nothing here has anything to say about a build.
    apply: 'serve',
    config() {
      if (!enabled()) return;
      return { server: opensBrowser() ? { open: '/' } : {} };
    },
    configureServer(server) {
      if (!enabled()) return;
      install(server.httpServer as DevHttpServer | null, server.middlewares);
    },
    configurePreviewServer(server) {
      if (enabled()) {
        install(server.httpServer as DevHttpServer | null, server.middlewares);
      }
    },
  };
}

/**
 * Attaches the mock to a Vite server.
 *
 * `httpServer` is null when Vite runs in middleware mode - which is how Storybook
 * hosts it. There is no socket to upgrade there and no shell to fetch, so the two
 * things that need one are skipped rather than refused: the API middleware is the
 * part a component catalogue wants, and it works either way.
 *
 * What is lost in that mode is the WebSocket stream at `/api/v1/events` and the
 * error-page preview at `/__error/{status}/{code}`, which asks the server for a
 * rendered shell. `state.shell` keeps the rejecting default it was seeded with, so
 * asking for one says what is missing instead of reading as an empty page.
 */
function install(httpServer: DevHttpServer | null | undefined, middlewares: Connect.Server): void {
  const fixtures = seed(loadIssuedInvitations(), Date.now(), loadPreferences());
  const state: MockState = {
    ...fixtures,
    ...mockScheduleState(fixtures),
    streams: new Set(),
    shell: () => Promise.reject(new Error('the mock dev server is not serving yet')),
  };
  if (httpServer !== null && httpServer !== undefined) {
    const server = httpServer;
    state.shell = () => fetchShell(server);
    server.on('upgrade', (request, socket) => handleUpgrade(state, request, socket));
  }
  middlewares.use((req, res, next) => void handle(state, req, res, next));
  runReconciler(state);
}

/**
 * The reconciler, as far as the queue can see it.
 *
 * A deadline in this table is the only thing on the Root console that moves on its own, and the
 * mock used to stop it: a seeded quiet period was pushed forward every time the page asked, so the
 * countdown never reached zero and no reader ever saw what happens when it does. The whole point of
 * the Next column is the moment after that, and it was the one moment development could not show.
 *
 * So the deadlines run. Every second the mock does what `internal/pendingci` does - looks at what is
 * due, moves it on, and tells whoever is listening - and the panel finds out the way it finds out in
 * production, over the stream. Nothing here is a fixture being edited: the sequence of states is the
 * service's own, so what a reader watches is the shape of the real thing at a speed a person can sit
 * through.
 *
 * The tick is unreferenced, so it does not hold the dev server open on its own. And it does not run
 * at all under `SMYKLOT_PANEL_DEV_MOCK_FROZEN`, which is what the browser sweeps set: a suite
 * measuring a table cannot have the table re-sort itself half way through the measurement.
 */
function runReconciler(state: MockState): void {
  if (process.env.SMYKLOT_PANEL_DEV_MOCK_FROZEN === '1') return;
  setInterval(() => reconcile(state), 1_000).unref();
}

/** How long a finished request rests in Recent before it is armed again, so the loop can be watched twice. */
const REARM_AFTER_MS = 25_000;

function reconcile(state: MockState): void {
  const now = Date.now();
  let changed = false;

  for (const [index, request] of state.pendingCI.entries()) {
    const next =
      request.lifecycle === 'armed'
        ? advanceArmed(request, now)
        : advanceFinished(request, now, state.queueLoop.has(request.id));
    if (next === request) continue;
    if (next.lifecycle !== 'armed' && request.lifecycle === 'armed') {
      state.queueLoop.add(request.id);
    }
    state.pendingCI[index] = next;
    changed = true;
  }

  if (advanceQueue(state, now)) changed = true;

  if (changed) broadcast(state, { type: 'resync' });
}

/**
 * One row's loop: it waits, it runs, it rests, it waits again.
 *
 * Proportioned so a reader watching the overview sees both marks: a row spends a third
 * of its cycle running, and the rows are held apart in phase, so the column almost
 * always carries one of each.
 */
const QUEUE_WAIT_MS = 45_000;
const QUEUE_RUN_MS = 30_000;
const QUEUE_REST_MS = 10_000;

const QUEUE_DONE = new Set(['succeeded', 'failed', 'cancelled', 'superseded']);

/**
 * The queue's rows, walked the way the pending-CI table's already are.
 *
 * The seeds carried the variety - one row running, one waiting on required checks, one
 * retrying after a rate limit - and nothing moved them, so every estimate in them was
 * stamped once at startup and went stale within the hour. A dev server left open for an
 * afternoon showed three rows all reading "now", which is not one of the states the
 * fixture describes and is not a state the service can be in either.
 *
 * So each row runs its own loop: a waiting row whose estimate passes starts, a running
 * row fills its progress and finishes, and a finished row rests and then waits again with
 * a fresh estimate. The resting shape comes off `queueRest`, so a row goes back to being
 * blocked on the thing it was blocked on rather than to a guess.
 *
 * Rows that are somebody's decision rather than the service's - `awaiting_approval` -
 * stand still, because nothing but a person moves them.
 */
function advanceQueue(state: MockState, now: number): boolean {
  let changed = false;

  /* Phase, not identity: the rows that take part are counted so each can be given a
     different slice of one cycle to start in, and they keep that difference for ever
     after - every row's cycle is the same length. */
  const looping = state.queue.filter((item) => state.queueRest.has(item.id));

  for (const [index, item] of state.queue.entries()) {
    const next = advanceQueueItem(state, item, now, looping.indexOf(item), looping.length);
    if (next === item) continue;
    state.queue[index] = next;
    changed = true;
  }

  return changed;
}

function advanceQueueItem(
  state: MockState,
  item: QueueItem,
  now: number,
  place: number,
  count: number,
): QueueItem {
  const at = (offsetMs: number) => new Date(now + offsetMs).toISOString();
  const rest = state.queueRest.get(item.id);
  if (item.state === 'awaiting_approval' || rest === undefined) return item;

  if (QUEUE_DONE.has(item.state)) {
    /* Only what this process watched SUCCEED, and only the loop's own doing. The seeded
       terminal rows are the past that Recent exists to show, and a past that arms itself
       again is not a past - that is the rule the pending-CI table follows. The state is
       checked as well as the id, because a row somebody cancelled is also terminal and
       also in the loop: without it the mock put a cancelled row back ten seconds later,
       which is the mock overruling the person using it. */
    if (item.state !== 'succeeded' || !state.queueLoop.has(item.id)) return item;
    const finished = Date.parse(item.finished_at ?? item.updated_at);
    if (Number.isNaN(finished) || now - finished < QUEUE_REST_MS) return item;

    /* A row rests as a WAITING row, whatever it was seeded as. `queue-sync-apply` is
       seeded mid-run, because that is the picture the fixture is drawing; putting that
       shape back would hand it a start time from before the process began and finish it
       again on the next tick, so it would flap between running and done and never be
       seen waiting. */
    const waiting: QueueItem = { ...rest };
    delete waiting.started_at;
    delete waiting.finished_at;

    return {
      ...waiting,
      state: rest.state === 'running' || QUEUE_DONE.has(rest.state) ? 'scheduled' : rest.state,
      ...(rest.progress_total === undefined ? {} : { progress_current: 0 }),
      not_before: at(0),
      eligible_at: at(QUEUE_WAIT_MS),
      estimated_start_at: at(QUEUE_WAIT_MS),
      created_at: at(0),
      updated_at: at(0),
      revision: item.revision + 1,
    };
  }

  if (item.state === 'running') {
    const started = Date.parse(item.started_at ?? item.updated_at);
    if (Number.isNaN(started)) return item;
    const through = Math.min(1, (now - started) / QUEUE_RUN_MS);
    const total = item.progress_total;
    if (through < 1) {
      /* A row with no progress to report still runs; it just has nothing to say while
         it does, so only the ones carrying a total tick. */
      if (total === undefined) return item;
      const done = Math.max(1, Math.min(total, Math.round(through * total)));
      if (done === item.progress_current) return item;

      /* Progress does NOT bump the revision. The revision is the token a reader holds
         between opening a row's dialog and pressing its button, and a row that bumped it
         every second refused every action with a conflict - which made every queue
         control in the panel untestable against this mock. A state change still bumps
         it: that is a row which genuinely is not what the reader was looking at. */
      return { ...item, progress_current: done, updated_at: at(0) };
    }

    state.queueLoop.add(item.id);

    return {
      ...item,
      state: 'succeeded',
      progress_current: total,
      finished_at: at(0),
      updated_at: at(0),
      revision: item.revision + 1,
    };
  }

  /* Everything else is waiting on its estimate, which is the row Active work draws its
     chip for. It starts when the estimate passes and not before.
     A seeded estimate can be further out than a whole cycle - the fixture describes a
     screenshot, where four and six minutes read well - so the first tick pulls it into
     this loop, each row at its own point in the cycle. */
  const due = Date.parse(item.estimated_start_at ?? item.eligible_at);
  if (Number.isNaN(due)) return item;
  if (due - now > QUEUE_WAIT_MS) {
    /* Inside the wait, never past it. A slot beyond `QUEUE_WAIT_MS` is further out than
       the test that put it there, so the next tick pulls it in again to the same place
       and the row never comes due at all - the whole table sat in waiting states and
       nothing moved for as long as anyone watched. */
    const slot = count === 0 ? QUEUE_WAIT_MS : (QUEUE_WAIT_MS * (place + 1)) / count;

    return {
      ...item,
      eligible_at: at(slot),
      estimated_start_at: at(slot),
      updated_at: at(0),
      revision: item.revision + 1,
    };
  }
  if (due > now) return item;

  const running: QueueItem = { ...item };
  /* What it was waiting on is not what it is doing: a running row that still carried its
     blocked reason read "Waiting on required checks" while it ran. */
  delete running.blocked_reason;

  return {
    ...running,
    state: 'running',
    started_at: at(0),
    estimated_start_at: at(0),
    work_ahead: 0,
    ...(item.progress_total === undefined ? {} : { progress_current: 0 }),
    updated_at: at(0),
    revision: item.revision + 1,
  };
}

/**
 * One armed request, one deadline.
 *
 * The states follow the service's: a request arrives with nothing observed, the first look finds
 * checks running, a later one finds them green, and green starts the quiet period whose expiry IS
 * the merge. Failing and no-checks stay where they are and are looked at again, because that is what
 * the service does with them - it is the pull request that has to change, not the reconciler.
 */
function advanceArmed(request: PendingCIRequest, now: number): PendingCIRequest {
  if (Date.parse(request.next_check_at) > now) return request;

  const at = (offsetMs: number) => new Date(now + offsetMs).toISOString();
  const looked = {
    ...request,
    updated_at: at(0),
    revision: request.revision + 1,
  };

  if (request.next_check_trigger === 'quiet_period' && request.last_observed_state === 'passing') {
    return {
      ...looked,
      lifecycle: 'merged',
      finished_at: at(0),
      next_check_at: at(0),
      next_check_trigger: 'cleanup',
      reason: 'Checks passed and stayed quiet for 30 s',
      /* Merged first, tidied after: the label and the reactions come off in a later pass, which is
         why the Cleanup column has a Pending state to draw at all. */
      cleanup_pending: true,
    };
  }

  /* Green starts the quiet period, whose expiry is the merge above. Everything else converges on
     running: checks appear where there were none, a run that could not be read is read on the next
     look, and a red one goes green because somebody pushed a fix. That last step is the mock taking
     a liberty - the service would find it red again - and it is the liberty that makes every row
     here move, so that every row can be watched all the way round and every button on one does
     something the reader can see. */
  if (request.last_observed_state === 'pending') {
    return {
      ...looked,
      last_observed_state: 'passing',
      next_check_at: at(QUIET_PERIOD_MS),
      next_check_trigger: 'quiet_period',
    };
  }

  return {
    ...looked,
    last_observed_state: 'pending',
    next_check_at: at(12_000),
    next_check_trigger: 'fallback',
  };
}

/** The quiet period the service holds a passing request for, from `internal/pendingci/policy.go`. */
const QUIET_PERIOD_MS = 30_000;

/**
 * What happens to a request after it has finished: its cleanup completes, and then, so the loop can
 * be watched more than once, somebody pushes again and it is armed afresh.
 */
function advanceFinished(
  request: PendingCIRequest,
  now: number,
  recyclable: boolean,
): PendingCIRequest {
  const finished = request.finished_at === undefined ? undefined : Date.parse(request.finished_at);
  if (finished === undefined || Number.isNaN(finished)) return request;

  const at = (offsetMs: number) => new Date(now + offsetMs).toISOString();

  if (request.cleanup_pending && now - finished > 6_000) {
    return {
      ...request,
      cleanup_pending: false,
      updated_at: at(0),
      revision: request.revision + 1,
    };
  }

  // Only what this process watched finish - see `queueLoop` on `MockState`.
  if (!recyclable || now - finished < REARM_AFTER_MS) return request;

  return {
    ...request,
    lifecycle: 'armed',
    schedule: 'active',
    head_sha: nextHeadSHA(request.head_sha),
    last_observed_state: '',
    reason: '',
    requested_at: at(0),
    updated_at: at(0),
    finished_at: undefined,
    next_check_at: at(10_000),
    next_check_trigger: 'command',
    cleanup_pending: false,
    revision: request.revision + 1,
  };
}

/** A new commit on the same branch, drawn from the old one so the short sha visibly changes. */
function nextHeadSHA(sha: string): string {
  const digits = [...'0123456789abcdef'];
  const rolled = [...sha.slice(0, 7)]
    .map((character) => cycled(digits, digits.indexOf(character) + 1))
    .join('');

  return `${rolled}${sha.slice(7)}`;
}

/**
 * The page the panel boots, borrowed from the server that is already serving it.
 *
 * Production patches its error descriptor into the built `index.html`, which
 * SvelteKit finished at build time. Dev has no such file: `src/app.html` is a
 * template, and its `%sveltekit.head%` and `%sveltekit.body%` are filled in by
 * SvelteKit as it answers a request and by nothing else - Vite's own
 * `transformIndexHtml` leaves them alone. Rendering the template here produced a
 * document carrying those two placeholders as text, which booted no panel at all.
 *
 * So the mock asks itself for the panel root, an address every route table has,
 * and patches what comes back. Which address produced the page does not matter:
 * the panel is client-rendered, and its router reads the location it boots at,
 * which is the one the reader asked for.
 */
async function fetchShell(httpServer: DevHttpServer): Promise<string> {
  const response = await fetch(new URL('/', selfOrigin(httpServer)), {
    headers: { accept: 'text/html', [SHELL_REQUEST_HEADER]: '1' },
  });
  if (!response.ok) {
    throw new Error(`the mock dev server answered ${response.status} for its own page`);
  }

  return await response.text();
}

/** Loopback for the addresses that name every interface rather than one. */
const LOOPBACK: Record<string, string> = { '::': '::1', '0.0.0.0': '127.0.0.1' };

/**
 * Where to reach this server from inside it.
 *
 * Its own bound address rather than the `Host` header the reader arrived with:
 * that header can name a tunnel or a machine, and a request meant to stay in this
 * process would leave it.
 */
function selfOrigin(httpServer: DevHttpServer): string {
  const address = httpServer.address();
  if (address === null || typeof address === 'string') {
    throw new Error('the mock dev server is not listening on a port');
  }
  const host = LOOPBACK[address.address] ?? address.address;

  return `http://${address.family === 'IPv6' ? `[${host}]` : host}:${address.port}`;
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
  /**
   * The error renderer asking for a page to patch. It has to reach SvelteKit
   * untouched: `/` applies whatever scenario the query string names, and resetting
   * the scenario is the last thing you want while looking at the error it produced.
   */
  if (req.headers[SHELL_REQUEST_HEADER] !== undefined) {
    next();
    return;
  }
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
  if (method === 'GET' && wantsDocument(req)) {
    const served = decodedPath(path);
    const navigable =
      served !== null &&
      (parsePanelRoute('/', served) !== null || parseInvitationToken('/', served) !== null);
    if (!navigable) {
      await respondError(state, req, res, 404, 'not_found', 'panel route not found');
      return;
    }
  }
  /* Fixture profile pictures. Production accounts carry GitHub avatar URLs;
     the mock serves its own so the panel exercises the image path rather than
     only ever the monogram fallback - which is how losing every real avatar
     once went unseen in dev. */
  const avatar = path.match(/^\/__avatar\/(?<login>[a-z0-9-]+)\.svg$/u);
  if (avatar && method === 'GET') {
    res.statusCode = 200;
    res.setHeader('Content-Type', 'image/svg+xml');
    res.end(devAvatarSVG(avatar.groups?.login ?? ''));
    return;
  }
  const publicInvitation = path.match(/^\/api\/v1\/invites\/(?<token>[^/]+)$/);
  if (publicInvitation && method === 'GET') {
    try {
      const invitation = findInvitationByToken(state, publicInvitation.groups?.token ?? '');
      respond(res, 200, publicInvitationValue(invitation));
    } catch (error) {
      if (error instanceof MockApiError) {
        respond(res, error.status, {
          error: { code: error.code, message: error.message, ...error.details },
        });
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
        /* A Root with no workspaces keeps the panel shell, because the console
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
    const syncPath = path.slice(route('').length);
    const syncConfigMatch = /^\/api\/v1\/targets\/([^/]+)\/sync\/config\/([^/]+)$/.exec(syncPath);
    if (syncConfigMatch) {
      // Keyed by workspace and kind together, because a workspace
      // configures each kind separately and the server stores them that way.
      const targetId = decodeURIComponent(syncConfigMatch[1] ?? '');
      const kind = decodeURIComponent(syncConfigMatch[2] ?? '');
      const config = mockSyncConfig(state, `${targetId}/${kind}`, kind);
      if (method === 'GET') {
        respond(res, 200, config);
        return;
      }
    }

    // What the path finder offers, aggregated the way the server aggregates it:
    // one row per path, carrying how many repositories already hold it.
    const syncPathsMatch = /^\/api\/v1\/targets\/([^/]+)\/sync\/paths$/.exec(
      path.slice(route('').length),
    );
    if (syncPathsMatch && method === 'GET') {
      const target = findTarget(state, decodeURIComponent(syncPathsMatch[1] ?? ''));
      const day = 24 * 60 * 60 * 1000;

      respond(
        res,
        200,
        syncPathIndex(
          target.repositories.map((repository) => {
            const name = repository.detail.repository.name;

            return {
              repository_id: repository.detail.repository.id,
              paths: mockRepositoryPaths(name),
              observed_at: new Date(Date.now() - mockRepositoryScanAge(name) * day).toISOString(),
            };
          }),
        ),
      );
      return;
    }

    // Every repository's answer about one kind, which is what the page about a
    // shared file reads: "who adjusts this" is a question about the whole
    // workspace.
    const syncOverrideMatch =
      /^\/api\/v1\/targets\/([^/]+)\/repositories\/([^/]+)\/sync\/([^/]+)$/.exec(
        path.slice(route('').length),
      );
    if (syncOverrideMatch) {
      // Keyed by repository and kind, because a repository answers each kind on
      // its own and the server stores one row per pair.
      const repositoryId = decodeURIComponent(syncOverrideMatch[2] ?? '');
      const kind = decodeURIComponent(syncOverrideMatch[3] ?? '');
      const key = `${repositoryId}/${kind}`;
      const override = state.syncOverrides.get(key) ?? {
        kind,
        enabled: null,
        document: {},
        revision: 0,
        unreadable: false,
      };
      if (method === 'GET') {
        respond(res, 200, override);
        return;
      }
    }

    const syncPlanMatch = /^\/api\/v1\/targets\/([^/]+)\/sync\/plan$/.exec(
      path.slice(route('').length),
    );
    if (syncPlanMatch && method === 'GET') {
      const targetId = decodeURIComponent(syncPlanMatch[1] ?? '');
      respond(res, 200, { plan: state.syncPlans.get(targetId) ?? null });
      return;
    }

    const syncStatusMatch = /^\/api\/v1\/targets\/([^/]+)\/sync\/status$/.exec(
      path.slice(route('').length),
    );
    if (syncStatusMatch && method === 'GET') {
      const targetId = decodeURIComponent(syncStatusMatch[1] ?? '');
      /* A workspace the seed never drew is a fleet nothing covers yet,
         not an error - the overview renders the empty answer. */
      respond(
        res,
        200,
        state.syncStatus.get(targetId) ?? {
          checked_at: new Date().toISOString(),
          repositories: [],
        },
      );
      return;
    }

    const syncFilesContextMatch = /^\/api\/v1\/targets\/([^/]+)\/sync\/files\/context$/.exec(
      path.slice(route('').length),
    );
    const syncFileRenderMatch = /^\/api\/v1\/targets\/([^/]+)\/sync\/files\/render$/.exec(
      path.slice(route('').length),
    );
    if (syncFileRenderMatch && method === 'POST') {
      findTarget(state, syncFileRenderMatch[1] ?? '');
      respond(res, 200, renderMockSyncFile(await readBody<unknown>(req)));
      return;
    }
    if (syncFilesContextMatch && method === 'GET') {
      const targetId = decodeURIComponent(syncFilesContextMatch[1] ?? '');
      const target = findTarget(state, targetId);
      const status = state.syncStatus.get(targetId);
      const rows = status?.repositories ?? [];
      const covered = rows.filter((row) => row.cells.files.state !== 'off').length;
      const adjustments = new Map<string, SyncFileMergeEntry>();
      for (const [key, override] of state.syncOverrides) {
        const [repositoryId, kind] = key.split('/');
        if (kind !== 'files' || repositoryId === undefined) continue;
        const name =
          target.repositories.find((repository) => repository.detail.repository.id === repositoryId)
            ?.detail.repository.name ?? repositoryId;
        const heldMerges = override.document.merges;
        if (Array.isArray(heldMerges)) {
          for (const merge of heldMerges as Array<Record<string, unknown>>) {
            if (typeof merge.path !== 'string') continue;
            adjustments.set(`${repositoryId}\u0000${merge.path}`, {
              repository: name,
              repository_id: repositoryId,
              path: merge.path,
              merge,
            });
          }
        }
        const heldFormats = override.document.formats;
        if (Array.isArray(heldFormats)) {
          for (const format of heldFormats) {
            if (
              typeof format !== 'object' ||
              format === null ||
              !('path' in format) ||
              typeof format.path !== 'string' ||
              !('formatting' in format)
            ) {
              continue;
            }
            const formatting = parseFormattingPatch(format.formatting);
            if (formatting === null) continue;
            const adjustmentKey = `${repositoryId}\u0000${format.path}`;
            const current = adjustments.get(adjustmentKey);
            adjustments.set(adjustmentKey, {
              repository: name,
              repository_id: repositoryId,
              path: format.path,
              ...(current?.merge === undefined ? {} : { merge: current.merge }),
              formatting,
            });
          }
        }
      }
      const repositoryPolicies: SyncFileRepositoryPolicy[] = rows.map((row) => {
        /* The fleet names repositories this workspace holds, so the lookup
           finds one. The `mock:` fallback is for a seed that names one it does
           not - which is the drift this fixture used to be built on. */
        const repository = target.repositories.find(
          (candidate) => candidate.detail.repository.name === row.repository,
        );
        const repositoryId = repository?.detail.repository.id ?? `mock:${row.repository}`;
        return {
          repository: repository?.detail.repository.name ?? row.repository,
          repository_id: repositoryId,
          default_branch: repository?.detail.repository.default_branch ?? 'main',
          base_policy:
            repository?.detail.effective_config.formatting ??
            target.value.effective_config.formatting,
        };
      });
      respond(res, 200, {
        repositories: rows.length,
        covered,
        known_paths: KNOWN_PATHS,
        base_formatting: target.value.effective_config.formatting,
        repository_policies: repositoryPolicies,
        merges: [...adjustments.values()],
      });
      return;
    }

    const syncApprovalMatch = /^\/api\/v1\/targets\/([^/]+)\/sync\/plans\/([^/]+)\/approval$/.exec(
      path.slice(route('').length),
    );
    if (syncApprovalMatch && method === 'POST') {
      const targetId = decodeURIComponent(syncApprovalMatch[1] ?? '');
      const plan = state.syncPlans.get(targetId);
      const input = await readBody<{ digest: string }>(req);
      if (!plan) {
        throw new MockApiError(404, 'not_found', 'there is no plan to approve');
      }
      if (plan.digest !== input.digest) {
        throw new MockApiError(
          409,
          'stale_plan',
          'this plan no longer matches the configuration; ask for a new one',
        );
      }
      const approved = {
        ...plan,
        state: 'approved' as const,
        approved_at: new Date().toISOString(),
      };
      state.syncPlans.set(targetId, approved);
      respond(res, 200, { plan: approved });
      return;
    }

    const syncDiscardMatch = /^\/api\/v1\/targets\/([^/]+)\/sync\/plans\/([^/]+)$/.exec(
      path.slice(route('').length),
    );
    if (syncDiscardMatch && method === 'DELETE') {
      const targetId = decodeURIComponent(syncDiscardMatch[1] ?? '');
      const planId = decodeURIComponent(syncDiscardMatch[2] ?? '');
      const plan = state.syncPlans.get(targetId);
      if (!plan || plan.id !== planId) {
        throw new MockApiError(404, 'not_found', 'there is no such plan to discard');
      }
      state.syncPlans.delete(targetId);
      respond(res, 200, { plan: { ...plan, state: 'discarded' } });
      return;
    }

    if (path === route('/api/v1/root/workspaces') && method === 'GET') {
      const ordered = [...state.targets].sort((left, right) =>
        left.value.type === right.value.type ? 0 : left.value.type === 'Organization' ? -1 : 1,
      );
      respond(res, 200, {
        workspaces: ordered.map((target) => rootWorkspaceValue(target)),
      });
      return;
    }
    if (path === route('/api/v1/root/workspaces/sync') && method === 'POST') {
      broadcast(state, { type: 'resync' });
      respond(res, 200, { target_ids: state.targets.map((target) => target.value.id) });
      return;
    }
    if (path === route('/api/v1/root/overview') && method === 'GET') {
      respond(res, 200, rootOverviewValue(state));
      return;
    }
    if (path === route('/api/v1/root/queue') && method === 'GET') {
      respond(res, 200, mockQueuePage(state.queue, parsed.searchParams));
      return;
    }
    const targetQueue = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/queue$/);
    if (targetQueue && method === 'GET') {
      const target = findTarget(state, targetQueue.groups?.target ?? '');
      respond(
        res,
        200,
        mockQueuePage(
          state.queue.filter((item) => item.target_id === target.value.id),
          parsed.searchParams,
        ),
      );
      return;
    }
    const rootQueueDetail = path.match(/^\/api\/v1\/root\/queue\/(?<item>[^/]+)$/);
    const targetQueueDetail = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/queue\/(?<item>[^/]+)$/,
    );
    if ((rootQueueDetail || targetQueueDetail) && method === 'GET') {
      const match = rootQueueDetail ?? targetQueueDetail;
      const target = targetQueueDetail
        ? findTarget(state, targetQueueDetail.groups?.target ?? '')
        : undefined;
      const item = findMockQueueItem(state.queue, match?.groups?.item ?? '', target?.value.id);
      respond(res, 200, mockQueueDetail(item));
      return;
    }
    const rootQueuePreview = path.match(
      /^\/api\/v1\/root\/queue\/(?<item>[^/]+)\/actions\/preview$/,
    );
    const targetQueuePreview = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/queue\/(?<item>[^/]+)\/actions\/preview$/,
    );
    if ((rootQueuePreview || targetQueuePreview) && method === 'POST') {
      const match = rootQueuePreview ?? targetQueuePreview;
      const target = targetQueuePreview
        ? findTarget(state, targetQueuePreview.groups?.target ?? '')
        : undefined;
      const input = await readBody<QueueActionInput>(req);
      const item = findMockQueueItem(state.queue, match?.groups?.item ?? '', target?.value.id);
      respond(res, 200, previewMockQueueAction(state, item, input));
      return;
    }
    const rootQueueAction = path.match(/^\/api\/v1\/root\/queue\/(?<item>[^/]+)\/actions$/);
    const targetQueueAction = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/queue\/(?<item>[^/]+)\/actions$/,
    );
    if ((rootQueueAction || targetQueueAction) && method === 'POST') {
      const match = rootQueueAction ?? targetQueueAction;
      const target = targetQueueAction
        ? findTarget(state, targetQueueAction.groups?.target ?? '')
        : undefined;
      const input = await readBody<QueueActionInput>(req);
      const item = applyMockQueueAction(
        state.queue,
        match?.groups?.item ?? '',
        input,
        target?.value.id,
      );
      broadcast(state, { type: 'queue.changed', target_id: item.target_id ?? '' });
      respond(res, 200, item);
      return;
    }
    if (path === route('/api/v1/root/schedule-profiles') && method === 'GET') {
      respond(res, 200, {
        profiles: structuredClone(
          state.scheduleProfiles.filter(
            (profile) => parsed.searchParams.get('archived') === 'true' || !profile.archived_at,
          ),
        ),
      });
      return;
    }
    if (path === route('/api/v1/root/schedule-profiles') && method === 'POST') {
      const input = await readBody<ScheduleProfileInput>(req);
      const profile = saveMockScheduleProfile(state, input);
      broadcast(state, { type: 'queue.changed' });
      respond(res, 201, profile);
      return;
    }
    const rootScheduleProfile = path.match(
      /^\/api\/v1\/root\/schedule-profiles\/(?<profile>[^/]+)$/,
    );
    if (rootScheduleProfile && method === 'PUT') {
      const input = await readBody<ScheduleProfileInput>(req);
      const profile = saveMockScheduleProfile(
        state,
        input,
        rootScheduleProfile.groups?.profile ?? '',
      );
      broadcast(state, { type: 'queue.changed' });
      respond(res, 200, profile);
      return;
    }
    if (rootScheduleProfile && method === 'DELETE') {
      const profile = findMockScheduleProfile(state, rootScheduleProfile.groups?.profile ?? '');
      const expected = Number(parsed.searchParams.get('expected_revision'));
      if (expected !== profile.revision) {
        throw new MockApiError(409, 'conflict', 'schedule profile changed; reload and try again');
      }
      if (profile.system) {
        throw new MockApiError(
          409,
          'profile_in_use',
          'system schedule profiles cannot be archived',
        );
      }
      const archived = {
        ...profile,
        archived_at: new Date().toISOString(),
        revision: profile.revision + 1,
      };
      state.scheduleProfiles = state.scheduleProfiles.map((candidate) =>
        candidate.id === archived.id ? archived : candidate,
      );
      respond(res, 200, structuredClone(archived));
      return;
    }
    if (path === route('/api/v1/root/job-policies') && method === 'GET') {
      const current = globalSchedulePolicies(state);
      const overrides = state.schedulePolicies.filter((policy) => policy.target_id !== undefined);
      const policySet: RootJobPolicies['policy_set'] = {
        current: structuredClone(current),
        deployment_defaults: structuredClone(state.scheduleDefaults),
        overrides: structuredClone(overrides),
        effective: structuredClone(current),
      };
      const document: RootJobPolicies = {
        policies: structuredClone(current),
        policy_set: policySet,
        statuses: mockPolicyStatuses(state),
      };
      respond(res, 200, document);
      return;
    }
    const rootJobPolicy = path.match(/^\/api\/v1\/root\/job-policies\/(?<kind>[^/]+)$/);
    if (rootJobPolicy && method === 'PUT') {
      const input = await readBody<QueuePolicyInput>(req);
      const policy = saveMockQueuePolicy(
        state,
        decodeURIComponent(rootJobPolicy.groups?.kind ?? '') as QueueWorkload,
        input,
      );
      broadcast(state, { type: 'queue.changed' });
      respond(res, 200, policy);
      return;
    }
    const rootWorkspacePolicy = path.match(
      /^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/job-policies\/(?<kind>[^/]+)$/,
    );
    if (rootWorkspacePolicy && method === 'PUT') {
      const target = findTarget(state, rootWorkspacePolicy.groups?.target ?? '');
      const input = await readBody<QueuePolicyInput>(req);
      const policy = saveMockQueuePolicy(
        state,
        decodeURIComponent(rootWorkspacePolicy.groups?.kind ?? '') as QueueWorkload,
        input,
        target.value.id,
      );
      broadcast(state, { type: 'queue.changed', target_id: target.value.id });
      respond(res, 200, policy);
      return;
    }
    if (rootWorkspacePolicy && method === 'DELETE') {
      const target = findTarget(state, rootWorkspacePolicy.groups?.target ?? '');
      const kind = decodeURIComponent(rootWorkspacePolicy.groups?.kind ?? '') as QueueWorkload;
      const override = state.schedulePolicies.find(
        (policy) => policy.kind === kind && policy.target_id === target.value.id,
      );
      if (override === undefined)
        throw new MockApiError(404, 'not_found', 'workspace schedule override not found');
      if (Number(parsed.searchParams.get('expected_revision')) !== override.revision) {
        throw new MockApiError(409, 'conflict', 'schedule policy changed; reload and try again');
      }
      state.schedulePolicies = state.schedulePolicies.filter((policy) => policy !== override);
      const inherited = globalSchedulePolicies(state).find((policy) => policy.kind === kind);
      if (inherited === undefined)
        throw new MockApiError(404, 'not_found', 'deployment schedule policy not found');
      broadcast(state, { type: 'queue.changed', target_id: target.value.id });
      respond(res, 200, structuredClone(inherited));
      return;
    }
    if (path === route('/api/v1/root/schedule-requests') && method === 'GET') {
      respond(res, 200, { requests: structuredClone(state.scheduleRequests) });
      return;
    }
    const rootScheduleDecision = path.match(
      /^\/api\/v1\/root\/schedule-requests\/(?<request>[^/]+)\/decision$/,
    );
    if (rootScheduleDecision && method === 'POST') {
      const input = await readBody<{
        approve: boolean;
        promote_profile: boolean;
        reason: string;
        expected_revision: number;
      }>(req);
      const request = findMockScheduleRequest(state, rootScheduleDecision.groups?.request ?? '');
      if (request.revision !== input.expected_revision) {
        throw new MockApiError(409, 'conflict', 'schedule request changed; reload and try again');
      }
      const saved: ScheduleRequest = {
        ...request,
        state: input.approve ? 'approved' : 'rejected',
        revision: request.revision + 1,
        updated_at: new Date().toISOString(),
      };
      state.scheduleRequests = state.scheduleRequests.map((candidate) =>
        candidate.id === saved.id ? saved : candidate,
      );
      if (input.approve) {
        let profileID = request.profile_id;
        if (request.custom_profile !== undefined) {
          const promoted = saveMockScheduleProfile(state, {
            name: request.custom_profile.name,
            timezone: request.custom_profile.timezone,
            windows: request.custom_profile.windows,
            exceptions: request.custom_profile.exceptions,
            expected_revision: 0,
          });
          profileID = promoted.id;
        }
        const effective = targetSchedulePolicies(state, request.target_id).effective.find(
          (policy) => policy.kind === request.kind,
        );
        if (effective !== undefined && profileID !== undefined) {
          saveMockQueuePolicy(
            state,
            request.kind,
            {
              enabled: effective.enabled,
              cadence_seconds: request.cadence / NANOSECONDS_PER_SECOND,
              profile_id: profileID,
              default_priority: request.default_priority,
              retry_delay_seconds: effective.retry_delay / NANOSECONDS_PER_SECOND,
              ...(effective.retention === undefined
                ? {}
                : { retention_seconds: effective.retention / NANOSECONDS_PER_SECOND }),
              ...(effective.approval_ttl === undefined
                ? {}
                : {
                    approval_lifetime_seconds: effective.approval_ttl / NANOSECONDS_PER_SECOND,
                  }),
              configuration: request.configuration,
              expected_revision: effective.revision,
            },
            request.target_id,
          );
        }
      }
      broadcast(state, { type: 'queue.changed', target_id: saved.target_id });
      respond(res, 200, structuredClone(saved));
      return;
    }
    const targetSchedules = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/schedules$/);
    if (targetSchedules && method === 'GET') {
      const target = findTarget(state, targetSchedules.groups?.target ?? '');
      const document: TargetSchedules = {
        policies: targetSchedulePolicies(state, target.value.id),
        profiles: structuredClone(
          state.scheduleProfiles.filter(
            (profile) => profile.target_id === undefined || profile.target_id === target.value.id,
          ),
        ),
        statuses: mockPolicyStatuses(state, target.value.id),
      };
      respond(res, 200, document);
      return;
    }
    const targetScheduleRequests = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/schedule-requests$/,
    );
    if (targetScheduleRequests && method === 'GET') {
      const target = findTarget(state, targetScheduleRequests.groups?.target ?? '');
      respond(res, 200, {
        requests: structuredClone(
          state.scheduleRequests.filter((request) => request.target_id === target.value.id),
        ),
      });
      return;
    }
    if (targetScheduleRequests && method === 'POST') {
      const target = findTarget(state, targetScheduleRequests.groups?.target ?? '');
      const input = await readBody<ScheduleRequestInput>(req);
      const effective = targetSchedulePolicies(state, target.value.id).effective.find(
        (policy) => policy.kind === input.kind,
      );
      if (effective === undefined)
        throw new MockApiError(404, 'not_found', 'schedule policy not found');
      if (input.base_revision !== effective.revision) {
        throw new MockApiError(409, 'conflict', 'schedule policy changed; reload and try again');
      }
      const now = new Date().toISOString();
      const saved: ScheduleRequest = {
        id: `request:mock-${state.scheduleCounter++}`,
        target_id: target.value.id,
        kind: input.kind,
        state: 'pending',
        base_revision: input.base_revision,
        ...(effective.target_id === undefined ? {} : { base_target_id: effective.target_id }),
        ...(input.profile_id === undefined ? {} : { profile_id: input.profile_id }),
        ...(input.custom_profile === undefined
          ? {}
          : { custom_profile: structuredClone(input.custom_profile) }),
        cadence: input.cadence_seconds * NANOSECONDS_PER_SECOND,
        default_priority: input.default_priority,
        configuration: structuredClone(input.configuration),
        reason: input.reason,
        requested_by: VIEWER.id,
        requester: VIEWER,
        revision: 1,
        created_at: now,
        updated_at: now,
      };
      state.scheduleRequests = [saved, ...state.scheduleRequests];
      broadcast(state, { type: 'queue.changed', target_id: target.value.id });
      respond(res, 201, structuredClone(saved));
      return;
    }
    const targetScheduleRequest = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/schedule-requests\/(?<request>[^/]+)$/,
    );
    if (targetScheduleRequest && method === 'DELETE') {
      const target = findTarget(state, targetScheduleRequest.groups?.target ?? '');
      const request = findMockScheduleRequest(state, targetScheduleRequest.groups?.request ?? '');
      if (request.target_id !== target.value.id)
        throw new MockApiError(404, 'not_found', 'schedule request not found');
      if (Number(parsed.searchParams.get('expected_revision')) !== request.revision) {
        throw new MockApiError(409, 'conflict', 'schedule request changed; reload and try again');
      }
      const withdrawn: ScheduleRequest = {
        ...request,
        state: 'withdrawn',
        revision: request.revision + 1,
        updated_at: new Date().toISOString(),
      };
      state.scheduleRequests = state.scheduleRequests.map((candidate) =>
        candidate.id === withdrawn.id ? withdrawn : candidate,
      );
      broadcast(state, { type: 'queue.changed', target_id: target.value.id });
      respond(res, 200, structuredClone(withdrawn));
      return;
    }
    if (path === route('/api/v1/root/runtime/settings') && method === 'GET') {
      respond(res, 200, rootRuntimeSettingsValue(state));
      return;
    }
    if (path === route('/api/v1/root/runtime/settings') && method === 'PUT') {
      const input = await readBody<RootRuntimeSettingsInput>(req);
      respond(res, 200, saveMockRootRuntimeSettings(state, input));
      return;
    }
    const rootRuntimeCheckpoint = path.match(
      /^\/api\/v1\/root\/runtime\/settings\/checkpoints\/(?<checkpoint>[^/]+)(?<restore>\/restore)?$/,
    );
    if (
      rootRuntimeCheckpoint &&
      method === 'GET' &&
      rootRuntimeCheckpoint.groups?.restore === undefined
    ) {
      const checkpoint = findMockRootRuntimeCheckpoint(
        state,
        rootRuntimeCheckpoint.groups?.checkpoint ?? '',
      );
      respond(res, 200, inspectMockRootRuntimeCheckpoint(state, checkpoint));
      return;
    }
    if (rootRuntimeCheckpoint?.groups?.restore !== undefined && method === 'POST') {
      const checkpoint = findMockRootRuntimeCheckpoint(
        state,
        rootRuntimeCheckpoint.groups?.checkpoint ?? '',
      );
      const input = await readBody<SettingsRestoreInput>(req);
      respond(res, 200, restoreMockRootRuntimeSettings(state, checkpoint, input));
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
      /^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/settings$/,
    );
    const targetSettingsCheckpoint = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/settings\/checkpoints\/(?<checkpoint>[^/]+)(?<restore>\/restore)?$/,
    );
    const rootTargetSettingsCheckpoint = path.match(
      /^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/settings\/checkpoints\/(?<checkpoint>[^/]+)(?<restore>\/restore)?$/,
    );
    const rootElevation = path.match(/^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/elevation$/);
    const rootElevationEnd = path.match(/^\/api\/v1\/root\/elevations\/(?<elevation>[^/]+)$/);
    const notificationRead = path.match(/^\/api\/v1\/notifications\/(?<notification>[^/]+)\/read$/);
    const scopedUsers = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/users$/);
    const rootScopedUsers = path.match(/^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/users$/);
    const userSuggestions = path.match(
      /^\/api\/v1\/(?:targets|root\/workspaces)\/(?<target>[^/]+)\/user-suggestions$/,
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
      /^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/users\/(?<account>[^/]+)$/,
    );
    const scopedUserDecisions = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/users\/(?<account>[^/]+)\/decisions$/,
    );
    const rootScopedUserDecisions = path.match(
      /^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/users\/(?<account>[^/]+)\/decisions$/,
    );
    const scopedInvitations = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/invitations$/);
    const rootScopedInvitations = path.match(
      /^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/invitations$/,
    );
    const reissueInvitation = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/invitations\/(?<invitation>[^/]+)\/reissue$/,
    );
    const rootScopedInvitationReissue = path.match(
      /^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/invitations\/(?<invitation>[^/]+)\/reissue$/,
    );
    const invitation = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/invitations\/(?<invitation>[^/]+)$/,
    );
    const rootScopedInvitation = path.match(
      /^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/invitations\/(?<invitation>[^/]+)$/,
    );
    const repositories = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/repositories$/);
    const rootRepositories = path.match(
      /^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/repositories$/,
    );
    const repository = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/repositories\/(?<repository>[^/]+)$/,
    );
    const rootRepository = path.match(
      /^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/repositories\/(?<repository>[^/]+)$/,
    );
    const repositoryConfigMigration = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/repositories\/(?<repository>[^/]+)\/config-migration$/,
    );
    const rootRepositoryConfigMigration = path.match(
      /^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/repositories\/(?<repository>[^/]+)\/config-migration$/,
    );
    const audit = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/audit$/);
    const auditExport = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/audit\.csv$/);
    const failures = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/failures$/);
    const rootTargetAudit = path.match(/^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/audit$/);
    const rootTargetFailures = path.match(
      /^\/api\/v1\/root\/workspaces\/(?<target>[^/]+)\/failures$/,
    );
    const rootHistory = path.match(/^\/api\/v1\/root\/history\/(?<history>audit|failures)$/);
    const rootAuditExport = path === '/api/v1/root/history/audit.csv';
    const rootPendingCICheck = path.match(
      /^\/api\/v1\/root\/pending-ci\/(?<request>[^/]+)\/check$/,
    );
    const rootPendingCI = path.match(/^\/api\/v1\/root\/pending-ci\/(?<request>[^/]+)$/);
    const workspaceUsers = scopedUsers ?? rootScopedUsers;
    const workspaceUser = scopedUser ?? rootScopedUser;
    const workspaceUserDecisions = scopedUserDecisions ?? rootScopedUserDecisions;
    const workspaceInvitations = scopedInvitations ?? rootScopedInvitations;
    const workspaceInvitationReissue = reissueInvitation ?? rootScopedInvitationReissue;
    const workspaceInvitation = invitation ?? rootScopedInvitation;
    const workspaceAudit = audit ?? rootTargetAudit;
    const workspaceFailures = failures ?? rootTargetFailures;
    const workspaceSettings = targetSettings ?? rootTargetSettings;
    const workspaceSettingsCheckpoint = targetSettingsCheckpoint ?? rootTargetSettingsCheckpoint;

    if (
      workspaceSettingsCheckpoint !== null &&
      method === 'GET' &&
      workspaceSettingsCheckpoint.groups?.restore === undefined
    ) {
      const target = findTarget(state, workspaceSettingsCheckpoint.groups?.target ?? '');
      const checkpoint = findMockWorkspaceSettingsCheckpoint(
        state,
        target.value.id,
        workspaceSettingsCheckpoint.groups?.checkpoint ?? '',
      );
      respond(res, 200, inspectMockWorkspaceSettingsCheckpoint(state, target, checkpoint));
      return;
    }

    if (workspaceSettingsCheckpoint?.groups?.restore !== undefined && method === 'POST') {
      const target = findTarget(state, workspaceSettingsCheckpoint.groups?.target ?? '');
      if (rootTargetSettingsCheckpoint !== null) requireRootWrite(state, target);
      const checkpoint = findMockWorkspaceSettingsCheckpoint(
        state,
        target.value.id,
        workspaceSettingsCheckpoint.groups?.checkpoint ?? '',
      );
      const input = await readBody<SettingsRestoreInput>(req);
      respond(res, 200, restoreMockWorkspaceSettings(state, target, checkpoint, input));
      return;
    }

    if (workspaceSettings && method === 'PUT') {
      const target = findTarget(state, workspaceSettings.groups?.target ?? '');
      if (rootTargetSettings !== null) requireRootWrite(state, target);
      const input = await readBody<WorkspaceSettingsBatchInput>(req);
      respond(res, 200, saveMockWorkspaceSettings(state, target, input));
      return;
    }

    if (rootPendingCICheck && method === 'POST') {
      const id = rootPendingCICheck.groups?.request ?? '';
      const request = findPendingCI(state, id);
      const input = await readBody<{ expected_revision: number }>(req);
      requirePendingCIRevision(request, input.expected_revision);
      const now = new Date().toISOString();
      request.schedule = 'active';
      request.next_check_at = now;
      request.next_check_trigger = 'manual';
      request.updated_at = now;
      request.revision += 1;
      /* "Check now" means now. The deadline above is what the service moves; the look that follows
         it is what the reader pressed the button for, and waiting up to a second for the next tick
         to notice made the button look like it had done nothing. Reconciling here also broadcasts,
         so the row that answers is the row the check produced rather than the one before it. */
      reconcile(state);
      respond(res, 200, structuredClone(findPendingCI(state, id)));
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

    /* The same records the audit page shows, written as the service writes them:
       a header, then one row each, filtered the way the page is filtered. */
    if (rootAuditExport && method === 'GET') {
      const categories = parsed.searchParams.getAll('category').filter((value) => value !== 'all');
      const rows = rootAuditEntries(state).filter(
        (entry) => categories.length === 0 || categories.includes(entry.category ?? ''),
      );
      const csv = [
        'when,actor,workspace,subject,category,action,summary,elevation',
        ...rows.map((entry) =>
          [
            entry.created_at,
            entry.actor.login,
            entry.workspace?.login ?? '',
            entry.subject?.login ?? '',
            entry.category ?? '',
            entry.action,
            `"${entry.summary.replaceAll('"', '""')}"`,
            String(entry.elevation_id !== undefined),
          ].join(','),
        ),
      ].join('\n');
      res.statusCode = 200;
      res.setHeader('Content-Type', 'text/csv; charset=utf-8');
      res.setHeader('Content-Disposition', 'attachment; filename="smyklot-audit-dev.csv"');
      res.end(`${csv}\n`);
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
                return (entry.workspace?.display_name ?? 'Smyklot').toLocaleLowerCase();
              }
              if (sort.startsWith('change_')) return entry.summary.toLocaleLowerCase();
              return entry.created_at;
            },
            (entry, query) =>
              [
                entry.category ?? '',
                entry.workspace?.display_name ?? 'Smyklot',
                entry.workspace?.login ?? '',
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
            workspace: target.value.account,
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
                item.workspace.display_name,
                item.workspace.login,
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

    if (path === route('/api/v1/notifications/read') && method === 'PUT') {
      let cleared = 0;
      state.notifications = state.notifications.map((notification) => {
        if (notification.read_at !== undefined) return notification;
        cleared += 1;
        return { ...notification, read_at: new Date().toISOString() };
      });
      respond(res, 200, { read: cleared });
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
        throw new MockApiError(404, 'not_found', 'no operator visit to this workspace was found');
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
        throw new MockApiError(409, 'conflict', 'you already own this workspace');
      if (!rootWorkspaceValue(target).available)
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
        throw new MockApiError(404, 'not_found', 'no operator visit to this workspace was found');
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
    if (rootRepositoryConfigMigration && method === 'POST') {
      const target = findTarget(state, rootRepositoryConfigMigration.groups?.target ?? '');
      requireRootWrite(state, target);
      const stored = findRepository(target, rootRepositoryConfigMigration.groups?.repository ?? '');
      respond(res, 200, resetMockConfigMigration(state, target, stored));
      return;
    }

    if (workspaceUserDecisions && method === 'GET') {
      const target = findTarget(state, workspaceUserDecisions.groups?.target ?? '');
      const accountId = decodeURIComponent(workspaceUserDecisions.groups?.account ?? '');
      const user = targetUsers(state, target.value.id).find(
        (entry) => entry.account.id === accountId,
      );
      if (user === undefined) throw new MockApiError(404, 'not_found', 'panel user not found');
      respond(res, 200, { decisions: mockDecisions(user, target.value) });
      return;
    }

    if (workspaceInvitations && method === 'GET') {
      const target = findTarget(state, workspaceInvitations.groups?.target ?? '');
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
    if (workspaceInvitations && method === 'POST') {
      const target = findTarget(state, workspaceInvitations.groups?.target ?? '');
      if (rootScopedInvitations !== null) requireRootWrite(state, target);
      const input = await readBody<AddTargetInvitationInput>(req);
      const created = createMockInvitation(state, input, target.value);
      broadcast(state, { type: 'invitation.changed', target_id: target.value.id });
      saveDevState(state);
      respond(res, 201, invitationValue(created));
      return;
    }
    if (workspaceInvitationReissue && method === 'POST') {
      const target = findTarget(state, workspaceInvitationReissue.groups?.target ?? '');
      if (rootScopedInvitationReissue !== null) requireRootWrite(state, target);
      const current = findInvitation(state, workspaceInvitationReissue.groups?.invitation ?? '');
      if (current.target_id !== target.value.id) {
        throw new MockApiError(404, 'not_found', 'workspace invitation not found');
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
    if (workspaceInvitation && method === 'DELETE') {
      const target = findTarget(state, workspaceInvitation.groups?.target ?? '');
      if (rootScopedInvitation !== null) requireRootWrite(state, target);
      const current = findInvitation(state, workspaceInvitation.groups?.invitation ?? '');
      if (current.target_id !== target.value.id) {
        throw new MockApiError(404, 'not_found', 'workspace invitation not found');
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
         People already on the workspace are dropped, the same as the service
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

    if (workspaceUsers && method === 'GET') {
      const target = findTarget(state, workspaceUsers.groups?.target ?? '');
      respond(res, 200, userPage(targetUsers(state, target.value.id), parsed.searchParams));
      return;
    }
    if (workspaceUsers && method === 'POST') {
      const target = findTarget(state, workspaceUsers.groups?.target ?? '');
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
        throw new MockApiError(409, 'conflict', 'this user already has access to this workspace');
      }
      access.set(user.account.id, targetAccess(input.role, false, 1));
      broadcast(state, { type: 'access.changed', target_id: target.value.id });
      respond(res, 201, scopedUserValue(state, target.value.id, user));
      return;
    }
    if (workspaceUser && method === 'PUT') {
      const target = findTarget(state, workspaceUser.groups?.target ?? '');
      if (rootScopedUser !== null) requireRootWrite(state, target);
      const user = findUser(state, workspaceUser.groups?.account ?? '');
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
    if (repositoryConfigMigration && method === 'POST') {
      const target = findTarget(state, repositoryConfigMigration.groups?.target ?? '');
      const stored = findRepository(target, repositoryConfigMigration.groups?.repository ?? '');
      respond(res, 200, resetMockConfigMigration(state, target, stored));
      return;
    }
    if (auditExport && method === 'GET') {
      const target = findTarget(state, auditExport.groups?.target ?? '');
      const csv = [
        'when,actor,repository,action,summary',
        ...target.audit.map((entry) =>
          [
            entry.created_at,
            entry.actor.login,
            entry.repository_full_name ?? '',
            entry.action,
            `"${entry.summary.replaceAll('"', '""')}"`,
          ].join(','),
        ),
      ].join('\n');
      res.statusCode = 200;
      res.setHeader('Content-Type', 'text/csv; charset=utf-8');
      res.setHeader('Content-Disposition', 'attachment; filename="smyklot-audit-dev.csv"');
      res.end(`${csv}\n`);
      return;
    }

    if (workspaceAudit && method === 'GET') {
      const target = findTarget(state, workspaceAudit.groups?.target ?? '');
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
              (change === 'repository' &&
                (entry.action.startsWith('repository.') ||
                  mockAuditCheckpointChangedKind(state, target.value.id, entry, ['repository']))) ||
              (change === 'account' &&
                (entry.action.startsWith('target.') ||
                  mockAuditCheckpointChangedKind(state, target.value.id, entry, ['target']))) ||
              (change === 'sync' &&
                mockAuditCheckpointChangedKind(state, target.value.id, entry, [
                  'sync_config',
                  'sync_override',
                ]));
            return matchesScope && matchesChange;
          },
        ),
      );
      return;
    }
    if (workspaceFailures && method === 'GET') {
      const target = findTarget(state, workspaceFailures.groups?.target ?? '');
      const kind = parsed.searchParams.get('kind') ?? 'all';
      /* Only failures take a window, because only the failures endpoint does.
         A mock that accepted `since` everywhere would be more permissive than
         the service, which is how a fixture starts lying about the wire. */
      const since = Date.parse(parsed.searchParams.get('since') ?? '');
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
            (kind === 'all' ||
              (kind === 'retryable' && failure.retryable) ||
              (kind === 'permanent' && !failure.retryable)) &&
            (Number.isNaN(since) || Date.parse(failure.occurred_at) >= since),
        ),
      );
      return;
    }
  } catch (error) {
    if (error instanceof MockApiError) {
      respond(res, error.status, {
        error: { code: error.code, message: error.message, ...error.details },
      });
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
     later request in the same process saw an account with no workspaces, and
     whatever was being looked at next quietly measured the wrong panel. */
  state.hideTargets = scenario === 'empty';
}

function findTarget(state: MockState, encodedId: string): MockTarget {
  const id = decodeURIComponent(encodedId);
  const target = state.targets.find((entry) => entry.value.id === id);
  if (target === undefined) throw new MockApiError(404, 'not_found', 'workspace not found');
  return target;
}

function changedMockSyncConfig(
  config: SyncConfig,
  input: WorkspaceSyncConfigSettingsInput,
): SyncConfig {
  const now = new Date().toISOString();
  const nextRevision = config.revision + 1;
  const labels = input.kind === 'labels';
  return {
    ...config,
    enabled: input.enabled,
    labels: labels ? structuredClone(input.labels) : config.labels,
    allow_removal: labels ? input.allow_removal : config.allow_removal,
    excludes: labels ? structuredClone(input.excludes) : config.excludes,
    document: labels ? config.document : structuredClone(input.document),
    revision: nextRevision,
    updated_by: VIEWER.login,
    updated_at: now,
    digest: `sha256:mock-${config.kind}-${nextRevision}-${Date.now()}`,
  };
}

interface MockWorkspaceSettingsPlan {
  target?: MockPreparedChange<PanelTarget>;
  repositories: Array<MockPreparedChange<RepositoryDetail> & { stored: MockRepository }>;
  syncConfigs: Array<MockPreparedChange<SyncConfig> & { key: string }>;
  syncOverrides: Array<
    MockPreparedChange<SyncOverride> & { key: string; repository: MockRepository }
  >;
}

interface MockPreparedChange<T> {
  before: T | null;
  changed: boolean;
  conflict: WorkspaceSettingsConflict | null;
  next: T;
}

function saveMockWorkspaceSettings(
  state: MockState,
  target: MockTarget,
  input: WorkspaceSettingsBatchInput,
): WorkspaceSettingsBatchResponse {
  validateMockWorkspaceSettingsBatch(input);
  const plan = prepareMockWorkspaceSettings(state, target, input);
  const conflicts = [
    ...(plan.target?.conflict === null || plan.target?.conflict === undefined
      ? []
      : [plan.target.conflict]),
    ...plan.repositories.flatMap(({ conflict }) => (conflict === null ? [] : [conflict])),
    ...plan.syncConfigs.flatMap(({ conflict }) => (conflict === null ? [] : [conflict])),
    ...plan.syncOverrides.flatMap(({ conflict }) => (conflict === null ? [] : [conflict])),
  ].sort(compareMockSettingsConflicts);
  if (conflicts.length > 0) {
    throw new MockApiError(
      409,
      'settings_conflict',
      'settings changed in another session; review the latest values',
      { conflicts },
    );
  }

  const changed =
    plan.target?.changed === true ||
    plan.repositories.some(({ changed }) => changed) ||
    plan.syncConfigs.some(({ changed }) => changed) ||
    plan.syncOverrides.some(({ changed }) => changed);
  const before = changed ? mockWorkspaceSettingsSnapshot(state, target) : null;
  if (changed) ensureMockWorkspaceSettingsBaseline(state, target);
  applyMockWorkspaceSettingsPlan(state, target, plan);
  const checkpoint =
    before === null
      ? null
      : createMockWorkspaceSettingsCheckpoint(
          state,
          target,
          'installation.settings.saved',
          before,
          mockWorkspaceSettingsSnapshot(state, target),
        );
  const response = mockWorkspaceSettingsResponse(state, target, plan);
  if (checkpoint !== null) {
    response.checkpoint_id = checkpoint.id;
    addAudit(
      target,
      checkpoint.action,
      mockWorkspaceSettingsAuditSummary(
        'Saved',
        checkpoint.items.filter(({ changed }) => changed).length,
      ),
      undefined,
      checkpoint.id,
    );
    broadcast(state, { type: 'target.changed', target_id: target.value.id });
  }
  return response;
}

function applyMockWorkspaceSettingsPlan(
  state: MockState,
  target: MockTarget,
  plan: MockWorkspaceSettingsPlan,
): void {
  if (plan.target?.changed === true) target.value = plan.target.next;
  for (const change of plan.repositories) {
    if (change.changed) change.stored.detail = change.next;
  }
  for (const change of plan.syncConfigs) {
    if (change.changed) state.sync.set(change.key, change.next);
  }
  for (const change of plan.syncOverrides) {
    if (change.changed) state.syncOverrides.set(change.key, change.next);
  }
  if (plan.target?.changed === true || plan.repositories.some(({ changed }) => changed)) {
    recomputeTarget(target);
  }
}

function mockWorkspaceSettingsResponse(
  state: MockState,
  target: MockTarget,
  plan: MockWorkspaceSettingsPlan,
): WorkspaceSettingsBatchResponse {
  const response: WorkspaceSettingsBatchResponse = {};
  if (plan.target !== undefined) response.target = mockWorkspaceTargetState(target.value);
  if (plan.repositories.length > 0) {
    response.repositories = plan.repositories
      .map(({ stored }) => mockWorkspaceRepositoryState(stored.detail))
      .sort((left, right) => left.repository_id.localeCompare(right.repository_id));
  }
  if (plan.syncConfigs.length > 0) {
    response.sync_configs = plan.syncConfigs
      .map(({ key, next }) =>
        mockWorkspaceSyncConfigState(target.value.id, state.sync.get(key) ?? next),
      )
      .sort((left, right) => left.kind.localeCompare(right.kind));
  }
  if (plan.syncOverrides.length > 0) {
    response.sync_overrides = plan.syncOverrides
      .map(({ key, next, repository }) =>
        mockWorkspaceSyncOverrideState(
          target.value.id,
          repository.detail.repository.id,
          state.syncOverrides.get(key) ?? next,
        ),
      )
      .sort(
        (left, right) =>
          left.repository_id.localeCompare(right.repository_id) ||
          left.kind.localeCompare(right.kind),
      );
  }
  return response;
}

function prepareMockWorkspaceSettings(
  state: MockState,
  target: MockTarget,
  input: WorkspaceSettingsBatchInput,
): MockWorkspaceSettingsPlan {
  return {
    target:
      input.target === undefined ? undefined : prepareMockTargetSettings(target, input.target),
    repositories: (input.repositories ?? []).map((change) =>
      prepareMockRepositorySettings(target, change),
    ),
    syncConfigs: (input.sync_configs ?? []).map((change) =>
      prepareMockSyncConfigSettings(state, target.value.id, change),
    ),
    syncOverrides: (input.sync_overrides ?? []).map((change) =>
      prepareMockSyncOverrideSettings(state, target, change),
    ),
  };
}

function prepareMockTargetSettings(
  target: MockTarget,
  input: WorkspaceTargetSettingsInput,
): MockPreparedChange<PanelTarget> {
  const current = mockWorkspaceTargetDocument(target.value);
  const proposed = {
    repository_default_enabled: input.repository_default_enabled,
    pending_ci_mode_default: input.pending_ci_mode_default,
    pending_ci_branch_patterns_default: structuredClone(input.pending_ci_branch_patterns_default),
    pending_ci_quiet_period_seconds_override: input.pending_ci_quiet_period_seconds_override,
    path_index_interval_seconds_override: input.path_index_interval_seconds_override,
    config_patch: structuredClone(input.config_patch),
  };
  const next = structuredClone(target.value);
  const changed = !sameMockDocument(current, proposed);
  if (changed) Object.assign(next, proposed, { revision: next.revision + 1 });
  return {
    before: structuredClone(target.value),
    changed,
    next,
    conflict:
      target.value.revision === input.expected_revision
        ? null
        : {
            resource: 'target',
            target_id: target.value.id,
            expected_revision: input.expected_revision,
            actual_revision: target.value.revision,
            latest: mockWorkspaceTargetState(target.value),
          },
  };
}

function prepareMockRepositorySettings(
  target: MockTarget,
  input: WorkspaceRepositorySettingsInput,
): MockPreparedChange<RepositoryDetail> & { stored: MockRepository } {
  const stored = findRepository(target, input.repository_id);
  const proposed = {
    enabled_override: input.enabled_override,
    pending_ci_mode_override: input.pending_ci_mode_override,
    pending_ci_branch_patterns_override: structuredClone(input.pending_ci_branch_patterns_override),
    pending_ci_quiet_period_seconds_override: input.pending_ci_quiet_period_seconds_override,
    path_index_interval_seconds_override: input.path_index_interval_seconds_override,
    config_patch: structuredClone(input.config_patch),
    ignore_repository_file: input.ignore_repository_file,
  };
  const next = structuredClone(stored.detail);
  const changed = !sameMockDocument(mockWorkspaceRepositoryDocument(stored.detail), proposed);
  if (changed) {
    next.repository.enabled_override = proposed.enabled_override;
    next.pending_ci_mode_override = proposed.pending_ci_mode_override;
    next.pending_ci_branch_patterns_override = proposed.pending_ci_branch_patterns_override;
    next.pending_ci_quiet_period_seconds_override =
      proposed.pending_ci_quiet_period_seconds_override;
    next.path_index_interval_seconds_override = proposed.path_index_interval_seconds_override;
    next.config_patch = proposed.config_patch;
    next.ignore_repository_file = proposed.ignore_repository_file;
    next.revision += 1;
    next.repository.updated_at = new Date().toISOString();
  }
  return {
    before: structuredClone(stored.detail),
    changed,
    stored,
    next,
    conflict:
      stored.detail.revision === input.expected_revision
        ? null
        : {
            resource: 'repository',
            target_id: target.value.id,
            repository_id: stored.detail.repository.id,
            expected_revision: input.expected_revision,
            actual_revision: stored.detail.revision,
            latest: mockWorkspaceRepositoryState(stored.detail),
          },
  };
}

function prepareMockSyncConfigSettings(
  state: MockState,
  targetId: string,
  input: WorkspaceSyncConfigSettingsInput,
): MockPreparedChange<SyncConfig> & { key: string } {
  const key = `${targetId}/${input.kind}`;
  const stored = state.sync.get(key);
  const current = structuredClone(stored ?? emptyMockSyncConfig(input.kind));
  const proposed =
    input.kind === 'labels'
      ? {
          labels: structuredClone(input.labels),
          allow_removal: input.allow_removal,
          excludes: structuredClone(input.excludes),
        }
      : structuredClone(input.document);
  const changed =
    current.enabled !== input.enabled ||
    !sameMockDocument(mockSyncConfigDocument(current), proposed);
  return {
    before: stored === undefined ? null : structuredClone(stored),
    changed,
    key,
    next: changed ? changedMockSyncConfig(current, input) : current,
    conflict:
      current.revision === input.expected_revision
        ? null
        : {
            resource: 'sync_config',
            target_id: targetId,
            kind: input.kind,
            expected_revision: input.expected_revision,
            actual_revision: current.revision,
            latest: mockWorkspaceSyncConfigState(targetId, current),
          },
  };
}

function prepareMockSyncOverrideSettings(
  state: MockState,
  target: MockTarget,
  input: WorkspaceSyncOverrideSettingsInput,
): MockPreparedChange<SyncOverride> & { key: string; repository: MockRepository } {
  const repository = findRepository(target, input.repository_id);
  const key = `${input.repository_id}/${input.kind}`;
  const stored = state.syncOverrides.get(key);
  const current = structuredClone(stored ?? emptyMockSyncOverride(input.kind));
  const changed =
    current.enabled !== input.enabled || !sameMockDocument(current.document, input.document);
  const next: SyncOverride = changed
    ? {
        ...current,
        enabled: input.enabled,
        document: structuredClone(input.document),
        revision: current.revision + 1,
        updated_by: VIEWER.login,
        updated_at: new Date().toISOString(),
      }
    : current;
  return {
    before: stored === undefined ? null : structuredClone(stored),
    changed,
    key,
    repository,
    next,
    conflict:
      current.revision === input.expected_revision
        ? null
        : {
            resource: 'sync_override',
            target_id: target.value.id,
            repository_id: repository.detail.repository.id,
            kind: input.kind,
            expected_revision: input.expected_revision,
            actual_revision: current.revision,
            latest: mockWorkspaceSyncOverrideState(
              target.value.id,
              repository.detail.repository.id,
              current,
            ),
          },
  };
}

function validateMockWorkspaceSettingsBatch(input: WorkspaceSettingsBatchInput): void {
  const repositories = mockBatchArray(input.repositories, 'repositories');
  const syncConfigs = mockBatchArray(input.sync_configs, 'Sync configurations');
  const syncOverrides = mockBatchArray(input.sync_overrides, 'repository Sync settings');
  const resources =
    repositories.length +
    syncConfigs.length +
    syncOverrides.length +
    (input.target === undefined ? 0 : 1);
  if (resources === 0) invalidMockSettingsBatch('settings save needs at least one resource');

  const keyed = [
    ...(input.target === undefined
      ? []
      : [{ key: 'target', revision: input.target.expected_revision }]),
    ...repositories.map((entry) => ({
      key: entry.repository_id === '' ? '' : `repository:${entry.repository_id}`,
      revision: entry.expected_revision,
    })),
    ...syncConfigs.map((entry) => ({
      key: SYNC_KINDS.includes(entry.kind) ? `sync:${entry.kind}` : '',
      revision: entry.expected_revision,
    })),
    ...syncOverrides.map((entry) => ({
      key:
        entry.repository_id !== '' && SYNC_KINDS.includes(entry.kind)
          ? `override:${entry.repository_id}:${entry.kind}`
          : '',
      revision: entry.expected_revision,
    })),
  ];
  const seen = new Set<string>();
  for (const { key, revision } of keyed) {
    validateMockRevision(revision);
    if (key.length === 0 || seen.has(key)) {
      invalidMockSettingsBatch('settings resources must be known and unique');
    }
    seen.add(key);
  }
}

function validateMockRevision(revision: number): void {
  if (!Number.isSafeInteger(revision) || revision < 0) {
    invalidMockSettingsBatch('settings revision must be a non-negative integer');
  }
}

function compareMockSettingsConflicts(
  left: WorkspaceSettingsConflict,
  right: WorkspaceSettingsConflict,
): number {
  const order = ['target', 'repository', 'sync_config', 'sync_override'];
  const key = (conflict: WorkspaceSettingsConflict): string =>
    [
      order.indexOf(conflict.resource),
      'repository_id' in conflict ? conflict.repository_id : '',
      'kind' in conflict ? conflict.kind : '',
    ].join('\u0000');
  return key(left).localeCompare(key(right));
}

interface MockWorkspaceSnapshotEntry {
  kind: SettingsCheckpointItem['kind'];
  repository_id?: string;
  repository_full_name?: string;
  sync_kind?: SyncKind;
  state: SettingsCheckpointState;
}

type MockWorkspaceSettingsSnapshot = Map<string, MockWorkspaceSnapshotEntry>;

function mockWorkspaceSettingsSnapshot(
  state: MockState,
  target: MockTarget,
): MockWorkspaceSettingsSnapshot {
  const snapshot: MockWorkspaceSettingsSnapshot = new Map();
  const add = (entry: MockWorkspaceSnapshotEntry): void => {
    snapshot.set(mockWorkspaceCheckpointIdentity(entry), entry);
  };
  add({
    kind: 'target',
    state: mockWorkspaceCheckpointState(
      mockWorkspaceTargetCheckpointDocument(target.value),
      target.value.revision,
    ),
  });
  for (const repository of target.repositories) {
    add({
      kind: 'repository',
      repository_id: repository.detail.repository.id,
      repository_full_name: repository.detail.repository.full_name,
      state: mockWorkspaceCheckpointState(
        mockWorkspaceRepositoryCheckpointDocument(repository.detail),
        repository.detail.revision,
      ),
    });
  }
  for (const [key, config] of state.sync) {
    if (!key.startsWith(`${target.value.id}/`)) continue;
    add({
      kind: 'sync_config',
      sync_kind: config.kind as SyncKind,
      state: mockWorkspaceCheckpointState(
        mockWorkspaceSyncConfigCheckpointDocument(config),
        config.revision,
      ),
    });
  }
  for (const repository of target.repositories) {
    for (const kind of SYNC_KINDS) {
      const repositoryId = repository.detail.repository.id;
      const override = state.syncOverrides.get(`${repositoryId}/${kind}`);
      if (override === undefined) continue;
      add({
        kind: 'sync_override',
        repository_id: repositoryId,
        repository_full_name: repository.detail.repository.full_name,
        sync_kind: override.kind as SyncKind,
        state: mockWorkspaceCheckpointState(
          mockWorkspaceSyncOverrideCheckpointDocument(override),
          override.revision,
        ),
      });
    }
  }
  return snapshot;
}

function ensureMockWorkspaceSettingsBaseline(
  state: MockState,
  target: MockTarget,
): SettingsCheckpoint {
  const existing = [...state.workspaceSettings.checkpoints.entries()].find(
    ([key, checkpoint]) =>
      key.startsWith(`${target.value.id}\u0000`) &&
      checkpoint.action === 'installation.settings.baseline',
  )?.[1];
  if (existing !== undefined) return existing;
  return createMockWorkspaceSettingsCheckpoint(
    state,
    target,
    'installation.settings.baseline',
    new Map(),
    mockWorkspaceSettingsSnapshot(state, target),
    undefined,
    undefined,
    false,
  );
}

function createMockWorkspaceSettingsCheckpoint(
  state: MockState,
  target: MockTarget,
  action:
    | 'installation.settings.baseline'
    | 'installation.settings.saved'
    | 'installation.settings.restored',
  before: MockWorkspaceSettingsSnapshot,
  after: MockWorkspaceSettingsSnapshot,
  restoredFromId?: string,
  restoredSide?: SettingsRestoreInput['state'],
  beforeCaptured = true,
): SettingsCheckpoint {
  const keys = [...new Set([...before.keys(), ...after.keys()])].sort();
  const items = keys.map((key) => {
    const earlier = before.get(key);
    const later = after.get(key);
    const identity = later ?? earlier;
    if (identity === undefined) throw new Error('empty workspace checkpoint identity');
    return mockWorkspaceCheckpointItem(
      identity,
      earlier?.state ?? null,
      later?.state ?? null,
      beforeCaptured,
      true,
    );
  });

  const id = String(state.workspaceSettings.checkpointCounter);
  state.workspaceSettings.checkpointCounter += 1;
  const checkpoint: SettingsCheckpoint = {
    id,
    action,
    actor: VIEWER,
    ...(restoredFromId === undefined ? {} : { restored_from_id: restoredFromId }),
    ...(restoredSide === undefined ? {} : { restored_side: restoredSide }),
    created_at: new Date().toISOString(),
    affected_kinds: [...new Set(items.filter(({ changed }) => changed).map(({ kind }) => kind))],
    items,
  };
  state.workspaceSettings.checkpoints.set(
    mockWorkspaceCheckpointKey(target.value.id, id),
    structuredClone(checkpoint),
  );
  return checkpoint;
}

function mockWorkspaceCheckpointItem(
  identity: Pick<
    MockWorkspaceSnapshotEntry,
    'kind' | 'repository_id' | 'repository_full_name' | 'sync_kind'
  >,
  before: SettingsCheckpointState | null,
  after: SettingsCheckpointState | null,
  beforeAvailable = true,
  afterAvailable = true,
): SettingsCheckpointItem {
  const optional = identity.kind === 'sync_config' || identity.kind === 'sync_override';
  const side = (
    available: boolean,
    value: SettingsCheckpointState | null,
    current: SettingsCheckpointState | null,
  ) => ({
    available,
    state: structuredClone(value),
    differs: available && !sameMockCheckpointState(value, current),
    restorable: available && (value !== null || optional),
  });
  return {
    ...identity,
    document_version: 1,
    before: side(beforeAvailable, before, after),
    after: side(afterAvailable, after, after),
    current: structuredClone(after),
    changed: beforeAvailable && afterAvailable && !sameMockCheckpointState(before, after),
  };
}

function mockWorkspaceSettingsAuditSummary(verb: 'Saved' | 'Restored', count: number): string {
  return `${verb} ${count} workspace ${count === 1 ? 'setting' : 'settings'}`;
}

function mockWorkspaceTargetDocument(target: PanelTarget) {
  return {
    repository_default_enabled: target.repository_default_enabled,
    pending_ci_mode_default: target.pending_ci_mode_default,
    pending_ci_branch_patterns_default: target.pending_ci_branch_patterns_default,
    pending_ci_quiet_period_seconds_override: target.pending_ci_quiet_period_seconds_override,
    path_index_interval_seconds_override: target.path_index_interval_seconds_override,
    config_patch: target.config_patch,
  };
}

function mockWorkspaceRepositoryDocument(detail: RepositoryDetail) {
  return {
    enabled_override: detail.repository.enabled_override,
    pending_ci_mode_override: detail.pending_ci_mode_override,
    pending_ci_branch_patterns_override: detail.pending_ci_branch_patterns_override,
    pending_ci_quiet_period_seconds_override: detail.pending_ci_quiet_period_seconds_override,
    path_index_interval_seconds_override: detail.path_index_interval_seconds_override,
    config_patch: detail.config_patch,
    ignore_repository_file: detail.ignore_repository_file,
  };
}

function mockWorkspaceTargetCheckpointDocument(target: PanelTarget): Record<string, unknown> {
  return {
    repository_default_enabled: target.repository_default_enabled,
    pending_ci_mode_default: target.pending_ci_mode_default,
    pending_ci_branch_patterns_default: structuredClone(target.pending_ci_branch_patterns_default),
    pending_ci_quiet_period_override: mockWorkspaceCheckpointDuration(
      target.pending_ci_quiet_period_seconds_override,
    ),
    path_index_interval_override: mockWorkspaceCheckpointDuration(
      target.path_index_interval_seconds_override,
    ),
    config_patch: structuredClone(target.config_patch),
  };
}

function mockWorkspaceRepositoryCheckpointDocument(
  detail: RepositoryDetail,
): Record<string, unknown> {
  return {
    enabled_override: detail.repository.enabled_override,
    pending_ci_mode_override: detail.pending_ci_mode_override,
    pending_ci_branch_patterns_override: structuredClone(
      detail.pending_ci_branch_patterns_override,
    ),
    pending_ci_quiet_period_override: mockWorkspaceCheckpointDuration(
      detail.pending_ci_quiet_period_seconds_override,
    ),
    path_index_interval_override: mockWorkspaceCheckpointDuration(
      detail.path_index_interval_seconds_override,
    ),
    config_patch: structuredClone(detail.config_patch),
    ignore_repository_file: detail.ignore_repository_file,
  };
}

function mockWorkspaceSyncConfigCheckpointDocument(config: SyncConfig): Record<string, unknown> {
  return {
    enabled: config.enabled,
    document: canonicalStringify(mockSyncConfigDocument(config)),
  };
}

function mockWorkspaceSyncOverrideCheckpointDocument(
  override: SyncOverride,
): Record<string, unknown> {
  return {
    enabled: override.enabled,
    document: canonicalStringify(override.document),
  };
}

function mockWorkspaceCheckpointDuration(seconds: number | null): number | null {
  return seconds === null ? null : seconds * 1_000_000_000;
}

function mockWorkspaceCheckpointState(
  document: Record<string, unknown>,
  revision: number,
): SettingsCheckpointState {
  const copy = structuredClone(document);
  return {
    document: copy,
    digest: `sha256:${createHash('sha256').update(canonicalStringify(copy)).digest('hex')}`,
    revision,
  };
}

function mockWorkspaceCheckpointIdentity(
  item: Pick<SettingsCheckpointItem, 'kind' | 'repository_id' | 'sync_kind'>,
): string {
  return [item.kind, item.repository_id ?? '', item.sync_kind ?? ''].join('\u0000');
}

function mockWorkspaceCheckpointKey(targetId: string, checkpointId: string): string {
  return `${targetId}\u0000${checkpointId}`;
}

function mockAuditCheckpointChangedKind(
  state: MockState,
  targetId: string,
  entry: AuditEntry,
  kinds: readonly SettingsCheckpointItemKind[],
): boolean {
  if (entry.settings_checkpoint_id === undefined) return false;
  const checkpoint = state.workspaceSettings.checkpoints.get(
    mockWorkspaceCheckpointKey(targetId, entry.settings_checkpoint_id),
  );
  return checkpoint?.items.some((item) => item.changed && kinds.includes(item.kind)) ?? false;
}

function findMockWorkspaceSettingsCheckpoint(
  state: MockState,
  targetId: string,
  encodedCheckpointId: string,
): SettingsCheckpoint {
  let checkpointId: string;
  try {
    checkpointId = decodeURIComponent(encodedCheckpointId);
  } catch {
    throw new MockApiError(404, 'not_found', 'settings checkpoint not found');
  }
  if (checkpointId === 'baseline') {
    const target = findTarget(state, encodeURIComponent(targetId));
    return ensureMockWorkspaceSettingsBaseline(state, target);
  }
  const checkpoint = state.workspaceSettings.checkpoints.get(
    mockWorkspaceCheckpointKey(targetId, checkpointId),
  );
  if (checkpoint === undefined) {
    throw new MockApiError(404, 'not_found', 'settings checkpoint not found');
  }
  return checkpoint;
}

function inspectMockWorkspaceSettingsCheckpoint(
  state: MockState,
  target: MockTarget,
  checkpoint: SettingsCheckpoint,
): SettingsCheckpoint {
  const inspection = structuredClone(checkpoint);
  const seen = new Set(inspection.items.map(mockWorkspaceCheckpointIdentity));
  for (const entry of mockWorkspaceSettingsSnapshot(state, target).values()) {
    if (entry.kind !== 'sync_config' && entry.kind !== 'sync_override') continue;
    if (seen.has(mockWorkspaceCheckpointIdentity(entry))) continue;
    inspection.items.push(
      mockWorkspaceCheckpointItem(
        entry,
        null,
        null,
        checkpoint.action !== 'installation.settings.baseline',
        true,
      ),
    );
  }
  inspection.items = inspection.items.map((item) => {
    const { current, incompatibility } = mockWorkspaceCheckpointCurrent(state, target, item);
    return {
      ...item,
      current: structuredClone(current),
      before: inspectMockWorkspaceCheckpointSide(item, item.before, current, incompatibility),
      after: inspectMockWorkspaceCheckpointSide(item, item.after, current, incompatibility),
    };
  });
  inspection.items.sort((left, right) =>
    mockWorkspaceCheckpointIdentity(left).localeCompare(mockWorkspaceCheckpointIdentity(right)),
  );
  return inspection;
}

function inspectMockWorkspaceCheckpointSide(
  item: SettingsCheckpointItem,
  side: SettingsCheckpointItem['before'],
  current: SettingsCheckpointState | null,
  incompatibility?: SettingsCheckpointIncompatibility,
): SettingsCheckpointItem['before'] {
  const historicalRestorable =
    side.state !== null || item.kind === 'sync_config' || item.kind === 'sync_override';
  return {
    ...side,
    state: structuredClone(side.state),
    differs: side.available && !sameMockCheckpointState(side.state, current),
    restorable: side.available && incompatibility === undefined && historicalRestorable,
    ...(incompatibility === undefined ? {} : { incompatibility }),
  };
}

function mockWorkspaceCheckpointCurrent(
  state: MockState,
  target: MockTarget,
  item: Pick<SettingsCheckpointItem, 'kind' | 'repository_id' | 'sync_kind'>,
): {
  current: SettingsCheckpointState | null;
  incompatibility?: SettingsCheckpointIncompatibility;
} {
  if (item.kind === 'target') {
    return {
      current: mockWorkspaceCheckpointState(
        mockWorkspaceTargetCheckpointDocument(target.value),
        target.value.revision,
      ),
    };
  }
  if (item.kind === 'repository') {
    const repository = target.repositories.find(
      ({ detail }) => detail.repository.id === item.repository_id,
    );
    if (repository === undefined) {
      return {
        current: null,
        incompatibility: {
          code: 'repository_unavailable',
          reason: 'This repository is no longer available in this workspace',
        },
      };
    }
    const current = mockWorkspaceCheckpointState(
      mockWorkspaceRepositoryCheckpointDocument(repository.detail),
      repository.detail.revision,
    );
    return repository.detail.repository.available
      ? { current }
      : {
          current,
          incompatibility: {
            code: 'repository_unavailable',
            reason: 'This repository is no longer available in this workspace',
          },
        };
  }
  if (item.kind === 'sync_config' && item.sync_kind !== undefined) {
    const config = state.sync.get(`${target.value.id}/${item.sync_kind}`);
    return {
      current:
        config === undefined
          ? null
          : mockWorkspaceCheckpointState(
              mockWorkspaceSyncConfigCheckpointDocument(config),
              config.revision,
            ),
    };
  }
  if (
    item.kind === 'sync_override' &&
    item.repository_id !== undefined &&
    item.sync_kind !== undefined
  ) {
    const repository = target.repositories.find(
      ({ detail }) => detail.repository.id === item.repository_id,
    );
    if (repository === undefined || !repository.detail.repository.available) {
      return {
        current: null,
        incompatibility: {
          code: 'repository_unavailable',
          reason: 'This repository is no longer available in this workspace',
        },
      };
    }
    const override = state.syncOverrides.get(`${item.repository_id}/${item.sync_kind}`);
    return {
      current:
        override === undefined
          ? null
          : mockWorkspaceCheckpointState(
              mockWorkspaceSyncOverrideCheckpointDocument(override),
              override.revision,
            ),
    };
  }
  return {
    current: null,
    incompatibility: {
      code: 'resource_unavailable',
      reason: 'This resource is not part of workspace settings',
    },
  };
}

function sameMockCheckpointState(
  left: SettingsCheckpointState | null,
  right: SettingsCheckpointState | null,
): boolean {
  if (left === null || right === null) return left === right;
  return left.digest === right.digest;
}

function restoreMockWorkspaceSettings(
  state: MockState,
  target: MockTarget,
  source: SettingsCheckpoint,
  input: SettingsRestoreInput,
): WorkspaceSettingsBatchResponse {
  const inspection = inspectMockWorkspaceSettingsCheckpoint(state, target, source);
  const selections = validateMockWorkspaceRestoreSelections(input);
  const items = new Map(
    inspection.items.map((item) => [mockWorkspaceCheckpointIdentity(item), item]),
  );
  const batch: WorkspaceSettingsBatchInput = {};
  const removals: Array<{
    key: string;
    item: SettingsCheckpointItem;
  }> = [];

  for (const selection of selections) {
    const key = mockWorkspaceCheckpointIdentity(selection);
    const item = items.get(key);
    if (item === undefined) {
      blockedMockWorkspaceRestore('selected resource is not represented by the checkpoint');
    }
    const selectedSide = item[input.state];
    if (!selectedSide.restorable) {
      blockedMockWorkspaceRestore(
        selectedSide.incompatibility?.reason ?? 'the selected settings cannot be restored',
      );
    }
    const actualRevision = item.current?.revision ?? 0;
    if (selection.expected_revision !== actualRevision) {
      throw new MockApiError(
        409,
        'settings_conflict',
        'settings changed in another session; inspect the checkpoint again',
      );
    }
    if (sameMockCheckpointState(selectedSide.state, item.current)) continue;
    if (selectedSide.state === null) {
      if (item.kind !== 'sync_config' && item.kind !== 'sync_override') {
        blockedMockWorkspaceRestore('the selected checkpoint state is incomplete');
      }
      removals.push({ key, item });
      continue;
    }
    appendMockWorkspaceRestoreInput(batch, selection, selectedSide.state.document);
  }

  const plan = mockWorkspaceRestorePlan(state, target, batch);
  const changed =
    plan.target?.changed === true ||
    plan.repositories.some(({ changed }) => changed) ||
    plan.syncConfigs.some(({ changed }) => changed) ||
    plan.syncOverrides.some(({ changed }) => changed);
  const effectiveRemovals = removals.filter(({ item }) => item.current !== null);
  if (!changed && effectiveRemovals.length === 0) {
    throw new MockApiError(
      409,
      'settings_restore_noop',
      'the selected settings already match the checkpoint',
    );
  }

  ensureMockWorkspaceSettingsBaseline(state, target);
  const before = mockWorkspaceSettingsSnapshot(state, target);
  applyMockWorkspaceSettingsPlan(state, target, plan);
  for (const { item } of effectiveRemovals) {
    if (item.kind === 'sync_config' && item.sync_kind !== undefined) {
      state.sync.delete(`${target.value.id}/${item.sync_kind}`);
    } else if (
      item.kind === 'sync_override' &&
      item.repository_id !== undefined &&
      item.sync_kind !== undefined
    ) {
      state.syncOverrides.delete(`${item.repository_id}/${item.sync_kind}`);
    }
  }
  const checkpoint = createMockWorkspaceSettingsCheckpoint(
    state,
    target,
    'installation.settings.restored',
    before,
    mockWorkspaceSettingsSnapshot(state, target),
    source.id,
    input.state,
  );
  const response = mockWorkspaceSettingsResponse(state, target, plan);
  response.checkpoint_id = checkpoint.id;
  addAudit(
    target,
    checkpoint.action,
    mockWorkspaceSettingsAuditSummary(
      'Restored',
      checkpoint.items.filter(({ changed }) => changed).length,
    ),
    undefined,
    checkpoint.id,
  );
  broadcast(state, { type: 'target.changed', target_id: target.value.id });
  return response;
}

function validateMockWorkspaceRestoreSelections(
  input: SettingsRestoreInput,
): SettingsRestoreInput['selections'] {
  if (input.state !== 'before' && input.state !== 'after') {
    invalidMockSettingsBatch('settings restore state must be before or after');
  }
  if (!Array.isArray(input.selections) || input.selections.length === 0) {
    invalidMockSettingsBatch('settings restore needs at least one selected resource');
  }
  if (input.selections.length > 4096) {
    invalidMockSettingsBatch('settings restore selects too many resources');
  }
  const seen = new Set<string>();
  for (const selection of input.selections) {
    validateMockRevision(selection.expected_revision);
    const repository = selection.repository_id?.trim() ?? '';
    const syncKind = selection.sync_kind;
    const valid =
      (selection.kind === 'target' && repository === '' && syncKind === undefined) ||
      (selection.kind === 'repository' && repository !== '' && syncKind === undefined) ||
      (selection.kind === 'sync_config' &&
        repository === '' &&
        syncKind !== undefined &&
        SYNC_KINDS.includes(syncKind)) ||
      (selection.kind === 'sync_override' &&
        repository !== '' &&
        syncKind !== undefined &&
        SYNC_KINDS.includes(syncKind));
    const key = mockWorkspaceCheckpointIdentity(selection);
    if (!valid || seen.has(key)) {
      invalidMockSettingsBatch('restore selections must be known and unique');
    }
    seen.add(key);
  }
  return input.selections;
}

function mockWorkspaceRestorePlan(
  state: MockState,
  target: MockTarget,
  batch: WorkspaceSettingsBatchInput,
): MockWorkspaceSettingsPlan {
  const hasResources =
    batch.target !== undefined ||
    (batch.repositories?.length ?? 0) > 0 ||
    (batch.sync_configs?.length ?? 0) > 0 ||
    (batch.sync_overrides?.length ?? 0) > 0;
  if (!hasResources) {
    return { repositories: [], syncConfigs: [], syncOverrides: [] };
  }
  validateMockWorkspaceSettingsBatch(batch);
  return prepareMockWorkspaceSettings(state, target, batch);
}

function blockedMockWorkspaceRestore(message: string): never {
  throw new MockApiError(409, 'settings_restore_blocked', message);
}

function appendMockWorkspaceRestoreInput(
  batch: WorkspaceSettingsBatchInput,
  selection: SettingsRestoreInput['selections'][number],
  document: Record<string, unknown>,
): void {
  switch (selection.kind) {
    case 'target':
      batch.target = {
        repository_default_enabled: mockCheckpointBoolean(document.repository_default_enabled),
        pending_ci_mode_default: mockCheckpointPendingCIMode(document.pending_ci_mode_default),
        pending_ci_branch_patterns_default: mockCheckpointBranchPatterns(
          document.pending_ci_branch_patterns_default,
        ),
        pending_ci_quiet_period_seconds_override: mockCheckpointSeconds(
          document.pending_ci_quiet_period_override,
        ),
        path_index_interval_seconds_override: mockCheckpointSeconds(
          document.path_index_interval_override,
        ),
        config_patch: mockCheckpointConfigPatch(document.config_patch),
        expected_revision: selection.expected_revision,
      };
      return;
    case 'repository':
      if (selection.repository_id === undefined) {
        blockedMockWorkspaceRestore('repository selection is incomplete');
      }
      (batch.repositories ??= []).push({
        repository_id: selection.repository_id,
        enabled_override: mockCheckpointNullableBoolean(document.enabled_override),
        pending_ci_mode_override: mockCheckpointNullablePendingCIMode(
          document.pending_ci_mode_override,
        ),
        pending_ci_branch_patterns_override: mockCheckpointNullableBranchPatterns(
          document.pending_ci_branch_patterns_override,
        ),
        pending_ci_quiet_period_seconds_override: mockCheckpointSeconds(
          document.pending_ci_quiet_period_override,
        ),
        path_index_interval_seconds_override: mockCheckpointSeconds(
          document.path_index_interval_override,
        ),
        config_patch: mockCheckpointConfigPatch(document.config_patch),
        ignore_repository_file: mockCheckpointBoolean(document.ignore_repository_file),
        expected_revision: selection.expected_revision,
      });
      return;
    case 'sync_config': {
      if (selection.sync_kind === undefined) {
        blockedMockWorkspaceRestore('Sync selection is incomplete');
      }
      const enabled = mockCheckpointBoolean(document.enabled);
      const nested = mockCheckpointNestedDocument(document.document);
      if (selection.sync_kind === 'labels') {
        (batch.sync_configs ??= []).push({
          kind: 'labels',
          enabled,
          labels: mockCheckpointLabels(nested.labels),
          allow_removal: mockCheckpointBoolean(nested.allow_removal),
          excludes: mockCheckpointStrings(nested.excludes),
          expected_revision: selection.expected_revision,
        });
      } else {
        (batch.sync_configs ??= []).push({
          kind: selection.sync_kind,
          enabled,
          document: nested,
          expected_revision: selection.expected_revision,
        });
      }
      return;
    }
    case 'sync_override':
      if (selection.repository_id === undefined || selection.sync_kind === undefined) {
        blockedMockWorkspaceRestore('repository Sync selection is incomplete');
      }
      (batch.sync_overrides ??= []).push({
        repository_id: selection.repository_id,
        kind: selection.sync_kind,
        enabled: mockCheckpointNullableBoolean(document.enabled),
        document: mockCheckpointNestedDocument(document.document),
        expected_revision: selection.expected_revision,
      });
      return;
    default:
      blockedMockWorkspaceRestore('unsupported workspace settings selection');
  }
}

function mockCheckpointBoolean(value: unknown): boolean {
  if (typeof value !== 'boolean') {
    blockedMockWorkspaceRestore('the selected checkpoint contains invalid settings');
  }
  return value;
}

function mockCheckpointNullableBoolean(value: unknown): boolean | null {
  if (value === null) return null;
  return mockCheckpointBoolean(value);
}

function mockCheckpointPendingCIMode(value: unknown): 'labels' | 'checks' {
  if (value !== 'labels' && value !== 'checks') {
    blockedMockWorkspaceRestore('the selected checkpoint contains invalid settings');
  }
  return value;
}

function mockCheckpointNullablePendingCIMode(value: unknown): 'labels' | 'checks' | null {
  return value === null ? null : mockCheckpointPendingCIMode(value);
}

function mockCheckpointBranchPatterns(value: unknown): { include: string[]; exclude: string[] } {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    blockedMockWorkspaceRestore('the selected checkpoint contains invalid settings');
  }
  const patterns = value as Record<string, unknown>;
  return {
    include: mockCheckpointStrings(patterns.include),
    exclude: mockCheckpointStrings(patterns.exclude),
  };
}

function mockCheckpointNullableBranchPatterns(
  value: unknown,
): { include: string[]; exclude: string[] } | null {
  return value === null ? null : mockCheckpointBranchPatterns(value);
}

function mockCheckpointSeconds(value: unknown): number | null {
  if (value === null) return null;
  if (
    typeof value !== 'number' ||
    !Number.isSafeInteger(value) ||
    value < 0 ||
    value % 1_000_000_000 !== 0
  ) {
    blockedMockWorkspaceRestore('the selected checkpoint contains an invalid duration');
  }
  return value / 1_000_000_000;
}

function mockCheckpointConfigPatch(value: unknown): ConfigPatch {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    blockedMockWorkspaceRestore('the selected checkpoint contains an invalid policy');
  }
  const patch = value as Record<string, unknown>;
  for (const [key, held] of Object.entries(patch)) {
    if (key === 'formatting') {
      if (parseFormattingPatch(held) === null) {
        blockedMockWorkspaceRestore('the selected checkpoint contains an invalid policy');
      }
      continue;
    }
    if (!CONFIG_KEYS.includes(key as ConfigKey) || !isMockConfigValue(key as ConfigKey, held)) {
      blockedMockWorkspaceRestore('the selected checkpoint contains an invalid policy');
    }
  }
  return structuredClone(patch) as ConfigPatch;
}

function isMockConfigValue(key: ConfigKey, value: unknown): boolean {
  if (key === 'allowed_commands') return Array.isArray(value) && value.every(isStringValue);
  if (key === 'command_aliases') {
    return (
      value !== null &&
      typeof value === 'object' &&
      !Array.isArray(value) &&
      Object.values(value).every(isStringValue)
    );
  }
  if (key === 'command_prefix') return typeof value === 'string';
  return typeof value === 'boolean';
}

function mockCheckpointNestedDocument(value: unknown): Record<string, unknown> {
  if (typeof value !== 'string') {
    blockedMockWorkspaceRestore('the selected checkpoint contains an invalid Sync document');
  }
  try {
    const parsed: unknown = JSON.parse(value);
    if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
      blockedMockWorkspaceRestore('the selected checkpoint contains an invalid Sync document');
    }
    return parsed as Record<string, unknown>;
  } catch (error) {
    if (error instanceof MockApiError) throw error;
    blockedMockWorkspaceRestore('the selected checkpoint contains an invalid Sync document');
  }
}

function mockCheckpointStrings(value: unknown): string[] {
  if (!Array.isArray(value) || !value.every(isStringValue)) {
    blockedMockWorkspaceRestore('the selected checkpoint contains an invalid string list');
  }
  return structuredClone(value);
}

function mockCheckpointLabels(
  value: unknown,
): Extract<WorkspaceSyncConfigSettingsInput, { kind: 'labels' }>['labels'] {
  if (!Array.isArray(value)) {
    blockedMockWorkspaceRestore('the selected checkpoint contains invalid labels');
  }
  const labels = value.map((entry) => {
    if (entry === null || typeof entry !== 'object' || Array.isArray(entry)) {
      blockedMockWorkspaceRestore('the selected checkpoint contains invalid labels');
    }
    const label = entry as Record<string, unknown>;
    if (
      typeof label.name !== 'string' ||
      typeof label.color !== 'string' ||
      (label.description !== undefined && typeof label.description !== 'string')
    ) {
      blockedMockWorkspaceRestore('the selected checkpoint contains invalid labels');
    }
    return {
      name: label.name,
      color: label.color,
      ...(label.description === undefined ? {} : { description: label.description }),
    };
  });
  return labels;
}

function mockSyncConfigDocument(config: SyncConfig): Record<string, unknown> {
  return config.kind === 'labels'
    ? {
        labels: structuredClone(config.labels),
        allow_removal: config.allow_removal,
        excludes: structuredClone(config.excludes),
      }
    : structuredClone(config.document);
}

function mockWorkspaceTargetState(target: PanelTarget): WorkspaceTargetSettingsState {
  return {
    target_id: target.id,
    ...structuredClone(mockWorkspaceTargetDocument(target)),
    revision: target.revision,
  };
}

function mockWorkspaceRepositoryState(detail: RepositoryDetail): WorkspaceRepositorySettingsState {
  return {
    repository_id: detail.repository.id,
    ...structuredClone(mockWorkspaceRepositoryDocument(detail)),
    revision: detail.revision,
  };
}

function mockWorkspaceSyncConfigState(
  targetId: string,
  config: SyncConfig,
): WorkspaceSyncConfigSettingsState {
  if (!SYNC_KINDS.includes(config.kind as SyncKind)) throw new Error('mock Sync kind is invalid');
  return {
    target_id: targetId,
    kind: config.kind as SyncKind,
    enabled: config.enabled,
    document: mockSyncConfigDocument(config),
    revision: config.revision,
  };
}

function mockWorkspaceSyncOverrideState(
  targetId: string,
  repositoryId: string,
  override: SyncOverride,
): WorkspaceSyncOverrideSettingsState {
  if (!SYNC_KINDS.includes(override.kind as SyncKind)) throw new Error('mock Sync kind is invalid');
  return {
    target_id: targetId,
    repository_id: repositoryId,
    kind: override.kind as SyncKind,
    enabled: override.enabled,
    document: structuredClone(override.document),
    revision: override.revision,
  };
}

function emptyMockSyncConfig(kind: SyncKind): SyncConfig {
  return {
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
}

function emptyMockSyncOverride(kind: SyncKind): SyncOverride {
  return { kind, enabled: null, document: {}, revision: 0, unreadable: false };
}

function sameMockDocument(left: unknown, right: unknown): boolean {
  return canonicalStringify(left) === canonicalStringify(right);
}

function mockBatchArray<T>(value: T[] | undefined, name: string): T[] {
  if (value === undefined) return [];
  if (!Array.isArray(value)) invalidMockSettingsBatch(`${name} must be a list`);
  return value;
}

function invalidMockSettingsBatch(message: string): never {
  throw new MockApiError(400, 'invalid_request', message);
}

function rootWorkspaceValue(target: MockTarget): RootWorkspace {
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

const ROOT_RUNTIME_REQUIRED_INPUT_KEYS = [
  'bot_config',
  'log_level',
  'reaction_poll_interval_seconds',
  'merge_after_ci_quiet_period_seconds',
  'path_index_interval_seconds',
  'session_ttl_seconds',
  'expected_revision',
] as const;
const ROOT_RUNTIME_INPUT_KEYS = [
  ...ROOT_RUNTIME_REQUIRED_INPUT_KEYS,
  'background_work_paused',
] as const;
const ROOT_RUNTIME_LOG_LEVELS = new Set(['debug', 'info', 'warn', 'error']);
const ROOT_RUNTIME_NANOSECONDS_PER_SECOND = 1_000_000_000;
const ROOT_RUNTIME_MAX_SESSION_SECONDS = 30 * 24 * 60 * 60;

function saveMockRootRuntimeSettings(
  state: MockState,
  input: RootRuntimeSettingsInput,
): RootRuntimeSettings {
  validateMockRootRuntimeSettingsInput(input);
  if (input.expected_revision !== state.runtime.revision) {
    throw new MockApiError(409, 'conflict', 'runtime settings changed; reload and try again');
  }

  const before = mockRootRuntimeCheckpointState(state);
  const nextDocument = mockRootRuntimeDocumentFromInput(state, input);
  if (sameMockDocument(before.document, nextDocument)) return rootRuntimeSettingsValue(state);

  ensureMockRootRuntimeBaseline(state);
  applyMockRootRuntimeDocument(state, nextDocument);
  state.runtime.revision += 1;
  state.runtime.updatedAt = new Date().toISOString();
  state.runtime.updatedBy = VIEWER;
  const after = mockRootRuntimeCheckpointState(state);
  const checkpoint = createMockRootRuntimeCheckpoint(
    state,
    'runtime.settings.saved',
    before,
    after,
  );
  addMockRootRuntimeAudit(state, checkpoint, 'Saved runtime settings');
  broadcast(state, { type: 'resync' });

  return { ...rootRuntimeSettingsValue(state), checkpoint_id: checkpoint.id };
}

function restoreMockRootRuntimeSettings(
  state: MockState,
  source: SettingsCheckpoint,
  input: SettingsRestoreInput,
): RootRuntimeSettings {
  if (
    (input.state !== 'before' && input.state !== 'after') ||
    !Array.isArray(input.selections) ||
    input.selections.length !== 1 ||
    input.selections[0]?.kind !== 'runtime' ||
    !Number.isSafeInteger(input.selections[0].expected_revision) ||
    input.selections[0].expected_revision < 0
  ) {
    throw new MockApiError(
      400,
      'invalid_request',
      'Root restore must select runtime settings at their current revision',
    );
  }
  if (input.selections[0].expected_revision !== state.runtime.revision) {
    throw new MockApiError(
      409,
      'conflict',
      'settings changed in another session; inspect the checkpoint again',
    );
  }
  const historical = source.items.find((item) => item.kind === 'runtime')?.[input.state];
  if (historical === undefined || !historical.restorable || historical.state === null) {
    throw new MockApiError(
      409,
      'settings_restore_blocked',
      'the selected settings cannot be restored',
    );
  }

  const before = mockRootRuntimeCheckpointState(state);
  if (before.digest === historical.state.digest) {
    throw new MockApiError(
      409,
      'settings_restore_noop',
      'the selected settings already match the checkpoint',
    );
  }
  const restoredInput = mockRootRuntimeInputFromDocument(
    historical.state.document,
    state.runtime.revision,
  );
  validateMockRootRuntimeSettingsInput(restoredInput);
  applyMockRootRuntimeDocument(state, historical.state.document);
  state.runtime.revision += 1;
  state.runtime.updatedAt = new Date().toISOString();
  state.runtime.updatedBy = VIEWER;
  const after = mockRootRuntimeCheckpointState(state);
  const checkpoint = createMockRootRuntimeCheckpoint(
    state,
    'runtime.settings.restored',
    before,
    after,
    source.id,
    input.state,
  );
  addMockRootRuntimeAudit(state, checkpoint, 'Restored runtime settings');
  broadcast(state, { type: 'resync' });

  return { ...rootRuntimeSettingsValue(state), checkpoint_id: checkpoint.id };
}

function inspectMockRootRuntimeCheckpoint(
  state: MockState,
  checkpoint: SettingsCheckpoint,
): SettingsCheckpoint {
  const inspection = structuredClone(checkpoint);
  const current = mockRootRuntimeCheckpointState(state);
  inspection.items = inspection.items.map((item) => ({
    ...item,
    current: structuredClone(current),
    before: {
      ...item.before,
      differs: item.before.available && !sameMockCheckpointState(item.before.state, current),
      restorable: item.before.available && item.before.state !== null,
    },
    after: {
      ...item.after,
      differs: item.after.available && !sameMockCheckpointState(item.after.state, current),
      restorable: item.after.available && item.after.state !== null,
    },
  }));
  return inspection;
}

function findMockRootRuntimeCheckpoint(
  state: MockState,
  encodedCheckpointId: string,
): SettingsCheckpoint {
  let checkpointId: string;
  try {
    checkpointId = decodeURIComponent(encodedCheckpointId);
  } catch {
    throw new MockApiError(404, 'not_found', 'settings checkpoint not found');
  }
  if (checkpointId === 'baseline') return ensureMockRootRuntimeBaseline(state);
  const checkpoint = state.runtime.checkpoints.get(checkpointId);
  if (checkpoint === undefined) {
    throw new MockApiError(404, 'not_found', 'settings checkpoint not found');
  }
  return checkpoint;
}

function createMockRootRuntimeCheckpoint(
  state: MockState,
  action: 'runtime.settings.baseline' | 'runtime.settings.saved' | 'runtime.settings.restored',
  before: SettingsCheckpointState | null,
  after: SettingsCheckpointState,
  restoredFromId?: string,
  restoredSide?: SettingsRestoreInput['state'],
  beforeCaptured = true,
): SettingsCheckpoint {
  const id = String(state.runtime.checkpointCounter);
  state.runtime.checkpointCounter += 1;
  const checkpoint: SettingsCheckpoint = {
    id,
    action,
    actor: VIEWER,
    ...(restoredFromId === undefined ? {} : { restored_from_id: restoredFromId }),
    ...(restoredSide === undefined ? {} : { restored_side: restoredSide }),
    created_at: new Date().toISOString(),
    affected_kinds: action === 'runtime.settings.baseline' ? [] : ['runtime'],
    items: [
      {
        kind: 'runtime',
        document_version: 1,
        before: {
          available: beforeCaptured,
          state: structuredClone(before),
          differs: beforeCaptured && !sameMockCheckpointState(before, after),
          restorable: beforeCaptured && before !== null,
        },
        after: {
          available: true,
          state: structuredClone(after),
          differs: false,
          restorable: true,
        },
        current: structuredClone(after),
        changed: beforeCaptured && !sameMockCheckpointState(before, after),
      },
    ],
  };
  state.runtime.checkpoints.set(id, structuredClone(checkpoint));
  return checkpoint;
}

function ensureMockRootRuntimeBaseline(state: MockState): SettingsCheckpoint {
  const existing = [...state.runtime.checkpoints.values()].find(
    ({ action }) => action === 'runtime.settings.baseline',
  );
  if (existing !== undefined) return existing;
  const current = mockRootRuntimeCheckpointState(state);
  return createMockRootRuntimeCheckpoint(
    state,
    'runtime.settings.baseline',
    null,
    current,
    undefined,
    undefined,
    false,
  );
}

function addMockRootRuntimeAudit(
  state: MockState,
  checkpoint: SettingsCheckpoint,
  summary: string,
): void {
  state.runtime.audit.unshift({
    id: `root-runtime-${checkpoint.id}`,
    category: 'runtime',
    actor: VIEWER,
    action: checkpoint.action,
    summary,
    settings_checkpoint_id: checkpoint.id,
    created_at: checkpoint.created_at,
  });
}

function mockRootRuntimeCheckpointState(state: MockState): SettingsCheckpointState {
  const document = mockRootRuntimeDocument(state);
  return {
    document,
    digest: mockRootRuntimeDigest(document),
    revision: state.runtime.revision,
  };
}

function mockRootRuntimeDocument(state: MockState): Record<string, unknown> {
  return {
    background_work_paused: state.runtime.backgroundWorkPaused,
    bot_config: copyOptionalConfig(state.runtime.behaviorOverride),
    log_level: state.runtime.logLevelOverride,
    poll_interval: mockRootRuntimeDuration(state.runtime.pollIntervalOverride),
    pending_ci_quiet_period: mockRootRuntimeDuration(state.runtime.pendingCIQuietPeriodOverride),
    session_ttl: mockRootRuntimeDuration(state.runtime.sessionTTLOverride),
    path_index_interval: mockRootRuntimeDuration(state.runtime.pathIndexIntervalOverride),
  };
}

function mockRootRuntimeDocumentFromInput(
  state: MockState,
  input: RootRuntimeSettingsInput,
): Record<string, unknown> {
  return {
    background_work_paused: input.background_work_paused ?? state.runtime.backgroundWorkPaused,
    bot_config: copyOptionalConfig(input.bot_config),
    log_level: input.log_level,
    poll_interval: mockRootRuntimeDuration(input.reaction_poll_interval_seconds),
    pending_ci_quiet_period: mockRootRuntimeDuration(input.merge_after_ci_quiet_period_seconds),
    session_ttl: mockRootRuntimeDuration(input.session_ttl_seconds),
    path_index_interval: mockRootRuntimeDuration(input.path_index_interval_seconds),
  };
}

function mockRootRuntimeInputFromDocument(
  document: Record<string, unknown>,
  expectedRevision: number,
): RootRuntimeSettingsInput {
  return {
    background_work_paused: mockRootRuntimePaused(document.background_work_paused),
    bot_config: mockRootRuntimeConfig(document.bot_config),
    log_level: mockRootRuntimeLogLevel(document.log_level),
    reaction_poll_interval_seconds: mockRootRuntimeSeconds(document.poll_interval),
    merge_after_ci_quiet_period_seconds: mockRootRuntimeSeconds(document.pending_ci_quiet_period),
    path_index_interval_seconds: mockRootRuntimeSeconds(document.path_index_interval),
    session_ttl_seconds: mockRootRuntimeSeconds(document.session_ttl),
    expected_revision: expectedRevision,
  };
}

function applyMockRootRuntimeDocument(state: MockState, document: Record<string, unknown>): void {
  const input = mockRootRuntimeInputFromDocument(document, state.runtime.revision);
  state.runtime.backgroundWorkPaused = input.background_work_paused ?? false;
  state.runtime.behaviorOverride = copyOptionalConfig(input.bot_config);
  state.runtime.logLevelOverride = input.log_level;
  state.runtime.pollIntervalOverride = input.reaction_poll_interval_seconds;
  state.runtime.pendingCIQuietPeriodOverride = input.merge_after_ci_quiet_period_seconds;
  state.runtime.pathIndexIntervalOverride = input.path_index_interval_seconds;
  state.runtime.sessionTTLOverride = input.session_ttl_seconds;
}

function mockRootRuntimeDuration(seconds: number | null): number | null {
  return seconds === null ? null : seconds * ROOT_RUNTIME_NANOSECONDS_PER_SECOND;
}

function mockRootRuntimeSeconds(value: unknown): number | null {
  if (value === null) return null;
  if (
    typeof value !== 'number' ||
    !Number.isSafeInteger(value) ||
    value < 0 ||
    value % ROOT_RUNTIME_NANOSECONDS_PER_SECOND !== 0
  ) {
    throw new MockApiError(
      409,
      'settings_restore_blocked',
      'the selected settings cannot be restored',
    );
  }
  return value / ROOT_RUNTIME_NANOSECONDS_PER_SECOND;
}

function mockRootRuntimePaused(value: unknown): boolean {
  if (typeof value !== 'boolean') {
    throw new MockApiError(409, 'settings_restore_blocked', 'background work pause is invalid');
  }
  return value;
}

function mockRootRuntimeConfig(value: unknown): ConfigValues | null {
  if (value === null) return null;
  if (!isMockRootRuntimeConfig(value)) {
    throw new MockApiError(
      409,
      'settings_restore_blocked',
      'the selected settings cannot be restored',
    );
  }
  return copyConfig(value);
}

function mockRootRuntimeLogLevel(value: unknown): string | null {
  if (value === null) return null;
  if (typeof value !== 'string' || !ROOT_RUNTIME_LOG_LEVELS.has(value)) {
    throw new MockApiError(
      409,
      'settings_restore_blocked',
      'the selected settings cannot be restored',
    );
  }
  return value;
}

function mockRootRuntimeDigest(document: Record<string, unknown>): string {
  return `sha256:${createHash('sha256').update(canonicalStringify(document)).digest('hex')}`;
}

function validateMockRootRuntimeSettingsInput(input: RootRuntimeSettingsInput): void {
  if (
    input === null ||
    typeof input !== 'object' ||
    Object.keys(input).some(
      (key) => !(ROOT_RUNTIME_INPUT_KEYS as readonly string[]).includes(key),
    ) ||
    ROOT_RUNTIME_REQUIRED_INPUT_KEYS.some((key) => !Object.hasOwn(input, key)) ||
    !Number.isSafeInteger(input.expected_revision) ||
    input.expected_revision < 0
  ) {
    invalidMockRootRuntimeSettings('every runtime setting and expected revision is required');
  }
  if (
    input.background_work_paused !== undefined &&
    typeof input.background_work_paused !== 'boolean'
  ) {
    invalidMockRootRuntimeSettings('background work pause must be true or false');
  }
  if (input.bot_config !== null && !isMockRootRuntimeConfig(input.bot_config)) {
    invalidMockRootRuntimeSettings('behavior defaults are invalid');
  }
  if (
    input.log_level !== null &&
    (typeof input.log_level !== 'string' || !ROOT_RUNTIME_LOG_LEVELS.has(input.log_level))
  ) {
    invalidMockRootRuntimeSettings('log level is invalid');
  }
  validateMockRootRuntimeDuration(
    input.reaction_poll_interval_seconds,
    0,
    86_400,
    'reaction sweep interval',
    true,
  );
  validateMockRootRuntimeDuration(
    input.merge_after_ci_quiet_period_seconds,
    0,
    86_400,
    'merge-after-CI quiet period',
  );
  validateMockRootRuntimeDuration(
    input.path_index_interval_seconds,
    0,
    DEV_MAX_PATH_INDEX_SECONDS,
    'file list refresh interval',
  );
  validateMockRootRuntimeDuration(
    input.session_ttl_seconds,
    60,
    ROOT_RUNTIME_MAX_SESSION_SECONDS,
    'session lifetime',
  );
}

function validateMockRootRuntimeDuration(
  value: number | null,
  minimum: number,
  maximum: number,
  label: string,
  zeroIsOptional = false,
): void {
  if (value === null) return;
  if (
    !Number.isSafeInteger(value) ||
    value < 0 ||
    value > maximum ||
    (value === 0 ? !zeroIsOptional && minimum > 0 : value < minimum)
  ) {
    invalidMockRootRuntimeSettings(`${label} is outside the supported range`);
  }
}

function isMockRootRuntimeConfig(value: unknown): value is ConfigValues {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return false;
  const candidate = value as Record<string, unknown>;
  if (Object.keys(candidate).length !== CONFIG_KEYS.length + 1) return false;
  if (parseFormattingPolicy(candidate.formatting) === null) return false;
  return CONFIG_KEYS.every((key) => {
    const held = candidate[key];
    if (key === 'allowed_commands') return Array.isArray(held) && held.every(isStringValue);
    if (key === 'command_aliases') {
      return (
        held !== null &&
        typeof held === 'object' &&
        !Array.isArray(held) &&
        Object.values(held).every(isStringValue)
      );
    }
    if (key === 'command_prefix') return typeof held === 'string';
    return typeof held === 'boolean';
  });
}

function isStringValue(value: unknown): value is string {
  return typeof value === 'string';
}

function invalidMockRootRuntimeSettings(message: string): never {
  throw new MockApiError(400, 'invalid_runtime_settings', message);
}

function rootRuntimeSettingsValue(state: MockState): RootRuntimeSettings {
  const behaviorOverride = copyOptionalConfig(state.runtime.behaviorOverride);
  return {
    background_work_paused: state.runtime.backgroundWorkPaused,
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
    merge_after_ci_quiet_period: {
      deployment_seconds: DEV_PENDING_CI_QUIET_SECONDS,
      override_seconds: state.runtime.pendingCIQuietPeriodOverride,
      effective_seconds: state.runtime.pendingCIQuietPeriodOverride ?? DEV_PENDING_CI_QUIET_SECONDS,
    },
    path_index_interval: {
      deployment_seconds: DEV_PATH_INDEX_SECONDS,
      override_seconds: state.runtime.pathIndexIntervalOverride,
      effective_seconds: state.runtime.pathIndexIntervalOverride ?? DEV_PATH_INDEX_SECONDS,
      max_seconds: DEV_MAX_PATH_INDEX_SECONDS,
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
  return structuredClone(value);
}

function rootOverviewValue(state: MockState): RootOverview {
  const workspaces = state.targets.map((target) => rootWorkspaceValue(target));
  const repositories = state.targets.flatMap((target) => target.repositories);
  const recentFailures = state.targets
    .flatMap((target) =>
      target.failures.map((failure) => ({ workspace: target.value.account, failure })),
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
      workspaces: state.targets.length,
      repositories: repositories.length,
      enabled_repositories: repositories.filter(
        (repository) => repository.detail.repository.effective_enabled,
      ).length,
    },
    ownership: {
      fresh: workspaces.filter(
        (workspace) => workspace.ownership.status === 'fresh' && !workspace.ownership.stale,
      ).length,
      stale: workspaces.filter(
        (workspace) => workspace.ownership.status === 'fresh' && workspace.ownership.stale,
      ).length,
      permission_pending: workspaces.filter(
        (workspace) => workspace.ownership.status === 'permission_pending',
      ).length,
      error: workspaces.filter((workspace) => workspace.ownership.status === 'error').length,
    },
    active_elevations: [...state.elevations.values()].filter(
      (elevation) => Date.parse(elevation.expires_at) > Date.now(),
    ).length,
    unread_security_events: state.notifications.filter(
      (notification) => notification.read_at === undefined,
    ).length,
    recent_failures: recentFailures,
    /* Read straight off the state, which the reconciler above is moving on its own. This used to
       push an expired quiet period forward on the way out, so the countdown never reached zero and
       the merge it counts down TO could not be watched. */
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

function findPendingCI(state: MockState, encodedID: string): PendingCIRequest {
  const id = decodeURIComponent(encodedID);
  const request = state.pendingCI.find((candidate) => candidate.id === id);
  if (request === undefined) {
    throw new MockApiError(404, 'not_found', 'pending CI request not found');
  }
  return request;
}

function globalSchedulePolicies(state: MockState): QueuePolicy[] {
  return state.schedulePolicies.filter((policy) => policy.target_id === undefined);
}

function targetSchedulePolicies(state: MockState, targetID: string): RootJobPolicies['policy_set'] {
  const current = globalSchedulePolicies(state);
  const overrides = state.schedulePolicies.filter((policy) => policy.target_id === targetID);
  const effective = current.map(
    (policy) => overrides.find((override) => override.kind === policy.kind) ?? policy,
  );

  return {
    current: structuredClone(current),
    deployment_defaults: structuredClone(state.scheduleDefaults),
    overrides: structuredClone(overrides),
    effective: structuredClone(effective),
  };
}

function mockPolicyStatuses(state: MockState, targetID?: string): QueuePolicyStatus[] {
  const now = Date.now();
  const policies =
    targetID === undefined
      ? globalSchedulePolicies(state)
      : targetSchedulePolicies(state, targetID).effective;

  return policies.map((policy, index) => {
    const current = state.queue.find(
      (item) =>
        item.kind === policy.kind &&
        (targetID === undefined || item.target_id === targetID) &&
        !['succeeded', 'failed', 'cancelled', 'superseded'].includes(item.state),
    );
    const last = state.queue.find(
      (item) =>
        item.kind === policy.kind &&
        (targetID === undefined || item.target_id === targetID) &&
        ['succeeded', 'failed', 'cancelled', 'superseded'].includes(item.state),
    );
    const next =
      current?.eligible_at ?? new Date(now + Number(policy.cadence) / 1_000_000).toISOString();
    return {
      kind: policy.kind,
      ...(targetID === undefined ? {} : { target_id: targetID }),
      ...(last === undefined ? {} : { last_run_at: last.finished_at ?? last.updated_at }),
      ...(last === undefined ? {} : { last_state: last.state }),
      next_eligibility_at: next,
      estimated_start_at: current?.estimated_start_at ?? next,
      work_ahead: current?.work_ahead ?? index % 3,
      ...(current === undefined
        ? {}
        : { current_state: current.state, current_queue_item_id: current.id }),
    };
  });
}

function findMockScheduleProfile(state: MockState, encodedID: string): ScheduleProfile {
  const id = decodeURIComponent(encodedID);
  const profile = state.scheduleProfiles.find((candidate) => candidate.id === id);
  if (profile === undefined) throw new MockApiError(404, 'not_found', 'schedule profile not found');
  return profile;
}

function saveMockScheduleProfile(
  state: MockState,
  input: ScheduleProfileInput,
  encodedID?: string,
): ScheduleProfile {
  const existing = encodedID === undefined ? undefined : findMockScheduleProfile(state, encodedID);
  if (existing !== undefined && input.expected_revision !== existing.revision) {
    throw new MockApiError(409, 'conflict', 'schedule profile changed; reload and try again');
  }
  if (existing === undefined && input.expected_revision !== 0) {
    throw new MockApiError(409, 'conflict', 'new schedule profiles start at revision zero');
  }
  const id =
    existing?.id ??
    `profile:mock-${state.scheduleCounter++}-${input.name
      .toLocaleLowerCase()
      .replaceAll(/[^a-z0-9]+/gu, '-')
      .replaceAll(/(^-|-$)/gu, '')}`;
  const saved: ScheduleProfile = {
    id,
    name: input.name,
    timezone: input.timezone,
    system: existing?.system ?? false,
    revision: (existing?.revision ?? 0) + 1,
    affected_workspaces: existing?.affected_workspaces ?? 0,
    affected_items: existing?.affected_items ?? 0,
    affected_policies: existing?.affected_policies ?? 0,
    windows: structuredClone(input.windows),
    exceptions: structuredClone(input.exceptions),
  };
  state.scheduleProfiles = [
    ...state.scheduleProfiles.filter((profile) => profile.id !== saved.id),
    saved,
  ];
  return structuredClone(saved);
}

function queuePolicyFromInput(
  existing: QueuePolicy,
  input: QueuePolicyInput,
  targetID?: string,
): QueuePolicy {
  if (input.expected_revision !== existing.revision) {
    throw new MockApiError(409, 'conflict', 'schedule policy changed; reload and try again');
  }
  return {
    kind: existing.kind,
    ...(targetID === undefined ? {} : { target_id: targetID }),
    enabled: input.enabled,
    cadence: input.cadence_seconds * NANOSECONDS_PER_SECOND,
    profile_id: input.profile_id,
    default_priority: input.default_priority,
    retry_delay: input.retry_delay_seconds * NANOSECONDS_PER_SECOND,
    ...(input.retention_seconds === undefined
      ? {}
      : { retention: input.retention_seconds * NANOSECONDS_PER_SECOND }),
    ...(input.approval_lifetime_seconds === undefined
      ? {}
      : { approval_ttl: input.approval_lifetime_seconds * NANOSECONDS_PER_SECOND }),
    ...(input.configuration === undefined
      ? {}
      : { configuration: structuredClone(input.configuration) }),
    revision: existing.revision + 1,
    updated_at: new Date().toISOString(),
  };
}

function saveMockQueuePolicy(
  state: MockState,
  kind: QueueWorkload,
  input: QueuePolicyInput,
  targetID?: string,
): QueuePolicy {
  const existing =
    state.schedulePolicies.find(
      (policy) => policy.kind === kind && policy.target_id === targetID,
    ) ??
    (targetID === undefined
      ? undefined
      : globalSchedulePolicies(state).find((policy) => policy.kind === kind));
  if (existing === undefined) throw new MockApiError(404, 'not_found', 'schedule policy not found');
  const saved = queuePolicyFromInput(existing, input, targetID);
  state.schedulePolicies = [
    ...state.schedulePolicies.filter(
      (policy) => !(policy.kind === saved.kind && policy.target_id === saved.target_id),
    ),
    saved,
  ];
  return structuredClone(saved);
}

function findMockScheduleRequest(state: MockState, encodedID: string): ScheduleRequest {
  const id = decodeURIComponent(encodedID);
  const request = state.scheduleRequests.find((candidate) => candidate.id === id);
  if (request === undefined) throw new MockApiError(404, 'not_found', 'schedule request not found');
  return request;
}

function mockQueuePage(items: QueueItem[], query = new URLSearchParams()): QueuePage {
  const summary = query.get('summary') === 'true';
  const facets: QueuePage['facets'] = summary
    ? {
        targets: [],
        repositories: [],
        profiles: [],
        states: [],
        workloads: [],
        priorities: [],
      }
    : {
        targets: uniqueQueueValues(items, (item) => item.target_id),
        repositories: uniqueQueueValues(items, (item) => item.repository_id),
        profiles: uniqueQueueValues(items, (item) => item.profile_id ?? 'immediate'),
        states: uniqueQueueValues(items, (item) => item.state),
        workloads: uniqueQueueValues(items, (item) => item.kind),
        priorities: uniqueQueueValues(items, (item) => item.priority),
      };
  const stateCounts = summary
    ? items.reduce<NonNullable<QueuePage['state_counts']>>((counts, item) => {
        counts[item.state] = (counts[item.state] ?? 0) + 1;
        return counts;
      }, {})
    : undefined;
  const states = queueQueryValues(query, 'state');
  const workloads = queueQueryValues(query, 'workload');
  const priorities = queueQueryValues(query, 'priority');
  const after = Date.parse(query.get('created_after') ?? '');
  const before = Date.parse(query.get('created_before') ?? '');
  const finishedAfter = Date.parse(query.get('finished_after') ?? '');
  const search = (query.get('search') ?? '').trim().toLowerCase();
  const filtered = items.filter(
    (item) =>
      matchesQueueQuery(query, 'workspace', item.target_id) &&
      matchesQueueQuery(query, 'repository', item.repository_id) &&
      matchesQueueQuery(query, 'profile', item.profile_id ?? 'immediate') &&
      (states.length === 0 || states.includes(item.state)) &&
      (workloads.length === 0 || workloads.includes(item.kind)) &&
      (priorities.length === 0 || priorities.includes(item.priority)) &&
      (Number.isNaN(after) || Date.parse(item.created_at) >= after) &&
      (Number.isNaN(before) || Date.parse(item.created_at) < before) &&
      (Number.isNaN(finishedAfter) ||
        (item.finished_at !== undefined && Date.parse(item.finished_at) >= finishedAfter)) &&
      (search === '' || `${item.title} ${item.summary ?? ''}`.toLowerCase().includes(search)),
  );
  const offset = Number(query.get('offset') ?? 0);
  const limit = Number(query.get('limit') ?? 50);
  const page = filtered.slice(offset, offset + limit);
  const nextOffset = offset + limit < filtered.length ? offset + limit : 0;

  return {
    items: structuredClone(page),
    next_offset: nextOffset,
    total: filtered.length,
    facets,
    state_counts: stateCounts,
  };
}

function uniqueQueueValues<T extends string>(
  items: QueueItem[],
  value: (item: QueueItem) => T | undefined,
): T[] {
  return [...new Set(items.flatMap((item) => value(item) ?? []))].sort();
}

function queueQueryValues(query: URLSearchParams, name: string): string[] {
  return query
    .getAll(name)
    .flatMap((value) => value.split(','))
    .filter((value) => value !== '');
}

function matchesQueueQuery(
  query: URLSearchParams,
  name: string,
  actual: string | undefined,
): boolean {
  const expected = query.get(name);
  return expected === null || expected === actual;
}

function findMockQueueItem(items: QueueItem[], encodedID: string, targetID?: string): QueueItem {
  const id = decodeURIComponent(encodedID);
  const item = items.find(
    (candidate) =>
      candidate.id === id && (targetID === undefined || candidate.target_id === targetID),
  );
  if (item === undefined) throw new MockApiError(404, 'not_found', 'queue item not found');

  return item;
}

function mockQueueDetail(item: QueueItem): QueueDetail {
  const events = [
    {
      id: 1,
      item_id: item.id,
      actor: 'system',
      kind: 'created',
      state: item.state,
      summary: `Queued ${item.title}`,
      created_at: item.created_at,
    },
  ];
  if (item.updated_at !== item.created_at) {
    events.push({
      id: 2,
      item_id: item.id,
      actor: 'system',
      kind: item.state === 'running' ? 'started' : 'updated',
      state: item.state,
      summary: item.state === 'running' ? `Started ${item.title}` : `Updated ${item.title}`,
      created_at: item.updated_at,
    });
  }

  return { item: structuredClone(item), events };
}

function applyMockQueueAction(
  items: QueueItem[],
  encodedID: string,
  input: QueueActionInput,
  targetID?: string,
): QueueItem {
  const item = findMockQueueItem(items, encodedID, targetID);
  const index = items.indexOf(item);
  if (item.revision !== input.expected_revision) {
    throw new MockApiError(409, 'conflict', 'queue item changed; reload and try again');
  }
  const now = new Date().toISOString();
  const updated: QueueItem = { ...item, revision: item.revision + 1, updated_at: now };
  if (input.type === 'set_priority' && input.priority !== undefined) {
    updated.priority = input.priority;
    updated.priority_overridden = true;
  } else if (input.type === 'run_now') {
    updated.state = 'ready';
    updated.immediate = true;
    updated.window_mode = 'bypass';
    updated.not_before = now;
    updated.eligible_at = now;
    updated.blocked_reason = undefined;
    updated.reason = input.reason;
  } else if (input.type === 'next_window') {
    updated.state = 'ready';
    updated.immediate = false;
    updated.window_mode = 'respect';
    updated.not_before = now;
    updated.eligible_at = now;
    updated.blocked_reason = undefined;
  } else if (input.type === 'schedule_at' && input.at !== undefined) {
    updated.state = Date.parse(input.at) <= Date.now() ? 'ready' : 'scheduled';
    updated.immediate = false;
    updated.window_mode = input.outside_window === true ? 'bypass' : 'respect';
    updated.not_before = input.at;
    updated.eligible_at = input.at;
    updated.reason = input.reason;
  } else if (input.type === 'cancel') {
    updated.state = 'cancelled';
    updated.finished_at = now;
    updated.reason = input.reason;
    updated.actions = [];
  }
  items[index] = updated;

  return structuredClone(updated);
}

function previewMockQueueAction(
  state: MockState,
  item: QueueItem,
  input: QueueActionInput,
): QueueSchedulePreview {
  if (item.revision !== input.expected_revision) {
    throw new MockApiError(409, 'conflict', 'queue item changed; reload and try again');
  }
  if (input.type !== 'schedule_at' || input.at === undefined) {
    throw new MockApiError(400, 'invalid_action', 'only exact-time scheduling can be previewed');
  }

  const requestedAt = new Date(input.at);
  if (Number.isNaN(requestedAt.getTime())) {
    throw new MockApiError(400, 'invalid_time', 'schedule time must be an ISO timestamp');
  }
  const outsideWindow = input.outside_window === true;
  const profile = outsideWindow
    ? undefined
    : state.scheduleProfiles.find((candidate) => candidate.id === item.profile_id);

  return {
    item_revision: item.revision,
    requested_at: requestedAt.toISOString(),
    eligible_at: requestedAt.toISOString(),
    outside_window: outsideWindow,
    ...(profile === undefined
      ? {}
      : {
          profile_id: profile.id,
          profile_name: profile.name,
          profile_timezone: profile.timezone,
        }),
  };
}

function requirePendingCIRevision(request: PendingCIRequest, revision: number): void {
  if (revision !== request.revision) {
    throw new MockApiError(409, 'conflict', 'pending CI request changed; reload and try again');
  }
}

/**
 * A record with enough in it to be a record.
 *
 * One event drew a timeline with no rail, no second mark and nothing to align - which is most of
 * what the page is. These are the events a passing request actually accumulates, in the order the
 * reconciler writes them.
 */
function pendingCIDetail(request: PendingCIRequest): PendingCIDetail {
  const armedAt = Date.parse(request.requested_at);
  const at = (offsetMs: number): string => new Date(armedAt + offsetMs).toISOString();

  return {
    request: structuredClone(request),
    events: [
      {
        id: `${request.id}:armed`,
        kind: 'armed',
        trigger: 'command',
        summary: `@${request.requester} commented /${request.merge_method} after ci and holds merge permission through CODEOWNERS`,
        created_at: request.requested_at,
      },
      {
        id: `${request.id}:wake`,
        kind: 'wake_received',
        trigger: 'webhook',
        event_name: 'check_suite',
        event_key: `check_suite:${request.head_sha.slice(0, 8)}:completed:success`,
        delivery_id: '8f3a1c7e-2b40-4d19-9a5e-71c0d2f4b8aa',
        summary:
          'A check_suite delivery reported completed checks and scheduled an immediate reconciliation',
        created_at: at(3 * 60_000),
      },
      {
        id: `${request.id}:reconcile`,
        kind: 'reconciliation_started',
        trigger: 'webhook',
        summary: 'Lease taken, pull request and check state read from GitHub',
        created_at: at(3 * 60_000 + 1_000),
      },
      {
        id: `${request.id}:observed`,
        kind: 'checks_observed',
        trigger: 'webhook',
        state: request.last_observed_state,
        event_key: `check_suite:${request.head_sha.slice(0, 8)}:completed:success`,
        summary: `All 11 checks green on ${request.head_sha.slice(0, 8)}; the previous observation was pending`,
        created_at: at(3 * 60_000 + 2_000),
      },
      {
        id: `${request.id}:quiet`,
        kind: 'reconciliation_started',
        trigger: 'quiet_period',
        summary: 'Quiet period started; merging unless a new check or commit arrives first',
        created_at: at(3 * 60_000 + 2_500),
      },
    ],
  };
}

function rootAuditEntries(state: MockState): AuditEntry[] {
  const workspaceEvents = state.targets.flatMap((target) =>
    target.audit.map((entry) => ({
      ...entry,
      id: `${target.value.id}-${entry.id}`,
      category: 'configuration' as const,
      target_id: target.value.id,
      workspace: target.value.account,
    })),
  );
  const primaryWorkspace = state.targets[0]?.value.account;
  const subject = state.users[0]?.account;
  const now = Date.now();
  const systemEvents: AuditEntry[] = [
    {
      id: 'root-runtime-1',
      category: 'runtime',
      actor: VIEWER,
      action: 'runtime.settings.saved',
      summary: 'Updated panel session lifetime',
      created_at: new Date(now - 7 * 60_000).toISOString(),
    },
    {
      id: 'root-elevation-1',
      category: 'elevation',
      workspace: primaryWorkspace,
      actor: VIEWER,
      elevation_id: 'elevation-204',
      action: 'elevation.started',
      summary: 'Started audited workspace access',
      created_at: new Date(now - 22 * 60_000).toISOString(),
    },
    {
      id: 'root-access-1',
      category: 'access',
      workspace: primaryWorkspace,
      actor: VIEWER,
      subject,
      action: 'target.access.updated',
      summary: 'Updated workspace access',
      created_at: new Date(now - 48 * 60_000).toISOString(),
    },
    {
      id: 'root-ownership-1',
      category: 'ownership',
      workspace: primaryWorkspace,
      actor: VIEWER,
      action: 'ownership.synced',
      summary: 'Synchronized workspace owners',
      created_at: new Date(now - 76 * 60_000).toISOString(),
    },
    {
      id: 'root-notification-1',
      category: 'notification',
      workspace: primaryWorkspace,
      actor: VIEWER,
      action: 'owner.notification.created',
      summary: 'Notified owners about elevated access',
      created_at: new Date(now - 80 * 60_000).toISOString(),
    },
  ];

  return [...workspaceEvents, ...state.runtime.audit, ...systemEvents].sort(
    (left, right) => Date.parse(right.created_at) - Date.parse(left.created_at),
  );
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
  throw new MockApiError(403, 'elevation_required', 'start elevated access for this workspace');
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
        'this user already has access to this workspace; change their role instead',
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

function scopedUserValue(state: MockState, targetId: string, user: PanelUser): PanelUser {
  const access = targetAccessFor(state, targetId).get(user.account.id);
  if (access === undefined) throw new MockApiError(404, 'not_found', 'workspace role not found');
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

function addAudit(
  target: MockTarget,
  action: string,
  summary: string,
  repository?: string,
  settingsCheckpointId?: string,
): void {
  const entry: AuditEntry = {
    id: `audit-${Date.now()}`,
    actor: VIEWER,
    action,
    summary,
    ...(settingsCheckpointId === undefined ? {} : { settings_checkpoint_id: settingsCheckpointId }),
    repository_full_name: repository,
    created_at: new Date().toISOString(),
  };
  target.audit.unshift(entry);
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
          summary: `suspended workspace access${user.target_access.suspension_reason === undefined ? '' : `: ${user.target_access.suspension_reason}`}`,
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
      summary: 'updated workspace access',
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

/**
 * A recognisable stand-in portrait: the account's initial on one neutral
 * ground, the same for every login - fixture pictures must never bring a
 * palette of their own into the shell. The initial alone tells them apart,
 * and the flat grey is what says "image rendered" against the tinted
 * monogram fallback.
 */
function devAvatarSVG(login: string): string {
  const letter = (login[0] ?? '?').toUpperCase();
  return (
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">` +
    `<rect width="64" height="64" fill="#3d434b"/>` +
    `<text x="32" y="43" text-anchor="middle" font-family="sans-serif" font-size="30" ` +
    `font-weight="700" fill="#e6e8eb">${letter}</text>` +
    `</svg>`
  );
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
  let page: string;
  try {
    page = await renderErrorDocument(state, status, code, message);
  } catch (error) {
    /* Loud rather than quiet. A mock that cannot borrow a page and answers with
       JSON instead looks exactly like a mock that decided the caller wanted JSON,
       which is the shape this spent a release wearing. */
    const reason = error instanceof Error ? error.message : String(error);
    respond(res, 500, { error: { code: 'mock_page_unavailable', message: reason } });
    return;
  }
  res.statusCode = status;
  res.setHeader('Content-Type', 'text/html; charset=utf-8');
  res.setHeader('Cache-Control', 'no-store');
  res.end(page);
}

/**
 * Mirrors `wantsDocument` in `internal/panel/error_page.go`, down to why it is not
 * a single header: `Sec-Fetch-Dest: document` settles a navigation, but its absence
 * settles nothing, because a service worker forwarding one through `fetch()` builds
 * a fresh request that carries no destination. The panel registers such a worker
 * here too, so this is not a production-only distinction.
 */
function wantsDocument(req: IncomingMessage): boolean {
  if (req.headers['sec-fetch-dest'] === 'document') return true;

  return (req.headers.accept ?? '')
    .split(',')
    .some((entry) => entry.split(';')[0]?.trim() === 'text/html');
}

async function renderErrorDocument(
  state: MockState,
  status: number,
  code: string,
  message: string,
): Promise<string> {
  const descriptor = escapeHtml(JSON.stringify({ status, code, message }));

  return (await state.shell())
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
