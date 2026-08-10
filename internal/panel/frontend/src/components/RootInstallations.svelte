<script lang="ts">
  import type { PanelApi } from '../lib/api';
  import { fuzzyCandidates } from '../lib/fuzzy';
  import type { RootRoute, ScopedPanelView } from '../lib/routes';
  import type { RootInstallation } from '../lib/types';
  import Icon from './Icon.svelte';
  import RootInstallationView from './RootInstallationView.svelte';
  import SearchField from './SearchField.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  const {
    route,
    api,
    listHref,
    hrefFor,
    onList,
    onNavigate,
  }: {
    route: RootRoute;
    api: PanelApi;
    listHref: string;
    hrefFor: (account: string, view: ScopedPanelView) => string;
    onList: () => void;
    onNavigate: (account: string, view: ScopedPanelView) => void;
  } = $props();

  let installations = $state<RootInstallation[]>([]);
  let loading = $state(true);
  let failure = $state<string | null>(null);
  let query = $state('');

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

  async function load(): Promise<void> {
    loading = true;
    failure = null;
    try {
      installations = await api.fetchRootInstallations();
    } catch (error) {
      failure = error instanceof Error ? error.message : String(error);
    } finally {
      loading = false;
    }
  }

  function navigate(event: MouseEvent, installation: RootInstallation): void {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey) return;
    event.preventDefault();
    onNavigate(installation.account.login, 'settings');
  }

  function ownershipLabel(installation: RootInstallation): string {
    if (installation.ownership.status === 'permission_pending') return 'Approval needed';
    if (installation.ownership.status === 'error') return 'Sync failed';
    if (installation.ownership.stale) return 'Stale';
    return 'Fresh';
  }

  function ownershipTone(installation: RootInstallation): 'fresh' | 'warning' | 'error' {
    if (installation.ownership.status === 'error') return 'error';
    if (installation.ownership.stale || installation.ownership.status === 'permission_pending')
      return 'warning';
    return 'fresh';
  }

  $effect(() => {
    void load();
  });
</script>

{#if route.rootView === 'installation' && selected !== null}
  {#key selected.id}
    <RootInstallationView
      installation={selected}
      view={route.view}
      {api}
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
    actionLabel="Reload catalog"
    onAction={load}
  />
{:else}
  <section class="root-installations" aria-labelledby="root-installations-heading">
    <div class="installation-controls">
      <div>
        <h3 id="root-installations-heading">Installation catalog</h3>
        <p>Ownership health and delivery scope across the application</p>
      </div>
      <button class="btn" type="button" disabled={loading} onclick={load}>
        <Icon name="refresh" size={17} />
        Refresh
      </button>
    </div>

    <SearchField
      label="Search installations"
      placeholder="Search installations"
      value={query}
      onInput={(value) => (query = value)}
    />

    <div class="installation-table-shell">
      <table>
        <thead>
          <tr>
            <th scope="col">Installation</th>
            <th scope="col">Repositories</th>
            <th scope="col">Owners</th>
            <th scope="col">Ownership sync</th>
            <th scope="col"><span class="visually-hidden">Open</span></th>
          </tr>
        </thead>
        <tbody>
          {#each visibleInstallations as installation (installation.id)}
            <tr class:unavailable={!installation.available}>
              <th scope="row">
                <span class="installation-identity">
                  <span class="installation-icon">
                    <Icon
                      name={installation.type === 'Organization' ? 'organization' : 'user'}
                      size={18}
                    />
                  </span>
                  <span>
                    <strong>{installation.account.display_name}</strong>
                    <small>@{installation.account.login} · #{installation.installation_id}</small>
                  </span>
                </span>
              </th>
              <td>
                <strong>{installation.repository_counts.total}</strong>
                <small>
                  {installation.repository_counts.enabled} enabled ·
                  {installation.repository_counts.disabled} disabled
                </small>
              </td>
              <td>
                <strong>{installation.ownership.owner_count}</strong>
                <small
                  >{installation.ownership.source === 'personal'
                    ? 'Account owner'
                    : 'Org admins'}</small
                >
              </td>
              <td>
                <span class="sync-state {ownershipTone(installation)}">
                  <span aria-hidden="true"></span>
                  {ownershipLabel(installation)}
                </span>
                {#if installation.ownership.detail !== undefined}
                  <small>{installation.ownership.detail}</small>
                {/if}
              </td>
              <td>
                <a
                  class="btn btn-row"
                  href={hrefFor(installation.account.login, 'settings')}
                  onclick={(event) => navigate(event, installation)}
                >
                  Inspect
                  <Icon name="chevron-right" size={15} />
                </a>
              </td>
            </tr>
          {:else}
            <tr>
              <td colspan="5" class="empty-cell">
                {#if loading}
                  Loading installation catalog…
                {:else if failure !== null}
                  <span role="alert">{failure}</span>
                {:else}
                  No installations match “{query.trim()}”
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
    min-width: 48rem;
    width: 100%;
  }

  th,
  td {
    border-bottom: 1px solid var(--border-subtle);
    padding: var(--space-3) var(--space-4);
    text-align: left;
    vertical-align: middle;
  }

  thead th {
    background: var(--table-header-bg);
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    font-weight: 650;
  }

  tbody th {
    font-weight: inherit;
  }

  tbody tr:last-child > * {
    border-bottom: 0;
  }

  tbody tr.unavailable {
    background: color-mix(in srgb, var(--warning) 3%, var(--surface-base));
  }

  td > strong,
  td > small {
    display: block;
  }

  td > small,
  .installation-identity small {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    margin-top: var(--space-1);
  }

  .installation-identity {
    align-items: center;
    display: inline-flex;
    gap: var(--space-3);
  }

  .installation-identity strong,
  .installation-identity small {
    display: block;
  }

  .installation-icon {
    align-items: center;
    background: color-mix(in srgb, #8b5cf6 10%, var(--surface-inset));
    border-radius: var(--radius-control);
    color: #7357bd;
    display: inline-flex;
    height: 2.25rem;
    justify-content: center;
    width: 2.25rem;
  }

  .sync-state {
    align-items: center;
    display: inline-flex;
    font-size: var(--font-size-compact);
    font-weight: 650;
    gap: var(--space-2);
  }

  .sync-state > span {
    background: currentColor;
    border-radius: 50%;
    height: 0.45rem;
    width: 0.45rem;
  }

  .sync-state.fresh {
    color: var(--admin);
  }

  .sync-state.warning {
    color: var(--warning);
  }

  .sync-state.error {
    color: var(--stop);
  }

  td:last-child {
    text-align: right;
  }

  .empty-cell {
    color: var(--text-secondary);
    height: 10rem;
    text-align: center;
  }

  @media (max-width: 42rem) {
    .installation-controls {
      align-items: stretch;
      flex-direction: column;
    }

    .installation-controls .btn {
      align-self: start;
    }
  }
</style>
