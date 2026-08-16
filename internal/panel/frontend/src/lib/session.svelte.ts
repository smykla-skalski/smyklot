/**
 * Session state shared across the panel's route tree.
 *
 * Absorbed from App.svelte: the viewer, targets, version counters, WebSocket
 * stream, and navigation. Route-derived state (view, root mode, active root
 * route) is read from SvelteKit's `$app/state` page object, not stored here.
 */

import { goto } from '$app/navigation';
import { base } from '$app/paths';
import { page } from '$app/state';
import { MediaQuery } from 'svelte/reactivity';

import type { QueryClient } from '@tanstack/svelte-query';

import type { PanelApi } from './api';
import type { PanelBuild } from './base';
import type { SessionEnded } from './panel-session';
import { DEFAULT_THEME_DISPLAY, isThemeDisplay, type ThemeDisplay } from './preferences';
import { createPrefsSync, type PrefsSync } from './preferences-sync';
import {
  panelDocumentTitle,
  panelRoutePath,
  rootSection,
  rootSectionRoute,
  type HistorySection,
  type PanelRoute,
  type PanelView,
  type RootRoute,
  type RootSection,
  type RouteDialog,
  type ScopedPanelView,
} from './routes';
import type { PanelTarget, PanelViewer, TargetSettingsInput } from './types';

type FailureSource = 'load' | 'sign-out' | 'stream';
type PanelFailure = { message: string; source: FailureSource };

export class PanelSession {
  readonly api: PanelApi;
  readonly build: PanelBuild;
  readonly prefs: PrefsSync;
  readonly base: string;
  readonly queryClient: QueryClient;

  loading = $state(true);
  viewer = $state<PanelViewer | null>(null);
  targets = $state<PanelTarget[]>([]);
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

  get currentView(): PanelView {
    return (page.params.view as PanelView) ?? 'settings';
  }

  get currentHistorySection(): HistorySection {
    const section = page.params.section as HistorySection | undefined;
    if (section === 'audit' || section === 'failures') return section;
    const rest = page.params.rest;
    if (typeof rest === 'string' && (rest === 'audit' || rest === 'failures')) return rest;
    return 'audit';
  }

  get currentRootRoute(): RootRoute {
    if (!this.isRootMode) return { rootView: 'overview' };
    const params = page.params;
    if (params.view !== undefined) {
      const account = params.account as string;
      const view = params.view as ScopedPanelView;
      const route: RootRoute = { rootView: 'installation', account, view };
      if (params.rest !== undefined) {
        const rest = params.rest as string;
        if (view === 'history' && (rest === 'audit' || rest === 'failures')) {
          return { ...route, section: rest };
        }
      }
      return route;
    }
    if (params.section === 'users') return { rootView: 'access-users' };
    if (params.section === 'invitations') return { rootView: 'access-invitations' };
    if (params.section === 'audit') return { rootView: 'history-audit' };
    if (params.section === 'failures') return { rootView: 'history-failures' };
    if (page.url.pathname === `${this.base}/root/installations`)
      return { rootView: 'installations' };
    if (page.url.pathname === `${this.base}/root/settings`) return { rootView: 'settings' };
    return { rootView: 'overview' };
  }

  get rootValue(): RootSection {
    return rootSection(this.currentRootRoute);
  }

  get documentTitle(): string {
    const active = this.isRootMode
      ? this.currentRootRoute
      : this.currentView === 'history'
        ? { account: '', view: this.currentView, section: this.currentHistorySection }
        : { account: '', view: this.currentView };
    return panelDocumentTitle(active as PanelRoute);
  }

  get tableScrollView(): boolean {
    if (this.isRootMode) {
      return (
        this.rootValue === 'history' ||
        this.rootValue === 'access' ||
        this.currentRootRoute.rootView === 'installations'
      );
    }
    return (
      this.selectedTarget !== null &&
      ['repositories', 'users', 'invitations', 'history'].includes(this.currentView)
    );
  }

  // --- Session lifecycle ---

  async load(): Promise<void> {
    this.loading = this.viewer === null;
    this.streamReady = false;
    this.queryClient.clear();
    this.failure = null;
    try {
      this.viewer = await this.api.fetchViewer();
      if (this.viewer === null) {
        this.targets = [];
        this.selectedId = null;
        return;
      }
      this.sessionEnded = null;
      this.prefs.adoptAccount(this.viewer.account.id);
      await this.refreshTargets();
      this.streamReady = true;
    } catch (error) {
      this.setFailure('load', error);
    } finally {
      this.loading = false;
    }
  }

  async refreshTargets(): Promise<boolean> {
    try {
      const listed = await this.api.fetchTargets();
      this.targets = listed;
      if (this.selectedId !== null && !listed.some((t) => t.id === this.selectedId)) {
        this.selectedId = null;
      }
      return true;
    } catch {
      return false;
    }
  }

  // --- Target selection ---

  async selectTarget(targetId: string): Promise<void> {
    const target = this.targets.find((t) => t.id === targetId);
    if (target === undefined || this.selectedId === targetId) return;
    await goto(this.routePath(this.routeFor(target, this.currentView)));
  }

  selectView(nextView: PanelView): void {
    const target = this.selectedTarget;
    if (target === null || this.currentView === nextView) return;
    goto(this.routePath(this.routeFor(target, nextView)));
  }

  selectHistorySection(section: HistorySection): void {
    const target = this.selectedTarget;
    if (target === null || this.currentHistorySection === section) return;
    goto(this.routePath(this.routeFor(target, 'history', section)));
  }

  selectUserSection(section: 'users' | 'invitations'): void {
    if (this.selectedTarget === null) return;
    if (this.currentView === section) return;
    goto(this.routePath(this.routeFor(this.selectedTarget, section)));
  }

  // --- Root navigation ---

  selectRootSection(section: RootSection): void {
    if (!this.isRootMode || this.rootValue === section) return;
    goto(this.routePath(rootSectionRoute(section)));
    this.resetPageScroll();
  }

  selectRootHistorySection(section: 'audit' | 'failures'): void {
    const route: RootRoute = {
      rootView: section === 'audit' ? 'history-audit' : 'history-failures',
    };
    if (this.currentRootRoute.rootView === route.rootView) return;
    goto(this.routePath(route));
  }

  selectRootAccessSection(section: 'users' | 'invitations'): void {
    const route: RootRoute = {
      rootView: section === 'users' ? 'access-users' : 'access-invitations',
    };
    if (this.currentRootRoute.rootView === route.rootView) return;
    goto(this.routePath(route));
  }

  selectRootInstallation(account: string, nextView: ScopedPanelView): void {
    const route = this.rootInstallationRoute(account, nextView);
    goto(this.routePath(route));
    this.resetPageScroll();
  }

  selectRootInstallationHistory(section: HistorySection): void {
    if (this.currentRootRoute.rootView !== 'installation') return;
    if (this.currentHistorySection === section) return;
    const route = this.rootInstallationRoute(this.currentRootRoute.account, 'history', section);
    goto(this.routePath(route));
  }

  selectRootInstallations(): void {
    goto(this.routePath({ rootView: 'installations' }));
    this.resetPageScroll();
  }

  enterRoot(): void {
    if (this.viewer?.system_role === 'none') return;
    goto(this.routePath({ rootView: 'overview' }));
    this.resetPageScroll();
  }

  returnToPanel(): void {
    const target = this.returnTarget;
    if (target === null) return;
    goto(this.routePath(this.routeFor(target, this.currentView)));
  }

  // --- Hrefs ---

  targetHref(target: PanelTarget): string {
    return panelRoutePath(this.base, this.routeFor(target, this.currentView));
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

  rootFailuresHref(): string {
    return panelRoutePath(this.base, { rootView: 'history-failures' });
  }

  rootInstallationHref(account: string, nextView: ScopedPanelView): string {
    return panelRoutePath(this.base, this.rootInstallationRoute(account, nextView));
  }

  returnHref(): string {
    return this.returnTarget === null
      ? '#'
      : panelRoutePath(this.base, this.routeFor(this.returnTarget, this.currentView));
  }

  inboxHref(): string {
    return panelRoutePath(this.base, { personal: 'inbox' });
  }

  openInbox(): void {
    goto(this.inboxHref());
  }

  // --- Mutations ---

  async updateTarget(input: TargetSettingsInput): Promise<void> {
    const target = this.selectedTarget;
    if (target === null) return;
    const updated = await this.api.updateTargetSettings(target.id, input);
    this.targets = this.targets.map((e) => (e.id === updated.id ? updated : e));
    this.invalidateTargetData(target.id);
  }

  repositoryChanged(targetId: string): void {
    if (this.viewer === null) return;
    this.invalidateTargetData(targetId);
    this.refreshFromStreamSafely();
  }

  // --- Stream ---

  async refreshFromStream(refreshViewer = false): Promise<void> {
    try {
      if (refreshViewer) {
        const currentViewer = await this.api.fetchViewer();
        if (currentViewer === null) {
          this.revokeAccess({ code: 'access_revoked', reason: '' });
          return;
        }
        this.viewer = currentViewer;
      }
      await this.refreshTargets();
      if (this.isRootMode && this.viewer?.system_role === 'none') {
        goto(this.routePath(this.routeFor(this.selectedTarget ?? this.targets[0]!, 'settings')), {
          replaceState: true,
        });
      }
      // Invalidate everything the stream could have changed. Each component
      // subscribes to its own key and refetches only what it needs.
      this.queryClient.invalidateQueries();
      this.clearFailure('stream');
    } catch (error) {
      this.setFailure('stream', error);
    }
  }

  refreshFromStreamSafely(): void {
    void this.refreshFromStream();
  }

  refreshAccessFromStream(): void {
    void this.refreshFromStream(true);
  }

  revokeAccess(ended: SessionEnded): void {
    this.sessionEnded = ended;
    this.viewer = null;
    this.targets = [];
    this.selectedId = null;
    this.streamReady = false;
    this.queryClient.clear();
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
      this.queryClient.clear();
      await this.load();
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

  private invalidateTargetData(targetId: string): void {
    this.queryClient.invalidateQueries({ queryKey: ['repositories', targetId] });
    this.queryClient.invalidateQueries({ queryKey: ['audit', targetId] });
    this.queryClient.invalidateQueries({ queryKey: ['failures', targetId] });
    this.queryClient.invalidateQueries({ queryKey: ['users', targetId] });
    this.queryClient.invalidateQueries({ queryKey: ['invitations', targetId] });
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
    nextView: ScopedPanelView,
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
