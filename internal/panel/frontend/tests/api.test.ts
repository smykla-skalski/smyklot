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
};

const REPOSITORY = {
  id: '4001',
  name: 'api',
  full_name: 'example/api',
  private: false,
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
      jsonResponse(200, { repositories: [REPOSITORY] }),
      jsonResponse(200, DETAIL),
    ]);
    const api = createPanelApi('/panel', stub.fetch);

    await expect(api.fetchTargets()).resolves.toEqual([TARGET]);
    await expect(api.fetchRepositories('2001')).resolves.toEqual([REPOSITORY]);
    await expect(api.fetchRepository('2001', '4001')).resolves.toEqual(DETAIL);
    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/targets',
      '/panel/api/v1/targets/2001/repositories',
      '/panel/api/v1/targets/2001/repositories/4001',
    ]);
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
    });
    await api.fetchFailures('2001', {
      query: '',
      sort: 'newest',
      limit: 50,
      kind: 'retryable',
    });

    expect(stub.calls.map((call) => call.url)).toEqual([
      '/panel/api/v1/targets/2001/audit?cursor=next%2Fpage&q=repository+settings&sort=oldest&limit=25&scope=repositories',
      '/panel/api/v1/targets/2001/failures?sort=newest&limit=50&kind=retryable',
    ]);
  });

  it('uses the mounted sign-in and sign-out routes', async () => {
    const stub = stubFetch([new Response(null, { status: 204 })]);
    const api = createPanelApi('/panel', stub.fetch);

    expect(api.signInUrl()).toBe('/panel/auth/github/start');
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
