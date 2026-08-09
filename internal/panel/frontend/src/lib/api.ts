import { panelUrl } from './base';
import type { PanelEventSourceFactory, PanelStreamHandlers } from './events';
import { openPanelStream, panelStreamUrl } from './events';
import type {
  AuditEntry,
  AuditHistoryRequest,
  DeliveryFailure,
  FailureHistoryRequest,
  Page,
  PanelErrorBody,
  PanelTarget,
  PanelViewer,
  RepositoryDetail,
  RepositoryPageRequest,
  RepositorySettingsInput,
  RepositorySummary,
  TargetSettingsInput,
} from './types';

export class PanelApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = 'PanelApiError';
  }
}

export interface PanelApi {
  fetchViewer(): Promise<PanelViewer | null>;
  fetchTargets(): Promise<PanelTarget[]>;
  updateTargetSettings(targetId: string, input: TargetSettingsInput): Promise<PanelTarget>;
  fetchRepositories(
    targetId: string,
    request: RepositoryPageRequest,
  ): Promise<Page<RepositorySummary>>;
  fetchRepository(targetId: string, repositoryId: string): Promise<RepositoryDetail>;
  updateRepositorySettings(
    targetId: string,
    repositoryId: string,
    input: RepositorySettingsInput,
  ): Promise<RepositoryDetail>;
  fetchAudit(targetId: string, request: AuditHistoryRequest): Promise<Page<AuditEntry>>;
  fetchFailures(targetId: string, request: FailureHistoryRequest): Promise<Page<DeliveryFailure>>;
  signOut(): Promise<void>;
  signInUrl(): string;
  openStream(handlers: PanelStreamHandlers): () => void;
}

type FetchLike = (input: string, init?: RequestInit) => Promise<Response>;

const browserEventSource: PanelEventSourceFactory = (url) => new EventSource(url);

export function createPanelApi(
  base: string,
  fetchImpl: FetchLike,
  createEventSource: PanelEventSourceFactory = browserEventSource,
): PanelApi {
  const request = async (path: string, init?: RequestInit): Promise<Response> => {
    const response = await fetchImpl(panelUrl(base, path), {
      ...init,
      credentials: 'same-origin',
    });
    if (!response.ok) {
      throw await readError(response);
    }
    return response;
  };

  const jsonRequest = async <T>(path: string, init?: RequestInit): Promise<T> => {
    const response = await request(path, init);
    return (await response.json()) as T;
  };

  const putJson = <T>(path: string, body: unknown): Promise<T> =>
    jsonRequest<T>(path, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });

  return {
    async fetchViewer(): Promise<PanelViewer | null> {
      const response = await fetchImpl(panelUrl(base, '/api/v1/session'), {
        credentials: 'same-origin',
      });
      if (response.status === 401) {
        return null;
      }
      if (!response.ok) {
        throw await readError(response);
      }
      return (await response.json()) as PanelViewer;
    },

    async fetchTargets(): Promise<PanelTarget[]> {
      const body = await jsonRequest<{ targets: PanelTarget[] }>('/api/v1/targets');
      return body.targets;
    },

    updateTargetSettings(targetId: string, input: TargetSettingsInput): Promise<PanelTarget> {
      return putJson(`/api/v1/targets/${pathSegment(targetId)}/settings`, input);
    },

    fetchRepositories(
      targetId: string,
      repositoryPage: RepositoryPageRequest,
    ): Promise<Page<RepositorySummary>> {
      return jsonRequest(
        withRepositoryQuery(
          `/api/v1/targets/${pathSegment(targetId)}/repositories`,
          repositoryPage,
        ),
      );
    },

    fetchRepository(targetId: string, repositoryId: string): Promise<RepositoryDetail> {
      return jsonRequest(
        `/api/v1/targets/${pathSegment(targetId)}/repositories/${pathSegment(repositoryId)}`,
      );
    },

    updateRepositorySettings(
      targetId: string,
      repositoryId: string,
      input: RepositorySettingsInput,
    ): Promise<RepositoryDetail> {
      return putJson(
        `/api/v1/targets/${pathSegment(targetId)}/repositories/${pathSegment(repositoryId)}/settings`,
        input,
      );
    },

    fetchAudit(targetId: string, history: AuditHistoryRequest): Promise<Page<AuditEntry>> {
      return jsonRequest(
        withHistoryQuery(`/api/v1/targets/${pathSegment(targetId)}/audit`, history, {
          scope: history.scope,
        }),
      );
    },

    fetchFailures(
      targetId: string,
      history: FailureHistoryRequest,
    ): Promise<Page<DeliveryFailure>> {
      return jsonRequest(
        withHistoryQuery(`/api/v1/targets/${pathSegment(targetId)}/failures`, history, {
          kind: history.kind,
        }),
      );
    },

    async signOut(): Promise<void> {
      await request('/api/v1/sign-out', { method: 'POST' });
    },

    signInUrl(): string {
      return panelUrl(base, '/auth/github/start');
    },

    openStream(handlers: PanelStreamHandlers): () => void {
      return openPanelStream(
        panelStreamUrl(base, window.location.href),
        handlers,
        createEventSource,
      );
    },
  };
}

function withHistoryQuery(
  path: string,
  history: { cursor?: string; query: string; sort: string; limit: number },
  filter: Record<string, string>,
): string {
  const parameters = new URLSearchParams();
  if (history.cursor !== undefined) parameters.set('cursor', history.cursor);
  if (history.query !== '') parameters.set('q', history.query);
  parameters.set('sort', history.sort);
  parameters.set('limit', String(history.limit));
  for (const [name, value] of Object.entries(filter)) parameters.set(name, value);

  return `${path}?${parameters.toString()}`;
}

function withRepositoryQuery(path: string, page: RepositoryPageRequest): string {
  const parameters = new URLSearchParams();
  if (page.cursor !== undefined) parameters.set('cursor', page.cursor);
  if (page.query !== '') parameters.set('q', page.query);
  parameters.set('sort', page.sort);
  parameters.set('limit', String(page.limit));
  parameters.set('state', page.state);
  for (const file of page.files) parameters.append('file', file);
  if (page.setting.mode === 'keys') {
    for (const key of page.setting.keys) parameters.append('setting', key);
  } else if (page.setting.mode !== 'all') {
    parameters.set('setting', page.setting.mode);
  }

  return `${path}?${parameters.toString()}`;
}

function pathSegment(value: string): string {
  return encodeURIComponent(value).replace(/\./g, '%2E');
}

async function readError(response: Response): Promise<PanelApiError> {
  let code = 'unknown';
  let message = `panel request failed with status ${response.status}`;
  try {
    const body = (await response.json()) as Partial<PanelErrorBody>;
    if (body.error?.code !== undefined) {
      code = body.error.code;
    }
    if (body.error?.message !== undefined && body.error.message !== '') {
      message = body.error.message;
    }
  } catch {
    // Proxies and crashes are not required to understand the panel envelope.
  }
  return new PanelApiError(response.status, code, message);
}
