<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SettingsSaveComposer from '#lib/components/SettingsSaveComposer.svelte';

  const base = {
    count: 3,
    onSave: fn(),
    onDiscard: fn(),
    onResolveConflict: fn(),
    onDismiss: fn(),
  };

  const { Story } = defineMeta({
    title: 'Views/SettingsSaveComposer',
    component: SettingsSaveComposer,
    args: base,
  });
</script>

<Story name="Changed settings" />
<Story name="Conflict" args={{ conflict: true, problem: 'Settings changed in another session.' }} />
<Story
  name="Validation failure"
  args={{
    problem: 'A label name is required.',
    problemHref: '#/sync/labels',
    problemLabel: 'Labels',
  }}
/>
<Story
  name="Saved"
  args={{ count: 0, notice: 'Reconciliation creates a plan only when repositories need changes.' }}
/>
