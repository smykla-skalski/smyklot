<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import PanelBoot from '#lib/components/PanelBoot.svelte';

  const { Story } = defineMeta({ title: 'Primitives/PanelBoot', component: PanelBoot });
</script>

<!--
  What the panel shows while it is still finding out who is asking.

  It appears after a 300ms delay rather than immediately: a boot that resolves in
  under a third of a second should show nothing at all, because a message that
  arrives and leaves faster than it can be read is a flicker, not information.
-->
<Story name="Booting" />
