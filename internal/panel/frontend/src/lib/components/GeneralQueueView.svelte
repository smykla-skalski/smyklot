<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { SvelteURLSearchParams } from 'svelte/reactivity';
  import type { PanelApi } from '#lib/api.js';
  import type {
    QueueActionInput,
    QueueActionType,
    QueueDetail,
    QueueItem,
    QueuePage,
    QueuePriority,
    QueueSchedulePreview,
    QueueWorkload,
  } from '#lib/types.js';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import PageHeader from './PageHeader.svelte';
  import Plate from './Plate.svelte';
  import QueueActionDialog from './QueueActionDialog.svelte';
  import QueueDetailDialog from './QueueDetailDialog.svelte';
  import QueueTable from './QueueTable.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import TableToolsMenu, { type ToolsFilter } from './TableToolsMenu.svelte';

  type Section = 'active' | 'approvals' | 'history';

  const {
    api,
    targetId,
    rootRole = '',
    canControl = false,
    refreshRevision = 0,
  }: {
    api: PanelApi;
    targetId?: string;
    rootRole?: string;
    canControl?: boolean;
    refreshRevision?: number;
  } = $props();

  let items = $state.raw<QueueItem[]>([]);
  let facets = $state.raw<QueuePage['facets']>({
    targets: [],
    repositories: [],
    profiles: [],
    states: [],
    workloads: [],
    priorities: [],
  });
  let section = $state<Section>('active');
  let workload = $state<QueueWorkload | 'all'>('all');
  let priority = $state<QueuePriority | 'all'>('all');
  let stateFilter = $state('all');
  let profile = $state('all');
  let installation = $state('all');
  let repository = $state('all');
  let timeRange = $state<'all' | '24h' | '7d'>('all');
  let loading = $state(true);
  let error = $state('');
  let announcement = $state('');
  let selected = $state<QueueItem | null>(null);
  let selectedAction = $state<QueueActionType | null>(null);
  let actionBusy = $state(false);
  let actionError = $state('');
  let detail = $state<QueueDetail | null>(null);
  let detailOpen = $state(false);
  let detailLoading = $state(false);
  let detailError = $state('');
  let detailRefreshRevision = -1;
  let now = $state(Date.now());
  let offset = $state(0);
  let nextOffset = $state(0);
  let total = $state(0);
  let loadGeneration = 0;
  let detailLoadGeneration = 0;
  const pageSize = 50;

  const workloads = $derived(facets.workloads);
  const profiles = $derived(facets.profiles);
  const installations = $derived(facets.targets);
  const repositories = $derived(facets.repositories);
  const states = $derived(facets.states.filter((value) => sectionStates(section).includes(value)));
  const rangeStart = $derived(total === 0 ? 0 : offset + 1);
  const rangeEnd = $derived(Math.min(offset + items.length, total));
  const query = $derived.by(queueQuery);
  const queueFilters = $derived<ToolsFilter[]>([
    {
      label: 'Workload',
      hint: 'Show one kind of background work',
      sections: [
        {
          options: [
            { value: 'all', label: 'All workloads' },
            ...workloads.map((kind) => ({
              value: kind,
              label: kind.replaceAll('_', ' '),
            })),
          ],
        },
      ],
      selected: [workload],
      fallbackValue: 'all',
      onChange: (values: string[]) => {
        workload = (values[0] ?? 'all') as QueueWorkload | 'all';
        offset = 0;
      },
    },
    {
      label: 'State',
      hint: 'Narrow the current Queue view',
      sections: [
        {
          options: [
            { value: 'all', label: 'All states' },
            ...states.map((value) => ({ value, label: value.replaceAll('_', ' ') })),
          ],
        },
      ],
      selected: [stateFilter],
      fallbackValue: 'all',
      onChange: (values: string[]) => {
        stateFilter = values[0] ?? 'all';
        offset = 0;
      },
    },
    {
      label: 'Window',
      hint: 'Filter by assigned execution window',
      sections: [
        {
          options: [
            { value: 'all', label: 'All windows' },
            ...profiles.map((value) => ({ value, label: value })),
          ],
        },
      ],
      selected: [profile],
      fallbackValue: 'all',
      onChange: (values: string[]) => {
        profile = values[0] ?? 'all';
        offset = 0;
      },
    },
    ...(targetId === undefined
      ? [
          {
            label: 'Installation',
            hint: 'Limit work to one installation',
            sections: [
              {
                options: [
                  { value: 'all', label: 'All installations' },
                  ...installations.map((value) => ({ value, label: value })),
                ],
              },
            ],
            selected: [installation],
            fallbackValue: 'all',
            onChange: (values: string[]) => {
              installation = values[0] ?? 'all';
              offset = 0;
            },
          },
        ]
      : []),
    {
      label: 'Repository',
      hint: 'Limit work to one repository',
      sections: [
        {
          options: [
            { value: 'all', label: 'All repositories' },
            ...repositories.map((value) => ({ value, label: value })),
          ],
        },
      ],
      selected: [repository],
      fallbackValue: 'all',
      onChange: (values: string[]) => {
        repository = values[0] ?? 'all';
        offset = 0;
      },
    },
    {
      label: 'Created',
      hint: 'Limit work by creation time',
      sections: [
        {
          options: [
            { value: 'all', label: 'Any time' },
            { value: '24h', label: 'Last 24 hours' },
            { value: '7d', label: 'Last 7 days' },
          ],
        },
      ],
      selected: [timeRange],
      fallbackValue: 'all',
      onChange: (values: string[]) => {
        timeRange = (values[0] ?? 'all') as 'all' | '24h' | '7d';
        offset = 0;
      },
    },
    {
      label: 'Priority',
      hint: 'Show one dispatch priority band',
      sections: [
        {
          options: [
            { value: 'all', label: 'All priorities' },
            { value: 'urgent', label: 'Urgent' },
            { value: 'high', label: 'High' },
            { value: 'normal', label: 'Normal' },
            { value: 'low', label: 'Low' },
          ],
        },
      ],
      selected: [priority],
      fallbackValue: 'all',
      onChange: (values: string[]) => {
        priority = (values[0] ?? 'all') as QueuePriority | 'all';
        offset = 0;
      },
    },
  ]);

  onMount(() => {
    const refresh = window.setInterval(() => {
      void load(false);
      const itemID = detailOpen ? detail?.item.id : undefined;
      if (itemID !== undefined) void loadDetail(itemID, false, refreshRevision);
    }, 30_000);
    const clock = window.setInterval(() => (now = Date.now()), 1_000);
    return () => {
      window.clearInterval(refresh);
      window.clearInterval(clock);
    };
  });

  $effect(() => {
    const revision = refreshRevision;
    const nextQuery = query;
    untrack(() => void load(items.length === 0, nextQuery, revision));
  });

  $effect(() => {
    const revision = refreshRevision;
    const itemID = detailOpen ? detail?.item.id : undefined;
    if (revision > detailRefreshRevision && itemID !== undefined) {
      untrack(() => {
        detailRefreshRevision = revision;
        void loadDetail(itemID, false, revision);
      });
    }
  });

  function sectionStates(value: Section): QueueItem['state'][] {
    if (value === 'approvals') return ['awaiting_approval'];
    if (value === 'history') return ['succeeded', 'failed', 'cancelled', 'superseded'];
    return ['scheduled', 'blocked', 'ready', 'running', 'retrying'];
  }

  function queueQuery(): string {
    const query = new SvelteURLSearchParams({ limit: String(pageSize), offset: String(offset) });
    const selectedStates =
      stateFilter === 'all' ? sectionStates(section) : [stateFilter as QueueItem['state']];
    query.set('state', selectedStates.join(','));
    if (workload !== 'all') query.set('workload', workload);
    if (priority !== 'all') query.set('priority', priority);
    if (profile !== 'all') query.set('profile', profile);
    if (targetId === undefined && installation !== 'all') {
      query.set('installation', installation);
    }
    if (repository !== 'all') query.set('repository', repository);
    if (timeRange !== 'all') {
      const age = timeRange === '24h' ? 86_400_000 : 604_800_000;
      query.set('created_after', new Date(Date.now() - age).toISOString());
    }

    return `?${query.toString()}`;
  }

  function selectSection(value: Section): void {
    section = value;
    stateFilter = 'all';
  }

  async function load(
    showLoading = true,
    query = queueQuery(),
    refreshAtStart = refreshRevision,
  ): Promise<void> {
    const generation = ++loadGeneration;
    if (showLoading) loading = true;
    try {
      const page =
        targetId === undefined
          ? await api.fetchRootQueue(query)
          : await api.fetchTargetQueue(targetId, query);
      if (generation !== loadGeneration || refreshAtStart !== refreshRevision) return;
      items = page.items;
      facets = page.facets;
      nextOffset = page.next_offset;
      total = page.total;
      error = '';
    } catch (cause) {
      if (generation !== loadGeneration || refreshAtStart !== refreshRevision) return;
      error = cause instanceof Error ? cause.message : String(cause);
    } finally {
      if (generation === loadGeneration) loading = false;
    }
  }

  function openAction(item: QueueItem, action: QueueActionType): void {
    if (!canControl && targetId !== undefined) return;
    selected = item;
    selectedAction = action;
    actionError = '';
  }

  async function openDetail(item: QueueItem): Promise<void> {
    detailOpen = true;
    detailLoading = true;
    detailError = '';
    detail = null;
    detailRefreshRevision = refreshRevision;
    await loadDetail(item.id, true, refreshRevision);
  }

  async function loadDetail(itemID: string, clear: boolean, refreshAtStart: number): Promise<void> {
    const generation = ++detailLoadGeneration;
    if (clear) detail = null;
    try {
      const refreshed =
        targetId === undefined
          ? await api.fetchRootQueueItem(itemID)
          : await api.fetchTargetQueueItem(targetId, itemID);
      if (generation !== detailLoadGeneration || !detailOpen || refreshAtStart !== refreshRevision)
        return;
      detail = refreshed;
    } catch (cause) {
      if (generation !== detailLoadGeneration || !detailOpen || refreshAtStart !== refreshRevision)
        return;
      detailError = cause instanceof Error ? cause.message : String(cause);
    } finally {
      if (generation === detailLoadGeneration && detailOpen && refreshAtStart === refreshRevision)
        detailLoading = false;
    }
  }

  function closeDetail(): void {
    detailLoadGeneration += 1;
    detailOpen = false;
    detailError = '';
    detailRefreshRevision = -1;
  }

  function closeAction(): void {
    if (actionBusy) return;
    selected = null;
    selectedAction = null;
    actionError = '';
  }

  async function submitAction(input: QueueActionInput): Promise<void> {
    if (selected === null) return;
    actionBusy = true;
    try {
      const updated =
        targetId === undefined
          ? await api.actOnRootQueue(selected.id, input)
          : await api.actOnTargetQueue(targetId, selected.id, input);
      items = items.map((item) => (item.id === updated.id ? updated : item));
      announcement = `${updated.title}: ${updated.state.replaceAll('_', ' ')}`;
      selected = null;
      selectedAction = null;
      actionError = '';
      void load(false);
    } catch (cause) {
      actionError = cause instanceof Error ? cause.message : String(cause);
      if (actionError.toLowerCase().includes('changed')) void load(false);
    } finally {
      actionBusy = false;
    }
  }

  function previewAction(input: QueueActionInput): Promise<QueueSchedulePreview> {
    if (selected === null) return Promise.reject(new Error('Queue item is no longer selected'));
    return targetId === undefined
      ? api.previewRootQueueAction(selected.id, input)
      : api.previewTargetQueueAction(targetId, selected.id, input);
  }
</script>

<section
  class="general-queue"
  aria-labelledby={targetId === undefined ? 'root-page-heading' : 'queue-heading'}
>
  {#if targetId === undefined}
    <RootPageHeader
      role={rootRole}
      title="Queue"
      subtitle="Every durable task, its schedule, and what is blocking it"
    >
      <Button onclick={() => void load()}>
        {#snippet icon()}<Icon name="refresh" size={14} strokeWidth={2} />{/snippet}
        Refresh
      </Button>
    </RootPageHeader>
  {:else}
    <PageHeader
      id="queue-heading"
      title="Queue"
      description="Background work accepted for this installation"
    >
      {#snippet actions()}
        <Button onclick={() => void load()}>
          {#snippet icon()}<Icon name="refresh" size={14} strokeWidth={2} />{/snippet}
          Refresh
        </Button>
      {/snippet}
    </PageHeader>
  {/if}

  <div class="queue-toolbar">
    <div class="queue-tabs">
      <SegmentedControl
        name="queue-view"
        label="Queue views"
        variant="navigation"
        value={section}
        options={[
          { value: 'active', label: 'Active' },
          { value: 'approvals', label: 'Approvals' },
          { value: 'history', label: 'History' },
        ]}
        onSelect={(value) => selectSection(value as Section)}
      />
    </div>
    <div class="queue-tools">
      <span aria-live="polite">
        {loading
          ? 'Updating…'
          : `${total} ${section === 'active' ? 'active' : section} item${total === 1 ? '' : 's'}`}
      </span>
      <TableToolsMenu label="Filter queue" sorts={[]} filters={queueFilters} />
    </div>
  </div>

  <p class="visually-hidden" aria-live="polite">{announcement}</p>
  {#if loading && items.length === 0}
    <Plate label="Loading"><p class="dim" role="status">Reading the durable queue…</p></Plate>
  {:else if error !== ''}
    <Plate label="Queue unavailable" tone="alarm">
      <p>{error}</p>
      <Button onclick={() => void load()}>Try again</Button>
    </Plate>
  {:else}
    <QueueTable
      {items}
      clock={() => now}
      onOpen={(item) => void openDetail(item)}
      onAction={openAction}
    />
    {#if total > 0}
      <nav class="queue-pagination" aria-label="Queue pages">
        <p>Showing {rangeStart}–{rangeEnd} of {total}</p>
        <div>
          <Button
            disabled={offset === 0 || loading}
            onclick={() => (offset = Math.max(0, offset - pageSize))}>Previous</Button
          >
          <Button disabled={nextOffset === 0 || loading} onclick={() => (offset = nextOffset)}
            >Next</Button
          >
        </div>
      </nav>
    {/if}
  {/if}
</section>

{#key `${selected?.id ?? ''}:${selectedAction ?? ''}`}
  <QueueActionDialog
    item={selected}
    action={selectedAction}
    busy={actionBusy}
    error={actionError}
    onClose={closeAction}
    onPreview={previewAction}
    onSubmit={(input) => void submitAction(input)}
  />
{/key}

<QueueDetailDialog
  open={detailOpen}
  {detail}
  loading={detailLoading}
  error={detailError}
  onClose={closeDetail}
/>

<style>
  .general-queue {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-height: 0;
    min-width: 0;
  }
  .queue-toolbar {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    padding-bottom: var(--space-1);
  }
  .queue-tabs {
    display: flex;
    justify-content: flex-start;
  }
  .queue-tools {
    align-items: center;
    color: var(--text-muted);
    display: flex;
    font-size: var(--font-size-compact);
    gap: var(--space-2);
  }
  .queue-tools > span {
    text-box: trim-both cap alphabetic;
  }
  .queue-pagination {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
  }
  .queue-pagination p {
    color: var(--dim);
    font-size: 0.78rem;
    margin: 0;
  }
  .queue-pagination div {
    display: flex;
    gap: var(--space-2);
  }
  @media (max-width: 36rem) {
    .queue-toolbar {
      align-items: stretch;
      flex-direction: column;
    }
    .queue-tabs :global(fieldset) {
      width: 100%;
    }
    .queue-tabs :global(label) {
      flex: 1;
    }
    .queue-tools {
      justify-content: space-between;
    }
  }
</style>
