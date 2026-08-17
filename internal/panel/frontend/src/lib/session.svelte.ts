/**
 * Session state shared across the panel's route tree.
 *
 * Absorbed from App.svelte: viewer and target shell state, query invalidation,
 * and navigation. Route-derived state (view, root mode, active root route) is
 * read from SvelteKit's `$app/state` page object, not stored here.
 */

import { goto } from '$app/navigation';
import { base, resolve } from '$app/paths';
import { page } from '$app/state';
import type { Pathname } from '$app/types';
import { createContext } from 'svelte';
import { MediaQuery } from 'svelte/reactivity';

import type { QueryClient } from '@tanstack/svelte-query';

import type { PanelApi } from './api';
import type { PanelBuild } from './base';
import type { PanelChangeEvent } from './events';
import type { SessionEnded } from './panel-session';
import { DEFAULT_THEME_DISPLAY, isThemeDisplay, type ThemeDisplay } from './preferences';
import { createPrefsSync, type PrefsSync } from './preferences-sync';
import {
  panelDocumentTitle,
  panelRoutePath,
  parsePanelRoute,
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
  readonly base: string;
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
    this.base = base;
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

  get isRootMode(): boolean {
    return page.url.pathname.startsWith(`${this.base}/root`);
  }

  get isInbox(): boolean {
    return (
      page.url.pathname === `${this.base}/inbox` || page.url.pathname === `${this.base}/inbox/`
    );
  }

  get isInvitation(): boolean {
    return page.url.pathname.startsWith(`${this.base}/invite/`);
  }

  /**
   * The address, read from the path rather than from the route's parameters.
   *
   * A parameter is only there if the route that matched happens to name one, and
   * which route matched is a detail of how `src/routes` is laid out: a view that
   * hosts a dialog is routed with the segments after it, one that hosts none is
   * routed without them, and history is routed by name with its section. Reading
   * `params.view` tied these getters to that shape and broke the moment it
   * changed - the installation's history came back as `settings`, and the
   * console's came back as the Root console's own history page.
   *
   * The path says the same thing under every shape, and `parsePanelRoute` is the
   * panel's one reading of it.
   */
  private get parsedRoute(): PanelRoute | null {
    return parsePanelRoute(this.base, page.url.pathname);
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

  openTarget(target: PanelTarget, replaceState = false): Promise<void> {
    return this.navigate(this.routeFor(target, this.currentView), replaceState);
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

  returnToPanel(replaceState = false): void {
    const target = this.returnTarget;
    if (target === null) {
      void goto(resolve(this.returnHref() as Pathname), { replaceState: true });
      return;
    }
    void this.navigate(this.returnRoute(target), replaceState);
  }

  // --- Hrefs ---

  targetHref(target: PanelTarget): string {
    return panelRoutePath(this.base, this.returnRoute(target));
  }

  viewHref(nextView: PanelView): string {
    const target = this.selectedTarget;
    return target === null ? '#' : panelRoutePath(this.base, this.routeFor(target, nextView));
  }

  rootHrefFor(section: RootSection): string {
    return panelRoutePath(this.base, rootSectionRoute(section));
  }

  rootDashboardHref(): string {
    return panelRoutePath(this.base, { rootView: 'overview' });
  }

  rootInstallationsHref(): string {
    return panelRoutePath(this.base, { rootView: 'installations' });
  }

  rootAuditHref(): string {
    return panelRoutePath(this.base, { rootView: 'history-audit' });
  }

  queueHref(): string {
    return panelRoutePath(this.base, { rootView: 'queue' });
  }

  queueRequestHref(request: string): string {
    return panelRoutePath(this.base, { rootView: 'queue-request', request });
  }

  rootFailuresHref(): string {
    return panelRoutePath(this.base, { rootView: 'history-failures' });
  }

  rootInstallationHref(account: string, nextView: RootInstallationView): string {
    return panelRoutePath(this.base, this.rootInstallationRoute(account, nextView));
  }

  returnHref(): string {
    return this.returnTarget === null
      ? `${this.base}/`
      : panelRoutePath(this.base, this.returnRoute(this.returnTarget));
  }

  inboxHref(): string {
    return panelRoutePath(this.base, { personal: 'inbox' });
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

  private routePath(route: PanelRoute): string {
    return panelRoutePath('', route);
  }

  private returnRoute(target: PanelTarget): PanelRoute {
    return this.routeFor(target, this.lastScopedView, this.lastScopedHistorySection);
  }

  private navigate(route: PanelRoute, replaceState = false): Promise<void> {
    return goto(resolve(this.routePath(route) as Pathname), { replaceState });
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
