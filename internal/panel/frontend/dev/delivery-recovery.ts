import type { DeliveryOperation, QueueItem } from '../src/lib/types';
import type {
  DeliveryRecoveryPreview,
  DeliveryRecoveryRequest,
  DeliveryRecoveryResult,
} from '../src/lib/delivery-recovery';

interface Operation {
  original: QueueItem;
  current: QueueItem;
  runId: number;
  revision: number;
  receipts: Map<string, { input: DeliveryRecoveryRequest; result: DeliveryRecoveryResult }>;
}
const operations = new WeakMap<QueueItem[], Map<string, Operation>>();
function operation(queue: QueueItem[], item: QueueItem): Operation {
  let entries = operations.get(queue);
  if (!entries) {
    entries = new Map();
    operations.set(queue, entries);
  }
  const existing =
    entries.get(item.id) ?? [...entries.values()].find((entry) => entry.current.id === item.id);
  if (existing) return existing;
  const created = {
    original: item,
    current: item,
    runId: queue.indexOf(item) + 1,
    revision: 1,
    receipts: new Map(),
  };
  entries.set(item.id, created);
  return created;
}
function source(queue: QueueItem[], target: string, sourceId: string): QueueItem | undefined {
  return queue.find(
    (item) =>
      item.target_id === target && item.source_kind === 'delivery' && item.source_id === sourceId,
  );
}
export function mockRecoveryPreview(
  queue: QueueItem[],
  target: string,
  sourceId: string,
): DeliveryRecoveryPreview {
  const item = source(queue, target, sourceId);
  if (!item) return { available: false, reason: 'history_unavailable', revision: 0 };
  const op = operation(queue, item);
  const reason =
    op.current.state === 'failed'
      ? 'available'
      : op.current.state === 'succeeded'
        ? 'already_succeeded'
        : 'already_running';
  return {
    available: reason === 'available',
    reason,
    revision: op.revision,
    current_run_id: op.runId,
    effect: item.title.includes('issue_comment')
      ? 'Process the original comment using current configuration and permissions.'
      : 'Process the saved event using current configuration and permissions.',
  };
}
export function mockRecoveryOperation(
  queue: QueueItem[],
  item: QueueItem,
): DeliveryOperation | undefined {
  if (item.kind !== 'webhook_delivery' || item.source_kind !== 'delivery') return undefined;
  const op = operation(queue, item);
  return {
    retained: true,
    revision: op.revision,
    current: {
      id: op.runId,
      status:
        op.current.state === 'failed'
          ? 'failed'
          : op.current.state === 'succeeded'
            ? 'succeeded'
            : 'running',
      payload_available: true,
      queue: { id: op.current.id, state: op.current.state, eligible_at: op.current.eligible_at },
    },
  };
}
export function mockRecoverDelivery(
  queue: QueueItem[],
  target: string,
  sourceId: string,
  input: DeliveryRecoveryRequest,
): { status: number; body: DeliveryRecoveryResult | object } {
  const item = source(queue, target, sourceId);
  if (!item)
    return {
      status: 404,
      body: {
        error: { code: 'history_unavailable', message: 'The saved event is no longer available.' },
      },
    };
  const op = operation(queue, item);
  const prior = op.receipts.get(input.request_key);
  if (
    prior &&
    prior.input.expected_run_id === input.expected_run_id &&
    prior.input.expected_revision === input.expected_revision
  ) {
    return { status: 200, body: { ...prior.result, repeated: true } };
  }
  if (
    prior ||
    !input.request_key ||
    op.current.state !== 'failed' ||
    input.expected_run_id !== op.runId ||
    input.expected_revision !== op.revision
  ) {
    return {
      status: 409,
      body: {
        error: { code: 'recovery_unavailable', message: 'Review the latest recovery state.' },
        current: mockRecoveryPreview(queue, target, sourceId),
      },
    };
  }
  op.runId = 100000 + queue.length;
  const now = new Date().toISOString();
  op.current = {
    ...op.current,
    id: `delivery:${op.runId}`,
    source_id: String(op.runId),
    state: 'scheduled',
    attempt: 0,
    revision: 1,
    summary: 'Retry requested',
    created_at: now,
    updated_at: now,
    eligible_at: now,
    not_before: now,
    blocked_reason: undefined,
  };
  op.revision++;
  queue.push(op.current);
  const result = { run_id: op.runId, queue_id: op.current.id, repeated: false };
  op.receipts.set(input.request_key, { input: { ...input }, result });
  return { status: 202, body: result };
}
