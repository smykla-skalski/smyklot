<script lang="ts">
  import { page } from '$app/state';
  import { beforeNavigate, goto } from '$app/navigation';
  import { createQuery, QueryClientProvider } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';

  import { initializePanel } from '#lib/boot.js';
  import { createPanelApi } from '#lib/api.js';
  import { readPanelBuild } from '#lib/base.js';
  import { basePath } from '#lib/paths.js';
  import { legacyInboxRoute } from '#lib/dialog-route.svelte.js';
  import { readPanelFailure } from '#lib/panel-error.js';
  import { PanelSession, setPanelSession } from '#lib/session.svelte.js';
  import { createPanelQueryClient } from '#lib/query-client.js';
  import { applyDocumentTheme } from '#lib/preferences.js';
  import { prefText } from '#lib/preferences-sync.js';
  import {
    setSyncDraftScope,
    staysInSyncDraftInstallation,
    SyncDraftScope,
  } from '#lib/sync-drafts.svelte.js';
  import { SYNC_KINDS, type PanelTarget } from '#lib/types.js';
  import {
    SYNC_SECTIONS,
    panelViewSection,
    routeSegmentLabel,
    type PanelView,
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
  import SyncSaveComposer from '#lib/components/SyncSaveComposer.svelte';
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
  const syncDraftScope = new SyncDraftScope();
  setSyncDraftScope(syncDraftScope);
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

  function confirmDraftDeparture(): boolean {
    const drafts = syncDraftScope.current;
    if (drafts === null || !drafts.dirty) return true;
    if (!window.confirm('Discard your unsaved Sync configuration changes?')) return false;
    syncDraftScope.discard();
    return true;
  }

  beforeNavigate(({ cancel, to, willUnload }) => {
    const drafts = syncDraftScope.current;
    if (drafts === null || !drafts.dirty) return;
    const target = session.targets.find((candidate) => candidate.id === drafts.targetId);
    const staysInInstallation =
      target !== undefined &&
      staysInSyncDraftInstallation(to?.route.id, to?.params?.account, target.account.login);
    if (staysInInstallation) return;

    // SvelteKit turns cancellation of a document unload into the browser's
    // native warning. For an in-app departure we can say exactly what is lost.
    if (willUnload || !confirmDraftDeparture()) cancel();
  });

  async function saveSyncDrafts(): Promise<void> {
    const drafts = syncDraftScope.current;
    if (drafts === null) return;
    if (await drafts.save(session.api.saveSyncConfigs)) {
      session.invalidateTargetData(drafts.targetId);
      await queryClient.invalidateQueries({ queryKey: ['sync-plan', drafts.targetId] });
    }
  }

  async function reloadSyncDrafts(): Promise<void> {
    const drafts = syncDraftScope.current;
    if (drafts === null) return;
    await drafts.refreshAfterConflict((targetId) =>
      Promise.all(SYNC_KINDS.map((kind) => session.api.fetchSyncConfig(targetId, kind))),
    );
  }

  function signOut(): void {
    if (!confirmDraftDeparture()) return;
    void session.signOut();
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

  /* The workspace console's map. Users wears the Access label the section
     grammar already gives it, and stays lit on the invitations view. */
  const WORKSPACE_ORDER = ['settings', 'repositories', 'sync', 'users', 'history'] as const;
  const workspaceIcon = {
    settings: 'sliders',
    repositories: 'repositories',
    sync: 'refresh',
    users: 'users',
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
    })),
  );

  const workspacePages = $derived.by((): SidebarPage[] =>
    WORKSPACE_ORDER.filter(
      (view) =>
        view !== 'users' || session.selectedTarget?.capabilities.manage_target_users === true,
    ).map((view) => ({
      id: view,
      label: routeSegmentLabel(panelViewSection(view)),
      icon: workspaceIcon[view],
      href: session.viewHref(view),
      active:
        !session.isInbox &&
        (session.currentView === view ||
          (view === 'users' && session.currentView === 'invitations')),
      kids: view === 'sync' ? syncKids : undefined,
    })),
  );

  const ROOT_ORDER = [
    'overview',
    'queue',
    'installations',
    'access',
    'history',
    'settings',
  ] as const satisfies readonly RootSection[];
  const rootIcon = {
    overview: 'system',
    queue: 'pending',
    installations: 'repositories',
    access: 'users',
    history: 'history',
    settings: 'sliders',
  } as const;

  const rootPages = $derived.by((): SidebarPage[] =>
    ROOT_ORDER.map((section) => ({
      id: section,
      label: routeSegmentLabel(section),
      icon: rootIcon[section],
      href: session.rootHrefFor(section),
      active: !session.isInbox && session.rootValue === section,
    })),
  );

  const showSidebar = $derived(
    session.viewer !== null && (session.isRootMode || session.selectedTarget !== null),
  );
</script>

<svelte:window onkeydown={closeDrawerOnEscape} />

<svelte:head>
  {#if !session.signedOut}
    <title>{session.documentTitle}</title>
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
            if (session.isRootMode) session.selectRootSection(pageRow.id as RootSection);
            else session.selectView(pageRow.id as PanelView);
          }}
          onSelectKid={(pageRow, kid) => {
            drawerOpen = false;
            if (pageRow.id === 'sync') session.selectSyncSection(kid.id as SyncSection);
          }}
        />
      {/if}

      {#if drawerOpen}
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions
             (Escape already closes the drawer; the scrim is a pointer affordance) -->
        <div class="side-scrim" onclick={() => (drawerOpen = false)}></div>
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
          {:else if session.viewer !== null}
            {@render children()}
          {/if}
        </div>
        {#if !session.isRootMode && syncDraftScope.current !== null && session.selectedTarget !== null}
          <SyncSaveComposer
            drafts={syncDraftScope.current}
            readOnly={!session.selectedTarget.capabilities.write}
            onSave={() => void saveSyncDrafts()}
            onReload={() => void reloadSyncDrafts()}
            sectionHref={(kind) => session.syncSectionHref(kind)}
            onOpenSection={(kind) => session.selectSyncSection(kind)}
          />
        {/if}
        <PageFooter {build} />
      </div>
    </main>
  {/if}
</QueryClientProvider>

<style>
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
