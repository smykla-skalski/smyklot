<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import RootPageHeader from '#lib/components/RootPageHeader.svelte';
  import StatusPill from '#lib/components/StatusPill.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/RootPageHeader',
    component: RootPageHeader,
    args: {
      title: 'Overview',
      subtitle: 'Live service, catalog, ownership, and security state',
    },
  });
</script>

<!--
  The Root console's preset over `PageHeader`, and all of it is the eyebrow: the
  console's own name, under the shield the sidebar wears.

  It used to open every page with "Root mode · Super Root" and a pill reading
  "Application scope" - three ways of saying where you are, on a purple shell whose
  sidebar is titled Operations and whose every page is the console's.
-->
<Story name="Default" />

<!-- Without a sentence: a page whose title is the whole of what it is. -->
<Story name="Title alone" args={{ subtitle: undefined }} />

<Story name="With live status">
  {#snippet template({ children, ...args })}
    <RootPageHeader {...args}>
      <StatusPill dot live>WebSocket live</StatusPill>
    </RootPageHeader>
  {/snippet}
</Story>
