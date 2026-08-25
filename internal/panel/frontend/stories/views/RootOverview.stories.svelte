<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

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

  const base = {
    api: stubApi(),
    rootRole: 'Super Root',
    installationsHref: '#/root/installations',
    elevationsHref: '#/root/access',
    failuresHref: '#/root/history',
    onOpenInstallations: fn(),
    onOpenElevations: fn(),
    onOpenFailures: fn(),
    inboxHref: '#/inbox',
    onOpenInbox: fn(),
    queueHref: '#/root/queue',
    onOpenQueue: fn(),
  };

  const { Story } = defineMeta({
    title: 'Views/RootOverview',
    component: RootOverview,
    args: base,
  });
</script>

<!--
  The Root console's dashboard. Pool pressure is deliberately not part of the
  database's health: `in_use === max` is a busy instant, not a fault, and a light that
  flapped on one would teach an operator to ignore it. `wait_count` is the durable
  evidence instead - it only grows.
-->
<Story name="Healthy">
  {#snippet template(args)}
    <Seeded seed={[[KEY, OVERVIEW], ...QUEUE_SEED]}><RootOverview {...args} /></Seeded>
  {/snippet}
</Story>

<!-- The database answering slowly, and callers having waited for a connection. -->
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

<!-- Ownership that needs an operator: a Members approval is blocking synchronisation. -->
<Story name="Ownership blocked">
  {#snippet template(args)}
    <Seeded
      seed={[
        [KEY, { ...OVERVIEW, ownership: { fresh: 1, stale: 0, permission_pending: 2, error: 1 } }],
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
