<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import RootPageHeader from '#lib/components/RootPageHeader.svelte';
  import StatusPill from '#lib/components/StatusPill.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/RootPageHeader',
    component: RootPageHeader,
    argTypes: { showScope: { control: 'boolean' } },
    args: {
      role: 'Super Root',
      title: 'Overview',
      subtitle: 'Live service, catalog, ownership, and security state',
      showScope: true,
    },
  });
</script>

<!--
  The Root console's preset over `PageHeader` - fifteen lines, all of them the kicker.
  The anatomy used to be copied: 72 of its ~81 CSS lines were identical to the panel
  header's, comments included.

  Scope is identity, so its pill sits on the kicker line rather than in the action
  slot, which holds only live status and real controls.
-->
<Story name="Default" />

<Story name="Without scope" args={{ showScope: false }} />

<Story name="With live status">
  {#snippet template({ children, ...args })}
    <RootPageHeader {...args}>
      <StatusPill dot live>WebSocket live</StatusPill>
    </RootPageHeader>
  {/snippet}
</Story>
