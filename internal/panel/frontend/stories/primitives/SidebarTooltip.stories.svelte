<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import SidebarTooltip from '#lib/components/SidebarTooltip.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/SidebarTooltip',
    component: SidebarTooltip,
    args: { text: 'Switch workspace' },
  });
</script>

<!--
  The label beside a sidebar control once the rail is collapsed. Three triggers carried
  their own copy of this span - the collapse button, the workspace switcher and the
  account row - and only the word differed.

  It renders hidden here, which is correct rather than broken: what shows it is a hover
  on an ancestor `IdentityBar` owns, because that is the component that knows the rail
  is collapsed and which row the pointer is on. Look at it in `Views/IdentityBar` with
  the collapsed story.

  Not `AppTooltip`: that one portals and positions against the viewport, which on a
  56px rail gets the edge wrong the moment the sidebar animates. This is a span inside
  its trigger, positioned against the rail.
-->
<Story name="Hidden until hovered">
  {#snippet template(args)}<SidebarTooltip {...args} />{/snippet}
</Story>
