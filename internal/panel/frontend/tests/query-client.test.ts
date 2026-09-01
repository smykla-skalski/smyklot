import { QueryClient } from '@tanstack/svelte-query';
import { describe, expect, it, vi } from 'vitest';

import {
  createPanelQueryClient,
  invalidateRootWorkspaceSettings,
  STALE_WITHOUT_STREAM,
} from '../src/lib/query-client';

/**
 * How long an answer is trusted, which is a decision and not a number.
 *
 * With the stream up the panel is told about every change, so nothing a clock
 * could add is worth the request; with the stream down there is nothing else, so
 * the clock is all there is. Both halves are here because only one of them is
 * cheap to see: a browser can watch a return to a view cost nothing, but it
 * would have to wait out the staleness to watch the other, so a change that
 * quietly stopped the panel from EVER refetching would look like the improvement
 * it is the opposite of.
 */
function staleness(live: boolean): unknown {
  return decide(createPanelQueryClient({ live }));
}

/** The staleTime the client would use now, which is a function of the stream. */
function decide(client: QueryClient): unknown {
  const staleTime = client.getDefaultOptions().queries?.staleTime;
  if (typeof staleTime !== 'function') throw new Error('staleTime is not a function of the stream');

  // The query is never read: the decision is about the connection, not the key.
  return staleTime({} as never);
}

/** The polling interval the client would use now, from the same liveness box. */
function polling(client: QueryClient): unknown {
  const interval = client.getDefaultOptions().queries?.refetchInterval;
  if (typeof interval !== 'function') throw new Error('refetchInterval is not a function');

  return interval({} as never);
}

describe('how long the panel trusts an answer [Unit]', () => {
  it('trusts it until the stream says otherwise', () => {
    expect(staleness(true)).toBe(Number.POSITIVE_INFINITY);
    expect(polling(createPanelQueryClient({ live: true }))).toBe(false);
  });

  it('falls back to the clock when there is no stream', () => {
    expect(staleness(false)).toBe(STALE_WITHOUT_STREAM);
    expect(polling(createPanelQueryClient({ live: false }))).toBe(STALE_WITHOUT_STREAM);
    expect(STALE_WITHOUT_STREAM).toBeLessThan(Number.POSITIVE_INFINITY);
  });

  it('reads the box each time, so losing the stream takes effect at once', () => {
    /* One shared object rather than a value captured when the client was built:
       the client is created before the session that owns the stream, and the
       socket comes and goes for the whole life of both. */
    const stream = { live: true };
    const client = createPanelQueryClient(stream);

    expect(decide(client)).toBe(Number.POSITIVE_INFINITY);
    expect(polling(client)).toBe(false);
    stream.live = false;
    expect(decide(client)).toBe(STALE_WITHOUT_STREAM);
    expect(polling(client)).toBe(STALE_WITHOUT_STREAM);
  });
});

describe('panel query client [Unit]', () => {
  it('refreshes Root aggregates after workspace settings change', async () => {
    const queryClient = new QueryClient();
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries').mockResolvedValue();

    await invalidateRootWorkspaceSettings(queryClient, 'target-1');

    expect(invalidate.mock.calls.map(([filters]) => filters?.queryKey)).toEqual([
      ['root-workspaces'],
      ['root-overview'],
      ['repositories', 'target-1'],
      ['targets'],
    ]);
  });
});
