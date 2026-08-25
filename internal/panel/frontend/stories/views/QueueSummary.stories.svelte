<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import QueueSummary from '#lib/components/QueueSummary.svelte';
  import { GENERAL_QUEUE, NOW } from '../support/fixtures.js';

  const active = GENERAL_QUEUE.filter((item) =>
    ['scheduled', 'blocked', 'ready', 'running', 'retrying'].includes(item.state),
  );
  const approvals = GENERAL_QUEUE.filter((item) => item.state === 'awaiting_approval').length;
  const review = active.filter((item) => ['blocked', 'retrying'].includes(item.state)).length;

  const { Story } = defineMeta({
    title: 'Views/QueueSummary',
    component: QueueSummary,
    args: {
      items: active,
      total: active.length,
      approvals,
      review,
      now: NOW,
      queueHref: '#/root/queue',
      onOpenQueue: fn(),
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
<Story name="Empty" args={{ items: [], total: 0, approvals: 0, review: 0 }} />

<Story name="Only approvals" args={{ items: [], total: 0, approvals: 2, review: 0 }} />
