<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import BackLink from '#lib/components/BackLink.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/BackLink',
    component: BackLink,
    args: { href: '#/queue', label: 'Queue', onNavigate: fn() },
  });
</script>

<!--
  A real address, so middle-click, Cmd-click and Copy link all work; `onNavigate`
  only takes over the plain click.
-->
<Story name="Default" />
