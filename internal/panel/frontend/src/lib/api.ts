import { panelUrl } from './base';
import { parseJson, type JsonValue } from './merge';
import type { PanelStreamHandle, PanelStreamHandlers, PanelWebSocketFactory } from './events';
import { openPanelStream, panelStreamUrl } from './events';
import type { RequestFlood } from './request-rate';
import { createRequestRate, floodMessage } from './request-rate';
import {
  parseSyncFileRenderResponse,
  type SyncFileRenderInput,
  type SyncFileRenderResponse,
} from './sync-file-render.generated';
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
  WorkspaceSettingsBatchInput,
  WorkspaceSettingsBatchResponse,
  SettingsCheckpoint,
  WorkspaceSettingsConflict,
  SettingsRestoreInput,
  PanelUser,
  PanelUserPageRequest,
  PanelViewer,
  NotificationPage,
  NotificationPageRequest,
  PendingCIRequest,
  PendingCIDetail,
  QueueActionInput,
  QueueDetail,
  QueueItem,
  QueuePage,
  QueuePolicy,
  RootJobPolicies,
  QueuePolicyInput,
  QueueSchedulePreview,
  ScheduleProfile,
  ScheduleProfileInput,
  ScheduleRequest,
  ScheduleRequestInput,
  TargetSchedules,
  RepositoryDetail,
  RepositoryPageRequest,
  RepositorySummary,
  RootElevation,
  RootElevationInput,
  RootWorkspace,
  RootOverview,
  RootPanelUser,
  RootPanelUserPageRequest,
  RootRuntimeSettings,
  RootRuntimeSettingsInput,
  SecurityNotification,
  ServicePerformance,
  SyncConfig,
  SyncOverride,
  SyncPathIndex,
  SyncPlan,
  SyncRunNowResponse,
  SyncFilesContext,
  SyncStatus,
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
    readonly kind?: string,
    readonly conflicts: WorkspaceSettingsConflict[] = [],
  ) {
    super(message);
    this.name = 'PanelApiError';
  }
}

export interface PanelApi {
  fetchViewer(): Promise<PanelViewer | null>;
  fetchTargets(): Promise<PanelTarget[]>;
  fetchRootWorkspaces(): Promise<RootWorkspace[]>;
  syncRootWorkspaces(): Promise<string[]>;
  fetchRootOverview(): Promise<RootOverview>;
  fetchRootPerformance(windowHours: number): Promise<ServicePerformance>;
  fetchRootPendingCI(requestId: string): Promise<PendingCIDetail>;
  checkRootPendingCI(requestId: string, expectedRevision: number): Promise<PendingCIRequest>;
  cancelRootPendingCI(requestId: string, expectedRevision: number): Promise<PendingCIRequest>;
  fetchRootQueue(query?: string): Promise<QueuePage>;
  fetchTargetQueue(targetId: string, query?: string): Promise<QueuePage>;
  fetchRootQueueItem(itemId: string): Promise<QueueDetail>;
  fetchTargetQueueItem(targetId: string, itemId: string): Promise<QueueDetail>;
  actOnRootQueue(itemId: string, input: QueueActionInput): Promise<QueueItem>;
  actOnTargetQueue(targetId: string, itemId: string, input: QueueActionInput): Promise<QueueItem>;
  previewRootQueueAction(itemId: string, input: QueueActionInput): Promise<QueueSchedulePreview>;
  previewTargetQueueAction(
    targetId: string,
    itemId: string,
    input: QueueActionInput,
  ): Promise<QueueSchedulePreview>;
  fetchRootScheduleProfiles(): Promise<ScheduleProfile[]>;
  fetchRootJobPolicies(): Promise<RootJobPolicies>;
  fetchRootScheduleRequests(): Promise<ScheduleRequest[]>;
  fetchTargetSchedules(targetId: string): Promise<TargetSchedules>;
  fetchTargetScheduleRequests(targetId: string): Promise<ScheduleRequest[]>;
  createRootScheduleProfile(input: ScheduleProfileInput): Promise<ScheduleProfile>;
  updateRootScheduleProfile(id: string, input: ScheduleProfileInput): Promise<ScheduleProfile>;
  archiveRootScheduleProfile(id: string, expectedRevision: number): Promise<ScheduleProfile>;
  updateRootJobPolicy(kind: string, input: QueuePolicyInput): Promise<QueuePolicy>;
  updateRootWorkspaceJobPolicy(
    targetId: string,
    kind: string,
    input: QueuePolicyInput,
  ): Promise<QueuePolicy>;
  deleteRootWorkspaceJobPolicy(
    targetId: string,
    kind: string,
    expectedRevision: number,
  ): Promise<QueuePolicy>;
  decideRootScheduleRequest(
    id: string,
    input: {
      approve: boolean;
      promote_profile: boolean;
      reason: string;
      expected_revision: number;
    },
  ): Promise<ScheduleRequest>;
  createTargetScheduleRequest(
    targetId: string,
    input: ScheduleRequestInput,
  ): Promise<ScheduleRequest>;
  withdrawTargetScheduleRequest(
    targetId: string,
    id: string,
    expectedRevision: number,
  ): Promise<ScheduleRequest>;
  runSyncNow(
    targetId: string,
    input: { expected_revision: number; reason: string },
  ): Promise<SyncRunNowResponse>;
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
  saveRootRuntimeSettings(input: RootRuntimeSettingsInput): Promise<RootRuntimeSettings>;
  fetchRootRuntimeSettingsBaseline(): Promise<SettingsCheckpoint>;
  fetchRootRuntimeSettingsCheckpoint(checkpointId: string): Promise<SettingsCheckpoint>;
  restoreRootRuntimeSettingsCheckpoint(
    checkpointId: string,
    input: SettingsRestoreInput,
  ): Promise<RootRuntimeSettings>;
  fetchRootAudit(request: AuditHistoryRequest): Promise<Page<AuditEntry>>;
  /** The same filtered audit as a file, said as an address the browser can download. */
  rootAuditExportHref(request: AuditHistoryRequest): string;
  auditExportHref(targetId: string, request: AuditHistoryRequest): string;
  fetchRootFailures(request: FailureHistoryRequest): Promise<Page<DeliveryFailure>>;
  fetchRootTargetSettings(targetId: string): Promise<PanelTarget>;
  saveRootWorkspaceSettings(
    targetId: string,
    input: WorkspaceSettingsBatchInput,
  ): Promise<WorkspaceSettingsBatchResponse>;
  fetchRootWorkspaceSettingsBaseline(targetId: string): Promise<SettingsCheckpoint>;
  fetchRootWorkspaceSettingsCheckpoint(
    targetId: string,
    checkpointId: string,
  ): Promise<SettingsCheckpoint>;
  restoreRootWorkspaceSettingsCheckpoint(
    targetId: string,
    checkpointId: string,
    input: SettingsRestoreInput,
  ): Promise<WorkspaceSettingsBatchResponse>;
  fetchRootRepositories(
    targetId: string,
    request: RepositoryPageRequest,
  ): Promise<Page<RepositorySummary>>;
  fetchRootRepository(targetId: string, repositoryId: string): Promise<RepositoryDetail>;
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
  /** Empties the inbox, and answers how many were unread rather than which. */
  markAllNotificationsRead(): Promise<{ read: number }>;
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
  saveWorkspaceSettings(
    targetId: string,
    input: WorkspaceSettingsBatchInput,
  ): Promise<WorkspaceSettingsBatchResponse>;
  fetchWorkspaceSettingsBaseline(targetId: string): Promise<SettingsCheckpoint>;
  fetchWorkspaceSettingsCheckpoint(
    targetId: string,
    checkpointId: string,
  ): Promise<SettingsCheckpoint>;
  restoreWorkspaceSettingsCheckpoint(
    targetId: string,
    checkpointId: string,
    input: SettingsRestoreInput,
  ): Promise<WorkspaceSettingsBatchResponse>;
  fetchRepositories(
    targetId: string,
    request: RepositoryPageRequest,
  ): Promise<Page<RepositorySummary>>;
  fetchRepository(targetId: string, repositoryId: string): Promise<RepositoryDetail>;
  /**
   * Puts a refused TOML migration back on the table. A refusal is durable and
   * never expires, so this is the only way back from it.
   */
  resetConfigMigration(targetId: string, repositoryId: string): Promise<RepositoryDetail>;
  resetRootConfigMigration(targetId: string, repositoryId: string): Promise<RepositoryDetail>;
  fetchSyncConfig(targetId: string, kind: string): Promise<SyncConfig>;
  fetchSyncPaths(targetId: string): Promise<SyncPathIndex>;
  fetchSyncOverride(targetId: string, repositoryId: string, kind: string): Promise<SyncOverride>;
  fetchSyncPlan(targetId: string): Promise<{ plan: SyncPlan | null }>;
  approveSyncPlan(targetId: string, planId: string, digest: string): Promise<{ plan: SyncPlan }>;
  discardSyncPlan(targetId: string, planId: string): Promise<void>;
  fetchSyncStatus(targetId: string): Promise<SyncStatus>;
  fetchSyncFilesContext(targetId: string): Promise<SyncFilesContext>;
  renderSyncFile(targetId: string, input: SyncFileRenderInput): Promise<SyncFileRenderResponse>;
  fetchAudit(targetId: string, request: AuditHistoryRequest): Promise<Page<AuditEntry>>;
  fetchFailures(targetId: string, request: FailureHistoryRequest): Promise<Page<DeliveryFailure>>;
  signOut(): Promise<void>;
  signInUrl(invitation?: InvitationSignIn, returnTo?: string): string;
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

    /* A rejection here is a request that never reached anybody, and it is the one
       failure the panel cannot phrase from a status. Turned into the same error
       every other failure is, so a view that already handles one handles this. */
    return Promise.resolve(fetchImpl(url, init)).catch((cause: unknown) => {
      throw unreachable(cause);
    });
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
   * A payload whose `document` fields keep their numbers as they were written.
   *
   * A sync adjustment is somebody's file, and the service composes it keeping
   * every number's digits: `1.50` stays `1.50` and an identifier past 2^53
   * keeps its last four. A JavaScript number holds neither, so a document read
   * with `response.json()` came back a different document and the panel drew a
   * file one digit from the one the repository would get.
   *
   * Read twice rather than with one clever reviver: the same body parsed
   * normally, and again keeping every number's source text, with only the
   * `document` grafted across. Everything else in these payloads is read as a
   * number by something - `revision` is compared, `expected_revision` is sent
   * back - and a box in one of those would break a comparison rather than a
   * rendering.
   */
  const documentRequest = async <T>(path: string, init?: RequestInit): Promise<T> => {
    const body = await (await request(path, init)).text();
    const payload = JSON.parse(body) as T;
    const literal = parseJson(body);
    if (literal === undefined) return payload;

    return graftDocuments(payload, literal) as T;
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

  /**
   * A PUT whose answer carries a document, read keeping every number's digits.
   *
   * The same read as `documentRequest`, on the way back out. A save answered
   * through `putJson` came back through `response.json()`, so `12345678901234567890`
   * returned as `...67000` and `1.50` as `1.5` - and the caller writes that
   * answer straight into the query cache under the key the literal-preserving
   * read fills, so one save degraded the document the pane then composed from
   * and stored on the next one.
   */
  const putDocument = <T>(path: string, body: unknown): Promise<T> =>
    documentRequest<T>(path, {
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

  const postDocument = <T>(path: string, body: unknown): Promise<T> =>
    documentRequest<T>(path, {
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

    async fetchRootWorkspaces(): Promise<RootWorkspace[]> {
      const body = await jsonRequest<{ workspaces: RootWorkspace[] }>('/api/v1/root/workspaces');
      return body.workspaces;
    },

    async syncRootWorkspaces(): Promise<string[]> {
      const body = await postJson<{ target_ids: string[] }>(
        '/api/v1/root/workspaces/sync',
        undefined,
      );
      return body.target_ids;
    },

    fetchRootOverview(): Promise<RootOverview> {
      return jsonRequest('/api/v1/root/overview');
    },

    fetchRootPerformance(windowHours: number): Promise<ServicePerformance> {
      return jsonRequest(`/api/v1/root/performance?window=${windowHours}`);
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

    fetchRootQueue(query = ''): Promise<QueuePage> {
      return jsonRequest(`/api/v1/root/queue${query}`);
    },

    fetchTargetQueue(targetId: string, query = ''): Promise<QueuePage> {
      return jsonRequest(`/api/v1/targets/${pathSegment(targetId)}/queue${query}`);
    },

    fetchRootQueueItem(itemId: string): Promise<QueueDetail> {
      return jsonRequest(`/api/v1/root/queue/${pathSegment(itemId)}`);
    },

    fetchTargetQueueItem(targetId: string, itemId: string): Promise<QueueDetail> {
      return jsonRequest(`/api/v1/targets/${pathSegment(targetId)}/queue/${pathSegment(itemId)}`);
    },

    actOnRootQueue(itemId: string, input: QueueActionInput): Promise<QueueItem> {
      return postJson(`/api/v1/root/queue/${pathSegment(itemId)}/actions`, input);
    },

    actOnTargetQueue(
      targetId: string,
      itemId: string,
      input: QueueActionInput,
    ): Promise<QueueItem> {
      return postJson(
        `/api/v1/targets/${pathSegment(targetId)}/queue/${pathSegment(itemId)}/actions`,
        input,
      );
    },

    previewRootQueueAction(itemId: string, input: QueueActionInput): Promise<QueueSchedulePreview> {
      return postJson(`/api/v1/root/queue/${pathSegment(itemId)}/actions/preview`, input);
    },

    previewTargetQueueAction(
      targetId: string,
      itemId: string,
      input: QueueActionInput,
    ): Promise<QueueSchedulePreview> {
      return postJson(
        `/api/v1/targets/${pathSegment(targetId)}/queue/${pathSegment(itemId)}/actions/preview`,
        input,
      );
    },

    async fetchRootScheduleProfiles(): Promise<ScheduleProfile[]> {
      const body = await jsonRequest<{ profiles: ScheduleProfile[] }>(
        '/api/v1/root/schedule-profiles',
      );
      return body.profiles;
    },

    fetchRootJobPolicies(): Promise<RootJobPolicies> {
      return jsonRequest('/api/v1/root/job-policies');
    },

    async fetchRootScheduleRequests(): Promise<ScheduleRequest[]> {
      const body = await jsonRequest<{ requests: ScheduleRequest[] }>(
        '/api/v1/root/schedule-requests',
      );
      return body.requests;
    },

    fetchTargetSchedules(targetId: string): Promise<TargetSchedules> {
      return jsonRequest(`/api/v1/targets/${pathSegment(targetId)}/schedules`);
    },

    async fetchTargetScheduleRequests(targetId: string): Promise<ScheduleRequest[]> {
      const body = await jsonRequest<{ requests: ScheduleRequest[] }>(
        `/api/v1/targets/${pathSegment(targetId)}/schedule-requests`,
      );
      return body.requests;
    },

    createRootScheduleProfile(input: ScheduleProfileInput): Promise<ScheduleProfile> {
      return postJson('/api/v1/root/schedule-profiles', input);
    },

    updateRootScheduleProfile(id: string, input: ScheduleProfileInput): Promise<ScheduleProfile> {
      return putJson(`/api/v1/root/schedule-profiles/${pathSegment(id)}`, input);
    },

    archiveRootScheduleProfile(id: string, expectedRevision: number): Promise<ScheduleProfile> {
      return jsonRequest(
        `/api/v1/root/schedule-profiles/${pathSegment(id)}?expected_revision=${expectedRevision}`,
        { method: 'DELETE' },
      );
    },

    updateRootJobPolicy(kind: string, input: QueuePolicyInput): Promise<QueuePolicy> {
      return putJson(`/api/v1/root/job-policies/${pathSegment(kind)}`, input);
    },

    updateRootWorkspaceJobPolicy(
      targetId: string,
      kind: string,
      input: QueuePolicyInput,
    ): Promise<QueuePolicy> {
      return putJson(
        `/api/v1/root/workspaces/${pathSegment(targetId)}/job-policies/${pathSegment(kind)}`,
        input,
      );
    },

    deleteRootWorkspaceJobPolicy(
      targetId: string,
      kind: string,
      expectedRevision: number,
    ): Promise<QueuePolicy> {
      return jsonRequest(
        `/api/v1/root/workspaces/${pathSegment(targetId)}/job-policies/${pathSegment(kind)}` +
          `?expected_revision=${expectedRevision}`,
        { method: 'DELETE' },
      );
    },

    decideRootScheduleRequest(
      id: string,
      input: {
        approve: boolean;
        promote_profile: boolean;
        reason: string;
        expected_revision: number;
      },
    ): Promise<ScheduleRequest> {
      return postJson(`/api/v1/root/schedule-requests/${pathSegment(id)}/decision`, input);
    },

    createTargetScheduleRequest(
      targetId: string,
      input: ScheduleRequestInput,
    ): Promise<ScheduleRequest> {
      return postJson(`/api/v1/targets/${pathSegment(targetId)}/schedule-requests`, input);
    },

    withdrawTargetScheduleRequest(
      targetId: string,
      id: string,
      expectedRevision: number,
    ): Promise<ScheduleRequest> {
      return jsonRequest(
        `/api/v1/targets/${pathSegment(targetId)}/schedule-requests/${pathSegment(id)}` +
          `?expected_revision=${expectedRevision}`,
        { method: 'DELETE' },
      );
    },

    runSyncNow(
      targetId: string,
      input: { expected_revision: number; reason: string },
    ): Promise<SyncRunNowResponse> {
      return postJson(`/api/v1/targets/${pathSegment(targetId)}/sync/run-now`, input);
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
      return jsonRequest('/api/v1/root/runtime/settings');
    },

    saveRootRuntimeSettings(input: RootRuntimeSettingsInput): Promise<RootRuntimeSettings> {
      return putJson('/api/v1/root/runtime/settings', input);
    },

    fetchRootRuntimeSettingsBaseline(): Promise<SettingsCheckpoint> {
      return documentRequest('/api/v1/root/runtime/settings/checkpoints/baseline');
    },

    fetchRootRuntimeSettingsCheckpoint(checkpointId: string): Promise<SettingsCheckpoint> {
      return documentRequest(
        `/api/v1/root/runtime/settings/checkpoints/${pathSegment(checkpointId)}`,
      );
    },

    restoreRootRuntimeSettingsCheckpoint(
      checkpointId: string,
      input: SettingsRestoreInput,
    ): Promise<RootRuntimeSettings> {
      return postDocument(
        `/api/v1/root/runtime/settings/checkpoints/${pathSegment(checkpointId)}/restore`,
        input,
      );
    },

    /**
     * Where the same filtered audit can be fetched as a file.
     *
     * An address rather than a request: the browser downloads it with the session
     * cookie it already has, which is what makes the button a link rather than a
     * fetch that would have to hold the whole export in memory to hand it back.
     */
    rootAuditExportHref(history: AuditHistoryRequest): string {
      return panelUrl(
        base,
        withHistoryQuery('/api/v1/root/history/audit.csv', history, {
          category: history.categories ?? [],
        }),
      );
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
        Page<{ workspace: DeliveryFailure['workspace']; failure: DeliveryFailure }>
      >(
        withHistoryQuery('/api/v1/root/history/failures', history, {
          kind: history.kind,
        }),
      );
      return {
        ...page,
        items: page.items.map(({ workspace, failure }) => ({ ...failure, workspace })),
      };
    },

    fetchRootTargetSettings(targetId: string): Promise<PanelTarget> {
      return jsonRequest(`/api/v1/root/workspaces/${pathSegment(targetId)}/settings`);
    },

    saveRootWorkspaceSettings(
      targetId: string,
      input: WorkspaceSettingsBatchInput,
    ): Promise<WorkspaceSettingsBatchResponse> {
      return putDocument(`/api/v1/root/workspaces/${pathSegment(targetId)}/settings`, input);
    },

    fetchRootWorkspaceSettingsBaseline(targetId: string): Promise<SettingsCheckpoint> {
      return documentRequest(
        `/api/v1/root/workspaces/${pathSegment(targetId)}/settings/checkpoints/baseline`,
      );
    },

    fetchRootWorkspaceSettingsCheckpoint(
      targetId: string,
      checkpointId: string,
    ): Promise<SettingsCheckpoint> {
      return documentRequest(
        `/api/v1/root/workspaces/${pathSegment(targetId)}/settings/checkpoints/${pathSegment(checkpointId)}`,
      );
    },

    restoreRootWorkspaceSettingsCheckpoint(
      targetId: string,
      checkpointId: string,
      input: SettingsRestoreInput,
    ): Promise<WorkspaceSettingsBatchResponse> {
      return postDocument(
        `/api/v1/root/workspaces/${pathSegment(targetId)}/settings/checkpoints/${pathSegment(checkpointId)}/restore`,
        input,
      );
    },

    fetchRootRepositories(
      targetId: string,
      repositoryPage: RepositoryPageRequest,
    ): Promise<Page<RepositorySummary>> {
      return jsonRequest(
        withRepositoryQuery(
          `/api/v1/root/workspaces/${pathSegment(targetId)}/repositories`,
          repositoryPage,
        ),
      );
    },

    fetchRootRepository(targetId: string, repositoryId: string): Promise<RepositoryDetail> {
      return jsonRequest(
        `/api/v1/root/workspaces/${pathSegment(targetId)}/repositories/${pathSegment(repositoryId)}`,
      );
    },

    fetchRootElevation(targetId: string): Promise<RootElevation> {
      return jsonRequest(`/api/v1/root/workspaces/${pathSegment(targetId)}/elevation`);
    },

    beginRootElevation(targetId: string, input: RootElevationInput): Promise<RootElevation> {
      return postJson(`/api/v1/root/workspaces/${pathSegment(targetId)}/elevation`, input);
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
        withAccessPageQuery(`/api/v1/root/workspaces/${pathSegment(targetId)}/users`, userPage),
      );
    },

    addRootTargetUser(targetId: string, input: AddTargetUserInput): Promise<PanelUser> {
      return postJson(`/api/v1/root/workspaces/${pathSegment(targetId)}/users`, input);
    },

    updateRootTargetUser(
      targetId: string,
      accountId: string,
      input: UpdateTargetUserInput,
    ): Promise<PanelUser> {
      return putJson(
        `/api/v1/root/workspaces/${pathSegment(targetId)}/users/${pathSegment(accountId)}`,
        input,
      );
    },

    fetchRootTargetInvitations(
      targetId: string,
      invitationPage: InvitationPageRequest,
    ): Promise<Page<PanelInvitation>> {
      return jsonRequest(
        withAccessPageQuery(
          `/api/v1/root/workspaces/${pathSegment(targetId)}/invitations`,
          invitationPage,
        ),
      );
    },

    createRootTargetInvitation(
      targetId: string,
      input: AddTargetInvitationInput,
    ): Promise<PanelInvitation> {
      return postJson(`/api/v1/root/workspaces/${pathSegment(targetId)}/invitations`, input);
    },

    reissueRootTargetInvitation(
      targetId: string,
      invitationId: string,
      expiresInDays: InvitationDays,
    ): Promise<PanelInvitation> {
      return postJson(
        `/api/v1/root/workspaces/${pathSegment(targetId)}/invitations/${pathSegment(invitationId)}/reissue`,
        { expires_in_days: expiresInDays },
      );
    },

    revokeRootTargetInvitation(targetId: string, invitationId: string): Promise<PanelInvitation> {
      return jsonRequest(
        `/api/v1/root/workspaces/${pathSegment(targetId)}/invitations/${pathSegment(invitationId)}`,
        { method: 'DELETE' },
      );
    },

    async fetchRootTargetUserDecisions(
      accountId: string,
      targetId: string,
    ): Promise<AccessDecision[]> {
      const body = await jsonRequest<{ decisions: AccessDecision[] }>(
        `/api/v1/root/workspaces/${pathSegment(targetId)}/users/${pathSegment(accountId)}/decisions`,
      );
      return body.decisions;
    },

    fetchRootTargetAudit(
      targetId: string,
      history: AuditHistoryRequest,
    ): Promise<Page<AuditEntry>> {
      return jsonRequest(
        withHistoryQuery(`/api/v1/root/workspaces/${pathSegment(targetId)}/audit`, history, {
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
        withHistoryQuery(`/api/v1/root/workspaces/${pathSegment(targetId)}/failures`, history, {
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

    markAllNotificationsRead(): Promise<{ read: number }> {
      return putJson('/api/v1/notifications/read', {});
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
      return suggest(`/api/v1/root/workspaces/${pathSegment(targetId)}/user-suggestions`, query);
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

    saveWorkspaceSettings(
      targetId: string,
      input: WorkspaceSettingsBatchInput,
    ): Promise<WorkspaceSettingsBatchResponse> {
      return putDocument(`/api/v1/targets/${pathSegment(targetId)}/settings`, input);
    },

    fetchWorkspaceSettingsBaseline(targetId: string): Promise<SettingsCheckpoint> {
      return documentRequest(
        `/api/v1/targets/${pathSegment(targetId)}/settings/checkpoints/baseline`,
      );
    },

    fetchWorkspaceSettingsCheckpoint(
      targetId: string,
      checkpointId: string,
    ): Promise<SettingsCheckpoint> {
      return documentRequest(
        `/api/v1/targets/${pathSegment(targetId)}/settings/checkpoints/${pathSegment(checkpointId)}`,
      );
    },

    restoreWorkspaceSettingsCheckpoint(
      targetId: string,
      checkpointId: string,
      input: SettingsRestoreInput,
    ): Promise<WorkspaceSettingsBatchResponse> {
      return postDocument(
        `/api/v1/targets/${pathSegment(targetId)}/settings/checkpoints/${pathSegment(checkpointId)}/restore`,
        input,
      );
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

    resetRootConfigMigration(targetId: string, repositoryId: string): Promise<RepositoryDetail> {
      return postJson(
        `/api/v1/root/workspaces/${pathSegment(targetId)}/repositories/` +
          `${pathSegment(repositoryId)}/config-migration`,
        {},
      );
    },

    resetConfigMigration(targetId: string, repositoryId: string): Promise<RepositoryDetail> {
      return postJson(
        `/api/v1/targets/${pathSegment(targetId)}/repositories/${pathSegment(repositoryId)}` +
          '/config-migration',
        {},
      );
    },

    /* Through `documentRequest`, like the two override reads: this is the
       TEMPLATE, which is what `composeFile` starts from, so a number that lost
       its digits here loses them in every repository's composed file. */
    fetchSyncConfig(targetId: string, kind: string): Promise<SyncConfig> {
      return documentRequest(
        `/api/v1/targets/${pathSegment(targetId)}/sync/config/${pathSegment(kind)}`,
      );
    },

    /**
     * Every path this workspace's repositories are known to hold.
     *
     * Fetched whole and matched in the browser: it is a list the workspace
     * already has, it changes about once a day, and a request per keystroke to
     * filter it would be a request per keystroke.
     */
    fetchSyncPaths(targetId: string): Promise<SyncPathIndex> {
      return jsonRequest(`/api/v1/targets/${pathSegment(targetId)}/sync/paths`);
    },

    fetchSyncOverride(targetId: string, repositoryId: string, kind: string): Promise<SyncOverride> {
      return documentRequest(
        `/api/v1/targets/${pathSegment(targetId)}/repositories/` +
          `${pathSegment(repositoryId)}/sync/${pathSegment(kind)}`,
      );
    },

    fetchSyncPlan(targetId: string): Promise<{ plan: SyncPlan | null }> {
      return jsonRequest(`/api/v1/targets/${pathSegment(targetId)}/sync/plan`);
    },

    fetchSyncStatus(targetId: string): Promise<SyncStatus> {
      return jsonRequest(`/api/v1/targets/${pathSegment(targetId)}/sync/status`);
    },

    fetchSyncFilesContext(targetId: string): Promise<SyncFilesContext> {
      return jsonRequest(`/api/v1/targets/${pathSegment(targetId)}/sync/files/context`);
    },

    async renderSyncFile(
      targetId: string,
      input: SyncFileRenderInput,
    ): Promise<SyncFileRenderResponse> {
      const response = await postJson<unknown>(
        `/api/v1/targets/${pathSegment(targetId)}/sync/files/render`,
        input,
      );
      return parseSyncFileRenderResponse(response);
    },

    // The digest goes back with the approval. It is what says the plan on the
    // screen is the plan in the database, so approving without it would agree
    // to whatever the configuration says by the time the request lands.
    approveSyncPlan(targetId: string, planId: string, digest: string): Promise<{ plan: SyncPlan }> {
      return postJson(
        `/api/v1/targets/${pathSegment(targetId)}/sync/plans/${pathSegment(planId)}/approval`,
        { digest },
      );
    },

    // Named rather than approved-away: a discarded plan asks nothing on
    // GitHub, and the next sweep computes a fresh one from whatever the
    // configuration says by then.
    async discardSyncPlan(targetId: string, planId: string): Promise<void> {
      await jsonRequest(
        `/api/v1/targets/${pathSegment(targetId)}/sync/plans/${pathSegment(planId)}`,
        { method: 'DELETE' },
      );
    },

    /** One workspace's filtered audit as a file, on the same terms as the console's. */
    auditExportHref(targetId: string, history: AuditHistoryRequest): string {
      return panelUrl(
        base,
        withHistoryQuery(`/api/v1/targets/${pathSegment(targetId)}/audit.csv`, history, {
          scope: history.scope ?? 'all',
          change: history.change ?? 'all',
        }),
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
          ...(history.since === undefined ? {} : { since: history.since }),
        }),
      );
    },

    async signOut(): Promise<void> {
      await request('/api/v1/sign-out', { method: 'POST' });
    },

    signInUrl(invitation?: InvitationSignIn, returnTo?: string): string {
      const path = panelUrl(base, '/auth/github/start');
      const query = new URLSearchParams();
      if (invitation !== undefined) {
        query.set('invite', invitation.token);
        query.set('action', invitation.action);
      }
      /* Where to come back to. The server decides whether it will honour this -
         a redirect target that arrives from the browser is an open redirect
         until something checks it - so this only has to say what was asked. */
      if (returnTo !== undefined && returnTo !== '') query.set('return_to', returnTo);
      const search = query.toString();

      return search === '' ? path : `${path}?${search}`;
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

/**
 * What to say when the answer carries no words of its own.
 *
 * The service sends an envelope with a sentence in it, and that sentence is
 * always preferred. Nothing else does: a proxy, a crash page, a gateway, a dev
 * server that lost its API middleware - each answers with a status and a body the
 * panel cannot read, and the fallback used to be `panel request failed with
 * status 404`, which is a line from a log put in front of a reader inside a red
 * banner. These say what happened instead, and none of them ends in a stop: the
 * views that show them compose them after a subject of their own.
 */
function describeStatus(status: number): string {
  if (status === 0) return 'the service could not be reached';
  if (status === 401) return 'this session is no longer signed in';
  if (status === 403) return 'this account is not allowed to see it';
  if (status === 404) return 'the service does not recognise this request';
  if (status === 409) return 'it changed while this page was open';
  if (status === 429) return 'too many requests went out at once';
  if (status >= 500) return 'the service could not answer';

  return 'the service refused the request';
}

/**
 * A request that never reached anybody.
 *
 * `fetch` rejects rather than resolving when the host is down, the network is
 * gone, or the connection is refused - which is exactly what a reader sees when
 * the service restarts under them. Untranslated, `TypeError: Failed to fetch`
 * reached the banner verbatim. Status 0 because there was no response to take one
 * from, and every caller already branches on `PanelApiError` rather than on the
 * shape of whatever `fetch` threw.
 */
export function unreachable(cause: unknown): PanelApiError {
  const failure = new PanelApiError(0, 'unreachable', describeStatus(0));

  return Object.assign(failure, { cause });
}

/**
 * Put the literal-preserving copy of every `document` back into the payload.
 *
 * Only that key, and only where both copies hold an object: this walks two
 * readings of one body, so the shapes match by construction, and anything it
 * does not recognise is left as the ordinary reading had it.
 */
function graftDocuments(payload: unknown, literal: JsonValue): unknown {
  if (Array.isArray(payload) && Array.isArray(literal)) {
    return payload.map((item, index) => graftDocuments(item, literal[index] ?? null));
  }
  if (typeof payload !== 'object' || payload === null || Array.isArray(payload)) return payload;
  if (typeof literal !== 'object' || literal === null || Array.isArray(literal)) return payload;

  const grafted: Record<string, unknown> = { ...(payload as Record<string, unknown>) };
  for (const key of Object.keys(grafted)) {
    const beside = (literal as Record<string, JsonValue>)[key];
    if (beside === undefined) continue;
    grafted[key] = key === 'document' ? beside : graftDocuments(grafted[key], beside);
  }

  return grafted;
}

async function readError(response: Response): Promise<PanelApiError> {
  let code = 'unknown';
  let message = describeStatus(response.status);
  let kind: string | undefined;
  let conflicts: WorkspaceSettingsConflict[] = [];
  try {
    const text = await response.text();
    const body = JSON.parse(text) as Partial<PanelErrorBody>;
    const literal = parseJson(text);
    const preserved = literal === undefined ? body : (graftDocuments(body, literal) as typeof body);
    if (body.error?.code !== undefined) {
      code = body.error.code;
    }
    if (body.error?.message !== undefined && body.error.message !== '') {
      message = body.error.message;
    }
    kind = body.error?.kind;
    conflicts = preserved.error?.conflicts ?? [];
  } catch {
    // Proxies and crashes are not required to understand the panel envelope.
  }
  return new PanelApiError(response.status, code, message, kind, conflicts);
}
