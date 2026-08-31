<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import RootOverview from '#lib/components/RootOverview.svelte';
  import { queueListKey, ROOT_OVERVIEW_ACTIVE_QUEUE } from '#lib/queue-cache.js';
  import type { QueueItem, QueuePage } from '#lib/types.js';
  import Seeded from '../support/Seeded.svelte';
  import { stubApi } from '../support/api.js';
  import { GENERAL_QUEUE, OVERVIEW } from '../support/fixtures.js';

  const KEY = ['root-overview'] as const;
  const ACTIVE_KEY = queueListKey(undefined, ROOT_OVERVIEW_ACTIVE_QUEUE);
  const active = GENERAL_QUEUE.filter((item) =>
    ['scheduled', 'blocked', 'ready', 'running', 'retrying'].includes(item.state),
  );

  function queuePage(items: QueueItem[]): QueuePage {
    return {
      items: items.slice(0, 3),
      next_offset: 0,
      total: items.length,
      state_counts: GENERAL_QUEUE.reduce<NonNullable<QueuePage['state_counts']>>((counts, item) => {
        counts[item.state] = (counts[item.state] ?? 0) + 1;
        return counts;
      }, {}),
      facets: {
        targets: [],
        repositories: [],
        profiles: [],
        states: [],
        workloads: [],
        priorities: [],
      },
    };
  }

  const QUEUE_SEED: Array<[readonly unknown[], QueuePage]> = [[ACTIVE_KEY, queuePage(active)]];

  const { Story } = defineMeta({
    title: 'Views/RootOverview',
    component: RootOverview,
    args: { api: stubApi() },
  });
</script>

<!--
  Where the console opens. The verdict counts asks rather than reporting health,
  and the service and its database are one quiet line at the foot - the page an
  operator keeps open answers "is anything waiting for me" before it answers
  anything about the plumbing.
-->
<Story name="Quiet">
  {#snippet template(args)}
    <Seeded
      seed={[
        [KEY, { ...OVERVIEW, active_elevations: 0, unread_security_events: 0 }],
        ...QUEUE_SEED,
      ]}
    >
      <RootOverview {...args} />
    </Seeded>
  {/snippet}
</Story>

<!-- Elevations and unread notifications, worded as asks rather than as tiles. -->
<Story name="Needs attention">
  {#snippet template(args)}
    <Seeded seed={[[KEY, OVERVIEW], ...QUEUE_SEED]}><RootOverview {...args} /></Seeded>
  {/snippet}
</Story>

<!-- The database answering slowly: the foot goes amber and says why. -->
<Story name="Database degraded">
  {#snippet template(args)}
    <Seeded
      seed={[
        [
          KEY,
          {
            ...OVERVIEW,
            service: {
              ...OVERVIEW.service,
              storage: 'degraded',
              database: {
                ...OVERVIEW.service.database,
                state: 'degraded',
                latency_ms: 184,
                connections: {
                  ...OVERVIEW.service.database.connections,
                  wait_count: 37,
                  wait_ms: 900,
                },
              },
            },
          },
        ],
        ...QUEUE_SEED,
      ]}
    >
      <RootOverview {...args} />
    </Seeded>
  {/snippet}
</Story>

<!-- Nothing has answered yet. -->
<Story name="Loading">
  {#snippet template(args)}
    <Seeded seed={QUEUE_SEED}>
      <RootOverview {...args} api={stubApi({ fetchRootOverview: () => new Promise(() => {}) })} />
    </Seeded>
  {/snippet}
</Story>
