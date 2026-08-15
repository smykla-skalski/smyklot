package panel

import (
	"net/http"
	"testing"
	"time"
)

// The TTL the harness configures, which the renewal thresholds are read against.
const harnessSessionTTL = 12 * time.Hour

func sessionExpiry(t *testing.T, harness *panelHarness, token string) time.Time {
	t.Helper()
	session, err := harness.store.GetSession(t.Context(), tokenHash(token), *harness.clock)
	if err != nil {
		t.Fatalf("read session: %v", err)
	}

	return session.ExpiresAt
}

func TestSessionIsNotRenewedWhileItHasLifeLeft(t *testing.T) {
	t.Parallel()
	harness := newPanelHarness(t, "panel-owner")
	session := harness.signIn(t)
	issued := sessionExpiry(t, harness, session.Value)

	/* Renewing on every request would write to the store on each one, for a
	   session that is nowhere near ending. */
	harness.advance(time.Hour)
	response := harness.request(t, http.MethodGet, "/panel/api/v1/session", nil, session)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := sessionExpiry(t, harness, session.Value); !got.Equal(issued) {
		t.Fatalf("expiry moved to %s from %s with most of the session left", got, issued)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			t.Fatal("a session that was not renewed re-sent its cookie")
		}
	}
}

func TestSessionIsRenewedWhileItIsBeingUsed(t *testing.T) {
	t.Parallel()
	harness := newPanelHarness(t, "panel-owner")
	session := harness.signIn(t)
	issued := sessionExpiry(t, harness, session.Value)

	// Into the last quarter of its life, still working.
	harness.advance(harnessSessionTTL - time.Hour)
	response := harness.request(t, http.MethodGet, "/panel/api/v1/session", nil, session)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	renewed := sessionExpiry(t, harness, session.Value)
	want := harness.clock.Add(harnessSessionTTL)
	if !renewed.Equal(want) {
		t.Fatalf("expiry = %s, want %s (was %s)", renewed, want, issued)
	}

	/* The record and the cookie have to move together. A record that outlives
	   the cookie ends the session at the original hour anyway. */
	var reissued *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			reissued = cookie
		}
	}
	if reissued == nil {
		t.Fatal("a renewed session did not re-send its cookie")
	}
	if reissued.Value != session.Value {
		t.Fatal("renewal rotated the token; a page making several requests at once would lose the race")
	}
	if reissued.MaxAge <= 0 {
		t.Fatalf("re-sent cookie has Max-Age %d", reissued.MaxAge)
	}
}

func TestUsingASessionAllDayKeepsItAlive(t *testing.T) {
	t.Parallel()
	harness := newPanelHarness(t, "panel-owner")
	session := harness.signIn(t)

	/* The thing this exists for: somebody with the panel open, working, well
	   past the point where the session it started with would have ended. */
	for range 8 {
		harness.advance(harnessSessionTTL - time.Hour)
		response := harness.request(t, http.MethodGet, "/panel/api/v1/session", nil, session)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d after %s of use", response.Code, time.Since(harness.now))
		}
	}
}

func TestRenewalStopsAtTheAgeCap(t *testing.T) {
	t.Parallel()
	harness := newPanelHarness(t, "panel-owner")
	session := harness.signIn(t)
	cap := harness.now.Add(maxSessionAge)

	/* A purely sliding session never ends while a tab is open, which would make
	   the configured lifetime mean nothing. Kept in constant use, this one still
	   ends: at the cap, and never a moment past it. */
	step := harnessSessionTTL - time.Hour
	refusedAt := time.Time{}
	for range int(maxSessionAge/step) + 2 {
		harness.advance(step)
		response := harness.request(t, http.MethodGet, "/panel/api/v1/session", nil, session)
		if response.Code == http.StatusUnauthorized {
			refusedAt = *harness.clock
			break
		}
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		if got := sessionExpiry(t, harness, session.Value); got.After(cap) {
			t.Fatalf("expiry %s is past the age cap %s", got, cap)
		}
	}
	if refusedAt.IsZero() {
		t.Fatalf("a session in constant use was still accepted past %s of renewals", maxSessionAge)
	}
	if refusedAt.Before(cap) {
		t.Fatalf("session ended at %s, before the cap %s", refusedAt, cap)
	}
}

func TestRenewalDeadlineNeverPassesTheAgeCap(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)

	// Early in the session's life, a renewal is a full TTL from now.
	early := created.Add(time.Hour)
	if got := sessionRenewalDeadline(created, early, harnessSessionTTL); !got.Equal(
		early.Add(harnessSessionTTL),
	) {
		t.Fatalf("early deadline = %s, want %s", got, early.Add(harnessSessionTTL))
	}

	// Near the cap it winds down to it rather than stopping short of it.
	late := created.Add(maxSessionAge - time.Hour)
	if got := sessionRenewalDeadline(created, late, harnessSessionTTL); !got.Equal(
		created.Add(maxSessionAge),
	) {
		t.Fatalf("late deadline = %s, want the cap %s", got, created.Add(maxSessionAge))
	}
}
