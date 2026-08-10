<script lang="ts">
  import type { PanelApi } from '../lib/api';
  import { formatDateTime } from '../lib/format';
  import { fuzzyCandidates } from '../lib/fuzzy';
  import type { RootRoute, ScopedPanelView } from '../lib/routes';
  import type { RootInstallation } from '../lib/types';
  import ActionMenu, { type ActionMenuItem } from './ActionMenu.svelte';
  import Chip from './Chip.svelte';
  import Icon from './Icon.svelte';
  import RootInstallationView from './RootInstallationView.svelte';
  import SearchField from './SearchField.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  const {
    route,
    api,
    refreshVersion,
    listHref,
    hrefFor,
    onList,
    onNavigate,
  }: {
    route: RootRoute;
    api: PanelApi;
    refreshVersion: number;
    listHref: string;
    hrefFor: (account: string, view: ScopedPanelView) => string;
    onList: () => void;
    onNavigate: (account: string, view: ScopedPanelView) => void;
  } = $props();

  let installations = $state<RootInstallation[]>([]);
  let loading = $state(true);
  let failure = $state<string | null>(null);
  let query = $state('');
  let syncing = $state(false);
  let syncProblem = $state<string | null>(null);
  let syncFeedback = $state('');
  let sequence = 0;

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

  async function load(version = refreshVersion): Promise<void> {
    const current = ++sequence;
    loading = true;
    failure = null;
    try {
      const loaded = await api.fetchRootInstallations();
      if (current !== sequence || version !== refreshVersion) return;
      installations = loaded;
    } catch (error) {
      if (current !== sequence || version !== refreshVersion) return;
      failure = error instanceof Error ? error.message : String(error);
    } finally {
      if (current === sequence) loading = false;
    }
  }

  async function syncCatalog(): Promise<void> {
    if (syncing) return;
    syncing = true;
    syncProblem = null;
    syncFeedback = '';
    try {
      const targetIDs = await api.syncRootInstallations();
      syncFeedback = `Synchronized ${targetIDs.length} installation${targetIDs.length === 1 ? '' : 's'}`;
    } catch (error) {
      syncProblem = error instanceof Error ? error.message : String(error);
    } finally {
      syncing = false;
    }
  }

  const INSTALLATION_VIEWS: readonly ActionMenuItem[] = [
    { id: 'settings', icon: 'settings', label: 'Settings' },
    { id: 'repositories', icon: 'repositories', label: 'Repositories' },
    { id: 'users', icon: 'users', label: 'Access' },
    { id: 'history', icon: 'history', label: 'History' },
  ];

  function navigate(
    event: MouseEvent,
    installation: RootInstallation,
    view: ScopedPanelView,
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

  function chooseView(installation: RootInstallation, view: string): void {
    if (view === 'settings' || view === 'repositories' || view === 'users' || view === 'history') {
      onNavigate(installation.account.login, view);
    }
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

  $effect(() => {
    void load(refreshVersion);
  });
</script>

{#if route.rootView === 'installation' && selected !== null}
  {#key selected.id}
    <RootInstallationView
      installation={selected}
      view={route.view}
      {api}
      {refreshVersion}
      {listHref}
      {hrefFor}
      {onList}
      {onNavigate}
    />
  {/key}
{:else if route.rootView === 'installation' && !loading}
  <TableEmptyState
    title="Installation not found"
    description="This installation is no longer present in the Root catalog"
    actionLabel="Sync now"
    onAction={syncCatalog}
  />
{:else}
  <section class="root-installations" aria-labelledby="root-installations-heading">
    <div class="installation-controls">
      <div>
        <h3 id="root-installations-heading">Installation catalog</h3>
        <p>Live ownership and delivery health across the application</p>
      </div>
      <div class="sync-controls">
        <span class="sync-feedback" class:problem={syncProblem !== null} aria-live="polite">
          {syncProblem ?? syncFeedback}
        </span>
        <button class="btn" type="button" disabled={syncing} onclick={() => void syncCatalog()}>
          <Icon name="refresh" size={17} />
          {syncing ? 'Synchronizing…' : 'Sync now'}
        </button>
      </div>
    </div>

    <SearchField
      label="Search installations"
      placeholder="Search installations"
      value={query}
      onInput={(value) => (query = value)}
    />

    <!-- Keyboard focus lets users scroll columns that overflow the viewport. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <div
      class="installation-table-shell"
      role="region"
      tabindex="0"
      aria-label="Installation catalog table"
    >
      <table>
        <caption class="visually-hidden">Installation catalog</caption>
        <thead>
          <tr>
            <th scope="col">Installation</th>
            <th scope="col" class="count-heading">Repositories</th>
            <th scope="col">Delivery</th>
            <th scope="col">Ownership</th>
            <th scope="col">Owners</th>
            <th scope="col"><span class="visually-hidden">Installation views</span></th>
          </tr>
        </thead>
        <tbody>
          {#each visibleInstallations as installation (installation.id)}
            <tr
              class="installation-row"
              class:unavailable={!installation.available}
              tabindex="0"
              onclick={(event) => clickRow(event, installation)}
              onkeydown={(event) => keyRow(event, installation)}
            >
              <th scope="row">
                <span class="installation-identity">
                  <span class="installation-icon">
                    <Icon
                      name={installation.type === 'Organization' ? 'organization' : 'user'}
                      size={18}
                    />
                  </span>
                  <span>
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
                  <span class="cell-dash" aria-label="No repositories">—</span>
                {:else}
                  <span class="count-stack">
                    <strong>{installation.repository_counts.total}</strong>
                    <small>
                      {installation.repository_counts.enabled} on ·
                      {installation.repository_counts.disabled} off
                    </small>
                  </span>
                {/if}
              </td>
              <td>
                <span title={deliveryTitle(installation)}>
                  <Chip
                    tone={installation.delivery_health.failed === 0 ? 'clear' : 'stop'}
                    small
                    dot
                  >
                    {installation.delivery_health.failed === 0
                      ? 'Healthy'
                      : `${installation.delivery_health.failed} failure${installation.delivery_health.failed === 1 ? '' : 's'}`}
                  </Chip>
                </span>
              </td>
              <td>
                <span class="chip-stack">
                  <Chip tone={ownershipTone(installation)} small dot>
                    {ownershipLabel(installation)}
                  </Chip>
                  {#if installation.ownership.detail !== undefined}
                    <small>{installation.ownership.detail}</small>
                  {/if}
                </span>
              </td>
              <td>
                <span class="owners-line">
                  {installation.ownership.owner_count} ·
                  {installation.ownership.source === 'personal' ? 'Account owner' : 'Org admins'}
                </span>
              </td>
              <td class="row-actions">
                <span class="row-go" aria-hidden="true">
                  <Icon name="chevron-right" size={14} />
                </span>
                <ActionMenu
                  label={`Views for ${installation.account.display_name}`}
                  items={INSTALLATION_VIEWS}
                  onSelect={(view) => chooseView(installation, view)}
                />
              </td>
            </tr>
          {:else}
            <tr class="state-row">
              <td colspan="6" class="empty-cell">
                {#if loading}
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
    display: grid;
    gap: var(--space-4);
    min-width: 0;
  }

  .installation-controls {
    align-items: end;
    display: flex;
    gap: var(--space-4);
    justify-content: space-between;
  }

  .sync-controls {
    align-items: center;
    display: flex;
    gap: var(--space-3);
  }

  .sync-feedback {
    color: var(--admin);
    font-size: var(--font-size-compact);
  }

  .sync-feedback.problem {
    color: var(--stop);
  }

  h3,
  p {
    margin: 0;
  }

  h3 {
    font-size: 1rem;
    letter-spacing: -0.015em;
  }

  .installation-controls p {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    margin-top: var(--space-1);
  }

  .installation-table-shell {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    overflow: auto;
  }

  table {
    border-collapse: collapse;
    min-width: 52rem;
    width: 100%;
  }

  th,
  td {
    border-bottom: 1px solid var(--rule);
    padding: var(--space-2) var(--space-3);
    text-align: left;
    vertical-align: middle;
  }

  th:first-child,
  td:first-child {
    padding-left: var(--space-4);
  }

  th:last-child,
  td:last-child {
    padding-right: var(--space-4);
  }

  thead th {
    background: var(--table-header-bg);
    color: var(--dim);
    font: 650 var(--font-size-compact) / 1.2 var(--sans);
    letter-spacing: 0.02em;
    padding-block: var(--space-3);
  }

  tbody th {
    font-weight: inherit;
  }

  tbody tr:last-child > * {
    border-bottom: 0;
  }

  .installation-row {
    cursor: pointer;
    transition: background-color var(--duration-fast) var(--ease-standard);
  }

  .installation-row:hover {
    background: var(--table-row-hover);
  }

  .installation-row:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: -2px;
  }

  tbody tr.unavailable {
    background: color-mix(in srgb, var(--warning) 3%, var(--surface-base));
  }

  .count-heading,
  .count-cell {
    text-align: right;
  }

  .count-stack {
    display: inline-grid;
    gap: 2px;
    justify-items: end;
  }

  .count-stack small,
  .chip-stack small,
  .installation-identity small {
    color: var(--text-secondary);
    font-size: var(--font-size-micro);
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
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
  }

  .cell-dash {
    color: var(--text-muted);
    opacity: 0.6;
  }

  .row-actions {
    text-align: right;
    white-space: nowrap;
    width: 4.5rem;
  }

  .row-actions :global(.action-menu) {
    display: inline-block;
    vertical-align: middle;
  }

  .row-go {
    color: var(--text-muted);
    display: inline-grid;
    opacity: 0;
    place-items: center;
    transition:
      opacity var(--duration-fast) var(--ease-standard),
      transform var(--duration-fast) var(--ease-standard);
    vertical-align: middle;
  }

  .installation-row:hover .row-go,
  .installation-row:focus-visible .row-go {
    opacity: 1;
    transform: translateX(2px);
  }

  .installation-identity {
    align-items: center;
    display: inline-flex;
    gap: var(--space-3);
  }

  .installation-identity small {
    display: block;
    margin-top: 1px;
  }

  .installation-link {
    color: var(--text-primary);
    display: block;
    font-weight: 650;
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
    color: var(--brand-action-text);
    display: inline-flex;
    flex: none;
    height: 2.25rem;
    justify-content: center;
    width: 2.25rem;
  }

  .empty-cell {
    color: var(--text-secondary);
    height: 12rem;
    text-align: center;
  }

  .empty-cell :global(.table-empty-state) {
    margin-inline: auto;
  }

  .state-row:hover {
    background: transparent;
  }

  @media (max-width: 42rem) {
    .installation-controls {
      align-items: stretch;
      flex-direction: column;
    }

    .installation-controls .btn {
      align-self: start;
    }

    .sync-controls {
      align-items: start;
      flex-direction: column;
    }
  }
</style>
