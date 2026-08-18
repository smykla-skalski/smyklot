<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import AccountTrigger from '#lib/components/AccountTrigger.svelte';
  import { ACCOUNT } from '../support/fixtures.js';

  const { Story } = defineMeta({
    title: 'Primitives/AccountTrigger',
    component: AccountTrigger,
    args: { account: ACCOUNT, handle: '@bart' },
  });
</script>

<!--
  The row at the foot of the sidebar that opens the account menu.

  What it looks like once the rail collapses is NOT decided here: `.collapsed` is on
  the sidebar, which is `IdentityBar`'s element, so the rules that shrink this to an
  avatar and swap in the tooltip stay there. This component owns the row; the rail owns
  what the rail does to it. See `Views/IdentityBar` for the collapsed shape.
-->
<Story name="Account row">
  {#snippet template(args)}<AccountTrigger {...args} />{/snippet}
</Story>

<!-- A long name, which truncates sideways and only sideways: the trim moves the box
     and not the glyphs, so `overflow: hidden` would cut the tail off the handle's
     "@" along the bottom edge. -->
<Story name="Long name">
  {#snippet template(args)}
    <AccountTrigger
      {...args}
      account={{ ...ACCOUNT, display_name: 'Bartosz Smykla-Skalski of the Long Name' }}
      handle="@a-considerably-longer-handle"
    />
  {/snippet}
</Story>
