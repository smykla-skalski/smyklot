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
    PanelTarget,
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
  import Modal from './Modal.svelte';
  import PageNavigation from './PageNavigation.svelte';
  import PageSizeSelect from './PageSizeSelect.svelte';
  import Plate from './Plate.svelte';
  import ScopePicker from './ScopePicker.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  type UserScope = 'global' | 'target';
  type UserAction = 'ban' | 'remove' | 'suspend' | 'restore' | 'unban' | 'remove_access';
  type TargetRole = Exclude<PanelRole, 'owner'>;
  type GrantedTargetRole = Exclude<TargetRole, 'none'>;

  const ACCESS_METHODS = [
    { value: 'add', label: 'Add directly' },
    { value: 'invite', label: 'Send invitation' },
  ] as const;

  const ROLE_FILTERS: FilterSection[] = [
    {
      options: [
        { value: 'owner', label: 'Owner', tone: 'bypassed' },
        { value: 'admin', label: 'Admin', tone: 'bypassed' },
        { value: 'editor', label: 'Editor', tone: 'default' },
        { value: 'viewer', label: 'Viewer', tone: 'valid' },
        { value: 'none', label: 'No access', tone: 'default' },
      ],
    },
  ];

  const INVITATION_ROLE_FILTERS: FilterSection[] = [
    {
      options: [
        { value: 'owner', label: 'Owner', tone: 'bypassed' },
        { value: 'admin', label: 'Admin', tone: 'bypassed' },
        { value: 'editor', label: 'Editor', tone: 'default' },
        { value: 'viewer', label: 'Viewer', tone: 'valid' },
      ],
    },
  ];

  const INVITATION_STATUS_FILTERS: FilterSection[] = [
    {
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
    targets,
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
    targets: readonly PanelTarget[];
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
  let invitationSort = $state<InvitationSort>('created_newest');
  let invitationRoles = $state<Exclude<PanelRole, 'none'>[]>([]);
  let invitationStatuses = $state<InvitationStatus[]>([]);
  let invitationLimit = $state(20);
  let invitationPageIndex = $state(0);

  let addModalOpen = $state(false);
  let addButton = $state<HTMLButtonElement | null>(null);
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
  let invitationBusy = $state<string | null>(null);
  let savingAccount = $state<string | null>(null);

  let userLoadVersion = 0;
  let invitationLoadVersion = 0;
  const now = Date.now();

  const users = $derived(userPage?.items ?? []);
  const invitations = $derived(invitationPage?.items ?? []);
  const userPageCount = $derived(Math.max(1, Math.ceil((userPage?.total ?? 0) / userLimit)));
  const invitationPageCount = $derived(
    Math.max(1, Math.ceil((invitationPage?.total ?? 0) / invitationLimit)),
  );
  const failure = $derived(actionFailure ?? userFailure ?? invitationFailure);
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
  const userStatusFilters = $derived<FilterSection[]>([
    {
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

  async function submitAdd(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const normalizedLogin = login.trim();
    if (normalizedLogin === '') return;
    adding = true;
    actionFailure = null;
    try {
      if (accessMethod === 'invite') {
        const created =
          scope === 'global'
            ? await createInvitation({
                login: normalizedLogin,
                role: addRole as AddGlobalInvitationInput['role'],
                target_id: targetId,
                expires_in_days: expiresInDays,
              })
            : await createTargetInvitation(targetId, {
                login: normalizedLogin,
                role: addRole as AddTargetInvitationInput['role'],
                expires_in_days: expiresInDays,
              });
        generatedLink = created.invite_url ?? '';
        feedback = `Invited @${normalizedLogin}`;
        await loadInvitations(0);
      } else if (scope === 'global') {
        await addUser({ login: normalizedLogin, role: addRole, target_id: targetId });
        feedback = `Added @${normalizedLogin}`;
        closeAddModal();
        await loadUsers(0);
      } else {
        await addTargetUser(targetId, {
          login: normalizedLogin,
          role: addRole as GrantedTargetRole,
        });
        feedback = `Added @${normalizedLogin}`;
        closeAddModal();
        await loadUsers(0);
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

  async function reissue(invitation: PanelInvitation): Promise<void> {
    invitationBusy = invitation.id;
    actionFailure = null;
    try {
      const updated = await reissueInvitation(invitation.id, 7);
      generatedLink = updated.invite_url ?? '';
      accessMethod = 'invite';
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
    addModalOpen = true;
  }

  function closeAddModal(): void {
    addModalOpen = false;
    generatedLink = '';
    login = '';
  }

  function selectScope(nextTarget: string | null): void {
    onScope(nextTarget);
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
          ? { id: 'unban', label: 'Unban user', description: 'Restore global access' }
          : {
              id: 'ban',
              label: 'Ban user',
              description: 'Suspend all panel access',
              tone: 'danger',
            },
        {
          id: 'remove',
          label: 'Remove user',
          description: 'Revoke roles and active invitations',
          tone: 'danger',
        },
      ];
    }
    return [
      user.target_access?.suspended === true
        ? { id: 'restore', label: 'Restore access', description: `Allow access to ${targetName}` }
        : {
            id: 'suspend',
            label: 'Suspend access',
            description: `Block access to ${targetName}`,
            tone: 'danger',
          },
      {
        id: 'remove_access',
        label: 'Remove access',
        description: 'Set the installation role to No access',
        tone: 'danger',
      },
    ];
  }

  function invitationActionItems(invitation: PanelInvitation): ActionMenuItem[] {
    if (invitation.status !== 'pending' && invitation.status !== 'expired') return [];
    return [
      { id: 'reissue', label: 'Reissue invitation', description: 'Create a new 7-day link' },
      {
        id: 'revoke',
        label: 'Revoke invitation',
        description: 'Invalidate this invitation',
        tone: 'danger',
      },
    ];
  }

  function chooseInvitationAction(invitation: PanelInvitation, action: string): void {
    if (action === 'reissue') {
      void reissue(invitation);
    } else if (action === 'revoke') {
      pendingInvitation = invitation;
    }
  }

  function requiredTargetAccess(user: PanelUser): NonNullable<PanelUser['target_access']> {
    const access = user.target_access;
    if (access === undefined) throw new Error('installation access is missing');
    return access;
  }

  function addRoles(): PanelRole[] {
    if (scope === 'global') {
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

  function roleLabel(role: PanelRole): string {
    if (role === 'none') return 'No access';
    return role[0]?.toLocaleUpperCase() + role.slice(1);
  }

  function selectedSummary(values: readonly string[], fallback: string): string {
    if (values.length === 0) return fallback;
    if (values.length === 1) return roleLabel(values[0] as PanelRole);
    return `${values.length} selected`;
  }

  function resultSummary(page: Page<unknown> | null, index: number, limit: number): string {
    if (page === null || page.total === 0) return '0 results';
    const start = index * limit + 1;
    const end = Math.min(start + page.items.length - 1, page.total);
    return `${start}–${end} of ${page.total}`;
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

{#snippet headerActions()}
  <div class="header-actions">
    <button class="btn btn-signal" type="button" bind:this={addButton} onclick={openAddModal}>
      <span class="add-icon" aria-hidden="true"></span>
      Add user
    </button>
    <ScopePicker
      global={scope === 'global'}
      {targetId}
      {targets}
      canSelectGlobal={canManageGlobal}
      onSelect={selectScope}
    />
  </div>
{/snippet}

<Plate label="Users" status={headerActions}>
  <div class="management-toolbar" aria-label="User list controls">
    <label class="search-field">
      <span class="visually-hidden">Search users</span>
      <span class="search-icon" aria-hidden="true"></span>
      <input class="text-input" type="search" placeholder="Search users" bind:value={userSearch} />
    </label>
    <FilterMenu
      label="Roles"
      summary={selectedSummary(userRoles, 'All roles')}
      hint="Show users with any selected role"
      sections={ROLE_FILTERS}
      selected={userRoles}
      multiple
      onChange={(values) => (userRoles = values as PanelRole[])}
    />
    <FilterMenu
      label="Status"
      summary={userStatuses.length === 0 ? 'All statuses' : `${userStatuses.length} selected`}
      hint="Show users with any selected status"
      sections={userStatusFilters}
      selected={userStatuses}
      multiple
      onChange={(values) => (userStatuses = values as PanelUserListStatus[])}
    />
    <label class="sort-field">
      <span class="visually-hidden">User sort order</span>
      <select class="select-input" bind:value={userSort} aria-label="User sort order">
        <option value="name_asc">Name A–Z</option>
        <option value="name_desc">Name Z–A</option>
        <option value="login_newest">Last login newest</option>
        <option value="login_oldest">Last login oldest</option>
        <option value="updated_newest">Recently changed</option>
        <option value="updated_oldest">Oldest changed</option>
      </select>
    </label>
    <span class="result-summary mono">{resultSummary(userPage, userPageIndex, userLimit)}</span>
    <PageSizeSelect
      value={userLimit}
      label="Users per page"
      onSelect={(value) => (userLimit = value)}
    />
  </div>

  <div class="stable-feedback" aria-live="polite">{feedback}</div>
  {#if failure !== null}<p class="form-error" role="alert">{failure}</p>{/if}

  <div class:loading={loadingUsers} class="user-results" aria-busy={loadingUsers}>
    {#if loadingUsers && userPage === null}
      <p class="result-state dim">Reading users…</p>
    {:else if users.length === 0}
      <p class="result-state dim">
        {userQuery === '' && userRoles.length === 0 && userStatuses.length === 0
          ? 'No users in this scope'
          : 'No users match these filters'}
      </p>
    {:else}
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div class="user-table-wrap" role="region" aria-label="Panel users" tabindex="0">
        <table class="user-table">
          <thead>
            <tr>
              <th>User</th>
              <th>Role</th>
              <th>Status</th>
              <th>Last login</th>
              <th><span class="visually-hidden">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {#each users as user (user.account.id)}
              <tr>
                <th scope="row">
                  <span class="user-identity">
                    <Avatar account={user.account} size={32} />
                    <span>
                      <strong>{user.account.display_name}</strong>
                      <span class="user-login mono">@{user.account.login}</span>
                    </span>
                  </span>
                </th>
                <td>
                  {#if user.manageable}
                    <select
                      class="select-input role-select"
                      aria-label="Role for {user.account.login}"
                      value={selectedRole(user)}
                      disabled={savingAccount === user.account.id}
                      onchange={(event) => void changeRole(user, event.currentTarget.value)}
                    >
                      {#if scope === 'global'}
                        {#each globalRoleOptions() as role (role.value)}
                          <option value={role.value}>{role.label}</option>
                        {/each}
                      {:else}
                        {#each targetRoleOptions() as role (role.value)}
                          <option value={role.value}>{role.label}</option>
                        {/each}
                      {/if}
                    </select>
                  {:else}
                    <Chip tone={user.root ? 'accent' : 'signal'}>{roleLabel(shownRole(user))}</Chip>
                  {/if}
                </td>
                <td>
                  <span class="status-cell">
                    <Chip tone={statusTone(user)} dot>{statusLabel(user)}</Chip>
                    {#if user.status === 'banned' || user.target_access?.suspended === true}
                      <DecisionHistory
                        label={`Access decision history for @${user.account.login}`}
                        reason={currentReason(user)}
                        decidedAt={currentDecisionAt(user)}
                        fetchDecisions={() =>
                          fetchUserDecisions(
                            user.account.id,
                            scope === 'target' ? targetId : undefined,
                          )}
                      />
                    {/if}
                  </span>
                </td>
                <td class="last-login">
                  {#if user.last_login_at === undefined}
                    <span class="dim">Never</span>
                  {:else}
                    <time datetime={user.last_login_at} title={formatTimestamp(user.last_login_at)}>
                      {formatRelative(user.last_login_at, now)}
                    </time>
                  {/if}
                </td>
                <td class="row-actions">
                  {#if user.manageable}
                    <ActionMenu
                      label={`Actions for @${user.account.login}`}
                      items={userActionItems(user)}
                      onSelect={(action) => beginAction(user, action as UserAction)}
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

  <div class="pagination-footer">
    <span class="result-summary mono">{resultSummary(userPage, userPageIndex, userLimit)}</span>
    <div class="pagination-actions">
      <PageSizeSelect
        value={userLimit}
        label="Users per page below results"
        onSelect={(value) => (userLimit = value)}
      />
      <PageNavigation
        pageIndex={userPageIndex}
        pageCount={userPageCount}
        disabled={loadingUsers}
        onSelect={selectUserPage}
      />
    </div>
  </div>

  <section class="invitations" aria-labelledby="invitations-heading">
    <div class="section-heading">
      <div>
        <h3 id="invitations-heading">Invitations</h3>
        <p>Pending and previous identity-locked access offers</p>
      </div>
    </div>

    <div class="management-toolbar invitation-toolbar" aria-label="Invitation list controls">
      <label class="search-field">
        <span class="visually-hidden">Search invitations</span>
        <span class="search-icon" aria-hidden="true"></span>
        <input
          class="text-input"
          type="search"
          placeholder="Search invitations"
          bind:value={invitationSearch}
        />
      </label>
      <FilterMenu
        label="Invitation roles"
        summary={selectedSummary(invitationRoles, 'All roles')}
        hint="Show invitations with any selected role"
        sections={INVITATION_ROLE_FILTERS}
        selected={invitationRoles}
        multiple
        onChange={(values) => (invitationRoles = values as Exclude<PanelRole, 'none'>[])}
      />
      <FilterMenu
        label="Invitation status"
        summary={invitationStatuses.length === 0
          ? 'All statuses'
          : `${invitationStatuses.length} selected`}
        hint="Show invitations with any selected status"
        sections={INVITATION_STATUS_FILTERS}
        selected={invitationStatuses}
        multiple
        onChange={(values) => (invitationStatuses = values as InvitationStatus[])}
      />
      <label class="sort-field">
        <span class="visually-hidden">Invitation sort order</span>
        <select class="select-input" bind:value={invitationSort} aria-label="Invitation sort order">
          <option value="created_newest">Newest first</option>
          <option value="created_oldest">Oldest first</option>
          <option value="expiry_soonest">Expires soonest</option>
          <option value="expiry_latest">Expires latest</option>
          <option value="name_asc">Name A–Z</option>
        </select>
      </label>
      <span class="result-summary mono">
        {resultSummary(invitationPage, invitationPageIndex, invitationLimit)}
      </span>
      <PageSizeSelect
        value={invitationLimit}
        label="Invitations per page"
        onSelect={(value) => (invitationLimit = value)}
      />
    </div>

    <div
      class:loading={loadingInvitations}
      class="invitation-results"
      aria-busy={loadingInvitations}
    >
      {#if loadingInvitations && invitationPage === null}
        <p class="result-state dim">Reading invitations…</p>
      {:else if invitations.length === 0}
        <p class="result-state dim">
          {invitationQuery === '' && invitationRoles.length === 0 && invitationStatuses.length === 0
            ? 'No invitations in this scope'
            : 'No invitations match these filters'}
        </p>
      {:else}
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <div class="user-table-wrap" role="region" aria-label="Panel invitations" tabindex="0">
          <table class="user-table invitation-table">
            <thead>
              <tr>
                <th>User</th>
                <th>Role</th>
                <th>Status</th>
                <th>Expires</th>
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
                  <td><Chip tone="signal">{roleLabel(invitation.role)}</Chip></td>
                  <td
                    ><Chip tone={invitationTone(invitation.status)} dot>{invitation.status}</Chip
                    ></td
                  >
                  <td class="last-login">
                    <time
                      datetime={invitation.expires_at}
                      title={formatTimestamp(invitation.expires_at)}
                    >
                      {formatDateTime(invitation.expires_at)}
                    </time>
                  </td>
                  <td class="row-actions">
                    {#if invitationActionItems(invitation).length > 0}
                      <ActionMenu
                        label={`Actions for @${invitation.account.login} invitation`}
                        items={invitationActionItems(invitation)}
                        onSelect={(action) => chooseInvitationAction(invitation, action)}
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

    <div class="pagination-footer">
      <span class="result-summary mono">
        {resultSummary(invitationPage, invitationPageIndex, invitationLimit)}
      </span>
      <div class="pagination-actions">
        <PageSizeSelect
          value={invitationLimit}
          label="Invitations per page below results"
          onSelect={(value) => (invitationLimit = value)}
        />
        <PageNavigation
          pageIndex={invitationPageIndex}
          pageCount={invitationPageCount}
          disabled={loadingInvitations}
          onSelect={selectInvitationPage}
        />
      </div>
    </div>
  </section>
</Plate>

<Modal
  id="add-user"
  open={addModalOpen}
  title={scope === 'global' ? 'Add a global user' : `Add a user to ${targetName}`}
  description={scope === 'global'
    ? 'Grant direct access or create a secure invitation for a GitHub identity'
    : `Grant access only to ${targetName} or create a secure invitation`}
  returnFocus={addButton}
  onClose={closeAddModal}
>
  <form id="add-user-form" class="add-user-form" onsubmit={submitAdd}>
    <SegmentedControl
      name="access-method"
      label="Access method"
      options={ACCESS_METHODS}
      value={accessMethod}
      onSelect={(value) => {
        accessMethod = value as 'add' | 'invite';
        generatedLink = '';
      }}
    />

    {#if generatedLink === ''}
      <label class="form-field">
        <span>GitHub login</span>
        <input
          class="text-input"
          autocomplete="off"
          placeholder="octocat"
          bind:value={login}
          required
          data-modal-focus
        />
        <small>The invitation is locked to this GitHub identity</small>
      </label>

      <div class="form-grid">
        <label class="form-field">
          <span>Role</span>
          <select class="select-input" bind:value={addRole} aria-label="Role">
            {#each addRoles() as role (role)}
              <option value={role}>{roleLabel(role)}</option>
            {/each}
          </select>
        </label>
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
  .header-actions,
  .management-toolbar,
  .pagination-footer,
  .pagination-actions,
  .status-cell {
    align-items: center;
    display: flex;
  }

  .header-actions {
    gap: 0.5rem;
    margin-left: auto;
  }

  .add-icon {
    height: 0.75rem;
    position: relative;
    width: 0.75rem;
  }

  .add-icon::before,
  .add-icon::after {
    background: currentColor;
    content: '';
    left: 50%;
    position: absolute;
    top: 50%;
    transform: translate(-50%, -50%);
  }

  .add-icon::before {
    height: 1px;
    width: 0.75rem;
  }

  .add-icon::after {
    height: 0.75rem;
    width: 1px;
  }

  .management-toolbar {
    --repository-control-height: 1.875rem;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .search-field {
    flex: 1 1 15rem;
    position: relative;
  }

  .search-field .text-input {
    font-size: 0.75rem;
    height: var(--repository-control-height);
    padding-left: 2rem;
    width: 100%;
  }

  .search-icon {
    border: 1.5px solid var(--dim);
    border-radius: 50%;
    height: 0.55rem;
    left: 0.7rem;
    position: absolute;
    top: 0.62rem;
    width: 0.55rem;
  }

  .search-icon::after {
    background: var(--dim);
    content: '';
    height: 1px;
    left: 0.42rem;
    position: absolute;
    top: 0.48rem;
    transform: rotate(45deg);
    width: 0.35rem;
  }

  .sort-field .select-input {
    font-size: 0.6875rem;
    height: var(--repository-control-height);
  }

  .result-summary {
    color: var(--dim);
    font-size: 0.625rem;
    margin-left: auto;
    white-space: nowrap;
  }

  .stable-feedback {
    color: var(--clear);
    font-size: 0.75rem;
    min-height: 1.125rem;
  }

  .form-error {
    font-size: 0.75rem;
    margin: 0 0 0.75rem;
  }

  .user-results {
    margin-top: 0;
  }

  .user-results.loading,
  .invitation-results.loading {
    cursor: progress;
  }

  .result-state {
    border: 1px dashed var(--rule);
    border-radius: var(--r-well);
    font-size: 0.8125rem;
    margin: 0;
    padding: 1.5rem;
    text-align: center;
  }

  .user-table-wrap {
    border: 1px solid var(--rule);
    border-radius: var(--r-well);
    overflow-x: auto;
  }

  .user-table-wrap:focus-visible {
    outline: 2px solid var(--brand);
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
    padding: 0.7rem 0.75rem;
    text-align: left;
    vertical-align: middle;
  }

  .user-table thead th {
    color: var(--dim);
    font: 600 0.6875rem/1 var(--mono);
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .user-table tbody tr:last-child th,
  .user-table tbody tr:last-child td {
    border-bottom: 0;
  }

  .user-table tbody tr:hover {
    background: var(--well);
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
    font-size: 0.875rem;
    font-weight: 700;
    line-height: 1.2;
  }

  .user-login {
    color: var(--dim);
    font-size: 0.6875rem;
    font-weight: 500;
    line-height: 1.2;
  }

  .last-login {
    color: var(--dim);
    font-size: 0.75rem;
  }

  .status-cell {
    gap: 0.25rem;
  }

  .role-select {
    min-width: 8.5rem;
  }

  .row-actions {
    text-align: right !important;
    width: 2.75rem;
  }

  .row-actions :global(.action-menu) {
    display: inline-block;
  }

  .pagination-footer {
    gap: 1rem;
    justify-content: space-between;
    min-height: 2.75rem;
    padding-top: 0.75rem;
  }

  .pagination-footer .result-summary {
    margin-left: 0;
  }

  .pagination-actions {
    gap: 0.625rem;
  }

  .invitations {
    border-top: 1px solid var(--rule);
    margin-top: 1rem;
    padding-top: 1rem;
  }

  .section-heading {
    margin-bottom: 0.75rem;
  }

  .section-heading h3 {
    color: var(--accent);
    font: 600 0.6875rem/1.2 var(--mono);
    letter-spacing: 0.1em;
    margin: 0;
    text-transform: uppercase;
  }

  .section-heading p {
    color: var(--dim);
    font-size: 0.75rem;
    margin: 0.25rem 0 0;
  }

  .invitation-results {
    margin-top: 1.125rem;
  }

  .invitation-table {
    min-width: 44rem;
  }

  .add-user-form {
    display: grid;
    gap: 1rem;
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

  .reason-textarea {
    background: var(--control-surface);
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
    .header-actions {
      width: 100%;
    }

    .header-actions :global(.scope-picker) {
      flex: 1;
    }

    .header-actions :global(.scope-picker summary) {
      max-width: none;
    }

    .management-toolbar > .result-summary {
      display: none;
    }
  }

  @media (max-width: 36rem) {
    .header-actions {
      align-items: stretch;
      flex-direction: column-reverse;
    }

    .pagination-footer {
      align-items: flex-start;
      flex-direction: column;
    }

    .pagination-actions {
      justify-content: space-between;
      width: 100%;
    }

    .form-grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
