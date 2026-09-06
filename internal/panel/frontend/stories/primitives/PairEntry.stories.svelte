<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import PairEntry from '#lib/components/PairEntry.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/PairEntry',
    component: PairEntry,
    args: {
      keyValue: 'ship',
      value: 'merge',
      keyLabel: 'Alias ship',
      valueLabel: 'Command for alias ship',
      removeLabel: 'Remove alias ship',
      options: [
        { value: 'approve', label: 'approve', description: 'Approves the pull request' },
        { value: 'merge', label: 'merge', description: 'Merges when checks pass' },
        { value: 'squash', label: 'squash', description: 'Squashes and merges' },
      ],
      validateKey: (key: string) =>
        /^[A-Za-z0-9_]{1,64}$/u.test(key) ? null : 'Use letters, numbers or underscores',
      onCommit: fn(),
      onRemove: fn(),
    },
  });
</script>

<Story name="Alias" />
<Story
  name="New alias"
  args={{
    keyValue: '',
    value: '',
    draft: true,
    keyLabel: 'New alias',
    valueLabel: 'Command for new alias',
    removeLabel: 'Discard alias',
  }}
/>
<Story name="Disabled" args={{ disabled: true }} />
