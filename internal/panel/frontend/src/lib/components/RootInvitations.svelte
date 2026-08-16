<script lang="ts">
  import { createInfiniteQuery, type InfiniteData } from '@tanstack/svelte-query';
  import { untrack, type Snippet } from 'svelte';
  import { useDebounce, useInterval } from 'runed';
  import { PanelApiError } from '../api';
  import { dialogRoute } from '../dialog-route.svelte';
  import { formatDateTime, formatRelative, formatTimestamp, formatUntil } from '../format';
  import type { FilterSection } from '../filter-menu';
  import type {
    AddRootInvitationInput,
    InvitationDays,
    InvitationPageRequest,
    InvitationSort,
    InvitationStatus,
    Page,
    PanelInvitation,
  } from '../types';
  import ActionMenu, { type ActionMenuItem } from './ActionMenu.svelte';
  import Avatar from './Avatar.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import Icon from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import Modal from './Modal.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import SearchField from './SearchField.svelte';
  import TableToolsMenu from './TableToolsMenu.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  type SortColumn = 'name' | 'created' | 'expiry';
  /** Name the dialogs in the address, and are the `id` each dialog carries. */
  const CREATE_DIALOG = 'root-invitation-create';
  const ACTION_DIALOG = 'root-invitation-action';

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
    create,
    reissue,
    revoke,
    canManage,
    actorLogin,
    navigation,
  }: {
    fetchPage: (request: InvitationPageRequest) => Promise<Page<PanelInvitation>>;
    create: (input: AddRootInvitationInput) => Promise<PanelInvitation>;
    reissue: (invitationId: string, expiresInDays: InvitationDays) => Promise<PanelInvitation>;
    revoke: (invitationId: string) => Promise<PanelInvitation>;
    canManage: boolean;
    /** The signed-in login, so naming yourself is answered before the press. */
    actorLogin: string;
    navigation?: Snippet;
  } = $props();

  // Ticks so the pending countdown and relative Created column keep aging.
  let now = $state(Date.now());
  let search = $state('');
  let query = $state('');
  let sort = $state<InvitationSort>('created_newest');
  let statuses = $state<InvitationStatus[]>([]);

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

  let actionTrigger = $state<HTMLElement | null>(null);
  let actionBusy = $state(false);
  let actionProblem = $state<string | null>(null);

  const limit = 20;
  const invitationQuery = createInfiniteQuery(() => ({
    queryKey: ['root-access', 'invitations', query, sort, [...statuses], limit],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      fetchPage({
        ...(pageParam === undefined ? {} : { cursor: pageParam }),
        query,
        sort,
        limit,
        roles: [],
        statuses: [...statuses],
      }),
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  }));
  const page = $derived(flattenPages(invitationQuery.data));
  const loading = $derived(invitationQuery.isFetching);
  const problem = $derived(
    invitationQuery.isError && !invitationQuery.isFetchNextPageError
      ? errorMessage(invitationQuery.error)
      : null,
  );
  const loadMoreProblem = $derived(
    invitationQuery.isFetchNextPageError ? errorMessage(invitationQuery.error) : null,
  );
  const invitations = $derived(page?.items ?? []);
  const hasFilters = $derived(query !== '' || statuses.length > 0);

  /* Both dialogs are whatever the address names, so a reload keeps the reader
     where they were. The invitation is looked up in the loaded page, and one that
     has since been used or revoked is no longer there to open. */
  const createOpen = $derived(dialogRoute.isOpen(CREATE_DIALOG));
  const actionInvitation = $derived.by(() => {
    const id = dialogRoute.param(ACTION_DIALOG, 'invitation');
    if (id === undefined) return null;
    return invitations.find((invitation) => invitation.id === id) ?? null;
  });
  const pendingAction = $derived.by(() => {
    if (actionInvitation === null) return null;
    const action = dialogRoute.param(ACTION_DIALOG, 'action');
    return action === 'reissue' || action === 'revoke' ? action : null;
  });

  useInterval(30_000, { callback: () => (now = Date.now()) });
  const debouncedSearch = useDebounce((value: string) => (query = value), 180);
  $effect(() => {
    const value = search.trim();
    untrack(() => void debouncedSearch(value));
  });

  async function loadPage(_cursor: string | undefined, append: boolean): Promise<void> {
    if (append) await invitationQuery.fetchNextPage();
    else await invitationQuery.refetch();
  }

  function loadNext(): void {
    if (invitationQuery.hasNextPage && !invitationQuery.isFetchingNextPage) {
      void invitationQuery.fetchNextPage();
    }
  }

  function flattenPages(data: InfiniteData<Page<PanelInvitation>> | undefined) {
    const pages = data?.pages;
    if (pages === undefined || pages.length === 0) return null;
    const last = pages.at(-1);
    return last === undefined ? null : { ...last, items: pages.flatMap((entry) => entry.items) };
  }

  function errorMessage(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
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
    dialogRoute.open(CREATE_DIALOG);
  }

  function closeCreate(): void {
    if (creating) return;
    if (dialogRoute.isOpen(CREATE_DIALOG)) dialogRoute.close();
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
    actionTrigger = trigger;
    actionProblem = null;
    dialogRoute.open(ACTION_DIALOG, { invitation: invitation.id, action });
  }

  function closeAction(): void {
    if (actionBusy) return;
    if (dialogRoute.isOpen(ACTION_DIALOG)) dialogRoute.close();
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
        /* The confirmation gives way to the dialog holding the new link, in the
           entry the confirmation was occupying: one press of Back leaves the
           reissue rather than walking back through the question. */
        dialogRoute.open(CREATE_DIALOG);
      } else {
        await revoke(actionInvitation.id);
        if (dialogRoute.isOpen(ACTION_DIALOG)) dialogRoute.close();
      }
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
    <!-- The status filter lives in a column heading, and the heading band is
         hidden once this table becomes a stack of cards. Without this the page
         offered a search field and nothing else. -->
    <TableToolsMenu
      label="Filter invitations"
      sorts={[]}
      filters={[
        {
          label: 'Status',
          hint: 'Filter invitation lifecycle',
          sections: STATUS_FILTERS,
          selected: statuses,
          multiple: true,
          onChange: selectStatuses,
        },
      ]}
    />
  </div>

  <div class:loading class="invitation-results" aria-busy={loading}>
    <!-- A refresh that failed over a loaded table has not made the table wrong. -->
    {#if problem !== null && page !== null}
      <ResultProblem
        title="Root invitations could not be loaded"
        {problem}
        busy={loading}
        onRetry={() => void loadPage(undefined, false)}
        overContent
      />
    {/if}

    {#if problem !== null && page === null}
      <ResultProblem
        title="Root invitations could not be loaded"
        {problem}
        busy={loading}
        onRetry={() => void loadPage(undefined, false)}
      />
    {:else if loading && page === null}
      <div class="table-skeleton" aria-hidden="true">
        {#each [0, 1, 2, 3, 4, 5] as index (index)}<span></span>{/each}
      </div>
    {:else}
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div
        class="table-scroll table-card"
        role="region"
        tabindex="0"
        aria-label="Root invitations table"
      >
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
                  <span class="heading-label band-trim">Status</span>
                  <FilterMenu
                    label="Invitation status"
                    summary={statuses.length === 0 ? 'All statuses' : `${statuses.length} selected`}
                    hint="Filter invitation lifecycle"
                    sections={STATUS_FILTERS}
                    selected={statuses}
                    multiple
                    align="end"
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
                    <span class="band-trim-stack"
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
      active={!loading && loadMoreProblem === null && page?.next_cursor != null}
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
  id={CREATE_DIALOG}
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
  id={ACTION_DIALOG}
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

  /* Surface, keyline, corner and lift come from `.table-card` in `app.css`. */
  .table-scroll {
    flex: 1;
    max-width: 100%;
    min-height: 0;
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

  /* The header's rule comes from `thead th` in `app.css`; this is the row
     separator. */
  td {
    border-bottom: 1px solid var(--rule);
  }

  th,
  td {
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

  /* Typography and ground come from `thead th` in `app.css`. */
  th {
    height: 2.5rem;
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

  .table-skeleton {
    min-height: 10rem;
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

  /* Only where the column headings are not: the Status heading carries the same
     filter while the table is a table. */
  .invitation-tools :global(.tools-trigger) {
    display: none;
  }

  @media (max-width: 64rem) {
    .invitation-tools :global(.tools-trigger) {
      display: inline-flex;
    }

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
