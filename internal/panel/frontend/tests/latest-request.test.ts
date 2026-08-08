import { describe, expect, it } from 'vitest';

import { LatestRequest } from '../src/lib/latest-request';

describe('LatestRequest', () => {
  it('only lets the newest overlapping read commit', () => {
    const requests = new LatestRequest();
    const older = requests.begin();
    const newer = requests.begin();

    expect(requests.isCurrent(older)).toBe(false);
    expect(requests.isCurrent(newer)).toBe(true);
  });

  it('invalidates reads when a write commits or the session ends', () => {
    const requests = new LatestRequest();
    const pending = requests.begin();

    requests.invalidate();

    expect(requests.isCurrent(pending)).toBe(false);
  });
});
