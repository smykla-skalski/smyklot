<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';
  import QueueInspector from '#lib/components/QueueInspector.svelte';
  import { PanelApiError } from '#lib/api.js';
  import type { QueueDetail } from '#lib/types.js';

  const { Story } = defineMeta({
    title: 'Views/QueueInspector',
    component: QueueInspector,
    args: {
      itemId: 'expired-record',
      fetchItem: async (): Promise<QueueDetail> => {
        throw new PanelApiError(404, 'not_found', 'Queue record not found');
      },
      onClose: fn(),
    },
  });
</script>

<Story name="Record no longer available" />
<Story
  name="Access changed"
  args={{
    itemId: 'restricted-record',
    fetchItem: async () => {
      throw new PanelApiError(403, 'forbidden', 'Access denied');
    },
  }}
/>
<Story
  name="Temporary outage"
  args={{
    itemId: 'unavailable-record',
    fetchItem: async () => {
      throw new PanelApiError(503, 'unavailable', 'Try later');
    },
  }}
/>
<Story
  name="Loading"
  args={{ itemId: 'loading-record', fetchItem: () => new Promise<QueueDetail>(() => {}) }}
/>
