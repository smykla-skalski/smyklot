<script lang="ts">
  import { createInfiniteQuery, type InfiniteData } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
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
  import Button, { type ButtonTone } from './Button.svelte';
  import CopyableLinkField from './CopyableLinkField.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import FormError from './FormError.svelte';
  import Select from './Select.svelte';
  import Callout from './Callout.svelte';
  import IdentityRow from './IdentityRow.svelte';
  import Skeleton from './Skeleton.svelte';
  import Avatar from './Avatar.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import DataTable from './DataTable.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import Icon from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import Modal from './Modal.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import SearchField from './SearchField.svelte';
  import SortIndicator from './SortIndicator.svelte';
  import TableToolsMenu from './TableToolsMenu.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  type SortColumn = 'name' | 'created' | 'expiry';
  /** Name the dialogs in the address, and are the `id` each dialog carries. */
  const CREATE_DIALOG = 'root-invitation-create';
  const ACTION_DIALOG = 'root-invitation-action';

  const STATUS_FILTERS = [
    {
      options: [
        { value: 'pending', label: 'Pending', tone: 'signal' },
        { value: 'accepted', label: 'Accepted', tone: 'valid' },
        { value: 'declined', label: 'Declined', tone: 'neutral' },
        { value: 'revoked', label: 'Revoked', tone: 'invalid' },
        { value: 'expired', label: 'Expired', tone: 'bypassed' },
      ],
    },
  ] satisfies readonly FilterSection[];

  /**
   * ## Why this is not merged with `UserManagement`'s invitation half
   *
   * The plan this branch followed called these two "one feature written twice" and
   * counted fourteen concerns matching one for one - ~450 lines of markup and ~250 of
   * CSS. That was true when it was written. It is not true now, and the reason is that
   * the merge already happened, from underneath: **twelve of the fourteen are the same
   * component in both files.** `DataTable`, `IdentityRow`, `Chip`, `TableEmptyState`,
   * `InfiniteLoadSentinel`, `ConfirmDialog`, `Callout`, `Select`, `Button`,
   * `CopyableLinkField`, `Skeleton` and `FormError` are imported by each. Extracting
   * the primitives and the table shell did the work a wrapper component was going to.
   *
   * What is left is not duplication:
   *
   * - **Different features.** This one reissues AND revokes; the installation's half
   *   only revokes. This one has a three-stage create - form, a confirmation when the
   *   invitee has declined before, then the link - and the other has two. `createStage`
   *   appears eleven times here and nowhere there.
   * - **Different words for the same act.** "The current link stops working immediately
   *   and its audit record remains" here; "The current link will stop working
   *   immediately and the audit record will remain" there. Same meaning, and a merge
   *   has to delete one of them - which is a copy decision, not a refactor. Both are
   *   deployed and neither is wrong.
   *
   * A wrapper over what remains would take a `reissue?`, a `stages`, a `confirmCopy`
   * and a `warning` - four props whose only job is to say which of the two callers is
   * calling. That is a component that has learned nothing, and it would put a seam
   * through the one part a reader of either page most needs to follow.
   *
   * Worth revisiting if the two features converge. Not worth forcing while they differ.
   */
  const {
    fetchPage,
    create,
    reissue,
    revoke,
    canManage,
    actorLogin,
  }: {
    fetchPage: (request: InvitationPageRequest) => Promise<Page<PanelInvitation>>;
    create: (input: AddRootInvitationInput) => Promise<PanelInvitation>;
    reissue: (invitationId: string, expiresInDays: InvitationDays) => Promise<PanelInvitation>;
    revoke: (invitationId: string) => Promise<PanelInvitation>;
    canManage: boolean;
    /** The signed-in login, so naming yourself is answered before the press. */
    actorLogin: string;
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
  /* Reissuing hands an invitation back; revoking takes it away. One value rather
     than two class toggles that nothing stopped from both being on. */
  const confirmTone = $derived<ButtonTone>(
    pendingAction === 'reissue' ? 'signal' : pendingAction === 'revoke' ? 'stop' : 'default',
  );

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

  <div class:loading class="invitation-results table-region" aria-busy={loading}>
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
      <Skeleton bars={false} --skeleton-min-height="10rem" />
    {:else}
      <DataTable
        class="table-scroll"
        pinned
        stacked
        caption="Root role invitations"
        regionLabel="Root invitations table"
        rows={invitations}
        rowKey={(invitation) => invitation.id}
        columnCount={6}
        onBodyScroll={loadFromScroll}
      >
        {#snippet head()}
          <tr>
            <th scope="col" aria-sort={sortDirection('name')}>
              <div class="table-heading">
                <button class="table-sort-button" type="button" onclick={() => toggleSort('name')}>
                  <span class="table-heading-label">Invitee</span><SortIndicator />
                </button>
              </div>
            </th>
            <th scope="col">
              <div class="table-heading">
                <span class="table-heading-label">System role</span>
              </div>
            </th>
            <th scope="col">
              <div class="table-heading">
                <span class="table-heading-label">Status</span>
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
              <div class="table-heading">
                <button
                  class="table-sort-button"
                  type="button"
                  onclick={() => toggleSort('expiry')}
                >
                  <span class="table-heading-label">Expires</span><SortIndicator />
                </button>
              </div>
            </th>
            <th scope="col" aria-sort={sortDirection('created')}>
              <div class="table-heading">
                <button
                  class="table-sort-button"
                  type="button"
                  onclick={() => toggleSort('created')}
                >
                  <span class="table-heading-label">Created</span><SortIndicator />
                </button>
              </div>
            </th>
            <th scope="col"><span class="visually-hidden">Actions</span></th>
          </tr>
        {/snippet}
        {#snippet cells(invitation)}
          <td data-label="User">
            <IdentityRow>
              {#snippet mark()}<Avatar account={invitation.account} size={32} />{/snippet}
              {#snippet name()}<strong>{invitation.account.display_name}</strong>{/snippet}
              {#snippet handle()}
                <span class="mono">@{invitation.account.login}</span>
              {/snippet}
            </IdentityRow>
          </td>
          <td data-label="System role"><Chip tone="signal">Root</Chip></td>
          <td data-label="Status">
            <Chip tone={statusTone(invitation.status)} dot>{statusLabel(invitation.status)}</Chip>
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
              <time datetime={invitation.expires_at} title={formatTimestamp(invitation.expires_at)}>
                {formatDateTime(invitation.expires_at)}
              </time>
            {:else}
              <!-- Expiry stops meaning anything once the invitation is resolved. -->
              <span class="cell-dash" aria-hidden="true">—</span>
            {/if}
          </td>
          <td data-label="Created">
            <time datetime={invitation.created_at} title={formatTimestamp(invitation.created_at)}>
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
        {/snippet}
        {#snippet empty()}
          <TableEmptyState
            title={hasFilters ? 'No invitations match' : 'No Root invitations'}
            description={hasFilters
              ? 'Try another search or clear the active filters'
              : 'Pending Root invitations will appear here'}
            actionLabel={hasFilters ? 'Clear filters' : undefined}
            onAction={hasFilters ? clearFilters : undefined}
          />
        {/snippet}
      </DataTable>
    {/if}
    <InfiniteLoadSentinel
      active={!loading && loadMoreProblem === null && page?.next_cursor != null}
      cursor={page?.next_cursor}
      onVisible={loadNext}
    />
    {#if loadMoreProblem !== null}
      <div class="load-more-alert" role="alert">
        <span>{loadMoreProblem}</span>
        <Button onclick={loadNext}>Try again</Button>
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
    <Callout class="root-warning">
      {#snippet icon()}<Icon name="warning" size={19} />{/snippet}
      <span
        >Declining was an answer. A new link reaches the same GitHub identity, and asking twice is
        visible to them and in the audit record</span
      >
    </Callout>
    <FormError message={createProblem} />
  {:else if createStage === 'form'}
    <form id="root-invitation-form" class="invitation-form" onsubmit={submitCreate}>
      <label>
        <span>GitHub login</span>
        <input autocomplete="off" placeholder="octocat" bind:value={login} required />
        {#if namingSelf}
          <small class="field-refusal">You cannot invite yourself</small>
        {/if}
      </label>
      <label>
        <span>Expires after</span>
        <Select
          bind:value={expiresInDays}
          aria-label="Root invitation expiry"
          options={[
            { value: 1, label: '1 day' },
            { value: 7, label: '7 days' },
            { value: 30, label: '30 days' },
          ]}
        />
      </label>
      <Callout class="root-warning">
        {#snippet icon()}<Icon name="warning" size={19} />{/snippet}
        <span>The recipient becomes a Root only after signing in and accepting this invitation</span
        >
      </Callout>
      <FormError message={createProblem} />
    </form>
  {:else}
    <CopyableLinkField
      label="Invitation link"
      value={generatedLink}
      failed={copyProblem !== null}
    />
  {/if}

  {#snippet footer()}
    {#if createStage === 'confirm'}
      <Button tone="ghost" onclick={() => (declinedLogin = null)}>Back</Button>
      <Button tone="signal" disabled={creating} onclick={() => void sendInvitation(true)}>
        {creating ? 'Creating…' : 'Invite again'}
      </Button>
    {:else}
      <Button tone="ghost" onclick={closeCreate}>
        {createStage === 'form' ? 'Cancel' : 'Done'}
      </Button>
    {/if}
    {#if createStage === 'form'}
      <Button
        tone="signal"
        type="submit"
        form="root-invitation-form"
        disabled={creating || login.trim() === '' || namingSelf}
      >
        {creating ? 'Creating…' : 'Create invitation'}
      </Button>
    {:else if createStage === 'link'}
      <Button tone="signal" onclick={() => void copyLink()}>Copy link</Button>
    {/if}
  {/snippet}
</Modal>

<ConfirmDialog
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
  onConfirm={() => void confirmAction()}
  {confirmTone}
  busy={actionBusy}
>
  <Callout class="root-warning">
    {#snippet icon()}
      <Icon name={pendingAction === 'reissue' ? 'refresh' : 'warning'} size={19} />
    {/snippet}
    <span>Confirm this system-role invitation change</span>
  </Callout>
  <FormError message={actionProblem} />
</ConfirmDialog>

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

  /* Layout, keyline and corner come from `.table-region` in `app.css`. */
  .invitation-results {
    min-height: 8rem;
  }

  /* Surface, keyline and corner come from `.table-card`; the scroll shell, the cell
     padding and the separator from `DataTable` and `.data-table`. These are this
     table's own settings for them. */
  :global(.table-scroll) {
    --table-cell-pad-block: 0.625rem;
    --table-cell-pad-inline: 0.75rem;
    --table-empty-height: 10rem;
    --table-heading-height: 2.5rem;
    --table-layout: fixed;
    --table-min-width: 48rem;

    flex: 1;
    max-width: 100%;
    min-height: 0;
  }

  /* The first column's wider inset, on both halves so the band and the rows below
     it start on the same edge. */
  td:first-child {
    padding-left: var(--space-4);
  }

  /* `:global`, because `thead` is `DataTable`'s element - the `th` inside it is
     this file's, but a descendant selector needs both ends to match. */
  :global(.table-scroll thead th:first-child) {
    --heading-pad-start: var(--space-4);
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

  /* The heading's row, its button and its arrow are shared - see `.table-heading`,
     `.table-sort-button` and `.sort-indicator` in `app.css`. This table kept the
     worst copy of all three: a `background: transparent` that outranked the shared
     hover and removed it, arrow rules written against the raw `<svg>`, and the
     rotationally-symmetric `sort` glyph, which says a column can be sorted and
     never which way it is. */

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

  .invitation-form {
    display: grid;
    gap: var(--space-4);
    grid-template-columns: minmax(0, 1fr) 9rem;
  }

  .invitation-form label {
    display: grid;
    gap: var(--space-2);
  }

  .invitation-form label > span {
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

  /* The generated link is no longer in this list. It goes through
     `CopyableLinkField`, which dresses it as `.text-input` like every other input in
     the panel - this form's own taller treatment was the one place that disagreed,
     and matching it here would have meant teaching a shared component about one
     dialog. */
  .invitation-form input,
  .invitation-form :global(select) {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--radius-control);
    color: var(--text);
    font: inherit;
    min-height: var(--control-height);
    padding: 0 var(--space-3);
    width: 100%;
  }

  /* The box itself is `Callout` now; what is left is where it sits in this form's
     grid, which is this component's business and not the callout's. `:global`
     because the element is the child component's. */
  :global(.root-warning) {
    grid-column: 1 / -1;
  }

  /* The form is a grid; the message spans it. */
  :global(.form-error) {
    grid-column: 1 / -1;
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

    .invitation-form {
      grid-template-columns: 1fr;
    }
  }

  @media (prefers-reduced-motion: reduce) {
  }
</style>
