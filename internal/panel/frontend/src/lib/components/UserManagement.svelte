<script lang="ts">
  import { createInfiniteQuery, createQuery, type InfiniteData } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import { useDebounce, useInterval } from 'runed';

  import { PanelApiError } from '../api';
  import { dialogRoute } from '../dialog-route.svelte';
  import { formatRelative, formatTimestamp, formatUntil } from '../format';
  import { receipts } from '../receipts.svelte';
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
    WorkspaceRole,
    PanelUser,
    PanelUserListStatus,
    PanelUserPageRequest,
    PanelUserSort,
    UpdateTargetUserInput,
  } from '../types';
  import CopyableLinkField from './CopyableLinkField.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import FormError from './FormError.svelte';
  import Select from './Select.svelte';
  import Callout from './Callout.svelte';
  import Skeleton from './Skeleton.svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import Pill, { type PillTone } from './Pill.svelte';
  import CopyReceipt from './CopyReceipt.svelte';
  import DecisionHistory from './DecisionHistory.svelte';
  import Icon, { type IconName } from './Icon.svelte';
  import LoginField from './LoginField.svelte';
  import Modal from './Modal.svelte';
  import PageHeader from './PageHeader.svelte';
  import ResultProblem from './ResultProblem.svelte';
  import SearchField from './SearchField.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import EmptyState from './EmptyState.svelte';
  import ListToolsMenu from './ListToolsMenu.svelte';

  type ManagementSection = 'users' | 'invitations';
  type SortDirection = 'ascending' | 'descending';
  /** Name the dialogs in the address, and are the `id` each dialog carries. */
  const ADD_DIALOG = 'add-user';
  const DECISION_DIALOG = 'access-decision';
  const INVITATION_DIALOG = 'invitation-action';
  const HISTORY_DIALOG = 'decision-history';

  type TargetRole = Exclude<WorkspaceRole, 'owner'>;
  type GrantedTargetRole = Exclude<TargetRole, 'none'>;

  const ACCESS_METHODS = [
    { value: 'add', label: 'Add directly', description: 'Grant access immediately' },
    { value: 'invite', label: 'Send invitation', description: 'Create a single-use link' },
  ] as const;

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

  const {
    section: activeSection,
    prefs = EPHEMERAL_PREFS,
    targetId,
    targetName,
    actorLogin,
    actorTargetRole,
    readOnly = false,
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
    actorTargetRole: WorkspaceRole;
    readOnly?: boolean;
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
  let userRoles = $state<WorkspaceRole[]>(
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
  let invitationRoles = $state<Exclude<WorkspaceRole, 'none'>[]>(
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
  let addRole = $state<WorkspaceRole>('viewer');
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

  let decisionTrigger = $state<HTMLElement | null>(null);
  /** Which choice card the decision dialog is standing on, before it is applied. */
  let decisionPick = $state<string | null>(null);
  let reason = $state('');
  let invitationActionTrigger = $state<HTMLElement | null>(null);
  let invitationBusy = $state<string | null>(null);
  let savingAccount = $state<string | null>(null);
  let historyTrigger = $state<HTMLElement | null>(null);

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
  const decisionUser = $derived(findUser(dialogRoute.param(DECISION_DIALOG, 'user')));
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
  const addRoleOptions = $derived(
    addRoles().map((role) => ({ value: role, label: roleLabel(role), icon: roleIcon(role) })),
  );
  /**
   * The three standings the list leads with, and how many people are in each.
   *
   * A standing is not one column. "Suspended" is a status the server filters on and
   * "removed" is the role `none`, so a segment owns BOTH lists rather than one each -
   * which is why the tools menu beside them carries sorts and no filters, and why the
   * stored role and status preferences are still what this writes. Every count carries
   * the search, because a count that ignores what is on screen counts something else.
   */
  const USER_VIEWS = {
    all: { roles: [], statuses: [] },
    suspended: { roles: [], statuses: ['suspended'] },
    removed: { roles: ['none'], statuses: [] },
  } as const satisfies Record<
    string,
    { roles: readonly WorkspaceRole[]; statuses: readonly PanelUserListStatus[] }
  >;
  type UserView = keyof typeof USER_VIEWS;
  const USER_VIEW_NAMES = Object.keys(USER_VIEWS) as UserView[];

  const INVITATION_VIEWS = {
    all: [],
    waiting: ['pending'],
    accepted: ['accepted'],
    expired: ['expired'],
  } as const satisfies Record<string, readonly InvitationStatus[]>;
  type InvitationView = keyof typeof INVITATION_VIEWS;
  const INVITATION_VIEW_NAMES = Object.keys(INVITATION_VIEWS) as InvitationView[];

  /** Whether two selections say the same thing, whatever order they were stored in. */
  function sameSelection(a: readonly string[], b: readonly string[]): boolean {
    return a.length === b.length && a.every((value) => b.includes(value));
  }

  /* `null` when the stored filters are some other combination - the segments show no
     selection rather than claiming one they are not applying. */
  const userView = $derived(
    USER_VIEW_NAMES.find(
      (name) =>
        sameSelection(USER_VIEWS[name].roles, userRoles) &&
        sameSelection(USER_VIEWS[name].statuses, userStatuses),
    ) ?? null,
  );

  const invitationView = $derived(
    INVITATION_VIEW_NAMES.find((name) =>
      sameSelection(INVITATION_VIEWS[name], invitationStatuses),
    ) ?? null,
  );

  function selectUserView(value: string): void {
    const view = USER_VIEWS[value as UserView];
    if (view === undefined) return;
    userRoles = [...view.roles];
    userStatuses = [...view.statuses];
  }

  function selectInvitationView(value: string): void {
    const statuses = INVITATION_VIEWS[value as InvitationView];
    if (statuses === undefined) return;
    invitationStatuses = [...statuses];
  }

  const userCountsQuery = createQuery(() => ({
    queryKey: ['user-view-counts', targetId, userQuery],
    queryFn: async (): Promise<Record<UserView, number>> => {
      const pages = await Promise.all(
        USER_VIEW_NAMES.map((name) =>
          fetchTargetUsers(targetId, {
            query: userQuery,
            // Fixed, because a total does not depend on the order it is read in.
            sort: 'name_asc',
            limit: 1,
            roles: [...USER_VIEWS[name].roles],
            statuses: [...USER_VIEWS[name].statuses],
          }),
        ),
      );
      return Object.fromEntries(
        USER_VIEW_NAMES.map((name, index) => [name, pages[index]?.total ?? 0]),
      ) as Record<UserView, number>;
    },
  }));

  const invitationCountsQuery = createQuery(() => ({
    queryKey: ['invitation-view-counts', targetId, invitationQuery, [...invitationRoles]],
    queryFn: async (): Promise<Record<InvitationView, number>> => {
      const pages = await Promise.all(
        INVITATION_VIEW_NAMES.map((name) =>
          fetchTargetInvitations(targetId, {
            query: invitationQuery,
            sort: 'name_asc',
            limit: 1,
            roles: [...invitationRoles],
            statuses: [...INVITATION_VIEWS[name]],
          }),
        ),
      );
      return Object.fromEntries(
        INVITATION_VIEW_NAMES.map((name, index) => [name, pages[index]?.total ?? 0]),
      ) as Record<InvitationView, number>;
    },
  }));

  const USER_SEGMENTS = $derived.by(() => {
    const counts = userCountsQuery.data;
    const badge = (name: UserView): string | undefined =>
      counts === undefined ? undefined : String(counts[name]);
    return [
      { value: 'all', label: 'All', badge: badge('all') },
      { value: 'suspended', label: 'Suspended', badge: badge('suspended') },
      { value: 'removed', label: 'Removed', badge: badge('removed') },
    ];
  });

  const INVITATION_SEGMENTS = $derived.by(() => {
    const counts = invitationCountsQuery.data;
    const badge = (name: InvitationView): string | undefined =>
      counts === undefined ? undefined : String(counts[name]);
    return [
      { value: 'all', label: 'All', badge: badge('all') },
      { value: 'waiting', label: 'Waiting', badge: badge('waiting') },
      { value: 'accepted', label: 'Accepted', badge: badge('accepted') },
      { value: 'expired', label: 'Expired', badge: badge('expired') },
    ];
  });

  /* The order each list is read in, now that there are no column headings to carry it:
     one sort at a time, each with its two directions, chosen from the tools menu. */
  const USER_SORT_PAIRS = {
    name: ['name_asc', 'name_desc'],
    role: ['role_asc', 'role_desc'],
    last_login: ['login_oldest', 'login_newest'],
  } as const satisfies Record<string, readonly [PanelUserSort, PanelUserSort]>;

  const INVITATION_SORT_PAIRS = {
    name: ['name_asc', 'name_desc'],
    role: ['role_asc', 'role_desc'],
    expires: ['expiry_soonest', 'expiry_latest'],
    created: ['created_oldest', 'created_newest'],
  } as const satisfies Record<string, readonly [InvitationSort, InvitationSort]>;

  function toggleUserSort(column: keyof typeof USER_SORT_PAIRS): void {
    const [ascending, descending] = USER_SORT_PAIRS[column];
    userSort = userSort === ascending ? descending : ascending;
  }

  function userSortDirection(column: keyof typeof USER_SORT_PAIRS): SortDirection | undefined {
    const [ascending, descending] = USER_SORT_PAIRS[column];
    if (userSort === ascending) return 'ascending';
    return userSort === descending ? 'descending' : undefined;
  }

  function toggleInvitationSort(column: keyof typeof INVITATION_SORT_PAIRS): void {
    const [ascending, descending] = INVITATION_SORT_PAIRS[column];
    invitationSort = invitationSort === ascending ? descending : ascending;
  }

  function invitationSortDirection(
    column: keyof typeof INVITATION_SORT_PAIRS,
  ): SortDirection | undefined {
    const [ascending, descending] = INVITATION_SORT_PAIRS[column];
    if (invitationSort === ascending) return 'ascending';
    return invitationSort === descending ? 'descending' : undefined;
  }

  /** What each list is showing of what there is, said once at its foot. */
  const shownUsers = $derived(shownRange(users.length, userPage?.total));
  const shownInvitations = $derived(shownRange(invitations.length, invitationPage?.total));

  /* The space before "of" is a non-breaking one, written as an escape: the count and
     what it counts are one atom, and a literal one is invisible to anybody reading the
     source afterwards. */
  function shownRange(shown: number, total: number | undefined): string {
    if (shown === 0) return 'Nothing to show';
    return `Showing 1-${shown}\u{a0}of ${total ?? shown}`;
  }

  /**
   * What a person's row says about them: their handle, then their standing and the one
   * fact that explains it - never a state pill beside a sentence saying the same thing.
   */
  function userSentence(user: PanelUser): string {
    const access = user.target_access;
    const parts = [`@${user.account.login}`];
    if (user.status === 'banned') {
      parts.push(`banned${since(user.banned_at)} - sign-in is refused everywhere`);
    } else if (access?.suspended === true) {
      parts.push(
        `suspended${since(access.updated_at)} - sign-in is refused until an administrator lifts it`,
      );
    } else if (shownRole(user) === 'none') {
      parts.push(`access removed${since(access?.updated_at)} - the audit holds the reason`);
    } else if (access?.source === 'root') {
      parts.push('operator - reaches every workspace');
    } else if (user.last_login_at === undefined) {
      parts.push('has never opened the panel');
    } else {
      parts.push(`last opened ${formatRelative(user.last_login_at, now)}`);
    }
    return parts.join(' · ');
  }

  /** A decision the row can date, or nothing at all rather than a guess at when. */
  function since(at: string | undefined): string {
    return at === undefined ? '' : ` ${formatRelative(at, now)}`;
  }

  /**
   * The exact instant the sentence is being relative about, for the row's tooltip.
   *
   * A sentence says "2 days ago" because that is what a reader wants at a glance; the
   * stamp is what they want when the answer matters, and it rides the sentence rather
   * than replacing it.
   */
  function userStamp(user: PanelUser): string | undefined {
    const access = user.target_access;
    const at =
      user.status === 'banned'
        ? user.banned_at
        : access?.suspended === true || shownRole(user) === 'none'
          ? access?.updated_at
          : user.last_login_at;
    return at === undefined ? undefined : formatTimestamp(at);
  }

  function invitationStamp(invitation: PanelInvitation): string {
    if (invitation.status === 'pending') {
      return `Expires ${formatTimestamp(invitation.expires_at)}`;
    }
    if (invitation.status === 'expired') return formatTimestamp(invitation.expires_at);
    return formatTimestamp(invitation.responded_at ?? invitation.created_at);
  }

  /**
   * What an invitation's row says: who asked, and what became of it.
   *
   * An expired row can never show a future date, because its state and its time come
   * from the same clock.
   */
  function invitationSentence(invitation: PanelInvitation): string {
    const asked = `Invited by ${invitation.created_by.display_name} ${formatRelative(
      invitation.created_at,
      now,
    )}`;
    const answered = invitation.responded_at ?? invitation.created_at;
    switch (invitation.status) {
      case 'pending':
        return `${asked} · the link works once and expires ${formatUntil(
          invitation.expires_at,
          now,
        )}`;
      case 'accepted':
        return `Accepted ${formatRelative(
          answered,
          now,
        )} - their access now lives on the Users page`;
      case 'declined':
        return `Declined ${formatRelative(answered, now)} - asking again is visible to them`;
      case 'revoked':
        return `Revoked ${formatRelative(answered, now)} - the link stopped working`;
      default:
        return `Never opened · expired ${formatRelative(invitation.expires_at, now)}`;
    }
  }

  useInterval(30_000, { callback: () => (now = Date.now()) });
  const debouncedUserSearch = useDebounce((value: string) => {
    userQuery = value;
  }, 180);
  const debouncedInvitationSearch = useDebounce((value: string) => {
    invitationQuery = value;
  }, 180);
  $effect(() => {
    const value = userSearch.trim();
    untrack(() => void debouncedUserSearch(value));
  });

  $effect(() => {
    const value = invitationSearch.trim();
    untrack(() => void debouncedInvitationSearch(value));
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
    await usersQuery.refetch();
  }

  async function reloadInvitations(): Promise<void> {
    await invitationsQuery.refetch();
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
        /* Sticky: the link is on the clipboard and the reader is about to paste it
           somewhere. A receipt that took itself away mid-paste would be the only
           record that it worked. */
        receipts.say(
          `Invite link made for @${normalizedLogin} - it works once and expires in ${expiresInDays} ${expiresInDays === 1 ? 'day' : 'days'}`,
          { sticky: true },
        );
        await reloadInvitations();
      } else {
        await addTargetUser(targetId, {
          login: normalizedLogin,
          role: addRole as GrantedTargetRole,
        });
        receipts.say(`Access granted - @${normalizedLogin} can open ${destination} now`);
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

  async function mutate(user: PanelUser, operation: () => Promise<void>): Promise<void> {
    savingAccount = user.account.id;
    actionFailure = null;
    try {
      await operation();
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
      receipts.say(`A fresh link is out for @${invitation.account.login}`, { sticky: true });
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
      receipts.say(`Invitation for @${invitation.account.login} revoked`);
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
      if (announce) receipts.say('Copied the invite link');
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
    if (decisionUser !== null) return;
    untrack(() => {
      reason = '';
      decisionPick = null;
    });
  });

  function hasDecisionHistory(user: PanelUser): boolean {
    return user.status === 'banned' || user.target_access?.suspended === true;
  }

  function openHistory(user: PanelUser, trigger: HTMLElement): void {
    if (!hasDecisionHistory(user)) return;
    historyTrigger = trigger;
    dialogRoute.open(HISTORY_DIALOG, { user: user.account.login });
  }

  function closeHistory(): void {
    if (dialogRoute.isOpen(HISTORY_DIALOG)) dialogRoute.close();
  }

  /**
   * The one act a person's row offers, and what it is called.
   *
   * One control rather than a menu: a menu of three where two are greyed says nothing a
   * reader can act on, and your own row simply has no button. What the act *is* comes
   * from the standing the row is already showing, so the button and the sentence can
   * never disagree.
   */
  type DecisionKind = 'role' | 'suspension' | 'restore';

  interface AccessChoice {
    value: string;
    title: string;
    why: string;
  }

  function decisionKind(user: PanelUser): DecisionKind {
    if (user.target_access?.suspended === true) return 'suspension';
    return shownRole(user) === 'none' ? 'restore' : 'role';
  }

  function decisionLabel(user: PanelUser): string {
    switch (decisionKind(user)) {
      case 'suspension':
        return 'Review suspension';
      case 'restore':
        return 'Restore access';
      default:
        return 'Change access';
    }
  }

  /** Whether this row has an act at all: a ban is decided in the console, not here. */
  function decidable(user: PanelUser): boolean {
    return !readOnly && user.manageable && user.status !== 'banned';
  }

  const ROLE_REASONS: Record<string, string> = {
    viewer: 'Sees everything, changes nothing',
    editor: 'Runs commands and edits sync',
    admin: 'Everything, access included',
  };

  function roleChoices(): AccessChoice[] {
    return addRoles().map((role) => ({
      value: `role:${role}`,
      title: roleLabel(role),
      why: ROLE_REASONS[role] ?? '',
    }));
  }

  function decisionChoices(user: PanelUser): AccessChoice[] {
    switch (decisionKind(user)) {
      case 'suspension':
        return [
          { value: 'keep', title: 'Keep suspended', why: 'Sign-in stays refused' },
          {
            value: 'restore',
            title: 'Lift the suspension',
            why: 'They can open this workspace again straight away',
          },
        ];
      case 'restore':
        return [
          { value: 'keep', title: 'Leave removed', why: 'Nothing changes' },
          ...roleChoices().map((choice) => ({
            ...choice,
            title: `Restore as ${choice.title}`,
          })),
        ];
      default:
        return [
          ...roleChoices(),
          {
            value: 'role:none',
            title: 'No access',
            why: 'Keeps their account and closes this workspace to them',
          },
          {
            value: 'suspend',
            title: 'Suspend access',
            why: 'Sign-in is refused until an administrator lifts it',
          },
        ];
    }
  }

  function decisionCurrent(user: PanelUser): string {
    return decisionKind(user) === 'role' ? `role:${selectedRole(user)}` : 'keep';
  }

  function decisionTitle(user: PanelUser): string {
    switch (decisionKind(user)) {
      case 'suspension':
        return `Review the suspension of @${user.account.login}`;
      case 'restore':
        return `Restore the access of @${user.account.login}`;
      default:
        return `Change the access of @${user.account.login}`;
    }
  }

  /** The verb names the act, never a generic Save. */
  function decisionVerb(user: PanelUser): string {
    switch (decisionKind(user)) {
      case 'suspension':
        return 'Decide the suspension';
      case 'restore':
        return 'Restore access';
      default:
        return 'Change the access';
    }
  }

  function openDecision(user: PanelUser, trigger: HTMLElement): void {
    decisionTrigger = trigger;
    reason = '';
    decisionPick = decisionCurrent(user);
    dialogRoute.open(DECISION_DIALOG, { user: user.account.login });
  }

  function closeDecision(): void {
    if (dialogRoute.isOpen(DECISION_DIALOG)) dialogRoute.close();
  }

  async function applyDecision(): Promise<void> {
    const user = decisionUser;
    const pick = decisionPick;
    if (user === null || pick === null || pick === decisionCurrent(user)) {
      closeDecision();
      return;
    }
    await mutate(user, async () => {
      const access = requiredTargetAccess(user);
      const role = pick.startsWith('role:') ? (pick.slice(5) as TargetRole) : access.role;
      const suspend = pick === 'suspend';
      await updateTargetUser(targetId, user.account.id, {
        role: role as TargetRole,
        suspended: suspend,
        suspension_reason: suspend ? reason.trim() || undefined : undefined,
        expected_revision: access.revision,
      });
      receipts.say(decisionFeedback(user, pick));
    });
    // A refused write leaves the dialog standing on the choice that was refused, so the
    // reason it gives is beside the thing it is about.
    if (actionFailure === null) closeDecision();
  }

  function decisionFeedback(user: PanelUser, pick: string): string {
    const handle = `@${user.account.login}`;
    if (pick === 'suspend') return `Suspended ${handle} for ${targetName}`;
    if (pick === 'restore') return `Lifted the suspension of ${handle}`;
    if (pick === 'role:none') return `Removed ${handle} from ${targetName}`;
    return `${handle} is now ${roleLabel(pick.slice(5) as WorkspaceRole)} in ${targetName}`;
  }

  /**
   * The acts an invitation offers, which are only the ones its state supports.
   *
   * A settled invitation drops its menu rather than greying one out, and a waiting one
   * hands over the link it already made - a fresh link is what "invite again" is for,
   * on the row where the old one is dead.
   */
  function askRevoke(invitation: PanelInvitation, trigger: HTMLElement): void {
    invitationActionTrigger = trigger;
    dialogRoute.open(INVITATION_DIALOG, { invitation: invitation.id, action: 'revoke' });
  }

  async function copyInvitationLink(invitation: PanelInvitation): Promise<void> {
    const url = invitation.invite_url;
    if (url === undefined) return;
    try {
      await navigator.clipboard.writeText(url);
      receipts.say(`Copied the invite link for @${invitation.account.login}`);
    } catch {
      actionFailure = 'The invitation link could not be copied';
    }
  }

  function requiredTargetAccess(user: PanelUser): NonNullable<PanelUser['target_access']> {
    const access = user.target_access;
    if (access === undefined) throw new Error('workspace access is missing');
    return access;
  }

  function addRoles(): WorkspaceRole[] {
    return actorTargetRole === 'owner' ? ['viewer', 'editor', 'admin'] : ['viewer', 'editor'];
  }

  function selectedRole(user: PanelUser): string {
    return user.target_access?.role ?? 'none';
  }

  function shownRole(user: PanelUser): WorkspaceRole {
    return user.target_access?.effective_role ?? 'none';
  }

  function statusLabel(user: PanelUser): string {
    if (user.status === 'banned') return 'Banned';
    if (user.target_access?.suspended === true) return 'Suspended';
    if (shownRole(user) === 'none') return 'Access removed';
    return 'Active';
  }

  /**
   * The state pill a row wears, or nothing at all when the state is the ordinary one.
   *
   * "Active" beside a role said nothing the role had not already said, on every row, so
   * the only thing a colour could mean was drowned out by the rows where it meant
   * nothing.
   */
  function stateLabel(user: PanelUser): string | null {
    const label = statusLabel(user);
    return label === 'Active' ? null : label;
  }

  function statusTone(user: PanelUser): PillTone {
    // Banned is permanent (red); suspended is a pause an administrator can
    // lift (amber). They carried identical chips once, and nobody could tell
    // the two states apart at a glance.
    if (user.status === 'banned') return 'danger';
    if (user.target_access?.suspended === true) return 'warning';
    if (shownRole(user) === 'none') return 'danger';
    return 'success';
  }

  /** Only the standing that outranks everyone else's is tinted; the rest are words. */
  function roleTone(role: WorkspaceRole): PillTone {
    return role === 'owner' ? 'role' : 'bare';
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

  function invitationTone(status: PanelInvitation['status']): PillTone {
    // Waiting is the one state still asking for something (amber); a dead link is red
    // whether it ran out or was taken back. Declined is the invitee's own answer rather
    // than a failure, and stays quiet.
    if (status === 'pending') return 'warning';
    if (status === 'accepted') return 'success';
    if (status === 'declined') return 'neutral';
    return 'danger';
  }

  /** "Waiting", not "Pending": the row says what it is doing, not what it is called. */
  function invitationStatusLabel(status: InvitationStatus): string {
    if (status === 'pending') return 'Waiting';
    return status.charAt(0).toUpperCase() + status.slice(1);
  }

  function roleLabel(role: WorkspaceRole): string {
    if (role === 'none') return 'No access';
    return role[0]?.toLocaleUpperCase() + role.slice(1);
  }

  function roleIcon(role: WorkspaceRole): IconName {
    if (role === 'owner') return 'owner';
    if (role === 'admin') return 'admin';
    if (role === 'editor') return 'editor';
    if (role === 'viewer') return 'viewer';
    return 'no-access';
  }

  function selectInvitationFilters(values: string[]): void {
    invitationRoles = values.filter((value): value is Exclude<WorkspaceRole, 'none'> =>
      ['admin', 'editor', 'viewer'].includes(value),
    );
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

<!--
@component
Who may act on one workspace, and who has been asked. Users and invitations are
sections of one page for the same reason they are in the Root console's access page:
the answer to "why can this person not do that" is in whichever of the two they are
in.

Both lists are object rows rather than tables: a person is a name, the standing they
hold, and the one sentence that explains it - never a state column beside a reason
column saying the same thing twice. The order they are read in and the invitation role
filter live in the tools menu beside the search, because there are no column headings
left to carry them.

`actorTargetRole` is what decides which acts are drawn. A member cannot change a role
above their own, and offering the control and refusing it afterwards is worse than not
offering it.
-->

{#snippet headerActions()}
  <Button tone="signal" bind:element={addButton} onclick={openAddModal}>
    {#snippet icon()}<Icon name="plus" size="sm" strokeWidth={2} />{/snippet}
    {invitingFirst ? 'Create invite link' : 'Add someone'}
  </Button>
{/snippet}

<section class="plate user-management" aria-labelledby="user-management-heading">
  <PageHeader
    id="user-management-heading"
    section="Access"
    title={activeSection === 'users' ? 'Users' : 'Invitations'}
    description={activeSection === 'users'
      ? 'People with current, suspended, or previous access to this workspace'
      : 'Open invitations and their outcomes'}
    actions={readOnly ? undefined : headerActions}
  />

  {#if activeSection === 'users'}
    <div class="filter-bar">
      <SearchField
        label="Search users"
        placeholder="Find a person"
        value={userSearch}
        onInput={(value) => (userSearch = value)}
      />
      <!-- The standing the list leads with. Everything narrower - the order it is
           read in - stays in the tools menu beside it. -->
      <SegmentedControl
        name="user-state"
        label="Show"
        options={USER_SEGMENTS}
        value={userView}
        onSelect={selectUserView}
      />
      <span class="push-end">
        <ListToolsMenu
          sorts={[
            {
              label: 'Name',
              direction: userSortDirection('name'),
              onToggle: () => toggleUserSort('name'),
            },
            {
              label: 'Role',
              direction: userSortDirection('role'),
              onToggle: () => toggleUserSort('role'),
            },
            {
              label: 'Last opened',
              direction: userSortDirection('last_login'),
              onToggle: () => toggleUserSort('last_login'),
            },
          ]}
          filters={[]}
        />
      </span>
    </div>
  {:else}
    <div class="filter-bar">
      <SearchField
        label="Search invitations"
        placeholder="Find an invitation"
        value={invitationSearch}
        onInput={(value) => (invitationSearch = value)}
      />
      <SegmentedControl
        name="invitation-state"
        label="State"
        options={INVITATION_SEGMENTS}
        value={invitationView}
        onSelect={selectInvitationView}
      />
      <span class="push-end">
        <ListToolsMenu
          sorts={[
            {
              label: 'Invitee',
              direction: invitationSortDirection('name'),
              onToggle: () => toggleInvitationSort('name'),
            },
            {
              label: 'Role',
              direction: invitationSortDirection('role'),
              onToggle: () => toggleInvitationSort('role'),
            },
            {
              label: 'Expires',
              direction: invitationSortDirection('expires'),
              onToggle: () => toggleInvitationSort('expires'),
            },
            {
              label: 'Created',
              direction: invitationSortDirection('created'),
              onToggle: () => toggleInvitationSort('created'),
            },
          ]}
          filters={[
            {
              label: 'Role',
              hint: 'Filter by invited permission level',
              sections: INVITATION_ROLE_FILTERS,
              selected: invitationRoles,
              multiple: true,
              onChange: selectInvitationFilters,
            },
          ]}
        />
      </span>
    </div>
  {/if}

  <FormError message={failure} />

  {#if activeSection === 'users'}
    <div id="users-list-panel" aria-label="Users">
      <div class={['user-results', loadingUsers && 'loading']} aria-busy={loadingUsers}>
        {#if loadingUsers && userPage === null}
          <Skeleton
            label="Loading users"
            --skeleton-bar-top="1.15rem"
            --skeleton-bar-a-width="min(13rem, 28%)"
          />
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
          {#if users.length === 0}
            {@const narrowed = userQuery !== '' || userView !== 'all'}
            <Card>
              <EmptyState
                title={narrowed ? 'Nobody matches' : 'Just you'}
                description={narrowed
                  ? 'Try another search, or show everyone'
                  : 'Nobody else has access to this workspace yet'}
                actionLabel={narrowed ? 'Show everyone' : undefined}
                onAction={narrowed ? clearUserFilters : undefined}
              />
            </Card>
          {:else}
            <Card>
              <ul class="object-list">
                {#each users as user (user.account.id)}
                  {@const state = stateLabel(user)}
                  <li>
                    <div class="object-row person-row">
                      {#if hasDecisionHistory(user)}
                        <!-- The decisions behind a suspension or a ban, under the whole
                               row: the address is a layer UNDER the content, so the one act
                               the row offers is still pressable inside it. -->
                        <button
                          class="row-hit"
                          type="button"
                          aria-label="Review the access decisions for @{user.account.login}"
                          onclick={(event) => openHistory(user, event.currentTarget)}
                        ></button>
                      {/if}
                      <span class="object-main">
                        <span class="object-name-row">
                          <span class="object-name">{user.account.display_name}</span>
                          <!-- A removed row says so once. The role IS "No access" there,
                                 and a pill for each would be the same word twice. -->
                          {#if shownRole(user) !== 'none'}
                            <Pill tone={roleTone(shownRole(user))}>
                              {roleLabel(shownRole(user))}
                            </Pill>
                          {/if}
                          {#if state !== null}
                            <Pill tone={statusTone(user)}>{state}</Pill>
                          {/if}
                        </span>
                        <span class="object-sum" title={userStamp(user)}>
                          {userSentence(user)}
                        </span>
                      </span>
                      <span class="object-side">
                        {#if decidable(user)}
                          <Button
                            tone="quiet"
                            disabled={savingAccount === user.account.id}
                            aria-label="{decisionLabel(user)} for @{user.account.login}"
                            onclick={(event) => openDecision(user, event.currentTarget)}
                          >
                            {decisionLabel(user)}
                          </Button>
                        {/if}
                      </span>
                    </div>
                  </li>
                {/each}
              </ul>
              <div class="list-foot">
                <span>{shownUsers}</span>
                {#if userPage?.next_cursor != null}
                  <Button tone="quiet" disabled={loadingUsers} onclick={() => void loadNextUsers()}>
                    Show more
                  </Button>
                {/if}
              </div>
            </Card>
          {/if}
        {/if}
        {#if userLoadMoreFailure !== null}
          <div class="load-more-alert" role="alert">
            <span>{userLoadMoreFailure}</span>
            <Button onclick={() => void loadNextUsers()}>Try again</Button>
          </div>
        {/if}
      </div>
    </div>
  {:else}
    <div id="invitations-list-panel" aria-label="Invitations">
      <div
        class={['invitation-results', loadingInvitations && 'loading']}
        aria-busy={loadingInvitations}
      >
        {#if loadingInvitations && invitationPage === null}
          <Skeleton
            label="Loading invitations"
            --skeleton-bar-top="1.15rem"
            --skeleton-bar-a-width="min(13rem, 28%)"
          />
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
          {#if invitations.length === 0}
            {@const narrowed = invitationQuery !== '' || invitationView !== 'all'}
            <Card>
              <EmptyState
                title={narrowed ? 'No invitations match' : 'No invitations are out'}
                description={narrowed
                  ? 'Try another search, or show them all'
                  : 'When you make an invite link, it and its state live here until it is redeemed or expires'}
                actionLabel={narrowed ? 'Show them all' : undefined}
                onAction={narrowed ? clearInvitationFilters : undefined}
              />
            </Card>
          {:else}
            <Card>
              <ul class="object-list">
                {#each invitations as invitation (invitation.id)}
                  {@const settled =
                    invitation.status !== 'pending' && invitation.status !== 'expired'}
                  <li>
                    <div class="object-row">
                      <span class="object-main">
                        <span class="object-name-row">
                          <span class="object-name">{invitation.account.login}</span>
                          {#if invitation.role !== undefined}
                            <Pill>{roleLabel(invitation.role)}</Pill>
                          {/if}
                          <Pill tone={invitationTone(invitation.status)}>
                            {invitationStatusLabel(invitation.status)}
                          </Pill>
                        </span>
                        <span class="object-sum" title={invitationStamp(invitation)}>
                          {invitationSentence(invitation)}
                        </span>
                      </span>
                      <!-- Each state offers only the acts it supports, and a settled one
                             offers none: an accepted invitation's person lives on Users
                             now, and a dead link cannot be revoked twice. -->
                      <span class="object-side">
                        {#if !readOnly && !settled}
                          {#if invitation.status === 'pending'}
                            {#if invitation.invite_url !== undefined}
                              <Button
                                tone="quiet"
                                aria-label="Copy the invite link for @{invitation.account.login}"
                                onclick={() => void copyInvitationLink(invitation)}
                              >
                                Copy link
                              </Button>
                            {:else}
                              <!-- The link this row was made with is not ours to show a
                                     second time, so the way to hand it over is a new one. -->
                              <Button
                                tone="quiet"
                                disabled={invitationBusy === invitation.id}
                                aria-label="Make a fresh link for @{invitation.account.login}"
                                onclick={(event) => void reissue(invitation, event.currentTarget)}
                              >
                                Make a fresh link
                              </Button>
                            {/if}
                            <Button
                              tone="quiet"
                              disabled={invitationBusy === invitation.id}
                              aria-label="Revoke the invitation for @{invitation.account.login}"
                              onclick={(event) => askRevoke(invitation, event.currentTarget)}
                            >
                              Revoke
                            </Button>
                          {:else}
                            <Button
                              tone="quiet"
                              disabled={invitationBusy === invitation.id}
                              aria-label="Invite again - @{invitation.account.login}"
                              onclick={(event) => void reissue(invitation, event.currentTarget)}
                            >
                              Invite again
                            </Button>
                          {/if}
                        {/if}
                      </span>
                    </div>
                  </li>
                {/each}
              </ul>
              <div class="list-foot">
                <span>{shownInvitations}</span>
                {#if invitationPage?.next_cursor != null}
                  <Button
                    tone="quiet"
                    disabled={loadingInvitations}
                    onclick={() => void loadNextInvitations()}
                  >
                    Show more
                  </Button>
                {/if}
              </div>
            </Card>
          {/if}
        {/if}
        {#if invitationLoadMoreFailure !== null}
          <div class="load-more-alert" role="alert">
            <span>{invitationLoadMoreFailure}</span>
            <Button onclick={() => void loadNextInvitations()}>Try again</Button>
          </div>
        {/if}
      </div>
    </div>
  {/if}
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
      <Callout>
        {#snippet icon()}<span class="warning-mark" aria-hidden="true">!</span>{/snippet}
        <div>
          <strong>Declining was an answer</strong>
          <p>
            A new link reaches the same GitHub identity, and asking twice is visible to them and in
            the audit record.
          </p>
        </div>
      </Callout>
    {:else if addStage === 'form'}
      <div class="add-scope-summary band-trim-kids">
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
                <Icon name={method.value === 'add' ? 'plus' : 'mail'} size="sm" strokeWidth={2} />
              </span>
              <span class="method-copy band-trim-kids">
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
          <Select
            value={addRole}
            aria-label="Role"
            onchange={(event) => (addRole = event.currentTarget.value as WorkspaceRole)}
          >
            {#each addRoleOptions as option (option.value)}
              <option value={option.value}>{option.label}</option>
            {/each}
          </Select>
        </label>
        {#if accessMethod === 'invite'}
          <label class="form-field">
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
        {/if}
      </div>
    {:else}
      <!-- Both lines trimmed to their ink, so `align-items: center` centres what
           you can see rather than two line boxes carrying half-leading the eye
           does not read. -->
      <div class="invitation-created band-trim-kids" aria-live="polite">
        <span class="success-mark" aria-hidden="true">✓</span>
        <div>
          <strong>Invitation ready</strong>
          <p>Share this single-use link with the invited GitHub user</p>
        </div>
      </div>
      <CopyableLinkField
        label="Invitation link"
        value={generatedLink}
        failed={linkCopied === 'failed'}
      />
    {/if}
    <FormError message={addFailure} />
  </form>

  {#snippet footer()}
    {#if addStage === 'confirm'}
      <Button tone="ghost" onclick={() => (declinedLogin = null)}>Back</Button>
      <Button tone="signal" disabled={adding} onclick={() => void grantAccess(true)}>
        {adding ? 'Sending…' : 'Invite again'}
      </Button>
    {:else}
      <Button tone="ghost" onclick={closeAddModal}>
        {addStage === 'form' ? 'Cancel' : 'Done'}
      </Button>
    {/if}
    {#if addStage === 'form'}
      <Button
        tone="signal"
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
      </Button>
    {:else if addStage === 'link'}
      <Button tone="signal" onclick={() => void copyGeneratedLink()}>Copy link</Button>
    {/if}
  {/snippet}
</Modal>

{#if decisionUser !== null}
  {@const user = decisionUser}
  <Modal
    id={DECISION_DIALOG}
    open
    title={decisionTitle(user)}
    description={decisionKind(user) === 'suspension'
      ? (currentReason(user) ?? 'The audit holds the reason')
      : undefined}
    returnFocus={decisionTrigger}
    onClose={closeDecision}
  >
    <!-- One radiogroup, because these are answers to one question rather than a menu
         of separate acts: what standing should this person have. Each card says what
         it does, so nothing has to be tried to find out. -->
    <div class="choice-cards ask-cards" role="radiogroup" aria-label={decisionTitle(user)}>
      {#each decisionChoices(user) as choice (choice.value)}
        <label class="choice-card" class:is-chosen={decisionPick === choice.value}>
          <input
            type="radio"
            name="access-decision"
            value={choice.value}
            checked={decisionPick === choice.value}
            onchange={() => (decisionPick = choice.value)}
          />
          <span class="choice-dot"></span>
          <span class="choice-title">{choice.title}</span>
          <span class="choice-why">{choice.why}</span>
        </label>
      {/each}
    </div>

    {#if decisionPick === 'suspend'}
      <label class="form-field reason-field">
        <span>Reason <small>Optional</small></span>
        <textarea
          class="reason-textarea"
          placeholder="Add context for other administrators"
          maxlength="500"
          rows="3"
          bind:value={reason}></textarea>
        <small>{reason.length}/500 characters</small>
      </label>
    {/if}

    {#snippet footer()}
      <Button tone="ghost" onclick={closeDecision}>Cancel</Button>
      <Button
        tone={decisionPick === 'suspend' || decisionPick === 'role:none' ? 'stop' : 'signal'}
        disabled={savingAccount !== null || decisionPick === decisionCurrent(user)}
        onclick={() => void applyDecision()}
      >
        {savingAccount === null ? decisionVerb(user) : 'Saving…'}
      </Button>
    {/snippet}
  </Modal>
{/if}

<ConfirmDialog
  id={INVITATION_DIALOG}
  open={pendingInvitation !== null}
  title={`Revoke invitation for @${pendingInvitation?.account.login ?? ''}`}
  description="The current link will stop working immediately and the audit record will remain"
  returnFocus={invitationActionTrigger}
  onClose={closeInvitationAction}
  onConfirm={() => void revoke()}
  confirmTone="stop"
  confirmLabel="Revoke invitation"
  busyLabel="Revoking…"
  busy={invitationBusy !== null}
>
  <Callout>
    {#snippet icon()}<span class="warning-mark" aria-hidden="true">!</span>{/snippet}
    <p>The user can only join if you create and share a new invitation</p>
  </Callout>
</ConfirmDialog>

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

  #users-list-panel,
  #invitations-list-panel {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }

  .user-results,
  .invitation-results {
    margin-top: 0;
  }

  /* The one act a row offers is quiet until the row is wanted, which is what keeps a
     column of them reading as a margin rather than as a column of buttons. */
  .person-row :global(.btn-quiet) {
    opacity: 0.75;
  }

  .person-row:hover :global(.btn-quiet),
  .person-row :global(.btn-quiet:focus-visible) {
    opacity: 1;
  }

  .reason-field {
    margin-block-start: var(--space-4);
  }

  .user-results.loading,
  .invitation-results.loading {
    cursor: progress;
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
    font: 700 var(--font-size-micro) / var(--leading-flat) var(--sans);
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .add-scope-summary strong {
    display: block;
    font-size: var(--font-size-body);
    line-height: var(--leading-flat);
    margin-top: 0.65rem;
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
    background: var(--surface-inset);
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
      background-color var(--duration-press) var(--ease-standard),
      border-color var(--duration-press) var(--ease-standard),
      transform var(--duration-press) var(--ease-standard);
  }

  .method-option:hover {
    background: var(--surface-raised);
    border-color: color-mix(in srgb, var(--text-muted) 56%, transparent);
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
    line-height: var(--leading-flat);
  }

  .method-copy small {
    color: var(--text-muted);
    display: block;
    font-size: var(--font-size-micro);
    line-height: var(--leading-flat);
    margin-top: 0.75rem;
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
    font: 600 0.75rem / var(--leading-flat) var(--sans);
  }

  .form-field > span small {
    color: var(--text-muted);
    font-weight: 400;
  }

  .form-field > small {
    color: var(--text-muted);
    font-size: 0.6875rem;
  }

  .identity-grid {
    display: grid;
    gap: 0.75rem;
    grid-template-columns: minmax(0, 1.75fr) minmax(7.5rem, 0.8fr);
  }

  .identity-grid.with-expiry {
    grid-template-columns: minmax(0, 1.75fr) minmax(6.75rem, 0.9fr) minmax(7.5rem, 0.9fr);
  }

  .reason-textarea {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--r-ctl);
    color: var(--text-primary);
    font: 0.8125rem/var(--leading-meta) var(--sans);
    min-height: 6rem;
    padding: 0.625rem;
    resize: vertical;
  }

  .reason-textarea::placeholder {
    color: var(--text-muted);
  }

  .invitation-created {
    align-items: center;
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-well);
    display: flex;
    gap: 0.75rem;
    padding: 0.75rem;
  }

  .invitation-created strong {
    display: block;
    font-size: 0.8125rem;
  }

  .invitation-created p,
  :global(.callout) p {
    color: var(--text-muted);
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

  /* Both tints sit near 1:1 against the card they are on - measured 1.00 to 1.24 across the four
     palettes - so neither disc had an edge. The ring is keyed to the mark's own colour, as the
     avatar monogram's is. */
  .success-mark,
  .warning-mark {
    box-shadow: inset 0 0 0 1px color-mix(in srgb, currentcolor 28%, transparent);
  }

  .success-mark {
    background: var(--success-tint);
    color: var(--success);
  }

  .warning-mark {
    background: var(--danger-tint);
    color: var(--danger);
  }

  /* The row's own stacking is `app.css`'s - every object list does it the same way. */
  @media (max-width: 48rem) {
    .identity-grid.with-expiry {
      grid-template-columns: minmax(0, 1.35fr) repeat(2, minmax(6.5rem, 0.75fr));
    }
  }

  @media (max-width: 36rem) {
    .add-scope-summary {
      grid-template-columns: auto minmax(0, 1fr);
    }

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
