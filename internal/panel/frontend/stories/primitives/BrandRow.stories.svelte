<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import BrandRow from '#lib/components/BrandRow.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/BrandRow',
    component: BrandRow,
    args: { part: 'PANEL', onToggleCollapsed: fn() },
  });
</script>

<!--
  The top of the sidebar. Two triggers that look like a pair and are not: the collapse
  button belongs to the rail and is only there on a desktop, the menu button belongs to
  the top bar and is only there on a phone. They share this row and nothing else.
-->
<Story name="Panel">
  {#snippet template(args)}<BrandRow {...args} />{/snippet}
</Story>

<!-- The Root console says so under the mark rather than anywhere else. -->
<Story name="Root console">
  {#snippet template(args)}<BrandRow {...args} part="ROOT MODE" />{/snippet}
</Story>

<!-- Collapsed: the button now offers to expand, and says so in its label as well as
     its icon, since at this width the label is all a screen reader has. -->
<Story name="Collapsed">
  {#snippet template(args)}<BrandRow {...args} collapsed />{/snippet}
</Story>

<!-- Nowhere to navigate - the invitation page, or signed out. The mark stands alone. -->
<Story name="No navigation">
  {#snippet template(args)}<BrandRow {...args} showNavigation={false} />{/snippet}
</Story>
