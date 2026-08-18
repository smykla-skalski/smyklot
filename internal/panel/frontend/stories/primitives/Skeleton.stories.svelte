<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Skeleton from '#lib/components/Skeleton.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Skeleton',
    component: Skeleton,
    argTypes: {
      rows: { control: { type: 'range', min: 1, max: 12, step: 1 } },
      bars: { control: 'boolean' },
    },
    args: { rows: 6, bars: true, label: 'Loading repositories' },
  });
</script>

<Story name="Playground">
  {#snippet template(args)}
    <Skeleton {...args} />
  {/snippet}
</Story>

<!--
  What five of the six tables drew, before the component existed. The bars hint at
  the columns underneath, which is why their widths are a per-table measurement
  rather than a constant.
-->
<Story name="Table rows">
  {#snippet template()}
    <Skeleton label="Loading repositories" />
  {/snippet}
</Story>

<!--
  Two tables draw plain rows with no bars at all. That was drift rather than a
  decision, but it is a decision now: it is a prop, and both of them still look
  exactly as they did.
-->
<Story name="Without bars" args={{ bars: false }}>
  {#snippet template(args)}
    <Skeleton {...args} />
  {/snippet}
</Story>

<!--
  The four geometries the tables actually use, side by side - the comparison that
  made the drift visible in the first place. Each column is one caller's custom
  properties, unchanged.
-->
<Story name="Every table's geometry">
  {#snippet template()}
    <div class="grid">
      <section>
        <h3>History</h3>
        <Skeleton
          rows={4}
          --skeleton-row-height="3rem"
          --skeleton-bar-a-width="min(12rem, 26%)"
          --skeleton-bar-b-left="46%"
          --skeleton-bar-b-width="min(16rem, 32%)"
        />
      </section>
      <section>
        <h3>Repositories</h3>
        <Skeleton rows={4} --skeleton-bar-a-width="min(14rem, 32%)" --skeleton-bar-b-left="48%" />
      </section>
      <section>
        <h3>Users</h3>
        <Skeleton rows={4} --skeleton-bar-top="1.15rem" --skeleton-bar-a-width="min(13rem, 28%)" />
      </section>
      <section>
        <h3>Root access</h3>
        <Skeleton rows={4} bars={false} />
      </section>
    </div>
  {/snippet}
</Story>

<style>
  .grid {
    display: grid;
    gap: var(--space-5);
    grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
  }
  h3 {
    color: var(--dim);
    font-size: var(--font-size-meta);
    margin: 0 0 var(--space-2);
  }
</style>
