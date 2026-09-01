<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import TopBar from '#lib/components/TopBar.svelte';
  import type { PanelTarget } from '#lib/types.js';

  import { TARGET } from '../support/fixtures';

  const NAMES: readonly [string, string][] = [
    ['smykla-skalski', 'Smykla Skalski'],
    ['bartsmykla', 'bartsmykla'],
    ['acme-robotics', 'Acme Robotics'],
  ];

  const TARGETS: PanelTarget[] = NAMES.map(([login, name], index) => ({
    ...TARGET,
    id: `ws-${index}`,
    account: { ...TARGET.account, id: `acct-${index}`, login, display_name: name },
  }));

  const { Story } = defineMeta({
    title: 'Views/TopBar',
    component: TopBar,
    args: {
      open: false,
      onToggle: fn(),
      title: 'Users',
      targets: TARGETS,
      selected: TARGETS[0],
      targetHref: (target: PanelTarget) => `#/workspace/${target.account.login}`,
      onSelectTarget: fn(),
      rootMode: false,
      console: null,
    },
  });
</script>

<!--
  The whole shell on a phone. It draws nothing above 48rem, so every story here pins
  the stage to a phone's width - at the catalogue's own width the bar is correctly
  absent and there would be nothing to look at.
-->
<Story name="On a workspace">
  {#snippet template(args)}
    <div class="stage">
      <TopBar {...args} />
    </div>
  {/snippet}
</Story>

<Story name="Pages open" args={{ open: true }}>
  {#snippet template(args)}
    <div class="stage">
      <TopBar {...args} />
    </div>
  {/snippet}
</Story>

<!-- In the console the bar says which console, not which workspace. -->
<Story
  name="On the console"
  args={{ rootMode: true, title: 'Workspaces', selected: null, console: null }}
>
  {#snippet template(args)}
    <div class="stage root-mode">
      <TopBar {...args} />
    </div>
  {/snippet}
</Story>

<!-- A page whose name is longer than the bar: it ellipsises rather than wrapping,
     because the page's own h1 below is never abbreviated. -->
<Story name="A long page name" args={{ title: 'Repository options' }}>
  {#snippet template(args)}
    <div class="stage narrow">
      <TopBar {...args} />
    </div>
  {/snippet}
</Story>

<style>
  .stage {
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-strip);
    inline-size: 375px;
    max-inline-size: 100%;
    overflow: hidden;
  }

  .narrow {
    inline-size: 320px;
  }

  /* The bar is drawn only below 48rem, and the catalogue is wider than that. The
     stage asks for it directly rather than shrinking the whole viewport. */
  .stage :global(.top-bar) {
    display: flex;
  }
</style>
