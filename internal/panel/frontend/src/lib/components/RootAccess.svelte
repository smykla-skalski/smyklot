<script lang="ts">
  import { createInfiniteQuery, createQuery, type InfiniteData } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import { useDebounce, useInterval } from 'runed';

  import { dialogRoute } from '../dialog-route.svelte';
  import { formatRelative, formatTimestamp } from '../format';
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
    RootInstallation,
    InstallationRole,
    SystemRole,
    UpdateRootUserInput,
  } from '../types';
  import ActionMenu, { type ActionMenuItem } from './ActionMenu.svelte';
  import Button, { type ButtonTone } from './Button.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import DataTable from './DataTable.svelte';
  import Select from './Select.svelte';
  import Callout from './Callout.svelte';
  import IdentityRow from './IdentityRow.svelte';
  import Skeleton from './Skeleton.svelte';
  import SortIndicator from './SortIndicator.svelte';
  import Avatar from './Avatar.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import Icon from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import LoginField from './LoginField.svelte';
  import Modal from './Modal.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import RootInvitations from './RootInvitations.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import SearchField from './SearchField.svelte';
  import TableToolsMenu from './TableToolsMenu.svelte';
  import SectionTabs from './SectionTabs.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  type AccessSection = 'users' | 'invitations';
  type SortColumn = 'name' | 'role' | 'last_login';
  type UserAction = (typeof USER_ACTIONS)[number];

  const USER_ACTIONS = ['promote_root', 'demote_root', 'restore', 'ban', 'remove'] as const;

  /** Name the dialogs in the address, and are the `id` each dialog carries. */
  const ACTION_DIALOG = 'root-user-action';
  const ADD_DIALOG = 'root-add-installation-user';

  /* Two lists, which are two addresses: tabs rather than a segmented control,
     which changes what is on screen and saves nothing. */
  const SECTIONS = [
    { id: 'users', label: 'Users' },
    { id: 'invitations', label: 'Invitations' },
  ] as const;
  const ROLE_FILTERS = [
    {
      options: [
        { value: 'super_root', label: 'Super Root' },
        { value: 'root', label: 'Root' },
        { value: 'none', label: 'Regular account' },
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
    rootRole,
    section,
    onSection,
    sectionHref,
    fetchUsers,
    updateUser,
    fetchInvitations,
    createInvitation,
    reissueInvitation,
    revokeInvitation,
    canManageInvitations,
    actorLogin,
    fetchInstallations,
    addInstallationUser,
    suggestUsers,
    onOpenInstallationAccess,
  }: {
    rootRole: string;
    section: AccessSection;
    onSection: (section: AccessSection) => void;
    /** Where each list lives; the strip is a strip of addresses. */
    sectionHref?: (section: AccessSection) => string;
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
    fetchInstallations: () => Promise<RootInstallation[]>;
    addInstallationUser: (targetId: string, input: AddTargetUserInput) => Promise<PanelUser>;
    /** Completes a login against the chosen installation's organization. */
    suggestUsers: (targetId: string, query: string) => Promise<PanelAccount[]>;
    onOpenInstallationAccess: (account: string) => void;
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
  let addRole = $state<Exclude<InstallationRole, 'none' | 'owner'>>('viewer');
  let installationQuery = $state('');
  let selectedInstallationID = $state('');
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
  const installationsQuery = createQuery(() => ({
    queryKey: ['root-installations'],
    queryFn: fetchInstallations,
    enabled: addOpen,
  }));
  const installations = $derived<RootInstallation[]>(installationsQuery.data ?? []);
  const installationsLoading = $derived(
    addOpen && installationsQuery.isFetching && installationsQuery.data === undefined,
  );
  const installationsProblem = $derived(
    addOpen && installationsQuery.error !== null ? errorMessage(installationsQuery.error) : null,
  );
  const defaultInstallationID = $derived(
    installations.find((installation) => installation.available)?.id ?? '',
  );
  const effectiveInstallationID = $derived(
    selectedInstallationID === '' ? defaultInstallationID : selectedInstallationID,
  );

  function userAction(value: string | undefined): UserAction | null {
    return USER_ACTIONS.find((action) => action === value) ?? null;
  }
  const selectedInstallation = $derived(
    installations.find((installation) => installation.id === effectiveInstallationID) ?? null,
  );
  const filteredInstallations = $derived.by(() => {
    const needle = installationQuery.trim().toLocaleLowerCase();
    if (needle === '') return installations;
    return installations.filter((installation) =>
      [installation.account.display_name, installation.account.login].some((value) =>
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

  function selectSection(value: string): void {
    if (value === 'users' || value === 'invitations') onSection(value);
  }

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

  function loadFromScroll(event: Event): void {
    const target = event.currentTarget as HTMLElement;
    if (target.scrollHeight - target.scrollTop - target.clientHeight < 260) loadNext();
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
    installationQuery = '';
    selectedInstallationID = '';
    addProblem = null;
  }

  function closeAddUser(): void {
    if (addSaving) return;
    if (dialogRoute.isOpen(ADD_DIALOG)) dialogRoute.close();
    addProblem = null;
  }

  async function submitAddUser(): Promise<void> {
    const installation = selectedInstallation;
    if (installation === null || !installation.available) return;
    if (!installation.owned_by_viewer) {
      if (dialogRoute.isOpen(ADD_DIALOG)) dialogRoute.close();
      onOpenInstallationAccess(installation.account.login);
      return;
    }
    const login = addLogin.trim();
    if (login === '') return;
    addSaving = true;
    addProblem = null;
    try {
      await addInstallationUser(installation.id, { login, role: addRole });
      feedback = `Added @${login} to ${installation.account.display_name}`;
      if (dialogRoute.isOpen(ADD_DIALOG)) dialogRoute.close();
      await loadPage(undefined, false);
    } catch (error) {
      addProblem = error instanceof Error ? error.message : String(error);
    } finally {
      addSaving = false;
    }
  }

  function systemRoleLabel(role: SystemRole): string {
    if (role === 'super_root') return 'Super Root';
    if (role === 'root') return 'Root';
    return 'Account';
  }

  function systemRoleTone(role: SystemRole): ChipTone {
    if (role === 'super_root') return 'accent';
    if (role === 'root') return 'signal';
    return 'neutral';
  }

  function statusLabel(status: PanelUserStatus): string {
    return status.charAt(0).toLocaleUpperCase() + status.slice(1);
  }

  function statusTone(status: PanelUserStatus): ChipTone {
    if (status === 'active') return 'clear';
    if (status === 'banned') return 'stop';
    return 'neutral';
  }

  function installationSummary(user: RootPanelUser): string {
    const relationships = user.owned_installations + user.assigned_installations;
    return `${relationships} installation${relationships === 1 ? '' : 's'}`;
  }

  function userActions(user: RootPanelUser): ActionMenuItem[] {
    const items: ActionMenuItem[] = [];
    if (user.can_manage_system_role) {
      items.push(
        user.system_role === 'root'
          ? {
              id: 'demote_root',
              icon: 'shield-slash',
              label: 'Remove Root role',
              description: 'Remove application-wide administration',
              tone: 'danger',
            }
          : {
              id: 'promote_root',
              icon: 'admin',
              label: 'Make Root',
              description: 'Grant application-wide administration',
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
    if (pendingAction === 'promote_root') return `Make ${name} a Root?`;
    if (pendingAction === 'demote_root') return `Remove Root access from ${name}?`;
    if (pendingAction === 'restore') return `Restore ${name}?`;
    if (pendingAction === 'ban') return `Ban ${name}?`;
    return `Remove ${name}?`;
  }

  function actionDescription(): string {
    if (pendingAction === 'promote_root') {
      return 'Root can read application-wide data and use audited installation elevation.';
    }
    if (pendingAction === 'demote_root') {
      return 'Their installation ownership and explicit assignments remain unchanged.';
    }
    if (pendingAction === 'restore') return 'The account can sign in again with retained access.';
    if (pendingAction === 'ban') return 'Every active session is revoked immediately.';
    return 'Sessions, assignments, and invitations are revoked. Audit identity is retained.';
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

{#snippet sectionSwitch()}
  <SectionTabs
    items={SECTIONS.map((one) => ({ ...one, href: sectionHref?.(one.id) ?? '#' }))}
    active={section}
    label="Root access lists"
    onNavigate={selectSection}
  />
{/snippet}

<section class="root-access" aria-labelledby="root-page-heading">
  <RootPageHeader
    role={rootRole}
    title="Access"
    subtitle={section === 'users'
      ? 'Every account known to Smyklot'
      : 'Pending system-level access'}
  >
    {#if section === 'invitations'}
      {#if canManageInvitations}
        <Button
          tone="signal"
          bind:element={inviteTrigger}
          onclick={() => invitations?.openCreate(inviteTrigger)}
        >
          {#snippet icon()}<Icon name="user-plus" size={14} strokeWidth={2} />{/snippet}
          Invite Root user
        </Button>
      {/if}
    {:else}
      <Button tone="signal" bind:element={addTrigger} onclick={openAddUser}>
        {#snippet icon()}<Icon name="user-plus" size={14} strokeWidth={2} />{/snippet}
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
      navigation={sectionSwitch}
    />
  {:else}
    <div class="access-toolbar">
      {@render sectionSwitch()}
      <span class="stable-feedback" aria-live="polite">{feedback}</span>
      <SearchField
        label="Search Root users"
        placeholder="Search users"
        value={search}
        onInput={(value) => (search = value)}
      />
      <!-- Both filters live in column headings, and the heading band is hidden
           once this table becomes a stack of cards. Without this the page
           offered a search field and nothing else. -->
      <TableToolsMenu
        label="Filter Root users"
        sorts={[]}
        filters={[
          {
            label: 'System role',
            hint: 'Filter application-level privileges',
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
    </div>

    <div class:loading class="user-results table-region" aria-busy={loading}>
      <!-- A refresh that failed over a loaded table has not made the table wrong. -->
      {#if problem !== null && page !== null}
        <ResultProblem
          title="Root users could not be loaded"
          {problem}
          busy={loading}
          onRetry={() => void loadPage(undefined, false)}
          overContent
        />
      {/if}

      {#if problem !== null && page === null}
        <ResultProblem
          title="Root users could not be loaded"
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
          caption="Application accounts"
          regionLabel="Root users table"
          rows={users}
          rowKey={(user) => String(user.account.id)}
          columnCount={6}
          onBodyScroll={loadFromScroll}
        >
          {#snippet head()}
            <tr>
              <th scope="col" aria-sort={sortDirection('name')}>
                <div class="table-heading">
                  <button
                    class="table-sort-button"
                    type="button"
                    onclick={() => toggleSort('name')}
                  >
                    <span class="table-heading-label">User</span><SortIndicator />
                  </button>
                </div>
              </th>
              <th scope="col" aria-sort={sortDirection('role')}>
                <div class="table-heading">
                  <button
                    class="table-sort-button"
                    type="button"
                    onclick={() => toggleSort('role')}
                  >
                    <span class="table-heading-label">System role</span><SortIndicator />
                  </button>
                  <FilterMenu
                    label="System role"
                    summary={systemRoles.length === 0
                      ? 'All system roles'
                      : `${systemRoles.length} selected`}
                    hint="Filter application-level privileges"
                    sections={ROLE_FILTERS}
                    selected={systemRoles}
                    multiple
                    align="end"
                    onChange={selectRoles}
                  />
                </div>
              </th>
              <th scope="col">
                <div class="table-heading">
                  <span class="table-heading-label">Status</span>
                  <FilterMenu
                    label="Status"
                    summary={statuses.length === 0 ? 'All statuses' : `${statuses.length} selected`}
                    hint="Filter account lifecycle state"
                    sections={STATUS_FILTERS}
                    selected={statuses}
                    multiple
                    align="end"
                    onChange={selectStatuses}
                  />
                </div>
              </th>
              <th scope="col">
                <div class="table-heading">
                  <span class="table-heading-label">Installations</span>
                </div>
              </th>
              <th scope="col" aria-sort={sortDirection('last_login')}>
                <div class="table-heading">
                  <button
                    class="table-sort-button"
                    type="button"
                    onclick={() => toggleSort('last_login')}
                  >
                    <span class="table-heading-label">Last login</span><SortIndicator />
                  </button>
                </div>
              </th>
              <th scope="col"><span class="visually-hidden">Actions</span></th>
            </tr>
          {/snippet}
          {#snippet cells(user)}
            <td data-label="User">
              <IdentityRow>
                {#snippet mark()}<Avatar account={user.account} size={32} />{/snippet}
                {#snippet name()}<strong>{user.account.display_name}</strong>{/snippet}
                {#snippet handle()}<span class="mono">@{user.account.login}</span>{/snippet}
              </IdentityRow>
            </td>
            <td data-label="System role">
              <Chip tone={systemRoleTone(user.system_role)}
                >{systemRoleLabel(user.system_role)}</Chip
              >
            </td>
            <td data-label="Status">
              <Chip tone={statusTone(user.status)} dot={user.status === 'active'}
                >{statusLabel(user.status)}</Chip
              >
            </td>
            <td class="band-trim-stack" data-label="Installations">
              <span class="relationship-count">{installationSummary(user)}</span>
              <span class="relationship-meta"
                >{user.owned_installations} owned · {user.assigned_installations} assigned</span
              >
            </td>
            <td data-label="Last login">
              {#if user.last_login_at !== undefined}
                <time
                  class="band-trim"
                  datetime={user.last_login_at}
                  title={formatTimestamp(user.last_login_at)}
                  >{formatRelative(user.last_login_at, now)}</time
                >
              {:else}<span class="dim band-trim">Never</span>{/if}
            </td>
            <td class="row-actions" data-label="Actions">
              {#if userActions(user).length > 0}
                <ActionMenu
                  label={`Actions for @${user.account.login}`}
                  items={userActions(user)}
                  onSelect={(action, trigger) => chooseUserAction(user, action, trigger)}
                />
              {/if}
            </td>
          {/snippet}
          {#snippet empty()}
            <TableEmptyState
              title="No accounts found"
              description={hasFilters
                ? 'Try another search or clear the active filters'
                : 'Accounts appear after their first authenticated session'}
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
        <Icon name={pendingAction === 'promote_root' ? 'warning' : 'info'} size={20} />
      {/snippet}
      <span>Review the account and effect before confirming.</span>
    </Callout>
  {/if}
  {#if actionProblem !== null}<p class="action-error" role="alert">{actionProblem}</p>{/if}
</ConfirmDialog>

<Modal
  id={ADD_DIALOG}
  open={addOpen}
  title="Add installation user"
  description="Choose the installation before assigning a Viewer, Editor, or Admin role."
  returnFocus={addTrigger}
  onClose={closeAddUser}
>
  <form
    id="root-add-installation-user-form"
    class="add-user-form"
    onsubmit={(event) => {
      event.preventDefault();
      void submitAddUser();
    }}
  >
    <!-- Completed against the chosen installation's organization, which is why
         the field reads the selection rather than owning a roster of its own.
         Before one is chosen there is nothing to complete against, and the field
         is what it always was. -->
    <LoginField
      id="root-add-installation-login"
      label="GitHub login"
      bind:value={addLogin}
      focusOnOpen
      suggest={(query) =>
        selectedInstallation === null
          ? Promise.resolve([])
          : suggestUsers(selectedInstallation.id, query)}
    />

    <fieldset class="installation-fieldset">
      <legend>Installation</legend>
      <SearchField
        label="Filter installations"
        placeholder="Find an installation"
        value={installationQuery}
        onInput={(value) => (installationQuery = value)}
      />
      {#if installationsLoading}
        <div class="installation-state" role="status">Loading installations…</div>
      {:else if installationsProblem !== null}
        <div class="installation-state installation-problem" role="alert">
          <span>{installationsProblem}</span>
          <Button tone="quiet" onclick={() => void installationsQuery.refetch()}>Try again</Button>
        </div>
      {:else if filteredInstallations.length === 0}
        <div class="installation-state">No installations match this search.</div>
      {:else}
        <div class="installation-options" role="radiogroup" aria-label="Installation">
          {#each filteredInstallations as installation (installation.id)}
            <label class:unavailable={!installation.available}>
              <input
                type="radio"
                name="root-installation"
                value={installation.id}
                disabled={!installation.available}
                checked={effectiveInstallationID === installation.id}
                onchange={() => (selectedInstallationID = installation.id)}
              />
              <span class="installation-option-copy">
                <strong>{installation.account.display_name}</strong>
                <span class="mono">@{installation.account.login}</span>
              </span>
              <Chip
                tone={installation.owned_by_viewer
                  ? 'clear'
                  : installation.available
                    ? 'warning'
                    : 'stop'}
              >
                {installation.owned_by_viewer
                  ? 'Owned'
                  : installation.available
                    ? 'Elevation required'
                    : 'Unavailable'}
              </Chip>
            </label>
          {/each}
        </div>
      {/if}
    </fieldset>

    <label>
      <span>Installation role</span>
      <Select
        bind:value={addRole}
        options={[
          { value: 'viewer', label: 'Viewer' },
          { value: 'editor', label: 'Editor' },
          { value: 'admin', label: 'Admin' },
        ]}
      />
    </label>

    {#if selectedInstallation !== null && !selectedInstallation.owned_by_viewer}
      <Callout tone="warning">
        {#snippet icon()}<Icon name="warning" size={18} />{/snippet}
        <span>
          This installation is not yours. Continue to its Access view to acknowledge and start the
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
      form="root-add-installation-user-form"
      disabled={addSaving ||
        selectedInstallation === null ||
        !selectedInstallation.available ||
        (selectedInstallation.owned_by_viewer && addLogin.trim() === '')}
    >
      {addSaving
        ? 'Adding…'
        : selectedInstallation?.owned_by_viewer === false
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

  .access-toolbar {
    /* One 34px row: the section switch leads, the search fills the rest. The
       primary action lives in the header slot, same anatomy as the mock. */
    --control-height: var(--control-height-compact);

    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    min-height: var(--control-height);
    padding-bottom: var(--space-3);
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
  .installation-fieldset legend {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    font-weight: 650;
  }

  .installation-fieldset {
    border: 0;
    display: grid;
    gap: var(--space-2);
    margin: 0;
    min-width: 0;
    padding: 0;
  }

  .installation-fieldset legend {
    margin-bottom: var(--space-2);
  }

  .installation-options {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    display: grid;
    max-height: 14rem;
    overflow-y: auto;
  }

  .installation-options > label {
    align-items: center;
    border-bottom: 1px solid var(--rule);
    cursor: pointer;
    display: grid;
    gap: var(--space-3);
    grid-template-columns: auto minmax(0, 1fr) auto;
    min-height: 3.5rem;
    padding: var(--space-2) var(--space-3);
  }

  .installation-options > label:last-child {
    border-bottom: 0;
  }

  .installation-options > label:hover:not(.unavailable) {
    background: var(--interactive-hover);
  }

  .installation-options > label:has(input:checked) {
    background: var(--brand-action-tint);
  }

  .installation-options > label.unavailable {
    cursor: not-allowed;
    opacity: 0.64;
  }

  .installation-option-copy {
    display: grid;
    min-width: 0;
  }

  .installation-option-copy strong,
  .installation-option-copy span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .installation-option-copy span,
  .installation-state {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .installation-state {
    border: 1px dashed var(--border-subtle);
    border-radius: var(--radius-control);
    padding: var(--space-4);
    text-align: center;
  }

  /* Layout, keyline and corner come from `.table-region` in `app.css`. */
  .user-results {
    min-height: 8rem;
  }

  /* Surface, keyline, corner and lift come from `.table-card`; the scroll shell,
     the cell padding and the separator from `DataTable` and `.data-table`. What
     is left is this table's own settings for them.

     `--cell-pad-block` is still named once, because the row height below is
     derived from it and a padding changed in one place and not the other would
     silently un-state the row height. */
  :global(.table-scroll) {
    --cell-pad-block: 0.625rem;
    --table-cell-pad-block: var(--cell-pad-block);
    --table-empty-height: 10rem;
    --table-cell-pad-inline: 0.75rem;
    --table-heading-height: 2.5rem;
    --table-layout: fixed;
    --table-min-width: 46rem;

    flex: 1;
    max-width: 100%;
    min-height: 0;
  }

  /* Stated, not inherited from whatever the tallest cell happens to hold.
     It used to come out at 61px because the row menu is 40px tall, and at 60.9px
     on the viewer's own row - which has no menu, since nobody may act on
     themselves - where two lines of untrimmed leading happened to measure the
     same. Trimming those lines to their band, which is what centres them, took
     that row to 54px and left one short row in the middle of the table. A row's
     height is a decision: the tallest control it has to hold, plus its own
     padding and rule. */
  /* Stated, not inherited from whatever the tallest cell happens to hold. It used
     to come out at 61px because the row menu is 40px tall, and at 60.9px on the
     viewer's own row - which has no menu, since nobody may act on themselves -
     where two lines of untrimmed leading happened to measure the same. Trimming
     those lines to their band took that row to 54px and left one short row in the
     middle of the table. A row's height is a decision: the tallest control it has
     to hold, plus its own padding and rule. */
  :global(.table-scroll tbody tr) {
    height: calc(var(--control-height) + 2 * var(--cell-pad-block) + 1px);
  }

  /* The first column's wider inset, on both halves of the table so the band and
     the rows below it start on the same edge. */
  :global(.table-scroll td:first-child) {
    padding-left: var(--space-4);
  }

  :global(.table-scroll thead th:first-child) {
    --heading-pad-start: var(--space-4);
  }

  th:first-child,
  td:first-child {
    width: 28%;
  }

  th:nth-child(2),
  td:nth-child(2) {
    width: 16%;
  }

  th:nth-child(3),
  td:nth-child(3) {
    width: 13%;
  }

  th:nth-child(4),
  td:nth-child(4) {
    width: 24%;
  }

  th:nth-child(5),
  td:nth-child(5) {
    text-align: right;
    width: 16%;
  }

  /* An end-aligned column: the words meet the same edge the values below them do,
     and the arrow leads rather than trails, which is what keeps it off that edge.
     Every design system states this rule the same way - a heading follows its
     column's alignment, and the sort mark moves to the other side to let it. */
  th:nth-child(5) .table-sort-button {
    flex-direction: row-reverse;
    justify-content: flex-start;
  }

  th:last-child,
  td:last-child {
    text-align: center;
    width: 3rem;
  }

  /* The heading's row, its button and its arrow are shared - see `.table-heading`,
     `.table-sort-button` and `.sort-indicator` in `app.css`. What was here was a
     second copy of the button's reset, a fourth copy of the arrow's rules written
     against a raw `<svg>`, and a `:global(.header-filter)` addressed to a class
     the popover stopped rendering. */

  .relationship-meta,
  time {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
    line-height: 1.25;
  }

  .relationship-count,
  .relationship-meta {
    display: block;
  }

  .relationship-count {
    font-size: var(--font-size-body);
  }

  .relationship-meta {
    margin-top: 0.15rem;
  }

  time {
    white-space: nowrap;
  }

  .row-actions {
    padding-inline: var(--space-1);
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
    color: var(--text);
    font: inherit;
    padding: var(--space-3);
    resize: vertical;
  }

  .action-error {
    color: var(--danger);
    font-size: var(--font-size-meta);
    margin: var(--space-3) 0 0;
  }

  /* Only where the column headings are not: they carry the same two filters while
     the table is a table. */
  .access-toolbar :global(.tools-trigger) {
    display: none;
  }

  @media (max-width: 64rem) {
    .access-toolbar :global(.tools-trigger) {
      display: inline-flex;
    }
  }

  @media (prefers-reduced-motion: reduce) {
  }
</style>
