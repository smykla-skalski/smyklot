package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/internal/panel"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func TestCommentRecoveryObservesWithoutExecuting(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body                               string
		stale, superseded, denied, invalidConfig bool
		reason                                   panel.DeliveryRecoveryReason
	}{
		{name: "command", body: "/squash", reason: panel.RecoveryAvailable},
		{name: "stale content", body: "/squash", stale: true, reason: panel.RecoverySourceChanged},
		{name: "superseded receipt", body: "/squash", superseded: true, reason: panel.RecoverySourceChanged},
		{name: "permission revoked", body: "/squash", denied: true, reason: panel.RecoveryPermissionChanged},
		{name: "help needs no command permission", body: "/help", denied: true, reason: panel.RecoveryAvailable},
		{name: "reaction actors checked at execution", body: "A regular comment", denied: true, reason: panel.RecoveryAvailable},
		{name: "configuration invalid", body: "/squash", invalidConfig: true, reason: panel.RecoveryConfigurationInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			event, err := webhook.ParseIssueComment(githubtest.Command(test.body))
			if err != nil {
				t.Fatal(err)
			}
			api := commentRecoveryAPI(t, event, test.stale, test.denied)
			defer api.Close()
			client, err := github.NewClient("token", api.URL)
			if err != nil {
				t.Fatal(err)
			}
			repoID := storage.RepositoryID(event.Repository.ID)
			input := storage.DeliveryRecoveryInput{TargetID: storage.InstallationID(event.Installation.ID), RepositoryID: &repoID, SourceOrder: 1, ClaimKey: "original"}
			srv := &server{cfg: &serveConfig{}, store: recoveryCheckStore{sourceAccepted: !test.superseded}, runtimeBotConfig: config.Default()}
			srv.configs = newRepoCache(time.Minute, func(context.Context, *github.Client, string, string, *repositoryConfigFile) (repositoryConfigFile, error) {
				if test.invalidConfig {
					return repositoryConfigFile{err: bot.ErrRepoConfigInvalid}, nil
				}
				return repositoryConfigFile{}, nil
			})
			check, err := srv.checkCommentRecoveryWithClient(t.Context(), input, event, client)
			if err != nil || check.Reason != test.reason {
				t.Fatalf("check = %+v, %v", check, err)
			}
		})
	}
}

func commentRecoveryAPI(t *testing.T, event *webhook.IssueCommentEvent, stale, denied bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("preview mutated GitHub: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(400)
			return
		}
		switch r.URL.Path {
		case "/repos/smykla-skalski/smyklot/issues/comments/555":
			body := event.Comment.Body
			if stale {
				body = "edited"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": event.Comment.ID, "body": body, "updated_at": event.Comment.UpdatedAt, "user": map[string]string{"login": event.Comment.User.Login, "type": event.Comment.User.Type}})
		case "/repos/smykla-skalski/smyklot/contents/.github/CODEOWNERS":
			w.WriteHeader(http.StatusNotFound)
		case "/repos/smykla-skalski/smyklot/collaborators/someone/permission":
			permission := "write"
			if denied {
				permission = "read"
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"permission": permission})
		default:
			t.Errorf("unexpected GitHub read %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}
