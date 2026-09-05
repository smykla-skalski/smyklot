<script module lang="ts">
  import { formatBytes, formatCount, formatElapsed, formatLatency } from '#lib/format.js';
  import type { PerformancePoint, PerformanceSeries, QueueWorkload } from '#lib/types.ts';
  import { WORKLOAD_COPY, workloadTitle } from '#lib/workloads.js';

  const DEFAULT_WINDOW = 24;

  const SAMPLE_INTERVAL = 5 * 60 * 1000;

  const WINDOWS = [
    { value: '24', label: 'Day' },
    { value: '168', label: 'Week' },
    { value: '720', label: 'Month' },
  ];

  function readMean(point: PerformancePoint): number {
    return point.mean_ms ?? 0;
  }

  function readValue(point: PerformancePoint): number {
    return point.value ?? 0;
  }

  function sayRows(value: number): string {
    return formatCount(Math.round(value), 'row');
  }

  function sayCalls(points: PerformanceSeries['points']): string {
    const calls = points.reduce((total, point) => total + (point.observations ?? 0), 0);

    return formatCount(calls, 'call');
  }

  function sayWaiting(value: number): string {
    return `${formatCount(Math.round(value), 'item')} waiting`;
  }

  function sayOldest(points: PerformanceSeries['points']): string {
    const oldest = points.at(-1)?.max_ms ?? 0;

    return oldest <= 0 ? 'nothing has waited' : `oldest ${formatElapsed(oldest)}`;
  }

  function sayWorkload(label: string): string {
    return label in WORKLOAD_COPY ? workloadTitle(label as QueueWorkload) : label;
  }

  function sayLane(label: string): string {
    return LANE_TITLES[label] ?? label;
  }

  function sayDatabase(label: string): string {
    return DATABASE_TITLES[label] ?? label;
  }

  const LANE_TITLES: Record<string, string> = {
    webhook: 'Webhook intake',
    pending_ci: 'Merge after CI',
    maintenance: 'Background work',
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
    caption?: (points: PerformanceSeries['points']) => string;
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
        heading: 'Statements, average',
        note: 'What one call of each of the busiest statements took. A line that climbs while nothing else changed is an index the ledger has outgrown.',
        empty: 'The service records this every five minutes once it has run a statement',
        series: queries,
        name: (label) => label,
        read: readMean,
        format: formatLatency,
        caption: sayCalls,
      },
      {
        id: 'performance-ledger',
        heading: 'Finished work kept',
        note: 'How many finished rows each workload still holds. Retention should take these down again; one that only ever climbs is a workload nothing is pruning.',
        empty: 'The service records this every five minutes',
        series: kept,
        name: sayWorkload,
        read: readValue,
        format: sayRows,
      },
      {
        id: 'performance-lanes',
        heading: 'Work waiting',
        note: "How deep each lane's backlog is, and beside it how long the oldest thing in it had been waiting. A lane that climbs and stays up is work arriving faster than it leaves.",
        empty: 'Nothing has been waiting in any lane',
        series: lanes,
        name: sayLane,
        read: readValue,
        format: sayWaiting,
        caption: sayOldest,
      },
      {
        id: 'performance-database',
        heading: 'The database itself',
        note: 'What the database reports about its own size, responsiveness and pool.',
        empty: 'The service records this every five minutes',
        series: database,
        name: sayDatabase,
        read: (point) => point.value ?? point.mean_ms ?? 0,
        format: sayDatabaseValue,
      },
    ];
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

<Card labelledby="service-performance">
  <div class="performance-head">
    <h2 class="group-name" id="service-performance">What the service has cost</h2>
    <SegmentedControl
      name="performance-window"
      label="How far back"
      compact
      options={WINDOWS}
      value={String(windowHours)}
      onSelect={(chosen) => (windowHours = Number(chosen))}
    />
  </div>

  {#if measurements.isPending && measured === undefined}
    <p class="performance-state" role="status">Reading what has been measured</p>
  {:else if measured === undefined}
    <ResultProblem
      title="These numbers could not be read"
      problem={measurements.error?.message ?? 'the database did not answer'}
      busy={measurements.isFetching}
      onRetry={() => void measurements.refetch()}
    />
  {:else}
    {#each groups as group (group.id)}
      <section class="performance-group" aria-labelledby={group.id}>
        <h3 class="performance-heading" id={group.id}>{group.heading}</h3>
        <p class="performance-note">{group.note}</p>
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
                caption={group.caption?.(one.points) ?? ''}
              />
            {/each}
          </div>
        {/if}
      </section>
    {/each}
  {/if}
</Card>

<!--
@component
What the service has cost itself, as a grid of small named charts.

It sits under Runtime's service health, where the live numbers are: those say what the
database is now, and these say how it got there. A ledger that grew for eleven days and
a statement that had quietly become a sequential scan were both invisible until someone
ran EXPLAIN by hand, and this is the page that would have shown them.

Every chart carries its own scale and its own name, so nothing is identified by colour
and no legend is needed. Two charts side by side are NOT comparable by height - the
current value is printed beside each name for that. The alternative, a dozen series
sharing one axis, needs a categorical palette, and one that survives colour-blind
separation runs out at about four hues.

The window is a segmented control rather than an address: which slice of time is being
read is a question a reader asks and answers in a second, and a reload landing back on
the day is the right default rather than a loss.
-->

<style>
  .performance-head {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    justify-content: space-between;
  }

  .performance-group {
    display: grid;
    gap: var(--space-2);
    margin-block-start: var(--space-6);
  }

  .performance-heading {
    color: var(--text-primary);
    font-size: var(--font-size-compact);
    line-height: var(--leading-compact);
    margin: 0;
  }

  .performance-note,
  .performance-state {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
    line-height: var(--leading-meta);
    margin: 0;
    max-inline-size: var(--measure-note);
  }

  .performance-grid {
    display: grid;
    gap: var(--space-5);
    grid-template-columns: repeat(auto-fill, minmax(13rem, 1fr));
    margin-block-start: var(--space-2);
  }
</style>
