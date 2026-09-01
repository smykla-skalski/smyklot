<script lang="ts">
  import { useQueryClient } from '@tanstack/svelte-query';
  import { plainClick } from '#lib/follow.js';
  import { getPanelSession, type PanelSession } from '#lib/session.svelte.js';
  import { getSettingsDraftRegistry } from '#lib/settings-drafts.svelte.js';
  import Button from './Button.svelte';
  import Plate from './Plate.svelte';

  const {
    view,
    clock = Date.now,
  }: {
    view: string;
    /** Passed through so catalogue wrappers can share the fixture timeline. */
    clock?: () => number;
  } = $props();

  const session = getPanelSession();
  const settingsDrafts = getSettingsDraftRegistry();
  const queryClient = useQueryClient();
  function fetchRepositories(request: Parameters<PanelSession['api']['fetchRepositories']>[1]) {
    if (session.selectedTarget === null) throw new Error('select a workspace first');
    return session.api.fetchRepositories(session.selectedTarget.id, request);
  }
  function loadRepository(repositoryId: string) {
    if (session.selectedTarget === null) throw new Error('select a workspace first');
    return session.api.fetchRepository(session.selectedTarget.id, repositoryId);
  }
  function loadSyncOverride(repositoryId: string) {
    if (session.selectedTarget === null) throw new Error('select a workspace first');
    return session.api.fetchSyncOverride(session.selectedTarget.id, repositoryId, 'files');
  }

  function loadSyncStatus() {
    if (session.selectedTarget === null) throw new Error('select a workspace first');
    return session.api.fetchSyncStatus(session.selectedTarget.id);
  }

  function chunkError(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }

  function settingsRestored(targetId: string): void {
    session.repositoryChanged(targetId);
    void Promise.all([
      queryClient.invalidateQueries({ queryKey: ['sync-override', targetId] }),
      queryClient.invalidateQueries({ queryKey: ['sync-plan', targetId] }),
      queryClient.invalidateQueries({ queryKey: ['audit', 'root'] }),
    ]);
  }
</script>

<!--
@component
Which of a workspace's views to draw.

Passed in rather than read from the address, because there is no longer one
address that reaches all of these: a view that hosts a dialog is routed with
the segments that follow it, one that hosts none is routed without them, and
history is routed with its section. That is what makes an address like
`/workspace/acme/defaults/anything` resolve to nothing and answer 404 from the wire.
-->

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
  {#if view === 'overview'}
    <div id="overview-panel">
      {#await import('./WorkspaceOverview.svelte')}
        {@render loadingView('the overview')}
      {:then { default: WorkspaceOverview }}
        {#key session.selectedTarget.id}
          <WorkspaceOverview target={session.selectedTarget} />
        {/key}
      {:catch error}
        {@render failedView('the overview', error)}
      {/await}
    </div>
  {:else if view === 'settings'}
    <div id="defaults-panel">
      {#await import('./TargetSettings.svelte')}
        {@render loadingView('workspace settings')}
      {:then { default: TargetSettings }}
        {#key session.selectedTarget.id}
          <TargetSettings
            target={session.selectedTarget}
            readOnly={!session.selectedTarget.capabilities.write}
            timing={{
              api: session.api,
              canRequest:
                session.selectedTarget.effective_role === 'admin' ||
                session.selectedTarget.effective_role === 'owner',
            }}
          />
        {/key}
      {:catch error}
        {@render failedView('workspace settings', error)}
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
            onResetConfigMigration={(targetId, repositoryId) =>
              session.api.resetConfigMigration(targetId, repositoryId)}
            onChanged={(targetId) => session.repositoryChanged(targetId)}
            onLoadSyncOverride={loadSyncOverride}
            onLoadSyncStatus={loadSyncStatus}
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
      {#await import('./SyncView.svelte')}
        {@render loadingView('sync')}
      {:then { default: SyncView }}
        {#key session.selectedTarget.id}
          <SyncView
            targetId={session.selectedTarget.id}
            section={session.currentSyncSection}
            rulesetName={session.currentSyncRuleset}
            readOnly={!session.selectedTarget.capabilities.write}
            canControl={['admin', 'owner'].includes(session.selectedTarget.effective_role)}
            fetchConfig={session.api.fetchSyncConfig}
            fetchPlan={session.api.fetchSyncPlan}
            approvePlan={session.api.approveSyncPlan}
            discardPlan={session.api.discardSyncPlan}
            runSyncNow={session.api.runSyncNow}
            fetchStatus={session.api.fetchSyncStatus}
            sectionHref={(s) => session.syncSectionHref(s)}
            onOpenSection={(s) => session.selectSyncSection(s)}
            rulesetHref={(name) => session.syncRulesetHref(name)}
            onOpenRuleset={(name) => session.selectSyncRuleset(name)}
            fileName={session.currentSyncFile}
            fileHref={(path) => session.syncFileHref(path)}
            onOpenFile={(path) => session.selectSyncFile(path)}
            fetchFilesContext={session.api.fetchSyncFilesContext}
            renderFile={session.api.renderSyncFile}
            fetchOverride={session.api.fetchSyncOverride}
            {clock}
          />
        {/key}
      {:catch error}
        {@render failedView('sync', error)}
      {/await}
    </div>
  {:else if view === 'queue'}
    <div id="queue-panel">
      {#await import('./GeneralQueueView.svelte')}
        {@render loadingView('queue')}
      {:then { default: GeneralQueueView }}
        {#key session.selectedTarget.id}
          <GeneralQueueView
            api={session.api}
            targetId={session.selectedTarget.id}
            section={session.currentQueueSection}
            onSelectSection={(value) => session.selectQueueSection(value)}
            canControl={session.selectedTarget.effective_role === 'admin' ||
              session.selectedTarget.effective_role === 'owner'}
            planHref={session.syncSectionHref('plan')}
            onOpenPlan={(event) => {
              if (!plainClick(event)) return;
              event.preventDefault();
              session.selectSyncSection('plan');
            }}
          />
        {/key}
      {:catch error}
        {@render failedView('queue', error)}
      {/await}
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
      {#await import('./HistoryPanel.svelte')}
        {@render loadingView('history')}
      {:then { default: HistoryPanel }}
        {#key session.selectedTarget.id}
          <HistoryPanel
            targetId={session.selectedTarget.id}
            section={session.currentHistorySection}
            fetchAudit={(request: Parameters<typeof session.api.fetchAudit>[1]) =>
              session.api.fetchAudit(session.selectedTarget!.id, request)}
            exportAudit={(request: Parameters<typeof session.api.fetchAudit>[1]) =>
              session.api.auditExportHref(session.selectedTarget!.id, request)}
            fetchFailures={(request: Parameters<typeof session.api.fetchFailures>[1]) =>
              session.api.fetchFailures(session.selectedTarget!.id, request)}
            fetchSettingsCheckpoint={session.api.fetchWorkspaceSettingsCheckpoint}
            fetchSettingsBaseline={session.api.fetchWorkspaceSettingsBaseline}
            restoreSettingsCheckpoint={session.api.restoreWorkspaceSettingsCheckpoint}
            readOnly={!session.selectedTarget.capabilities.write}
            hasUnsavedSettingsDrafts={settingsDrafts.hasDirty({
              type: 'workspace',
              targetId: session.selectedTarget.id,
            })}
            onSettingsRestored={() => settingsRestored(session.selectedTarget!.id)}
            prefs={session.prefs}
            repositoryHref={(failure) =>
              session.repositoryHref(
                failure.repository_full_name.slice(
                  failure.repository_full_name.lastIndexOf('/') + 1,
                ),
              )}
          />
        {/key}
      {:catch error}
        {@render failedView('history', error)}
      {/await}
    </div>
  {/if}
{:else if session.failure === null}
  <Plate label="No workspaces">
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
  #queue-panel,
  #access-panel,
  #history-panel {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }
  .route-loading {
    color: var(--text-muted);
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
    background: var(--brand-action-tint);
    border: 1px solid color-mix(in srgb, var(--brand-action) 34%, transparent);
    border-radius: var(--radius-control);
    color: var(--brand-action);
    display: inline-flex;
    font: 650 1.25rem/var(--leading-flat) var(--sans);
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
