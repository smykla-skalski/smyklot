<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import StructuredMergeRules from '#lib/components/StructuredMergeRules.svelte';
  import type { SyncFileMerge } from '#lib/types.js';

  const { Story } = defineMeta({
    title: 'Views/StructuredMergeRules',
    component: StructuredMergeRules,
    args: {
      idPrefix: 'story-merge-rules',
      merge: { path: 'renovate.json', overrides: { enabled: true } },
      onChange: fn(),
    },
  });

  const LIST_MERGE = {
    path: 'renovate.json',
    overrides: { extends: ['config:recommended'], ignorePaths: ['vendor/**'] },
    arrays: [
      { path: '$.extends', strategy: 'append' },
      { path: '$.ignorePaths', strategy: 'prepend' },
    ],
    deduplicate: true,
  } satisfies SyncFileMerge;
</script>

<Story name="Default behavior" />
<Story name="Lists and repeated entries" args={{ merge: LIST_MERGE }} />
<Story name="Read only" args={{ merge: LIST_MERGE, readOnly: true }} />
<Story
  name="Unfinished list rule"
  args={{
    merge: {
      path: 'renovate.json',
      overrides: { extends: ['config:recommended'] },
      arrays: [{ path: '', strategy: 'append' }],
    },
  }}
/>
