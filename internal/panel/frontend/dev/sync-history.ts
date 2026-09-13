import { mockLiveSyncPlan } from './sync-run-now.js';
import type { MockState } from './fixtures';
import type { Page, SyncPlan, SyncPlanSummary } from '../src/lib/types';

export function mockSyncHistory(state: MockState, targetId: string): SyncPlan[] {
  const plans = new Map((state.syncHistory.get(targetId) ?? []).map((plan) => [plan.id, plan]));
  const live = mockLiveSyncPlan(state, targetId) ?? state.syncPlans.get(targetId);
  if (live) plans.set(live.id, live);
  return [...plans.values()].sort(
    (a, b) => b.computed_at.localeCompare(a.computed_at) || b.id.localeCompare(a.id),
  );
}

export function mockSyncHistoryPage(
  plans: SyncPlan[],
  limit: number,
  cursor: string | null,
): Page<SyncPlanSummary> {
  const before = cursor
    ? (JSON.parse(Buffer.from(cursor, 'base64url').toString()) as {
        computed_at: string;
        id: string;
      })
    : null;
  const remaining = before
    ? plans.filter(
        (plan) =>
          plan.computed_at < before.computed_at ||
          (plan.computed_at === before.computed_at && plan.id < before.id),
      )
    : plans;
  const selected = remaining.slice(0, limit);
  const last = selected.at(-1);
  return {
    items: selected.map(({ id, trigger, state, counts, computed_at, finished_at }) => ({
      id,
      trigger,
      state,
      counts,
      computed_at,
      finished_at,
    })),
    total: plans.length,
    next_cursor:
      remaining.length > limit && last
        ? Buffer.from(JSON.stringify({ computed_at: last.computed_at, id: last.id })).toString(
            'base64url',
          )
        : null,
  };
}
