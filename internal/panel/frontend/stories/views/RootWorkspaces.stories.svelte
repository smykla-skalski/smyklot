<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import RootWorkspaces from '#lib/components/RootWorkspaces.svelte';
  import Seeded from '../support/Seeded.svelte';
  import { stubApi } from '../support/api.js';
  import { WORKSPACES } from '../support/fixtures.js';

  const KEY = ['root-workspaces'] as const;

  const base = {
    route: { rootView: 'workspaces' as const },
    api: stubApi({ fetchRootWorkspaces: async () => WORKSPACES }),
    actorLogin: 'bart',
    listHref: '#/root/workspaces',
    hrefFor: (account: string, view: string) => `#/root/workspaces/${account}/${view}`,
    onList: fn(),
    onNavigate: fn(),
    historySection: 'audit' as const,
    onHistorySection: fn(),
  };

  const { Story } = defineMeta({
    title: 'Views/RootWorkspaces',
    component: RootWorkspaces,
    args: base,
  });
</script>

<!--
  Every workspace as one sentence. A standing appears only where somebody has to do
  something about it: an ageing snapshot is what the next sweep is for, so a stale
  owner list wears nothing and says when it last synced instead.
-->
<Story name="Catalogue">
  {#snippet template(args)}
    <Seeded seed={[[KEY, WORKSPACES]]}><RootWorkspaces {...args} /></Seeded>
  {/snippet}
</Story>

<!-- Nothing connected yet - the console's first-run state. -->
<Story name="Empty">
  {#snippet template(args)}
    <Seeded seed={[[KEY, []]]}>
      <RootWorkspaces {...args} api={stubApi({ fetchRootWorkspaces: async () => [] })} />
    </Seeded>
  {/snippet}
</Story>

<Story name="Loading">
  {#snippet template(args)}
    <Seeded>
      <RootWorkspaces
        {...args}
        api={stubApi({ fetchRootWorkspaces: () => new Promise(() => {}) })}
      />
    </Seeded>
  {/snippet}
</Story>
