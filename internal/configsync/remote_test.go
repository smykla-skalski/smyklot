package configsync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

var remoteHead = strings.Repeat("a", 40)

type remoteCall struct {
	method string
	path   string
	status int
	answer string
	check  func(*testing.T, *http.Request)
}

func scriptedRemote(t *testing.T, calls ...remoteCall) *github.Client {
	t.Helper()
	var mu sync.Mutex
	next := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if next >= len(calls) {
			t.Errorf("unexpected remote call %s %s", request.Method, request.URL.RequestURI())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		call := calls[next]
		next++
		if request.Method != call.method || request.URL.Path != call.path {
			t.Errorf("remote call = %s %s, want %s %s", request.Method, request.URL.RequestURI(), call.method, call.path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if call.check != nil {
			call.check(t, request)
		}
		w.Header().Set("Content-Type", "application/json")
		if call.status != 0 {
			w.WriteHeader(call.status)
		}
		_, _ = fmt.Fprint(w, call.answer)
	}))
	t.Cleanup(func() {
		server.Close()
		mu.Lock()
		defer mu.Unlock()
		if next != len(calls) {
			t.Errorf("remote performed %d of %d expected calls", next, len(calls))
		}
	})
	client, err := github.NewClient("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func remoteReadCalls(entries string) []remoteCall {
	return []remoteCall{
		{method: "GET", path: "/repos/acme/web/git/ref/heads/main", answer: `{"object":{"sha":"` + remoteHead + `"}}`},
		{method: "GET", path: "/repos/acme/web/git/trees/" + remoteHead, answer: `{"tree":` + entries + `}`},
	}
}

func contentCall(path, content string) remoteCall {
	encoded, _ := json.Marshal(map[string]any{
		"type": "file", "encoding": "base64", "size": len(content),
		"content": base64.StdEncoding.EncodeToString([]byte(content)), "sha": orgsync.BlobID([]byte(content)),
	})
	return remoteCall{
		method: "GET", path: "/repos/acme/web/contents/" + path, answer: string(encoded),
		check: func(t *testing.T, request *http.Request) {
			if request.URL.Query().Get("ref") != remoteHead {
				t.Error("file contents were not bound to the observed commit")
			}
		},
	}
}

func remoteEntry(path, content string) map[string]any {
	return map[string]any{"path": path, "mode": "100644", "type": "blob", "sha": orgsync.BlobID([]byte(content)), "size": len(content)}
}

func TestRemoteReaderUsesOneCommitAndSearchOrder(t *testing.T) {
	content := "runner='action'\nquiet_success=false\n"
	for _, filePath := range []string{".smyklot.toml", ".smyklot/config.toml"} {
		t.Run(filePath, func(t *testing.T) {
			entries, _ := json.Marshal([]any{remoteEntry(".github/smyklot.yaml", "quiet_success: true\n"), remoteEntry(filePath, content)})
			calls := append(remoteReadCalls(string(entries)), contentCall(filePath, content))
			file, err := ReadRemoteFile(context.Background(), scriptedRemote(t, calls...), RemoteLocation{
				Owner: "acme", Repository: "web", DefaultBranch: "main", Scope: config.PanelFileRepository,
			})
			if err != nil || file.Head != remoteHead || file.Path != filePath || file.WritePath != filePath || file.Migrate ||
				len(file.Ignored) != 1 || file.Ignored[0] != ".github/smyklot.yaml" || file.Source.Runner == nil ||
				*file.Source.Runner != config.RunnerAction {
				t.Fatalf("read = %+v (%v)", file, err)
			}
		})
	}
}

func TestRemoteReaderKeepsMissingEmptyAndLegacyDistinct(t *testing.T) {
	for _, example := range []struct {
		path, content string
		migrate       bool
	}{
		{},
		{path: ".smyklot.toml"},
		{path: ".github/smyklot.yaml", content: "runner: action\n", migrate: true},
	} {
		t.Run(example.path, func(t *testing.T) {
			entries := []any{}
			if example.path != "" {
				entries = append(entries, remoteEntry(example.path, example.content))
			}
			encoded, _ := json.Marshal(entries)
			calls := remoteReadCalls(string(encoded))
			if example.path != "" {
				calls = append(calls, contentCall(example.path, example.content))
			}
			file, err := ReadRemoteFile(context.Background(), scriptedRemote(t, calls...), RemoteLocation{
				Owner: "acme", Repository: "web", DefaultBranch: "main", Scope: config.PanelFileRepository,
			})
			if err != nil || file.Path != example.path || file.Source.Snapshot.Exists != (example.path != "") ||
				file.WritePath != ".smyklot.toml" || file.Migrate != example.migrate {
				t.Fatalf("read = %+v (%v)", file, err)
			}
		})
	}
}

func TestRemoteReaderRefusesUnsafeOrInconsistentFiles(t *testing.T) {
	for name, entries := range map[string][]any{
		"symlink":        {map[string]any{"path": ".smyklot.toml", "mode": "120000"}},
		"directory":      {map[string]any{"path": ".smyklot.toml", "mode": "040000"}},
		"blocked parent": {remoteEntry(".smyklot", "text")},
		"oversize":       {map[string]any{"path": ".smyklot.toml", "mode": "100644", "size": config.MaxFileDocumentBytes + 1}},
		"blob mismatch":  {remoteEntry(".smyklot.toml", "quiet_success=true\n")},
	} {
		t.Run(name, func(t *testing.T) {
			encoded, _ := json.Marshal(entries)
			calls := remoteReadCalls(string(encoded))
			if name == "blob mismatch" {
				calls = append(calls, contentCall(".smyklot.toml", "quiet_success=false\n"))
			}
			_, err := ReadRemoteFile(context.Background(), scriptedRemote(t, calls...), RemoteLocation{
				Owner: "acme", Repository: "web", DefaultBranch: "main", Scope: config.PanelFileRepository,
			})
			if err == nil {
				t.Fatal("accepted an unsafe or inconsistent file")
			}
		})
	}
}

func TestWorkspaceFileIsSeparateFromRepositoryConfiguration(t *testing.T) {
	content := "[panel]\nversion=1\nscope='workspace'\n[panel.settings]\n" +
		"repository_default_enabled=true\nmerge_mode='checks'\nprotected_refs={include=['~DEFAULT_BRANCH']}\n"
	entries, _ := json.Marshal([]any{
		remoteEntry(".smyklot.toml", "quiet_success=true\n"), remoteEntry(WorkspaceFilePath, content),
	})
	calls := append(remoteReadCalls(string(entries)), contentCall(WorkspaceFilePath, content))
	location := remoteLocation()
	location.Scope = config.PanelFileWorkspace
	file, err := ReadRemoteFile(context.Background(), scriptedRemote(t, calls...), location)
	if err != nil || file.Path != WorkspaceFilePath || file.WritePath != WorkspaceFilePath || len(file.Ignored) != 0 {
		t.Fatalf("workspace file = %+v (%v)", file, err)
	}
}

func TestRemoteReaderWalksTruncatedTrees(t *testing.T) {
	content := "quiet_success=true\n"
	entry := remoteEntry(".smyklot.toml", content)
	entries, _ := json.Marshal([]any{entry})
	calls := remoteReadCalls(`[]`)
	calls[1].answer = `{"truncated":true,"tree":[]}`
	calls = append(calls,
		remoteCall{method: "GET", path: "/repos/acme/web/git/trees/" + remoteHead, answer: `{"tree":` + string(entries) + `}`},
		contentCall(".smyklot.toml", content))
	file, err := ReadRemoteFile(context.Background(), scriptedRemote(t, calls...), remoteLocation())
	if err != nil || file.Path != ".smyklot.toml" {
		t.Fatalf("truncated-tree file = %+v (%v)", file, err)
	}
}

func TestRemoteReaderDoesNotBlockSelectedFileOnIgnoredPath(t *testing.T) {
	content := "quiet_success=true\n"
	entries, _ := json.Marshal([]any{
		remoteEntry(".smyklot.toml", content), map[string]any{"path": ".smyklot/config.toml", "mode": "120000"},
	})
	calls := append(remoteReadCalls(string(entries)), contentCall(".smyklot.toml", content))
	file, err := ReadRemoteFile(context.Background(), scriptedRemote(t, calls...), remoteLocation())
	if err != nil || file.Path != ".smyklot.toml" || len(file.Ignored) != 1 {
		t.Fatalf("ignored path affected selected file: %+v (%v)", file, err)
	}
}

func TestRemoteReaderDoesNotConfuseUnreadableTreeWithMissingFile(t *testing.T) {
	calls := remoteReadCalls(`[]`)
	calls[1].status = http.StatusNotFound
	calls[1].answer = `{}`
	file, err := ReadRemoteFile(context.Background(), scriptedRemote(t, calls...), remoteLocation())
	if err == nil || file.Source.Snapshot.Exists {
		t.Fatalf("unreadable tree became an observation: %+v (%v)", file, err)
	}
}
