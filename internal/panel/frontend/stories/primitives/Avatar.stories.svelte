<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Avatar from '#lib/components/Avatar.svelte';
  import type { PanelAccount } from '#lib/types.js';

  const ACCOUNT: PanelAccount = {
    id: '1001',
    provider: 'github:https://api.github.com',
    subject_id: '1001',
    login: 'ada',
    display_name: 'Ada Lovelace',
    avatar_url: null,
  };

  const { Story } = defineMeta({
    title: 'Primitives/Avatar',
    component: Avatar,
    argTypes: {
      size: { control: { type: 'range', min: 16, max: 64, step: 4 } },
      shape: { control: 'inline-radio', options: ['person', 'workspace'] },
    },
    args: { account: ACCOUNT, size: 32, shape: 'person' },
  });
</script>

<Story name="Playground" />

<!--
  The outline is GitHub's own distinction - people round, organisations square - and
  it earns its place in the top bar on a phone, where the workspace switcher and the
  account menu lose their labels and stand next to each other as two identical discs.
-->
<Story name="Person and workspace">
  {#snippet template()}
    <div class="row">
      <Avatar account={ACCOUNT} size={32} />
      <Avatar
        account={{ ...ACCOUNT, display_name: 'Smykla Skalski' }}
        size={32}
        shape="workspace"
      />
    </div>
  {/snippet}
</Story>

<Story name="Sizes">
  {#snippet template()}
    <div class="row">
      {#each [16, 20, 24, 32, 48, 64] as size (size)}
        <Avatar account={ACCOUNT} {size} />
      {/each}
    </div>
  {/snippet}
</Story>

<style>
  .row {
    align-items: center;
    display: flex;
    gap: var(--space-3);
  }
</style>
