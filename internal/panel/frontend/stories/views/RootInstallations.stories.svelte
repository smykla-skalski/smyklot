<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import RootInstallations from '#lib/components/RootInstallations.svelte';
  import Seeded from '../support/Seeded.svelte';
  import { stubApi } from '../support/api.js';
  import { INSTALLATIONS } from '../support/fixtures.js';

  const KEY = ['root-installations'] as const;

  const base = {
    route: { rootView: 'installations' as const },
    api: stubApi({ fetchRootInstallations: async () => INSTALLATIONS }),
    rootRole: 'Super Root',
    actorLogin: 'bart',
    listHref: '#/root/installations',
    hrefFor: (account: string, view: string) => `#/root/installations/${account}/${view}`,
    onList: fn(),
    onNavigate: fn(),
    historySection: 'audit' as const,
    onHistorySection: fn(),
  };

  const { Story } = defineMeta({
    title: 'Views/RootInstallations',
    component: RootInstallations,
    args: base,
  });
</script>

<!--
  Live ownership and delivery health for every installation. Stale is drift rather
  than danger, so it takes the neutral tone and stays out of the problem count; only
  the warning and danger states are things an operator has to act on.
-->
<Story name="Catalogue">
  {#snippet template(args)}
    <Seeded seed={[[KEY, INSTALLATIONS]]}><RootInstallations {...args} /></Seeded>
  {/snippet}
</Story>

<!-- Nothing connected yet - the console's first-run state. -->
<Story name="Empty">
  {#snippet template(args)}
    <Seeded seed={[[KEY, []]]}>
      <RootInstallations {...args} api={stubApi({ fetchRootInstallations: async () => [] })} />
    </Seeded>
  {/snippet}
</Story>

<Story name="Loading">
  {#snippet template(args)}
    <Seeded><RootInstallations {...args} /></Seeded>
  {/snippet}
</Story>
