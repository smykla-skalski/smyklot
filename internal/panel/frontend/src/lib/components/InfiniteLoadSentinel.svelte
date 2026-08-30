<script lang="ts">
  import { useIntersectionObserver } from 'runed';

  const {
    active,
    cursor,
    onVisible,
  }: {
    active: boolean;
    cursor: string | null | undefined;
    onVisible: () => void;
  } = $props();

  let sentinel = $state<HTMLDivElement>();

  useIntersectionObserver(
    () => sentinel,
    (entries) => {
      if (entries.some((entry) => entry.isIntersecting)) onVisible();
    },
    { rootMargin: '384px 0px' },
  );
</script>

<!--
@component
The marker a long list watches for, to ask for the next page when the reader gets near
the end. It draws nothing and it is not a control: it is a position, and the observer
that watches it belongs to the list.

It renders only while there is more to fetch. With no cursor there is no next page, and
a sentinel left in the page would go on reporting itself into view at the bottom of a
list that has already ended.

This is what a cursor-loaded table has instead of a pager. Nothing has counted the
rows, so there is no total to show and no page to turn.
-->

{#if active && cursor != null}
  <div class="infinite-load-sentinel" bind:this={sentinel} aria-hidden="true"></div>
{/if}

<style>
  .infinite-load-sentinel {
    height: 1px;
    pointer-events: none;
    width: 100%;
  }
</style>
