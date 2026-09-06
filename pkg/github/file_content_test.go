package github_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

func TestFileContentDistinguishesMissingEmptyAndTruncated(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		status  int
		want    string
		missing bool
		invalid bool
	}{
		{name: "missing", status: http.StatusNotFound, body: `{}`, missing: true},
		{name: "empty", body: `{"type":"file","encoding":"base64","content":"","size":0}`},
		{name: "content", body: `{"encoding":"base64","content":"YQo=\n","size":2}`, want: "a\n"},
		{name: "size too large", body: `{"encoding":"base64","content":"","size":33}`, invalid: true},
		{name: "negative size", body: `{"encoding":"base64","content":"","size":-1}`, invalid: true},
		{name: "size mismatch", body: `{"encoding":"base64","content":"","size":1}`, invalid: true},
		{name: "directory", body: `[]`, invalid: true},
		{name: "symlink", body: `{"type":"symlink","encoding":"base64","content":""}`, invalid: true},
		{name: "submodule", body: `{"type":"submodule","encoding":"base64","content":""}`, invalid: true},
		{name: "unknown encoding", body: `{"encoding":"utf-8","content":"a"}`, invalid: true},
		{name: "missing encoding", body: `{"content":""}`, invalid: true},
		{name: "no blob id", body: `{"encoding":"none","content":"","size":12}`, invalid: true},
		{name: "bad base64", body: `{"encoding":"base64","content":"!!!"}`, invalid: true},
		{name: "malformed JSON", body: `{`, invalid: true},
		{name: "missing content", body: `{"encoding":"base64"}`, invalid: true},
		{name: "decoded size too large", body: fileJSON(strings.Repeat("a", 33)), invalid: true},
		{name: "bounded response", body: `{"encoding":"base64","content":"","padding":"` + strings.Repeat("x", 70000) + `"}`, invalid: true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if item.status != 0 {
					w.WriteHeader(item.status)
				}
				_, _ = w.Write([]byte(item.body))
			}))
			t.Cleanup(server.Close)
			client, err := github.NewClient("test-token", server.URL)
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.GetFileContent(context.Background(), "owner", "repo", "config.toml", "", 32)
			if item.invalid {
				if !errors.Is(err, github.ErrResponseParse) || got != nil {
					t.Fatalf("expected failed read, got %q, %v", got, err)
				}
				return
			}
			if err != nil || string(got) != item.want || (got == nil) != item.missing {
				t.Fatalf("got %q (nil=%v), %v", got, got == nil, err)
			}
		})
	}
}

type fileBlobCase struct {
	name   string
	blob   string
	status int
	valid  bool
}

const observedFileBlob = "0123456789abcdef0123456789abcdef01234567"

func TestFileContentReadsOnlyTheObservedLargeBlob(t *testing.T) {
	const blob = observedFileBlob
	cases := []fileBlobCase{
		{name: "complete", blob: `{"sha":"` + blob + `","encoding":"base64","content":"YQo=","size":2}`, valid: true},
		{name: "missing blob", status: http.StatusNotFound, blob: `{}`},
		{name: "wrong id", blob: `{"sha":"other","encoding":"base64","content":"YQo=","size":2}`},
		{name: "wrong size", blob: `{"sha":"` + blob + `","encoding":"base64","content":"YQ==","size":1}`},
		{name: "truncated", blob: `{"sha":"` + blob + `","encoding":"base64","content":"","size":2}`},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			checkObservedBlobRead(t, item)
		})
	}
}

func checkObservedBlobRead(t *testing.T, item fileBlobCase) {
	t.Helper()
	const blob = observedFileBlob
	const commit = "abcdef0123456789abcdef0123456789abcdef01"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case "/repos/owner/repo/contents/config.toml":
			if r.URL.Query().Get("ref") != commit {
				t.Error("contents read lost immutable ref")
			}
			_, _ = w.Write([]byte(`{"type":"file","sha":"` + blob + `","encoding":"none","content":"","size":2,"download_url":"https://untrusted.invalid/file"}`))
		case "/repos/owner/repo/git/blobs/" + blob:
			if item.status != 0 {
				w.WriteHeader(item.status)
			}
			_, _ = w.Write([]byte(item.blob))
		default:
			t.Errorf("unexpected file request %s", r.URL)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetFileContent(context.Background(), "owner", "repo", "config.toml", commit, 32)
	if requests.Load() != 2 || (err == nil) != item.valid {
		t.Fatalf("requests=%d, content=%q, error=%v", requests.Load(), got, err)
	}
	if item.valid && string(got) != "a\n" {
		t.Fatalf("wrong content: %q", got)
	}
}

func fileJSON(content string) string {
	encoded, _ := json.Marshal(map[string]string{
		"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(content)),
	})
	return string(encoded)
}
