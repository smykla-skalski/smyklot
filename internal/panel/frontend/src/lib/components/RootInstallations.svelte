<script lang="ts">
  import { plainClick } from '#lib/follow.js';
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import type { PanelApi } from '../api';
  import { formatDateTime } from '../format';
  import { fuzzyCandidates } from '../fuzzy';
  import { monogram } from '../identity';
  import type { RootRoute, RootInstallationView } from '../routes';
  import {
    getSettingsDraftRegistry,
    type SettingsDirtyControl,
    type SettingsScope,
  } from '../settings-drafts.svelte';
  import type { RootInstallation } from '../types';
  import Chip from './Chip.svelte';
  import DataTable from './DataTable.svelte';
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
  }: {
    route: RootRoute;
    api: PanelApi;
    rootRole: string;
    actorLogin: string;
    listHref: string;
    hrefFor: (account: string, view: RootInstallationView) => string;
    onList: () => void;
    onNavigate: (account: string, view: RootInstallationView) => void;
    historySection: 'audit' | 'failures';
  } = $props();

  const settingsDrafts = getSettingsDraftRegistry();
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
    if (!plainClick(event)) return;
    event.preventDefault();
    onNavigate(installation.account.login, view);
  }

  function clickRow(event: MouseEvent, installation: RootInstallation): void {
    const target = event.target instanceof Element ? event.target : null;
    if (target?.closest('button, a, summary, input') !== null) return;
    navigate(event, installation, dirtyInstallationView(installation.id));
  }

  function keyRow(event: KeyboardEvent, installation: RootInstallation): void {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    const target = event.target instanceof Element ? event.target : null;
    if (target?.closest('button, a, summary, input') !== null) return;
    event.preventDefault();
    onNavigate(installation.account.login, dirtyInstallationView(installation.id));
  }

  function installationSettingsScope(targetId: string): SettingsScope {
    return { type: 'installation', targetId };
  }

  function dirtyInstallationControls(targetId: string): SettingsDirtyControl[] {
    return settingsDrafts
      .dirtyControls(installationSettingsScope(targetId))
      .toSorted((left, right) => left.changedAt - right.changedAt);
  }

  function dirtyInstallationCount(targetId: string): number {
    return dirtyInstallationControls(targetId).length;
  }

  function dirtyInstallationView(targetId: string): RootInstallationView {
    const control = dirtyInstallationControls(targetId).find(
      (candidate) =>
        candidate.location.section === 'defaults' || candidate.location.section === 'repositories',
    );
    return control?.location.section === 'repositories' ? 'repositories' : 'defaults';
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
  /* Named once, so the count the empty row spans is counted rather than typed -
     `colspan="5"` was a number that had to be remembered every time a column
     moved. */
  const COLUMNS = [
    { label: 'Installation' },
    { label: 'Repositories', class: 'count-heading' },
    { label: 'Delivery' },
    { label: 'Ownership' },
    { label: 'Owners' },
  ];
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
        {onList}
        {historySection}
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

    <DataTable
      class="installation-table-shell"
      pinned
      caption="Installation catalog"
      regionLabel="Installation catalog table"
      rows={visibleInstallations}
      rowKey={(installation) => installation.id}
      columns={COLUMNS}
      rowAttrs={(installation) => ({
        class: [
          'installation-row data-row',
          dirtyInstallationCount(installation.id) > 0 && 'is-unsaved',
        ]
          .filter(Boolean)
          .join(' '),
        'data-unsaved': dirtyInstallationCount(installation.id) > 0 || undefined,
        tabindex: 0,
        onclick: (event: MouseEvent) => clickRow(event, installation),
        onkeydown: (event: KeyboardEvent) => keyRow(event, installation),
      })}
    >
      {#snippet cells(installation)}
        {@const dirtyCount = dirtyInstallationCount(installation.id)}
        {@const destination = dirtyInstallationView(installation.id)}
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
                href={hrefFor(installation.account.login, destination)}
                onclick={(event) => navigate(event, installation, destination)}
              >
                {installation.account.display_name}
              </a>
              <small>@{installation.account.login} · #{installation.installation_id}</small>
            </span>
            {#if dirtyCount > 0}
              <span
                class="installation-unsaved"
                aria-label={`${dirtyCount} unsaved ${dirtyCount === 1 ? 'setting' : 'settings'}`}
                title={`${dirtyCount} unsaved ${dirtyCount === 1 ? 'setting' : 'settings'}`}
              >
                <Chip tone="warning" small>Unsaved changes</Chip>
              </span>
            {/if}
          </span>
        </th>
        <td class="count-cell">
          {#if installation.repository_counts.total === 0}
            <span class="cell-dash band-trim" aria-label="No repositories">—</span>
          {:else}
            <span class="repo-count band-trim">
              <b>{installation.repository_counts.enabled}</b>
              of {installation.repository_counts.enabled + installation.repository_counts.disabled} enabled
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
      {/snippet}
      {#snippet empty()}
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
      {/snippet}
    </DataTable>
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

  /* On a phone the search cannot hold its 15rem basis - it takes the row
     and gives with it. */
  @media (max-width: 30rem) {
    .installation-tools {
      grid-template-columns: minmax(0, 1fr);
    }

    .installation-tools :global(.search-field) {
      flex: 1 1 auto;
      min-inline-size: 0;
    }
  }

  .slot-note.problem {
    color: var(--stop);
  }

  /* Everything this block used to hold - the scroll shell, `border-collapse`, the
     cell padding and separator, the band height, the row-header weight - is now
     `DataTable` and the `.data-table` rules in `app.css`. What is left is this
     table's own: its scroll floor, and the columns.

     The first-child and last-child padding overrides went with it. They restated
     `var(--space-3)`, which is the padding every cell already had, so all four
     were writing a value over itself. */
  :global(.installation-table-shell) {
    --table-cell-font-size: var(--font-size-meta);
    --table-min-width: 52rem;
  }

  /* `:global`, and it has to be: `.installation-row` is a class on a `<tr>` that
     `DataTable` renders, so the row carries the component's scope class and not
     this file's. Anchored through the class passed to `DataTable`, so it reaches
     this table's rows and no others. */
  :global(.installation-table-shell .installation-row) {
    cursor: pointer;
    height: 3.75rem;
    transition: background-color var(--duration-fast) var(--ease-standard);
  }

  :global(.installation-table-shell .installation-row:focus-visible) {
    outline: 2px solid var(--focus);
    outline-offset: -2px;
  }

  :global(.installation-table-shell .installation-row.is-unsaved) {
    box-shadow: inset 2px 0 var(--brand-action);
  }

  /* Left, like every other column. The mock reads this cell as a sentence
     ("10 of 28 enabled"), not as a figure to scan down, so right-aligning it
     put the header and the value on two different edges. */
  :global(.installation-table-shell :is(.count-heading, .count-cell)) {
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
  /* Clipped past the edge rather than at it.
     ----------------------------------------
     `.band-trim-stack` ends this line's box on its baseline, which is what centres
     the pair against the monogram beside them. The trim moves the box and not the
     glyphs, so the `y` in a login and the tail of the `@` still paint below it, and
     `overflow: hidden` cut them off along the bottom of every row. Chrome is the
     only engine that implements the trim, so it was the only one showing it.

     A little room outside the box, not an open block axis: this is a table row, and
     ink that escaped it would land in the row underneath. 0.4em is what the queue's
     pull-request names already ask for, and the deepest descender here is 0.18em. */
  .installation-identity small {
    color: var(--text-muted);
    font: 400 var(--font-size-compact) / 1.2 var(--mono);
    overflow: clip;
    overflow-clip-margin: 0.4em;
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
    max-width: 100%;
  }

  .installation-unsaved {
    display: inline-flex;
    flex: none;
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

  /* The layout is `pinned` on `DataTable` now; these are the part of it that could
     not move - the explicit widths a fixed row-table needs so thead and tbody line
     up. The approved catalog's 2fr 1.2fr 1.1fr 1.5fr 1fr, written as percentages of
     the 6.8fr total so they hold at any table width.

     `:global` and anchored, because every element named here is one `DataTable`
     renders. */
  @media (min-width: 64.001rem) {
    :global(.installation-table-shell :is(th, td):nth-child(1)) {
      width: 29.412%;
    }

    :global(.installation-table-shell :is(th, td):nth-child(2)) {
      width: 17.647%;
    }

    :global(.installation-table-shell :is(th, td):nth-child(3)) {
      width: 16.176%;
    }

    :global(.installation-table-shell :is(th, td):nth-child(4)) {
      width: 22.059%;
    }

    :global(.installation-table-shell :is(th, td):nth-child(5)) {
      width: 14.706%;
    }
  }
</style>
