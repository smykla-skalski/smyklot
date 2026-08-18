<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import QueueSummary from '#lib/components/QueueSummary.svelte';
  import { NOW, QUEUE } from '../support/fixtures.js';

  const { Story } = defineMeta({
    title: 'Views/QueueSummary',
    component: QueueSummary,
    args: {
      queue: QUEUE,
      now: NOW,
      queueHref: '#/root/queue',
      onOpenQueue: fn(),
      requestHref: (id: string) => `#/root/queue/request/${id}`,
      onOpenRequest: fn(),
    },
  });
</script>

<!--
  The queue as the Root dashboard shows it. `now` is a fixed instant, so the
  countdowns and the "4 minutes ago" read the same every time this is opened - the
  component's own clock ticks from whatever it is given.
-->
<Story name="Waiting and recent" />

<!-- Nothing armed: the summary has to say so rather than draw an empty table. -->
<Story name="Empty" args={{ queue: { active: [], deferred: [], recent: [] } }} />

<Story
  name="Only deferred"
  args={{ queue: { active: [], deferred: QUEUE.deferred, recent: [] } }}
/>
