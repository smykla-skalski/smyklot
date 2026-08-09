package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
	storagesqlite "github.com/smykla-skalski/smyklot/internal/storage/sqlite"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestSQLite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SQLite Storage Suite")
}

var _ = Describe("SQLite store [Unit]", func() {
	var (
		ctx   context.Context
		store *storagesqlite.Store
		now   time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		now = time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)

		var err error
		store, err = storagesqlite.Open(ctx, filepath.Join(GinkgoT().TempDir(), "panel.db"))
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(store.Close()).To(Succeed())
	})

	It("caps sessions per account and removes expired sessions on read", func() {
		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())

		first := storage.Session{
			TokenHash: "first-token-hash",
			AccountID: account.ID,
			CreatedAt: now,
			ExpiresAt: now.Add(time.Hour),
		}
		second := storage.Session{
			TokenHash: "second-token-hash",
			AccountID: account.ID,
			CreatedAt: now.Add(time.Second),
			ExpiresAt: now.Add(time.Hour),
		}

		Expect(store.CreateSession(ctx, first, 1)).To(Succeed())
		Expect(store.CreateSession(ctx, second, 1)).To(Succeed())

		_, err := store.GetSession(ctx, first.TokenHash, now)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		live, err := store.GetSession(ctx, second.TokenHash, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(live).To(Equal(second))

		expired := second
		expired.TokenHash = "expired-token-hash"
		expired.CreatedAt = now.Add(2 * time.Second)
		expired.ExpiresAt = now.Add(-time.Second)
		Expect(store.CreateSession(ctx, expired, 2)).To(Succeed())

		_, err = store.GetSession(ctx, expired.TokenHash, now)
		Expect(errors.Is(err, storage.ErrExpired)).To(BeTrue())
		_, err = store.GetSession(ctx, expired.TokenHash, now)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
	})

	It("binds panel ownership to one immutable account", func() {
		owner := testAccount(now)
		other := owner
		other.ID = "github:2"
		other.SubjectID = "2"
		other.Login = "someone-else"
		Expect(store.UpsertAccount(ctx, owner)).To(Succeed())
		Expect(store.UpsertAccount(ctx, other)).To(Succeed())

		claimed, err := store.ClaimOwner(ctx, owner.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())
		claimed, err = store.ClaimOwner(ctx, other.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeFalse())

		allowed, err := store.IsOwner(ctx, owner.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(allowed).To(BeTrue())
		allowed, err = store.IsOwner(ctx, other.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(allowed).To(BeFalse())
	})

	It("grants a newly discovered installation to the existing owner", func() {
		owner := testAccount(now)
		Expect(store.UpsertAccount(ctx, owner)).To(Succeed())
		claimed, err := store.ClaimOwner(ctx, owner.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeTrue())

		first := testInstallation(owner, now, nil)
		second := first
		second.TargetID = "github:installation:200"
		second.InstallationID = "200"
		Expect(store.ReconcileInstallation(ctx, first)).To(Succeed())
		firstTarget, err := store.GetTarget(ctx, first.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(firstTarget.RepositoryDefaultEnabled).To(BeFalse())
		Expect(store.GrantOwnerAccess(ctx, first.TargetID, now)).To(Succeed())
		Expect(store.ReconcileInstallation(ctx, second)).To(Succeed())
		Expect(store.GrantOwnerAccess(ctx, second.TargetID, now.Add(time.Minute))).To(Succeed())

		targets, err := store.ListTargets(ctx, owner.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(targets).To(HaveLen(2))
	})

	It("preserves an OAuth profile when catalog data is less detailed", func() {
		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())

		catalogAccount := account
		catalogAccount.Login = "smykla-renamed"
		catalogAccount.DisplayName = catalogAccount.Login
		catalogAccount.AvatarURL = nil
		catalogAccount.UpdatedAt = now.Add(time.Minute)
		installation := testInstallation(catalogAccount, now.Add(time.Minute), nil)
		installation.Kind = storage.TargetUser
		Expect(store.ReconcileInstallation(ctx, installation)).To(Succeed())

		target, err := store.GetTarget(ctx, installation.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(target.Account.Login).To(Equal("smykla-renamed"))
		Expect(target.Account.DisplayName).To(Equal("Smykla Skalski"))
		Expect(target.Account.AvatarURL).To(Equal(account.AvatarURL))
	})

	It("reconciles GitHub catalog state without overwriting local controls", func() {
		account := testAccount(now)
		initial := testInstallation(account, now, []storage.RepositorySnapshot{
			testRepository("repo-1", "smykla-skalski/smyklot", false),
			testRepository("repo-2", "smykla-skalski/platform-infra", true),
		})
		Expect(store.ReconcileInstallation(ctx, initial)).To(Succeed())
		Expect(store.ReplaceAccountAccess(ctx, account.ID, []string{initial.TargetID}, now)).To(Succeed())
		allowed, err := store.CanAccessTarget(ctx, account.ID, initial.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(allowed).To(BeTrue())
		allowed, err = store.CanAccessTarget(ctx, account.ID, "missing-target")
		Expect(err).NotTo(HaveOccurred())
		Expect(allowed).To(BeFalse())

		targets, err := store.ListTargets(ctx, account.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(targets).To(HaveLen(1))
		Expect(targets[0].RepositoryCounts).To(Equal(storage.RepositoryCounts{
			Total: 2, Enabled: 0, Disabled: 2,
		}))

		quietSuccess := false
		target, err := store.UpdateTargetSettings(ctx, storage.TargetSettingsChange{
			TargetID:                 initial.TargetID,
			ActorAccountID:           account.ID,
			RepositoryDefaultEnabled: true,
			ConfigPatch:              config.Patch{QuietSuccess: &quietSuccess},
			ExpectedRevision:         1,
			ChangedAt:                now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(target.Revision).To(Equal(int64(2)))

		emptyAliases := map[string]string{}
		disabled := false
		repository, err := store.UpdateRepositorySettings(ctx, storage.RepositorySettingsChange{
			TargetID:             initial.TargetID,
			RepositoryID:         "repo-1",
			ActorAccountID:       account.ID,
			EnabledOverride:      &disabled,
			ConfigPatch:          config.Patch{CommandAliases: &emptyAliases},
			IgnoreRepositoryFile: false,
			ExpectedRevision:     1,
			ChangedAt:            now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.Revision).To(Equal(int64(2)))

		refreshed := testInstallation(account, now.Add(3*time.Minute), []storage.RepositorySnapshot{
			testRepository("repo-1", "smykla-skalski/smyklot-renamed", true),
			testRepository("repo-3", "smykla-skalski/new-repository", false),
		})
		Expect(store.ReconcileInstallation(ctx, refreshed)).To(Succeed())

		target, err = store.GetTarget(ctx, initial.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(target.RepositoryDefaultEnabled).To(BeTrue())
		Expect(target.Revision).To(Equal(int64(2)))
		Expect(target.ConfigPatch.QuietSuccess).NotTo(BeNil())
		Expect(*target.ConfigPatch.QuietSuccess).To(BeFalse())
		Expect(target.RepositoryCounts).To(Equal(storage.RepositoryCounts{
			Total: 2, Enabled: 1, Disabled: 1,
		}))

		repositories, err := store.ListRepositories(ctx, initial.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(repositories).To(HaveLen(2))
		Expect(repositories[0].FullName).To(Equal("smykla-skalski/new-repository"))
		Expect(repositories[1].FullName).To(Equal("smykla-skalski/smyklot-renamed"))
		Expect(repositories[1].Private).To(BeTrue())
		Expect(repositories[1].EnabledOverride).To(HaveValue(BeFalse()))
		Expect(repositories[1].ConfigPatch.CommandAliases).To(HaveValue(BeEmpty()))

		_, err = store.UpdateTargetSettings(ctx, storage.TargetSettingsChange{
			TargetID:         initial.TargetID,
			ActorAccountID:   account.ID,
			ExpectedRevision: 1,
			ChangedAt:        now.Add(4 * time.Minute),
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())

		audit, err := store.ListAudit(ctx, initial.TargetID, storage.AuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(audit.Items).To(HaveLen(2))
		Expect(audit.Total).To(Equal(2))
		Expect(audit.Items[0].Action).To(Equal("repository.settings.updated"))
		Expect(audit.Items[0].RepositoryFullName).To(HaveValue(Equal("smykla-skalski/smyklot")))
		Expect(audit.Items[1].Action).To(Equal("target.settings.updated"))

		accountAudit, err := store.ListAudit(ctx, initial.TargetID, storage.AuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{
				Limit: 10,
				Order: storage.HistoryOldest,
				Query: "account defaults",
			},
			Scope: storage.AuditAccount,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(accountAudit.Total).To(Equal(1))
		Expect(accountAudit.Items).To(HaveLen(1))
		Expect(accountAudit.Items[0].Action).To(Equal("target.settings.updated"))
	})

	It("retains file diagnostics while deriving the bypassed state", func() {
		account, target := seedInstallation(ctx, store, now)
		problem := "line 7: command_aliases must be a mapping"
		stateChanged, err := store.UpdateRepositoryFileState(ctx, storage.RepositoryFileState{
			TargetID:     target.TargetID,
			RepositoryID: "repo-1",
			Status:       storage.RepositoryFileInvalid,
			Error:        &problem,
			ObservedAt:   now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(stateChanged).To(BeTrue())
		stateChanged, err = store.UpdateRepositoryFileState(ctx, storage.RepositoryFileState{
			TargetID:     target.TargetID,
			RepositoryID: "repo-1",
			Status:       storage.RepositoryFileInvalid,
			Error:        &problem,
			ObservedAt:   now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(stateChanged).To(BeFalse())

		repository, err := store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ConfigFileStatus).To(Equal(storage.RepositoryFileInvalid))

		repository, err = store.UpdateRepositorySettings(ctx, storage.RepositorySettingsChange{
			TargetID:             target.TargetID,
			RepositoryID:         "repo-1",
			ActorAccountID:       account.ID,
			ConfigPatch:          config.Patch{},
			IgnoreRepositoryFile: true,
			ExpectedRevision:     repository.Revision,
			ChangedAt:            now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ConfigFileStatus).To(Equal(storage.RepositoryFileBypassed))
		Expect(repository.ConfigFileError).To(HaveValue(Equal(problem)))
	})

	It("reconciles the complete catalog without deleting removed target settings", func() {
		account := testAccount(now)
		first := testInstallation(account, now, []storage.RepositorySnapshot{
			testRepository("repo-1", "smykla-skalski/smyklot", false),
		})
		second := first
		second.TargetID = "github:installation:200"
		second.InstallationID = "200"
		second.Repositories = []storage.RepositorySnapshot{
			testRepository("repo-2", "smykla-skalski/other", false),
		}
		Expect(store.ReconcileCatalog(ctx, []storage.InstallationSnapshot{first, second})).To(Succeed())
		Expect(store.ReplaceAccountAccess(
			ctx,
			account.ID,
			[]string{first.TargetID, second.TargetID},
			now,
		)).To(Succeed())

		Expect(store.ReconcileCatalog(ctx, []storage.InstallationSnapshot{second})).To(Succeed())
		allowed, err := store.CanAccessTarget(ctx, account.ID, first.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(allowed).To(BeFalse())
		target, err := store.GetTarget(ctx, first.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(target.Available).To(BeFalse())
	})

	It("discovers a recreated repository that reuses an unavailable repository name", func() {
		account := testAccount(now)
		initial := testInstallation(account, now, []storage.RepositorySnapshot{
			testRepository("repo-1", "smykla-skalski/smyklot", false),
		})
		Expect(store.ReconcileInstallation(ctx, initial)).To(Succeed())

		recreated := testInstallation(account, now.Add(time.Minute), []storage.RepositorySnapshot{
			testRepository("repo-2", "smykla-skalski/smyklot", true),
		})
		Expect(store.ReconcileInstallation(ctx, recreated)).To(Succeed())

		repositories, err := store.ListRepositories(ctx, initial.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(repositories).To(HaveLen(1))
		Expect(repositories[0].ID).To(Equal("repo-2"))
		Expect(repositories[0].Private).To(BeTrue())
		oldRepository, err := store.GetRepository(ctx, initial.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(oldRepository.Available).To(BeFalse())
	})

	It("paginates, searches, filters, and sorts available repositories", func() {
		account := testAccount(now)
		installation := testInstallation(account, now, []storage.RepositorySnapshot{
			testRepository("repo-alpha", "smykla-skalski/alpha", false),
			testRepository("repo-beta", "smykla-skalski/beta", true),
			testRepository("repo-delta", "smykla-skalski/delta", false),
			testRepository("repo-gamma", "smykla-skalski/gamma", false),
		})
		Expect(store.ReconcileInstallation(ctx, installation)).To(Succeed())

		enabled := true
		_, err := store.UpdateRepositorySettings(ctx, storage.RepositorySettingsChange{
			TargetID:             installation.TargetID,
			RepositoryID:         "repo-beta",
			ActorAccountID:       account.ID,
			EnabledOverride:      &enabled,
			ConfigPatch:          config.Patch{QuietSuccess: &enabled},
			IgnoreRepositoryFile: false,
			ExpectedRevision:     1,
			ChangedAt:            now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		stateChanged, err := store.UpdateRepositoryFileState(ctx, storage.RepositoryFileState{
			TargetID:     installation.TargetID,
			RepositoryID: "repo-gamma",
			Status:       storage.RepositoryFileInvalid,
			ObservedAt:   now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(stateChanged).To(BeTrue())
		stateChanged, err = store.UpdateRepositoryFileState(ctx, storage.RepositoryFileState{
			TargetID:     installation.TargetID,
			RepositoryID: "repo-alpha",
			Status:       storage.RepositoryFileValid,
			ObservedAt:   now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(stateChanged).To(BeTrue())

		prefix := "!"
		_, err = store.UpdateRepositorySettings(ctx, storage.RepositorySettingsChange{
			TargetID:             installation.TargetID,
			RepositoryID:         "repo-delta",
			ActorAccountID:       account.ID,
			ConfigPatch:          config.Patch{CommandPrefix: &prefix},
			IgnoreRepositoryFile: false,
			ExpectedRevision:     1,
			ChangedAt:            now.Add(3 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		first, err := store.ListRepositoryPage(ctx, installation.TargetID, storage.RepositoryPageRequest{
			Limit: 2,
			Order: storage.RepositoryNameDescending,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Total).To(Equal(4))
		Expect(first.NextOffset).To(Equal(2))
		Expect(first.Items).To(HaveLen(2))
		Expect(first.Items[0].Name).To(Equal("gamma"))
		Expect(first.Items[1].Name).To(Equal("delta"))

		second, err := store.ListRepositoryPage(ctx, installation.TargetID, storage.RepositoryPageRequest{
			Offset: 2,
			Limit:  2,
			Order:  storage.RepositoryNameDescending,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.NextOffset).To(BeZero())
		Expect(second.Items).To(HaveLen(2))
		Expect(second.Items[0].Name).To(Equal("beta"))

		enabledOnly, err := store.ListRepositoryPage(ctx, installation.TargetID, storage.RepositoryPageRequest{
			Limit:            10,
			Order:            storage.RepositoryNameAscending,
			EffectiveEnabled: &enabled,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(enabledOnly.Total).To(Equal(1))
		Expect(enabledOnly.Items[0].Name).To(Equal("beta"))

		customOnly, err := store.ListRepositoryPage(ctx, installation.TargetID, storage.RepositoryPageRequest{
			Limit:              10,
			Order:              storage.RepositoryNameAscending,
			HasConfigOverrides: &enabled,
			ConfigOverrideKeys: []string{config.KeyQuietSuccess},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(customOnly.Total).To(Equal(1))
		Expect(customOnly.Items[0].Name).To(Equal("beta"))

		matching, err := store.ListRepositoryPage(ctx, installation.TargetID, storage.RepositoryPageRequest{
			Limit:        10,
			Order:        storage.RepositoryNewest,
			Query:        "GAM",
			FileStatuses: []storage.RepositoryFileStatus{storage.RepositoryFileInvalid},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(matching.Total).To(Equal(1))
		Expect(matching.Items[0].Name).To(Equal("gamma"))

		matching, err = store.ListRepositoryPage(ctx, installation.TargetID, storage.RepositoryPageRequest{
			Limit: 10,
			Order: storage.RepositoryNameAscending,
			FileStatuses: []storage.RepositoryFileStatus{
				storage.RepositoryFileValid,
				storage.RepositoryFileInvalid,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(matching.Total).To(Equal(2))
		Expect(matching.Items[0].Name).To(Equal("alpha"))
		Expect(matching.Items[1].Name).To(Equal("gamma"))

		matching, err = store.ListRepositoryPage(ctx, installation.TargetID, storage.RepositoryPageRequest{
			Limit: 10,
			Order: storage.RepositoryNameAscending,
			ConfigOverrideKeys: []string{
				config.KeyQuietSuccess,
				config.KeyCommandPrefix,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(matching.Total).To(Equal(2))
		Expect(matching.Items[0].Name).To(Equal("beta"))
		Expect(matching.Items[1].Name).To(Equal("delta"))
	})

	It("recovers running deliveries left by a stopped process", func() {
		_, target := seedInstallation(ctx, store, now)
		claim := storage.DeliveryClaim{
			ClaimKey:           "issue_comment:created:repo:comment:revision",
			DeliveryID:         "delivery-before-restart",
			TargetID:           target.TargetID,
			RepositoryFullName: "smykla-skalski/smyklot",
			Event:              "issue_comment",
			ClaimedAt:          now,
		}

		result, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Disposition).To(Equal(storage.DeliveryClaimAccepted))
		redelivery := claim
		redelivery.DeliveryID = "delivery-after-restart"
		result, err = store.ClaimDelivery(ctx, redelivery)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Disposition).To(Equal(storage.DeliveryClaimInProgress))

		Expect(store.RecoverRunningDeliveries(ctx, now.Add(time.Minute))).To(Succeed())
		result, err = store.ClaimDelivery(ctx, redelivery)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Disposition).To(Equal(storage.DeliveryClaimAccepted))

		failures, err := store.ListFailures(ctx, target.TargetID, storage.FailurePageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(failures.Items).To(HaveLen(1))
		Expect(failures.Items[0].DeliveryID).To(Equal(claim.DeliveryID))
		Expect(failures.Items[0].Stage).To(Equal("recovery"))
		Expect(failures.Items[0].Reason).To(Equal("service stopped before delivery finished"))
		Expect(failures.Items[0].Retryable).To(BeTrue())
	})

	It("finalizes only the claimed attempt when GitHub reuses a delivery ID", func() {
		_, target := seedInstallation(ctx, store, now)
		claim := storage.DeliveryClaim{
			ClaimKey:           "issue_comment:created:repo:comment:revision",
			DeliveryID:         "reused-delivery-id",
			TargetID:           target.TargetID,
			RepositoryFullName: "smykla-skalski/smyklot",
			Event:              "issue_comment",
			ClaimedAt:          now,
		}

		firstResult, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(firstResult.Disposition).To(Equal(storage.DeliveryClaimAccepted))
		failure := storage.DeliveryFailureChange{
			ClaimID:   firstResult.ID,
			Stage:     "github",
			Reason:    "temporary GitHub failure",
			Retryable: true,
			FailedAt:  now.Add(time.Minute),
		}
		Expect(store.FailDelivery(ctx, failure)).To(Succeed())

		secondResult, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(secondResult.Disposition).To(Equal(storage.DeliveryClaimAccepted))
		Expect(store.FailDelivery(ctx, failure)).To(Succeed())
		Expect(store.CompleteDelivery(ctx, secondResult.ID, now.Add(2*time.Minute))).To(Succeed())

		result, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Disposition).To(Equal(storage.DeliveryClaimRetained))
		failures, err := store.ListFailures(ctx, target.TargetID, storage.FailurePageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(failures.Items).To(HaveLen(1))
	})

	It("persists delivery claims, failures, pagination, and retention", func() {
		_, target := seedInstallation(ctx, store, now)
		first := storage.DeliveryClaim{
			ClaimKey:           "issue_comment:created:repo:comment:revision",
			DeliveryID:         "delivery-1",
			TargetID:           target.TargetID,
			RepositoryFullName: "smykla-skalski/smyklot",
			Event:              "issue_comment",
			ClaimedAt:          now,
		}

		firstResult, err := store.ClaimDelivery(ctx, first)
		Expect(err).NotTo(HaveOccurred())
		Expect(firstResult.Disposition).To(Equal(storage.DeliveryClaimAccepted))
		redelivery := first
		redelivery.DeliveryID = "delivery-redelivery"
		result, err := store.ClaimDelivery(ctx, redelivery)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Disposition).To(Equal(storage.DeliveryClaimInProgress))

		abandoned := first
		abandoned.ClaimKey = "issue_comment:created:repo:another-comment:revision"
		abandoned.DeliveryID = "delivery-abandoned"
		abandonedResult, err := store.ClaimDelivery(ctx, abandoned)
		Expect(err).NotTo(HaveOccurred())
		Expect(abandonedResult.Disposition).To(Equal(storage.DeliveryClaimAccepted))
		Expect(store.AbandonDelivery(ctx, abandonedResult.ID)).To(Succeed())
		abandonedResult, err = store.ClaimDelivery(ctx, abandoned)
		Expect(err).NotTo(HaveOccurred())
		Expect(abandonedResult.Disposition).To(Equal(storage.DeliveryClaimAccepted))
		Expect(store.CompleteDelivery(ctx, abandonedResult.ID, now.Add(time.Minute))).To(Succeed())
		Expect(store.CompleteDelivery(ctx, abandonedResult.ID, now.Add(time.Minute))).To(Succeed())

		Expect(store.FailDelivery(ctx, storage.DeliveryFailureChange{
			ClaimID:   firstResult.ID,
			Stage:     "config",
			Reason:    "repository configuration is invalid",
			Retryable: true,
			FailedAt:  now.Add(time.Minute),
		})).To(Succeed())
		Expect(store.FailDelivery(ctx, storage.DeliveryFailureChange{
			ClaimID:   firstResult.ID,
			Stage:     "config",
			Reason:    "repository configuration is invalid",
			Retryable: true,
			FailedAt:  now.Add(time.Minute),
		})).To(Succeed())

		second := first
		second.DeliveryID = "delivery-2"
		second.ClaimedAt = now.Add(2 * time.Minute)
		secondResult, err := store.ClaimDelivery(ctx, second)
		Expect(err).NotTo(HaveOccurred())
		Expect(secondResult.Disposition).To(Equal(storage.DeliveryClaimAccepted))
		Expect(store.FailDelivery(ctx, storage.DeliveryFailureChange{
			ClaimID:   secondResult.ID,
			Stage:     "github",
			Reason:    "temporary GitHub failure",
			Retryable: true,
			FailedAt:  now.Add(3 * time.Minute),
		})).To(Succeed())

		page, err := store.ListFailures(ctx, target.TargetID, storage.FailurePageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 1},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Total).To(Equal(2))
		Expect(page.Items[0].DeliveryID).To(Equal(second.DeliveryID))
		Expect(page.NextOffset).To(Equal(1))

		older, err := store.ListFailures(ctx, target.TargetID, storage.FailurePageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{
				Offset: page.NextOffset,
				Limit:  1,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(older.Items).To(HaveLen(1))
		Expect(older.Items[0].DeliveryID).To(Equal(first.DeliveryID))

		Expect(store.PruneDeliveries(ctx, now.Add(2*time.Minute))).To(Succeed())
		retryable := true
		matching, err := store.ListFailures(ctx, target.TargetID, storage.FailurePageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{
				Limit: 10,
				Order: storage.HistoryOldest,
				Query: "temporary GitHub",
			},
			Retryable: &retryable,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(matching.Total).To(Equal(1))
		Expect(matching.Items).To(HaveLen(1))
		Expect(matching.Items[0].DeliveryID).To(Equal(second.DeliveryID))

		remaining, err := store.ListFailures(ctx, target.TargetID, storage.FailurePageRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(remaining.Items).To(HaveLen(1))
		Expect(remaining.Items[0].DeliveryID).To(Equal(second.DeliveryID))
	})
})

func testAccount(now time.Time) storage.Account {
	avatarURL := "https://avatars.example/bart"

	return storage.Account{
		ID:          "github:1",
		Provider:    "github",
		SubjectID:   "1",
		Login:       "smykla-skalski",
		DisplayName: "Smykla Skalski",
		AvatarURL:   &avatarURL,
		UpdatedAt:   now,
	}
}

func testInstallation(
	account storage.Account,
	now time.Time,
	repositories []storage.RepositorySnapshot,
) storage.InstallationSnapshot {
	return storage.InstallationSnapshot{
		TargetID:       "github:installation:100",
		InstallationID: "100",
		Kind:           storage.TargetOrganization,
		Account:        account,
		Repositories:   repositories,
		SyncedAt:       now,
	}
}

func testRepository(id, fullName string, private bool) storage.RepositorySnapshot {
	name := fullName
	for index := len(fullName) - 1; index >= 0; index-- {
		if fullName[index] == '/' {
			name = fullName[index+1:]
			break
		}
	}

	return storage.RepositorySnapshot{ID: id, Name: name, FullName: fullName, Private: private}
}

func seedInstallation(
	ctx context.Context,
	store *storagesqlite.Store,
	now time.Time,
) (storage.Account, storage.InstallationSnapshot) {
	account := testAccount(now)
	target := testInstallation(account, now, []storage.RepositorySnapshot{
		testRepository("repo-1", "smykla-skalski/smyklot", false),
	})
	Expect(store.ReconcileInstallation(ctx, target)).To(Succeed())

	return account, target
}
