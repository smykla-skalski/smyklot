<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import ServicePerformance from '#lib/components/ServicePerformance.svelte';
  import type {
    PerformancePoint,
    PerformanceSeries,
    ServicePerformance as Measurements,
  } from '#lib/types.js';

  function hours(count: number, shape: (step: number) => number): PerformancePoint[] {
    const start = Date.UTC(2026, 8, 4, 8);
    return Array.from({ length: count }, (_unused, step) => ({
      at: new Date(start + step * 3_600_000).toISOString(),
      observations: 120 + step * 3,
      mean_ms: shape(step),
      value: shape(step),
    }));
  }

  function series(label: string, shape: (step: number) => number): PerformanceSeries {
    return { label, points: hours(24, shape) };
  }

  const MEASURED: Measurements = {
    since: new Date(Date.UTC(2026, 8, 4, 8)).toISOString(),
    until: new Date(Date.UTC(2026, 8, 5, 8)).toISOString(),
    metrics: {
      query: [
        series('latestRecurringItem', (step) => 3 + step * step * 0.4),
        series('Store.ListWorkQueue', (step) => 22 + Math.sin(step / 3) * 4),
        series('activeRecurringItems', (step) => 8 + Math.cos(step / 4) * 2),
      ],
      ledger: [
        series('reaction_scan', (step) => 4000 + step * 180),
        series('config_migration', (step) => 3900 + step * 175),
        series('webhook_delivery', (step) => 60000 + step * 90),
      ],
      lane: [
        series('maintenance', (step) => 4 + Math.round(Math.sin(step / 4) * 3 + 3)),
        series('webhook', (step) => (step % 7 === 0 ? 12 : 1)),
      ],
      database: [
        series('size_bytes', (step) => 620_000_000 + step * 1_400_000),
        series('round_trip', (step) => 12 + Math.sin(step / 5) * 3),
      ],
    },
  };

  const EMPTY: Measurements = {
    since: MEASURED.since,
    until: MEASURED.until,
    metrics: { query: [], ledger: [], lane: [], database: [] },
  };

  const { Story } = defineMeta({
    title: 'Views/ServicePerformance',
    component: ServicePerformance,
    args: {
      fetchPerformance: () => Promise.resolve(MEASURED),
    },
  });
</script>

<!--
  What the service has cost itself, under Runtime's live health. The statements chart is
  the one that would have caught a query which had quietly become a sequential scan, and
  the kept-work chart the ledger that grew for eleven days.
-->
<Story name="Measured">
  {#snippet template(args)}
    <div class="stage">
      <ServicePerformance {...args} />
    </div>
  {/snippet}
</Story>

<Story name="Nothing measured yet" args={{ fetchPerformance: () => Promise.resolve(EMPTY) }}>
  {#snippet template(args)}
    <div class="stage">
      <ServicePerformance {...args} />
    </div>
  {/snippet}
</Story>

<Story
  name="Unreadable"
  args={{ fetchPerformance: () => Promise.reject(new Error('the database did not answer')) }}
>
  {#snippet template(args)}
    <div class="stage">
      <ServicePerformance {...args} />
    </div>
  {/snippet}
</Story>

<style>
  .stage {
    max-inline-size: var(--content-max);
    padding: var(--space-4);
  }
</style>
