<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { createQuery, QueryClientProvider } from '@tanstack/svelte-query';
  import { tick, untrack } from 'svelte';

  import { initializePanel } from '#lib/boot.js';
  import { createPanelApi } from '#lib/api.js';
  import { panelAddress } from '#lib/addresses.js';
  import { readPanelBuild } from '#lib/base.js';
  import { basePath } from '#lib/paths.js';
  import { legacyInboxRoute } from '#lib/dialog-route.svelte.js';
  import { readPanelFailure } from '#lib/panel-error.js';
  import { PanelSession, setPanelSession } from '#lib/session.svelte.js';
  import { createPanelQueryClient } from '#lib/query-client.js';
  import { applyDocumentTheme } from '#lib/preferences.js';
  import { prefText } from '#lib/preferences-sync.js';
  import {
    rebaseInstallationConflicts,
    saveInstallationDrafts,
  } from '#lib/installation-settings-save.js';
  import { rebaseRootSettingsConflict, saveRootSettingsDraft } from '#lib/root-settings-save.js';
  import { ROOT_SETTINGS_SCOPE } from '#lib/runtime-settings.js';
  import {
    setSettingsDraftRegistry,
    SettingsDraftRegistry,
    type SettingsDirtyControl,
    type SettingsLocation,
    type SettingsScope,
  } from '#lib/settings-drafts.svelte.js';
  import { SettingsDraftAttentionController } from '#lib/settings-draft-attention.js';
  import type { PanelTarget } from '#lib/types.js';
  import {
    ACCESS_SECTIONS,
    HISTORY_SECTIONS,
    ROOT_RUNTIME_SECTIONS,
    SYNC_SECTIONS,
    panelViewSection,
    routeSegmentLabel,
    type PanelSection,
    type PanelView,
    type RootInstallationView,
    type RootRuntimeSection,
    type RootSection,
    type SyncSection,
  } from '#lib/routes.js';
  import type { ThemeDisplay } from '#lib/preferences.js';

  import Button from '#lib/components/Button.svelte';
  import ErrorPage from '#lib/components/ErrorPage.svelte';
  import PageFooter from '#lib/components/PageFooter.svelte';
  import Plate from '#lib/components/Plate.svelte';
  import Rail from '#lib/components/Rail.svelte';
  import Sidebar, { type SidebarPage } from '#lib/components/Sidebar.svelte';
  import SignInPage from '#lib/components/SignInPage.svelte';
  import SettingsSaveComposer from '#lib/components/SettingsSaveComposer.svelte';
  import SettingsDraftAttention, {
    type SettingsDraftAttentionKind,
  } from '#lib/components/SettingsDraftAttention.svelte';
  import NightPage from '#lib/components/NightPage.svelte';
  import PanelBoot from '#lib/components/PanelBoot.svelte';

  import '../app.css';

  initializePanel(document);

  const api = createPanelApi(basePath, (input, init) => fetch(input, init));
  const build = readPanelBuild(document);
  const pageFailure = readPanelFailure(document);

  /* One box, read by the query client and written by the stream below. It says
     whether changes are arriving as they happen, which is what decides how long
     an answer is trusted - see `query-client`. */
  const streamLiveness = { live: false };
  const queryClient = createPanelQueryClient(streamLiveness);
  const session = new PanelSession(api, build, queryClient, streamLiveness);
  setPanelSession(session);
  const settingsDraftRegistry = new SettingsDraftRegistry();
  setSettingsDraftRegistry(settingsDraftRegistry);
  let attentionNotice = $state<'restored' | 'inactive' | null>(null);
  let dismissedStorageProblem = $state<string | null>(null);
  let resolvingSettingsConflict = $state(false);
  let selectedSaveProblemControl = $state<SettingsDirtyControl | null>(null);
  const viewerAccountId = $derived(session.viewer?.account.id ?? null);
  const settingsDraftsReady = $derived(
    viewerAccountId === null || settingsDraftRegistry.accountId === viewerAccountId,
  );
  const dirtyTargetIds = $derived.by(() => new Set(settingsDraftRegistry.dirtyTargetIds));
  const firstDirtyControl = $derived.by(
    () =>
      settingsDraftRegistry
        .dirtyControls()
        .toSorted((left, right) => left.changedAt - right.changedAt)[0],
  );
  const settingsAttentionHref = $derived(settingsReviewHref(firstDirtyControl));
  const rootDirty = $derived(settingsDraftRegistry.hasDirty(ROOT_SETTINGS_SCOPE));
  const rootDirtyControls = $derived.by((): SettingsDirtyControl[] =>
    settingsDraftRegistry
      .dirtyControls(ROOT_SETTINGS_SCOPE)
      .toSorted((left, right) => left.changedAt - right.changedAt),
  );
  const rootSettingsOperation = $derived(settingsDraftRegistry.operation(ROOT_SETTINGS_SCOPE));
  const rootSettingsConflict = $derived(settingsDraftRegistry.hasConflicts(ROOT_SETTINGS_SCOPE));
  const rootProblemControl = $derived(rootDirtyControls[0]);
  const selectedSettingsScope = $derived.by((): SettingsScope | null => {
    const targetId = session.selectedTarget?.id;
    return targetId === undefined ? null : { type: 'installation', targetId };
  });
  const selectedDirtyControls = $derived.by((): SettingsDirtyControl[] => {
    if (selectedSettingsScope === null) return [];
    return settingsDraftRegistry
      .dirtyControls(selectedSettingsScope)
      .toSorted((left, right) => left.changedAt - right.changedAt);
  });
  const selectedSettingsOperation = $derived.by(() =>
    selectedSettingsScope === null
      ? { saving: false, problem: null, notice: null }
      : settingsDraftRegistry.operation(selectedSettingsScope),
  );
  const selectedSettingsConflict = $derived(
    selectedSettingsScope !== null && settingsDraftRegistry.hasConflicts(selectedSettingsScope),
  );
  const selectedProblemControl = $derived.by(() => {
    const failed = selectedSaveProblemControl;
    if (
      failed !== null &&
      selectedDirtyControls.some(
        (control) => control.resourceKey === failed.resourceKey && control.id === failed.id,
      )
    ) {
      return failed;
    }
    return selectedDirtyControls[0];
  });
  const selectedProblemHref = $derived(settingsProblemHref(selectedProblemControl));
  const selectedProblemLabel = $derived(settingsProblemLabel(selectedProblemControl));
  const hasSettingsAttention = $derived(settingsDraftRegistry.timestamps().attentionAt !== null);
  const shellDocumentTitle = $derived(
    hasSettingsAttention ? `Unsaved · ${session.documentTitle}` : session.documentTitle,
  );
  const visibleStorageProblem = $derived(
    settingsDraftRegistry.storageProblem !== null &&
      settingsDraftRegistry.storageProblem !== dismissedStorageProblem
      ? settingsDraftRegistry.storageProblem
      : null,
  );
  const shownAttentionKind = $derived.by((): SettingsDraftAttentionKind | null => {
    if (visibleStorageProblem !== null) return 'storage-problem';
    return settingsDraftRegistry.dirty ? attentionNotice : null;
  });

  function selectedSettingsDirtyAt(location: SettingsLocation): boolean {
    return (
      selectedSettingsScope !== null &&
      settingsDraftRegistry.dirtyAt(selectedSettingsScope, location)
    );
  }

  function markEveryDirtySettingsScope(): boolean {
    let marked = false;
    for (const targetId of settingsDraftRegistry.dirtyTargetIds) {
      marked = settingsDraftRegistry.markAttention({ type: 'installation', targetId }) || marked;
    }
    return settingsDraftRegistry.markAttention(ROOT_SETTINGS_SCOPE) || marked;
  }

  function dismissSettingsAttention(): void {
    if (shownAttentionKind === 'storage-problem') {
      dismissedStorageProblem = settingsDraftRegistry.storageProblem;
      return;
    }
    attentionNotice = null;
  }

  const settingsDraftAttentionController = new SettingsDraftAttentionController(document, () => {
    untrack(() => {
      if (markEveryDirtySettingsScope()) attentionNotice = 'inactive';
    });
  });
  const viewerQuery = createQuery(
    () => ({
      queryKey: ['viewer'],
      queryFn: api.fetchViewer,
      enabled: session.isInvitation === false,
    }),
    () => queryClient,
  );
  const targetsQuery = createQuery(
    () => ({
      queryKey: ['targets', viewerQuery.data?.account.id],
      queryFn: api.fetchTargets,
      enabled:
        session.isInvitation === false &&
        viewerQuery.data !== undefined &&
        viewerQuery.data !== null,
    }),
    () => queryClient,
  );
  const notificationCountQuery = createQuery(
    () => ({
      queryKey: ['notifications', 'unread', viewerQuery.data?.account.id],
      queryFn: () => api.fetchNotifications({ limit: 1 }),
      enabled:
        session.isInvitation === false &&
        viewerQuery.data !== undefined &&
        viewerQuery.data !== null &&
        session.isInbox === false,
    }),
    () => queryClient,
  );
  const notificationUnread = $derived(
    notificationCountQuery.data?.unread ?? session.notificationUnread,
  );

  const { children } = $props();

  $effect(() => {
    // `syncRouteContext` reads the route and its parameters, so tracking them is what
    // this depends on.
    session.syncRouteContext();
  });

  $effect(() => {
    const state = {
      viewer: viewerQuery.data,
      targets: targetsQuery.data,
      viewerPending: viewerQuery.isPending,
      targetsPending: targetsQuery.isPending,
      viewerError: viewerQuery.error,
      targetsError: targetsQuery.error,
    };
    untrack(() => session.syncQueries(state));
  });

  $effect(() => {
    const accountId = viewerAccountId;
    untrack(() => {
      if (settingsDraftRegistry.accountId === accountId) return;
      attentionNotice = null;
      dismissedStorageProblem = null;
      const restored = settingsDraftRegistry.hydrate(accountId);
      if (restored.restoredResources > 0) {
        markEveryDirtySettingsScope();
        attentionNotice = 'restored';
      }
    });
  });

  $effect(() => {
    const problem = settingsDraftRegistry.storageProblem;
    if (
      dismissedStorageProblem !== null &&
      (problem === null || problem !== dismissedStorageProblem)
    ) {
      dismissedStorageProblem = null;
    }
  });

  $effect(() => {
    const timestamps = settingsDraftRegistry.timestamps();
    settingsDraftAttentionController.update({
      dirty: settingsDraftRegistry.dirty,
      lastChangedAt: timestamps.lastChangedAt,
      attentionAt: timestamps.attentionAt,
    });
  });

  $effect(() => () => {
    settingsDraftAttentionController.dispose();
    settingsDraftRegistry.dispose();
  });

  // --- Target resolution: watches the route's account param ---
  $effect(() => {
    if (session.viewer === null || session.loading) return;
    if (session.isRootMode || session.isInbox || session.isInvitation) return;
    const account = page.params.account;
    if (account === undefined) {
      if (!session.isRootMode && session.targets.length > 0 && session.selectedId === null) {
        const last = session.prefs.get('last_installation');
        const login = prefText(last);
        const target =
          session.targets.find((t) => t.account.login.toLowerCase() === login.toLowerCase()) ??
          session.targets[0];
        if (target !== undefined) {
          session.selectedId = target.id;
          void session.openTarget(target, true);
        }
      }
      return;
    }
    const folded = account.toLowerCase();
    const target = session.targets.find((t) => t.account.login.toLowerCase() === folded);
    if (target !== undefined) {
      session.selectedId = target.id;
      session.prefs.set('last_installation', target.account.login);
      return;
    }

    const remembered = prefText(session.prefs.get('last_installation')).toLowerCase();
    const fallback =
      session.targets.find((candidate) => candidate.account.login.toLowerCase() === remembered) ??
      session.targets[0];
    if (fallback !== undefined) {
      session.selectedId = fallback.id;
      void session.openTarget(fallback, true);
    }
  });

  // --- WebSocket stream ---
  /**
   * The stream belongs to the session, not to the route.
   *
   * This is the whole dependency, as a value rather than as the reads that
   * produce it. The condition used to be written inside the effect, where
   * `session.isInvitation` reads the pathname - so every navigation re-ran it,
   * closed the socket and opened another. A new socket answers with `ready`, and
   * `ready` is a full resync, so moving between two views refetched everything
   * on both of them: the panel was telling itself its data was stale because it
   * had just reconnected to say so. Measured at one new socket per navigation.
   *
   * A derived only propagates when its value changes, and this one stays `true`
   * across every address the stream should be open on.
   */
  const streamWanted = $derived(session.streamReady && !session.isInvitation);

  $effect(() => {
    if (!streamWanted) return;

    /* Nothing inside is a dependency. `dialQuery` is read from the preference
       state the stream itself then writes to, which is the other way this effect
       could come to re-run on its own output. */
    return untrack(() => {
      const stream = api.openStream(
        {
          onLive: (live) => session.setStreamLive(live),
          onResync: () => session.refreshAccessFromStream(),
          onChange: (event) => session.invalidateChange(event),
          onRevoked: (event) => session.revokeAccess(event),
          onPrefsReady: (info) => session.prefs.onPrefsReady(info),
          onPrefsChanged: (event) => session.prefs.onPrefsChanged(event),
          onPrefsRejected: (keys) => session.prefs.onPrefsRejected(keys),
        },
        session.prefs.dialQuery,
      );
      session.prefs.attach(stream.send);

      return () => {
        session.prefs.detach();
        stream.stop();
      };
    });
  });

  // --- Prefs live-sync to app-level controls ---
  $effect(() =>
    session.prefs.subscribe((keys: string[]) => {
      if (keys.includes('theme')) session.theme = session.storedTheme();
      if (keys.includes('sidebar'))
        session.sidebarCollapsed = session.prefs.get('sidebar') === 'collapsed';
    }),
  );

  // --- Session lost ---
  $effect(() =>
    api.onSessionLost((code) => {
      if (session.viewer === null) return;
      session.revokeAccess({ code, reason: '' });
    }),
  );

  // --- Theme application ---
  $effect(() => {
    applyDocumentTheme(document, session.resolvedTheme, session.isRootMode);
  });

  // Bookmarks from when the inbox was a dialog still lead to the inbox page.
  $effect(() => {
    if (!legacyInboxRoute(page.url.search) || session.isInbox) return;
    void goto(session.inboxHref(), { replace: true });
  });

  // --- The shell's two columns ---
  /* Below 64rem the sidebar is a drawer over the content; the rail's pages
     toggle opens it, and any navigation, the scrim or Escape closes it. */
  let drawerOpen = $state(false);

  $effect(() => {
    void page.url.pathname;
    drawerOpen = false;
  });

  function closeDrawerOnEscape(event: KeyboardEvent): void {
    if (event.key === 'Escape' && drawerOpen) drawerOpen = false;
  }

  function signOut(): void {
    void session.signOut();
  }

  async function saveSelectedSettings(): Promise<void> {
    const targetId = session.selectedTarget?.id;
    if (targetId === undefined) return;
    selectedSaveProblemControl = null;
    const result = await saveInstallationDrafts(
      settingsDraftRegistry,
      targetId,
      api.saveInstallationSettings,
    );
    if (!result.saved) {
      selectedSaveProblemControl = result.problemControl ?? null;
      return;
    }
    session.repositoryChanged(targetId);
    await queryClient.invalidateQueries({ queryKey: ['sync-plan', targetId] });
  }

  async function saveRootSettings(): Promise<void> {
    const result = await saveRootSettingsDraft(
      settingsDraftRegistry,
      api.fetchRootRuntimeSettings,
      api.saveRootRuntimeSettings,
    );
    if (!result.saved || result.settings === undefined) return;
    queryClient.setQueryData(['root-settings'], result.settings);
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['root-overview'] }),
      queryClient.invalidateQueries({ queryKey: ['audit', 'root'] }),
    ]);
  }

  async function updateRootSettingsDraft(): Promise<void> {
    if (resolvingSettingsConflict) return;
    resolvingSettingsConflict = true;
    await tick();
    try {
      const latest = await api.fetchRootRuntimeSettings();
      queryClient.setQueryData(['root-settings'], latest);
      rebaseRootSettingsConflict(settingsDraftRegistry, latest);
      settingsDraftRegistry.resolveExternalConflicts(ROOT_SETTINGS_SCOPE);
      if (!settingsDraftRegistry.hasConflicts(ROOT_SETTINGS_SCOPE)) {
        settingsDraftRegistry.dismissProblem(ROOT_SETTINGS_SCOPE);
      }
    } catch (cause) {
      const attempt = settingsDraftRegistry.beginSave(ROOT_SETTINGS_SCOPE);
      if (attempt !== null) {
        settingsDraftRegistry.failSave(
          attempt,
          cause instanceof Error ? cause.message : String(cause),
        );
      }
    } finally {
      resolvingSettingsConflict = false;
    }
  }

  function discardRootSettings(): void {
    settingsDraftRegistry.discardScope(ROOT_SETTINGS_SCOPE);
  }

  function dismissRootSettingsNotice(): void {
    settingsDraftRegistry.dismissNotice(ROOT_SETTINGS_SCOPE);
  }

  function openRootSettingsProblem(): void {
    session.selectRootRuntimeSection('settings');
  }

  async function updateSelectedSettingsDraft(): Promise<void> {
    const targetId = session.selectedTarget?.id;
    if (targetId === undefined || selectedSettingsScope === null) return;
    const scope = selectedSettingsScope;
    selectedSaveProblemControl = null;
    resolvingSettingsConflict = true;
    await tick();
    rebaseInstallationConflicts(settingsDraftRegistry, targetId);
    settingsDraftRegistry.resolveExternalConflicts(scope);
    if (!settingsDraftRegistry.hasConflicts(scope)) {
      settingsDraftRegistry.dismissProblem(scope);
    }
    resolvingSettingsConflict = false;
  }

  function discardSelectedSettings(): void {
    selectedSaveProblemControl = null;
    if (selectedSettingsScope !== null) settingsDraftRegistry.discardScope(selectedSettingsScope);
  }

  function dismissSelectedSettingsNotice(): void {
    if (selectedSettingsScope !== null) settingsDraftRegistry.dismissNotice(selectedSettingsScope);
  }

  function settingsProblemHref(control: SettingsDirtyControl | undefined): string | undefined {
    if (control === undefined) return undefined;
    if (control.location.section === 'sync') {
      const section = syncSection(control);
      return section === null ? session.viewHref('sync') : session.syncSectionHref(section);
    }
    if (control.location.section === 'repositories') return session.viewHref('repositories');
    if (control.location.section === 'defaults') return session.viewHref('defaults');
    return undefined;
  }

  function settingsReviewHref(control: SettingsDirtyControl | undefined): string | undefined {
    if (control === undefined) return undefined;
    if (control.resource.type === 'runtime') return session.rootRuntimeHref('settings');

    const targetId = control.resource.targetId;
    const target = session.targets.find((candidate) => candidate.id === targetId);
    if (target === undefined) return undefined;
    const account = target.account.login;
    if (control.location.section === 'repositories') {
      return panelAddress({ account, view: 'repositories' });
    }
    if (control.location.section === 'defaults') {
      return panelAddress({ account, view: 'defaults' });
    }
    if (control.location.section === 'sync') {
      const section = syncSection(control) ?? 'overview';
      return section === 'overview'
        ? panelAddress({ account, view: 'sync' })
        : panelAddress({ account, view: 'sync', sync: section });
    }
    return undefined;
  }

  function settingsProblemLabel(control: SettingsDirtyControl | undefined): string | undefined {
    if (control === undefined) return undefined;
    if (control.location.section === 'sync') {
      const section = syncSection(control);
      return section === null ? 'Sync' : routeSegmentLabel(section);
    }
    return control.location.section === 'repositories' ? 'Repositories' : 'Workspace defaults';
  }

  function openSettingsProblem(): void {
    const control = selectedProblemControl;
    if (control === undefined) return;
    if (control.location.section === 'sync') {
      session.selectSyncSection(syncSection(control) ?? 'overview');
    } else if (control.location.section === 'repositories') {
      session.selectView('repositories');
    } else if (control.location.section === 'defaults') {
      session.selectView('defaults');
    }
  }

  function syncSection(control: SettingsDirtyControl): SyncSection | null {
    const section = control.location.path[0];
    return SYNC_SECTIONS.some((candidate) => candidate === section)
      ? (section as SyncSection)
      : null;
  }

  /* The waiting plan's scale, spoken quietly on the sidebar's Plan row. Only a
     computed plan is waiting on anyone - an applied or expired one is history. */
  const syncPlanQuery = createQuery(
    () => ({
      queryKey: ['sync-plan', session.selectedTarget?.id],
      queryFn: () => api.fetchSyncPlan(session.selectedTarget?.id ?? ''),
      enabled:
        session.isInvitation === false &&
        session.viewer !== null &&
        !session.isRootMode &&
        session.selectedTarget !== null,
    }),
    () => queryClient,
  );
  const planCount = $derived.by((): number | undefined => {
    const plan = syncPlanQuery.data?.plan;
    if (plan === null || plan === undefined || plan.state !== 'computed') return undefined;
    return plan.counts.create + plan.counts.update + plan.counts.delete;
  });

  const WORKSPACE_ORDER = [
    'defaults',
    'repositories',
    'sync',
    'queue',
    'schedules',
    'access',
    'history',
  ] as const satisfies readonly PanelSection[];
  const workspaceIcon = {
    defaults: 'sliders',
    repositories: 'repositories',
    sync: 'refresh',
    queue: 'pending',
    schedules: 'sliders',
    access: 'users',
    history: 'history',
  } as const;

  const syncKids = $derived(
    SYNC_SECTIONS.map((section) => ({
      id: section,
      label: routeSegmentLabel(section),
      href: session.syncSectionHref(section),
      active:
        !session.isInbox &&
        session.currentView === 'sync' &&
        session.currentSyncSection === section,
      count: section === 'plan' ? planCount : undefined,
      signal: section === 'plan' && planCount !== undefined,
      dirty:
        section !== 'overview' &&
        section !== 'plan' &&
        selectedSettingsDirtyAt({ section: 'sync', path: [section] }),
    })),
  );

  const accessKids = $derived(
    ACCESS_SECTIONS.map((section) => ({
      id: section,
      label: routeSegmentLabel(section),
      href: session.accessHref(section),
      active: !session.isInbox && session.currentView === section,
    })),
  );

  const historyKids = $derived(
    HISTORY_SECTIONS.map((section) => ({
      id: section,
      label: routeSegmentLabel(section),
      href: session.historyHref(section),
      active:
        !session.isInbox &&
        session.currentView === 'history' &&
        session.currentHistorySection === section,
    })),
  );

  const workspacePages = $derived.by((): SidebarPage[] =>
    WORKSPACE_ORDER.filter(
      (section) =>
        section !== 'access' || session.selectedTarget?.capabilities.manage_target_users === true,
    ).map((section) => {
      const view: PanelView = section === 'access' ? 'users' : section;
      return {
        id: section,
        label: routeSegmentLabel(section),
        icon: workspaceIcon[section],
        href:
          section === 'sync'
            ? session.syncSectionHref('overview')
            : section === 'access'
              ? session.accessHref('users')
              : section === 'history'
                ? session.historyHref('audit')
                : session.viewHref(view),
        active: !session.isInbox && panelViewSection(session.currentView) === section,
        dirty:
          section === 'defaults'
            ? selectedSettingsDirtyAt({ section: 'defaults' })
            : section === 'repositories'
              ? selectedSettingsDirtyAt({ section: 'repositories' })
              : undefined,
        kids:
          section === 'sync'
            ? syncKids
            : section === 'access'
              ? accessKids
              : section === 'history'
                ? historyKids
                : undefined,
      };
    }),
  );

  const ROOT_ORDER = [
    'overview',
    'queue',
    'schedules',
    'installations',
    'access',
    'history',
    'runtime',
  ] as const satisfies readonly RootSection[];
  const rootIcon = {
    overview: 'system',
    queue: 'pending',
    schedules: 'sliders',
    installations: 'repositories',
    access: 'users',
    history: 'history',
    runtime: 'sliders',
  } as const;

  const rootAccessKids = $derived(
    ACCESS_SECTIONS.map((section) => ({
      id: section,
      label: routeSegmentLabel(section),
      href: session.rootAccessHref(section),
      active:
        !session.isInbox &&
        (section === 'users'
          ? session.currentRootRoute.rootView === 'access-users'
          : session.currentRootRoute.rootView === 'access-invitations'),
    })),
  );

  const rootHistoryKids = $derived(
    HISTORY_SECTIONS.map((section) => ({
      id: section,
      label: routeSegmentLabel(section),
      href: section === 'audit' ? session.rootAuditHref() : session.rootFailuresHref(),
      active:
        !session.isInbox &&
        (section === 'audit'
          ? session.currentRootRoute.rootView === 'history-audit'
          : session.currentRootRoute.rootView === 'history-failures'),
    })),
  );

  const rootRuntimeKids = $derived(
    ROOT_RUNTIME_SECTIONS.map((section) => ({
      id: section,
      label: routeSegmentLabel(section),
      href: session.rootRuntimeHref(section),
      active: !session.isInbox && session.currentRootRoute.rootView === `runtime-${section}`,
      dirty:
        section === 'settings' &&
        settingsDraftRegistry.dirtyAt(ROOT_SETTINGS_SCOPE, { section: 'runtime' }),
    })),
  );

  const rootInstallationKids = $derived.by(() => {
    const route = session.currentRootRoute;
    if (route.rootView !== 'installation') return undefined;
    const target = session.targets.find(
      (candidate) => candidate.account.login.toLowerCase() === route.account.toLowerCase(),
    );
    const scope: SettingsScope | null =
      target === undefined ? null : { type: 'installation', targetId: target.id };
    const leaves = [
      { id: 'defaults', view: 'defaults' },
      { id: 'repositories', view: 'repositories' },
      { id: 'users', view: 'users' },
      { id: 'invitations', view: 'invitations' },
      { id: 'audit', view: 'history', section: 'audit' },
      { id: 'failures', view: 'history', section: 'failures' },
    ] as const;

    return leaves.map((leaf) => ({
      id: leaf.id,
      label: routeSegmentLabel(leaf.id),
      href: session.rootInstallationHref(
        route.account,
        leaf.view as RootInstallationView,
        'section' in leaf ? leaf.section : undefined,
      ),
      active:
        route.view === leaf.view &&
        (leaf.view !== 'history' ||
          ('section' in leaf && session.currentHistorySection === leaf.section)),
      dirty:
        scope !== null && (leaf.id === 'defaults' || leaf.id === 'repositories')
          ? settingsDraftRegistry.dirtyAt(scope, { section: leaf.id })
          : undefined,
    }));
  });

  const rootPages = $derived.by((): SidebarPage[] =>
    ROOT_ORDER.map((section) => ({
      id: section,
      label: routeSegmentLabel(section),
      icon: rootIcon[section],
      href: session.rootHrefFor(section),
      active: !session.isInbox && session.rootValue === section,
      dirty:
        section === 'installations'
          ? dirtyTargetIds.size > 0
          : section === 'runtime'
            ? rootDirty
            : undefined,
      kids:
        section === 'access'
          ? rootAccessKids
          : section === 'history'
            ? rootHistoryKids
            : section === 'runtime'
              ? rootRuntimeKids
              : section === 'installations'
                ? rootInstallationKids
                : undefined,
    })),
  );

  const showSidebar = $derived(
    session.viewer !== null && (session.isRootMode || session.selectedTarget !== null),
  );
</script>

<svelte:window onkeydown={closeDrawerOnEscape} />

<svelte:head>
  {#if !session.signedOut}
    <title>{shellDocumentTitle}</title>
  {/if}
</svelte:head>

<QueryClientProvider client={queryClient}>
  {#if pageFailure !== null}
    <ErrorPage {api} base={basePath} {build} failure={pageFailure} />
  {:else if session.isInvitation}
    {@render children()}
  {:else if session.loading}
    <!-- Which layout this is has not been answered yet, so neither is drawn.
         See `PanelBoot` for what the shell did instead. -->
    <PanelBoot />
  {:else if session.signedOut}
    <SignInPage {api} {build} ended={session.sessionEnded} />
  {:else if session.awaitingInstallation}
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
        <Button tone="signal" onclick={() => void session.load()}>Reload panel</Button>
      </div>
    </NightPage>
  {:else}
    <a class="skip-link" href="#panel-content">Skip to panel content</a>
    <main
      class="app-shell"
      class:sidebar-collapsed={session.effectiveSidebarCollapsed}
      class:root-mode={session.isRootMode}
      class:side-open={drawerOpen}
    >
      <Rail
        viewer={session.viewer}
        targets={session.targets}
        selectedId={session.selectedId}
        targetHref={(t: PanelTarget) => session.targetHref(t)}
        onSelectTarget={(targetId: string) => void session.selectTarget(targetId)}
        rootMode={session.isRootMode}
        rootEnabled={session.viewer !== null && session.viewer.system_role !== 'none'}
        rootEntryHref={session.rootEntryHref()}
        onEnterRoot={() => session.enterRoot()}
        inboxHref={session.inboxHref()}
        inboxActive={session.isInbox}
        onSelectInbox={() => session.openInbox()}
        unreadCount={notificationUnread}
        {dirtyTargetIds}
        {rootDirty}
        theme={session.theme}
        onSelectTheme={(t: ThemeDisplay) => session.selectTheme(t)}
        onSignOut={signOut}
        pagesOpen={drawerOpen}
        onTogglePages={() => (drawerOpen = !drawerOpen)}
      />

      {#if showSidebar}
        <Sidebar
          kicker={session.isRootMode ? 'Root console' : 'Workspace'}
          title={session.isRootMode
            ? 'Operations'
            : (session.selectedTarget?.account.display_name ??
              session.selectedTarget?.account.login ??
              '')}
          pages={session.isRootMode ? rootPages : workspacePages}
          collapsed={session.effectiveSidebarCollapsed}
          onToggleCollapsed={() => session.toggleSidebar()}
          onSelectPage={(pageRow) => {
            drawerOpen = false;
            if (session.isRootMode) {
              if (pageRow.id === 'access') session.selectRootAccessSection('users');
              else if (pageRow.id === 'history') session.selectRootHistorySection('audit');
              else if (pageRow.id === 'installations') session.selectRootInstallations();
              else session.selectRootSection(pageRow.id as RootSection);
            } else if (pageRow.id === 'sync') session.selectSyncSection('overview');
            else if (pageRow.id === 'access') session.selectUserSection('users');
            else if (pageRow.id === 'history') session.selectHistorySection('audit');
            else session.selectView(pageRow.id as PanelView);
          }}
          onSelectKid={(pageRow, kid) => {
            drawerOpen = false;
            if (!session.isRootMode) {
              if (pageRow.id === 'sync') session.selectSyncSection(kid.id as SyncSection);
              else if (pageRow.id === 'access')
                session.selectUserSection(kid.id as 'users' | 'invitations');
              else if (pageRow.id === 'history')
                session.selectHistorySection(kid.id as 'audit' | 'failures');
              return;
            }

            if (pageRow.id === 'queue') session.selectQueueSection(kid.id as 'waiting' | 'recent');
            else if (pageRow.id === 'access')
              session.selectRootAccessSection(kid.id as 'users' | 'invitations');
            else if (pageRow.id === 'history')
              session.selectRootHistorySection(kid.id as 'audit' | 'failures');
            else if (pageRow.id === 'runtime')
              session.selectRootRuntimeSection(kid.id as RootRuntimeSection);
            else if (pageRow.id === 'installations') {
              const route = session.currentRootRoute;
              if (route.rootView !== 'installation') return;
              if (kid.id === 'audit' || kid.id === 'failures') {
                session.selectRootInstallationHistory(kid.id);
              } else {
                session.selectRootInstallation(route.account, kid.id as RootInstallationView);
              }
            }
          }}
        />
      {/if}

      {#if drawerOpen}
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions
             (Escape already closes the drawer; the scrim is a pointer affordance) -->
        <div class="side-scrim" onclick={() => (drawerOpen = false)}></div>
      {/if}

      {#if shownAttentionKind !== null}
        <div class="shell-notification-layer">
          <SettingsDraftAttention
            kind={shownAttentionKind}
            count={settingsDraftRegistry.dirtyControlCount}
            problem={visibleStorageProblem}
            reviewHref={settingsAttentionHref}
            onDismiss={dismissSettingsAttention}
          />
        </div>
      {/if}

      <div class="workspace" class:table-scroll-view={session.tableScrollView}>
        <div id="panel-content" class="workspace-content" tabindex="-1">
          {#if session.failure !== null}
            <Plate label="Problem" tone="alarm">
              <p>{session.failure.message}</p>
              <Button onclick={() => void session.load()}>Try again</Button>
            </Plate>
          {/if}
          {#if session.loading && session.viewer === null}
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
          {:else if session.viewer !== null && !settingsDraftsReady}
            <p class="visually-hidden" role="status">Restoring unsaved settings</p>
          {:else if session.viewer !== null}
            {@render children()}
          {/if}
        </div>
        {#if !session.isRootMode && selectedSettingsScope !== null && session.selectedTarget !== null}
          <SettingsSaveComposer
            count={selectedDirtyControls.length}
            saving={selectedSettingsOperation.saving}
            resolving={resolvingSettingsConflict}
            problem={selectedSettingsOperation.problem}
            problemHref={selectedProblemHref}
            problemLabel={selectedProblemLabel}
            notice={selectedSettingsOperation.notice}
            conflict={selectedSettingsConflict}
            readOnly={!session.selectedTarget.capabilities.write}
            onSave={() => void saveSelectedSettings()}
            onDiscard={discardSelectedSettings}
            onResolveConflict={() => void updateSelectedSettingsDraft()}
            onDismiss={dismissSelectedSettingsNotice}
            onOpenProblem={openSettingsProblem}
          />
        {:else if session.isRootMode && session.currentRootRoute.rootView !== 'installation'}
          <SettingsSaveComposer
            count={rootDirtyControls.length}
            saving={rootSettingsOperation.saving}
            resolving={resolvingSettingsConflict}
            problem={rootSettingsOperation.problem}
            problemHref={rootProblemControl === undefined
              ? undefined
              : session.rootRuntimeHref('settings')}
            problemLabel={rootProblemControl === undefined ? undefined : 'Runtime settings'}
            notice={rootSettingsOperation.notice}
            conflict={rootSettingsConflict}
            onSave={() => void saveRootSettings()}
            onDiscard={discardRootSettings}
            onResolveConflict={() => void updateRootSettingsDraft()}
            onDismiss={dismissRootSettingsNotice}
            onOpenProblem={openRootSettingsProblem}
          />
        {/if}
        <PageFooter {build} />
      </div>
    </main>
  {/if}
</QueryClientProvider>

<style>
  .shell-notification-layer {
    inset-block-start: var(--space-4);
    inset-inline-end: var(--space-4);
    max-width: calc(100vw - 2 * var(--space-4));
    pointer-events: none;
    position: fixed;
    width: 32rem;
    z-index: 40;
  }

  .shell-notification-layer :global(*) {
    pointer-events: auto;
  }

  .panel-skeleton {
    display: grid;
    gap: var(--space-3);
  }
  .skeleton-line {
    /* The panel's one placeholder pulse, from `app.css`. The endpoints stay this
       placeholder's own - the eight copies of this keyframe had drifted into three
       different ranges, and collapsing them was not allowed to move any of them. */
    --skeleton-from: 0.52;
    --skeleton-to: 0.9;
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
</style>
