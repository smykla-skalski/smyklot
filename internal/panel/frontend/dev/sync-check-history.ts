import type { MockState } from './fixtures';
import type { Page, SyncCheckObservation } from '../src/lib/types';

type Reply = {
  status: number;
  body: Page<SyncCheckObservation> | { error: { code: string; message: string } };
};

export function mockSyncCheckPage(
  state: Pick<MockState, 'syncCheckResults' | 'syncCheckObservations'>,
  targetId: string,
  checkId: string,
  params: URLSearchParams,
): Reply {
  const error = (status: number, message: string): Reply => ({
    status,
    body: { error: { code: status === 404 ? 'not_found' : 'invalid_check_query', message } },
  });
  const retained = state.syncCheckResults.get(checkId);
  if (
    retained?.targetId !== targetId ||
    !retained.result.outcome ||
    !state.syncCheckObservations.has(checkId)
  )
    return error(404, 'Check evidence not found');
  for (const [key] of params)
    if (!['limit', 'cursor'].includes(key) || params.getAll(key).length !== 1)
      return error(400, 'Unsupported check evidence query');
  const rawLimit = params.get('limit') || '20';
  const limit = Number(rawLimit);
  if (!/^\+?\d+$/u.test(rawLimit) || !Number.isInteger(limit) || limit <= 0 || limit > 100)
    return error(400, 'Invalid check evidence page size');
  let after = 0;
  const cursor = params.get('cursor');
  if (cursor) {
    try {
      if (cursor.length > 1024 || !/^[A-Za-z0-9_-]+$/u.test(cursor))
        throw new Error('Invalid cursor');
      const decoded = JSON.parse(Buffer.from(cursor, 'base64url').toString()) as {
        check_id: string;
        after: number;
      } | null;
      if (
        !decoded ||
        decoded.check_id !== checkId ||
        !Number.isSafeInteger(decoded.after) ||
        decoded.after <= 0
      )
        throw new Error('Invalid cursor');
      after = decoded.after;
    } catch {
      return error(400, 'Invalid check evidence cursor');
    }
  }
  const observations = state.syncCheckObservations.get(checkId)!;
  return {
    status: 200,
    body: {
      items: structuredClone(observations.slice(after, after + limit)),
      total: observations.length,
      next_cursor:
        after + limit < observations.length
          ? Buffer.from(JSON.stringify({ check_id: checkId, after: after + limit })).toString(
              'base64url',
            )
          : null,
    },
  };
}
