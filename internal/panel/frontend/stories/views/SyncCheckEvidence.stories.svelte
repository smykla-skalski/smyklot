<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SyncCheckEvidence from '#lib/components/SyncCheckEvidence.svelte';
  import { NOW, TARGET } from '../support/fixtures.js';
  import type { SyncCheckObservation } from '#lib/types.js';
  const rows: SyncCheckObservation[] = Array.from({ length: 12 }, (_, index) => ({
    repository_id: String(index),
    repository: `example/repository-${index + 1}`,
    kind: 'labels',
    outcome: index === 0 ? 'failed' : 'matched',
    cached: index === 11,
    observed_at: new Date(NOW - (index === 11 ? 86400000 : 0)).toISOString(),
    input_digest: 'saved-input',
    ...(index === 0 ? { reason: 'GitHub could not be reached.' } : {}),
  }));
  const { Story } = defineMeta({
    title: 'Views/SyncCheckEvidence',
    component: SyncCheckEvidence,
    args: {
      targetId: TARGET.id,
      checkId: 'mixed',
      api: {
        fetchSyncCheckObservations: async (_target, _check, request) => {
          const offset = Number(request.cursor ?? 0);
          return {
            items: rows.slice(offset, offset + request.limit),
            total: rows.length,
            next_cursor:
              offset + request.limit < rows.length ? String(offset + request.limit) : null,
          };
        },
      },
    },
  });
</script>

<Story name="Paged evidence" />
<Story
  name="Empty"
  args={{
    checkId: 'empty',
    api: { fetchSyncCheckObservations: async () => ({ items: [], total: 0, next_cursor: null }) },
  }}
/>
<Story
  name="Unavailable"
  args={{
    checkId: 'error',
    api: {
      fetchSyncCheckObservations: async () => {
        throw new Error('Unavailable');
      },
    },
  }}
/>
<Story
  name="Loading"
  args={{ checkId: 'loading', api: { fetchSyncCheckObservations: () => new Promise(() => {}) } }}
/>
