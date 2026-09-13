package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type recoveryWorkerFixture struct {
	service  *server
	stub     *githubStub
	target   string
	cookie   *http.Cookie
	original int64
	source   pendingci.SourceRevisionRequest
	path     string
	request  []byte
}

func newRecoveryWorkerFixture() recoveryWorkerFixture {
	return newRecoveryEventFixture(webhook.EventIssueComment, commandDelivery("/approve"))
}

func newRecoveryEventFixture(eventName string, payload []byte) recoveryWorkerFixture {
	GinkgoHelper()
	stub := newGitHubStub()
	stub.installations = `[{"id":987,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}}]`
	stub.repos = `{"repositories":[{"id":123456,"name":"smyklot","default_branch":"main","full_name":"smykla-skalski/smyklot","owner":{"login":"smykla-skalski"}}]}`
	stub.members = `[{"id":42,"login":"bart"}]`
	endpoint := httptest.NewServer(stub)
	DeferCleanup(endpoint.Close)
	service, err := newServer(&serveConfig{
		database:    GinkgoT().TempDir() + "/recovery.sqlite3",
		webhookPath: defaultWebhookPath, webhookSecret: []byte(testSecret),
		apiBaseURL: endpoint.URL, botUsername: bot.DefaultBotUsername,
		appClientID: "Iv1.test", appPrivateKey: githubtest.AppPrivateKey(),
		botConfig: config.Default(), logWriter: io.Discard,
		panel: &panelServeConfig{
			publicOrigin: "https://smyklot.example", basePath: defaultPanelBase,
			superRootID: 42, clientID: "Iv1.test", clientSecret: "oauth-secret",
			authorizeURL: endpoint.URL + "/authorize", tokenURL: endpoint.URL + "/token",
			sessionTTL: defaultPanelTTL,
		},
	})
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(service.Close)
	targets, err := service.SyncCatalog(GinkgoT().Context())
	Expect(err).NotTo(HaveOccurred())
	Expect(targets).To(HaveLen(1))
	target, err := service.store.GetTarget(GinkgoT().Context(), targets[0])
	Expect(err).NotTo(HaveOccurred())
	_, err = service.store.SaveInstallationSettings(GinkgoT().Context(), storage.SaveInstallationSettingsRequest{
		TargetID: target.ID, ActorAccountID: target.Account.ID, ChangedAt: time.Now(),
		Target: &storage.InstallationTargetSettingsChange{RepositoryDefaultEnabled: true, ExpectedRevision: target.Revision},
	})
	Expect(err).NotTo(HaveOccurred())

	now := time.Now().UTC()
	owner := storage.Account{ID: githubProvider(endpoint.URL) + ":user:42", Provider: githubProvider(endpoint.URL), SubjectID: "42", Login: "bart", UpdatedAt: now}
	Expect(service.store.UpsertAccount(GinkgoT().Context(), owner)).To(Succeed())
	Expect(service.store.ReconcileSuperRoot(GinkgoT().Context(), owner.ID, now)).To(Succeed())
	const token = "worker-recovery-session"
	digest := sha256.Sum256([]byte(token))
	Expect(service.store.CreateSession(GinkgoT().Context(), storage.Session{
		TokenHash: hex.EncodeToString(digest[:]), AccountID: owner.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}, 1)).To(Succeed())

	if eventName == webhook.EventIssueComment {
		stub.observeIssueComment(payload)
	}
	repository := storage.RepositoryID(githubtest.DefaultRepoID)
	claim, err := service.store.ClaimDelivery(GinkgoT().Context(), storage.DeliveryClaim{
		ClaimKey: "original-worker-recovery", DeliveryID: "original-worker-recovery",
		TargetID: target.ID, RepositoryID: &repository, RepositoryFullName: "smykla-skalski/smyklot",
		Event: eventName, Payload: payload, ClaimedAt: now,
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(service.store.FailDelivery(GinkgoT().Context(), storage.DeliveryFailureChange{
		ClaimID: claim.ID, Stage: "execute", Reason: "original configuration failure", FailedAt: now,
	})).To(Succeed())
	operation, err := service.store.GetDeliveryOperation(GinkgoT().Context(), target.ID, claim.ID)
	Expect(err).NotTo(HaveOccurred())
	var source pendingci.SourceRevisionRequest
	if eventName == webhook.EventIssueComment {
		event, parseErr := webhook.ParseIssueComment(payload)
		Expect(parseErr).NotTo(HaveOccurred())
		source = pendingci.SourceRevisionRequest{RepositoryID: repository, PullRequest: event.Issue.Number, CommentID: event.Comment.ID, Revision: event.Comment.UpdatedAt, Sequence: pendingci.CommentSequence(event.Action), SourceOrder: operation.SourceOrder, EventKey: "original-worker-recovery", ObservedAt: now}
	}
	request, err := json.Marshal(map[string]any{"expected_run_id": claim.ID, "expected_revision": operation.Revision, "request_key": "worker-recovery"})
	Expect(err).NotTo(HaveOccurred())
	return recoveryWorkerFixture{
		service: service, stub: stub, target: target.ID, original: claim.ID,
		cookie: &http.Cookie{Name: "smyklot_panel_session", Value: token},
		path:   fmt.Sprintf("/panel/api/v1/targets/%s/deliveries/%d/recovery", target.ID, claim.ID), request: request,
		source: source,
	}
}

func (f recoveryWorkerFixture) call(method string) *httptest.ResponseRecorder {
	GinkgoHelper()
	request := httptest.NewRequest(method, "https://smyklot.example"+f.path, bytes.NewReader(f.request))
	request.AddCookie(f.cookie)
	request.Header.Set("Origin", "https://smyklot.example")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	f.service.panel.Handler().ServeHTTP(response, request)
	return response
}

var _ = Describe("Recovered delivery worker [Unit]", func() {
	DescribeTable("executes recovery through current source and permission guards", func(change string, approvals int) {
		f := newRecoveryWorkerFixture()
		originalQueue, err := f.service.store.GetQueueItem(GinkgoT().Context(), fmt.Sprintf("delivery:%d", f.original))
		Expect(err).NotTo(HaveOccurred())
		preview := f.call(http.MethodGet)
		Expect(preview.Code).To(Equal(http.StatusOK), preview.Body.String())
		Expect(preview.Body.String()).To(ContainSubstring(`"available":true`))
		Expect(f.stub.countCalls(http.MethodPost, approveReviews)).To(BeZero())
		response := f.call(http.MethodPost)
		Expect(response.Code).To(Equal(http.StatusAccepted), response.Body.String())
		var result struct {
			RunID int64 `json:"run_id"`
		}
		Expect(json.Unmarshal(response.Body.Bytes(), &result)).To(Succeed())
		Expect(result.RunID).NotTo(Equal(f.original))

		switch change {
		case "permission":
			// The preview and submission completed before permission changed. No worker ran yet.
			f.stub.codeowners = "* @someone-else\n"
		case "comment":
			f.stub.observeIssueComment(commandDelivery("/help"))
		case "source order":
			newer := f.source
			newer.EventKey = "newer-event"
			newer.SourceOrder++
			accepted, err := f.service.store.ClaimSourceRevision(GinkgoT().Context(), newer)
			Expect(err).NotTo(HaveOccurred())
			Expect(accepted.Accepted).To(BeTrue())
		}
		f.service.deliveries.Start(GinkgoT().Context())
		DeferCleanup(func() { Expect(f.service.deliveries.Shutdown(context.Background())).To(Succeed()) })
		Eventually(func() storage.DeliveryStatus {
			op, err := f.service.store.GetDeliveryOperation(GinkgoT().Context(), f.target, f.original)
			Expect(err).NotTo(HaveOccurred())
			Expect(op.Current.ID).To(Equal(result.RunID))
			Expect(op.SourceOrder).To(Equal(f.source.SourceOrder))
			return op.Current.Status
		}).Within(eventuallyWindow).Should(BeElementOf(storage.DeliverySucceeded, storage.DeliveryFailed))
		Expect(f.stub.countCalls(http.MethodPost, approveReviews)).To(Equal(approvals))
		if change == "none" {
			retained, err := f.service.store.CheckSourceRevision(GinkgoT().Context(), f.source)
			Expect(err).NotTo(HaveOccurred())
			Expect(retained.Accepted).To(BeTrue())
			Expect(retained.SourceOrder).To(Equal(f.source.SourceOrder))
		}
		original, err := f.service.store.GetDeliveryOperation(GinkgoT().Context(), f.target, f.original)
		Expect(err).NotTo(HaveOccurred())
		repeated := f.call(http.MethodPost)
		Expect(repeated.Code).To(Equal(http.StatusOK), repeated.Body.String())
		Expect(repeated.Body.String()).To(ContainSubstring(`"repeated":true`))
		after, err := f.service.store.GetDeliveryOperation(GinkgoT().Context(), f.target, f.original)
		Expect(err).NotTo(HaveOccurred())
		Expect(after.Current.ID).To(Equal(original.Current.ID))
		Expect(after.Revision).To(Equal(original.Revision))
		queue, err := f.service.store.GetQueueItem(GinkgoT().Context(), fmt.Sprintf("delivery:%d", f.original))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(queue.State)).To(Equal("failed"))
		Expect(queue).To(Equal(originalQueue))
		fmt.Fprintf(GinkgoWriter, "case=%s original=%d recovered=%d status=%s approvals=%d original_preserved=true\n", change, f.original, result.RunID, after.Current.Status, approvals)
	}, Entry("accepted command", "none", 1), Entry("permission revoked after submission", "permission", 0), Entry("comment changed after submission", "comment", 0), Entry("newer source with the same timestamp", "source order", 0))
})
