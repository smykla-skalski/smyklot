<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import RootOverview from '#lib/components/RootOverview.svelte';
  import Seeded from '../support/Seeded.svelte';
  import { stubApi } from '../support/api.js';
  import { OVERVIEW } from '../support/fixtures.js';

  const KEY = ['root-overview'] as const;

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
    requestHref: (id: string) => `#/root/queue/request/${id}`,
    onOpenRequest: fn(),
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
    <Seeded seed={[[KEY, OVERVIEW]]}><RootOverview {...args} /></Seeded>
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
      ]}
    >
      <RootOverview {...args} />
    </Seeded>
  {/snippet}
</Story>

<!-- Nothing has answered yet. -->
<Story name="Loading">
  {#snippet template(args)}
    <Seeded>
      <RootOverview {...args} api={stubApi({ fetchRootOverview: () => new Promise(() => {}) })} />
    </Seeded>
  {/snippet}
</Story>
