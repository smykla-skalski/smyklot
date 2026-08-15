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
    applyDocumentTheme,
    DEFAULT_THEME_DISPLAY,
    isThemeDisplay,
    type ThemeDisplay,
  } from './lib/preferences';
  import { createPrefsSync, prefText } from './lib/preferences-sync';
  import {
    panelDocumentTitle,
    resolveDocumentTitleRoute,
    resolvePanelRoute,
    rootSection,
    rootSectionRoute,
    type HistorySection,
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
  let requestedDocumentRoute = $state<PanelRoute | null>(null);
  let identityBar = $state<ReturnType<typeof IdentityBar> | null>(null);
  let activeRootRoute = $state<RootRoute>({ rootView: 'overview' });
  /* History's table is part of the address, so a reload lands on the table the
     reader was on rather than snapping back to Audit. */
  let historySection = $state<HistorySection>('audit');
  let streamReady = $state(false);
  let revokedReason = $state<string | null>(null);
  const prefs = createPrefsSync();

  function storedTheme(): ThemeDisplay {
    const value = prefs.get('theme');
    return typeof value === 'string' && isThemeDisplay(value) ? value : DEFAULT_THEME_DISPLAY;
  }

  let sidebarCollapsed = $state(prefs.get('sidebar') === 'collapsed');
  /* Between the mobile bar and full desktop the sidebar auto-collapses to the
     icon rail, so 1024px screens keep navigation instead of the phone layout. */
  const narrowRail = new MediaQuery('(min-width: 48.0625rem) and (max-width: 72rem)');
  const effectiveSidebarCollapsed = $derived(sidebarCollapsed || narrowRail.current);
  let theme = $state<ThemeDisplay>(storedTheme());
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
  const rootRole = $derived(viewer?.system_role === 'super_root' ? 'Super Root' : 'Root');
  const activeDocumentRoute = $derived(
    rootMode
      ? activeRootRoute
      : view === 'history'
        ? { account: '', view, section: historySection }
        : { account: '', view },
  );
  const documentTitle = $derived(
    panelDocumentTitle(
      resolveDocumentTitleRoute(
        activeDocumentRoute,
        requestedDocumentRoute,
        loading || viewer === null || failure?.source === 'load',
      ),
    ),
  );
  const returnTarget = $derived(selectedTarget ?? targets[0] ?? null);
  const tableScrollView = $derived(
    rootMode
      ? rootValue === 'history' ||
          rootValue === 'access' ||
          // The list pins like every other table view; the installation detail
          // page is mixed content and stays in document flow.
          activeRootRoute.rootView === 'installations'
      : selectedTarget !== null &&
          ['repositories', 'users', 'invitations', 'history'].includes(view),
  );

  function forwardTableWheel(event: WheelEvent): void {
    if (
      !tableScrollView ||
      !window.matchMedia('(min-width: 48.0625rem)').matches ||
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
    requestedDocumentRoute = router.current();
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
      prefs.adoptAccount(viewer.account.id);
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
      if (requested.rootView === 'installation' && requested.section !== undefined) {
        historySection = requested.section;
      }
      /* Canonicalise: a history address always names its table, so a reload has
         nothing to guess at. */
      const canonicalRoot: RootRoute =
        requested.rootView === 'installation' &&
        requested.view === 'history' &&
        requested.section === undefined
          ? { ...requested, section: historySection }
          : requested;
      activeRootRoute = canonicalRoot;
      if (navigation === 'push') {
        router.push(canonicalRoot);
      } else if (navigation === 'replace' || canonicalRoot !== requested) {
        router.replace(canonicalRoot);
      }
      return;
    }

    const installationRoute = requested !== null && !('rootView' in requested) ? requested : null;
    const lastInstallation = prefText(prefs.get('last_installation'));
    const resolved = resolvePanelRoute(
      targets.map((target) => target.account.login),
      installationRoute,
      lastInstallation === '' ? null : lastInstallation,
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
    if (resolved.section !== undefined) historySection = resolved.section;
    prefs.set('last_installation', target.account.login);
    const canonical = routeFor(target, resolved.view, resolved.section ?? historySection);

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

  function routeFor(
    target: PanelTarget,
    nextView: PanelView,
    section: HistorySection = historySection,
  ): PanelRoute {
    return nextView === 'history'
      ? { account: target.account.login, view: nextView, section }
      : { account: target.account.login, view: nextView };
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

  function rootAuditHref(): string {
    return router.path({ rootView: 'history-audit' });
  }

  function rootFailuresHref(): string {
    return router.path({ rootView: 'history-failures' });
  }

  function rootInstallationHref(account: string, nextView: ScopedPanelView): string {
    return router.path(rootInstallationRoute(account, nextView));
  }

  function rootInstallationRoute(
    account: string,
    nextView: ScopedPanelView,
    section: HistorySection = historySection,
  ): RootRoute {
    return nextView === 'history'
      ? { rootView: 'installation', account, view: nextView, section }
      : { rootView: 'installation', account, view: nextView };
  }

  function selectRootInstallation(account: string, nextView: ScopedPanelView): void {
    const route = rootInstallationRoute(account, nextView);
    activeRootRoute = route;
    router.push(route);
    resetPageScroll();
  }

  function selectRootInstallationHistory(section: HistorySection): void {
    if (activeRootRoute.rootView !== 'installation' || historySection === section) return;
    historySection = section;
    const route = rootInstallationRoute(activeRootRoute.account, 'history', section);
    activeRootRoute = route;
    router.push(route);
  }

  function selectRootInstallations(): void {
    selectRootView({ rootView: 'installations' });
  }

  function selectRootView(route: RootRoute): void {
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
        prefs.set('last_installation', selectedTarget.account.login);
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

  function selectHistorySection(section: HistorySection): void {
    const target = selectedTarget;
    if (target === null || historySection === section) return;
    historySection = section;
    router.push(routeFor(target, 'history', section));
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
    const stream = api.openStream(
      {
        onResync: refreshAccessFromStream,
        onChange: (event) =>
          event.type === 'access.changed' ? refreshAccessFromStream() : refreshFromStreamSafely(),
        onRevoked: (event) => revokeAccess(event.reason),
        onPrefsReady: (info) => prefs.onPrefsReady(info),
        onPrefsChanged: (event) => prefs.onPrefsChanged(event),
        onPrefsRejected: (keys) => prefs.onPrefsRejected(keys),
      },
      prefs.dialQuery,
    );
    prefs.attach(stream.send);
    return () => {
      prefs.detach();
      stream.stop();
    };
  });

  // Preference changes from the user's other sessions apply live to the
  // app-level controls; table state is read at mount instead, so remote
  // changes there land on the next remount rather than mid-interaction.
  $effect(() =>
    prefs.subscribe((keys) => {
      if (keys.includes('theme')) theme = storedTheme();
      if (keys.includes('sidebar')) sidebarCollapsed = prefs.get('sidebar') === 'collapsed';
    }),
  );

  $effect(() =>
    router.subscribe((route) => {
      requestedDocumentRoute = route;
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
    prefs.set('sidebar', sidebarCollapsed ? 'collapsed' : 'expanded');
  }

  function selectTheme(nextTheme: ThemeDisplay): void {
    theme = nextTheme;
    prefs.set('theme', nextTheme);
  }

  $effect(() => {
    applyDocumentTheme(document, resolvedTheme, rootMode);
  });

  void load();
</script>

<svelte:head>
  <title>{documentTitle}</title>
</svelte:head>

<a class="skip-link" href="#panel-content">Skip to panel content</a>

<main
  class="app-shell"
  class:sidebar-collapsed={effectiveSidebarCollapsed}
  class:root-mode={rootMode}
>
  <IdentityBar
    bind:this={identityBar}
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
    collapsed={effectiveSidebarCollapsed}
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

      <!-- Only when there is no workspace to show. Signing out holds the one it
           has until the server confirms, rather than blanking on the request. -->
      {#if loading && viewer === null}
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
          class:root-table-view={tableScrollView}
          aria-labelledby="root-page-heading"
        >
          {#if rootValue === 'overview'}
            <RootOverview
              {api}
              {rootRole}
              refreshVersion={rootDataVersion}
              installationsHref={rootInstallationsHref()}
              elevationsHref={rootAuditHref()}
              failuresHref={rootFailuresHref()}
              onOpenInstallations={selectRootInstallations}
              onOpenElevations={() => selectRootView({ rootView: 'history-audit' })}
              onOpenFailures={() => selectRootView({ rootView: 'history-failures' })}
              onOpenInbox={() => identityBar?.openInbox()}
            />
          {:else if rootValue === 'installations'}
            <RootInstallations
              route={activeRootRoute}
              {api}
              {rootRole}
              actorLogin={viewer.account.login}
              refreshVersion={rootDataVersion}
              listHref={rootInstallationsHref()}
              hrefFor={rootInstallationHref}
              onList={selectRootInstallations}
              onNavigate={selectRootInstallation}
              {historySection}
              onHistorySection={selectRootInstallationHistory}
            />
          {:else if rootValue === 'history'}
            <HistoryPanel
              context="root"
              targetId="root"
              {rootRole}
              section={activeRootRoute.rootView === 'history-failures' ? 'failures' : 'audit'}
              onSection={selectRootHistorySection}
              refreshVersion={rootDataVersion}
              fetchAudit={api.fetchRootAudit}
              fetchFailures={api.fetchRootFailures}
            />
          {:else if rootValue === 'access'}
            <RootAccess
              {rootRole}
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
              actorLogin={viewer.account.login}
              fetchInstallations={api.fetchRootInstallations}
              addInstallationUser={api.addRootTargetUser}
              onOpenInstallationAccess={(account) => selectRootInstallation(account, 'users')}
            />
          {:else}
            <RootSettings
              {rootRole}
              refreshVersion={runtimeSettingsVersion}
              fetchSettings={api.fetchRootRuntimeSettings}
              updateSettings={api.updateRootRuntimeSettings}
            />
          {/if}
        </section>
      {:else}
        {#if selectedTarget !== null}
          {#if view === 'settings'}
            <div id="settings-panel">
              {#key selectedTarget.id}
                <TargetSettings
                  target={selectedTarget}
                  readOnly={!selectedTarget.capabilities.write}
                  onUpdate={updateTarget}
                />
              {/key}
            </div>
          {:else if view === 'repositories'}
            <div id="repositories-panel">
              {#key selectedTarget.id}
                <RepositoryList
                  targetId={selectedTarget.id}
                  defaultEnabled={selectedTarget.repository_default_enabled}
                  refreshVersion={repositoryDetailsVersion}
                  fetchPage={fetchRepositories}
                  onLoad={loadRepository}
                  onUpdate={updateRepository}
                  onChanged={() => repositoryChanged(selectedTarget.id)}
                  readOnly={!selectedTarget.capabilities.write}
                  {prefs}
                />
              {/key}
            </div>
          {:else if isAccessView(view)}
            <div id="access-panel">
              {#key selectedTarget.id}
                <UserManagement
                  section={view}
                  {prefs}
                  targetId={selectedTarget.id}
                  targetName={selectedTarget.account.display_name}
                  actorLogin={viewer.account.login}
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
            <div id="history-panel">
              {#key selectedTarget.id}
                <HistoryPanel
                  targetId={selectedTarget.id}
                  refreshVersion={historyVersion}
                  section={historySection}
                  onSection={selectHistorySection}
                  fetchAudit={(request) => api.fetchAudit(selectedTarget.id, request)}
                  fetchFailures={(request) => api.fetchFailures(selectedTarget.id, request)}
                  {prefs}
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
    gap: 0;
  }

  .root-workspace.root-table-view {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
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
