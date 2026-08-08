import { describe, expect, it, vi } from 'vitest';

import { copyText } from '../src/lib/clipboard';

describe('copyText', () => {
  it('reports the write it made', async () => {
    const writeText = vi.fn<(value: string) => Promise<void>>().mockResolvedValue(undefined);
    await expect(copyText('harness://pair?payload=abc', { writeText })).resolves.toBe('copied');
    expect(writeText).toHaveBeenCalledWith('harness://pair?payload=abc');
  });

  // Safari and Firefox refuse the write outright when the gesture is not
  // recognised, and a page that claimed success would leave someone pasting the
  // last thing they copied into a pairing prompt.
  it('reports a refused write rather than claiming success', async () => {
    const writeText = vi.fn<(value: string) => Promise<void>>().mockRejectedValue(new Error('no'));
    await expect(copyText('harness://pair', { writeText })).resolves.toBe('unavailable');
  });

  it('reports a browser with no clipboard access at all', async () => {
    await expect(copyText('harness://pair', undefined)).resolves.toBe('unavailable');
  });
});
