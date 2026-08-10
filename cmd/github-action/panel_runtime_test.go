package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type transientAbandonStore struct {
	storage.Store
	attempts     atomic.Int32
	retryStarted chan struct{}
	retryRelease chan struct{}
}

type runtimePanelEvent struct {
	Version      int    `json:"version"`
	Type         string `json:"type"`
	TargetID     string `json:"target_id"`
	RepositoryID string `json:"repository_id"`
}

func (s *transientAbandonStore) AbandonDelivery(ctx context.Context, claimID int64) error {
	if s.attempts.Add(1) == 1 {
		return errors.New("database is busy")
	}
	select {
	case <-s.retryStarted:
	default:
		close(s.retryStarted)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.retryRelease:
	}

	return s.Store.AbandonDelivery(ctx, claimID)
}

var _ = Describe("Production panel runtime [Unit]", func() {
	var (
		stub     *githubStub
		endpoint *httptest.Server
		service  *server
	)

	BeforeEach(func() {
		stub = newGitHubStub()
		endpoint = httptest.NewServer(stub)
		DeferCleanup(endpoint.Close)

		var err error
		service, err = newServer(&serveConfig{
			webhookPath:   defaultWebhookPath,
			webhookSecret: []byte(testSecret),
			apiBaseURL:    endpoint.URL,
			botUsername:   defaultBotUsername,
			appClientID:   "Iv1.test",
			appPrivateKey: githubtest.AppPrivateKey(),
			botConfig:     config.Default(),
			logWriter:     io.Discard,
			panel: &panelServeConfig{
				publicOrigin: "https://smyklot.example",
				basePath:     defaultPanelBase,
				statePath:    GinkgoT().TempDir() + "/panel.sqlite3",
				superRootID:  42,
				clientID:     "Iv1.test",
				clientSecret: "oauth-secret",
				authorizeURL: endpoint.URL + "/authorize",
				tokenURL:     endpoint.URL + "/token",
				sessionTTL:   defaultPanelTTL,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(service.Close)
	})

	It("mounts the embedded application under the configured public path", func() {
		response := httptest.NewRecorder()
		service.handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panel/", nil))

		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(response.Header().Get("Content-Security-Policy")).NotTo(BeEmpty())
		Expect(response.Body.String()).To(ContainSubstring("Smyklot"))
		Expect(response.Body.String()).NotTo(ContainSubstring("/__smyklot_panel_base__"))
	})

	It("mounts the panel at the public root without shadowing service routes", func() {
		rootService, err := newServer(&serveConfig{
			webhookPath:   defaultWebhookPath,
			webhookSecret: []byte(testSecret),
			apiBaseURL:    endpoint.URL,
			botUsername:   defaultBotUsername,
			appClientID:   "Iv1.test",
			appPrivateKey: githubtest.AppPrivateKey(),
			botConfig:     config.Default(),
			logWriter:     io.Discard,
			panel: &panelServeConfig{
				publicOrigin: "https://smyklot.com",
				basePath:     "",
				statePath:    GinkgoT().TempDir() + "/panel.sqlite3",
				superRootID:  42,
				clientID:     "Iv1.test",
				clientSecret: "oauth-secret",
				authorizeURL: endpoint.URL + "/authorize",
				tokenURL:     endpoint.URL + "/token",
				sessionTTL:   defaultPanelTTL,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(rootService.Close)

		root := httptest.NewRecorder()
		rootService.handler().ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/", nil))
		Expect(root.Code).To(Equal(http.StatusOK))
		Expect(root.Body.String()).To(ContainSubstring("Smyklot"))

		health := httptest.NewRecorder()
		rootService.handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		Expect(health.Code).To(Equal(http.StatusOK))
	})

	It("synchronizes immutable GitHub identity and repository metadata", func() {
		stub.installations = `[{"id":111,"account":{"id":7,"login":"smykla-skalski","type":"Organization","avatar_url":"https://avatars.example/7"}}]`
		stub.repos = `{"total_count":1,"repositories":[{"id":31,"name":"smyklot","full_name":"smykla-skalski/smyklot","private":true,"owner":{"login":"smykla-skalski"}}]}`

		targetIDs, err := service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())
		Expect(targetIDs).To(Equal([]string{"github:installation:111"}))

		target, err := service.store.GetTarget(GinkgoT().Context(), targetIDs[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(target.Account.SubjectID).To(Equal("7"))
		Expect(target.Account.AvatarURL).To(HaveValue(Equal("https://avatars.example/7")))
		repositories, err := service.store.ListRepositories(GinkgoT().Context(), target.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(repositories).To(HaveLen(1))
		Expect(repositories[0].ID).To(Equal("github:repository:31"))
		Expect(repositories[0].Private).To(BeTrue())
	})

	It("serializes full catalog reads so an older snapshot cannot commit last", func() {
		stub.installations = `[{"id":111,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}}]`
		stub.repos = `{"repositories":[]}`
		stub.installationsStarted = make(chan struct{})
		stub.installationsRelease = make(chan struct{})

		firstDone := make(chan error, 1)
		go func() {
			_, err := service.SyncCatalog(GinkgoT().Context())
			firstDone <- err
		}()
		Eventually(stub.installationsStarted).Should(BeClosed())

		stub.setInstallations(`[
			{"id":111,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}},
			{"id":222,"account":{"id":8,"login":"new-org","type":"Organization"}}
		]`)
		secondDone := make(chan error, 1)
		go func() {
			_, err := service.SyncCatalog(GinkgoT().Context())
			secondDone <- err
		}()

		Consistently(func() int {
			return stub.countCalls(http.MethodGet, "/app/installations")
		}).Should(Equal(1))
		close(stub.installationsRelease)
		Eventually(firstDone).Should(Receive(Succeed()))
		Eventually(secondDone).Should(Receive(Succeed()))

		newTarget, err := service.store.GetTarget(
			GinkgoT().Context(),
			"github:installation:222",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(newTarget.Available).To(BeTrue())
	})

	It("shows the root owner installations discovered after sign-in", func() {
		stub.installations = `[{"id":111,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}}]`
		stub.repos = `{"repositories":[{"id":31,"name":"smyklot","full_name":"smykla-skalski/smyklot","owner":{"login":"smykla-skalski"}}]}`
		stub.members = `[{"id":42,"login":"smykla-skalski"}]`
		_, err := service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())

		now := time.Now().UTC()
		owner := storage.Account{
			ID:          githubProvider(endpoint.URL) + ":user:42",
			Provider:    githubProvider(endpoint.URL),
			SubjectID:   "42",
			Login:       "smykla-skalski",
			DisplayName: "Smykla Skalski",
			UpdatedAt:   now,
		}
		Expect(service.store.UpsertAccount(GinkgoT().Context(), owner)).To(Succeed())
		Expect(service.store.ReconcileSuperRoot(GinkgoT().Context(), owner.ID, now)).To(Succeed())
		stub.installations = `[
			{"id":111,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}},
			{"id":222,"account":{"id":8,"login":"another-org","type":"Organization"}}
		]`
		service.maintainPanel(GinkgoT().Context())

		targets, err := service.store.ListTargets(GinkgoT().Context(), owner.ID, time.Now().UTC())
		Expect(err).NotTo(HaveOccurred())
		Expect(targets).To(HaveLen(2))
	})

	It("persists every GitHub organization admin as an installation Owner", func() {
		stub.installations = `[{"id":111,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}}]`
		stub.members = `[
			{"id":42,"login":"bart","avatar_url":"https://avatars.example/42"},
			{"id":43,"login":"ada"}
		]`
		targetIDs, err := service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())
		target, err := service.store.GetTarget(GinkgoT().Context(), targetIDs[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(target.Ownership.Source).To(Equal(storage.OwnershipSourceOrganizationAdmin))
		Expect(target.Ownership.Status).To(Equal(storage.OwnershipStatusFresh))
		Expect(target.Ownership.OwnerCount).To(Equal(2))
		Expect(target.Ownership.Detail).To(BeNil())
	})

	It("records installation permission approval without hiding catalog diagnostics", func() {
		stub.installations = `[{"id":111,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}}]`
		stub.membersStatus = http.StatusForbidden
		stub.members = `{"message":"Resource not accessible by integration"}`
		targetIDs, err := service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())
		target, err := service.store.GetTarget(GinkgoT().Context(), targetIDs[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(target.Available).To(BeTrue())
		Expect(target.Ownership.Status).To(Equal(storage.OwnershipStatusPermissionPending))
		Expect(target.Ownership.OwnerCount).To(BeZero())
		Expect(target.Ownership.Detail).To(HaveValue(ContainSubstring("permission")))
	})

	It("announces catalog changes after the catalog commits", func() {
		stub.installations = `[{"id":111,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}}]`
		stub.repos = `{"repositories":[{"id":31,"name":"smyklot","full_name":"smykla-skalski/smyklot","owner":{"login":"smykla-skalski"}}]}`
		stub.members = `[{"id":42,"login":"smykla-skalski"}]`
		_, err := service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())

		now := time.Now().UTC()
		owner := storage.Account{
			ID:          githubProvider(endpoint.URL) + ":user:42",
			Provider:    githubProvider(endpoint.URL),
			SubjectID:   "42",
			Login:       "smykla-skalski",
			DisplayName: "Smykla Skalski",
			UpdatedAt:   now,
		}
		Expect(service.store.UpsertAccount(GinkgoT().Context(), owner)).To(Succeed())
		Expect(service.store.ReconcileSuperRoot(GinkgoT().Context(), owner.ID, now)).To(Succeed())
		const sessionToken = "catalog-event-session"
		digest := sha256.Sum256([]byte(sessionToken))
		Expect(service.store.CreateSession(GinkgoT().Context(), storage.Session{
			TokenHash: hex.EncodeToString(digest[:]),
			AccountID: owner.ID,
			CreatedAt: now,
			ExpiresAt: now.Add(time.Hour),
		}, 1)).To(Succeed())

		panelEndpoint := httptest.NewServer(service.panel.Handler())
		DeferCleanup(panelEndpoint.Close)
		headers := http.Header{}
		headers.Set("Cookie", (&http.Cookie{
			Name:  "smyklot_panel_session",
			Value: sessionToken,
		}).String())
		headers.Set("Origin", "https://smyklot.example")
		connection, response, err := websocket.Dial(
			GinkgoT().Context(),
			"ws"+strings.TrimPrefix(panelEndpoint.URL, "http")+"/panel/api/v1/events",
			&websocket.DialOptions{HTTPHeader: headers},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusSwitchingProtocols))
		DeferCleanup(connection.CloseNow)

		events := make(chan runtimePanelEvent, 4)
		go func() {
			for {
				var event runtimePanelEvent
				if err := wsjson.Read(GinkgoT().Context(), connection, &event); err != nil {
					return
				}
				events <- event
			}
		}()
		var event runtimePanelEvent
		Eventually(events).Should(Receive(&event))
		Expect(event.Version).To(Equal(1))
		Expect(event.Type).To(Equal("ready"))

		stub.installations = `[
			{"id":111,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}},
			{"id":222,"account":{"id":8,"login":"another-org","type":"Organization"}}
		]`
		service.maintainPanel(GinkgoT().Context())
		Eventually(events).Should(Receive(&event))
		Expect(event.Type).To(Equal("resync"))

		targets, err := service.store.ListTargets(GinkgoT().Context(), owner.ID, time.Now().UTC())
		Expect(err).NotTo(HaveOccurred())
		Expect(targets).To(HaveLen(2))

		stub.repoConfig = "disable_mentions: true\n"
		client, err := github.NewClient("installation-token", endpoint.URL)
		Expect(err).NotTo(HaveOccurred())
		_, err = service.serviceConfig(
			GinkgoT().Context(),
			client,
			"github:installation:222",
			"github:repository:31",
			"another-org",
			"smyklot",
		)
		Expect(err).NotTo(HaveOccurred())
		Eventually(events).Should(Receive(&event))
		Expect(event.Type).To(Equal("repository.changed"))
		Expect(event.TargetID).To(Equal("github:installation:222"))
		Expect(event.RepositoryID).To(Equal("github:repository:31"))
		_, err = service.serviceConfig(
			GinkgoT().Context(),
			client,
			"github:installation:222",
			"github:repository:31",
			"another-org",
			"smyklot",
		)
		Expect(err).NotTo(HaveOccurred())
		Consistently(events).ShouldNot(Receive())

		stub.installations = `[]`
		_, err = service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())
		Eventually(events).Should(Receive(&event))
		Expect(event.Type).To(Equal("resync"))
		targets, err = service.store.ListTargets(GinkgoT().Context(), owner.ID, time.Now().UTC())
		Expect(err).NotTo(HaveOccurred())
		Expect(targets).To(BeEmpty())
	})

	It("resolves fresh panel settings over cached repository configuration", func() {
		stub.installations = `[{"id":111,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}}]`
		stub.repos = `{"repositories":[{"id":31,"name":"smyklot","full_name":"smykla-skalski/smyklot","owner":{"login":"smykla-skalski"}}]}`
		stub.repoConfig = "command_prefix: '?'\ndisable_mentions: true\n"
		targetIDs, err := service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())
		target, err := service.store.GetTarget(GinkgoT().Context(), targetIDs[0])
		Expect(err).NotTo(HaveOccurred())

		targetPrefix := "!"
		quietSuccess := true
		target, err = service.store.UpdateTargetSettings(
			GinkgoT().Context(),
			storage.TargetSettingsChange{
				TargetID:                 target.ID,
				ActorAccountID:           target.Account.ID,
				RepositoryDefaultEnabled: true,
				ConfigPatch: config.Patch{
					CommandPrefix: &targetPrefix,
					QuietSuccess:  &quietSuccess,
				},
				ExpectedRevision: target.Revision,
				ChangedAt:        time.Now(),
			},
		)
		Expect(err).NotTo(HaveOccurred())
		repository, err := service.store.GetRepository(
			GinkgoT().Context(),
			target.ID,
			"github:repository:31",
		)
		Expect(err).NotTo(HaveOccurred())
		repositoryPrefix := "#"
		repository, err = service.store.UpdateRepositorySettings(
			GinkgoT().Context(),
			storage.RepositorySettingsChange{
				TargetID:       target.ID,
				RepositoryID:   repository.ID,
				ActorAccountID: target.Account.ID,
				ConfigPatch: config.Patch{
					CommandPrefix: &repositoryPrefix,
				},
				ExpectedRevision: repository.Revision,
				ChangedAt:        time.Now(),
			},
		)
		Expect(err).NotTo(HaveOccurred())
		client, err := github.NewClient("installation-token", endpoint.URL)
		Expect(err).NotTo(HaveOccurred())

		effective, err := service.serviceConfig(
			GinkgoT().Context(),
			client,
			target.ID,
			repository.ID,
			"smykla-skalski",
			"smyklot",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(effective.CommandPrefix).To(Equal("#"))
		Expect(effective.QuietSuccess).To(BeTrue())
		Expect(effective.DisableMentions).To(BeTrue())

		freshPrefix := "%"
		_, err = service.store.UpdateRepositorySettings(
			GinkgoT().Context(),
			storage.RepositorySettingsChange{
				TargetID:       target.ID,
				RepositoryID:   repository.ID,
				ActorAccountID: target.Account.ID,
				ConfigPatch: config.Patch{
					CommandPrefix: &freshPrefix,
				},
				ExpectedRevision: repository.Revision,
				ChangedAt:        time.Now(),
			},
		)
		Expect(err).NotTo(HaveOccurred())
		effective, err = service.serviceConfig(
			GinkgoT().Context(),
			client,
			target.ID,
			repository.ID,
			"smykla-skalski",
			"smyklot",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(effective.CommandPrefix).To(Equal("%"))
		Expect(stub.countCalls(http.MethodGet, "/contents/.github/smyklot.yaml")).To(Equal(1))
	})

	It("reveals the observed repository file state when bypass is disabled", func() {
		stub.installations = `[{"id":111,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}}]`
		stub.repos = `{"repositories":[{"id":31,"name":"smyklot","full_name":"smykla-skalski/smyklot","owner":{"login":"smykla-skalski"}}]}`
		stub.repoConfig = "command_aliases: invalid\n"
		targetIDs, err := service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())
		target, err := service.store.GetTarget(GinkgoT().Context(), targetIDs[0])
		Expect(err).NotTo(HaveOccurred())
		repository, err := service.store.GetRepository(
			GinkgoT().Context(),
			target.ID,
			"github:repository:31",
		)
		Expect(err).NotTo(HaveOccurred())
		repository, err = service.store.UpdateRepositorySettings(
			GinkgoT().Context(),
			storage.RepositorySettingsChange{
				TargetID:             target.ID,
				RepositoryID:         repository.ID,
				ActorAccountID:       target.Account.ID,
				IgnoreRepositoryFile: true,
				ExpectedRevision:     repository.Revision,
				ChangedAt:            time.Now(),
			},
		)
		Expect(err).NotTo(HaveOccurred())
		client, err := github.NewClient("installation-token", endpoint.URL)
		Expect(err).NotTo(HaveOccurred())

		_, err = service.serviceConfig(
			GinkgoT().Context(),
			client,
			target.ID,
			repository.ID,
			"smykla-skalski",
			"smyklot",
		)
		Expect(err).NotTo(HaveOccurred())
		repository, err = service.store.GetRepository(
			GinkgoT().Context(),
			target.ID,
			repository.ID,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ConfigFileStatus).To(Equal(storage.RepositoryFileBypassed))

		repository, err = service.store.UpdateRepositorySettings(
			GinkgoT().Context(),
			storage.RepositorySettingsChange{
				TargetID:             target.ID,
				RepositoryID:         repository.ID,
				ActorAccountID:       target.Account.ID,
				IgnoreRepositoryFile: false,
				ExpectedRevision:     repository.Revision,
				ChangedAt:            time.Now(),
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ConfigFileStatus).To(Equal(storage.RepositoryFileInvalid))
	})

	It("enforces repository enablement and durable delivery claims", func() {
		stub.installations = `[{"id":987,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}}]`
		stub.repos = `{"repositories":[{"id":123456,"name":"smyklot","full_name":"smykla-skalski/smyklot","owner":{"login":"smykla-skalski"}}]}`
		targetIDs, err := service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())

		public := httptest.NewServer(service.handler())
		DeferCleanup(public.Close)
		workers := service.startWorkers()
		DeferCleanup(func() {
			service.closeQueue()
			workers.Wait()
		})
		body := commandDelivery("/approve")
		response := postDelivery(public, "issue_comment", "disabled-delivery", body, nil)
		Expect(response.StatusCode).To(Equal(http.StatusAccepted))
		Eventually(func() int {
			redelivery := postDelivery(public, "issue_comment", "disabled-delivery", body, nil)

			return redelivery.StatusCode
		}).Within(eventuallyWindow).Should(Equal(http.StatusOK))
		Expect(stub.countCalls(http.MethodPost, approveReviews)).To(BeZero())

		target, err := service.store.GetTarget(GinkgoT().Context(), targetIDs[0])
		Expect(err).NotTo(HaveOccurred())
		_, err = service.store.UpdateTargetSettings(
			GinkgoT().Context(),
			storage.TargetSettingsChange{
				TargetID:                 target.ID,
				ActorAccountID:           target.Account.ID,
				RepositoryDefaultEnabled: true,
				ExpectedRevision:         target.Revision,
				ChangedAt:                time.Now(),
			},
		)
		Expect(err).NotTo(HaveOccurred())

		stub.repos = `{"repositories":[]}`
		_, err = service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())
		repository, err := service.store.GetRepository(
			GinkgoT().Context(),
			target.ID,
			"github:repository:123456",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.Available).To(BeFalse())
		stub.repos = `{"repositories":[{"id":123456,"name":"smyklot","full_name":"smykla-skalski/smyklot","owner":{"login":"smykla-skalski"}}]}`

		body = delivery("edited", "/approve", "User", "2026-08-08T10:01:00Z", true)
		response = postDelivery(public, "issue_comment", "enabled-delivery", body, nil)
		Expect(response.StatusCode).To(Equal(http.StatusAccepted))
		Eventually(func() bool {
			refreshed, refreshErr := service.store.GetRepository(
				GinkgoT().Context(),
				target.ID,
				repository.ID,
			)

			return refreshErr == nil && refreshed.Available
		}).Within(eventuallyWindow).Should(BeTrue())
		Eventually(func() int {
			return stub.countCalls(http.MethodPost, approveReviews)
		}).Within(eventuallyWindow).Should(Equal(1))

		Eventually(func() int {
			redelivery := postDelivery(public, "issue_comment", "redelivery", body, nil)

			return redelivery.StatusCode
		}).Within(eventuallyWindow).Should(Equal(http.StatusOK))
		Consistently(func() int {
			return stub.countCalls(http.MethodPost, approveReviews)
		}).Should(Equal(1))
	})

	It("acknowledges a burst before one delayed on-demand catalog refresh", func() {
		stub.installations = `[{"id":987,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}}]`
		stub.repos = `{"repositories":[{"id":123456,"name":"smyklot","full_name":"smykla-skalski/smyklot","owner":{"login":"smykla-skalski"}}]}`
		targetIDs, err := service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())
		target, err := service.store.GetTarget(GinkgoT().Context(), targetIDs[0])
		Expect(err).NotTo(HaveOccurred())
		_, err = service.store.UpdateTargetSettings(
			GinkgoT().Context(),
			storage.TargetSettingsChange{
				TargetID:                 target.ID,
				ActorAccountID:           target.Account.ID,
				RepositoryDefaultEnabled: true,
				ExpectedRevision:         target.Revision,
				ChangedAt:                time.Now(),
			},
		)
		Expect(err).NotTo(HaveOccurred())
		stub.repos = `{"repositories":[]}`
		_, err = service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())
		stub.repos = `{"repositories":[{"id":123456,"name":"smyklot","full_name":"smykla-skalski/smyklot","owner":{"login":"smykla-skalski"}}]}`
		stub.installationsStarted = make(chan struct{})
		stub.installationsRelease = make(chan struct{})
		catalogCalls := stub.countCalls(http.MethodGet, "/app/installations")

		workers := service.startWorkers()
		DeferCleanup(func() {
			service.closeQueue()
			workers.Wait()
		})
		public := httptest.NewServer(service.handler())
		DeferCleanup(public.Close)
		for index := range 4 {
			body := delivery(
				"edited",
				"/approve",
				"User",
				fmt.Sprintf("2026-08-08T10:0%d:00Z", index+1),
				true,
			)
			response := postDelivery(
				public,
				"issue_comment",
				fmt.Sprintf("delayed-catalog-%d", index),
				body,
				nil,
			)
			Expect(response.StatusCode).To(Equal(http.StatusAccepted))
		}
		Eventually(stub.installationsStarted).Should(BeClosed())
		Consistently(func() int {
			return stub.countCalls(http.MethodPost, approveReviews)
		}).Should(BeZero())
		close(stub.installationsRelease)
		Eventually(func() int {
			return stub.countCalls(http.MethodPost, approveReviews)
		}).Within(eventuallyWindow).Should(Equal(4))
		Expect(stub.countCalls(http.MethodGet, "/app/installations") - catalogCalls).To(Equal(1))
	})

	It("retries a queue-full claim release so redelivery remains possible", func() {
		stub.installations = `[{"id":987,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}}]`
		stub.repos = `{"repositories":[{"id":123456,"name":"smyklot","full_name":"smykla-skalski/smyklot","owner":{"login":"smykla-skalski"}}]}`
		_, err := service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())

		baseStore := service.store
		flakyStore := &transientAbandonStore{
			Store:        baseStore,
			retryStarted: make(chan struct{}),
			retryRelease: make(chan struct{}),
		}
		service.store = flakyStore
		for range cap(service.jobs) {
			service.jobs <- job{}
		}

		event, err := webhook.ParseIssueComment(commandDelivery("/approve"))
		Expect(err).NotTo(HaveOccurred())
		delivery := job{
			event:      event,
			key:        event.IdempotencyKey(),
			deliveryID: "queue-full-redelivery",
			logger:     service.logger,
		}
		response := httptest.NewRecorder()
		service.dispatch(GinkgoT().Context(), response, delivery)
		Expect(response.Code).To(Equal(http.StatusServiceUnavailable))
		Eventually(flakyStore.retryStarted).Should(BeClosed())
		inProgress := httptest.NewRecorder()
		service.dispatch(GinkgoT().Context(), inProgress, delivery)
		Expect(inProgress.Code).To(Equal(http.StatusServiceUnavailable))

		<-service.jobs
		close(flakyStore.retryRelease)
		Eventually(func() int {
			redelivery := httptest.NewRecorder()
			service.dispatch(GinkgoT().Context(), redelivery, delivery)

			return redelivery.Code
		}).Within(eventuallyWindow).Should(Equal(http.StatusAccepted))
		Expect(flakyStore.attempts.Load()).To(BeNumerically(">=", 2))
	})

	It("rejects an installation without an immutable account identity", func() {
		_, err := installationSnapshot(
			"",
			github.Installation{ID: 1, Account: "org", AccountType: "Organization"},
			[]github.Repository{{ID: 1, Name: "repo"}},
			time.Now(),
		)
		Expect(err).To(MatchError(ContainSubstring("installation identity")))
	})

	It("uses the canonical public provider identifier", func() {
		Expect(githubProvider("")).To(Equal("github:https://api.github.com"))
		Expect(githubProvider("https://github.example/api/v3/")).
			To(Equal("github:https://github.example/api/v3"))
		Expect(strings.HasPrefix(githubProvider(endpoint.URL), "github:http://")).To(BeTrue())
	})
})
