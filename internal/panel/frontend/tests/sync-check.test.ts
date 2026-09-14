import { describe, expect, it } from 'vitest';
import { checkOutcome, checkOutcomeTitle } from '../src/lib/sync-check';
import type { SyncCheckOutcome } from '../src/lib/types';

const result: SyncCheckOutcome = {
  completed_at: '2026-09-13T12:00:00Z',
  disposition: 'checked',
  summary: 'Compared saved settings',
  counts: { matched: 2 },
  cached: 1,
  missing_permissions: null,
};
describe('retained check interpretation', () => {
  it('keeps fresh counts separate from cached observations', () => {
    expect(checkOutcome(result)).toEqual(result);
    expect(checkOutcomeTitle(result)).toBe('Check finished');
    expect(checkOutcomeTitle({ ...result, counts: { different: 1 } })).toBe(
      'Check found differences',
    );
    expect(checkOutcomeTitle({ ...result, counts: { matched: 50, failed: 1 } })).toBe(
      'Check finished with gaps',
    );
    expect(checkOutcomeTitle({ ...result, missing_permissions: ['files'] })).toBe(
      'Check finished with gaps',
    );
  });
  it.each([
    { ...result, blocking_plan_id: 'earlier' },
    { ...result, disposition: 'deferred', blocking_plan_id: ' ' },
    { ...result, disposition: 'deferred', blocking_plan_id: 42 },
    undefined,
    {},
    { ...result, counts: { matched: -1 } },
    { ...result, counts: { matched: 0.5 } },
    { ...result, counts: { invented: 0 } },
    { ...result, cached: -1 },
    { ...result, missing_permissions: ['unknown'] },
    { ...result, completed_at: 'bad' },
    { ...result, disposition: 'unknown' },
  ])('does not invent a successful outcome for malformed details %j', (value) => {
    expect(checkOutcome(value)).toBeNull();
  });
  it('retains the exact blocker of a deferred check', () => {
    const deferred = { ...result, disposition: 'deferred', blocking_plan_id: 'earlier:plan' };
    expect(checkOutcome(deferred)).toEqual(deferred);
  });
  it.each(['disabled', 'unpermitted', 'deferred'] as const)(
    'does not describe %s as a successful comparison',
    (disposition) => {
      expect(checkOutcomeTitle({ ...result, disposition })).not.toContain('finished');
    },
  );
});
