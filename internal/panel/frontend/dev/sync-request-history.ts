import { VIEWER, type MockState } from './fixtures.js';
import type { SyncRequestAcceptance, SyncRequestHistory } from '../src/lib/types.js';

type Cursor = {
  version: 1;
  actor_id: string;
  target_id: string;
  accepted_at: string;
  action: 'check' | 'dispatch';
  request_key: string;
};
type Reply =
  | { status: 200; body: SyncRequestHistory }
  | { status: 400 | 404; body: { error: { code: string; message: string } } };

/** Discover accepted work without reading or changing its current queue entry. */
export function mockSyncRequests(
  state: Pick<MockState, 'targets' | 'syncCheckReceipts' | 'syncDispatchReceipts'>,
  targetId: string,
  params: URLSearchParams,
  actorId = VIEWER.id,
): Reply {
  const target = state.targets.find((entry) => entry.value.id === targetId);
  if (!target || !['owner', 'admin', 'editor', 'viewer'].includes(target.value.effective_role))
    return { status: 404, body: { error: { code: 'not_found', message: 'workspace not found' } } };
  const invalid = (): Reply => ({
    status: 400,
    body: {
      error: { code: 'invalid_request_history_query', message: 'Invalid request history query' },
    },
  });
  for (const [key] of params)
    if (!['limit', 'cursor'].includes(key) || params.getAll(key).length !== 1) return invalid();
  const rawLimit = params.get('limit') || '20';
  const limit = Number(rawLimit);
  if (!/^\+?\d+$/u.test(rawLimit) || !Number.isInteger(limit) || limit < 1 || limit > 100)
    return invalid();
  let after: Cursor | null;
  try {
    after = decodeCursor(params.get('cursor'), actorId, targetId);
  } catch {
    return invalid();
  }
  const accepted: SyncRequestAcceptance[] = [];
  for (const [key, receipt] of state.syncCheckReceipts) {
    const [actor, requestKey] = JSON.parse(key) as [string, string];
    if (actor === actorId && receipt.targetId === targetId)
      accepted.push({
        action: 'check',
        request_key: requestKey,
        check_id: receipt.checkId,
        reason: receipt.reason,
        accepted_at: receipt.acceptedAt,
      });
  }
  for (const [key, receipt] of state.syncDispatchReceipts) {
    const [actor, requestKey] = JSON.parse(key) as [string, string];
    if (actor === actorId && receipt.targetId === targetId)
      accepted.push({
        action: 'dispatch',
        request_key: requestKey,
        queue_id: receipt.queueId,
        plan_id: receipt.planId,
        expected_revision: receipt.expectedRevision,
        reason: receipt.reason,
        accepted_at: receipt.acceptedAt,
      });
  }
  const ordered = accepted
    .sort(compare)
    .filter((item) => after === null || compare(item, after) > 0);
  const items = ordered.slice(0, limit);
  const last = items.at(-1);
  const cursor: Cursor | null =
    ordered.length > limit && last
      ? {
          version: 1,
          actor_id: actorId,
          target_id: targetId,
          accepted_at: last.accepted_at,
          action: last.action,
          request_key: last.request_key,
        }
      : null;
  return {
    status: 200,
    body: {
      items,
      next_cursor: cursor ? Buffer.from(JSON.stringify(cursor)).toString('base64url') : null,
    },
  };
}

function compare(
  a: Pick<Cursor, 'accepted_at' | 'action' | 'request_key'>,
  b: Pick<Cursor, 'accepted_at' | 'action' | 'request_key'>,
): number {
  return (
    Date.parse(b.accepted_at) - Date.parse(a.accepted_at) ||
    (a.action === b.action ? 0 : a.action === 'dispatch' ? -1 : 1) ||
    Buffer.compare(Buffer.from(b.request_key), Buffer.from(a.request_key))
  );
}

function decodeCursor(raw: string | null, actorId: string, targetId: string): Cursor | null {
  if (!raw) return null;
  if (
    raw.length > 4096 ||
    !/^[A-Za-z0-9_-]+$/u.test(raw) ||
    Buffer.from(raw, 'base64url').toString('base64url') !== raw
  )
    throw new Error('Invalid cursor');
  const cursor = JSON.parse(Buffer.from(raw, 'base64url').toString()) as Partial<Cursor> | null;
  if (
    !cursor ||
    Object.keys(cursor).some(
      (key) =>
        !['version', 'actor_id', 'target_id', 'accepted_at', 'action', 'request_key'].includes(key),
    ) ||
    cursor.version !== 1 ||
    cursor.actor_id !== actorId ||
    cursor.target_id !== targetId ||
    typeof cursor.accepted_at !== 'string' ||
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/u.test(
      cursor.accepted_at,
    ) ||
    !Number.isFinite(Date.parse(cursor.accepted_at)) ||
    cursor.accepted_at.startsWith('0001-01-01T00:00:00') ||
    !['check', 'dispatch'].includes(cursor.action ?? '') ||
    typeof cursor.request_key !== 'string' ||
    !cursor.request_key ||
    cursor.request_key.trim() !== cursor.request_key ||
    Buffer.byteLength(cursor.request_key) > 200
  )
    throw new Error('Invalid cursor');
  return cursor as Cursor;
}
