<script lang="ts">
  import { createInfiniteQuery, type InfiniteData } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import { useDebounce, useInterval } from 'runed';
  import { PanelApiError } from '../api';
  import { dialogRoute } from '../dialog-route.svelte';
  import type { FilterSection } from '../filter-menu';
  import { receipts } from '../receipts.svelte';
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
  import Card from './Card.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import FormError from './FormError.svelte';
  import Select from './Select.svelte';
  import Callout from './Callout.svelte';
  import Skeleton from './Skeleton.svelte';
  import Icon from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import Modal from './Modal.svelte';
  import Pill, { type PillTone } from './Pill.svelte';
  import RelativeTime from './RelativeTime.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import SearchField from './SearchField.svelte';
  import ListToolsMenu, { type ToolsSort } from './ListToolsMenu.svelte';

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
  /* The empty state carries its own way in, so focus returns to the button that
     was actually pressed rather than to the header's. */
  let emptyTrigger = $state<HTMLButtonElement | null>(null);
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

  /* The order lives in the tools menu now that the rows are sentences: a column
     heading is where a reader looks for sort, and there are no headings. */
  const toolSorts = $derived<ToolsSort[]>(
    (
      [
        ['Name', 'name'],
        ['Expiry', 'expiry'],
        ['Sent', 'created'],
      ] as const
    ).map(([label, column]) => ({
      label,
      direction: sortDirection(column),
      onToggle: () => toggleSort(column),
    })),
  );

  function selectStatuses(values: string[]): void {
    statuses = values.filter((value): value is InvitationStatus =>
      ['pending', 'accepted', 'declined', 'revoked', 'expired'].includes(value),
    );
  }

  function statusLabel(status: InvitationStatus): string {
    return status.charAt(0).toLocaleUpperCase() + status.slice(1);
  }

  /**
   * A standing worth a word, and nothing where the row is simply waiting.
   *
   * A list of invitations is a list of pending ones; a pill on every row saying
   * "Pending" is a column that has learned nothing. What is left is the three
   * ways one ends.
   */
  function standing(status: InvitationStatus): PillTone | null {
    if (status === 'pending') return null;
    if (status === 'accepted') return 'success';
    if (status === 'revoked' || status === 'declined') return 'danger';

    return 'warning';
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
      /* Sticky: the link is the whole of what this did, and it works once. */
      receipts.say(
        `Operator invite link made for @${invitation.account.login} - it works once and expires in ${expiresInDays} ${expiresInDays === 1 ? 'day' : 'days'}`,
        { sticky: true },
      );
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
      receipts.say(`Copied the invite link for @${generatedFor}`);
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
        receipts.say(`A fresh link is out for @${updated.account.login} - it expires in 7 days`, {
          sticky: true,
        });
      } else {
        await revoke(actionInvitation.id);
        receipts.say(`Invitation for @${actionInvitation.account.login} revoked`);
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

<!--
@component
Every invitation across every workspace, which is the operator's view of them -
who was asked, by whom, and what has become of it.

## Why this is not merged with `UserManagement`'s invitation half

The plan this branch followed called these two "one feature written twice" and
counted fourteen concerns matching one for one - ~450 lines of markup and ~250 of
CSS. That was true when it was written. It is not true now, and the reason is that
the merge already happened, from underneath: **twelve of the fourteen are the same
component in both files.** `IdentityRow`, `Chip`, `EmptyState`,
`InfiniteLoadSentinel`, `ConfirmDialog`, `Callout`, `Select`, `Button`,
`CopyableLinkField`, `Skeleton`, `ListToolsMenu` and `FormError` are imported by
each. Extracting the primitives did the work a wrapper component was going to.

What is left is not duplication:

- **Different features.** This one reissues AND revokes; the workspace's half
  only revokes. This one has a three-stage create - form, a confirmation when the
  invitee has declined before, then the link - and the other has two. `createStage`
  appears eleven times here and nowhere there.
- **Different words for the same act.** "The current link stops working immediately
  and its audit record remains" here; "The current link will stop working
  immediately and the audit record will remain" there. Same meaning, and a merge
  has to delete one of them - which is a copy decision, not a refactor. Both are
  deployed and neither is wrong.

A wrapper over what remains would take a `reissue?`, a `stages`, a `confirmCopy`
and a `warning` - four props whose only job is to say which of the two callers is
calling. That is a component that has learned nothing, and it would put a seam
through the one part a reader of either page most needs to follow.

Worth revisiting if the two features converge. Not worth forcing while they differ.
-->

<section class="root-invitations" aria-label="Operator invitations">
  <!-- The bar only appears where there is a list to narrow. An empty page offering
       a search field is a page telling a reader their search came up dry when they
       have not searched for anything. -->
  {#if invitations.length > 0 || hasFilters}
    <div class="filter-bar">
      <SearchField
        label="Find an invitation"
        placeholder="Find an invitation"
        value={search}
        onInput={(value) => (search = value)}
      />
      <span class="push-end">
        <ListToolsMenu
          label="Filter invitations"
          sorts={toolSorts}
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
      </span>
    </div>
  {/if}

  <div class:loading class="invitation-results list-region" aria-busy={loading}>
    <!-- A refresh that failed over a loaded list has not made the list wrong. -->
    {#if problem !== null && page !== null}
      <ResultProblem
        title="The invitations could not be read"
        {problem}
        busy={loading}
        onRetry={() => void loadPage(undefined, false)}
        overContent
      />
    {/if}

    {#if problem !== null && page === null}
      <ResultProblem
        title="The invitations could not be read"
        {problem}
        busy={loading}
        onRetry={() => void loadPage(undefined, false)}
      />
    {:else if loading && page === null}
      <Skeleton bars={false} --skeleton-min-height="10rem" />
    {:else}
      <Card>
        {#if invitations.length === 0}
          <!-- What would be here, and the one next step - not a magnifying glass
               pretending a search came up dry. -->
          <div class="state-panel">
            {#if hasFilters}
              <span
                ><strong>Nothing matches.</strong> No invitation here answers to what is being asked</span
              >
              <Button onclick={clearFilters}>Clear the filters</Button>
            {:else}
              <span
                ><strong>No operator invitations are open.</strong> Invite an operator to create a one-time
                link with an expiry</span
              >
              {#if canManage}
                <Button
                  tone="signal"
                  bind:element={emptyTrigger}
                  onclick={() => openCreate(emptyTrigger)}
                >
                  {#snippet icon()}<Icon name="user-plus" size="sm" strokeWidth={2} />{/snippet}
                  Invite an operator
                </Button>
              {/if}
            {/if}
          </div>
        {:else}
          <ul class="object-list">
            {#each invitations as invitation (invitation.id)}
              {@const tone = standing(invitation.status)}
              <li>
                <div class="object-row">
                  <span class="object-main">
                    <span class="object-name-row">
                      <span class="object-name">{invitation.account.display_name}</span>
                      {#if tone !== null}
                        <Pill {tone}>{statusLabel(invitation.status)}</Pill>
                      {/if}
                    </span>
                    <span class="object-sum"
                      >@{invitation.account.login} · invited
                      <RelativeTime value={invitation.created_at} nowMs={now} />
                      {#if invitation.status === 'pending'}· the link expires
                        <RelativeTime
                          class="expires-soon"
                          value={invitation.expires_at}
                          nowMs={now}
                          future
                        />
                      {:else if invitation.status === 'expired'}· the link expired
                        <RelativeTime value={invitation.expires_at} nowMs={now} />
                      {/if}</span
                    >
                  </span>
                  <span class="object-side">
                    {#if actionItems(invitation).length > 0}
                      <ActionMenu
                        label={`Actions for @${invitation.account.login} invitation`}
                        items={actionItems(invitation)}
                        onSelect={(action, trigger) => chooseAction(invitation, action, trigger)}
                      />
                    {/if}
                  </span>
                </div>
              </li>
            {/each}
          </ul>
          <div class="list-foot">
            <span
              >Showing 1-{invitations.length} of {page?.total ?? invitations.length}{hasFilters
                ? ' matching'
                : ''}</span
            >
          </div>
        {/if}
      </Card>
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
      : 'Invite an operator'}
  description={createStage === 'link'
    ? 'Share this single-use link with the named GitHub user'
    : createStage === 'confirm'
      ? `@${declinedLogin} turned down the last invitation`
      : 'An operator reads the whole service and may enter any workspace - only the lead operator may invite one'}
  returnFocus={createTrigger}
  onClose={closeCreate}
>
  {#if createStage === 'confirm'}
    <Callout class="root-warning">
      {#snippet icon()}<Icon name="warning" size="md" />{/snippet}
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
          aria-label="Invitation expiry"
          options={[
            { value: 1, label: '1 day' },
            { value: 7, label: '7 days' },
            { value: 30, label: '30 days' },
          ]}
        />
      </label>
      <Callout class="root-warning">
        {#snippet icon()}<Icon name="warning" size="md" />{/snippet}
        <span
          >The recipient becomes an operator only after signing in and accepting this invitation</span
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
      <Icon name={pendingAction === 'reissue' ? 'refresh' : 'warning'} size="md" />
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

  .filter-bar :global(.search-field) {
    flex: 1 1 12rem;
    max-inline-size: 20rem;
    min-inline-size: 0;
  }

  .invitation-results {
    min-height: 8rem;
  }

  /* The one time on the row a reader is counting down to. */
  :global(.expires-soon) {
    color: var(--text-secondary);
    font-weight: 600;
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
    color: var(--danger);
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
    color: var(--text-primary);
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

  @media (max-width: 64rem) {
    .invitation-form {
      grid-template-columns: 1fr;
    }
  }

  @media (prefers-reduced-motion: reduce) {
  }
</style>
