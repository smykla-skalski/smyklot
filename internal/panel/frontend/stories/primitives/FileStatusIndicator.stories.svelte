<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import FileStatusIndicator from '#lib/components/FileStatusIndicator.svelte';
  import type { RepositoryFileStatus } from '#lib/types.js';

  const STATUSES: RepositoryFileStatus[] = ['valid', 'missing', 'invalid', 'bypassed'];

  const { Story } = defineMeta({
    title: 'Primitives/FileStatusIndicator',
    component: FileStatusIndicator,
    argTypes: {
      status: { control: 'select', options: STATUSES },
      showLabel: { control: 'boolean' },
    },
    args: { id: 'story-file', status: 'valid', showLabel: false },
  });
</script>

<Story name="Playground" />

<!--
  Every state the column can draw. No state here is told apart by hue alone - each
  carries its own glyph - because three of these pairs cannot survive one dichromacy
  or another.
-->
<Story name="Every state">
  {#snippet template()}
    <div class="row">
      {#each STATUSES as status (status)}
        <FileStatusIndicator id={`story-${status}`} {status} showLabel />
      {/each}
    </div>
  {/snippet}
</Story>

<style>
  .row {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-4);
  }
</style>
