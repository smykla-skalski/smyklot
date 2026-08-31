<script lang="ts">
  import { plainClick } from '#lib/follow.js';
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import type { PanelApi } from '../api';
  import { fuzzyCandidates } from '../fuzzy';
  import type { RootRoute, RootInstallationView } from '../routes';
  import {
    getSettingsDraftRegistry,
    type SettingsDirtyControl,
    type SettingsScope,
  } from '../settings-drafts.svelte';
  import type { RootInstallation } from '../types';
  import Card from './Card.svelte';
  import Icon from './Icon.svelte';
  import Pill, { type PillTone } from './Pill.svelte';
  import RelativeTime from './RelativeTime.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import SearchField from './SearchField.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  const {
    route,
    api,
    actorLogin,
    listHref,
    hrefFor,
    onList,
    onNavigate,
    historySection,
  }: {
    route: RootRoute;
    api: PanelApi;
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

  /* Ten at a time, and a search shows everything it matched: a person who typed a
     name is asking about that name, not about the first ten of it. */
  const PAGE = 10;
  let limit = $state(PAGE);

  /* One clock for the page, floored to the minute - every row reads a sync time
     against it. */
  const nowMs = Math.floor(Date.now() / 60_000) * 60_000;

  const selected = $derived(
    route.rootView === 'installation'
      ? (installations.find(
          (installation) =>
            installation.account.login.toLocaleLowerCase() === route.account.toLocaleLowerCase(),
        ) ?? null)
      : null,
  );
  const visible = $derived(
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
  const shown = $derived(query.trim() === '' ? visible.slice(0, limit) : visible);

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

  /* A standing appears only where somebody has to do something about it. Stale is
     an ageing snapshot rather than a fault - the next sweep fixes it - so the row
     wears nothing and says when it last synced, which is the same fact without
     the alarm. */
  function attentionPill(installation: RootInstallation): { tone: PillTone; label: string } | null {
    if (installation.ownership.status === 'error') {
      return { tone: 'danger', label: 'Owner list unavailable' };
    }
    if (installation.ownership.status === 'permission_pending') {
      return { tone: 'warning', label: 'Owner approval waiting' };
    }

    return null;
  }

  /* Only a refusal replaces the row's sentence. A workspace waiting on an approval
     still has an owner list, an age and repositories to say something about; one
     whose list could not be read has nothing true to say but why. */
  function unreadable(installation: RootInstallation): boolean {
    return installation.ownership.status === 'error';
  }

  function repositorySentence(installation: RootInstallation): string {
    const counts = installation.repository_counts;
    if (counts.total === 0) return 'No repositories yet';
    if (counts.enabled === 0) {
      return `${counts.total} ${counts.total === 1 ? 'repository' : 'repositories'}, none with commands on`;
    }

    return `${counts.total} ${counts.total === 1 ? 'repository' : 'repositories'}, ${counts.enabled} with commands on`;
  }
</script>

<!--
@component
Every workspace the service knows, and the way into any one of them. The list and the
single workspace are one component because they are one address space: opening one
stands in place of the list rather than over it, so the navigation still reads
Workspaces and Back still means the list.

One row is one sentence - how much of the workspace Smyklot answers in, when its owner
list last synced, whether anything has failed - and a standing appears only where
somebody has to do something about it. The twin health columns this page used to carry
said the same two facts in two places and made a reader read a legend to learn which.

The workspace view is imported lazily. It is the heaviest page in the console and most
visits to this route never open one.
-->

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
  <div class="view-frame">
    <RootPageHeader
      title="Workspaces"
      subtitle="GitHub organizations and accounts with Smyklot installed"
      headingId="root-page-heading"
    >
      {#if syncProblem !== null || syncFeedback !== ''}
        <span class="slot-note" class:problem={syncProblem !== null}>
          {syncProblem ?? syncFeedback}
        </span>
      {/if}
      <!-- The catalog is a background lane and refreshes on its own; this is the
           one place an operator can ask for it now rather than at the next sweep. -->
      <button
        class="btn btn-quiet"
        type="button"
        disabled={syncing}
        onclick={() => void syncCatalog()}
      >
        <Icon name="refresh" size="sm" />
        <span class="button-label">{syncing ? 'Synchronizing…' : 'Sync now'}</span>
      </button>
    </RootPageHeader>

    <div class="filter-bar">
      <SearchField
        label="Find a workspace"
        placeholder="Find a workspace"
        value={query}
        onInput={(value) => (query = value)}
      />
    </div>

    <Card>
      {#if visible.length === 0}
        <div class="state-panel">
          {#if loading && installations.length === 0}
            <span>Reading the catalog</span>
          {:else if failure !== null}
            <span role="alert"><strong>The catalog could not be read.</strong> {failure}</span>
          {:else if query.trim() === ''}
            <span
              ><strong>No workspaces yet.</strong> A workspace appears here once somebody installs Smyklot
              on their organization or account</span
            >
          {:else}
            <span><strong>Nothing matches.</strong> No workspace is called “{query.trim()}”</span>
          {/if}
        </div>
      {:else}
        <ul class="object-list">
          {#each shown as installation (installation.id)}
            {@const dirtyCount = dirtyInstallationCount(installation.id)}
            {@const destination = dirtyInstallationView(installation.id)}
            {@const failed = installation.delivery_health.failed}
            <li>
              <div
                class="object-row"
                class:is-unsaved={dirtyCount > 0}
                data-unsaved={dirtyCount > 0 || undefined}
              >
                <span class="object-main">
                  <span class="object-name-row">
                    <!-- The name is not a link, and the row is not a target. Opening a
                         workspace here reopens the whole shell inside it, wearing the
                         operator strip - a consequence a stray press on a row should
                         not have, so it is asked for by name. -->
                    <span class="object-name">{installation.account.display_name}</span>
                    {#if attentionPill(installation) !== null}
                      <Pill tone={attentionPill(installation)?.tone}>
                        {attentionPill(installation)?.label}
                      </Pill>
                    {/if}
                    {#if dirtyCount > 0}
                      <Pill tone="warning">
                        {dirtyCount} unsaved {dirtyCount === 1 ? 'setting' : 'settings'}
                      </Pill>
                    {/if}
                  </span>
                  <!-- One sentence, not five columns. What a workspace is (how much of
                       it Smyklot answers in), when its owner list last synced, and
                       whether anything has failed - and where the owner list cannot be
                       read at all, the reason stands in place of the rest, because a
                       sync time on a list nobody could read is a lie. -->
                  <span class="object-sum" class:is-refused={unreadable(installation)}>
                    {#if unreadable(installation)}
                      {installation.ownership.detail ??
                        'The owner list cannot be read until an organization owner grants the Members permission again'}
                    {:else}
                      {repositorySentence(installation)} · owner list synced
                      <RelativeTime value={installation.ownership.synced_at} {nowMs} />
                      <!-- The space before the block, never inside it: Svelte trims a
                           block's leading whitespace, and the separator arrived stuck
                           to the last letter of the time. -->
                      {#if failed > 0}· {failed}
                        {failed === 1 ? 'failure' : 'failures'} kept{/if}
                    {/if}
                  </span>
                </span>
                <span class="object-side">
                  <a
                    class="btn btn-quiet"
                    href={hrefFor(installation.account.login, destination)}
                    onclick={(event) => navigate(event, installation, destination)}
                    aria-label="Open {installation.account.display_name} as operator"
                  >
                    <span class="button-label">Open as operator</span>
                  </a>
                </span>
              </div>
            </li>
          {/each}
        </ul>
        <div class="list-foot">
          <span
            >Showing 1-{shown.length} of {visible.length}{query.trim() === ''
              ? ''
              : ` matching · ${installations.length} in all`}</span
          >
          {#if shown.length < visible.length}
            <button class="btn btn-quiet" type="button" onclick={() => (limit += PAGE)}>
              <span class="button-label"
                >Show {Math.min(PAGE, visible.length - shown.length)} more</span
              >
            </button>
          {/if}
        </div>
      {/if}
    </Card>
  </div>
{/if}

<style>
  .installation-loading {
    color: var(--text-muted);
    margin: 0;
  }

  .filter-bar :global(.search-field) {
    flex: 1 1 12rem;
    max-inline-size: 20rem;
    min-inline-size: 0;
  }

  .slot-note {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    white-space: nowrap;
  }

  .slot-note.problem {
    color: var(--danger);
  }
</style>
