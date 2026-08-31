<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import PageHeader from '#lib/components/PageHeader.svelte';
  import RootPageHeader from '#lib/components/RootPageHeader.svelte';
  import Button from '#lib/components/Button.svelte';
  import Chip from '#lib/components/Chip.svelte';
  import Icon from '#lib/components/Icon.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/PageHeader',
    component: PageHeader,
    args: {
      id: 'story-heading',
      title: 'Access',
      description: 'Roles, invitations, and access decisions for this workspace',
    },
  });
</script>

<!--
  A grid rather than a flex row, so the action slot shares the TITLE's row and centres
  on it. Centring against the whole heading block hung the button below the title as
  soon as the description wrapped underneath.

  There were two of these components and 72 of their ~81 CSS lines were identical,
  comments included - and `HistoryPanel` rendered one or the other for the same page
  depending on which console it was in.
-->
<Story name="Default" />

<Story name="With actions">
  {#snippet template(args)}
    <PageHeader {...args}>
      {#snippet actions()}
        <Button tone="signal">
          {#snippet icon()}<Icon name="user-plus" size="sm" strokeWidth={2} />{/snippet}
          Add user
        </Button>
      {/snippet}
    </PageHeader>
  {/snippet}
</Story>

<!--
  The title's row is a control tall whether or not there is a control in it, so the
  gap to the description does not depend on what the slot happens to hold. Compare
  this with the story above: the description sits in the same place in both.
-->
<Story name="Actions do not move the description">
  {#snippet template(args)}
    <div>
      <PageHeader {...args} title="Without actions" />
      <PageHeader {...args} id="story-heading-2" title="With actions">
        {#snippet actions()}
          <Chip tone="accent">3 unread</Chip>
        {/snippet}
      </PageHeader>
    </div>
  {/snippet}
</Story>

<!--
  The Root console's preset. The kicker says whose authority the page is under, and
  scope is identity so its pill sits on that line rather than in the action slot.
  With a kicker the three rows become four and everything below it moves down one.
-->
<Story name="Root console">
  {#snippet template()}
    <RootPageHeader
      title="Overview"
      subtitle="Live service, catalog, ownership, and security state"
    />
  {/snippet}
</Story>
