<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SyncDocumentForm from '#lib/components/SyncDocumentForm.svelte';

  const { Story } = defineMeta({
    title: 'Views/SyncDocumentForm',
    component: SyncDocumentForm,
    argTypes: {
      enabled: { control: 'boolean' },
      unreadable: { control: 'boolean' },
      readOnly: { control: 'boolean' },
      saving: { control: 'boolean' },
      changed: { control: 'boolean' },
      disabled: { control: 'boolean' },
    },
    args: {
      heading: 'Labels',
      noun: 'labels',
      lead: 'The labels every repository in this installation should carry',
      enabled: true,
      unreadable: false,
      readOnly: false,
      saving: false,
      changed: false,
      disabled: false,
      onToggle: fn(),
      onSave: fn(),
    },
  });
</script>

<!--
  The shell every sync document shares: a switch, a lead, the document's own editor,
  and Save. A kind nobody asked for is not waiting on anything, so the editor only
  appears while the switch is on.
-->
<Story name="Enabled">
  {#snippet template({ children, ...args })}
    <SyncDocumentForm {...args}>
      <p>The document's own editor goes here.</p>
    </SyncDocumentForm>
  {/snippet}
</Story>

<Story name="Switched off" args={{ enabled: false }}>
  {#snippet template({ children, ...args })}
    <SyncDocumentForm {...args}>
      <p>The document's own editor goes here.</p>
    </SyncDocumentForm>
  {/snippet}
</Story>

<!-- Changed, so Save becomes pressable and the note under it explains the stakes. -->
<Story name="Unsaved changes" args={{ changed: true }}>
  {#snippet template({ children, ...args })}
    <SyncDocumentForm {...args}>
      <p>The document's own editor goes here.</p>
    </SyncDocumentForm>
  {/snippet}
</Story>

<Story name="Saving" args={{ changed: true, saving: true }}>
  {#snippet template({ children, ...args })}
    <SyncDocumentForm {...args}>
      <p>The document's own editor goes here.</p>
    </SyncDocumentForm>
  {/snippet}
</Story>

<!-- The file on GitHub cannot be parsed, so nothing here can be trusted to save. -->
<Story name="Unreadable" args={{ unreadable: true }}>
  {#snippet template({ children, ...args })}
    <SyncDocumentForm {...args}>
      <p>The document's own editor goes here.</p>
    </SyncDocumentForm>
  {/snippet}
</Story>

<Story name="Read only" args={{ readOnly: true }}>
  {#snippet template({ children, ...args })}
    <SyncDocumentForm {...args}>
      <p>The document's own editor goes here.</p>
    </SyncDocumentForm>
  {/snippet}
</Story>

<Story name="With a problem" args={{ problem: 'GitHub refused the write: 403' }}>
  {#snippet template({ children, ...args })}
    <SyncDocumentForm {...args}>
      <p>The document's own editor goes here.</p>
    </SyncDocumentForm>
  {/snippet}
</Story>
