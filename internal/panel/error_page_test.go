package panel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// A reader who types an address or follows a stale link gets the panel's own
// page. Everything else - which is to say every fetch the running panel makes -
// keeps the JSON body it already parses.
func TestPanelErrorDocumentServedToBrowsersOnly(t *testing.T) {
	harness := newPanelHarness(t, "owner")

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
			// Script cannot set Sec-Fetch-Dest, so this is the exact signal.
			name:    "the panel's own fetch",
			headers: map[string]string{"Sec-Fetch-Dest": "empty", "Accept": "*/*"},
		},
		{
			name:    "an event stream",
			headers: map[string]string{"Sec-Fetch-Dest": "empty", "Accept": "text/event-stream"},
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

// Every page the sign-in round trip can land a reader on. These are the ones
// that are not reachable any other way: a person clicks Sign in, something goes
// wrong at GitHub, and this is the whole of what they see.
func TestPanelSignInFailuresRenderPages(t *testing.T) {
	harness := newPanelHarness(t, "owner")

	for _, testCase := range []struct {
		name   string
		path   string
		status int
		code   string
	}{
		{
			name:   "cancelled at GitHub",
			path:   "/panel/auth/github/callback?error=access_denied",
			status: http.StatusUnauthorized,
			code:   "sign_in_failed",
		},
		{
			name:   "the callback carried nothing",
			path:   "/panel/auth/github/callback",
			status: http.StatusBadRequest,
			code:   "sign_in_failed",
		},
		{
			name:   "finished in another browser",
			path:   "/panel/auth/github/callback?state=someone-else&code=abc",
			status: http.StatusUnauthorized,
			code:   "sign_in_failed",
		},
		{
			name:   "an invitation link that lost half its address",
			path:   "/panel/auth/github/start?invite=short&action=accept",
			status: http.StatusBadRequest,
			code:   "invalid_invitation",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, testCase.path, nil)
			request.Header.Set("Sec-Fetch-Dest", "document")
			response := httptest.NewRecorder()
			harness.handler.ServeHTTP(response, request)

			if response.Code != testCase.status {
				t.Fatalf("status = %d, want %d", response.Code, testCase.status)
			}
			if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(
				contentType, "text/html",
			) {
				t.Fatalf("content type = %q, want html", contentType)
			}
			if !strings.Contains(response.Body.String(), `&#34;code&#34;:&#34;`+testCase.code) {
				t.Fatalf("document does not name %s: %s", testCase.code, response.Body.String())
			}
		})
	}
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
