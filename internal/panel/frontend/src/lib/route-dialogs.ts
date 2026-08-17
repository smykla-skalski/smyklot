/**
 * Where a dialog sits in the address.
 *
 * A dialog stands on top of a view and is nearly always about one row of it, so
 * it reads as part of that view's path rather than as a parameter bolted onto
 * it: `/i/acme/repositories/api-gateway/file`, not
 * `/i/acme/repositories?dialog=repository-settings&repository=4005`. The first
 * says what a person would say out loud, survives being pasted into a message,
 * and is the same shape as every other address the panel writes.
 *
 * Two rules keep the grammar unambiguous, which matters because the segments
 * that follow a view are mostly names people chose:
 *
 * - How many segments follow the view decides which dialog it is, before any of
 *   them is read. `/users/add` is one segment and is always the add dialog;
 *   every dialog about a person is two, so somebody whose login is `add` is
 *   still reachable at `/users/add/history`.
 * - A name is only ever read in the first position. `/repositories/file/file` is
 *   the repository called `file`, opened on its File pane.
 *
 * Dialogs with no row and no view to sit on are not here. They keep the query
 * string, which is where the inbox lived until it became a page of its own.
 */

export const REPOSITORY_SECTIONS = ['file', 'behavior', 'commands'] as const;
/* Segment spellings, like `ROOT_USER_ACTIONS` below, because that is what the parser
   compares against. `remove-access` is the only action an installation's user table
   offers besides suspending and restoring, and the list used to say `remove` - so the
   address the panel wrote for it read back as nothing at all, and a reload or a pasted
   link answered 404 for a dialog that had opened perfectly well in the session. */
export const USER_ACTIONS = ['history', 'suspend', 'restore', 'remove-access'] as const;
/* No `history` here, unlike an installation's user table: decisions are made
   inside an installation, so the Root table offers no history and nothing
   renders one. The grammar used to accept the segment anyway, which made
   `/root/access/users/<login>/history` resolve to the table with nothing open
   instead of saying the address does not exist. */
export const ROOT_USER_ACTIONS = [
  'promote-root',
  'demote-root',
  'restore',
  'ban',
  'remove',
] as const;

/** What the address says is open: the dialog's name, and how to find its subject. */
export interface RouteDialog {
  name: string;
  params: Readonly<Record<string, string>>;
}

/**
 * The installation views a dialog can hang off.
 *
 * A list rather than a predicate because the routes are built from it: a view
 * that hosts no dialog has no route with anything after it, so an address that
 * puts something there resolves to nothing and is answered 404 by the server
 * rather than 200 and a not-found page drawn by the browser.
 */
export const DIALOG_HOST_VIEWS = ['repositories', 'users', 'invitations'] as const;

/**
 * The Root console's own tables, which take the same grammar as an
 * installation's because they list the same things and a Root reading a link
 * should not have to learn a second shape.
 */
export const ACCESS_DIALOG_HOSTS = ['access-users', 'access-invitations'] as const;

/** The views a dialog can hang off, named as the panel's own routes name them. */
export type DialogHost = (typeof DIALOG_HOST_VIEWS)[number] | (typeof ACCESS_DIALOG_HOSTS)[number];

const dialogHosts: ReadonlySet<string> = new Set<string>([
  ...DIALOG_HOST_VIEWS,
  ...ACCESS_DIALOG_HOSTS,
]);

export function isDialogHost(view: string): view is DialogHost {
  return dialogHosts.has(view);
}

/**
 * Reads a dialog out of the segments that follow a view.
 *
 * `null` means these segments are not a dialog this view knows, which the caller treats
 * as an address that does not resolve rather than as a view with nothing open - a
 * mistyped repository name should say so, not silently show the list.
 *
 * The segments arrive **decoded**, because that is how the router hands them over:
 * `page.params.rest` has already been through it. Decoding here as well would mean
 * decoding twice, which loses a name holding a per-cent sign - and throws on one where
 * the two characters after it are not hexadecimal. Whoever holds a raw pathname decodes
 * it before calling, which is what `decodeSegments` is exported for.
 */
export function parseDialogSegments(host: DialogHost, decoded: string[]): RouteDialog | null {
  if (decoded.length === 0) return null;

  switch (host) {
    case 'repositories':
      return parseRepositoryDialog(decoded);
    case 'users':
      return parseUserDialog(decoded, 'user-action', 'decision-history', 'add-user', USER_ACTIONS);
    case 'access-users':
      return parseUserDialog(
        decoded,
        'root-user-action',
        null,
        'root-add-installation-user',
        ROOT_USER_ACTIONS,
      );
    case 'invitations':
      return parseInvitationDialog(decoded, 'invitation-action');
    case 'access-invitations':
      return parseInvitationDialog(decoded, 'root-invitation-action', 'root-invitation-create');
  }
}

/** The segments an open dialog adds to its view's path, or `null` for none. */
export function dialogSegments(host: DialogHost, dialog: RouteDialog | null): string[] | null {
  if (dialog === null) return null;

  switch (dialog.name) {
    case 'repository-settings': {
      const repository = dialog.params.repository ?? '';
      if (repository === '') return null;
      const section = dialog.params.section;
      /* The File pane is where the dialog opens, so its address is the bare
         repository. A section is written only when it is not the one a reader
         would have landed on anyway. */
      return section === undefined || section === 'file' ? [repository] : [repository, section];
    }
    case 'decision-history':
      return subjectSegments(dialog.params.user, 'history');
    case 'user-action':
    case 'root-user-action':
      return subjectSegments(dialog.params.user, dialog.params.action);
    case 'add-user':
      return host === 'users' ? ['add'] : null;
    case 'root-add-installation-user':
      return host === 'access-users' ? ['add'] : null;
    case 'invitation-action':
    case 'root-invitation-action':
      return subjectSegments(dialog.params.invitation, dialog.params.action);
    case 'root-invitation-create':
      return host === 'access-invitations' ? ['new'] : null;
    default:
      return null;
  }
}

function subjectSegments(subject: string | undefined, verb: string | undefined): string[] | null {
  if (subject === undefined || subject === '' || verb === undefined || verb === '') return null;

  return [subject, toSegment(verb)];
}

function parseRepositoryDialog(segments: string[]): RouteDialog | null {
  const [repository, section] = segments;
  if (segments.length > 2 || repository === undefined || repository === '') return null;
  if (section === undefined) {
    return { name: 'repository-settings', params: { repository, section: 'file' } };
  }
  if (!REPOSITORY_SECTIONS.some((known) => known === section)) return null;

  return { name: 'repository-settings', params: { repository, section } };
}

function parseUserDialog(
  segments: string[],
  actionName: string,
  /** `null` for a table with no history to open, which is what the Root one is. */
  historyName: string | null,
  addName: string | null,
  actions: readonly string[],
): RouteDialog | null {
  if (segments.length === 1) {
    return addName !== null && segments[0] === 'add' ? { name: addName, params: {} } : null;
  }
  if (segments.length !== 2) return null;

  const [user, verb] = segments;
  if (user === undefined || user === '' || verb === undefined) return null;
  if (!actions.some((known) => known === verb)) return null;
  if (verb === 'history')
    return historyName === null ? null : { name: historyName, params: { user } };

  return { name: actionName, params: { user, action: fromSegment(verb) } };
}

function parseInvitationDialog(
  segments: string[],
  actionName: string,
  createName?: string,
): RouteDialog | null {
  if (segments.length === 1) {
    return createName !== undefined && segments[0] === 'new'
      ? { name: createName, params: {} }
      : null;
  }
  if (segments.length !== 2) return null;

  const [invitation, verb] = segments;
  if (invitation === undefined || invitation === '') return null;
  if (verb !== 'revoke' && verb !== 'reissue') return null;

  return { name: actionName, params: { invitation, action: verb } };
}

/** Decodes path segments, or `null` when one of them is not a valid escape. */
export function decodeSegments(segments: string[]): string[] | null {
  const decoded: string[] = [];
  for (const segment of segments) {
    try {
      decoded.push(decodeURIComponent(segment));
    } catch {
      return null;
    }
  }

  return decoded;
}

/*
 * An action is `remove_access` in the panel's own vocabulary and `remove-access`
 * in an address, because that is how every other segment the panel writes reads.
 * The two spellings never meet: one is what a handler switches on, the other is
 * what a person sees.
 */
function toSegment(action: string): string {
  return action.replaceAll('_', '-');
}

function fromSegment(segment: string): string {
  return segment.replaceAll('-', '_');
}
