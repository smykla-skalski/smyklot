<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import Modal from '#lib/components/Modal.svelte';
  import Chip from '#lib/components/Chip.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Modal',
    component: Modal,
    argTypes: {
      variant: { control: 'select', options: ['dialog', 'inspector', 'wide'] },
      open: { control: 'boolean' },
    },
    args: {
      id: 'story-modal',
      open: true,
      title: 'Remove access',
      variant: 'dialog',
      onClose: fn(),
    },
  });
</script>

<!--
  These stories are the proof that the `.app-shell` decorator does its job: the modal
  portals to `.app-shell` by selector, so if the wrapper were missing the overlay
  would render outside the palette - or not at all.
-->

<Story name="Dialog">
  {#snippet template({ children, ...args })}
    <Modal {...args}>
      <p>Marta Wisniewska loses access to every repository in this workspace.</p>
    </Modal>
  {/snippet}
</Story>

<Story name="With footer">
  {#snippet template({ children, ...args })}
    <Modal {...args}>
      <p>Marta Wisniewska loses access to every repository in this workspace.</p>
      {#snippet footer()}
        <button class="btn" type="button">Cancel</button>
        <button class="btn btn-stop" type="button">Remove access</button>
      {/snippet}
    </Modal>
  {/snippet}
</Story>

<Story
  name="Inspector"
  args={{ variant: 'inspector', title: 'smyklot', description: 'Repository settings' }}
>
  {#snippet template({ children, ...args })}
    <Modal {...args}>
      {#snippet headerExtra()}
        <Chip tone="signal" dot>Enabled</Chip>
      {/snippet}
      <p>The inspector variant is the one a row opens into.</p>
    </Modal>
  {/snippet}
</Story>

<Story name="Wide" args={{ variant: 'wide', title: 'Configuration' }}>
  {#snippet template({ children, ...args })}
    <Modal {...args}>
      <p>The wide variant carries a form that will not fold into a dialog.</p>
    </Modal>
  {/snippet}
</Story>
