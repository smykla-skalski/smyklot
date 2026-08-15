/**
 * What the panel says when the server answered with an error instead of a page.
 *
 * The server writes the status, its code and its own short message into the
 * document it serves; this turns that into something written for the person
 * reading it. The server's message stays out of the page - it is written for a
 * developer reading a JSON body, and "GitHub sign-in belongs to another browser"
 * tells a reader neither what happened nor what to do about it.
 */

const ERROR_META_NAME = 'smyklot-panel-error';

/** What the server said: the wire status, its code, and its own message. */
export interface PanelFailure {
  status: number;
  code: string;
  message: string;
}

/**
 * Where the one button goes. The page names a kind rather than a URL so the
 * catalogue stays a piece of writing, and so no entry can offer a link that this
 * page has no way to build.
 */
export type ErrorActionKind = 'panel' | 'sign-in';

export interface ErrorAction {
  kind: ErrorActionKind;
  label: string;
}

export interface ErrorContent {
  /** The number, kept because it is the one thing every reader recognises. */
  status: number;
  /** Names the state. Stands above the card, as the page's heading. */
  title: string;
  /** One sentence: what happened, in the reader's terms rather than the API's. */
  lead: string;
  /** Why it happened, and what will change it. */
  note: string;
  /** At most one, and only where pressing it can actually help. */
  action: ErrorAction | null;
}

/**
 * Reads what the server left in the document. Absent on every page the panel
 * means to serve, so this returning null is the normal case rather than a fault.
 */
export function readPanelFailure(source: Document): PanelFailure | null {
  const content = source
    .querySelector(`meta[name="${ERROR_META_NAME}"]`)
    ?.getAttribute('content')
    ?.trim();
  if (content === undefined || content === '') return null;

  try {
    return asFailure(JSON.parse(content));
  } catch {
    return null;
  }
}

function asFailure(value: unknown): PanelFailure | null {
  if (typeof value !== 'object' || value === null) return null;
  const { status, code, message } = value as Record<string, unknown>;
  if (typeof status !== 'number' || !Number.isFinite(status)) return null;

  return {
    status,
    code: typeof code === 'string' ? code : '',
    message: typeof message === 'string' ? message : '',
  };
}

const SIGN_IN_AGAIN: ErrorAction = { kind: 'sign-in', label: 'Sign in with GitHub' };
const OPEN_PANEL: ErrorAction = { kind: 'panel', label: 'Go to the panel' };

/*
 * Keyed by status and code together, because one code covers several different
 * situations: a sign-in can fail because the reader changed their mind at
 * GitHub, because the browser it started in is not this one, or because GitHub
 * itself did not answer, and those three want different words and different
 * buttons.
 *
 * There is deliberately no entry here for a 404 of any kind. A reader who
 * followed a link to nothing does not need to be told which of the panel's
 * features the address would have belonged to - naming it tells them about a
 * thing they cannot reach, which is worse than saying nothing. Every 404 falls
 * through to the plain one below.
 */
const BY_STATUS_AND_CODE: Readonly<Record<string, ErrorContent>> = {
  '401:sign_in_failed': {
    status: 401,
    title: 'Sign-in stopped',
    lead: 'Sign-in was not completed',
    note: 'GitHub did not confirm who you are, so nothing was signed in and nothing was changed. You can start again from here',
    action: SIGN_IN_AGAIN,
  },
  '400:sign_in_failed': {
    status: 400,
    title: 'Sign-in incomplete',
    lead: "GitHub's reply was missing something",
    note: 'The address GitHub came back to did not carry everything needed to finish signing in. Starting again usually settles it',
    action: SIGN_IN_AGAIN,
  },
  '502:sign_in_failed': {
    status: 502,
    title: 'GitHub unreachable',
    lead: 'GitHub did not confirm the sign-in',
    note: 'Smyklot asked GitHub who you are and got no usable answer. This one is between the two of them, so it is worth trying again in a moment',
    action: SIGN_IN_AGAIN,
  },
  '502:catalog_unavailable': {
    status: 502,
    title: 'GitHub unreachable',
    lead: 'Your installations could not be loaded',
    note: 'GitHub did not answer when Smyklot asked which installations you have, so the sign-in stopped short. Nothing is wrong with your account. Try again in a moment',
    action: SIGN_IN_AGAIN,
  },
  '403:forbidden': {
    status: 403,
    title: 'No access',
    lead: 'This GitHub account cannot open the panel',
    note: 'The panel is open to people who own an installation of Smyklot, and to anyone invited by name. If you were expecting to get in, ask whoever runs Smyklot to invite this account',
    action: { kind: 'sign-in', label: 'Try a different account' },
  },
  '400:invalid_invitation': {
    status: 400,
    title: 'Link not valid',
    lead: 'This link is not complete',
    note: 'Part of the address is missing, which usually means it was broken across two lines somewhere on the way here. Ask whoever sent it for a fresh one',
    action: null,
  },
  '401:invalid_invitation': {
    status: 401,
    title: 'Wrong browser',
    lead: 'This was started in another browser',
    note: 'Answering an invitation has to finish in the browser that began it. Open the link again here and it will go through',
    action: null,
  },
  '403:wrong_identity': {
    status: 403,
    title: 'Wrong account',
    lead: 'This invitation names a different GitHub account',
    note: 'It was issued to one account by name, and that is not the one you signed in with. Sign in as the account it was sent to, or ask for one addressed to this account',
    action: null,
  },
  '410:invitation_expired': {
    status: 410,
    title: 'Expired',
    lead: 'This invitation has expired',
    note: 'Invitations are good for a set number of days and this one is past it. Ask whoever sent it to issue a new one',
    action: null,
  },
  '409:invitation_used': {
    status: 409,
    title: 'Already answered',
    lead: 'This invitation has already been answered',
    note: 'It was accepted, declined or withdrawn, so there is nothing left to respond to. If you already accepted it, sign in and it will be waiting',
    action: SIGN_IN_AGAIN,
  },
};

/*
 * The fallback, by status alone. Everything here is written to be true whatever
 * produced it, because that is exactly what these answer: a status nobody wrote
 * a specific page for.
 */
const BY_STATUS: Readonly<Record<number, ErrorContent>> = {
  400: {
    status: 400,
    title: 'Not valid',
    lead: 'That request could not be read',
    note: 'Part of the address looks wrong or incomplete. Check it, or start again from the panel',
    action: OPEN_PANEL,
  },
  401: {
    status: 401,
    title: 'Not signed in',
    lead: 'You are not signed in',
    note: 'This page is only there for people Smyklot recognises. Sign in with GitHub and try the address again',
    action: SIGN_IN_AGAIN,
  },
  403: {
    status: 403,
    title: 'No access',
    lead: 'You cannot open this',
    note: 'Your account is signed in but it is not allowed here. If you were expecting to get in, ask whoever runs Smyklot for access',
    action: OPEN_PANEL,
  },
  404: {
    status: 404,
    title: 'Not found',
    lead: 'This address does not lead anywhere',
    note: 'The link may be out of date, or part of it may have been lost on the way here. Nothing is broken, there is just nothing at this address',
    action: OPEN_PANEL,
  },
  409: {
    status: 409,
    title: 'Already changed',
    lead: 'This was already answered somewhere else',
    note: 'Something changed between the page being opened and this being sent, so it was left alone. Open it again to see where it stands',
    action: OPEN_PANEL,
  },
  410: {
    status: 410,
    title: 'Gone',
    lead: 'This is no longer here',
    note: 'It existed once and has since been withdrawn or run out of time. There is nothing to reload',
    action: OPEN_PANEL,
  },
  429: {
    status: 429,
    title: 'Too fast',
    lead: 'That came through too quickly',
    note: 'Smyklot is turning requests away for a moment to keep up. Waiting a little and trying again is all this needs',
    action: OPEN_PANEL,
  },
  500: {
    status: 500,
    title: 'Something broke',
    lead: 'Smyklot could not finish that',
    note: 'The fault is on this side and it has been written to the log. Trying again often works, because most of what causes this does not last',
    action: OPEN_PANEL,
  },
  502: {
    status: 502,
    title: 'GitHub unreachable',
    lead: 'GitHub did not answer',
    note: 'Smyklot needs GitHub to finish this and did not get a usable reply. Nothing is wrong on your side. Try again in a moment',
    action: OPEN_PANEL,
  },
  503: {
    status: 503,
    title: 'Unavailable',
    lead: 'Smyklot is not taking requests',
    note: 'The service is restarting or is being held back on purpose. It usually comes back within a minute or two',
    action: OPEN_PANEL,
  },
  504: {
    status: 504,
    title: 'Timed out',
    lead: 'That took too long to answer',
    note: 'Something Smyklot was waiting on did not come back in time. Trying again is worthwhile, and often faster the second time',
    action: OPEN_PANEL,
  },
};

/**
 * What to show. A status and code that was written for wins; then the status on
 * its own; then which half of the range it falls in - because a reader still has
 * to be told whose problem it is, even for a status nobody anticipated.
 */
export function describeFailure(failure: PanelFailure): ErrorContent {
  const specific = BY_STATUS_AND_CODE[`${failure.status}:${failure.code}`];
  if (specific !== undefined) return specific;

  const byStatus = BY_STATUS[failure.status];
  if (byStatus !== undefined) return byStatus;

  return failure.status >= 500
    ? {
        status: failure.status,
        title: 'Something broke',
        lead: 'Smyklot could not finish that',
        note: 'The fault is on this side rather than yours, and it has been written to the log. Trying again is worth a go',
        action: OPEN_PANEL,
      }
    : {
        status: failure.status,
        title: 'Not available',
        lead: 'That could not be opened',
        note: 'The address was understood but there is nothing here to show for it. Start again from the panel',
        action: OPEN_PANEL,
      };
}
