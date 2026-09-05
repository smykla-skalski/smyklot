<script module lang="ts">
  import { formatDateTime, formatDayAndMonth, formatTimeOfDay } from '#lib/format.js';

  const DAY = 24 * 60 * 60 * 1000;

  const FLOOR_SHARE = 0.5;

  function tickLabel(at: Date, span: number): string {
    return span > 2 * DAY ? formatDayAndMonth(at.toISOString()) : formatTimeOfDay(at.toISOString());
  }

  function fullLabel(at: Date): string {
    return formatDateTime(at.toISOString());
  }

  function niceStep(rough: number): number {
    const power = 10 ** Math.floor(Math.log10(rough));
    const scaled = rough / power;

    return (scaled >= 5 ? 5 : scaled >= 2 ? 2 : 1) * power;
  }

  function niceDomain(low: number, high: number): [number, number] {
    if (!(high > low)) return [low, low + 1];

    const step = niceStep((high - low) / 2);

    return [Math.floor(low / step) * step, Math.ceil(high / step) * step];
  }

  function readableTicks(low: number, high: number, say: (value: number) => string): number[] {
    if (say(low) === say(high)) return [low];

    const middle = (low + high) / 2;
    const collides = say(middle) === say(low) || say(middle) === say(high);

    return collides ? [low, high] : [low, middle, high];
  }
</script>

<script lang="ts">
  import { Axis, Chart, Highlight, Spline, Svg, Tooltip } from 'layerchart';

  import type { PerformancePoint } from '#lib/types.ts';

  const {
    label,
    points,
    read,
    format,
    tick,
    caption = '',
  }: {
    label: string;
    points: PerformancePoint[];
    read: (point: PerformancePoint) => number;
    format: (value: number) => string;
    tick: (value: number) => string;
    caption?: string;
  } = $props();

  const drawn = $derived(points.map((point) => ({ at: new Date(point.at), value: read(point) })));
  const latest = $derived(drawn.at(-1)?.value ?? 0);
  const peak = $derived(drawn.reduce((most, point) => Math.max(most, point.value), 0));
  const floor = $derived(drawn.reduce((least, point) => Math.min(least, point.value), peak));
  const zeroed = $derived(peak <= 0 || floor < peak * FLOOR_SHARE);
  const domain = $derived<[number, number]>(
    zeroed ? niceDomain(0, peak === 0 ? 1 : peak) : niceDomain(floor, peak),
  );
  const marks = $derived(readableTicks(domain[0], domain[1], tick));
  const span = $derived(
    drawn.length < 2 ? 0 : (drawn.at(-1)?.at.getTime() ?? 0) - (drawn[0]?.at.getTime() ?? 0),
  );
</script>

<figure class="performance-chart">
  <figcaption>
    <span class="chart-head">
      <span class="chart-name">{label}</span>
      <span class="chart-latest">{format(latest)}</span>
    </span>
    {#if caption !== ''}<span class="chart-caption">{caption}</span>{/if}
  </figcaption>
  {#if drawn.length === 0}
    <p class="chart-empty">Nothing measured yet</p>
  {:else if drawn.length === 1}
    <p class="chart-empty">One hour measured so far</p>
  {:else}
    <div class="chart-plot">
      <Chart
        data={drawn}
        x="at"
        y="value"
        yDomain={domain}
        padding={{ left: 40, bottom: 18, right: 4, top: 6 }}
        tooltipContext={{ mode: 'bisect-x' }}
      >
        <Svg>
          <Axis placement="left" grid rule format={tick} ticks={marks} />
          <Axis placement="bottom" rule format={(at: Date) => tickLabel(at, span)} ticks={2} />
          <Spline class="chart-line" />
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

It takes TWO formatters, and the difference matters. `format` says the whole thing and
is what the reader is told once, beside the name and in the tooltip - "4,050 rows".
`tick` is the axis, where the unit is already known and repeating it eleven times both
says nothing and pushes every plot in the row to a different left edge, which is what
made a grid of these look ragged. Ticks are bare numbers; the left gutter is fixed, so
neighbours line up.

The mark is a LINE, always, and never a filled area. A fill reads as magnitude measured
from the axis, which is only true when the axis is at zero - and a zero baseline is
exactly what a series like the database's size on disk cannot have: its lowest point is
within a few percent of its highest, so drawn from zero it is a flat block that hides
the only thing worth seeing. Those get a domain that starts at their own floor, which
the axis states; everything whose floor is under half its peak keeps zero. Two marks for
the two cases was the alternative, and a grid where some charts are filled and some are
not reads as an accident rather than as a distinction.

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

  .chart-head {
    align-items: center;
    column-gap: var(--space-3);
    display: flex;
    justify-content: space-between;
  }

  .chart-name {
    color: var(--text-primary);
    font-size: var(--font-size-compact);
    line-height: var(--leading-compact);
    min-inline-size: 0;
    overflow-wrap: break-word;
    text-box: trim-both cap alphabetic;
  }

  .chart-latest {
    color: var(--text-primary);
    flex: none;
    font-family: var(--mono);
    font-size: var(--font-size-body);
    line-height: var(--leading-body);
    text-box: trim-both cap alphabetic;
    white-space: nowrap;
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
    block-size: 6.5rem;
    min-inline-size: 0;
  }

  .chart-plot :global(.grid line) {
    stroke: var(--chart-grid);
    stroke-dasharray: 2 3;
  }

  .chart-plot :global(.chart-line) {
    stroke: var(--chart-ink);
    stroke-width: 2;
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
