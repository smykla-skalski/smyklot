<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import ConfirmDialog from '#lib/components/ConfirmDialog.svelte';
  import Callout from '#lib/components/Callout.svelte';
  import Icon from '#lib/components/Icon.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/ConfirmDialog',
    component: ConfirmDialog,
    argTypes: { busy: { control: 'boolean' } },
    args: {
      id: 'story-confirm',
      open: true,
      title: 'Ban @marta-w?',
      description: 'They lose access to every workspace and the audit record remains',
      busy: false,
      onClose: fn(),
      onConfirm: fn(),
    },
  });
</script>

<!--
  Four dialogs asked a question and ended with the same eleven lines under four
  names. The bodies genuinely differ - one takes a reason for the audit record, one
  shows a consequence, one shows nothing - so the body stays the caller's, and only
  the footer is shared.

  The confirm's tone is what says whether the thing being done takes something away
  or gives it.
-->
<Story name="Destructive" args={{ confirmTone: 'stop' }}>
  {#snippet template({ children, ...args })}
    <ConfirmDialog {...args}>
      <Callout>
        {#snippet icon()}<Icon name="warning" size={20} />{/snippet}
        <p>Review this change carefully before confirming</p>
      </Callout>
    </ConfirmDialog>
  {/snippet}
</Story>

<Story
  name="Granting"
  args={{
    confirmTone: 'signal',
    title: 'Make @marta-w a Root?',
    description: 'They gain every Root permission on this deployment',
  }}
>
  {#snippet template({ children, ...args })}
    <ConfirmDialog {...args}>
      <Callout>
        {#snippet icon()}<Icon name="info" size={20} />{/snippet}
        <span>Review the account and effect before confirming</span>
      </Callout>
    </ConfirmDialog>
  {/snippet}
</Story>

<!-- While the request is in flight the confirm is disabled and reads differently. -->
<Story name="In flight" args={{ confirmTone: 'stop', busy: true }}>
  {#snippet template({ children, ...args })}
    <ConfirmDialog {...args}>
      <Callout>
        {#snippet icon()}<Icon name="warning" size={20} />{/snippet}
        <p>Review this change carefully before confirming</p>
      </Callout>
    </ConfirmDialog>
  {/snippet}
</Story>

<!-- A caller can rename both buttons; the revoke dialog does. -->
<Story
  name="Named actions"
  args={{
    confirmTone: 'stop',
    confirmLabel: 'Revoke invitation',
    busyLabel: 'Revoking…',
    title: 'Revoke invitation for @marta-w',
  }}
>
  {#snippet template({ children, ...args })}
    <ConfirmDialog {...args}>
      <Callout>
        {#snippet icon()}<Icon name="warning" size={20} />{/snippet}
        <p>The user can only join if you create and share a new invitation</p>
      </Callout>
    </ConfirmDialog>
  {/snippet}
</Story>
