<script lang="ts">
  import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import { useInterval } from 'runed';
  import { PanelApiError, type PanelApi } from '../api';
  import { dialogRoute } from '../dialog-route.svelte';
  import { formatTimestamp } from '../format';
  import { monogram } from '../identity';
  import { invalidateRootInstallationSettings } from '../query-client';
  import type { HistorySection, ScopedPanelView } from '../routes';
  import type {
    PanelTarget,
    RepositoryDetail,
    RepositoryPageRequest,
    RepositorySettingsInput,
    RootElevation,
    RootInstallation,
    TargetSettingsInput,
  } from '../types';
  import Icon from './Icon.svelte';
  import HistoryPanel from './HistoryPanel.svelte';
  import Modal from './Modal.svelte';
  import RepositoryList from './RepositoryList.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import TargetSettings from './TargetSettings.svelte';
  import UserManagement from './UserManagement.svelte';

  const {
    installation,
    view,
    actorLogin,
    historySection,
    onHistorySection,
    api,
    listHref,
    hrefFor,
    onList,
    onNavigate,
  }: {
    installation: RootInstallation;
    view: ScopedPanelView;
    api: PanelApi;
    actorLogin: string;
    listHref: string;
    hrefFor: (account: string, view: ScopedPanelView) => string;
    onList: () => void;
    onNavigate: (account: string, view: ScopedPanelView) => void;
    historySection: HistorySection;
    onHistorySection: (section: HistorySection) => void;
  } = $props();

  /** Names the dialog in the address, and is the `id` the dialog carries. */
  const ELEVATION_DIALOG = 'root-elevation';

  const queryClient = useQueryClient();
  const detailKey = $derived(['root-installations', installation.id, 'detail'] as const);
  const detailQuery = createQuery(() => ({
    queryKey: detailKey,
    queryFn: async () => ({
      target: await api.fetchRootTargetSettings(installation.id),
      elevation: await loadElevation(),
    }),
  }));
  const beginElevationMutation = createMutation(() => ({
    mutationFn: (input: { acknowledged: true; reason?: string }) =>
      api.beginRootElevation(installation.id, input),
    onSettled: () => queryClient.invalidateQueries({ queryKey: detailKey }),
  }));
  const endElevationMutation = createMutation(() => ({
    mutationFn: (elevationId: string) => api.endRootElevation(elevationId),
    onSettled: () => queryClient.invalidateQueries({ queryKey: detailKey }),
  }));
  const targetSettingsMutation = createMutation(() => ({
    mutationFn: (input: TargetSettingsInput) =>
      api.updateRootTargetSettings(installation.id, input),
    onSettled: () => invalidateRootInstallationSettings(queryClient, installation.id),
  }));
  const target = $derived<PanelTarget | null>(detailQuery.data?.target ?? null);
  const elevation = $derived<RootElevation | null>(detailQuery.data?.elevation ?? null);
  const loading = $derived(detailQuery.isFetching);
  let detailFailure = $state<string | null>(null);
  const failure = $derived(
    detailFailure ?? (detailQuery.error === null ? null : message(detailQuery.error)),
  );
  let elevationFailure = $state<string | null>(null);
  /* Whatever the address names, so a reload keeps the reader in the dialog they
     were reading rather than dropping them back onto the installation. */
  const elevationModalOpen = $derived(dialogRoute.isOpen(ELEVATION_DIALOG));
  let elevationAcknowledged = $state(false);
  let elevationReason = $state('');
  const elevationPending = $derived(
    beginElevationMutation.isPending || endElevationMutation.isPending,
  );
  let elevationTrigger = $state<HTMLButtonElement | null>(null);
  let now = $state(Date.now());
  const elevationClock = useInterval(1_000, {
    immediate: false,
    callback: () => (now = Date.now()),
  });

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

  async function load(): Promise<void> {
    detailFailure = null;
    await detailQuery.refetch();
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
    dialogRoute.open(ELEVATION_DIALOG);
  }

  function closeElevation(): void {
    if (elevationPending) return;
    if (dialogRoute.isOpen(ELEVATION_DIALOG)) dialogRoute.close();
  }

  async function beginElevation(): Promise<void> {
    if (!elevationAcknowledged || elevationPending) return;
    elevationFailure = null;
    try {
      await beginElevationMutation.mutateAsync({
        acknowledged: true,
        ...(elevationReason.trim() === '' ? {} : { reason: elevationReason.trim() }),
      });
      if (dialogRoute.isOpen(ELEVATION_DIALOG)) dialogRoute.close();
    } catch (error) {
      elevationFailure = message(error);
    }
  }

  async function endElevation(): Promise<void> {
    const current = elevation;
    if (current === null || elevationPending) return;
    elevationFailure = null;
    try {
      await endElevationMutation.mutateAsync(current.id);
    } catch (error) {
      elevationFailure = message(error);
    }
  }

  async function expireElevation(): Promise<void> {
    if (elevation === null) return;
    try {
      await detailQuery.refetch();
    } catch (error) {
      detailFailure = message(error);
    }
  }

  async function updateTarget(input: TargetSettingsInput): Promise<void> {
    await targetSettingsMutation.mutateAsync(input);
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

  function resetConfigMigration(targetId: string, repositoryId: string): Promise<RepositoryDetail> {
    return api.resetRootConfigMigration(targetId, repositoryId);
  }

  function repositoryChanged(targetId: string): void {
    void queryClient.invalidateQueries({ queryKey: ['repositories', targetId] });
    void queryClient.invalidateQueries({ queryKey: ['root-installations'] });
    void queryClient.invalidateQueries({ queryKey: ['root-overview'] });
  }

  function countdown(seconds: number): string {
    const minutes = Math.floor(seconds / 60);
    return `${String(minutes).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`;
  }

  function message(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }

  $effect(() => {
    const active = elevation !== null;
    untrack(() => {
      if (!active) {
        elevationClock.pause();
        return;
      }
      now = Date.now();
      elevationClock.resume();
    });
  });

  $effect(() => {
    if (elevation !== null && remainingSeconds === 0) void expireElevation();
  });
</script>

<section class="installation-view" aria-labelledby="root-page-heading">
  <header class="installation-heading">
    <div class="installation-title">
      <a class="back-link" href={listHref} onclick={returnToList}>
        <Icon name="chevron-left" size={14} />
        <span class="cap-trim">Installations</span>
      </a>
      <div>
        <span class="installation-mark">
          <span class="cap-trim">
            {monogram(installation.account.display_name, installation.account.login)}
          </span>
        </span>
        <span>
          <h2 id="root-page-heading">{installation.account.display_name}</h2>
          <p>@{installation.account.login} · GitHub installation #{installation.installation_id}</p>
        </span>
      </div>
    </div>

    <div class="access-summary">
      {#if target !== null && ownsInstallation}
        <span class="status-pill"
          ><Icon name="shield" size={14} /><span class="cap-trim">Owner access</span></span
        >
      {:else if elevation !== null}
        <span class="status-pill"
          ><Icon name="warning" size={14} /><span class="cap-trim">Elevated</span></span
        >
      {:else}
        <button
          class="btn root-access-button"
          type="button"
          bind:this={elevationTrigger}
          disabled={!canElevate || loading}
          onclick={openElevation}
        >
          <Icon name="lock" size={16} />
          Request write access
        </button>
      {/if}
    </div>
  </header>

  {#if elevation !== null}
    <aside class="elevation-banner">
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

  {#if !ownsInstallation && elevation === null && !canElevate}
    <p class="access-hint">Fresh Owners are required before elevated access can start</p>
  {/if}

  <!-- A refresh that failed over a loaded view has not made the view wrong, so
       the failure is a line above it and the panel stays where it is. -->
  {#if failure !== null && target !== null}
    <ResultProblem
      title="Could not refresh this installation"
      problem={failure}
      busy={loading}
      onRetry={() => void load()}
      overContent
    />
  {/if}

  <!-- Only while there is nothing to read yet. A refresh over a loaded view
       leaves it standing, or the whole panel blinks out on every event. -->
  {#if loading && target === null && failure === null}
    <div class="root-loading" role="status">Reading installation diagnostics…</div>
  {:else if failure !== null && target === null}
    <div class="root-loading problem" role="alert">
      <strong>Could not load this installation</strong>
      <p>{failure}</p>
      <button class="btn" type="button" onclick={() => void load()} disabled={loading}>
        {loading ? 'Trying again…' : 'Try again'}
      </button>
    </div>
  {:else if target !== null && view === 'settings'}
    <TargetSettings {target} readOnly={!canWrite} onUpdate={updateTarget} />
  {:else if target !== null && view === 'repositories'}
    <RepositoryList
      targetId={installation.id}
      defaultEnabled={target.repository_default_enabled}
      fetchPage={fetchRepositories}
      onLoad={loadRepository}
      onUpdate={updateRepository}
      onResetConfigMigration={resetConfigMigration}
      onChanged={repositoryChanged}
      readOnly={!canWrite}
    />
  {:else if target !== null && (view === 'users' || view === 'invitations')}
    <UserManagement
      section={view}
      targetId={installation.id}
      targetName={installation.account.display_name}
      {actorLogin}
      actorTargetRole={canWrite ? 'owner' : 'none'}
      readOnly={!canWrite}
      onSection={selectAccessSection}
      fetchTargetUsers={api.fetchRootTargetUsers}
      addTargetUser={api.addRootTargetUser}
      suggestUsers={api.suggestRootTargetUsers}
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
      section={historySection}
      onSection={onHistorySection}
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
  id={ELEVATION_DIALOG}
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
    gap: 0;
    min-height: 0;
  }

  .installation-heading,
  .access-summary,
  .installation-title > div,
  .elevation-banner {
    align-items: center;
    display: flex;
  }

  /* Embedded child views ship their own page headers; the pill navigation
     already names the section, so content starts immediately. */
  .installation-view :global(.panel-header) {
    display: none;
  }

  .installation-heading {
    gap: var(--space-6);
    justify-content: space-between;
    padding: var(--space-2) 0 var(--space-4);
  }

  .installation-title {
    display: grid;
    gap: var(--space-2);
  }

  .installation-title > div {
    gap: var(--space-3);
  }

  /* The back link sits where a kicker sits, so it dresses like one. */
  .back-link {
    align-items: center;
    color: var(--brand-action-text);
    display: inline-flex;
    font: 700 var(--font-size-micro) / 1 var(--sans);
    gap: var(--space-1);
    letter-spacing: 0.08em;
    text-decoration: none;
    text-transform: uppercase;
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
    background: var(--brand-action-tint);
    border-radius: var(--radius-control);
    /* Self-keyed keyline: mixed from the mark's own foreground, so the tint
       fill stays visible on any surface. */
    box-shadow: inset 0 0 0 1px color-mix(in srgb, currentcolor 28%, transparent);
    color: var(--brand-action-text);
    font-size: 0.875rem;
    font-weight: 700;
    height: 2.75rem;
    width: 2.75rem;
  }

  h2,
  p {
    margin: 0;
  }

  h2 {
    font-size: 1.375rem;
    font-weight: 700;
    letter-spacing: -0.035em;
    /* Whole pixels, same rule as the page headers: 1.2 of 22px is 26.4, and the
       fraction lands the avatar row and the nav under it off the device grid. */
    line-height: round(1.2em, 1px);
    margin: 0;
  }

  .installation-title p {
    color: var(--text-secondary);
    font: 450 var(--font-size-compact) / 1.4 var(--mono);
    margin-top: var(--space-1);
  }

  .elevation-banner p,
  .root-loading p {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    margin-top: var(--space-1);
  }

  .root-access-button,
  .root-confirm {
    background: color-mix(in srgb, var(--brand-action) 12%, var(--surface-base));
    border-color: color-mix(in srgb, var(--brand-action) 45%, var(--control-border));
    color: var(--brand-action-text);
  }

  .access-hint {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
  }

  .elevation-banner {
    background: color-mix(in srgb, var(--brand-action) 8%, var(--surface-base));
    border: 1px solid color-mix(in srgb, var(--brand-action) 38%, var(--border-subtle));
    border-inline-start: 0.3rem solid var(--brand-action);
    border-radius: var(--radius-control);
    gap: var(--space-3);
    padding: var(--space-3);
  }

  .elevation-banner > div {
    flex: 1;
    min-width: 0;
  }

  .elevation-icon {
    color: var(--brand-action-text);
  }

  .elevation-countdown {
    color: var(--text-primary);
    font: 700 0.9rem/1 var(--mono);
  }

  .installation-navigation {
    /* The island hugs its tabs (mock .pill-nav is inline-flex) — align-self stops
       a column parent's default stretch from widening it to the full row. */
    align-self: flex-start;
    background: color-mix(in srgb, var(--brand-action) 4%, var(--surface-inset));
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    display: inline-flex;
    gap: var(--space-1);
    /* The child views' own page headers are hidden here, so the tabs owe the
       first card the gap a header would have carried - the same 1rem the plates
       keep between each other. */
    margin-bottom: var(--space-4);
    max-width: 100%;
    overflow-x: auto;
    padding: var(--space-1);
    width: fit-content;
  }

  .installation-navigation a {
    border: 1px solid transparent;
    border-radius: calc(var(--radius-control) - 2px);
    color: var(--text-secondary);
    font-size: var(--font-size-control);
    font-weight: 650;
    line-height: 1;
    padding: 0.4375rem var(--space-3);
    text-box: trim-both cap alphabetic;
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
    border-color: color-mix(in srgb, var(--brand-action) 30%, var(--border-subtle));
    box-shadow: 0 1px 2px var(--shadow);
    color: var(--brand-action-text);
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
    background: color-mix(in srgb, var(--brand-action) 8%, var(--surface-inset));
    border: 1px solid color-mix(in srgb, var(--brand-action) 28%, var(--border-subtle));
    border-radius: var(--radius-control);
    display: grid;
    gap: var(--space-3);
    grid-template-columns: auto minmax(0, 1fr);
    padding: var(--space-4);
  }

  .elevation-warning > span {
    color: var(--brand-action-text);
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
</style>
