<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import PerformanceChart from '#lib/components/PerformanceChart.svelte';
  import type { PerformancePoint } from '#lib/types.js';

  function hours(count: number, shape: (step: number) => number): PerformancePoint[] {
    const start = Date.UTC(2026, 8, 4, 8);
    return Array.from({ length: count }, (_unused, step) => ({
      at: new Date(start + step * 3_600_000).toISOString(),
      observations: 40 + step,
      mean_ms: shape(step),
      max_ms: shape(step) * 3,
    }));
  }

  const STEADY = hours(24, (step) => 4 + Math.sin(step / 3) * 0.8);
  const CLIMBING = hours(24, (step) => 3 + step * step * 0.09);

  const { Story } = defineMeta({
    title: 'Views/PerformanceChart',
    component: PerformanceChart,
    args: {
      label: 'Store.ListWorkQueue',
      points: STEADY,
      read: (point: PerformancePoint) => point.mean_ms ?? 0,
      format: (value: number) => `${value.toFixed(2)} ms`,
      tick: (value: number) => String(Number(value.toFixed(1))),
      caption: '968 calls in the window',
    },
  });
</script>

<!--
  One measured series, named rather than coloured. Three of them side by side is the
  shape the page uses: each carries its own scale, so what a reader compares is a line
  against its own past rather than against its neighbour.
-->
<Story name="Steady">
  {#snippet template(args)}
    <div class="stage">
      <PerformanceChart {...args} />
    </div>
  {/snippet}
</Story>

<Story name="Climbing" args={{ points: CLIMBING, label: 'latestRecurringItem' }}>
  {#snippet template(args)}
    <div class="stage">
      <PerformanceChart {...args} />
    </div>
  {/snippet}
</Story>

<Story name="Nothing measured" args={{ points: [] }}>
  {#snippet template(args)}
    <div class="stage">
      <PerformanceChart {...args} />
    </div>
  {/snippet}
</Story>

<style>
  .stage {
    inline-size: 16rem;
    padding: var(--space-4);
  }
</style>
