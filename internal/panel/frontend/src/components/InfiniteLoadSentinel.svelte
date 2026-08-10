<script lang="ts">
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

  $effect(() => {
    if (!active || cursor == null || sentinel === undefined) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) onVisible();
      },
      { rootMargin: '384px 0px' },
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  });
</script>

{#if active}
  <div class="infinite-load-sentinel" bind:this={sentinel} aria-hidden="true"></div>
{/if}

<style>
  .infinite-load-sentinel {
    height: 1px;
    pointer-events: none;
    width: 100%;
  }
</style>
