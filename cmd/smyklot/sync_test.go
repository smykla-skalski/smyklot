package main

import (
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
	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/orgsync/apply"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("Org sync [Unit]", func() {
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
			botUsername:   bot.DefaultBotUsername,
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

		Expect(service.sync.PlanInstallation(
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
			settled[0].AppliedAt = time.Now().UTC().Add(-apply.RecheckInterval - time.Minute)
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

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

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
			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

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
			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

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

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

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
			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

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

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(HaveOccurred())

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

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

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

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

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

		// Reported inside security_and_analysis with the rest and changed
		// through an endpoint of its own, so it is a second action on the same
		// kind. Proved end to end because this is the one place both halves are
		// exercised: a settings body carrying the key would be ignored by
		// GitHub and recorded here as applied
		It("switches Dependabot security updates through their own endpoint", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			stub.repoSettings = `{"has_wiki":true,"security_and_analysis":{
				"dependabot_security_updates":{"status":"disabled"}}}`
			configureKind(target, orgsync.KindSettings,
				`{"has_wiki":false,"dependabot_security_updates":true}`)

			plan(target)
			computed, actions := livePlan(target)
			Expect(actions).To(HaveLen(2))
			approve(computed)

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			// The settings request carries the setting the endpoint takes, and
			// nothing it does not
			Expect(stub.settingsWrites).To(HaveLen(1))

			var sent map[string]any
			Expect(json.Unmarshal([]byte(stub.settingsWrites[0]), &sent)).To(Succeed())
			Expect(sent).To(HaveKeyWithValue("has_wiki", false))
			Expect(sent).To(HaveLen(1))

			Expect(stub.dependabotWrites).To(Equal([]string{http.MethodPut}))
		})

		// A repository GitHub says nothing about cannot be given the feature,
		// and the plan leaves it alone rather than proposing a request that can
		// only be refused on this sweep and on every one after it
		It("leaves Dependabot alone where the repository does not report it", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			stub.repoSettings = `{"has_wiki":true}`
			configureKind(target, orgsync.KindSettings,
				`{"has_wiki":false,"dependabot_security_updates":true}`)

			plan(target)
			computed, actions := livePlan(target)
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Subject).To(Equal(orgsync.SettingsSubject))
			approve(computed)

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())
			Expect(stub.dependabotWrites).To(BeEmpty())
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

		// The whole way through, because the two switch arms this adds are the
		// only thing between a configured ruleset and nothing happening at all.
		// A kind whose planner is wired and whose executor is not reports a plan
		// applied that nothing performed
		It("creates the ruleset the plan named", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			configureKind(target, orgsync.KindRulesets, `{"rulesets":[
				{"name":"main-branch-protection","target":"branch",
				 "enforcement":"active",
				 "conditions":{"include":["refs/heads/main"]},
				 "rules":{"deletion":true,"non_fast_forward":true}}]}`)

			plan(target)
			computed, actions := livePlan(target)
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Kind).To(Equal(orgsync.KindRulesets))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationCreate))
			approve(computed)

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			Expect(stub.rulesetWrites).To(HaveLen(1))
			Expect(stub.rulesetWrites[0]).To(
				HavePrefix("POST /repos/smykla-skalski/smyklot/rulesets "))
			Expect(stub.rulesetWrites[0]).To(ContainSubstring(`"name":"main-branch-protection"`))
			Expect(stub.rulesetWrites[0]).To(ContainSubstring(`"type":"deletion"`))
		})

		// The listing carries no rules, so a planner that compared against it
		// would find every rule missing and rewrite a matching repository on
		// every tick for ever
		It("reads a ruleset whole before deciding it has drifted", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			stub.repoRulesets = `[{"id":7,"name":"main-branch-protection",
				"target":"branch","enforcement":"active","source_type":"Repository"}]`
			stub.rulesetBodies = map[int64]string{7: `{"id":7,
				"name":"main-branch-protection","target":"branch","enforcement":"active",
				"conditions":{"ref_name":{"include":["refs/heads/main"],"exclude":[]}},
				"rules":[{"type":"deletion"}]}`}
			configureKind(target, orgsync.KindRulesets, `{"rulesets":[
				{"name":"main-branch-protection","target":"branch",
				 "enforcement":"active",
				 "conditions":{"include":["refs/heads/main"]},
				 "rules":{"deletion":true}}]}`)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))

			// And the next tick asks GitHub nothing at all about it
			reads := stub.countCalls(http.MethodGet, "/repos/smykla-skalski/smyklot/rulesets")
			plan(target)
			Expect(stub.countCalls(http.MethodGet, "/repos/smykla-skalski/smyklot/rulesets")).
				To(Equal(reads))
		})

		// The whole way through, because a domain field the wiring never fills
		// is a plan that reads exactly as it would if the rule were not there.
		// Everything else about this ruleset matches, so nothing but the
		// unreadable rule puts it in a plan at all
		It("says what a replacement drops that this version cannot express", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			stub.repoRulesets = `[{"id":7,"name":"main-branch-protection",
				"target":"branch","enforcement":"active","source_type":"Repository"}]`
			stub.rulesetBodies = map[int64]string{7: `{"id":7,
				"name":"main-branch-protection","target":"branch","enforcement":"active",
				"conditions":{"ref_name":{"include":["refs/heads/main"],"exclude":[]}},
				"rules":[{"type":"deletion"},
				         {"type":"commit_message_pattern",
				          "parameters":{"operator":"starts_with","pattern":"feat"}}]}`}
			configureKind(target, orgsync.KindRulesets, `{"rulesets":[
				{"name":"main-branch-protection","target":"branch",
				 "enforcement":"active",
				 "conditions":{"include":["refs/heads/main"]},
				 "rules":{"deletion":true}}]}`)

			plan(target)

			_, actions := livePlan(target)
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationUpdate))
			Expect(actions[0].After).To(ContainSubstring("this drops commit_message_pattern"))
		})

		// A ruleset nothing can address produces no action, and an empty answer
		// is what a repository that already matches produces too. Recorded as
		// settled it would look finished and be left alone for six hours, so
		// the one repository nothing manages would be the one nothing looks at
		It("never records a repository holding two of a name as settled", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			stub.repoRulesets = `[
				{"id":7,"name":"main-branch-protection","target":"branch",
				 "enforcement":"active","source_type":"Repository"},
				{"id":8,"name":"main-branch-protection","target":"branch",
				 "enforcement":"active","source_type":"Repository"}]`

			// Both readable, and both already matching. Without them the read
			// fails and this spec passes on the error path instead, proving
			// nothing about the ambiguity it is named for
			whole := `{"id":%d,"name":"main-branch-protection","target":"branch",
				"enforcement":"active",
				"conditions":{"ref_name":{"include":["refs/heads/main"],"exclude":[]}},
				"rules":[{"type":"deletion"}]}`
			stub.rulesetBodies = map[int64]string{
				7: fmt.Sprintf(whole, 7),
				8: fmt.Sprintf(whole, 8),
			}

			configureKind(target, orgsync.KindRulesets, `{"rulesets":[
				{"name":"main-branch-protection","target":"branch",
				 "enforcement":"active",
				 "conditions":{"include":["refs/heads/main"]},
				 "rules":{"deletion":true}}]}`)

			plan(target)

			// No plan, because there is no action anything could take
			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))

			// And what is recorded is why, with no digest, so the next sweep
			// reads it again rather than believing it matches
			state, err := service.store.ListSyncRepositoryState(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(HaveLen(1))
			Expect(state[0].AppliedDigest).To(BeEmpty())
			Expect(state[0].Problem).To(ContainSubstring(
				"more than one ruleset here carries a configured name"))

			reads := stub.countCalls(http.MethodGet, "/repos/smykla-skalski/smyklot/rulesets")
			plan(target)
			Expect(stub.countCalls(http.MethodGet, "/repos/smykla-skalski/smyklot/rulesets")).
				To(BeNumerically(">", reads))
		})

		// The same silence at the other end. One ruleset here is unresolvable
		// and another needs creating, so there is real work to plan - and a
		// kind whose every action applied is a kind the executor records as
		// settled, against the digest for the whole configuration. Acting on
		// the half it can address would mark the half it cannot as up to date
		It("plans nothing for a kind it cannot resolve, however much of it it can", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			stub.repoRulesets = `[
				{"id":7,"name":"main-branch-protection","target":"branch",
				 "enforcement":"active","source_type":"Repository"},
				{"id":8,"name":"main-branch-protection","target":"branch",
				 "enforcement":"active","source_type":"Repository"}]`

			whole := `{"id":%d,"name":"main-branch-protection","target":"branch",
				"enforcement":"active",
				"conditions":{"ref_name":{"include":["refs/heads/main"],"exclude":[]}},
				"rules":[{"type":"deletion"}]}`
			stub.rulesetBodies = map[int64]string{
				7: fmt.Sprintf(whole, 7),
				8: fmt.Sprintf(whole, 8),
			}

			// The second one is unambiguous and this repository has none of it
			configureKind(target, orgsync.KindRulesets, `{"rulesets":[
				{"name":"main-branch-protection","target":"branch",
				 "enforcement":"active",
				 "conditions":{"include":["refs/heads/main"]},
				 "rules":{"deletion":true}},
				{"name":"release-protection","target":"branch",
				 "enforcement":"active",
				 "conditions":{"include":["refs/heads/release/*"]},
				 "rules":{"non_fast_forward":true}}]}`)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))

			state, err := service.store.ListSyncRepositoryState(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(HaveLen(1))
			Expect(state[0].AppliedDigest).To(BeEmpty())
			Expect(state[0].Problem).NotTo(BeEmpty())
		})

		// The tool this replaces had no delete path at all, so a ruleset dropped
		// from configuration went on enforcing for ever. The id comes off the
		// plan rather than being looked up again, because by apply time the name
		// may belong to something else
		It("removes a ruleset configuration no longer names", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			stub.repoRulesets = `[{"id":9,"name":"old-protection",
				"target":"branch","enforcement":"active","source_type":"Repository"}]`
			configureKind(target, orgsync.KindRulesets,
				`{"rulesets":[],"allow_removal":true}`)

			plan(target)
			computed, actions := livePlan(target)
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationDelete))
			approve(computed)

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			Expect(stub.rulesetWrites).To(HaveLen(1))
			Expect(stub.rulesetWrites[0]).To(
				HavePrefix("DELETE /repos/smykla-skalski/smyklot/rulesets/9 "))
			Expect(syncAuditActions(service, target)).
				To(ContainElement(string(orgsync.AuditDeleted)))
		})

		// Not this repository's to change and not its to delete. Proposing it
		// would put work in a plan that could only fail
		It("leaves a ruleset the organization defines alone", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			stub.repoRulesets = `[{"id":99,"name":"org-wide",
				"target":"branch","enforcement":"active","source_type":"Organization"}]`
			configureKind(target, orgsync.KindRulesets,
				`{"rulesets":[],"allow_removal":true}`)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
			Expect(stub.rulesetWrites).To(BeEmpty())
		})

		// A plan holding work GitHub is going to refuse asks somebody to approve
		// a promise it cannot keep. The panel checks what somebody types; this
		// covers a row written before a rule existed
		It("plans nothing from a stored ruleset GitHub would refuse", func() {
			target := granting(`{"issues":"write","administration":"write"}`)
			configureKind(target, orgsync.KindRulesets, `{"rulesets":[
				{"name":"main","target":"branch","enforcement":"active",
				 "conditions":{"include":["refs/tags/v*"]}}]}`)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		It("does nothing for a plan nobody approved", func() {
			target := seed()
			configure(target, `{"labels":[{"name":"bug","color":"d73a4a"}]}`)
			plan(target)

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())
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

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

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

	Describe("files", func() {
		const contributing = `{"files":[` +
			`{"path":"CONTRIBUTING.md","content":"# Contributing\n"}]}`

		// Files need contents, which is a permission of its own.
		grantContents := func() storage.Target {
			GinkgoHelper()

			return granting(`{"issues":"write","contents":"write"}`)
		}

		// adjusting is what one repository changes about its files, which is
		// the layer the merge engine exists for.
		adjusting := func(target storage.Target, document string) {
			GinkgoHelper()

			repositories, err := service.store.ListRepositories(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())

			_, err = service.store.SetSyncRepositoryOverride(
				GinkgoT().Context(), orgsync.RepositoryOverrideChange{
					RepositoryID: repositories[0].ID, Kind: orgsync.KindFiles,
					Document: []byte(document),
					ActorID:  target.Account.ID, Now: time.Now().UTC(),
				})
			Expect(err).NotTo(HaveOccurred())
		}

		applied := func(target storage.Target) {
			GinkgoHelper()

			plan(target)
			computed, _ := livePlan(target)
			approve(computed)
			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())
		}

		It("proposes a file the repository does not have", func() {
			target := grantContents()
			configureKind(target, orgsync.KindFiles, contributing)

			plan(target)
			_, actions := livePlan(target)

			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Kind).To(Equal(orgsync.KindFiles))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationCreate))
			Expect(actions[0].Subject).To(Equal("CONTRIBUTING.md"))
		})

		// One request per repository. git names an object by hashing its
		// contents, so a listing of the tree answers whether every managed file
		// already says what it should - the tool this replaces downloaded each
		// of them from each repository on every run.
		It("leaves a repository whose files already match alone", func() {
			target := grantContents()
			stub.repoTree = fmt.Sprintf(
				`{"sha":"basetree","tree":[{"path":"CONTRIBUTING.md","type":"blob",`+
					`"mode":"100644","sha":%q,"size":16}],"truncated":false}`,
				orgsync.BlobID([]byte("# Contributing\n")))
			configureKind(target, orgsync.KindFiles, contributing)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		// The tree the commit is built from still has the retired path, so the
		// removal goes into it.
		holdingRenovaterc := `{"tree":[{"path":".renovaterc","type":"blob",` +
			`"mode":"100644","sha":"old","size":2}]}`

		It("puts every path into one commit behind one pull request", func() {
			target := grantContents()
			stub.repoTree = `{"sha":"basetree","tree":[{"path":".renovaterc",` +
				`"type":"blob","mode":"100644","sha":"old","size":2}],"truncated":false}`
			stub.repoLevels = map[string]string{"basetree": holdingRenovaterc}
			configureKind(target, orgsync.KindFiles, `{"files":[`+
				`{"path":"CONTRIBUTING.md","content":"# Contributing\n"},`+
				`{"path":"SECURITY.md","content":"# Security\n"}],`+
				`"retired":[".renovaterc"]}`)

			plan(target)
			_, actions := livePlan(target)
			Expect(actions).To(HaveLen(3))

			applied(target)

			Expect(stub.createdCommits).To(HaveLen(1))
			Expect(stub.createdTrees).To(HaveLen(1))

			// All three in the one tree, and the removal spelled the way git
			// spells one: an entry with no object
			Expect(stub.createdTrees[0]).To(ContainSubstring(`"CONTRIBUTING.md"`))
			Expect(stub.createdTrees[0]).To(ContainSubstring(`"SECURITY.md"`))
			Expect(stub.createdTrees[0]).To(ContainSubstring(`".renovaterc"`))
			Expect(stub.createdTrees[0]).To(ContainSubstring(`"sha":null`))

			Expect(stub.createdPRs).To(HaveLen(1))
			Expect(stub.createdPRs[0]).To(ContainSubstring("CONTRIBUTING.md"))
			Expect(stub.createdPRs[0]).To(ContainSubstring(".renovaterc"))
		})

		// The plan is computed against the default branch and the commit is
		// built on the proposal branch, which already carries what an earlier
		// tick put there. A tree entry removing a path that is not in the tree
		// it is built from is a 422 - and the proposal comes round again on the
		// reconcile horizon for as long as it sits unmerged, so this failed
		// every six hours rather than once.
		It("does not remove again what the branch has already removed", func() {
			target := grantContents()
			stub.repoTree = `{"sha":"basetree","tree":[{"path":".renovaterc",` +
				`"type":"blob","mode":"100644","sha":"old","size":2}],"truncated":false}`
			configureKind(target, orgsync.KindFiles,
				`{"files":[{"path":"CONTRIBUTING.md","content":"# Contributing\n"}],`+
					`"retired":[".renovaterc"]}`)

			plan(target)
			computed, actions := livePlan(target)
			approve(computed)

			// The branch is there with an earlier tick's commit, whose tree has
			// already taken the file out
			written, err := orgsync.DecodeFile(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			stub.branchRefs[written.Proposal] = "earliercommit"
			stub.migrationTipTree = "branchtree"
			stub.repoTrees = map[string]string{
				"branchtree": `{"tree":[],"truncated":false}`,
			}

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			Expect(stub.createdTrees).To(HaveLen(1))
			Expect(stub.createdTrees[0]).NotTo(ContainSubstring(".renovaterc"))
			Expect(stub.createdTrees[0]).To(ContainSubstring("CONTRIBUTING.md"))

			// The pull request still says the file goes, because the proposal
			// is what the branch does to the default branch rather than what
			// this one commit adds to it
			Expect(stub.createdPRs[0]).To(ContainSubstring(".renovaterc"))
		})

		// The same again with nothing else in the change, so every entry is
		// dropped and there is no tree left to build. GitHub documents the
		// entry list as required, so asking it to build a tree from none of
		// them either fails or hands back the tree it was given - and a
		// repository's proposal should not turn on which. The answer is known
		// without the request.
		It("asks for no tree where the branch already carries the whole change", func() {
			target := grantContents()
			stub.repoTree = `{"sha":"basetree","tree":[{"path":".renovaterc",` +
				`"type":"blob","mode":"100644","sha":"old","size":2}],"truncated":false}`
			configureKind(target, orgsync.KindFiles,
				`{"files":[],"retired":[".renovaterc"]}`)

			plan(target)
			computed, actions := livePlan(target)
			approve(computed)

			written, err := orgsync.DecodeFile(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			stub.branchRefs[written.Proposal] = "earliercommit"
			stub.migrationTipTree = "branchtree"
			stub.repoTrees = map[string]string{
				"branchtree": `{"tree":[],"truncated":false}`,
			}

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			Expect(stub.createdTrees).To(BeEmpty())
			Expect(stub.createdCommits).To(BeEmpty())

			// And the proposal is still kept current, because it is what the
			// branch does to the default branch rather than what one commit
			// added to it.
			Expect(stub.createdPRs).NotTo(BeEmpty())
		})

		// Nothing is ever force-pushed. The tool this replaces rebuilt the
		// branch from the default branch on every run and force-updated the
		// reference, so a reviewer's fixup was gone on the next sync with no
		// error and no trace.
		It("never asks GitHub to discard what is on the branch", func() {
			target := grantContents()
			configureKind(target, orgsync.KindFiles, contributing)

			applied(target)

			Expect(stub.forcedPushes).To(BeZero())
		})

		It("builds on the branch rather than rebuilding it", func() {
			target := grantContents()
			configureKind(target, orgsync.KindFiles, contributing)

			plan(target)
			computed, actions := livePlan(target)
			approve(computed)

			// The branch is already there, carrying somebody's commit
			written, err := orgsync.DecodeFile(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			stub.branchRefs[written.Proposal] = "humancommit"
			stub.migrationTipTree = "human-tree"
			stub.createdTreeSHA = "newtree"

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			// Built from the branch's own tree, so what is on it survives
			Expect(stub.createdTrees).To(HaveLen(1))
			Expect(stub.createdTrees[0]).To(ContainSubstring(`"base_tree":"human-tree"`))
			Expect(stub.createdCommits[0]).To(ContainSubstring(`"humancommit"`))
			Expect(stub.forcedPushes).To(BeZero())
		})

		// A proposal in front of a repository is a question already asked, and
		// settles it whichever way it went. Planning it again would produce the
		// same plan, needing the same approval, adopting or re-asking the same
		// pull request, once every horizon for as long as it sat there - and a
		// refusal that is not durable is a repository asked forever. The branch
		// is named after what the files should end up saying, so this answers
		// for this change and a configuration that moves asks again.
		DescribeTable("plans nothing more while a proposal is outstanding",
			func(pulls string) {
				target := grantContents()
				configureKind(target, orgsync.KindFiles, contributing)
				stub.branchPRs = pulls

				plan(target)

				_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
				Expect(err).To(MatchError(storage.ErrNotFound))

				// And it is written down as settled, so the next reconcile does
				// not read the repository again
				state, err := service.store.ListSyncRepositoryState(
					GinkgoT().Context(), target.ID)
				Expect(err).NotTo(HaveOccurred())
				Expect(state).To(HaveLen(1))
				Expect(state[0].Kind).To(Equal(orgsync.KindFiles))
			},

			Entry("one still open", `[{"number":9,"state":"open"}]`),
			Entry("one the repository closed",
				`[{"number":9,"state":"closed","merged_at":null}]`),
		)

		// A repository with delete_branch_on_merge took the branch away the
		// moment the pull request landed, so there is nothing to build on.
		It("starts from the default branch where a merged one was tidied away", func() {
			target := grantContents()
			configureKind(target, orgsync.KindFiles, contributing)
			stub.branchPRs = `[{"number":9,"state":"closed","merged":true,` +
				`"merged_at":"2026-08-17T00:00:00Z","head":{"sha":"mergedcommit"}}]`

			applied(target)

			Expect(stub.createdTrees[0]).To(ContainSubstring(`"base_tree":"basetree"`))
			Expect(stub.createdPRs).To(HaveLen(1))
		})

		// GitHub leaves a merged branch in place by default, and the branch is
		// named after the outcome, so the same one comes round again the moment
		// somebody changes the file back. Its tip is already in the default
		// branch, so building on it once more would produce a commit carrying
		// nothing and a pull request GitHub refuses to open - and reading that
		// refusal as "the files are right" recorded the repository as matching
		// while the file was gone, for ever, because nothing would ask again.
		It("proposes again where a merged branch was left and the file changed back", func() {
			target := grantContents()
			configureKind(target, orgsync.KindFiles, contributing)

			plan(target)
			computed, actions := livePlan(target)
			approve(computed)

			// The branch is still there, pointing at what merged, and the
			// default branch no longer holds the file - which is why the
			// planner produced a create action at all
			written, err := orgsync.DecodeFile(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			stub.branchRefs[written.Proposal] = "mergedcommit"
			stub.branchPRs = `[{"number":9,"state":"closed","merged":true,` +
				`"merged_at":"2026-08-17T00:00:00Z"}]`

			// The merged tip already holds exactly what is being proposed, so a
			// commit built on it would produce the tree it started from and
			// carry nothing at all
			stub.migrationTipTree = "branchtree"
			stub.createdTreeSHA = "branchtree"
			stub.repoTrees = map[string]string{
				"branchtree": fmt.Sprintf(
					`{"tree":[{"path":"CONTRIBUTING.md","type":"blob","mode":"100644",`+
						`"sha":%q,"size":16}],"truncated":false}`,
					orgsync.BlobID([]byte("# Contributing\n"))),
			}

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			// Built past the merged tip, on the default branch, so the commit
			// carries something and the proposal opens. Built on the tip, the
			// tree would be the one it started from and nothing would be
			// committed at all - which is the shape that reported success with
			// the file still missing.
			Expect(stub.createdTrees).To(HaveLen(1))
			Expect(stub.createdTrees[0]).To(ContainSubstring(`"base_tree":"basetree"`))
			Expect(stub.createdCommits).To(HaveLen(1))
			Expect(stub.createdPRs).To(HaveLen(1))
			Expect(stub.forcedPushes).To(BeZero())

			applied, _, err := service.store.GetSyncPlan(
				GinkgoT().Context(), target.ID, computed.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied.State).To(Equal(orgsync.PlanApplied))
		})

		// The other side of it: a plan replayed after somebody merged, where
		// the default branch does hold the file. Built on the default branch,
		// the tree is the one it started from, so there is nothing to commit
		// and nothing to open - answered here rather than by asking GitHub to
		// open a pull request and reading its refusal.
		It("proposes nothing where the default branch already says it", func() {
			target := grantContents()
			configureKind(target, orgsync.KindFiles, contributing)

			plan(target)
			computed, actions := livePlan(target)
			approve(computed)

			written, err := orgsync.DecodeFile(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			stub.branchRefs[written.Proposal] = "mergedcommit"
			stub.branchPRs = `[{"number":9,"state":"closed","merged":true,` +
				`"merged_at":"2026-08-17T00:00:00Z"}]`

			// The default branch now holds exactly what was proposed
			stub.repoTrees = map[string]string{
				"basetree": fmt.Sprintf(
					`{"tree":[{"path":"CONTRIBUTING.md","type":"blob","mode":"100644",`+
						`"sha":%q,"size":16}],"truncated":false}`,
					orgsync.BlobID([]byte("# Contributing\n"))),
			}
			stub.createdTreeSHA = "basetree"

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			Expect(stub.createdCommits).To(BeEmpty())
			Expect(stub.createdPRs).To(BeEmpty())

			applied, _, err := service.store.GetSyncPlan(
				GinkgoT().Context(), target.ID, computed.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied.State).To(Equal(orgsync.PlanApplied))
		})

		// Nothing here removes a branch. GitHub's delete has no
		// compare-and-swap - unlike the move, which it refuses when it is not a
		// fast-forward - so a commit landing between reading a branch and
		// removing it would be gone with no error and no trace.
		It("builds past a merged branch rather than taking it away", func() {
			target := grantContents()
			configureKind(target, orgsync.KindFiles, contributing)

			plan(target)
			computed, actions := livePlan(target)
			approve(computed)

			written, err := orgsync.DecodeFile(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			stub.branchRefs[written.Proposal] = "theircommit"
			stub.migrationTipTree = "their-tree"
			stub.branchPRs = `[{"number":9,"state":"closed","merged":true,` +
				`"merged_at":"2026-08-17T00:00:00Z","head":{"sha":"mergedcommit"}}]`

			// The stub answers a delete with a 500, so an apply that tried one
			// would fail here rather than pass quietly
			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			// On the default branch, not on the merged tip: what merged is in
			// the default branch, so a commit built on the tip again would
			// carry nothing.
			Expect(stub.createdTrees[0]).To(ContainSubstring(`"base_tree":"basetree"`))
			Expect(stub.createdCommits[0]).To(ContainSubstring(`"basecommit"`))
			Expect(stub.forcedPushes).To(BeZero())
			Expect(stub.branchRefs).To(HaveKey(written.Proposal))

			// And on the old tip as well, which is what makes moving the branch
			// there a fast-forward. GitHub squashes and rebases as well as
			// merging, and after either of those the tip is not in the default
			// branch at all - so without this the move is refused and the
			// repository is stuck re-planning and re-failing for ever.
			Expect(stub.createdCommits[0]).To(ContainSubstring(`"theircommit"`))
		})

		// The plan refused a retired path that was a directory on the default
		// branch. The tree the commit is built on is the proposal branch, a
		// different tree that can hold a different thing there - and a tree
		// entry removing a directory removes everything under it.
		It("refuses to remove a path the branch has turned into a directory", func() {
			target := grantContents()
			stub.repoTree = `{"sha":"basetree","tree":[{"path":".renovaterc",` +
				`"type":"blob","mode":"100644","sha":"old","size":2}],"truncated":false}`
			configureKind(target, orgsync.KindFiles,
				`{"files":[{"path":"CONTRIBUTING.md","content":"# Contributing\n"}],`+
					`"retired":[".renovaterc"]}`)

			plan(target)
			computed, actions := livePlan(target)
			approve(computed)

			written, err := orgsync.DecodeFile(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			stub.branchRefs[written.Proposal] = "earliercommit"
			stub.migrationTipTree = "branchtree"
			stub.repoTrees = map[string]string{
				"branchtree": `{"tree":[{"path":".renovaterc","type":"tree",` +
					`"mode":"040000","sha":"d1"}],"truncated":false}`,
			}

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			Expect(stub.createdTrees).To(BeEmpty())

			applied, planActions, err := service.store.GetSyncPlan(
				GinkgoT().Context(), target.ID, computed.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied.State).To(Equal(orgsync.PlanFailed))
			Expect(planActions[0].Error).To(ContainSubstring("not an ordinary file"))
		})

		// The same question asked of a write. Anybody with push rights can put
		// a directory on the bot's own branch, and a blob written where one is
		// takes everything under it with no record that it was ever there.
		It("refuses to write a path the branch has turned into a directory", func() {
			target := grantContents()
			stub.repoTree = `{"sha":"basetree","tree":[],"truncated":false}`
			configureKind(target, orgsync.KindFiles,
				`{"files":[{"path":"docs.md","content":"# Docs\n"}]}`)

			plan(target)
			computed, actions := livePlan(target)
			approve(computed)

			written, err := orgsync.DecodeFile(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			stub.branchRefs[written.Proposal] = "earliercommit"
			stub.migrationTipTree = "branchtree"
			stub.repoTrees = map[string]string{
				"branchtree": `{"tree":[{"path":"docs.md","type":"tree",` +
					`"mode":"040000","sha":"d1"}],"truncated":false}`,
			}

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			Expect(stub.createdTrees).To(BeEmpty())

			applied, planActions, err := service.store.GetSyncPlan(
				GinkgoT().Context(), target.ID, computed.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied.State).To(Equal(orgsync.PlanFailed))
			Expect(planActions[0].Error).To(ContainSubstring("not an ordinary file"))
		})

		// A removal whose parent the branch has turned into a file. Nothing is
		// there to remove - git puts a blob or a directory at a name, never
		// both - so the removal is left out and the rest of the change is
		// committed. Refusing instead would stop a repository over a path it
		// does not have, which is the answer a write in the same place gets
		// because a write would take the file in the way with it.
		It("leaves out a removal from under a path the branch made a file", func() {
			target := grantContents()
			stub.repoTree = `{"sha":"basetree","tree":[{"path":"docs",` +
				`"type":"tree","mode":"040000","sha":"d1"},{"path":"docs/old.md",` +
				`"type":"blob","mode":"100644","sha":"old","size":2}],"truncated":false}`
			configureKind(target, orgsync.KindFiles,
				`{"files":[{"path":"CONTRIBUTING.md","content":"# Contributing\n"}],`+
					`"retired":["docs/old.md"]}`)

			plan(target)
			computed, actions := livePlan(target)
			approve(computed)

			written, err := orgsync.DecodeFile(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			stub.branchRefs[written.Proposal] = "earliercommit"
			stub.migrationTipTree = "branchtree"

			// And read the long way round, so the apply path answers the same
			// whether GitHub listed the branch's tree or declined to
			stub.repoTrees = map[string]string{"branchtree": `{"tree":[],"truncated":true}`}
			stub.repoLevels = map[string]string{
				"branchtree": `{"tree":[{"path":"docs","type":"blob",` +
					`"mode":"100644","sha":"b1","size":3}]}`,
			}

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			Expect(stub.createdTrees).To(HaveLen(1))
			Expect(stub.createdTrees[0]).NotTo(ContainSubstring("docs/old.md"))
			Expect(stub.createdTrees[0]).To(ContainSubstring("CONTRIBUTING.md"))

			applied, _, err := service.store.GetSyncPlan(
				GinkgoT().Context(), target.ID, computed.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied.State).To(Equal(orgsync.PlanApplied))
		})

		// The organization retires one path for every repository, and one of
		// them happens to keep a file where that path's directory would be. It
		// never had the retired file and no commit could remove it, so it is
		// synchronized like any other - where refusing put the whole repository
		// out of sync for ever over a path it does not have.
		It("syncs a repository holding a file where a retired path would sit", func() {
			target := grantContents()
			stub.repoTree = `{"sha":"basetree","tree":[{"path":"docs",` +
				`"type":"blob","mode":"100644","sha":"b1","size":4}],"truncated":false}`
			configureKind(target, orgsync.KindFiles,
				`{"files":[{"path":"CONTRIBUTING.md","content":"# Contributing\n"}],`+
					`"retired":["docs/old.md"]}`)

			plan(target)
			_, actions := livePlan(target)

			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Subject).To(Equal("CONTRIBUTING.md"))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationCreate))
		})

		// And of the directory above it. A tree entry at a/b.md where a is a
		// blob replaces the blob with a directory, which is the same silent
		// destruction from the other side.
		It("refuses to write under a path the branch has turned into a file", func() {
			target := grantContents()
			stub.repoTree = `{"sha":"basetree","tree":[],"truncated":false}`
			configureKind(target, orgsync.KindFiles,
				`{"files":[{"path":"docs/index.md","content":"# Docs\n"}]}`)

			plan(target)
			computed, actions := livePlan(target)
			approve(computed)

			written, err := orgsync.DecodeFile(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			stub.branchRefs[written.Proposal] = "earliercommit"
			stub.migrationTipTree = "branchtree"
			stub.repoTrees = map[string]string{
				"branchtree": `{"tree":[{"path":"docs","type":"blob",` +
					`"mode":"100644","sha":"b1","size":3}],"truncated":false}`,
			}

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			Expect(stub.createdTrees).To(BeEmpty())

			applied, planActions, err := service.store.GetSyncPlan(
				GinkgoT().Context(), target.ID, computed.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied.State).To(Equal(orgsync.PlanFailed))
			Expect(planActions[0].Error).To(ContainSubstring("is not a directory"))
		})

		// An attempt that died between recording one action and recording the
		// next leaves the first saying something about a change the retry then
		// makes whole. The retry replays everything, so its answer is every
		// action's answer.
		It("clears what an earlier attempt recorded once the retry lands", func() {
			target := grantContents()
			configureKind(target, orgsync.KindFiles, `{"files":[`+
				`{"path":"CONTRIBUTING.md","content":"# Contributing\n"},`+
				`{"path":"SECURITY.md","content":"# Security\n"}]}`)

			plan(target)
			computed, actions := livePlan(target)
			approve(computed)

			// One of them recorded failed, the other left where it was
			Expect(service.store.RecordSyncActionOutcome(
				GinkgoT().Context(), orgsync.ActionOutcome{
					ActionID: actions[0].ID, State: orgsync.ActionFailed,
					Error: "the process died here",
				})).To(Succeed())

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			applied, planActions, err := service.store.GetSyncPlan(
				GinkgoT().Context(), target.ID, computed.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied.State).To(Equal(orgsync.PlanApplied))

			for _, action := range planActions {
				Expect(action.State).To(Equal(orgsync.ActionApplied))
				Expect(action.Error).To(BeEmpty())
			}
		})

		// One opened between the plan and the apply, which is the window the
		// planner cannot close. The proposal is adopted rather than doubled.
		It("keeps an open pull request rather than opening a second", func() {
			target := grantContents()
			configureKind(target, orgsync.KindFiles, contributing)

			plan(target)
			computed, _ := livePlan(target)
			approve(computed)

			stub.branchPRs = `[{"number":9,"state":"open"}]`

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			Expect(stub.createdPRs).To(BeEmpty())
			Expect(stub.editedPRs).To(HaveLen(1))
			Expect(stub.editedPRs[0]).To(ContainSubstring("CONTRIBUTING.md"))
		})

		It("writes what a repository adjusts rather than the plain template", func() {
			target := grantContents()
			configureKind(target, orgsync.KindFiles,
				`{"files":[{"path":"renovate.json",`+
					`"content":"{\"extends\":[\"config:recommended\"]}"}]}`)
			adjusting(target, `{"merges":[{"path":"renovate.json",`+
				`"overrides":{"timezone":"Europe/Warsaw"}}]}`)

			plan(target)
			_, actions := livePlan(target)

			written, err := orgsync.DecodeFile(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(written.Content)).To(ContainSubstring("Europe/Warsaw"))
			Expect(string(written.Content)).To(ContainSubstring("config:recommended"))
		})

		// Fail-closed. The tool this replaces reported a failed merge as a
		// warning and wrote the raw template over the repository's file, so a
		// broken adjustment destroyed the customization it described.
		It("proposes nothing where a repository's adjustment cannot be used", func() {
			target := grantContents()
			configureKind(target, orgsync.KindFiles,
				`{"files":[{"path":"renovate.json","content":"{}"}]}`)
			adjusting(target, `{"merges":[{"path":"package.json"}]}`)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))

			// And what is recorded is why, with no digest: asked again once
			// somebody fixes it rather than left looking finished for six
			// hours, and readable meanwhile by whoever has to fix it
			state, err := service.store.ListSyncRepositoryState(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(HaveLen(1))
			Expect(state[0].AppliedDigest).To(BeEmpty())
			Expect(state[0].Problem).To(ContainSubstring(
				"the adjustments saved for this repository cannot be used"))
			Expect(state[0].Problem).To(ContainSubstring("package.json"))
		})

		It("stands down without the permission it needs", func() {
			target := granting(`{"issues":"write"}`)
			configureKind(target, orgsync.KindFiles, contributing)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		// A repository with no commits has nowhere to propose against, and
		// GitHub names a default branch for one anyway - the name is
		// configuration and is there long before the branch is. Read as a
		// repository that simply has none of the managed files, the planner
		// emitted a create for each, a person approved them, and the apply
		// refused for want of a branch to build on - which spends the
		// installation's one live plan slot and marks every plan riding with it
		// failed, on every reconcile, for ever.
		It("leaves a repository with no commits alone, and says why", func() {
			target := grantContents()
			stub.treeNotFound = true
			configureKind(target, orgsync.KindFiles, contributing)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))

			state, err := service.store.ListSyncRepositoryState(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(HaveLen(1))
			Expect(state[0].AppliedDigest).To(BeEmpty())
			Expect(state[0].Problem).To(ContainSubstring("no commits"))
		})

		// GitHub refuses to open a pull request for a branch carrying nothing
		// the base does not, and reaching that means the planner found the
		// default branch wanting - so it says the branch is stale, never that
		// the files are right. Read as success, it recorded a repository as
		// matching while what it should hold was missing, and the branch is
		// named after the outcome, so nothing would ever ask again.
		It("fails rather than reading an empty pull request as done", func() {
			target := grantContents()
			configureKind(target, orgsync.KindFiles, contributing)
			plan(target)
			computed, _ := livePlan(target)
			approve(computed)

			stub.refuseEmptyPR = true

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(Succeed())

			applied, actions, err := service.store.GetSyncPlan(
				GinkgoT().Context(), target.ID, computed.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied.State).To(Equal(orgsync.PlanFailed))
			Expect(actions[0].State).To(Equal(orgsync.ActionFailed))
			Expect(actions[0].Error).To(ContainSubstring("No commits between"))

			// And nothing recorded, so the repository is asked again rather
			// than left looking finished behind a proposal that was refused
			state, err := service.store.ListSyncRepositoryState(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(BeEmpty())
		})

		// GitHub keeps workflow files behind a permission of their own and
		// enforces it where the ref moves, so a plan that did not check would be
		// read and approved by a person and refused by GitHub at its last step -
		// and because the apply failed, nothing is recorded, so the same plan is
		// computed, approved and refused again on every reconcile after it.
		It("stands down where a workflow is configured and not permitted", func() {
			target := grantContents()
			configureKind(target, orgsync.KindFiles, `{"files":[
				{"path":".github/workflows/ci.yaml","content":"name: CI\n"}]}`)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		// Removing one is writing the tree that no longer holds it, which GitHub
		// refuses for the same reason it refuses adding one.
		//
		// The repository holds the path, so there is a removal to plan: without
		// it the plan would be empty whatever the permission said, and this
		// would pass with the check taken out.
		It("stands down where a retired path is a workflow", func() {
			target := grantContents()
			stub.repoTree = `{"sha":"basetree","tree":[
				{"path":".github","type":"tree","mode":"040000","sha":"d1"},
				{"path":".github/workflows","type":"tree","mode":"040000","sha":"d2"},
				{"path":".github/workflows/old.yaml","type":"blob","mode":"100644",
				 "sha":"old","size":2}],"truncated":false}`
			configureKind(target, orgsync.KindFiles,
				`{"files":[],"retired":[".github/workflows/old.yaml"]}`)

			plan(target)

			_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).To(MatchError(storage.ErrNotFound))
		})

		It("plans the removal of a retired workflow once permitted", func() {
			target := granting(`{"issues":"write","contents":"write","workflows":"write"}`)
			stub.repoTree = `{"sha":"basetree","tree":[
				{"path":".github","type":"tree","mode":"040000","sha":"d1"},
				{"path":".github/workflows","type":"tree","mode":"040000","sha":"d2"},
				{"path":".github/workflows/old.yaml","type":"blob","mode":"100644",
				 "sha":"old","size":2}],"truncated":false}`
			configureKind(target, orgsync.KindFiles,
				`{"files":[],"retired":[".github/workflows/old.yaml"]}`)

			plan(target)

			_, actions := livePlan(target)
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationDelete))
		})

		// An excluded path is skipped everywhere the planner looks, so asking
		// for a permission on its account stands the whole kind down for the
		// whole installation over a file nothing was going to touch.
		It("asks for nothing extra for a workflow it excludes", func() {
			target := grantContents()
			configureKind(target, orgsync.KindFiles, `{"files":[
				{"path":".github/workflows/ci.yaml","content":"name: CI\n"},
				{"path":"CONTRIBUTING.md","content":"# Contributing\n"}],
				"excludes":[".github/workflows/*"]}`)

			plan(target)

			_, actions := livePlan(target)
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Subject).To(Equal("CONTRIBUTING.md"))
		})

		// A 404 on the tree read is not only a repository with no commits: a
		// branch renamed since the catalog looked, and one this installation can
		// no longer read, answer the same way. Naming one of them as the cause
		// puts a claim in front of somebody that the read cannot support.
		It("does not name a cause the read cannot tell apart", func() {
			target := grantContents()
			stub.treeNotFound = true
			configureKind(target, orgsync.KindFiles, contributing)

			plan(target)

			state, err := service.store.ListSyncRepositoryState(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(HaveLen(1))
			Expect(state[0].Problem).To(ContainSubstring("no commits"))
			Expect(state[0].Problem).To(ContainSubstring("renamed"))
			Expect(state[0].Problem).To(ContainSubstring("no longer read"))
		})

		It("proposes a workflow once the installation permits it", func() {
			target := granting(`{"issues":"write","contents":"write","workflows":"write"}`)
			configureKind(target, orgsync.KindFiles, `{"files":[
				{"path":".github/workflows/ci.yaml","content":"name: CI\n"}]}`)

			plan(target)

			_, actions := livePlan(target)
			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Subject).To(Equal(".github/workflows/ci.yaml"))
		})

		// The apply is where GitHub enforces it, so the apply is where being
		// wrong costs a commit built and a ref refused. A permission can be
		// withdrawn between the plan being approved and being applied, which is
		// what revoking one is for.
		It("refuses to apply a workflow whose permission was revoked", func() {
			target := granting(`{"issues":"write","contents":"write","workflows":"write"}`)
			configureKind(target, orgsync.KindFiles, `{"files":[
				{"path":".github/workflows/ci.yaml","content":"name: CI\n"}]}`)
			plan(target)
			computed, _ := livePlan(target)
			approve(computed)

			stub.installations = `[{"id":411,"account":` +
				`{"id":7,"login":"smykla-skalski","type":"Organization"},` +
				`"permissions":{"issues":"write","contents":"write"}}]`
			_, err := service.SyncCatalog(GinkgoT().Context())
			Expect(err).NotTo(HaveOccurred())

			Expect(service.sync.ApplyPlans(GinkgoT().Context())).To(HaveOccurred())

			// Nothing built and nothing pushed: the commit is what GitHub
			// refuses, and a branch left behind would be a proposal for a change
			// no pull request could carry.
			Expect(stub.createdCommits).To(BeEmpty())
			held, _, err := service.store.GetSyncPlan(
				GinkgoT().Context(), target.ID, computed.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(held.State).To(Equal(orgsync.PlanApplying))
		})

		// git puts a blob wherever a tree entry names one, and says nothing
		// about what it replaced. A configured path that is a directory in one
		// repository, or that sits under a file there, is a change that would
		// destroy something - so that repository is refused whole rather than
		// having the one path quietly skipped.
		DescribeTable("refuses a repository whose contents the change would destroy",
			func(tree string) {
				target := grantContents()
				stub.repoTree = tree
				configureKind(target, orgsync.KindFiles,
					`{"files":[{"path":"docs/guide.md","content":"# Guide\n"}]}`)

				plan(target)

				_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
				Expect(err).To(MatchError(storage.ErrNotFound))

				// And what is recorded is why, with no digest, so it is answered
				// again the moment somebody resolves it rather than left
				// looking finished
				state, err := service.store.ListSyncRepositoryState(
					GinkgoT().Context(), target.ID)
				Expect(err).NotTo(HaveOccurred())
				Expect(state).To(HaveLen(1))
				Expect(state[0].AppliedDigest).To(BeEmpty())
				Expect(state[0].Problem).To(ContainSubstring("these files cannot be composed"))
				Expect(state[0].Problem).To(ContainSubstring("docs/guide.md"))
			},
			Entry("the path is a directory there", `{"sha":"basetree","tree":[
				{"path":"docs","type":"tree","mode":"040000","sha":"d1"},
				{"path":"docs/guide.md","type":"tree","mode":"040000","sha":"d2"}
			],"truncated":false}`),
			Entry("the path is a symbolic link there", `{"sha":"basetree","tree":[
				{"path":"docs","type":"tree","mode":"040000","sha":"d1"},
				{"path":"docs/guide.md","type":"blob","mode":"120000","sha":"b1","size":9}
			],"truncated":false}`),
			Entry("a directory on the way to it is a file there", `{"sha":"basetree","tree":[
				{"path":"docs","type":"blob","mode":"100644","sha":"b1","size":4}
			],"truncated":false}`),
		)

		// A refusal is rewritten every sweep until the repository can be synced,
		// which is what makes it worth reading - and nothing rewrites it once
		// the repository leaves the planner's scope. Left there it states, for
		// ever, a reason nobody can act on, and usually the very reason
		// somebody switched the kind off.
		DescribeTable("takes a refusal off what it has stopped looking at",
			func(leave func(target storage.Target)) {
				target := grantContents()
				stub.repoTree = `{"sha":"basetree","tree":[
					{"path":"docs","type":"blob","mode":"100644","sha":"b1","size":4}
				],"truncated":false}`
				configureKind(target, orgsync.KindFiles,
					`{"files":[{"path":"docs/guide.md","content":"# Guide\n"}]}`)

				plan(target)

				state, err := service.store.ListSyncRepositoryState(
					GinkgoT().Context(), target.ID)
				Expect(err).NotTo(HaveOccurred())
				Expect(state).To(HaveLen(1))
				Expect(state[0].Problem).NotTo(BeEmpty())

				leave(target)
				plan(target)

				state, err = service.store.ListSyncRepositoryState(
					GinkgoT().Context(), target.ID)
				Expect(err).NotTo(HaveOccurred())
				Expect(state).To(HaveLen(1))
				Expect(state[0].Problem).To(BeEmpty())
			},
			Entry("the repository switches the kind off", func(target storage.Target) {
				override(target, orgsync.KindFiles, false)
			}),
			Entry("the installation switches the kind off", func(target storage.Target) {
				GinkgoHelper()

				stored, err := service.store.GetSyncConfig(
					GinkgoT().Context(), target.ID, orgsync.KindFiles)
				Expect(err).NotTo(HaveOccurred())

				_, err = service.store.SetSyncConfig(
					GinkgoT().Context(), orgsync.ConfigChange{
						TargetID: target.ID, Kind: orgsync.KindFiles, Enabled: false,
						Document: stored.Document, ActorID: target.Account.ID,
						Now: time.Now().UTC(), Revision: stored.Revision,
					})
				Expect(err).NotTo(HaveOccurred())
			}),

			// The row survives because a repository is soft-deleted rather than
			// removed, so the foreign key holds and the listing still carries
			// it - which is what makes this reachable at all.
			Entry("the repository leaves the installation", func(_ storage.Target) {
				GinkgoHelper()

				stub.repos = `{"total_count":0,"repositories":[]}`
				_, err := service.SyncCatalog(GinkgoT().Context())
				Expect(err).NotTo(HaveOccurred())
			}),
		)

		// A kind waiting on a permission is one somebody is still expecting to
		// run, and its refusals are as true as they were. Cleared, the pane
		// built to say why nothing is happening answers "nothing is wrong" -
		// while the reason sits on a page the reader is not looking at.
		It("keeps a refusal while the kind waits on a permission", func() {
			target := grantContents()
			stub.repoTree = `{"sha":"basetree","tree":[
				{"path":"docs","type":"blob","mode":"100644","sha":"b1","size":4}
			],"truncated":false}`
			configureKind(target, orgsync.KindFiles,
				`{"files":[{"path":"docs/guide.md","content":"# Guide\n"}]}`)

			plan(target)

			state, err := service.store.ListSyncRepositoryState(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(HaveLen(1))
			Expect(state[0].Problem).NotTo(BeEmpty())

			// The organization adds a workflow, which needs a permission this
			// installation has not granted, so the whole kind stands down.
			stored, err := service.store.GetSyncConfig(
				GinkgoT().Context(), target.ID, orgsync.KindFiles)
			Expect(err).NotTo(HaveOccurred())

			_, err = service.store.SetSyncConfig(GinkgoT().Context(), orgsync.ConfigChange{
				TargetID: target.ID, Kind: orgsync.KindFiles, Enabled: true,
				Document: []byte(`{"files":[
					{"path":"docs/guide.md","content":"# Guide\n"},
					{"path":".github/workflows/ci.yaml","content":"name: CI\n"}]}`),
				ActorID: target.Account.ID, Now: time.Now().UTC(),
				Revision: stored.Revision,
			})
			Expect(err).NotTo(HaveOccurred())

			plan(target)

			state, err = service.store.ListSyncRepositoryState(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(HaveLen(1))
			Expect(state[0].Problem).NotTo(BeEmpty())
		})

		// The other end of the same record. A repository that plans work is a
		// repository nothing is stopping any more, and a refusal left standing
		// would have the panel saying the files are not being synced here while
		// a plan to sync them waited for approval.
		It("takes a refusal off once the repository can be planned", func() {
			target := grantContents()
			stub.repoTree = `{"sha":"basetree","tree":[
				{"path":"docs","type":"blob","mode":"100644","sha":"b1","size":4}
			],"truncated":false}`
			configureKind(target, orgsync.KindFiles,
				`{"files":[{"path":"docs/guide.md","content":"# Guide\n"}]}`)

			plan(target)

			state, err := service.store.ListSyncRepositoryState(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(HaveLen(1))
			Expect(state[0].Problem).NotTo(BeEmpty())

			// Somebody makes docs a directory, so the change composes
			stub.repoTree = `{"sha":"basetree","tree":[
				{"path":"docs","type":"tree","mode":"040000","sha":"d1"}
			],"truncated":false}`

			plan(target)

			_, actions, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(actions).NotTo(BeEmpty())

			// The refusal is gone, and no digest has taken its place: nothing
			// has been applied yet, and the executor is what records that
			state, err = service.store.ListSyncRepositoryState(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(HaveLen(1))
			Expect(state[0].Problem).To(BeEmpty())
			Expect(state[0].AppliedDigest).To(BeEmpty())
		})

		// GitHub declines to list a tree past a hundred thousand entries, and a
		// path missing from a listing that stopped early is not a path a
		// repository does not have. Reading it a level at a time settles it,
		// and the levels answer what git holds rather than whether a file can
		// be downloaded - which is the only read that can see a directory.
		Describe("a repository too large to list", func() {
			BeforeEach(func() {
				stub.repoTree = `{"sha":"basetree","tree":[],"truncated":true}`
			})

			It("reads the paths it cares about a level at a time", func() {
				target := grantContents()
				stub.repoLevels = map[string]string{
					"main": `{"tree":[{"path":"docs","type":"tree",` +
						`"mode":"040000","sha":"d1"}]}`,
					"d1": fmt.Sprintf(
						`{"tree":[{"path":"guide.md","type":"blob","mode":"100644",`+
							`"sha":%q,"size":8}]}`, orgsync.BlobID([]byte("# Guide\n"))),
				}
				configureKind(target, orgsync.KindFiles,
					`{"files":[{"path":"docs/guide.md","content":"# Guide\n"}]}`)

				plan(target)

				// It already matches, so there is nothing to propose - which is
				// the answer only an exact read can give
				_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
				Expect(err).To(MatchError(storage.ErrNotFound))
			})

			It("still sees a directory it would have written over", func() {
				target := grantContents()
				stub.repoLevels = map[string]string{
					"main": `{"tree":[{"path":"docs","type":"blob",` +
						`"mode":"100644","sha":"b1","size":4}]}`,
				}
				configureKind(target, orgsync.KindFiles,
					`{"files":[{"path":"docs/guide.md","content":"# Guide\n"}]}`)

				plan(target)

				_, _, err := service.store.GetLiveSyncPlan(GinkgoT().Context(), target.ID)
				Expect(err).To(MatchError(storage.ErrNotFound))
			})
		})

		It("writes a path whose directory is a directory", func() {
			target := grantContents()
			stub.repoTree = `{"sha":"basetree","tree":[` +
				`{"path":"docs","type":"tree","mode":"040000","sha":"d1"}],"truncated":false}`
			configureKind(target, orgsync.KindFiles,
				`{"files":[{"path":"docs/guide.md","content":"# Guide\n"}]}`)

			plan(target)
			_, actions := livePlan(target)

			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Subject).To(Equal("docs/guide.md"))
		})
	})

	// What the panel's path finder offers, and what it costs to keep current.
	//
	// The list is the expensive read - megabytes and a second's work for a
	// repository holding thousands of files - and the commit its branch points
	// at is a few hundred bytes. So the refresh asks the cheap question first
	// and reads the tree only where the answer has changed, which is what makes
	// the interval a choice rather than a budget.
	Describe("the path index", func() {
		var refresh func(storage.Target)
		var indexed func() storage.Target

		BeforeEach(func() {
			stub.repoTree = `{"sha":"basetree","tree":[` +
				`{"path":"README.md","type":"blob","mode":"100644","sha":"b1","size":7},` +
				`{"path":"docs","type":"tree","mode":"040000","sha":"d1"}],"truncated":false}`

			/* An installation that has configured nothing gets no index at
			   all - the cost of one is a ref read per repository per interval
			   and a whole tree wherever a branch has moved, and the majority of
			   installations never use sync. So every spec here starts from one
			   that HAS configured it, which is also the only state in which
			   anybody is typing a path into the finder. */
			indexed = func() storage.Target {
				GinkgoHelper()

				target := seed()
				configureKind(target, orgsync.KindFiles,
					`{"files":[{"path":"renovate.json","content":"{}\n"}]}`)

				return target
			}

			refresh = func(target storage.Target) {
				GinkgoHelper()

				service.sync.RefreshPaths(
					GinkgoT().Context(), client(), target.ID, service.pathIndexInterval(),
				)
			}
		})

		// Stored back in time, which is the only way a spec can say "a whole
		// interval has passed" without waiting one.
		due := func(target storage.Target) orgsync.RepositoryPaths {
			GinkgoHelper()

			rows, err := service.store.ListSyncRepositoryPaths(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(1))

			row := rows[0]
			row.ObservedAt = time.Now().UTC().Add(-2 * defaultPathIndexInterval)
			Expect(service.store.SetSyncRepositoryPaths(GinkgoT().Context(), row)).To(Succeed())

			return row
		}

		It("records the paths and the commit they were read at", func() {
			target := indexed()

			refresh(target)

			rows, err := service.store.ListSyncRepositoryPaths(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(1))
			// Ordinary files only: a directory is not somewhere a template can
			// be written.
			Expect(rows[0].Paths).To(Equal([]string{"README.md"}))
			Expect(rows[0].HeadSHA).To(Equal("basecommit"))
		})

		It("reads no tree at all when the branch has not moved", func() {
			target := indexed()
			refresh(target)
			due(target)

			before := stub.countCalls(http.MethodGet, "/git/trees/main")
			refresh(target)

			Expect(stub.countCalls(http.MethodGet, "/git/trees/main")).To(Equal(before))
			// And the list is still recorded as current, so the reader is told
			// it was confirmed rather than that it is a day old.
			rows, err := service.store.ListSyncRepositoryPaths(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(rows[0].ObservedAt).To(BeTemporally("~", time.Now().UTC(), time.Minute))
			Expect(rows[0].Paths).To(Equal([]string{"README.md"}))
		})

		It("reads the tree again when the branch has moved", func() {
			target := indexed()
			refresh(target)
			due(target)

			stub.repoHead = "movedcommit"
			stub.repoTree = `{"sha":"newtree","tree":[` +
				`{"path":"CONTRIBUTING.md","type":"blob","mode":"100644",` +
				`"sha":"b2","size":9}],"truncated":false}`
			refresh(target)

			rows, err := service.store.ListSyncRepositoryPaths(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(rows[0].Paths).To(Equal([]string{"CONTRIBUTING.md"}))
			Expect(rows[0].HeadSHA).To(Equal("movedcommit"))
		})

		// A list written before the commit was recorded carries no commit, and
		// an empty string is not a commit anything can be level with. Read once
		// more rather than believed for ever.
		It("reads the tree when the stored commit is unknown", func() {
			target := indexed()
			refresh(target)
			row := due(target)
			row.HeadSHA = ""
			Expect(service.store.SetSyncRepositoryPaths(GinkgoT().Context(), row)).To(Succeed())

			before := stub.countCalls(http.MethodGet, "/git/trees/main")
			refresh(target)

			Expect(stub.countCalls(http.MethodGet, "/git/trees/main")).To(Equal(before + 1))
		})

		// A repository with nothing in it has a default branch by name and no
		// branch to point at. Nothing to offer, and nothing to say about it.
		It("records an empty list for a repository with no commits", func() {
			target := indexed()
			stub.repoHead = ""
			stub.treeNotFound = true

			refresh(target)

			rows, err := service.store.ListSyncRepositoryPaths(GinkgoT().Context(), target.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(rows).To(HaveLen(1))
			Expect(rows[0].Paths).To(BeEmpty())
			Expect(rows[0].HeadSHA).To(BeEmpty())
		})

		// How often the check happens, which is a choice because the check is
		// cheap. Nearest wins: the repository if it says, then its
		// installation, then the process.
		Describe("how often it looks", func() {
			// The installation says hardly ever, so a repository due under the
			// process's own interval is left alone.
			It("takes the installation's interval over the process's", func() {
				target := indexed()
				refresh(target)
				due(target)

				_, err := service.store.UpdateTargetSettings(
					GinkgoT().Context(), targetSettingsWithPathIndex(target, 6*24*time.Hour))
				Expect(err).NotTo(HaveOccurred())

				before := stub.countCalls(http.MethodGet, "/git/ref/heads/main")
				refresh(target)

				Expect(stub.countCalls(http.MethodGet, "/git/ref/heads/main")).To(Equal(before))
			})

			// And the repository beats its installation, which is the level
			// that exists for the one repository that is the exception.
			It("takes the repository's interval over the installation's", func() {
				target := indexed()
				refresh(target)
				due(target)

				updated, err := service.store.UpdateTargetSettings(
					GinkgoT().Context(), targetSettingsWithPathIndex(target, 6*24*time.Hour))
				Expect(err).NotTo(HaveOccurred())

				repositories, err := service.store.ListRepositories(
					GinkgoT().Context(), updated.ID)
				Expect(err).NotTo(HaveOccurred())
				Expect(repositories).To(HaveLen(1))

				every := time.Duration(0)
				_, err = service.store.UpdateRepositorySettings(
					GinkgoT().Context(), storage.RepositorySettingsChange{
						TargetID: updated.ID, RepositoryID: repositories[0].ID,
						ActorAccountID:            updated.Account.ID,
						PathIndexIntervalOverride: &every,
						ExpectedRevision:          repositories[0].Revision,
						ChangedAt:                 time.Now().UTC(),
					})
				Expect(err).NotTo(HaveOccurred())

				before := stub.countCalls(http.MethodGet, "/git/ref/heads/main")
				refresh(target)

				Expect(stub.countCalls(http.MethodGet, "/git/ref/heads/main")).
					To(Equal(before + 1))
			})
		})

		// GitHub will not list a very large tree in one answer, and the list it
		// does give says only that there is more. Divided rather than accepted:
		// truncation is a property of a RESPONSE, so a subtree of a tree too
		// large to list is usually listable whole.
		Describe("a tree GitHub will not list whole", func() {
			BeforeEach(func() {
				stub.repoTree = `{"sha":"basetree","tree":[],"truncated":true}`
				stub.repoLevels = map[string]string{
					"main": `{"tree":[` +
						`{"path":"README.md","type":"blob","mode":"100644","sha":"b1","size":7},` +
						`{"path":"docs","type":"tree","mode":"040000","sha":"d1"}]}`,
				}
				stub.repoTrees = map[string]string{
					"d1": `{"sha":"d1","tree":[` +
						`{"path":"guide.md","type":"blob","mode":"100644","sha":"b2","size":8}],` +
						`"truncated":false}`,
				}
			})

			It("divides it and keeps every path", func() {
				target := indexed()

				refresh(target)

				rows, err := service.store.ListSyncRepositoryPaths(GinkgoT().Context(), target.ID)
				Expect(err).NotTo(HaveOccurred())
				// The subtree's own paths carry the directory they sit in,
				// since a listing names its entries relative to itself.
				Expect(rows[0].Paths).To(Equal([]string{"README.md", "docs/guide.md"}))
				Expect(rows[0].Partial).To(BeFalse())
			})
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

// targetSettingsWithPathIndex says how often this installation's repositories
// have their file lists checked, leaving its other settings as they are.
func targetSettingsWithPathIndex(
	target storage.Target,
	every time.Duration,
) storage.TargetSettingsChange {
	return storage.TargetSettingsChange{
		TargetID:                       target.ID,
		ActorAccountID:                 target.Account.ID,
		RepositoryDefaultEnabled:       target.RepositoryDefaultEnabled,
		PendingCIModeDefault:           target.PendingCIModeDefault,
		PendingCIBranchPatternsDefault: target.PendingCIBranchPatternsDefault,
		PathIndexIntervalOverride:      &every,
		ConfigPatch:                    target.ConfigPatch,
		ExpectedRevision:               target.Revision,
		ChangedAt:                      time.Now().UTC(),
	}
}

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
