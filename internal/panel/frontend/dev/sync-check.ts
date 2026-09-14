import type { MockState } from './fixtures.js';
import type { SyncCheckResponse } from '../src/lib/types.js';
import { mockCheckCapability } from './sync-capability.js';

type Reply = {
  status: number;
  body: SyncCheckResponse | { error: { code: string; message: string } };
};

export function mockSyncCheck(
  state: MockState,
  targetId: string,
  checkId: string,
  now = Date.now(),
): Reply {
  const missing = (): Reply => ({
    status: 404,
    body: { error: { code: 'not_found', message: 'Check not found' } },
  });
  const target = state.targets.find((target) => target.value.id === targetId);
  if (!target || target.value.effective_role === 'none') return missing();
  const retained = state.syncCheckResults.get(checkId);
  const result = retained?.targetId === targetId ? structuredClone(retained.result) : null;
  const item = state.queue.find(
    (item) => item.id === checkId && item.target_id === targetId && item.kind === 'sync_scan',
  );
  if (!result && !item) return missing();
  return {
    status: 200,
    body: {
      check_id: checkId,
      target_id: targetId,
      observed_at: new Date(now).toISOString(),
      result,
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
    },
  };
}
