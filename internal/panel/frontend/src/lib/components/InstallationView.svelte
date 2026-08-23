<script lang="ts">
  import { untrack } from 'svelte';
  import { createMutation, useQueryClient } from '@tanstack/svelte-query';
  import { getPanelSession, type PanelSession } from '#lib/session.svelte.js';
  import { getSyncDraftScope } from '#lib/sync-drafts.svelte.js';
  import type { SyncConfigBatchResponse, TargetSettingsInput } from '#lib/types.js';
  import Button from './Button.svelte';
  import Plate from './Plate.svelte';

  /**
   * Which of an installation's views to draw.
   *
   * Passed in rather than read from the address, because there is no longer one
   * address that reaches all of these: a view that hosts a dialog is routed with
   * the segments that follow it, one that hosts none is routed without them, and
   * history is routed with its section. That is what makes an address like
   * `/i/acme/settings/anything` resolve to nothing and answer 404 from the wire.
   */
  const {
    view,
    clock = Date.now,
  }: {
    view: string;
    /** Passed through so catalogue wrappers can share the fixture timeline. */
    clock?: () => number;
  } = $props();

  const session = getPanelSession();
  const syncDraftScope = getSyncDraftScope();
  const queryClient = useQueryClient();
  const syncDrafts = $derived.by(() => {
    const targetId = session.selectedTarget?.id;
    const drafts = syncDraftScope.current;
    return targetId !== undefined && drafts?.targetId === targetId ? drafts : null;
  });

  $effect(() => {
    const targetId = session.selectedTarget?.id;
    if (targetId === undefined || (view !== 'sync' && view !== 'history')) return;
    untrack(() => syncDraftScope.forTarget(targetId));
  });
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

  function loadSyncOverride(repositoryId: string) {
    if (session.selectedTarget === null) throw new Error('select an installation first');
    return session.api.fetchSyncOverride(session.selectedTarget.id, repositoryId, 'files');
  }

  function saveSyncOverride(
    repositoryId: string,
    input: Parameters<PanelSession['api']['saveSyncOverride']>[3],
  ) {
    if (session.selectedTarget === null) throw new Error('select an installation first');
    return session.api.saveSyncOverride(session.selectedTarget.id, repositoryId, 'files', input);
  }

  function chunkError(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }

  function syncConfigRestored(targetId: string, result: SyncConfigBatchResponse): void {
    const drafts = syncDraftScope.forTarget(targetId);
    drafts.acceptCommitted(
      result.configs,
      'Restored. Reconciliation creates a plan only when repositories need changes.',
    );
    session.invalidateTargetData(targetId);
    void queryClient.invalidateQueries({ queryKey: ['sync-plan', targetId] });
  }
</script>

{#snippet loadingView(label: string)}
  <p class="route-loading" role="status">Loading {label}…</p>
{/snippet}

{#snippet failedView(label: string, error: unknown)}
  <Plate label="Problem" tone="alarm">
    <p>Could not load {label}: {chunkError(error)}</p>
    <Button onclick={() => window.location.reload()}>Reload panel</Button>
  </Plate>
{/snippet}

{#if session.selectedTarget !== null}
  {#if view === 'settings'}
    <div id="settings-panel">
      {#await import('./TargetSettings.svelte')}
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
      {#await import('./RepositoryList.svelte')}
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
            onLoadSyncOverride={loadSyncOverride}
            onSaveSyncOverride={saveSyncOverride}
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
      {#if syncDrafts === null}
        {@render loadingView('sync')}
      {:else}
        {#await import('./SyncView.svelte')}
          {@render loadingView('sync')}
        {:then { default: SyncView }}
          {#key session.selectedTarget.id}
            <SyncView
              targetId={session.selectedTarget.id}
              section={session.currentSyncSection}
              rulesetName={session.currentSyncRuleset}
              readOnly={!session.selectedTarget.capabilities.write}
              fetchConfig={session.api.fetchSyncConfig}
              drafts={syncDrafts}
              fetchPlan={session.api.fetchSyncPlan}
              approvePlan={session.api.approveSyncPlan}
              discardPlan={session.api.discardSyncPlan}
              fetchStatus={session.api.fetchSyncStatus}
              sectionHref={(s) => session.syncSectionHref(s)}
              onOpenSection={(s) => session.selectSyncSection(s)}
              rulesetHref={(name) => session.syncRulesetHref(name)}
              onOpenRuleset={(name) => session.selectSyncRuleset(name)}
              fileName={session.currentSyncFile}
              fileHref={(path) => session.syncFileHref(path)}
              onOpenFile={(path) => session.selectSyncFile(path)}
              fetchFilesContext={session.api.fetchSyncFilesContext}
              fetchOverride={session.api.fetchSyncOverride}
              saveOverride={session.api.saveSyncOverride}
              {clock}
            />
          {/key}
        {:catch error}
          {@render failedView('sync', error)}
        {/await}
      {/if}
    </div>
  {:else if view === 'users' || view === 'invitations'}
    <div id="access-panel">
      {#await import('./UserManagement.svelte')}
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
      {#if syncDrafts === null}
        {@render loadingView('history')}
      {:else}
        {#await import('./HistoryPanel.svelte')}
          {@render loadingView('history')}
        {:then { default: HistoryPanel }}
          {#key session.selectedTarget.id}
            <HistoryPanel
              targetId={session.selectedTarget.id}
              section={session.currentHistorySection}
              fetchAudit={(request: Parameters<typeof session.api.fetchAudit>[1]) =>
                session.api.fetchAudit(session.selectedTarget!.id, request)}
              fetchFailures={(request: Parameters<typeof session.api.fetchFailures>[1]) =>
                session.api.fetchFailures(session.selectedTarget!.id, request)}
              fetchSyncCheckpoint={session.api.fetchSyncConfigCheckpoint}
              restoreSyncCheckpoint={session.api.restoreSyncConfigCheckpoint}
              readOnly={!session.selectedTarget.capabilities.write}
              hasUnsavedSyncDrafts={syncDrafts.dirty}
              onSyncRestored={(result) => syncConfigRestored(session.selectedTarget!.id, result)}
              prefs={session.prefs}
            />
          {/key}
        {:catch error}
          {@render failedView('history', error)}
        {/await}
      {/if}
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
      <Button tone="signal" onclick={() => void session.load()}>Reload panel</Button>
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
    .empty-panel-state :global(.btn) {
      grid-column: 1 / -1;
      justify-self: start;
    }
  }
</style>
