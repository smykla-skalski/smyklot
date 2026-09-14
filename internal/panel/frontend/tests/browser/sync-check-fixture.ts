import type { QueueItem, SyncCheckOutcome, SyncCheckResponse } from '../../src/lib/types';

/** Explicit API fixture for tests that vary historical and execution facts independently. */
export function checkResponse(item: QueueItem): SyncCheckResponse {
  if (item.kind !== 'sync_scan') throw new Error('Expected a sync check fixture');
  const target = item.target_id ?? '2001';
  return {
    check_id: item.id,
    target_id: target,
    observed_at: new Date().toISOString(),
    result:
      item.details?.outcome || item.details?.result_plan_id
        ? {
            outcome: item.details.outcome as SyncCheckOutcome | undefined,
            result_plan_id: item.details.result_plan_id as string | undefined,
          }
        : null,
    execution: {
      state: item.state,
      summary: item.summary ?? '',
      progress_current: item.progress_current,
      progress_total: item.progress_total,
      attempt: item.attempt,
      started_at: item.started_at ?? null,
      finished_at: item.finished_at ?? null,
    },
    check: {
      action: 'check',
      target_id: target,
      available: true,
      reason: 'available',
      effect: 'request_repository_check',
    },
  };
}
