<script lang="ts">
  import { formatDateTime, formatRelative, formatTimestamp } from '../lib/format';
  import type { FilterSection } from '../lib/filter-menu';
  import type {
    AccessDecision,
    AddGlobalInvitationInput,
    AddGlobalUserInput,
    AddTargetInvitationInput,
    AddTargetUserInput,
    InvitationDays,
    InvitationPageRequest,
    InvitationSort,
    InvitationStatus,
    Page,
    PanelInvitation,
    PanelRole,
    PanelUser,
    PanelUserListStatus,
    PanelUserPageRequest,
    PanelUserSort,
    UpdateGlobalUserInput,
    UpdateTargetUserInput,
  } from '../lib/types';
  import ActionMenu, { type ActionMenuItem } from './ActionMenu.svelte';
  import Avatar from './Avatar.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import DecisionHistory from './DecisionHistory.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import Icon, { type IconName } from './Icon.svelte';
  import Modal from './Modal.svelte';
  import PaginationBar from './PaginationBar.svelte';
  import PanelHeader from './PanelHeader.svelte';
  import RolePicker, { type RolePickerOption } from './RolePicker.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  type UserScope = 'global' | 'target';
  type ManagementSection = 'users' | 'invitations';
  type SortDirection = 'ascending' | 'descending';
  type UserSortColumn = 'name' | 'last_login';
  type InvitationSortColumn = 'name' | 'expires';
  type UserAction = 'ban' | 'remove' | 'suspend' | 'restore' | 'unban' | 'remove_access';
  type TargetRole = Exclude<PanelRole, 'owner'>;
  type GrantedTargetRole = Exclude<TargetRole, 'none'>;

  const ACCESS_METHODS = [
    { value: 'add', label: 'Add directly', description: 'Grant access immediately' },
    { value: 'invite', label: 'Send invitation', description: 'Create a single-use link' },
  ] as const;

  const ROLE_FILTERS: FilterSection[] = [
    {
      label: 'Roles',
      options: [
        { value: 'owner', label: 'Owner' },
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
        { value: 'owner', label: 'Owner' },
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
        { value: 'declined', label: 'Declined', tone: 'invalid' },
        { value: 'revoked', label: 'Revoked', tone: 'invalid' },
      ],
    },
  ];

  const {
    scope,
    targetId,
    targetName,
    actorTargetRole,
    canManageGlobal,
    canManageOwners,
    refreshVersion = 0,
    onScope,
    fetchUsers,
    addUser,
    updateUser,
    fetchTargetUsers,
    addTargetUser,
    updateTargetUser,
    fetchInvitations,
    createInvitation,
    fetchTargetInvitations,
    createTargetInvitation,
    reissueInvitation,
    revokeInvitation,
    fetchUserDecisions,
  }: {
    scope: UserScope;
    targetId: string;
    targetName: string;
    actorTargetRole: PanelRole;
    canManageGlobal: boolean;
    canManageOwners: boolean;
    refreshVersion?: number;
    onScope: (targetId: string | null) => void;
    fetchUsers: (request: PanelUserPageRequest) => Promise<Page<PanelUser>>;
    addUser: (input: AddGlobalUserInput) => Promise<PanelUser>;
    updateUser: (accountId: string, input: UpdateGlobalUserInput) => Promise<PanelUser>;
    fetchTargetUsers: (targetId: string, request: PanelUserPageRequest) => Promise<Page<PanelUser>>;
    addTargetUser: (targetId: string, input: AddTargetUserInput) => Promise<PanelUser>;
    updateTargetUser: (
      targetId: string,
      accountId: string,
      input: UpdateTargetUserInput,
    ) => Promise<PanelUser>;
    fetchInvitations: (request: InvitationPageRequest) => Promise<Page<PanelInvitation>>;
    createInvitation: (input: AddGlobalInvitationInput) => Promise<PanelInvitation>;
    fetchTargetInvitations: (
      targetId: string,
      request: InvitationPageRequest,
    ) => Promise<Page<PanelInvitation>>;
    createTargetInvitation: (
      targetId: string,
      input: AddTargetInvitationInput,
    ) => Promise<PanelInvitation>;
    reissueInvitation: (
      invitationId: string,
      expiresInDays: InvitationDays,
    ) => Promise<PanelInvitation>;
    revokeInvitation: (invitationId: string) => Promise<PanelInvitation>;
    fetchUserDecisions: (accountId: string, targetId?: string) => Promise<AccessDecision[]>;
  } = $props();

  let userPage = $state<Page<PanelUser> | null>(null);
  let invitationPage = $state<Page<PanelInvitation> | null>(null);
  let loadingUsers = $state(true);
  let loadingInvitations = $state(true);
  let userFailure = $state<string | null>(null);
  let invitationFailure = $state<string | null>(null);
  let actionFailure = $state<string | null>(null);
  let feedback = $state('');

  let userSearch = $state('');
  let userQuery = $state('');
  let userSort = $state<PanelUserSort>('name_asc');
  let userRoles = $state<PanelRole[]>([]);
  let userStatuses = $state<PanelUserListStatus[]>([]);
  let userLimit = $state(20);
  let userPageIndex = $state(0);

  let invitationSearch = $state('');
  let invitationQuery = $state('');
  let invitationSort = $state<InvitationSort>('name_asc');
  let invitationRoles = $state<Exclude<PanelRole, 'none'>[]>([]);
  let invitationStatuses = $state<InvitationStatus[]>([]);
  let invitationLimit = $state(20);
  let invitationPageIndex = $state(0);
  let activeSection = $state<ManagementSection>('users');

  let addModalOpen = $state(false);
  let addButton = $state<HTMLButtonElement | null>(null);
  let addReturnFocus = $state<HTMLElement | null>(null);
  let addScopeTargetId = $state<string | null>(null);
  let login = $state('');
  let addRole = $state<PanelRole>('viewer');
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

  let userLoadVersion = 0;
  let invitationLoadVersion = 0;
  const now = Date.now();

  const users = $derived(userPage?.items ?? []);
  const invitations = $derived(invitationPage?.items ?? []);
  const userPageCount = $derived(Math.max(1, Math.ceil((userPage?.total ?? 0) / userLimit)));
  const invitationPageCount = $derived(
    Math.max(1, Math.ceil((invitationPage?.total ?? 0) / invitationLimit)),
  );
  const failure = $derived(
    actionFailure ?? (activeSection === 'users' ? userFailure : invitationFailure),
  );
  const scopeOptions = $derived([
    { value: 'global', label: 'Global', tone: 'accent' as const },
    { value: 'target', label: targetName, tone: 'accent' as const },
  ]);
  const addRoleOptions = $derived(
    addRoles().map((role) => ({ value: role, label: roleLabel(role), icon: roleIcon(role) })),
  );
  const userRequestKey = $derived(
    JSON.stringify([
      scope,
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
      scope,
      targetId,
      invitationQuery,
      invitationSort,
      invitationRoles,
      invitationStatuses,
      invitationLimit,
      refreshVersion,
    ]),
  );
  const userFilterSections = $derived<FilterSection[]>([
    ...ROLE_FILTERS,
    {
      label: 'Status',
      options: [
        { value: 'active', label: 'Active', tone: 'valid' },
        { value: 'banned', label: 'Banned', tone: 'invalid' },
        ...(scope === 'target'
          ? [{ value: 'suspended', label: 'Suspended', tone: 'bypassed' as const }]
          : []),
      ],
    },
  ]);

  $effect(() => {
    const value = userSearch;
    const timeout = window.setTimeout(() => (userQuery = value.trim()), 180);
    return () => window.clearTimeout(timeout);
  });

  $effect(() => {
    const value = invitationSearch;
    const timeout = window.setTimeout(() => (invitationQuery = value.trim()), 180);
    return () => window.clearTimeout(timeout);
  });

  $effect(() => {
    const requestKey = userRequestKey;
    userPageIndex = 0;
    void loadUsers(0, requestKey);
  });

  $effect(() => {
    const requestKey = invitationRequestKey;
    invitationPageIndex = 0;
    void loadInvitations(0, requestKey);
  });

  async function loadUsers(index: number, _requestKey = userRequestKey): Promise<void> {
    if (_requestKey !== userRequestKey) return;
    const version = ++userLoadVersion;
    const requestedScope = scope;
    const requestedTarget = targetId;
    loadingUsers = true;
    userFailure = null;
    const request: PanelUserPageRequest = {
      ...(index === 0 ? {} : { cursor: String(index * userLimit) }),
      query: userQuery,
      sort: userSort,
      limit: userLimit,
      roles: [...userRoles],
      statuses: [...userStatuses],
    };
    try {
      const listed =
        requestedScope === 'global'
          ? await fetchUsers(request)
          : await fetchTargetUsers(requestedTarget, request);
      if (version !== userLoadVersion) return;
      const lastIndex = Math.max(0, Math.ceil(listed.total / userLimit) - 1);
      if (index > lastIndex) {
        await loadUsers(lastIndex, _requestKey);
        return;
      }
      userPage = listed;
      userPageIndex = index;
    } catch (error) {
      if (version === userLoadVersion) userFailure = errorMessage(error);
    } finally {
      if (version === userLoadVersion) loadingUsers = false;
    }
  }

  async function loadInvitations(index: number, _requestKey = invitationRequestKey): Promise<void> {
    if (_requestKey !== invitationRequestKey) return;
    const version = ++invitationLoadVersion;
    const requestedScope = scope;
    const requestedTarget = targetId;
    loadingInvitations = true;
    invitationFailure = null;
    const request: InvitationPageRequest = {
      ...(index === 0 ? {} : { cursor: String(index * invitationLimit) }),
      query: invitationQuery,
      sort: invitationSort,
      limit: invitationLimit,
      roles: [...invitationRoles],
      statuses: [...invitationStatuses],
    };
    try {
      const listed =
        requestedScope === 'global'
          ? await fetchInvitations(request)
          : await fetchTargetInvitations(requestedTarget, request);
      if (version !== invitationLoadVersion) return;
      const lastIndex = Math.max(0, Math.ceil(listed.total / invitationLimit) - 1);
      if (index > lastIndex) {
        await loadInvitations(lastIndex, _requestKey);
        return;
      }
      invitationPage = listed;
      invitationPageIndex = index;
    } catch (error) {
      if (version === invitationLoadVersion) invitationFailure = errorMessage(error);
    } finally {
      if (version === invitationLoadVersion) loadingInvitations = false;
    }
  }

  function clearUserFilters(): void {
    userSearch = '';
    userQuery = '';
    userRoles = [];
    userStatuses = [];
  }

  function clearInvitationFilters(): void {
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
    const selectedTargetId = addScopeTargetId;
    const destination = selectedTargetId === null ? 'global access' : targetName;
    try {
      if (accessMethod === 'invite') {
        const created =
          selectedTargetId === null
            ? await createInvitation({
                login: normalizedLogin,
                role: addRole as AddGlobalInvitationInput['role'],
                target_id: targetId,
                expires_in_days: expiresInDays,
              })
            : await createTargetInvitation(selectedTargetId, {
                login: normalizedLogin,
                role: addRole as AddTargetInvitationInput['role'],
                expires_in_days: expiresInDays,
              });
        generatedLink = created.invite_url ?? '';
        feedback = `Invited @${normalizedLogin} to ${destination}`;
        if (addScopeMatchesCurrent()) await loadInvitations(0);
      } else if (selectedTargetId === null) {
        await addUser({ login: normalizedLogin, role: addRole, target_id: targetId });
        feedback = `Added @${normalizedLogin} to global access`;
        closeAddModal();
        if (scope === 'global') await loadUsers(0);
      } else {
        await addTargetUser(selectedTargetId, {
          login: normalizedLogin,
          role: addRole as GrantedTargetRole,
        });
        feedback = `Added @${normalizedLogin} to ${destination}`;
        closeAddModal();
        if (scope === 'target' && selectedTargetId === targetId) await loadUsers(0);
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
      if (scope === 'global') {
        await updateUser(user.account.id, {
          global_role: value as PanelRole,
          status: user.status,
          ban_reason: user.ban_reason,
          expected_revision: user.revision,
        });
      } else {
        const targetAccess = requiredTargetAccess(user);
        await updateTargetUser(targetId, user.account.id, {
          role: value === 'inherit' ? null : (value as TargetRole),
          suspended: targetAccess.suspended,
          suspension_reason: targetAccess.suspension_reason,
          expected_revision: targetAccess.revision,
        });
      }
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
      if (action === 'ban') {
        await updateUser(user.account.id, {
          global_role: user.global_role,
          status: 'banned',
          ban_reason: reason.trim() || undefined,
          expected_revision: user.revision,
        });
        feedback = `Banned @${user.account.login}`;
      } else if (action === 'remove') {
        await updateUser(user.account.id, {
          global_role: user.global_role,
          status: 'removed',
          expected_revision: user.revision,
        });
        feedback = `Removed @${user.account.login}`;
      } else if (action === 'unban') {
        await updateUser(user.account.id, {
          global_role: user.global_role,
          status: 'active',
          expected_revision: user.revision,
        });
        feedback = `Unbanned @${user.account.login}`;
      } else {
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
      }
    });
  }

  async function mutate(user: PanelUser, operation: () => Promise<void>): Promise<void> {
    savingAccount = user.account.id;
    actionFailure = null;
    try {
      await operation();
      cancelAction();
      await Promise.all([loadUsers(userPageIndex), loadInvitations(invitationPageIndex)]);
    } catch (error) {
      await loadUsers(userPageIndex);
      actionFailure = errorMessage(error);
    } finally {
      savingAccount = null;
    }
  }

  async function reissue(invitation: PanelInvitation, trigger: HTMLElement | null): Promise<void> {
    invitationBusy = invitation.id;
    actionFailure = null;
    try {
      const updated = await reissueInvitation(invitation.id, 7);
      generatedLink = updated.invite_url ?? '';
      accessMethod = 'invite';
      addScopeTargetId = scope === 'global' ? null : targetId;
      addReturnFocus = trigger;
      addModalOpen = true;
      feedback = `Reissued invitation for @${invitation.account.login}`;
      await loadInvitations(invitationPageIndex);
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
      await revokeInvitation(invitation.id);
      feedback = `Revoked invitation for @${invitation.account.login}`;
      pendingInvitation = null;
      await loadInvitations(invitationPageIndex);
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
    addScopeTargetId = scope === 'global' ? null : targetId;
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

  function addScopeMatchesCurrent(): boolean {
    return addScopeTargetId === null
      ? scope === 'global'
      : scope === 'target' && addScopeTargetId === targetId;
  }

  function selectSection(section: ManagementSection): void {
    activeSection = section;
  }

  function moveSection(event: KeyboardEvent): void {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    const next = event.key === 'ArrowLeft' || event.key === 'Home' ? 'users' : 'invitations';
    activeSection = next;
    queueMicrotask(() => document.querySelector<HTMLButtonElement>(`#${next}-list-tab`)?.focus());
  }

  function selectScopeMode(value: string): void {
    if (value === 'global') {
      onScope(null);
    } else if (value === 'target') {
      onScope(targetId);
    }
  }

  function selectUserSort(column: UserSortColumn): void {
    userSort =
      column === 'name'
        ? userSort === 'name_asc'
          ? 'name_desc'
          : 'name_asc'
        : userSort === 'login_newest'
          ? 'login_oldest'
          : 'login_newest';
  }

  function selectInvitationSort(column: InvitationSortColumn): void {
    invitationSort =
      column === 'name'
        ? invitationSort === 'name_asc'
          ? 'name_desc'
          : 'name_asc'
        : invitationSort === 'expiry_soonest'
          ? 'expiry_latest'
          : 'expiry_soonest';
  }

  function userSortDirection(column: UserSortColumn): SortDirection | undefined {
    if (column === 'name') {
      if (userSort === 'name_asc') return 'ascending';
      if (userSort === 'name_desc') return 'descending';
      return undefined;
    }
    if (userSort === 'login_oldest') return 'ascending';
    if (userSort === 'login_newest') return 'descending';
    return undefined;
  }

  function invitationSortDirection(column: InvitationSortColumn): SortDirection | undefined {
    if (column === 'name') {
      if (invitationSort === 'name_asc') return 'ascending';
      if (invitationSort === 'name_desc') return 'descending';
      return undefined;
    }
    if (invitationSort === 'expiry_soonest') return 'ascending';
    if (invitationSort === 'expiry_latest') return 'descending';
    return undefined;
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

  function selectUserPage(index: number): void {
    if (loadingUsers || index === userPageIndex) return;
    userPageIndex = Math.min(userPageCount - 1, Math.max(0, index));
    void loadUsers(userPageIndex);
  }

  function selectInvitationPage(index: number): void {
    if (loadingInvitations || index === invitationPageIndex) return;
    invitationPageIndex = Math.min(invitationPageCount - 1, Math.max(0, index));
    void loadInvitations(invitationPageIndex);
  }

  function userActionItems(user: PanelUser): ActionMenuItem[] {
    if (!user.manageable) return [];
    if (scope === 'global') {
      return [
        user.status === 'banned'
          ? {
              id: 'unban',
              icon: 'success',
              label: 'Unban user',
              description: 'Restore global access',
            }
          : {
              id: 'ban',
              icon: 'ban',
              label: 'Ban user',
              description: 'Suspend all panel access',
              tone: 'danger',
            },
        {
          id: 'remove',
          icon: 'trash',
          label: 'Remove user',
          description: 'Revoke roles and active invitations',
          tone: 'danger',
        },
      ];
    }
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
    if (invitation.status !== 'pending' && invitation.status !== 'expired') return [];
    return [
      {
        id: 'reissue',
        icon: 'refresh',
        label: 'Reissue invitation',
        description: 'Create a new 7-day link',
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

  function addRoles(selectedTargetId = addScopeTargetId): PanelRole[] {
    if (selectedTargetId === null) {
      return canManageOwners
        ? ['viewer', 'editor', 'admin', 'owner']
        : ['viewer', 'editor', 'admin'];
    }
    return actorTargetRole === 'owner' ? ['viewer', 'editor', 'admin'] : ['viewer', 'editor'];
  }

  function targetRoleOptions(): Array<{ value: string; label: string }> {
    const options = [
      { value: 'inherit', label: 'Inherit global' },
      { value: 'none', label: 'No access' },
      { value: 'viewer', label: 'Viewer' },
      { value: 'editor', label: 'Editor' },
    ];
    if (actorTargetRole === 'owner') options.push({ value: 'admin', label: 'Admin' });
    return options;
  }

  function globalRoleOptions(): Array<{ value: PanelRole; label: string }> {
    const options: Array<{ value: PanelRole; label: string }> = [
      { value: 'none', label: 'No access' },
      { value: 'viewer', label: 'Viewer' },
      { value: 'editor', label: 'Editor' },
      { value: 'admin', label: 'Admin' },
    ];
    if (canManageOwners) options.push({ value: 'owner', label: 'Owner' });
    return options;
  }

  function selectableRoleOptions(user: PanelUser): RolePickerOption[] {
    if (scope === 'global') {
      return globalRoleOptions().map((option) => ({
        ...option,
        icon: roleIcon(option.value),
      }));
    }
    return targetRoleOptions().map((option) => ({
      ...option,
      icon:
        option.value === 'inherit'
          ? roleIcon(user.global_role)
          : roleIcon(option.value as PanelRole),
    }));
  }

  function selectedRole(user: PanelUser): string {
    if (scope === 'global') return user.global_role;
    return user.target_access?.role ?? 'inherit';
  }

  function shownRole(user: PanelUser): PanelRole {
    return scope === 'global'
      ? user.global_role
      : (user.target_access?.effective_role ?? user.global_role);
  }

  function statusLabel(user: PanelUser): string {
    if (user.status === 'banned') return 'Banned';
    if (scope === 'target' && user.target_access?.suspended === true) return 'Suspended';
    return 'Active';
  }

  function statusTone(user: PanelUser): ChipTone {
    return user.status === 'banned' || user.target_access?.suspended === true ? 'stop' : 'clear';
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
    return 'stop';
  }

  function invitationStatusLabel(status: InvitationStatus): string {
    return status.charAt(0).toUpperCase() + status.slice(1);
  }

  function roleLabel(role: PanelRole): string {
    if (role === 'none') return 'No access';
    return role[0]?.toLocaleUpperCase() + role.slice(1);
  }

  function roleIcon(role: PanelRole): IconName {
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
    userRoles = values.filter((value): value is PanelRole =>
      ['owner', 'admin', 'editor', 'viewer', 'none'].includes(value),
    );
    userStatuses = values.filter((value): value is PanelUserListStatus =>
      ['active', 'banned', 'suspended'].includes(value),
    );
  }

  function selectInvitationFilters(values: string[]): void {
    invitationRoles = values.filter((value): value is Exclude<PanelRole, 'none'> =>
      ['owner', 'admin', 'editor', 'viewer'].includes(value),
    );
    invitationStatuses = values.filter((value): value is InvitationStatus =>
      ['pending', 'accepted', 'expired', 'declined', 'revoked'].includes(value),
    );
  }

  function actionTitle(): string {
    const login = actionUser?.account.login ?? '';
    switch (pendingAction) {
      case 'ban':
        return `Ban @${login}`;
      case 'remove':
        return `Remove @${login}`;
      case 'suspend':
        return `Suspend @${login}`;
      case 'restore':
        return `Restore @${login}`;
      case 'unban':
        return `Unban @${login}`;
      case 'remove_access':
        return `Remove @${login} from ${targetName}`;
      default:
        return 'Confirm access change';
    }
  }

  function actionDescription(): string {
    switch (pendingAction) {
      case 'ban':
        return 'This immediately revokes every active session and blocks panel access';
      case 'remove':
        return 'This removes all roles, installation overrides, and pending invitations';
      case 'suspend':
        return `This blocks access to ${targetName} until an administrator restores it`;
      case 'restore':
      case 'unban':
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
  <button class="sort-button" type="button" onclick={onSelect}>
    <span>{label}</span>
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

{#snippet roleBadge(role: PanelRole)}
  <span class="role-badge role-{role}">
    <Icon name={roleIcon(role)} size={14} />
    <span>{roleLabel(role)}</span>
  </span>
{/snippet}

{#snippet headerActions()}
  {#if canManageGlobal}
    <div class="scope-mode">
      <span>Access scope</span>
      <SegmentedControl
        name="user-access-scope"
        label="Access scope"
        options={scopeOptions}
        value={scope}
        align="end"
        compact
        onSelect={selectScopeMode}
      />
    </div>
  {:else}
    <span class="scope-context">
      <Icon name="organization" size={16} />
      <span>{targetName}</span>
    </span>
  {/if}
{/snippet}

<section class="plate user-management" aria-labelledby="user-management-heading">
  <PanelHeader
    id="user-management-heading"
    title="Users"
    description="Manage roles, invitations, and access decisions"
    actions={headerActions}
  />

  <div class="user-management-body">
    <div class="management-navigation">
      <div class="section-tabs" role="tablist" aria-label="User management lists">
        <button
          id="users-list-tab"
          class="section-tab"
          class:selected={activeSection === 'users'}
          type="button"
          role="tab"
          aria-selected={activeSection === 'users'}
          aria-controls="users-list-panel"
          tabindex={activeSection === 'users' ? 0 : -1}
          onclick={() => selectSection('users')}
          onkeydown={moveSection}
        >
          <span>Users</span>
          <span class="section-count">{userPage?.total ?? '…'}</span>
        </button>
        <button
          id="invitations-list-tab"
          class="section-tab"
          class:selected={activeSection === 'invitations'}
          type="button"
          role="tab"
          aria-selected={activeSection === 'invitations'}
          aria-controls="invitations-list-panel"
          tabindex={activeSection === 'invitations' ? 0 : -1}
          onclick={() => selectSection('invitations')}
          onkeydown={moveSection}
        >
          <span>Invitations</span>
          <span class="section-count">{invitationPage?.total ?? '…'}</span>
        </button>
      </div>
      <div class="stable-feedback" aria-live="polite">{feedback}</div>
      <button
        class="btn btn-signal tab-add"
        type="button"
        bind:this={addButton}
        onclick={openAddModal}
      >
        <Icon name="user-plus" size={17} />
        <span class="button-label">Add user</span>
      </button>
    </div>

    {#if failure !== null}<p class="form-error" role="alert">{failure}</p>{/if}

    {#if activeSection === 'users'}
      <div id="users-list-panel" role="tabpanel" aria-labelledby="users-list-tab">
        <div class="management-toolbar" aria-label="User list controls">
          <SearchField
            label="Search users"
            placeholder="Search users"
            value={userSearch}
            onInput={(value) => (userSearch = value)}
          />
          <FilterMenu
            label="User filters"
            summary={filterSummary(userRoles.length + userStatuses.length)}
            hint="Filter by role or access status"
            sections={userFilterSections}
            selected={[...userRoles, ...userStatuses]}
            multiple
            align="end"
            wide
            showIcon
            onChange={selectUserFilters}
          />
        </div>

        <div class:loading={loadingUsers} class="user-results" aria-busy={loadingUsers}>
          {#if loadingUsers && userPage === null}
            <div class="table-skeleton" aria-hidden="true">
              {#each [0, 1, 2, 3, 4, 5] as index (index)}
                <span></span>
              {/each}
            </div>
            <p class="visually-hidden" role="status">Loading users</p>
          {:else if users.length === 0}
            {@const hasUserFilters =
              userQuery !== '' || userRoles.length > 0 || userStatuses.length > 0}
            <div class="result-state dim">
              <strong>{hasUserFilters ? 'No users match' : 'No users in this scope'}</strong>
              <span>
                {hasUserFilters
                  ? 'Try another search or clear the current filters'
                  : 'Added users will appear here'}
              </span>
              {#if hasUserFilters}
                <button class="btn" type="button" onclick={clearUserFilters}>Clear filters</button>
              {/if}
            </div>
          {:else}
            <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
            <div class="user-table-wrap" role="region" aria-label="Panel users" tabindex="0">
              <table class="user-table">
                <caption class="visually-hidden">
                  Panel users. Select a sortable column header to change the sort order.
                </caption>
                <thead>
                  <tr>
                    <th aria-sort={userSortDirection('name')}>
                      {@render sortButton('User', userSortDirection('name'), () =>
                        selectUserSort('name'),
                      )}
                    </th>
                    <th>Role</th>
                    <th>Status</th>
                    <th aria-sort={userSortDirection('last_login')}>
                      {@render sortButton('Last login', userSortDirection('last_login'), () =>
                        selectUserSort('last_login'),
                      )}
                    </th>
                    <th><span class="visually-hidden">Actions</span></th>
                  </tr>
                </thead>
                <tbody>
                  {#each users as user (user.account.id)}
                    <tr
                      class:history-row={hasDecisionHistory(user)}
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
                        {#if user.manageable}
                          <RolePicker
                            label="Role for {user.account.login}"
                            value={selectedRole(user)}
                            options={selectableRoleOptions(user)}
                            disabled={savingAccount === user.account.id}
                            onSelect={(value) => void changeRole(user, value)}
                          />
                        {:else}
                          {@render roleBadge(shownRole(user))}
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
                        {#if user.manageable}
                          <ActionMenu
                            label={`Actions for @${user.account.login}`}
                            items={userActionItems(user)}
                            onSelect={(action, trigger) =>
                              beginAction(user, action as UserAction, trigger ?? undefined)}
                          />
                        {/if}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>

        <PaginationBar
          label="Users"
          pageIndex={userPageIndex}
          pageCount={userPageCount}
          pageSize={userLimit}
          itemCount={users.length}
          total={userPage?.total ?? 0}
          disabled={loadingUsers}
          onPageSelect={selectUserPage}
          onPageSizeSelect={(value) => (userLimit = value)}
        />
      </div>
    {:else}
      <div id="invitations-list-panel" role="tabpanel" aria-labelledby="invitations-list-tab">
        <div class="management-toolbar" aria-label="Invitation list controls">
          <SearchField
            label="Search invitations"
            placeholder="Search invitations"
            value={invitationSearch}
            onInput={(value) => (invitationSearch = value)}
          />
          <FilterMenu
            label="Invitation filters"
            summary={filterSummary(invitationRoles.length + invitationStatuses.length)}
            hint="Filter by role or invitation status"
            sections={[...INVITATION_ROLE_FILTERS, ...INVITATION_STATUS_FILTERS]}
            selected={[...invitationRoles, ...invitationStatuses]}
            multiple
            align="end"
            wide
            showIcon
            onChange={selectInvitationFilters}
          />
        </div>

        <div
          class:loading={loadingInvitations}
          class="invitation-results"
          aria-busy={loadingInvitations}
        >
          {#if loadingInvitations && invitationPage === null}
            <div class="table-skeleton" aria-hidden="true">
              {#each [0, 1, 2, 3, 4, 5] as index (index)}
                <span></span>
              {/each}
            </div>
            <p class="visually-hidden" role="status">Loading invitations</p>
          {:else if invitations.length === 0}
            {@const hasInvitationFilters =
              invitationQuery !== '' || invitationRoles.length > 0 || invitationStatuses.length > 0}
            <div class="result-state dim">
              <strong>
                {hasInvitationFilters ? 'No invitations match' : 'No invitations in this scope'}
              </strong>
              <span>
                {hasInvitationFilters
                  ? 'Try another search or clear the current filters'
                  : 'New invitations will appear here'}
              </span>
              {#if hasInvitationFilters}
                <button class="btn" type="button" onclick={clearInvitationFilters}
                  >Clear filters</button
                >
              {/if}
            </div>
          {:else}
            <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
            <div class="user-table-wrap" role="region" aria-label="Panel invitations" tabindex="0">
              <table class="user-table invitation-table">
                <caption class="visually-hidden">
                  Panel invitations. Select a sortable column header to change the sort order.
                </caption>
                <thead>
                  <tr>
                    <th aria-sort={invitationSortDirection('name')}>
                      {@render sortButton('User', invitationSortDirection('name'), () =>
                        selectInvitationSort('name'),
                      )}
                    </th>
                    <th>Role</th>
                    <th>Status</th>
                    <th aria-sort={invitationSortDirection('expires')}>
                      {@render sortButton('Expires', invitationSortDirection('expires'), () =>
                        selectInvitationSort('expires'),
                      )}
                    </th>
                    <th><span class="visually-hidden">Actions</span></th>
                  </tr>
                </thead>
                <tbody>
                  {#each invitations as invitation (invitation.id)}
                    <tr>
                      <th scope="row">
                        <span class="user-identity">
                          <Avatar account={invitation.account} size={32} />
                          <span>
                            <strong>{invitation.account.display_name}</strong>
                            <span class="user-login mono">@{invitation.account.login}</span>
                          </span>
                        </span>
                      </th>
                      <td data-label="Role">{@render roleBadge(invitation.role)}</td>
                      <td data-label="Status"
                        ><Chip tone={invitationTone(invitation.status)} dot
                          >{invitationStatusLabel(invitation.status)}</Chip
                        ></td
                      >
                      <td class="last-login" data-label="Expires">
                        <time
                          datetime={invitation.expires_at}
                          title={formatTimestamp(invitation.expires_at)}
                        >
                          {formatDateTime(invitation.expires_at)}
                        </time>
                      </td>
                      <td class="row-actions" data-label="Actions">
                        {#if invitationActionItems(invitation).length > 0}
                          <ActionMenu
                            label={`Actions for @${invitation.account.login} invitation`}
                            items={invitationActionItems(invitation)}
                            onSelect={(action, trigger) =>
                              chooseInvitationAction(invitation, action, trigger)}
                          />
                        {/if}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>

        <PaginationBar
          label="Invitations"
          pageIndex={invitationPageIndex}
          pageCount={invitationPageCount}
          pageSize={invitationLimit}
          itemCount={invitations.length}
          total={invitationPage?.total ?? 0}
          disabled={loadingInvitations}
          onPageSelect={selectInvitationPage}
          onPageSizeSelect={(value) => (invitationLimit = value)}
        />
      </div>
    {/if}
  </div>
</section>

{#if historyUser !== null}
  <DecisionHistory
    open
    label={`Access details for @${historyUser.account.login}`}
    scopeLabel={scope === 'global' ? 'Global' : targetName}
    status={statusLabel(historyUser)}
    reason={currentReason(historyUser)}
    decidedAt={currentDecisionAt(historyUser)}
    returnFocus={historyTrigger}
    fetchDecisions={() =>
      fetchUserDecisions(historyUser!.account.id, scope === 'target' ? targetId : undefined)}
    onClose={closeHistory}
  />
{/if}

<Modal
  id="add-user"
  open={addModalOpen}
  title="Add user"
  description="Grant access now or send a secure invitation"
  returnFocus={addReturnFocus}
  onClose={closeAddModal}
>
  <form id="add-user-form" class="add-user-form" onsubmit={submitAdd}>
    {#if generatedLink === ''}
      <div class="add-scope-summary">
        <span class="add-scope-icon" aria-hidden="true">
          <Icon name={addScopeTargetId === null ? 'globe' : 'organization'} size={18} />
        </span>
        <span>
          <small>Access scope</small>
          <strong>{addScopeTargetId === null ? 'Global' : targetName}</strong>
        </span>
        <small>Scope follows the Users page</small>
      </div>

      <fieldset class="method-picker">
        <legend>Access method</legend>
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
                <Icon name={method.value === 'add' ? 'user-plus' : 'pending'} size={18} />
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
        </label>
        <div class="form-field">
          <span>Role</span>
          <RolePicker
            label="Role"
            value={addRole}
            options={addRoleOptions}
            variant="field"
            onSelect={(value) => (addRole = value as PanelRole)}
          />
        </div>
        {#if accessMethod === 'invite'}
          <label class="form-field">
            <span>Expires after</span>
            <select class="select-input" bind:value={expiresInDays} aria-label="Invitation expiry">
              <option value={1}>1 day</option>
              <option value={7}>7 days</option>
              <option value={30}>30 days</option>
            </select>
          </label>
        {/if}
      </div>
      <small class="identity-help">
        {accessMethod === 'invite'
          ? 'The invitation only works for this GitHub identity'
          : 'GitHub login identifies the account to add'}
      </small>
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
    <button class="btn" type="button" onclick={closeAddModal}>
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
            ? 'Inviting…'
            : 'Adding…'
          : accessMethod === 'invite'
            ? 'Create invitation'
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
  {#if pendingAction === 'ban' || pendingAction === 'suspend'}
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
    <button class="btn" type="button" data-modal-focus onclick={cancelAction}>Cancel</button>
    <button
      class="btn"
      class:btn-stop={pendingAction !== 'restore' && pendingAction !== 'unban'}
      class:btn-signal={pendingAction === 'restore' || pendingAction === 'unban'}
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
    <button class="btn" type="button" data-modal-focus onclick={() => (pendingInvitation = null)}
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
  .management-toolbar {
    align-items: center;
    display: flex;
  }

  .user-management {
    --local-control-height: var(--control-height-compact);

    background: transparent;
    border: 0;
    border-radius: 0;
    box-shadow: none;
    margin-bottom: 0;
    overflow: visible;
  }

  .user-management-body {
    background: transparent;
    border: 0;
    border-radius: 0;
    min-width: 0;
    overflow: visible;
  }

  .scope-mode {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }

  .scope-mode > span {
    color: var(--text-muted);
    font: 600 var(--font-size-compact) / 1 var(--sans);
  }

  .scope-context {
    align-items: center;
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    color: var(--text-secondary);
    display: inline-flex;
    font: 600 var(--font-size-compact) / 1 var(--sans);
    gap: var(--space-2);
    height: var(--control-height-compact);
    padding: 0 var(--space-3);
  }

  .management-navigation {
    align-items: center;
    display: flex;
    gap: var(--space-3);
    margin-bottom: var(--space-3);
    min-height: var(--control-height);
  }

  .section-tabs {
    align-items: center;
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    display: flex;
    gap: var(--control-inset);
    padding: var(--control-inset);
  }

  .section-tab {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: calc(var(--radius-control) - var(--control-inset));
    color: var(--text-muted);
    display: inline-flex;
    font: 650 var(--font-size-body) / 1 var(--sans);
    gap: var(--space-2);
    height: var(--control-height-compact);
    min-width: 0;
    padding: 0 var(--space-3);
    transition:
      background-color var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out),
      color var(--duration-fast) var(--ease-out);
  }

  .section-tab:hover {
    background: var(--interactive-hover);
    color: var(--text-primary);
  }

  .section-tab.selected {
    background: var(--surface-base);
    box-shadow: 0 1px 2px var(--shadow-color);
    color: var(--text-primary);
  }

  .section-tab:focus-visible {
    box-shadow: inset 0 0 0 2px var(--focus);
    outline: 0;
  }

  .section-count {
    color: var(--text-muted);
    font: 600 var(--font-size-compact) / 1 var(--mono);
    font-variant-numeric: tabular-nums;
  }

  .section-tab.selected .section-count {
    color: var(--brand-action-text);
  }

  .tab-add {
    height: var(--control-height-compact);
    margin-left: 0;
  }

  .button-label {
    align-items: center;
    display: inline-flex;
    height: 100%;
    line-height: 1;
  }

  .management-toolbar {
    background: transparent;
    border: 0;
    flex-wrap: wrap;
    gap: var(--space-2);
    padding: 0 0 var(--space-3);
  }

  .management-toolbar :global(.search-field) {
    flex: 1 1 15rem;
  }

  .stable-feedback {
    color: var(--clear);
    font-size: var(--font-size-meta);
    margin-left: auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .form-error {
    font-size: var(--font-size-meta);
    margin: 0;
    padding: 0 0 var(--space-3);
  }

  .user-results,
  .invitation-results {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-bottom: 0;
    border-radius: var(--radius-surface) var(--radius-surface) 0 0;
    margin-top: 0;
    overflow: hidden;
  }

  .user-management :global(.pagination-bar) {
    border: 1px solid var(--border-subtle);
    border-radius: 0 0 var(--radius-surface) var(--radius-surface);
  }

  .user-results.loading,
  .invitation-results.loading {
    cursor: progress;
  }

  .result-state {
    align-items: center;
    border: 1px dashed var(--rule);
    border-radius: var(--r-well);
    display: flex;
    flex-direction: column;
    font-size: 0.8125rem;
    gap: var(--space-2);
    justify-content: center;
    margin: var(--space-4);
    padding: 1.5rem;
    text-align: center;
  }

  .result-state strong {
    color: var(--text);
  }

  .table-skeleton {
    display: grid;
  }

  .table-skeleton span {
    animation: user-skeleton-pulse 1.35s ease-in-out infinite alternate;
    border-bottom: 1px solid var(--rule);
    display: block;
    height: 3.625rem;
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
    overflow-x: auto;
  }

  .user-table-wrap:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }

  .user-table {
    border-collapse: collapse;
    min-width: 50rem;
    width: 100%;
  }

  .user-table th,
  .user-table td {
    border-bottom: 1px solid var(--rule);
    padding: var(--space-3);
    text-align: left;
    vertical-align: middle;
  }

  .user-table thead th {
    background: var(--table-header-bg);
    color: var(--dim);
    font: 650 var(--font-size-compact) / 1.2 var(--sans);
    letter-spacing: 0.02em;
  }

  .user-table thead th:has(.sort-button) {
    padding: 0;
  }

  .sort-button {
    align-items: center;
    background: transparent;
    border: 0;
    color: inherit;
    display: flex;
    font: inherit;
    gap: 0.45rem;
    justify-content: flex-start;
    letter-spacing: inherit;
    padding: var(--space-3);
    text-align: left;
    text-transform: inherit;
    transition:
      background-color 120ms ease-out,
      color 120ms ease-out;
    width: 100%;
  }

  .sort-button:hover,
  .sort-button:focus-visible {
    background: var(--interactive-hover);
    color: var(--text);
  }

  .sort-indicator {
    color: var(--text-muted);
    display: grid;
    place-items: center;
  }

  .sort-indicator.ascending,
  .sort-indicator.descending {
    color: var(--brand-action-text);
  }

  .sort-indicator.descending {
    transform: rotate(180deg);
  }

  .user-table tbody tr:last-child th,
  .user-table tbody tr:last-child td {
    border-bottom: 0;
  }

  .user-table tbody tr:hover {
    background: var(--table-row-hover);
  }

  .user-table tbody tr.history-row {
    cursor: pointer;
  }

  .user-table tbody tr.history-row:hover {
    background: var(--strip-lift);
  }

  .user-table tbody tr.history-row:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: -2px;
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
    transform: translateY(-1px);
    vertical-align: middle;
  }

  .role-badge {
    align-items: center;
    background: var(--surface-inset);
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    color: var(--text);
    display: inline-flex;
    font: 600 var(--font-size-compact) / 1 var(--sans);
    gap: 0.45rem;
    min-height: 1.875rem;
    padding: 0 0.55rem;
    white-space: nowrap;
  }

  .role-owner {
    background: var(--surface-inset);
    border-color: var(--rule);
    color: var(--text);
  }

  .row-actions {
    text-align: right !important;
    width: 2.75rem;
  }

  .row-actions :global(.action-menu) {
    display: inline-block;
  }

  .invitation-table {
    min-width: 44rem;
  }

  .add-user-form {
    display: grid;
    gap: 0.875rem;
  }

  .add-scope-summary {
    align-items: center;
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-surface);
    display: grid;
    gap: var(--space-3);
    grid-template-columns: auto minmax(0, 1fr) auto;
    padding: var(--space-3);
  }

  .add-scope-icon {
    align-items: center;
    background: var(--brand-action-tint);
    border-radius: 50%;
    color: var(--brand-action-text);
    display: inline-flex;
    height: 2rem;
    justify-content: center;
    width: 2rem;
  }

  .add-scope-summary > span:nth-child(2) {
    display: grid;
    gap: var(--space-1);
  }

  .add-scope-summary small {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  .add-scope-summary strong {
    font-size: var(--font-size-body);
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
    gap: 0.625rem;
    grid-template-columns: auto minmax(0, 1fr) auto;
    min-height: 3.5rem;
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

  .method-option.selected {
    background: var(--brand-action-tint);
    border-color: color-mix(in srgb, var(--brand-action) 60%, transparent);
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

  .method-icon {
    align-items: center;
    background: var(--strip-lift);
    border-radius: 50%;
    color: var(--dim);
    display: inline-flex;
    height: 1.75rem;
    justify-content: center;
    width: 1.75rem;
  }

  .method-option.selected .method-icon {
    background: color-mix(in srgb, var(--brand-action) 18%, transparent);
    color: var(--brand-action-text);
  }

  .method-copy {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .method-copy strong {
    font-size: 0.75rem;
    line-height: 1.3;
  }

  .method-copy small {
    color: var(--dim);
    font-size: 0.625rem;
    line-height: 1.35;
  }

  .method-check {
    border: 1px solid var(--control-border);
    border-radius: 50%;
    height: 0.875rem;
    position: relative;
    width: 0.875rem;
  }

  .method-option.selected .method-check {
    border-color: var(--brand-action);
  }

  .method-option.selected .method-check::after {
    background: var(--brand-action);
    border-radius: 50%;
    content: '';
    inset: 2px;
    position: absolute;
  }

  .form-field {
    display: grid;
    gap: 0.4rem;
  }

  .form-field > span {
    font-size: 0.75rem;
    font-weight: 600;
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
    grid-template-columns: minmax(0, 1.55fr) minmax(6.75rem, 0.72fr) minmax(7.5rem, 0.8fr);
  }

  .identity-help {
    color: var(--dim);
    font-size: 0.6875rem;
    margin-top: -0.5rem;
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
    .scope-mode {
      width: 100%;
    }

    .scope-mode :global(fieldset) {
      flex: 1;
    }

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

    .section-tab {
      padding-inline: var(--space-2);
    }

    .tab-add {
      font-size: 0;
      gap: 0;
      padding-inline: 0.625rem;
    }

    .add-scope-summary {
      grid-template-columns: auto minmax(0, 1fr);
    }

    .add-scope-summary > small:last-child {
      grid-column: 2;
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
