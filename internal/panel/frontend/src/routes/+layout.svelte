<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { base, resolve } from '$app/paths';
  import type { Pathname } from '$app/types';
  import { createQuery, QueryClientProvider } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';

  import { initializePanel } from '$lib/boot';
  import { createPanelApi } from '$lib/api';
  import { readPanelBuild } from '$lib/base';
  import { legacyInboxRoute } from '$lib/dialog-route.svelte';
  import { readPanelFailure } from '$lib/panel-error';
  import { PanelSession, setPanelSession } from '$lib/session.svelte';
  import { createPanelQueryClient } from '$lib/query-client';
  import { applyDocumentTheme } from '$lib/preferences';
  import { prefText } from '$lib/preferences-sync';
  import type { PanelTarget } from '$lib/types';
  import type { PanelView, RootSection } from '$lib/routes';
  import type { ThemeDisplay } from '$lib/preferences';

  import ErrorPage from '$lib/components/ErrorPage.svelte';
  import IdentityBar from '$lib/components/IdentityBar.svelte';
  import PageFooter from '$lib/components/PageFooter.svelte';
  import Plate from '$lib/components/Plate.svelte';
  import SignInPage from '$lib/components/SignInPage.svelte';
  import NightPage from '$lib/components/NightPage.svelte';

  import '../app.css';

  initializePanel(document);

  const api = createPanelApi(base, (input, init) => fetch(input, init));
  const build = readPanelBuild(document);
  const pageFailure = readPanelFailure(document);

  const queryClient = createPanelQueryClient();
  const session = new PanelSession(api, build, queryClient);
  setPanelSession(session);
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
    session.syncRouteContext(page.params.view, page.params.rest);
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
  $effect(() => {
    if (!session.streamReady || session.isInvitation) return;
    const stream = api.openStream(
      {
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
    void goto(resolve('/inbox' as Pathname), { replaceState: true });
  });
</script>

<svelte:head>
  {#if !session.signedOut}
    <title>{session.documentTitle}</title>
  {/if}
</svelte:head>

<QueryClientProvider client={queryClient}>
  {#if pageFailure !== null}
    <ErrorPage {api} {base} {build} failure={pageFailure} />
  {:else if session.isInvitation}
    {@render children()}
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
        <button class="btn btn-signal" type="button" onclick={() => void session.load()}>
          Reload panel
        </button>
      </div>
    </NightPage>
  {:else}
    <a class="skip-link" href="#panel-content">Skip to panel content</a>
    <main
      class="app-shell"
      class:sidebar-collapsed={session.effectiveSidebarCollapsed}
      class:root-mode={session.isRootMode}
    >
      <IdentityBar
        bind:this={session.identityBar}
        viewer={session.viewer}
        targets={session.targets}
        selectedId={session.selectedId}
        targetHref={(t: PanelTarget) => session.targetHref(t)}
        onSelectTarget={(targetId: string) => void session.selectTarget(targetId)}
        onSignOut={() => void session.signOut()}
        view={session.currentView}
        viewHref={(v: PanelView) => session.viewHref(v)}
        onSelectView={(v: PanelView) => session.selectView(v)}
        showUsers={session.selectedTarget?.capabilities.manage_target_users === true}
        showViews={session.selectedTarget !== null || session.isRootMode}
        showNavigation={session.viewer !== null &&
          (session.isRootMode || session.selectedTarget !== null)}
        collapsed={session.effectiveSidebarCollapsed}
        onToggleCollapsed={() => session.toggleSidebar()}
        theme={session.theme}
        onSelectTheme={(t: ThemeDisplay) => session.selectTheme(t)}
        rootMode={session.isRootMode}
        rootValue={session.rootValue}
        rootHrefFor={(s: RootSection) => session.rootHrefFor(s)}
        onSelectRoot={(s: RootSection) => session.selectRootSection(s)}
        rootDashboardHref={session.rootDashboardHref()}
        onEnterRoot={() => session.enterRoot()}
        returnHref={session.returnHref()}
        onReturnToPanel={() => session.returnToPanel()}
        inboxHref={session.inboxHref()}
        inboxActive={session.isInbox}
        onSelectInbox={() => session.openInbox()}
        unreadCount={notificationUnread}
      />

      <div class="workspace" class:table-scroll-view={session.tableScrollView}>
        <div id="panel-content" class="workspace-content" tabindex="-1">
          {#if session.failure !== null}
            <Plate label="Problem" tone="alarm">
              <p>{session.failure.message}</p>
              <button class="btn" onclick={() => void session.load()}>Try again</button>
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
  @keyframes skeleton-pulse {
    from {
      opacity: 0.52;
    }
    to {
      opacity: 0.9;
    }
  }
</style>
