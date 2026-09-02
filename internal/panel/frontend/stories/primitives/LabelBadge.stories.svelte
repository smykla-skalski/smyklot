<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import LabelBadge from '#lib/components/LabelBadge.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/LabelBadge',
    component: LabelBadge,
  });

  const labels = [
    { name: 'dependencies', color: '0e8a16', description: 'Dependency updates' },
    { name: 'good first issue', color: '7057ff' },
    { name: 'bug', color: 'd73a4a' },
    { name: 'wontfix', color: 'ffffff' },
    { name: 'documentation', color: '0075ca' },
  ];
</script>

<!--
  A label as GitHub means it: the colour, then the name. `wontfix` is white -
  the ring is what keeps it from being a hole in the card.
-->
<Story name="Every colour a label can be">
  {#snippet template()}
    <div style="display: grid; gap: var(--space-3); justify-items: start;">
      {#each labels as label (label.name)}
        <LabelBadge {label} />
      {/each}
    </div>
  {/snippet}
</Story>

<!--
  The plan row's size, sitting in the line of 12px mono it belongs to. The badge
  must not set that line's height.
-->
<Story name="Compact, in a plan row">
  {#snippet template()}
    <div
      style="align-items: center; display: flex; font-family: var(--mono); font-size: var(--font-size-compact); gap: var(--space-3);"
    >
      <span style="color: var(--diff-add-ink);">+ add</span>
      <LabelBadge label={labels[0]} size="compact" />
      <span style="color: var(--text-muted);">Dependency updates, mostly Renovate's</span>
    </div>
  {/snippet}
</Story>

<!--
  A colour that is not six hex digits reaches this from somewhere that is not
  GitHub, and painting it would let a stored value decide a declaration. The
  muted disc says "a label, colour unknown" rather than drawing nothing.
-->
<Story name="A colour it will not paint">
  {#snippet template()}
    <div style="display: grid; gap: var(--space-3); justify-items: start;">
      <LabelBadge label={{ name: 'from somewhere else', color: 'rebeccapurple' }} />
      <LabelBadge label={{ name: 'empty', color: '' }} />
    </div>
  {/snippet}
</Story>
