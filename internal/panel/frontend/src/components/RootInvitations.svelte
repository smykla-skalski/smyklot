<script lang="ts">
  import type { Snippet } from 'svelte';
  import { PanelApiError } from '../lib/api';
  import { formatDateTime, formatRelative, formatTimestamp, formatUntil } from '../lib/format';
  import type { FilterSection } from '../lib/filter-menu';
  import type {
    AddRootInvitationInput,
    InvitationDays,
    InvitationPageRequest,
    InvitationSort,
    InvitationStatus,
    Page,
    PanelInvitation,
  } from '../lib/types';
  import ActionMenu, { type ActionMenuItem } from './ActionMenu.svelte';
  import Avatar from './Avatar.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import Icon from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import Modal from './Modal.svelte';
  import SearchField from './SearchField.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  type SortColumn = 'name' | 'created' | 'expiry';
  type InvitationAction = 'reissue' | 'revoke';

  const STATUS_FILTERS = [
    {
      options: [
        { value: 'pending', label: 'Pending', tone: 'default' },
        { value: 'accepted', label: 'Accepted', tone: 'valid' },
        { value: 'declined', label: 'Declined', tone: 'default' },
        { value: 'revoked', label: 'Revoked', tone: 'invalid' },
        { value: 'expired', label: 'Expired', tone: 'bypassed' },
      ],
    },
  ] satisfies readonly FilterSection[];

  const {
    fetchPage,
    refreshVersion,
    create,
    reissue,
    revoke,
    canManage,
    actorLogin,
    navigation,
  }: {
    fetchPage: (request: InvitationPageRequest) => Promise<Page<PanelInvitation>>;
    refreshVersion: number;
    create: (input: AddRootInvitationInput) => Promise<PanelInvitation>;
    reissue: (invitationId: string, expiresInDays: InvitationDays) => Promise<PanelInvitation>;
    revoke: (invitationId: string) => Promise<PanelInvitation>;
    canManage: boolean;
    /** The signed-in login, so naming yourself is answered before the press. */
    actorLogin: string;
    navigation?: Snippet;
  } = $props();

  let page = $state<Page<PanelInvitation> | null>(null);
  // Ticks so the pending countdown and relative Created column keep aging.
  let now = $state(Date.now());
  let search = $state('');
  let query = $state('');
  let sort = $state<InvitationSort>('created_newest');
  let statuses = $state<InvitationStatus[]>([]);
  let loading = $state(false);
  let problem = $state<string | null>(null);
  let loadMoreProblem = $state<string | null>(null);
  let sequence = 0;

  let createOpen = $state(false);
  let createTrigger = $state<HTMLElement | null>(null);
  let login = $state('');
  let expiresInDays = $state<InvitationDays>(7);
  let creating = $state(false);
  let createProblem = $state<string | null>(null);
  let copyProblem = $state<string | null>(null);
  let generatedLink = $state('');
  let generatedFor = $state('');

  /**
   * The login the server said had declined, which turns the dialog into a question.
   *
   * Declining is an answer, so asking again is a decision made on purpose rather than by pressing
   * the same button twice. The gate itself is the server's: it knows the whole history.
   */
  let declinedLogin = $state<string | null>(null);
  const createStage = $derived(
    generatedLink !== '' ? 'link' : declinedLogin !== null ? 'confirm' : 'form',
  );

  /** The one standing the panel knows for certain, and can therefore answer before the press. */
  const namingSelf = $derived(
    login.trim() !== '' && login.trim().toLowerCase() === actorLogin.trim().toLowerCase(),
  );

  let actionInvitation = $state<PanelInvitation | null>(null);
  let pendingAction = $state<InvitationAction | null>(null);
  let actionTrigger = $state<HTMLElement | null>(null);
  let actionBusy = $state(false);
  let actionProblem = $state<string | null>(null);

  const limit = 20;
  const requestKey = $derived(JSON.stringify([query, sort, statuses, limit, refreshVersion]));
  const invitations = $derived(page?.items ?? []);
  const hasFilters = $derived(query !== '' || statuses.length > 0);

  $effect(() => {
    const tick = setInterval(() => {
      now = Date.now();
    }, 30_000);
    return () => clearInterval(tick);
  });

  $effect(() => {
    const next = search.trim();
    const timeout = window.setTimeout(() => (query = next), 180);
    return () => window.clearTimeout(timeout);
  });

  $effect(() => {
    void loadPage(undefined, false, requestKey);
  });

  async function loadPage(
    cursor: string | undefined,
    append: boolean,
    key = requestKey,
  ): Promise<void> {
    if (key !== requestKey || loading) return;
    const version = ++sequence;
    loading = true;
    if (append) loadMoreProblem = null;
    else problem = null;
    try {
      const loaded = await fetchPage({
        ...(cursor === undefined ? {} : { cursor }),
        query,
        sort,
        limit,
        roles: [],
        statuses,
      });
      if (version !== sequence || key !== requestKey) return;
      page =
        append && page !== null ? { ...loaded, items: [...page.items, ...loaded.items] } : loaded;
    } catch (error) {
      if (version !== sequence || key !== requestKey) return;
      const message = error instanceof Error ? error.message : String(error);
      if (append) loadMoreProblem = message;
      else problem = message;
    } finally {
      if (version === sequence) loading = false;
    }
  }

  function loadNext(): void {
    const cursor = page?.next_cursor;
    if (cursor !== null && cursor !== undefined) void loadPage(cursor, true);
  }

  function loadFromScroll(event: Event): void {
    const target = event.currentTarget as HTMLElement;
    if (target.scrollHeight - target.scrollTop - target.clientHeight < 260) loadNext();
  }

  function clearFilters(): void {
    search = '';
    query = '';
    statuses = [];
  }

  function toggleSort(column: SortColumn): void {
    const pairs: Record<SortColumn, readonly [InvitationSort, InvitationSort]> = {
      name: ['name_asc', 'name_desc'],
      created: ['created_oldest', 'created_newest'],
      expiry: ['expiry_soonest', 'expiry_latest'],
    };
    const [ascending, descending] = pairs[column];
    sort = sort === ascending ? descending : ascending;
  }

  function sortDirection(column: SortColumn): 'ascending' | 'descending' | undefined {
    if (column === 'name' && sort.startsWith('name_')) {
      return sort === 'name_asc' ? 'ascending' : 'descending';
    }
    if (column === 'created' && sort.startsWith('created_')) {
      return sort === 'created_oldest' ? 'ascending' : 'descending';
    }
    if (column === 'expiry' && sort.startsWith('expiry_')) {
      return sort === 'expiry_soonest' ? 'ascending' : 'descending';
    }
    return undefined;
  }

  function selectStatuses(values: string[]): void {
    statuses = values.filter((value): value is InvitationStatus =>
      ['pending', 'accepted', 'declined', 'revoked', 'expired'].includes(value),
    );
  }

  function statusLabel(status: InvitationStatus): string {
    return status.charAt(0).toLocaleUpperCase() + status.slice(1);
  }

  function statusTone(status: InvitationStatus): ChipTone {
    if (status === 'accepted') return 'clear';
    if (status === 'pending') return 'signal';
    if (status === 'revoked') return 'stop';
    if (status === 'expired') return 'warning';
    return 'neutral';
  }

  /* The trigger lives in the page header, which RootAccess owns, so the button
     hands its own element in for focus return. */
  export function openCreate(trigger: HTMLElement | null): void {
    createTrigger = trigger;
    login = '';
    expiresInDays = 7;
    generatedLink = '';
    generatedFor = '';
    createProblem = null;
    copyProblem = null;
    declinedLogin = null;
    createOpen = true;
  }

  function closeCreate(): void {
    if (creating) return;
    createOpen = false;
    // Cleared at both ends. Every opener establishes this state too, but leaving it behind here
    // means a third entry point inherits a message about a link that is no longer on screen.
    createProblem = null;
    copyProblem = null;
    declinedLogin = null;
  }

  async function submitCreate(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    await sendInvitation(false);
  }

  async function sendInvitation(acknowledged: boolean): Promise<void> {
    if (creating || login.trim() === '' || namingSelf) return;
    creating = true;
    createProblem = null;
    try {
      const invitation = await create({
        login: login.trim(),
        expires_in_days: expiresInDays,
        ...(acknowledged ? { acknowledge_declined: true } : {}),
      });
      declinedLogin = null;
      generatedLink = invitation.invite_url ?? '';
      generatedFor = invitation.account.login;
      page = null;
      await loadPage(undefined, false);
    } catch (error) {
      if (error instanceof PanelApiError && error.code === 'invitation_declined') {
        declinedLogin = login.trim();
      } else {
        createProblem = error instanceof Error ? error.message : String(error);
      }
    } finally {
      creating = false;
    }
  }

  async function copyLink(): Promise<void> {
    if (generatedLink === '') return;
    try {
      await navigator.clipboard.writeText(generatedLink);
      copyProblem = null;
    } catch {
      // A clipboard that refuses used to reject into nothing at all, so the press looked like it
      // had worked and the link was not where you went looking for it. The message says what to do
      // instead, next to the field it is about.
      copyProblem = 'Copy it from the field above, the clipboard was not available';
    }
  }

  function actionItems(invitation: PanelInvitation): ActionMenuItem[] {
    if (!canManage || !['pending', 'expired'].includes(invitation.status)) return [];
    return [
      {
        id: 'reissue',
        icon: 'refresh',
        label: 'Reissue invitation',
        description: 'Create a new single-use link',
      },
      {
        id: 'revoke',
        icon: 'ban',
        label: 'Revoke invitation',
        description: 'Invalidate this invitation',
        tone: 'danger',
      },
    ];
  }

  function chooseAction(
    invitation: PanelInvitation,
    action: string,
    trigger: HTMLElement | null,
  ): void {
    if (action !== 'reissue' && action !== 'revoke') return;
    actionInvitation = invitation;
    pendingAction = action;
    actionTrigger = trigger;
    actionProblem = null;
  }

  function closeAction(): void {
    if (actionBusy) return;
    actionInvitation = null;
    pendingAction = null;
    actionProblem = null;
  }

  async function confirmAction(): Promise<void> {
    if (actionInvitation === null || pendingAction === null || actionBusy) return;
    actionBusy = true;
    actionProblem = null;
    try {
      if (pendingAction === 'reissue') {
        const updated = await reissue(actionInvitation.id, 7);
        generatedLink = updated.invite_url ?? '';
        generatedFor = updated.account.login;
        login = updated.account.login;
        // This door bypasses openCreate, so it clears the same fields itself.
        createProblem = null;
        copyProblem = null;
        declinedLogin = null;
        createOpen = true;
      } else {
        await revoke(actionInvitation.id);
      }
      actionInvitation = null;
      pendingAction = null;
      page = null;
      await loadPage(undefined, false);
    } catch (error) {
      actionProblem = error instanceof Error ? error.message : String(error);
    } finally {
      actionBusy = false;
    }
  }
</script>

<section class="root-invitations" aria-label="Root invitations">
  <div class="invitation-tools">
    {@render navigation?.()}
    <SearchField
      label="Search Root invitations"
      placeholder="Search invitations"
      value={search}
      onInput={(value) => (search = value)}
    />
  </div>

  <div class:loading class="invitation-results" aria-busy={loading}>
    {#if problem !== null}
      <div class="result-state" role="alert">
        <strong>Root invitations could not be loaded</strong>
        <span>{problem}</span>
        <button class="btn" type="button" onclick={() => void loadPage(undefined, false)}>
          Try again
        </button>
      </div>
    {:else if loading && page === null}
      <div class="table-skeleton" aria-hidden="true">
        {#each [0, 1, 2, 3, 4, 5] as index (index)}<span></span>{/each}
      </div>
    {:else}
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div class="table-scroll" role="region" tabindex="0" aria-label="Root invitations table">
        <table>
          <caption class="visually-hidden">Root role invitations</caption>
          <thead>
            <tr>
              <th scope="col" aria-sort={sortDirection('name')}>
                <button class="table-sort-button" type="button" onclick={() => toggleSort('name')}>
                  <span>Invitee</span><Icon name="sort" size={14} />
                </button>
              </th>
              <th scope="col">System role</th>
              <th scope="col">
                <div class="heading-layout">
                  <span>Status</span>
                  <FilterMenu
                    label="Invitation status"
                    summary={statuses.length === 0 ? 'All statuses' : `${statuses.length} selected`}
                    hint="Filter invitation lifecycle"
                    sections={STATUS_FILTERS}
                    selected={statuses}
                    multiple
                    align="end"
                    showIcon
                    iconOnly
                    placement="header"
                    onChange={selectStatuses}
                  />
                </div>
              </th>
              <th scope="col" aria-sort={sortDirection('expiry')}>
                <button
                  class="table-sort-button"
                  type="button"
                  onclick={() => toggleSort('expiry')}
                >
                  <span>Expires</span><Icon name="sort" size={14} />
                </button>
              </th>
              <th scope="col" aria-sort={sortDirection('created')}>
                <button
                  class="table-sort-button"
                  type="button"
                  onclick={() => toggleSort('created')}
                >
                  <span>Created</span><Icon name="sort" size={14} />
                </button>
              </th>
              <th scope="col"><span class="visually-hidden">Actions</span></th>
            </tr>
          </thead>
          <tbody data-panel-scroll onscroll={loadFromScroll}>
            {#each invitations as invitation (invitation.id)}
              <tr>
                <td data-label="User">
                  <span class="identity">
                    <Avatar account={invitation.account} size={32} />
                    <span
                      ><strong>{invitation.account.display_name}</strong><span class="mono"
                        >@{invitation.account.login}</span
                      ></span
                    >
                  </span>
                </td>
                <td data-label="System role"><Chip tone="signal">Root</Chip></td>
                <td data-label="Status">
                  <Chip tone={statusTone(invitation.status)} dot
                    >{statusLabel(invitation.status)}</Chip
                  >
                </td>
                <td data-label="Expires">
                  {#if invitation.status === 'pending'}
                    <time
                      class="expires-soon"
                      datetime={invitation.expires_at}
                      title={formatTimestamp(invitation.expires_at)}
                    >
                      {formatUntil(invitation.expires_at, now)}
                    </time>
                  {:else if invitation.status === 'expired'}
                    <time
                      datetime={invitation.expires_at}
                      title={formatTimestamp(invitation.expires_at)}
                    >
                      {formatDateTime(invitation.expires_at)}
                    </time>
                  {:else}
                    <!-- Expiry stops meaning anything once the invitation is resolved. -->
                    <span class="cell-dash" aria-hidden="true">—</span>
                  {/if}
                </td>
                <td data-label="Created">
                  <time
                    datetime={invitation.created_at}
                    title={formatTimestamp(invitation.created_at)}
                  >
                    {formatRelative(invitation.created_at, now)}
                  </time>
                </td>
                <td class="row-actions" data-label="Actions">
                  {#if actionItems(invitation).length > 0}
                    <ActionMenu
                      label={`Actions for @${invitation.account.login} invitation`}
                      items={actionItems(invitation)}
                      onSelect={(action, trigger) => chooseAction(invitation, action, trigger)}
                    />
                  {/if}
                </td>
              </tr>
            {:else}
              <tr class="empty-row">
                <td colspan="6">
                  <TableEmptyState
                    title={hasFilters ? 'No invitations match' : 'No Root invitations'}
                    description={hasFilters
                      ? 'Try another search or clear the active filters'
                      : 'Pending Root invitations will appear here'}
                    actionLabel={hasFilters ? 'Clear filters' : undefined}
                    onAction={hasFilters ? clearFilters : undefined}
                  />
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
    <InfiniteLoadSentinel
      active={!loading && page?.next_cursor != null}
      cursor={page?.next_cursor}
      onVisible={loadNext}
    />
    {#if loadMoreProblem !== null}
      <div class="load-more-alert" role="alert">
        <span>{loadMoreProblem}</span>
        <button class="btn" type="button" onclick={loadNext}>Try again</button>
      </div>
    {/if}
  </div>
</section>

<Modal
  id="root-invitation-create"
  open={createOpen}
  title={createStage === 'link'
    ? `Invitation ready for @${generatedFor}`
    : createStage === 'confirm'
      ? 'Invite again?'
      : 'Invite a Root user'}
  description={createStage === 'link'
    ? 'Share this single-use link with the named GitHub user'
    : createStage === 'confirm'
      ? `@${declinedLogin} turned down the last Root invitation`
      : 'Only Super Root can grant application-wide administration'}
  returnFocus={createTrigger}
  onClose={closeCreate}
>
  {#if createStage === 'confirm'}
    <div class="root-warning">
      <Icon name="warning" size={19} />
      <span
        >Declining was an answer. A new link reaches the same GitHub identity, and asking twice is
        visible to them and in the audit record</span
      >
    </div>
    {#if createProblem !== null}<p class="form-error" role="alert">{createProblem}</p>{/if}
  {:else if createStage === 'form'}
    <form id="root-invitation-form" class="invitation-form" onsubmit={submitCreate}>
      <label>
        <span>GitHub login</span>
        <input
          autocomplete="off"
          placeholder="octocat"
          bind:value={login}
          required
          data-modal-focus
        />
        {#if namingSelf}
          <small class="field-refusal">You cannot invite yourself</small>
        {/if}
      </label>
      <label>
        <span>Expires after</span>
        <span class="select-wrap">
          <select
            class="select-input"
            bind:value={expiresInDays}
            aria-label="Root invitation expiry"
          >
            <option value={1}>1 day</option>
            <option value={7}>7 days</option>
            <option value={30}>30 days</option>
          </select>
          <Icon name="chevron-down" size={14} strokeWidth={2} />
        </span>
      </label>
      <div class="root-warning">
        <Icon name="warning" size={19} />
        <span>The recipient becomes a Root only after signing in and accepting this invitation</span
        >
      </div>
      {#if createProblem !== null}<p class="form-error" role="alert">{createProblem}</p>{/if}
    </form>
  {:else}
    <label class="link-field">
      <span>Invitation link</span>
      <input class="mono" readonly value={generatedLink} data-modal-focus />
    </label>
    {#if copyProblem !== null}<p class="form-error" role="alert">{copyProblem}</p>{/if}
  {/if}

  {#snippet footer()}
    {#if createStage === 'confirm'}
      <button class="btn btn-ghost" type="button" onclick={() => (declinedLogin = null)}>
        Back
      </button>
      <button
        class="btn btn-signal"
        type="button"
        disabled={creating}
        onclick={() => void sendInvitation(true)}
      >
        {creating ? 'Creating…' : 'Invite again'}
      </button>
    {:else}
      <button class="btn btn-ghost" type="button" onclick={closeCreate}>
        {createStage === 'form' ? 'Cancel' : 'Done'}
      </button>
    {/if}
    {#if createStage === 'form'}
      <button
        class="btn btn-signal"
        type="submit"
        form="root-invitation-form"
        disabled={creating || login.trim() === '' || namingSelf}
      >
        {creating ? 'Creating…' : 'Create invitation'}
      </button>
    {:else if createStage === 'link'}
      <button class="btn btn-signal" type="button" onclick={() => void copyLink()}>Copy link</button
      >
    {/if}
  {/snippet}
</Modal>

<Modal
  id="root-invitation-action"
  open={actionInvitation !== null && pendingAction !== null}
  title={pendingAction === 'reissue'
    ? `Reissue invitation for @${actionInvitation?.account.login ?? ''}?`
    : `Revoke invitation for @${actionInvitation?.account.login ?? ''}?`}
  description={pendingAction === 'reissue'
    ? 'The existing link is replaced by a fresh link valid for 7 days'
    : 'The current link stops working immediately and its audit record remains'}
  returnFocus={actionTrigger}
  onClose={closeAction}
>
  <div class="root-warning" data-modal-focus tabindex="-1">
    <Icon name={pendingAction === 'reissue' ? 'refresh' : 'warning'} size={19} />
    <span>Confirm this system-role invitation change</span>
  </div>
  {#if actionProblem !== null}<p class="form-error" role="alert">{actionProblem}</p>{/if}

  {#snippet footer()}
    <button class="btn btn-ghost" type="button" onclick={closeAction}>Cancel</button>
    <button
      class="btn"
      class:btn-signal={pendingAction === 'reissue'}
      class:btn-stop={pendingAction === 'revoke'}
      type="button"
      disabled={actionBusy}
      onclick={() => void confirmAction()}
    >
      {actionBusy ? 'Saving…' : 'Confirm'}
    </button>
  {/snippet}
</Modal>

<style>
  .root-invitations {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }

  .invitation-tools {
    /* One 34px row: the section switch leads, the search fills the rest. */
    --control-height: var(--control-height-compact);

    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    min-height: var(--control-height);
    padding-bottom: var(--space-3);
  }

  .invitation-results {
    background: var(--table-filler-bg);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 8rem;
    overflow: hidden;
    position: relative;
  }

  .table-scroll {
    background: var(--surface-base);
    flex: 1;
    max-width: 100%;
    min-height: 0;
    overflow-x: auto;
  }

  table {
    background: var(--surface-base);
    /* Separated, not collapsed: a collapsed border is shared between adjacent
       rows, so each cell owns half of it and every row box lands on a .5. */
    border-collapse: separate;
    border-spacing: 0;
    min-width: 48rem;
    table-layout: fixed;
    width: 100%;
  }

  th,
  td {
    border-bottom: 1px solid var(--rule);
    padding: 0.625rem 0.75rem;
    text-align: left;
    vertical-align: middle;
  }

  th:first-child,
  td:first-child {
    padding-left: var(--space-4);
  }

  thead th:first-child .table-sort-button {
    padding-left: var(--space-4);
  }

  th {
    background: var(--table-header-bg);
    color: var(--dim);
    font: 650 var(--font-size-compact) / 1.2 var(--sans);
    height: 2.5rem;
    letter-spacing: 0.02em;
  }

  th:has(.table-sort-button) {
    padding: 0;
  }

  th:first-child,
  td:first-child {
    width: 26%;
  }

  th:nth-child(2),
  td:nth-child(2),
  th:nth-child(3),
  td:nth-child(3) {
    width: 14%;
  }

  th:nth-child(4),
  td:nth-child(4),
  th:nth-child(5),
  td:nth-child(5) {
    width: 21%;
  }

  th:last-child,
  td:last-child {
    text-align: center;
    width: 3rem;
  }

  .table-sort-button,
  .heading-layout {
    align-items: center;
    display: flex;
    height: 100%;
  }

  .table-sort-button {
    background: transparent;
    border: 0;
    color: inherit;
    font: inherit;
    gap: var(--space-2);
    justify-content: flex-start;
    padding: 0.625rem 0.75rem;
    width: 100%;
  }

  .table-sort-button :global(svg) {
    opacity: 0;
    transition:
      opacity var(--duration-fast) var(--ease-standard),
      transform var(--duration-fast) var(--ease-standard);
  }

  .table-sort-button:hover :global(svg),
  .table-sort-button:focus-visible :global(svg) {
    opacity: 0.55;
  }

  th[aria-sort='ascending'] .table-sort-button :global(svg),
  th[aria-sort='descending'] .table-sort-button :global(svg) {
    opacity: 1;
  }

  th[aria-sort='descending'] .table-sort-button :global(svg) {
    transform: rotate(180deg);
  }

  .heading-layout {
    justify-content: space-between;
  }

  .heading-layout :global(.header-filter) {
    margin-inline: var(--space-1);
  }

  .identity {
    align-items: center;
    display: flex;
    gap: var(--space-2);
    min-width: 0;
  }

  .identity > span:last-child {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .identity strong,
  .identity .mono {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .identity strong {
    font-size: var(--font-size-body);
    line-height: 1.2;
  }

  .identity .mono,
  time {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    line-height: 1.25;
  }

  time {
    white-space: nowrap;
  }

  .cell-dash {
    color: var(--text-muted);
    opacity: 0.6;
  }

  .expires-soon {
    color: var(--text-secondary);
    font-weight: 600;
  }

  .row-actions {
    padding-inline: var(--space-1);
  }

  .empty-row td {
    height: 10rem;
  }

  .empty-row td :global(.table-empty-state) {
    margin-inline: auto;
  }

  .result-state,
  .table-skeleton {
    min-height: 10rem;
  }

  .result-state {
    align-items: center;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    justify-content: center;
    padding: var(--space-6);
    text-align: center;
  }

  .table-skeleton {
    display: grid;
  }

  .table-skeleton span {
    animation: root-invitation-pulse 1.35s ease-in-out infinite alternate;
    border-bottom: 1px solid var(--rule);
    height: 3.5rem;
  }

  .invitation-form {
    display: grid;
    gap: var(--space-4);
    grid-template-columns: minmax(0, 1fr) 9rem;
  }

  .invitation-form label,
  .link-field {
    display: grid;
    gap: var(--space-2);
  }

  .invitation-form label > span,
  .link-field > span {
    font-weight: 650;
  }

  /* Sits under the field that caused it rather than beside the disabled button, so the reason and
     the thing to change are in the same place. */
  .field-refusal {
    color: var(--stop);
    font-size: var(--font-size-compact);
    font-weight: 500;
    /* The label's own grid gap already spaces it; this closes it back up to a helper's distance. */
    margin-top: calc(var(--space-2) * -1 + 0.25rem);
  }

  .invitation-form input,
  .invitation-form select,
  .link-field input {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--radius-control);
    color: var(--text);
    font: inherit;
    min-height: var(--control-height);
    padding: 0 var(--space-3);
    width: 100%;
  }

  .root-warning {
    align-items: center;
    background: var(--interactive-hover);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    color: var(--text-secondary);
    display: flex;
    gap: var(--space-3);
    grid-column: 1 / -1;
    padding: var(--space-3);
  }

  .form-error {
    color: var(--danger);
    font-size: var(--font-size-meta);
    grid-column: 1 / -1;
    margin: 0;
  }

  @keyframes root-invitation-pulse {
    from {
      opacity: 0.48;
    }
    to {
      opacity: 0.88;
    }
  }

  @media (min-width: 64.001rem) {
    .table-scroll,
    table {
      display: flex;
      flex: 1;
      flex-direction: column;
      min-height: 0;
    }

    thead {
      display: block;
      flex: none;
    }

    tbody {
      background: var(--table-filler-bg);
      display: block;
      flex: 1;
      min-height: 0;
      overflow-y: auto;
      overscroll-behavior-y: contain;
    }

    thead tr,
    tbody tr {
      display: table;
      table-layout: fixed;
      width: 100%;
    }

    tbody tr {
      background: var(--surface-base);
      transition: background-color var(--duration-fast) var(--ease-standard);
    }

    tbody tr:not(.empty-row):hover {
      background: var(--table-row-hover);
    }
  }

  @media (max-width: 64rem) {
    .invitation-tools {
      grid-template-columns: 1fr;
    }

    table {
      min-width: 0;
    }

    thead {
      display: none;
    }

    tbody,
    tr,
    td {
      display: block;
      width: 100% !important;
    }

    tbody tr {
      border-bottom: 1px solid var(--rule);
      padding: var(--space-3);
    }

    td {
      align-items: center;
      border: 0;
      display: grid;
      gap: var(--space-3);
      grid-template-columns: 7rem minmax(0, 1fr);
      padding: var(--space-2) 0;
      text-align: left !important;
    }

    td::before {
      color: var(--text-muted);
      content: attr(data-label);
      font: 650 var(--font-size-compact) / 1.2 var(--sans);
    }

    .empty-row td {
      display: flex;
      height: 12rem;
      justify-content: center;
    }

    .empty-row td::before {
      content: none;
    }

    .invitation-form {
      grid-template-columns: 1fr;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .table-skeleton span {
      animation: none;
    }
  }
</style>
