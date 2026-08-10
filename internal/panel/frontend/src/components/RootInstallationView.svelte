<script lang="ts">
  import { PanelApiError, type PanelApi } from '../lib/api';
  import { formatTimestamp } from '../lib/format';
  import type { ScopedPanelView } from '../lib/routes';
  import type {
    PanelTarget,
    RepositoryDetail,
    RepositoryPageRequest,
    RepositorySettingsInput,
    RootElevation,
    RootInstallation,
    TargetSettingsInput,
  } from '../lib/types';
  import Icon from './Icon.svelte';
  import HistoryPanel from './HistoryPanel.svelte';
  import Modal from './Modal.svelte';
  import RepositoryList from './RepositoryList.svelte';
  import TargetSettings from './TargetSettings.svelte';
  import UserManagement from './UserManagement.svelte';

  const {
    installation,
    view,
    api,
    refreshVersion,
    listHref,
    hrefFor,
    onList,
    onNavigate,
  }: {
    installation: RootInstallation;
    view: ScopedPanelView;
    api: PanelApi;
    refreshVersion: number;
    listHref: string;
    hrefFor: (account: string, view: ScopedPanelView) => string;
    onList: () => void;
    onNavigate: (account: string, view: ScopedPanelView) => void;
  } = $props();

  let target = $state<PanelTarget | null>(null);
  let elevation = $state<RootElevation | null>(null);
  let loading = $state(true);
  let failure = $state<string | null>(null);
  let elevationFailure = $state<string | null>(null);
  let elevationModalOpen = $state(false);
  let elevationAcknowledged = $state(false);
  let elevationReason = $state('');
  let elevationPending = $state(false);
  let elevationTrigger = $state<HTMLButtonElement | null>(null);
  let now = $state(Date.now());
  let repositoryVersion = $state(0);
  let loadSequence = 0;

  const remainingSeconds = $derived(
    elevation === null
      ? 0
      : Math.max(0, Math.ceil((Date.parse(elevation.expires_at) - now) / 1000)),
  );
  const canElevate = $derived(
    installation.available &&
      installation.ownership.status === 'fresh' &&
      !installation.ownership.stale &&
      installation.ownership.owner_count > 0,
  );
  const ownsInstallation = $derived(target?.access_source === 'owner');
  const canWrite = $derived(target?.capabilities.write === true);

  async function load(version = refreshVersion): Promise<void> {
    const current = ++loadSequence;
    loading = true;
    failure = null;
    try {
      const currentTarget = await api.fetchRootTargetSettings(installation.id);
      const currentElevation = await loadElevation();
      if (current !== loadSequence || version !== refreshVersion) return;
      target = currentTarget;
      elevation = currentElevation;
      repositoryVersion += 1;
    } catch (error) {
      if (current !== loadSequence || version !== refreshVersion) return;
      failure = message(error);
    } finally {
      if (current === loadSequence) loading = false;
    }
  }

  async function loadElevation(): Promise<RootElevation | null> {
    try {
      return await api.fetchRootElevation(installation.id);
    } catch (error) {
      if (error instanceof PanelApiError && [404, 409, 410].includes(error.status)) return null;
      throw error;
    }
  }

  function navigate(event: MouseEvent, next: ScopedPanelView): void {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey) return;
    event.preventDefault();
    onNavigate(installation.account.login, next);
  }

  function selectAccessSection(section: 'users' | 'invitations'): void {
    onNavigate(installation.account.login, section);
  }

  function returnToList(event: MouseEvent): void {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey) return;
    event.preventDefault();
    onList();
  }

  function openElevation(): void {
    elevationAcknowledged = false;
    elevationReason = '';
    elevationFailure = null;
    elevationModalOpen = true;
  }

  function closeElevation(): void {
    if (elevationPending) return;
    elevationModalOpen = false;
  }

  async function beginElevation(): Promise<void> {
    if (!elevationAcknowledged || elevationPending) return;
    elevationPending = true;
    elevationFailure = null;
    try {
      elevation = await api.beginRootElevation(installation.id, {
        acknowledged: true,
        ...(elevationReason.trim() === '' ? {} : { reason: elevationReason.trim() }),
      });
      target = await api.fetchRootTargetSettings(installation.id);
      elevationModalOpen = false;
    } catch (error) {
      elevationFailure = message(error);
    } finally {
      elevationPending = false;
    }
  }

  async function endElevation(): Promise<void> {
    const current = elevation;
    if (current === null || elevationPending) return;
    elevationPending = true;
    elevationFailure = null;
    try {
      await api.endRootElevation(current.id);
      elevation = null;
      target = await api.fetchRootTargetSettings(installation.id);
      repositoryVersion += 1;
    } catch (error) {
      elevationFailure = message(error);
    } finally {
      elevationPending = false;
    }
  }

  async function expireElevation(): Promise<void> {
    if (elevation === null) return;
    elevation = null;
    try {
      target = await api.fetchRootTargetSettings(installation.id);
      repositoryVersion += 1;
    } catch (error) {
      failure = message(error);
    }
  }

  async function updateTarget(input: TargetSettingsInput): Promise<void> {
    target = await api.updateRootTargetSettings(installation.id, input);
    repositoryVersion += 1;
  }

  function fetchRepositories(request: RepositoryPageRequest) {
    return api.fetchRootRepositories(installation.id, request);
  }

  function loadRepository(repositoryId: string): Promise<RepositoryDetail> {
    return api.fetchRootRepository(installation.id, repositoryId);
  }

  function updateRepository(
    repositoryId: string,
    input: RepositorySettingsInput,
  ): Promise<RepositoryDetail> {
    return api.updateRootRepositorySettings(installation.id, repositoryId, input);
  }

  function countdown(seconds: number): string {
    const minutes = Math.floor(seconds / 60);
    return `${String(minutes).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`;
  }

  function message(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }

  $effect(() => {
    void load(refreshVersion);
  });

  $effect(() => {
    if (elevation === null) return;
    now = Date.now();
    const timer = window.setInterval(() => (now = Date.now()), 1_000);
    return () => window.clearInterval(timer);
  });

  $effect(() => {
    if (elevation !== null && remainingSeconds === 0) void expireElevation();
  });
</script>

<section class="installation-view" aria-labelledby="root-installation-heading">
  <header class="installation-heading">
    <div class="installation-title">
      <a class="back-link" href={listHref} onclick={returnToList}>
        <Icon name="chevron-left" size={16} />
        All installations
      </a>
      <div>
        <span class="installation-mark">
          <Icon name={installation.type === 'Organization' ? 'organization' : 'user'} size={20} />
        </span>
        <span>
          <h3 id="root-installation-heading">{installation.account.display_name}</h3>
          <p>@{installation.account.login} · GitHub installation #{installation.installation_id}</p>
        </span>
      </div>
    </div>

    <div class="access-summary">
      {#if target !== null && ownsInstallation}
        <span class="access-state owner"><Icon name="owner" size={16} /> Owner access</span>
      {:else if elevation !== null}
        <span class="access-state elevated"><Icon name="warning" size={16} /> Elevated</span>
      {:else}
        <button
          class="btn root-access-button"
          type="button"
          bind:this={elevationTrigger}
          disabled={!canElevate || loading}
          title={canElevate
            ? undefined
            : 'Fresh Owners are required before elevated access can start'}
          onclick={openElevation}
        >
          <Icon name="lock" size={16} />
          Request write access
        </button>
      {/if}
    </div>
  </header>

  {#if elevation !== null}
    <aside class="elevation-banner" aria-live="polite">
      <span class="elevation-icon"><Icon name="warning" size={19} /></span>
      <div>
        <strong>Elevated access to {installation.account.display_name}</strong>
        <p>
          Every write is audited and notifies Owners
          {#if elevation.reason !== undefined}
            · {elevation.reason}{/if}
        </p>
      </div>
      <span class="elevation-countdown" title={`Ends ${formatTimestamp(elevation.expires_at)}`}>
        {countdown(remainingSeconds)}
      </span>
      <button class="btn btn-stop" type="button" disabled={elevationPending} onclick={endElevation}>
        End access
      </button>
    </aside>
  {/if}

  {#if elevationFailure !== null}
    <p class="form-error" role="alert">{elevationFailure}</p>
  {/if}

  <nav
    class="installation-navigation"
    aria-label={`Root views for ${installation.account.display_name}`}
  >
    {#each ['settings', 'repositories', 'users', 'history'] as section (section)}
      {@const item = section as ScopedPanelView}
      <a
        class:active={view === item || (item === 'users' && view === 'invitations')}
        href={hrefFor(installation.account.login, item)}
        aria-current={view === item || (item === 'users' && view === 'invitations')
          ? 'page'
          : undefined}
        onclick={(event) => navigate(event, item)}
      >
        {item === 'users' ? 'Access' : item[0]?.toLocaleUpperCase() + item.slice(1)}
      </a>
    {/each}
  </nav>

  {#if loading}
    <div class="root-loading" role="status">Reading installation diagnostics…</div>
  {:else if failure !== null}
    <div class="root-loading problem" role="alert">
      <strong>Could not load this installation</strong>
      <p>{failure}</p>
      <button class="btn" type="button" onclick={() => void load(refreshVersion)}>Try again</button>
    </div>
  {:else if target !== null && view === 'settings'}
    <TargetSettings {target} readOnly={!canWrite} onUpdate={updateTarget} />
  {:else if target !== null && view === 'repositories'}
    <RepositoryList
      targetId={installation.id}
      refreshVersion={repositoryVersion}
      fetchPage={fetchRepositories}
      onLoad={loadRepository}
      onUpdate={updateRepository}
      onChanged={() => (repositoryVersion += 1)}
      readOnly={!canWrite}
    />
  {:else if target !== null && (view === 'users' || view === 'invitations')}
    <UserManagement
      section={view}
      targetId={installation.id}
      targetName={installation.account.display_name}
      actorTargetRole={canWrite ? 'owner' : 'none'}
      refreshVersion={repositoryVersion}
      readOnly={!canWrite}
      onSection={selectAccessSection}
      fetchTargetUsers={api.fetchRootTargetUsers}
      addTargetUser={api.addRootTargetUser}
      updateTargetUser={api.updateRootTargetUser}
      fetchTargetInvitations={api.fetchRootTargetInvitations}
      createTargetInvitation={api.createRootTargetInvitation}
      reissueInvitation={api.reissueRootTargetInvitation}
      revokeInvitation={api.revokeRootTargetInvitation}
      fetchUserDecisions={api.fetchRootTargetUserDecisions}
    />
  {:else if target !== null && view === 'history'}
    <HistoryPanel
      targetId={installation.id}
      refreshVersion={repositoryVersion}
      fetchAudit={(request) => api.fetchRootTargetAudit(installation.id, request)}
      fetchFailures={(request) => api.fetchRootTargetFailures(installation.id, request)}
    />
  {:else}
    <div class="root-loading">
      <strong>This installation view is unavailable</strong>
      <p>Return to the installation catalog and choose a supported destination.</p>
    </div>
  {/if}
</section>

<Modal
  id="root-elevation"
  open={elevationModalOpen}
  title={`Elevate access to ${installation.account.display_name}`}
  description="This grants write access for 15 minutes. It cannot be extended by activity."
  returnFocus={elevationTrigger}
  onClose={closeElevation}
>
  <div class="elevation-warning">
    <span><Icon name="warning" size={22} /></span>
    <p>
      You do not own this installation. Every change is permanently audited and every identified
      Owner receives an in-app security notification.
    </p>
  </div>

  <label class="acknowledgment">
    <input type="checkbox" bind:checked={elevationAcknowledged} data-modal-focus />
    <span>
      I understand the consequences and want to enter audited elevated access for this installation.
    </span>
  </label>

  <label class="reason-field">
    <span>Reason <small>Optional, included in the audit trail and Owner notifications</small></span>
    <textarea
      class="text-input"
      rows="3"
      maxlength="500"
      placeholder="For example: investigating failed repository deliveries"
      bind:value={elevationReason}></textarea>
  </label>

  {#if elevationFailure !== null}
    <p class="form-error" role="alert">{elevationFailure}</p>
  {/if}

  {#snippet footer()}
    <button class="btn" type="button" disabled={elevationPending} onclick={closeElevation}
      >Cancel</button
    >
    <button
      class="btn root-confirm"
      type="button"
      disabled={!elevationAcknowledged || elevationPending}
      onclick={beginElevation}
    >
      {elevationPending ? 'Starting access…' : 'Start 15-minute access'}
    </button>
  {/snippet}
</Modal>

<style>
  .installation-view {
    display: grid;
    gap: var(--space-4);
    min-height: 0;
  }

  .installation-heading,
  .installation-heading > div,
  .installation-title > div,
  .elevation-banner,
  .access-state {
    align-items: center;
    display: flex;
  }

  .installation-heading {
    gap: var(--space-5);
    justify-content: space-between;
  }

  .installation-title {
    display: grid;
    gap: var(--space-3);
  }

  .installation-title > div {
    gap: var(--space-3);
  }

  .back-link {
    align-items: center;
    color: var(--text-secondary);
    display: inline-flex;
    font-size: var(--font-size-compact);
    gap: var(--space-1);
    text-decoration: none;
    width: fit-content;
  }

  .back-link:hover {
    color: var(--text-primary);
  }

  .installation-mark,
  .elevation-icon {
    align-items: center;
    display: inline-flex;
    justify-content: center;
  }

  .installation-mark {
    background: color-mix(in srgb, #8b5cf6 12%, var(--surface-inset));
    border: 1px solid color-mix(in srgb, #8b5cf6 25%, var(--border-subtle));
    border-radius: var(--radius-control);
    color: #7357bd;
    height: 2.75rem;
    width: 2.75rem;
  }

  h3,
  p {
    margin: 0;
  }

  h3 {
    font-size: 1.25rem;
    letter-spacing: -0.025em;
  }

  .installation-title p,
  .elevation-banner p,
  .root-loading p {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    margin-top: var(--space-1);
  }

  .access-state {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    font-size: var(--font-size-compact);
    font-weight: 700;
    gap: var(--space-2);
    height: var(--control-height);
    padding: 0 var(--space-3);
  }

  .access-state.owner {
    background: var(--accent-tint);
    color: var(--accent);
  }

  .access-state.elevated,
  .root-access-button,
  .root-confirm {
    background: color-mix(in srgb, #8b5cf6 12%, var(--surface-base));
    border-color: color-mix(in srgb, #8b5cf6 45%, var(--control-border));
    color: color-mix(in srgb, #6d54bd 88%, var(--text-primary));
  }

  .elevation-banner {
    background: color-mix(in srgb, #8b5cf6 8%, var(--surface-base));
    border: 1px solid color-mix(in srgb, #8b5cf6 38%, var(--border-subtle));
    border-inline-start: 0.3rem solid #8b5cf6;
    border-radius: var(--radius-control);
    gap: var(--space-3);
    padding: var(--space-3);
  }

  .elevation-banner > div {
    flex: 1;
    min-width: 0;
  }

  .elevation-icon {
    color: #7357bd;
  }

  .elevation-countdown {
    color: var(--text-primary);
    font: 700 0.9rem/1 var(--mono);
  }

  .installation-navigation {
    background: color-mix(in srgb, #8b5cf6 4%, var(--surface-inset));
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    display: flex;
    gap: var(--space-1);
    overflow-x: auto;
    padding: var(--space-1);
  }

  .installation-navigation a {
    border: 1px solid transparent;
    border-radius: calc(var(--radius-control) - 0.15rem);
    color: var(--text-secondary);
    font-size: var(--font-size-control);
    font-weight: 650;
    padding: var(--space-2) var(--space-3);
    text-decoration: none;
    white-space: nowrap;
  }

  .installation-navigation a:hover {
    background: var(--interactive-hover);
    color: var(--text-primary);
  }

  .installation-navigation a:active {
    background: var(--interactive-pressed-bg);
    transform: scale(0.98);
  }

  .installation-navigation a.active {
    background: var(--surface-base);
    border-color: color-mix(in srgb, #8b5cf6 30%, var(--border-subtle));
    box-shadow: 0 1px 2px var(--shadow);
    color: #6d54bd;
  }

  .root-loading {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    min-height: 10rem;
    padding: var(--space-6);
  }

  .root-loading.problem {
    border-color: color-mix(in srgb, var(--stop) 30%, var(--border-subtle));
  }

  .root-loading .btn {
    margin-top: var(--space-4);
  }

  .elevation-warning {
    align-items: start;
    background: color-mix(in srgb, #8b5cf6 8%, var(--surface-inset));
    border: 1px solid color-mix(in srgb, #8b5cf6 28%, var(--border-subtle));
    border-radius: var(--radius-control);
    display: grid;
    gap: var(--space-3);
    grid-template-columns: auto minmax(0, 1fr);
    padding: var(--space-4);
  }

  .elevation-warning > span {
    color: #7357bd;
  }

  .elevation-warning p {
    line-height: 1.55;
  }

  .acknowledgment,
  .reason-field {
    display: grid;
    margin-top: var(--space-4);
  }

  .acknowledgment {
    align-items: start;
    cursor: pointer;
    gap: var(--space-3);
    grid-template-columns: auto minmax(0, 1fr);
    line-height: 1.5;
  }

  .acknowledgment input {
    height: 1.1rem;
    margin: 0.2rem 0 0;
    width: 1.1rem;
  }

  .reason-field {
    gap: var(--space-2);
  }

  .reason-field > span {
    font-size: var(--font-size-control);
    font-weight: 650;
  }

  .reason-field small {
    color: var(--text-secondary);
    display: block;
    font-weight: 450;
    margin-top: var(--space-1);
  }

  .reason-field textarea {
    height: auto;
    line-height: 1.5;
    padding-block: var(--space-2);
    resize: vertical;
  }

  @media (max-width: 46rem) {
    .installation-heading,
    .elevation-banner {
      align-items: stretch;
      flex-direction: column;
    }

    .access-summary,
    .access-summary > * {
      width: 100%;
    }

    .elevation-countdown {
      font-size: 1.05rem;
    }
  }

  :global(:root[data-theme='dark']) .installation-mark,
  :global(:root[data-theme='dark']) .elevation-icon,
  :global(:root[data-theme='dark']) .installation-navigation a.active,
  :global(:root[data-theme='dark']) .access-state.elevated,
  :global(:root[data-theme='dark']) .root-access-button,
  :global(:root[data-theme='dark']) .root-confirm {
    color: #c4b5fd;
  }
</style>
