<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import QueueView from '#lib/components/QueueView.svelte';
  import Seeded from '../support/Seeded.svelte';
  import { stubApi } from '../support/api.js';
  import { OVERVIEW } from '../support/fixtures.js';

  /* One query, and the whole overview behind it: the queue is a live split of
     `pending_ci` into what is waiting and what has finished, so both sections read the
     same cache entry rather than fetching separately. */
  const KEY = ['root-overview'] as const;

  const empty = {
    ...OVERVIEW,
    pending_ci: { active: [], deferred: [], recent: [] },
  };

  const base = {
    api: stubApi({ fetchRootOverview: async () => OVERVIEW }),
    rootRole: 'Super Root',
    section: 'waiting' as const,
    onSection: fn(),
    onOpenRequest: fn(),
  };

  const { Story } = defineMeta({
    title: 'Views/QueueView',
    component: QueueView,
    args: base,
  });
</script>

<!--
  What is armed and waiting on checks. This is the one table that kept its own
  markup: its rows animate in, out and between positions, and a transition is a
  compile-time directive that cannot be handed to `DataTable` - see the note there.
-->
<Story name="Waiting">
  {#snippet template(args)}
    <Seeded seed={[[KEY, OVERVIEW]]}><QueueView {...args} /></Seeded>
  {/snippet}
</Story>

<!-- What has merged, been cancelled, or been replaced. Different columns, same table. -->
<Story name="Recent">
  {#snippet template(args)}
    <Seeded seed={[[KEY, OVERVIEW]]}><QueueView {...args} section="recent" /></Seeded>
  {/snippet}
</Story>

<!--
  Nothing waiting. A real answer rather than a wait, which is why it says so in words
  instead of leaving a placeholder standing.
-->
<Story name="Empty">
  {#snippet template(args)}
    <Seeded seed={[[KEY, empty]]}>
      <QueueView {...args} api={stubApi({ fetchRootOverview: async () => empty })} />
    </Seeded>
  {/snippet}
</Story>
