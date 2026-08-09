<script lang="ts">
  import { formatDateTime, formatRelative, formatTimestamp } from '../lib/format';
  import type {
    AddGlobalInvitationInput,
    AddGlobalUserInput,
    AddTargetInvitationInput,
    AddTargetUserInput,
    InvitationDays,
    PanelInvitation,
    PanelRole,
    PanelUser,
    UpdateGlobalUserInput,
    UpdateTargetUserInput,
  } from '../lib/types';
  import Avatar from './Avatar.svelte';
  import Chip, { type ChipTone } from './Chip.svelte';
  import Plate from './Plate.svelte';

  type UserScope = 'global' | 'target';
  type PendingAction = 'ban' | 'remove' | 'suspend' | null;
  type TargetRole = Exclude<PanelRole, 'owner'>;
  type GrantedTargetRole = Exclude<TargetRole, 'none'>;

  const {
    scope,
    targetId,
    targetName,
    actorTargetRole,
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
  }: {
    scope: UserScope;
    targetId: string;
    targetName: string;
    actorTargetRole: PanelRole;
    canManageOwners: boolean;
    refreshVersion?: number;
    onScope: (scope: UserScope) => void;
    fetchUsers: () => Promise<PanelUser[]>;
    addUser: (input: AddGlobalUserInput) => Promise<PanelUser>;
    updateUser: (accountId: string, input: UpdateGlobalUserInput) => Promise<PanelUser>;
    fetchTargetUsers: (targetId: string) => Promise<PanelUser[]>;
    addTargetUser: (targetId: string, input: AddTargetUserInput) => Promise<PanelUser>;
    updateTargetUser: (
      targetId: string,
      accountId: string,
      input: UpdateTargetUserInput,
    ) => Promise<PanelUser>;
    fetchInvitations: () => Promise<PanelInvitation[]>;
    createInvitation: (input: AddGlobalInvitationInput) => Promise<PanelInvitation>;
    fetchTargetInvitations: (targetId: string) => Promise<PanelInvitation[]>;
    createTargetInvitation: (
      targetId: string,
      input: AddTargetInvitationInput,
    ) => Promise<PanelInvitation>;
    reissueInvitation: (
      invitationId: string,
      expiresInDays: InvitationDays,
    ) => Promise<PanelInvitation>;
    revokeInvitation: (invitationId: string) => Promise<PanelInvitation>;
  } = $props();

  let users = $state<PanelUser[]>([]);
  let invitations = $state<PanelInvitation[]>([]);
  let loading = $state(true);
  let failure = $state<string | null>(null);
  let feedback = $state('');
  let query = $state('');
  let login = $state('');
  let addRole = $state<PanelRole>('viewer');
  let accessMethod = $state<'add' | 'invite'>('add');
  let expiresInDays = $state<InvitationDays>(7);
  let generatedLink = $state('');
  let invitationBusy = $state<string | null>(null);
  let adding = $state(false);
  let savingAccount = $state<string | null>(null);
  let pendingAccount = $state<string | null>(null);
  let pendingAction = $state<PendingAction>(null);
  let reason = $state('');
  let loadVersion = 0;
  const now = Date.now();

  const visibleUsers = $derived(
    users.filter((user) => {
      const needle = query.trim().toLocaleLowerCase();
      return (
        needle === '' ||
        user.account.login.toLocaleLowerCase().includes(needle) ||
        user.account.display_name.toLocaleLowerCase().includes(needle)
      );
    }),
  );

  $effect(() => {
    const requestedScope = scope;
    const requestedTarget = targetId;
    void load(requestedScope, requestedTarget, refreshVersion);
  });

  async function load(
    requestedScope = scope,
    requestedTarget = targetId,
    requestedRefreshVersion = refreshVersion,
  ): Promise<void> {
    if (requestedRefreshVersion !== refreshVersion) return;
    const version = ++loadVersion;
    loading = true;
    failure = null;
    try {
      const [listed, listedInvitations] = await Promise.all([
        requestedScope === 'global' ? fetchUsers() : fetchTargetUsers(requestedTarget),
        requestedScope === 'global' ? fetchInvitations() : fetchTargetInvitations(requestedTarget),
      ]);
      if (version !== loadVersion) return;
      users = listed;
      invitations = listedInvitations;
    } catch (error) {
      if (version === loadVersion) failure = errorMessage(error);
    } finally {
      if (version === loadVersion) loading = false;
    }
  }

  async function submitAdd(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const normalizedLogin = login.trim();
    if (normalizedLogin === '') return;
    adding = true;
    failure = null;
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
      } else if (scope === 'global') {
        await addUser({ login: normalizedLogin, role: addRole, target_id: targetId });
        feedback = `Added @${normalizedLogin}`;
      } else {
        await addTargetUser(targetId, {
          login: normalizedLogin,
          role: addRole as GrantedTargetRole,
        });
        feedback = `Added @${normalizedLogin}`;
      }
      login = '';
      await load();
    } catch (error) {
      failure = errorMessage(error);
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

  function beginAction(user: PanelUser, action: Exclude<PendingAction, null>): void {
    pendingAccount = user.account.id;
    pendingAction = action;
    reason = '';
  }

  function cancelAction(): void {
    pendingAccount = null;
    pendingAction = null;
    reason = '';
  }

  async function confirmAction(user: PanelUser): Promise<void> {
    const action = pendingAction;
    if (action === null) return;
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
      } else {
        const targetAccess = requiredTargetAccess(user);
        await updateTargetUser(targetId, user.account.id, {
          role: targetAccess.role,
          suspended: true,
          suspension_reason: reason.trim() || undefined,
          expected_revision: targetAccess.revision,
        });
        feedback = `Suspended @${user.account.login} for ${targetName}`;
      }
    });
  }

  async function restore(user: PanelUser): Promise<void> {
    await mutate(user, async () => {
      if (scope === 'global') {
        await updateUser(user.account.id, {
          global_role: user.global_role,
          status: 'active',
          expected_revision: user.revision,
        });
        feedback = `Unbanned @${user.account.login}`;
      } else {
        const targetAccess = requiredTargetAccess(user);
        await updateTargetUser(targetId, user.account.id, {
          role: targetAccess.role,
          suspended: false,
          expected_revision: targetAccess.revision,
        });
        feedback = `Restored @${user.account.login} for ${targetName}`;
      }
    });
  }

  async function mutate(user: PanelUser, operation: () => Promise<void>): Promise<void> {
    savingAccount = user.account.id;
    failure = null;
    try {
      await operation();
      cancelAction();
      await load();
    } catch (error) {
      failure = errorMessage(error);
      await load();
    } finally {
      savingAccount = null;
    }
  }

  async function reissue(invitation: PanelInvitation): Promise<void> {
    invitationBusy = invitation.id;
    failure = null;
    try {
      const updated = await reissueInvitation(invitation.id, expiresInDays);
      generatedLink = updated.invite_url ?? '';
      feedback = `Reissued invitation for @${invitation.account.login}`;
      await load();
    } catch (error) {
      failure = errorMessage(error);
    } finally {
      invitationBusy = null;
    }
  }

  async function revoke(invitation: PanelInvitation): Promise<void> {
    invitationBusy = invitation.id;
    failure = null;
    try {
      await revokeInvitation(invitation.id);
      feedback = `Revoked invitation for @${invitation.account.login}`;
      await load();
    } catch (error) {
      failure = errorMessage(error);
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
      failure = 'The invitation link could not be copied';
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

  function errorMessage(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }
</script>

<Plate label="Users" tone="lead">
  <div class="scope-row" aria-label="User scope">
    <button
      class="scope-button"
      class:active={scope === 'global'}
      aria-pressed={scope === 'global'}
      onclick={() => onScope('global')}>Global</button
    >
    <button
      class="scope-button"
      class:active={scope === 'target'}
      aria-pressed={scope === 'target'}
      onclick={() => onScope('target')}>{targetName}</button
    >
  </div>

  <div class="management-toolbar">
    <label class="search-field">
      <span class="visually-hidden">Search users</span>
      <input class="text-input" type="search" placeholder="Search users" bind:value={query} />
    </label>
    <div class="access-method" role="group" aria-label="Access method">
      <button
        class="scope-button"
        class:active={accessMethod === 'add'}
        aria-pressed={accessMethod === 'add'}
        onclick={() => (accessMethod = 'add')}>Add</button
      >
      <button
        class="scope-button"
        class:active={accessMethod === 'invite'}
        aria-pressed={accessMethod === 'invite'}
        onclick={() => (accessMethod = 'invite')}>Invite</button
      >
    </div>
    <form
      class="add-user"
      aria-label={accessMethod === 'invite' ? 'Invite GitHub user' : 'Add GitHub user'}
      onsubmit={submitAdd}
    >
      <label>
        <span class="visually-hidden">GitHub login</span>
        <input
          class="text-input"
          autocomplete="off"
          placeholder="GitHub login"
          bind:value={login}
          required
        />
      </label>
      <label>
        <span class="visually-hidden">Role</span>
        <select class="select-input" bind:value={addRole} aria-label="Role">
          {#each addRoles() as role (role)}
            <option value={role}>{roleLabel(role)}</option>
          {/each}
        </select>
      </label>
      {#if accessMethod === 'invite'}
        <label>
          <span class="visually-hidden">Invitation expiry</span>
          <select class="select-input" bind:value={expiresInDays} aria-label="Invitation expiry">
            <option value={1}>1 day</option>
            <option value={7}>7 days</option>
            <option value={30}>30 days</option>
          </select>
        </label>
      {/if}
      <button class="btn btn-signal" type="submit" disabled={adding || login.trim() === ''}>
        {adding
          ? accessMethod === 'invite'
            ? 'Inviting…'
            : 'Adding…'
          : accessMethod === 'invite'
            ? 'Invite'
            : 'Add'}
      </button>
    </form>
  </div>

  {#if generatedLink !== ''}
    <div class="generated-link">
      <label>
        <span>Invitation link</span>
        <input class="text-input mono" readonly value={generatedLink} />
      </label>
      <button class="btn copy-button" onclick={() => void copyGeneratedLink()}>Copy</button>
    </div>
  {/if}

  <div class="stable-feedback" aria-live="polite">{feedback}</div>
  {#if failure !== null}
    <p class="form-error" role="alert">{failure}</p>
  {/if}

  {#if loading}
    <p class="dim">Reading users…</p>
  {:else if visibleUsers.length === 0}
    <p class="dim">
      {query.trim() === '' ? 'No users in this scope' : 'No users match this search'}
    </p>
  {:else}
    <div class="user-table-wrap" role="region" aria-label="Panel users">
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
          {#each visibleUsers as user (user.account.id)}
            <tr>
              <th scope="row">
                <span class="user-identity">
                  <Avatar account={user.account} size={32} />
                  <span>
                    <strong>{user.account.display_name}</strong>
                    <span class="user-login">@{user.account.login}</span>
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
                <Chip tone={statusTone(user)} dot>{statusLabel(user)}</Chip>
                {#if user.ban_reason !== undefined}
                  <span class="status-reason">{user.ban_reason}</span>
                {:else if user.target_access?.suspension_reason !== undefined}
                  <span class="status-reason">{user.target_access.suspension_reason}</span>
                {/if}
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
                  {#if scope === 'global'}
                    {#if user.status === 'banned'}
                      <button class="btn btn-row" onclick={() => void restore(user)}>Unban</button>
                    {:else}
                      <button class="btn btn-row" onclick={() => beginAction(user, 'ban')}
                        >Ban</button
                      >
                    {/if}
                    <button
                      class="btn btn-row btn-stop"
                      onclick={() => beginAction(user, 'remove')}
                    >
                      Remove
                    </button>
                  {:else if user.target_access?.suspended === true}
                    <button class="btn btn-row" onclick={() => void restore(user)}>Restore</button>
                  {:else}
                    <button class="btn btn-row" onclick={() => beginAction(user, 'suspend')}>
                      Suspend
                    </button>
                  {/if}
                {/if}
              </td>
            </tr>
            {#if pendingAccount === user.account.id && pendingAction !== null}
              <tr class="confirm-row">
                <td colspan="5">
                  <form
                    onsubmit={(event) => {
                      event.preventDefault();
                      void confirmAction(user);
                    }}
                  >
                    <strong>
                      {pendingAction === 'remove'
                        ? `Remove @${user.account.login}?`
                        : pendingAction === 'ban'
                          ? `Ban @${user.account.login}?`
                          : `Suspend @${user.account.login} for ${targetName}?`}
                    </strong>
                    {#if pendingAction !== 'remove'}
                      <label>
                        <span class="visually-hidden">Reason</span>
                        <input
                          class="text-input reason-input"
                          placeholder="Reason (optional)"
                          maxlength="500"
                          bind:value={reason}
                        />
                      </label>
                    {/if}
                    <button class="btn btn-row btn-stop" type="submit">Confirm</button>
                    <button class="btn btn-row" type="button" onclick={cancelAction}>Cancel</button>
                  </form>
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  <section class="invitations" aria-labelledby="invitations-heading">
    <div class="section-heading">
      <h3 id="invitations-heading">Invitations</h3>
      <span class="dim">{invitations.length}</span>
    </div>
    {#if loading}
      <p class="dim">Reading invitations…</p>
    {:else if invitations.length === 0}
      <p class="dim">No invitations in this scope</p>
    {:else}
      <div class="user-table-wrap" role="region" aria-label="Panel invitations">
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
                      <span class="user-login">@{invitation.account.login}</span>
                    </span>
                  </span>
                </th>
                <td><Chip tone="signal">{roleLabel(invitation.role)}</Chip></td>
                <td
                  ><Chip tone={invitationTone(invitation.status)} dot>{invitation.status}</Chip></td
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
                  {#if invitation.status === 'pending' || invitation.status === 'expired'}
                    <button
                      class="btn btn-row"
                      disabled={invitationBusy === invitation.id}
                      onclick={() => void reissue(invitation)}>Reissue</button
                    >
                    <button
                      class="btn btn-row btn-stop"
                      disabled={invitationBusy === invitation.id}
                      onclick={() => void revoke(invitation)}>Revoke</button
                    >
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>
</Plate>

<style>
  .scope-row {
    border-bottom: 1px solid var(--rule);
    display: flex;
    gap: 0.25rem;
    margin: -0.375rem 0 1rem;
  }

  .scope-button {
    background: transparent;
    border: 0;
    border-bottom: 2px solid transparent;
    color: var(--dim);
    font-weight: 650;
    min-height: var(--control-height);
    padding: 0 0.75rem;
  }

  .scope-button.active {
    border-bottom-color: var(--signal);
    color: var(--text);
  }

  .management-toolbar,
  .add-user,
  .access-method {
    align-items: center;
    display: flex;
    gap: 0.5rem;
  }

  .management-toolbar {
    flex-wrap: wrap;
    justify-content: space-between;
    margin-bottom: 0.5rem;
  }

  .access-method {
    border-bottom: 1px solid var(--rule);
  }

  .search-field {
    flex: 1 1 14rem;
  }

  .search-field .text-input {
    width: 100%;
  }

  .add-user {
    flex: 1 1 auto;
    justify-content: flex-end;
  }

  .stable-feedback {
    color: var(--clear);
    min-height: 1.4rem;
  }

  .generated-link {
    align-items: end;
    background: var(--well);
    border: 1px solid var(--rule);
    border-radius: var(--r-well);
    display: grid;
    gap: 0.5rem;
    grid-template-columns: minmax(0, 1fr) auto;
    margin: 0.25rem 0 0.75rem;
    padding: 0.75rem;
  }

  .generated-link label {
    display: grid;
    gap: 0.35rem;
  }

  .generated-link label > span,
  .section-heading h3 {
    color: var(--dim);
    font: 600 0.6875rem/1.2 var(--mono);
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .generated-link .text-input {
    width: 100%;
  }

  .copy-button {
    min-width: 4.75rem;
  }

  .invitations {
    border-top: 1px solid var(--rule);
    margin-top: 1rem;
    padding-top: 1rem;
  }

  .section-heading {
    align-items: baseline;
    display: flex;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
  }

  .section-heading h3 {
    margin: 0;
  }

  .invitation-table {
    min-width: 44rem;
  }

  .form-error {
    margin: 0 0 0.75rem;
  }

  .user-table-wrap {
    border: 1px solid var(--rule);
    border-radius: var(--r-well);
    overflow-x: auto;
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

  .user-table tbody tr:not(.confirm-row):hover {
    background: var(--well);
  }

  .user-identity {
    align-items: center;
    display: flex;
    gap: 0.625rem;
    min-width: 12rem;
  }

  .user-identity strong,
  .user-login,
  .status-reason {
    display: block;
  }

  .user-login,
  .status-reason,
  .last-login {
    color: var(--dim);
    font-size: 0.8125rem;
  }

  .status-reason {
    margin-top: 0.25rem;
    max-width: 14rem;
  }

  .role-select {
    min-width: 8.5rem;
  }

  .row-actions {
    text-align: right !important;
    white-space: nowrap;
  }

  .row-actions .btn + .btn {
    margin-left: 0.25rem;
  }

  .confirm-row td {
    background: var(--stop-tint);
  }

  .confirm-row form {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    justify-content: flex-end;
  }

  .confirm-row strong {
    margin-right: auto;
  }

  .reason-input {
    min-width: 15rem;
  }

  @media (max-width: 48rem) {
    .management-toolbar,
    .add-user,
    .access-method {
      align-items: stretch;
    }

    .add-user {
      justify-content: flex-start;
      width: 100%;
    }

    .add-user label:first-child {
      flex: 1;
    }

    .add-user label:first-child .text-input {
      width: 100%;
    }
  }

  @media (max-width: 32rem) {
    .add-user {
      flex-wrap: wrap;
    }

    .add-user label:first-child {
      flex-basis: 100%;
    }

    .generated-link {
      align-items: stretch;
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
