import { PanelApiError, type PanelApi } from './api';
import { SyncRequestIntentStore } from './sync-request-intent';
import type { SyncRunNowInput, SyncRunNowIntent } from './types';

export type AcceptedSyncRequest = { action: 'check' | 'dispatch'; key: string };
export interface SyncRequestDependencies {
  runSyncNow: PanelApi['runSyncNow'];
  fetchOperation?: PanelApi['fetchSyncOperation'];
  refresh: (targetId: string) => Promise<void>;
  storage: () => Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;
}

/** One actor/workspace owns an in-flight command across all of its routed views. */
export class SyncRequestController {
  runningNow = $state(false);
  runNotice = $state('');
  relatedPlan = $state<{ id: string; label: string } | null>(null);
  requestedCheckId = $state<string | null>(null);
  requestedDispatchId = $state<string | null>(null);
  pendingRequest = $state.raw<SyncRunNowInput | null>(null);
  requestStorageProblem = $state<string | null>(null);
  requestUncertain = $state(false);
  requestCleanupPending = $state(false);
  requestNotRecorded = $state(false);
  localAcceptance = $state<AcceptedSyncRequest | null>(null);
  error = $state<string | null>(null);
  private requestStore: SyncRequestIntentStore | null = null;

  constructor(
    readonly actorId: string,
    readonly targetId: string,
    private readonly dependencies: SyncRequestDependencies,
  ) {}

  readRequestStorage(): void {
    if (this.runningNow || this.pendingRequest) return;
    if (!this.actorId) {
      this.requestStorageProblem = 'Your account must be loaded before requesting sync.';
      return;
    }
    try {
      this.requestStore = new SyncRequestIntentStore(
        this.dependencies.storage(),
        this.actorId,
        this.targetId,
      );
      this.pendingRequest = this.requestStore.read();
      this.requestUncertain = this.pendingRequest !== null;
      this.requestStorageProblem = null;
    } catch {
      this.requestStorageProblem =
        'Browser storage could not be read. Retry access to recover any saved request. New requests remain paused.';
    }
  }

  async confirmRequest(canControl: boolean, onConfirmed?: () => void): Promise<boolean> {
    const request = this.pendingRequest;
    if (!request || !this.dependencies.fetchOperation || this.runningNow) return false;
    const requestTargetId = this.targetId;
    this.runningNow = true;
    this.requestNotRecorded = false;
    this.error = null;
    try {
      const operation = await this.dependencies.fetchOperation(
        requestTargetId,
        request.action,
        request.request_key,
      );
      const accepted = operation.acceptance;
      if (
        operation.target_id !== requestTargetId ||
        accepted.action !== request.action ||
        accepted.request_key !== request.request_key ||
        accepted.reason !== request.reason.trim() ||
        (accepted.action === 'check' && !accepted.check_id?.trim()) ||
        (accepted.action === 'dispatch' && !accepted.queue_id?.trim()) ||
        (request.action === 'dispatch' &&
          (accepted.action !== 'dispatch' ||
            accepted.plan_id !== request.plan_id ||
            accepted.expected_revision !== request.expected_revision))
      )
        throw new Error(
          'The saved request could not be matched to this response. Keep it and try confirming again.',
        );
      onConfirmed?.();
      this.localAcceptance = { action: request.action, key: request.request_key };
      this.requestCleanupPending = true;
      this.requestUncertain = false;
      this.finishRequestRecovery();
      this.runNotice = 'Your request was accepted. Open it to see what happened.';
      return true;
    } catch (cause) {
      this.requestNotRecorded = cause instanceof PanelApiError && cause.status === 404;
      this.error = this.requestNotRecorded
        ? canControl
          ? 'Your request is not recorded yet. It may still be arriving. Confirm again, or retry the original request without creating a new one.'
          : 'Your request is not recorded yet. It may still be arriving. Confirm again to check its status.'
        : messageOf(cause);
      return false;
    } finally {
      this.runningNow = false;
    }
  }

  async submit(input: SyncRunNowIntent): Promise<void> {
    if (this.runningNow) return;
    if (this.pendingRequest && input.request_key !== this.pendingRequest.request_key) {
      this.error = 'Recover the previous sync request before starting another.';
      return;
    }
    const requestTargetId = this.targetId;
    const recovering = this.requestUncertain;
    this.localAcceptance = null;
    this.requestNotRecorded = false;
    this.runningNow = true;
    this.error = null;
    this.runNotice = '';
    this.relatedPlan = null;
    this.requestedCheckId = null;
    this.requestedDispatchId = null;
    try {
      if (!this.pendingRequest) {
        try {
          if (!this.requestStore || this.requestStorageProblem)
            throw new Error('Storage unavailable');
          this.pendingRequest = this.requestStore.begin(input);
        } catch {
          this.requestStorageProblem =
            'The request could not be saved in browser storage and has not been sent. Retry browser storage before starting again.';
          return;
        }
      }
      // The exact pending command is already known. Storage failure must not prevent
      // its idempotent recovery or replace its identity with a new request.
      const request = this.pendingRequest;
      const response = await this.dependencies.runSyncNow(requestTargetId, request);
      const responsePlanId = response.plan?.id;
      const validPlan =
        typeof responsePlanId === 'string' &&
        responsePlanId.trim() !== '' &&
        responsePlanId === responsePlanId.trim() &&
        (request.action === 'check' || responsePlanId === request.plan_id);
      const valid =
        request.action === 'check'
          ? (response.status === 'changes_pending' && validPlan) ||
            (response.status === 'check_accepted' && !!response.check_id)
          : ((response.status === 'approval_required' || response.status === 'already_running') &&
              validPlan) ||
            (response.status === 'dispatch_accepted' &&
              response.plan_id === request.plan_id &&
              !!response.queue_id);
      if (!valid)
        throw new Error(
          'The sync response could not be confirmed. Recover the request before starting another.',
        );
      if (
        response.status === 'changes_pending' ||
        response.status === 'approval_required' ||
        response.status === 'already_running'
      ) {
        this.relatedPlan = {
          id: responsePlanId!,
          label:
            response.status === 'changes_pending'
              ? 'Review earlier changes'
              : response.status === 'approval_required'
                ? 'Review changes'
                : 'View changes',
        };
      }
      this.requestCleanupPending = true;
      this.requestUncertain = false;
      this.finishRequestRecovery();
      if (response.status === 'check_accepted' || response.status === 'dispatch_accepted') {
        this.localAcceptance = { action: request.action, key: request.request_key };
        this.runNotice = 'Your request was accepted. Open it to see what happened.';
        if (response.status === 'check_accepted') this.requestedCheckId = response.check_id!;
        else this.requestedDispatchId = response.plan_id!;
      }
      if (response.status === 'changes_pending')
        this.runNotice = 'No check was started because earlier changes were pending.';
      if (response.status === 'approval_required')
        this.runNotice = 'No run was started because these changes needed approval.';
      if (response.status === 'already_running')
        this.runNotice = 'No new run was started because these changes were already running.';
      await this.dependencies.refresh(requestTargetId);
    } catch (cause) {
      if (
        !recovering &&
        this.pendingRequest &&
        cause instanceof PanelApiError &&
        [400, 404, 409].includes(cause.status) &&
        [
          'invalid_request',
          'not_found',
          'stale_revision',
          'unsupported_plan_state',
          'conflict',
        ].includes(cause.code)
      ) {
        try {
          this.requestStore!.clear(this.pendingRequest.request_key);
          this.pendingRequest = null;
          void this.dependencies.refresh(requestTargetId);
        } catch {
          /* Preserve the request if storage cannot confirm its removal. */
        }
      }
      if (this.pendingRequest) this.requestUncertain = true;
      this.error = messageOf(cause);
    } finally {
      this.runningNow = false;
    }
  }

  finishRequestRecovery(): void {
    if (!this.pendingRequest || !this.requestCleanupPending) return;
    try {
      if (!this.requestStore) throw new Error('Storage unavailable');
      this.requestStore.clear(this.pendingRequest.request_key);
      this.pendingRequest = null;
      this.requestCleanupPending = false;
      this.requestUncertain = false;
      this.requestStorageProblem = null;
    } catch {
      this.requestStorageProblem =
        'The response was received, but its saved request could not be cleared. New requests are paused.';
    }
  }
}

function messageOf(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause);
}
