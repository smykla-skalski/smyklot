<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { createQuery, QueryClientProvider } from '@tanstack/svelte-query';
  import { tick, untrack } from 'svelte';

  import { initializePanel, panelHasSettled } from '#lib/boot.js';
  import { createPanelApi } from '#lib/api.js';
  import { panelAddress } from '#lib/addresses.js';
  import { readPanelBuild } from '#lib/base.js';
  import { basePath } from '#lib/paths.js';
  import { legacyInboxRoute } from '#lib/dialog-route.svelte.js';
  import { readPanelFailure } from '#lib/panel-error.js';
  import { readSignInFailure, signedOutReturn } from '#lib/sign-in-return.js';
  import { PanelSession, setPanelSession } from '#lib/session.svelte.js';
  import { createPanelQueryClient } from '#lib/query-client.js';
  import { applyDocumentTheme } from '#lib/preferences.js';
  import { syncIssues } from '#lib/sync-health.js';
  import { prefText } from '#lib/preferences-sync.js';
  import { rebaseWorkspaceConflicts, saveWorkspaceDrafts } from '#lib/workspace-settings-save.js';
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
    SYNC_SECTIONS,
    SYNC_SECTION_LABELS,
    panelViewSection,
    routeSegmentLabel,
    type PanelView,
    type RootWorkspaceView,
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
  import FindPalette, { type FindEntry } from '#lib/components/FindPalette.svelte';
  import MutationReceipt from '#lib/components/MutationReceipt.svelte';
  import { setFinder } from '#lib/finder.svelte.js';
  import Sidebar, {
    isGroup,
    type SidebarEntry,
    type SidebarRow,
  } from '#lib/components/Sidebar.svelte';
  import SignInPage from '#lib/components/SignInPage.svelte';
  import TopBar from '#lib/components/TopBar.svelte';
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

  /* A sign-in that did not finish comes back to the front door carrying its
     status and code, which is the pair the panel already keeps the words under.
     Read once, from the address the server redirected to. */
  const signInFailure = $derived(readSignInFailure(page.url.search));
  /* Where a reader was going when they were asked who they are, and how to say
     it. The address is the one they are standing on, so a pasted link survives
     the round trip; the server decides whether it will honour it. */
  const returnTo = $derived(signedOutReturn(page.url.pathname, page.url.search));

  /* One box, read by the query client and written by the stream below. It says
     whether changes are arriving as they happen, which is what decides how long
     an answer is trusted - see `query-client`. */
  const streamLiveness = { live: false };
  const queryClient = createPanelQueryClient(streamLiveness);
  const session = new PanelSession(api, build, queryClient, streamLiveness);
  setPanelSession(session);
  const settingsDraftRegistry = new SettingsDraftRegistry();
  setSettingsDraftRegistry(settingsDraftRegistry);
  /* A getter and not the value: what the finder can reach changes with the console and
     the workspace, and a context holding one snapshot would hand the search page the
     answer that was true when the shell mounted. */
  setFinder(() => ({
    entries: findEntries,
    lookup: session.isRootMode ? undefined : findLookup,
    crossLabel: session.isRootMode ? 'this workspace' : 'the console',
    scopeName: session.isRootMode
      ? 'Operations console'
      : (session.selectedTarget?.account.display_name ??
        session.selectedTarget?.account.login ??
        ''),
  }));
  let attentionNotice = $state<'inactive' | null>(null);
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
  const rootValidationProblem = $derived(
    settingsDraftRegistry.validationProblem(ROOT_SETTINGS_SCOPE),
  );
  const rootSettingsConflict = $derived(settingsDraftRegistry.hasConflicts(ROOT_SETTINGS_SCOPE));
  const rootProblemControl = $derived(rootDirtyControls[0]);
  const selectedSettingsScope = $derived.by((): SettingsScope | null => {
    const targetId = session.selectedTarget?.id;
    return targetId === undefined ? null : { type: 'workspace', targetId };
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
  const selectedValidationProblem = $derived(
    selectedSettingsScope === null
      ? null
      : settingsDraftRegistry.validationProblem(selectedSettingsScope),
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
      marked = settingsDraftRegistry.markAttention({ type: 'workspace', targetId }) || marked;
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
      enabled: session.isPublicPage === false,
    }),
    () => queryClient,
  );
  const targetsQuery = createQuery(
    () => ({
      queryKey: ['targets', viewerQuery.data?.account.id],
      queryFn: api.fetchTargets,
      enabled:
        session.isPublicPage === false &&
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
        session.isPublicPage === false &&
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

  /* Reads nothing, so it runs once the routed tree is mounted - which is one of the two
     things the page was waiting on. See `panelHasSettled`. */
  $effect(() => {
    void panelHasSettled(document);
  });

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
      /* A draft that came back from an earlier session says so where the draft is:
         the tree marks every scope holding one, and the composer counts them on the
         page that owns them. It used to raise a notice as well, which announced a
         thing the reader had not done and covered the page saying it. */
      if (settingsDraftRegistry.hydrate(accountId).restoredResources > 0) {
        markEveryDirtySettingsScope();
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
    if (session.isRootMode || session.isPublicPage) return;
    const account = page.params.account;
    if (account === undefined) {
      if (session.targets.length > 0 && session.selectedId === null) {
        const last = session.prefs.get('last_workspace');
        const login = prefText(last);
        const target =
          session.targets.find((t) => t.account.login.toLowerCase() === login.toLowerCase()) ??
          session.targets[0];
        if (target !== undefined) {
          session.selectedId = target.id;
          /* A PERSONAL PAGE STILL HAS A WORKSPACE BEHIND IT, but is not one: the inbox
             and the search page carry no account, and opening one would take a reader
             straight back off the page they asked for. So the workspace is selected -
             the chrome names it and the search reaches its pages - and nothing moves. */
          if (!session.isPersonal) void session.openTarget(target, true);
        }
      }
      return;
    }
    const folded = account.toLowerCase();
    const target = session.targets.find((t) => t.account.login.toLowerCase() === folded);
    if (target !== undefined) {
      session.selectedId = target.id;
      session.prefs.set('last_workspace', target.account.login);
      return;
    }

    const remembered = prefText(session.prefs.get('last_workspace')).toLowerCase();
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
   * `session.isPublicPage` reads the pathname - so every navigation re-ran it,
   * closed the socket and opened another. A new socket answers with `ready`, and
   * `ready` is a full resync, so moving between two views refetched everything
   * on both of them: the panel was telling itself its data was stale because it
   * had just reconnected to say so. Measured at one new socket per navigation.
   *
   * A derived only propagates when its value changes, and this one stays `true`
   * across every address the stream should be open on.
   */
  const streamWanted = $derived(session.streamReady && !session.isPublicPage);

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
    const result = await saveWorkspaceDrafts(
      settingsDraftRegistry,
      targetId,
      api.saveWorkspaceSettings,
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
    rebaseWorkspaceConflicts(settingsDraftRegistry, targetId);
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
    if (control.location.section === 'defaults') return session.viewHref('settings');
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
      return panelAddress({ account, view: 'settings' });
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
    return control.location.section === 'repositories' ? 'Repositories' : 'Workspace settings';
  }

  function openSettingsProblem(): void {
    const control = selectedProblemControl;
    if (control === undefined) return;
    if (control.location.section === 'sync') {
      session.selectSyncSection(syncSection(control) ?? 'overview');
    } else if (control.location.section === 'repositories') {
      session.selectView('repositories');
    } else if (control.location.section === 'defaults') {
      session.selectView('settings');
    }
  }

  function syncSection(control: SettingsDirtyControl): SyncSection | null {
    const section = control.location.path[0];
    return SYNC_SECTIONS.some((candidate) => candidate === section)
      ? (section as SyncSection)
      : null;
  }

  /* Only actionable sync issues earn a navigation badge. */
  const syncPlanQuery = createQuery(
    () => ({
      queryKey: ['sync-plan', session.selectedTarget?.id],
      queryFn: () => api.fetchSyncPlan(session.selectedTarget?.id ?? ''),
      enabled:
        session.isPublicPage === false &&
        session.viewer !== null &&
        !session.isRootMode &&
        session.selectedTarget !== null,
    }),
    () => queryClient,
  );
  const syncStatusQuery = createQuery(
    () => ({
      queryKey: ['sync-status', session.selectedTarget?.id],
      queryFn: () => api.fetchSyncStatus(session.selectedTarget?.id ?? ''),
      enabled:
        !session.isPublicPage &&
        session.viewer !== null &&
        !session.isRootMode &&
        session.selectedTarget !== null,
    }),
    () => queryClient,
  );
  const syncIssueCount = $derived(
    syncIssues(syncStatusQuery.data ?? null, syncPlanQuery.data?.plan ?? null).length || undefined,
  );

  /* How much has stopped, said on the row that opens it. One row of the page is
     asked for and only its total is read - the tree wants the number, not the list.
     Both sides ask, and which one is enabled is what decides which is answered. */
  const failureCountQuery = createQuery(
    () => ({
      queryKey: ['failure-count', session.isRootMode ? 'root' : (session.selectedTarget?.id ?? '')],
      queryFn: () => {
        const ask = { query: '', sort: 'newest' as const, limit: 1, kind: 'all' as const };

        return session.isRootMode
          ? api.fetchRootFailures(ask)
          : api.fetchFailures(session.selectedTarget?.id ?? '', ask);
      },
      enabled:
        session.isPublicPage === false &&
        session.viewer !== null &&
        (session.isRootMode || session.selectedTarget !== null),
    }),
    () => queryClient,
  );
  const failureCount = $derived.by((): number | undefined => {
    const total = failureCountQuery.data?.total ?? 0;

    return total === 0 ? undefined : total;
  });

  /**
   * The workspace tree: every page one row, under the headings that group them.
   *
   * The order and the words are the design's, not the route table's - "Sync status"
   * rather than the segment `overview`, "Repository options" rather than `settings`,
   * and "Workspace settings" standing apart at the foot rather than leading as
   * `Defaults`. No two rows in one tree may share a label, which is why the sync
   * board is Status and its options page says options.
   */
  const workspaceEntries = $derived.by((): SidebarEntry[] => {
    const rows: SidebarEntry[] = [
      {
        id: 'overview',
        label: 'Overview',
        icon: 'gauge',
        href: session.viewHref('overview'),
        active: !session.isInbox && session.currentView === 'overview',
      },
      {
        id: 'repositories',
        label: 'Repositories',
        icon: 'book',
        href: session.viewHref('repositories'),
        active: !session.isInbox && panelViewSection(session.currentView) === 'repositories',
        dirty: selectedSettingsDirtyAt({ section: 'repositories' }),
      },
      {
        id: 'queue',
        label: 'Queue',
        icon: 'pending',
        href: session.queueSectionHref('active'),
        active: !session.isInbox && session.currentView === 'queue',
      },
      { kind: 'group', id: 'group-sync', label: 'Sync' },
      ...SYNC_SECTIONS.filter((section) => section !== 'plan').map((section): SidebarRow => {
        const icons = {
          overview: 'refresh',
          labels: 'tag',
          settings: 'sliders',
          rulesets: 'branch',
          files: 'file',
          plan: 'plan',
        } as const;
        return {
          id: `sync-${section}`,
          label: SYNC_SECTION_LABELS[section],
          icon: icons[section],
          href: session.syncSectionHref(section),
          active:
            !session.isInbox &&
            session.currentView === 'sync' &&
            (session.currentSyncSection === section ||
              (section === 'overview' && session.currentSyncSection === 'plan')),
          count: section === 'overview' ? syncIssueCount : undefined,
          signal: section === 'overview' && syncIssueCount !== undefined,
          dirty:
            section !== 'overview' && selectedSettingsDirtyAt({ section: 'sync', path: [section] }),
        };
      }),
    ];

    if (session.selectedTarget?.capabilities.manage_target_users === true) {
      rows.push(
        { kind: 'group', id: 'group-access', label: 'Access' },
        {
          id: 'access-users',
          label: 'Users',
          icon: 'users',
          href: session.accessHref('users'),
          active: !session.isInbox && session.currentView === 'users',
        },
        {
          id: 'access-invitations',
          label: 'Invitations',
          icon: 'mail',
          href: session.accessHref('invitations'),
          active: !session.isInbox && session.currentView === 'invitations',
        },
      );
    }

    rows.push(
      { kind: 'group', id: 'group-activity', label: 'Activity' },
      {
        id: 'history-audit',
        label: 'Audit',
        icon: 'history',
        href: session.historyHref('audit'),
        active:
          !session.isInbox &&
          session.currentView === 'history' &&
          session.currentHistorySection === 'audit',
      },
      {
        id: 'history-failures',
        label: 'Failures',
        icon: 'failure',
        href: session.historyHref('failures'),
        active:
          !session.isInbox &&
          session.currentView === 'history' &&
          session.currentHistorySection === 'failures',
        count: failureCount,
        signal: failureCount !== undefined,
      },
      {
        id: 'settings',
        label: 'Workspace settings',
        icon: 'gear',
        href: session.viewHref('settings'),
        active: !session.isInbox && panelViewSection(session.currentView) === 'settings',
        dirty: selectedSettingsDirtyAt({ section: 'defaults' }),
        foot: true,
      },
    );

    return rows;
  });

  /** One workspace opened inside the console, as its own group of rows. */
  const rootWorkspaceRows = $derived.by((): SidebarEntry[] => {
    const route = session.currentRootRoute;
    if (route.rootView !== 'workspace') return [];
    const target = session.targets.find(
      (candidate) => candidate.account.login.toLowerCase() === route.account.toLowerCase(),
    );
    const scope: SettingsScope | null =
      target === undefined ? null : { type: 'workspace', targetId: target.id };
    const leaves = [
      { id: 'settings', view: 'settings', label: 'Workspace settings', icon: 'gear' },
      { id: 'repositories', view: 'repositories', label: 'Repositories', icon: 'book' },
      { id: 'users', view: 'users', label: 'Users', icon: 'users' },
      { id: 'invitations', view: 'invitations', label: 'Invitations', icon: 'mail' },
      { id: 'audit', view: 'history', section: 'audit', label: 'Audit', icon: 'history' },
      {
        id: 'failures',
        view: 'history',
        section: 'failures',
        label: 'Failures',
        icon: 'failure',
      },
    ] as const;

    return [
      { kind: 'group', id: 'group-workspace', label: target?.account.login ?? route.account },
      ...leaves.map((leaf): SidebarRow => ({
        id: `workspace-${leaf.id}`,
        label: leaf.label,
        icon: leaf.icon,
        href: session.rootWorkspaceHref(
          route.account,
          leaf.view as RootWorkspaceView,
          'section' in leaf ? leaf.section : undefined,
        ),
        active:
          route.view === leaf.view &&
          (leaf.view !== 'history' ||
            ('section' in leaf && session.currentHistorySection === leaf.section)),
        /* The tree's word and the draft store's word part company here: the page is
           addressed and written "settings", and what it holds is still the workspace's
           defaults for its repositories, which is what the store files them under. */
        dirty:
          scope !== null && (leaf.id === 'settings' || leaf.id === 'repositories')
            ? settingsDraftRegistry.dirtyAt(scope, {
                section: leaf.id === 'settings' ? 'defaults' : leaf.id,
              })
            : undefined,
      })),
    ];
  });

  const rootEntries = $derived.by((): SidebarEntry[] => [
    {
      id: 'overview',
      label: 'Overview',
      icon: 'gauge',
      href: session.rootHrefFor('overview'),
      active: !session.isInbox && session.rootValue === 'overview',
    },
    {
      id: 'workspaces',
      label: 'Workspaces',
      icon: 'book',
      href: session.rootHrefFor('workspaces'),
      active: !session.isInbox && session.rootValue === 'workspaces',
      dirty: dirtyTargetIds.size > 0,
    },
    {
      id: 'queue',
      label: 'Queue',
      icon: 'pending',
      href: session.rootQueueSectionHref('active'),
      active: !session.isInbox && session.rootValue === 'queue',
    },
    {
      id: 'schedules',
      label: 'Schedules',
      icon: 'calendar',
      href: session.rootHrefFor('schedules'),
      active: !session.isInbox && session.rootValue === 'schedules',
    },
    {
      id: 'history-audit',
      label: 'Audit',
      icon: 'history',
      href: session.rootAuditHref(),
      active: !session.isInbox && session.currentRootRoute.rootView === 'history-audit',
    },
    {
      id: 'history-failures',
      label: 'Failures',
      icon: 'failure',
      href: session.rootFailuresHref(),
      active: !session.isInbox && session.currentRootRoute.rootView === 'history-failures',
      count: failureCount,
      signal: failureCount !== undefined,
    },
    { kind: 'group', id: 'group-access', label: 'Access' },
    {
      id: 'access-users',
      label: 'Users',
      icon: 'users',
      href: session.rootAccessHref('users'),
      active: !session.isInbox && session.currentRootRoute.rootView === 'access-users',
    },
    {
      id: 'access-invitations',
      label: 'Invitations',
      icon: 'mail',
      href: session.rootAccessHref('invitations'),
      active: !session.isInbox && session.currentRootRoute.rootView === 'access-invitations',
    },
    { kind: 'group', id: 'group-system', label: 'System' },
    {
      id: 'runtime-service',
      label: 'Service health',
      icon: 'server',
      href: session.rootRuntimeHref('service'),
      active: !session.isInbox && session.currentRootRoute.rootView === 'runtime-service',
    },
    {
      id: 'runtime-settings',
      label: 'Service settings',
      icon: 'gear',
      href: session.rootRuntimeHref('settings'),
      active: !session.isInbox && session.currentRootRoute.rootView === 'runtime-settings',
      dirty: settingsDraftRegistry.dirtyAt(ROOT_SETTINGS_SCOPE, { section: 'runtime' }),
    },
    ...rootWorkspaceRows,
  ]);

  /**
   * What the phone's bar says you are looking at.
   *
   * The tree's own word for the page, not the document title: the tab says
   * "Users | Access | SMYKLOT" because a tab is read out of context, and the bar is
   * read directly under the tree that named it. The inbox is not in either tree, and a
   * page the tree does not carry falls back to the console it is in.
   */
  const topBarTitle = $derived.by(() => {
    if (session.isInbox) return 'Inbox';
    const rows = (session.isRootMode ? rootEntries : workspaceEntries).filter(
      (entry): entry is SidebarRow => !isGroup(entry),
    );
    const here = rows.find((row) => row.active);
    if (here !== undefined) return here.label;
    return session.isRootMode
      ? 'Operations'
      : (session.selectedTarget?.account.display_name ??
          session.selectedTarget?.account.login ??
          'Workspace');
  });

  /** Where a row leads, by its id. The tree is flat, so this is one switch. */
  function openSidebarRow(row: SidebarRow): void {
    drawerOpen = false;
    const [head, tail] = row.id.split('-') as [string, string | undefined];

    if (session.isRootMode) {
      if (head === 'workspace' && tail !== undefined) {
        const route = session.currentRootRoute;
        if (route.rootView !== 'workspace') return;
        if (tail === 'audit' || tail === 'failures') session.selectRootWorkspaceHistory(tail);
        else session.selectRootWorkspace(route.account, tail as RootWorkspaceView);
        return;
      }
      if (head === 'queue') session.selectRootQueueSection('active');
      else if (head === 'access') session.selectRootAccessSection(tail as 'users' | 'invitations');
      else if (head === 'history') session.selectRootHistorySection(tail as 'audit' | 'failures');
      else if (head === 'runtime') session.selectRootRuntimeSection(tail as RootRuntimeSection);
      else if (row.id === 'workspaces') session.selectRootWorkspaces();
      else session.selectRootSection(row.id as RootSection);
      return;
    }

    if (head === 'sync') session.selectSyncSection(tail as SyncSection);
    else if (head === 'access') session.selectUserSection(tail as 'users' | 'invitations');
    else if (head === 'history') session.selectHistorySection(tail as 'audit' | 'failures');
    else if (row.id === 'queue') session.selectQueueSection('active');
    else session.selectView(row.id as PanelView);
  }

  /**
   * What each page IS, for the palette - never what its address is.
   *
   * Beside the tree rather than inside it: a row carries a word a reader chooses
   * from a list they are already looking at, and a search result carries a sentence
   * to someone who has not found the list yet.
   */
  const PAGE_SAYS: Record<string, string> = {
    repositories: 'every repository and its switch',
    queue: 'what Smyklot is about to do',
    schedules: 'when the background work runs',
    'sync-overview': 'the sync board - which repositories are settled',
    'sync-labels': 'the labels every repository should carry',
    'sync-settings': 'repository settings the sync holds in step',
    'sync-rulesets': 'branch protections the sync holds in step',
    'sync-files': 'shared files the sync copies around',
    'sync-plan': 'changes waiting for an apply',
    'access-users': 'who is in this workspace',
    'access-invitations': 'links that bring people in',
    'history-audit': 'everything done here, day by day',
    'history-failures': 'work that stopped, and why',
    settings: 'what every repository here inherits',
    overview: 'what needs an operator',
    workspaces: 'every workspace the service serves',
    'runtime-service': 'the service, its credentials and the store it runs on',
    'runtime-settings': 'what the deployment sets, and what you set here',
  };

  let searchOpen = $state(false);

  function pageEntries(rows: readonly SidebarEntry[], cross?: string): FindEntry[] {
    return rows
      .filter((entry): entry is SidebarRow => !isGroup(entry))
      .map((row) => ({
        group: 'Pages',
        title: row.label,
        say: PAGE_SAYS[row.id] ?? '',
        href: row.href,
        cross,
        select: () => {
          if (cross === undefined) openSidebarRow(row);
          else void goto(row.href);
        },
      }));
  }

  const findEntries = $derived.by((): FindEntry[] => {
    const here = session.isRootMode ? rootEntries : workspaceEntries;
    const there = session.isRootMode ? workspaceEntries : rootEntries;
    const crossName = session.isRootMode ? 'This workspace' : 'Operations';
    return [
      ...pageEntries(here),
      {
        group: 'Pages',
        title: 'Inbox',
        say: 'what happened in the workspaces you belong to',
        href: session.inboxHref(),
        select: () => session.openInbox(),
      },
      ...session.targets
        .filter((target) => target.id !== session.selectedId || session.isRootMode)
        .map((target): FindEntry => ({
          group: 'Workspaces',
          title: target.account.display_name || target.account.login,
          say: `@${target.account.login}`,
          href: session.targetHref(target),
          select: () => void session.selectTarget(target.id),
        })),
      ...pageEntries(there, crossName),
    ];
  });

  /** Repositories and people are asked for, because only the service knows them. */
  async function findLookup(query: string): Promise<FindEntry[]> {
    const targetId = session.selectedTarget?.id;
    if (targetId === undefined) return [];
    const [repositories, people] = await Promise.all([
      session.api
        .fetchRepositories(targetId, {
          query,
          sort: 'name_asc',
          limit: 6,
          state: 'all',
          files: [],
          setting: { mode: 'all' },
        })
        .catch(() => null),
      session.api.suggestUsers(targetId, query).catch(() => null),
    ]);
    const account = session.selectedTarget?.account.login ?? '';
    return [
      ...(repositories?.items ?? []).map((repository): FindEntry => ({
        group: 'Repositories',
        title: repository.name,
        say: repository.effective_enabled ? 'on' : 'off - Smyklot stands down there',
        href: session.repositoryHref(repository.name),
        select: () => session.openRepository(repository.name),
      })),
      ...(people ?? []).slice(0, 5).map((person): FindEntry => ({
        group: 'People',
        title: person.display_name || person.login,
        say: `@${person.login} in ${account}`,
        href: session.accessHref('users'),
        select: () => session.selectUserSection('users'),
      })),
    ];
  }

  function searchShortcut(event: KeyboardEvent): void {
    if (event.key !== 'k' || !(event.metaKey || event.ctrlKey)) return;
    event.preventDefault();
    searchOpen = !searchOpen;
  }

  const showSidebar = $derived(
    session.viewer !== null && (session.isRootMode || session.selectedTarget !== null),
  );
</script>

<svelte:window
  onkeydown={(event) => {
    closeDrawerOnEscape(event);
    searchShortcut(event);
  }}
/>

<svelte:head>
  {#if !session.signedOut}
    <title>{shellDocumentTitle}</title>
  {/if}
</svelte:head>

<QueryClientProvider client={queryClient}>
  {#if pageFailure !== null}
    <ErrorPage {api} base={basePath} {build} failure={pageFailure} />
  {:else if session.isPublicPage}
    {@render children()}
  {:else if session.loading}
    <!-- Which layout this is has not been answered yet, so neither is drawn.
         See `PanelBoot` for what the shell did instead. -->
    <PanelBoot />
  {:else if session.signedOut}
    <SignInPage {api} {build} ended={session.sessionEnded} failed={signInFailure} {returnTo} />
  {:else if session.awaitingWorkspace}
    <NightPage title="No workspaces" documentTitle="No workspaces" {build} size="compact">
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
    {#if showSidebar}
      <FindPalette
        bind:open={searchOpen}
        placeholder={session.isRootMode ? 'Search the console' : 'Search this workspace'}
        entries={findEntries}
        lookup={session.isRootMode ? undefined : findLookup}
        crossLabel={session.isRootMode ? 'this workspace' : 'the console'}
      />
    {/if}
    {#if showSidebar}
      <!-- The phone's whole shell. Above 48rem it draws nothing: the rail and the
           sidebar are both in flow there and this would be a third chrome column. -->
      <TopBar
        open={drawerOpen}
        onToggle={() => (drawerOpen = !drawerOpen)}
        title={topBarTitle}
        targets={session.targets}
        selected={session.selectedTarget}
        targetHref={(target: PanelTarget) => session.targetHref(target)}
        onSelectTarget={(targetId: string) => void session.selectTarget(targetId)}
        {dirtyTargetIds}
        rootMode={session.isRootMode}
        console={session.viewer !== null && session.viewer.system_role !== 'none'
          ? { href: session.rootEntryHref(), onEnter: () => session.enterRoot() }
          : null}
      />
    {/if}
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
      />

      {#if showSidebar}
        <Sidebar
          kicker={session.isRootMode ? 'Console' : 'Workspace'}
          title={session.isRootMode
            ? 'Operations'
            : (session.selectedTarget?.account.display_name ??
              session.selectedTarget?.account.login ??
              '')}
          entries={session.isRootMode ? rootEntries : workspaceEntries}
          collapsed={session.effectiveSidebarCollapsed}
          onToggleCollapsed={() => session.toggleSidebar()}
          onSelectRow={openSidebarRow}
          onOpenSearch={() => (searchOpen = true)}
          searchLabel={session.isRootMode ? 'Search the console' : 'Search this workspace'}
          chrome={{
            targets: session.targets,
            selected: session.selectedTarget,
            targetHref: (target: PanelTarget) => session.targetHref(target),
            onSelectTarget: (targetId: string) => void session.selectTarget(targetId),
            dirtyTargetIds,
            rootMode: session.isRootMode,
            rootEnabled: session.viewer !== null && session.viewer.system_role !== 'none',
            rootEntryHref: session.rootEntryHref(),
            onEnterRoot: () => session.enterRoot(),
            inboxHref: session.inboxHref(),
            inboxActive: session.isInbox,
            onSelectInbox: () => session.openInbox(),
            unreadCount: notificationUnread,
            viewer: session.viewer,
            theme: session.theme,
            onSelectTheme: (t: ThemeDisplay) => session.selectTheme(t),
            onSignOut: signOut,
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

      <div class="workspace">
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
            invalidProblem={selectedValidationProblem}
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
        {:else if session.isRootMode && session.currentRootRoute.rootView !== 'workspace'}
          <SettingsSaveComposer
            count={rootDirtyControls.length}
            saving={rootSettingsOperation.saving}
            resolving={resolvingSettingsConflict}
            problem={rootSettingsOperation.problem}
            invalidProblem={rootValidationProblem}
            problemHref={rootProblemControl === undefined
              ? undefined
              : session.rootRuntimeHref('settings')}
            problemLabel={rootProblemControl === undefined ? undefined : 'Service settings'}
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
  <!-- The shell's, not a page's: a change made in a dialog is reported once the dialog
       has closed, and a receipt a page owned would leave with the page. -->
  <MutationReceipt />
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
    animation: skeleton-pulse var(--rhythm-shimmer) var(--ease-inout) infinite alternate;
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
    color: var(--text-muted);
    margin: var(--space-2) 0 0;
    max-width: var(--measure-note);
  }
  .install-mark {
    align-items: center;
    background: var(--brand-action-tint);
    border: 1px solid color-mix(in srgb, var(--brand-action) 34%, transparent);
    border-radius: var(--radius-control);
    color: var(--brand-action);
    display: inline-flex;
    font: 650 1.5rem/1 var(--sans);
    height: 3rem;
    justify-content: center;
    width: 3rem;
  }
</style>
