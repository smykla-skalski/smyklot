package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

func recoveryReauthorizationProvider(stub *githubStub) http.Handler {
	stub.prHead = "new-head"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body string
		switch {
		case r.URL.Path == "/graphql":
			body = `{"data":{"repository":{"mergeQueue":null}}}`
		case strings.HasSuffix(r.URL.Path, "/protection/required_status_checks"):
			body = fmt.Sprintf(`{"checks":[{"context":%q,"app_id":1197525}]}`, storage.PendingCICheckName)
		case strings.HasSuffix(r.URL.Path, "/rules/branches/main"):
			body = `[]`
		case strings.HasSuffix(r.URL.Path, "/check-runs/701"):
			body = fmt.Sprintf(`{"id":701,"name":%q,"external_id":%q,"head_sha":"new-head","app":{"id":1197525},"html_url":"https://github.example/checks/701"}`, storage.PendingCICheckName, "smyklot:merge-after-ci:"+storage.RepositoryID(123456)+":new-head")
		default:
			stub.ServeHTTP(w, r)
			return
		}
		stub.mu.Lock()
		stub.calls = append(stub.calls, r.Method+" "+r.URL.Path)
		stub.mu.Unlock()
		_, _ = w.Write([]byte(body))
	})
}

func recoveryReauthorizationPayload() []byte {
	return fmt.Appendf(nil, `{
 "action":"requested_action",
 "check_run":{"id":701,"name":%q,"external_id":%q,"head_sha":"new-head","app":{"id":1197525},"pull_requests":[{"number":42}]},
 "requested_action":{"identifier":"reauthorize"},"sender":{"login":"someone"},
 "repository":{"id":123456,"name":"smyklot","full_name":"smykla-skalski/smyklot","owner":{"login":"smykla-skalski"}},"installation":{"id":987}
 }`, storage.PendingCICheckName, "smyklot:merge-after-ci:"+storage.RepositoryID(123456)+":new-head")
}

func armRecoveryReauthorization(f recoveryWorkerFixture) pendingci.Request {
	GinkgoHelper()
	now := time.Now().UTC()
	ensure := func(head string) pendingci.CheckSlot {
		slot, err := f.service.store.EnsureCheckSlot(GinkgoT().Context(), pendingci.EnsureCheckSlotRequest{
			TargetID: f.target, InstallationID: 987, RepositoryID: storage.RepositoryID(123456), RepositoryFullName: "smykla-skalski/smyklot",
			PullRequest: 42, HeadSHA: head, AppID: 1197525, Name: storage.PendingCICheckName,
			ExternalID:    "smyklot:merge-after-ci:" + storage.RepositoryID(123456) + ":" + head,
			DesiredStatus: "in_progress", DesiredTitle: "Authorization required", DesiredSummary: "The head changed", DesiredDigest: head, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		return slot
	}
	original := ensure("old-head")
	armed, err := f.service.store.Arm(GinkgoT().Context(), pendingci.ArmRequest{
		TargetID: f.target, InstallationID: 987, RepositoryID: storage.RepositoryID(123456), RepositoryFullName: "smykla-skalski/smyklot",
		PullRequest: 42, HeadSHA: "old-head", BaseBranch: "main", MergeMethod: pendingci.MergeMethodSquash,
		Requester: "someone", SourceCommentID: 555, SourceRevision: now.Format(time.RFC3339Nano), SourceSequence: 1, SourceOrder: 1,
		ArtifactKind: pendingci.ArtifactCheck, CheckSlotID: &original.ID, RequestedAt: now,
	})
	Expect(err).NotTo(HaveOccurred())
	slot := ensure("new-head")
	_, err = f.service.store.BindCheckRun(GinkgoT().Context(), pendingci.BindCheckRunRequest{
		ID: slot.ID, ExpectedRevision: slot.Revision, CheckRunID: 701, CheckURL: "https://github.example/checks/701", BoundAt: now,
	})
	Expect(err).NotTo(HaveOccurred())
	waiting, err := f.service.store.RequireReauthorization(GinkgoT().Context(), pendingci.RequireReauthorizationRequest{
		ID: armed.Request.ID, ExpectedRevision: armed.Request.Revision, CandidateHeadSHA: "new-head", CandidateBase: "main", CandidateCheckID: slot.ID, ObservedAt: now,
	})
	Expect(err).NotTo(HaveOccurred())
	return waiting
}

var _ = Describe("Recovered reauthorization worker [Unit]", func() {
	DescribeTable("checks the action again before restoring approval", func(change string) {
		f := newRecoveryProviderFixture(webhook.EventCheckRun, recoveryReauthorizationPayload(), recoveryReauthorizationProvider)
		waiting := armRecoveryReauthorization(f)
		preview := f.call(http.MethodGet)
		Expect(preview.Code).To(Equal(http.StatusOK), preview.Body.String())
		Expect(preview.Body.String()).To(ContainSubstring(`"available":true`))
		Expect(preview.Body.String()).To(ContainSubstring("restore the required approval"))
		accepted := f.call(http.MethodPost)
		Expect(accepted.Code).To(Equal(http.StatusAccepted), accepted.Body.String())
		Expect(f.stub.countCalls(http.MethodPost, approveReviews)).To(BeZero())
		before, err := f.service.store.Get(GinkgoT().Context(), waiting.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(before).To(Equal(waiting), "preview and acceptance must not authorize the request")
		switch change {
		case "permission":
			f.stub.codeowners = "* @someone-else\n"
		case "head":
			f.stub.prHead = "even-newer-head"
		}
		runRecoveredNotification(f)
		after, err := f.service.store.Get(GinkgoT().Context(), waiting.ID)
		Expect(err).NotTo(HaveOccurred())
		if change != "none" {
			Expect(after.AuthorizationState).To(Equal(pendingci.AuthorizationReauthorizationNeeded))
			Expect(f.stub.countCalls(http.MethodPost, approveReviews)).To(BeZero())
		} else {
			Expect(after.AuthorizationState).To(Equal(pendingci.AuthorizationAuthorized))
			Expect(after.HeadSHA).To(Equal("new-head"))
			Expect(after.AuthorizedBy).To(Equal("someone"))
			Expect(f.stub.countCalls(http.MethodPost, approveReviews)).To(Equal(1))
		}
		Expect(f.stub.countCalls(http.MethodPut, "/merge")).To(BeZero())
		repeated := f.call(http.MethodPost)
		Expect(repeated.Code).To(Equal(http.StatusOK), repeated.Body.String())
		final, err := f.service.store.Get(GinkgoT().Context(), waiting.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(final).To(Equal(after))
	}, Entry("original requester is still allowed", "none"), Entry("original requester loses permission after submission", "permission"), Entry("pull request changes after submission", "head"))
})
