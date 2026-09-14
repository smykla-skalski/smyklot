<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';
  import SyncCheckInspector from '#lib/components/SyncCheckInspector.svelte';
  import { TARGET } from '../support/fixtures.js';
  import { check } from '../support/sync-requests.js';

  const { Story } = defineMeta({
    title: 'Views/SyncCheckInspector',
    component: SyncCheckInspector,
    args: {
      actorId: 'catalogue-actor',
      targetId: TARGET.id,
      itemId: check.check_id,
      fetchCheck: async () => check,
      syncResultHref: (id) => `?plan=${id}`,
      checkHref: (id) => `?check=${id}`,
      onClose: fn(),
    },
  });
</script>

<Story name="Completed check" />
<Story
  name="Retained comparison"
  args={{ fetchCheck: async () => ({ ...check, execution: null }) }}
/>
<Story name="Loading" args={{ fetchCheck: () => new Promise(() => {}) }} />
<Story
  name="Unavailable"
  args={{
    fetchCheck: async () => {
      throw new Error('Unavailable');
    },
  }}
/>
