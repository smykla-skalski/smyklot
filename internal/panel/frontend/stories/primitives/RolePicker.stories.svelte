<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import RolePicker from '#lib/components/RolePicker.svelte';

  const ROLES = [
    { value: 'viewer', label: 'Viewer', icon: 'viewer' as const },
    { value: 'editor', label: 'Editor', icon: 'editor' as const },
    { value: 'admin', label: 'Admin', icon: 'admin' as const },
  ];

  const { Story } = defineMeta({
    title: 'Primitives/RolePicker',
    component: RolePicker,
    argTypes: { disabled: { control: 'boolean' } },
    args: {
      label: 'Role for @ada',
      value: 'editor',
      options: ROLES,
      disabled: false,
      onSelect: fn(),
    },
  });
</script>

<Story name="Playground" />

<!-- A row whose role cannot be changed still shows it, and says why by being quiet. -->
<Story name="Disabled" args={{ disabled: true }} />
