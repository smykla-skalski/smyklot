<script lang="ts">
  import HistoryPanel from './components/HistoryPanel.svelte';
  import IdentityBar from './components/IdentityBar.svelte';
  import InboxView from './components/InboxView.svelte';
  import PageFooter from './components/PageFooter.svelte';
  import Plate from './components/Plate.svelte';
  import RepositoryList from './components/RepositoryList.svelte';
  import RootAccess from './components/RootAccess.svelte';
  import RootInstallations from './components/RootInstallations.svelte';
  import RootOverview from './components/RootOverview.svelte';
  import RootSettings from './components/RootSettings.svelte';
  import NightPage from './components/NightPage.svelte';
  import SignInPage from './components/SignInPage.svelte';
  import TargetSettings from './components/TargetSettings.svelte';
  import UserManagement from './components/UserManagement.svelte';
  import type { PanelApi } from './lib/api';
  import type { PanelBuild } from './lib/base';
  import { dialogRoute, legacyInboxRoute } from './lib/dialog-route.svelte';
  import { LatestRequest } from './lib/latest-request';
  import {
    applyDocumentTheme,
    DEFAULT_THEME_DISPLAY,
    isThemeDisplay,
    type ThemeDisplay,
  } from './lib/preferences';
  import type { SessionEnded } from './lib/panel-session';
  import { createPrefsSync, prefText } from './lib/preferences-sync';
  import {
    panelDocumentTitle,
    resolveDocumentTitleRoute,
    resolvePanelRoute,
    rootSection,
    rootSectionRoute,
    type HistorySection,
    type InstallationRoute,
    type PanelRoute,
    type PanelRouter,
    type PanelView,
    type PersonalView,
    type ResolvedPanelRoute,
    type RootRoute,
    type RootSection,
    type RouteDialog,
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

  const {
    api,
    base,
    build,
    router,
  }: { api: PanelApi; base: string; build: PanelBuild; router: PanelRouter } = $props();

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
  /**
   * The reader's own page, when one is open over everything else.
   *
   * Not a view of the workspace and not a section of the console: while it is set
   * it decides what the workspace shows, and the sidebar around it keeps the
   * workspace it was reached from so there is somewhere to go back to.
   */
  let personalView = $state<PersonalView | null>(null);
  let notificationUnread = $state(0);
  let requestedDocumentRoute = $state<PanelRoute | null>(null);
  let activeRootRoute = $state<RootRoute>({ rootView: 'overview' });
  /* History's table is part of the address, so a reload lands on the table the
     reader was on rather than snapping back to Audit. */
  let historySection = $state<HistorySection>('audit');
  let streamReady = $state(false);
  /* Why the session went, when it went while someone was using it. Kept as the
     code as well as the words, because pressing Sign out and having an account
     removed are not the same event and must not read as the same page. */
  let sessionEnded = $state<SessionEnded | null>(null);
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
  const notificationReads = new LatestRequest();

  const selectedTarget = $derived(
    selectedId === null ? null : (targets.find((target) => target.id === selectedId) ?? null),
  );
  const rootValue = $derived(rootSection(activeRootRoute));
  const rootRole = $derived(viewer?.system_role === 'super_root' ? 'Super Root' : 'Root');
  const activeDocumentRoute = $derived<PanelRoute>(
    personalView !== null
      ? { personal: personalView }
      : rootMode
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
  /* There is a panel to show only once the viewer is known. Waiting counts as
     neither, so the shell holds until then rather than flashing a front door at
     someone who turns out to have a session. */
  const signedOut = $derived(!loading && viewer === null && failure === null);

  /* Signed in, nothing installed, and nothing else to be doing here: not a Root
     with a console to reach, and no failure whose retry is the way out. */
  const awaitingInstallation = $derived(
    !loading &&
      viewer !== null &&
      failure === null &&
      targets.length === 0 &&
      !rootMode &&
      viewer.system_role === 'none',
  );
  const returnTarget = $derived(selectedTarget ?? targets[0] ?? null);
  const tableScrollView = $derived(
    personalView !== null
      ? // The inbox is a feed and scrolls as a document; nothing is pinned.
        false
      : rootMode
        ? rootValue === 'history' ||
          rootValue === 'access' ||
          // The list pins like every other table view; the installation detail
          // page is mixed content and stays in document flow.
          activeRootRoute.rootView === 'installations'
        : selectedTarget !== null &&
          ['repositories', 'users', 'invitations', 'history'].includes(view),
  );

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
      // Signed in again, so whatever ended the last session is history.
      sessionEnded = null;
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
    if (target === null || (view === nextView && personalView === null)) return;
    view = nextView;
    navigate(routeFor(target, nextView));
  }

  /**
   * Every push goes through here, so one place decides whether a personal page is
   * still showing.
   *
   * It used to be `router.push` at a dozen call sites. Adding a page that stands
   * over the workspace to that shape means every one of them has to remember to
   * take it down, and the one that forgets leaves the inbox on screen with the
   * address of somewhere else.
   */
  function navigate(route: PanelRoute): void {
    personalView = 'personal' in route ? route.personal : null;
    router.push(route);
  }

  /* Through `activateRoute` rather than straight to the router, because arriving
     at a personal page is more than a push: it leaves the console and settles a
     workspace underneath, and that has to be the same whether the reader pressed
     the row or opened the address. */
  function selectPersonal(next: PersonalView): void {
    if (personalView === next) return;
    void activateRoute({ personal: next }, 'push');
    resetPageScroll();
  }

  async function activateRoute(
    requested: PanelRoute | null,
    navigation: 'none' | 'push' | 'replace',
  ): Promise<void> {
    if (requested !== null && 'personal' in requested) {
      /* A page of the reader's own leaves the console rather than hiding inside
         it: the address says nothing about a console, so a reload could not put
         one back, and the same address has to look the same both times.

         The workspace context is still settled underneath, or the sidebar beside
         the page would carry a switcher naming nothing and view links leading
         nowhere. */
      personalView = requested.personal;
      rootMode = false;
      settleInstallationContext();
      if (navigation === 'push') router.push(requested);
      else if (navigation === 'replace') router.replace(requested);
      return;
    }
    personalView = null;

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
    const resolved = resolveWorkspace(installationRoute);
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
    /* The dialog rides along. Canonicalising is about the view - `/root/access`
       becoming `/root/access/users`, a remembered installation filling in - and
       must not throw away what the address said was open on top of it, which is
       what a pasted link to a dialog is entirely made of. */
    const canonical = routeFor(
      target,
      resolved.view,
      resolved.section ?? historySection,
      installationRoute?.dialog,
    );

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

  /**
   * Which workspace an address means: its own account, or the last one the reader
   * was in.
   *
   * Both the address of a workspace view and a personal page that names none need
   * the same answer, and each had its own copy of the call.
   */
  function resolveWorkspace(requested: InstallationRoute | null): ResolvedPanelRoute | null {
    const lastInstallation = prefText(prefs.get('last_installation'));

    return resolvePanelRoute(
      targets.map((target) => target.account.login),
      requested,
      lastInstallation === '' ? null : lastInstallation,
    );
  }

  /**
   * Picks up the workspace the reader was last in, without navigating to it.
   *
   * A personal page names no workspace, so arriving at one directly would leave
   * the sidebar with nothing selected and every view link pointing at `#`. This
   * settles the same context the panel would have resolved for a bare address,
   * and the address stays the personal one.
   */
  function settleInstallationContext(): void {
    const resolved = resolveWorkspace(null);
    if (resolved === null) return;
    const target = targetForAccount(resolved.account);
    // Already in a workspace: keep the view the reader was reading, not the default.
    if (target === undefined || selectedId === target.id) return;
    selectedId = target.id;
    view = resolved.view;
    prefs.set('last_installation', target.account.login);
    repositoryDetailsVersion += 1;
    historyVersion += 1;
    userVersion += 1;
  }

  function routeFor(
    target: PanelTarget,
    nextView: PanelView,
    section: HistorySection = historySection,
    dialog?: RouteDialog,
  ): PanelRoute {
    const route: PanelRoute =
      nextView === 'history'
        ? { account: target.account.login, view: nextView, section }
        : { account: target.account.login, view: nextView };

    return dialog === undefined ? route : { ...route, dialog };
  }

  /** What the address currently says is open, so a rewrite of it can say so too. */
  function currentDialog(): RouteDialog | undefined {
    const route = router.current();

    return route !== null && 'dialog' in route ? route.dialog : undefined;
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

  function inboxHref(): string {
    return router.path({ personal: 'inbox' });
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
    navigate(route);
    resetPageScroll();
  }

  function selectRootInstallationHistory(section: HistorySection): void {
    if (activeRootRoute.rootView !== 'installation' || historySection === section) return;
    historySection = section;
    const route = rootInstallationRoute(activeRootRoute.account, 'history', section);
    activeRootRoute = route;
    navigate(route);
  }

  function selectRootInstallations(): void {
    selectRootView({ rootView: 'installations' });
  }

  function selectRootView(route: RootRoute): void {
    activeRootRoute = route;
    navigate(route);
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
    navigate(route);
    resetPageScroll();
  }

  function selectRootHistorySection(section: 'audit' | 'failures'): void {
    const route: RootRoute = {
      rootView: section === 'audit' ? 'history-audit' : 'history-failures',
    };
    if (activeRootRoute.rootView === route.rootView) return;
    activeRootRoute = route;
    navigate(route);
  }

  function selectRootAccessSection(section: 'users' | 'invitations'): void {
    const route: RootRoute = {
      rootView: section === 'users' ? 'access-users' : 'access-invitations',
    };
    if (activeRootRoute.rootView === route.rootView) return;
    activeRootRoute = route;
    navigate(route);
  }

  function enterRoot(): void {
    if (viewer?.system_role === 'none') return;
    const route: RootRoute = { rootView: 'overview' };
    rootMode = true;
    activeRootRoute = route;
    navigate(route);
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
          // The stream said something changed and the session went with it, so this
          // is what the panel worked out rather than what it was told.
          revokeAccess({ code: 'access_revoked', reason: '' });
          return;
        }
        viewer = currentViewer;
      }
      if (!(await refreshTargets()) || !streamRefreshes.isCurrent(refresh)) return;
      if (rootMode && viewer?.system_role === 'none') {
        rootMode = false;
        await activateRoute(null, 'replace');
      } else if (personalView !== null) {
        /* The reader is on a page of their own. Rewriting the address because the
           stream said something changed must not walk them off it, which writing
           the workspace underneath would do. */
        router.replace({ personal: personalView });
      } else if (rootMode) {
        router.replace(activeRootRoute);
      } else if (selectedId === null) {
        await activateRoute(router.current(), 'replace');
      } else if (selectedTarget !== null) {
        prefs.set('last_installation', selectedTarget.account.login);
        /* Rewriting the address because the stream said something changed must
           not close what the reader has open on top of it. */
        router.replace(routeFor(selectedTarget, view, historySection, currentDialog()));
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

  /**
   * The count on the sidebar's Inbox row, read without reading the inbox.
   *
   * The page reports its own count back while it is open, which is what makes
   * marking something read take the badge down; this is for every other moment,
   * when nothing on screen is asking the server about notifications.
   */
  async function refreshNotificationUnread(): Promise<void> {
    const read = notificationReads.begin();
    try {
      const page = await api.fetchNotifications({ limit: 1 });
      /* A count that was already old when it arrived must not replace a newer
         one. The reader who opens the inbox while this is in flight gets the
         page's own count first, and this landing after it would put the badge
         back to what it was before they read anything. */
      if (!notificationReads.isCurrent(read)) return;
      notificationUnread = page.unread;
    } catch {
      /* A badge is not worth a failure of its own. The count keeps the last number
         it knew rather than the panel growing an error about a sidebar dot. */
    }
  }

  function refreshAccessFromStream(): void {
    void refreshFromStream(true);
  }

  function selectHistorySection(section: HistorySection): void {
    const target = selectedTarget;
    if (target === null || historySection === section) return;
    historySection = section;
    navigate(routeFor(target, 'history', section));
  }

  function selectUserSection(section: 'users' | 'invitations'): void {
    if (!isAccessView(view) || view === section || selectedTarget === null) return;
    view = section;
    navigate(routeFor(selectedTarget, section));
  }

  function isAccessView(candidate: PanelView): candidate is 'users' | 'invitations' {
    return candidate === 'users' || candidate === 'invitations';
  }

  function revokeAccess(ended: SessionEnded): void {
    sessionEnded = ended;
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
        onRevoked: (event) => revokeAccess(event),
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

  /* Any refused request means the session is gone, wherever in the panel it came
     from. Without this a reader sat inside a workspace that could no longer load
     anything, reading "sign in to use the panel" over stale rows, with a Try
     again button that could only fail the same way. */
  $effect(() =>
    api.onSessionLost((code) => {
      if (viewer === null) return;
      revokeAccess({ code, reason: '' });
    }),
  );

  $effect(() =>
    router.subscribe((route) => {
      requestedDocumentRoute = route;
      if (viewer !== null && !loading) void activateRoute(route, 'none');
    }),
  );

  /* Keyed to the account rather than to the viewer object, which the stream
     replaces whenever anything about access changes: the count is the same count
     until somebody else is signed in. */
  const viewerAccountId = $derived(viewer?.account.id ?? null);

  /* Not while the inbox is open. The page reads the same endpoint and reports the
     same count back, so asking here as well is the one request that answers a
     question already being answered - twice over on every stream event. */
  $effect(() => {
    if (viewerAccountId === null || personalView === 'inbox' || notificationVersion < 0) return;
    void refreshNotificationUnread();
  });

  /* Dialogs live in the query string, which the panel's own route writer drops
     whenever it writes a path. That is the behaviour we want - walking to another
     view closes what was open on top of the old one - so the two routers share the
     address without needing to know about each other. */
  $effect(() => dialogRoute.attach(window, base));

  async function signOut(): Promise<void> {
    loading = true;
    failure = null;
    try {
      await api.signOut();
      /* Set here rather than left to the revocation the server broadcasts. The
         socket is closing at the same moment, so whether that event arrives is a
         race, and losing it would land someone on the page that greets a stranger
         straight after they pressed Sign out. */
      sessionEnded = { code: 'signed_out', reason: 'You signed out' };
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

  /**
   * Rewrites the address the inbox had while it was a dialog.
   *
   * Called before anything reads the address: the dialog router attaches on mount,
   * and letting it see the name of a dialog no part of the panel will open leaves
   * it thinking something is up there.
   */
  function adoptLegacyInboxAddress(): void {
    const legacy = legacyInboxRoute(window.location.search);
    if (legacy !== null) router.replace(legacy);
  }

  adoptLegacyInboxAddress();
  void load();
</script>

<!-- The block goes inside the head rather than the head inside a block, which
     Svelte does not allow. The sign-in page names the document itself, so this
     stands down rather than leaving two titles to fight over the tab. -->
<svelte:head>
  {#if !signedOut}
    <title>{documentTitle}</title>
  {/if}
</svelte:head>

<!-- Nobody to show a panel to, so there is no panel: the sign-in page takes the
     whole window rather than sitting in a workspace beside an empty sidebar. A
     load failure keeps the shell, because its retry is the way out of it and the
     night page has nowhere to put one. -->
{#if signedOut}
  <SignInPage {api} {build} ended={sessionEnded} />
{:else if awaitingInstallation}
  <!-- A workspace with no workspaces in it is not a workspace. This reader has an
       account and nothing to administer yet, which is the same kind of moment as
       an invitation or a sign-in: one thing to say and one thing to do. It gets
       the same page those get, rather than an empty shell with a sidebar of
       tabs that lead nowhere. -->
  <NightPage title="No installations" documentTitle="No installations" {build} size="compact">
    <div class="install-prompt">
      <span class="install-mark" aria-hidden="true">+</span>
      <div class="install-copy">
        <strong>Install Smyklot to begin</strong>
        <p>
          Install the Smyklot GitHub App on an organization or personal account, then reload this
          panel
        </p>
      </div>
      <button class="btn btn-signal" type="button" onclick={load}>Reload panel</button>
    </div>
  </NightPage>
{:else}
  <a class="skip-link" href="#panel-content">Skip to panel content</a>

  <main
    class="app-shell"
    class:sidebar-collapsed={effectiveSidebarCollapsed}
    class:root-mode={rootMode}
  >
    <!-- `showNavigation` covers a Root who owns no installation: no workspace for
         the view rows to lead to, not in the console yet, and so - before this -
         no navigation at all and no way to reach the console except by typing its
         address. The rail is worth showing to anyone with somewhere to go from it,
         and the view rows stand down by themselves through `showViews`. -->
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
      showViews={selectedTarget !== null}
      showNavigation={viewer !== null &&
        (rootMode ||
          selectedTarget !== null ||
          personalView !== null ||
          viewer.system_role !== 'none')}
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
      inboxHref={inboxHref()}
      inboxActive={personalView === 'inbox'}
      onSelectInbox={() => selectPersonal('inbox')}
      unreadCount={notificationUnread}
    />

    <!-- No wheel handler. A pointer over the page chrome used to have its wheel
         events applied to the table by hand and the browser's own handling
         cancelled, which meant no momentum and no elastic overscroll: the list
         moved a fixed distance per notch and stopped dead at both ends. Scrolling
         is the platform's, everywhere. -->
    <div class="workspace" class:table-scroll-view={tableScrollView}>
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
          <!-- Only reachable with a failure above it; the signed-out page is a page
             of its own now, and the branch that chooses it sits outside this. -->
        {:else if personalView === 'inbox'}
          <InboxView
            fetchPage={api.fetchNotifications}
            markRead={api.markNotificationRead}
            refreshVersion={notificationVersion}
            onUnread={(unread) => {
              // The page has read the whole thing; any badge poll still in the
              // air is answering an older question.
              notificationReads.invalidate();
              notificationUnread = unread;
            }}
          />
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
                inboxHref={inboxHref()}
                onOpenInbox={() => selectPersonal('inbox')}
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
                section={activeRootRoute.rootView === 'access-invitations'
                  ? 'invitations'
                  : 'users'}
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
                suggestUsers={api.suggestRootTargetUsers}
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
                    suggestUsers={api.suggestUsers}
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
                    Install the Smyklot GitHub App on an organization or personal account, then
                    reload this panel
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

  /* A Root with no installations keeps the shell - the console is still there to
     be reached - so this row survives for them. Everyone else gets the night page
     below, where the same words are a column. */
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

  /* One column, centred. The row above is a shape for a panel that has other
     things on it; on its own page this is the only thing there, and the mark
     stands over what it introduces rather than beside it. */
  .install-prompt {
    display: grid;
    gap: var(--space-4);
    justify-items: center;
    text-align: center;
  }

  .install-copy strong {
    display: block;
    font-size: 1rem;
  }

  .install-copy p {
    color: var(--dim);
    margin: var(--space-2) 0 0;
    max-width: 26rem;
  }

  .install-mark {
    align-items: center;
    background: var(--accent-tint);
    border: 1px solid color-mix(in srgb, var(--accent) 34%, transparent);
    border-radius: var(--radius-control);
    color: var(--accent);
    display: inline-flex;
    font: 650 1.5rem/1 var(--sans);
    height: 3rem;
    justify-content: center;
    width: 3rem;
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
