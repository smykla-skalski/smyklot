import { randomUUID } from 'node:crypto';
import { syncPlanSeed, type MockState } from './fixtures.js';
import { SYNC_KINDS, type QueueItem, type SyncCell, type SyncPlan } from '../src/lib/types.js';
import { recordMockSyncEvent } from './sync-queue.js';

type State = Pick<MockState, 'syncStatus' | 'syncPlans' | 'queue' | 'syncQueueEvents'>;

/** Refresh only evidence represented by the development fixture, never a saved policy alone. */
export function finishMockSyncScan(state: State, item: QueueItem, at: string): string {
  const targetId = item.target_id!;
  if (['computed', 'approved', 'applying'].includes(state.syncPlans.get(targetId)?.state ?? ''))
    return 'A sync plan is already in progress. See Sync status for details.';
  const status = state.syncStatus.get(targetId);
  if (!status || status.repositories.length === 0) return 'No enabled repositories to check';
  const template = syncPlanSeed((offset) => new Date(Date.parse(at) + offset).toISOString());
  const actions: SyncPlan['actions'] = [];
  const counts = new Map<string, number>();
  for (const row of status.repositories) {
    for (const kind of SYNC_KINDS) {
      const old = row.cells[kind];
      if (old.state === 'off') continue;
      let cell = observed(old, at, row.reason);
      if (cell.observed_outcome === 'different') {
        const candidates = template.actions.filter(
          (action) => action.repository === row.repository && action.kind === kind,
        );
        if (candidates.length === 0) {
          cell = {
            state: 'check_failed',
            observed_outcome: 'failed',
            observed_at: at,
            reason:
              'The development fixture has no change details for this repository. No changes were queued.',
          };
        } else {
          actions.push(...candidates);
          cell.state = 'pending';
          cell.changes = candidates.length;
        }
      }
      row.cells[kind] = cell;
      const outcome = cell.observed_outcome ?? 'unknown';
      counts.set(outcome, (counts.get(outcome) ?? 0) + 1);
    }
  }
  if (counts.size > 0) status.latest_observed_at = at;
  if (actions.length > 0) {
    const resultPlanId = queuePlan(state, targetId, template, actions, at);
    item.details = { ...item.details, result_plan_id: resultPlanId };
  }
  const labels = [
    ['failed', 'failed'],
    ['blocked', 'could not proceed'],
    ['different', 'found differences'],
    ['proposed', 'found an open proposal'],
    ['declined', 'found a declined proposal'],
    ['matched', 'matched saved settings'],
  ];
  const parts = labels.flatMap(([key, label]) => {
    const count = counts.get(key!) ?? 0;
    return count ? [`${count} ${count === 1 ? 'check' : 'checks'} ${label}`] : [];
  });
  if (!parts.length) return 'No enabled repositories to check';
  if (actions.length) parts.push(`${actions.length} changes queued`);
  return `${parts.join('. ')}. See Sync status for details.`;
}

function observed(cell: SyncCell, at: string, repositoryReason?: string): SyncCell {
  const states: Partial<Record<SyncCell['state'], SyncCell['observed_outcome']>> = {
    in_step: 'matched',
    applied: 'matched',
    pending: 'different',
    needs_sync: 'different',
    proposed: 'proposed',
    declined: 'declined',
    refused: 'blocked',
    check_failed: 'failed',
  };
  const outcome = states[cell.state];
  if (!outcome)
    return {
      state: 'check_failed',
      observed_outcome: 'failed',
      observed_at: at,
      reason:
        'The development fixture has no fresh repository evidence for these saved settings. No changes were queued.',
    };
  return {
    ...cell,
    state: cell.state === 'applied' ? 'in_step' : cell.state,
    observed_at: at,
    observed_outcome: outcome,
    changes: 0,
    reason: cell.reason ?? (outcome === 'blocked' ? repositoryReason : undefined),
  };
}

function queuePlan(
  state: State,
  targetId: string,
  template: SyncPlan,
  actions: SyncPlan['actions'],
  at: string,
): string {
  const id = `plan:${randomUUID()}`;
  const plan: SyncPlan = {
    ...template,
    id,
    digest: `mock:${id}`,
    trigger: 'manual',
    state: 'approved',
    execution_stage: 'Queued for automatic sync',
    computed_at: at,
    approved_at: at,
    actions: structuredClone(actions),
    counts: actions.reduce(
      (counts, action) => ({ ...counts, [action.operation]: counts[action.operation] + 1 }),
      { create: 0, update: 0, delete: 0 },
    ),
  };
  const item: QueueItem = {
    id: `sync-apply:${randomUUID()}`,
    kind: 'sync_apply',
    lane: 'maintenance',
    target_id: targetId,
    source_kind: 'sync_plan',
    source_id: id,
    title: 'Sync shared configuration',
    summary: `${actions.length} changes queued automatically`,
    state: 'ready',
    priority: 'normal',
    priority_overridden: false,
    window_mode: 'respect',
    immediate: false,
    not_before: at,
    eligible_at: at,
    estimated_start_at: at,
    work_ahead: 0,
    progress_current: 0,
    progress_total: actions.length,
    attempt: 0,
    revision: 1,
    created_at: at,
    updated_at: at,
    details: plan.counts,
  };
  state.syncPlans.set(targetId, plan);
  state.queue.push(item);
  recordMockSyncEvent(state, item, 'created', item.summary!, at);
  return plan.id;
}
