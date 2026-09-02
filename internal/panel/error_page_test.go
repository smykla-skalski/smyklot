package panel

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
)

// A reader who types an address or follows a stale link gets the panel's own
// page. Everything else - which is to say every fetch the running panel makes -
// keeps the JSON body it already parses.
//
// Signed in, because a not-found page is now something only a reader with a
// session is shown: signed out, every page address answers with the sign-in
// shell so that the route table cannot be read off the difference. See
// TestPanelHidesItsRouteTableFromStrangers.
func TestPanelErrorDocumentServedToBrowsersOnly(t *testing.T) {
	harness := newPanelHarness(t, "owner")
	session := harness.signIn(t)

	for _, testCase := range []struct {
		name    string
		headers map[string]string
		page    bool
	}{
		{
			name:    "a browser navigating",
			headers: map[string]string{"Sec-Fetch-Dest": "document", "Accept": "text/html"},
			page:    true,
		},
		{
			// Nothing but a browser can say "document", so it settles the question.
			name:    "the panel's own fetch",
			headers: map[string]string{"Sec-Fetch-Dest": "empty", "Accept": "*/*"},
		},
		{
			name:    "an event stream",
			headers: map[string]string{"Sec-Fetch-Dest": "empty", "Accept": "text/event-stream"},
		},
		{
			// A reload of any panel page, once the service worker has installed.
			// The worker answers navigations by passing the request back through
			// fetch(), which does not carry the destination over - so a real
			// navigation arrives saying "empty" and still asking for a document.
			// Reading the absence of "document" as a denial served this reader the
			// JSON error body as text, which is what smyklot.com was doing.
			name: "a navigation a service worker forwarded",
			headers: map[string]string{
				"Sec-Fetch-Dest": "empty",
				"Sec-Fetch-Mode": "same-origin",
				"Accept":         "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			},
			page: true,
		},
		{
			// Older clients that send no Sec-Fetch headers fall back to Accept.
			name:    "a browser with no fetch metadata",
			headers: map[string]string{"Accept": "text/html,application/xhtml+xml,*/*;q=0.8"},
			page:    true,
		},
		{
			// `*/*` must not count as asking for a page, or every API caller that
			// omits Sec-Fetch-Dest is handed HTML it cannot parse.
			name:    "a client that accepts anything",
			headers: map[string]string{"Accept": "*/*"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/panel/nothing/here", nil)
			request.AddCookie(session)
			for name, value := range testCase.headers {
				request.Header.Set(name, value)
			}
			response := httptest.NewRecorder()
			harness.handler.ServeHTTP(response, request)

			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", response.Code)
			}
			if testCase.page {
				requirePage(t, response)

				return
			}
			requireJSON(t, response)
		})
	}
}

func requirePage(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(
		contentType, "text/html",
	) {
		t.Fatalf("content type = %q, want html", contentType)
	}
	if !strings.Contains(response.Body.String(), `name="smyklot-panel-error"`) {
		t.Fatalf("page carries no error descriptor: %s", response.Body.String())
	}
}

func requireJSON(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(
		contentType, "application/json",
	) {
		t.Fatalf("content type = %q, want json", contentType)
	}
	if !strings.Contains(response.Body.String(), `"code":"not_found"`) {
		t.Fatalf("json body = %s", response.Body.String())
	}
}

// The descriptor is the whole interface between the two halves: the page boots
// from it and renders from it, so the status has to survive escaping intact.
func TestPanelErrorDocumentCarriesTheFailure(t *testing.T) {
	harness := newPanelHarness(t, "owner")

	request := httptest.NewRequest(http.MethodGet, "/panel/nothing/here", nil)
	request.AddCookie(harness.signIn(t))
	request.Header.Set("Sec-Fetch-Dest", "document")
	response := httptest.NewRecorder()
	harness.handler.ServeHTTP(response, request)

	body := response.Body.String()
	for _, fragment := range []string{
		`&#34;status&#34;:404`,
		`&#34;code&#34;:&#34;not_found&#34;`,
		// Without scripting the reader has been told nothing else, so the
		// document says which error it was in words as well.
		`<noscript>404 - panel route not found</noscript>`,
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("document is missing %q: %s", fragment, body)
		}
	}
	// The status belongs to this request. A cached 404 would outlive whatever
	// made it one, and a cached sign-in failure is worse.
	if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cache)
	}
}

// Signing in takes a reader back to the address they asked for.
//
// A pasted link to a deep page is the ordinary way into one, and the sign-in
// round trip used to drop it: everybody landed on the front page of everything,
// including the reader who had just asked for one workspace's plan. The card
// even said "you come back here afterwards", which was not true.
//
// The refusals are the part that matters. This value is a redirect target that
// arrives from the browser, so anything able to leave the origin is an open
// redirect - a link that genuinely starts `https://smyklot.example/` landing on
// somebody else's sign-in form. Refusing is always safe, because the landing
// page is where the reader went before any of this existed.
func TestPanelSignInReturnsToTheAddressAsked(t *testing.T) {
	harness := newPanelHarness(t, "owner")

	returnedTo := func(t *testing.T, query string) string {
		t.Helper()
		start := httptest.NewRecorder()
		harness.handler.ServeHTTP(
			start, httptest.NewRequest(http.MethodGet, "/panel/auth/github/start"+query, nil),
		)
		if start.Code != http.StatusFound {
			t.Fatalf("start = %d", start.Code)
		}
		authorize, err := url.Parse(start.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		callback := httptest.NewRequest(
			http.MethodGet,
			"/panel/auth/github/callback?code=code&state="+
				url.QueryEscape(authorize.Query().Get("state")),
			nil,
		)
		callback.AddCookie(responseCookie(t, start, stateCookieName))
		for _, cookie := range start.Result().Cookies() {
			if cookie.Name == returnCookieName && cookie.MaxAge > 0 {
				callback.AddCookie(cookie)
			}
		}
		finished := httptest.NewRecorder()
		harness.handler.ServeHTTP(finished, callback)
		if finished.Code != http.StatusFound {
			t.Fatalf("callback = %d %s", finished.Code, finished.Body.String())
		}

		return finished.Header().Get("Location")
	}

	for _, honoured := range []struct {
		name string
		path string
	}{
		{name: "the address that was asked for", path: "/panel/workspace/smykla-skalski/sync/plan"},
		{name: "a query the address carried", path: "/panel/root/queue?section=waiting"},
	} {
		t.Run(honoured.name, func(t *testing.T) {
			got := returnedTo(t, "?return_to="+url.QueryEscape(honoured.path))
			if got != honoured.path {
				t.Fatalf("landed on %q, want %q", got, honoured.path)
			}
		})
	}

	// Every one of these has to end on the landing page, whatever it asked for.
	for _, escape := range []struct {
		name string
		path string
	}{
		{name: "an absolute URL", path: "https://evil.example/phish"},
		{name: "protocol-relative", path: "//evil.example/phish"},
		{name: "a backslash browsers normalise", path: "/\\evil.example/phish"},
		{name: "a scheme with no host", path: "javascript:alert(1)"},
		{name: "outside the base path", path: "/elsewhere/entirely"},
		{name: "not a path at all", path: "evil.example"},
		{name: "longer than any address", path: "/panel/" + strings.Repeat("a", maxReturnPath)},
		/* Dot segments, which are the case that gets PAST a prefix check: these
		   start with `/panel/` and `http.Redirect` cleans them to somewhere else
		   before the header is written. The plain "outside the base path" entry
		   above never reaches that code, because it is refused a step earlier. */
		{name: "climbing out with dot segments", path: "/panel/../../evil.example"},
		{name: "climbing out to a sibling mount", path: "/panel/./../root"},
		{name: "climbing out mid-path", path: "/panel/workspace/../../elsewhere"},
	} {
		t.Run(escape.name, func(t *testing.T) {
			if got := returnedTo(t, "?return_to="+url.QueryEscape(escape.path)); got != "/panel/" {
				t.Fatalf("landed on %q, want the landing page", got)
			}
		})
	}
}

// A return path is bound to the browser that asked for it, the way an invitation
// intent is. Tampering buys nothing on its own, because the path is checked again
// on the way out - but a value this server did not write has no business steering
// where a sign-in lands.
func TestPanelSignInIgnoresAnUnsignedReturn(t *testing.T) {
	harness := newPanelHarness(t, "owner")

	start := httptest.NewRecorder()
	harness.handler.ServeHTTP(
		start, httptest.NewRequest(http.MethodGet, "/panel/auth/github/start", nil),
	)
	authorize, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	callback := httptest.NewRequest(
		http.MethodGet,
		"/panel/auth/github/callback?code=code&state="+
			url.QueryEscape(authorize.Query().Get("state")),
		nil,
	)
	callback.AddCookie(responseCookie(t, start, stateCookieName))
	callback.AddCookie(&http.Cookie{
		Name: returnCookieName, Value: "/panel/root/queue.not-a-signature",
	})
	finished := httptest.NewRecorder()
	harness.handler.ServeHTTP(finished, callback)

	if got := finished.Header().Get("Location"); got != "/panel/" {
		t.Fatalf("landed on %q, want the landing page", got)
	}
}

// A sign-in that did not work ends where signing in begins.
//
// These failures used to render a page of their own, which put the reason and
// the button that answers it on different screens. They come back to the front
// door instead, carrying the status and code the card looks the words up by - so
// the reader reads what happened with the retry underneath it.
//
// An invitation link that is malformed is NOT one of these. Nothing was
// attempted and there is nothing to retry: the address itself is wrong, and that
// is a page.
func TestPanelSignInFailuresReturnToTheCard(t *testing.T) {
	harness := newPanelHarness(t, "owner")

	for _, testCase := range []struct {
		name string
		path string
		want string
	}{
		{
			name: "cancelled at GitHub",
			path: "/panel/auth/github/callback?error=access_denied",
			want: "401:sign_in_failed",
		},
		{
			name: "the callback carried nothing",
			path: "/panel/auth/github/callback",
			want: "400:sign_in_failed",
		},
		{
			name: "finished in another browser",
			path: "/panel/auth/github/callback?state=someone-else&code=abc",
			want: "401:sign_in_failed",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, testCase.path, nil)
			request.Header.Set("Sec-Fetch-Dest", "document")
			response := httptest.NewRecorder()
			harness.handler.ServeHTTP(response, request)

			if response.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302", response.Code)
			}
			location, err := url.Parse(response.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			if location.Path != "/panel/" {
				t.Fatalf("landed on %q, want the sign-in page", location.Path)
			}
			if got := location.Query().Get(signInFailedParam); got != testCase.want {
				t.Fatalf("%s = %q, want %q", signInFailedParam, got, testCase.want)
			}
		})
	}

	t.Run("an invitation link that lost half its address", func(t *testing.T) {
		request := httptest.NewRequest(
			http.MethodGet, "/panel/auth/github/start?invite=short&action=accept", nil,
		)
		request.Header.Set("Sec-Fetch-Dest", "document")
		response := httptest.NewRecorder()
		harness.handler.ServeHTTP(response, request)

		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", response.Code)
		}
		if !strings.Contains(response.Body.String(), `&#34;code&#34;:&#34;invalid_invitation`) {
			t.Fatalf("document does not name the invitation: %s", response.Body.String())
		}
	})
}

// The page the panel serves when nothing is wrong must not look like an error to
// the script that boots it, and must still say something without scripting.
func TestPanelIndexCarriesNoFailure(t *testing.T) {
	harness := newPanelHarness(t, "owner")

	request := httptest.NewRequest(http.MethodGet, "/panel/", nil)
	request.Header.Set("Sec-Fetch-Dest", "document")
	response := httptest.NewRecorder()
	harness.handler.ServeHTTP(response, request)

	requireResponse(t, response, "panel index", http.StatusOK,
		`name="smyklot-panel-error" content=""`,
		`<noscript>The Smyklot panel needs JavaScript to run.</noscript>`,
	)
	for _, sentinel := range []string{errorSentinel, noscriptSentinel} {
		if strings.Contains(response.Body.String(), sentinel) {
			t.Fatalf("served index still carries %s", sentinel)
		}
	}
}

// An error document is built by substitution, so a placeholder that goes missing
// would quietly serve a page reporting nothing at all. It has to fail loudly, at
// startup, rather than the first time something goes wrong in production.
func TestPanelAssetBundleRequiresErrorPlaceholders(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		index string
	}{
		{name: "no error placeholder", index: `<!doctype html><noscript>__smyklot_panel_noscript__</noscript>`},
		{name: "no noscript placeholder", index: `<!doctype html><meta content="__smyklot_panel_error__">`},
		{
			name:  "two error placeholders",
			index: `<!doctype html><meta content="__smyklot_panel_error__"><meta content="__smyklot_panel_error__"><noscript>__smyklot_panel_noscript__</noscript>`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := newAssetBundle(Config{
				BasePath: "/panel",
				Assets: fstest.MapFS{
					"index.html": &fstest.MapFile{Data: []byte(testCase.index)},
					"sw.js":      &fstest.MapFile{Data: []byte(`const V = '__smyklot_panel_version__';`)},
				},
			})
			if err == nil {
				t.Fatal("bundle built without both placeholders")
			}
		})
	}
}
