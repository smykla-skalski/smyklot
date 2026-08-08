<script lang="ts">
  import HistoryPanel from './components/HistoryPanel.svelte';
  import HelpPanel from './components/HelpPanel.svelte';
  import IdentityBar from './components/IdentityBar.svelte';
  import PageFooter from './components/PageFooter.svelte';
  import Plate from './components/Plate.svelte';
  import RepositoryList from './components/RepositoryList.svelte';
  import SignedOut from './components/SignedOut.svelte';
  import TargetSettings from './components/TargetSettings.svelte';
  import ViewTabs, { type PanelView } from './components/ViewTabs.svelte';
  import type { PanelApi } from './lib/api';
  import type { PanelBuild } from './lib/base';
  import { LatestRequest } from './lib/latest-request';
  import type {
    PanelTarget,
    PanelViewer,
    RepositoryDetail,
    RepositorySettingsInput,
    RepositorySummary,
    TargetSettingsInput,
  } from './lib/types';

  type FailureSource = 'load' | 'repositories' | 'sign-out' | 'stream';
  type PanelFailure = { message: string; source: FailureSource };

  const { api, iconUrl, build }: { api: PanelApi; iconUrl: string; build: PanelBuild } = $props();

  let loading = $state(true);
  let repositoriesLoading = $state(false);
  let viewer = $state<PanelViewer | null>(null);
  let targets = $state<PanelTarget[]>([]);
  let selectedId = $state<string | null>(null);
  let repositories = $state<RepositorySummary[]>([]);
  let failure = $state<PanelFailure | null>(null);
  let historyVersion = $state(0);
  let repositoryDetailsVersion = $state(0);
  let view = $state<PanelView>('settings');
  let streamReady = $state(false);
  const targetReads = new LatestRequest();
  const repositoryReads = new LatestRequest();
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
        repositories = [];
        return;
      }
      if (!(await refreshTargets())) return;
      if (selectedId !== null) await loadRepositories(selectedId);
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
    if (selectedId === null || !listed.some((target) => target.id === selectedId)) {
      const nextSelectedId = listed[0]?.id ?? null;
      if (selectedId !== nextSelectedId) {
        selectedId = nextSelectedId;
        repositories = [];
        repositoryReads.invalidate();
      }
    }

    return true;
  }

  async function selectTarget(targetId: string): Promise<void> {
    if (selectedId === targetId) return;
    selectedId = targetId;
    repositories = [];
    failure = null;
    await loadRepositories(targetId);
    historyVersion += 1;
  }

  async function loadRepositories(
    targetId: string,
    isRelevant: () => boolean = () => true,
  ): Promise<boolean> {
    const read = repositoryReads.begin();
    repositoriesLoading = true;
    try {
      const listed = await api.fetchRepositories(targetId);
      if (repositoryReads.isCurrent(read) && selectedId === targetId && isRelevant()) {
        repositories = listed;
        repositoryDetailsVersion += 1;
        clearFailure('repositories');
        return true;
      }
    } catch (error) {
      if (repositoryReads.isCurrent(read) && isRelevant()) setFailure('repositories', error);
    } finally {
      if (repositoryReads.isCurrent(read)) repositoriesLoading = false;
    }

    return false;
  }

  async function updateTarget(input: TargetSettingsInput): Promise<void> {
    const target = selectedTarget;
    if (target === null) return;
    const updated = await api.updateTargetSettings(target.id, input);
    targetReads.invalidate();
    targets = targets.map((entry) => (entry.id === updated.id ? updated : entry));
    if (selectedId !== null) await loadRepositories(selectedId);
    historyVersion += 1;
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

  function repositoryChanged(targetId: string, detail: RepositoryDetail): void {
    if (viewer === null) return;
    if (selectedId === targetId) {
      repositories = repositories.map((entry) =>
        entry.id === detail.repository.id ? detail.repository : entry,
      );
    }
    historyVersion += 1;
    refreshFromStreamSafely();
  }

  async function refreshFromStream(): Promise<void> {
    const refresh = streamRefreshes.begin();
    try {
      if (!(await refreshTargets()) || !streamRefreshes.isCurrent(refresh)) return;
      if (selectedId !== null) {
        if (
          !(await loadRepositories(selectedId, () => streamRefreshes.isCurrent(refresh))) ||
          !streamRefreshes.isCurrent(refresh)
        )
          return;
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

  async function signOut(): Promise<void> {
    loading = true;
    failure = null;
    try {
      await api.signOut();
      viewer = null;
      targets = [];
      repositories = [];
      selectedId = null;
      view = 'settings';
      streamReady = false;
      targetReads.invalidate();
      repositoryReads.invalidate();
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
      <ViewTabs value={view} onSelect={(nextView) => (view = nextView)} />

      {#if view === 'settings'}
        <div id="settings-panel" role="tabpanel" aria-labelledby="settings-tab">
          {#key selectedTarget.id}
            <TargetSettings target={selectedTarget} onUpdate={updateTarget} />
          {/key}
        </div>
      {:else if view === 'repositories'}
        <div id="repositories-panel" role="tabpanel" aria-labelledby="repositories-tab">
          {#if repositoriesLoading && repositories.length === 0}
            <Plate label="Installed repositories">
              <p class="dim">Reading repositories…</p>
            </Plate>
          {:else}
            {#key selectedTarget.id}
              <RepositoryList
                {repositories}
                refreshVersion={repositoryDetailsVersion}
                onLoad={loadRepository}
                onUpdate={updateRepository}
                onChanged={(detail) => repositoryChanged(selectedTarget.id, detail)}
              />
            {/key}
          {/if}
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
