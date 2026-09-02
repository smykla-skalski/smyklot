<script lang="ts">
  import { createInfiniteQuery, createQuery, type InfiniteData } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import { useDebounce, useInterval } from 'runed';

  import { dialogRoute } from '../dialog-route.svelte';
  import type { FilterSection } from '../filter-menu';
  import type {
    Page,
    AddTargetUserInput,
    AddRootInvitationInput,
    PanelAccount,
    InvitationDays,
    InvitationPageRequest,
    PanelInvitation,
    PanelUser,
    PanelUserStatus,
    RootPanelUser,
    RootPanelUserPageRequest,
    RootPanelUserSort,
    RootWorkspace,
    WorkspaceRole,
    SystemRole,
    UpdateRootUserInput,
  } from '../types';
  import ActionMenu, { type ActionMenuItem } from './ActionMenu.svelte';
  import Button, { type ButtonTone } from './Button.svelte';
  import Card from './Card.svelte';
  import Chip from './Chip.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import Select from './Select.svelte';
  import Callout from './Callout.svelte';
  import Skeleton from './Skeleton.svelte';
  import Icon from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import LoginField from './LoginField.svelte';
  import Modal from './Modal.svelte';
  import Pill, { type PillTone } from './Pill.svelte';
  import RelativeTime from './RelativeTime.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import RootInvitations from './RootInvitations.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import SearchField from './SearchField.svelte';
  import ListToolsMenu, { type ToolsSort } from './ListToolsMenu.svelte';

  type AccessSection = 'users' | 'invitations';
  type SortColumn = 'name' | 'role' | 'last_login';
  type UserAction = (typeof USER_ACTIONS)[number];

  const USER_ACTIONS = ['promote_root', 'demote_root', 'restore', 'ban', 'remove'] as const;

  /** Name the dialogs in the address, and are the `id` each dialog carries. */
  const ACTION_DIALOG = 'root-user-action';
  const ADD_DIALOG = 'root-add-workspace-user';

  /* One word for what used to be three - "Root user", "Root invitation" and
     "system-level access" all meant somebody who may operate the service. */
  const ROLE_FILTERS = [
    {
      options: [
        { value: 'super_root', label: 'Lead operator' },
        { value: 'root', label: 'Operator' },
        { value: 'none', label: 'Everyone else' },
      ],
    },
  ] satisfies readonly FilterSection[];
  const STATUS_FILTERS = [
    {
      options: [
        { value: 'active', label: 'Active', tone: 'valid' },
        { value: 'banned', label: 'Banned', tone: 'invalid' },
        { value: 'removed', label: 'Removed', tone: 'neutral' },
      ],
    },
  ] satisfies readonly FilterSection[];

  const {
    section,
    fetchUsers,
    updateUser,
    fetchInvitations,
    createInvitation,
    reissueInvitation,
    revokeInvitation,
    canManageInvitations,
    actorLogin,
    fetchWorkspaces,
    addWorkspaceUser,
    suggestUsers,
    onOpenWorkspaceAccess,
  }: {
    section: AccessSection;
    fetchUsers: (request: RootPanelUserPageRequest) => Promise<Page<RootPanelUser>>;
    updateUser: (accountId: string, input: UpdateRootUserInput) => Promise<void>;
    fetchInvitations: (request: InvitationPageRequest) => Promise<Page<PanelInvitation>>;
    createInvitation: (input: AddRootInvitationInput) => Promise<PanelInvitation>;
    reissueInvitation: (
      invitationId: string,
      expiresInDays: InvitationDays,
    ) => Promise<PanelInvitation>;
    revokeInvitation: (invitationId: string) => Promise<PanelInvitation>;
    canManageInvitations: boolean;
    /** The signed-in login, so naming yourself is answered before the press. */
    actorLogin: string;
    fetchWorkspaces: () => Promise<RootWorkspace[]>;
    addWorkspaceUser: (targetId: string, input: AddTargetUserInput) => Promise<PanelUser>;
    /** Completes a login against the chosen workspace's organization. */
    suggestUsers: (targetId: string, query: string) => Promise<PanelAccount[]>;
    onOpenWorkspaceAccess: (account: string) => void;
  } = $props();

  let search = $state('');
  let query = $state('');
  let sort = $state<RootPanelUserSort>('name_asc');
  let systemRoles = $state<SystemRole[]>([]);
  let statuses = $state<PanelUserStatus[]>([]);
  let actionTrigger = $state<HTMLElement | null>(null);
  let reason = $state('');
  let saving = $state(false);
  let actionProblem = $state<string | null>(null);
  let addTrigger = $state<HTMLButtonElement | null>(null);
  let inviteTrigger = $state<HTMLButtonElement | null>(null);
  let invitations = $state<RootInvitations | null>(null);
  let addLogin = $state('');
  let addRole = $state<Exclude<WorkspaceRole, 'none' | 'owner'>>('viewer');
  let workspaceQuery = $state('');
  let selectedWorkspaceID = $state('');
  let addSaving = $state(false);
  let addProblem = $state<string | null>(null);
  let feedback = $state('');
  const limit = 20;
  // Ticks so relative login times keep aging in a long Root session.
  let now = $state(Date.now());
  const usersQuery = createInfiniteQuery(() => ({
    queryKey: ['root-access', 'users', query, sort, [...systemRoles], [...statuses], limit],
    enabled: section === 'users',
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      fetchUsers({
        ...(pageParam === undefined ? {} : { cursor: pageParam }),
        query,
        sort,
        limit,
        systemRoles: [...systemRoles],
        statuses: [...statuses],
      }),
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  }));
  const page = $derived(flattenPages(usersQuery.data));
  const loading = $derived(usersQuery.isFetching);
  const problem = $derived(
    usersQuery.isError && !usersQuery.isFetchNextPageError ? errorMessage(usersQuery.error) : null,
  );
  const loadMoreProblem = $derived(
    usersQuery.isFetchNextPageError ? errorMessage(usersQuery.error) : null,
  );
  const users = $derived(page?.items ?? []);
  const hasFilters = $derived(query !== '' || systemRoles.length > 0 || statuses.length > 0);

  /* Both dialogs are whatever the address names, so a reload keeps the reader
     where they were. The account is named by login and looked up in the loaded
     page: an address naming somebody who is no longer listed opens nothing. */
  const addOpen = $derived(dialogRoute.isOpen(ADD_DIALOG));
  const actionUser = $derived.by(() => {
    const login = dialogRoute.param(ACTION_DIALOG, 'user');
    if (login === undefined) return null;
    return users.find((user) => user.account.login === login) ?? null;
  });
  const pendingAction = $derived(
    actionUser === null ? null : userAction(dialogRoute.param(ACTION_DIALOG, 'action')),
  );
  /* What the confirmation button reads as: taking something away is destructive,
     giving something is the action the dialog is here for, and anything else is a
     plain control. One value rather than two class toggles that could both be on. */
  const confirmTone = $derived<ButtonTone>(
    pendingAction === 'ban' || pendingAction === 'remove' || pendingAction === 'demote_root'
      ? 'stop'
      : pendingAction === 'promote_root' || pendingAction === 'restore'
        ? 'signal'
        : 'default',
  );
  const workspacesQuery = createQuery(() => ({
    queryKey: ['root-workspaces'],
    queryFn: fetchWorkspaces,
    enabled: addOpen,
  }));
  const workspaces = $derived<RootWorkspace[]>(workspacesQuery.data ?? []);
  const workspacesLoading = $derived(
    addOpen && workspacesQuery.isFetching && workspacesQuery.data === undefined,
  );
  const workspacesProblem = $derived(
    addOpen && workspacesQuery.error !== null ? errorMessage(workspacesQuery.error) : null,
  );
  const defaultWorkspaceID = $derived(
    workspaces.find((workspace) => workspace.available)?.id ?? '',
  );
  const effectiveWorkspaceID = $derived(
    selectedWorkspaceID === '' ? defaultWorkspaceID : selectedWorkspaceID,
  );

  function userAction(value: string | undefined): UserAction | null {
    return USER_ACTIONS.find((action) => action === value) ?? null;
  }
  const selectedWorkspace = $derived(
    workspaces.find((workspace) => workspace.id === effectiveWorkspaceID) ?? null,
  );
  const filteredWorkspaces = $derived.by(() => {
    const needle = workspaceQuery.trim().toLocaleLowerCase();
    if (needle === '') return workspaces;
    return workspaces.filter((workspace) =>
      [workspace.account.display_name, workspace.account.login].some((value) =>
        value.toLocaleLowerCase().includes(needle),
      ),
    );
  });

  useInterval(30_000, { callback: () => (now = Date.now()) });
  const debouncedSearch = useDebounce((value: string) => (query = value), 180);
  $effect(() => {
    const value = search.trim();
    untrack(() => void debouncedSearch(value));
  });

  function toggleSort(column: SortColumn): void {
    const pairs: Record<SortColumn, readonly [RootPanelUserSort, RootPanelUserSort]> = {
      name: ['name_asc', 'name_desc'],
      role: ['role_asc', 'role_desc'],
      last_login: ['login_oldest', 'login_newest'],
    };
    const [ascending, descending] = pairs[column];
    sort = sort === ascending ? descending : ascending;
  }

  function sortDirection(column: SortColumn): 'ascending' | 'descending' | undefined {
    const prefix = column === 'last_login' ? 'login_' : `${column}_`;
    if (!sort.startsWith(prefix)) return undefined;
    return sort.endsWith('desc') || sort === 'login_newest' ? 'descending' : 'ascending';
  }

  /* The order lives in the tools menu now that the rows are sentences rather than
     columns: a column heading is where a reader looks for sort, and there are no
     headings to look at. */
  const toolSorts = $derived<ToolsSort[]>(
    (
      [
        ['Name', 'name'],
        ['Service role', 'role'],
        ['Last signed in', 'last_login'],
      ] as const
    ).map(([label, column]) => ({
      label,
      direction: sortDirection(column),
      onToggle: () => toggleSort(column),
    })),
  );

  function selectRoles(values: string[]): void {
    systemRoles = values.filter((value): value is SystemRole =>
      ['none', 'root', 'super_root'].includes(value),
    );
  }

  function selectStatuses(values: string[]): void {
    statuses = values.filter((value): value is PanelUserStatus =>
      ['active', 'banned', 'removed'].includes(value),
    );
  }

  async function loadPage(_cursor: string | undefined, append: boolean): Promise<void> {
    if (append) await usersQuery.fetchNextPage();
    else await usersQuery.refetch();
  }

  function loadNext(): void {
    if (usersQuery.hasNextPage && !usersQuery.isFetchingNextPage) {
      void usersQuery.fetchNextPage();
    }
  }

  function flattenPages(data: InfiniteData<Page<RootPanelUser>> | undefined) {
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
    systemRoles = [];
    statuses = [];
  }

  function openAddUser(): void {
    dialogRoute.open(ADD_DIALOG);
    addLogin = '';
    addRole = 'viewer';
    workspaceQuery = '';
    selectedWorkspaceID = '';
    addProblem = null;
  }

  function closeAddUser(): void {
    if (addSaving) return;
    if (dialogRoute.isOpen(ADD_DIALOG)) dialogRoute.close();
    addProblem = null;
  }

  async function submitAddUser(): Promise<void> {
    const workspace = selectedWorkspace;
    if (workspace === null || !workspace.available) return;
    if (!workspace.owned_by_viewer) {
      if (dialogRoute.isOpen(ADD_DIALOG)) dialogRoute.close();
      onOpenWorkspaceAccess(workspace.account.login);
      return;
    }
    const login = addLogin.trim();
    if (login === '') return;
    addSaving = true;
    addProblem = null;
    try {
      await addWorkspaceUser(workspace.id, { login, role: addRole });
      feedback = `Added @${login} to ${workspace.account.display_name}`;
      if (dialogRoute.isOpen(ADD_DIALOG)) dialogRoute.close();
      await loadPage(undefined, false);
    } catch (error) {
      addProblem = error instanceof Error ? error.message : String(error);
    } finally {
      addSaving = false;
    }
  }

  /**
   * A standing worth a word, or nothing at all.
   *
   * An ordinary account in good standing wears no pill: the column that said
   * "Account" on every row said nothing, and a page of identical pills teaches a
   * reader to stop reading them. What is left is what somebody might act on -
   * who may operate the service, and who cannot get in.
   */
  function userStanding(user: RootPanelUser): { tone: PillTone; label: string } | null {
    if (user.status === 'banned') return { tone: 'danger', label: 'Signed out and blocked' };
    if (user.status === 'removed') return { tone: 'danger', label: 'Access removed' };
    if (user.system_role === 'super_root') return { tone: 'warning', label: 'Lead operator' };
    if (user.system_role === 'root') return { tone: 'warning', label: 'Operator' };

    return null;
  }

  /** What a person has in the product, said the way they would say it. */
  function membership(user: RootPanelUser): string {
    const owned = user.owned_workspaces;
    const assigned = user.assigned_workspaces;
    if (owned > 0) {
      const said = `owns ${owned} ${owned === 1 ? 'workspace' : 'workspaces'}`;

      return assigned === 0 ? said : `${said}, member of ${assigned} more`;
    }
    if (assigned > 0) {
      return `member of ${assigned} ${assigned === 1 ? 'workspace' : 'workspaces'}`;
    }

    return 'no workspaces yet';
  }

  function userActions(user: RootPanelUser): ActionMenuItem[] {
    const items: ActionMenuItem[] = [];
    if (user.can_manage_system_role) {
      items.push(
        user.system_role === 'root'
          ? {
              id: 'demote_root',
              icon: 'shield-slash',
              label: 'Stop being an operator',
              description: 'Take away the console and every workspace it reaches',
              tone: 'danger',
            }
          : {
              id: 'promote_root',
              icon: 'admin',
              label: 'Make an operator',
              description: 'Give the console and audited access to every workspace',
            },
      );
    }
    if (!user.manageable || user.status === 'removed') return items;
    if (user.status === 'banned') {
      items.push({
        id: 'restore',
        icon: 'refresh',
        label: 'Restore account',
        description: 'Allow this account to sign in again',
      });
    } else {
      items.push({
        id: 'ban',
        icon: 'ban',
        label: 'Ban account',
        description: 'Revoke sessions and block sign-in',
        tone: 'danger',
      });
    }
    items.push({
      id: 'remove',
      icon: 'trash',
      label: 'Remove account',
      description: 'Revoke sessions, access, and invitations',
      tone: 'danger',
    });
    return items;
  }

  function chooseUserAction(
    user: RootPanelUser,
    action: string,
    trigger: HTMLElement | null,
  ): void {
    const chosen = userAction(action);
    if (chosen === null) return;
    actionTrigger = trigger;
    reason = '';
    actionProblem = null;
    dialogRoute.open(ACTION_DIALOG, { user: user.account.login, action: chosen });
  }

  function closeUserAction(): void {
    if (saving) return;
    if (dialogRoute.isOpen(ACTION_DIALOG)) dialogRoute.close();
    actionProblem = null;
  }

  function actionTitle(): string {
    const name = actionUser?.account.display_name ?? 'this account';
    if (pendingAction === 'promote_root') return `Make ${name} an operator?`;
    if (pendingAction === 'demote_root') return `${name} stops being an operator?`;
    if (pendingAction === 'restore') return `Restore ${name}?`;
    if (pendingAction === 'ban') return `Ban ${name}?`;
    return `Remove ${name}?`;
  }

  function actionDescription(): string {
    if (pendingAction === 'promote_root') {
      return 'An operator reads the whole service and may enter any workspace - every visit is announced and audited';
    }
    if (pendingAction === 'demote_root') {
      return 'What they own and what they were given stay exactly as they are';
    }
    if (pendingAction === 'restore') return 'The account can sign in again with retained access';
    if (pendingAction === 'ban') return 'Every active session is revoked immediately';
    return 'Sessions, assignments, and invitations are revoked. Audit identity is retained';
  }

  async function confirmUserAction(): Promise<void> {
    if (actionUser === null || pendingAction === null || saving) return;
    saving = true;
    actionProblem = null;
    const input: UpdateRootUserInput =
      pendingAction === 'promote_root' || pendingAction === 'demote_root'
        ? {
            system_role: pendingAction === 'promote_root' ? 'root' : 'none',
            expected_revision: actionUser.revision,
          }
        : {
            status:
              pendingAction === 'restore'
                ? 'active'
                : pendingAction === 'ban'
                  ? 'banned'
                  : 'removed',
            ...(reason.trim() === '' ? {} : { reason: reason.trim() }),
            expected_revision: actionUser.revision,
          };
    try {
      await updateUser(actionUser.account.id, input);
      if (dialogRoute.isOpen(ACTION_DIALOG)) dialogRoute.close();
      await loadPage(undefined, false);
    } catch (error) {
      actionProblem = error instanceof Error ? error.message : String(error);
    } finally {
      saving = false;
    }
  }
</script>

<!--
@component
Who may use the panel at all, which is the Root console's own question rather than any
workspace's. Users and invitations are two sections of one page because the answer to
"why can this person not get in" is in whichever of the two they are not in.

`canManageInvitations` draws the acts rather than disabling them. An operator who may
read the list but not change it sees the same records without controls that would
refuse.
-->

<section class="root-access" aria-labelledby="root-page-heading">
  <RootPageHeader
    title={section === 'users' ? 'Users' : 'Invitations'}
    subtitle={section === 'users'
      ? 'People with current, pending, or previous Smyklot access and their workspace memberships'
      : 'Invitations to operate the service, not a workspace. Every workspace visit is announced and audited'}
  >
    {#if section === 'invitations'}
      {#if canManageInvitations}
        <Button
          tone="signal"
          bind:element={inviteTrigger}
          onclick={() => invitations?.openCreate(inviteTrigger)}
        >
          {#snippet icon()}<Icon name="user-plus" size="sm" strokeWidth={2} />{/snippet}
          Invite an operator
        </Button>
      {/if}
    {:else}
      <Button tone="signal" bind:element={addTrigger} onclick={openAddUser}>
        {#snippet icon()}<Icon name="user-plus" size="sm" strokeWidth={2} />{/snippet}
        Add user
      </Button>
    {/if}
  </RootPageHeader>

  {#if section === 'invitations'}
    <RootInvitations
      bind:this={invitations}
      fetchPage={fetchInvitations}
      create={createInvitation}
      reissue={reissueInvitation}
      revoke={revokeInvitation}
      canManage={canManageInvitations}
      {actorLogin}
    />
  {:else}
    <div class="filter-bar">
      <SearchField
        label="Find a user"
        placeholder="Find a user"
        value={search}
        onInput={(value) => (search = value)}
      />
      <span class="push-end">
        <span class="stable-feedback" aria-live="polite">{feedback}</span>
        <ListToolsMenu
          label="Filter users"
          sorts={toolSorts}
          filters={[
            {
              label: 'Service role',
              hint: 'Filter what a person may do to the service itself',
              sections: ROLE_FILTERS,
              selected: systemRoles,
              multiple: true,
              onChange: selectRoles,
            },
            {
              label: 'Status',
              hint: 'Filter account lifecycle state',
              sections: STATUS_FILTERS,
              selected: statuses,
              multiple: true,
              onChange: selectStatuses,
            },
          ]}
        />
      </span>
    </div>

    <div class:loading class="user-results" aria-busy={loading}>
      <!-- A refresh that failed over a loaded table has not made the table wrong. -->
      {#if problem !== null && page !== null}
        <ResultProblem
          title="The people could not be read"
          {problem}
          busy={loading}
          onRetry={() => void loadPage(undefined, false)}
          overContent
        />
      {/if}

      {#if problem !== null && page === null}
        <ResultProblem
          title="The people could not be read"
          {problem}
          busy={loading}
          onRetry={() => void loadPage(undefined, false)}
        />
      {:else if loading && page === null}
        <Skeleton bars={false} --skeleton-min-height="10rem" />
      {:else}
        <Card>
          {#if users.length === 0}
            <div class="state-panel">
              {#if hasFilters}
                <span
                  ><strong>Nothing matches.</strong> No account here answers to what is being asked</span
                >
                <Button onclick={clearFilters}>Clear the filters</Button>
              {:else}
                <span
                  ><strong>Nobody yet.</strong> An account appears here the first time somebody signs
                  in</span
                >
              {/if}
            </div>
          {:else}
            <ul class="object-list">
              {#each users as user (user.account.id)}
                {@const standing = userStanding(user)}
                <li>
                  <div class="object-row">
                    <span class="object-main">
                      <span class="object-name-row">
                        <span class="object-name">{user.account.display_name}</span>
                        {#if standing !== null}
                          <Pill tone={standing.tone}>{standing.label}</Pill>
                        {/if}
                      </span>
                      <!-- The handle, what they have in the product, and when they were
                           last here: three columns became the sentence they were always
                           read as. -->
                      <span class="object-sum"
                        >@{user.account.login} · {membership(user)} ·
                        {#if user.last_login_at !== undefined}signed in
                          <RelativeTime value={user.last_login_at} nowMs={now} />
                        {:else}has not signed in yet{/if}</span
                      >
                    </span>
                    <span class="object-side">
                      {#if userActions(user).length > 0}
                        <ActionMenu
                          label={`Actions for @${user.account.login}`}
                          items={userActions(user)}
                          onSelect={(action, trigger) => chooseUserAction(user, action, trigger)}
                        />
                      {/if}
                    </span>
                  </div>
                </li>
              {/each}
            </ul>
            <div class="list-foot">
              <span
                >Showing 1-{users.length} of {page?.total ?? users.length}{hasFilters
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
          <span>{loadMoreProblem}</span><Button onclick={loadNext}>Try again</Button>
        </div>
      {/if}
    </div>
  {/if}
</section>

<ConfirmDialog
  id={ACTION_DIALOG}
  open={actionUser !== null && pendingAction !== null}
  title={actionTitle()}
  description={actionDescription()}
  returnFocus={actionTrigger}
  onClose={closeUserAction}
  onConfirm={() => void confirmUserAction()}
  {confirmTone}
  busy={saving}
>
  {#if pendingAction === 'ban' || pendingAction === 'remove'}
    <label class="reason-field">
      <span>Reason <small>Optional</small></span>
      <textarea
        placeholder="Add context to the immutable audit record"
        maxlength="500"
        rows="4"
        bind:value={reason}></textarea>
      <small>{reason.length}/500 characters</small>
    </label>
  {:else}
    <Callout tabindex={-1}>
      {#snippet icon()}
        <Icon name={pendingAction === 'promote_root' ? 'warning' : 'info'} size="md" />
      {/snippet}
      <span>Review the account and effect before confirming</span>
    </Callout>
  {/if}
  {#if actionProblem !== null}<p class="action-error" role="alert">{actionProblem}</p>{/if}
</ConfirmDialog>

<Modal
  id={ADD_DIALOG}
  open={addOpen}
  title="Add someone to a workspace"
  description="Choose the workspace before assigning a Viewer, Editor, or Admin role"
  returnFocus={addTrigger}
  onClose={closeAddUser}
>
  <form
    id="root-add-workspace-user-form"
    class="add-user-form"
    onsubmit={(event) => {
      event.preventDefault();
      void submitAddUser();
    }}
  >
    <!-- Completed against the chosen workspace's organization, which is why
         the field reads the selection rather than owning a roster of its own.
         Before one is chosen there is nothing to complete against, and the field
         is what it always was. -->
    <LoginField
      id="root-add-workspace-login"
      label="GitHub login"
      bind:value={addLogin}
      focusOnOpen
      suggest={(query) =>
        selectedWorkspace === null
          ? Promise.resolve([])
          : suggestUsers(selectedWorkspace.id, query)}
    />

    <fieldset class="workspace-fieldset">
      <legend>Workspace</legend>
      <SearchField
        label="Find a workspace"
        placeholder="Find a workspace"
        value={workspaceQuery}
        onInput={(value) => (workspaceQuery = value)}
      />
      {#if workspacesLoading}
        <div class="workspace-state" role="status">Reading the workspaces…</div>
      {:else if workspacesProblem !== null}
        <div class="workspace-state workspace-problem" role="alert">
          <span>{workspacesProblem}</span>
          <Button tone="quiet" onclick={() => void workspacesQuery.refetch()}>Try again</Button>
        </div>
      {:else if filteredWorkspaces.length === 0}
        <div class="workspace-state">No workspace matches this search</div>
      {:else}
        <div class="workspace-options" role="radiogroup" aria-label="Workspace">
          {#each filteredWorkspaces as workspace (workspace.id)}
            <label class:unavailable={!workspace.available}>
              <input
                type="radio"
                name="root-workspace"
                value={workspace.id}
                disabled={!workspace.available}
                checked={effectiveWorkspaceID === workspace.id}
                onchange={() => (selectedWorkspaceID = workspace.id)}
              />
              <span class="workspace-option-copy">
                <strong>{workspace.account.display_name}</strong>
                <span class="mono">@{workspace.account.login}</span>
              </span>
              <Chip
                tone={workspace.owned_by_viewer
                  ? 'clear'
                  : workspace.available
                    ? 'warning'
                    : 'stop'}
              >
                {workspace.owned_by_viewer
                  ? 'Owned'
                  : workspace.available
                    ? 'Needs an operator visit'
                    : 'Unavailable'}
              </Chip>
            </label>
          {/each}
        </div>
      {/if}
    </fieldset>

    <label>
      <span>Role in that workspace</span>
      <Select
        bind:value={addRole}
        options={[
          { value: 'viewer', label: 'Viewer' },
          { value: 'editor', label: 'Editor' },
          { value: 'admin', label: 'Admin' },
        ]}
      />
    </label>

    {#if selectedWorkspace !== null && !selectedWorkspace.owned_by_viewer}
      <Callout tone="warning">
        {#snippet icon()}<Icon name="warning" size="md" />{/snippet}
        <span>
          This workspace is not yours. Continue to its Access view to acknowledge and start the
          audited 15-minute elevation before adding the user.
        </span>
      </Callout>
    {/if}
    {#if addProblem !== null}<p class="action-error" role="alert">{addProblem}</p>{/if}
  </form>

  {#snippet footer()}
    <Button tone="ghost" disabled={addSaving} onclick={closeAddUser}>Cancel</Button>
    <Button
      tone="signal"
      type="submit"
      form="root-add-workspace-user-form"
      disabled={addSaving ||
        selectedWorkspace === null ||
        !selectedWorkspace.available ||
        (selectedWorkspace.owned_by_viewer && addLogin.trim() === '')}
    >
      {addSaving
        ? 'Adding…'
        : selectedWorkspace?.owned_by_viewer === false
          ? 'Open audited access'
          : 'Add user'}
    </Button>
  {/snippet}
</Modal>

<style>
  .root-access {
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

  .stable-feedback {
    color: var(--text-secondary);
    flex: none;
    font-size: var(--font-size-meta);
    max-width: 18rem;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Collapses entirely when there is no message, so no gap is reserved. */
  .stable-feedback:empty {
    display: none;
  }

  .add-user-form,
  .add-user-form > label {
    display: grid;
    gap: var(--space-2);
  }

  .add-user-form {
    gap: var(--space-4);
  }

  .add-user-form > label > span,
  .workspace-fieldset legend {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    font-weight: 650;
  }

  .workspace-fieldset {
    border: 0;
    display: grid;
    gap: var(--space-2);
    margin: 0;
    min-width: 0;
    padding: 0;
  }

  .workspace-fieldset legend {
    margin-bottom: var(--space-2);
  }

  .workspace-options {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    display: grid;
    max-height: 14rem;
    overflow-y: auto;
  }

  .workspace-options > label {
    align-items: center;
    border-bottom: 1px solid var(--border-subtle);
    cursor: pointer;
    display: grid;
    gap: var(--space-3);
    grid-template-columns: auto minmax(0, 1fr) auto;
    min-height: 3.5rem;
    padding: var(--space-2) var(--space-3);
  }

  .workspace-options > label:last-child {
    border-bottom: 0;
  }

  .workspace-options > label:hover:not(.unavailable) {
    background: var(--interactive-hover);
  }

  .workspace-options > label:has(input:checked) {
    background: var(--brand-action-tint);
  }

  .workspace-options > label.unavailable {
    cursor: not-allowed;
    opacity: 0.64;
  }

  .workspace-option-copy {
    display: grid;
    min-width: 0;
  }

  .workspace-option-copy strong,
  .workspace-option-copy span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .workspace-option-copy span,
  .workspace-state {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .workspace-state {
    border: 1px dashed var(--border-subtle);
    border-radius: var(--radius-control);
    padding: var(--space-4);
    text-align: center;
  }

  .user-results {
    min-height: 8rem;
  }

  .reason-field {
    display: grid;
    gap: var(--space-2);
  }

  .reason-field > span {
    font-weight: 650;
  }

  .reason-field small {
    color: var(--text-muted);
    font-size: var(--font-size-meta);
  }

  .reason-field textarea {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--radius-control);
    color: var(--text-primary);
    font: inherit;
    padding: var(--space-3);
    resize: vertical;
  }

  .action-error {
    color: var(--danger);
    font-size: var(--font-size-meta);
    margin: var(--space-3) 0 0;
  }
</style>
