/**
 * Query client setup for the panel.
 *
 * The client is created once and shared across the route tree via SvelteKit
 * context. The WebSocket stream handler calls `invalidateQueries` on it instead
 * of bumping version counters.
 */
import { QueryClient } from '@tanstack/svelte-query';

/**
 * Whether the panel is currently being told about changes as they happen.
 *
 * A plain mutable object rather than a signal: the query client is built before
 * the session that owns the stream, and this is read inside query options where
 * a stale closure would be worse than a shared box. `PanelSession` sets it from
 * the stream's own handshake and clears it when the socket goes.
 */
export interface StreamLiveness {
  live: boolean;
}

/**
 * How long data is trusted with no stream to correct it.
 *
 * The panel's own number from before there was a stream to rely on, kept for
 * exactly the case it was written for: a reader whose connection cannot hold a
 * WebSocket still gets a view that catches up on its own.
 */
export const STALE_WITHOUT_STREAM = 30_000;

/**
 * How long an answer is kept after the last view of it closes.
 *
 * Long enough to cover moving around the panel and coming back, which is the
 * whole point: it was five minutes, so a reader who looked at Settings, read
 * some history and returned to Repositories was served a spinner and a fresh
 * request for rows that had not changed.
 */
const GC_TIME = 30 * 60_000;

export function createPanelQueryClient(stream: StreamLiveness = { live: false }): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        /*
         * Fresh until something says otherwise, and while the stream is up
         * something does.
         *
         * A 30-second staleness was a guess at how long data stays true,
         * standing in for knowing. The panel does know: the service opens a
         * WebSocket and names every change on it, and `invalidateChange` turns
         * each one into an invalidation of exactly the keys it touched. With
         * both, every navigation after the first 30 seconds re-fetched rows that
         * the stream had already promised were unchanged - a request per view
         * per visit, answering a question that had been answered.
         *
         * So the guess is only used when there is nothing better. A function
         * rather than a fixed value because the stream comes and goes: drop the
         * connection and the clock is what is left, and the view starts catching
         * up on its own again without anything having to be re-created.
         */
        staleTime: () => (stream.live ? Number.POSITIVE_INFINITY : STALE_WITHOUT_STREAM),
        refetchInterval: () => (stream.live ? false : STALE_WITHOUT_STREAM),
        gcTime: GC_TIME,
        /* Coming back to the tab is a moment when data MIGHT have changed, and
           the stream says when it did - including everything missed while the
           socket was down, because a reconnect replies `ready` and that is a
           full resync. Refetching everything on focus as well is the same
           duplicated question as above, asked at the worst time: the reader is
           looking straight at the view when it happens. */
        refetchOnWindowFocus: () => !stream.live,
        retry: 1,
      },
      mutations: {
        retry: 0,
      },
    },
  });
}

/** Refreshes every Root view whose counts can change with installation settings. */
export async function invalidateRootInstallationSettings(
  queryClient: QueryClient,
  installationId: string,
): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ['root-installations'] }),
    queryClient.invalidateQueries({ queryKey: ['root-overview'] }),
    queryClient.invalidateQueries({ queryKey: ['repositories', installationId] }),
    queryClient.invalidateQueries({ queryKey: ['targets'] }),
  ]);
}
