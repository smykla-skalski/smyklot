<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';
  import SyncRequests from '#lib/components/SyncRequests.svelte';
  import { NOW, TARGET } from '../support/fixtures.js';
  import { acceptance } from '../support/sync-requests.js';

  const { Story } = defineMeta({
    title: 'Views/SyncRequests',
    component: SyncRequests,
    args: {
      actorId: 'catalogue-actor',
      targetId: TARGET.id,
      nowMs: NOW,
      fetchRequests: async () => ({ items: [acceptance], next_cursor: null }),
      requestHref: (action, key) => `?request=${action}:${key}`,
      onOpenRequest: fn(),
    },
  });
</script>

<Story name="Accepted requests" />
<Story name="Empty" args={{ fetchRequests: async () => ({ items: [], next_cursor: null }) }} />
<Story name="Loading" args={{ fetchRequests: () => new Promise(() => {}) }} />
<Story
  name="Unavailable"
  args={{
    fetchRequests: async () => {
      throw new Error('Unavailable');
    },
  }}
/>
