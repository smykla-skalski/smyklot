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
  let invitationSort = $state<InvitationSort>('name_asc');
  let invitationRoles = $state<Exclude<PanelRole, 'none'>[]>([]);
  let invitationStatuses = $state<InvitationStatus[]>([]);
  let invitationLimit = $state(20);
  let invitationPageIndex = $state(0);
  let activeSection = $state<ManagementSection>('users');

  let addModalOpen = $state(false);
  let addButton = $state<HTMLButtonElement | null>(null);
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
  const addScopeTarget = $derived(
    addScopeTargetId === null
      ? undefined
      : targets.find((target) => target.id === addScopeTargetId),
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
    const selectedTargetId = addScopeTargetId;
    const destination =
      selectedTargetId === null
        ? 'global access'
        : (addScopeTarget?.account.display_name ?? 'the selected organization');
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

  async function reissue(invitation: PanelInvitation): Promise<void> {
    invitationBusy = invitation.id;
    actionFailure = null;
    try {
      const updated = await reissueInvitation(invitation.id, 7);
      generatedLink = updated.invite_url ?? '';
      accessMethod = 'invite';
      addScopeTargetId = scope === 'global' ? null : targetId;
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
    addModalOpen = true;
  }

  function closeAddModal(): void {
    addModalOpen = false;
    generatedLink = '';
    login = '';
  }

  function selectAddScope(nextTarget: string | null): void {
    addScopeTargetId = nextTarget;
    const roles = addRoles(nextTarget);
    if (!roles.includes(addRole)) addRole = roles[0] ?? 'viewer';
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
    const tab = (event.currentTarget as HTMLElement)
      .closest<HTMLElement>('[role="tablist"]')
      ?.querySelector<HTMLButtonElement>(`#${next}-list-tab`);
    tab?.focus();
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

  function sortGlyph(direction: SortDirection | undefined): string {
    if (direction === 'ascending') return '▲';
    if (direction === 'descending') return '▼';
    return '◇';
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

  function addRoles(selectedTargetId = addScopeTargetId): PanelRole[] {
    if (selectedTargetId === null) {
      return canManageOwners
        ? ['viewer', 'editor', 'admin', 'owner']
        : ['viewer', 'editor', 'admin'];
    }
    const target = targets.find((candidate) => candidate.id === selectedTargetId);
    return target?.effective_role === 'owner'
      ? ['viewer', 'editor', 'admin']
      : ['viewer', 'editor'];
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

  function roleLevel(role: PanelRole): number {
    return ['none', 'viewer', 'editor', 'admin', 'owner'].indexOf(role);
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

{#snippet sortButton(label: string, direction: SortDirection | undefined, onSelect: () => void)}
  <button class="sort-button" type="button" onclick={onSelect}>
    <span>{label}</span>
    <span class="sort-indicator" aria-hidden="true">{sortGlyph(direction)}</span>
  </button>
{/snippet}

{#snippet roleBadge(role: PanelRole)}
  <span class="role-badge role-{role}">
    <span class="role-level" aria-hidden="true">
      {#each [0, 1, 2, 3] as level (level)}
        <span class:filled={level < roleLevel(role)}></span>
      {/each}
    </span>
    <span>{roleLabel(role)}</span>
  </span>
{/snippet}

{#snippet headerActions()}
  <div class="header-actions">
    <div class="scope-control">
      <span class="scope-label">Scope</span>
      <ScopePicker
        global={scope === 'global'}
        {targetId}
        {targets}
        canSelectGlobal={canManageGlobal}
        onSelect={selectScope}
      />
    </div>
  </div>
{/snippet}

<Plate label="Users" status={headerActions}>
  <div class="list-tabs">
    <div class="tab-options" role="tablist" aria-label="User management lists">
      <button
        id="users-list-tab"
        class="list-tab"
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
        <span class="tab-count mono">{userPage?.total ?? '—'}</span>
      </button>
      <button
        id="invitations-list-tab"
        class="list-tab"
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
        <span class="tab-count mono">{invitationPage?.total ?? '—'}</span>
      </button>
    </div>
    <button
      class="btn btn-signal tab-add"
      type="button"
      bind:this={addButton}
      onclick={openAddModal}
    >
      <span class="add-icon" aria-hidden="true"></span>
      Add user
    </button>
  </div>

  <div class="stable-feedback" aria-live="polite">{feedback}</div>
  {#if failure !== null}<p class="form-error" role="alert">{failure}</p>{/if}

  {#if activeSection === 'users'}
    <div id="users-list-panel" role="tabpanel" aria-labelledby="users-list-tab">
      <div class="management-toolbar" aria-label="User list controls">
        <label class="search-field">
          <span class="visually-hidden">Search users</span>
          <span class="search-icon" aria-hidden="true"></span>
          <input
            class="text-input"
            type="search"
            placeholder="Search users"
            bind:value={userSearch}
          />
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
        <PageSizeSelect
          value={userLimit}
          label="Users per page above results"
          onSelect={(value) => (userLimit = value)}
        />
      </div>

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
                        {@render roleBadge(shownRole(user))}
                      {/if}
                    </td>
                    <td>
                      <Chip tone={statusTone(user)} dot>{statusLabel(user)}</Chip>
                    </td>
                    <td class="last-login">
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
          <PageNavigation
            pageIndex={userPageIndex}
            pageCount={userPageCount}
            disabled={loadingUsers}
            onSelect={selectUserPage}
          />
          <PageSizeSelect
            value={userLimit}
            label="Users per page below results"
            onSelect={(value) => (userLimit = value)}
          />
        </div>
      </div>
    </div>
  {:else}
    <div id="invitations-list-panel" role="tabpanel" aria-labelledby="invitations-list-tab">
      <div class="management-toolbar" aria-label="Invitation list controls">
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
        <PageSizeSelect
          value={invitationLimit}
          label="Invitations per page above results"
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
            {invitationQuery === '' &&
            invitationRoles.length === 0 &&
            invitationStatuses.length === 0
              ? 'No invitations in this scope'
              : 'No invitations match these filters'}
          </p>
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
                    <td>{@render roleBadge(invitation.role)}</td>
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
          <PageNavigation
            pageIndex={invitationPageIndex}
            pageCount={invitationPageCount}
            disabled={loadingInvitations}
            onSelect={selectInvitationPage}
          />
          <PageSizeSelect
            value={invitationLimit}
            label="Invitations per page below results"
            onSelect={(value) => (invitationLimit = value)}
          />
        </div>
      </div>
    </div>
  {/if}
</Plate>

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
  returnFocus={addButton}
  onClose={closeAddModal}
>
  <form id="add-user-form" class="add-user-form" onsubmit={submitAdd}>
    {#if generatedLink === ''}
      <div class="form-field">
        <span>Scope</span>
        <ScopePicker
          global={addScopeTargetId === null}
          targetId={addScopeTargetId ?? targetId}
          {targets}
          canSelectGlobal={canManageGlobal}
          variant="field"
          label="Access scope"
          onSelect={selectAddScope}
        />
        <small>Choose global access or one installation</small>
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
              <span class="method-icon method-{method.value}" aria-hidden="true"></span>
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
  .pagination-actions {
    align-items: center;
    display: flex;
  }

  .header-actions {
    gap: 0.5rem;
    margin-left: auto;
  }

  .scope-control {
    align-items: center;
    display: flex;
    gap: 0.5rem;
  }

  .scope-label {
    color: var(--dim);
    font: 600 0.5625rem/1 var(--mono);
    letter-spacing: 0.09em;
    text-transform: uppercase;
  }

  .list-tabs {
    align-items: stretch;
    border-bottom: 1px solid var(--rule);
    display: flex;
    margin: -1.125rem -1.125rem 0;
    padding: 0 1.125rem;
  }

  .tab-options {
    display: flex;
    gap: 0.25rem;
  }

  .tab-add {
    height: 1.875rem;
    margin: auto 0 auto auto;
  }

  .list-tab {
    align-items: center;
    background: transparent;
    border: 0;
    border-bottom: 2px solid transparent;
    color: var(--dim);
    display: inline-flex;
    font-size: 0.75rem;
    font-weight: 650;
    gap: 0.5rem;
    min-height: 2.75rem;
    padding: 0 0.625rem;
    transition:
      background-color 120ms ease-out,
      border-color 120ms ease-out,
      color 120ms ease-out;
  }

  .list-tab:hover {
    background: var(--well);
    color: var(--text);
  }

  .list-tab.selected {
    border-bottom-color: var(--accent);
    color: var(--text);
  }

  .tab-count {
    align-items: center;
    background: var(--well);
    border: 1px solid var(--rule);
    border-radius: 999px;
    color: var(--dim);
    display: inline-flex;
    box-sizing: border-box;
    font-size: 0.5625rem;
    font-weight: 650;
    height: 1.25rem;
    justify-content: center;
    line-height: 1;
    min-width: 1.5rem;
    padding: 0 0.375rem;
  }

  .list-tab.selected .tab-count {
    background: var(--accent-tint);
    border-color: color-mix(in srgb, var(--accent) 35%, transparent);
    color: var(--accent);
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
    --page-size-control-height: 1.875rem;
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
    margin-top: 1.125rem;
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
    padding: 0.7rem 0.75rem;
    text-align: left;
    text-transform: inherit;
    transition:
      background-color 120ms ease-out,
      color 120ms ease-out;
    width: 100%;
  }

  .sort-button:hover,
  .sort-button:focus-visible {
    background: var(--well);
    color: var(--text);
  }

  .sort-indicator {
    color: var(--signal);
    font-size: 0.55rem;
    letter-spacing: 0;
  }

  .user-table tbody tr:last-child th,
  .user-table tbody tr:last-child td {
    border-bottom: 0;
  }

  .user-table tbody tr:hover {
    background: var(--well);
  }

  .user-table tbody tr.history-row {
    cursor: pointer;
  }

  .user-table tbody tr.history-row:hover {
    background: var(--strip-lift);
  }

  .user-table tbody tr.history-row:focus-visible {
    outline: 2px solid var(--brand);
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

  .role-select {
    font: 600 0.625rem/1 var(--sans);
    height: 1.625rem;
    min-width: 7.5rem;
    padding-left: 0.55rem;
    padding-right: 1.65rem;
  }

  .role-badge {
    align-items: center;
    background: var(--well);
    border: 1px solid var(--rule);
    border-radius: var(--r-ctl);
    color: var(--text);
    display: inline-flex;
    font: 650 0.625rem/1 var(--mono);
    gap: 0.45rem;
    min-height: 1.625rem;
    padding: 0 0.55rem;
    text-transform: uppercase;
    white-space: nowrap;
  }

  .role-owner {
    background: var(--accent-tint);
    border-color: color-mix(in srgb, var(--accent) 35%, transparent);
    color: var(--accent);
  }

  .role-level {
    align-items: end;
    display: inline-grid;
    flex: none;
    gap: 1px;
    grid-template-columns: repeat(4, 2px);
    height: 0.625rem;
  }

  .role-level > span {
    background: var(--rule);
    border-radius: 1px;
    height: 0.25rem;
  }

  .role-level > span:nth-child(2) {
    height: 0.375rem;
  }

  .role-level > span:nth-child(3) {
    height: 0.5rem;
  }

  .role-level > span:nth-child(4) {
    height: 0.625rem;
  }

  .role-level > span.filled {
    background: currentColor;
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

  .invitation-results {
    margin-top: 1.125rem;
  }

  .invitation-table {
    min-width: 44rem;
  }

  .add-user-form {
    display: grid;
    gap: 0.875rem;
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
    background: var(--signal-tint);
    border-color: color-mix(in srgb, var(--signal) 60%, transparent);
  }

  .method-option:has(input:focus-visible) {
    outline: 2px solid var(--brand);
    outline-offset: 2px;
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
    font: 700 0.875rem/1 var(--mono);
    height: 1.75rem;
    justify-content: center;
    width: 1.75rem;
  }

  .method-icon::before {
    content: '+';
  }

  .method-invite::before {
    content: '↗';
  }

  .method-option.selected .method-icon {
    background: color-mix(in srgb, var(--signal) 18%, transparent);
    color: var(--signal);
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
    border-color: var(--signal);
  }

  .method-option.selected .method-check::after {
    background: var(--signal);
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

    .identity-grid.with-expiry {
      grid-template-columns: minmax(0, 1.35fr) repeat(2, minmax(6.5rem, 0.75fr));
    }
  }

  @media (max-width: 36rem) {
    .header-actions {
      align-items: stretch;
    }

    .scope-control {
      display: grid;
      grid-template-columns: auto minmax(0, 1fr);
      width: 100%;
    }

    .scope-control :global(.scope-picker) {
      min-width: 0;
      width: 100%;
    }

    .list-tabs {
      padding-right: 0.75rem;
    }

    .list-tab {
      padding-inline: 0.45rem;
    }

    .tab-add {
      padding-inline: 0.625rem;
    }

    .pagination-footer {
      align-items: flex-start;
      flex-direction: column;
    }

    .pagination-actions {
      justify-content: space-between;
      width: 100%;
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

  @media (prefers-reduced-motion: reduce) {
    .method-option:active {
      transform: none;
    }
  }
</style>
