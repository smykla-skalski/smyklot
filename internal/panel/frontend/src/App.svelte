<script lang="ts">
  import HistoryPanel from './components/HistoryPanel.svelte';
  import IdentityBar from './components/IdentityBar.svelte';
  import PageFooter from './components/PageFooter.svelte';
  import Plate from './components/Plate.svelte';
  import RepositoryList from './components/RepositoryList.svelte';
  import RootAccess from './components/RootAccess.svelte';
  import RootInstallations from './components/RootInstallations.svelte';
  import RootOverview from './components/RootOverview.svelte';
  import RootSettings from './components/RootSettings.svelte';
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
    rootSection,
    rootSectionRoute,
    type PanelRoute,
    type PanelRouter,
    type PanelView,
    type RootRoute,
    type RootSection,
    type ScopedPanelView,
  } from './lib/routes';
  import type {
    PanelTarget,
    PanelViewer,
    RepositoryDetail,
    RepositoryPageRequest,
    RepositorySettingsInput,
    TargetSettingsInput,
  } from './lib/types';
  import { MediaQuery } from 'svelte/reactivity';

  type FailureSource = 'load' | 'sign-out' | 'stream';
  type PanelFailure = { message: string; source: FailureSource };

  const { api, build, router }: { api: PanelApi; build: PanelBuild; router: PanelRouter } =
    $props();

  let loading = $state(true);
  let viewer = $state<PanelViewer | null>(null);
  let targets = $state<PanelTarget[]>([]);
  let selectedId = $state<string | null>(null);
  let failure = $state<PanelFailure | null>(null);
  let historyVersion = $state(0);
  let repositoryDetailsVersion = $state(0);
  let userVersion = $state(0);
  let notificationVersion = $state(0);
  let runtimeSettingsVersion = $state(0);
  let rootDataVersion = $state(0);
  let view = $state<PanelView>('settings');
  let rootMode = $state(false);
  let activeRootRoute = $state<RootRoute>({ rootView: 'overview' });
  let streamReady = $state(false);
  let revokedReason = $state<string | null>(null);
  let sidebarCollapsed = $state(readSidebarDisplay() === 'collapsed');
  let theme = $state<ThemeDisplay>(readThemeDisplay());
  const systemDarkTheme = new MediaQuery('prefers-color-scheme: dark');
  const resolvedTheme = $derived(
    theme === 'system' && systemDarkTheme.current ? 'dark' : theme === 'system' ? 'light' : theme,
  );
  const targetReads = new LatestRequest();
  const streamRefreshes = new LatestRequest();

  const selectedTarget = $derived(
    selectedId === null ? null : (targets.find((target) => target.id === selectedId) ?? null),
  );
  const rootValue = $derived(rootSection(activeRootRoute));
  const returnTarget = $derived(selectedTarget ?? targets[0] ?? null);
  const tableScrollView = $derived(
    rootMode
      ? rootValue === 'history' || rootValue === 'access'
      : selectedTarget !== null &&
          ['repositories', 'users', 'invitations', 'history'].includes(view),
  );

  function forwardTableWheel(event: WheelEvent): void {
    if (
      !tableScrollView ||
      !window.matchMedia('(min-width: 64.001rem)').matches ||
      event.defaultPrevented ||
      event.ctrlKey ||
      event.deltaY === 0 ||
      Math.abs(event.deltaX) > Math.abs(event.deltaY)
    )
      return;

    const target = event.target;
    if (
      target instanceof Element &&
      target.closest('[data-panel-scroll], [role="dialog"], [role="menu"]') !== null
    )
      return;

    const workspace = event.currentTarget as HTMLDivElement;
    const scroller = workspace.querySelector<HTMLElement>('[data-panel-scroll]');
    if (scroller === null || scroller.scrollHeight <= scroller.clientHeight) return;

    const previous = scroller.scrollTop;
    const delta =
      event.deltaMode === 1
        ? event.deltaY * 16
        : event.deltaMode === 2
          ? event.deltaY * scroller.clientHeight
          : event.deltaY;
    scroller.scrollTop += delta;
    if (scroller.scrollTop !== previous) event.preventDefault();
  }

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
    await activateRoute(routeFor(target, view), 'push');
  }

  function selectView(nextView: PanelView): void {
    const target = selectedTarget;
    if (target === null || view === nextView) return;
    view = nextView;
    router.push(routeFor(target, nextView));
  }

  async function activateRoute(
    requested: PanelRoute | null,
    navigation: 'none' | 'push' | 'replace',
  ): Promise<void> {
    if (requested !== null && 'rootView' in requested && viewer?.system_role !== 'none') {
      rootMode = true;
      activeRootRoute = requested;
      if (navigation === 'push') {
        router.push(requested);
      } else if (navigation === 'replace') {
        router.replace(requested);
      }
      return;
    }

    const installationRoute = requested !== null && !('rootView' in requested) ? requested : null;
    const resolved = resolvePanelRoute(
      targets.map((target) => target.account.login),
      installationRoute,
      readLastInstallation(),
    );
    if (resolved === null) {
      selectedId = null;
      return;
    }

    const target = targetForAccount(resolved.account);
    if (target === undefined) return;

    const targetChanged = selectedId !== target.id;
    rootMode = false;
    selectedId = target.id;
    view = resolved.view;
    writeLastInstallation(target.account.login);
    const canonical = routeFor(target, resolved.view);

    if (navigation === 'push') {
      router.push(canonical);
    } else if (navigation === 'replace' || !sameRoute(installationRoute, canonical)) {
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
    return { account: target.account.login, view: nextView };
  }

  function targetHref(target: PanelTarget): string {
    return router.path(routeFor(target, view));
  }

  function viewHref(nextView: PanelView): string {
    const target = selectedTarget;
    return target === null ? '#' : router.path(routeFor(target, nextView));
  }

  function rootHrefFor(section: RootSection): string {
    return router.path(rootSectionRoute(section));
  }

  function rootDashboardHref(): string {
    return router.path({ rootView: 'overview' });
  }

  function rootInstallationsHref(): string {
    return router.path({ rootView: 'installations' });
  }

  function rootFailuresHref(): string {
    return router.path({ rootView: 'history-failures' });
  }

  function rootInstallationHref(account: string, nextView: ScopedPanelView): string {
    return router.path({ rootView: 'installation', account, view: nextView });
  }

  function selectRootInstallation(account: string, nextView: ScopedPanelView): void {
    const route: RootRoute = { rootView: 'installation', account, view: nextView };
    activeRootRoute = route;
    router.push(route);
    resetPageScroll();
  }

  function selectRootInstallations(): void {
    const route: RootRoute = { rootView: 'installations' };
    activeRootRoute = route;
    router.push(route);
    resetPageScroll();
  }

  function resetPageScroll(): void {
    queueMicrotask(() => window.scrollTo({ top: 0, left: 0 }));
  }

  function returnHref(): string {
    return returnTarget === null ? '#' : router.path(routeFor(returnTarget, view));
  }

  function sameRoute(left: PanelRoute | null, right: PanelRoute): boolean {
    return left !== null && router.path(left) === router.path(right);
  }

  function selectRootSection(section: RootSection): void {
    if (!rootMode || rootValue === section) return;
    const route = rootSectionRoute(section);
    activeRootRoute = route;
    router.push(route);
    resetPageScroll();
  }

  function selectRootHistorySection(section: 'audit' | 'failures'): void {
    const route: RootRoute = {
      rootView: section === 'audit' ? 'history-audit' : 'history-failures',
    };
    if (activeRootRoute.rootView === route.rootView) return;
    activeRootRoute = route;
    router.push(route);
  }

  function selectRootAccessSection(section: 'users' | 'invitations'): void {
    const route: RootRoute = {
      rootView: section === 'users' ? 'access-users' : 'access-invitations',
    };
    if (activeRootRoute.rootView === route.rootView) return;
    activeRootRoute = route;
    router.push(route);
  }

  function enterRoot(): void {
    if (viewer?.system_role === 'none') return;
    const route: RootRoute = { rootView: 'overview' };
    rootMode = true;
    activeRootRoute = route;
    router.push(route);
    resetPageScroll();
  }

  function returnToPanel(): void {
    const target = returnTarget;
    if (target === null) return;
    void activateRoute(routeFor(target, view), 'push');
  }

  function rootPageTitle(route: RootRoute): string {
    if (route.rootView === 'overview') return 'Overview';
    if (route.rootView === 'installations' || route.rootView === 'installation')
      return 'Installations';
    if (route.rootView === 'access-users' || route.rootView === 'access-invitations')
      return 'Access';
    if (route.rootView === 'history-audit' || route.rootView === 'history-failures')
      return 'History';
    return 'Settings';
  }

  function rootPageDescription(route: RootRoute): string {
    if (route.rootView === 'overview')
      return 'Application health, ownership, and security activity';
    if (route.rootView === 'installations' || route.rootView === 'installation')
      return 'Every GitHub installation connected to Smyklot';
    if (route.rootView === 'access-users' || route.rootView === 'access-invitations')
      return 'Application accounts, invitations, and system roles';
    if (route.rootView === 'history-audit' || route.rootView === 'history-failures')
      return 'Application-wide audit events and failures';
    return 'Runtime behavior and deployment-backed defaults';
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
      if (rootMode && viewer?.system_role === 'none') {
        rootMode = false;
        await activateRoute(null, 'replace');
      } else if (rootMode) {
        router.replace(activeRootRoute);
      } else if (selectedId === null) {
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
      notificationVersion += 1;
      runtimeSettingsVersion += 1;
      rootDataVersion += 1;
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

  function selectUserSection(section: 'users' | 'invitations'): void {
    if (!isAccessView(view) || view === section || selectedTarget === null) return;
    view = section;
    router.push(routeFor(selectedTarget, section));
  }

  function isAccessView(candidate: PanelView): candidate is 'users' | 'invitations' {
    return candidate === 'users' || candidate === 'invitations';
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
    writeThemeDisplay(nextTheme);
  }

  $effect(() => {
    document.documentElement.dataset.theme = resolvedTheme;
  });

  void load();
</script>

<a class="skip-link" href="#panel-content">Skip to panel content</a>

<main class="app-shell" class:sidebar-collapsed={sidebarCollapsed} class:root-mode={rootMode}>
  <IdentityBar
    {viewer}
    {targets}
    {selectedId}
    {targetHref}
    onSelectTarget={(targetId) => void selectTarget(targetId)}
    onSignOut={signOut}
    {view}
    {viewHref}
    onSelectView={selectView}
    showUsers={selectedTarget?.capabilities.manage_target_users === true}
    showNavigation={viewer !== null && (rootMode || selectedTarget !== null)}
    collapsed={sidebarCollapsed}
    onToggleCollapsed={toggleSidebar}
    {theme}
    onSelectTheme={selectTheme}
    {rootMode}
    {rootValue}
    {rootHrefFor}
    onSelectRoot={selectRootSection}
    rootDashboardHref={rootDashboardHref()}
    onEnterRoot={enterRoot}
    returnHref={returnHref()}
    onReturnToPanel={returnToPanel}
    fetchNotifications={api.fetchNotifications}
    markNotificationRead={api.markNotificationRead}
    {notificationVersion}
  />

  <div class="workspace" class:table-scroll-view={tableScrollView} onwheel={forwardTableWheel}>
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
      {:else if rootMode}
        <section
          class="root-workspace"
          class:root-table-view={rootValue === 'history' || rootValue === 'access'}
          aria-labelledby="root-page-heading"
        >
          <header class="root-page-header">
            <div>
              <p class="root-eyebrow">
                Root mode · {viewer.system_role === 'super_root' ? 'Super Root' : 'Root'}
              </p>
              <h2 id="root-page-heading">{rootPageTitle(activeRootRoute)}</h2>
              <p>{rootPageDescription(activeRootRoute)}</p>
            </div>
            <span class="root-boundary">Application scope</span>
          </header>

          {#if rootValue === 'overview'}
            <RootOverview
              {api}
              refreshVersion={rootDataVersion}
              installationsHref={rootInstallationsHref()}
              failuresHref={rootFailuresHref()}
            />
          {:else if rootValue === 'installations'}
            <RootInstallations
              route={activeRootRoute}
              {api}
              refreshVersion={rootDataVersion}
              listHref={rootInstallationsHref()}
              hrefFor={rootInstallationHref}
              onList={selectRootInstallations}
              onNavigate={selectRootInstallation}
            />
          {:else if rootValue === 'history'}
            <HistoryPanel
              context="root"
              targetId="root"
              section={activeRootRoute.rootView === 'history-failures' ? 'failures' : 'audit'}
              onSection={selectRootHistorySection}
              refreshVersion={rootDataVersion}
              fetchAudit={api.fetchRootAudit}
              fetchFailures={api.fetchRootFailures}
            />
          {:else if rootValue === 'access'}
            <RootAccess
              section={activeRootRoute.rootView === 'access-invitations' ? 'invitations' : 'users'}
              refreshVersion={rootDataVersion}
              onSection={selectRootAccessSection}
              fetchUsers={api.fetchRootUsers}
              updateUser={api.updateRootUser}
              fetchInvitations={api.fetchRootInvitations}
              createInvitation={api.createRootInvitation}
              reissueInvitation={api.reissueRootInvitation}
              revokeInvitation={api.revokeRootInvitation}
              canManageInvitations={viewer.system_role === 'super_root'}
              fetchInstallations={api.fetchRootInstallations}
              addInstallationUser={api.addRootTargetUser}
              onOpenInstallationAccess={(account) => selectRootInstallation(account, 'users')}
            />
          {:else}
            <RootSettings
              refreshVersion={runtimeSettingsVersion}
              fetchSettings={api.fetchRootRuntimeSettings}
              updateSettings={api.updateRootRuntimeSettings}
            />
          {/if}
        </section>
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
          {:else if isAccessView(view)}
            <div id="access-panel" aria-labelledby="users-navigation">
              {#key selectedTarget.id}
                <UserManagement
                  section={view}
                  targetId={selectedTarget.id}
                  targetName={selectedTarget.account.display_name}
                  actorTargetRole={selectedTarget.effective_role}
                  refreshVersion={userVersion}
                  onSection={selectUserSection}
                  fetchTargetUsers={api.fetchTargetUsers}
                  addTargetUser={api.addTargetUser}
                  updateTargetUser={api.updateTargetUser}
                  fetchTargetInvitations={api.fetchTargetInvitations}
                  createTargetInvitation={api.createTargetInvitation}
                  reissueInvitation={api.reissueTargetInvitation}
                  revokeInvitation={api.revokeTargetInvitation}
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
  #repositories-panel,
  #access-panel,
  #history-panel {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }

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

  .root-workspace {
    display: grid;
    gap: var(--space-6);
  }

  .root-workspace.root-table-view {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }

  .root-page-header {
    align-items: end;
    display: flex;
    gap: var(--space-6);
    justify-content: space-between;
  }

  .root-page-header h2 {
    font-size: clamp(1.55rem, 2.4vw, 2rem);
    letter-spacing: -0.035em;
    line-height: 1.05;
    margin: 0;
  }

  .root-page-header p:not(.root-eyebrow) {
    color: var(--text-secondary);
    margin: var(--space-2) 0 0;
  }

  .root-eyebrow,
  .root-boundary {
    color: #6d54bd;
    font: 700 var(--font-size-compact) / 1 var(--sans);
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .root-eyebrow {
    margin: 0 0 var(--space-3);
  }

  .root-boundary {
    background: color-mix(in srgb, #8b5cf6 10%, var(--surface-base));
    border: 1px solid color-mix(in srgb, #8b5cf6 28%, var(--border-subtle));
    border-radius: var(--radius-control);
    color: color-mix(in srgb, #6d54bd 82%, var(--text-primary));
    padding: var(--space-2) var(--space-3);
    white-space: nowrap;
  }

  :global(:root[data-theme='dark']) .root-eyebrow,
  :global(:root[data-theme='dark']) .root-boundary {
    color: #c4b5fd;
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

    .root-page-header {
      align-items: start;
      flex-direction: column;
      gap: var(--space-3);
    }
  }
</style>
