<script lang="ts">
  import { formatRelative, formatTimestamp } from '../lib/format';
  import type { FilterSection } from '../lib/filter-menu';
  import type {
    Page,
    AddTargetUserInput,
    AddRootInvitationInput,
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
  } from '../lib/types';
  import ActionMenu, { type ActionMenuItem } from './ActionMenu.svelte';
  import Avatar from './Avatar.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import FilterMenu from './FilterMenu.svelte';
  import Icon from './Icon.svelte';
  import InfiniteLoadSentinel from './InfiniteLoadSentinel.svelte';
  import Modal from './Modal.svelte';
  import RootInvitations from './RootInvitations.svelte';
  import RootPageHeader from './RootPageHeader.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import TableEmptyState from './TableEmptyState.svelte';

  type AccessSection = 'users' | 'invitations';
  type SortColumn = 'name' | 'role' | 'last_login';
  type UserAction = 'promote_root' | 'demote_root' | 'restore' | 'ban' | 'remove';

  const SECTIONS = [
    { value: 'users', label: 'Users', tone: 'accent' },
    { value: 'invitations', label: 'Invitations', tone: 'accent' },
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
        { value: 'removed', label: 'Removed', tone: 'default' },
      ],
    },
  ] satisfies readonly FilterSection[];

  const {
    rootRole,
    section,
    refreshVersion,
    onSection,
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
    onOpenInstallationAccess,
  }: {
    rootRole: string;
    section: AccessSection;
    refreshVersion: number;
    onSection: (section: AccessSection) => void;
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
    onOpenInstallationAccess: (account: string) => void;
  } = $props();

  let page = $state<Page<RootPanelUser> | null>(null);
  let search = $state('');
  let query = $state('');
  let sort = $state<RootPanelUserSort>('name_asc');
  let systemRoles = $state<SystemRole[]>([]);
  let statuses = $state<PanelUserStatus[]>([]);
  let loading = $state(false);
  let problem = $state<string | null>(null);
  let loadMoreProblem = $state<string | null>(null);
  let actionUser = $state<RootPanelUser | null>(null);
  let pendingAction = $state<UserAction | null>(null);
  let actionTrigger = $state<HTMLElement | null>(null);
  let reason = $state('');
  let saving = $state(false);
  let actionProblem = $state<string | null>(null);
  let addOpen = $state(false);
  let addTrigger = $state<HTMLButtonElement | null>(null);
  let inviteTrigger = $state<HTMLButtonElement | null>(null);
  let invitations = $state<RootInvitations | null>(null);
  let addLogin = $state('');
  let addRole = $state<Exclude<InstallationRole, 'none' | 'owner'>>('viewer');
  let installationQuery = $state('');
  let installations = $state<RootInstallation[]>([]);
  let selectedInstallationID = $state('');
  let installationsLoading = $state(false);
  let addSaving = $state(false);
  let addProblem = $state<string | null>(null);
  let feedback = $state('');
  let sequence = 0;
  const limit = 20;
  // Ticks so relative login times keep aging in a long Root session.
  let now = $state(Date.now());
  const requestKey = $derived(
    JSON.stringify([query, sort, systemRoles, statuses, limit, refreshVersion]),
  );
  const users = $derived(page?.items ?? []);
  const hasFilters = $derived(query !== '' || systemRoles.length > 0 || statuses.length > 0);
  const selectedInstallation = $derived(
    installations.find((installation) => installation.id === selectedInstallationID) ?? null,
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

  $effect(() => {
    const tick = setInterval(() => {
      now = Date.now();
    }, 30_000);
    return () => clearInterval(tick);
  });

  $effect(() => {
    const next = search.trim();
    const timeout = window.setTimeout(() => (query = next), 180);
    return () => window.clearTimeout(timeout);
  });

  $effect(() => {
    if (section === 'users') void loadPage(undefined, false, requestKey);
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

  async function loadPage(
    cursor: string | undefined,
    append: boolean,
    key = requestKey,
  ): Promise<void> {
    if (key !== requestKey || loading) return;
    const version = ++sequence;
    loading = true;
    if (append) loadMoreProblem = null;
    else problem = null;
    try {
      const loaded = await fetchUsers({
        ...(cursor === undefined ? {} : { cursor }),
        query,
        sort,
        limit,
        systemRoles,
        statuses,
      });
      if (version !== sequence || key !== requestKey) return;
      page =
        append && page !== null ? { ...loaded, items: [...page.items, ...loaded.items] } : loaded;
    } catch (error) {
      if (version !== sequence || key !== requestKey) return;
      const message = error instanceof Error ? error.message : String(error);
      if (append) loadMoreProblem = message;
      else problem = message;
    } finally {
      if (version === sequence) loading = false;
    }
  }

  function loadNext(): void {
    const cursor = page?.next_cursor;
    if (cursor !== null && cursor !== undefined) void loadPage(cursor, true);
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

  async function openAddUser(): Promise<void> {
    addOpen = true;
    addLogin = '';
    addRole = 'viewer';
    installationQuery = '';
    selectedInstallationID = '';
    addProblem = null;
    installationsLoading = true;
    try {
      installations = await fetchInstallations();
      selectedInstallationID =
        installations.find((installation) => installation.available)?.id ?? '';
    } catch (error) {
      addProblem = error instanceof Error ? error.message : String(error);
    } finally {
      installationsLoading = false;
    }
  }

  function closeAddUser(): void {
    if (addSaving) return;
    addOpen = false;
    addProblem = null;
  }

  async function submitAddUser(): Promise<void> {
    const installation = selectedInstallation;
    if (installation === null || !installation.available) return;
    if (!installation.owned_by_viewer) {
      addOpen = false;
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
      addOpen = false;
      page = null;
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
    if (!['promote_root', 'demote_root', 'restore', 'ban', 'remove'].includes(action)) return;
    actionUser = user;
    pendingAction = action as UserAction;
    actionTrigger = trigger;
    reason = '';
    actionProblem = null;
  }

  function closeUserAction(): void {
    if (saving) return;
    actionUser = null;
    pendingAction = null;
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
      actionUser = null;
      pendingAction = null;
      page = null;
      await loadPage(undefined, false);
    } catch (error) {
      actionProblem = error instanceof Error ? error.message : String(error);
    } finally {
      saving = false;
    }
  }
</script>

{#snippet sectionSwitch()}
  <SegmentedControl
    name="root-access-section"
    label="Root access lists"
    options={SECTIONS}
    value={section}
    variant="navigation"
    onSelect={selectSection}
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
        <button
          class="btn btn-signal"
          type="button"
          bind:this={inviteTrigger}
          onclick={() => invitations?.openCreate(inviteTrigger)}
        >
          <Icon name="user-plus" size={14} strokeWidth={2} />
          <span class="button-label">Invite Root user</span>
        </button>
      {/if}
    {:else}
      <button
        class="btn btn-signal"
        type="button"
        bind:this={addTrigger}
        onclick={() => void openAddUser()}
      >
        <Icon name="user-plus" size={14} strokeWidth={2} />
        <span class="button-label">Add user</span>
      </button>
    {/if}
  </RootPageHeader>

  {#if section === 'invitations'}
    <RootInvitations
      bind:this={invitations}
      {refreshVersion}
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
    </div>

    <div class:loading class="user-results" aria-busy={loading}>
      {#if problem !== null}
        <div class="result-state" role="alert">
          <strong>Root users could not be loaded</strong>
          <span>{problem}</span>
          <button class="btn" type="button" onclick={() => void loadPage(undefined, false)}>
            Try again
          </button>
        </div>
      {:else if loading && page === null}
        <div class="table-skeleton" aria-hidden="true">
          {#each [0, 1, 2, 3, 4, 5] as index (index)}<span></span>{/each}
        </div>
      {:else}
        <!-- Keyboard focus lets users scroll columns that overflow the viewport. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <div class="table-scroll" role="region" tabindex="0" aria-label="Root users table">
          <table>
            <caption class="visually-hidden">Application accounts</caption>
            <thead>
              <tr>
                <th scope="col" aria-sort={sortDirection('name')}>
                  <button
                    class="table-sort-button"
                    type="button"
                    onclick={() => toggleSort('name')}
                  >
                    <span>User</span><Icon name="sort" size={14} />
                  </button>
                </th>
                <th scope="col" aria-sort={sortDirection('role')}>
                  <div class="heading-layout">
                    <button
                      class="table-sort-button"
                      type="button"
                      onclick={() => toggleSort('role')}
                    >
                      <span>System role</span><Icon name="sort" size={14} />
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
                      showIcon
                      iconOnly
                      placement="header"
                      onChange={selectRoles}
                    />
                  </div>
                </th>
                <th scope="col">
                  <div class="heading-layout">
                    <span class="heading-label">Status</span>
                    <FilterMenu
                      label="Status"
                      summary={statuses.length === 0
                        ? 'All statuses'
                        : `${statuses.length} selected`}
                      hint="Filter account lifecycle state"
                      sections={STATUS_FILTERS}
                      selected={statuses}
                      multiple
                      align="end"
                      showIcon
                      iconOnly
                      placement="header"
                      onChange={selectStatuses}
                    />
                  </div>
                </th>
                <th scope="col">Installations</th>
                <th scope="col" aria-sort={sortDirection('last_login')}>
                  <button
                    class="table-sort-button"
                    type="button"
                    onclick={() => toggleSort('last_login')}
                  >
                    <span>Last login</span><Icon name="sort" size={14} />
                  </button>
                </th>
                <th scope="col"><span class="visually-hidden">Actions</span></th>
              </tr>
            </thead>
            <tbody data-panel-scroll onscroll={loadFromScroll}>
              {#each users as user (user.account.id)}
                <tr>
                  <td data-label="User">
                    <span class="identity">
                      <Avatar account={user.account} size={32} />
                      <span
                        ><strong>{user.account.display_name}</strong><span class="mono"
                          >@{user.account.login}</span
                        ></span
                      >
                    </span>
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
                  <td data-label="Installations">
                    <span class="relationship-count">{installationSummary(user)}</span>
                    <span class="relationship-meta"
                      >{user.owned_installations} owned · {user.assigned_installations} assigned</span
                    >
                  </td>
                  <td data-label="Last login">
                    {#if user.last_login_at !== undefined}
                      <time
                        datetime={user.last_login_at}
                        title={formatTimestamp(user.last_login_at)}
                        >{formatRelative(user.last_login_at, now)}</time
                      >
                    {:else}<span class="dim">Never</span>{/if}
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
                </tr>
              {:else}
                <tr class="empty-row">
                  <td colspan="6">
                    <TableEmptyState
                      title="No accounts found"
                      description={hasFilters
                        ? 'Try another search or clear the active filters'
                        : 'Accounts appear after their first authenticated session'}
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
        active={!loading && page?.next_cursor != null}
        cursor={page?.next_cursor}
        onVisible={loadNext}
      />
      {#if loadMoreProblem !== null}
        <div class="load-more-alert" role="alert">
          <span>{loadMoreProblem}</span><button class="btn" type="button" onclick={loadNext}
            >Try again</button
          >
        </div>
      {/if}
    </div>
  {/if}
</section>

<Modal
  id="root-user-action"
  open={actionUser !== null && pendingAction !== null}
  title={actionTitle()}
  description={actionDescription()}
  returnFocus={actionTrigger}
  onClose={closeUserAction}
>
  {#if pendingAction === 'ban' || pendingAction === 'remove'}
    <label class="reason-field">
      <span>Reason <small>Optional</small></span>
      <textarea
        placeholder="Add context to the immutable audit record"
        maxlength="500"
        rows="4"
        bind:value={reason}
        data-modal-focus></textarea>
      <small>{reason.length}/500 characters</small>
    </label>
  {:else}
    <div class="confirmation-note" data-modal-focus tabindex="-1">
      <Icon name={pendingAction === 'promote_root' ? 'warning' : 'info'} size={20} />
      <span>Review the account and effect before confirming.</span>
    </div>
  {/if}
  {#if actionProblem !== null}<p class="action-error" role="alert">{actionProblem}</p>{/if}

  {#snippet footer()}
    <button class="btn btn-ghost" type="button" data-modal-focus onclick={closeUserAction}
      >Cancel</button
    >
    <button
      class="btn"
      class:btn-stop={pendingAction === 'ban' ||
        pendingAction === 'remove' ||
        pendingAction === 'demote_root'}
      class:btn-signal={pendingAction === 'promote_root' || pendingAction === 'restore'}
      type="button"
      disabled={saving}
      onclick={() => void confirmUserAction()}
    >
      {saving ? 'Saving…' : 'Confirm'}
    </button>
  {/snippet}
</Modal>

<Modal
  id="root-add-installation-user"
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
    <label>
      <span>GitHub login</span>
      <input
        class="text-input"
        type="text"
        autocomplete="off"
        placeholder="octocat"
        bind:value={addLogin}
        data-modal-focus
      />
    </label>

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
                bind:group={selectedInstallationID}
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
      <span class="select-wrap">
        <select class="select-input" bind:value={addRole}>
          <option value="viewer">Viewer</option>
          <option value="editor">Editor</option>
          <option value="admin">Admin</option>
        </select>
        <Icon name="chevron-down" size={14} strokeWidth={2} />
      </span>
    </label>

    {#if selectedInstallation !== null && !selectedInstallation.owned_by_viewer}
      <div class="elevation-note">
        <Icon name="warning" size={18} />
        <span>
          This installation is not yours. Continue to its Access view to acknowledge and start the
          audited 15-minute elevation before adding the user.
        </span>
      </div>
    {/if}
    {#if addProblem !== null}<p class="action-error" role="alert">{addProblem}</p>{/if}
  </form>

  {#snippet footer()}
    <button class="btn btn-ghost" type="button" disabled={addSaving} onclick={closeAddUser}
      >Cancel</button
    >
    <button
      class="btn btn-signal"
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
    </button>
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

  .elevation-note {
    align-items: flex-start;
    background: var(--warning-tint);
    border: 1px solid color-mix(in srgb, var(--warning) 30%, var(--warning-tint));
    border-radius: var(--radius-control);
    color: var(--text-secondary);
    display: flex;
    font-size: var(--font-size-compact);
    gap: var(--space-2);
    padding: var(--space-3);
  }

  .user-results {
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

  .table-scroll {
    background: var(--surface-base);
    flex: 1;
    max-width: 100%;
    min-height: 0;
    overflow-x: auto;
  }

  table {
    background: var(--surface-base);
    /* Separated, not collapsed: a collapsed border is shared between adjacent
       rows, so each cell owns half of it and every row box lands on a .5. */
    border-collapse: separate;
    border-spacing: 0;
    min-width: 46rem;
    table-layout: fixed;
    width: 100%;
  }

  th,
  td {
    border-bottom: 1px solid var(--rule);
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

  th {
    background: var(--table-header-bg);
    color: var(--dim);
    font: 650 var(--font-size-compact) / 1.2 var(--sans);
    height: 2.5rem;
    letter-spacing: 0.02em;
  }

  th:has(.table-sort-button) {
    padding: 0;
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

  th:nth-child(5) .table-sort-button {
    justify-content: flex-end;
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

  .heading-layout .table-sort-button {
    flex: 1;
    min-width: 0;
    width: auto;
  }

  .heading-layout :global(.header-filter) {
    margin-inline: var(--space-1);
  }

  .heading-label {
    padding-left: 0;
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

  .confirmation-note {
    align-items: center;
    background: var(--interactive-hover);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    color: var(--text-secondary);
    display: flex;
    gap: var(--space-3);
    padding: var(--space-3);
  }

  .action-error {
    color: var(--danger);
    font-size: var(--font-size-meta);
    margin: var(--space-3) 0 0;
  }

  .empty-row td {
    height: 10rem;
  }

  .empty-row td :global(.table-empty-state) {
    margin-inline: auto;
  }

  .result-state,
  .table-skeleton {
    min-height: 10rem;
  }

  .result-state {
    align-items: center;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    justify-content: center;
    padding: var(--space-6);
    text-align: center;
  }

  .result-state span {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
  }

  .table-skeleton {
    display: grid;
  }

  .table-skeleton span {
    animation: root-access-pulse 1.35s ease-in-out infinite alternate;
    border-bottom: 1px solid var(--rule);
    height: 3.5rem;
  }

  @keyframes root-access-pulse {
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
      overscroll-behavior-y: contain;
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

  @media (max-width: 64rem) {
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
  }

  @media (prefers-reduced-motion: reduce) {
    .table-skeleton span {
      animation: none;
    }
  }
</style>
