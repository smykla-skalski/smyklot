<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import WorkspaceMenu from '#lib/components/WorkspaceMenu.svelte';
  import type { PanelTarget } from '#lib/types.js';

  import { TARGET } from '../support/fixtures';

  const NAMES: readonly [string, string][] = [
    ['smykla-skalski', 'Smykla Skalski'],
    ['bartsmykla', 'bartsmykla'],
    ['acme-robotics', 'Acme Robotics'],
    ['northwind', 'Northwind'],
    ['oak-pine', 'Oak & Pine'],
    ['zephyr', 'Zephyr'],
  ];

  const TARGETS: PanelTarget[] = NAMES.map(([login, name], index) => ({
    ...TARGET,
    id: `ws-${index}`,
    account: { ...TARGET.account, id: `acct-${index}`, login, display_name: name },
  }));

  const { Story } = defineMeta({
    title: 'Views/WorkspaceMenu',
    component: WorkspaceMenu,
    args: {
      targets: TARGETS,
      targetHref: (target: PanelTarget) => `#/workspace/${target.account.login}`,
      onSelectTarget: fn(),
      open: true,
      console: null,
    },
  });
</script>

<!--
  The workspaces a reader can open. One menu, opened from the rail's counted fold
  button and from the collapsed sidebar's own mark - where the rail is gone and this
  is the only way to change workspace.
-->
<Story name="Workspaces">
  {#snippet template(args)}
    <div class="stage">
      <WorkspaceMenu {...args}>
        {#snippet trigger(attributes)}
          <button {...attributes} class="btn" type="button">Switch workspace</button>
        {/snippet}
      </WorkspaceMenu>
    </div>
  {/snippet}
</Story>

<!-- With the console on it: what the collapsed shell offers, where the rail's shield
     is not on screen to be pressed. -->
<Story name="With the console" args={{ console: { href: '#/root', onEnter: fn() } }}>
  {#snippet template(args)}
    <div class="stage">
      <WorkspaceMenu {...args}>
        {#snippet trigger(attributes)}
          <button {...attributes} class="btn" type="button">Switch workspace</button>
        {/snippet}
      </WorkspaceMenu>
    </div>
  {/snippet}
</Story>

<!-- Unsaved configuration marks its workspace, wherever the reader left it. -->
<Story name="Unsaved elsewhere" args={{ dirtyTargetIds: new Set(['ws-2']) }}>
  {#snippet template(args)}
    <div class="stage">
      <WorkspaceMenu {...args}>
        {#snippet trigger(attributes)}
          <button {...attributes} class="btn" type="button">Switch workspace</button>
        {/snippet}
      </WorkspaceMenu>
    </div>
  {/snippet}
</Story>

<style>
  .stage {
    block-size: 26rem;
    display: flex;
  }
</style>
