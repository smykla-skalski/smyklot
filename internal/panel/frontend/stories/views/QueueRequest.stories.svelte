<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import QueueRequest from '#lib/components/QueueRequest.svelte';
  import Seeded from '../support/Seeded.svelte';
  import { stubApi } from '../support/api.js';
  import { NOW, QUEUE } from '../support/fixtures.js';
  import type { PendingCIDetail, PendingCIEvent } from '#lib/types.js';

  const at = (offsetMs: number) => new Date(NOW + offsetMs).toISOString();
  const REQUEST = QUEUE.active[0]!;
  const KEY = ['root-pending-ci', REQUEST.id] as const;

  /* `id` matters: `QueueRequest` keys its event list on it, so leaving it out made
     every key `undefined` and Svelte threw `each_key_duplicate` - in a production
     build as well as in dev. The `as PendingCIEvent` cast is what kept tsc quiet
     about it. */
  let nextEventId = 0;
  const event = (kind: string, offsetMs: number, over: Partial<PendingCIEvent> = {}) =>
    ({
      id: (nextEventId += 1),
      kind,
      trigger: 'poll',
      occurred_at: at(offsetMs),
      ...over,
    }) as PendingCIEvent;

  const DETAIL: PendingCIDetail = {
    request: REQUEST,
    events: [
      event('armed', -8 * 60_000, { trigger: 'command' }),
      event('wake_received', -6 * 60_000, { trigger: 'webhook' }),
      event('reconciliation_started', -6 * 60_000),
      event('checks_observed', -90_000, { state: 'passing' }),
    ],
  };

  const base = {
    api: stubApi({ fetchRootPendingCI: async () => DETAIL }),
    requestId: REQUEST.id,
    queueHref: '#/root/queue',
    onBack: fn(),
  };

  const { Story } = defineMeta({
    title: 'Views/QueueRequest',
    component: QueueRequest,
    args: base,
  });
</script>

<!--
  One armed request, and everything that has happened to it. The countdown ticks every
  second rather than every thirty - which is the right rate for a clock running out
  and the wrong one for "4 minutes ago".

  One mark per kind of event, and the shape carries it: the rail is a column of
  identical circles otherwise.
-->
<Story name="Armed and passing">
  {#snippet template(args)}
    <Seeded seed={[[KEY, DETAIL]]}><QueueRequest {...args} /></Seeded>
  {/snippet}
</Story>

<!-- Merged: the outcome replaces the countdown, and the timeline ends. -->
<Story name="Merged">
  {#snippet template(args)}
    <Seeded
      seed={[
        [
          KEY,
          {
            request: { ...REQUEST, lifecycle: 'merged', finished_at: at(-30_000) },
            events: [...DETAIL.events, event('merge_started', -45_000), event('finished', -30_000)],
          },
        ],
      ]}
    >
      <QueueRequest {...args} />
    </Seeded>
  {/snippet}
</Story>

<!-- Superseded: the pull request moved on underneath it, which is not a failure. -->
<Story name="Superseded">
  {#snippet template(args)}
    <Seeded
      seed={[
        [
          KEY,
          {
            request: {
              ...REQUEST,
              lifecycle: 'superseded',
              finished_at: at(-60_000),
              reason: 'A newer commit was pushed',
            },
            events: [...DETAIL.events, event('superseded', -60_000)],
          },
        ],
      ]}
    >
      <QueueRequest {...args} />
    </Seeded>
  {/snippet}
</Story>

<!-- The record could not be read. -->
<Story name="Unavailable">
  {#snippet template(args)}
    <Seeded>
      <QueueRequest
        {...args}
        api={stubApi({
          fetchRootPendingCI: async () => {
            throw new Error('The service did not answer in time');
          },
        })}
      />
    </Seeded>
  {/snippet}
</Story>
