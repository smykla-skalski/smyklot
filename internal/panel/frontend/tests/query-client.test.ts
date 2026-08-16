import { QueryClient } from '@tanstack/svelte-query';
import { describe, expect, it, vi } from 'vitest';

import { invalidateRootInstallationSettings } from '../src/lib/query-client';

describe('panel query client [Unit]', () => {
  it('refreshes Root aggregates after installation settings change', async () => {
    const queryClient = new QueryClient();
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries').mockResolvedValue();

    await invalidateRootInstallationSettings(queryClient, 'target-1');

    expect(invalidate.mock.calls.map(([filters]) => filters?.queryKey)).toEqual([
      ['root-installations'],
      ['root-overview'],
      ['repositories', 'target-1'],
      ['targets'],
    ]);
  });
});
