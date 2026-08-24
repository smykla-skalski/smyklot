/**
 * Session state shared across the panel's route tree.
 *
 * Absorbed from App.svelte: viewer and target shell state, query invalidation,
 * and navigation. Route-derived state (view, root mode, active root route) is
 * read from SvelteKit's `$app/state` page object, not stored here.
 */

import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { page } from '$app/state';
import { createContext } from 'svelte';
import { MediaQuery } from 'svelte/reactivity';

import type { QueryClient } from '@tanstack/svelte-query';

import type { PanelApi } from './api';
import type { PanelBuild } from './base';
import { panelAddress, panelRouteAt } from './addresses';
import { basePath } from './paths';

/**
 * Whether the address is this segment or something below it, base and all.
 *
 * Decoded first, because this only ever answers for an address the router matched no route
 * for - and the reason it matched none is that the server decided what to serve from the
 * decoded path while the router reads the raw one. `/root%2Finstallations` is the console
 * to the server and nothing to the router, so it has to be the console here too. Whole
 * segments, so `/rootbeer` is not.
 */
function at(segment: string): boolean {
  let pathname = page.url.pathname;
  try {
    pathname = decodeURIComponent(pathname);
  } catch {
    // A malformed escape is not an address the server would have served either.
    return false;
  }

  return pathname === `${basePath}${segment}` || pathname.startsWith(`${basePath}${segment}/`);
}
import type { PanelChangeEvent } from './events';
import type { SessionEnded } from './panel-session';
import { readLastConsolePage, readLastWorkspacePage, writeLastPage } from './last-page';
import { DEFAULT_THEME_DISPLAY, isThemeDisplay, type ThemeDisplay } from './preferences';
import { createPrefsSync, type PrefsSync } from './preferences-sync';
import type { StreamLiveness } from './query-client';
import {
  panelDocumentTitle,
  rootSection,
  rootSectionRoute,
  type HistorySection,
  type InstallationRoute,
  type PanelRoute,
  type PanelView,
  type RepositoryPage,
  type RepositorySection,
  type RootInstallationView,
  type RootRoute,
  type RootRuntimeSection,
  type RootSection,
  type SyncSection,
  type RouteDialog,
} from './routes';
import type { NotificationPage, PanelTarget, PanelViewer } from './types';

type FailureSource = 'load' | 'sign-out';
type PanelFailure = { message: string; source: FailureSource };

export interface SessionQueryState {
  viewer: PanelViewer | null | undefined;
  targets: PanelTarget[] | undefined;
  viewerPending: boolean;
  targetsPending: boolean;
  viewerError: unknown;
  targetsError: unknown;
}

export class PanelSession {
  readonly api: PanelApi;
  readonly build: PanelBuild;
  readonly prefs: PrefsSync;
  readonly queryClient: QueryClient;

  loading = $state(true);
  viewer = $state.raw<PanelViewer | null>(null);
  targets = $state.raw<PanelTarget[]>([]);
  selectedId = $state<string | null>(null);
  failure = $state<PanelFailure | null>(null);
  notificationUnread = $state(0);
  queueRevision = $state(0);
  streamReady = $state(false);
  /** Set from the stream's handshake; see `StreamLiveness`. */
  private readonly stream: StreamLiveness;
  sessionEnded = $state<SessionEnded | null>(null);

  theme = $state<ThemeDisplay>('system');
  sidebarCollapsed = $state(false);
  private lastScopedView = $state<PanelView>('defaults');
  private lastScopedHistorySection = $state<HistorySection>('audit');
  /**
   * The whole page each side was last on, which is where crossing to it goes back to.
   *
   * The two fields above answer a different question - what a reader was looking at, so
   * that another installation opens on the same view - and neither can answer this one.
   * A repository page is the repositories view, so remembering only the view brought
   * somebody who left one back to the list they had opened it from.
   */
  private lastWorkspacePage = $state.raw<InstallationRoute | null>(null);
  private lastConsolePage = $state.raw<RootRoute | null>(null);

  readonly narrowRail = new MediaQuery('(min-width: 48.0625rem) and (max-width: 72rem)');
  readonly systemDarkTheme = new MediaQuery('prefers-color-scheme: dark');

  constructor(
    api: PanelApi,
    build: PanelBuild,
    queryClient: QueryClient,
    /* The same box the query client reads. Handed in rather than owned here
       because the client is built first and the two have to agree. */
    stream: StreamLiveness = { live: false },
  ) {
    this.api = api;
    this.build = build;
    this.queryClient = queryClient;
    this.stream = stream;
    this.prefs = createPrefsSync();
    this.sidebarCollapsed = this.prefs.get('sidebar') === 'collapsed';
    this.theme = this.storedTheme();
    this.lastWorkspacePage = readLastWorkspacePage();
    this.lastConsolePage = readLastConsolePage();
  }

  // --- Derived ---

  get selectedTarget(): PanelTarget | null {
    if (this.selectedId === null) return null;
    return this.targets.find((t) => t.id === this.selectedId) ?? null;
  }

  get rootRole(): string {
    return this.viewer?.system_role === 'super_root' ? 'Super Root' : 'Root';
  }

  get signedOut(): boolean {
    return !this.loading && this.viewer === null && this.failure === null;
  }

  get awaitingInstallation(): boolean {
    return (
      !this.loading &&
      this.viewer !== null &&
      this.failure === null &&
      this.targets.length === 0 &&
      !this.isRootMode &&
      this.viewer.system_role === 'none'
    );
  }

  get returnTarget(): PanelTarget | null {
    return this.selectedTarget ?? this.targets[0] ?? null;
  }

  get effectiveSidebarCollapsed(): boolean {
    return this.sidebarCollapsed || this.narrowRail.current;
  }

  get resolvedTheme(): 'dark' | 'light' {
    if (this.theme === 'system' && this.systemDarkTheme.current) return 'dark';
    if (this.theme === 'system') return 'light';
    return this.theme;
  }

  /**
   * Which part of the panel is open, asked of the route rather than the address.
   *
   * A route id is what SvelteKit matched, so it carries no base and no trailing slash -
   * both of which these had to spell out when they read the pathname.
   *
   * The address is still the answer when nothing matched. The server decides what to
   * serve from the decoded path and the router matches on the undecoded one, so the two
   * disagree about an address holding `%2F`: the server answers `/root%2Finstallations`
   * with the console, the router matches no route, and the panel would otherwise wear the
   * wrong chrome on a page the server had already named. All three ask the same way, and
   * `at` compares whole segments so `/rootbeer` is not the console.
   */
  get isRootMode(): boolean {
    return page.route.id?.startsWith('/root') ?? at('/root');
  }

  get isInbox(): boolean {
    return page.route.id === '/inbox' || (page.route.id === null && at('/inbox'));
  }

  get isInvitation(): boolean {
    return (
      page.route.id === '/invite/[token=invitationToken]' ||
      (page.route.id === null && at('/invite'))
    );
  }

  /**
   * What is being looked at, read from the route SvelteKit matched.
   *
   * The id and the parameters together, never the parameters alone. A parameter is only
   * there if the matched route names one, and which route matched is a detail of how
   * `src/routes` is laid out: a view hosting a dialog is routed with the segments after
   * it, one hosting none is routed without them, and history is routed by name with its
   * section. Reading `params.view` on its own tied these getters to that shape and broke
   * the moment it changed - the installation's history came back as `settings`, and the
   * console's came back as the Root console's own history page. The id says which shape
   * it is, so `panelRouteAt` can read every one of them correctly.
   *
   * This used to parse the pathname a second time, with the route tree written out by
   * hand. That copy is still there for the mock server, which runs under plain Node - but
   * the panel no longer reads it, so a renamed route cannot mean one thing to the router
   * and another to the panel.
   */
  private get parsedRoute(): PanelRoute | null {
    return panelRouteAt(page.route.id, page.params);
  }

  get currentView(): PanelView {
    const route = this.parsedRoute;

    return route !== null && 'view' in route ? route.view : this.lastScopedView;
  }

  get currentHistorySection(): HistorySection {
    const route = this.parsedRoute;
    if (route !== null && 'section' in route && route.section !== undefined) return route.section;

    return this.lastScopedHistorySection;
  }

  /**
   * The repository the address names, or `null` for the list.
   *
   * One getter for both surfaces. A workspace and the console reach the same
   * page by different paths, and the only thing either of them draws differently
   * is the chrome around it, so what is open is worth asking once.
   */
  get currentRepository(): RepositoryPage | null {
    const route = this.parsedRoute;
    if (route === null || 'personal' in route) return null;
    if ('rootView' in route) {
      return route.rootView === 'installation' ? (route.repository ?? null) : null;
    }

    return route.repository ?? null;
  }

  syncRouteContext(): void {
    // Nothing is recorded from a page that failed to load. The address still names a view
    // and the chrome still shows it, but a reader who pasted a broken link was never on
    // it, so Return would otherwise take them somewhere they had not been. `page.error`
    // covers every load failure rather than the one shape this used to test for.
    if (page.error !== null) return;
    if (this.isInbox || this.isInvitation) return;
    const route = this.parsedRoute;
    if (route === null || 'personal' in route) return;
    // Each side records only its own page, so entering one never overwrites where the
    // other was left. The route says which side this is; `isRootMode` answers for the
    // chrome, which is a wider question - it is still the console while a page under it
    // is failing to load, and nothing is recorded from one of those.
    if ('rootView' in route) {
      this.lastConsolePage = route;
      writeLastPage('console', page.route.id, page.params);
      return;
    }
    this.lastScopedView = route.view;
    if (route.view === 'history' && route.section !== undefined) {
      this.lastScopedHistorySection = route.section;
    }
    this.lastWorkspacePage = route;
    writeLastPage('workspace', page.route.id, page.params);
  }

  /**
   * Read from the path, for the reason `parsedRoute` gives: which parameters
   * exist depends on which route matched, and the console's addresses are now
   * spread across three of them. Reading `params.section` sent an installation's
   * history to the Root console's own history page, because both routes call
   * that parameter `section`.
   */
  get currentRootRoute(): RootRoute {
    if (!this.isRootMode) return { rootView: 'overview' };
    const route = this.parsedRoute;

    return route !== null && 'rootView' in route ? route : { rootView: 'overview' };
  }

  get rootValue(): RootSection {
    return rootSection(this.currentRootRoute);
  }

  /**
   * The title says where the reader is, which for two views is deeper than the
   * view's own name: history is a table and sync is a section, and both are
   * addresses somebody can be sent. The account is left empty because the title
   * never carries it - the workspace is in the chrome.
   */
  get documentTitle(): string {
    if (this.isInbox) return panelDocumentTitle({ personal: 'inbox' });
    if (this.isRootMode) return panelDocumentTitle(this.currentRootRoute);

    const view = this.currentView;
    const route: InstallationRoute = { account: '', view };
    if (view === 'history') route.section = this.currentHistorySection;
    if (view === 'sync') {
      route.sync = this.currentSyncSection;
      route.syncRuleset = this.currentSyncRuleset ?? undefined;
      route.syncFile = this.currentSyncFile ?? undefined;
    }

    return panelDocumentTitle(route);
  }

  get tableScrollView(): boolean {
    if (this.isRootMode) {
      const route = this.currentRootRoute;
      if (route.rootView === 'installation') {
        return (
          ['repositories', 'users', 'invitations', 'history'].includes(route.view) &&
          (route.view !== 'repositories' || route.repository === undefined)
        );
      }

      return (
        this.rootValue === 'history' ||
        this.rootValue === 'access' ||
        route.rootView === 'installations'
      );
    }

    return (
      this.selectedTarget !== null &&
      ['repositories', 'users', 'invitations', 'history'].includes(this.currentView) &&
      /* Repository lists own a bounded table scroller. A repository detail is a
         document-length settings page, so trapping its workspace at 100dvh clips
         the controls below the fold with no element left that can scroll. */
      (this.currentView !== 'repositories' || this.currentRepository === null)
    );
  }

  // --- Session lifecycle ---

  async load(): Promise<void> {
    this.loading = true;
    this.failure = null;
    await this.queryClient.invalidateQueries({ queryKey: ['viewer'] });
    await this.queryClient.invalidateQueries({ queryKey: ['targets'] });
  }

  syncQueries(state: SessionQueryState): void {
    const initialError =
      state.viewer === undefined
        ? state.viewerError
        : state.viewer !== null && state.targets === undefined
          ? state.targetsError
          : null;
    if (initialError !== null) {
      this.setFailure('load', initialError);
      this.loading = false;
      this.streamReady = false;
      return;
    }

    if (state.viewer === undefined) {
      this.loading = state.viewerPending;
      return;
    }

    if (state.viewer === null) {
      this.viewer = null;
      this.targets = [];
      this.selectedId = null;
      this.loading = false;
      this.streamReady = false;
      this.clearFailure('load');
      return;
    }

    const accountChanged = this.viewer?.account.id !== state.viewer.account.id;
    this.viewer = state.viewer;
    this.sessionEnded = null;
    if (accountChanged) this.prefs.adoptAccount(state.viewer.account.id);

    if (state.targets === undefined) {
      this.loading = state.targetsPending;
      return;
    }

    this.targets = state.targets;
    if (
      this.selectedId !== null &&
      !state.targets.some((target) => target.id === this.selectedId)
    ) {
      this.selectedId = null;
    }
    this.loading = false;
    this.streamReady = true;
    this.clearFailure('load');
  }

  // --- Target selection ---

  async selectTarget(targetId: string): Promise<void> {
    const target = this.targets.find((t) => t.id === targetId);
    /* The selected workspace is still one press away from elsewhere: from the
       inbox or the Root console its tile is the way back to its pages - and
       from a record inside it, the same press is the way back to the view,
       for the reason selectView gives. */
    const alreadyThere =
      this.selectedId === targetId &&
      !this.isInbox &&
      !this.isRootMode &&
      this.currentRepository === null;
    if (target === undefined || alreadyThere) return;
    await this.openTarget(target);
  }

  openTarget(target: PanelTarget, replace = false): Promise<void> {
    return this.navigate(this.routeFor(target, this.currentView), replace);
  }

  selectView(nextView: PanelView): void {
    const target = this.selectedTarget;
    if (target === null) return;
    /* Pressing the view you are already in is nothing to do - unless you are
       inside a record of it. The repository page IS the repositories view, so
       the navigation still reads Repositories while you are on one, and this is
       how a reader who presses it expects to reach the list. Without the second
       clause the item was inert on exactly the screen it was most needed. */
    const alreadyThere = this.currentView === nextView && this.currentRepository === null;
    if (alreadyThere && !this.isInbox) return;
    void this.navigate(this.routeFor(target, nextView));
  }

  selectHistorySection(section: HistorySection): void {
    const target = this.selectedTarget;
    if (
      target === null ||
      (this.currentView === 'history' && this.currentHistorySection === section)
    )
      return;
    void this.navigate(this.routeFor(target, 'history', section));
  }

  // --- Sync sections ---

  /** The sync section the address names; the bare view is the overview. */
  get currentSyncSection(): SyncSection {
    const route = this.parsedRoute;
    if (route !== null && 'view' in route && route.view === 'sync' && route.sync !== undefined) {
      return route.sync;
    }
    return 'overview';
  }

  selectSyncSection(section: SyncSection): void {
    const target = this.selectedTarget;
    if (target === null) return;
    if (
      this.currentView === 'sync' &&
      this.currentSyncSection === section &&
      this.currentSyncRuleset === null
    ) {
      return;
    }
    void this.navigate(this.syncRoute(target, section));
  }

  /** The ruleset page the address names, or null on the list and everywhere else. */
  get currentSyncRuleset(): string | null {
    const route = this.parsedRoute;
    if (route !== null && 'view' in route && route.view === 'sync') {
      return route.syncRuleset ?? null;
    }
    return null;
  }

  /** Opening a ruleset is a place to come back from, so it pushes. */
  selectSyncRuleset(name: string): void {
    const target = this.selectedTarget;
    if (target === null || this.currentSyncRuleset === name) return;
    const account = target.account.login;
    void this.navigate({ account, view: 'sync', sync: 'rulesets', syncRuleset: name });
  }

  syncRulesetHref(name: string): string {
    const target = this.selectedTarget;
    if (target === null) return '#';
    return panelAddress({
      account: target.account.login,
      view: 'sync',
      sync: 'rulesets',
      syncRuleset: name,
    });
  }

  /** The template page the address names, or null on the list and everywhere else. */
  get currentSyncFile(): string | null {
    const route = this.parsedRoute;
    if (route !== null && 'view' in route && route.view === 'sync') {
      return route.syncFile ?? null;
    }
    return null;
  }

  /** Opening a template is a place to come back from, so it pushes. */
  selectSyncFile(path: string): void {
    const target = this.selectedTarget;
    if (target === null || this.currentSyncFile === path) return;
    const account = target.account.login;
    void this.navigate({ account, view: 'sync', sync: 'files', syncFile: path });
  }

  syncFileHref(path: string): string {
    const target = this.selectedTarget;
    if (target === null) return '#';
    return panelAddress({
      account: target.account.login,
      view: 'sync',
      sync: 'files',
      syncFile: path,
    });
  }

  syncSectionHref(section: SyncSection): string {
    const target = this.selectedTarget;
    if (target === null) return '#';
    return panelAddress(this.syncRoute(target, section));
  }

  private syncRoute(target: PanelTarget, section: SyncSection): PanelRoute {
    const account = target.account.login;
    return section === 'overview'
      ? { account, view: 'sync' }
      : { account, view: 'sync', sync: section };
  }

  // --- One repository ---

  /** Opening a repository is a place to come back from, so it pushes. */
  openRepository(name: string): void {
    void this.navigate(this.repositoryRoute({ name, section: 'file' }));
    this.resetPageScroll();
  }

  /**
   * The pane rides the address too, so a link points at the commands a colleague
   * was asked to look at. It replaces rather than pushes: moving between the
   * panes is part of reading one repository, not a second place to come back
   * from, and Back should leave for the list rather than walk the panes.
   */
  selectRepositorySection(section: RepositorySection): void {
    const open = this.currentRepository;
    if (open === null || open.section === section) return;
    void this.navigate(this.repositoryRoute({ name: open.name, section }), true);
  }

  /** Back to the list. A push, so it pairs with the push that opened the page. */
  closeRepository(): void {
    void this.navigate(this.repositoryRoute(null));
    this.resetPageScroll();
  }

  repositoryHref(name: string, section: RepositorySection = 'file'): string {
    return panelAddress(this.repositoryRoute({ name, section }));
  }

  /** The list this page was opened from, in whichever surface that was. */
  repositoriesHref(): string {
    return panelAddress(this.repositoryRoute(null));
  }

  selectUserSection(section: 'users' | 'invitations'): void {
    if (this.selectedTarget === null) return;
    if (this.currentView === section) return;
    void this.navigate(this.routeFor(this.selectedTarget, section));
  }

  // --- Root navigation ---

  selectRootSection(section: RootSection): void {
    if (!this.isRootMode || this.rootValue === section) return;
    void this.navigate(rootSectionRoute(section));
    this.resetPageScroll();
  }

  selectQueueSection(section: 'waiting' | 'recent'): void {
    const route: RootRoute = { rootView: section === 'waiting' ? 'queue' : 'queue-recent' };
    if (this.currentRootRoute.rootView === route.rootView) return;
    void this.navigate(route);
  }

  /** A request is a page of its own, so opening one is navigation and resets the scroll. */
  openQueueRequest(request: string): void {
    void this.navigate({ rootView: 'queue-request', request });
    this.resetPageScroll();
  }

  selectRootHistorySection(section: 'audit' | 'failures'): void {
    const route: RootRoute = {
      rootView: section === 'audit' ? 'history-audit' : 'history-failures',
    };
    if (this.currentRootRoute.rootView === route.rootView) return;
    void this.navigate(route);
  }

  selectRootAccessSection(section: 'users' | 'invitations'): void {
    const route: RootRoute = {
      rootView: section === 'users' ? 'access-users' : 'access-invitations',
    };
    if (this.currentRootRoute.rootView === route.rootView) return;
    void this.navigate(route);
  }

  selectRootRuntimeSection(section: RootRuntimeSection): void {
    const route: RootRoute = {
      rootView: `runtime-${section}`,
    };
    if (this.currentRootRoute.rootView === route.rootView) return;
    void this.navigate(route);
  }

  selectRootInstallation(account: string, nextView: RootInstallationView): void {
    const route = this.rootInstallationRoute(account, nextView);
    void this.navigate(route);
    this.resetPageScroll();
  }

  selectRootInstallationHistory(section: HistorySection): void {
    const current = this.currentRootRoute;
    if (current.rootView !== 'installation') return;
    if (current.view === 'history' && this.currentHistorySection === section) return;
    const route = this.rootInstallationRoute(current.account, 'history', section);
    void this.navigate(route);
  }

  selectRootInstallations(): void {
    void this.navigate({ rootView: 'installations' });
    this.resetPageScroll();
  }

  enterRoot(): void {
    if (this.viewer?.system_role === 'none') return;
    void this.navigate(this.consoleRoute());
    this.resetPageScroll();
  }

  returnToPanel(replace = false): void {
    const route = this.returnRoute();
    if (route === null) {
      void goto(this.returnHref(), { replace: true });
      return;
    }
    void this.navigate(route, replace);
  }

  // --- Hrefs ---

  targetHref(target: PanelTarget): string {
    return panelAddress(this.targetRoute(target));
  }

  viewHref(nextView: PanelView): string {
    const target = this.selectedTarget;
    return target === null ? '#' : panelAddress(this.routeFor(target, nextView));
  }

  rootHrefFor(section: RootSection): string {
    return panelAddress(rootSectionRoute(section));
  }

  /**
   * Where one of history's tables lives, and one of access's lists.
   *
   * Both were strips whose halves are addresses - one drawn as a segmented
   * control, which is the control that changes what is on screen and saves
   * nothing. A tab is a place, and a place has an href a person can middle-click
   * and copy; these are what let the strips say so.
   */
  historyHref(section: HistorySection): string {
    const target = this.selectedTarget;

    return target === null ? '#' : panelAddress(this.routeFor(target, 'history', section));
  }

  accessHref(section: 'users' | 'invitations'): string {
    const target = this.selectedTarget;

    return target === null ? '#' : panelAddress(this.routeFor(target, section));
  }

  queueSectionHref(section: 'waiting' | 'recent'): string {
    return panelAddress({ rootView: section === 'waiting' ? 'queue' : 'queue-recent' });
  }

  rootAccessHref(section: 'users' | 'invitations'): string {
    return panelAddress({
      rootView: section === 'users' ? 'access-users' : 'access-invitations',
    });
  }

  rootRuntimeHref(section: RootRuntimeSection): string {
    return panelAddress({
      rootView: `runtime-${section}`,
    });
  }

  /** Where the console opens: the page it was left on, or its front page the first time. */
  rootEntryHref(): string {
    return panelAddress(this.consoleRoute());
  }

  rootInstallationsHref(): string {
    return panelAddress({ rootView: 'installations' });
  }

  rootAuditHref(): string {
    return panelAddress({ rootView: 'history-audit' });
  }

  queueHref(): string {
    return panelAddress({ rootView: 'queue' });
  }

  queueRequestHref(request: string): string {
    return panelAddress({ rootView: 'queue-request', request });
  }

  rootFailuresHref(): string {
    return panelAddress({ rootView: 'history-failures' });
  }

  /**
   * Where one installation's view lives on the console.
   *
   * The history section is a parameter rather than always the current one:
   * history's own strip is two addresses, and a builder that always answered
   * with the section being looked at would give both tabs the same href.
   */
  rootInstallationHref(
    account: string,
    nextView: RootInstallationView,
    section?: HistorySection,
  ): string {
    return panelAddress(this.rootInstallationRoute(account, nextView, section));
  }

  returnHref(): string {
    const route = this.returnRoute();

    return route === null ? resolve('/') : panelAddress(route);
  }

  inboxHref(): string {
    return panelAddress({ personal: 'inbox' });
  }

  openInbox(): void {
    void this.navigate({ personal: 'inbox' });
  }

  repositoryChanged(targetId: string): void {
    if (this.viewer === null) return;
    this.invalidateTargetData(targetId);
    this.invalidateRepositoryAggregates();
  }

  updateNotificationUnread(unread: number): void {
    this.notificationUnread = unread;
    this.queryClient.setQueriesData<NotificationPage>(
      { queryKey: ['notifications', 'unread'] },
      (current) =>
        current === undefined || current.unread === unread ? current : { ...current, unread },
    );
  }

  // --- Stream ---

  /**
   * The stream is up, or it is not.
   *
   * What reads this is how long an answer is trusted: with changes arriving as
   * they happen there is nothing for a clock to add, and without them there is
   * nothing else. Losing the socket therefore puts the panel back to catching up
   * on its own, and getting it back stops that again - and the reconnect replies
   * `ready`, which `onResync` turns into a full refresh, so nothing missed while
   * it was down stays missed.
   */
  setStreamLive(live: boolean): void {
    this.stream.live = live;
  }

  refreshAccessFromStream(): void {
    void this.queryClient.invalidateQueries({ queryKey: ['viewer'] });
    void this.queryClient.invalidateQueries({ queryKey: ['targets'] });
    void this.queryClient.invalidateQueries({
      predicate: (query) => !['viewer', 'targets'].includes(String(query.queryKey[0])),
    });
  }

  invalidateChange(event: PanelChangeEvent): void {
    if (event.type === 'queue.changed') {
      this.queueRevision += 1;
      void this.queryClient.invalidateQueries({ queryKey: ['sync-plan'] });
      return;
    }
    const targetId = event.target_id;
    // The server has no notification-specific event. Any audited change can
    // produce a new Owner notification, which is why the old shell refreshed
    // this count on every change frame too.
    void this.queryClient.invalidateQueries({ queryKey: ['notifications'] });
    switch (event.type) {
      case 'target.changed':
        void this.queryClient.invalidateQueries({ queryKey: ['targets'] });
        void this.queryClient.invalidateQueries({ queryKey: ['root-installations'] });
        void this.queryClient.invalidateQueries({ queryKey: ['root-overview'] });
        this.invalidateTargetData(targetId);
        return;
      case 'repository.changed':
        void this.queryClient.invalidateQueries({ queryKey: ['repositories', targetId] });
        void this.queryClient.invalidateQueries({ queryKey: ['repository', targetId] });
        /* What a repository says about a kind of sync is its own key, and keys
           match by prefix, so neither of the two above reaches it. Without this
           a colleague's save left this browser rendering the document it had
           and sending the revision it came with, so every Save it tried was
           answered 409 until the page was reloaded. Prefixed by the target
           rather than the repository, because one event stands for whichever
           repository changed. */
        void this.queryClient.invalidateQueries({ queryKey: ['sync-override', targetId] });
        this.invalidateRepositoryAggregates();
        return;
      case 'audit.changed':
        void this.queryClient.invalidateQueries({ queryKey: ['audit', targetId] });
        void this.queryClient.invalidateQueries({ queryKey: ['audit', 'root'] });
        return;
      case 'failure.changed':
        void this.queryClient.invalidateQueries({ queryKey: ['failures', targetId] });
        void this.queryClient.invalidateQueries({ queryKey: ['failures', 'root'] });
        void this.queryClient.invalidateQueries({ queryKey: ['root-overview'] });
        return;
      case 'users.changed':
        void this.queryClient.invalidateQueries({ queryKey: ['users', targetId] });
        void this.queryClient.invalidateQueries({ queryKey: ['access-decisions', targetId] });
        void this.queryClient.invalidateQueries({ queryKey: ['root-access', 'users'] });
        return;
      case 'invitation.changed':
        void this.queryClient.invalidateQueries({ queryKey: ['invitations', targetId] });
        void this.queryClient.invalidateQueries({ queryKey: ['root-access', 'invitations'] });
        return;
      case 'access.changed':
        this.refreshAccessFromStream();
    }
  }

  revokeAccess(ended: SessionEnded): void {
    this.sessionEnded = ended;
    this.viewer = null;
    this.targets = [];
    this.selectedId = null;
    this.streamReady = false;
    this.queryClient.setQueryData(['viewer'], null);
    this.queryClient.removeQueries({ predicate: (query) => query.queryKey[0] !== 'viewer' });
  }

  async signOut(): Promise<void> {
    this.loading = true;
    this.failure = null;
    try {
      await this.api.signOut();
      this.sessionEnded = { code: 'signed_out', reason: 'You signed out' };
      this.viewer = null;
      this.targets = [];
      this.selectedId = null;
      this.streamReady = false;
      this.queryClient.setQueryData(['viewer'], null);
      this.queryClient.removeQueries({ predicate: (query) => query.queryKey[0] !== 'viewer' });
    } catch (error) {
      this.setFailure('sign-out', error);
      this.loading = false;
    }
  }

  // --- Theme & sidebar ---

  selectTheme(nextTheme: ThemeDisplay): void {
    this.theme = nextTheme;
    this.prefs.set('theme', nextTheme);
  }

  toggleSidebar(): void {
    this.sidebarCollapsed = !this.sidebarCollapsed;
    this.prefs.set('sidebar', this.sidebarCollapsed ? 'collapsed' : 'expanded');
  }

  // --- Internal ---

  storedTheme(): ThemeDisplay {
    const value = this.prefs.get('theme');
    return typeof value === 'string' && isThemeDisplay(value) ? value : DEFAULT_THEME_DISPLAY;
  }

  /**
   * Where Return goes: the page the reader left, while it is still theirs to open.
   *
   * A remembered page outlives the tab's reloads, and can outlive the reader too - the
   * next person to sign in on this tab has their own installations. So it is offered only
   * while the account it names is one of them, and anything else falls back to what the
   * panel answered before it remembered anything: the selected installation, on the view
   * that was last looked at.
   */
  private returnRoute(): PanelRoute | null {
    const remembered = this.lastWorkspacePage;
    if (remembered !== null && this.installed(remembered.account)) return remembered;
    const target = this.returnTarget;

    return target === null ? null : this.targetRoute(target);
  }

  /**
   * Where the console opens.
   *
   * No check against what exists, unlike the workspace side. Whoever may enter the
   * console may read every installation in it, so a page here is only ever stale in the
   * way a bookmark is - and the one reader it could have belonged to instead, a Root
   * whose role has since been taken away, is turned back at the door by `enterRoot`.
   */
  private consoleRoute(): RootRoute {
    return this.lastConsolePage ?? { rootView: 'overview' };
  }

  private installed(account: string): boolean {
    const folded = account.toLowerCase();

    return this.targets.some((target) => target.account.login.toLowerCase() === folded);
  }

  /** The same view on another installation, which is what its own link promises. */
  private targetRoute(target: PanelTarget): PanelRoute {
    return this.routeFor(target, this.lastScopedView, this.lastScopedHistorySection);
  }

  private navigate(route: PanelRoute, replace = false): Promise<void> {
    return goto(panelAddress(route), { replace });
  }

  invalidateTargetData(targetId: string): void {
    this.queryClient.invalidateQueries({ queryKey: ['repositories', targetId] });
    this.queryClient.invalidateQueries({ queryKey: ['repository', targetId] });
    this.queryClient.invalidateQueries({ queryKey: ['audit', targetId] });
    this.queryClient.invalidateQueries({ queryKey: ['failures', targetId] });
    this.queryClient.invalidateQueries({ queryKey: ['users', targetId] });
    this.queryClient.invalidateQueries({ queryKey: ['invitations', targetId] });
  }

  private invalidateRepositoryAggregates(): void {
    void this.queryClient.invalidateQueries({ queryKey: ['targets'] });
    void this.queryClient.invalidateQueries({ queryKey: ['root-installations'] });
    void this.queryClient.invalidateQueries({ queryKey: ['root-overview'] });
  }

  private routeFor(
    target: PanelTarget,
    nextView: PanelView,
    section: HistorySection = this.currentHistorySection,
    dialog?: RouteDialog,
  ): PanelRoute {
    const route: PanelRoute =
      nextView === 'history'
        ? { account: target.account.login, view: nextView, section }
        : { account: target.account.login, view: nextView };
    return dialog === undefined ? route : { ...route, dialog };
  }

  /**
   * The repositories address of whichever surface the reader is already in.
   *
   * `null` for the list itself. Built from the current route rather than from a
   * flag passed down, so a component that lists repositories can offer the link
   * without knowing whether it is being drawn in a workspace or in the console.
   */
  private repositoryRoute(repository: RepositoryPage | null): PanelRoute {
    const root = this.currentRootRoute;
    if (this.isRootMode && root.rootView === 'installation') {
      const route: RootRoute = {
        rootView: 'installation',
        account: root.account,
        view: 'repositories',
      };

      return repository === null ? route : { ...route, repository };
    }

    const route: InstallationRoute = {
      account: this.selectedTarget?.account.login ?? '',
      view: 'repositories',
    };

    return repository === null ? route : { ...route, repository };
  }

  private rootInstallationRoute(
    account: string,
    nextView: RootInstallationView,
    section: HistorySection | undefined = this.currentHistorySection,
  ): RootRoute {
    return nextView === 'history'
      ? { rootView: 'installation', account, view: nextView, section }
      : { rootView: 'installation', account, view: nextView };
  }

  private resetPageScroll(): void {
    queueMicrotask(() => window.scrollTo({ top: 0, left: 0 }));
  }

  private setFailure(source: FailureSource, error: unknown): void {
    this.failure = { message: this.errorMessage(error), source };
  }

  private clearFailure(...sources: FailureSource[]): void {
    if (this.failure !== null && sources.includes(this.failure.source)) this.failure = null;
  }

  private errorMessage(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }
}

export const [getPanelSession, setPanelSession] = createContext<PanelSession>();
