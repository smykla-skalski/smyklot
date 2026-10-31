package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/pendingci/gate"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/storagetest"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

// The scenario "Push after the command" from
// .dogvane/findings/merge-after-ci-2026-10-31-reauthorize-waits.md, one step
// helper per sentence. The bounds are the scenario's; only the quiet period is
// shortened so the spec runs in seconds.
const (
	scenarioQuietPeriod   = 2 * time.Second
	scenarioNoMergeWait   = 10 * time.Second
	scenarioReconcileWait = 5 * time.Second
	scenarioMergeSlack    = 5 * time.Second
	scenarioSetupWindow   = 15 * time.Second

	scenarioAppID    = int64(1197525)
	scenarioCIAppID  = int64(15368)
	scenarioFirstSHA = "head-after-command"
	scenarioPushSHA  = "head-after-push"
)

var _ = Describe("Push after the command [Unit]", func() {
	It("merges only after a person reauthorizes, and reconciles within five seconds of that", func() {
		s := startScenarioService()

		s.writeSquashAfterCI()
		s.pushOneMoreCommit()
		s.waitUntilCIIsGreen()
		s.waitTenSecondsWithoutMerge()
		authorizedAt := s.clickReauthorize()
		s.expectReconciliationWithin(scenarioReconcileWait, authorizedAt)
		s.expectMergeWithin(scenarioQuietPeriod + scenarioMergeSlack)
	})
})

type scenarioService struct {
	server  *server
	stub    *githubStub
	checks  *scenarioCheckRuns
	public  *httptest.Server
	request pendingci.Request
	next    int
}

func startScenarioService() *scenarioService {
	GinkgoHelper()
	stub := newGitHubStub()
	stub.installations = `[{"id":987,"account":{"id":7,"login":"smykla-skalski","type":"Organization"},` +
		`"permissions":{"administration":"write","checks":"write","contents":"write",` +
		`"merge_queues":"read","pull_requests":"write","statuses":"write"}}]`
	stub.repos = `{"repositories":[{"id":123456,"name":"smyklot","default_branch":"main","full_name":"smykla-skalski/smyklot","owner":{"login":"smykla-skalski"}}]}`
	stub.members = `[{"id":42,"login":"bart"}]`
	stub.prHead = scenarioFirstSHA
	checks := &scenarioCheckRuns{runs: map[int64]scenarioCheckRun{}}
	endpoint := httptest.NewServer(scenarioProvider(stub, checks))
	DeferCleanup(endpoint.Close)

	service, err := newServer(&serveConfig{
		database:             storagetest.Connection(GinkgoT(), GinkgoT().TempDir()),
		webhookPath:          defaultWebhookPath,
		webhookSecret:        []byte(testSecret),
		apiBaseURL:           endpoint.URL,
		botUsername:          bot.DefaultBotUsername,
		appClientID:          "Iv1.test",
		appPrivateKey:        githubtest.AppPrivateKey(),
		botConfig:            config.Default(),
		logWriter:            io.Discard,
		pendingCIQuietPeriod: scenarioQuietPeriod,
		panel: &panelServeConfig{
			publicOrigin: "https://smyklot.example", basePath: defaultPanelBase,
			superRootID: 42, clientID: "Iv1.test", clientSecret: "oauth-secret",
			authorizeURL: endpoint.URL + "/authorize", tokenURL: endpoint.URL + "/token",
			sessionTTL: defaultPanelTTL,
		},
	})
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(service.Close)
	enableChecksMode(service)

	service.deliveries.Start(GinkgoT().Context())
	DeferCleanup(func() { Expect(service.deliveries.Shutdown(context.Background())).To(Succeed()) })
	startPendingCITestScheduler(service)
	public := httptest.NewServer(service.handler())
	DeferCleanup(public.Close)

	return &scenarioService{server: service, stub: stub, checks: checks, public: public}
}

// enableChecksMode is the installation's panel configuration: merge after CI
// in checks mode, with the repository protection already reconciled. The
// scenario starts on a repository that is set up, so this is not a step.
func enableChecksMode(service *server) {
	GinkgoHelper()
	ctx := GinkgoT().Context()
	targets, err := service.SyncCatalog(ctx)
	Expect(err).NotTo(HaveOccurred())
	Expect(targets).To(HaveLen(1))
	target, err := service.store.GetTarget(ctx, targets[0])
	Expect(err).NotTo(HaveOccurred())
	_, err = service.store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
		TargetID: target.ID, ActorAccountID: target.Account.ID, ChangedAt: time.Now(),
		Target: &storage.InstallationTargetSettingsChange{
			RepositoryDefaultEnabled: true, PendingCIModeDefault: storage.PendingCIModeChecks,
			PendingCIBranchPatternsDefault: storage.DefaultPendingCIBranchPatterns(),
			ExpectedRevision:               target.Revision,
		},
	})
	Expect(err).NotTo(HaveOccurred())
	repositoryID := storage.RepositoryID(githubtest.DefaultRepoID)
	gate, err := service.store.GetPendingCIRepositoryGate(ctx, repositoryID)
	Expect(err).NotTo(HaveOccurred())
	Expect(gate.DesiredMode).To(Equal(storage.PendingCIModeChecks))
	appID := scenarioAppID
	rulesetID := int64(1)
	_, err = service.store.UpdatePendingCIRepositoryGate(ctx, storage.PendingCIGateChange{
		RepositoryID: repositoryID, ExpectedRevision: gate.Revision,
		EffectiveMode: storage.PendingCIEffectiveChecks, Readiness: storage.PendingCIReady,
		Reason: "ready", AppID: &appID, RulesetID: &rulesetID, ObservedAt: time.Now().UTC(),
	})
	Expect(err).NotTo(HaveOccurred())
}

// Step 1: on a pull request, write the command "squash after ci".
func (s *scenarioService) writeSquashAfterCI() {
	GinkgoHelper()
	s.deliver(webhook.EventIssueComment, commandDelivery("/squash after ci"))
	Eventually(func(g Gomega) {
		request, err := s.server.store.GetArmed(
			GinkgoT().Context(), storage.RepositoryID(githubtest.DefaultRepoID), githubtest.DefaultPRNumber,
		)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(request.MergeMethod).To(Equal(pendingci.MergeMethodSquash))
		g.Expect(request.HeadSHA).To(Equal(scenarioFirstSHA))
		s.request = request
	}).Within(scenarioSetupWindow).Should(Succeed(), "the command must arm a merge-after-CI request")
}

// Step 2: push one more commit to the pull request.
func (s *scenarioService) pushOneMoreCommit() {
	GinkgoHelper()
	s.stub.prHead = scenarioPushSHA
	s.deliver(webhook.EventPullRequest, scenarioPullRequestSynchronize(scenarioPushSHA))
	Eventually(func(g Gomega) {
		run, found := s.checks.smyklotRun(scenarioPushSHA)
		g.Expect(found).To(BeTrue(), "the Smyklot check must appear on the new head")
		g.Expect(run.conclusion).To(Equal("action_required"))
		g.Expect(run.actions).To(ContainElement(gate.ReauthorizeAction))
	}).Within(scenarioSetupWindow).Should(Succeed(), "the new head must ask for reauthorization")
}

// Step 3: wait until all CI checks are green.
func (s *scenarioService) waitUntilCIIsGreen() {
	GinkgoHelper()
	run := s.checks.add(scenarioCheckRun{
		name: "ci / build", headSHA: scenarioPushSHA, appID: scenarioCIAppID,
		status: "completed", conclusion: "success",
	})
	s.deliver(webhook.EventCheckRun, scenarioCheckCompleted("check_run", run))
	s.deliver(webhook.EventCheckSuite, scenarioCheckCompleted("check_suite", run))
}

// Step 4: wait 10 more seconds. Smyklot must not merge.
func (s *scenarioService) waitTenSecondsWithoutMerge() {
	GinkgoHelper()
	Consistently(func() int {
		return s.stub.countCalls(http.MethodPut, "/merge")
	}, scenarioNoMergeWait).Should(BeZero(), "a pushed head must not merge on the old authorization")
}

// Step 5: click Reauthorize on the Smyklot check.
func (s *scenarioService) clickReauthorize() time.Time {
	GinkgoHelper()
	run, found := s.checks.smyklotRun(scenarioPushSHA)
	Expect(found).To(BeTrue())
	clickedAt := time.Now().UTC()
	s.deliver(webhook.EventCheckRun, scenarioRequestedAction(run, gate.ReauthorizeAction, "someone"))
	var authorized pendingci.Request
	Eventually(func(g Gomega) {
		request, err := s.server.store.Get(GinkgoT().Context(), s.request.ID)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(request.AuthorizationState).To(Equal(pendingci.AuthorizationAuthorized))
		authorized = request
	}).Within(scenarioReconcileWait).Should(Succeed(), "the click must authorize the pushed head")
	Expect(authorized.HeadSHA).To(Equal(scenarioPushSHA))
	Expect(authorized.AuthorizedBy).To(Equal("someone"))
	Expect(authorized.AuthorizedAt).To(BeTemporally(">=", clickedAt.Add(-time.Second)))

	return authorized.AuthorizedAt
}

// Step 6: within 5 seconds, Smyklot must record a reconciliation of the request.
func (s *scenarioService) expectReconciliationWithin(bound time.Duration, authorizedAt time.Time) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		events, err := s.server.store.ListEvents(
			GinkgoT().Context(), pendingci.EventFilter{RequestID: s.request.ID},
		)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(events).To(ContainElement(SatisfyAll(
			HaveField("Kind", pendingci.EventReconciliationStarted),
			HaveField("CreatedAt", BeTemporally(">=", authorizedAt)),
			HaveField("CreatedAt", BeTemporally("<=", authorizedAt.Add(bound))),
		)))
	}).Within(bound+eventuallyWindow).Should(Succeed(),
		"the reauthorized request must be reconciled within five seconds, not on the next fallback check")
}

// Step 7: wait for the quiet period and 5 more seconds. Smyklot must merge the
// pull request.
func (s *scenarioService) expectMergeWithin(bound time.Duration) {
	GinkgoHelper()
	Eventually(func() int {
		return s.stub.countCalls(http.MethodPut, "/merge")
	}).Within(bound).Should(Equal(1), "the pull request must merge once the quiet period passes")
	Expect(s.checks.wasMerged()).To(BeTrue())
}

func (s *scenarioService) deliver(event string, body []byte) {
	GinkgoHelper()
	s.next++
	deliverAccepted(s.public, s.stub, event, "scenario-delivery-"+strconv.Itoa(s.next), body)
}

func scenarioPullRequestSynchronize(head string) []byte {
	return fmt.Appendf(nil, `{
  "action": "synchronize", "number": 42,
  "pull_request": {"merged": false, "updated_at": "2026-10-31T12:00:00Z", "head": {"sha": %q}},
  "repository": {"id": 123456, "name": "smyklot", "full_name": "smykla-skalski/smyklot", "owner": {"login": "smykla-skalski"}},
  "installation": {"id": 987}
}`, head)
}

func scenarioCheckCompleted(subject string, run scenarioCheckRun) []byte {
	return fmt.Appendf(nil, `{
  "action": "completed",
  %q: {"id": %d, "name": %q, "head_sha": %q, "status": "completed", "conclusion": "success",
    "updated_at": "2026-10-31T12:01:00Z", "app": {"id": %d}, "pull_requests": [{"number": 42}]},
  "repository": {"id": 123456, "name": "smyklot", "full_name": "smykla-skalski/smyklot", "owner": {"login": "smykla-skalski"}},
  "installation": {"id": 987}
}`, subject, run.id, run.name, run.headSHA, run.appID)
}

func scenarioRequestedAction(run scenarioCheckRun, action, actor string) []byte {
	return fmt.Appendf(nil, `{
  "action": "requested_action",
  "check_run": {"id": %d, "name": %q, "external_id": %q, "head_sha": %q, "app": {"id": %d}, "pull_requests": [{"number": 42}]},
  "requested_action": {"identifier": %q}, "sender": {"login": %q},
  "repository": {"id": 123456, "name": "smyklot", "full_name": "smykla-skalski/smyklot", "owner": {"login": "smykla-skalski"}},
  "installation": {"id": 987}
}`, run.id, run.name, run.externalID, run.headSHA, run.appID, action, actor)
}

// scenarioCheckRuns is the check-runs API: what Smyklot writes on the pull
// request, what CI writes, and what both read back by head.
type scenarioCheckRuns struct {
	mu     sync.Mutex
	runs   map[int64]scenarioCheckRun
	nextID int64
	merged bool
}

type scenarioCheckRun struct {
	id         int64
	name       string
	headSHA    string
	externalID string
	appID      int64
	status     string
	conclusion string
	actions    []string
}

func (c *scenarioCheckRuns) add(run scenarioCheckRun) scenarioCheckRun {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	run.id = c.nextID
	c.runs[run.id] = run

	return run
}

func (c *scenarioCheckRuns) smyklotRun(head string) (scenarioCheckRun, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, run := range c.runs {
		if run.headSHA == head && run.appID == scenarioAppID && run.name == storage.PendingCICheckName {
			return run, true
		}
	}

	return scenarioCheckRun{}, false
}

func (c *scenarioCheckRuns) get(id int64) (scenarioCheckRun, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	run, found := c.runs[id]

	return run, found
}

func (c *scenarioCheckRuns) markMerged() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.merged = true
}

func (c *scenarioCheckRuns) wasMerged() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.merged
}

func (c *scenarioCheckRuns) write(w http.ResponseWriter, r *http.Request, id int64) {
	var body struct {
		Name       string `json:"name"`
		HeadSHA    string `json:"head_sha"`
		ExternalID string `json:"external_id"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		Actions    []struct {
			Identifier string `json:"identifier"`
		} `json:"actions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}
	c.mu.Lock()
	run, found := c.runs[id]
	if !found {
		c.nextID++
		run = scenarioCheckRun{id: c.nextID, headSHA: body.HeadSHA, appID: scenarioAppID}
	}
	run.name, run.externalID, run.status, run.conclusion = body.Name, body.ExternalID, body.Status, body.Conclusion
	run.actions = nil
	for _, action := range body.Actions {
		run.actions = append(run.actions, action.Identifier)
	}
	c.runs[run.id] = run
	c.mu.Unlock()
	if !found {
		w.WriteHeader(http.StatusCreated)
	}
	_, _ = w.Write(run.json())
}

func (c *scenarioCheckRuns) list(w http.ResponseWriter, head string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	items := make([]string, 0, len(c.runs))
	for _, run := range c.runs {
		if run.headSHA == head {
			items = append(items, string(run.json()))
		}
	}
	_, _ = fmt.Fprintf(w, `{"total_count":%d,"check_runs":[%s]}`, len(items), strings.Join(items, ","))
}

func (run scenarioCheckRun) json() []byte {
	return fmt.Appendf(nil,
		`{"id":%d,"name":%q,"head_sha":%q,"external_id":%q,"status":%q,"conclusion":%q,`+
			`"html_url":"https://github.example/checks/%d","app":{"id":%d}}`,
		run.id, run.name, run.headSHA, run.externalID, run.status, run.conclusion, run.id, run.appID,
	)
}

// scenarioProvider is the GitHub the scenario runs against: a protected main
// branch that requires the Smyklot check, the check-runs API, and a merge
// endpoint that merges.
func scenarioProvider(stub *githubStub, checks *scenarioCheckRuns) http.Handler {
	noteCall := func(r *http.Request) {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		stub.calls = append(stub.calls, r.Method+" "+r.URL.Path)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		var body string
		switch {
		case path == "/graphql":
			body = `{"data":{"repository":{"mergeQueue":null}}}`
		case strings.HasSuffix(path, "/protection/required_status_checks"):
			body = fmt.Sprintf(`{"checks":[{"context":%q,"app_id":%d}]}`, storage.PendingCICheckName, scenarioAppID)
		case strings.HasSuffix(path, "/rules/branches/main"):
			body = `[]`
		case strings.HasSuffix(path, "/issues/42/events"):
			body = `[]`
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/check-runs"):
			noteCall(r)
			checks.write(w, r, 0)

			return
		case strings.Contains(path, "/check-runs/"):
			noteCall(r)
			id, err := strconv.ParseInt(path[strings.LastIndex(path, "/")+1:], 10, 64)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)

				return
			}
			if r.Method == http.MethodPatch {
				checks.write(w, r, id)

				return
			}
			run, found := checks.get(id)
			if !found {
				w.WriteHeader(http.StatusNotFound)

				return
			}
			body = string(run.json())
		case strings.Contains(path, "/commits/") && strings.HasSuffix(path, "/check-runs"):
			noteCall(r)
			head := strings.TrimSuffix(path[strings.Index(path, "/commits/")+len("/commits/"):], "/check-runs")
			checks.list(w, head)

			return
		case r.Method == http.MethodPut && strings.HasSuffix(path, "/pulls/42/merge"):
			checks.markMerged()
			body = `{"merged":true,"sha":"merge-sha","message":"Pull Request successfully merged"}`
		default:
			stub.ServeHTTP(w, r)

			return
		}
		noteCall(r)
		_, _ = w.Write([]byte(body))
	})
}
