/**
 * Why a reader is looking at the sign-in page rather than the panel.
 *
 * Arriving with no session at all is the ordinary case and says nothing. The rest
 * are sessions that ended while someone was using them, and they are not the same
 * thing: signing out is something you did and is over, while having an account
 * removed is something done to you that signing in again will not undo. The panel
 * showed both under "Access revoked", which reads as an accusation when all you
 * did was press Sign out.
 */

/** What ended the session: the code it was ended with, and the words that came with it. */
export interface SessionEnded {
  code: string;
  reason: string;
}

export interface SessionNotice {
  /** Names the state; stands above the card as the page's heading. */
  title: string;
  /** One line: what happened. */
  lead: string;
  /** What it means, and what to do about it. */
  note: string;
  /** Whether signing in again is worth offering. */
  offersSignIn: boolean;
}

/*
 * Keyed on the code rather than on the reason, because the reason is prose the
 * server may reword and two of these arrive with a reason that repeats the title.
 * `signed_out` comes from pressing Sign out, the two account codes from a Root
 * removing or banning the account, and `access_revoked` is what the panel works
 * out for itself when the stream says access changed and the viewer is gone.
 *
 * "No access" twice is deliberate: it is the same answer the 403 page gives, and a
 * reader who cannot get in is in one situation however they got there.
 */
const BY_CODE: Readonly<Record<string, SessionNotice>> = {
  signed_out: {
    title: 'Signed out',
    lead: 'Your session on this browser has ended',
    note: 'Nothing else changed. Signing in again takes you straight back to where you were',
    offersSignIn: true,
  },
  account_removed: {
    title: 'No access',
    lead: 'Your Smyklot account was removed',
    note: 'Signing in again will not bring it back. If you think this is a mistake, ask whoever runs Smyklot',
    offersSignIn: false,
  },
  account_banned: {
    title: 'No access',
    lead: 'Your Smyklot account was banned',
    note: 'Signing in again will not lift it. If you think this is a mistake, ask whoever runs Smyklot',
    offersSignIn: false,
  },
  /* What the server says when a request arrives without a session behind it. The
     panel used to leave a reader inside a workspace it could no longer load, so
     this is the code that now takes them out to the front door. */
  unauthenticated: {
    title: 'Session ended',
    lead: 'You are no longer signed in',
    note: 'Sessions do not last forever, and signing in again picks up where you left off',
    offersSignIn: true,
  },
  session_revoked: {
    title: 'Session ended',
    lead: 'This session was ended',
    note: 'It was signed out from somewhere else, or it ran out. Signing in again starts a new one',
    offersSignIn: true,
  },
  access_revoked: {
    title: 'Session ended',
    lead: 'Your access to the panel has changed',
    note: 'Sign in again to see where you stand',
    offersSignIn: true,
  },
};

/**
 * What to show for a session that ended.
 *
 * An unknown code keeps the server's own words, which are written for a person,
 * and offers the way back - the alternative is telling someone their access is
 * gone on no evidence.
 */
export function describeSessionEnd(ended: SessionEnded): SessionNotice {
  const known = BY_CODE[ended.code];
  if (known !== undefined) return known;

  const reason = ended.reason.trim();

  return {
    title: 'Session ended',
    lead: reason === '' ? 'Your session has ended' : reason,
    note: 'Sign in again to carry on',
    offersSignIn: true,
  };
}
