import type {
  SyncCheckResponse,
  SyncOperationResponse,
  SyncRequestAcceptance,
} from '#lib/types.js';
import { NOW, TARGET } from './fixtures.js';

const observedAt = new Date(NOW).toISOString();
export const acceptance: SyncRequestAcceptance = {
  action: 'check',
  request_key: 'catalogue-check',
  check_id: 'catalogue-check',
  reason: 'Verify repository labels after saving the shared configuration',
  accepted_at: new Date(NOW - 120_000).toISOString(),
};
export const check: SyncCheckResponse = {
  check_id: 'catalogue-check',
  target_id: TARGET.id,
  observed_at: observedAt,
  result: {
    outcome: {
      completed_at: observedAt,
      disposition: 'checked',
      summary: 'Compared repository labels with the saved configuration',
      counts: { matched: 2 },
      cached: 0,
      missing_permissions: [],
    },
  },
  execution: {
    state: 'succeeded',
    summary: 'Repository comparison finished',
    progress_current: 2,
    progress_total: 2,
    attempt: 1,
    started_at: acceptance.accepted_at,
    finished_at: observedAt,
  },
  check: {
    action: 'check',
    target_id: TARGET.id,
    available: true,
    reason: 'available',
    effect: 'request_repository_check',
  },
};
export const operation: SyncOperationResponse = {
  target_id: TARGET.id,
  acceptance,
  observation_started_at: observedAt,
  observed_at: observedAt,
  comparison: check.result,
  plan: null,
  execution: check.execution,
  check: check.check,
  dispatch: null,
};
