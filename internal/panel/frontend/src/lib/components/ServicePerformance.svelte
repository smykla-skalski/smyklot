<script module lang="ts">
  import {
    formatBytes,
    formatCompact,
    formatCount,
    formatElapsed,
    formatLatency,
  } from '#lib/format.js';
  import type { PerformancePoint, PerformanceSeries, QueueWorkload } from '#lib/types.ts';
  import { WORKLOAD_COPY, workloadTitle } from '#lib/workloads.js';

  const DEFAULT_WINDOW = 24;

  const SAMPLE_INTERVAL = 5 * 60 * 1000;

  const WINDOWS = [
    { value: '24', label: '24 hours' },
    { value: '168', label: '7 days' },
    { value: '720', label: '30 days' },
  ];

  function readMean(point: PerformancePoint): number {
    return point.mean_ms ?? 0;
  }

  function readValue(point: PerformancePoint): number {
    return point.value ?? 0;
  }

  function sayRows(value: number): string {
    return formatCompact(Math.round(value), 'row');
  }

  // A statement that has started failing is what this page exists to find, so
  // the count says so where there is one rather than waiting for a chart of
  // its own.
  function sayCalls(points: PerformanceSeries['points']): string {
    const calls = points.reduce((total, point) => total + (point.observations ?? 0), 0);
    const failed = points.reduce((total, point) => total + (point.failures ?? 0), 0);
    const said = `${formatCompact(calls, 'call')} in the window`;

    return failed === 0 ? said : `${said}, ${formatCompact(failed)} failed`;
  }

  function sayWaiting(value: number): string {
    return `${formatCount(Math.round(value), 'item')} waiting`;
  }

  function sayOldest(points: PerformanceSeries['points']): string {
    const oldest = points.at(-1)?.max_ms ?? 0;

    return oldest <= 0 ? 'nothing waiting right now' : `longest wait ${formatElapsed(oldest)}`;
  }

  function sayChange(points: PerformanceSeries['points'], say: (value: number) => string): string {
    const first = points[0]?.value ?? 0;
    const last = points.at(-1)?.value ?? 0;
    const change = last - first;

    if (change === 0) return 'unchanged over this window';

    return `${change > 0 ? 'up' : 'down'} ${say(Math.abs(change))} over this window`;
  }

  function tickLatency(value: number): string {
    if (value >= 10) return String(Math.round(value));

    return String(Number(value.toFixed(value >= 1 ? 1 : 2)));
  }

  function tickRows(value: number): string {
    return formatCompact(Math.round(value));
  }

  function sayWorkload(label: string): string {
    return label in WORKLOAD_COPY ? workloadTitle(label as QueueWorkload) : label;
  }

  function sayStatement(label: string): string {
    return label
      .split('.')
      .map((part) => part.replace(/([a-z\d])([A-Z])/gu, '$1 $2').toLocaleLowerCase())
      .filter((part) => part !== 'store')
      .join(' · ')
      .replace(/^./u, (first) => first.toLocaleUpperCase());
  }

  function sayLane(label: string): string {
    return LANE_TITLES[label] ?? label;
  }

  function sayDatabase(label: string): string {
    return DATABASE_TITLES[label] ?? label;
  }

  const LANE_TITLES: Record<string, string> = {
    webhook: 'Webhook queue',
    pending_ci: 'CI re-check queue',
    maintenance: 'Background queue',
  };

  const DATABASE_TITLES: Record<string, string> = {
    size_bytes: 'Size on disk',
    round_trip: 'Round trip',
    pool_in_use: 'Connections in use',
    pool_waits: 'Waits for a connection',
  };

  interface PerformanceGroup {
    id: string;
    heading: string;
    note: string;
    empty: string;
    series: PerformanceSeries[];
    name: (label: string) => string;
    read: (point: PerformancePoint) => number;
    format: (value: number, label: string) => string;
    tick: (value: number, label: string) => string;
    caption: (points: PerformanceSeries['points'], label: string) => string;
  }

  // Every kind of work is measured, including the ones holding nothing, so a
  // count that reaches zero is drawn reaching zero rather than stopping. A kind
  // that held nothing for the whole window has no line to draw.
  function everHeldRows(one: PerformanceSeries): boolean {
    return one.points.some((point) => (point.value ?? 0) > 0);
  }

  function sections(
    queries: PerformanceSeries[],
    kept: PerformanceSeries[],
    lanes: PerformanceSeries[],
    database: PerformanceSeries[],
  ): PerformanceGroup[] {
    return [
      {
        id: 'performance-queries',
        heading: 'How long a database read takes',
        note: 'One call of each of the busiest reads. A line that climbs while nothing else changed is a query that has outgrown its index',
        empty: 'Reads are recorded as they run, and none has run yet',
        series: queries,
        name: sayStatement,
        read: readMean,
        format: formatLatency,
        tick: tickLatency,
        caption: sayCalls,
      },
      {
        id: 'performance-ledger',
        heading: 'Rows kept after work finishes',
        note: 'Retention should take each of these down again. One that only ever climbs is work nothing is pruning',
        empty: 'Rows are counted as work finishes, and nothing has finished yet',
        series: kept.filter(everHeldRows),
        name: sayWorkload,
        read: readValue,
        format: sayRows,
        tick: tickRows,
        caption: (points) => sayChange(points, sayRows),
      },
      {
        id: 'performance-lanes',
        heading: 'Work waiting to run',
        note: 'A queue that climbs and stays up is work arriving faster than it leaves',
        empty: 'Every queue has been empty',
        series: lanes,
        name: sayLane,
        read: readValue,
        format: sayWaiting,
        tick: tickRows,
        caption: sayOldest,
      },
      {
        id: 'performance-database',
        heading: 'What the database says about itself',
        note: 'Its size on disk, how quickly it answers, and how hard its connection pool is working',
        empty: 'The database has not been sampled yet',
        series: database,
        name: sayDatabase,
        read: (point) => point.value ?? point.mean_ms ?? 0,
        format: sayDatabaseValue,
        tick: (value, label) =>
          label === 'size_bytes' ? formatBytes(value) : tickLatencyOrCount(value, label),
        caption: sayDatabaseCaption,
      },
    ];
  }

  function tickLatencyOrCount(value: number, label: string): string {
    return label === 'round_trip' ? tickLatency(value) : tickRows(value);
  }

  function sayDatabaseCaption(points: PerformanceSeries['points'], label: string): string {
    if (label === 'round_trip') {
      const worst = points.reduce((slowest, point) => Math.max(slowest, point.max_ms ?? 0), 0);

      return `slowest ${formatLatency(worst)}`;
    }

    if (label === 'pool_waits') {
      const waits = points.reduce((most, point) => Math.max(most, point.value ?? 0), 0);

      return waits <= 0
        ? 'never waited for a connection'
        : `busiest stretch waited ${formatCount(waits, 'time')}`;
    }

    return sayChange(points, (value) => sayDatabaseValue(value, label));
  }

  function sayDatabaseValue(value: number, label: string): string {
    if (label === 'size_bytes') return formatBytes(value);
    if (label === 'round_trip') return formatLatency(value);

    return formatCount(Math.round(value), label === 'pool_waits' ? 'wait' : 'connection');
  }
</script>

<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';

  import Card from './Card.svelte';
  import EmptyState from './EmptyState.svelte';
  import PerformanceChart from './PerformanceChart.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import type { ServicePerformance as Measurements } from '#lib/types.ts';

  const {
    fetchPerformance,
  }: {
    fetchPerformance: (windowHours: number) => Promise<Measurements>;
  } = $props();

  let windowHours = $state(DEFAULT_WINDOW);

  const measurements = createQuery(() => ({
    queryKey: ['root', 'performance', windowHours],
    queryFn: () => fetchPerformance(windowHours),
    staleTime: SAMPLE_INTERVAL,
    refetchInterval: SAMPLE_INTERVAL,
  }));

  const measured = $derived(measurements.data);
  const queries = $derived(measured?.metrics.query ?? []);
  const kept = $derived(measured?.metrics.ledger ?? []);
  const lanes = $derived(measured?.metrics.lane ?? []);
  const database = $derived(measured?.metrics.database ?? []);
  const groups = $derived(sections(queries, kept, lanes, database));
</script>

<div class="filter-bar">
  <SegmentedControl
    name="performance-window"
    label="How far back"
    options={WINDOWS}
    value={String(windowHours)}
    onSelect={(chosen) => (windowHours = Number(chosen))}
  />
</div>

{#if measurements.isPending && measured === undefined}
  <Card>
    <p class="performance-state" role="status">Reading what has been measured</p>
  </Card>
{:else if measured === undefined}
  <Card>
    <ResultProblem
      title="These numbers could not be read"
      problem={measurements.error?.message ?? 'the database did not answer'}
      busy={measurements.isFetching}
      onRetry={() => void measurements.refetch()}
    />
  </Card>
{:else}
  {#each groups as group (group.id)}
    <Card labelledby={group.id}>
      <div class="card-head"><h2 class="card-title" id={group.id}>{group.heading}</h2></div>
      <p class="group-note">{group.note}</p>
      {#if group.series.length === 0}
        <EmptyState title="Nothing measured in this window" description={group.empty} />
      {:else}
        <div class="performance-grid">
          {#each group.series as one (one.label)}
            <PerformanceChart
              label={group.name(one.label)}
              points={one.points}
              read={group.read}
              format={(value: number) => group.format(value, one.label)}
              tick={(value: number) => group.tick(value, one.label)}
              caption={group.caption(one.points, one.label)}
            />
          {/each}
        </div>
      {/if}
    </Card>
  {/each}
{/if}

<!--
@component
How the service has been running, as one card per family of measurements.

It sits under Runtime's service health, where the live numbers are: those say what the
database is now, and these say how it got there. A ledger that grew for eleven days and
a statement that had quietly become a sequential scan were both invisible until someone
ran EXPLAIN by hand, and this is the page that would have shown them.

A CARD PER FAMILY, and not one card of four sections. The four are read separately -
nobody asks about query latency and connection waits in the same breath - and a section
heading inside a card is a level the page does not have: the cards above this one are
`h2` under the page's `h1`, so a fifth card holding four `h3` sections read as one thing
with parts rather than as four peers. Every card is the panel's own card head and note,
which is what carries the distances; this component states none of them.

The window belongs to all four, so it is a toolbar over the stack rather than a control
in one card's head. A segmented control rather than an address: which slice of time is
being read is a question a reader asks and answers in a second, and a reload landing
back on the day is the right default rather than a loss.

Every chart carries its own scale and its own name, so nothing is identified by colour
and no legend is needed. Two charts side by side are NOT comparable by height - the
current value is printed beside each name for that. The alternative, a dozen series
sharing one axis, needs a categorical palette, and one that survives colour-blind
separation runs out at about four hues.
-->

<style>
  .performance-state {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    line-height: var(--leading-meta);
    margin: 0;
  }

  .performance-grid {
    display: grid;
    gap: var(--space-6);
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  @media (max-width: 75rem) {
    .performance-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  @media (max-width: 48rem) {
    .performance-grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
