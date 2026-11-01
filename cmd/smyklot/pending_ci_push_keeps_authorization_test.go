package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/internal/pendingci/gate"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

const (
	scenarioNoticeWindow   = 5 * time.Second
	scenarioOtherBase      = "release"
	scenarioAnotherAccount = "another-account"
)

var _ = Describe("Push keeps authorization [Unit]", func() {
	It("Push after the command", func() {
		s := startPushScenario()

		s.writeSquashAfterCI()
		s.expectApprovals(1, scenarioSetupWindow)
		s.expectWaitingForCIOn(scenarioFirstSHA, scenarioSetupWindow)
		noticedAt := s.pushOneMoreCommitAs(scenarioAnotherAccount)
		s.expectApprovals(2, remaining(noticedAt))
		s.expectWaitingForCIOn(scenarioPushSHA, remaining(noticedAt))
		s.waitUntilCIIsGreen()
		s.expectMergeWithin(scenarioQuietPeriod + scenarioMergeSlack)
	})

	It("Push after the person lost permission", func() {
		s := startPushScenario()

		s.writeSquashAfterCI()
		s.expectWaitingForCIOn(scenarioFirstSHA, scenarioSetupWindow)
		s.removeFromCodeowners(githubtest.DefaultAuthor)
		noticedAt := s.pushOneMoreCommitAs(githubtest.DefaultAuthor)
		s.expectReauthorizeActionOn(scenarioPushSHA, remaining(noticedAt))
		s.expectNoNewApproval(remaining(noticedAt))
		s.waitUntilCIIsGreen()
		s.expectNoMergeFor(scenarioQuietPeriod + scenarioNoMergeWait)
		s.putBackInCodeowners(githubtest.DefaultAuthor)
		authorizedAt := s.clickReauthorize()
		s.expectReconciliationWithin(scenarioReconcileWait, authorizedAt)
		s.expectMergeWithin(scenarioQuietPeriod + scenarioMergeSlack)
	})

	It("Base change after the command", func() {
		s := startPushScenario()
		s.includeBase(scenarioOtherBase)

		s.writeSquashAfterCI()
		s.expectWaitingForCIOn(scenarioFirstSHA, scenarioSetupWindow)
		noticedAt := s.changeBaseBranch(scenarioOtherBase)
		s.expectReauthorizeActionOn(scenarioFirstSHA, remaining(noticedAt))
		s.waitUntilCIIsGreenOn(scenarioFirstSHA)
		s.expectNoMergeFor(scenarioQuietPeriod + scenarioNoMergeWait)
	})
})

type pushScenario struct {
	*scenarioService
	reviews *scenarioReviews
}

func startPushScenario() *pushScenario {
	GinkgoHelper()
	reviews := &scenarioReviews{}

	return &pushScenario{scenarioService: startScenarioService(reviews.wrap), reviews: reviews}
}

func remaining(noticedAt time.Time) time.Duration {
	return time.Until(noticedAt.Add(scenarioNoticeWindow))
}

func (s *pushScenario) includeBase(branch string) {
	GinkgoHelper()
	ctx := GinkgoT().Context()
	target, err := s.server.store.GetTarget(ctx, storage.InstallationID(987))
	Expect(err).NotTo(HaveOccurred())
	_, err = s.server.store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
		TargetID: target.ID, ActorAccountID: target.Account.ID, ChangedAt: time.Now(),
		Target: &storage.InstallationTargetSettingsChange{
			RepositoryDefaultEnabled: true, PendingCIModeDefault: storage.PendingCIModeChecks,
			PendingCIBranchPatternsDefault: storage.PendingCIBranchPatterns{
				Include: []string{"~DEFAULT_BRANCH", "refs/heads/" + branch}, Exclude: []string{},
			},
			ExpectedRevision: target.Revision,
		},
	})
	Expect(err).NotTo(HaveOccurred())
}

func (s *pushScenario) expectApprovals(count int, within time.Duration) {
	GinkgoHelper()
	Eventually(s.reviews.posted).Within(within).Should(Equal(count),
		"Smyklot must approve the pull request as the bot")
}

func (s *pushScenario) expectNoNewApproval(window time.Duration) {
	GinkgoHelper()
	Consistently(s.reviews.posted, window).Should(Equal(1),
		"Smyklot must not approve for a person who may no longer give the command")
}

func (s *pushScenario) expectWaitingForCIOn(head string, within time.Duration) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		run, found := s.checks.smyklotRun(head)
		g.Expect(found).To(BeTrue(), "the Smyklot check must appear on %s", head)
		g.Expect(run.status).To(Equal("in_progress"))
		g.Expect(run.actions).To(BeEmpty())
	}).Within(within).Should(Succeed(), "the Smyklot check on %s must wait for CI with no action", head)
}

func (s *pushScenario) expectReauthorizeActionOn(head string, within time.Duration) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		run, found := s.checks.smyklotRun(head)
		g.Expect(found).To(BeTrue(), "the Smyklot check must appear on %s", head)
		g.Expect(run.conclusion).To(Equal("action_required"))
		g.Expect(run.actions).To(ContainElement(gate.ReauthorizeAction))
	}).Within(within).Should(Succeed(), "the Smyklot check on %s must ask for reauthorization", head)
}

func (s *pushScenario) pushOneMoreCommitAs(sender string) time.Time {
	GinkgoHelper()
	s.stub.prHead = scenarioPushSHA
	s.reviews.dropApprovals()
	s.deliver(webhook.EventPullRequest, scenarioPullRequestSynchronizeBy(scenarioPushSHA, sender))

	return time.Now()
}

func (s *pushScenario) changeBaseBranch(branch string) time.Time {
	GinkgoHelper()
	s.stub.prBase = branch
	s.deliver(webhook.EventPullRequest, scenarioPullRequestBaseEdited(scenarioFirstSHA, branch))

	return time.Now()
}

func (s *pushScenario) removeFromCodeowners(login string) {
	GinkgoHelper()
	Expect(s.stub.codeowners).To(ContainSubstring("@" + login))
	s.stub.codeowners = "* @someone-else\n"
}

func (s *pushScenario) putBackInCodeowners(login string) {
	GinkgoHelper()
	s.stub.codeowners = "* @" + login + "\n"
}

func (s *pushScenario) waitUntilCIIsGreenOn(head string) {
	GinkgoHelper()
	run := s.checks.add(scenarioCheckRun{
		name: "ci / build", headSHA: head, appID: scenarioCIAppID,
		status: "completed", conclusion: "success",
	})
	s.deliver(webhook.EventCheckRun, scenarioCheckCompleted("check_run", run))
	s.deliver(webhook.EventCheckSuite, scenarioCheckCompleted("check_suite", run))
}

func (s *pushScenario) expectNoMergeFor(window time.Duration) {
	GinkgoHelper()
	Consistently(func() int {
		return s.stub.countCalls(http.MethodPut, "/merge")
	}, window).Should(BeZero(), "a request that waits for reauthorization must not merge")
}

func scenarioPullRequestSynchronizeBy(head, sender string) []byte {
	return fmt.Appendf(nil, `{
  "action": "synchronize", "number": 42, "sender": {"login": %q},
  "pull_request": {"merged": false, "updated_at": "2026-11-01T12:00:00Z", "head": {"sha": %q}},
  "repository": {"id": 123456, "name": "smyklot", "full_name": "smykla-skalski/smyklot", "owner": {"login": "smykla-skalski"}},
  "installation": {"id": 987}
}`, sender, head)
}

func scenarioPullRequestBaseEdited(head, base string) []byte {
	return fmt.Appendf(nil, `{
  "action": "edited", "number": 42, "sender": {"login": "author"},
  "changes": {"base": {"ref": {"from": "main"}}},
  "pull_request": {"merged": false, "updated_at": "2026-11-01T12:00:00Z", "head": {"sha": %q}, "base": {"ref": %q}},
  "repository": {"id": 123456, "name": "smyklot", "full_name": "smykla-skalski/smyklot", "owner": {"login": "smykla-skalski"}},
  "installation": {"id": 987}
}`, head, base)
}

type scenarioReviews struct {
	mu        sync.Mutex
	approvers []string
	count     int
}

func (r *scenarioReviews) posted() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.count
}

func (r *scenarioReviews) dropApprovals() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.approvers = nil
}

func (r *scenarioReviews) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, approveReviews):
			r.serve(w, req)
		case strings.Contains(req.URL.Path, "/rules/branches/"):
			_, _ = w.Write([]byte(`[]`))
		default:
			next.ServeHTTP(w, req)
		}
	})
}

func (r *scenarioReviews) serve(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if req.Method == http.MethodGet {
		items := make([]string, 0, len(r.approvers))
		for i, login := range r.approvers {
			items = append(items, fmt.Sprintf(`{"id":%d,"state":"APPROVED","user":{"login":%q}}`, i+1, login))
		}
		_, _ = fmt.Fprintf(w, "[%s]", strings.Join(items, ","))

		return
	}
	var body struct {
		Event string `json:"event"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Event != "APPROVE" {
		w.WriteHeader(http.StatusUnprocessableEntity)

		return
	}
	r.count++
	r.approvers = append(r.approvers, bot.DefaultBotUsername)
	_, _ = fmt.Fprintf(w, `{"id":%d,"state":"APPROVED","user":{"login":%q}}`, r.count, bot.DefaultBotUsername)
}
