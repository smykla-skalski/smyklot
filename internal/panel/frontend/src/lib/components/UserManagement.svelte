<script lang="ts">
  import {
    columnFilteringFeature,
    createColumnHelper,
    createTable,
    filterFn_includesString,
    rowSortingFeature,
    tableFeatures,
  } from '@tanstack/svelte-table';
  import type { ColumnFiltersState, SortingState, Updater } from '@tanstack/svelte-table';
  import { createVirtualizer } from '@tanstack/svelte-virtual';
  import { createInfiniteQuery, type InfiniteData } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import { MediaQuery } from 'svelte/reactivity';
  import { get } from 'svelte/store';
  import { useDebounce, useInterval } from 'runed';

  import { PanelApiError } from '../api';
  import { dialogRoute } from '../dialog-route.svelte';
  import { formatDateTime, formatRelative, formatTimestamp, formatUntil } from '../format';
  import { monogram } from '../identity';
  import type { FilterSection } from '../filter-menu';
  import {
    EPHEMERAL_PREFS,
    prefList,
    prefOption,
    prefText,
    type PrefsAccessor,
  } from '../preferences-sync';
  import type {
    AccessDecision,
    AddTargetInvitationInput,
    AddTargetUserInput,
    InvitationDays,
    InvitationPageRequest,
    InvitationSort,
    InvitationStatus,
    Page,
    PanelAccount,
    PanelInvitation,
    InstallationRole,
    PanelUser,
    PanelUserListStatus,
    PanelUserPageRequest,
    PanelUserSort,
    UpdateTargetUserInput,
  } from '../types';
  import ActionMenu, { type ActionMenuItem } from './ActionMenu.svelte';
  import Avatar from './Avatar.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import CopyReceipt from './CopyReceipt.svelte';
  import DecisionHistory from './DecisionHistory.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import Icon, { type IconName } from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import LoginField from './LoginField.svelte';
  import Modal from './Modal.svelte';
  import PanelHeader from './PanelHeader.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import RolePicker, { type RolePickerOption } from './RolePicker.svelte';
  import SearchField from './SearchField.svelte';
  import NavigationTabs from './NavigationTabs.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  type ManagementSection = 'users' | 'invitations';
  type SortDirection = 'ascending' | 'descending';
  type UserSortColumn = 'name' | 'role' | 'last_login';
  type InvitationSortColumn = 'name' | 'role' | 'expires';
  /** Name the dialogs in the address, and are the `id` each dialog carries. */
  const ADD_DIALOG = 'add-user';
  const ACTION_DIALOG = 'user-action';
  const INVITATION_DIALOG = 'invitation-action';
  const HISTORY_DIALOG = 'decision-history';

  type UserAction = 'suspend' | 'restore' | 'remove_access';
  type TargetRole = Exclude<InstallationRole, 'owner'>;
  type GrantedTargetRole = Exclude<TargetRole, 'none'>;

  const ACCESS_METHODS = [
    { value: 'add', label: 'Add directly', description: 'Grant access immediately' },
    { value: 'invite', label: 'Send invitation', description: 'Create a single-use link' },
  ] as const;

  const ROLE_FILTERS: FilterSection[] = [
    {
      label: 'Roles',
      options: [
        { value: 'admin', label: 'Admin' },
        { value: 'editor', label: 'Editor' },
        { value: 'viewer', label: 'Viewer' },
        { value: 'none', label: 'No access' },
      ],
    },
  ];

  const INVITATION_ROLE_FILTERS: FilterSection[] = [
    {
      label: 'Roles',
      options: [
        { value: 'admin', label: 'Admin' },
        { value: 'editor', label: 'Editor' },
        { value: 'viewer', label: 'Viewer' },
      ],
    },
  ];

  const INVITATION_STATUS_FILTERS: FilterSection[] = [
    {
      label: 'Status',
      options: [
        { value: 'pending', label: 'Pending', tone: 'default' },
        { value: 'accepted', label: 'Accepted', tone: 'valid' },
        { value: 'expired', label: 'Expired', tone: 'bypassed' },
        { value: 'declined', label: 'Declined', tone: 'default' },
        { value: 'revoked', label: 'Revoked', tone: 'invalid' },
      ],
    },
  ];
  const ACCESS_TABLE_FEATURES = tableFeatures({
    columnFilteringFeature,
    filterFns: { includesString: filterFn_includesString },
    rowSortingFeature,
  });
  const userColumn = createColumnHelper<typeof ACCESS_TABLE_FEATURES, PanelUser>();
  const USER_COLUMNS = userColumn.columns([
    userColumn.accessor((user) => user.account.display_name, {
      id: 'name',
      enableColumnFilter: false,
    }),
    userColumn.accessor((user) => user.target_access?.effective_role ?? 'none', {
      id: 'role',
    }),
    userColumn.accessor('status', { id: 'status', enableSorting: false }),
    userColumn.accessor('last_login_at', { id: 'last_login', enableColumnFilter: false }),
    userColumn.accessor('updated_at', { id: 'updated', enableColumnFilter: false }),
    userColumn.display({ id: 'actions', enableColumnFilter: false, enableSorting: false }),
  ]);
  const invitationColumn = createColumnHelper<typeof ACCESS_TABLE_FEATURES, PanelInvitation>();
  const INVITATION_COLUMNS = invitationColumn.columns([
    invitationColumn.accessor((invitation) => invitation.account.display_name, {
      id: 'name',
      enableColumnFilter: false,
    }),
    invitationColumn.accessor('role', { id: 'role' }),
    invitationColumn.accessor('status', { id: 'status', enableSorting: false }),
    invitationColumn.accessor('expires_at', { id: 'expires', enableColumnFilter: false }),
    invitationColumn.accessor('created_at', { id: 'created', enableColumnFilter: false }),
    invitationColumn.display({ id: 'actions', enableColumnFilter: false, enableSorting: false }),
  ]);

  const {
    section: activeSection,
    prefs = EPHEMERAL_PREFS,
    targetId,
    targetName,
    actorLogin,
    actorTargetRole,
    readOnly = false,
    onSection,
    fetchTargetUsers,
    addTargetUser,
    suggestUsers,
    updateTargetUser,
    fetchTargetInvitations,
    createTargetInvitation,
    reissueInvitation,
    revokeInvitation,
    fetchUserDecisions,
  }: {
    section: ManagementSection;
    prefs?: PrefsAccessor;
    targetId: string;
    targetName: string;
    /** The signed-in login, so the one refusal the panel can make for itself is made here. */
    actorLogin: string;
    actorTargetRole: InstallationRole;
    readOnly?: boolean;
    onSection: (section: ManagementSection) => void;
    fetchTargetUsers: (targetId: string, request: PanelUserPageRequest) => Promise<Page<PanelUser>>;
    addTargetUser: (targetId: string, input: AddTargetUserInput) => Promise<PanelUser>;
    /** Completes a login as it is typed; returns none when no roster is readable. */
    suggestUsers: (targetId: string, query: string) => Promise<PanelAccount[]>;
    updateTargetUser: (
      targetId: string,
      accountId: string,
      input: UpdateTargetUserInput,
    ) => Promise<PanelUser>;
    fetchTargetInvitations: (
      targetId: string,
      request: InvitationPageRequest,
    ) => Promise<Page<PanelInvitation>>;
    createTargetInvitation: (
      targetId: string,
      input: AddTargetInvitationInput,
    ) => Promise<PanelInvitation>;
    reissueInvitation: (
      targetId: string,
      invitationId: string,
      expiresInDays: InvitationDays,
    ) => Promise<PanelInvitation>;
    revokeInvitation: (targetId: string, invitationId: string) => Promise<PanelInvitation>;
    fetchUserDecisions: (accountId: string, targetId: string) => Promise<AccessDecision[]>;
  } = $props();

  // Table state deliberately captures the preferences at mount; remote
  // changes apply on the next remount instead of mid-interaction.
  // svelte-ignore state_referenced_locally
  const initialPrefs = prefs;

  let actionFailure = $state<string | null>(null);
  let feedback = $state('');

  const USER_SORTS: readonly PanelUserSort[] = [
    'name_asc',
    'name_desc',
    'role_asc',
    'role_desc',
    'updated_newest',
    'updated_oldest',
    'login_newest',
    'login_oldest',
  ];
  const INVITATION_SORTS: readonly InvitationSort[] = [
    'created_newest',
    'created_oldest',
    'expiry_soonest',
    'expiry_latest',
    'name_asc',
    'name_desc',
    'role_asc',
    'role_desc',
  ];

  let userSearch = $state(prefText(initialPrefs.get('table.users.search')));
  let userQuery = $state(prefText(initialPrefs.get('table.users.search')));
  let userSort = $state<PanelUserSort>(
    prefOption(initialPrefs.get('table.users.sort'), USER_SORTS, 'name_asc'),
  );
  let userRoles = $state<InstallationRole[]>(
    prefList(initialPrefs.get('table.users.roles'), ['none', 'viewer', 'editor', 'admin']),
  );
  let userStatuses = $state<PanelUserListStatus[]>(
    prefList(initialPrefs.get('table.users.statuses'), ['active', 'banned', 'suspended']),
  );
  const userLimit = 20;

  let invitationSearch = $state(prefText(initialPrefs.get('table.invitations.search')));
  let invitationQuery = $state(prefText(initialPrefs.get('table.invitations.search')));
  let invitationSort = $state<InvitationSort>(
    prefOption(initialPrefs.get('table.invitations.sort'), INVITATION_SORTS, 'name_asc'),
  );
  let invitationRoles = $state<Exclude<InstallationRole, 'none'>[]>(
    prefList(initialPrefs.get('table.invitations.roles'), ['viewer', 'editor', 'admin']),
  );
  let invitationStatuses = $state<InvitationStatus[]>(
    prefList(initialPrefs.get('table.invitations.statuses'), [
      'pending',
      'accepted',
      'declined',
      'revoked',
      'expired',
    ]),
  );

  // One persistence effect instead of a write at every mutation site: any
  // change to the tracked state syncs, and the initial run is a no-op because
  // the state was just read from the same preferences.
  $effect(() => {
    prefs.set('table.users.sort', userSort);
    prefs.set('table.users.roles', [...userRoles]);
    prefs.set('table.users.statuses', [...userStatuses]);
    prefs.set('table.users.search', userQuery);
    prefs.set('table.invitations.sort', invitationSort);
    prefs.set('table.invitations.roles', [...invitationRoles]);
    prefs.set('table.invitations.statuses', [...invitationStatuses]);
    prefs.set('table.invitations.search', invitationQuery);
  });
  const invitationLimit = 20;
  const usersQuery = createInfiniteQuery(() => ({
    queryKey: [
      'users',
      targetId,
      userQuery,
      userSort,
      [...userRoles],
      [...userStatuses],
      userLimit,
    ],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      fetchTargetUsers(targetId, {
        ...(pageParam === undefined ? {} : { cursor: pageParam }),
        query: userQuery,
        sort: userSort,
        limit: userLimit,
        roles: [...userRoles],
        statuses: [...userStatuses],
      }),
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  }));
  const invitationsQuery = createInfiniteQuery(() => ({
    queryKey: [
      'invitations',
      targetId,
      invitationQuery,
      invitationSort,
      [...invitationRoles],
      [...invitationStatuses],
      invitationLimit,
    ],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      fetchTargetInvitations(targetId, {
        ...(pageParam === undefined ? {} : { cursor: pageParam }),
        query: invitationQuery,
        sort: invitationSort,
        limit: invitationLimit,
        roles: [...invitationRoles],
        statuses: [...invitationStatuses],
      }),
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  }));
  const userPage = $derived(flattenPages(usersQuery.data));
  const invitationPage = $derived(flattenPages(invitationsQuery.data));
  const loadingUsers = $derived(usersQuery.isFetching);
  const loadingInvitations = $derived(invitationsQuery.isFetching);
  const userFailure = $derived(
    usersQuery.isError && !usersQuery.isFetchNextPageError ? errorMessage(usersQuery.error) : null,
  );
  const invitationFailure = $derived(
    invitationsQuery.isError && !invitationsQuery.isFetchNextPageError
      ? errorMessage(invitationsQuery.error)
      : null,
  );
  const userLoadMoreFailure = $derived(
    usersQuery.isFetchNextPageError ? errorMessage(usersQuery.error) : null,
  );
  const invitationLoadMoreFailure = $derived(
    invitationsQuery.isFetchNextPageError ? errorMessage(invitationsQuery.error) : null,
  );

  let addButton = $state<HTMLButtonElement | null>(null);
  let addReturnFocus = $state<HTMLElement | null>(null);
  let login = $state('');
  let addRole = $state<InstallationRole>('viewer');
  let accessMethod = $state<'add' | 'invite'>('add');
  /**
   * Why the dialog was opened, kept apart from which method is selected in it.
   *
   * The list you are looking at decides which of the two is the obvious one, so opening the dialog
   * from Invitations starts on the invitation and says so in its heading. The heading then holds
   * still while you are inside: flipping the method picker changes what the submit button will do,
   * and the button already says which. A title that rewrote itself under the pointer would only
   * make you re-read it.
   */
  let addIntent = $state<'add' | 'invite'>('add');
  const invitingFirst = $derived(activeSection === 'invitations');

  /** Refusals the dialog raises itself, rather than the toolbar line behind it. */
  let addFailure = $state<string | null>(null);

  /**
   * The login the server said had declined, which turns the dialog into a question.
   *
   * The gate is the server's - it knows the whole history, the panel only knows the page it has
   * loaded - so the dialog does not try to predict it. It sends, is told, and asks.
   */
  let declinedLogin = $state<string | null>(null);

  /**
   * Naming yourself is refused here as well as there, because it is the one standing the panel
   * knows for certain and the only one it can answer before the press. The server refuses it too:
   * this is a shorter path to the same answer, not the answer itself.
   */
  const namingSelf = $derived(
    login.trim() !== '' && login.trim().toLowerCase() === actorLogin.trim().toLowerCase(),
  );
  const selfRefusal = $derived(
    accessMethod === 'invite' ? 'You cannot invite yourself' : 'You cannot change your own access',
  );
  let expiresInDays = $state<InvitationDays>(7);
  let generatedLink = $state('');
  let adding = $state(false);

  /**
   * What the dialog is for right now. The link outranks the question, because reaching a link
   * means the question has already been answered.
   */
  const addStage = $derived(
    generatedLink !== '' ? 'link' : declinedLogin !== null ? 'confirm' : 'form',
  );

  let actionTrigger = $state<HTMLElement | null>(null);
  let reason = $state('');
  let invitationActionTrigger = $state<HTMLElement | null>(null);
  let invitationBusy = $state<string | null>(null);
  let savingAccount = $state<string | null>(null);
  let historyTrigger = $state<HTMLElement | null>(null);
  let userResults = $state<HTMLDivElement>();
  let invitationResults = $state<HTMLDivElement>();
  let userScroll = $state<HTMLTableSectionElement>();
  let invitationScroll = $state<HTMLTableSectionElement>();

  // Ticks so "5 minutes ago" keeps aging in a long session; a captured
  // timestamp would freeze every relative time at first render.
  let now = $state(Date.now());

  const users = $derived(userPage?.items ?? []);
  const invitations = $derived(invitationPage?.items ?? []);

  /* Every dialog here is whatever the address names, so a reload keeps the reader
     on what they were doing and a link to it can be sent to someone else.

     People are named by login and invitations by id, and both are looked up in
     the loaded page: an address naming somebody who is not there opens nothing,
     which is the right answer for a link to a person whose access has since been
     removed. */
  const addModalOpen = $derived(dialogRoute.isOpen(ADD_DIALOG));
  const actionUser = $derived(findUser(dialogRoute.param(ACTION_DIALOG, 'user')));
  const pendingAction = $derived(
    actionUser === null ? null : userAction(dialogRoute.param(ACTION_DIALOG, 'action')),
  );
  const pendingInvitation = $derived(
    findInvitation(dialogRoute.param(INVITATION_DIALOG, 'invitation')),
  );
  const historyUser = $derived(findUser(dialogRoute.param(HISTORY_DIALOG, 'user')));

  function findUser(login: string | undefined): PanelUser | null {
    if (login === undefined) return null;
    return users.find((user) => user.account.login === login) ?? null;
  }

  function findInvitation(id: string | undefined): PanelInvitation | null {
    if (id === undefined) return null;
    return invitations.find((invitation) => invitation.id === id) ?? null;
  }

  function userAction(value: string | undefined): UserAction | null {
    return value === 'suspend' || value === 'restore' || value === 'remove_access' ? value : null;
  }
  // Initial-load failures render inside the table region with a retry; the
  // toolbar line is for action failures and refresh failures over live data.
  const failure = $derived(
    actionFailure ??
      (activeSection === 'users'
        ? userPage === null
          ? null
          : userFailure
        : invitationPage === null
          ? null
          : invitationFailure),
  );
  const sectionOptions = $derived([
    {
      value: 'users',
      label: 'Users',
      tone: 'accent' as const,
    },
    {
      value: 'invitations',
      label: 'Invitations',
      tone: 'accent' as const,
    },
  ]);
  const addRoleOptions = $derived(
    addRoles().map((role) => ({ value: role, label: roleLabel(role), icon: roleIcon(role) })),
  );
  const userStatusFilterSections = $derived<FilterSection[]>([
    {
      options: [
        { value: 'active', label: 'Active', tone: 'valid' },
        { value: 'banned', label: 'Banned', tone: 'invalid' },
        { value: 'suspended', label: 'Suspended', tone: 'bypassed' as const },
      ],
    },
  ]);
  const userTable = createTable({
    features: ACCESS_TABLE_FEATURES,
    columns: USER_COLUMNS,
    get data() {
      return users;
    },
    getRowId: (user) => user.account.id,
    manualFiltering: true,
    manualSorting: true,
    state: {
      get sorting() {
        return userSortingState();
      },
      get columnFilters() {
        return userColumnFilters();
      },
    },
    onSortingChange: selectUserSorting,
    onColumnFiltersChange: selectUserColumnFilters,
  });
  const invitationTable = createTable({
    features: ACCESS_TABLE_FEATURES,
    columns: INVITATION_COLUMNS,
    get data() {
      return invitations;
    },
    getRowId: (invitation) => invitation.id,
    manualFiltering: true,
    manualSorting: true,
    state: {
      get sorting() {
        return invitationSortingState();
      },
      get columnFilters() {
        return invitationColumnFilters();
      },
    },
    onSortingChange: selectInvitationSorting,
    onColumnFiltersChange: selectInvitationColumnFilters,
  });
  const userTableRows = $derived(userTable.getRowModel().rows);
  const invitationTableRows = $derived(invitationTable.getRowModel().rows);
  const desktopTableLayout = new MediaQuery('min-width: 64.001rem', true);
  const userVirtualizer = createVirtualizer<HTMLTableSectionElement, HTMLTableRowElement>({
    count: 0,
    estimateSize: () => 56,
    getScrollElement: () => userScroll ?? null,
    overscan: 6,
  });
  const invitationVirtualizer = createVirtualizer<HTMLTableSectionElement, HTMLTableRowElement>({
    count: 0,
    estimateSize: () => 56,
    getScrollElement: () => invitationScroll ?? null,
    overscan: 6,
  });
  const userRenderRows = $derived.by(() =>
    desktopTableLayout.current
      ? $userVirtualizer.getVirtualItems().map((row) => ({ ...row, virtual: true as const }))
      : userTableRows.map((row, index) => ({
          index,
          key: row.id,
          size: 0,
          start: 0,
          virtual: false as const,
        })),
  );
  const invitationRenderRows = $derived.by(() =>
    desktopTableLayout.current
      ? $invitationVirtualizer.getVirtualItems().map((row) => ({ ...row, virtual: true as const }))
      : invitationTableRows.map((row, index) => ({
          index,
          key: row.id,
          size: 0,
          start: 0,
          virtual: false as const,
        })),
  );

  useInterval(30_000, { callback: () => (now = Date.now()) });
  const debouncedUserSearch = useDebounce((value: string) => {
    userQuery = value;
    scrollResultsToTop(userResults);
  }, 180);
  const debouncedInvitationSearch = useDebounce((value: string) => {
    invitationQuery = value;
    scrollResultsToTop(invitationResults);
  }, 180);
  $effect(() => {
    const value = userSearch.trim();
    untrack(() => void debouncedUserSearch(value));
  });

  $effect(() => {
    const value = invitationSearch.trim();
    untrack(() => void debouncedInvitationSearch(value));
  });

  $effect(() => {
    const rows = userTableRows;
    const desktop = desktopTableLayout.current;
    untrack(() => {
      get(userVirtualizer).setOptions({
        count: desktop ? rows.length : 0,
        getScrollElement: () => userScroll ?? null,
        getItemKey: (index) => rows[index]?.id ?? index,
      });
    });
  });

  $effect(() => {
    const rows = invitationTableRows;
    const desktop = desktopTableLayout.current;
    untrack(() => {
      get(invitationVirtualizer).setOptions({
        count: desktop ? rows.length : 0,
        getScrollElement: () => invitationScroll ?? null,
        getItemKey: (index) => rows[index]?.id ?? index,
      });
    });
  });

  $effect(() => {
    if (!desktopTableLayout.current) return;
    const rows = activeSection === 'users' ? userTableRows : invitationTableRows;
    const items =
      activeSection === 'users'
        ? $userVirtualizer.getVirtualItems()
        : $invitationVirtualizer.getVirtualItems();
    const last = items.at(-1);
    if (last === undefined || last.index < rows.length - 5) return;
    untrack(() => {
      if (activeSection === 'users') void loadNextUsers();
      else void loadNextInvitations();
    });
  });

  async function loadUsers(_cursor?: string, append = false): Promise<void> {
    if (append) await usersQuery.fetchNextPage();
    else await usersQuery.refetch();
  }

  async function loadInvitations(_cursor?: string, append = false): Promise<void> {
    if (append) await invitationsQuery.fetchNextPage();
    else await invitationsQuery.refetch();
  }

  async function loadNextUsers(): Promise<void> {
    if (!usersQuery.hasNextPage || usersQuery.isFetchingNextPage) return;
    await usersQuery.fetchNextPage();
  }

  async function loadNextInvitations(): Promise<void> {
    if (!invitationsQuery.hasNextPage || invitationsQuery.isFetchingNextPage) return;
    await invitationsQuery.fetchNextPage();
  }

  async function reloadUsers(): Promise<void> {
    scrollResultsToTop(userResults);
    await usersQuery.refetch();
  }

  async function reloadInvitations(): Promise<void> {
    scrollResultsToTop(invitationResults);
    await invitationsQuery.refetch();
  }

  function clearUserFilters(): void {
    scrollResultsToTop(userResults);
    userSearch = '';
    userQuery = '';
    userRoles = [];
    userStatuses = [];
  }

  function clearInvitationFilters(): void {
    scrollResultsToTop(invitationResults);
    invitationSearch = '';
    invitationQuery = '';
    invitationRoles = [];
    invitationStatuses = [];
  }

  async function submitAdd(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    await grantAccess(false);
  }

  /**
   * One attempt at whichever method is selected, and the answer to a refusal it can act on.
   *
   * `acknowledged` is the second press. It is passed rather than read from `declinedLogin` so the
   * two attempts are visibly different calls: the first never carries it, the second always does.
   */
  async function grantAccess(acknowledged: boolean): Promise<void> {
    const normalizedLogin = login.trim();
    if (normalizedLogin === '' || namingSelf) return;
    adding = true;
    addFailure = null;
    const destination = targetName;
    try {
      if (accessMethod === 'invite') {
        const created = await createTargetInvitation(targetId, {
          login: normalizedLogin,
          role: addRole as AddTargetInvitationInput['role'],
          expires_in_days: expiresInDays,
          ...(acknowledged ? { acknowledge_declined: true } : {}),
        });
        declinedLogin = null;
        generatedLink = created.invite_url ?? '';
        await copyGeneratedLink(false);
        feedback = `Invited @${normalizedLogin} to ${destination}`;
        await reloadInvitations();
      } else {
        await addTargetUser(targetId, {
          login: normalizedLogin,
          role: addRole as GrantedTargetRole,
        });
        feedback = `Added @${normalizedLogin} to ${destination}`;
        closeAddModal();
        await reloadUsers();
      }
      login = '';
    } catch (error) {
      if (error instanceof PanelApiError && error.code === 'invitation_declined') {
        declinedLogin = normalizedLogin;
      } else {
        addFailure = errorMessage(error);
      }
    } finally {
      adding = false;
    }
  }

  async function changeRole(user: PanelUser, value: string): Promise<void> {
    await mutate(user, async () => {
      const targetAccess = requiredTargetAccess(user);
      await updateTargetUser(targetId, user.account.id, {
        role: value as TargetRole,
        suspended: targetAccess.suspended,
        suspension_reason: targetAccess.suspension_reason,
        expected_revision: targetAccess.revision,
      });
      feedback = `Updated @${user.account.login}`;
    });
  }

  function beginAction(user: PanelUser, action: UserAction, trigger?: HTMLElement): void {
    actionTrigger = trigger ?? null;
    reason = '';
    dialogRoute.open(ACTION_DIALOG, { user: user.account.login, action });
  }

  function cancelAction(): void {
    if (dialogRoute.isOpen(ACTION_DIALOG)) dialogRoute.close();
    reason = '';
  }

  async function confirmAction(): Promise<void> {
    const user = actionUser;
    const action = pendingAction;
    if (user === null || action === null) return;
    await mutate(user, async () => {
      const targetAccess = requiredTargetAccess(user);
      await updateTargetUser(targetId, user.account.id, {
        role: action === 'remove_access' ? 'none' : targetAccess.role,
        suspended: action === 'suspend',
        suspension_reason: action === 'suspend' ? reason.trim() || undefined : undefined,
        expected_revision: targetAccess.revision,
      });
      feedback =
        action === 'suspend'
          ? `Suspended @${user.account.login} for ${targetName}`
          : action === 'restore'
            ? `Restored @${user.account.login} for ${targetName}`
            : `Removed @${user.account.login} from ${targetName}`;
    });
  }

  async function mutate(user: PanelUser, operation: () => Promise<void>): Promise<void> {
    savingAccount = user.account.id;
    actionFailure = null;
    try {
      await operation();
      cancelAction();
      await Promise.all([reloadUsers(), reloadInvitations()]);
    } catch (error) {
      await reloadUsers();
      actionFailure = errorMessage(error);
    } finally {
      savingAccount = null;
    }
  }

  async function reissue(invitation: PanelInvitation, trigger: HTMLElement | null): Promise<void> {
    invitationBusy = invitation.id;
    actionFailure = null;
    try {
      const updated = await reissueInvitation(targetId, invitation.id, 7);
      generatedLink = updated.invite_url ?? '';
      accessMethod = 'invite';
      addIntent = 'invite';
      addReturnFocus = trigger;
      dialogRoute.open(ADD_DIALOG);
      // Whoever opens the dialog owns its whole state. This door bypasses openAddModal, so it
      // clears the same fields rather than trusting that the last close did.
      addFailure = null;
      declinedLogin = null;
      // A reissued link is a new link that has to be shared, which is the same reason the created
      // one goes to the clipboard by itself. Coming in through this door rather than the other one
      // should not change what the dialog has already done for you.
      linkCopied = null;
      await copyGeneratedLink(false);
      feedback = `Reissued invitation for @${invitation.account.login}`;
      await reloadInvitations();
    } catch (error) {
      actionFailure = errorMessage(error);
    } finally {
      invitationBusy = null;
    }
  }

  async function revoke(): Promise<void> {
    const invitation = pendingInvitation;
    if (invitation === null) return;
    invitationBusy = invitation.id;
    actionFailure = null;
    try {
      await revokeInvitation(targetId, invitation.id);
      feedback = `Revoked invitation for @${invitation.account.login}`;
      closeInvitationAction();
      await reloadInvitations();
    } catch (error) {
      actionFailure = errorMessage(error);
    } finally {
      invitationBusy = null;
    }
  }

  /**
   * Whether the link is on the clipboard, said in the dialog rather than behind it.
   *
   * The link is copied as soon as it exists, because that is the only reason to generate one, and
   * a dialog that quietly puts something on your clipboard is worse than one that does not.
   *
   * The two outcomes are not the same kind of message and are not shown in the same place. Success
   * is a receipt for something already done: it rides the heading, and leaves once it has been
   * read, because a confirmation that never goes away starts reading as part of the dialog. Failure
   * is an instruction - copy it from the field yourself - so it stays, next to the field it is
   * about.
   */
  let linkCopied = $state<'copied' | 'failed' | null>(null);

  /**
   * Bumped on every copy so the receipt is a new element each time.
   *
   * Pressing Copy link while the previous receipt is still on screen has to be answered. Without
   * this the element is the one already running its animation, and the press would look ignored.
   */
  let copyReceipt = $state(0);

  async function copyGeneratedLink(announce = true): Promise<void> {
    if (generatedLink === '') return;
    try {
      await navigator.clipboard.writeText(generatedLink);
      linkCopied = 'copied';
      copyReceipt += 1;
      if (announce) feedback = 'Copied invitation link';
    } catch {
      linkCopied = 'failed';
      // Never claimed on the copy that happens by itself: the message below the field says what
      // actually happened, and a banner for something nobody asked for would be noise.
      if (announce) actionFailure = 'The invitation link could not be copied';
    }
  }

  function openAddModal(): void {
    generatedLink = '';
    linkCopied = null;
    addFailure = null;
    declinedLogin = null;
    addRole = 'viewer';
    accessMethod = invitingFirst ? 'invite' : 'add';
    addIntent = accessMethod;
    addReturnFocus = addButton;
    dialogRoute.open(ADD_DIALOG);
  }

  function closeAddModal(): void {
    if (dialogRoute.isOpen(ADD_DIALOG)) dialogRoute.close();
  }

  function closeInvitationAction(): void {
    if (dialogRoute.isOpen(INVITATION_DIALOG)) dialogRoute.close();
  }

  /* The dialog's own fields are cleared when it goes away, not when Cancel is
     pressed. Back closes it without ever reaching a handler, and a half-typed
     login left behind would be waiting inside it the next time it opened. */
  $effect(() => {
    if (addModalOpen) return;
    untrack(() => {
      generatedLink = '';
      addFailure = null;
      declinedLogin = null;
      login = '';
    });
  });

  $effect(() => {
    if (actionUser !== null) return;
    untrack(() => (reason = ''));
  });

  function selectSection(section: string): void {
    if (section === 'users' || section === 'invitations') onSection(section);
  }

  function selectUserSort(column: UserSortColumn): void {
    scrollResultsToTop(userResults);
    const target = userTable.getColumn(column);
    target?.toggleSorting(target.getIsSorted() === 'asc');
  }

  function selectInvitationSort(column: InvitationSortColumn): void {
    scrollResultsToTop(invitationResults);
    const target = invitationTable.getColumn(column);
    target?.toggleSorting(target.getIsSorted() === 'asc');
  }

  function userSortDirection(column: UserSortColumn): SortDirection | undefined {
    const direction = userTable.getColumn(column)?.getIsSorted();
    return direction === 'asc' ? 'ascending' : direction === 'desc' ? 'descending' : undefined;
  }

  function invitationSortDirection(column: InvitationSortColumn): SortDirection | undefined {
    const direction = invitationTable.getColumn(column)?.getIsSorted();
    return direction === 'asc' ? 'ascending' : direction === 'desc' ? 'descending' : undefined;
  }

  function userSortingState(): SortingState {
    const mapping: Record<PanelUserSort, { id: string; desc: boolean }> = {
      name_asc: { id: 'name', desc: false },
      name_desc: { id: 'name', desc: true },
      role_asc: { id: 'role', desc: false },
      role_desc: { id: 'role', desc: true },
      login_newest: { id: 'last_login', desc: true },
      login_oldest: { id: 'last_login', desc: false },
      updated_newest: { id: 'updated', desc: true },
      updated_oldest: { id: 'updated', desc: false },
    };
    return [mapping[userSort]];
  }

  function invitationSortingState(): SortingState {
    const mapping: Record<InvitationSort, { id: string; desc: boolean }> = {
      name_asc: { id: 'name', desc: false },
      name_desc: { id: 'name', desc: true },
      role_asc: { id: 'role', desc: false },
      role_desc: { id: 'role', desc: true },
      expiry_soonest: { id: 'expires', desc: false },
      expiry_latest: { id: 'expires', desc: true },
      created_newest: { id: 'created', desc: true },
      created_oldest: { id: 'created', desc: false },
    };
    return [mapping[invitationSort]];
  }

  function userColumnFilters(): ColumnFiltersState {
    return [
      { id: 'role', value: userRoles },
      { id: 'status', value: userStatuses },
    ];
  }

  function invitationColumnFilters(): ColumnFiltersState {
    return [
      { id: 'role', value: invitationRoles },
      { id: 'status', value: invitationStatuses },
    ];
  }

  function selectUserSorting(next: Updater<SortingState>): void {
    const resolved = typeof next === 'function' ? next(userSortingState()) : next;
    const selected = resolved[0];
    if (selected === undefined) return;
    const mapping: Record<string, readonly [PanelUserSort, PanelUserSort]> = {
      name: ['name_asc', 'name_desc'],
      role: ['role_asc', 'role_desc'],
      last_login: ['login_oldest', 'login_newest'],
      updated: ['updated_oldest', 'updated_newest'],
    };
    const options = mapping[selected.id];
    if (options !== undefined) userSort = options[selected.desc ? 1 : 0];
  }

  function selectInvitationSorting(next: Updater<SortingState>): void {
    const resolved = typeof next === 'function' ? next(invitationSortingState()) : next;
    const selected = resolved[0];
    if (selected === undefined) return;
    const mapping: Record<string, readonly [InvitationSort, InvitationSort]> = {
      name: ['name_asc', 'name_desc'],
      role: ['role_asc', 'role_desc'],
      expires: ['expiry_soonest', 'expiry_latest'],
      created: ['created_oldest', 'created_newest'],
    };
    const options = mapping[selected.id];
    if (options !== undefined) invitationSort = options[selected.desc ? 1 : 0];
  }

  function selectUserColumnFilters(next: Updater<ColumnFiltersState>): void {
    const resolved = typeof next === 'function' ? next(userColumnFilters()) : next;
    const selected = (id: string): string[] => {
      const value = resolved.find((filter) => filter.id === id)?.value;
      return Array.isArray(value) ? value.map(String) : [];
    };
    selectUserFilters([...selected('role'), ...selected('status')]);
  }

  function selectInvitationColumnFilters(next: Updater<ColumnFiltersState>): void {
    const resolved = typeof next === 'function' ? next(invitationColumnFilters()) : next;
    const selected = (id: string): string[] => {
      const value = resolved.find((filter) => filter.id === id)?.value;
      return Array.isArray(value) ? value.map(String) : [];
    };
    selectInvitationFilters([...selected('role'), ...selected('status')]);
  }

  function userAt(index: number): PanelUser {
    const row = userTableRows[index];
    if (row === undefined) throw new Error(`missing virtual user row ${index}`);
    return row.original;
  }

  function invitationAt(index: number): PanelInvitation {
    const row = invitationTableRows[index];
    if (row === undefined) throw new Error(`missing virtual invitation row ${index}`);
    return row.original;
  }

  function hasDecisionHistory(user: PanelUser): boolean {
    return user.status === 'banned' || user.target_access?.suspended === true;
  }

  /* Which row is being held down. `:active` on a `<tr>` matches but does not
     repaint it - the row stayed on its hover colour with `matches(':active')`
     already true - so the state is carried as a class the row can be styled by
     like anything else. */
  let pressedRow = $state<string | null>(null);

  function holdRow(user: PanelUser): void {
    if (!hasDecisionHistory(user)) return;
    pressedRow = user.account.id;
  }

  function releaseRow(): void {
    pressedRow = null;
  }

  function openHistory(user: PanelUser, trigger: HTMLElement): void {
    if (!hasDecisionHistory(user)) return;
    historyTrigger = trigger;
    dialogRoute.open(HISTORY_DIALOG, { user: user.account.login });
  }

  function clickHistoryRow(event: MouseEvent, user: PanelUser): void {
    if (!hasDecisionHistory(user)) return;
    if (
      event.target instanceof Element &&
      event.target.closest('button, select, input, textarea, a, summary') !== null
    )
      return;
    openHistory(user, event.currentTarget as HTMLElement);
  }

  function keyHistoryRow(event: KeyboardEvent, user: PanelUser): void {
    if (event.target !== event.currentTarget || !['Enter', ' '].includes(event.key)) return;
    event.preventDefault();
    openHistory(user, event.currentTarget as HTMLElement);
  }

  function closeHistory(): void {
    if (dialogRoute.isOpen(HISTORY_DIALOG)) dialogRoute.close();
  }

  function scrollResultsToTop(results: HTMLDivElement | undefined): void {
    if (window.matchMedia('(min-width: 64.001rem)').matches) {
      results?.querySelector<HTMLElement>('[data-panel-scroll]')?.scrollTo({ top: 0 });
    }
  }

  function userActionItems(user: PanelUser): ActionMenuItem[] {
    if (readOnly || !user.manageable) return [];
    return [
      user.target_access?.suspended === true
        ? {
            id: 'restore',
            icon: 'success',
            label: 'Restore access',
            description: `Allow access to ${targetName}`,
          }
        : {
            id: 'suspend',
            icon: 'ban',
            label: 'Suspend access',
            description: `Block access to ${targetName}`,
            tone: 'danger',
          },
      {
        id: 'remove_access',
        icon: 'no-access',
        label: 'Remove access',
        description: 'Set the installation role to No access',
        tone: 'danger',
      },
    ];
  }

  function invitationActionItems(invitation: PanelInvitation): ActionMenuItem[] {
    if (readOnly || (invitation.status !== 'pending' && invitation.status !== 'expired')) return [];
    const items: ActionMenuItem[] = [
      {
        id: 'reissue',
        icon: 'refresh',
        label: 'Reissue invitation',
        description: 'Create a new single-use link',
      },
    ];
    // An expired link is already dead; revoking it would be a no-op.
    if (invitation.status === 'pending') {
      items.push({
        id: 'revoke',
        icon: 'ban',
        label: 'Revoke invitation',
        description: 'Invalidate this invitation',
        tone: 'danger',
      });
    }
    return items;
  }

  function chooseInvitationAction(
    invitation: PanelInvitation,
    action: string,
    trigger: HTMLElement | null,
  ): void {
    if (action === 'reissue') {
      void reissue(invitation, trigger);
    } else if (action === 'revoke') {
      invitationActionTrigger = trigger;
      dialogRoute.open(INVITATION_DIALOG, { invitation: invitation.id, action: 'revoke' });
    }
  }

  function requiredTargetAccess(user: PanelUser): NonNullable<PanelUser['target_access']> {
    const access = user.target_access;
    if (access === undefined) throw new Error('installation access is missing');
    return access;
  }

  function addRoles(): InstallationRole[] {
    return actorTargetRole === 'owner' ? ['viewer', 'editor', 'admin'] : ['viewer', 'editor'];
  }

  function targetRoleOptions(): Array<{ value: string; label: string }> {
    const options = [
      { value: 'none', label: 'No access' },
      { value: 'viewer', label: 'Viewer' },
      { value: 'editor', label: 'Editor' },
    ];
    if (actorTargetRole === 'owner') options.push({ value: 'admin', label: 'Admin' });
    return options;
  }

  function selectableRoleOptions(): RolePickerOption[] {
    return targetRoleOptions().map((option) => ({
      ...option,
      icon: roleIcon(option.value as InstallationRole),
    }));
  }

  function selectedRole(user: PanelUser): string {
    return user.target_access?.role ?? 'none';
  }

  function shownRole(user: PanelUser): InstallationRole {
    return user.target_access?.effective_role ?? 'none';
  }

  function statusLabel(user: PanelUser): string {
    if (user.status === 'banned') return 'Banned';
    if (user.target_access?.suspended === true) return 'Suspended';
    return 'Active';
  }

  function statusTone(user: PanelUser): ChipTone {
    // Banned is permanent (red); suspended is a pause an administrator can
    // lift (amber). They carried identical chips once, and nobody could tell
    // the two states apart at a glance.
    if (user.status === 'banned') return 'stop';
    if (user.target_access?.suspended === true) return 'warning';
    return 'clear';
  }

  function currentReason(user: PanelUser): string | undefined {
    return user.status === 'banned'
      ? user.ban_reason
      : user.target_access?.suspended === true
        ? user.target_access.suspension_reason
        : undefined;
  }

  function currentDecisionAt(user: PanelUser): string | undefined {
    return user.status === 'banned'
      ? user.banned_at
      : user.target_access?.suspended === true
        ? user.target_access.updated_at
        : undefined;
  }

  function invitationTone(status: PanelInvitation['status']): ChipTone {
    if (status === 'pending') return 'signal';
    if (status === 'accepted') return 'clear';
    if (status === 'expired') return 'warning';
    // Declined is the invitee's own answer, not a failure; revoked is an
    // administrator veto and keeps the alarm color.
    if (status === 'declined') return 'neutral';
    return 'stop';
  }

  function invitationStatusLabel(status: InvitationStatus): string {
    return status.charAt(0).toUpperCase() + status.slice(1);
  }

  function roleLabel(role: InstallationRole): string {
    if (role === 'none') return 'No access';
    return role[0]?.toLocaleUpperCase() + role.slice(1);
  }

  function roleIcon(role: InstallationRole): IconName {
    if (role === 'owner') return 'owner';
    if (role === 'admin') return 'admin';
    if (role === 'editor') return 'editor';
    if (role === 'viewer') return 'viewer';
    return 'no-access';
  }

  function filterSummary(count: number): string {
    if (count === 0) return 'Filters';
    return `${count} filter${count === 1 ? '' : 's'}`;
  }

  function selectUserFilters(values: string[]): void {
    scrollResultsToTop(userResults);
    userRoles = values.filter((value): value is InstallationRole =>
      ['owner', 'admin', 'editor', 'viewer', 'none'].includes(value),
    );
    userStatuses = values.filter((value): value is PanelUserListStatus =>
      ['active', 'banned', 'suspended'].includes(value),
    );
  }

  function selectInvitationFilters(values: string[]): void {
    scrollResultsToTop(invitationResults);
    invitationRoles = values.filter((value): value is Exclude<InstallationRole, 'none'> =>
      ['admin', 'editor', 'viewer'].includes(value),
    );
    invitationStatuses = values.filter((value): value is InvitationStatus =>
      ['pending', 'accepted', 'expired', 'declined', 'revoked'].includes(value),
    );
  }

  function actionTitle(): string {
    const login = actionUser?.account.login ?? '';
    switch (pendingAction) {
      case 'suspend':
        return `Suspend @${login}`;
      case 'restore':
        return `Restore @${login}`;
      case 'remove_access':
        return `Remove @${login} from ${targetName}`;
      default:
        return 'Confirm access change';
    }
  }

  function actionDescription(): string {
    switch (pendingAction) {
      case 'suspend':
        return `This blocks access to ${targetName} until an administrator restores it`;
      case 'restore':
        return 'This restores access using the user’s current role';
      case 'remove_access':
        return 'This keeps the user account but sets this installation to No access';
      default:
        return '';
    }
  }

  function errorMessage(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }

  function flattenPages<T>(data: InfiniteData<Page<T>> | undefined): Page<T> | null {
    const pages = data?.pages;
    if (pages === undefined || pages.length === 0) return null;
    const last = pages.at(-1);
    return last === undefined ? null : { ...last, items: pages.flatMap((page) => page.items) };
  }
</script>

{#snippet sortButton(label: string, direction: SortDirection | undefined, onSelect: () => void)}
  <button class="sort-button table-sort-button" type="button" onclick={onSelect}>
    <span class="cap-trim">{label}</span>
    <span
      class:ascending={direction === 'ascending'}
      class:descending={direction === 'descending'}
      class="sort-indicator"
      aria-hidden="true"
    >
      <Icon name="sort" size={14} />
    </span>
  </button>
{/snippet}

{#snippet roleValue(role: InstallationRole)}
  <span class="role-value role-{role}">
    <span class="role-value-icon" aria-hidden="true"><Icon name={roleIcon(role)} size={14} /></span>
    <span class="band-trim">{roleLabel(role)}</span>
  </span>
{/snippet}

{#snippet headerActions()}
  <button class="btn btn-signal" type="button" bind:this={addButton} onclick={openAddModal}>
    <Icon name="user-plus" size={14} strokeWidth={2} />
    <span class="button-label">{invitingFirst ? 'Invite user' : 'Add user'}</span>
  </button>
{/snippet}

<section class="plate user-management" aria-labelledby="user-management-heading">
  <PanelHeader
    id="user-management-heading"
    title="Access"
    description="Roles, invitations, and access decisions for this workspace"
    actions={readOnly ? undefined : headerActions}
  />

  <div class="user-management-body">
    <div class="management-navigation">
      <NavigationTabs
        label="User management lists"
        options={sectionOptions}
        value={activeSection}
        onSelect={selectSection}
      />
      <div class="stable-feedback" aria-live="polite">{feedback}</div>
      {#if activeSection === 'users'}
        <SearchField
          label="Search users"
          placeholder="Search users"
          value={userSearch}
          onInput={(value) => (userSearch = value)}
        />
      {:else}
        <SearchField
          label="Search invitations"
          placeholder="Search invitations"
          value={invitationSearch}
          onInput={(value) => (invitationSearch = value)}
        />
      {/if}
    </div>

    {#if failure !== null}<p class="form-error" role="alert">{failure}</p>{/if}

    {#if activeSection === 'users'}
      <div id="users-list-panel" aria-label="Users">
        <div
          class:loading={loadingUsers}
          class="user-results"
          bind:this={userResults}
          aria-busy={loadingUsers}
        >
          {#if loadingUsers && userPage === null}
            <div class="table-skeleton" aria-hidden="true">
              {#each [0, 1, 2, 3, 4, 5] as index (index)}
                <span></span>
              {/each}
            </div>
            <p class="visually-hidden" role="status">Loading users</p>
          {:else if userFailure !== null && userPage === null}
            <ResultProblem
              title="Users could not be loaded"
              problem={userFailure}
              busy={loadingUsers}
              onRetry={() => void loadUsers(undefined, false)}
            />
          {:else}
            <!-- The table survived the failed refresh, so the failure is a line above it. -->
            {#if userFailure !== null}
              <ResultProblem
                title="Users could not be loaded"
                problem={userFailure}
                busy={loadingUsers}
                onRetry={() => void loadUsers(undefined, false)}
                overContent
              />
            {/if}
            <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
            <div
              class="user-table-wrap table-card"
              role="region"
              aria-label="Panel users"
              tabindex="0"
            >
              <table class="user-table">
                <caption class="visually-hidden">
                  Panel users. Select a sortable column header to change the sort order
                </caption>
                <thead>
                  <tr>
                    <th aria-sort={userSortDirection('name')}>
                      {@render sortButton('User', userSortDirection('name'), () =>
                        selectUserSort('name'),
                      )}
                    </th>
                    <th aria-sort={userSortDirection('role')}>
                      <div class="table-heading-layout">
                        {@render sortButton('Role', userSortDirection('role'), () =>
                          selectUserSort('role'),
                        )}
                        <FilterMenu
                          label="Role"
                          summary={filterSummary(userRoles.length)}
                          hint="Filter by permission level"
                          sections={ROLE_FILTERS}
                          selected={userRoles}
                          multiple
                          align="end"
                          onChange={(values) => userTable.getColumn('role')?.setFilterValue(values)}
                        />
                      </div>
                    </th>
                    <th class="filterable-heading">
                      <div class="table-heading-layout">
                        <span class="cap-trim">Status</span>
                        <FilterMenu
                          label="Status"
                          summary={filterSummary(userStatuses.length)}
                          hint="Filter by access status"
                          sections={userStatusFilterSections}
                          selected={userStatuses}
                          multiple
                          align="end"
                          onChange={(values) =>
                            userTable.getColumn('status')?.setFilterValue(values)}
                        />
                      </div>
                    </th>
                    <th aria-sort={userSortDirection('last_login')}>
                      {@render sortButton('Last login', userSortDirection('last_login'), () =>
                        selectUserSort('last_login'),
                      )}
                    </th>
                    <th><span class="visually-hidden">Actions</span></th>
                  </tr>
                </thead>
                <tbody bind:this={userScroll} data-panel-scroll>
                  {#if users.length === 0}
                    {@const hasUserFilters =
                      userQuery !== '' || userRoles.length > 0 || userStatuses.length > 0}
                    <tr class="empty-row">
                      <td colspan="5">
                        <TableEmptyState
                          title={hasUserFilters
                            ? 'No users match'
                            : 'No users for this installation'}
                          description={hasUserFilters
                            ? 'Try another search or clear the active filters'
                            : 'Added users will appear here'}
                          actionLabel={hasUserFilters ? 'Clear filters' : undefined}
                          onAction={hasUserFilters ? clearUserFilters : undefined}
                        />
                      </td>
                    </tr>
                  {/if}
                  {#if desktopTableLayout.current}
                    <tr
                      class="virtual-spacer"
                      aria-hidden="true"
                      style:height={`${$userVirtualizer.getTotalSize()}px`}
                      ><td colspan="5"></td></tr
                    >
                  {/if}
                  {#each userRenderRows as virtualRow (virtualRow.key)}
                    {@const user = userAt(virtualRow.index)}
                    <!-- The virtualiser's offset goes in a custom property rather
                         than straight into `transform`, so the press can add a
                         scale to the same property without overwriting the value
                         that puts the row on screen. -->
                    <tr
                      class:virtual-row={virtualRow.virtual}
                      class:history-row={hasDecisionHistory(user)}
                      class:pressing={pressedRow === user.account.id}
                      style:height={virtualRow.virtual ? `${virtualRow.size}px` : undefined}
                      style:--row-y={virtualRow.virtual ? `${virtualRow.start}px` : '0px'}
                      tabindex={hasDecisionHistory(user) ? 0 : undefined}
                      onclick={(event) => clickHistoryRow(event, user)}
                      onkeydown={(event) => keyHistoryRow(event, user)}
                      onpointerdown={() => holdRow(user)}
                      onpointerup={releaseRow}
                      onpointercancel={releaseRow}
                      onpointerleave={releaseRow}
                    >
                      <th scope="row">
                        <span class="user-identity">
                          <Avatar account={user.account} size={32} />
                          <!-- The hint sits outside the stack, not at the end of it:
                               `.band-trim-stack` trims the last line's descender space, and a
                               clipped span is a last child with no line to trim. -->
                          {#if hasDecisionHistory(user)}
                            <span class="visually-hidden">
                              Select this row to review access decision history
                            </span>
                          {/if}
                          <span class="band-trim-stack">
                            <strong>{user.account.display_name}</strong>
                            <span class="user-login mono">@{user.account.login}</span>
                          </span>
                        </span>
                      </th>
                      <td data-label="Role">
                        {#if user.manageable && !readOnly}
                          <RolePicker
                            label="Role for {user.account.login}"
                            value={selectedRole(user)}
                            options={selectableRoleOptions()}
                            disabled={savingAccount === user.account.id}
                            onSelect={(value) => void changeRole(user, value)}
                          />
                        {:else}
                          {@render roleValue(shownRole(user))}
                        {/if}
                      </td>
                      <td data-label="Status">
                        <Chip tone={statusTone(user)} dot>{statusLabel(user)}</Chip>
                      </td>
                      <td class="last-login" data-label="Last login">
                        {#if user.last_login_at === undefined}
                          <span class="dim"><span class="cap-trim">Never</span></span>
                        {:else}
                          <time
                            datetime={user.last_login_at}
                            title={formatTimestamp(user.last_login_at)}
                          >
                            <!-- Wrapped so there is a box to trim. These cells keep the control
                                 height so a row does not shrink, and a bare text node inside a
                                 34px flex box centres by its em box, which is 0.34px above the
                                 words - the whole column sat there. -->
                            <span class="cap-trim">{formatRelative(user.last_login_at, now)}</span>
                          </time>
                        {/if}
                      </td>
                      <td class="row-actions" data-label="Actions">
                        {#if user.manageable && !readOnly}
                          <ActionMenu
                            label={`Actions for @${user.account.login}`}
                            items={userActionItems(user)}
                            onSelect={(action, trigger) =>
                              beginAction(user, action as UserAction, trigger ?? undefined)}
                          />
                        {:else}
                          <span
                            class="action-slot-empty"
                            title="No actions available"
                            aria-hidden="true"
                          >
                            <Icon name="more" size={22} />
                          </span>
                        {/if}
                        <!-- After the actions rather than before, and always
                             drawn: it points out of the row, and it is what says
                             this row opens something where its neighbours do
                             not. Revealing it on hover only told a reader that
                             after they had already guessed. -->
                        {#if hasDecisionHistory(user)}
                          <span class="row-go" aria-hidden="true">
                            <Icon name="chevron-right" size={14} />
                          </span>
                        {/if}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
          <InfiniteLoadSentinel
            active={!desktopTableLayout.current &&
              !loadingUsers &&
              userLoadMoreFailure === null &&
              userPage?.next_cursor != null}
            cursor={userPage?.next_cursor}
            onVisible={() => void loadNextUsers()}
          />
          {#if userLoadMoreFailure !== null}
            <div class="load-more-alert" role="alert">
              <span>{userLoadMoreFailure}</span>
              <button class="btn" type="button" onclick={() => void loadNextUsers()}
                >Try again</button
              >
            </div>
          {/if}
        </div>
      </div>
    {:else}
      <div id="invitations-list-panel" aria-label="Invitations">
        <div
          class:loading={loadingInvitations}
          class="invitation-results"
          bind:this={invitationResults}
          aria-busy={loadingInvitations}
        >
          {#if loadingInvitations && invitationPage === null}
            <div class="table-skeleton" aria-hidden="true">
              {#each [0, 1, 2, 3, 4, 5] as index (index)}
                <span></span>
              {/each}
            </div>
            <p class="visually-hidden" role="status">Loading invitations</p>
          {:else if invitationFailure !== null && invitationPage === null}
            <ResultProblem
              title="Invitations could not be loaded"
              problem={invitationFailure}
              busy={loadingInvitations}
              onRetry={() => void loadInvitations(undefined, false)}
            />
          {:else}
            <!-- The table survived the failed refresh, so the failure is a line above it. -->
            {#if invitationFailure !== null}
              <ResultProblem
                title="Invitations could not be loaded"
                problem={invitationFailure}
                busy={loadingInvitations}
                onRetry={() => void loadInvitations(undefined, false)}
                overContent
              />
            {/if}
            <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
            <div
              class="user-table-wrap table-card"
              role="region"
              aria-label="Panel invitations"
              tabindex="0"
            >
              <table class="user-table invitation-table">
                <caption class="visually-hidden">
                  Panel invitations. Select a sortable column header to change the sort order
                </caption>
                <thead>
                  <tr>
                    <th aria-sort={invitationSortDirection('name')}>
                      {@render sortButton('Invitee', invitationSortDirection('name'), () =>
                        selectInvitationSort('name'),
                      )}
                    </th>
                    <th aria-sort={invitationSortDirection('role')}>
                      <div class="table-heading-layout">
                        {@render sortButton('Role', invitationSortDirection('role'), () =>
                          selectInvitationSort('role'),
                        )}
                        <FilterMenu
                          label="Role"
                          summary={filterSummary(invitationRoles.length)}
                          hint="Filter by invited permission level"
                          sections={INVITATION_ROLE_FILTERS}
                          selected={invitationRoles}
                          multiple
                          align="end"
                          onChange={(values) =>
                            invitationTable.getColumn('role')?.setFilterValue(values)}
                        />
                      </div>
                    </th>
                    <th class="filterable-heading">
                      <div class="table-heading-layout">
                        <span class="cap-trim">Status</span>
                        <FilterMenu
                          label="Status"
                          summary={filterSummary(invitationStatuses.length)}
                          hint="Filter by invitation status"
                          sections={INVITATION_STATUS_FILTERS}
                          selected={invitationStatuses}
                          multiple
                          align="end"
                          onChange={(values) =>
                            invitationTable.getColumn('status')?.setFilterValue(values)}
                        />
                      </div>
                    </th>
                    <th class="sent-heading">
                      <div class="table-heading-layout"><span class="cap-trim">Sent</span></div>
                    </th>
                    <th aria-sort={invitationSortDirection('expires')}>
                      {@render sortButton('Expires', invitationSortDirection('expires'), () =>
                        selectInvitationSort('expires'),
                      )}
                    </th>
                    <th><span class="visually-hidden">Actions</span></th>
                  </tr>
                </thead>
                <tbody bind:this={invitationScroll} data-panel-scroll>
                  {#if invitations.length === 0}
                    {@const hasInvitationFilters =
                      invitationQuery !== '' ||
                      invitationRoles.length > 0 ||
                      invitationStatuses.length > 0}
                    <tr class="empty-row">
                      <td colspan="6">
                        <TableEmptyState
                          title={hasInvitationFilters
                            ? 'No invitations match'
                            : 'No invitations for this installation'}
                          description={hasInvitationFilters
                            ? 'Try another search or clear the active filters'
                            : 'New invitations will appear here'}
                          actionLabel={hasInvitationFilters ? 'Clear filters' : undefined}
                          onAction={hasInvitationFilters ? clearInvitationFilters : undefined}
                        />
                      </td>
                    </tr>
                  {/if}
                  {#if desktopTableLayout.current}
                    <tr
                      class="virtual-spacer"
                      aria-hidden="true"
                      style:height={`${$invitationVirtualizer.getTotalSize()}px`}
                      ><td colspan="6"></td></tr
                    >
                  {/if}
                  {#each invitationRenderRows as virtualRow (virtualRow.key)}
                    {@const invitation = invitationAt(virtualRow.index)}
                    <tr
                      class:virtual-row={virtualRow.virtual}
                      style:height={virtualRow.virtual ? `${virtualRow.size}px` : undefined}
                      style:transform={virtualRow.virtual
                        ? `translateY(${virtualRow.start}px)`
                        : undefined}
                    >
                      <th scope="row">
                        <span class="user-identity">
                          <Avatar account={invitation.account} size={32} />
                          <span class="band-trim-stack">
                            <strong>{invitation.account.display_name}</strong>
                            <span class="user-login mono">@{invitation.account.login}</span>
                          </span>
                        </span>
                      </th>
                      <td data-label="Role">{@render roleValue(invitation.role ?? 'none')}</td>
                      <td data-label="Status"
                        ><Chip tone={invitationTone(invitation.status)} dot
                          >{invitationStatusLabel(invitation.status)}</Chip
                        ></td
                      >
                      <td class="last-login" data-label="Sent">
                        <time
                          datetime={invitation.created_at}
                          title={formatTimestamp(invitation.created_at)}
                        >
                          <span class="cap-trim">{formatRelative(invitation.created_at, now)}</span>
                        </time>
                      </td>
                      <td class="last-login" data-label="Expires">
                        {#if invitation.status === 'pending'}
                          <time
                            class="expires-soon"
                            datetime={invitation.expires_at}
                            title={formatTimestamp(invitation.expires_at)}
                          >
                            <span class="cap-trim">{formatUntil(invitation.expires_at, now)}</span>
                          </time>
                        {:else if invitation.status === 'expired'}
                          <time
                            datetime={invitation.expires_at}
                            title={formatTimestamp(invitation.expires_at)}
                          >
                            <span class="cap-trim">{formatDateTime(invitation.expires_at)}</span>
                          </time>
                        {:else}
                          <!-- Expiry stops meaning anything once the invitation is resolved. -->
                          <span class="cell-dash" aria-hidden="true"
                            ><span class="cap-trim">—</span></span
                          >
                        {/if}
                      </td>
                      <td class="row-actions" data-label="Actions">
                        {#if invitationActionItems(invitation).length > 0}
                          <ActionMenu
                            label={`Actions for @${invitation.account.login} invitation`}
                            items={invitationActionItems(invitation)}
                            onSelect={(action, trigger) =>
                              chooseInvitationAction(invitation, action, trigger)}
                          />
                        {:else}
                          <span
                            class="action-slot-empty"
                            title="No actions available"
                            aria-hidden="true"
                          >
                            <Icon name="more" size={22} />
                          </span>
                        {/if}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
          <InfiniteLoadSentinel
            active={!desktopTableLayout.current &&
              !loadingInvitations &&
              invitationLoadMoreFailure === null &&
              invitationPage?.next_cursor != null}
            cursor={invitationPage?.next_cursor}
            onVisible={() => void loadNextInvitations()}
          />
          {#if invitationLoadMoreFailure !== null}
            <div class="load-more-alert" role="alert">
              <span>{invitationLoadMoreFailure}</span>
              <button class="btn" type="button" onclick={() => void loadNextInvitations()}
                >Try again</button
              >
            </div>
          {/if}
        </div>
      </div>
    {/if}
  </div>
</section>

{#if historyUser !== null}
  <DecisionHistory
    open
    label={`Access details for @${historyUser.account.login}`}
    scopeLabel={targetName}
    status={statusLabel(historyUser)}
    reason={currentReason(historyUser)}
    decidedAt={currentDecisionAt(historyUser)}
    returnFocus={historyTrigger}
    queryKey={['access-decisions', targetId, historyUser.account.id]}
    fetchDecisions={() => fetchUserDecisions(historyUser!.account.id, targetId)}
    onClose={closeHistory}
  />
{/if}

{#snippet copyReceiptNotice()}
  <CopyReceipt
    shown={linkCopied === 'copied'}
    pulse={copyReceipt}
    onDone={() => (linkCopied = null)}
  />
{/snippet}

<Modal
  id={ADD_DIALOG}
  open={addModalOpen}
  title={addStage === 'confirm'
    ? 'Invite again?'
    : addIntent === 'invite'
      ? 'Invite user'
      : 'Add user'}
  description={addStage === 'link'
    ? undefined
    : addStage === 'confirm'
      ? `@${declinedLogin} turned down the last invitation to ${targetName}`
      : addIntent === 'invite'
        ? 'Send a single-use invitation, or grant access right away'
        : 'Grant access now or send a single-use invitation'}
  headerExtra={addStage === 'link' ? copyReceiptNotice : undefined}
  returnFocus={addReturnFocus}
  onClose={closeAddModal}
>
  <form id="add-user-form" class="add-user-form" onsubmit={submitAdd}>
    {#if addStage === 'confirm'}
      <div class="confirmation-note">
        <span class="warning-mark" aria-hidden="true">!</span>
        <div>
          <strong>Declining was an answer</strong>
          <p>
            A new link reaches the same GitHub identity, and asking twice is visible to them and in
            the audit record.
          </p>
        </div>
      </div>
    {:else if addStage === 'form'}
      <div class="add-scope-summary">
        <span class="add-scope-icon" aria-hidden="true">
          <span class="cap-trim">{monogram(targetName, targetName).slice(0, 1)}</span>
        </span>
        <span>
          <small>Workspace</small>
          <strong>{targetName}</strong>
        </span>
      </div>

      <fieldset class="method-picker">
        <legend class="visually-hidden">Access method</legend>
        <div class="method-options">
          {#each ACCESS_METHODS as method (method.value)}
            <label class="method-option" class:selected={accessMethod === method.value}>
              <input
                type="radio"
                name="access-method"
                value={method.value}
                bind:group={accessMethod}
              />
              <span class="method-icon" aria-hidden="true">
                <Icon name={method.value === 'add' ? 'plus' : 'mail'} size={14} strokeWidth={2} />
              </span>
              <span class="method-copy">
                <strong>{method.label}</strong>
                <small>{method.description}</small>
              </span>
              <span class="method-check" aria-hidden="true"></span>
            </label>
          {/each}
        </div>
      </fieldset>

      <div class="identity-grid" class:with-expiry={accessMethod === 'invite'}>
        <LoginField
          id="add-user-login"
          label="GitHub login"
          bind:value={login}
          refused={namingSelf}
          focusOnOpen
          help={namingSelf
            ? selfRefusal
            : accessMethod === 'invite'
              ? 'The invitation only works for this GitHub identity'
              : 'GitHub login identifies the account to add'}
          suggest={(query) => suggestUsers(targetId, query)}
        />
        <label class="form-field">
          <span>Role</span>
          <span class="select-wrap">
            <select
              class="select-input"
              value={addRole}
              aria-label="Role"
              onchange={(event) => (addRole = event.currentTarget.value as InstallationRole)}
            >
              {#each addRoleOptions as option (option.value)}
                <option value={option.value}>{option.label}</option>
              {/each}
            </select>
            <Icon name="chevron-down" size={14} strokeWidth={2} />
          </span>
        </label>
        {#if accessMethod === 'invite'}
          <label class="form-field">
            <span>Expires after</span>
            <span class="select-wrap">
              <select
                class="select-input"
                bind:value={expiresInDays}
                aria-label="Invitation expiry"
              >
                <option value={1}>1 day</option>
                <option value={7}>7 days</option>
                <option value={30}>30 days</option>
              </select>
              <Icon name="chevron-down" size={14} strokeWidth={2} />
            </span>
          </label>
        {/if}
      </div>
    {:else}
      <div class="invitation-created" aria-live="polite">
        <span class="success-mark" aria-hidden="true">✓</span>
        <div>
          <strong>Invitation ready</strong>
          <p>Share this single-use link with the invited GitHub user</p>
        </div>
      </div>
      <label class="form-field">
        <span>Invitation link</span>
        <input class="text-input mono" readonly value={generatedLink} />
      </label>
      {#if linkCopied === 'failed'}
        <p class="link-clipboard" role="alert">
          <Icon name="alert" size={13} strokeWidth={2} />
          Copy it from the field above, the clipboard was not available
        </p>
      {/if}
    {/if}
    {#if addFailure !== null}<p class="form-error" role="alert">{addFailure}</p>{/if}
  </form>

  {#snippet footer()}
    {#if addStage === 'confirm'}
      <button class="btn btn-ghost" type="button" onclick={() => (declinedLogin = null)}>
        Back
      </button>
      <button
        class="btn btn-signal"
        type="button"
        disabled={adding}
        onclick={() => void grantAccess(true)}
      >
        {adding ? 'Sending…' : 'Invite again'}
      </button>
    {:else}
      <button class="btn btn-ghost" type="button" onclick={closeAddModal}>
        {addStage === 'form' ? 'Cancel' : 'Done'}
      </button>
    {/if}
    {#if addStage === 'form'}
      <button
        class="btn btn-signal"
        type="submit"
        form="add-user-form"
        disabled={adding || login.trim() === '' || namingSelf}
      >
        {adding
          ? accessMethod === 'invite'
            ? 'Sending…'
            : 'Adding…'
          : accessMethod === 'invite'
            ? 'Send invitation'
            : 'Add user'}
      </button>
    {:else if addStage === 'link'}
      <button
        class="btn btn-signal copy-button"
        type="button"
        onclick={() => void copyGeneratedLink()}
      >
        Copy link
      </button>
    {/if}
  {/snippet}
</Modal>

<Modal
  id={ACTION_DIALOG}
  open={actionUser !== null && pendingAction !== null}
  title={actionTitle()}
  description={actionDescription()}
  returnFocus={actionTrigger}
  onClose={cancelAction}
>
  {#if pendingAction === 'suspend'}
    <label class="form-field">
      <span>Reason <small>Optional</small></span>
      <textarea
        class="reason-textarea"
        placeholder="Add context for other administrators"
        maxlength="500"
        rows="4"
        bind:value={reason}
        data-modal-focus></textarea>
      <small>{reason.length}/500 characters</small>
    </label>
  {:else}
    <div class="confirmation-note">
      <span class="warning-mark" aria-hidden="true">!</span>
      <p>Review this change carefully before confirming</p>
    </div>
  {/if}

  {#snippet footer()}
    <button class="btn btn-ghost" type="button" data-modal-focus onclick={cancelAction}
      >Cancel</button
    >
    <button
      class="btn"
      class:btn-stop={pendingAction !== 'restore'}
      class:btn-signal={pendingAction === 'restore'}
      type="button"
      disabled={savingAccount !== null}
      onclick={() => void confirmAction()}
    >
      {savingAccount === null ? 'Confirm' : 'Saving…'}
    </button>
  {/snippet}
</Modal>

<Modal
  id={INVITATION_DIALOG}
  open={pendingInvitation !== null}
  title={`Revoke invitation for @${pendingInvitation?.account.login ?? ''}`}
  description="The current link will stop working immediately and the audit record will remain"
  returnFocus={invitationActionTrigger}
  onClose={closeInvitationAction}
>
  <div class="confirmation-note">
    <span class="warning-mark" aria-hidden="true">!</span>
    <p>The user can only join if you create and share a new invitation</p>
  </div>

  {#snippet footer()}
    <button class="btn btn-ghost" type="button" data-modal-focus onclick={closeInvitationAction}
      >Cancel</button
    >
    <button
      class="btn btn-stop"
      type="button"
      disabled={invitationBusy !== null}
      onclick={() => void revoke()}
    >
      {invitationBusy === null ? 'Revoke invitation' : 'Revoking…'}
    </button>
  {/snippet}
</Modal>

<style>
  .user-management {
    --local-control-height: var(--control-height-compact);

    background: transparent;
    border: 0;
    border-radius: 0;
    box-shadow: none;
    display: flex;
    flex: 1;
    flex-direction: column;
    margin-bottom: 0;
    min-height: 0;
    overflow: visible;
  }

  .user-management-body {
    background: transparent;
    border: 0;
    border-radius: 0;
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
    min-width: 0;
    overflow: visible;
  }

  #users-list-panel,
  #invitations-list-panel {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }

  .management-navigation {
    /* One 34px row: tabs, feedback, and search share the toolbar line. */
    --control-height: var(--control-height-compact);

    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    min-height: var(--control-height);
    padding-bottom: var(--space-3);
  }

  /* `.button-label` is trimmed to its band in `app.css`, which is what puts the
     word on the icon's centre. This used to stretch it to the button's full
     height and centre inside it instead - box centring, which is what left the
     label 0.47px above the icon, and being a flex container it also made the trim
     a no-op. */

  .stable-feedback {
    color: var(--clear);
    flex: none;
    font-size: var(--font-size-meta);
    max-width: 18rem;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .stable-feedback:empty {
    display: none;
  }

  .form-error {
    font-size: var(--font-size-meta);
    margin: 0;
    padding: 0 0 var(--space-3);
  }

  .user-results,
  .invitation-results {
    background: var(--table-filler-bg);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    display: flex;
    flex-direction: column;
    flex: 1;
    margin-top: 0;
    min-height: 0;
    overflow: hidden;
    position: relative;
  }

  .user-results.loading,
  .invitation-results.loading {
    cursor: progress;
  }

  .empty-row td {
    border-bottom: 0;
    height: 12rem;
  }

  .empty-row td :global(.table-empty-state) {
    margin-inline: auto;
  }

  .table-skeleton {
    display: grid;
  }

  .table-skeleton span {
    animation: user-skeleton-pulse 1.35s ease-in-out infinite alternate;
    border-bottom: 1px solid var(--rule);
    display: block;
    height: 3.5rem;
    position: relative;
  }

  .table-skeleton span::before,
  .table-skeleton span::after {
    background: var(--surface-inset);
    border-radius: var(--radius-control);
    content: '';
    height: 0.75rem;
    left: var(--space-4);
    position: absolute;
    top: 1.15rem;
    width: min(13rem, 28%);
  }

  .table-skeleton span::after {
    left: 46%;
    width: min(8rem, 18%);
  }

  @keyframes user-skeleton-pulse {
    from {
      opacity: 0.48;
    }

    to {
      opacity: 0.88;
    }
  }

  /* Surface, keyline, corner and lift come from `.table-card` in `app.css`. */
  .user-table-wrap {
    display: flex;
    flex: 1;
    min-height: 0;
  }

  .user-table-wrap:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }

  .user-table {
    background: var(--surface-base);
    /* Separated, not collapsed: a collapsed border is shared between adjacent
       rows, so each cell owns half of it and every row box lands on a .5. */
    border-collapse: separate;
    border-spacing: 0;
    min-width: 50rem;
    table-layout: fixed;
    width: 100%;
  }

  /* The header's rule and its type come from `thead th` in `app.css`. A
     `font-size` on `th` here would outrank it - a class selector beats two
     element ones - and this table's heading would be 13px against everyone
     else's 11. */
  /* `tbody th` as well as `td`: the identity cell is a row header, and without
     the separator it is a pixel taller than the cells beside it - which centres
     its contents half a pixel lower than the rest of the row. */
  .user-table td,
  .user-table tbody th {
    border-bottom: 1px solid var(--rule);
    font-size: var(--font-size-meta);
  }

  .user-table th,
  .user-table td {
    padding: var(--space-2) var(--space-3);
    text-align: left;
    vertical-align: middle;
  }

  .user-table th:first-child,
  .user-table td:first-child {
    padding-left: var(--space-3);
  }

  .user-table th:last-child,
  .user-table td:last-child {
    padding-right: var(--space-3);
  }

  /* Typography and ground come from `thead th` in `app.css`. */
  .user-table thead th {
    height: 2.5rem;
  }

  .user-table thead th:has(.sort-button) {
    padding: 0;
  }

  .user-table thead th:first-child .sort-button {
    padding-left: var(--space-4);
  }

  .filterable-heading {
    padding-block: 0 !important;
  }

  .table-heading-layout {
    align-items: center;
    display: flex;
    height: 100%;
    justify-content: space-between;
    min-width: 0;
  }

  .table-heading-layout :global(.header-filter) {
    margin-inline: var(--space-1);
  }

  .sort-button {
    align-items: center;
    background: transparent;
    border: 0;
    color: inherit;
    display: flex;
    font: inherit;
    gap: 0.45rem;
    height: 100%;
    justify-content: flex-start;
    letter-spacing: inherit;
    padding: var(--space-2) var(--space-3);
    text-align: left;
    text-transform: inherit;
    transition:
      background-color 120ms ease-out,
      color 120ms ease-out;
    min-width: 0;
    overflow: hidden;
    width: 100%;
  }

  .table-heading-layout .sort-button {
    flex: 1;
    width: auto;
  }

  .sent-heading {
    padding-block: 0 !important;
  }

  .sort-indicator {
    color: var(--text-muted);
    display: grid;
    opacity: 0;
    place-items: center;
    transition: opacity 120ms ease-out;
  }

  .sort-button:hover .sort-indicator,
  .sort-button:focus-visible .sort-indicator {
    opacity: 0.55;
  }

  .sort-indicator.ascending,
  .sort-indicator.descending {
    color: var(--brand-action-text);
    opacity: 1;
  }

  .sort-indicator.descending {
    transform: rotate(180deg);
  }

  .user-table tbody tr:hover {
    background: var(--table-row-hover);
  }

  .user-table tbody tr.history-row {
    cursor: pointer;
    transition:
      background-color var(--duration-fast) var(--ease-standard),
      transform var(--duration-press) var(--ease-standard);
  }

  .user-table tbody tr.history-row:hover {
    background: var(--table-row-hover);
  }

  /* A row that can be pressed acknowledges the press the way every other control
     in the panel does: it steps its ground and gets slightly smaller. The scale
     goes through the same property the virtualiser uses for the row's position,
     which is why that position is a variable - written straight into `transform`
     it would be overwritten here and the row would jump to the top of the list. */
  .user-table tbody tr.history-row {
    transform: translateY(var(--row-y, 0px));
    transform-origin: center;
  }

  .user-table tbody tr.history-row.pressing {
    background: var(--table-row-pressed);
    transform: translateY(var(--row-y, 0px)) scale(var(--press-scale-surface));
  }

  .user-table tbody tr.history-row:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: -2px;
  }

  @media (min-width: 64.001rem) {
    .user-results,
    .invitation-results {
      overflow: hidden;
    }

    .user-table {
      display: flex;
      flex: 1;
      flex-direction: column;
      min-height: 0;
    }

    .user-table thead {
      display: block;
      flex: none;
    }

    .user-table tbody {
      background: var(--table-filler-bg);
      display: block;
      flex: 1;
      min-height: 0;
      overflow-y: auto;
      position: relative;
    }

    .user-table thead tr,
    .user-table tbody tr {
      display: grid;
      grid-template-columns:
        minmax(16rem, 1.55fr) minmax(10rem, 1fr) minmax(8rem, 0.8fr) minmax(9rem, 0.9fr)
        4.25rem;
      width: 100%;
    }

    .invitation-table thead tr,
    .invitation-table tbody tr {
      grid-template-columns:
        minmax(13rem, 1.4fr) minmax(7.5rem, 0.9fr) minmax(7.5rem, 0.8fr) minmax(6.5rem, 0.7fr)
        minmax(7.5rem, 0.8fr) 4.25rem;
    }

    /* The last row keeps its separator - see the note on the same spot in
       RepositoryList. Overscrolling pulls the rows off the table's bottom edge,
       and a last row with no line of its own ends in nothing while it is held
       there. */

    /* In the flow, with a height of its own - see the same rule in
       RepositoryList. A table is as tall as its contents now, and something
       absolutely positioned contributes none, so the message disappeared and
       left a bare header behind it. */
    .user-table tbody tr.empty-row {
      align-content: center;
      grid-template-columns: minmax(0, 1fr);
      min-height: 12rem;
    }

    /* The grid rows above repaint the row ground at a higher specificity than the plain
         `:hover` rule outside this block, so the pointer state has to be restated here or it never
         reaches the screen. */
    /* Not the empty state - see the same rule in RepositoryList. */
    .user-table tbody tr:not(.virtual-spacer, .empty-row):hover {
      background: var(--table-row-hover);
    }

    /* And the press with it, for the same reason and one more: the rule above is
       later in the sheet than the one that paints a held row, and carries the same
       specificity, so without this the row kept its hover colour under the pointer
       while the scale went ahead - which reads as the press half working. */
    .user-table tbody tr.history-row.pressing {
      background: var(--table-row-pressed);
    }

    .user-table tbody tr:not(.virtual-spacer) {
      background: var(--surface-base);
      /* Pin the grid track to the row's fixed height: auto-sizing would take
         the tallest cell's border-box, push the bottom border one pixel past
         the virtual row, and let the next row paint over every separator. */
      grid-template-rows: 100%;
    }

    .user-table tbody tr:not(.virtual-spacer) > th,
    .user-table tbody tr:not(.virtual-spacer) > td {
      align-items: center;
      display: flex;
    }

    .user-table tbody .row-actions {
      justify-content: flex-end;
    }

    .user-table tbody .virtual-row {
      left: 0;
      position: absolute;
      top: 0;
      /* The offset the virtualiser measured. It arrives as a variable so a press
         can add a scale without losing it - see .history-row.pressing. */
      transform: translateY(var(--row-y, 0px));
    }

    .user-table tbody .virtual-spacer {
      background: transparent;
      border: 0;
      display: block;
      pointer-events: none;
      width: 1px;
    }

    .virtual-spacer td {
      display: none;
    }
  }

  .user-identity {
    align-items: center;
    display: flex;
    gap: 0.625rem;
    min-width: 12rem;
  }

  .user-identity > span:last-child {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    min-width: 0;
  }

  .user-identity strong {
    font-size: var(--font-size-body);
    font-weight: 600;
    line-height: 1.2;
  }

  .user-login {
    color: var(--dim);
    font-size: var(--font-size-compact);
    font-weight: 400;
    line-height: 1.2;
  }

  .last-login {
    color: var(--dim);
    font-size: 0.75rem;
  }

  .last-login > time,
  .last-login > span {
    align-items: center;
    display: inline-flex;
    height: var(--control-height-compact);
    line-height: 1;
    vertical-align: middle;
  }

  .user-table tbody :global(.role-trigger) {
    background: transparent;
    border-color: transparent;
    /* Pull the trigger's padding and border back so its icon sits at the
       same x as the fixed-role rows below and above it. */
    margin-left: calc(-0.5rem - 1px);
  }

  .user-table tbody :global(.role-trigger .role-chevron) {
    opacity: 0;
    transition: opacity var(--duration-fast) var(--ease-standard);
  }

  .user-table tbody tr:hover :global(.role-trigger:not(:disabled)),
  .user-table tbody :global(.role-trigger:focus-visible),
  .user-table tbody :global(.role-trigger[aria-expanded='true']) {
    background: var(--control-surface);
    border-color: var(--control-border);
  }

  .user-table tbody tr:hover :global(.role-trigger .role-chevron),
  .user-table tbody :global(.role-trigger:focus-visible .role-chevron),
  .user-table tbody :global(.role-trigger[aria-expanded='true'] .role-chevron) {
    opacity: 1;
  }

  .role-value {
    align-items: center;
    color: var(--text-secondary);
    display: inline-flex;
    font: 600 var(--font-size-compact) / 1 var(--sans);
    gap: var(--space-2);
    min-height: var(--control-height-compact);
    white-space: nowrap;
  }

  .role-value-icon {
    color: var(--text-muted);
    display: grid;
    flex: 0 0 1.125rem;
    place-items: center;
    width: 1.125rem;
  }

  .row-actions {
    gap: var(--space-1);
    text-align: right !important;
    width: 4.25rem;
  }

  .row-actions :global(.action-menu) {
    display: inline-block;
  }

  /* Always there. It is the only thing that separates a row you can open from one
     you cannot, so hiding it until hover answered the question only for people who
     had already asked it. Quiet enough at rest that a column of them reads as a
     margin rather than as a column of arrows, and it leans out on hover. */
  .row-go {
    color: var(--text-muted);
    display: inline-grid;
    opacity: 0.55;
    place-items: center;
    transition:
      opacity var(--duration-fast) var(--ease-standard),
      transform var(--duration-fast) var(--ease-standard);
  }

  tr.history-row:hover .row-go,
  tr.history-row:focus-visible .row-go {
    opacity: 1;
    transform: translateX(2px);
  }

  .action-slot-empty {
    align-items: center;
    color: var(--text-muted);
    display: inline-flex;
    height: 2.5rem;
    justify-content: center;
    opacity: 0.3;
    width: 2.5rem;
  }

  .cell-dash {
    color: var(--text-muted);
    opacity: 0.6;
  }

  .expires-soon {
    color: var(--text-secondary);
    font-weight: 600;
  }

  .invitation-table {
    min-width: 44rem;
  }

  .add-user-form {
    display: grid;
    gap: 0.875rem;
  }

  /* The card keeps its stature whatever its copy measures - trimming the two
     lines to their ink took 5.5px out of the content, and a summary card's
     height is a shape decision, not a consequence of the leading. */
  .add-scope-summary {
    align-items: center;
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: 0.625rem;
    display: grid;
    gap: var(--space-3);
    grid-template-columns: auto minmax(0, 1fr) auto;
    min-height: 4rem;
    padding: var(--space-3);
  }

  .add-scope-icon {
    align-items: center;
    background: var(--brand-action-tint);
    border-radius: 50%;
    /* Self-keyed keyline: the tint fill measures near 1:1 against the box. */
    box-shadow: inset 0 0 0 1px color-mix(in srgb, currentcolor 28%, transparent);
    color: var(--brand-action-text);
    display: inline-flex;
    font-size: 0.8125rem;
    font-weight: 700;
    height: 2rem;
    justify-content: center;
    width: 2rem;
  }

  /* Block, not grid: the name is inline in the mock, so it sits in the body's
     22.5px line box rather than a 18px one of its own. A grid row would size to
     the glyphs and shorten the card. */
  .add-scope-summary > span:nth-child(2) {
    display: block;
  }

  /* Both lines trimmed to cap..baseline, so the copy block's BOX equals its ink
     and the card's centring centres what the eye reads. Untrimmed, the kicker's
     leading above and the name's descender below are not symmetric and the pair
     sat 2.7px above the card's middle. 0.65rem holds the 21.4px between the two
     baselines that the untrimmed pair already had. */
  .add-scope-summary small {
    color: var(--text-muted);
    display: block;
    font: 700 var(--font-size-micro) / 1 var(--sans);
    letter-spacing: 0.06em;
    text-box: trim-both cap alphabetic;
    text-transform: uppercase;
  }

  .add-scope-summary strong {
    display: block;
    font-size: var(--font-size-body);
    line-height: 1;
    margin-top: 0.65rem;
    text-box: trim-both cap alphabetic;
  }

  .method-picker {
    border: 0;
    margin: 0;
    min-width: 0;
    padding: 0;
  }

  .method-picker legend {
    font-size: 0.75rem;
    font-weight: 600;
    margin-bottom: 0.4rem;
    padding: 0;
  }

  .method-options {
    display: grid;
    gap: 0.625rem;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .method-option {
    align-items: center;
    background: var(--well);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    cursor: pointer;
    display: grid;
    gap: 0.6rem;
    grid-template-columns: auto minmax(0, 1fr) auto;
    /* Holds the height the untrimmed copy used to give it; see the scope card. */
    min-height: 3.75rem;
    padding: 0.625rem;
    transition:
      background-color 120ms ease-out,
      border-color 120ms ease-out,
      transform 80ms ease-out;
  }

  .method-option:hover {
    background: var(--strip-lift);
    border-color: color-mix(in srgb, var(--dim) 56%, transparent);
  }

  /* Mixed against the control border, not against transparent: over the tinted
     fill a 60% alpha edge composites lighter than the mock's opaque one. */
  .method-option.selected {
    background: var(--brand-action-tint);
    border-color: color-mix(in srgb, var(--brand-action) 60%, var(--control-border));
  }

  .method-option:has(input:focus-visible) {
    border-color: var(--focus);
    box-shadow: inset 0 0 0 1px var(--focus);
    outline: 0;
  }

  .method-option input {
    height: 1px;
    opacity: 0;
    position: absolute;
    width: 1px;
  }

  /* The circle reads the same selected or not - only the card behind it
     changes. Tinting the fill on selection made the chosen method look like a
     different control from the one beside it. */
  .method-icon {
    align-items: center;
    background: var(--surface-base);
    border-radius: 50%;
    /* Self-keyed keyline, same recipe as every avatar and icon circle. */
    box-shadow: inset 0 0 0 1px color-mix(in srgb, currentcolor 28%, transparent);
    color: var(--brand-action-text);
    display: inline-flex;
    height: 1.75rem;
    justify-content: center;
    width: 1.75rem;
  }

  /* Block, so the description keeps the body line box the mock gives it. */
  .method-copy {
    display: block;
    min-width: 0;
  }

  /* Same treatment as the scope card above: trimmed to ink, spaced by the step
     that keeps the 20.4px the two baselines already had. */
  .method-copy strong {
    display: block;
    font-size: 0.75rem;
    line-height: 1;
    text-box: trim-both cap alphabetic;
  }

  .method-copy small {
    color: var(--dim);
    display: block;
    font-size: var(--font-size-micro);
    line-height: 1;
    margin-top: 0.75rem;
    text-box: trim-both cap alphabetic;
  }

  .method-check {
    align-self: center;
    border: 1.5px solid var(--border-strong);
    border-radius: 50%;
    height: 0.875rem;
    width: 0.875rem;
  }

  /* A filled disc with the surface punched back through it, not a ring with a
     dot drawn inside - the two differ by a pixel at this size. */
  .method-option.selected .method-check {
    background: var(--brand-action);
    border-color: var(--brand-action);
    box-shadow: inset 0 0 0 3px var(--surface-base);
  }

  .form-field {
    /* `start`, not the default `normal`: these fields sit in a row stretched by
       whichever column carries help text, and stretched auto rows share the
       slack out - which pushed the Role and Expires controls a row's worth
       below the login input they are supposed to line up with. */
    align-content: start;
    display: grid;
    gap: 0.4rem;
  }

  .form-field > span {
    font: 600 0.75rem / 1 var(--sans);
  }

  .form-field > span small {
    color: var(--dim);
    font-weight: 400;
  }

  .form-field > small {
    color: var(--dim);
    font-size: 0.6875rem;
  }

  .form-field .text-input,
  .form-field .select-input {
    width: 100%;
  }

  .form-grid {
    display: grid;
    gap: 0.75rem;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .identity-grid {
    display: grid;
    gap: 0.75rem;
    grid-template-columns: minmax(0, 1.75fr) minmax(7.5rem, 0.8fr);
  }

  .identity-grid.with-expiry {
    grid-template-columns: minmax(0, 1.75fr) minmax(6.75rem, 0.9fr) minmax(7.5rem, 0.9fr);
  }

  .identity-help {
    color: var(--dim);
    font-size: 0.6875rem;
    font-weight: 400;
    line-height: 1.35;
    /* The grid gap is 0.4rem; the mock puts 0.35rem above helpers. */
    margin-top: -0.05rem;
  }

  /* The helper says why the button is off rather than a line appearing under it: the reason
     belongs to the field that caused it, and it takes the place of help that no longer applies. */
  .identity-help.refused {
    color: var(--stop);
    font-weight: 500;
  }

  .reason-textarea {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text);
    font: 0.8125rem/1.45 var(--sans);
    min-height: 6rem;
    padding: 0.625rem;
    resize: vertical;
  }

  .reason-textarea::placeholder {
    color: var(--dim);
  }

  .invitation-created,
  .confirmation-note {
    align-items: center;
    background: var(--well);
    border: 1px solid var(--rule);
    border-radius: var(--r-well);
    display: flex;
    gap: 0.75rem;
    padding: 0.75rem;
  }

  /* Both lines trimmed to their ink, so `align-items: center` centres what you can see rather than
     two line boxes carrying half-leading the eye does not read. */
  .invitation-created strong {
    display: block;
    font-size: 0.8125rem;
    text-box: trim-both cap alphabetic;
  }

  .invitation-created p,
  .confirmation-note p {
    color: var(--dim);
    font-size: 0.75rem;
    margin: 0.15rem 0 0;
  }

  .invitation-created p {
    text-box: trim-both cap alphabetic;
  }

  .link-clipboard {
    align-items: center;
    color: var(--warning);
    display: flex;
    font-size: var(--font-size-compact);
    gap: 0.35rem;
    margin: 0.4rem 0 0;
  }

  .success-mark,
  .warning-mark {
    align-items: center;
    border-radius: 50%;
    display: inline-flex;
    flex: none;
    font-weight: 700;
    height: 1.75rem;
    justify-content: center;
    width: 1.75rem;
  }

  /* Both tints sit near 1:1 against the card they are on - measured 1.00 to 1.24 across the four
     palettes - so neither disc had an edge. The ring is keyed to the mark's own colour, as the
     avatar monogram's is. */
  .success-mark,
  .warning-mark {
    box-shadow: inset 0 0 0 1px color-mix(in srgb, currentcolor 28%, transparent);
  }

  .success-mark {
    background: var(--clear-tint);
    color: var(--clear);
  }

  .warning-mark {
    background: var(--stop-tint);
    color: var(--stop);
  }

  .copy-button {
    min-width: 6.75rem;
  }

  @media (max-width: 48rem) {
    .identity-grid.with-expiry {
      grid-template-columns: minmax(0, 1.35fr) repeat(2, minmax(6.5rem, 0.75fr));
    }

    .user-table-wrap {
      overflow: visible;
      padding: var(--space-3);
    }

    .user-table {
      display: block;
      min-width: 0;
    }

    .user-table thead {
      display: block;
    }

    /* Wrapped, because these are four independent chips rather than a row that
       has to stay a row: unwrapped, the last of them was cut off by the card's
       edge with no way to reach it. */
    .user-table thead tr {
      align-items: center;
      border: 0;
      display: flex;
      flex-wrap: wrap;
      gap: var(--space-2);
      padding: 0 0 var(--space-3);
    }

    .user-table thead th {
      display: block;
      padding: 0;
    }

    /* A heading with nothing to operate is a column name for a column that is
       no longer drawn, so it goes. One carrying a *filter* is not that: Status
       has no sort button, only a funnel, and hiding it took the funnel with it
       - on a phone there was no way to filter users or invitations by status at
       all. The control was still in the page, focusable, in a 1px box. */
    .user-table thead th:not(:has(.sort-button)):not(:has(.filter-trigger)) {
      clip-path: inset(50%);
      height: 1px;
      overflow: hidden;
      position: absolute;
      white-space: nowrap;
      width: 1px;
    }

    /* Dressed as the sort chips beside it: it does the same job in the same row,
       and the border is what makes either read as something to press. */
    .user-table thead th.filterable-heading .table-heading-layout {
      background: var(--control-bg);
      border: 1px solid var(--control-border);
      border-radius: var(--radius-control);
      gap: var(--space-1);
      height: var(--control-height-compact);
      padding-inline: var(--space-3) var(--space-1);
    }

    .user-table thead .sort-button {
      background: var(--control-bg);
      border: 1px solid var(--control-border);
      border-radius: var(--radius-control);
      color: var(--dim);
      height: var(--control-height-compact);
      padding: 0 var(--space-3);
    }

    .user-table thead .sort-button:hover,
    .user-table thead .sort-button:focus-visible {
      background: var(--control-bg-hover);
      color: var(--text);
    }

    .user-table tbody {
      display: grid;
      gap: var(--space-2);
    }

    .user-table tbody tr {
      background: var(--surface-raised);
      border: 1px solid var(--border-subtle);
      border-radius: var(--radius-control);
      display: grid;
      gap: var(--space-3);
      grid-template-columns: repeat(2, minmax(0, 1fr));
      padding: var(--space-3);
      position: relative;
    }

    .user-table th,
    .user-table td {
      border: 0;
      display: grid;
      gap: var(--space-1);
      padding: 0;
    }

    .user-table tbody th {
      grid-column: 1 / -1;
    }

    .user-table td:not(.row-actions)::before {
      color: var(--text-muted);
      content: attr(data-label);
      font: 650 var(--font-size-compact) / 1 var(--sans);
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }

    .user-table .row-actions {
      position: absolute;
      right: var(--space-3);
      top: var(--space-3);
    }

    .user-table .user-identity {
      min-width: 0;
      padding-right: 2.5rem;
    }
  }

  @media (max-width: 36rem) {
    .management-navigation {
      gap: var(--space-1);
    }

    .add-scope-summary {
      grid-template-columns: auto minmax(0, 1fr);
    }

    .form-grid,
    .identity-grid,
    .identity-grid.with-expiry {
      grid-template-columns: minmax(0, 1fr);
    }

    .method-options {
      grid-template-columns: minmax(0, 1fr);
    }
  }

  @media (max-width: 22rem) {
    .user-table tbody tr {
      grid-template-columns: minmax(0, 1fr);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .method-option:active {
      transform: none;
    }
  }
</style>
