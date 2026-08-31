<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import AccountMenu from '#lib/components/AccountMenu.svelte';
  import type { PanelViewer } from '#lib/types.js';

  import { ACCOUNT } from '../support/fixtures';

  const VIEWER: PanelViewer = {
    account: ACCOUNT,
    system_role: 'root',
    status: 'active',
    target_count: 6,
  };

  const { Story } = defineMeta({
    title: 'Views/AccountMenu',
    component: AccountMenu,
    argTypes: {
      theme: { control: 'select', options: ['system', 'light', 'dark'] },
    },
    args: {
      viewer: VIEWER,
      theme: 'system',
      onSelectTheme: fn(),
      onSignOut: fn(),
      name: 'story-theme',
    },
  });
</script>

<!--
  Who is signed in, how the panel is themed, and the way out. The rail opens this
  menu, and so does the collapsed sidebar's foot - one menu, so the account a reader
  learns in one shell is the same account in the other.
-->
<Story name="Account">
  {#snippet template(args)}
    <div class="stage">
      <AccountMenu {...args}>
        {#snippet trigger(attributes)}
          <button {...attributes} class="btn" type="button">Account</button>
        {/snippet}
      </AccountMenu>
    </div>
  {/snippet}
</Story>

<style>
  .stage {
    block-size: 16rem;
    display: flex;
  }
</style>
