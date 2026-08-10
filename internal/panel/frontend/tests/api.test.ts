import { describe, expect, it } from 'vitest';

import { PanelApiError, createPanelApi } from '../src/lib/api';
import type { ConfigSources, ConfigValues, PanelTarget, RepositoryDetail } from '../src/lib/types';

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

const CONFIG: ConfigValues = {
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

const SOURCES = Object.fromEntries(
  Object.keys(CONFIG).map((key) => [key, 'process']),
) as ConfigSources;

const VIEWER = {
  account: {
    id: '1001',
    provider: 'github:https://api.github.com',
    subject_id: '1001',
    login: 'ada',
    display_name: 'Ada Lovelace',
    avatar_url: null,
  },
  root: true,
  status: 'active' as const,
  global_role: 'owner' as const,
  capabilities: {
    read: true,
    write: true,
    manage_target_users: true,
    manage_global_users: true,
    manage_owners: true,
  },
  target_count: 1,
};

const TARGET: PanelTarget = {
  id: '2001',
  installation_id: '3001',
  type: 'Organization',
  account: { ...VIEWER.account, id: '2001', subject_id: '2001', login: 'example' },
  repository_default_enabled: false,
  config_patch: {},
  inherited_config: CONFIG,
  effective_config: CONFIG,
  config_sources: SOURCES,
  revision: 1,
  repository_counts: { total: 1, enabled: 0, disabled: 1 },
  effective_role: 'owner',
  access_source: 'root',
  capabilities: VIEWER.capabilities,
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
  config_override_count: 0,
  config_file_status: 'missing' as const,
  updated_at: '2026-08-08T10:00:00Z',
};

const DETAIL: RepositoryDetail = {
  repository: REPOSITORY,
  config_patch: {},
  inherited_config: CONFIG,
  effective_config: CONFIG,
  config_sources: SOURCES,
  config_file_patch: {},
  ignore_repository_file: false,
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

  it('uses full replacement PUTs with optimistic revisions', async () => {
    const updatedTarget = { ...TARGET, repository_default_enabled: true, revision: 2 };
    const updatedDetail = { ...DETAIL, revision: 2 };
    const stub = stubFetch([jsonResponse(200, updatedTarget), jsonResponse(200, updatedDetail)]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.updateTargetSettings('2001', {
      repository_default_enabled: true,
      config_patch: { quiet_success: true },
      expected_revision: 1,
    });
    await api.updateRepositorySettings('2001', '4001', {
      enabled_override: true,
      config_patch: { allowed_commands: [] },
      ignore_repository_file: false,
      expected_revision: 1,
    });

    expect(stub.calls[0]?.init?.method).toBe('PUT');
    expect(stub.calls[0]?.init?.headers).toEqual({ 'Content-Type': 'application/json' });
    expect(JSON.parse(String(stub.calls[0]?.init?.body))).toMatchObject({ expected_revision: 1 });
    expect(stub.calls[1]?.url).toBe('/panel/api/v1/targets/2001/repositories/4001/settings');
  });

  it('encodes slashes and traversal segments as path data', async () => {
    const stub = stubFetch([jsonResponse(200, DETAIL)]);
    const api = createPanelApi('/panel', stub.fetch);

    await api.fetchRepository('a/b', '..');

    expect(stub.calls[0]?.url).toBe('/panel/api/v1/targets/a%2Fb/repositories/%2E%2E');
  });
});

describe('user management', () => {
  it('uses scoped user endpoints and preserves nullable inheritance', async () => {
    const user = {
      account: VIEWER.account,
      root: false,
      status: 'active' as const,
      global_role: 'editor' as const,
      revision: 1,
      created_at: '2026-08-08T10:00:00Z',
      updated_at: '2026-08-08T10:00:00Z',
      manageable: true,
    };
    const stub = stubFetch([
      jsonResponse(200, { items: [user], next_cursor: null, total: 1 }),
      jsonResponse(201, user),
      jsonResponse(200, user),
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

    await api.fetchUsers(page);
    await api.addUser({ login: 'ada', role: 'editor', target_id: 'target.1' });
    await api.updateUser('github:user:1', {
      global_role: 'viewer',
      status: 'active',
      expected_revision: 1,
    });
    await api.fetchTargetUsers('target.1', { ...page, cursor: '20' });
    await api.addTargetUser('target.1', { login: 'ada', role: 'viewer' });
    await api.updateTargetUser('target.1', 'github:user:1', {
      role: null,
      suspended: false,
      expected_revision: 1,
    });
    await api.fetchUserDecisions('github:user:1', 'target.1');

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/users?q=ada+user&sort=name_asc&limit=20&role=editor&status=active',
      '/panel/api/v1/users',
      '/panel/api/v1/users/github%3Auser%3A1',
      '/panel/api/v1/targets/target%2E1/users?cursor=20&q=ada+user&sort=name_asc&limit=20&role=editor&status=active',
      '/panel/api/v1/targets/target%2E1/users',
      '/panel/api/v1/targets/target%2E1/users/github%3Auser%3A1',
      '/panel/api/v1/targets/target%2E1/users/github%3Auser%3A1/decisions',
    ]);
    expect(JSON.parse(String(stub.calls[5]?.init?.body))).toMatchObject({ role: null });
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

    await api.fetchInvitations(page);
    await api.createInvitation({
      login: 'ada',
      role: 'viewer',
      target_id: 'target.1',
      expires_in_days: 7,
    });
    await api.fetchTargetInvitations('target.1', page);
    await api.createTargetInvitation('target.1', {
      login: 'ada',
      role: 'viewer',
      expires_in_days: 1,
    });
    await api.fetchInvitation('token/value');
    await api.reissueInvitation('invite.1', 30);
    await api.revokeInvitation('invite.1');

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/invitations?q=ada&sort=expiry_soonest&limit=10&role=viewer&status=pending&status=expired',
      '/panel/api/v1/invitations',
      '/panel/api/v1/targets/target%2E1/invitations?q=ada&sort=expiry_soonest&limit=10&role=viewer&status=pending&status=expired',
      '/panel/api/v1/targets/target%2E1/invitations',
      '/panel/api/v1/invites/token%2Fvalue',
      '/panel/api/v1/invitations/invite%2E1/reissue',
      '/panel/api/v1/invitations/invite%2E1',
    ]);
    expect(stub.calls[6]?.init?.method).toBe('DELETE');
  });
});

describe('history and authentication routes', () => {
  it('encodes history cursors and exposes both histories', async () => {
    const emptyPage = { items: [], next_cursor: null, total: 0 };
    const stub = stubFetch([jsonResponse(200, emptyPage), jsonResponse(200, emptyPage)]);
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

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/targets/2001/audit?cursor=next%2Fpage&q=repository+settings&sort=oldest&limit=25&scope=repositories&change=repository',
      '/panel/api/v1/targets/2001/failures?sort=newest&limit=50&kind=retryable',
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

  it('falls back to the HTTP status for a non-envelope failure', async () => {
    const stub = stubFetch([new Response('<html>bad gateway</html>', { status: 502 })]);

    await expect(createPanelApi('', stub.fetch).fetchTargets()).rejects.toEqual(
      expect.objectContaining<Partial<PanelApiError>>({
        status: 502,
        code: 'unknown',
        message: 'panel request failed with status 502',
      }),
    );
  });
});
