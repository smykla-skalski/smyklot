<script lang="ts">
  import { page } from '$app/state';
  import { createMutation, useQueryClient } from '@tanstack/svelte-query';
  import { getPanelSession, type PanelSession } from '$lib/session.svelte';
  import type { TargetSettingsInput } from '$lib/types';
  import Plate from '$lib/components/Plate.svelte';

  const session = getPanelSession();
  const queryClient = useQueryClient();
  const view = $derived(page.params.view as string);
  const targetSettingsMutation = createMutation(() => ({
    mutationFn: ({ targetId, input }: { targetId: string; input: TargetSettingsInput }) =>
      session.api.updateTargetSettings(targetId, input),
    onSettled: (_data, _error, { targetId }) => {
      session.invalidateTargetData(targetId);
      return queryClient.invalidateQueries({ queryKey: ['targets'] });
    },
  }));

  async function updateTarget(input: TargetSettingsInput): Promise<void> {
    const target = session.selectedTarget;
    if (target === null) throw new Error('select an installation first');
    await targetSettingsMutation.mutateAsync({ targetId: target.id, input });
  }

  function fetchRepositories(request: Parameters<PanelSession['api']['fetchRepositories']>[1]) {
    if (session.selectedTarget === null) throw new Error('select an installation first');
    return session.api.fetchRepositories(session.selectedTarget.id, request);
  }
  function loadRepository(repositoryId: string) {
    if (session.selectedTarget === null) throw new Error('select an installation first');
    return session.api.fetchRepository(session.selectedTarget.id, repositoryId);
  }
  function updateRepository(
    repositoryId: string,
    input: Parameters<PanelSession['api']['updateRepositorySettings']>[2],
  ) {
    if (session.selectedTarget === null) throw new Error('select an installation first');
    return session.api.updateRepositorySettings(session.selectedTarget.id, repositoryId, input);
  }

  function chunkError(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }
</script>

{#snippet loadingView(label: string)}
  <p class="route-loading" role="status">Loading {label}…</p>
{/snippet}

{#snippet failedView(label: string, error: unknown)}
  <Plate label="Problem" tone="alarm">
    <p>Could not load {label}: {chunkError(error)}</p>
    <button class="btn" type="button" onclick={() => window.location.reload()}>Reload panel</button>
  </Plate>
{/snippet}

{#if session.selectedTarget !== null}
  {#if view === 'settings'}
    <div id="settings-panel">
      {#await import('$lib/components/TargetSettings.svelte')}
        {@render loadingView('settings')}
      {:then { default: TargetSettings }}
        {#key session.selectedTarget.id}
          <TargetSettings
            target={session.selectedTarget}
            readOnly={!session.selectedTarget.capabilities.write}
            onUpdate={updateTarget}
          />
        {/key}
      {:catch error}
        {@render failedView('settings', error)}
      {/await}
    </div>
  {:else if view === 'repositories'}
    <div id="repositories-panel">
      {#await import('$lib/components/RepositoryList.svelte')}
        {@render loadingView('repositories')}
      {:then { default: RepositoryList }}
        {#key session.selectedTarget.id}
          <RepositoryList
            targetId={session.selectedTarget.id}
            defaultEnabled={session.selectedTarget.repository_default_enabled}
            fetchPage={fetchRepositories}
            onLoad={loadRepository}
            onUpdate={updateRepository}
            onResetConfigMigration={(targetId, repositoryId) =>
              session.api.resetConfigMigration(targetId, repositoryId)}
            onChanged={(targetId) => session.repositoryChanged(targetId)}
            readOnly={!session.selectedTarget.capabilities.write}
            prefs={session.prefs}
          />
        {/key}
      {:catch error}
        {@render failedView('repositories', error)}
      {/await}
    </div>
  {:else if view === 'sync'}
    <div id="sync-panel">
      {#await import('$lib/components/SyncView.svelte')}
        {@render loadingView('sync')}
      {:then { default: SyncView }}
        {#key session.selectedTarget.id}
          <SyncView
            targetId={session.selectedTarget.id}
            readOnly={!session.selectedTarget.capabilities.write}
            fetchConfig={session.api.fetchSyncConfig}
            saveConfig={session.api.saveSyncConfig}
            fetchPlan={session.api.fetchSyncPlan}
            approvePlan={session.api.approveSyncPlan}
          />
        {/key}
      {:catch error}
        {@render failedView('sync', error)}
      {/await}
    </div>
  {:else if view === 'users' || view === 'invitations'}
    <div id="access-panel">
      {#await import('$lib/components/UserManagement.svelte')}
        {@render loadingView('access')}
      {:then { default: UserManagement }}
        {#key session.selectedTarget.id}
          <UserManagement
            section={view as 'users' | 'invitations'}
            prefs={session.prefs}
            targetId={session.selectedTarget.id}
            targetName={session.selectedTarget.account.display_name}
            actorLogin={session.viewer?.account.login ?? ''}
            actorTargetRole={session.selectedTarget.effective_role}
            onSection={(s: 'users' | 'invitations') => session.selectUserSection(s)}
            fetchTargetUsers={session.api.fetchTargetUsers}
            addTargetUser={session.api.addTargetUser}
            suggestUsers={session.api.suggestUsers}
            updateTargetUser={session.api.updateTargetUser}
            fetchTargetInvitations={session.api.fetchTargetInvitations}
            createTargetInvitation={session.api.createTargetInvitation}
            reissueInvitation={session.api.reissueTargetInvitation}
            revokeInvitation={session.api.revokeTargetInvitation}
            fetchUserDecisions={session.api.fetchUserDecisions}
          />
        {/key}
      {:catch error}
        {@render failedView('access', error)}
      {/await}
    </div>
  {:else if view === 'history'}
    <div id="history-panel">
      {#await import('$lib/components/HistoryPanel.svelte')}
        {@render loadingView('history')}
      {:then { default: HistoryPanel }}
        {#key session.selectedTarget.id}
          <HistoryPanel
            targetId={session.selectedTarget.id}
            section={session.currentHistorySection}
            onSection={(s: 'audit' | 'failures') => session.selectHistorySection(s)}
            fetchAudit={(request: Parameters<typeof session.api.fetchAudit>[1]) =>
              session.api.fetchAudit(session.selectedTarget!.id, request)}
            fetchFailures={(request: Parameters<typeof session.api.fetchFailures>[1]) =>
              session.api.fetchFailures(session.selectedTarget!.id, request)}
            prefs={session.prefs}
          />
        {/key}
      {:catch error}
        {@render failedView('history', error)}
      {/await}
    </div>
  {/if}
{:else if session.failure === null}
  <Plate label="No installations">
    <div class="empty-panel-state">
      <span class="empty-panel-mark" aria-hidden="true">+</span>
      <div>
        <strong>Install Smyklot to begin</strong>
        <p class="dim">
          Install the Smyklot GitHub App on an organization or personal account, then reload this
          panel
        </p>
      </div>
      <button class="btn btn-signal" type="button" onclick={() => void session.load()}>
        Reload panel
      </button>
    </div>
  </Plate>
{/if}

<style>
  #repositories-panel,
  #access-panel,
  #history-panel {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }
  .route-loading {
    color: var(--dim);
    margin: 0;
  }
  .empty-panel-state {
    align-items: center;
    display: grid;
    gap: var(--space-4);
    grid-template-columns: auto minmax(0, 1fr) auto;
    min-height: 7rem;
  }
  .empty-panel-state p {
    margin: var(--space-1) 0 0;
    max-width: 42rem;
  }
  .empty-panel-mark {
    align-items: center;
    background: var(--accent-tint);
    border: 1px solid color-mix(in srgb, var(--accent) 34%, transparent);
    border-radius: var(--radius-control);
    color: var(--accent);
    display: inline-flex;
    font: 650 1.25rem/1 var(--sans);
    height: 2.5rem;
    justify-content: center;
    width: 2.5rem;
  }
  @media (max-width: 36rem) {
    .empty-panel-state {
      align-items: start;
      grid-template-columns: auto minmax(0, 1fr);
    }
    .empty-panel-state .btn {
      grid-column: 1 / -1;
      justify-self: start;
    }
  }
</style>
