<script lang="ts">
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import type { PanelApi } from '../api';
  import { formatDateTime } from '../format';
  import { fuzzyCandidates } from '../fuzzy';
  import { monogram } from '../identity';
  import type { HistorySection, RootRoute, RootInstallationView } from '../routes';
  import type { RootInstallation } from '../types';
  import Chip from './Chip.svelte';
  import Icon from './Icon.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import SearchField from './SearchField.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  const {
    route,
    api,
    rootRole,
    actorLogin,
    listHref,
    hrefFor,
    onList,
    onNavigate,
    historySection,
    onHistorySection,
  }: {
    route: RootRoute;
    api: PanelApi;
    rootRole: string;
    actorLogin: string;
    listHref: string;
    hrefFor: (account: string, view: RootInstallationView) => string;
    onList: () => void;
    onNavigate: (account: string, view: RootInstallationView) => void;
    historySection: HistorySection;
    onHistorySection: (section: HistorySection) => void;
  } = $props();

  const installationsQuery = createQuery(() => ({
    queryKey: ['root-installations'],
    queryFn: () => api.fetchRootInstallations(),
  }));
  const syncMutation = createMutation(() => ({
    mutationFn: () => api.syncRootInstallations(),
    onSuccess: async () => {
      await installationsQuery.refetch();
    },
  }));
  const installations = $derived<RootInstallation[]>(installationsQuery.data ?? []);
  const loading = $derived(installationsQuery.isFetching);
  const failure = $derived(
    installationsQuery.error === null
      ? null
      : installationsQuery.error instanceof Error
        ? installationsQuery.error.message
        : String(installationsQuery.error),
  );
  let query = $state('');
  let syncProblem = $state<string | null>(null);
  let syncFeedback = $state('');
  const syncing = $derived(syncMutation.isPending);

  const selected = $derived(
    route.rootView === 'installation'
      ? (installations.find(
          (installation) =>
            installation.account.login.toLocaleLowerCase() === route.account.toLocaleLowerCase(),
        ) ?? null)
      : null,
  );
  const visibleInstallations = $derived(
    fuzzyCandidates(
      installations.map((installation) => ({
        id: installation.id,
        label: installation.account.display_name,
        keywords: [installation.account.login, installation.installation_id],
        installation,
      })),
      query,
    ).map((candidate) => candidate.installation),
  );

  async function syncCatalog(): Promise<void> {
    if (syncing) return;
    syncProblem = null;
    syncFeedback = '';
    try {
      const targetIDs = await syncMutation.mutateAsync();
      syncFeedback = `Synchronized ${targetIDs.length} installation${targetIDs.length === 1 ? '' : 's'}`;
    } catch (error) {
      syncProblem = error instanceof Error ? error.message : String(error);
    }
  }

  function navigate(
    event: MouseEvent,
    installation: RootInstallation,
    view: RootInstallationView,
  ): void {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey) return;
    event.preventDefault();
    onNavigate(installation.account.login, view);
  }

  function clickRow(event: MouseEvent, installation: RootInstallation): void {
    const target = event.target instanceof Element ? event.target : null;
    if (target?.closest('button, a, summary, input') !== null) return;
    navigate(event, installation, 'settings');
  }

  function keyRow(event: KeyboardEvent, installation: RootInstallation): void {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    const target = event.target instanceof Element ? event.target : null;
    if (target?.closest('button, a, summary, input') !== null) return;
    event.preventDefault();
    onNavigate(installation.account.login, 'settings');
  }

  function deliveryTitle(installation: RootInstallation): string | undefined {
    const latest = installation.delivery_health.last_failure_at;
    return latest === undefined ? undefined : `Latest failure ${formatDateTime(latest)}`;
  }

  function ownershipLabel(installation: RootInstallation): string {
    if (installation.ownership.status === 'permission_pending') return 'Approval needed';
    if (installation.ownership.status === 'error') return 'Sync failed';
    if (installation.ownership.stale) return 'Stale';
    return 'Fresh';
  }

  function ownershipTone(installation: RootInstallation): 'clear' | 'neutral' | 'warning' | 'stop' {
    if (installation.ownership.status === 'error') return 'stop';
    if (installation.ownership.status === 'permission_pending') return 'warning';
    // Stale is drift, not danger: a quiet state until a sync runs.
    if (installation.ownership.stale) return 'neutral';
    return 'clear';
  }
</script>

{#if route.rootView === 'installation' && selected !== null}
  {#await import('./RootInstallationView.svelte')}
    <p class="installation-loading" role="status">Loading installation…</p>
  {:then { default: RootInstallationView }}
    {#key selected.id}
      <RootInstallationView
        installation={selected}
        view={route.view}
        {api}
        {actorLogin}
        {listHref}
        {hrefFor}
        {onList}
        {onNavigate}
        {historySection}
        {onHistorySection}
      />
    {/key}
  {:catch error}
    <TableEmptyState
      title="Installation view could not be loaded"
      description={error instanceof Error ? error.message : String(error)}
      actionLabel="Reload panel"
      onAction={() => window.location.reload()}
    />
  {/await}
{:else if route.rootView === 'installation' && !loading}
  <TableEmptyState
    title="Installation not found"
    description="This installation is no longer present in the Root catalog"
    actionLabel="Sync now"
    onAction={syncCatalog}
  />
{:else}
  <section class="root-installations" aria-labelledby="root-page-heading">
    <RootPageHeader
      role={rootRole}
      title="Installations"
      subtitle="Live ownership and delivery health for every GitHub installation connected to Smyklot"
    >
      {#if syncProblem !== null || syncFeedback !== ''}
        <span class="slot-note" class:problem={syncProblem !== null}>
          {syncProblem ?? syncFeedback}
        </span>
      {/if}
      <button class="btn" type="button" disabled={syncing} onclick={() => void syncCatalog()}>
        <Icon name="refresh" size={14} />
        <span class="cap-trim">{syncing ? 'Synchronizing…' : 'Sync now'}</span>
      </button>
    </RootPageHeader>

    <div class="installation-tools">
      <SearchField
        label="Search installations"
        placeholder="Search installations"
        value={query}
        onInput={(value) => (query = value)}
      />
    </div>

    <!-- Keyboard focus lets users scroll columns that overflow the viewport. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <div
      class="installation-table-shell table-card"
      role="region"
      tabindex="0"
      aria-label="Installation catalog table"
    >
      <table>
        <caption class="visually-hidden">Installation catalog</caption>
        <thead>
          <tr>
            <th scope="col"><span class="cap-trim">Installation</span></th>
            <th scope="col" class="count-heading"><span class="cap-trim">Repositories</span></th>
            <th scope="col"><span class="cap-trim">Delivery</span></th>
            <th scope="col"><span class="cap-trim">Ownership</span></th>
            <th scope="col"><span class="cap-trim">Owners</span></th>
          </tr>
        </thead>
        <tbody data-panel-scroll>
          {#each visibleInstallations as installation (installation.id)}
            <tr
              class="installation-row"
              tabindex="0"
              onclick={(event) => clickRow(event, installation)}
              onkeydown={(event) => keyRow(event, installation)}
            >
              <th scope="row">
                <span class="installation-identity">
                  <span class="installation-icon">
                    <span class="cap-trim">
                      {monogram(installation.account.display_name, installation.account.login)}
                    </span>
                  </span>
                  <span class="band-trim-stack">
                    <a
                      class="installation-link"
                      href={hrefFor(installation.account.login, 'settings')}
                      onclick={(event) => navigate(event, installation, 'settings')}
                    >
                      {installation.account.display_name}
                    </a>
                    <small>@{installation.account.login} · #{installation.installation_id}</small>
                  </span>
                </span>
              </th>
              <td class="count-cell">
                {#if installation.repository_counts.total === 0}
                  <span class="cell-dash band-trim" aria-label="No repositories">—</span>
                {:else}
                  <span class="repo-count band-trim">
                    <b>{installation.repository_counts.enabled}</b>
                    of {installation.repository_counts.enabled +
                      installation.repository_counts.disabled} enabled
                  </span>
                {/if}
              </td>
              <td>
                <span title={deliveryTitle(installation)}>
                  <Chip tone={installation.delivery_health.failed === 0 ? 'clear' : 'stop'} dot>
                    {installation.delivery_health.failed === 0
                      ? 'Healthy'
                      : `${installation.delivery_health.failed} failure${installation.delivery_health.failed === 1 ? '' : 's'}`}
                  </Chip>
                </span>
              </td>
              <td>
                <!-- The reason rides on the chip rather than a second line: the
                     mock keeps every catalog row to one chip high, and the same
                     text is spelled out on the overview's ownership card. -->
                <span class="chip-stack" title={installation.ownership.detail}>
                  <Chip tone={ownershipTone(installation)} dot>
                    {ownershipLabel(installation)}
                  </Chip>
                </span>
              </td>
              <td>
                {#if installation.ownership.owner_count === 0}
                  <span class="cell-dash band-trim" aria-label="No owners">—</span>
                {:else}
                  <span class="owners-line band-trim">
                    {installation.ownership.owner_count} ·
                    {installation.ownership.source === 'personal' ? 'Account owner' : 'Org admins'}
                  </span>
                {/if}
              </td>
            </tr>
          {:else}
            <tr class="state-row">
              <td colspan="5" class="empty-cell">
                {#if loading && installations.length === 0}
                  Loading installation catalog…
                {:else if failure !== null}
                  <span role="alert">{failure}</span>
                {:else}
                  <TableEmptyState
                    title="No installations match"
                    description={`Nothing in the catalog matches “${query.trim()}”`}
                    actionLabel="Clear search"
                    onAction={() => (query = '')}
                  />
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </section>
{/if}

<style>
  .root-installations {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 0;
    min-height: 0;
    min-width: 0;
  }

  .installation-loading {
    color: var(--dim);
    margin: 0;
  }

  /* The same controls row every other table view has: one 34px line with the
     shared space-3 under it, so the table starts the same distance below the
     search here as it does on Repositories and Access. Without the wrapper the
     field sat directly on the table's top rule. */
  .installation-tools {
    align-items: center;
    display: grid;
    flex: none;
    gap: var(--space-2);
    grid-template-columns: minmax(16rem, 1fr);
    padding: 0 0 var(--space-3);
  }

  /* SearchField declares flex: 1 1 15rem for row layouts; in this column the
     basis would become height, so pin it to its natural control height — the
     shared 34px toolbar height every other view's controls row uses. */
  .installation-tools :global(.search-field) {
    --local-control-height: var(--control-height-compact);

    flex: none;
  }

  .slot-note {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    white-space: nowrap;
  }

  .slot-note.problem {
    color: var(--stop);
  }

  /* Surface, keyline, corner and lift come from `.table-card` in `app.css` -
     this was the only table drawing them, and the other five were bare. */
  .installation-table-shell {
    overflow-y: auto;
  }

  /* `separate`, not `collapse`: a collapsed border is shared between adjacent
     rows, so each cell ends up owning half of it and every row box lands on a
     .5 - the header measured 40.5 against the approved table's 41, and every
     row 59.5 inside a 60px row. Separated borders keep each box whole. */
  table {
    border-collapse: separate;
    border-spacing: 0;
    min-width: 52rem;
    width: 100%;
  }

  /* The header's rule and its type come from `thead th` in `app.css`. A
     `font-size` on `th` here would outrank it - a class selector beats two
     element ones - and this table's heading would be the only 13px one. */
  /* `tbody th` as well as `td`: the identity cell is a row header, and without
     the separator it is a pixel taller than the cells beside it. */
  td,
  tbody th {
    border-bottom: 1px solid var(--rule);
    font-size: var(--font-size-meta);
  }

  th,
  td {
    padding: var(--space-2) var(--space-3);
    text-align: left;
    vertical-align: middle;
  }

  th:first-child,
  td:first-child {
    padding-left: var(--space-3);
  }

  th:last-child,
  td:last-child {
    padding-right: var(--space-3);
  }

  /* text-box only trims a block box, and a cell's label is inline by default. */
  thead th .cap-trim {
    display: block;
  }

  /* 2.5rem of band plus its own rule. NOT via box-sizing: content-box - the
     sticky-header layout gives thead and tbody rows the same percentage column
     widths, and under content-box the header's percentages stop including its
     24px of padding, so the two grids drift apart by a whole cell. */
  thead th {
    height: calc(2.5rem + 1px);
    padding-block: 0;
  }

  tbody th {
    font-weight: inherit;
  }

  .installation-row {
    cursor: pointer;
    height: 3.75rem;
    transition: background-color var(--duration-fast) var(--ease-standard);
  }

  .installation-row:hover {
    background: var(--table-row-hover);
  }

  .installation-row:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: -2px;
  }

  /* Left, like every other column. The mock reads this cell as a sentence
     ("10 of 28 enabled"), not as a figure to scan down, so right-aligning it
     put the header and the value on two different edges. */
  .count-heading,
  .count-cell {
    text-align: left;
  }

  /* Table body copy is one size across every column: the count read a step
     smaller than the name beside it, so the row had two baselines' worth of
     type in it. */
  .repo-count {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    white-space: nowrap;
  }

  .repo-count b {
    color: var(--text-primary);
  }

  /* Mono, like every other handle and id in the product: the login and the
     installation number are values to read character by character, and in sans
     the pair measured a fifth narrower than the approved row. */
  .installation-identity small {
    color: var(--text-muted);
    font: 400 var(--font-size-compact) / 1.2 var(--mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .chip-stack {
    display: inline-grid;
    gap: 0.25rem;
    justify-items: start;
  }

  /* The tooltip wrapper is inline by default, and its line box would ride
     2px below the cell center. */
  td > span[title] {
    display: inline-flex;
  }

  .owners-line {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
  }

  .cell-dash {
    color: var(--text-muted);
    opacity: 0.6;
  }

  .installation-identity {
    align-items: center;
    display: inline-flex;
    gap: var(--space-2);
  }

  .installation-identity small {
    display: block;
    margin-top: 0.2rem;
  }

  /* The account name is the row's headline and carries body size, not the
     meta size the rest of the cell uses. */
  .installation-link {
    color: var(--text-primary);
    display: block;
    font: 700 var(--font-size-body) / 1.25 var(--sans);
    text-decoration: none;
  }

  .installation-link:hover {
    text-decoration: underline;
  }

  /* The shell tokens carry the violet in Root context, so no literals here. */
  .installation-icon {
    align-items: center;
    background: var(--brand-action-tint);
    border-radius: var(--radius-control);
    /* Self-keyed keyline, same recipe as the detail-header mark. */
    box-shadow: inset 0 0 0 1px color-mix(in srgb, currentcolor 28%, transparent);
    color: var(--brand-action-text);
    display: inline-flex;
    flex: none;
    font-size: 0.75rem;
    font-weight: 700;
    height: 2.25rem;
    justify-content: center;
    width: 2.25rem;
  }

  .empty-cell {
    color: var(--text-secondary);
    height: 12rem;
    text-align: center;
  }

  .state-row:hover {
    background: transparent;
  }

  /* Pinned table mode, matching the other table views: the view fills the
     workspace, the column header stays put, and only the rows scroll. */
  @media (min-width: 64.001rem) {
    .installation-table-shell,
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

    /* Fixed row-tables need explicit widths so thead and tbody columns line up.
       These are the approved catalog's 2fr 1.2fr 1.1fr 1.5fr 1fr, written as
       percentages of the 6.8fr total so they hold at any table width. */
    th:nth-child(1),
    td:nth-child(1) {
      width: 29.412%;
    }

    th:nth-child(2),
    td:nth-child(2) {
      width: 17.647%;
    }

    th:nth-child(3),
    td:nth-child(3) {
      width: 16.176%;
    }

    th:nth-child(4),
    td:nth-child(4) {
      width: 22.059%;
    }

    th:nth-child(5),
    td:nth-child(5) {
      width: 14.706%;
    }
  }
</style>
