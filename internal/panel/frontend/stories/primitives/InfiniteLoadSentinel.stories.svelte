<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import InfiniteLoadSentinel from '#lib/components/InfiniteLoadSentinel.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/InfiniteLoadSentinel',
    component: InfiniteLoadSentinel,
    argTypes: { active: { control: 'boolean' } },
    args: { active: true, cursor: 'eyJpZCI6IjQwMDEifQ', onVisible: fn() },
  });
</script>

<!--
  Draws nothing. It watches for its own arrival in the viewport through runed's
  `useIntersectionObserver` and asks the caller for the next page; the `cursor` is
  what makes a second ask for the same page impossible.

  It is here so the catalogue is honest about what exists, and so a change to it has
  somewhere to be looked at.
-->
<Story name="Watching">
  {#snippet template(args)}
    <div class="frame">
      <p>The sentinel sits at the end of a list, drawing nothing:</p>
      <InfiniteLoadSentinel {...args} />
    </div>
  {/snippet}
</Story>

<!-- Nothing left to fetch, so it stops asking. -->
<Story name="Exhausted" args={{ active: false, cursor: null }}>
  {#snippet template(args)}
    <div class="frame">
      <p>Every page has been read:</p>
      <InfiniteLoadSentinel {...args} />
    </div>
  {/snippet}
</Story>

<style>
  .frame {
    border: 1px dashed var(--control-border);
    border-radius: var(--radius-control);
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    padding: var(--space-4);
  }
</style>
