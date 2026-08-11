import { describe, expect, it } from 'vitest';

import { inkDescends } from '../src/lib/ink-align';

describe('inkDescends', () => {
  it('is false for a label whose ink stops at the baseline', () => {
    for (const label of ['Fresh', 'Stale', '1 failure', 'Enablement', 'Permanent', 'Read']) {
      expect(inkDescends(label), label).toBe(false);
    }
  });

  it('is true for a label that puts ink below the baseline', () => {
    for (const label of ['Healthy', 'Bypassed', 'Sync now', 'Retryable', 'Approval needed']) {
      expect(inkDescends(label), label).toBe(true);
    }
  });

  it('reads a whole row, so a descender anywhere in it counts', () => {
    expect(inkDescends('Success replies')).toBe(true);
    expect(inkDescends('Mention commands')).toBe(false);
  });
});
