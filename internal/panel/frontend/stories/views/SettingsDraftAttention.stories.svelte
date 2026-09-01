<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import SettingsDraftAttention from '#lib/components/SettingsDraftAttention.svelte';

  const { Story } = defineMeta({
    title: 'Views/SettingsDraftAttention',
    component: SettingsDraftAttention,
    args: {
      kind: 'inactive',
      count: 3,
      problem: null,
      reviewHref: '/workspace/smykla-skalski/repositories',
      onDismiss: fn(),
    },
  });
</script>

<Story name="Inactive tab" />
<Story name="One setting" args={{ count: 1 }} />
<Story
  name="Storage unavailable"
  args={{
    kind: 'storage-problem',
    count: 1,
    problem: 'Browser storage is unavailable. Unsaved changes will not survive',
  }}
/>
