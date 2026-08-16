import { panelUrl } from './base';
import type { PanelStreamHandle, PanelStreamHandlers, PanelWebSocketFactory } from './events';
import { openPanelStream, panelStreamUrl } from './events';
import type { RequestFlood } from './request-rate';
import { createRequestRate, floodMessage } from './request-rate';
import type {
  AuditEntry,
  AuditHistoryRequest,
  AddTargetInvitationInput,
  AddRootInvitationInput,
  AddTargetUserInput,
  AccessDecision,
  DeliveryFailure,
  FailureHistoryRequest,
  Page,
  PanelAccount,
  PanelErrorBody,
  PanelTarget,
  PanelInvitation,
  InvitationPageRequest,
  PanelUser,
  PanelUserPageRequest,
  PanelViewer,
  NotificationPage,
  NotificationPageRequest,
  PendingCIRequest,
  PendingCIDetail,
  RepositoryDetail,
  RepositoryPageRequest,
  RepositorySettingsInput,
  RepositorySummary,
  RootElevation,
  RootElevationInput,
  RootInstallation,
  RootOverview,
  RootPanelUser,
  RootPanelUserPageRequest,
  RootRuntimeSettings,
  RootRuntimeSettingsInput,
  SecurityNotification,
  TargetSettingsInput,
  InvitationDays,
  InvitationSignIn,
  UpdateTargetUserInput,
  UpdateRootUserInput,
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
  fetchRootInstallations(): Promise<RootInstallation[]>;
  syncRootInstallations(): Promise<string[]>;
  fetchRootOverview(): Promise<RootOverview>;
  fetchRootPendingCI(requestId: string): Promise<PendingCIDetail>;
  checkRootPendingCI(requestId: string, expectedRevision: number): Promise<PendingCIRequest>;
  cancelRootPendingCI(requestId: string, expectedRevision: number): Promise<PendingCIRequest>;
  fetchRootUsers(request: RootPanelUserPageRequest): Promise<Page<RootPanelUser>>;
  updateRootUser(accountId: string, input: UpdateRootUserInput): Promise<void>;
  fetchRootInvitations(request: InvitationPageRequest): Promise<Page<PanelInvitation>>;
  createRootInvitation(input: AddRootInvitationInput): Promise<PanelInvitation>;
  reissueRootInvitation(
    invitationId: string,
    expiresInDays: InvitationDays,
  ): Promise<PanelInvitation>;
  revokeRootInvitation(invitationId: string): Promise<PanelInvitation>;
  fetchRootRuntimeSettings(): Promise<RootRuntimeSettings>;
  updateRootRuntimeSettings(input: RootRuntimeSettingsInput): Promise<RootRuntimeSettings>;
  fetchRootAudit(request: AuditHistoryRequest): Promise<Page<AuditEntry>>;
  fetchRootFailures(request: FailureHistoryRequest): Promise<Page<DeliveryFailure>>;
  fetchRootTargetSettings(targetId: string): Promise<PanelTarget>;
  updateRootTargetSettings(targetId: string, input: TargetSettingsInput): Promise<PanelTarget>;
  fetchRootRepositories(
    targetId: string,
    request: RepositoryPageRequest,
  ): Promise<Page<RepositorySummary>>;
  fetchRootRepository(targetId: string, repositoryId: string): Promise<RepositoryDetail>;
  updateRootRepositorySettings(
    targetId: string,
    repositoryId: string,
    input: RepositorySettingsInput,
  ): Promise<RepositoryDetail>;
  fetchRootElevation(targetId: string): Promise<RootElevation>;
  beginRootElevation(targetId: string, input: RootElevationInput): Promise<RootElevation>;
  endRootElevation(elevationId: string): Promise<RootElevation>;
  fetchRootTargetUsers(targetId: string, request: PanelUserPageRequest): Promise<Page<PanelUser>>;
  addRootTargetUser(targetId: string, input: AddTargetUserInput): Promise<PanelUser>;
  updateRootTargetUser(
    targetId: string,
    accountId: string,
    input: UpdateTargetUserInput,
  ): Promise<PanelUser>;
  fetchRootTargetInvitations(
    targetId: string,
    request: InvitationPageRequest,
  ): Promise<Page<PanelInvitation>>;
  createRootTargetInvitation(
    targetId: string,
    input: AddTargetInvitationInput,
  ): Promise<PanelInvitation>;
  reissueRootTargetInvitation(
    targetId: string,
    invitationId: string,
    expiresInDays: InvitationDays,
  ): Promise<PanelInvitation>;
  revokeRootTargetInvitation(targetId: string, invitationId: string): Promise<PanelInvitation>;
  fetchRootTargetUserDecisions(accountId: string, targetId: string): Promise<AccessDecision[]>;
  fetchRootTargetAudit(targetId: string, request: AuditHistoryRequest): Promise<Page<AuditEntry>>;
  fetchRootTargetFailures(
    targetId: string,
    request: FailureHistoryRequest,
  ): Promise<Page<DeliveryFailure>>;
  fetchNotifications(request: NotificationPageRequest): Promise<NotificationPage>;
  markNotificationRead(notificationId: string): Promise<SecurityNotification>;
  fetchTargetUsers(targetId: string, request: PanelUserPageRequest): Promise<Page<PanelUser>>;
  addTargetUser(targetId: string, input: AddTargetUserInput): Promise<PanelUser>;
  /** Logins offered while one is being typed; see `UserCompletion`. */
  suggestUsers(targetId: string, query: string): Promise<PanelAccount[]>;
  suggestRootTargetUsers(targetId: string, query: string): Promise<PanelAccount[]>;
  updateTargetUser(
    targetId: string,
    accountId: string,
    input: UpdateTargetUserInput,
  ): Promise<PanelUser>;
  fetchTargetInvitations(
    targetId: string,
    request: InvitationPageRequest,
  ): Promise<Page<PanelInvitation>>;
  createTargetInvitation(
    targetId: string,
    input: AddTargetInvitationInput,
  ): Promise<PanelInvitation>;
  fetchInvitation(token: string): Promise<PanelInvitation>;
  reissueTargetInvitation(
    targetId: string,
    invitationId: string,
    expiresInDays: InvitationDays,
  ): Promise<PanelInvitation>;
  revokeTargetInvitation(targetId: string, invitationId: string): Promise<PanelInvitation>;
  fetchUserDecisions(accountId: string, targetId: string): Promise<AccessDecision[]>;
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
  signInUrl(invitation?: InvitationSignIn): string;
  openStream(handlers: PanelStreamHandlers, dialQuery?: () => string): PanelStreamHandle;
  /**
   * Told whenever the server refuses a request because there is no session
   * behind it. Returns its own unsubscribe.
   *
   * Every view fetches for itself and every view handled a 401 as its own
   * failure, so an expired session showed up as a panel full of loaded content
   * with "sign in to use the panel" written across it and a Try again button
   * that could only fail again. One place learns it, and the app leaves.
   */
  onSessionLost(handler: (code: string) => void): () => void;
}

type FetchLike = (input: string, init?: RequestInit) => Promise<Response>;

const browserWebSocket: PanelWebSocketFactory = (url) => {
  const socket = new WebSocket(url);
  return {
    addEventListener: (type, listener) => socket.addEventListener(type, (event) => listener(event)),
    close: (code, reason) => socket.close(code, reason),
    send: (data) => socket.send(data),
  };
};

export function createPanelApi(
  base: string,
  fetchImpl: FetchLike,
  createWebSocket: PanelWebSocketFactory = browserWebSocket,
): PanelApi {
  const sessionLost = new Set<(code: string) => void>();
  const rate = createRequestRate();
  const reported = new Set<string>();

  /**
   * Says so, once, and only in the browser console.
   *
   * Not thrown, and not shown to anybody: a reader whose panel is caught in a
   * loop is better off with a page that works than with an error explaining why
   * it should not. This is here so that whoever opens the console - which is
   * where a developer looks and where a report from a colleague starts - finds
   * the cause written down instead of a request log scrolling past.
   */
  const reportFlood = (flood: RequestFlood | null): void => {
    if (flood === null || reported.has(flood.address)) return;
    reported.add(flood.address);
    console.error(`[smyklot] ${floodMessage(flood)}`);
  };

  /* Counted here rather than in `request`, because `request` is not the only way
     out: the session probe goes straight to fetch, since a 401 there is the
     ordinary way of asking whether anyone is signed in rather than a failure. A
     loop on that one would have gone unreported. */
  const counted: FetchLike = (url, init) => {
    reportFlood(rate.record(url));

    return fetchImpl(url, init);
  };

  const request = async (path: string, init?: RequestInit): Promise<Response> => {
    const response = await counted(panelUrl(base, path), {
      ...init,
      credentials: 'same-origin',
    });
    if (!response.ok) {
      const failure = await readError(response);
      /* Announced before it is thrown, so the app can leave for the sign-in page
         while the caller still handles its own failure as it always did. The
         session probe is deliberately not routed through here: a 401 there is the
         ordinary way of asking whether anyone is signed in. */
      if (response.status === 401) {
        for (const handler of sessionLost) handler(failure.code);
      }
      throw failure;
    }
    return response;
  };

  const jsonRequest = async <T>(path: string, init?: RequestInit): Promise<T> => {
    const response = await request(path, init);
    return (await response.json()) as T;
  };

  /**
   * Reads a completion list, and never fails loudly.
   *
   * Completion is an offer beside a field that works without it, so a request
   * that does not arrive leaves the field exactly as it was. Surfacing this as an
   * error would put a failure in front of somebody who is only typing a name, and
   * whatever they type is still resolved when they submit.
   */
  const suggest = async (path: string, query: string): Promise<PanelAccount[]> => {
    try {
      const response = await jsonRequest<{ items?: PanelAccount[] }>(
        `${path}?q=${encodeURIComponent(query)}`,
      );

      return response.items ?? [];
    } catch {
      return [];
    }
  };

  const putJson = <T>(path: string, body: unknown): Promise<T> =>
    jsonRequest<T>(path, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });

  const postJson = <T>(path: string, body: unknown): Promise<T> =>
    jsonRequest<T>(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });

  return {
    async fetchViewer(): Promise<PanelViewer | null> {
      const response = await counted(panelUrl(base, '/api/v1/session'), {
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

    async fetchRootInstallations(): Promise<RootInstallation[]> {
      const body = await jsonRequest<{ installations: RootInstallation[] }>(
        '/api/v1/root/installations',
      );
      return body.installations;
    },

    async syncRootInstallations(): Promise<string[]> {
      const body = await postJson<{ target_ids: string[] }>(
        '/api/v1/root/installations/sync',
        undefined,
      );
      return body.target_ids;
    },

    fetchRootOverview(): Promise<RootOverview> {
      return jsonRequest('/api/v1/root/overview');
    },

    fetchRootPendingCI(requestId: string): Promise<PendingCIDetail> {
      return jsonRequest(`/api/v1/root/pending-ci/${pathSegment(requestId)}`);
    },

    checkRootPendingCI(requestId: string, expectedRevision: number): Promise<PendingCIRequest> {
      return postJson(`/api/v1/root/pending-ci/${pathSegment(requestId)}/check`, {
        expected_revision: expectedRevision,
      });
    },

    cancelRootPendingCI(requestId: string, expectedRevision: number): Promise<PendingCIRequest> {
      return jsonRequest(`/api/v1/root/pending-ci/${pathSegment(requestId)}`, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ expected_revision: expectedRevision }),
      });
    },

    fetchRootUsers(userPage: RootPanelUserPageRequest): Promise<Page<RootPanelUser>> {
      return jsonRequest(withRootUserPageQuery('/api/v1/root/access/users', userPage));
    },

    async updateRootUser(accountId: string, input: UpdateRootUserInput): Promise<void> {
      await request(`/api/v1/root/access/users/${pathSegment(accountId)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      });
    },

    fetchRootInvitations(invitationPage: InvitationPageRequest): Promise<Page<PanelInvitation>> {
      return jsonRequest(withAccessPageQuery('/api/v1/root/access/invitations', invitationPage));
    },

    createRootInvitation(input: AddRootInvitationInput): Promise<PanelInvitation> {
      return postJson('/api/v1/root/access/invitations', input);
    },

    reissueRootInvitation(
      invitationId: string,
      expiresInDays: InvitationDays,
    ): Promise<PanelInvitation> {
      return postJson(`/api/v1/root/access/invitations/${pathSegment(invitationId)}/reissue`, {
        expires_in_days: expiresInDays,
      });
    },

    revokeRootInvitation(invitationId: string): Promise<PanelInvitation> {
      return jsonRequest(`/api/v1/root/access/invitations/${pathSegment(invitationId)}`, {
        method: 'DELETE',
      });
    },

    fetchRootRuntimeSettings(): Promise<RootRuntimeSettings> {
      return jsonRequest('/api/v1/root/settings');
    },

    updateRootRuntimeSettings(input: RootRuntimeSettingsInput): Promise<RootRuntimeSettings> {
      return putJson('/api/v1/root/settings', input);
    },

    fetchRootAudit(history: AuditHistoryRequest): Promise<Page<AuditEntry>> {
      return jsonRequest(
        withHistoryQuery('/api/v1/root/history/audit', history, {
          category: history.categories ?? [],
        }),
      );
    },

    async fetchRootFailures(history: FailureHistoryRequest): Promise<Page<DeliveryFailure>> {
      const page = await jsonRequest<
        Page<{ installation: DeliveryFailure['installation']; failure: DeliveryFailure }>
      >(
        withHistoryQuery('/api/v1/root/history/failures', history, {
          kind: history.kind,
        }),
      );
      return {
        ...page,
        items: page.items.map(({ installation, failure }) => ({ ...failure, installation })),
      };
    },

    fetchRootTargetSettings(targetId: string): Promise<PanelTarget> {
      return jsonRequest(`/api/v1/root/installations/${pathSegment(targetId)}/settings`);
    },

    updateRootTargetSettings(targetId: string, input: TargetSettingsInput): Promise<PanelTarget> {
      return putJson(`/api/v1/root/installations/${pathSegment(targetId)}/settings`, input);
    },

    fetchRootRepositories(
      targetId: string,
      repositoryPage: RepositoryPageRequest,
    ): Promise<Page<RepositorySummary>> {
      return jsonRequest(
        withRepositoryQuery(
          `/api/v1/root/installations/${pathSegment(targetId)}/repositories`,
          repositoryPage,
        ),
      );
    },

    fetchRootRepository(targetId: string, repositoryId: string): Promise<RepositoryDetail> {
      return jsonRequest(
        `/api/v1/root/installations/${pathSegment(targetId)}/repositories/${pathSegment(repositoryId)}`,
      );
    },

    updateRootRepositorySettings(
      targetId: string,
      repositoryId: string,
      input: RepositorySettingsInput,
    ): Promise<RepositoryDetail> {
      return putJson(
        `/api/v1/root/installations/${pathSegment(targetId)}/repositories/${pathSegment(repositoryId)}/settings`,
        input,
      );
    },

    fetchRootElevation(targetId: string): Promise<RootElevation> {
      return jsonRequest(`/api/v1/root/installations/${pathSegment(targetId)}/elevation`);
    },

    beginRootElevation(targetId: string, input: RootElevationInput): Promise<RootElevation> {
      return postJson(`/api/v1/root/installations/${pathSegment(targetId)}/elevation`, input);
    },

    endRootElevation(elevationId: string): Promise<RootElevation> {
      return jsonRequest(`/api/v1/root/elevations/${pathSegment(elevationId)}`, {
        method: 'DELETE',
      });
    },

    fetchRootTargetUsers(
      targetId: string,
      userPage: PanelUserPageRequest,
    ): Promise<Page<PanelUser>> {
      return jsonRequest(
        withAccessPageQuery(`/api/v1/root/installations/${pathSegment(targetId)}/users`, userPage),
      );
    },

    addRootTargetUser(targetId: string, input: AddTargetUserInput): Promise<PanelUser> {
      return postJson(`/api/v1/root/installations/${pathSegment(targetId)}/users`, input);
    },

    updateRootTargetUser(
      targetId: string,
      accountId: string,
      input: UpdateTargetUserInput,
    ): Promise<PanelUser> {
      return putJson(
        `/api/v1/root/installations/${pathSegment(targetId)}/users/${pathSegment(accountId)}`,
        input,
      );
    },

    fetchRootTargetInvitations(
      targetId: string,
      invitationPage: InvitationPageRequest,
    ): Promise<Page<PanelInvitation>> {
      return jsonRequest(
        withAccessPageQuery(
          `/api/v1/root/installations/${pathSegment(targetId)}/invitations`,
          invitationPage,
        ),
      );
    },

    createRootTargetInvitation(
      targetId: string,
      input: AddTargetInvitationInput,
    ): Promise<PanelInvitation> {
      return postJson(`/api/v1/root/installations/${pathSegment(targetId)}/invitations`, input);
    },

    reissueRootTargetInvitation(
      targetId: string,
      invitationId: string,
      expiresInDays: InvitationDays,
    ): Promise<PanelInvitation> {
      return postJson(
        `/api/v1/root/installations/${pathSegment(targetId)}/invitations/${pathSegment(invitationId)}/reissue`,
        { expires_in_days: expiresInDays },
      );
    },

    revokeRootTargetInvitation(targetId: string, invitationId: string): Promise<PanelInvitation> {
      return jsonRequest(
        `/api/v1/root/installations/${pathSegment(targetId)}/invitations/${pathSegment(invitationId)}`,
        { method: 'DELETE' },
      );
    },

    async fetchRootTargetUserDecisions(
      accountId: string,
      targetId: string,
    ): Promise<AccessDecision[]> {
      const body = await jsonRequest<{ decisions: AccessDecision[] }>(
        `/api/v1/root/installations/${pathSegment(targetId)}/users/${pathSegment(accountId)}/decisions`,
      );
      return body.decisions;
    },

    fetchRootTargetAudit(
      targetId: string,
      history: AuditHistoryRequest,
    ): Promise<Page<AuditEntry>> {
      return jsonRequest(
        withHistoryQuery(`/api/v1/root/installations/${pathSegment(targetId)}/audit`, history, {
          scope: history.scope ?? 'all',
          change: history.change ?? 'all',
        }),
      );
    },

    fetchRootTargetFailures(
      targetId: string,
      history: FailureHistoryRequest,
    ): Promise<Page<DeliveryFailure>> {
      return jsonRequest(
        withHistoryQuery(`/api/v1/root/installations/${pathSegment(targetId)}/failures`, history, {
          kind: history.kind,
        }),
      );
    },

    fetchNotifications(notificationPage: NotificationPageRequest): Promise<NotificationPage> {
      const parameters = new URLSearchParams({ limit: String(notificationPage.limit) });
      if (notificationPage.cursor !== undefined) {
        parameters.set('cursor', notificationPage.cursor);
      }
      return jsonRequest(`/api/v1/notifications?${parameters.toString()}`);
    },

    markNotificationRead(notificationId: string): Promise<SecurityNotification> {
      return putJson(`/api/v1/notifications/${pathSegment(notificationId)}/read`, {});
    },

    fetchTargetUsers(targetId: string, userPage: PanelUserPageRequest): Promise<Page<PanelUser>> {
      return jsonRequest(
        withAccessPageQuery(`/api/v1/targets/${pathSegment(targetId)}/users`, userPage),
      );
    },

    addTargetUser(targetId: string, input: AddTargetUserInput): Promise<PanelUser> {
      return postJson(`/api/v1/targets/${pathSegment(targetId)}/users`, input);
    },

    suggestUsers(targetId: string, query: string): Promise<PanelAccount[]> {
      return suggest(`/api/v1/targets/${pathSegment(targetId)}/user-suggestions`, query);
    },

    suggestRootTargetUsers(targetId: string, query: string): Promise<PanelAccount[]> {
      return suggest(`/api/v1/root/installations/${pathSegment(targetId)}/user-suggestions`, query);
    },

    updateTargetUser(
      targetId: string,
      accountId: string,
      input: UpdateTargetUserInput,
    ): Promise<PanelUser> {
      return putJson(
        `/api/v1/targets/${pathSegment(targetId)}/users/${pathSegment(accountId)}`,
        input,
      );
    },

    fetchTargetInvitations(
      targetId: string,
      invitationPage: InvitationPageRequest,
    ): Promise<Page<PanelInvitation>> {
      return jsonRequest(
        withAccessPageQuery(`/api/v1/targets/${pathSegment(targetId)}/invitations`, invitationPage),
      );
    },

    createTargetInvitation(
      targetId: string,
      input: AddTargetInvitationInput,
    ): Promise<PanelInvitation> {
      return postJson(`/api/v1/targets/${pathSegment(targetId)}/invitations`, input);
    },

    fetchInvitation(token: string): Promise<PanelInvitation> {
      return jsonRequest(`/api/v1/invites/${pathSegment(token)}`);
    },

    reissueTargetInvitation(
      targetId: string,
      invitationId: string,
      expiresInDays: InvitationDays,
    ): Promise<PanelInvitation> {
      return postJson(
        `/api/v1/targets/${pathSegment(targetId)}/invitations/${pathSegment(invitationId)}/reissue`,
        { expires_in_days: expiresInDays },
      );
    },

    revokeTargetInvitation(targetId: string, invitationId: string): Promise<PanelInvitation> {
      return jsonRequest(
        `/api/v1/targets/${pathSegment(targetId)}/invitations/${pathSegment(invitationId)}`,
        { method: 'DELETE' },
      );
    },

    async fetchUserDecisions(accountId: string, targetId: string): Promise<AccessDecision[]> {
      const path = `/api/v1/targets/${pathSegment(targetId)}/users/${pathSegment(accountId)}/decisions`;
      const body = await jsonRequest<{ decisions: AccessDecision[] }>(path);
      return body.decisions;
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
          scope: history.scope ?? 'all',
          change: history.change ?? 'all',
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

    signInUrl(invitation?: InvitationSignIn): string {
      const path = panelUrl(base, '/auth/github/start');
      if (invitation === undefined) return path;
      const query = new URLSearchParams({
        invite: invitation.token,
        action: invitation.action,
      });
      return `${path}?${query.toString()}`;
    },

    openStream(handlers: PanelStreamHandlers, dialQuery?: () => string): PanelStreamHandle {
      // The URL is rebuilt per connect attempt so reconnect handshakes carry
      // the current preference revision and checksum.
      return openPanelStream(
        () => {
          const url = panelStreamUrl(base, window.location.href);
          const query = dialQuery?.();
          return query === undefined || query === '' ? url : `${url}?${query}`;
        },
        handlers,
        createWebSocket,
      );
    },

    onSessionLost(handler: (code: string) => void): () => void {
      sessionLost.add(handler);
      return () => {
        sessionLost.delete(handler);
      };
    },
  };
}

function withHistoryQuery(
  path: string,
  history: { cursor?: string; query: string; sort: string; limit: number },
  filter: Record<string, string | readonly string[]>,
): string {
  const parameters = new URLSearchParams();
  if (history.cursor !== undefined) parameters.set('cursor', history.cursor);
  if (history.query !== '') parameters.set('q', history.query);
  parameters.set('sort', history.sort);
  parameters.set('limit', String(history.limit));
  for (const [name, value] of Object.entries(filter)) {
    if (typeof value !== 'string') {
      for (const item of value) parameters.append(name, item);
    } else {
      parameters.set(name, value);
    }
  }

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

function withAccessPageQuery(
  path: string,
  page: {
    cursor?: string;
    query: string;
    sort: string;
    limit: number;
    roles: readonly string[];
    statuses: readonly string[];
  },
): string {
  const parameters = new URLSearchParams();
  if (page.cursor !== undefined) parameters.set('cursor', page.cursor);
  if (page.query !== '') parameters.set('q', page.query);
  parameters.set('sort', page.sort);
  parameters.set('limit', String(page.limit));
  for (const role of page.roles) parameters.append('role', role);
  for (const status of page.statuses) parameters.append('status', status);

  return `${path}?${parameters.toString()}`;
}

function withRootUserPageQuery(path: string, page: RootPanelUserPageRequest): string {
  const parameters = new URLSearchParams();
  if (page.cursor !== undefined) parameters.set('cursor', page.cursor);
  if (page.query !== '') parameters.set('q', page.query);
  parameters.set('sort', page.sort);
  parameters.set('limit', String(page.limit));
  for (const role of page.systemRoles) parameters.append('system_role', role);
  for (const status of page.statuses) parameters.append('status', status);

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
