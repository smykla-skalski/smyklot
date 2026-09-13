<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';
  import SyncHistory from '#lib/components/SyncHistory.svelte';
  import { NOW, TARGET } from '../support/fixtures.js';
  import { seed } from '../../dev/fixtures';
  import { mockSyncHistory, mockSyncHistoryPage } from '../../dev/sync-history';
  const state = seed(undefined, NOW);
  const { Story } = defineMeta({
    title: 'Views/SyncHistory',
    component: SyncHistory,
    args: {
      targetId: TARGET.id,
      nowMs: NOW,
      fetchHistory: async (id, request) =>
        mockSyncHistoryPage(mockSyncHistory(state, id), request.limit, request.cursor ?? null),
      resultHref: (id) => `#/sync/history/${id}`,
      onOpenResult: fn(),
      onStatus: fn(),
    },
  });
</script>

<Story name="History" />
<Story
  name="Empty"
  args={{
    targetId: 'empty',
    fetchHistory: async () => ({ items: [], total: 0, next_cursor: null }),
  }}
/>
<Story
  name="Unavailable"
  args={{
    targetId: 'unavailable',
    fetchHistory: async () => {
      throw new Error('The server could not be reached');
    },
  }}
/>
