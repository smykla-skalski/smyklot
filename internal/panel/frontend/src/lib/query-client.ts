/**
 * Query client setup for the panel.
 *
 * The client is created once and shared across the route tree via SvelteKit
 * context. The WebSocket stream handler calls `invalidateQueries` on it
 * instead of bumping version counters.
 */
import { QueryClient } from '@tanstack/svelte-query';

export function createPanelQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        // The panel is an admin tool that needs fresh data. A short stale time
        // means a tab left open and returned to refetches without a visible
        // loading state (stale-while-revalidate).
        staleTime: 30_000,
        gcTime: 5 * 60_000,
        refetchOnWindowFocus: true,
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
