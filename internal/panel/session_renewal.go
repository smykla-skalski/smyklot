package panel

import (
	"net/http"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

/*
Keeping a session alive while it is being used.

A session was stamped with an expiry when it was issued and never touched again,
so it ended at a fixed hour whatever its owner was doing. Somebody who had the
panel open all day was signed out mid-sentence, and because the panel only learns
this from the next refused request, what they saw was a workspace that had
stopped answering.

So the clock now measures idleness rather than age: a request made while the
session is in the last stretch of its life pushes the end further out. Nothing is
rotated - the same token gets a later expiry, which is what makes this safe to
run on every request of a page that makes several at once. Rotating instead would
mean two requests in flight racing to replace the same cookie, and the loser
holding one that no longer works.

An age cap stands behind it, because a purely sliding session never ends while a
tab is open, which would leave the configured lifetime meaning nothing. Past the
cap the session runs out on its original terms and asks for a fresh sign-in.
*/

// renewalFraction is how much of the remaining life triggers a renewal. At a
// quarter, a session is renewed at most a few times over its life rather than on
// every request, and any tab still being used is renewed long before it ends.
const renewalFraction = 4

// maxSessionAge caps how long renewals can carry one sign-in. Long enough that a
// working week does not ask for a new one, short enough that a forgotten browser
// does not hold a session forever.
const maxSessionAge = 30 * 24 * time.Hour

/*
renewSession pushes a session's expiry out when it is near its end.

Called for every authenticated request, and does nothing for almost all of them:
the store is only written when the session has less than renewalFraction of its
configured life left.

A failure here is deliberately silent. The request that carried it is authentic
and has already been authorized; refusing it because the expiry could not be
written would turn a storage hiccup into a sign-out, which is the thing this
exists to prevent.
*/
func (s *Server) renewSession(w http.ResponseWriter, r *http.Request, session storage.Session) {
	ttl := s.sessionTTL()
	if ttl <= 0 {
		return
	}
	now := s.now().UTC()
	if session.ExpiresAt.Sub(now) > ttl/renewalFraction {
		return
	}

	extended := sessionRenewalDeadline(session.CreatedAt, now, ttl)
	if !extended.After(session.ExpiresAt) {
		// Already at the age cap: this sign-in ends when it was always going to.
		return
	}
	if err := s.store.ExtendSession(r.Context(), session.TokenHash, extended, now); err != nil {
		return
	}

	/* The cookie carries its own lifetime, so the browser has to be told too.
	   Without this the record outlives the cookie and the session ends at the
	   original hour anyway, with nothing in the request to show why. */
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return
	}
	// Measured against the clock the server was given, not the wall clock: they
	// are the same in production and only the injected one is the truth here.
	s.setCookie(w, sessionCookieName, cookie.Value, extended.Sub(now))
}

/*
sessionRenewalDeadline is when a session issued or renewed at `now` would end,
given the age cap.

The cap is applied to the expiry rather than enforced by refusing to renew, so a
session approaching it winds down instead of stopping short.
*/
func sessionRenewalDeadline(createdAt, now time.Time, ttl time.Duration) time.Time {
	limit := createdAt.Add(maxSessionAge)
	extended := now.Add(ttl)
	if extended.After(limit) {
		return limit
	}

	return extended
}
