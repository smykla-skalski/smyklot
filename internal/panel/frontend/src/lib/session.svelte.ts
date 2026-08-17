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
import { DEFAULT_THEME_DISPLAY, isThemeDisplay, type ThemeDisplay } from './preferences';
import { createPrefsSync, type PrefsSync } from './preferences-sync';
import {
  panelDocumentTitle,
  rootSection,
  rootSectionRoute,
  type HistorySection,
  type PanelRoute,
  type PanelView,
  type RootInstallationView,
  type RootRoute,
  type RootSection,
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
  streamReady = $state(false);
  sessionEnded = $state<SessionEnded | null>(null);
  identityBar = $state<ReturnType<typeof import('./components/IdentityBar.svelte').default> | null>(
    null,
  );

  theme = $state<ThemeDisplay>('system');
  sidebarCollapsed = $state(false);
  private lastScopedView = $state<PanelView>('settings');
  private lastScopedHistorySection = $state<HistorySection>('audit');

  readonly narrowRail = new MediaQuery('(min-width: 48.0625rem) and (max-width: 72rem)');
  readonly systemDarkTheme = new MediaQuery('prefers-color-scheme: dark');

  constructor(api: PanelApi, build: PanelBuild, queryClient: QueryClient) {
    this.api = api;
    this.build = build;
    this.queryClient = queryClient;
    this.prefs = createPrefsSync();
    this.sidebarCollapsed = this.prefs.get('sidebar') === 'collapsed';
    this.theme = this.storedTheme();
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

  syncRouteContext(): void {
    // Nothing is recorded from a page that failed to load. The address still names a view
    // and the chrome still shows it, but a reader who pasted a broken link was never on
    // it, so Return would otherwise take them somewhere they had not been. `page.error`
    // covers every load failure rather than the one shape this used to test for.
    if (page.error !== null) return;
    if (this.isRootMode || this.isInbox || this.isInvitation) return;
    const route = this.parsedRoute;
    if (route === null || !('view' in route)) return;
    this.lastScopedView = route.view;
    if (route.view === 'history' && route.section !== undefined) {
      this.lastScopedHistorySection = route.section;
    }
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

  get documentTitle(): string {
    if (this.isInbox) return panelDocumentTitle({ personal: 'inbox' });
    const active = this.isRootMode
      ? this.currentRootRoute
      : this.currentView === 'history'
        ? { account: '', view: this.currentView, section: this.currentHistorySection }
        : { account: '', view: this.currentView };
    return panelDocumentTitle(active as PanelRoute);
  }

  get tableScrollView(): boolean {
    if (this.isRootMode) {
      const route = this.currentRootRoute;
      if (route.rootView === 'installation') {
        return ['repositories', 'users', 'invitations', 'history'].includes(route.view);
      }

      return (
        this.rootValue === 'history' ||
        this.rootValue === 'access' ||
        route.rootView === 'installations'
      );
    }

    return (
      this.selectedTarget !== null &&
      ['repositories', 'users', 'invitations', 'history'].includes(this.currentView)
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
    if (target === undefined || (this.selectedId === targetId && !this.isInbox)) return;
    await this.openTarget(target);
  }

  openTarget(target: PanelTarget, replace = false): Promise<void> {
    return this.navigate(this.routeFor(target, this.currentView), replace);
  }

  selectView(nextView: PanelView): void {
    const target = this.selectedTarget;
    if (target === null || (this.currentView === nextView && !this.isInbox)) return;
    void this.navigate(this.routeFor(target, nextView));
  }

  selectHistorySection(section: HistorySection): void {
    const target = this.selectedTarget;
    if (target === null || this.currentHistorySection === section) return;
    void this.navigate(this.routeFor(target, 'history', section));
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

  selectRootInstallation(account: string, nextView: RootInstallationView): void {
    const route = this.rootInstallationRoute(account, nextView);
    void this.navigate(route);
    this.resetPageScroll();
  }

  selectRootInstallationHistory(section: HistorySection): void {
    if (this.currentRootRoute.rootView !== 'installation') return;
    if (this.currentHistorySection === section) return;
    const route = this.rootInstallationRoute(this.currentRootRoute.account, 'history', section);
    void this.navigate(route);
  }

  selectRootInstallations(): void {
    void this.navigate({ rootView: 'installations' });
    this.resetPageScroll();
  }

  enterRoot(): void {
    if (this.viewer?.system_role === 'none') return;
    void this.navigate({ rootView: 'overview' });
    this.resetPageScroll();
  }

  returnToPanel(replace = false): void {
    const target = this.returnTarget;
    if (target === null) {
      void goto(this.returnHref(), { replace: true });
      return;
    }
    void this.navigate(this.returnRoute(target), replace);
  }

  // --- Hrefs ---

  targetHref(target: PanelTarget): string {
    return panelAddress(this.returnRoute(target));
  }

  viewHref(nextView: PanelView): string {
    const target = this.selectedTarget;
    return target === null ? '#' : panelAddress(this.routeFor(target, nextView));
  }

  rootHrefFor(section: RootSection): string {
    return panelAddress(rootSectionRoute(section));
  }

  rootDashboardHref(): string {
    return panelAddress({ rootView: 'overview' });
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

  rootInstallationHref(account: string, nextView: RootInstallationView): string {
    return panelAddress(this.rootInstallationRoute(account, nextView));
  }

  returnHref(): string {
    return this.returnTarget === null
      ? resolve('/')
      : panelAddress(this.returnRoute(this.returnTarget));
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

  refreshAccessFromStream(): void {
    void this.queryClient.invalidateQueries({ queryKey: ['viewer'] });
    void this.queryClient.invalidateQueries({ queryKey: ['targets'] });
    void this.queryClient.invalidateQueries({
      predicate: (query) => !['viewer', 'targets'].includes(String(query.queryKey[0])),
    });
  }

  invalidateChange(event: PanelChangeEvent): void {
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

  private returnRoute(target: PanelTarget): PanelRoute {
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

  private rootInstallationRoute(
    account: string,
    nextView: RootInstallationView,
    section: HistorySection = this.currentHistorySection,
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
