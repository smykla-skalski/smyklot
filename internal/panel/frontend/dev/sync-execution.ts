import type { MockState } from './fixtures.js';
import {
  SYNC_KINDS,
  type QueueItem,
  type SyncAction,
  type SyncKind,
  type SyncPlan,
} from '../src/lib/types.js';
import { finishMockSyncScan } from './sync-scan.js';
import { recordMockSyncEvent } from './sync-queue.js';

type State = Pick<
  MockState,
  | 'queue'
  | 'syncPlans'
  | 'syncHistory'
  | 'syncStatus'
  | 'targets'
  | 'syncQueueEvents'
  | 'syncCheckObservations'
>;
const RUN_MS = 4_000;

/** One finite occurrence, with all read models updated before the next broadcast. */
export function advanceMockSync(state: State, now: number): boolean {
  let changed = false;
  for (const item of [...state.queue]) {
    if (item.kind !== 'sync_scan' && item.kind !== 'sync_apply') continue;
    if (item.state !== 'ready' && item.state !== 'scheduled' && item.state !== 'running') continue;
    const at = new Date(now).toISOString();
    const plan = item.kind === 'sync_apply' ? state.syncPlans.get(item.target_id!) : undefined;
    if (item.kind === 'sync_apply' && (!plan || plan.id !== item.source_id)) {
      finishItem(
        state,
        item,
        'cancelled',
        'The original plan is no longer active. No changes were applied.',
        at,
      );
      changed = true;
      continue;
    }
    if (plan && plan.state === 'computed') continue;
    if (plan && !['approved', 'applying'].includes(plan.state)) {
      finishItem(
        state,
        item,
        'cancelled',
        'This plan has finished. No further changes were applied.',
        at,
      );
      changed = true;
      continue;
    }
    if (item.state !== 'running') {
      if (Date.parse(item.eligible_at) > now) continue;
      item.state = 'running';
      item.started_at = at;
      item.updated_at = at;
      item.immediate = false;
      item.attempt++;
      item.revision++;
      item.summary =
        item.kind === 'sync_scan'
          ? 'Checking repository settings'
          : 'Processing shared configuration changes';
      if (plan) {
        plan.state = 'applying';
        plan.execution_stage = 'Sync in progress';
      }
      recordMockSyncEvent(state, item, 'started', item.summary, at);
      changed = true;
    } else if (now - Date.parse(item.started_at ?? item.updated_at) >= RUN_MS) {
      if (plan) finishPlan(state, item, plan, at);
      else finishItem(state, item, 'succeeded', finishMockSyncScan(state, item, at), at);
      changed = true;
    }
  }
  return changed;
}

function finishItem(
  state: State,
  item: QueueItem,
  result: 'succeeded' | 'failed' | 'cancelled',
  summary: string,
  at: string,
): void {
  item.state = result;
  item.summary = summary;
  item.finished_at = at;
  item.updated_at = at;
  item.revision++;
  item.actions = [];
  recordMockSyncEvent(state, item, result, summary, at);
}

function finishPlan(state: State, item: QueueItem, plan: SyncPlan, at: string): void {
  const targetId = item.target_id!;
  const status = state.syncStatus.get(targetId);
  const changedInput = plan.actions.some((action) => {
    const row = status?.repositories.find((row) => row.repository === action.repository);
    const cell = row?.cells[action.kind as SyncKind];
    return cell?.state === 'outdated' || cell?.state === 'unknown';
  });
  if (changedInput) {
    plan.state = 'stale';
    plan.execution_stage = 'Saved settings changed';
    finishItem(
      state,
      item,
      'cancelled',
      'Saved settings changed. This plan was not applied. Request a fresh check.',
      at,
    );
  } else {
    const login = state.targets.find((target) => target.value.id === targetId)?.value.account.login;
    plan.actions = plan.actions.map((action): SyncAction => {
      if (action.state !== 'pending') return action;
      const cell = status?.repositories.find((row) => row.repository === action.repository)?.cells[
        action.kind as SyncKind
      ];
      if (cell?.state === 'off')
        return { ...action, state: 'skipped', blocker: 'Sync is disabled for this category.' };
      if (action.blocker || action.error)
        return { ...action, state: 'failed', error: action.error ?? action.blocker };
      return {
        ...action,
        state: 'applied',
        // A fixture destination, shared by all file actions for this repository.
        ...(action.kind === 'files' && login
          ? { proposal_url: `https://github.com/${login}/${action.repository}/pull/42` }
          : {}),
      };
    });
    updateObservations(state, targetId, plan.actions, at);
    const failures = plan.actions.filter((action) => action.state === 'failed').length;
    const processed = plan.actions.filter((action) => action.state === 'applied').length;
    const skipped = plan.actions.filter((action) => action.state === 'skipped').length;
    plan.state = failures ? 'failed' : 'applied';
    plan.execution_stage = failures ? 'Sync needs attention' : 'Sync completed';
    item.progress_current = plan.actions.length;
    finishItem(
      state,
      item,
      failures ? 'failed' : 'succeeded',
      `${processed} changes processed, ${failures} failed, ${skipped} skipped. See Sync status for repository outcomes.`,
      at,
    );
  }
  plan.finished_at = at;
  state.syncHistory.set(targetId, [
    ...(state.syncHistory.get(targetId) ?? []),
    structuredClone(plan),
  ]);
  state.syncPlans.delete(targetId);
}

function updateObservations(
  state: State,
  targetId: string,
  actions: SyncAction[],
  at: string,
): void {
  const status = state.syncStatus.get(targetId);
  if (!status) return;
  let observed = false;
  for (const row of status.repositories) {
    for (const kind of SYNC_KINDS) {
      const group = actions.filter(
        (action) => action.repository === row.repository && action.kind === kind,
      );
      const failure = group.find((action) => action.state === 'failed');
      const applied = group.find((action) => action.state === 'applied');
      if (!failure && !applied) continue;
      row.cells[kind] = failure
        ? {
            state: 'check_failed',
            observed_outcome: 'failed',
            observed_at: at,
            reason: failure.error ?? 'The change could not be applied.',
          }
        : kind === 'files'
          ? {
              state: 'proposed',
              observed_outcome: 'proposed',
              observed_at: at,
              proposal_url: applied?.proposal_url,
              reason: 'A pull request was opened. Its changes still need to be merged.',
            }
          : { state: 'applied', observed_outcome: 'applied', observed_at: at };
      observed = true;
    }
  }
  if (observed) status.latest_observed_at = at;
}
