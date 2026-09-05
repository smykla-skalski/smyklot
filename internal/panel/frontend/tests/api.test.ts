import { describe, expect, it } from 'vitest';

import { PanelApiError, createPanelApi } from '../src/lib/api';
import { CONFIG_KEYS } from '../src/lib/config';
import { defaultFormattingPolicy, formattingSources } from '../src/lib/formatting';
import type {
  ConfigSources,
  ConfigValues,
  PanelTarget,
  RepositoryDetail,
  SettingsCheckpoint,
} from '../src/lib/types';

interface RecordedCall {
  url: string;
  init?: RequestInit;
}

function stubFetch(responses: Response[]): {
  calls: RecordedCall[];
  fetch: (input: string, init?: RequestInit) => Promise<Response>;
} {
  const calls: RecordedCall[] = [];
  const queue = [...responses];
  return {
    calls,
    fetch: (url, init) => {
      calls.push({ url, init });
      const next = queue.shift();
      if (next === undefined) throw new Error(`unexpected request to ${url}`);
      return Promise.resolve(next);
    },
  };
}

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

function emptyResponse(status: number): Response {
  return new Response(null, { status });
}

const CONFIG: ConfigValues = {
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

const SOURCES = Object.fromEntries(CONFIG_KEYS.map((key) => [key, 'process'])) as ConfigSources;

const VIEWER = {
  account: {
    id: '1001',
    provider: 'github:https://api.github.com',
    subject_id: '1001',
    login: 'ada',
    display_name: 'Ada Lovelace',
    avatar_url: null,
  },
  system_role: 'super_root' as const,
  status: 'active' as const,
  target_count: 1,
};

const OWNER_CAPABILITIES = {
  read: true,
  write: true,
  manage_target_users: true,
};

const TARGET: PanelTarget = {
  id: '2001',
  installation_id: '3001',
  type: 'Organization',
  account: { ...VIEWER.account, id: '2001', subject_id: '2001', login: 'example' },
  repository_default_enabled: false,
  pending_ci_mode_default: 'checks',
  pending_ci_branch_patterns_default: { include: ['~DEFAULT_BRANCH'], exclude: [] },
  pending_ci_quiet_period_seconds_override: null,
  pending_ci_quiet_period_seconds_inherited: 30,
  path_index_interval_seconds_override: null,
  path_index_interval_seconds_inherited: 3600,
  pending_ci_permissions: {
    checks_write: true,
    administration_write: true,
    merge_queues_read: true,
    commit_statuses_read: true,
  },
  config_patch: {},
  inherited_config: CONFIG,
  effective_config: CONFIG,
  config_sources: SOURCES,
  formatting_sources: formattingSources('process'),
  revision: 1,
  repository_counts: { total: 1, enabled: 0, disabled: 1 },
  effective_role: 'owner',
  access_source: 'owner',
  capabilities: OWNER_CAPABILITIES,
};

const REPOSITORY = {
  id: '4001',
  name: 'api',
  full_name: 'example/api',
  private: false,
  default_branch: 'main',
  available: true,
  enabled_override: null,
  effective_enabled: false,
  enabled_source: 'target' as const,
  pending_ci_mode: 'checks' as const,
  pending_ci_mode_source: 'target' as const,
  config_override_count: 0,
  config_file_status: 'missing' as const,
  updated_at: '2026-08-08T10:00:00Z',
};

describe('file rendering', () => {
  it('posts the complete typed render request to the encoded workspace route', async () => {
    const response = {
      valid: true,
      final_content: '{}\n',
      matches_formatting: false,
      diagnostics: [],
    };
    const stub = stubFetch([jsonResponse(200, response)]);
    const api = createPanelApi('/panel', stub.fetch);
    const input = {
      path: 'renovate.json',
      draft_content: '{}',
      template_formatting: { common: { final_newline: 'insert' as const } },
    };

    await expect(api.renderSyncFile('target.1', input)).resolves.toEqual(response);
    expect(stub.calls[0]).toMatchObject({
      url: '/panel/api/v1/targets/target%2E1/sync/files/render',
      init: {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      },
    });
  });
});

const DETAIL: RepositoryDetail = {
  repository: REPOSITORY,
  config_patch: {},
  inherited_config: CONFIG,
  effective_config: CONFIG,
  config_sources: SOURCES,
  formatting_sources: formattingSources('process'),
  config_file_patch: {},
  config_migration: 'none' as const,
  ignore_repository_file: false,
  pending_ci_mode_override: null,
  pending_ci_mode_inherited: 'checks',
  pending_ci_branch_patterns_override: null,
  pending_ci_branch_patterns_inherited: { include: ['~DEFAULT_BRANCH'], exclude: [] },
  pending_ci_quiet_period_seconds_override: null,
  pending_ci_quiet_period_seconds_inherited: 30,
  path_index_interval_seconds_override: null,
  path_index_interval_seconds_inherited: 3600,
  revision: 1,
};

describe('session', () => {
  it('returns the signed-in owner with same-origin credentials', async () => {
    const stub = stubFetch([jsonResponse(200, VIEWER)]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchViewer()).resolves.toEqual(VIEWER);
    expect(stub.calls[0]).toMatchObject({
      url: '/panel/api/v1/session',
      init: { credentials: 'same-origin' },
    });
  });

  it('treats an absent session as the signed-out state', async () => {
    const stub = stubFetch([
      jsonResponse(401, { error: { code: 'unauthenticated', message: 'sign in first' } }),
    ]);
    await expect(createPanelApi('/panel', stub.fetch).fetchViewer()).resolves.toBeNull();
  });

  it('preserves a structured server error', async () => {
    const stub = stubFetch([
      jsonResponse(503, { error: { code: 'storage', message: 'storage is unavailable' } }),
    ]);
    await expect(createPanelApi('/panel', stub.fetch).fetchViewer()).rejects.toMatchObject({
      status: 503,
      code: 'storage',
      message: 'storage is unavailable',
    });
  });
});

describe('targets and repositories', () => {
  it('unwraps target and repository collections', async () => {
    const stub = stubFetch([
      jsonResponse(200, { targets: [TARGET] }),
      jsonResponse(200, { items: [REPOSITORY], next_cursor: null, total: 1 }),
      jsonResponse(200, DETAIL),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchTargets()).resolves.toEqual([TARGET]);
    await expect(
      api.fetchRepositories('2001', {
        query: '',
        sort: 'name_asc',
        limit: 20,
        state: 'all',
        files: [],
        setting: { mode: 'all' },
      }),
    ).resolves.toEqual({ items: [REPOSITORY], next_cursor: null, total: 1 });
    await expect(api.fetchRepository('2001', '4001')).resolves.toEqual(DETAIL);
    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/targets',
      '/panel/api/v1/targets/2001/repositories?sort=name_asc&limit=20&state=all',
      '/panel/api/v1/targets/2001/repositories/4001',
    ]);
  });

  it('encodes repository pagination, search, sorting, and filters', async () => {
    const stub = stubFetch([
      jsonResponse(200, { items: [REPOSITORY], next_cursor: '40', total: 62 }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.fetchRepositories('2001', {
      cursor: '20',
      query: 'service api',
      sort: 'newest',
      limit: 20,
      state: 'enabled',
      files: ['invalid', 'missing'],
      setting: { mode: 'keys', keys: ['quiet_success', 'command_prefix'] },
    });

    expect(stub.calls[0]?.url).toBe(
      '/panel/api/v1/targets/2001/repositories?cursor=20&q=service+api&sort=newest&limit=20&state=enabled&file=invalid&file=missing&setting=quiet_success&setting=command_prefix',
    );
  });

  it('saves every workspace settings resource through one literal-preserving PUT', async () => {
    const answer =
      '{"checkpoint_id":"checkpoint.1","sync_configs":[{"target_id":"target/1",' +
      '"kind":"files","enabled":true,"document":{"app_id":12345678901234567890},' +
      '"revision":8}]}';
    const stub = stubFetch([
      new Response(answer, { status: 200, headers: { 'content-type': 'application/json' } }),
      jsonResponse(200, { checkpoint_id: 'checkpoint.2' }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);
    const input = {
      target: {
        repository_default_enabled: true,
        pending_ci_mode_default: 'checks' as const,
        pending_ci_branch_patterns_default: { include: ['~DEFAULT_BRANCH'], exclude: [] },
        pending_ci_quiet_period_seconds_override: null,
        path_index_interval_seconds_override: 3600,
        config_patch: { quiet_success: true },
        expected_revision: 4,
      },
      sync_configs: [
        {
          kind: 'files' as const,
          enabled: true,
          document: { templates: [] },
          expected_revision: 7,
        },
      ],
    };

    const saved = await api.saveWorkspaceSettings('target/1', input);
    await api.saveRootWorkspaceSettings('target/1', input);

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/targets/target%2F1/settings',
      '/panel/api/v1/root/workspaces/target%2F1/settings',
    ]);
    expect(stub.calls.every((call) => call.init?.method === 'PUT')).toBe(true);
    expect(JSON.parse(String(stub.calls[0]?.init?.body))).toEqual(input);
    expect(JSON.stringify(saved.sync_configs?.[0]?.document)).toBe(
      '{"app_id":12345678901234567890}',
    );
  });

  it('keeps structured batch conflicts and their latest documents', async () => {
    const conflict =
      '{"error":{"code":"conflict","message":"settings changed in another session",' +
      '"conflicts":[{"resource":"sync_config","target_id":"target.1","kind":"files",' +
      '"expected_revision":7,"actual_revision":8,"latest":{"target_id":"target.1",' +
      '"kind":"files","enabled":true,"document":{"app_id":12345678901234567890},' +
      '"revision":8}}]}}';
    const stub = stubFetch([
      new Response(conflict, { status: 409, headers: { 'content-type': 'application/json' } }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    let failure: unknown;
    try {
      await api.saveWorkspaceSettings('target.1', {
        sync_configs: [
          {
            kind: 'files',
            enabled: true,
            document: { templates: [] },
            expected_revision: 7,
          },
        ],
      });
    } catch (error) {
      failure = error;
    }

    expect(failure).toMatchObject({
      status: 409,
      code: 'conflict',
      conflicts: [
        {
          resource: 'sync_config',
          target_id: 'target.1',
          kind: 'files',
          expected_revision: 7,
          actual_revision: 8,
        },
      ],
    });
    const latest = (failure as PanelApiError).conflicts[0]?.latest;
    expect(JSON.stringify(latest)).toContain('12345678901234567890');
  });

  it('encodes slashes and traversal segments as path data', async () => {
    const stub = stubFetch([jsonResponse(200, DETAIL)]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.fetchRepository('a/b', '..');

    expect(stub.calls[0]?.url).toBe('/panel/api/v1/targets/a%2Fb/repositories/%2E%2E');
  });

  it('inspects and restores selected workspace settings', async () => {
    const checkpoint =
      '{"id":"checkpoint/1","action":"installation.settings.saved",' +
      '"actor":{"id":"1001","provider":"github:https://api.github.com",' +
      '"subject_id":"1001","login":"ada","display_name":"Ada Lovelace",' +
      '"avatar_url":null},"created_at":"2026-08-23T08:00:00Z",' +
      '"affected_kinds":["sync_config"],"items":[{"kind":"sync_config",' +
      '"sync_kind":"files","document_version":1,"before":{"available":true,' +
      '"state":null,"differs":true,"restorable":true},"after":{"available":true,' +
      '"state":{"document":{"app_id":12345678901234567890},"digest":"after",' +
      '"revision":7},"differs":false,"restorable":true},"current":{"document":' +
      '{"app_id":12345678901234567890},"digest":"after","revision":7},' +
      '"changed":true}]}';
    const restored = { sync_configs: [], checkpoint_id: 'checkpoint/2' };
    const stub = stubFetch([
      new Response(checkpoint, { status: 200, headers: { 'content-type': 'application/json' } }),
      jsonResponse(200, restored),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    const inspected = await api.fetchWorkspaceSettingsCheckpoint('target/1', 'checkpoint/1');
    await expect(
      api.restoreWorkspaceSettingsCheckpoint('target/1', 'checkpoint/1', {
        state: 'before',
        selections: [{ kind: 'sync_config', sync_kind: 'files', expected_revision: 7 }],
      }),
    ).resolves.toEqual(restored);

    expect(JSON.stringify(inspected.items[0]?.after.state?.document)).toBe(
      '{"app_id":12345678901234567890}',
    );
    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/targets/target%2F1/settings/checkpoints/checkpoint%2F1',
      '/panel/api/v1/targets/target%2F1/settings/checkpoints/checkpoint%2F1/restore',
    ]);
    expect(stub.calls[1]?.init?.method).toBe('POST');
    expect(JSON.parse(String(stub.calls[1]?.init?.body))).toEqual({
      state: 'before',
      selections: [{ kind: 'sync_config', sync_kind: 'files', expected_revision: 7 }],
    });
  });
});

describe('Root workspace access', () => {
  it('inspects and restores workspace settings through Root routes', async () => {
    const checkpoint: SettingsCheckpoint = {
      id: 'checkpoint/1',
      action: 'installation.settings.saved',
      actor: VIEWER.account,
      created_at: '2026-08-23T08:00:00Z',
      affected_kinds: ['target'],
      items: [],
    };
    const restored = { target: TARGET, checkpoint_id: 'checkpoint/2' };
    const stub = stubFetch([jsonResponse(200, checkpoint), jsonResponse(200, restored)]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.fetchRootWorkspaceSettingsCheckpoint('target/1', 'checkpoint/1');
    await api.restoreRootWorkspaceSettingsCheckpoint('target/1', 'checkpoint/1', {
      state: 'after',
      selections: [{ kind: 'target', expected_revision: 2 }],
    });

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/root/workspaces/target%2F1/settings/checkpoints/checkpoint%2F1',
      '/panel/api/v1/root/workspaces/target%2F1/settings/checkpoints/checkpoint%2F1/restore',
    ]);
    expect(stub.calls[1]?.init?.method).toBe('POST');
    expect(JSON.parse(String(stub.calls[1]?.init?.body))).toEqual({
      state: 'after',
      selections: [{ kind: 'target', expected_revision: 2 }],
    });
  });

  it('runs the Root catalog synchronization endpoint', async () => {
    const stub = stubFetch([jsonResponse(200, { target_ids: ['target.1', 'target.2'] })]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.syncRootWorkspaces()).resolves.toEqual(['target.1', 'target.2']);
    expect(stub.calls[0]?.url).toBe('/panel/api/v1/root/workspaces/sync');
    expect(stub.calls[0]?.init?.method).toBe('POST');
  });

  it('checks and cancels pending CI requests with optimistic revisions', async () => {
    const request = {
      id: 'pending/1',
      repository_full_name: 'example/api',
      pull_request: 42,
      head_sha: 'abc123',
      merge_method: 'squash' as const,
      required_checks_only: false,
      requester: 'ada',
      lifecycle: 'armed' as const,
      schedule: 'active' as const,
      next_check_at: '2026-08-15T10:00:00Z',
      next_check_trigger: 'webhook' as const,
      last_observed_state: 'pending',
      reason: '',
      requested_at: '2026-08-15T09:00:00Z',
      updated_at: '2026-08-15T09:55:00Z',
      cleanup_pending: false,
      revision: 4,
    };
    const stub = stubFetch([jsonResponse(200, request), jsonResponse(200, request)]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.checkRootPendingCI(request.id, request.revision);
    await api.cancelRootPendingCI(request.id, request.revision);

    expect(stub.calls).toEqual([
      {
        url: '/panel/api/v1/root/pending-ci/pending%2F1/check',
        init: {
          method: 'POST',
          credentials: 'same-origin',
          headers: { 'Content-Type': 'application/json' },
          body: '{"expected_revision":4}',
        },
      },
      {
        url: '/panel/api/v1/root/pending-ci/pending%2F1',
        init: {
          method: 'DELETE',
          credentials: 'same-origin',
          headers: { 'Content-Type': 'application/json' },
          body: '{"expected_revision":4}',
        },
      },
    ]);
  });

  it('reads the durable pending CI timeline', async () => {
    const detail = {
      request: {
        id: 'pending/1',
        repository_full_name: 'example/api',
        pull_request: 42,
        head_sha: 'abc123',
        merge_method: 'squash' as const,
        required_checks_only: false,
        requester: 'ada',
        lifecycle: 'merged' as const,
        schedule: 'active' as const,
        next_check_at: '2026-08-15T10:00:30Z',
        next_check_trigger: 'cleanup' as const,
        last_observed_state: 'passing',
        reason: 'merged after CI passed',
        requested_at: '2026-08-15T09:00:00Z',
        updated_at: '2026-08-15T10:00:30Z',
        finished_at: '2026-08-15T10:00:30Z',
        cleanup_pending: false,
        revision: 8,
      },
      events: [
        {
          id: 'event/1',
          kind: 'wake_received' as const,
          trigger: 'webhook' as const,
          event_name: 'check_suite',
          delivery_id: 'delivery-123',
          summary: 'Received a CI state webhook',
          created_at: '2026-08-15T10:00:00Z',
        },
      ],
    };
    const stub = stubFetch([jsonResponse(200, detail)]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchRootPendingCI('pending/1')).resolves.toEqual(detail);
    expect(stub.calls).toEqual([
      {
        url: '/panel/api/v1/root/pending-ci/pending%2F1',
        init: { credentials: 'same-origin' },
      },
    ]);
  });

  it('uses dedicated Root reads, writes, and elevation routes', async () => {
    const elevation = {
      id: 'elevation.1',
      target_id: 'target.1',
      reason: 'Investigating a failed delivery',
      started_at: '2026-08-10T10:00:00Z',
      expires_at: '2026-08-10T10:15:00Z',
    };
    const workspace = {
      id: 'target.1',
      installation_id: '3001',
      type: 'Organization' as const,
      account: TARGET.account,
      available: true,
      owned_by_viewer: true,
      repository_counts: TARGET.repository_counts,
      delivery_health: { failed: 0 },
      ownership: {
        source: 'organization_admin' as const,
        status: 'fresh' as const,
        synced_at: '2026-08-10T10:00:00Z',
        owner_count: 1,
        stale: false,
      },
    };
    const stub = stubFetch([
      jsonResponse(200, {
        service: {
          status: 'healthy',
          version: '1.0.0',
          service_host: 'smyklot.example',
          uptime_seconds: 120,
          storage: 'healthy',
        },
        catalog: { workspaces: 1, repositories: 1, enabled_repositories: 0 },
        ownership: { fresh: 1, stale: 0, permission_pending: 0, error: 0 },
        active_elevations: 0,
        unread_security_events: 0,
        recent_failures: [],
      }),
      jsonResponse(200, { workspaces: [workspace] }),
      jsonResponse(200, TARGET),
      jsonResponse(200, { items: [REPOSITORY], next_cursor: null, total: 1 }),
      jsonResponse(200, DETAIL),
      jsonResponse(404, { error: { code: 'not_found', message: 'not active' } }),
      jsonResponse(201, elevation),
      jsonResponse(200, { ...elevation, ended_at: '2026-08-10T10:05:00Z' }),
      jsonResponse(200, { items: [], next_cursor: null, total: 0 }),
      emptyResponse(204),
      jsonResponse(200, {
        since: '2026-08-10T10:00:00Z',
        until: '2026-08-17T10:00:00Z',
        metrics: { query: [], ledger: [], lane: [], database: [] },
      }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);
    const repositoryPage = {
      query: '',
      sort: 'name_asc' as const,
      limit: 20,
      state: 'all' as const,
      files: [],
      setting: { mode: 'all' as const },
    };

    await expect(api.fetchRootOverview()).resolves.toMatchObject({
      service: { status: 'healthy', version: '1.0.0' },
      catalog: { workspaces: 1, repositories: 1 },
    });
    await expect(api.fetchRootWorkspaces()).resolves.toEqual([workspace]);
    await api.fetchRootTargetSettings('target.1');
    await api.fetchRootRepositories('target.1', repositoryPage);
    await api.fetchRootRepository('target.1', 'repo.1');
    await expect(api.fetchRootElevation('target.1')).rejects.toMatchObject({
      status: 404,
      code: 'not_found',
    });
    await api.beginRootElevation('target.1', {
      acknowledged: true,
      reason: 'Investigating a failed delivery',
    });
    await api.endRootElevation('elevation.1');
    await api.fetchRootUsers({
      cursor: '20',
      query: 'ada',
      sort: 'role_desc',
      limit: 20,
      systemRoles: ['root', 'super_root'],
      statuses: ['active', 'banned'],
    });
    await api.updateRootUser('account.1', { system_role: 'root', expected_revision: 2 });
    await api.fetchRootPerformance(168);

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/root/overview',
      '/panel/api/v1/root/workspaces',
      '/panel/api/v1/root/workspaces/target%2E1/settings',
      '/panel/api/v1/root/workspaces/target%2E1/repositories?sort=name_asc&limit=20&state=all',
      '/panel/api/v1/root/workspaces/target%2E1/repositories/repo%2E1',
      '/panel/api/v1/root/workspaces/target%2E1/elevation',
      '/panel/api/v1/root/workspaces/target%2E1/elevation',
      '/panel/api/v1/root/elevations/elevation%2E1',
      '/panel/api/v1/root/access/users?cursor=20&q=ada&sort=role_desc&limit=20&system_role=root&system_role=super_root&status=active&status=banned',
      '/panel/api/v1/root/access/users/account%2E1',
      '/panel/api/v1/root/performance?window=168',
    ]);
    expect(stub.calls[6]?.init?.method).toBe('POST');
    expect(JSON.parse(String(stub.calls[6]?.init?.body))).toEqual({
      acknowledged: true,
      reason: 'Investigating a failed delivery',
    });
    expect(stub.calls[7]?.init?.method).toBe('DELETE');
    expect(stub.calls[9]?.init?.method).toBe('PUT');
    expect(JSON.parse(String(stub.calls[9]?.init?.body))).toEqual({
      system_role: 'root',
      expected_revision: 2,
    });
  });

  it('uses workspace-scoped Root access and history endpoints', async () => {
    const user = {
      account: VIEWER.account,
      system_role: 'none' as const,
      status: 'active' as const,
      revision: 1,
      created_at: '2026-08-08T10:00:00Z',
      updated_at: '2026-08-08T10:00:00Z',
      manageable: true,
    };
    const invitation = {
      id: 'invite.1',
      account: VIEWER.account,
      role: 'viewer' as const,
      status: 'pending' as const,
      expires_at: '2026-08-15T10:00:00Z',
      created_by: VIEWER.account,
      created_at: '2026-08-08T10:00:00Z',
    };
    const emptyPage = { items: [], next_cursor: null, total: 0 };
    const stub = stubFetch([
      jsonResponse(200, { items: [user], next_cursor: null, total: 1 }),
      jsonResponse(201, user),
      jsonResponse(200, user),
      jsonResponse(200, { decisions: [] }),
      jsonResponse(200, { items: [invitation], next_cursor: null, total: 1 }),
      jsonResponse(201, invitation),
      jsonResponse(200, invitation),
      jsonResponse(200, { ...invitation, status: 'revoked' }),
      jsonResponse(200, emptyPage),
      jsonResponse(200, emptyPage),
    ]);
    const api = createPanelApi('/panel', stub.fetch);
    const userPage = {
      query: 'ada',
      sort: 'role_desc' as const,
      limit: 20,
      roles: ['editor' as const],
      statuses: ['active' as const],
    };
    const invitationPage = {
      query: 'ada',
      sort: 'expiry_soonest' as const,
      limit: 20,
      roles: ['viewer' as const],
      statuses: ['pending' as const],
    };

    await api.fetchRootTargetUsers('target.1', userPage);
    await api.addRootTargetUser('target.1', { login: 'ada', role: 'viewer' });
    await api.updateRootTargetUser('target.1', 'github:user:1', {
      role: 'editor',
      suspended: false,
      expected_revision: 1,
    });
    await api.fetchRootTargetUserDecisions('github:user:1', 'target.1');
    await api.fetchRootTargetInvitations('target.1', invitationPage);
    await api.createRootTargetInvitation('target.1', {
      login: 'ada',
      role: 'viewer',
      expires_in_days: 7,
    });
    await api.reissueRootTargetInvitation('target.1', 'invite.1', 30);
    await api.revokeRootTargetInvitation('target.1', 'invite.1');
    await api.fetchRootTargetAudit('target.1', {
      query: '',
      sort: 'newest',
      limit: 20,
      scope: 'all',
      change: 'all',
    });
    await api.fetchRootTargetFailures('target.1', {
      query: '',
      sort: 'newest',
      limit: 20,
      kind: 'all',
    });

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/root/workspaces/target%2E1/users?q=ada&sort=role_desc&limit=20&role=editor&status=active',
      '/panel/api/v1/root/workspaces/target%2E1/users',
      '/panel/api/v1/root/workspaces/target%2E1/users/github%3Auser%3A1',
      '/panel/api/v1/root/workspaces/target%2E1/users/github%3Auser%3A1/decisions',
      '/panel/api/v1/root/workspaces/target%2E1/invitations?q=ada&sort=expiry_soonest&limit=20&role=viewer&status=pending',
      '/panel/api/v1/root/workspaces/target%2E1/invitations',
      '/panel/api/v1/root/workspaces/target%2E1/invitations/invite%2E1/reissue',
      '/panel/api/v1/root/workspaces/target%2E1/invitations/invite%2E1',
      '/panel/api/v1/root/workspaces/target%2E1/audit?sort=newest&limit=20&scope=all&change=all',
      '/panel/api/v1/root/workspaces/target%2E1/failures?sort=newest&limit=20&kind=all',
    ]);
  });
});

describe('Root invitations', () => {
  it('lists and manages system-role invitations through Root routes', async () => {
    const invitation = {
      id: 'root.invite.1',
      account: { ...VIEWER.account, id: 'root.2', subject_id: 'root.2', login: 'grace' },
      system_role: 'root' as const,
      status: 'pending' as const,
      expires_at: '2026-08-17T10:00:00Z',
      created_by: VIEWER.account,
      created_at: '2026-08-10T10:00:00Z',
      invite_url: 'https://smyklot.example/invite/root-token',
    };
    const stub = stubFetch([
      jsonResponse(200, { items: [invitation], next_cursor: null, total: 1 }),
      jsonResponse(201, invitation),
      jsonResponse(200, invitation),
      jsonResponse(200, { ...invitation, status: 'revoked' }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.fetchRootInvitations({
      cursor: '20',
      query: 'grace',
      sort: 'expiry_soonest',
      limit: 10,
      roles: [],
      statuses: ['pending', 'expired'],
    });
    await api.createRootInvitation({ login: 'grace', expires_in_days: 7 });
    await api.reissueRootInvitation('root.invite.1', 30);
    await api.revokeRootInvitation('root.invite.1');

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/root/access/invitations?cursor=20&q=grace&sort=expiry_soonest&limit=10&status=pending&status=expired',
      '/panel/api/v1/root/access/invitations',
      '/panel/api/v1/root/access/invitations/root%2Einvite%2E1/reissue',
      '/panel/api/v1/root/access/invitations/root%2Einvite%2E1',
    ]);
    expect(stub.calls.map((call) => call.init?.method)).toEqual([
      undefined,
      'POST',
      'POST',
      'DELETE',
    ]);
    expect(JSON.parse(String(stub.calls[1]?.init?.body))).toEqual({
      login: 'grace',
      expires_in_days: 7,
    });
    expect(JSON.parse(String(stub.calls[2]?.init?.body))).toEqual({ expires_in_days: 30 });
  });
});

describe('Root runtime settings', () => {
  it('reads and replaces live runtime overrides through the Root route', async () => {
    const runtime = {
      behavior_defaults: { deployment: CONFIG, override: null, effective: CONFIG },
      log_level: { deployment: 'info', override: null, effective: 'info' },
      reaction_poll_interval: {
        deployment_seconds: 300,
        override_seconds: null,
        effective_seconds: 300,
      },
      merge_after_ci_quiet_period: {
        deployment_seconds: 30,
        override_seconds: null,
        effective_seconds: 30,
      },
      path_index_interval: {
        deployment_seconds: 3_600,
        override_seconds: null,
        effective_seconds: 3_600,
      },
      session_lifetime: {
        deployment_seconds: 86_400,
        override_seconds: null,
        effective_seconds: 86_400,
      },
      revision: 3,
      service: {
        version: '1.0.0',
        uptime_seconds: 120,
        storage: 'healthy',
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
    const updated = {
      ...runtime,
      log_level: { ...runtime.log_level, override: 'debug', effective: 'debug' },
      reaction_poll_interval: {
        ...runtime.reaction_poll_interval,
        override_seconds: 90,
        effective_seconds: 90,
      },
      merge_after_ci_quiet_period: {
        ...runtime.merge_after_ci_quiet_period,
        override_seconds: 45,
        effective_seconds: 45,
      },
      session_lifetime: {
        ...runtime.session_lifetime,
        override_seconds: 3_600,
        effective_seconds: 3_600,
      },
      revision: 4,
    };
    const stub = stubFetch([jsonResponse(200, runtime), jsonResponse(200, updated)]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchRootRuntimeSettings()).resolves.toEqual(runtime);
    await expect(
      api.saveRootRuntimeSettings({
        bot_config: null,
        log_level: 'debug',
        reaction_poll_interval_seconds: 90,
        merge_after_ci_quiet_period_seconds: 45,
        path_index_interval_seconds: 7_200,
        session_ttl_seconds: 3_600,
        expected_revision: 3,
      }),
    ).resolves.toEqual(updated);

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/root/runtime/settings',
      '/panel/api/v1/root/runtime/settings',
    ]);
    expect(stub.calls[1]?.init?.method).toBe('PUT');
    expect(JSON.parse(String(stub.calls[1]?.init?.body))).toEqual({
      bot_config: null,
      log_level: 'debug',
      reaction_poll_interval_seconds: 90,
      merge_after_ci_quiet_period_seconds: 45,
      path_index_interval_seconds: 7_200,
      session_ttl_seconds: 3_600,
      expected_revision: 3,
    });
  });

  it('inspects and restores Root runtime checkpoints through the canonical route', async () => {
    const checkpoint: SettingsCheckpoint = {
      id: 'checkpoint/1',
      action: 'runtime.settings.saved',
      actor: VIEWER.account,
      created_at: '2026-08-23T08:00:00Z',
      affected_kinds: ['runtime'],
      items: [
        {
          kind: 'runtime',
          document_version: 1,
          before: {
            available: true,
            state: null,
            differs: true,
            restorable: true,
          },
          after: {
            available: true,
            state: {
              document: {
                bot_config: null,
                log_level: 'debug',
                poll_interval: null,
                pending_ci_quiet_period: 45_000_000_000,
                path_index_interval: null,
                session_ttl: null,
              },
              digest: 'after',
              revision: 4,
            },
            differs: true,
            restorable: true,
          },
          current: null,
          changed: true,
        },
      ],
    };
    const restored = { revision: 5, checkpoint_id: 'checkpoint/2' };
    const stub = stubFetch([jsonResponse(200, checkpoint), jsonResponse(200, restored)]);
    const api = createPanelApi('/panel', stub.fetch);
    const inspected = structuredClone(checkpoint);
    inspected.items[0]!.after.state!.document.pending_ci_quiet_period = {
      rawJSON: '45000000000',
    };

    await expect(api.fetchRootRuntimeSettingsCheckpoint('checkpoint/1')).resolves.toEqual(
      inspected,
    );
    await expect(
      api.restoreRootRuntimeSettingsCheckpoint('checkpoint/1', {
        state: 'after',
        selections: [{ kind: 'runtime', expected_revision: 4 }],
      }),
    ).resolves.toEqual(restored);

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/root/runtime/settings/checkpoints/checkpoint%2F1',
      '/panel/api/v1/root/runtime/settings/checkpoints/checkpoint%2F1/restore',
    ]);
    expect(stub.calls[1]?.init?.method).toBe('POST');
    expect(JSON.parse(String(stub.calls[1]?.init?.body))).toEqual({
      state: 'after',
      selections: [{ kind: 'runtime', expected_revision: 4 }],
    });
  });
});

describe('settings checkpoint baselines', () => {
  it('fetches each baseline from its canonical settings route', async () => {
    const workspaceBaseline: SettingsCheckpoint = {
      id: 'workspace-baseline',
      action: 'installation.settings.baseline',
      actor: VIEWER.account,
      created_at: '2026-08-23T07:00:00Z',
      affected_kinds: [],
      items: [],
    };
    const runtimeBaseline: SettingsCheckpoint = {
      ...workspaceBaseline,
      id: 'runtime-baseline',
      action: 'runtime.settings.baseline',
    };
    const stub = stubFetch([
      jsonResponse(200, workspaceBaseline),
      jsonResponse(200, workspaceBaseline),
      jsonResponse(200, runtimeBaseline),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchWorkspaceSettingsBaseline('target/1')).resolves.toEqual(
      workspaceBaseline,
    );
    await expect(api.fetchRootWorkspaceSettingsBaseline('target/1')).resolves.toEqual(
      workspaceBaseline,
    );
    await expect(api.fetchRootRuntimeSettingsBaseline()).resolves.toEqual(runtimeBaseline);

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/targets/target%2F1/settings/checkpoints/baseline',
      '/panel/api/v1/root/workspaces/target%2F1/settings/checkpoints/baseline',
      '/panel/api/v1/root/runtime/settings/checkpoints/baseline',
    ]);
  });
});

describe('security notifications', () => {
  it('pages the Owner inbox and marks notifications read', async () => {
    const notification = {
      id: '12',
      workspace: TARGET.account,
      actor: VIEWER.account,
      elevation_id: 'elevation.1',
      audit_event_id: '25',
      action: 'installation.settings.saved',
      reason: 'Production incident',
      created_at: '2026-08-10T10:00:00Z',
    };
    const stub = stubFetch([
      jsonResponse(200, {
        items: [notification],
        next_cursor: '20',
        total: 21,
        unread: 1,
      }),
      jsonResponse(200, { ...notification, read_at: '2026-08-10T10:05:00Z' }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.fetchNotifications({ cursor: '10', limit: 10 });
    await api.markNotificationRead('notification.12');

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/notifications?limit=10&cursor=10',
      '/panel/api/v1/notifications/notification%2E12/read',
    ]);
    expect(stub.calls[1]?.init?.method).toBe('PUT');
  });
});

describe('user management', () => {
  it('uses scoped user endpoints and preserves nullable inheritance', async () => {
    const user = {
      account: VIEWER.account,
      system_role: 'none' as const,
      status: 'active' as const,
      revision: 1,
      created_at: '2026-08-08T10:00:00Z',
      updated_at: '2026-08-08T10:00:00Z',
      manageable: true,
    };
    const stub = stubFetch([
      jsonResponse(200, { items: [user], next_cursor: null, total: 1 }),
      jsonResponse(201, user),
      jsonResponse(200, user),
      jsonResponse(200, { decisions: [] }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    const page = {
      query: 'ada user',
      sort: 'name_asc' as const,
      limit: 20,
      roles: ['editor' as const],
      statuses: ['active' as const],
    };

    await api.fetchTargetUsers('target.1', { ...page, cursor: '20' });
    await api.addTargetUser('target.1', { login: 'ada', role: 'viewer' });
    await api.updateTargetUser('target.1', 'github:user:1', {
      role: null,
      suspended: false,
      expected_revision: 1,
    });
    await api.fetchUserDecisions('github:user:1', 'target.1');

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/targets/target%2E1/users?cursor=20&q=ada+user&sort=name_asc&limit=20&role=editor&status=active',
      '/panel/api/v1/targets/target%2E1/users',
      '/panel/api/v1/targets/target%2E1/users/github%3Auser%3A1',
      '/panel/api/v1/targets/target%2E1/users/github%3Auser%3A1/decisions',
    ]);
    expect(JSON.parse(String(stub.calls[2]?.init?.body))).toMatchObject({ role: null });
  });

  it('creates, reviews, reissues, and revokes identity-locked invitations', async () => {
    const invitation = {
      id: 'invite.1',
      account: VIEWER.account,
      role: 'viewer' as const,
      status: 'pending' as const,
      expires_at: '2026-08-15T10:00:00Z',
      created_by: VIEWER.account,
      created_at: '2026-08-08T10:00:00Z',
      invite_url: 'https://example.test/panel/invite/token',
    };
    const stub = stubFetch([
      jsonResponse(200, { items: [invitation], next_cursor: null, total: 1 }),
      jsonResponse(201, invitation),
      jsonResponse(200, invitation),
      jsonResponse(200, invitation),
      jsonResponse(200, invitation),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    const page = {
      query: 'ada',
      sort: 'expiry_soonest' as const,
      limit: 10,
      roles: ['viewer' as const],
      statuses: ['pending' as const, 'expired' as const],
    };

    await api.fetchTargetInvitations('target.1', page);
    await api.createTargetInvitation('target.1', {
      login: 'ada',
      role: 'viewer',
      expires_in_days: 1,
    });
    await api.fetchInvitation('token/value');
    await api.reissueTargetInvitation('target.1', 'invite.1', 30);
    await api.revokeTargetInvitation('target.1', 'invite.1');

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/targets/target%2E1/invitations?q=ada&sort=expiry_soonest&limit=10&role=viewer&status=pending&status=expired',
      '/panel/api/v1/targets/target%2E1/invitations',
      '/panel/api/v1/invites/token%2Fvalue',
      '/panel/api/v1/targets/target%2E1/invitations/invite%2E1/reissue',
      '/panel/api/v1/targets/target%2E1/invitations/invite%2E1',
    ]);
    expect(stub.calls[4]?.init?.method).toBe('DELETE');
  });
});

describe('history and authentication routes', () => {
  it('encodes history cursors and exposes both histories', async () => {
    const emptyPage = { items: [], next_cursor: null, total: 0 };
    const rootFailure = {
      workspace: TARGET.account,
      failure: {
        id: 'failure.1',
        delivery_id: 'delivery.1',
        repository_full_name: 'smykla-skalski/smyklot',
        event: 'workspace_repositories',
        stage: 'provider',
        reason: 'provider unavailable',
        retryable: true,
        occurred_at: '2026-08-10T10:00:00Z',
      },
    };
    const stub = stubFetch([
      jsonResponse(200, emptyPage),
      jsonResponse(200, emptyPage),
      jsonResponse(200, emptyPage),
      jsonResponse(200, { items: [rootFailure], next_cursor: null, total: 1 }),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.fetchAudit('2001', {
      cursor: 'next/page',
      query: 'repository settings',
      sort: 'oldest',
      limit: 25,
      scope: 'repositories',
      change: 'repository',
    });
    await api.fetchFailures('2001', {
      query: '',
      sort: 'newest',
      limit: 50,
      kind: 'retryable',
    });
    await api.fetchRootAudit({
      query: '',
      sort: 'actor_asc',
      limit: 20,
      categories: ['access', 'elevation'],
    });
    await expect(
      api.fetchRootFailures({ query: 'provider', sort: 'newest', limit: 10, kind: 'permanent' }),
    ).resolves.toMatchObject({
      items: [{ id: 'failure.1', workspace: TARGET.account }],
    });

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/targets/2001/audit?cursor=next%2Fpage&q=repository+settings&sort=oldest&limit=25&scope=repositories&change=repository',
      '/panel/api/v1/targets/2001/failures?sort=newest&limit=50&kind=retryable',
      '/panel/api/v1/root/history/audit?sort=actor_asc&limit=20&category=access&category=elevation',
      '/panel/api/v1/root/history/failures?q=provider&sort=newest&limit=10&kind=permanent',
    ]);
  });

  it('uses the mounted sign-in and sign-out routes', async () => {
    const stub = stubFetch([new Response(null, { status: 204 })]);
    const api = createPanelApi('/panel', stub.fetch);

    expect(api.signInUrl()).toBe('/panel/auth/github/start');
    expect(api.signInUrl({ token: 'one token', action: 'accept' })).toBe(
      '/panel/auth/github/start?invite=one+token&action=accept',
    );
    await api.signOut();
    expect(stub.calls[0]).toMatchObject({
      url: '/panel/api/v1/sign-out',
      init: { method: 'POST', credentials: 'same-origin' },
    });
  });

  /* A gateway, a proxy or a crash page answers with a status and a body the panel cannot read, and
     the sentence it falls back to goes straight into a red banner in front of a reader. It used to
     be `panel request failed with status 502`, which is a line from a log. */
  it('says what happened when a failure carries no envelope', async () => {
    const stub = stubFetch([new Response('<html>bad gateway</html>', { status: 502 })]);

    await expect(createPanelApi('', stub.fetch).fetchTargets()).rejects.toEqual(
      expect.objectContaining<Partial<PanelApiError>>({
        status: 502,
        code: 'unknown',
        message: 'the service could not answer',
      }),
    );
  });

  it.each([
    [404, 'the service does not recognise this request'],
    [403, 'this account is not allowed to see it'],
    [409, 'it changed while this page was open'],
    [418, 'the service refused the request'],
  ])('says what happened for a bare %i', async (status, said) => {
    const stub = stubFetch([new Response('nope', { status })]);

    await expect(createPanelApi('', stub.fetch).fetchTargets()).rejects.toMatchObject({
      status,
      message: said,
    });
  });

  /* The one failure with no status to phrase from. `fetch` REJECTS when the host is down or the
     connection is refused - which is what a reader gets while the service restarts under them - and
     `TypeError: Failed to fetch` reached the banner verbatim. */
  it('turns a request that reached nobody into an error the views know', async () => {
    const api = createPanelApi('', () => Promise.reject(new TypeError('Failed to fetch')));

    await expect(api.fetchTargets()).rejects.toEqual(
      expect.objectContaining<Partial<PanelApiError>>({
        name: 'PanelApiError',
        status: 0,
        code: 'unreachable',
        message: 'the service could not be reached',
      }),
    );
  });
});
