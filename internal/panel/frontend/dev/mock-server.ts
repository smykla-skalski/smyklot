import type { Server as HttpServer, IncomingMessage, ServerResponse } from 'node:http';
import type { Connect, Plugin } from 'vite';

import type {
  AuditEntry,
  ConfigKey,
  ConfigPatch,
  ConfigSources,
  ConfigValues,
  DeliveryFailure,
  Page,
  PanelAccount,
  PanelTarget,
  RepositoryDetail,
  RepositorySettingsInput,
  RepositorySummary,
  TargetSettingsInput,
} from '../src/lib/types';

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

interface MockState {
  signedIn: boolean;
  forceFailure: boolean;
  targets: MockTarget[];
  streams: Set<ServerResponse>;
}

function enabled(): boolean {
  return process.env.SMYKLOT_PANEL_DEV_MOCK === '1';
}

function seed(): MockState {
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
        enabledOverride: index % 3 === 0 ? true : index % 3 === 1 ? false : null,
        filePatch: index % 4 === 0 ? { command_prefix: `/${name} ` } : {},
        fileError: index % 7 === 0 ? 'line 4: unknown setting' : undefined,
        panelPatch: index % 5 === 0 ? { quiet_success: index % 2 === 0 } : {},
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
      occurred_at: iso(-42 * 60_000),
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

  return {
    signedIn: true,
    forceFailure: false,
    targets: [organization, personal],
    streams: new Set(),
  };
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
      return { base: '/', server: { open: '/' } };
    },
    transformIndexHtml(html) {
      if (!enabled()) return html;
      return html
        .replaceAll('/__smyklot_panel_base__', '')
        .replaceAll('__smyklot_panel_version__', 'dev')
        .replaceAll('__smyklot_panel_service__', 'local mock service');
    },
    configureServer(server) {
      if (enabled()) install(server.httpServer as DevHttpServer, server.middlewares);
    },
    configurePreviewServer(server) {
      if (enabled()) install(server.httpServer as DevHttpServer, server.middlewares);
    },
  };
}

function install(httpServer: DevHttpServer | undefined, middlewares: Connect.Server): void {
  if (httpServer === undefined) throw new Error('the mock dev server has no HTTP server');
  const state = seed();
  middlewares.use((req, res, next) => void handle(state, req, res, next));
}

async function handle(
  state: MockState,
  req: IncomingMessage,
  res: ServerResponse,
  next: Connect.NextFunction,
): Promise<void> {
  const parsed = new URL(req.url ?? '/', 'http://localhost');
  const path = parsed.pathname;
  const method = req.method ?? 'GET';

  if (path === '/' && method === 'GET') {
    applyScenario(state, parsed.searchParams.get('scenario'));
    next();
    return;
  }
  if (path.startsWith('/__smyklot_panel_base__')) {
    respond(res, 404, { error: { code: 'not_found', message: 'the mock panel is mounted at /' } });
    return;
  }
  if (path === route('/auth/github/start') && method === 'GET') {
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
    state.signedIn = false;
    respond(res, 204, null);
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
      respond(res, 200, { account: VIEWER, target_count: state.targets.length });
      return;
    }
    if (path === route('/api/v1/targets') && method === 'GET') {
      respond(res, 200, { targets: state.targets.map((target) => target.value) });
      return;
    }
    if (path === route('/api/v1/events') && method === 'GET') {
      res.writeHead(200, {
        'Cache-Control': 'no-store',
        Connection: 'keep-alive',
        'Content-Type': 'text/event-stream',
      });
      res.write('event: ready\ndata: {}\n\n');
      state.streams.add(res);
      req.once('close', () => state.streams.delete(res));
      return;
    }

    const targetSettings = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/settings$/);
    const repositories = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/repositories$/);
    const repository = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/repositories\/(?<repository>[^/]+)$/,
    );
    const repositorySettings = path.match(
      /^\/api\/v1\/targets\/(?<target>[^/]+)\/repositories\/(?<repository>[^/]+)\/settings$/,
    );
    const audit = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/audit$/);
    const failures = path.match(/^\/api\/v1\/targets\/(?<target>[^/]+)\/failures$/);

    if (targetSettings && method === 'PUT') {
      const target = findTarget(state, targetSettings.groups?.target ?? '');
      const input = await readBody<TargetSettingsInput>(req);
      requireRevision(target.value.revision, input.expected_revision);
      target.value.repository_default_enabled = input.repository_default_enabled;
      target.value.config_patch = structuredClone(input.config_patch);
      target.value.revision += 1;
      recomputeTarget(target);
      addAudit(target, 'target.settings.updated', 'updated account defaults');
      broadcast(state, { type: 'target', target_id: target.value.id });
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
        type: 'repository',
        target_id: target.value.id,
        repository_id: stored.detail.repository.id,
      });
      respond(res, 200, stored.detail);
      return;
    }
    if (audit && method === 'GET') {
      const target = findTarget(state, audit.groups?.target ?? '');
      const scope = parsed.searchParams.get('scope') ?? 'all';
      respond(
        res,
        200,
        historyPage(
          target.audit,
          parsed.searchParams,
          (entry) => entry.created_at,
          (entry, query) =>
            [
              entry.actor.display_name,
              entry.actor.login,
              entry.action,
              entry.summary,
              entry.repository_full_name ?? '',
            ].some((value) => value.toLocaleLowerCase().includes(query)),
          (entry) =>
            scope === 'all' ||
            (scope === 'account' && entry.repository_full_name === undefined) ||
            (scope === 'repositories' && entry.repository_full_name !== undefined),
        ),
      );
      return;
    }
    if (failures && method === 'GET') {
      const target = findTarget(state, failures.groups?.target ?? '');
      const kind = parsed.searchParams.get('kind') ?? 'all';
      respond(
        res,
        200,
        historyPage(
          target.failures,
          parsed.searchParams,
          (failure) => failure.occurred_at,
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
  if (scenario === 'empty') state.targets = [];
}

function findTarget(state: MockState, encodedId: string): MockTarget {
  const id = decodeURIComponent(encodedId);
  const target = state.targets.find((entry) => entry.value.id === id);
  if (target === undefined)
    throw new MockApiError(404, 'not_found', 'installation target not found');
  return target;
}

function findRepository(target: MockTarget, encodedId: string): MockRepository {
  const id = decodeURIComponent(encodedId);
  const repository = target.repositories.find((entry) => entry.detail.repository.id === id);
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

function historyPage<T>(
  items: T[],
  parameters: URLSearchParams,
  timestamp: (item: T) => string,
  matches: (item: T, query: string) => boolean,
  visible: (item: T) => boolean,
): Page<T> {
  const requestedLimit = Number.parseInt(parameters.get('limit') ?? '', 10);
  const limit =
    Number.isFinite(requestedLimit) && requestedLimit > 0
      ? Math.min(requestedLimit, 100)
      : DEFAULT_PAGE_SIZE;
  const query = (parameters.get('q') ?? '').trim().toLocaleLowerCase();
  const ordered = items
    .filter((item) => visible(item) && (query === '' || matches(item, query)))
    .sort((left, right) => timestamp(left).localeCompare(timestamp(right)));
  if (parameters.get('sort') !== 'oldest') ordered.reverse();

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
  const frame = JSON.stringify(event);
  for (const stream of state.streams) stream.write(`data: ${frame}\n\n`);
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

export default mockServer;
