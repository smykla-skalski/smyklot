<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';
  import DeliveryRetry from '#lib/components/DeliveryRetry.svelte';
  const { Story } = defineMeta({
    title: 'Views/DeliveryRetry',
    component: DeliveryRetry,
    args: {
      targetId: 'workspace',
      deliveryId: 'story-available',
      root: false,
      onAccepted: fn(),
      onPendingChange: fn(),
      api: {
        previewDeliveryRecovery: async () => ({
          available: true,
          reason: 'available',
          current_run_id: 1,
          revision: 1,
          effect: 'Process the original comment using current configuration and permissions.',
        }),
        retryDelivery: async () => ({ run_id: 2, queue_id: 'delivery:2', repeated: false }),
      },
    },
  });
</script>

<Story name="Review available retry" />
<Story
  name="Configuration needs attention"
  args={{
    deliveryId: 'story-unavailable',
    api: {
      previewDeliveryRecovery: async () => ({
        available: false,
        reason: 'configuration_invalid',
        current_run_id: 1,
        revision: 1,
      }),
      retryDelivery: async () => {
        throw new Error('Unavailable recovery must not be submitted');
      },
    },
  }}
/>
<Story
  name="Result cannot be confirmed"
  args={{
    deliveryId: 'story-uncertain',
    api: {
      previewDeliveryRecovery: async () => ({
        available: true,
        reason: 'available',
        current_run_id: 1,
        revision: 1,
        effect: 'Recheck current CI state.',
      }),
      retryDelivery: async () => {
        throw new Error('Connection lost');
      },
    },
  }}
/>
