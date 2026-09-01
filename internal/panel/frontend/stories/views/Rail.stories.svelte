<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import Rail from '#lib/components/Rail.svelte';
  import type { PanelTarget, PanelViewer } from '#lib/types.js';

  import { ACCOUNT, TARGET } from '../support/fixtures';

  /* The demo run mirrors the approved mock's rail: enough workspaces that the
     fold has something to hold at short heights. */
  const NAMES: readonly [string, string][] = [
    ['smykla-skalski', 'Smykla Skalski'],
    ['bartsmykla', 'bartsmykla'],
    ['acme-robotics', 'Acme Robotics'],
    ['northwind', 'Northwind'],
    ['oak-pine', 'Oak & Pine'],
    ['quarterdeck', 'Quarterdeck'],
    ['riverline', 'Riverline'],
    ['tailwheel', 'Tailwheel'],
    ['umbra', 'Umbra'],
    ['vantage-labs', 'Vantage Labs'],
    ['zephyr', 'Zephyr'],
  ];

  const TARGETS: PanelTarget[] = NAMES.map(([login, name], index) => ({
    ...TARGET,
    id: `ws-${index}`,
    account: { ...TARGET.account, id: `acct-${index}`, login, display_name: name },
  }));

  const VIEWER: PanelViewer = {
    account: ACCOUNT,
    system_role: 'root',
    status: 'active',
    target_count: TARGETS.length,
  };

  const { Story } = defineMeta({
    title: 'Views/Rail',
    component: Rail,
    argTypes: {
      rootMode: { control: 'boolean' },
      rootEnabled: { control: 'boolean' },
      inboxActive: { control: 'boolean' },
      unreadCount: { control: 'number' },
    },
    args: {
      viewer: VIEWER,
      targets: TARGETS,
      selectedId: 'ws-0',
      targetHref: (target: PanelTarget) => `#/workspace/${target.account.login}`,
      onSelectTarget: fn(),
      rootMode: false,
      rootEnabled: true,
      rootEntryHref: '#/root',
      onEnterRoot: fn(),
      inboxHref: '#/inbox',
      inboxActive: false,
      onSelectInbox: fn(),
      unreadCount: 2,
      theme: 'system',
      onSelectTheme: fn(),
      onSignOut: fn(),
    },
  });
</script>

<!--
  The account's consoles in one always-visible column: every workspace as an
  identity tile, the Root console, the inbox and the user. Switching is one
  press. More workspaces than the height takes fold behind a counted button
  whose menu carries a search - the active workspace never folds.
-->
<Story name="Workspace run">
  {#snippet template(args)}
    <div class="stage"><Rail {...args} /></div>
  {/snippet}
</Story>

<!-- A short shell: the run folds and the counted button takes the last slot. -->
<Story name="Folded">
  {#snippet template(args)}
    <div class="stage short"><Rail {...args} /></div>
  {/snippet}
</Story>

<!--
  Unsaved state reaches every scale of the rail: a visible workspace, one
  behind the fold, the fold summary, and Root. All keep the same tile geometry.
-->
<Story
  name="Unsaved settings"
  args={{ dirtyTargetIds: new Set(['ws-0', 'ws-8']), rootDirty: true }}
>
  {#snippet template(args)}
    <div class="stage short"><Rail {...args} /></div>
  {/snippet}
</Story>

<!-- The Root console pressed: the shield wears the thumb, workspaces stay one press away. -->
<Story name="Root console" args={{ rootMode: true }}>
  {#snippet template(args)}
    <div class="stage"><Rail {...args} /></div>
  {/snippet}
</Story>

<!-- Fewer workspaces than slots: no fold button at all rather than a "+0". -->
<Story name="Few workspaces" args={{ targets: TARGETS.slice(0, 3), unreadCount: 0 }}>
  {#snippet template(args)}
    <div class="stage"><Rail {...args} /></div>
  {/snippet}
</Story>

<style>
  /* The rail sizes itself to 100dvh; the stage clips it to a story-sized
     shell so the fold logic has a real height to answer. */
  .stage {
    block-size: 40rem;
    contain: size layout;
    display: flex;
    overflow: hidden;
  }

  .stage :global(.rail) {
    height: 100%;
  }

  .short {
    block-size: 26rem;
  }
</style>
