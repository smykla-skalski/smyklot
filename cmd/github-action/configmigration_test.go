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

	// seed puts one repository in the catalog and returns its target.
	seed := func() string {
		GinkgoHelper()

		targetIDs, err := service.SyncCatalog(GinkgoT().Context())
		Expect(err).NotTo(HaveOccurred())
		Expect(targetIDs).To(HaveLen(1))

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
		It("adopts a branch it pushed before rather than pushing a second one", func() {
			seed()
			stub.migrationRef = "commitsha"
			stub.branchPRs = `[{"number":77,"state":"open","merged":false}]`

			targetID := seed()
			propose(targetID)

			Expect(stub.createdTrees).To(BeEmpty())
			Expect(stub.createdPRs).To(BeEmpty())
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

	// The proposal merged, or somebody moved the file by hand. Either way the
	// state has to stop describing a pull request that has done its job
	It("forgets a proposal once the file is no longer the legacy one", func() {
		stub.repoConfig = "quiet_success: true\n"
		targetID := seed()
		propose(targetID)
		Expect(repository(targetID).ConfigMigration).
			To(Equal(storage.ConfigMigrationProposed))

		stub.repoConfig = ""
		stub.repoConfigTOML = "quiet_success = true\n"

		propose(targetID)
		found := repository(targetID)
		Expect(found.ConfigMigration).To(Equal(storage.ConfigMigrationNone))
		Expect(found.ConfigMigrationPR).To(BeNil())
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
		Expect(string(content)).To(Equal("quiet_success = true\n"))
	})
})
