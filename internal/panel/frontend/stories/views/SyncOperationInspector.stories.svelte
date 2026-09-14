<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';
  import SyncOperationInspector from '#lib/components/SyncOperationInspector.svelte';
  import { TARGET } from '../support/fixtures.js';
  import { operation } from '../support/sync-requests.js';

  const { Story } = defineMeta({
    title: 'Views/SyncOperationInspector',
    component: SyncOperationInspector,
    args: {
      actorId: 'catalogue-actor',
      targetId: TARGET.id,
      selection: { action: 'check', requestKey: 'catalogue-check' },
      fetchOperation: async () => operation,
      resultHref: (id) => `?plan=${id}`,
      checkHref: (id) => `?check=${id}`,
      onClose: fn(),
    },
  });
</script>

<Story name="Accepted check" />
<Story
  name="Execution unavailable"
  args={{ fetchOperation: async () => ({ ...operation, execution: null }) }}
/>
<Story name="Loading" args={{ fetchOperation: () => new Promise(() => {}) }} />
<Story
  name="Unavailable"
  args={{
    fetchOperation: async () => {
      throw new Error('Unavailable');
    },
  }}
/>
