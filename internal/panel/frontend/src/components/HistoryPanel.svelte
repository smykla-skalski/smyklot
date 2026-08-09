<script lang="ts">
  import { tick } from 'svelte';

  import { formatDateTime, formatRelative, formatTimestamp } from '../lib/format';
  import type { FilterSection } from '../lib/filter-menu';
  import { readTimeDisplay, writeTimeDisplay } from '../lib/preferences';
  import type { TimeDisplay } from '../lib/preferences';
  import type {
    AuditEntry,
    AuditHistoryRequest,
    AuditScope,
    DeliveryFailure,
    FailureHistoryRequest,
    FailureKind,
    HistorySort,
    Page,
  } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import Chip from './Chip.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import HistoryDisplayMenu from './HistoryDisplayMenu.svelte';
  import Icon from './Icon.svelte';
  import PaginationBar from './PaginationBar.svelte';
  import PanelHeader from './PanelHeader.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  type HistoryType = 'audit' | 'failures';

  const HISTORY_TYPES = [
    { value: 'audit', label: 'Audit', tone: 'accent' },
    { value: 'failures', label: 'Failures', tone: 'accent' },
  ] as const;

  const AUDIT_SCOPE_FILTERS = [
    {
      options: [
        { value: 'all', label: 'All changes' },
        { value: 'account', label: 'Account changes' },
        { value: 'repositories', label: 'Repository changes' },
      ],
    },
  ] satisfies readonly FilterSection[];

  const FAILURE_KIND_FILTERS = [
    {
      options: [
        { value: 'all', label: 'All failures' },
        { value: 'permanent', label: 'Permanent' },
        { value: 'retryable', label: 'Retryable' },
      ],
    },
  ] satisfies readonly FilterSection[];

  const {
    targetId,
    refreshVersion,
    fetchAudit,
    fetchFailures,
  }: {
    targetId: string;
    refreshVersion: number;
    fetchAudit: (request: AuditHistoryRequest) => Promise<Page<AuditEntry>>;
    fetchFailures: (request: FailureHistoryRequest) => Promise<Page<DeliveryFailure>>;
  } = $props();

  let historyType = $state<HistoryType>('audit');
  let search = $state('');
  let appliedQuery = $state('');
  let sort = $state<HistorySort>('newest');
  let auditScope = $state<AuditScope>('all');
  let failureKind = $state<FailureKind>('all');
  let timeDisplay = $state<TimeDisplay>(readTimeDisplay());
  let limit = $state<number>(20);
  let auditPage = $state<Page<AuditEntry> | null>(null);
  let failurePage = $state<Page<DeliveryFailure> | null>(null);
  let pageIndex = $state(0);
  let loading = $state(false);
  let problem = $state<string | null>(null);
  let now = $state(Date.now());
  let requestSequence = 0;
  let historyTools: HTMLDivElement;
  let scrollAfterPageSizeChange = false;

  const currentPage = $derived(historyType === 'audit' ? auditPage : failurePage);
  const itemCount = $derived(currentPage?.items.length ?? 0);
  const total = $derived(currentPage?.total ?? 0);
  const pageCount = $derived(Math.max(1, Math.ceil(total / limit)));
  const hasFilters = $derived(
    appliedQuery !== '' || (historyType === 'audit' ? auditScope !== 'all' : failureKind !== 'all'),
  );
  const description = $derived(
    historyType === 'audit'
      ? 'Account and repository configuration changes'
      : 'Webhook deliveries that need investigation',
  );
  const requestKey = $derived(
    [
      targetId,
      refreshVersion,
      historyType,
      appliedQuery,
      sort,
      auditScope,
      failureKind,
      limit,
    ].join(':'),
  );

  $effect(() => {
    const nextQuery = search.trim();
    const timer = window.setTimeout(() => {
      appliedQuery = nextQuery;
    }, 250);
    return () => window.clearTimeout(timer);
  });

  $effect(() => {
    void resetAndLoad(requestKey);
  });

  $effect(() => {
    const tick = window.setInterval(() => {
      now = Date.now();
    }, 30_000);
    return () => window.clearInterval(tick);
  });

  function selectHistoryType(value: string): void {
    if (value === 'audit' || value === 'failures') historyType = value;
  }

  function selectTimeDisplay(value: TimeDisplay): void {
    timeDisplay = value;
    writeTimeDisplay(value);
  }

  function toggleSort(): void {
    sort = sort === 'newest' ? 'oldest' : 'newest';
  }

  function selectAuditScope(values: string[]): void {
    const value = values[0];
    if (value === 'all' || value === 'account' || value === 'repositories') auditScope = value;
  }

  function selectFailureKind(values: string[]): void {
    const value = values[0];
    if (value === 'all' || value === 'permanent' || value === 'retryable') failureKind = value;
  }

  function auditScopeLabel(): string {
    return (
      AUDIT_SCOPE_FILTERS[0]?.options.find((option) => option.value === auditScope)?.label ?? ''
    );
  }

  function failureKindLabel(): string {
    return (
      FAILURE_KIND_FILTERS[0]?.options.find((option) => option.value === failureKind)?.label ?? ''
    );
  }

  function shortDelivery(deliveryId: string): string {
    return deliveryId.length > 12 ? deliveryId.slice(0, 8) : deliveryId;
  }

  function displayTime(value: string): string {
    return timeDisplay === 'relative' ? formatRelative(value, now) : formatDateTime(value);
  }

  function auditSummary(value: string): string {
    return sentenceCase(value.replace(/\s+for\s*$/i, ''));
  }

  function sentenceCase(value: string): string {
    const text = value.trim().replace(/\.+$/, '');
    if (text === '') return text;

    return text.charAt(0).toLocaleUpperCase() + text.slice(1);
  }

  function repositoryName(fullName: string): string {
    const name = fullName.slice(fullName.lastIndexOf('/') + 1);
    return name === '' ? fullName : name;
  }

  async function resetAndLoad(key: string): Promise<void> {
    pageIndex = 0;
    await loadPage(0, key);
  }

  async function loadPage(index: number, key: string): Promise<void> {
    const sequence = ++requestSequence;
    loading = true;
    problem = null;
    const cursor = index === 0 ? undefined : String(index * limit);
    try {
      if (historyType === 'audit') {
        const page = await fetchAudit({
          cursor,
          query: appliedQuery,
          sort,
          limit,
          scope: auditScope,
        });
        if (sequence === requestSequence && key === requestKey) {
          if (index > 0 && page.total <= index * limit) {
            await resetAndLoad(key);
            return;
          }
          auditPage = page;
        }
      } else {
        const page = await fetchFailures({
          cursor,
          query: appliedQuery,
          sort,
          limit,
          kind: failureKind,
        });
        if (sequence === requestSequence && key === requestKey) {
          if (index > 0 && page.total <= index * limit) {
            await resetAndLoad(key);
            return;
          }
          failurePage = page;
        }
      }
    } catch (error) {
      if (sequence === requestSequence && key === requestKey) {
        problem = error instanceof Error ? error.message : String(error);
      }
    } finally {
      if (sequence === requestSequence && key === requestKey) {
        loading = false;
        await scrollToResultsAfterPageSizeChange();
      }
    }
  }

  function selectPageSize(nextLimit: number): void {
    if (nextLimit === limit) return;
    scrollAfterPageSizeChange = true;
    limit = nextLimit;
  }

  async function scrollToResultsAfterPageSizeChange(): Promise<void> {
    if (!scrollAfterPageSizeChange) return;
    scrollAfterPageSizeChange = false;
    await tick();
    historyTools.scrollIntoView({ block: 'start' });
  }

  async function selectPage(nextIndex: number): Promise<void> {
    const bounded = Math.min(pageCount - 1, Math.max(0, nextIndex));
    if (bounded === pageIndex || loading) return;
    pageIndex = bounded;
    await loadPage(bounded, requestKey);
  }

  function retry(): void {
    void loadPage(pageIndex, requestKey);
  }

  function clearFilters(): void {
    search = '';
    appliedQuery = '';
    auditScope = 'all';
    failureKind = 'all';
  }
</script>

{#snippet headerActions()}
  <SegmentedControl
    name="history-type"
    label="History type"
    options={HISTORY_TYPES}
    value={historyType}
    align="end"
    onSelect={selectHistoryType}
  />
{/snippet}

<section
  class="plate history-panel"
  class:absolute-time={timeDisplay === 'absolute'}
  aria-labelledby="history-heading"
>
  <PanelHeader id="history-heading" title="History" {description} actions={headerActions} />

  <div class="history-tools" bind:this={historyTools}>
    <SearchField
      label="Search history"
      placeholder={historyType === 'audit' ? 'Search changes' : 'Search failures'}
      value={search}
      onInput={(value) => (search = value)}
    />

    <div class="filter-field scope-field">
      {#if historyType === 'audit'}
        <FilterMenu
          label="Audit scope"
          summary={auditScopeLabel()}
          hint="Choose which configuration changes to show"
          sections={AUDIT_SCOPE_FILTERS}
          selected={[auditScope]}
          showIcon
          onChange={selectAuditScope}
        />
      {:else}
        <FilterMenu
          label="Failure kind"
          summary={failureKindLabel()}
          hint="Choose which delivery failures to show"
          sections={FAILURE_KIND_FILTERS}
          selected={[failureKind]}
          showIcon
          onChange={selectFailureKind}
        />
      {/if}
    </div>

    <HistoryDisplayMenu value={timeDisplay} onSelect={selectTimeDisplay} />
  </div>

  <div class:loading class="history-results" aria-busy={loading}>
    {#if problem !== null}
      <div class="result-state" role="alert">
        <strong>History could not be loaded</strong>
        <span>{problem}</span>
        <button class="btn" onclick={retry}>Try again</button>
      </div>
    {:else if loading && currentPage === null}
      <div class="table-skeleton" aria-hidden="true">
        {#each [0, 1, 2, 3, 4, 5] as index (index)}
          <span></span>
        {/each}
      </div>
      <p class="visually-hidden" role="status">Loading history</p>
    {:else if historyType === 'audit'}
      <!-- Keyboard focus lets users scroll columns that overflow the viewport. -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div class="table-scroll" role="region" tabindex="0" aria-label="Audit history table">
        <table class="history-table audit-table">
          <caption class="visually-hidden">Audit history</caption>
          <colgroup>
            <col class="actor-column" />
            <col class="change-column" />
            <col class="target-column" />
            <col class="time-column" />
          </colgroup>
          <thead>
            <tr>
              <th scope="col">Actor</th>
              <th scope="col">Change</th>
              <th scope="col">Target</th>
              <th scope="col" aria-sort={sort === 'newest' ? 'descending' : 'ascending'}>
                <button class="sort-button" type="button" onclick={toggleSort}>
                  <span>When</span>
                  <span
                    class:descending={sort === 'newest'}
                    class="sort-indicator"
                    aria-hidden="true"
                  >
                    <Icon name="sort" size={14} />
                  </span>
                </button>
              </th>
            </tr>
          </thead>
          <tbody>
            {#each auditPage?.items ?? [] as entry (entry.id)}
              <tr>
                <td data-label="Actor">
                  <span class="actor">
                    <Avatar account={entry.actor} size={24} />
                    <strong>{entry.actor.display_name}</strong>
                  </span>
                </td>
                <td data-label="Change">
                  <span class="cell-primary">{auditSummary(entry.summary)}</span>
                  <span class="cell-meta mono">{entry.action}</span>
                </td>
                <td data-label="Target">
                  {#if entry.repository_full_name !== undefined}
                    <code title={entry.repository_full_name}>
                      {repositoryName(entry.repository_full_name)}
                    </code>
                  {:else}
                    <span class="dim">Account</span>
                  {/if}
                </td>
                <td data-label="When">
                  <time
                    class="table-time"
                    datetime={entry.created_at}
                    title={formatTimestamp(entry.created_at)}
                  >
                    {displayTime(entry.created_at)}
                  </time>
                </td>
              </tr>
            {:else}
              <tr>
                <td class="empty-cell dim" colspan="4">
                  <strong>No configuration changes found</strong>
                  {#if hasFilters}
                    <button class="btn" type="button" onclick={clearFilters}>Clear filters</button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <!-- Keyboard focus lets users scroll columns that overflow the viewport. -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div
        class="table-scroll"
        role="region"
        tabindex="0"
        aria-label="Delivery failure history table"
      >
        <table class="history-table failure-table">
          <caption class="visually-hidden">Delivery failure history</caption>
          <colgroup>
            <col class="status-column" />
            <col class="repository-column" />
            <col class="failure-column" />
            <col class="time-column" />
          </colgroup>
          <thead>
            <tr>
              <th scope="col">Status</th>
              <th scope="col">Repository</th>
              <th scope="col">Failure</th>
              <th scope="col" aria-sort={sort === 'newest' ? 'descending' : 'ascending'}>
                <button class="sort-button" type="button" onclick={toggleSort}>
                  <span>When</span>
                  <span
                    class:descending={sort === 'newest'}
                    class="sort-indicator"
                    aria-hidden="true"
                  >
                    <Icon name="sort" size={14} />
                  </span>
                </button>
              </th>
            </tr>
          </thead>
          <tbody>
            {#each failurePage?.items ?? [] as failure (failure.id)}
              <tr class="failure-row">
                <td data-label="Status">
                  <Chip tone={failure.retryable ? 'warning' : 'stop'} dot>
                    {failure.retryable ? 'Retryable' : 'Failed'}
                  </Chip>
                </td>
                <td data-label="Repository">
                  <code title={failure.repository_full_name}>
                    {repositoryName(failure.repository_full_name)}
                  </code>
                </td>
                <td data-label="Failure">
                  <span class="cell-primary">{sentenceCase(failure.reason)}</span>
                  <span class="cell-meta mono">
                    {failure.event} · {failure.stage} ·
                    <span title={`Delivery ${failure.delivery_id}`}>
                      delivery {shortDelivery(failure.delivery_id)}
                    </span>
                  </span>
                </td>
                <td data-label="When">
                  <time
                    class="table-time"
                    datetime={failure.occurred_at}
                    title={formatTimestamp(failure.occurred_at)}
                  >
                    {displayTime(failure.occurred_at)}
                  </time>
                </td>
              </tr>
            {:else}
              <tr>
                <td class="empty-cell dim" colspan="4">
                  <strong>No delivery failures found</strong>
                  {#if hasFilters}
                    <button class="btn" type="button" onclick={clearFilters}>Clear filters</button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>

  {#if problem === null && !(loading && currentPage === null)}
    <PaginationBar
      label="History rows"
      {pageIndex}
      {pageCount}
      pageSize={limit}
      {itemCount}
      {total}
      disabled={loading}
      onPageSelect={(nextIndex) => void selectPage(nextIndex)}
      onPageSizeSelect={selectPageSize}
    />
  {/if}
</section>

<style>
  .history-panel {
    --local-control-height: var(--control-height-compact);

    background: transparent;
    border: 0;
    border-radius: 0;
    box-shadow: none;
    margin-bottom: 0;
    overflow: visible;
  }

  .history-tools {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: 0;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: minmax(12rem, 1fr) max-content auto;
    padding: 0 0 var(--space-3);
  }

  .filter-field {
    min-width: 0;
  }

  .filter-field {
    display: block;
  }

  .scope-field {
    width: 9.75rem;
  }

  .scope-field :global(.filter-menu),
  .scope-field :global(summary) {
    width: 100%;
  }

  .history-results {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-bottom: 0;
    border-radius: var(--radius-surface) var(--radius-surface) 0 0;
    min-height: 5rem;
    overflow: hidden;
    transition: opacity 120ms ease-out;
  }

  .history-results.loading {
    opacity: 0.55;
  }

  .table-scroll {
    max-width: 100%;
    overflow-x: auto;
  }

  .history-table {
    border-collapse: collapse;
    font-size: 0.75rem;
    min-width: 40rem;
    table-layout: fixed;
    width: 100%;
  }

  .history-table th,
  .history-table td {
    padding: 0.625rem 0.75rem;
    text-align: left;
    vertical-align: middle;
  }

  .history-table th:last-child,
  .history-table td:last-child {
    text-align: right;
  }

  .history-table th {
    background: var(--table-header-bg);
    color: var(--dim);
    font: 650 var(--font-size-compact) / 1.2 var(--sans);
    letter-spacing: 0.02em;
  }

  .history-table tbody tr {
    border-top: 1px solid var(--rule);
    transition: background-color 100ms ease-out;
  }

  .history-table tbody tr:hover {
    background: var(--table-row-hover);
  }

  .history-table th:has(.sort-button) {
    padding: 0;
  }

  .sort-button {
    align-items: center;
    background: transparent;
    border: 0;
    color: inherit;
    display: flex;
    font: inherit;
    gap: var(--space-2);
    justify-content: flex-end;
    letter-spacing: inherit;
    padding: 0.625rem 0.75rem;
    text-align: right;
    text-transform: inherit;
    width: 100%;
  }

  .sort-button:hover,
  .sort-button:focus-visible {
    background: var(--interactive-hover);
    color: var(--text-primary);
  }

  .sort-indicator {
    color: var(--brand-action-text);
    display: grid;
    place-items: center;
  }

  .sort-indicator.descending {
    transform: rotate(180deg);
  }

  .history-table code {
    display: inline-block;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    vertical-align: middle;
    white-space: nowrap;
  }

  .actor-column {
    width: 8.5rem;
  }

  .target-column,
  .repository-column {
    width: 11rem;
  }

  .status-column {
    width: 7rem;
  }

  .time-column {
    width: 7.5rem;
  }

  .absolute-time .time-column {
    width: 9.5rem;
  }

  .actor {
    align-items: center;
    display: flex;
    gap: 0.5rem;
    min-width: 0;
  }

  .actor strong {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cell-primary,
  .cell-meta {
    display: block;
    overflow-wrap: anywhere;
  }

  .cell-primary {
    line-height: 1.35;
  }

  .cell-meta {
    color: var(--dim);
    font-size: 0.5625rem;
    line-height: 1.3;
    margin-top: 0.15rem;
  }

  .table-time {
    align-items: center;
    color: var(--dim);
    display: inline-flex;
    font-family: var(--mono);
    font-size: 0.625rem;
    height: var(--control-height-compact);
    line-height: 1;
    transform: translateY(-1px);
    vertical-align: middle;
    white-space: nowrap;
  }

  .empty-cell {
    height: 9rem;
    text-align: center !important;
  }

  .empty-cell strong {
    color: var(--text);
    display: block;
    margin-bottom: var(--space-2);
  }

  .table-skeleton {
    display: grid;
  }

  .table-skeleton span {
    animation: history-skeleton-pulse 1.35s ease-in-out infinite alternate;
    border-bottom: 1px solid var(--rule);
    display: block;
    height: 3.25rem;
    position: relative;
  }

  .table-skeleton span::before,
  .table-skeleton span::after {
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    content: '';
    height: 0.75rem;
    left: var(--space-4);
    position: absolute;
    top: 1rem;
    width: min(12rem, 26%);
  }

  .table-skeleton span::after {
    left: 46%;
    width: min(16rem, 32%);
  }

  @keyframes history-skeleton-pulse {
    from {
      opacity: 0.48;
    }

    to {
      opacity: 0.88;
    }
  }

  .result-state {
    align-items: center;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    justify-content: center;
    min-height: 9rem;
    padding: 1.5rem;
    text-align: center;
  }

  .result-state span {
    color: var(--dim);
    font-size: 0.75rem;
  }

  .history-panel :global(.pagination-bar) {
    border: 1px solid var(--border-subtle);
    border-radius: 0 0 var(--radius-surface) var(--radius-surface);
  }

  @media (max-width: 48rem) {
    .table-scroll {
      overflow: visible;
      padding: var(--space-3);
    }

    .history-table {
      display: block;
      min-width: 0;
    }

    .history-table colgroup {
      clip-path: inset(50%);
      height: 1px;
      overflow: hidden;
      position: absolute;
      white-space: nowrap;
      width: 1px;
    }

    .history-table thead {
      display: block;
    }

    .history-table thead tr {
      border: 0;
      display: flex;
      justify-content: flex-end;
      padding: 0 0 var(--space-3);
    }

    .history-table thead th {
      padding: 0;
    }

    .history-table thead th:not(:has(.sort-button)) {
      clip-path: inset(50%);
      height: 1px;
      overflow: hidden;
      position: absolute;
      white-space: nowrap;
      width: 1px;
    }

    .history-table thead .sort-button {
      background: var(--control-bg);
      border: 1px solid var(--control-border);
      border-radius: var(--radius-control);
      color: var(--dim);
      height: var(--control-height-compact);
      padding: 0 var(--space-3);
    }

    .history-table thead .sort-button:hover,
    .history-table thead .sort-button:focus-visible {
      background: var(--control-bg-hover);
      color: var(--text);
    }

    .history-table tbody {
      display: grid;
      gap: var(--space-2);
    }

    .history-table tbody tr {
      background: var(--surface-raised);
      border: 1px solid var(--border-subtle);
      border-radius: var(--radius-control);
      display: grid;
      gap: var(--space-3);
      grid-template-columns: repeat(2, minmax(0, 1fr));
      padding: var(--space-3);
    }

    .history-table td {
      display: grid;
      gap: var(--space-1);
      padding: 0;
      text-align: left !important;
    }

    .history-table td::before {
      color: var(--text-muted);
      content: attr(data-label);
      font: 650 var(--font-size-compact) / 1 var(--sans);
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }

    .history-table .empty-cell {
      display: block;
      grid-column: 1 / -1;
      height: auto;
      padding: var(--space-5);
    }

    .history-table .empty-cell::before {
      display: none;
    }
  }

  @media (max-width: 36rem) {
    .history-tools {
      grid-template-columns: 1fr 1fr;
    }

    .history-tools :global(.search-field) {
      grid-column: 1 / -1;
    }

    .scope-field {
      width: 100%;
    }
  }

  @media (max-width: 22rem) {
    .history-tools {
      grid-template-columns: 1fr;
    }

    .history-tools :global(.search-field) {
      grid-column: auto;
    }

    .history-table tbody tr {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
