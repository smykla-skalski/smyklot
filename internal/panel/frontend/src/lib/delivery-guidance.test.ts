import { describe, expect, it } from 'vitest';
import { deliveryNextStep } from './queue-words';
import type { DeliveryOperation } from './types';

const failed = { state: 'failed' as const, eligible_at: '2026-09-01T12:00:00Z' };

describe('delivery execution guidance', () => {
  it('uses the successor schedule instead of the old failed run', () => {
    const operation: DeliveryOperation = {
      retained: true,
      revision: 3,
      current: {
        id: 2,
        status: 'running',
        payload_available: true,
        queue: { id: 'delivery:2', state: 'retrying', eligible_at: '2026-09-13T14:00:00Z' },
      },
    };
    expect(deliveryNextStep(failed, operation)).toEqual({
      message: 'An automatic retry is scheduled.',
      eligibleAt: '2026-09-13T14:00:00Z',
    });
  });
  it('does not describe an active run as stopped when its queue was pruned', () => {
    expect(
      deliveryNextStep(failed, {
        retained: true,
        revision: 3,
        current: { id: 2, status: 'running', payload_available: true, queue: null },
      }),
    ).toEqual({ message: 'A delivery run is active, but its queue record is no longer retained.' });
  });
  it('shows successful recovery independently of the original failure', () => {
    expect(
      deliveryNextStep(failed, {
        retained: true,
        revision: 3,
        current: { id: 2, status: 'succeeded', payload_available: true, queue: null },
      }).message,
    ).toContain('latest delivery run succeeded');
  });
  it('does not infer an execution when its cursor is absent', () => {
    expect(
      deliveryNextStep(failed, { retained: true, revision: 0, current: null }).message,
    ).toContain('No current execution is retained');
  });
  it('explains pruned delivery history without inventing a retry', () => {
    expect(
      deliveryNextStep(failed, { retained: false, revision: 0, current: null }).message,
    ).toContain('delivery history is no longer retained');
  });
});
