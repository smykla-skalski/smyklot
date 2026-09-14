package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppInstallationURL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, slug, page, want string }{
		{"cloud", "custom-bot", "https://github.com/apps/custom-bot", "https://github.com/apps/custom-bot/installations/new"},
		{"enterprise", "custom-bot", "https://git.example/github-apps/custom-bot", "https://git.example/github-apps/custom-bot/installations/new"},
		{"enterprise http", "bot", "http://git.example/github-apps/bot", "http://git.example/github-apps/bot/installations/new"},
		{"missing", "bot", "", ""},
		{"missing slug", "", "https://github.com/apps/bot", ""},
		{"wrong app", "bot", "https://github.com/apps/other", ""},
		{"relative", "bot", "/apps/bot", ""},
		{"credentials", "bot", "https://user:password@git.example/apps/bot", ""},
		{"scheme", "bot", "javascript:alert(1)", ""},
		{"query", "bot", "https://github.com/apps/bot?next=other", ""},
		{"fragment", "bot", "https://github.com/apps/bot#other", ""},
		{"path", "../bot", "https://github.com/apps/../bot", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/app" || r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer app-jwt" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"slug": tc.slug, "html_url": tc.page})
			}))
			t.Cleanup(api.Close)
			client, err := NewAppClient("app-jwt", api.URL)
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.AppInstallationURL(t.Context())
			if (err != nil) != (tc.want == "") || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
