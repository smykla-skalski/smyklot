package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("Label sync [Unit]", func() {
	var (
		service  *server
		stub     *githubStub
		endpoint *httptest.Server
	)

	BeforeEach(func() {
		stub = newGitHubStub()
		stub.installations = `[{"id":411,"account":` +
			`{"id":7,"login":"smykla-skalski","type":"Organization"}}]`
		stub.repos = `{"repositories":[{"id":41,"name":"smyklot",` +
			`"full_name":"smykla-skalski/smyklot","default_branch":"main",` +
			`"owner":{"login":"smykla-skalski"}}]}`
		endpoint = httptest.NewServer(stub)
		DeferCleanup(endpoint.Close)

		var err error
		service, err = newServer(&serveConfig{
			database:      GinkgoT().TempDir() + "/panel.sqlite3",
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

	// seed puts the installation in the catalog and returns its target.
	seed := func() storage.Target {
		GinkgoHelper()

		targetIDs, err := service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())
		Expect(targetIDs).To(HaveLen(1))

		target, err := service.store.GetTarget(GinkgoT().Context(), targetIDs[0])
		Expect(err).NotTo(HaveOccurred())

		return target
	}

	configure := func(target storage.Target, document string) orgsync.Config {
		GinkgoHelper()

		config, err := service.store.SetSyncConfig(
			GinkgoT().Context(), orgsync.ConfigChange{
				TargetID: target.ID, Kind: orgsync.KindLabels, Enabled: true,
				Document: []byte(document), ActorID: target.Account.ID,
				Now: time.Now().UTC(),
			})
		Expect(err).NotTo(HaveOccurred())

		return config
	}

	client := func() *github.Client {
		GinkgoHelper()

		client, err := github.NewClient("installation-token", endpoint.URL)
		Expect(err).NotTo(HaveOccurred())

		return client
	}

	plan := func(target storage.Target) {
		GinkgoHelper()

		Expect(service.planInstallationSync(
			GinkgoT().Context(), client(), target.ID, orgsync.TriggerReconcile,
		)).To(Succeed())
	}

	livePlan := func(target storage.Target) (orgsync.Plan, []orgsync.Action) {
		GinkgoHelper()

		plan, actions, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
		Expect(err).NotTo(HaveOccurred())

		return plan, actions
	}

	approve := func(plan orgsync.Plan) {
		GinkgoHelper()

		_, err := service.store.ApproveSyncPlan(GinkgoT().Context(), orgsync.PlanApproval{
			TargetID: plan.TargetID, PlanID: plan.ID, Digest: plan.Digest,
			ActorID: plan.ActorAccountID, Now: time.Now().UTC(),
		})
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("planning", func() {
		It("proposes the labels a repository is missing", func() {
			target := seed()
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)

			plan(target)

			computed, actions := livePlan(target)
			Expect(computed.Trigger).To(Equal(orgsync.TriggerReconcile))
			Expect(computed.Counts).To(Equal(orgsync.Counts{Create: 1}))
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Subject).To(Equal("bug"))
		})

		// Nothing switched on means nothing to compare against, and a plan
		// nobody asked for would hold the installation's one live slot
		It("plans nothing when sync is switched off", func() {
			target := seed()
			_, err := service.store.SetSyncConfig(
				GinkgoT().Context(), orgsync.ConfigChange{
					TargetID: target.ID, Kind: orgsync.KindLabels, Enabled: false,
					Document: []byte(`{"labels":[{"name":"bug","color":"d73a4a"}]}`),
					ActorID:  target.Account.ID, Now: time.Now().UTC(),
				})
			Expect(err).NotTo(HaveOccurred())

			plan(target)

			_, _, err = service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		It("plans nothing when the repository already matches", func() {
			target := seed()
			stub.repoLabels = `[{"name":"bug","color":"d73a4a","description":""}]`
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		// A repository that already matches appears in no plan, so an apply
		// would never record it - and without a record it is read from GitHub
		// again on every tick for ever, which is the whole cost the recorded
		// digest exists to remove. Reading its labels and finding no work is
		// the proof, so that is when it is written down
		It("stops asking GitHub about a repository that already matches", func() {
			target := seed()
			stub.repoLabels = `[{"name":"bug","color":"d73a4a","description":""}]`
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)

			plan(target)

			state, err := service.store.ListSyncRepositoryState(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(HaveLen(1))

			// And the next tick asks GitHub nothing at all about it
			reads := stub.countCalls(http.MethodGet, "/repos/smykla-skalski/smyklot/labels")
			plan(target)
			Expect(stub.countCalls(http.MethodGet, "/repos/smykla-skalski/smyklot/labels")).
				To(Equal(reads))
		})

		// A reconcile that changed nothing is not an event, and one row a tick
		// would be about a hundred and seventy-five thousand a year
		It("writes no audit entry for a plan with nothing in it", func() {
			target := seed()
			stub.repoLabels = `[{"name":"bug","color":"d73a4a","description":""}]`
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)

			plan(target)

			Expect(syncAuditActions(service, target)).To(BeEmpty())
		})

		It("records what a plan would do when there is something to do", func() {
			target := seed()
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)

			plan(target)

			Expect(syncAuditActions(service, target)).
				To(ConsistOf(string(orgsync.AuditPlanned)))
		})

		// An exclusion has to survive the round trip through storage and reach
		// the planner. It did not: the stored document had one shape and the
		// planner decoded another, so every exclusion somebody configured was
		// dropped on the way in and the planner ran with none
		It("leaves an excluded label alone", func() {
			target := seed()
			configure(target, `{"labels":[{"name":"ci/lint","color":"d73a4a"}],`+
				`"excludes":["ci/*"]}`)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		// And the dangerous direction: with removal switched on, an exclusion
		// that did not reach the planner means a label somebody protected by
		// hand is deleted
		It("does not remove an excluded label", func() {
			target := seed()
			stub.repoLabels = `[{"name":"ci/lint","color":"ffffff","description":""}]`
			configure(target, `{"labels":[],"allow_removal":true,"excludes":["ci/*"]}`)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		// Pressing "sync now" beside a running reconcile has to be harmless,
		// which is what the one-live-plan index buys
		It("leaves a plan already in flight alone", func() {
			target := seed()
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)

			plan(target)
			first, _ := livePlan(target)

			plan(target)
			second, _ := livePlan(target)

			Expect(second.ID).To(Equal(first.ID))
		})
	})

	Describe("applying", func() {
		It("creates the label the plan named", func() {
			target := seed()
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)
			plan(target)
			computed, _ := livePlan(target)
			approve(computed)

			Expect(service.applySyncPlans(GinkgoT().Context())).To(Succeed())

			Expect(stub.labelWrites).To(HaveLen(1))
			Expect(stub.labelWrites[0]).To(HavePrefix("POST /repos/smykla-skalski/smyklot/labels"))
			Expect(stub.labelWrites[0]).To(ContainSubstring(`"name":"bug"`))
			Expect(stub.labelWrites[0]).To(ContainSubstring(`"color":"d73a4a"`))

			applied, _, err := service.store.GetSyncPlan(
				GinkgoT().Context(), target.ID, computed.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied.State).To(Equal(orgsync.PlanApplied))
		})

		// The digest is what stops the next reconcile asking GitHub about a
		// repository that already has what the configuration says
		It("records what the repository now has, so the next tick costs nothing", func() {
			target := seed()
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)
			plan(target)
			computed, _ := livePlan(target)
			approve(computed)
			Expect(service.applySyncPlans(GinkgoT().Context())).To(Succeed())

			state, err := service.store.ListSyncRepositoryState(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(HaveLen(1))

			// And a second reconcile proposes nothing, without the repository
			// having changed on GitHub
			writes := len(stub.labelWrites)
			plan(target)
			_, _, err = service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
			Expect(stub.labelWrites).To(HaveLen(writes))
		})

		It("records that the plan finished", func() {
			target := seed()
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)
			plan(target)
			computed, _ := livePlan(target)
			approve(computed)
			Expect(service.applySyncPlans(GinkgoT().Context())).To(Succeed())

			Expect(syncAuditActions(service, target)).
				To(ContainElement(string(orgsync.AuditFinished)))
		})

		// Deletion is off by default and destroys something somebody may have
		// made by hand, so it is never the part nobody was told about
		It("records a removal on its own", func() {
			target := seed()
			stub.repoLabels = `[{"name":"wontfix","color":"ffffff","description":""}]`
			configure(target, `{"labels":[],"allow_removal":true}`)
			plan(target)
			computed, _ := livePlan(target)
			Expect(computed.Counts).To(Equal(orgsync.Counts{Delete: 1}))
			approve(computed)

			Expect(service.applySyncPlans(GinkgoT().Context())).To(Succeed())

			Expect(stub.labelWrites).To(HaveLen(1))
			Expect(stub.labelWrites[0]).To(HavePrefix("DELETE "))
			Expect(syncAuditActions(service, target)).
				To(ContainElement(string(orgsync.AuditDeleted)))
		})

		// A failure is not the end of it. The plan closes failed, nothing is
		// recorded as applied, and the next reconcile computes the work again -
		// which is what makes a transient refusal from GitHub recoverable
		// without anybody doing anything
		It("plans the work again after a plan failed", func() {
			target := seed()
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)
			plan(target)
			computed, _ := livePlan(target)
			approve(computed)

			stub.brokenRepo = "smyklot"
			Expect(service.applySyncPlans(GinkgoT().Context())).To(Succeed())

			failed, _, err := service.store.GetSyncPlan(
				GinkgoT().Context(), target.ID, computed.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(failed.State).To(Equal(orgsync.PlanFailed))

			// GitHub answers again, and the next reconcile proposes the same
			// work rather than believing it was done
			stub.brokenRepo = ""
			plan(target)
			again, actions := livePlan(target)
			Expect(again.ID).NotTo(Equal(computed.ID))
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Subject).To(Equal("bug"))
		})

		It("does nothing for a plan nobody approved", func() {
			target := seed()
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)
			plan(target)

			Expect(service.applySyncPlans(GinkgoT().Context())).To(Succeed())
			Expect(stub.labelWrites).To(BeEmpty())
		})

		// A failure has to be recorded against the action rather than reported
		// as a success, which is the whole of what the tool this replaces got
		// wrong: it set its counts from the diff and printed them either way
		It("records the action that failed, and fails the plan", func() {
			target := seed()
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)
			plan(target)
			computed, _ := livePlan(target)
			approve(computed)

			stub.brokenRepo = "smyklot"

			Expect(service.applySyncPlans(GinkgoT().Context())).To(Succeed())

			applied, actions, err := service.store.GetSyncPlan(
				GinkgoT().Context(), target.ID, computed.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied.State).To(Equal(orgsync.PlanFailed))
			Expect(actions[0].State).To(Equal(orgsync.ActionFailed))
			Expect(actions[0].Error).NotTo(BeEmpty())

			// And nothing is recorded as applied, so the next reconcile tries
			// again rather than believing the work is done
			state, err := service.store.ListSyncRepositoryState(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(BeEmpty())
		})
	})

	// Saving while a plan is on screen has to invalidate it, or somebody can
	// approve work they never saw
	It("retires a plan when the configuration changes underneath it", func() {
		target := seed()
		saved := configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)
		plan(target)
		computed, _ := livePlan(target)

		_, err := service.store.SetSyncConfig(GinkgoT().Context(), orgsync.ConfigChange{
			TargetID: target.ID, Kind: orgsync.KindLabels, Enabled: true,
			Document: []byte(`{"labels":[{"name":"bug","color":"000000"}]}`),
			ActorID:  target.Account.ID, Now: time.Now().UTC(), Revision: saved.Revision,
		})
		Expect(err).NotTo(HaveOccurred())

		stale, _, err := service.store.GetSyncPlan(GinkgoT().Context(), target.ID, computed.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stale.State).To(Equal(orgsync.PlanStale))

		// And approving it is refused, rather than applying what nobody read
		_, err = service.store.ApproveSyncPlan(GinkgoT().Context(), orgsync.PlanApproval{
			TargetID: target.ID, PlanID: computed.ID, Digest: computed.Digest,
			ActorID: target.Account.ID, Now: time.Now().UTC(),
		})
		Expect(err).To(HaveOccurred())
	})
})

// syncAuditActions reads which sync events an installation recorded.
func syncAuditActions(service *server, target storage.Target) []string {
	GinkgoHelper()

	page, err := service.store.ListRootAudit(GinkgoT().Context(), storage.RootAuditPageRequest{
		HistoryPageRequest: storage.HistoryPageRequest{Limit: 50},
		Categories:         []storage.AuditCategory{storage.AuditCategorySync},
		TargetID:           &target.ID,
	})
	Expect(err).NotTo(HaveOccurred())

	actions := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		actions = append(actions, item.Action)
	}

	return actions
}
