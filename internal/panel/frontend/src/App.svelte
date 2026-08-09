<script lang="ts">
  import HistoryPanel from './components/HistoryPanel.svelte';
  import HelpPanel from './components/HelpPanel.svelte';
  import IdentityBar from './components/IdentityBar.svelte';
  import PageFooter from './components/PageFooter.svelte';
  import Plate from './components/Plate.svelte';
  import RepositoryList from './components/RepositoryList.svelte';
  import SignedOut from './components/SignedOut.svelte';
  import TargetSettings from './components/TargetSettings.svelte';
  import UserManagement from './components/UserManagement.svelte';
  import type { PanelApi } from './lib/api';
  import type { PanelBuild } from './lib/base';
  import { LatestRequest } from './lib/latest-request';
  import {
    readLastInstallation,
    readSidebarDisplay,
    readThemeDisplay,
    type ThemeDisplay,
    writeLastInstallation,
    writeSidebarDisplay,
    writeThemeDisplay,
  } from './lib/preferences';
  import {
    resolvePanelRoute,
    type PanelRoute,
    type PanelRouter,
    type PanelView,
  } from './lib/routes';
  import type {
    PanelTarget,
    PanelViewer,
    RepositoryDetail,
    RepositoryPageRequest,
    RepositorySettingsInput,
    TargetSettingsInput,
  } from './lib/types';

  type FailureSource = 'load' | 'sign-out' | 'stream';
  type PanelFailure = { message: string; source: FailureSource };

  const {
    api,
    iconUrl,
    build,
    router,
  }: { api: PanelApi; iconUrl: string; build: PanelBuild; router: PanelRouter } = $props();

  let loading = $state(true);
  let viewer = $state<PanelViewer | null>(null);
  let targets = $state<PanelTarget[]>([]);
  let selectedId = $state<string | null>(null);
  let failure = $state<PanelFailure | null>(null);
  let historyVersion = $state(0);
  let repositoryDetailsVersion = $state(0);
  let userVersion = $state(0);
  let view = $state<PanelView>('settings');
  let globalUsers = $state(false);
  let streamReady = $state(false);
  let revokedReason = $state<string | null>(null);
  let sidebarCollapsed = $state(readSidebarDisplay() === 'collapsed');
  let theme = $state<ThemeDisplay>(readThemeDisplay());
  const targetReads = new LatestRequest();
  const streamRefreshes = new LatestRequest();

  const selectedTarget = $derived(
    selectedId === null ? null : (targets.find((target) => target.id === selectedId) ?? null),
  );

  async function load(): Promise<void> {
    loading = viewer === null;
    streamReady = false;
    streamRefreshes.invalidate();
    failure = null;
    try {
      viewer = await api.fetchViewer();
      if (viewer === null) {
        targets = [];
        selectedId = null;
        return;
      }
      if (!(await refreshTargets())) return;
      await activateRoute(router.current(), 'replace');
      historyVersion += 1;
      streamReady = true;
    } catch (error) {
      setFailure('load', error);
    } finally {
      loading = false;
    }
  }

  async function refreshTargets(): Promise<boolean> {
    const read = targetReads.begin();
    let listed: PanelTarget[];
    try {
      listed = await api.fetchTargets();
    } catch (error) {
      if (!targetReads.isCurrent(read)) return false;
      throw error;
    }
    if (!targetReads.isCurrent(read)) return false;
    targets = listed;
    if (selectedId !== null && !listed.some((target) => target.id === selectedId)) {
      selectedId = null;
    }

    return true;
  }

  async function selectTarget(targetId: string): Promise<void> {
    const target = targets.find((entry) => entry.id === targetId);
    if (target === undefined || selectedId === targetId) return;
    if (view === 'help' || (view === 'users' && globalUsers)) {
      selectedId = target.id;
      writeLastInstallation(target.account.login);
      failure = null;
      repositoryDetailsVersion += 1;
      historyVersion += 1;
      userVersion += 1;
      return;
    }
    await activateRoute(routeFor(target, view), 'push');
  }

  function selectView(nextView: PanelView): void {
    const target = selectedTarget;
    if (target === null || view === nextView) return;
    view = nextView;
    if (nextView === 'users') globalUsers = viewer?.capabilities.manage_global_users === true;
    router.push(routeFor(target, nextView));
  }

  async function activateRoute(
    requested: PanelRoute | null,
    navigation: 'none' | 'push' | 'replace',
  ): Promise<void> {
    const resolved = resolvePanelRoute(
      targets.map((target) => target.account.login),
      requested,
      readLastInstallation(),
    );
    if (resolved === null) {
      selectedId = null;
      return;
    }

    const target = targetForAccount(resolved.account);
    if (target === undefined) return;

    const targetChanged = selectedId !== target.id;
    selectedId = target.id;
    view = resolved.view;
    globalUsers =
      requested?.view === 'users' &&
      !('account' in requested) &&
      viewer?.capabilities.manage_global_users === true;
    writeLastInstallation(target.account.login);
    const canonical = routeFor(target, resolved.view);

    if (navigation === 'push') {
      router.push(canonical);
    } else if (navigation === 'replace' || !sameRoute(requested, canonical)) {
      router.replace(canonical);
    }

    if (!targetChanged) return;
    failure = null;
    repositoryDetailsVersion += 1;
    historyVersion += 1;
    userVersion += 1;
  }

  function targetForAccount(account: string): PanelTarget | undefined {
    const folded = account.toLowerCase();
    return targets.find((target) => target.account.login.toLowerCase() === folded);
  }

  function routeFor(target: PanelTarget, nextView: PanelView): PanelRoute {
    if (nextView === 'help') return { view: 'help' };
    if (nextView === 'users' && globalUsers) return { view: 'users' };
    return { account: target.account.login, view: nextView };
  }

  function targetHref(target: PanelTarget): string {
    return router.path(routeFor(target, view));
  }

  function viewHref(nextView: PanelView): string {
    const target = selectedTarget;
    return target === null ? '#' : router.path(routeFor(target, nextView));
  }

  function sameRoute(left: PanelRoute | null, right: PanelRoute): boolean {
    if (left === null || left.view !== right.view) return false;
    if (!('account' in left) || !('account' in right)) {
      return !('account' in left) && !('account' in right);
    }
    return left.account === right.account;
  }

  async function updateTarget(input: TargetSettingsInput): Promise<void> {
    const target = selectedTarget;
    if (target === null) return;
    const updated = await api.updateTargetSettings(target.id, input);
    targetReads.invalidate();
    targets = targets.map((entry) => (entry.id === updated.id ? updated : entry));
    repositoryDetailsVersion += 1;
    historyVersion += 1;
    userVersion += 1;
  }

  function fetchRepositories(
    request: RepositoryPageRequest,
  ): ReturnType<PanelApi['fetchRepositories']> {
    const target = selectedTarget;
    if (target === null) throw new Error('select an installation first');
    return api.fetchRepositories(target.id, request);
  }

  function loadRepository(repositoryId: string): Promise<RepositoryDetail> {
    const target = selectedTarget;
    if (target === null) throw new Error('select an installation first');
    return api.fetchRepository(target.id, repositoryId);
  }

  function updateRepository(
    repositoryId: string,
    input: RepositorySettingsInput,
  ): Promise<RepositoryDetail> {
    const target = selectedTarget;
    if (target === null) throw new Error('select an installation first');
    return api.updateRepositorySettings(target.id, repositoryId, input);
  }

  function repositoryChanged(targetId: string): void {
    if (viewer === null) return;
    if (selectedId === targetId) repositoryDetailsVersion += 1;
    historyVersion += 1;
    refreshFromStreamSafely();
  }

  async function refreshFromStream(refreshViewer = false): Promise<void> {
    const refresh = streamRefreshes.begin();
    try {
      if (refreshViewer) {
        const currentViewer = await api.fetchViewer();
        if (!streamRefreshes.isCurrent(refresh)) return;
        if (currentViewer === null) {
          revokeAccess('Your panel access was revoked');
          return;
        }
        viewer = currentViewer;
      }
      if (!(await refreshTargets()) || !streamRefreshes.isCurrent(refresh)) return;
      if (selectedId === null) {
        await activateRoute(router.current(), 'replace');
      } else if (selectedTarget !== null) {
        writeLastInstallation(selectedTarget.account.login);
        router.replace(routeFor(selectedTarget, view));
      }
      if (selectedId !== null) {
        repositoryDetailsVersion += 1;
        historyVersion += 1;
        userVersion += 1;
      }
      clearFailure('stream');
    } catch (error) {
      if (streamRefreshes.isCurrent(refresh)) setFailure('stream', error);
    }
  }

  function refreshFromStreamSafely(): void {
    void refreshFromStream();
  }

  function refreshAccessFromStream(): void {
    void refreshFromStream(true);
  }

  function selectUserScope(targetId: string | null): void {
    if (view !== 'users') return;
    if (targetId === null) {
      if (viewer?.capabilities.manage_global_users !== true || selectedTarget === null) return;
      globalUsers = true;
      router.push({ view: 'users' });
      userVersion += 1;
      return;
    }
    const target = targets.find((entry) => entry.id === targetId);
    if (target === undefined || !target.capabilities.manage_target_users) return;
    globalUsers = false;
    selectedId = target.id;
    writeLastInstallation(target.account.login);
    router.push({ account: target.account.login, view: 'users' });
    userVersion += 1;
  }

  function revokeAccess(reason: string): void {
    revokedReason = reason;
    viewer = null;
    targets = [];
    selectedId = null;
    streamReady = false;
    targetReads.invalidate();
    streamRefreshes.invalidate();
  }

  $effect(() => {
    if (!streamReady) return;
    return api.openStream({
      onResync: refreshAccessFromStream,
      onChange: (event) =>
        event.type === 'access.changed' ? refreshAccessFromStream() : refreshFromStreamSafely(),
      onRevoked: (event) => revokeAccess(event.reason),
    });
  });

  $effect(() =>
    router.subscribe((route) => {
      if (viewer !== null && !loading) void activateRoute(route, 'none');
    }),
  );

  async function signOut(): Promise<void> {
    loading = true;
    failure = null;
    try {
      await api.signOut();
      viewer = null;
      targets = [];
      selectedId = null;
      streamReady = false;
      targetReads.invalidate();
      streamRefreshes.invalidate();
      await load();
    } catch (error) {
      setFailure('sign-out', error);
      loading = false;
    }
  }

  function setFailure(source: FailureSource, error: unknown): void {
    failure = { message: errorMessage(error), source };
  }

  function clearFailure(...sources: FailureSource[]): void {
    if (failure !== null && sources.includes(failure.source)) failure = null;
  }

  function errorMessage(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }

  function toggleSidebar(): void {
    sidebarCollapsed = !sidebarCollapsed;
    writeSidebarDisplay(sidebarCollapsed ? 'collapsed' : 'expanded');
  }

  function selectTheme(nextTheme: ThemeDisplay): void {
    theme = nextTheme;
    document.documentElement.dataset.theme = nextTheme;
    writeThemeDisplay(nextTheme);
  }

  void load();
</script>

<a class="skip-link" href="#panel-content">Skip to panel content</a>

<main class="app-shell" class:sidebar-collapsed={sidebarCollapsed}>
  <IdentityBar
    {viewer}
    {iconUrl}
    {targets}
    {selectedId}
    {targetHref}
    onSelectTarget={(targetId) => void selectTarget(targetId)}
    onSignOut={signOut}
    {view}
    {viewHref}
    onSelectView={selectView}
    showUsers={viewer?.capabilities.manage_global_users === true ||
      selectedTarget?.capabilities.manage_target_users === true}
    showNavigation={viewer !== null && selectedTarget !== null}
    collapsed={sidebarCollapsed}
    onToggleCollapsed={toggleSidebar}
    {theme}
    onSelectTheme={selectTheme}
  />

  <div class="workspace">
    <div id="panel-content" class="workspace-content" tabindex="-1">
      {#if failure !== null}
        <Plate label="Problem" tone="alarm">
          <p>{failure.message}</p>
          <button class="btn" onclick={load}>Try again</button>
        </Plate>
      {/if}

      {#if loading}
        <Plate label="Panel">
          <div class="panel-skeleton" aria-hidden="true">
            <span class="skeleton-line skeleton-title"></span>
            <span class="skeleton-line skeleton-copy"></span>
            <span class="skeleton-line skeleton-control"></span>
            <span class="skeleton-line skeleton-row"></span>
            <span class="skeleton-line skeleton-row"></span>
            <span class="skeleton-line skeleton-row"></span>
          </div>
          <p class="visually-hidden" role="status">Loading panel</p>
        </Plate>
      {:else if viewer === null}
        {#if revokedReason !== null}
          <Plate label="Access revoked" tone="alarm">
            <p>{revokedReason}</p>
            <a class="btn" href={api.signInUrl()}>Sign in</a>
          </Plate>
        {:else if failure === null}
          <SignedOut href={api.signInUrl()} />
        {/if}
      {:else}
        {#if selectedTarget !== null}
          {#if view === 'settings'}
            <div id="settings-panel" aria-labelledby="settings-navigation">
              {#key selectedTarget.id}
                <TargetSettings
                  target={selectedTarget}
                  readOnly={!selectedTarget.capabilities.write}
                  onUpdate={updateTarget}
                />
              {/key}
            </div>
          {:else if view === 'repositories'}
            <div id="repositories-panel" aria-labelledby="repositories-navigation">
              {#key selectedTarget.id}
                <RepositoryList
                  targetId={selectedTarget.id}
                  refreshVersion={repositoryDetailsVersion}
                  fetchPage={fetchRepositories}
                  onLoad={loadRepository}
                  onUpdate={updateRepository}
                  onChanged={() => repositoryChanged(selectedTarget.id)}
                  readOnly={!selectedTarget.capabilities.write}
                />
              {/key}
            </div>
          {:else if view === 'users'}
            <div id="users-panel" aria-labelledby="users-navigation">
              {#key `${selectedTarget.id}:${globalUsers}`}
                <UserManagement
                  scope={globalUsers ? 'global' : 'target'}
                  targetId={selectedTarget.id}
                  targetName={selectedTarget.account.display_name}
                  actorTargetRole={selectedTarget.effective_role}
                  canManageGlobal={viewer.capabilities.manage_global_users}
                  canManageOwners={viewer.capabilities.manage_owners}
                  refreshVersion={userVersion}
                  onScope={selectUserScope}
                  fetchUsers={api.fetchUsers}
                  addUser={api.addUser}
                  updateUser={api.updateUser}
                  fetchTargetUsers={api.fetchTargetUsers}
                  addTargetUser={api.addTargetUser}
                  updateTargetUser={api.updateTargetUser}
                  fetchInvitations={api.fetchInvitations}
                  createInvitation={api.createInvitation}
                  fetchTargetInvitations={api.fetchTargetInvitations}
                  createTargetInvitation={api.createTargetInvitation}
                  reissueInvitation={api.reissueInvitation}
                  revokeInvitation={api.revokeInvitation}
                  fetchUserDecisions={api.fetchUserDecisions}
                />
              {/key}
            </div>
          {:else if view === 'history'}
            <div id="history-panel" aria-labelledby="history-navigation">
              {#key selectedTarget.id}
                <HistoryPanel
                  targetId={selectedTarget.id}
                  refreshVersion={historyVersion}
                  fetchAudit={(request) => api.fetchAudit(selectedTarget.id, request)}
                  fetchFailures={(request) => api.fetchFailures(selectedTarget.id, request)}
                />
              {/key}
            </div>
          {:else}
            <div id="help-panel" aria-labelledby="help-navigation">
              <HelpPanel />
            </div>
          {/if}
        {:else if failure === null}
          <Plate label="No installations">
            <div class="empty-panel-state">
              <span class="empty-panel-mark" aria-hidden="true">+</span>
              <div>
                <strong>Install Smyklot to begin</strong>
                <p class="dim">
                  Install the Smyklot GitHub App on an organization or personal account, then reload
                  this panel
                </p>
              </div>
              <button class="btn btn-signal" type="button" onclick={load}>Reload panel</button>
            </div>
          </Plate>
        {/if}
      {/if}
    </div>

    <PageFooter {build} />
  </div>
</main>

<style>
  .panel-skeleton {
    display: grid;
    gap: var(--space-3);
  }

  .skeleton-line {
    animation: skeleton-pulse 1.35s ease-in-out infinite alternate;
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    display: block;
    height: 2.75rem;
  }

  .skeleton-title {
    height: 1.25rem;
    width: min(14rem, 48%);
  }

  .skeleton-copy {
    height: 0.75rem;
    width: min(28rem, 76%);
  }

  .skeleton-control {
    height: var(--control-height);
    margin-top: var(--space-2);
    width: min(22rem, 100%);
  }

  .skeleton-row {
    height: 3.25rem;
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

  @keyframes skeleton-pulse {
    from {
      opacity: 0.52;
    }

    to {
      opacity: 0.9;
    }
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
