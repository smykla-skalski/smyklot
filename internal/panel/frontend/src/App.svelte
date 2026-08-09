<script lang="ts">
  import HistoryPanel from './components/HistoryPanel.svelte';
  import HelpPanel from './components/HelpPanel.svelte';
  import IdentityBar from './components/IdentityBar.svelte';
  import PageFooter from './components/PageFooter.svelte';
  import Plate from './components/Plate.svelte';
  import RepositoryList from './components/RepositoryList.svelte';
  import SignedOut from './components/SignedOut.svelte';
  import TargetSettings from './components/TargetSettings.svelte';
  import ViewTabs from './components/ViewTabs.svelte';
  import type { PanelApi } from './lib/api';
  import type { PanelBuild } from './lib/base';
  import { LatestRequest } from './lib/latest-request';
  import { readLastInstallation, writeLastInstallation } from './lib/preferences';
  import {
    resolvePanelRoute,
    type PanelRoute,
    type PanelRouter,
    type PanelView,
  } from './lib/routes';
  import type {
    PanelTarget,
    PanelViewer,
    RepositoryDetail,
    RepositoryPageRequest,
    RepositorySettingsInput,
    TargetSettingsInput,
  } from './lib/types';

  type FailureSource = 'load' | 'sign-out' | 'stream';
  type PanelFailure = { message: string; source: FailureSource };

  const {
    api,
    iconUrl,
    build,
    router,
  }: { api: PanelApi; iconUrl: string; build: PanelBuild; router: PanelRouter } = $props();

  let loading = $state(true);
  let viewer = $state<PanelViewer | null>(null);
  let targets = $state<PanelTarget[]>([]);
  let selectedId = $state<string | null>(null);
  let failure = $state<PanelFailure | null>(null);
  let historyVersion = $state(0);
  let repositoryDetailsVersion = $state(0);
  let view = $state<PanelView>('settings');
  let streamReady = $state(false);
  const targetReads = new LatestRequest();
  const streamRefreshes = new LatestRequest();

  const selectedTarget = $derived(
    selectedId === null ? null : (targets.find((target) => target.id === selectedId) ?? null),
  );

  async function load(): Promise<void> {
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
    if (view === 'help') {
      selectedId = target.id;
      writeLastInstallation(target.account.login);
      failure = null;
      repositoryDetailsVersion += 1;
      historyVersion += 1;
      return;
    }
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
    const resolved = resolvePanelRoute(
      targets.map((target) => target.account.login),
      requested,
      readLastInstallation(),
    );
    if (resolved === null) {
      selectedId = null;
      return;
    }

    const target = targetForAccount(resolved.account);
    if (target === undefined) return;

    const targetChanged = selectedId !== target.id;
    selectedId = target.id;
    view = resolved.view;
    writeLastInstallation(target.account.login);
    const canonical = routeFor(target, resolved.view);

    if (navigation === 'push') {
      router.push(canonical);
    } else if (navigation === 'replace' || !sameRoute(requested, canonical)) {
      router.replace(canonical);
    }

    if (!targetChanged) return;
    failure = null;
    repositoryDetailsVersion += 1;
    historyVersion += 1;
  }

  function targetForAccount(account: string): PanelTarget | undefined {
    const folded = account.toLowerCase();
    return targets.find((target) => target.account.login.toLowerCase() === folded);
  }

  function routeFor(target: PanelTarget, nextView: PanelView): PanelRoute {
    if (nextView === 'help') return { view: 'help' };
    return { account: target.account.login, view: nextView };
  }

  function targetHref(target: PanelTarget): string {
    return router.path(routeFor(target, view));
  }

  function viewHref(nextView: PanelView): string {
    const target = selectedTarget;
    return target === null ? '#' : router.path(routeFor(target, nextView));
  }

  function sameRoute(left: PanelRoute | null, right: PanelRoute): boolean {
    if (left === null || left.view !== right.view) return false;
    if (left.view === 'help' || right.view === 'help') return true;
    return left.account === right.account;
  }

  async function updateTarget(input: TargetSettingsInput): Promise<void> {
    const target = selectedTarget;
    if (target === null) return;
    const updated = await api.updateTargetSettings(target.id, input);
    targetReads.invalidate();
    targets = targets.map((entry) => (entry.id === updated.id ? updated : entry));
    repositoryDetailsVersion += 1;
    historyVersion += 1;
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

  async function refreshFromStream(): Promise<void> {
    const refresh = streamRefreshes.begin();
    try {
      if (!(await refreshTargets()) || !streamRefreshes.isCurrent(refresh)) return;
      if (selectedId === null) {
        await activateRoute(router.current(), 'replace');
      } else if (selectedTarget !== null) {
        writeLastInstallation(selectedTarget.account.login);
        router.replace(routeFor(selectedTarget, view));
      }
      if (selectedId !== null) {
        repositoryDetailsVersion += 1;
        historyVersion += 1;
      }
      clearFailure('stream');
    } catch (error) {
      if (streamRefreshes.isCurrent(refresh)) setFailure('stream', error);
    }
  }

  function refreshFromStreamSafely(): void {
    void refreshFromStream();
  }

  $effect(() => {
    if (viewer === null || !streamReady) return;
    return api.openStream({
      onResync: refreshFromStreamSafely,
      onChange: refreshFromStreamSafely,
    });
  });

  $effect(() =>
    router.subscribe((route) => {
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

  void load();
</script>

<main class="shell">
  <IdentityBar
    {viewer}
    {iconUrl}
    {targets}
    {selectedId}
    {targetHref}
    onSelectTarget={(targetId) => void selectTarget(targetId)}
    onSignOut={signOut}
  />

  {#if failure !== null}
    <Plate label="Problem" tone="alarm">
      <p>{failure.message}</p>
      <button class="btn" onclick={load}>Try again</button>
    </Plate>
  {/if}

  {#if loading}
    <Plate label="Panel">
      <p class="dim">Reading the panel…</p>
    </Plate>
  {:else if viewer === null}
    {#if failure === null}<SignedOut href={api.signInUrl()} />{/if}
  {:else}
    {#if selectedTarget !== null}
      <ViewTabs value={view} hrefFor={viewHref} onSelect={selectView} />

      {#if view === 'settings'}
        <div id="settings-panel" role="tabpanel" aria-labelledby="settings-tab">
          {#key selectedTarget.id}
            <TargetSettings target={selectedTarget} onUpdate={updateTarget} />
          {/key}
        </div>
      {:else if view === 'repositories'}
        <div id="repositories-panel" role="tabpanel" aria-labelledby="repositories-tab">
          {#key selectedTarget.id}
            <RepositoryList
              targetId={selectedTarget.id}
              refreshVersion={repositoryDetailsVersion}
              fetchPage={fetchRepositories}
              onLoad={loadRepository}
              onUpdate={updateRepository}
              onChanged={() => repositoryChanged(selectedTarget.id)}
            />
          {/key}
        </div>
      {:else if view === 'history'}
        <div id="history-panel" role="tabpanel" aria-labelledby="history-tab">
          {#key selectedTarget.id}
            <HistoryPanel
              targetId={selectedTarget.id}
              refreshVersion={historyVersion}
              fetchAudit={(request) => api.fetchAudit(selectedTarget.id, request)}
              fetchFailures={(request) => api.fetchFailures(selectedTarget.id, request)}
            />
          {/key}
        </div>
      {:else}
        <div id="help-panel" role="tabpanel" aria-labelledby="help-tab">
          <HelpPanel />
        </div>
      {/if}
    {/if}
  {/if}

  <PageFooter {build} />
</main>
