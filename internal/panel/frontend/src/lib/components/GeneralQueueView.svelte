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
  import Select from './Select.svelte';

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
  const pageSize = 50;

  const workloads = $derived(facets.workloads);
  const profiles = $derived(facets.profiles);
  const installations = $derived(facets.targets);
  const repositories = $derived(facets.repositories);
  const states = $derived(facets.states.filter((value) => sectionStates(section).includes(value)));
  const rangeStart = $derived(total === 0 ? 0 : offset + 1);
  const rangeEnd = $derived(Math.min(offset + items.length, total));
  const query = $derived.by(queueQuery);

  onMount(() => {
    const refresh = window.setInterval(() => void load(false), 30_000);
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
    if (clear) detail = null;
    try {
      const refreshed =
        targetId === undefined
          ? await api.fetchRootQueueItem(itemID)
          : await api.fetchTargetQueueItem(targetId, itemID);
      if (!detailOpen || refreshAtStart !== refreshRevision) return;
      detail = refreshed;
    } catch (cause) {
      if (!detailOpen || refreshAtStart !== refreshRevision) return;
      detailError = cause instanceof Error ? cause.message : String(cause);
    } finally {
      if (detailOpen && refreshAtStart === refreshRevision) detailLoading = false;
    }
  }

  function closeDetail(): void {
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

  <div class="queue-controls">
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
    <div class="queue-filters" aria-label="Queue filters">
      <label>
        <span>Workload</span>
        <Select bind:value={workload} onchange={() => (offset = 0)}>
          <option value="all">All workloads</option>
          {#each workloads as kind (kind)}
            <option value={kind}>{kind.replaceAll('_', ' ')}</option>
          {/each}
        </Select>
      </label>
      <label>
        <span>State</span>
        <Select bind:value={stateFilter} onchange={() => (offset = 0)}>
          <option value="all">All states</option>
          {#each states as value (value)}
            <option {value}>{value.replaceAll('_', ' ')}</option>
          {/each}
        </Select>
      </label>
      <label>
        <span>Window</span>
        <Select bind:value={profile} onchange={() => (offset = 0)}>
          <option value="all">All windows</option>
          {#each profiles as value (value)}<option {value}>{value}</option>{/each}
        </Select>
      </label>
      {#if targetId === undefined}
        <label>
          <span>Installation</span>
          <Select bind:value={installation} onchange={() => (offset = 0)}>
            <option value="all">All installations</option>
            {#each installations as value (value)}<option {value}>{value}</option>{/each}
          </Select>
        </label>
      {/if}
      <label>
        <span>Repository</span>
        <Select bind:value={repository} onchange={() => (offset = 0)}>
          <option value="all">All repositories</option>
          {#each repositories as value (value)}<option {value}>{value}</option>{/each}
        </Select>
      </label>
      <label>
        <span>Created</span>
        <Select bind:value={timeRange} onchange={() => (offset = 0)}>
          <option value="all">Any time</option>
          <option value="24h">Last 24 hours</option>
          <option value="7d">Last 7 days</option>
        </Select>
      </label>
      <label>
        <span>Priority</span>
        <Select bind:value={priority} onchange={() => (offset = 0)}>
          <option value="all">All priorities</option>
          <option value="urgent">Urgent</option>
          <option value="high">High</option>
          <option value="normal">Normal</option>
          <option value="low">Low</option>
        </Select>
      </label>
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
  .queue-controls {
    display: grid;
    gap: var(--space-3);
    padding-bottom: var(--space-1);
  }
  .queue-tabs {
    display: flex;
    justify-content: flex-start;
  }
  .queue-filters {
    align-items: end;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: repeat(auto-fit, minmax(8.5rem, 1fr));
  }
  .queue-filters label {
    display: grid;
    gap: var(--space-1);
    min-width: 0;
  }
  .queue-filters label > span {
    color: var(--text-muted);
    font-size: var(--font-size-micro);
    font-weight: 650;
    letter-spacing: 0.06em;
    text-transform: uppercase;
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
    .queue-filters {
      grid-template-columns: 1fr;
    }
    .queue-tabs :global(fieldset) {
      width: 100%;
    }
    .queue-tabs :global(label) {
      flex: 1;
    }
  }
</style>
