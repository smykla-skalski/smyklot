import { VIEWER, type MockState } from './fixtures.js';
import { mockCheckCapability, mockDispatchCapability } from './sync-capability.js';
import type { SyncOperationResponse, SyncRequestAcceptance } from '../src/lib/types.js';

type Reply =
  | { status: 200; body: SyncOperationResponse }
  | { status: 400 | 404; body: { error: { code: string; message: string } } };

export function mockSyncOperation(
  state: MockState,
  targetId: string,
  action: string,
  requestKey: string,
  params: URLSearchParams,
  now = Date.now(),
  actorId = VIEWER.id,
): Reply {
  const missing = (): Reply => ({
    status: 404,
    body: { error: { code: 'not_found', message: 'Accepted request not found' } },
  });
  const target = state.targets.find((target) => target.value.id === targetId);
  if (!target || !['owner', 'admin', 'editor', 'viewer'].includes(target.value.effective_role))
    return missing();
  if (
    params.size ||
    !['check', 'dispatch'].includes(action) ||
    !requestKey ||
    requestKey.trim() !== requestKey ||
    new TextEncoder().encode(requestKey).length > 200
  )
    return {
      status: 400,
      body: {
        error: {
          code: 'invalid_request_lookup',
          message: 'An exact check or dispatch request identity is required',
        },
      },
    };
  const key = JSON.stringify([actorId, requestKey]);
  let acceptance: SyncRequestAcceptance;
  let queueId: string;
  if (action === 'check') {
    const receipt = state.syncCheckReceipts.get(key);
    if (!receipt || receipt.targetId !== targetId) return missing();
    acceptance = {
      action: 'check',
      request_key: requestKey,
      reason: receipt.reason,
      accepted_at: receipt.acceptedAt,
      check_id: receipt.checkId,
    };
    queueId = receipt.checkId;
  } else {
    const receipt = state.syncDispatchReceipts.get(key);
    if (!receipt || receipt.targetId !== targetId) return missing();
    acceptance = {
      action: 'dispatch',
      request_key: requestKey,
      reason: receipt.reason,
      accepted_at: receipt.acceptedAt,
      plan_id: receipt.planId,
      queue_id: receipt.queueId,
      expected_revision: receipt.expectedRevision,
    };
    queueId = receipt.queueId;
  }
  const item = state.queue.find(
    (item) =>
      item.id === queueId &&
      item.target_id === targetId &&
      (acceptance.action === 'check'
        ? item.kind === 'sync_scan'
        : item.kind === 'sync_apply' &&
          item.source_kind === 'sync_plan' &&
          item.source_id === acceptance.plan_id),
  );
  const comparison =
    acceptance.action === 'check' ? state.syncCheckResults.get(acceptance.check_id) : undefined;
  const live = state.syncPlans.get(targetId);
  const plan =
    acceptance.action === 'dispatch'
      ? live?.id === acceptance.plan_id
        ? live
        : (state.syncHistory.get(targetId) ?? []).find((plan) => plan.id === acceptance.plan_id)
      : undefined;
  const observedAt = new Date(now).toISOString();
  return {
    status: 200,
    body: {
      target_id: targetId,
      acceptance,
      observation_started_at: observedAt,
      observed_at: observedAt,
      comparison: comparison?.targetId === targetId ? structuredClone(comparison.result) : null,
      plan: plan
        ? {
            id: plan.id,
            trigger: plan.trigger,
            state: plan.state,
            counts: { ...plan.counts },
            computed_at: plan.computed_at,
            ...(plan.finished_at ? { finished_at: plan.finished_at } : {}),
          }
        : null,
      execution: item
        ? {
            state: item.state,
            summary: item.summary ?? '',
            progress_current: item.progress_current,
            progress_total: item.progress_total,
            attempt: item.attempt,
            started_at: item.started_at ?? null,
            finished_at: item.finished_at ?? null,
          }
        : null,
      check: mockCheckCapability(state, targetId, now),
      dispatch: plan
        ? mockDispatchCapability(plan, targetId, item, target.value.effective_role, now)
        : null,
    },
  };
}
