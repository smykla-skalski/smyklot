<script module lang="ts">
  import { formatDateTime, formatDayAndMonth, formatTimeOfDay } from '#lib/format.js';

  const DAY = 24 * 60 * 60 * 1000;

  function tickLabel(at: Date, span: number): string {
    return span > 2 * DAY ? formatDayAndMonth(at.toISOString()) : formatTimeOfDay(at.toISOString());
  }

  function fullLabel(at: Date): string {
    return formatDateTime(at.toISOString());
  }
</script>

<script lang="ts">
  import { Area, Axis, Chart, Highlight, Svg, Tooltip } from 'layerchart';

  import type { PerformancePoint } from '#lib/types.ts';

  const {
    label,
    points,
    read,
    format,
    caption = '',
  }: {
    label: string;
    points: PerformancePoint[];
    read: (point: PerformancePoint) => number;
    format: (value: number) => string;
    caption?: string;
  } = $props();

  const drawn = $derived(points.map((point) => ({ at: new Date(point.at), value: read(point) })));
  const latest = $derived(drawn.at(-1)?.value ?? 0);
  const peak = $derived(drawn.reduce((worst, point) => Math.max(worst, point.value), 0));
  const span = $derived(
    drawn.length < 2 ? 0 : (drawn.at(-1)?.at.getTime() ?? 0) - (drawn[0]?.at.getTime() ?? 0),
  );
</script>

<figure class="performance-chart">
  <figcaption>
    <span class="chart-name">{label}</span>
    <span class="chart-latest">{format(latest)}</span>
    {#if caption !== ''}<span class="chart-caption">{caption}</span>{/if}
  </figcaption>
  {#if drawn.length === 0}
    <p class="chart-empty">Nothing measured yet</p>
  {:else}
    <div class="chart-plot">
      <Chart
        data={drawn}
        x="at"
        y="value"
        yDomain={[0, peak === 0 ? 1 : peak]}
        yNice
        padding={{ left: 0, bottom: 16, right: 0, top: 4 }}
        tooltipContext={{ mode: 'bisect-x' }}
      >
        <Svg>
          <Axis placement="bottom" rule format={(at: Date) => tickLabel(at, span)} ticks={2} />
          <Area line={{ class: 'chart-line' }} class="chart-fill" />
          <Highlight points lines />
        </Svg>
        <Tooltip.Root portal={{ target: '.app-shell' }}>
          {#snippet children({ data }: { data: { at: Date; value: number } })}
            <Tooltip.Header>{fullLabel(data.at)}</Tooltip.Header>
            <Tooltip.List>
              <Tooltip.Item {label} value={format(data.value)} />
            </Tooltip.List>
          {/snippet}
        </Tooltip.Root>
      </Chart>
    </div>
  {/if}
</figure>

<!--
@component
One measured series over time, drawn small and named rather than coloured.

Reach for it in a grid of its own kind: a page that has to show a dozen series shows a
dozen of these rather than a dozen lines sharing one axis. Series on one axis need a
categorical palette, and a palette that survives colour-blind separation runs out at
about four hues - past that identity is colour alone, which is the thing a legend
exists to avoid. Small multiples need no palette, because the chart's own heading is
its identity and every one of them uses the same ink.

It draws its own scale from its own data, so two of these side by side are NOT
comparable by height. That is the trade: a shared scale flattens the quiet series into
the axis, and what a reader asks of this page is whether one thing changed, not whether
it is larger than its neighbour. The current value is printed beside the name, which is
what keeps magnitudes readable.

The plot is interactive by default - a crosshair follows the pointer and a tooltip says
the hour and the value - and a series with nothing in it is a sentence rather than an
empty axis.
-->

<style>
  .performance-chart {
    display: grid;
    gap: var(--space-2);
    margin: 0;
    min-inline-size: 0;
  }

  figcaption {
    display: grid;
    gap: var(--space-1);
  }

  .chart-name {
    color: var(--text-primary);
    font-size: var(--font-size-compact);
    line-height: var(--leading-compact);
    overflow-wrap: anywhere;
  }

  .chart-latest {
    color: var(--text-secondary);
    font-family: var(--mono);
    font-size: var(--font-size-meta);
    line-height: var(--leading-meta);
  }

  .chart-caption,
  .chart-empty {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    line-height: var(--leading-micro);
  }

  .chart-empty {
    margin: 0;
  }

  .chart-plot {
    block-size: 5rem;
    min-inline-size: 0;
  }

  .chart-plot :global(.chart-line) {
    stroke: var(--chart-ink);
    stroke-width: 2;
  }

  .chart-plot :global(.chart-fill) {
    fill: var(--chart-fill);
  }

  .chart-plot :global(.tick text),
  .chart-plot :global(.axis text) {
    fill: var(--text-muted);
    font-size: var(--font-size-nano);
  }

  .chart-plot :global(.tick line),
  .chart-plot :global(.rule line) {
    stroke: var(--chart-grid);
  }
</style>
