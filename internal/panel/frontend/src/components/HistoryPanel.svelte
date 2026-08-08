<script lang="ts">
  import { formatDateTime, formatRelative, formatTimestamp } from '../lib/format';
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
  import HistoryDisplayMenu from './HistoryDisplayMenu.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  type HistoryType = 'audit' | 'failures';

  const PAGE_SIZES = [10, 20, 50] as const;
  const HISTORY_TYPES = [
    { value: 'audit', label: 'Audit' },
    { value: 'failures', label: 'Failures' },
  ] as const;

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
  let cursors = $state<Array<string | undefined>>([undefined]);
  let pageIndex = $state(0);
  let loading = $state(false);
  let problem = $state<string | null>(null);
  let now = $state(Date.now());
  let requestSequence = 0;

  const currentPage = $derived(historyType === 'audit' ? auditPage : failurePage);
  const itemCount = $derived(currentPage?.items.length ?? 0);
  const total = $derived(currentPage?.total ?? 0);
  const rangeStart = $derived(total === 0 ? 0 : pageIndex * limit + 1);
  const rangeEnd = $derived(total === 0 ? 0 : rangeStart + itemCount - 1);
  const pageCount = $derived(Math.max(1, Math.ceil(total / limit)));
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
    cursors = [undefined];
    pageIndex = 0;
    await loadPage(undefined, key);
  }

  async function loadPage(cursor: string | undefined, key: string): Promise<void> {
    const sequence = ++requestSequence;
    loading = true;
    problem = null;
    try {
      if (historyType === 'audit') {
        const page = await fetchAudit({
          cursor,
          query: appliedQuery,
          sort,
          limit,
          scope: auditScope,
        });
        if (sequence === requestSequence && key === requestKey) auditPage = page;
      } else {
        const page = await fetchFailures({
          cursor,
          query: appliedQuery,
          sort,
          limit,
          kind: failureKind,
        });
        if (sequence === requestSequence && key === requestKey) failurePage = page;
      }
    } catch (error) {
      if (sequence === requestSequence && key === requestKey) {
        problem = error instanceof Error ? error.message : String(error);
      }
    } finally {
      if (sequence === requestSequence && key === requestKey) loading = false;
    }
  }

  async function nextPage(): Promise<void> {
    const cursor = currentPage?.next_cursor;
    if (cursor === null || cursor === undefined || loading) return;
    const nextIndex = pageIndex + 1;
    cursors = [...cursors.slice(0, nextIndex), cursor];
    pageIndex = nextIndex;
    await loadPage(cursor, requestKey);
  }

  async function previousPage(): Promise<void> {
    if (pageIndex === 0 || loading) return;
    const previousIndex = pageIndex - 1;
    pageIndex = previousIndex;
    await loadPage(cursors[previousIndex], requestKey);
  }

  function retry(): void {
    void loadPage(cursors[pageIndex], requestKey);
  }
</script>

<section
  class="plate history-panel"
  class:absolute-time={timeDisplay === 'absolute'}
  aria-labelledby="activity-heading"
>
  <header class="history-header">
    <div class="history-heading">
      <h2 id="activity-heading">Activity</h2>
      <p>{description}</p>
    </div>
    <SegmentedControl
      name="history-type"
      label="History type"
      options={HISTORY_TYPES}
      value={historyType}
      align="end"
      onSelect={selectHistoryType}
    />
  </header>

  <div class="history-tools">
    <label class="search-field">
      <span class="visually-hidden">Search history</span>
      <span class="search-icon" aria-hidden="true"></span>
      <input
        class="text-input"
        type="search"
        placeholder={historyType === 'audit' ? 'Search changes' : 'Search failures'}
        bind:value={search}
      />
    </label>

    <label class="filter-field scope-field">
      {#if historyType === 'audit'}
        <select class="select-input" bind:value={auditScope} aria-label="Audit scope">
          <option value="all">All changes</option>
          <option value="account">Account changes</option>
          <option value="repositories">Repository changes</option>
        </select>
      {:else}
        <select class="select-input" bind:value={failureKind} aria-label="Failure kind">
          <option value="all">All failures</option>
          <option value="permanent">Permanent</option>
          <option value="retryable">Retryable</option>
        </select>
      {/if}
    </label>

    <label class="filter-field order-field">
      <select class="select-input" bind:value={sort} aria-label="History sort order">
        <option value="newest">Newest first</option>
        <option value="oldest">Oldest first</option>
      </select>
    </label>

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
      <div class="result-state dim">Loading activity…</div>
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
              <th scope="col">When</th>
            </tr>
          </thead>
          <tbody>
            {#each auditPage?.items ?? [] as entry (entry.id)}
              <tr>
                <td>
                  <span class="actor">
                    <Avatar account={entry.actor} size={24} />
                    <strong>{entry.actor.display_name}</strong>
                  </span>
                </td>
                <td>
                  <span class="cell-primary">{auditSummary(entry.summary)}</span>
                  <span class="cell-meta mono">{entry.action}</span>
                </td>
                <td>
                  {#if entry.repository_full_name !== undefined}
                    <code title={entry.repository_full_name}>
                      {repositoryName(entry.repository_full_name)}
                    </code>
                  {:else}
                    <span class="dim">Account</span>
                  {/if}
                </td>
                <td>
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
                  No configuration changes match these filters
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
              <th scope="col">When</th>
            </tr>
          </thead>
          <tbody>
            {#each failurePage?.items ?? [] as failure (failure.id)}
              <tr class="failure-row">
                <td>
                  <Chip tone={failure.retryable ? 'warning' : 'stop'} dot>
                    {failure.retryable ? 'Retryable' : 'Failed'}
                  </Chip>
                </td>
                <td>
                  <code title={failure.repository_full_name}>
                    {repositoryName(failure.repository_full_name)}
                  </code>
                </td>
                <td>
                  <span class="cell-primary">{sentenceCase(failure.reason)}</span>
                  <span class="cell-meta mono">
                    {failure.event} · {failure.stage} ·
                    <span title={`Delivery ${failure.delivery_id}`}>
                      delivery {shortDelivery(failure.delivery_id)}
                    </span>
                  </span>
                </td>
                <td>
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
                  No delivery failures match these filters
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>

  {#if problem === null && !(loading && currentPage === null)}
    <footer class="pagination" aria-label="History pagination">
      <p class="range mono">
        <strong>{rangeStart}–{rangeEnd}</strong>
        of {total}
      </p>

      <div class="page-actions">
        <button
          class="btn page-button"
          disabled={pageIndex === 0 || loading}
          onclick={previousPage}
        >
          <span aria-hidden="true">←</span>
          Previous
        </button>
        <span class="page-number mono">Page {pageIndex + 1} of {pageCount}</span>
        <button
          class="btn page-button"
          disabled={currentPage?.next_cursor === null || loading}
          onclick={nextPage}
        >
          Next
          <span aria-hidden="true">→</span>
        </button>
      </div>

      <label class="rows-field">
        <span>Rows</span>
        <select class="select-input" bind:value={limit} aria-label="Rows per page">
          {#each PAGE_SIZES as size (size)}
            <option value={size}>{size}</option>
          {/each}
        </select>
      </label>
    </footer>
  {/if}
</section>

<style>
  .history-panel {
    --history-control-height: 30px;

    overflow: hidden;
  }

  .history-header {
    align-items: center;
    border-bottom: 1px solid var(--rule);
    display: flex;
    gap: 1rem;
    justify-content: space-between;
    min-height: 4rem;
    padding: 0.625rem 1.125rem;
  }

  .history-heading {
    min-width: 0;
  }

  .history-heading h2 {
    font-size: 0.9375rem;
    line-height: 1.2;
    margin: 0;
  }

  .history-heading p {
    color: var(--dim);
    font-size: 0.75rem;
    line-height: 1.35;
    margin: 0.15rem 0 0;
  }

  .history-tools {
    align-items: center;
    background: var(--well);
    border-bottom: 1px solid var(--rule);
    display: grid;
    gap: 0.5rem;
    grid-template-columns: minmax(12rem, 1fr) max-content max-content auto;
    padding: 0.625rem 1.125rem;
  }

  .search-field,
  .filter-field {
    min-width: 0;
  }

  .search-field {
    align-items: center;
    display: flex;
    position: relative;
  }

  .search-field input {
    font-size: 0.6875rem;
    height: var(--history-control-height);
    padding-left: 2rem;
    width: 100%;
  }

  .search-icon {
    border: 1.5px solid var(--dim);
    border-radius: 50%;
    height: 0.65rem;
    left: 0.75rem;
    opacity: 0.8;
    pointer-events: none;
    position: absolute;
    width: 0.65rem;
  }

  .search-icon::after {
    background: var(--dim);
    content: '';
    height: 1.5px;
    position: absolute;
    right: -0.3rem;
    top: 0.5rem;
    transform: rotate(45deg);
    width: 0.35rem;
  }

  .filter-field {
    display: block;
  }

  .filter-field select {
    font-size: 0.6875rem;
    height: var(--history-control-height);
    min-width: 0;
    width: 100%;
  }

  .scope-field {
    width: 9.75rem;
  }

  .order-field {
    width: 7.5rem;
  }

  .history-results {
    min-height: 5rem;
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
    background: var(--well);
    color: var(--dim);
    font: 600 0.5625rem/1 var(--mono);
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .history-table tbody tr {
    border-top: 1px solid var(--rule);
    transition: background-color 100ms ease-out;
  }

  .history-table tbody tr:hover {
    background: var(--strip-lift);
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
    color: var(--dim);
    font-family: var(--mono);
    font-size: 0.625rem;
    white-space: nowrap;
  }

  .empty-cell {
    height: 9rem;
    text-align: center !important;
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

  .pagination {
    align-items: center;
    background: var(--well);
    border-top: 1px solid var(--rule);
    display: grid;
    gap: 0.75rem;
    grid-template-columns: 1fr auto 1fr;
    min-height: 3.75rem;
    padding: 0.625rem 1.125rem;
  }

  .range,
  .page-number {
    color: var(--dim);
    font-size: 0.625rem;
    margin: 0;
    white-space: nowrap;
  }

  .range strong {
    color: var(--text);
    font-weight: 600;
  }

  .page-actions {
    align-items: center;
    display: flex;
    gap: 0.5rem;
  }

  .page-button {
    background: var(--strip-lift);
    font-size: 0.75rem;
    padding: 0 0.625rem;
  }

  .rows-field {
    align-items: center;
    display: flex;
    gap: 0.5rem;
    justify-self: end;
  }

  .rows-field > span {
    color: var(--dim);
    font: 600 0.5625rem/1 var(--mono);
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .rows-field select {
    font-size: 0.75rem;
    width: 4.25rem;
  }

  @media (max-width: 36rem) {
    .history-header {
      align-items: stretch;
      flex-direction: column;
    }

    .history-tools {
      grid-template-columns: 1fr 1fr auto;
    }

    .search-field {
      grid-column: 1 / -1;
    }

    .scope-field,
    .order-field {
      width: 100%;
    }

    .pagination {
      grid-template-columns: 1fr auto;
    }

    .page-actions {
      grid-column: 1 / -1;
      grid-row: 1;
      justify-content: space-between;
    }

    .range {
      grid-column: 1;
      grid-row: 2;
    }

    .rows-field {
      grid-column: 2;
      grid-row: 2;
    }
  }

  @media (max-width: 26rem) {
    .history-tools {
      grid-template-columns: 1fr;
    }

    .search-field {
      grid-column: auto;
    }
  }
</style>
