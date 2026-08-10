<script lang="ts">
  import { formatRelative, formatTimestamp } from '../lib/format';
  import type { FilterSection } from '../lib/filter-menu';
  import type {
    Page,
    PanelUserStatus,
    RootPanelUser,
    RootPanelUserPageRequest,
    RootPanelUserSort,
    SystemRole,
  } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import Icon from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  type AccessSection = 'users' | 'invitations';
  type SortColumn = 'name' | 'role' | 'last_login';

  const SECTIONS = [
    { value: 'users', label: 'Users', tone: 'accent' },
    { value: 'invitations', label: 'Invitations', tone: 'accent' },
  ] as const;
  const ROLE_FILTERS = [
    {
      options: [
        { value: 'super_root', label: 'Super Root' },
        { value: 'root', label: 'Root' },
        { value: 'none', label: 'Regular account' },
      ],
    },
  ] satisfies readonly FilterSection[];
  const STATUS_FILTERS = [
    {
      options: [
        { value: 'active', label: 'Active', tone: 'valid' },
        { value: 'banned', label: 'Banned', tone: 'invalid' },
        { value: 'removed', label: 'Removed', tone: 'bypassed' },
      ],
    },
  ] satisfies readonly FilterSection[];

  const {
    section,
    onSection,
    fetchUsers,
  }: {
    section: AccessSection;
    onSection: (section: AccessSection) => void;
    fetchUsers: (request: RootPanelUserPageRequest) => Promise<Page<RootPanelUser>>;
  } = $props();

  let page = $state<Page<RootPanelUser> | null>(null);
  let search = $state('');
  let query = $state('');
  let sort = $state<RootPanelUserSort>('name_asc');
  let systemRoles = $state<SystemRole[]>([]);
  let statuses = $state<PanelUserStatus[]>([]);
  let loading = $state(false);
  let problem = $state<string | null>(null);
  let loadMoreProblem = $state<string | null>(null);
  let sequence = 0;
  const limit = 20;
  const now = Date.now();
  const requestKey = $derived(JSON.stringify([query, sort, systemRoles, statuses, limit]));
  const users = $derived(page?.items ?? []);
  const hasFilters = $derived(query !== '' || systemRoles.length > 0 || statuses.length > 0);

  $effect(() => {
    const next = search.trim();
    const timeout = window.setTimeout(() => (query = next), 180);
    return () => window.clearTimeout(timeout);
  });

  $effect(() => {
    if (section === 'users') void loadPage(undefined, false, requestKey);
  });

  function selectSection(value: string): void {
    if (value === 'users' || value === 'invitations') onSection(value);
  }

  function toggleSort(column: SortColumn): void {
    const pairs: Record<SortColumn, readonly [RootPanelUserSort, RootPanelUserSort]> = {
      name: ['name_asc', 'name_desc'],
      role: ['role_asc', 'role_desc'],
      last_login: ['login_oldest', 'login_newest'],
    };
    const [ascending, descending] = pairs[column];
    sort = sort === ascending ? descending : ascending;
  }

  function sortDirection(column: SortColumn): 'ascending' | 'descending' | undefined {
    const prefix = column === 'last_login' ? 'login_' : `${column}_`;
    if (!sort.startsWith(prefix)) return undefined;
    return sort.endsWith('desc') || sort === 'login_newest' ? 'descending' : 'ascending';
  }

  function selectRoles(values: string[]): void {
    systemRoles = values.filter((value): value is SystemRole =>
      ['none', 'root', 'super_root'].includes(value),
    );
  }

  function selectStatuses(values: string[]): void {
    statuses = values.filter((value): value is PanelUserStatus =>
      ['active', 'banned', 'removed'].includes(value),
    );
  }

  async function loadPage(
    cursor: string | undefined,
    append: boolean,
    key = requestKey,
  ): Promise<void> {
    if (key !== requestKey || loading) return;
    const version = ++sequence;
    loading = true;
    if (append) loadMoreProblem = null;
    else problem = null;
    try {
      const loaded = await fetchUsers({
        ...(cursor === undefined ? {} : { cursor }),
        query,
        sort,
        limit,
        systemRoles,
        statuses,
      });
      if (version !== sequence || key !== requestKey) return;
      page =
        append && page !== null ? { ...loaded, items: [...page.items, ...loaded.items] } : loaded;
    } catch (error) {
      if (version !== sequence || key !== requestKey) return;
      const message = error instanceof Error ? error.message : String(error);
      if (append) loadMoreProblem = message;
      else problem = message;
    } finally {
      if (version === sequence) loading = false;
    }
  }

  function loadNext(): void {
    const cursor = page?.next_cursor;
    if (cursor !== null && cursor !== undefined) void loadPage(cursor, true);
  }

  function loadFromScroll(event: Event): void {
    const target = event.currentTarget as HTMLElement;
    if (target.scrollHeight - target.scrollTop - target.clientHeight < 260) loadNext();
  }

  function clearFilters(): void {
    search = '';
    query = '';
    systemRoles = [];
    statuses = [];
  }

  function systemRoleLabel(role: SystemRole): string {
    if (role === 'super_root') return 'Super Root';
    if (role === 'root') return 'Root';
    return 'Account';
  }

  function systemRoleTone(role: SystemRole): ChipTone {
    if (role === 'super_root') return 'accent';
    if (role === 'root') return 'signal';
    return 'neutral';
  }

  function statusLabel(status: PanelUserStatus): string {
    return status.charAt(0).toLocaleUpperCase() + status.slice(1);
  }

  function statusTone(status: PanelUserStatus): ChipTone {
    if (status === 'active') return 'clear';
    if (status === 'banned') return 'stop';
    return 'neutral';
  }

  function installationSummary(user: RootPanelUser): string {
    const relationships = user.owned_installations + user.assigned_installations;
    return `${relationships} installation${relationships === 1 ? '' : 's'}`;
  }
</script>

<section class="root-access" aria-labelledby="root-page-heading">
  <div class="access-navigation">
    <SegmentedControl
      name="root-access-section"
      label="Root access lists"
      options={SECTIONS}
      value={section}
      variant="navigation"
      onSelect={selectSection}
    />
    <p>{section === 'users' ? 'Every account known to Smyklot' : 'Pending system-level access'}</p>
  </div>

  {#if section === 'invitations'}
    <div class="invitation-foundation">
      <TableEmptyState
        title="No Root invitations"
        description="System-role invitations will appear here after the guarded invitation workflow is enabled"
      />
    </div>
  {:else}
    <div class="access-tools">
      <SearchField
        label="Search Root users"
        placeholder="Search users"
        value={search}
        onInput={(value) => (search = value)}
      />
    </div>

    <div class:loading class="user-results" aria-busy={loading}>
      {#if problem !== null}
        <div class="result-state" role="alert">
          <strong>Root users could not be loaded</strong>
          <span>{problem}</span>
          <button class="btn" type="button" onclick={() => void loadPage(undefined, false)}>
            Try again
          </button>
        </div>
      {:else if loading && page === null}
        <div class="table-skeleton" aria-hidden="true">
          {#each [0, 1, 2, 3, 4, 5] as index (index)}<span></span>{/each}
        </div>
      {:else}
        <!-- Keyboard focus lets users scroll columns that overflow the viewport. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <div class="table-scroll" role="region" tabindex="0" aria-label="Root users table">
          <table>
            <caption class="visually-hidden">Application accounts</caption>
            <thead>
              <tr>
                <th scope="col" aria-sort={sortDirection('name')}>
                  <button
                    class="table-sort-button"
                    type="button"
                    onclick={() => toggleSort('name')}
                  >
                    <span>User</span><Icon name="sort" size={14} />
                  </button>
                </th>
                <th scope="col" aria-sort={sortDirection('role')}>
                  <div class="heading-layout">
                    <button
                      class="table-sort-button"
                      type="button"
                      onclick={() => toggleSort('role')}
                    >
                      <span>System role</span><Icon name="sort" size={14} />
                    </button>
                    <FilterMenu
                      label="System role"
                      summary={systemRoles.length === 0
                        ? 'All system roles'
                        : `${systemRoles.length} selected`}
                      hint="Filter application-level privileges"
                      sections={ROLE_FILTERS}
                      selected={systemRoles}
                      multiple
                      align="end"
                      showIcon
                      iconOnly
                      placement="header"
                      onChange={selectRoles}
                    />
                  </div>
                </th>
                <th scope="col">
                  <div class="heading-layout">
                    <span class="heading-label">Status</span>
                    <FilterMenu
                      label="Status"
                      summary={statuses.length === 0
                        ? 'All statuses'
                        : `${statuses.length} selected`}
                      hint="Filter account lifecycle state"
                      sections={STATUS_FILTERS}
                      selected={statuses}
                      multiple
                      align="end"
                      showIcon
                      iconOnly
                      placement="header"
                      onChange={selectStatuses}
                    />
                  </div>
                </th>
                <th scope="col">Installations</th>
                <th scope="col" aria-sort={sortDirection('last_login')}>
                  <button
                    class="table-sort-button"
                    type="button"
                    onclick={() => toggleSort('last_login')}
                  >
                    <span>Last login</span><Icon name="sort" size={14} />
                  </button>
                </th>
              </tr>
            </thead>
            <tbody data-panel-scroll onscroll={loadFromScroll}>
              {#each users as user (user.account.id)}
                <tr>
                  <td data-label="User">
                    <span class="identity">
                      <Avatar account={user.account} size={32} />
                      <span
                        ><strong>{user.account.display_name}</strong><span class="mono"
                          >@{user.account.login}</span
                        ></span
                      >
                    </span>
                  </td>
                  <td data-label="System role">
                    <Chip tone={systemRoleTone(user.system_role)}
                      >{systemRoleLabel(user.system_role)}</Chip
                    >
                  </td>
                  <td data-label="Status">
                    <Chip tone={statusTone(user.status)} dot={user.status === 'active'}
                      >{statusLabel(user.status)}</Chip
                    >
                  </td>
                  <td data-label="Installations">
                    <span class="relationship-count">{installationSummary(user)}</span>
                    <span class="relationship-meta"
                      >{user.owned_installations} owned · {user.assigned_installations} assigned</span
                    >
                  </td>
                  <td data-label="Last login">
                    {#if user.last_login_at !== undefined}
                      <time
                        datetime={user.last_login_at}
                        title={formatTimestamp(user.last_login_at)}
                        >{formatRelative(user.last_login_at, now)}</time
                      >
                    {:else}<span class="dim">Never</span>{/if}
                  </td>
                </tr>
              {:else}
                <tr class="empty-row">
                  <td colspan="5">
                    <TableEmptyState
                      title="No accounts found"
                      description={hasFilters
                        ? 'Try another search or clear the active filters'
                        : 'Accounts appear after their first authenticated session'}
                      actionLabel={hasFilters ? 'Clear filters' : undefined}
                      onAction={hasFilters ? clearFilters : undefined}
                    />
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
      <InfiniteLoadSentinel
        active={!loading && page?.next_cursor != null}
        cursor={page?.next_cursor}
        onVisible={loadNext}
      />
      {#if loadMoreProblem !== null}
        <div class="load-more-alert" role="alert">
          <span>{loadMoreProblem}</span><button class="btn" type="button" onclick={loadNext}
            >Try again</button
          >
        </div>
      {/if}
    </div>
  {/if}
</section>

<style>
  .root-access {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }

  .access-navigation {
    align-items: center;
    display: flex;
    justify-content: space-between;
    padding-bottom: var(--space-3);
  }

  .access-navigation p {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    margin: 0;
  }

  .access-tools {
    padding-bottom: var(--space-3);
  }

  .user-results,
  .invitation-foundation {
    background: var(--table-filler-bg);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 8rem;
    overflow: hidden;
    position: relative;
  }

  .invitation-foundation {
    align-items: center;
    justify-content: center;
    padding: var(--space-8);
  }

  .table-scroll {
    background: var(--surface-base);
    flex: 1;
    max-width: 100%;
    min-height: 0;
    overflow-x: auto;
  }

  table {
    background: var(--surface-base);
    border-collapse: collapse;
    min-width: 46rem;
    table-layout: fixed;
    width: 100%;
  }

  th,
  td {
    border-bottom: 1px solid var(--rule);
    padding: 0.625rem 0.75rem;
    text-align: left;
    vertical-align: middle;
  }

  th {
    background: var(--table-header-bg);
    color: var(--dim);
    font: 650 var(--font-size-compact) / 1.2 var(--sans);
    letter-spacing: 0.02em;
  }

  th[aria-sort='ascending'],
  th[aria-sort='descending'] {
    background: var(--table-sorted-bg);
  }

  th:has(.table-sort-button) {
    padding: 0;
  }

  th:first-child,
  td:first-child {
    width: 28%;
  }

  th:nth-child(2),
  td:nth-child(2) {
    width: 16%;
  }

  th:nth-child(3),
  td:nth-child(3) {
    width: 13%;
  }

  th:nth-child(4),
  td:nth-child(4) {
    width: 24%;
  }

  th:last-child,
  td:last-child {
    text-align: right;
    width: 19%;
  }

  .table-sort-button,
  .heading-layout {
    align-items: center;
    display: flex;
    height: 100%;
  }

  .table-sort-button {
    background: transparent;
    border: 0;
    color: inherit;
    font: inherit;
    gap: var(--space-2);
    justify-content: flex-start;
    padding: 0.625rem 0.75rem;
    width: 100%;
  }

  .heading-layout {
    justify-content: space-between;
  }

  .heading-layout .table-sort-button {
    flex: 1;
    min-width: 0;
    width: auto;
  }

  .heading-layout :global(.header-filter) {
    margin-inline: var(--space-1);
  }

  .heading-label {
    padding-left: 0.75rem;
  }

  .identity {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    min-width: 0;
  }

  .identity > span:last-child {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .identity strong,
  .identity .mono {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .identity strong {
    font-size: var(--font-size-body);
    line-height: 1.2;
  }

  .identity .mono,
  .relationship-meta,
  time {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    line-height: 1.25;
  }

  .relationship-count,
  .relationship-meta {
    display: block;
  }

  .relationship-count {
    font-size: var(--font-size-body);
  }

  .relationship-meta {
    margin-top: 0.15rem;
  }

  time {
    white-space: nowrap;
  }

  .empty-row td {
    height: 10rem;
  }

  .empty-row td :global(.table-empty-state) {
    margin-inline: auto;
  }

  .result-state,
  .table-skeleton {
    min-height: 10rem;
  }

  .result-state {
    align-items: center;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    justify-content: center;
    padding: var(--space-6);
    text-align: center;
  }

  .result-state span {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
  }

  .table-skeleton {
    display: grid;
  }

  .table-skeleton span {
    animation: root-access-pulse 1.35s ease-in-out infinite alternate;
    border-bottom: 1px solid var(--rule);
    height: 3.25rem;
  }

  .load-more-alert {
    align-items: center;
    background: var(--popover-bg);
    border: 1px solid var(--popover-border);
    border-radius: var(--radius-control);
    bottom: var(--space-3);
    box-shadow: var(--shadow-popover);
    display: flex;
    gap: var(--space-3);
    left: 50%;
    padding: var(--space-2) var(--space-3);
    position: absolute;
    transform: translateX(-50%);
  }

  @keyframes root-access-pulse {
    from {
      opacity: 0.48;
    }
    to {
      opacity: 0.88;
    }
  }

  @media (min-width: 64.001rem) {
    .table-scroll,
    table {
      display: flex;
      flex: 1;
      flex-direction: column;
      min-height: 0;
    }

    thead {
      display: block;
      flex: none;
    }

    tbody {
      background: var(--table-filler-bg);
      display: block;
      flex: 1;
      min-height: 0;
      overflow-y: auto;
      overscroll-behavior: contain;
    }

    thead tr,
    tbody tr {
      display: table;
      table-layout: fixed;
      width: 100%;
    }

    tbody tr {
      background: var(--surface-base);
    }

    tbody tr:last-child td {
      border-bottom: 0;
    }
  }

  @media (max-width: 64rem) {
    .access-navigation {
      align-items: start;
      flex-direction: column;
      gap: var(--space-2);
    }

    table {
      min-width: 0;
    }

    thead {
      display: none;
    }

    tbody,
    tr,
    td {
      display: block;
      width: 100% !important;
    }

    tbody tr {
      border-bottom: 1px solid var(--rule);
      padding: var(--space-3);
    }

    td {
      align-items: center;
      border: 0;
      display: grid;
      gap: var(--space-3);
      grid-template-columns: 7rem minmax(0, 1fr);
      padding: var(--space-2) 0;
      text-align: left !important;
    }

    td::before {
      color: var(--text-muted);
      content: attr(data-label);
      font: 650 var(--font-size-compact) / 1.2 var(--sans);
    }

    .empty-row td {
      display: flex;
      height: 12rem;
      justify-content: center;
    }

    .empty-row td::before {
      content: none;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .table-skeleton span {
      animation: none;
    }
  }
</style>
