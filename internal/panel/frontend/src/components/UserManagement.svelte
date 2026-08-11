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
  import { MediaQuery } from 'svelte/reactivity';
  import { get } from 'svelte/store';

  import { formatDateTime, formatRelative, formatTimestamp, formatUntil } from '../lib/format';
  import { monogram } from '../lib/identity';
  import type { FilterSection } from '../lib/filter-menu';
  import {
    EPHEMERAL_PREFS,
    prefList,
    prefOption,
    prefText,
    type PrefsAccessor,
  } from '../lib/preferences-sync';
  import type {
    AccessDecision,
    AddTargetInvitationInput,
    AddTargetUserInput,
    InvitationDays,
    InvitationPageRequest,
    InvitationSort,
    InvitationStatus,
    Page,
    PanelInvitation,
    InstallationRole,
    PanelUser,
    PanelUserListStatus,
    PanelUserPageRequest,
    PanelUserSort,
    UpdateTargetUserInput,
  } from '../lib/types';
  import ActionMenu, { type ActionMenuItem } from './ActionMenu.svelte';
  import Avatar from './Avatar.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import DecisionHistory from './DecisionHistory.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import Icon, { type IconName } from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import Modal from './Modal.svelte';
  import PanelHeader from './PanelHeader.svelte';
  import RolePicker, { type RolePickerOption } from './RolePicker.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  type ManagementSection = 'users' | 'invitations';
  type SortDirection = 'ascending' | 'descending';
  type UserSortColumn = 'name' | 'role' | 'last_login';
  type InvitationSortColumn = 'name' | 'role' | 'expires';
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
    actorTargetRole,
    refreshVersion = 0,
    readOnly = false,
    onSection,
    fetchTargetUsers,
    addTargetUser,
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
    actorTargetRole: InstallationRole;
    refreshVersion?: number;
    readOnly?: boolean;
    onSection: (section: ManagementSection) => void;
    fetchTargetUsers: (targetId: string, request: PanelUserPageRequest) => Promise<Page<PanelUser>>;
    addTargetUser: (targetId: string, input: AddTargetUserInput) => Promise<PanelUser>;
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

  let userPage = $state<Page<PanelUser> | null>(null);
  let invitationPage = $state<Page<PanelInvitation> | null>(null);
  let loadingUsers = $state(true);
  let loadingInvitations = $state(true);
  let userFailure = $state<string | null>(null);
  let invitationFailure = $state<string | null>(null);
  let userLoadMoreFailure = $state<string | null>(null);
  let invitationLoadMoreFailure = $state<string | null>(null);
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

  let addModalOpen = $state(false);
  let addButton = $state<HTMLButtonElement | null>(null);
  let addReturnFocus = $state<HTMLElement | null>(null);
  let login = $state('');
  let addRole = $state<InstallationRole>('viewer');
  let accessMethod = $state<'add' | 'invite'>('add');
  let expiresInDays = $state<InvitationDays>(7);
  let generatedLink = $state('');
  let adding = $state(false);

  let actionUser = $state<PanelUser | null>(null);
  let pendingAction = $state<UserAction | null>(null);
  let actionTrigger = $state<HTMLElement | null>(null);
  let reason = $state('');
  let pendingInvitation = $state<PanelInvitation | null>(null);
  let invitationActionTrigger = $state<HTMLElement | null>(null);
  let invitationBusy = $state<string | null>(null);
  let savingAccount = $state<string | null>(null);
  let historyUser = $state<PanelUser | null>(null);
  let historyTrigger = $state<HTMLElement | null>(null);
  let userResults = $state<HTMLDivElement>();
  let invitationResults = $state<HTMLDivElement>();
  let userScroll = $state<HTMLTableSectionElement>();
  let invitationScroll = $state<HTMLTableSectionElement>();

  let userLoadVersion = 0;
  let invitationLoadVersion = 0;
  // Ticks so "5 minutes ago" keeps aging in a long session; a captured
  // timestamp would freeze every relative time at first render.
  let now = $state(Date.now());

  const users = $derived(userPage?.items ?? []);
  const invitations = $derived(invitationPage?.items ?? []);
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
  const userRequestKey = $derived(
    JSON.stringify([
      targetId,
      userQuery,
      userSort,
      userRoles,
      userStatuses,
      userLimit,
      refreshVersion,
    ]),
  );
  const invitationRequestKey = $derived(
    JSON.stringify([
      targetId,
      invitationQuery,
      invitationSort,
      invitationRoles,
      invitationStatuses,
      invitationLimit,
      refreshVersion,
    ]),
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

  $effect(() => {
    const tick = setInterval(() => {
      now = Date.now();
    }, 30_000);
    return () => clearInterval(tick);
  });

  $effect(() => {
    const value = userSearch;
    const timeout = window.setTimeout(() => {
      userQuery = value.trim();
      scrollResultsToTop(userResults);
    }, 180);
    return () => window.clearTimeout(timeout);
  });

  $effect(() => {
    const value = invitationSearch;
    const timeout = window.setTimeout(() => {
      invitationQuery = value.trim();
      scrollResultsToTop(invitationResults);
    }, 180);
    return () => window.clearTimeout(timeout);
  });

  $effect(() => {
    const requestKey = userRequestKey;
    userLoadMoreFailure = null;
    scrollResultsToTop(userResults);
    void loadUsers(undefined, false, requestKey);
  });

  $effect(() => {
    const requestKey = invitationRequestKey;
    invitationLoadMoreFailure = null;
    scrollResultsToTop(invitationResults);
    void loadInvitations(undefined, false, requestKey);
  });

  $effect(() => {
    const rows = userTableRows;
    get(userVirtualizer).setOptions({
      count: desktopTableLayout.current ? rows.length : 0,
      getScrollElement: () => userScroll ?? null,
      getItemKey: (index) => rows[index]?.id ?? index,
    });
  });

  $effect(() => {
    const rows = invitationTableRows;
    get(invitationVirtualizer).setOptions({
      count: desktopTableLayout.current ? rows.length : 0,
      getScrollElement: () => invitationScroll ?? null,
      getItemKey: (index) => rows[index]?.id ?? index,
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
    if (activeSection === 'users') void loadNextUsers();
    else void loadNextInvitations();
  });

  async function loadUsers(
    cursor: string | undefined,
    append: boolean,
    _requestKey = userRequestKey,
  ): Promise<void> {
    if (_requestKey !== userRequestKey) return;
    const version = ++userLoadVersion;
    const requestedTarget = targetId;
    loadingUsers = true;
    if (!append) userFailure = null;
    else userLoadMoreFailure = null;
    const request: PanelUserPageRequest = {
      ...(cursor === undefined ? {} : { cursor }),
      query: userQuery,
      sort: userSort,
      limit: userLimit,
      roles: [...userRoles],
      statuses: [...userStatuses],
    };
    try {
      const listed = await fetchTargetUsers(requestedTarget, request);
      if (version !== userLoadVersion) return;
      userPage =
        append && userPage !== null
          ? { ...listed, items: [...userPage.items, ...listed.items] }
          : listed;
    } catch (error) {
      if (version === userLoadVersion) {
        if (append) userLoadMoreFailure = errorMessage(error);
        else userFailure = errorMessage(error);
      }
    } finally {
      if (version === userLoadVersion) loadingUsers = false;
    }
  }

  async function loadInvitations(
    cursor: string | undefined,
    append: boolean,
    _requestKey = invitationRequestKey,
  ): Promise<void> {
    if (_requestKey !== invitationRequestKey) return;
    const version = ++invitationLoadVersion;
    const requestedTarget = targetId;
    loadingInvitations = true;
    if (!append) invitationFailure = null;
    else invitationLoadMoreFailure = null;
    const request: InvitationPageRequest = {
      ...(cursor === undefined ? {} : { cursor }),
      query: invitationQuery,
      sort: invitationSort,
      limit: invitationLimit,
      roles: [...invitationRoles],
      statuses: [...invitationStatuses],
    };
    try {
      const listed = await fetchTargetInvitations(requestedTarget, request);
      if (version !== invitationLoadVersion) return;
      invitationPage =
        append && invitationPage !== null
          ? { ...listed, items: [...invitationPage.items, ...listed.items] }
          : listed;
    } catch (error) {
      if (version === invitationLoadVersion) {
        if (append) invitationLoadMoreFailure = errorMessage(error);
        else invitationFailure = errorMessage(error);
      }
    } finally {
      if (version === invitationLoadVersion) loadingInvitations = false;
    }
  }

  async function loadNextUsers(): Promise<void> {
    const cursor = userPage?.next_cursor;
    if (loadingUsers || cursor === null || cursor === undefined) return;
    await loadUsers(cursor, true);
  }

  async function loadNextInvitations(): Promise<void> {
    const cursor = invitationPage?.next_cursor;
    if (loadingInvitations || cursor === null || cursor === undefined) return;
    await loadInvitations(cursor, true);
  }

  async function reloadUsers(): Promise<void> {
    userPage = null;
    scrollResultsToTop(userResults);
    await loadUsers(undefined, false);
  }

  async function reloadInvitations(): Promise<void> {
    invitationPage = null;
    scrollResultsToTop(invitationResults);
    await loadInvitations(undefined, false);
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
    const normalizedLogin = login.trim();
    if (normalizedLogin === '') return;
    adding = true;
    actionFailure = null;
    const destination = targetName;
    try {
      if (accessMethod === 'invite') {
        const created = await createTargetInvitation(targetId, {
          login: normalizedLogin,
          role: addRole as AddTargetInvitationInput['role'],
          expires_in_days: expiresInDays,
        });
        generatedLink = created.invite_url ?? '';
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
      actionFailure = errorMessage(error);
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
    actionUser = user;
    pendingAction = action;
    actionTrigger = trigger ?? null;
    reason = '';
  }

  function cancelAction(): void {
    actionUser = null;
    pendingAction = null;
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
      addReturnFocus = trigger;
      addModalOpen = true;
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
      pendingInvitation = null;
      await reloadInvitations();
    } catch (error) {
      actionFailure = errorMessage(error);
    } finally {
      invitationBusy = null;
    }
  }

  async function copyGeneratedLink(): Promise<void> {
    if (generatedLink === '') return;
    try {
      await navigator.clipboard.writeText(generatedLink);
      feedback = 'Copied invitation link';
    } catch {
      actionFailure = 'The invitation link could not be copied';
    }
  }

  function openAddModal(): void {
    generatedLink = '';
    addRole = 'viewer';
    accessMethod = 'add';
    addReturnFocus = addButton;
    addModalOpen = true;
  }

  function closeAddModal(): void {
    addModalOpen = false;
    generatedLink = '';
    login = '';
  }

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

  function openHistory(user: PanelUser, trigger: HTMLElement): void {
    if (!hasDecisionHistory(user)) return;
    historyUser = user;
    historyTrigger = trigger;
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
    historyUser = null;
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
      pendingInvitation = invitation;
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
    <span>{roleLabel(role)}</span>
  </span>
{/snippet}

{#snippet headerActions()}
  <button class="btn btn-signal" type="button" bind:this={addButton} onclick={openAddModal}>
    <Icon name="user-plus" size={14} strokeWidth={2} />
    <span class="button-label">Add user</span>
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
      <SegmentedControl
        name="user-management-list"
        label="User management lists"
        options={sectionOptions}
        value={activeSection}
        variant="navigation"
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
            <div class="result-state" role="alert">
              <strong>Users could not be loaded</strong>
              <span>{userFailure}</span>
              <button class="btn" onclick={() => void loadUsers(undefined, false)}>Try again</button
              >
            </div>
          {:else}
            <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
            <div class="user-table-wrap" role="region" aria-label="Panel users" tabindex="0">
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
                          showIcon
                          iconOnly
                          placement="header"
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
                          showIcon
                          iconOnly
                          placement="header"
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
                    <tr
                      class:virtual-row={virtualRow.virtual}
                      class:history-row={hasDecisionHistory(user)}
                      style:height={virtualRow.virtual ? `${virtualRow.size}px` : undefined}
                      style:transform={virtualRow.virtual
                        ? `translateY(${virtualRow.start}px)`
                        : undefined}
                      tabindex={hasDecisionHistory(user) ? 0 : undefined}
                      onclick={(event) => clickHistoryRow(event, user)}
                      onkeydown={(event) => keyHistoryRow(event, user)}
                    >
                      <th scope="row">
                        <span class="user-identity">
                          <Avatar account={user.account} size={32} />
                          <span>
                            <strong>{user.account.display_name}</strong>
                            <span class="user-login mono">@{user.account.login}</span>
                            {#if hasDecisionHistory(user)}
                              <span class="visually-hidden">
                                Select this row to review access decision history
                              </span>
                            {/if}
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
                          <span class="dim">Never</span>
                        {:else}
                          <time
                            datetime={user.last_login_at}
                            title={formatTimestamp(user.last_login_at)}
                          >
                            {formatRelative(user.last_login_at, now)}
                          </time>
                        {/if}
                      </td>
                      <td class="row-actions" data-label="Actions">
                        {#if hasDecisionHistory(user)}
                          <span class="row-go" aria-hidden="true">
                            <Icon name="chevron-right" size={14} />
                          </span>
                        {/if}
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
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
          <InfiniteLoadSentinel
            active={!desktopTableLayout.current && !loadingUsers && userPage?.next_cursor != null}
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
            <div class="result-state" role="alert">
              <strong>Invitations could not be loaded</strong>
              <span>{invitationFailure}</span>
              <button class="btn" onclick={() => void loadInvitations(undefined, false)}>
                Try again
              </button>
            </div>
          {:else}
            <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
            <div class="user-table-wrap" role="region" aria-label="Panel invitations" tabindex="0">
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
                          showIcon
                          iconOnly
                          placement="header"
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
                          showIcon
                          iconOnly
                          placement="header"
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
                          <span>
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
                          {formatRelative(invitation.created_at, now)}
                        </time>
                      </td>
                      <td class="last-login" data-label="Expires">
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
    fetchDecisions={() => fetchUserDecisions(historyUser!.account.id, targetId)}
    onClose={closeHistory}
  />
{/if}

<Modal
  id="add-user"
  open={addModalOpen}
  title="Add user"
  description="Grant access now or send a single-use invitation"
  returnFocus={addReturnFocus}
  onClose={closeAddModal}
>
  <form id="add-user-form" class="add-user-form" onsubmit={submitAdd}>
    {#if generatedLink === ''}
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
        <label class="form-field login-field">
          <span>GitHub login</span>
          <input
            class="text-input"
            autocomplete="off"
            placeholder="octocat"
            bind:value={login}
            required
            data-modal-focus
          />
          <small class="identity-help">
            {accessMethod === 'invite'
              ? 'The invitation only works for this GitHub identity'
              : 'GitHub login identifies the account to add'}
          </small>
        </label>
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
    {/if}
  </form>

  {#snippet footer()}
    <button class="btn btn-ghost" type="button" onclick={closeAddModal}>
      {generatedLink === '' ? 'Cancel' : 'Done'}
    </button>
    {#if generatedLink === ''}
      <button
        class="btn btn-signal"
        type="submit"
        form="add-user-form"
        disabled={adding || login.trim() === ''}
      >
        {adding
          ? accessMethod === 'invite'
            ? 'Sending…'
            : 'Adding…'
          : accessMethod === 'invite'
            ? 'Send invitation'
            : 'Add user'}
      </button>
    {:else}
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
  id="user-action"
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
  id="invitation-action"
  open={pendingInvitation !== null}
  title={`Revoke invitation for @${pendingInvitation?.account.login ?? ''}`}
  description="The current link will stop working immediately and the audit record will remain"
  returnFocus={invitationActionTrigger}
  onClose={() => (pendingInvitation = null)}
>
  <div class="confirmation-note">
    <span class="warning-mark" aria-hidden="true">!</span>
    <p>The user can only join if you create and share a new invitation</p>
  </div>

  {#snippet footer()}
    <button
      class="btn btn-ghost"
      type="button"
      data-modal-focus
      onclick={() => (pendingInvitation = null)}>Cancel</button
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

  .button-label {
    align-items: center;
    display: inline-flex;
    height: 100%;
    line-height: 1;
  }

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

  .result-state {
    align-items: center;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    justify-content: center;
    min-height: 9rem;
    padding: 1.5rem;
    text-align: center;
  }

  .result-state span {
    color: var(--dim);
    font-size: var(--font-size-meta);
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

  .user-table-wrap {
    background: var(--surface-base);
    display: flex;
    flex: 1;
    min-height: 0;
    overflow-x: auto;
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

  .user-table th,
  .user-table td {
    border-bottom: 1px solid var(--rule);
    font-size: var(--font-size-meta);
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

  .user-table thead th {
    background: var(--table-header-bg);
    color: var(--dim);
    font: 650 var(--font-size-compact) / 1.2 var(--sans);
    height: 2.5rem;
    letter-spacing: 0.02em;
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
      overscroll-behavior-y: contain;
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

    .user-table tbody tr.empty-row {
      align-content: center;
      grid-template-columns: minmax(0, 1fr);
      inset: 0;
      position: absolute;
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

  .row-go {
    color: var(--text-muted);
    display: inline-grid;
    opacity: 0;
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

  .method-option:active {
    transform: translateY(1px);
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

  .invitation-created strong {
    font-size: 0.8125rem;
  }

  .invitation-created p,
  .confirmation-note p {
    color: var(--dim);
    font-size: 0.75rem;
    margin: 0.15rem 0 0;
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

    .user-table thead tr {
      align-items: center;
      border: 0;
      display: flex;
      gap: var(--space-2);
      padding: 0 0 var(--space-3);
    }

    .user-table thead th {
      display: block;
      padding: 0;
    }

    .user-table thead th:not(:has(.sort-button)) {
      clip-path: inset(50%);
      height: 1px;
      overflow: hidden;
      position: absolute;
      white-space: nowrap;
      width: 1px;
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
