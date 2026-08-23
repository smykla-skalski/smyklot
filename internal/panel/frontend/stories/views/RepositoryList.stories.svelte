<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import RepositoryList from '#lib/components/RepositoryList.svelte';
  import type { SyncOverride } from '#lib/types.js';
  import { NOW, REPOSITORIES, REPOSITORY_DETAIL, TARGET } from '../support/fixtures.js';

  /*
   * No seeded cache, deliberately.
   *
   * This view's query key has eight parts, five of them read from saved preferences,
   * and a seed has to reproduce every one or it lands under a different key and the
   * table draws its header over nothing - silently, with nothing in the console. It
   * needs no seed: `fetchPage` is an ordinary prop rather than a call through `api`, so
   * the query resolves against the story's own function. Seed a cache only where the
   * data arrives through `api`, which a story's stub refuses on purpose.
   *
   * What each row DOES need is a selected workspace, because its link is built from
   * one. `PanelShell` selects a target for every story - see the note there.
   */
  const base = {
    targetId: TARGET.id,
    defaultEnabled: false,
    fetchPage: () =>
      Promise.resolve({ items: REPOSITORIES, next_cursor: null, total: REPOSITORIES.length }),
    onLoad: () => Promise.resolve(REPOSITORY_DETAIL),
    onResetConfigMigration: () => Promise.resolve(REPOSITORY_DETAIL),
    onChanged: fn(),
    /* `onLoadSyncOverride !== null` is what decides whether the opened repository is
       offered a Sync pane at all - the list asks that question rather than being told
       - so leaving these out is not "no data", it is a list whose fourth pane does not
       exist. An installation's own page passes them; the Root view of somebody else's
       does not, and `Without sync` below is that surface. */
    onLoadSyncOverride: () => Promise.resolve(SYNC_OVERRIDE),
  };

  const SYNC_OVERRIDE: SyncOverride = {
    kind: 'files',
    enabled: null,
    document: {
      merges: [
        { path: 'renovate.json', strategy: 'deep-merge', overrides: { timezone: 'Europe/Warsaw' } },
      ],
    },
    revision: 1,
    updated_by: 'bart',
    updated_at: new Date(NOW - 2 * 60 * 60_000).toISOString(),
    unreadable: false,
  };

  const { Story } = defineMeta({
    title: 'Views/RepositoryList',
    component: RepositoryList,
    args: base,
  });
</script>

<!--
  Every repository the installation reaches, and whether the bot acts on it. The name
  is the only flexible column and it has a floor: a bare `1fr` resolves to min-content,
  and a hundred-character repository took 812px of a 977px row and pushed the other
  four columns off the end of it.
-->
<Story name="Repositories">
  {#snippet template(args)}<RepositoryList {...args} />{/snippet}
</Story>

<!-- Nothing installed yet, or nothing matching. The row says which. -->
<Story name="Empty">
  {#snippet template(args)}
    <RepositoryList
      {...args}
      fetchPage={() => Promise.resolve({ items: [], next_cursor: null, total: 0 })}
    />
  {/snippet}
</Story>

<!--
  A reader who may not change anything. The enablement switch in each row goes; the
  column stays, because whether the bot acts here is still worth reading.
-->
<Story name="Read only">
  {#snippet template(args)}<RepositoryList {...args} readOnly />{/snippet}
</Story>

<!--
  The Root view of somebody else's installation. Sync is configured on the
  installation's own page and has no Root address, so the pane is not offered and the
  switch inside an opened repository shows three rather than four - `RepositoryList`
  reads that off these two being absent rather than being told separately.
-->
<Story name="Without sync" args={{ onLoadSyncOverride: null }} />
