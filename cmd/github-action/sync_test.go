package main

import (
	"encoding/json"
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
		// With the permission label sync needs, which GitHub always reports:
		// the field is required on the installation object, so a listing
		// without it is a malformed answer rather than a real installation.
		stub.installations = `[{"id":411,"account":` +
			`{"id":7,"login":"smykla-skalski","type":"Organization"},` +
			`"permissions":{"issues":"write"}}]`
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

	configureKind := func(
		target storage.Target, kind orgsync.Kind, document string,
	) orgsync.Config {
		GinkgoHelper()

		config, err := service.store.SetSyncConfig(
			GinkgoT().Context(), orgsync.ConfigChange{
				TargetID: target.ID, Kind: kind, Enabled: true,
				Document: []byte(document), ActorID: target.Account.ID,
				Now: time.Now().UTC(),
			})
		Expect(err).NotTo(HaveOccurred())

		return config
	}

	configure := func(target storage.Target, document string) orgsync.Config {
		GinkgoHelper()

		return configureKind(target, orgsync.KindLabels, document)
	}

	client := func() *github.Client {
		GinkgoHelper()

		client, err := github.NewClient("installation-token", endpoint.URL)
		Expect(err).NotTo(HaveOccurred())

		return client
	}

	// granting re-lists the installation with the permissions given and
	// reconciles, which is how a grant or a revocation actually reaches the
	// service: GitHub reports it and the sweep stores it.
	granting := func(permissions string) storage.Target {
		GinkgoHelper()

		stub.installations = `[{"id":411,"account":` +
			`{"id":7,"login":"smykla-skalski","type":"Organization"},` +
			`"permissions":` + permissions + `}]`

		return seed()
	}

	// override is a repository's own answer for one kind, which is the layer a
	// person uses to leave one repository out.
	override := func(target storage.Target, kind orgsync.Kind, enabled bool) {
		GinkgoHelper()

		repositories, err := service.store.ListRepositories(GinkgoT().Context(), target.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(repositories).NotTo(BeEmpty())

		_, err = service.store.SetSyncRepositoryOverride(
			GinkgoT().Context(), orgsync.RepositoryOverrideChange{
				RepositoryID: repositories[0].ID, Kind: kind, Enabled: &enabled,
				ActorID: target.Account.ID, Now: time.Now().UTC(),
			})
		Expect(err).NotTo(HaveOccurred())
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

		// An installation that has not approved a newly requested permission is
		// the ordinary state during a rollout, not a fault. Planning anyway
		// would produce a plan whose every action 403s, once per repository,
		// every tick - a history full of refusals that are really one question
		// nobody has been asked
		It("plans nothing for a kind the installation has not permitted", func() {
			target := granting(`{"issues":"read"}`)
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))

			// And it asked GitHub nothing, rather than finding out by refusal
			Expect(stub.countCalls(
				http.MethodGet, "/repos/smykla-skalski/smyklot/labels")).To(BeZero())
		})

		// Not a level GitHub returns for the permissions Smyklot reads, but one
		// it returns elsewhere - so this is what stops a permission added here
		// later being read as refused
		It("plans for an installation that granted admin", func() {
			target := granting(`{"issues":"admin"}`)
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)

			plan(target)

			_, actions := livePlan(target)
			Expect(actions).To(HaveLen(1))
		})

		// GitHub marks the permissions field required on the installation
		// object, so an answer without it is malformed rather than an
		// installation that granted nothing. Proceeding would mean writing to
		// somebody's repositories on an answer that could not be read, and a
		// 403 is the smaller problem
		It("plans nothing for an installation whose permissions could not be read", func() {
			target := granting(`{}`)
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
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

		// Each kind settles on its own, against its own digest and its own
		// record. Reading another kind's rows here answers the question with
		// the wrong kind's answer, and a settings sync that never settles asks
		// GitHub about every repository on every tick for ever
		It("stops asking GitHub about a repository whose settings already match", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			stub.repoSettings = `{"has_wiki":true}`
			configureKind(target, orgsync.KindSettings, `{"has_wiki":true}`)

			plan(target)

			reads := stub.countCalls(http.MethodGet, "/repos/smykla-skalski/smyklot")
			Expect(reads).NotTo(BeZero())

			plan(target)
			Expect(stub.countCalls(http.MethodGet, "/repos/smykla-skalski/smyklot")).
				To(Equal(reads))
		})

		// The record says what a repository looked like when it was read, which
		// is a fact about the past. Nothing on GitHub stops somebody renaming a
		// label by hand afterwards, and a record with no horizon means the one
		// thing a reconcile exists to correct is the one thing it cannot see
		It("looks again at a repository it has not read for a while", func() {
			target := seed()
			stub.repoLabels = `[{"name":"bug","color":"d73a4a","description":""}]`
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)

			plan(target)
			reads := stub.countCalls(
				http.MethodGet, "/repos/smykla-skalski/smyklot/labels")
			Expect(reads).NotTo(BeZero())

			// The same record, written as though the read behind it were older
			// than the horizon. Nothing else changes: the configuration is the
			// same and so is its digest.
			settled, err := service.store.ListSyncRepositoryState(
				GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(settled).To(HaveLen(1))
			settled[0].AppliedAt = time.Now().UTC().Add(-syncRecheckInterval - time.Minute)
			Expect(service.store.RecordSyncRepositoryState(
				GinkgoT().Context(), settled)).To(Succeed())

			// Somebody has been at it by hand in the meantime
			stub.repoLabels = `[{"name":"bug","color":"ffffff","description":""}]`

			plan(target)

			Expect(stub.countCalls(
				http.MethodGet, "/repos/smykla-skalski/smyklot/labels")).
				To(BeNumerically(">", reads))

			_, actions := livePlan(target)
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationUpdate))
			Expect(actions[0].Subject).To(Equal("bug"))
		})

		// A repository decides each kind on its own: somebody may want their
		// labels left alone and their settings kept in step
		It("leaves out a repository that turned this kind off", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			stub.repoSettings = `{"has_wiki":true}`
			configureKind(target, orgsync.KindSettings, `{"has_wiki":false}`)
			override(target, orgsync.KindSettings, false)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		// And the other direction, which is the one that silently does too
		// little: turning labels off for a repository must not take its
		// settings with it
		It("keeps syncing the kinds a repository did not turn off", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			stub.repoSettings = `{"has_wiki":true}`
			configureKind(target, orgsync.KindSettings, `{"has_wiki":false}`)
			override(target, orgsync.KindLabels, false)

			plan(target)

			_, actions := livePlan(target)
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Kind).To(Equal(orgsync.KindSettings))
		})

		// The panel refuses this at the keyboard, but a row written before a
		// rule existed - or by a hand on the database - reaches the planner
		// anyway, and a plan holding work GitHub is going to refuse asks
		// somebody to approve a promise it cannot keep
		It("plans nothing from a stored document GitHub would refuse", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			stub.repoSettings = `{"has_wiki":true}`
			configureKind(target, orgsync.KindSettings,
				`{"has_wiki":false,"merge_commit_title":"NONSENSE"}`)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))

			// And it asked GitHub nothing, rather than finding out by refusal
			Expect(stub.countCalls(http.MethodGet, "/repos/smykla-skalski/smyklot")).
				To(BeZero())
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

		// A plan is approved by a person and applied later, and a permission can
		// be revoked in between - that is what revoking one is for. Without a
		// second check the plan's every action would be refused one at a time,
		// and the revocation would read as a repository that failed rather than
		// as a decision somebody made
		It("refuses to apply a plan whose permission was revoked", func() {
			target := seed()
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)
			plan(target)
			computed, _ := livePlan(target)
			approve(computed)

			// The installation withdraws the permission, which the next catalog
			// reconcile records.
			stub.installations = `[{"id":411,"account":` +
				`{"id":7,"login":"smykla-skalski","type":"Organization"},` +
				`"permissions":{"issues":"read"}}]`
			_, err := service.SyncCatalog(GinkgoT().Context())
			Expect(err).NotTo(HaveOccurred())

			Expect(service.applySyncPlans(GinkgoT().Context())).To(HaveOccurred())

			// Nothing was written to GitHub, and the plan keeps its lease rather
			// than being closed: granting the permission back is all it needs
			Expect(stub.labelWrites).To(BeEmpty())
			held, _, err := service.store.GetSyncPlan(
				GinkgoT().Context(), target.ID, computed.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(held.State).To(Equal(orgsync.PlanApplying))
		})

		It("changes the settings the plan named, and only those", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			stub.repoSettings = `{"has_wiki": true, "has_issues": true,
				"delete_branch_on_merge": false, "allow_squash_merge": true}`
			configureKind(target, orgsync.KindSettings,
				`{"has_wiki":false,"delete_branch_on_merge":true}`)

			plan(target)
			computed, actions := livePlan(target)
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Kind).To(Equal(orgsync.KindSettings))
			approve(computed)

			Expect(service.applySyncPlans(GinkgoT().Context())).To(Succeed())

			Expect(stub.settingsWrites).To(HaveLen(1))

			var sent map[string]any
			Expect(json.Unmarshal([]byte(stub.settingsWrites[0]), &sent)).To(Succeed())
			Expect(sent).To(HaveKeyWithValue("has_wiki", false))
			Expect(sent).To(HaveKeyWithValue("delete_branch_on_merge", true))

			// Nothing about the settings nobody configured. Against an endpoint
			// that replaces what it is sent, writing those back is how a sync
			// undoes somebody else's change
			Expect(sent).NotTo(HaveKey("has_issues"))
			Expect(sent).NotTo(HaveKey("allow_squash_merge"))
			Expect(sent).To(HaveLen(2))
		})

		// The security features travel nested, with a status string, and a
		// feature the repository does not have is absent from GitHub's answer
		// rather than reported off - so this is the one place the whole shape
		// is proved against something that parses it back
		It("switches on the security features, and leaves the missing one", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			stub.repoSettings = `{"has_wiki":true,"security_and_analysis":{
				"secret_scanning":{"status":"disabled"},
				"secret_scanning_push_protection":{"status":"disabled"}}}`
			configureKind(target, orgsync.KindSettings,
				`{"advanced_security":true,"secret_scanning":true,`+
					`"secret_scanning_push_protection":true}`)

			plan(target)
			computed, actions := livePlan(target)
			Expect(actions).To(HaveLen(1))
			approve(computed)

			Expect(service.applySyncPlans(GinkgoT().Context())).To(Succeed())

			Expect(stub.settingsWrites).To(HaveLen(1))

			var sent map[string]any
			Expect(json.Unmarshal([]byte(stub.settingsWrites[0]), &sent)).To(Succeed())
			Expect(sent).To(HaveKeyWithValue("security_and_analysis", map[string]any{
				"secret_scanning":                 map[string]any{"status": "enabled"},
				"secret_scanning_push_protection": map[string]any{"status": "enabled"},
			}))

			// Advanced security was never mentioned by GitHub, so this
			// repository does not have it - and asking for it would have been a
			// 422 that took the two features beside it down as well
			Expect(actions[0].After).To(ContainSubstring("advanced_security"))
		})

		// An installation may have approved one kind and not another, and the
		// one it approved should still run
		It("plans the permitted kind and leaves the other", func() {
			target := granting(`{"issues":"write"}`)
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)
			configureKind(target, orgsync.KindSettings, `{"has_wiki":false}`)
			stub.repoSettings = `{"has_wiki": true}`

			plan(target)

			_, actions := livePlan(target)
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Kind).To(Equal(orgsync.KindLabels))
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
