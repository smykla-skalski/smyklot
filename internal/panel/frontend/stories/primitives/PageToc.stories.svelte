<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import PageToc from '#lib/components/PageToc.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/PageToc',
    component: PageToc,
    args: {
      entries: [
        { id: 'ws-newrepos', label: 'New repositories' },
        { id: 'ws-merging', label: 'Merging' },
        { id: 'ws-behavior', label: 'Behavior' },
        { id: 'ws-commands', label: 'Commands' },
        { id: 'ws-timing', label: 'Timing' },
      ],
    },
  });
</script>

<!--
  The index only draws where there is room for it beside the reading column, and only
  where the page it indexes actually scrolls - both from a class it sets on the frame it
  stands in. Here it is given that frame and a page tall enough to earn one.
-->
<Story name="Default">
  {#snippet template(args)}
    <div class="view-frame page-scrolls">
      <div class="page-main">
        <section class="card" id="ws-newrepos">
          <div class="card-head"><h2 class="card-title">New repositories</h2></div>
        </section>
        <section class="card" id="ws-merging">
          <div class="card-head"><h2 class="card-title">Merging</h2></div>
        </section>
      </div>
      <PageToc {...args} />
    </div>
  {/snippet}
</Story>

<!--
  A page whose cards all fit on the screen gets its width back and no index at all.

  Not tagged `blank`: the tag says the STORY draws nothing, and this one draws the page
  the index is missing from - which is the whole of what it has to show. What is absent
  here is the component, and a story that renders its subject's absence still renders.
-->
<Story name="Page that does not scroll">
  {#snippet template(args)}
    <div class="view-frame">
      <div class="page-main">
        <section class="card">
          <div class="card-head"><h2 class="card-title">One card</h2></div>
        </section>
      </div>
      <PageToc {...args} />
    </div>
  {/snippet}
</Story>
