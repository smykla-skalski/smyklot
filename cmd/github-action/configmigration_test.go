package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/githubtest"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

var _ = Describe("Configuration migration [Unit]", func() {
	var (
		service  *server
		stub     *githubStub
		endpoint *httptest.Server
	)

	BeforeEach(func() {
		stub = newGitHubStub()
		stub.installations = `[{"id":411,"account":{"id":7,"login":"smykla-skalski","type":"Organization"}}]`
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

	// seed puts one repository in the catalog, switched on, and returns its
	// target. Repositories arrive disabled, and a repository nobody has turned
	// on is not one to open a pull request at.
	seed := func() string {
		GinkgoHelper()

		targetIDs, err := service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())
		Expect(targetIDs).To(HaveLen(1))

		target, err := service.store.GetTarget(GinkgoT().Context(), targetIDs[0])
		Expect(err).NotTo(HaveOccurred())

		_, err = service.store.UpdateTargetSettings(
			GinkgoT().Context(),
			storage.TargetSettingsChange{
				TargetID:                 target.ID,
				ActorAccountID:           target.Account.ID,
				RepositoryDefaultEnabled: true,
				ExpectedRevision:         target.Revision,
				ChangedAt:                time.Now().UTC(),
			},
		)
		Expect(err).NotTo(HaveOccurred())

		return targetIDs[0]
	}

	// propose runs one sweep tick's worth of the migration, reading the
	// repository's file the way the sweep does.
	propose := func(targetID string) {
		GinkgoHelper()

		client, err := github.NewClient("installation-token", endpoint.URL)
		Expect(err).NotTo(HaveOccurred())

		file, err := fetchRepositoryConfig(
			GinkgoT().Context(), client, "smykla-skalski", "smyklot", nil,
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(service.proposeConfigMigration(
			GinkgoT().Context(),
			client,
			targetID,
			github.Repository{
				ID: 41, Owner: "smykla-skalski", Name: "smyklot", DefaultBranch: "main",
			},
			file,
		)).To(Succeed())
	}

	repository := func(targetID string) storage.Repository {
		GinkgoHelper()

		found, err := service.store.GetRepository(
			GinkgoT().Context(), targetID, "github:repository:41",
		)
		Expect(err).NotTo(HaveOccurred())

		return found
	}

	Describe("a repository still on the legacy file", func() {
		BeforeEach(func() {
			stub.repoConfig = "quiet_success: true\ncommand_prefix: \"!\"\n"
		})

		It("proposes the move, in one commit that adds and removes", func() {
			targetID := seed()
			propose(targetID)

			Expect(stub.createdPRs).To(HaveLen(1))

			var pull struct {
				Title string `json:"title"`
				Head  string `json:"head"`
				Base  string `json:"base"`
				Body  string `json:"body"`
			}
			Expect(json.Unmarshal([]byte(stub.createdPRs[0]), &pull)).To(Succeed())
			Expect(pull.Head).To(Equal(migrationBranch))
			Expect(pull.Base).To(Equal("main"))
			Expect(pull.Body).To(ContainSubstring(".github/smyklot.yaml"))

			// One commit, so the repository is never in a state carrying both
			// files - which is the state that would have Smyklot reading one
			// and whoever is reviewing reading the other
			Expect(stub.createdTrees).To(HaveLen(1))

			var tree struct {
				BaseTree string `json:"base_tree"`
				Tree     []struct {
					Path string  `json:"path"`
					SHA  *string `json:"sha"`
				} `json:"tree"`
			}
			Expect(json.Unmarshal([]byte(stub.createdTrees[0]), &tree)).To(Succeed())

			// The tree the base commit records, not the commit. The API
			// documents base_tree as a tree object, and a reference points at a
			// commit - building on the wrong one is a 422 nobody would see
			// until a repository was offered the migration for real
			Expect(tree.BaseTree).To(Equal("basetree"))
			Expect(tree.Tree).To(HaveLen(2))
			Expect(tree.Tree[0].Path).To(Equal(migrationTarget))
			Expect(tree.Tree[0].SHA).NotTo(BeNil())
			Expect(tree.Tree[1].Path).To(Equal(".github/smyklot.yaml"))
			Expect(tree.Tree[1].SHA).To(BeNil())

			found := repository(targetID)
			Expect(found.ConfigMigration).To(Equal(storage.ConfigMigrationProposed))
			Expect(found.ConfigMigrationPR).To(HaveValue(Equal(77)))
		})

		It("asks once rather than once per sweep tick", func() {
			targetID := seed()
			propose(targetID)

			stub.branchPRs = `[{"number":77,"state":"open","merged":false}]`
			propose(targetID)

			Expect(stub.createdPRs).To(HaveLen(1))
			Expect(repository(targetID).ConfigMigration).
				To(Equal(storage.ConfigMigrationProposed))
		})

		// A pull request somebody closed is a decision, not a timeout
		It("stops asking once the proposal is refused", func() {
			targetID := seed()
			propose(targetID)

			stub.branchPRs = `[{"number":77,"state":"closed","merged":false}]`
			propose(targetID)
			Expect(repository(targetID).ConfigMigration).
				To(Equal(storage.ConfigMigrationDeclined))

			// And stays refused, without asking GitHub anything more about it
			asked := stub.countCalls(http.MethodGet, "/pulls")
			propose(targetID)
			Expect(stub.createdPRs).To(HaveLen(1))
			Expect(stub.countCalls(http.MethodGet, "/pulls")).To(Equal(asked))
		})

		// An earlier tick got as far as pushing the branch and no further, so
		// the state on disk says nothing has happened while GitHub disagrees
		It("adopts a branch with an open proposal rather than pushing over it", func() {
			stub.migrationRef = "commitsha"
			stub.branchPRs = `[{"number":77,"state":"open","merged":false}]`

			targetID := seed()
			propose(targetID)

			Expect(stub.createdTrees).To(BeEmpty())
			Expect(stub.createdPRs).To(BeEmpty())
			Expect(stub.forcedPushes).To(BeZero(),
				"somebody's review was pushed over")
			Expect(repository(targetID).ConfigMigration).
				To(Equal(storage.ConfigMigrationProposed))
		})

		// An earlier tick pushed the branch and never opened anything from it.
		// Adopting that as "in progress" left it a dead end no tick could
		// leave, because nothing re-drove the proposal from a branch that
		// already existed
		It("proposes from a branch nothing was opened from", func() {
			stub.migrationRef = "commitsha"

			targetID := seed()
			propose(targetID)

			Expect(stub.createdPRs).To(HaveLen(1))
			Expect(stub.forcedPushes).To(Equal(1))
			Expect(repository(targetID).ConfigMigration).
				To(Equal(storage.ConfigMigrationProposed))
		})

		// A branch named after the bot is still a branch anybody can push to,
		// and rebuilding one replaces its history. Somebody who closed the
		// proposal, pushed a fixup and had an operator clear the refusal would
		// have watched their commit disappear with no error and no trace
		It("leaves a branch somebody else pushed to alone", func() {
			stub.migrationRef = "commitsha"
			stub.migrationTip = "fix the thing the bot got wrong"

			targetID := seed()
			propose(targetID)

			Expect(stub.forcedPushes).To(BeZero(), "somebody's commit was pushed over")
			Expect(stub.createdTrees).To(BeEmpty())
			Expect(stub.createdPRs).To(BeEmpty())

			// And nothing is written down, because the state resolves itself:
			// whoever pushed opens a pull request and the next tick adopts it
			Expect(repository(targetID).ConfigMigration).
				To(Equal(storage.ConfigMigrationNone))

			stub.branchPRs = `[{"number":77,"state":"open","merged":false}]`
			propose(targetID)
			Expect(repository(targetID).ConfigMigration).
				To(Equal(storage.ConfigMigrationProposed))
		})

		// The panel's only way back from a refusal. It used to undo itself on
		// the next sweep tick: the branch was still there, the closed proposal
		// was still findable, and adopting it wrote the refusal straight back
		It("asks again after an operator clears a refusal", func() {
			targetID := seed()
			propose(targetID)

			stub.branchPRs = `[{"number":77,"state":"closed","merged":false}]`
			propose(targetID)
			Expect(repository(targetID).ConfigMigration).
				To(Equal(storage.ConfigMigrationDeclined))

			Expect(service.store.SetRepositoryConfigMigration(
				GinkgoT().Context(),
				storage.RepositoryConfigMigration{
					TargetID:     targetID,
					RepositoryID: "github:repository:41",
					State:        storage.ConfigMigrationNone,
				},
			)).To(Succeed())

			propose(targetID)

			Expect(stub.createdPRs).To(HaveLen(2), "the reset did not survive a sweep tick")
			Expect(repository(targetID).ConfigMigration).
				To(Equal(storage.ConfigMigrationProposed))
		})
	})

	// Seven requests to discover, and a permission nobody has granted will not
	// appear because the bot asked again twelve times an hour
	It("stops asking when GitHub refuses the push outright", func() {
		stub.repoConfig = "quiet_success: true\n"
		stub.refuseBranchPush = true
		targetID := seed()

		propose(targetID)
		Expect(repository(targetID).ConfigMigration).
			To(Equal(storage.ConfigMigrationBlocked))

		// And spends nothing more on it
		spent := len(stub.createdTrees)
		propose(targetID)
		Expect(stub.createdTrees).To(HaveLen(spent))
	})

	// A rate limit is the same request worth making again, so it must not be
	// mistaken for a permission that will never be granted
	It("keeps asking when GitHub is only busy", func() {
		stub.repoConfig = "quiet_success: true\n"
		stub.busyOnBranchPush = true
		targetID := seed()

		client, err := github.NewClient("installation-token", endpoint.URL)
		Expect(err).NotTo(HaveOccurred())
		file, err := fetchRepositoryConfig(
			GinkgoT().Context(), client, "smykla-skalski", "smyklot", nil,
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(service.proposeConfigMigration(
			GinkgoT().Context(), client, targetID,
			github.Repository{
				ID: 41, Owner: "smykla-skalski", Name: "smyklot", DefaultBranch: "main",
			},
			file,
		)).NotTo(Succeed())

		Expect(repository(targetID).ConfigMigration).To(Equal(storage.ConfigMigrationNone))
	})

	// Converting a file that does not parse would launder a broken file into a
	// valid-looking one saying something nobody wrote
	It("leaves a file it cannot read alone", func() {
		stub.repoConfig = "command_aliases: invalid\n"
		targetID := seed()
		propose(targetID)

		Expect(stub.createdPRs).To(BeEmpty())
		Expect(repository(targetID).ConfigMigration).To(Equal(storage.ConfigMigrationNone))
	})

	It("leaves a repository that has already migrated alone", func() {
		stub.repoConfigTOML = "quiet_success = true\n"
		targetID := seed()
		propose(targetID)

		Expect(stub.createdPRs).To(BeEmpty())
	})

	It("leaves a repository with no configuration file alone", func() {
		targetID := seed()
		propose(targetID)

		Expect(stub.createdPRs).To(BeEmpty())
	})

	// The migration used to sit after the sweep's stand-down check, so a
	// repository that had pinned itself to the Action was silently and
	// permanently excluded - and the Action cannot migrate it either, having
	// nowhere to remember a refusal. It keeps its legacy file for ever.
	It("reaches a repository that has stood the service down", func() {
		stub.repoConfig = "quiet_success: true\nrunner: action\n"
		targetID := seed()

		client, err := github.NewClient("installation-token", endpoint.URL)
		Expect(err).NotTo(HaveOccurred())
		Expect(service.sweepRepo(
			GinkgoT().Context(),
			client,
			targetID,
			411,
			github.Repository{
				ID: 41, Owner: "smykla-skalski", Name: "smyklot", DefaultBranch: "main",
			},
			true,
		)).To(Succeed())

		Expect(stub.createdPRs).To(HaveLen(1))
		Expect(repository(targetID).ConfigMigration).
			To(Equal(storage.ConfigMigrationProposed))
	})

	// A repository nobody has switched on is not one to open a pull request at.
	// This used to follow from where the call sat in the sweep; the call moved
	// above the stand-down check so that repositories pinned to the Action can
	// be migrated too, so the rule is the migration's own now
	It("leaves a repository nobody has switched on alone", func() {
		stub.repoConfig = "quiet_success: true\n"

		targetIDs, err := service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())

		propose(targetIDs[0])
		Expect(stub.createdPRs).To(BeEmpty())
	})

	// A repository that told the panel to ignore its file is not one to open a
	// pull request at about the formatting of that file
	It("leaves a bypassed file alone", func() {
		stub.repoConfig = "quiet_success: true\n"
		targetID := seed()

		found := repository(targetID)
		target, err := service.store.GetTarget(GinkgoT().Context(), targetID)
		Expect(err).NotTo(HaveOccurred())
		_, err = service.store.UpdateRepositorySettings(
			GinkgoT().Context(),
			storage.RepositorySettingsChange{
				TargetID:             targetID,
				RepositoryID:         found.ID,
				ActorAccountID:       target.Account.ID,
				IgnoreRepositoryFile: true,
				ExpectedRevision:     found.Revision,
				ChangedAt:            time.Now().UTC(),
			},
		)
		Expect(err).NotTo(HaveOccurred())

		propose(targetID)
		Expect(stub.createdPRs).To(BeEmpty())
	})

	// GitHub is the one that knows what became of a proposal, so the state
	// follows its answer rather than the file
	It("forgets a proposal once it merges", func() {
		stub.repoConfig = "quiet_success: true\n"
		targetID := seed()
		propose(targetID)
		Expect(repository(targetID).ConfigMigration).
			To(Equal(storage.ConfigMigrationProposed))

		stub.branchPRs = `[{"number":77,"state":"closed","merged":true}]`
		propose(targetID)

		found := repository(targetID)
		Expect(found.ConfigMigration).To(Equal(storage.ConfigMigrationNone))
		Expect(found.ConfigMigrationPR).To(BeNil())
	})

	// The refusal was about a file that is not there any more, so showing it
	// would describe a decision about nothing
	It("forgets a refusal once the repository is on TOML", func() {
		stub.repoConfig = "quiet_success: true\n"
		stub.refuseBranchPush = true
		targetID := seed()
		propose(targetID)
		Expect(repository(targetID).ConfigMigration).
			To(Equal(storage.ConfigMigrationBlocked))

		stub.repoConfig = ""
		stub.repoConfigTOML = "quiet_success = true\n"
		propose(targetID)

		Expect(repository(targetID).ConfigMigration).To(Equal(storage.ConfigMigrationNone))
	})

	// A file that stopped parsing is not a repository that changed its mind,
	// and clearing the refusal on it would have Smyklot ask again the moment
	// the file was fixed - the nagging the refusal exists to prevent
	It("keeps a refusal while the file merely stops parsing", func() {
		stub.repoConfig = "quiet_success: true\n"
		stub.refuseBranchPush = true
		targetID := seed()
		propose(targetID)
		Expect(repository(targetID).ConfigMigration).
			To(Equal(storage.ConfigMigrationBlocked))

		stub.repoConfig = "command_aliases: invalid\n"
		propose(targetID)

		Expect(repository(targetID).ConfigMigration).
			To(Equal(storage.ConfigMigrationBlocked))
	})

	It("says nothing without somewhere to remember a refusal", func() {
		stub.repoConfig = "quiet_success: true\n"
		targetID := seed()
		service.panel = nil

		propose(targetID)
		Expect(stub.createdPRs).To(BeEmpty())
	})

	// A file says what a repository chose. Writing out the settings it did not
	// choose would pin twelve defaults it never asked for
	It("proposes the settings the old file carried, and nothing else", func() {
		stub.repoConfig = "quiet_success: true\n"
		targetID := seed()
		propose(targetID)

		Expect(stub.createdBlobs).To(HaveLen(1))

		var blob struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		}
		Expect(json.Unmarshal([]byte(stub.createdBlobs[0]), &blob)).To(Succeed())
		Expect(blob.Encoding).To(Equal("base64"))

		content, err := base64.StdEncoding.DecodeString(blob.Content)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(HaveSuffix("quiet_success = true\n"))

		// The file has to say what it is to somebody who was not here when the
		// pull request arrived, and still parse as the settings it carries.
		// The schema directive goes first because that is the only line taplo
		// reads it on
		Expect(string(content)).To(HavePrefix("#:schema " + config.SchemaURL + "\n"))
		Expect(string(content)).To(ContainSubstring("# Smyklot reads this file"))

		patch, err := config.ParsePatch(config.FormatTOML, content)
		Expect(err).NotTo(HaveOccurred())
		Expect(patch.SetKeys()).To(Equal([]string{config.KeyQuietSuccess}))
	})
})
