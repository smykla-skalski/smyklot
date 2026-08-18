<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import InboxView from '#lib/components/InboxView.svelte';
  import Seeded from '../support/Seeded.svelte';
  import { NOTIFICATIONS } from '../support/fixtures.js';

  const KEY = ['notifications'] as const;
  const page = (over = {}) => ({ ...NOTIFICATIONS, ...over });
  const infinite = (data: unknown) => ({ pages: [data], pageParams: [undefined] });

  const base = {
    fetchPage: async () => NOTIFICATIONS,
    markRead: async (id: string) => NOTIFICATIONS.items.find((n) => n.id === id)!,
    onUnread: fn(),
  };

  const { Story } = defineMeta({ title: 'Views/InboxView', component: InboxView, args: base });
</script>

<!--
  Audited Root activity on workspaces the reader owns. Events group by elevation, so
  one Root session reads as one thing rather than as five separate writes.
-->
<Story name="With events">
  {#snippet template(args)}
    <Seeded seed={[[KEY, infinite(NOTIFICATIONS)]]}><InboxView {...args} /></Seeded>
  {/snippet}
</Story>

<!-- Everything read: the count says so rather than the page looking unvisited. -->
<Story name="All read">
  {#snippet template(args)}
    <Seeded
      seed={[
        [
          KEY,
          infinite(
            page({
              unread: 0,
              items: NOTIFICATIONS.items.map((n) => ({ ...n, read_at: n.created_at })),
            }),
          ),
        ],
      ]}
    >
      <InboxView {...args} />
    </Seeded>
  {/snippet}
</Story>

<!-- Nothing has ever happened here, which is the good state and has to read as one. -->
<Story name="Empty">
  {#snippet template(args)}
    <Seeded seed={[[KEY, infinite(page({ items: [], total: 0, unread: 0 }))]]}>
      <InboxView {...args} />
    </Seeded>
  {/snippet}
</Story>

<Story name="Loading">
  {#snippet template(args)}
    <Seeded><InboxView {...args} fetchPage={() => new Promise(() => {})} /></Seeded>
  {/snippet}
</Story>
