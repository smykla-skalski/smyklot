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
