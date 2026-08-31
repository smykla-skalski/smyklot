<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import WorkspaceOverview from '#lib/components/WorkspaceOverview.svelte';
  import { TARGET } from '../support/fixtures.js';

  /*
   * One prop and six queries, all answered through `session.api`, which
   * `PanelShell` backs with the mock's own fixtures. Nothing to seed here: the
   * page is a picture of six other pages, and stubbing any one of them would
   * describe a workspace that cannot exist.
   */
  const { Story } = defineMeta({
    title: 'Views/WorkspaceOverview',
    component: WorkspaceOverview,
    args: { target: TARGET },
  });
</script>

<!--
  Where a workspace opens. The verdict leads and counts what a person has to act
  on - never a health badge - and the three cards under it are the queue, the
  audit and the pulse, each shown small enough to be a glance rather than a
  visit.
-->
<Story name="Needs review" />
