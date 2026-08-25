package storagetest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/workqueue"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// DeclareSpecs declares every engine-neutral storage expectation.
//
// Both engines run exactly these specs, so parity is something the suite
// proves rather than something a second implementation is trusted to preserve.
// Call it inside a Describe that names the engine.
func DeclareSpecs(harness Harness) {
	var (
		ctx   context.Context
		store storage.Store
		now   time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		now = time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
		store = harness.Open(ctx)
	})

	AfterEach(func() {
		Expect(store.Close()).To(Succeed())
	})

	// The concurrency specs share this store rather than opening one of their
	// own, so every spec in the suite opens exactly one.
	Context("under concurrent callers", func() {
		declareConcurrencySpecs(func() (context.Context, storage.Store, time.Time) {
			return ctx, store, now
		})
	})
	declarePendingCISpecs(func() (context.Context, storage.Store, time.Time) {
		return ctx, store, now
	})
	declareOrgSyncSpecs(func() (context.Context, storage.Store, time.Time) {
		return ctx, store, now
	})
	declareWorkQueueSpecs(func() (context.Context, storage.Store, time.Time) {
		return ctx, store, now
	})
	declareRuntimeSettingsHistorySpecs(harness, func() (context.Context, storage.Store, time.Time) {
		return ctx, store, now
	})
	declareInstallationSettingsSpecs(harness, func() (context.Context, storage.Store, time.Time) {
		return ctx, store, now
	})

	It("describes the database it is talking to", func() {
		status := store.Status(ctx)

		Expect(status.Engine).ToNot(BeEmpty())
		Expect(status.Reachable).To(BeTrue())
		Expect(status.Error).To(BeEmpty())
		Expect(status.Latency).To(BeNumerically(">", 0))

		// A release and nothing around it. An engine that reports its
		// packaging beside its version trims that in its own dialect, so
		// anything but digits and dots here means the trim was dropped and
		// every panel would print the build string.
		Expect(status.Version).To(MatchRegexp(`^\d+(\.\d+)*$`))

		// Open applies every migration before it returns a store, so a zero
		// means the query failed to find the runner's bookkeeping rather than
		// that this database has none.
		Expect(status.SchemaVersion).To(BeNumerically(">", 0))

		// Not merely "not negative": zero is what a size query that silently
		// returned nothing would report, and it is the one answer a database
		// holding a migrated schema cannot honestly give.
		Expect(status.SizeBytes).To(BeNumerically(">", 0))

		Expect(status.Connections.Max).To(BeNumerically(">", 0))
		Expect(status.Connections.Open).To(BeNumerically(">", 0))
	})

	It("reports a database it can no longer reach, and still names it", func() {
		Expect(store.Close()).To(Succeed())

		status := store.Status(ctx)

		Expect(status.Reachable).To(BeFalse())
		Expect(status.Error).ToNot(BeEmpty())

		// The engine is what this adapter is, not what a server answered, so a
		// reader is still told which database has gone rather than shown a
		// blank where its name was.
		Expect(status.Engine).ToNot(BeEmpty())
		Expect(status.Version).To(BeEmpty())
		Expect(status.SchemaVersion).To(BeZero())
		Expect(status.SizeBytes).To(BeZero())
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
		revoked, err := store.RevokeAccountSessions(
			ctx,
			account.ID,
			"banned",
			"policy breach",
			now.Add(2*time.Second),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(revoked).To(Equal([]string{second.TokenHash}))
		stored, err := store.GetSession(ctx, second.TokenHash, now.Add(3*time.Second))
		Expect(errors.Is(err, storage.ErrRevoked)).To(BeTrue())
		Expect(stored.RevokeReason).To(HaveValue(Equal("policy breach")))

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

	It("extends a live session forwards only, and never revives a dead one", func() {
		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())
		live := storage.Session{
			TokenHash: "live-token-hash",
			AccountID: account.ID,
			CreatedAt: now,
			ExpiresAt: now.Add(time.Hour),
		}
		Expect(store.CreateSession(ctx, live, 4)).To(Succeed())

		Expect(store.ExtendSession(ctx, live.TokenHash, now.Add(4*time.Hour), now)).To(Succeed())
		extended, err := store.GetSession(ctx, live.TokenHash, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(extended.ExpiresAt).To(BeTemporally("==", now.Add(4*time.Hour)))

		/* Two requests renewing at once must not be able to disagree. The one
		   carrying the earlier expiry changes nothing rather than pulling the
		   session's end back in. */
		Expect(store.ExtendSession(ctx, live.TokenHash, now.Add(2*time.Hour), now)).To(Succeed())
		unchanged, err := store.GetSession(ctx, live.TokenHash, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(unchanged.ExpiresAt).To(BeTemporally("==", now.Add(4*time.Hour)))

		// A session revoked between the read and this write stays revoked.
		revoked := storage.Session{
			TokenHash: "revoked-token-hash",
			AccountID: account.ID,
			CreatedAt: now,
			ExpiresAt: now.Add(time.Hour),
		}
		Expect(store.CreateSession(ctx, revoked, 4)).To(Succeed())
		_, err = store.RevokeAccountSessions(ctx, account.ID, "banned", "policy breach", now)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.ExtendSession(ctx, revoked.TokenHash, now.Add(4*time.Hour), now)).To(Succeed())
		_, err = store.GetSession(ctx, revoked.TokenHash, now)
		Expect(errors.Is(err, storage.ErrRevoked)).To(BeTrue())

		// Nor does one that had already run out come back.
		dead := storage.Session{
			TokenHash: "dead-token-hash",
			AccountID: account.ID,
			CreatedAt: now.Add(-2 * time.Hour),
			ExpiresAt: now.Add(-time.Hour),
		}
		Expect(store.CreateSession(ctx, dead, 4)).To(Succeed())
		Expect(store.ExtendSession(ctx, dead.TokenHash, now.Add(4*time.Hour), now)).To(Succeed())
		_, err = store.GetSession(ctx, dead.TokenHash, now)
		Expect(errors.Is(err, storage.ErrExpired)).To(BeTrue())
	})

	It("persists runtime overrides, audits them, and only shortens sessions", func() {
		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())
		session := storage.Session{
			TokenHash: "runtime-settings-session",
			AccountID: account.ID,
			CreatedAt: now,
			ExpiresAt: now.Add(12 * time.Hour),
		}
		Expect(store.CreateSession(ctx, session, 1)).To(Succeed())

		initial, err := store.GetRuntimeSettings(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(initial.Revision).To(BeZero())

		botConfig := config.Default()
		botConfig.QuietSuccess = true
		logLevel := "debug"
		pollInterval := 90 * time.Second
		pendingCIQuietPeriod := 45 * time.Second
		sessionTTL := 2 * time.Hour
		saved, err := store.SaveRuntimeSettings(ctx, storage.RuntimeSettingsChange{
			BotConfig:                     botConfig,
			LogLevel:                      &logLevel,
			PollInterval:                  &pollInterval,
			PendingCIQuietPeriod:          &pendingCIQuietPeriod,
			SessionTTL:                    &sessionTTL,
			EffectivePendingCIQuietPeriod: pendingCIQuietPeriod,
			EffectiveSessionTTL:           sessionTTL,
			ExpectedRevision:              0,
			ActorAccountID:                account.ID,
			ChangedAt:                     now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		updated := saved.Settings
		Expect(saved.CheckpointID).NotTo(BeNil())
		Expect(updated.Revision).To(Equal(int64(1)))
		Expect(updated.BotConfig).NotTo(BeNil())
		Expect(updated.BotConfig.QuietSuccess).To(BeTrue())
		Expect(updated.LogLevel).To(HaveValue(Equal(logLevel)))
		Expect(updated.PollInterval).To(HaveValue(Equal(pollInterval)))
		Expect(updated.PendingCIQuietPeriod).To(HaveValue(Equal(pendingCIQuietPeriod)))
		Expect(updated.SessionTTL).To(HaveValue(Equal(sessionTTL)))
		Expect(updated.UpdatedBy.ID).To(Equal(account.ID))

		shortened, err := store.GetSession(ctx, session.TokenHash, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(shortened.ExpiresAt).To(Equal(now.Add(2 * time.Hour)))

		longerTTL := 8 * time.Hour
		saved, err = store.SaveRuntimeSettings(ctx, storage.RuntimeSettingsChange{
			SessionTTL:                    &longerTTL,
			EffectivePendingCIQuietPeriod: 30 * time.Second,
			EffectiveSessionTTL:           longerTTL,
			ExpectedRevision:              1,
			ActorAccountID:                account.ID,
			ChangedAt:                     now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		updated = saved.Settings
		Expect(updated.Revision).To(Equal(int64(2)))
		unchanged, err := store.GetSession(ctx, session.TokenHash, now.Add(2*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(unchanged.ExpiresAt).To(Equal(now.Add(2 * time.Hour)))

		_, err = store.SaveRuntimeSettings(ctx, storage.RuntimeSettingsChange{
			EffectivePendingCIQuietPeriod: 30 * time.Second,
			EffectiveSessionTTL:           time.Hour,
			ExpectedRevision:              1,
			ActorAccountID:                account.ID,
			ChangedAt:                     now.Add(3 * time.Minute),
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())

		resetResult, err := store.SaveRuntimeSettings(ctx, storage.RuntimeSettingsChange{
			EffectivePendingCIQuietPeriod: 30 * time.Second,
			EffectiveSessionTTL:           time.Hour,
			ExpectedRevision:              2,
			ActorAccountID:                account.ID,
			ChangedAt:                     now.Add(3 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		reset := resetResult.Settings
		Expect(reset.Revision).To(Equal(int64(3)))
		Expect(reset.BotConfig).To(BeNil())
		Expect(reset.LogLevel).To(BeNil())
		Expect(reset.PollInterval).To(BeNil())
		Expect(reset.PendingCIQuietPeriod).To(BeNil())
		Expect(reset.SessionTTL).To(BeNil())
		shortest, err := store.GetSession(ctx, session.TokenHash, now.Add(3*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(shortest.ExpiresAt).To(Equal(now.Add(time.Hour)))

		audit, err := store.ListRootAudit(ctx, storage.RootAuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
			Categories:         []storage.AuditCategory{storage.AuditCategoryRuntime},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(audit.Items).To(HaveLen(3))
		Expect(audit.Items[0].Action).To(Equal("runtime.settings.saved"))
		Expect(audit.Items[0].SettingsCheckpointID).NotTo(BeNil())
	})

	It("rejects runtime states its own checkpoint reader cannot restore", func() {
		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())
		tooFast := 500 * time.Millisecond
		tooSlow := storage.MaxRuntimePollInterval + time.Second
		shortSession := 30 * time.Second
		invalidBot := config.Default()
		invalidBot.Runner = config.Runner("unknown")
		for _, change := range []storage.RuntimeSettingsChange{
			{PollInterval: &tooFast},
			{PollInterval: &tooSlow},
			{SessionTTL: &shortSession},
			{BotConfig: invalidBot},
		} {
			change.EffectivePendingCIQuietPeriod = 30 * time.Second
			change.EffectiveSessionTTL = time.Hour
			change.ActorAccountID = account.ID
			change.ChangedAt = now.Add(time.Minute)
			_, err := store.SaveRuntimeSettings(ctx, change)
			Expect(err).To(HaveOccurred())
		}
		settings, err := store.GetRuntimeSettings(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(settings.Revision).To(BeZero())
	})

	It("binds elevated writes to one Root session and notifies every Owner", func() {
		root, owner, target, session := seedElevationScenario(ctx, store, now)
		reason := "investigate a reported configuration incident"
		elevation, err := store.BeginElevation(ctx, storage.ElevationGrant{
			ID: "elevation-1", SessionTokenHash: session.TokenHash,
			RootAccountID: root.ID, TargetID: target.TargetID,
			Reason: &reason, StartedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(elevation.ExpiresAt).To(Equal(now.Add(storage.ElevationLifetime)))
		_, err = store.BeginElevation(ctx, storage.ElevationGrant{
			ID: "elevation-2", SessionTokenHash: session.TokenHash,
			RootAccountID: root.ID, TargetID: target.TargetID, StartedAt: now,
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())

		saved, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: root.ID,
			ElevationID: &elevation.ID, SessionTokenHash: session.TokenHash,
			ChangedAt: now.Add(time.Minute),
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled: true, ExpectedRevision: 1,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.Target).NotTo(BeNil())
		Expect(saved.Target.Revision).To(Equal(int64(2)))

		notifications, err := store.ListSecurityNotifications(
			ctx, owner.ID, storage.NotificationPageRequest{Limit: 10},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(notifications.Total).To(Equal(1))
		Expect(notifications.Unread).To(Equal(1))
		Expect(notifications.Items[0].ElevationID).To(Equal(elevation.ID))
		Expect(notifications.Items[0].Actor.ID).To(Equal(root.ID))
		Expect(notifications.Items[0].Target.ID).To(Equal(owner.ID))
		Expect(notifications.Items[0].Reason).To(HaveValue(Equal(reason)))

		read, err := store.MarkSecurityNotificationRead(
			ctx, owner.ID, notifications.Items[0].ID, now.Add(2*time.Minute),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(read.ReadAt).NotTo(BeNil())
		notifications, err = store.ListSecurityNotifications(ctx, owner.ID, storage.NotificationPageRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(notifications.Unread).To(BeZero())

		Expect(store.DeleteExpiredAuth(ctx, now.Add(16*time.Minute))).To(Succeed())
		_, err = store.GetElevation(ctx, session.TokenHash, target.TargetID, now.Add(16*time.Minute))
		Expect(errors.Is(err, storage.ErrExpired)).To(BeTrue())
		expiryAudit, err := store.ListRootAudit(ctx, storage.RootAuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
			Categories:         []storage.AuditCategory{storage.AuditCategoryElevation},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(expiryAudit.Items).To(ContainElement(And(
			HaveField("Action", "elevation.expired"),
			HaveField("ElevationID", HaveValue(Equal(elevation.ID))),
		)))
		ended, err := store.EndElevation(
			ctx, elevation.ID, session.TokenHash, storage.ElevationEnded, now.Add(17*time.Minute),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(ended.EndReason).To(HaveValue(Equal(storage.ElevationExpired)))
	})

	It("rejects elevation when installation Owners are stale", func() {
		root, _, target, session := seedElevationScenario(ctx, store, now)
		target.Ownership.SyncedAt = now.Add(-storage.OwnershipFreshFor - time.Second)
		target.SyncedAt = target.Ownership.SyncedAt
		Expect(store.ReconcileInstallation(ctx, target)).To(Succeed())

		_, err := store.BeginElevation(ctx, storage.ElevationGrant{
			ID: "stale-elevation", SessionTokenHash: session.TokenHash,
			RootAccountID: root.ID, TargetID: target.TargetID, StartedAt: now,
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
	})

	It("binds configuration migration resets to their Root elevation", func() {
		root, owner, target, session := seedElevationScenario(ctx, store, now)
		elevation, err := store.BeginElevation(ctx, storage.ElevationGrant{
			ID: "elevation-config-migration", SessionTokenHash: session.TokenHash,
			RootAccountID: root.ID, TargetID: target.TargetID, StartedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())

		proposal := 12
		Expect(store.SetRepositoryConfigMigration(ctx, storage.RepositoryConfigMigration{
			TargetID: target.TargetID, RepositoryID: "repo-1",
			State: storage.ConfigMigrationDeclined, PullRequest: &proposal,
		})).To(Succeed())
		Expect(store.SetRepositoryConfigMigration(ctx, storage.RepositoryConfigMigration{
			TargetID: target.TargetID, RepositoryID: "repo-1",
			State: storage.ConfigMigrationNone, ActorAccountID: &root.ID,
			ElevationID: &elevation.ID, SessionTokenHash: session.TokenHash,
			ChangedAt: now.Add(time.Minute),
		})).To(Succeed())
		// A retry after the first response was lost is a no-op, not a second
		// decision with another audit event and Owner notification.
		Expect(store.SetRepositoryConfigMigration(ctx, storage.RepositoryConfigMigration{
			TargetID: target.TargetID, RepositoryID: "repo-1",
			State: storage.ConfigMigrationNone, ActorAccountID: &root.ID,
			ElevationID: &elevation.ID, SessionTokenHash: session.TokenHash,
			ChangedAt: now.Add(2 * time.Minute),
		})).To(Succeed())

		notifications, err := store.ListSecurityNotifications(
			ctx, owner.ID, storage.NotificationPageRequest{Limit: 10},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(notifications.Items).To(ConsistOf(And(
			HaveField("Action", "repository.config_migration.reset"),
			HaveField("ElevationID", elevation.ID),
		)))
		audit, err := store.ListRootAudit(ctx, storage.RootAuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
			Categories:         []storage.AuditCategory{storage.AuditCategoryConfiguration},
			TargetID:           &target.TargetID,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(audit.Items).To(ContainElement(And(
			HaveField("Action", "repository.config_migration.reset"),
			HaveField("ElevationID", HaveValue(Equal(elevation.ID))),
		)))
		Expect(audit.Items).To(HaveLen(1))

		Expect(store.SetRepositoryConfigMigration(ctx, storage.RepositoryConfigMigration{
			TargetID: target.TargetID, RepositoryID: "repo-1",
			State: storage.ConfigMigrationBlocked,
		})).To(Succeed())
		Expect(store.SetRepositoryConfigMigration(ctx, storage.RepositoryConfigMigration{
			TargetID: target.TargetID, RepositoryID: "repo-1",
			State: storage.ConfigMigrationNone, ActorAccountID: &root.ID,
			ElevationID: &elevation.ID, SessionTokenHash: session.TokenHash,
			ChangedAt: now.Add(3 * time.Minute),
		})).To(Succeed())
		repository, err := store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ConfigMigration).To(Equal(storage.ConfigMigrationNone))

		proposal = 13
		Expect(store.SetRepositoryConfigMigration(ctx, storage.RepositoryConfigMigration{
			TargetID: target.TargetID, RepositoryID: "repo-1",
			State: storage.ConfigMigrationProposed, PullRequest: &proposal,
		})).To(Succeed())
		err = store.SetRepositoryConfigMigration(ctx, storage.RepositoryConfigMigration{
			TargetID: target.TargetID, RepositoryID: "repo-1",
			State: storage.ConfigMigrationNone, ActorAccountID: &root.ID,
			ElevationID: &elevation.ID, SessionTokenHash: session.TokenHash,
			ChangedAt: now.Add(4 * time.Minute),
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
		repository, err = store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ConfigMigration).To(Equal(storage.ConfigMigrationProposed))

		_, err = store.EndElevation(
			ctx, elevation.ID, session.TokenHash, storage.ElevationRevoked, now.Add(5*time.Minute),
		)
		Expect(err).NotTo(HaveOccurred())
		proposal = 14
		Expect(store.SetRepositoryConfigMigration(ctx, storage.RepositoryConfigMigration{
			TargetID: target.TargetID, RepositoryID: "repo-1",
			State: storage.ConfigMigrationDeclined, PullRequest: &proposal,
		})).To(Succeed())
		err = store.SetRepositoryConfigMigration(ctx, storage.RepositoryConfigMigration{
			TargetID: target.TargetID, RepositoryID: "repo-1",
			State: storage.ConfigMigrationNone, ActorAccountID: &root.ID,
			ElevationID: &elevation.ID, SessionTokenHash: session.TokenHash,
			ChangedAt: now.Add(6 * time.Minute),
		})
		Expect(errors.Is(err, storage.ErrExpired)).To(BeTrue())
		repository, err = store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ConfigMigration).To(Equal(storage.ConfigMigrationDeclined))
	})

	It("records elevated access and invitation writes with Owner notifications", func() {
		root, owner, target, session := seedElevationScenario(ctx, store, now)
		elevation, err := store.BeginElevation(ctx, storage.ElevationGrant{
			ID: "elevation-access", SessionTokenHash: session.TokenHash,
			RootAccountID: root.ID, TargetID: target.TargetID, StartedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())

		subject := owner
		subject.ID = "github:member"
		subject.SubjectID = "member"
		subject.Login = "member"
		subject.DisplayName = "Installation Member"
		Expect(store.UpsertAccount(ctx, subject)).To(Succeed())
		_, err = store.CreatePanelUser(ctx, storage.PanelUserCreate{
			AccountID: subject.ID, ActorAccountID: root.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())

		role := storage.InstallationRoleEditor
		_, err = store.SetTargetAccess(ctx, storage.TargetAccessChange{
			TargetID: target.TargetID, SubjectAccountID: subject.ID, ActorAccountID: root.ID,
			ElevationID: &elevation.ID, SessionTokenHash: session.TokenHash,
			Role: &role, ChangedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		invitee := subject
		invitee.ID = "github:invitee"
		invitee.SubjectID = "invitee"
		invitee.Login = "invitee"
		invitee.DisplayName = "Invited Member"
		Expect(store.UpsertAccount(ctx, invitee)).To(Succeed())
		viewer := storage.InstallationRoleViewer
		_, err = store.CreateInvitation(ctx, storage.InvitationCreate{
			ID: "elevated-invitation", TokenHash: "elevated-invitation-token",
			AccountID: invitee.ID, TargetID: &target.TargetID, Role: &viewer,
			ElevationID: &elevation.ID, SessionTokenHash: session.TokenHash,
			ExpiresAt: now.Add(24 * time.Hour), CreatedByAccount: root.ID,
			CreatedAt: now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		notifications, err := store.ListSecurityNotifications(
			ctx, owner.ID, storage.NotificationPageRequest{Limit: 10},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(notifications.Total).To(Equal(2))
		Expect(notifications.Items).To(ConsistOf(
			HaveField("Action", "target.access.updated"),
			HaveField("Action", "invitation.created"),
		))
		for _, notification := range notifications.Items {
			Expect(notification.ElevationID).To(Equal(elevation.ID))
		}

		audit, err := store.ListRootAudit(ctx, storage.RootAuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 20},
			Categories:         []storage.AuditCategory{storage.AuditCategoryAccess},
		})
		Expect(err).NotTo(HaveOccurred())
		for _, action := range []string{"target.access.updated", "invitation.created"} {
			Expect(audit.Items).To(ContainElement(And(
				HaveField("Action", action),
				HaveField("ElevationID", HaveValue(Equal(elevation.ID))),
			)))
		}
		notificationAudit, err := store.ListRootAudit(ctx, storage.RootAuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 20},
			Categories:         []storage.AuditCategory{storage.AuditCategoryNotification},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(notificationAudit.Items).To(HaveLen(2))
		for _, event := range notificationAudit.Items {
			Expect(event.Action).To(Equal("owner.notification.created"))
			Expect(event.Subject).NotTo(BeNil())
			Expect(event.Subject.ID).To(Equal(owner.ID))
			Expect(event.ElevationID).To(HaveValue(Equal(elevation.ID)))
		}
	})

	It("summarizes Root operational state", func() {
		root, _, target, session := seedElevationScenario(ctx, store, now)
		_, err := store.BeginElevation(ctx, storage.ElevationGrant{
			ID: "overview-elevation", SessionTokenHash: session.TokenHash,
			RootAccountID: root.ID, TargetID: target.TargetID, StartedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())

		claim, err := store.ClaimDelivery(ctx, storage.DeliveryClaim{
			ClaimKey: "github:overview:failure", DeliveryID: "overview-failure",
			TargetID: target.TargetID, RepositoryFullName: "smykla-skalski/smyklot",
			Event: "issue_comment", ClaimedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(store.FailDelivery(ctx, storage.DeliveryFailureChange{
			ClaimID: claim.ID, Stage: "github", Reason: "provider timeout",
			Retryable: true, FailedAt: now.Add(time.Minute),
		})).To(Succeed())

		overview, err := store.GetRootOverview(ctx, root.ID, now.Add(2*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(overview.InstallationCount).To(Equal(1))
		Expect(overview.RepositoryCount).To(Equal(1))
		Expect(overview.EnabledRepositoryCount).To(BeZero())
		Expect(overview.OwnershipFresh).To(Equal(1))
		Expect(overview.OwnershipStale).To(BeZero())
		Expect(overview.ActiveElevations).To(Equal(1))
		Expect(overview.RecentFailures).To(HaveLen(1))
		Expect(overview.RecentFailures[0].Failure.DeliveryID).To(Equal("overview-failure"))
		Expect(overview.RecentFailures[0].Target.Login).To(Equal(target.Account.Login))
		targets, err := store.ListRootTargets(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(targets).To(HaveLen(1))
		Expect(targets[0].DeliveryHealth.Failed).To(Equal(1))
		Expect(targets[0].DeliveryHealth.LastFailureAt).To(
			HaveValue(Equal(now.Add(time.Minute))),
		)

		audit, err := store.ListRootAudit(ctx, storage.RootAuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
			Categories:         []storage.AuditCategory{storage.AuditCategoryElevation},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(audit.Total).To(Equal(1))
		Expect(audit.Items[0].Action).To(Equal("elevation.started"))
		Expect(audit.Items[0].Target).NotTo(BeNil())
		Expect(audit.Items[0].Target.Login).To(Equal(target.Account.Login))

		failures, err := store.ListRootFailures(ctx, storage.FailurePageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 10, Query: "provider"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(failures.Total).To(Equal(1))
		Expect(failures.Items[0].Failure.DeliveryID).To(Equal("overview-failure"))
	})

	It("rolls back an elevated write when Owner notifications cannot commit", func() {
		root, _, target, session := seedElevationScenario(ctx, store, now)
		elevation, err := store.BeginElevation(ctx, storage.ElevationGrant{
			ID: "elevation-rollback", SessionTokenHash: session.TokenHash,
			RootAccountID: root.ID, TargetID: target.TargetID, StartedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())

		harness.RejectSecurityNotifications(ctx)

		_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: root.ID,
			ElevationID: &elevation.ID, SessionTokenHash: session.TokenHash,
			ChangedAt: now.Add(time.Minute),
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled: true, ExpectedRevision: 1,
			},
		})
		Expect(err).To(HaveOccurred())
		unchanged, err := store.GetTarget(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(unchanged.RepositoryDefaultEnabled).To(BeFalse())
		Expect(unchanged.Revision).To(Equal(int64(1)))
		audit, err := store.ListAudit(ctx, target.TargetID, storage.AuditPageRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(audit.Total).To(BeZero())
	})

	It("reassigns the singleton Super Root and demotes the former one", func() {
		owner := testAccount(now)
		other := owner
		other.ID = "github:2"
		other.SubjectID = "2"
		other.Login = "someone-else"
		Expect(store.UpsertAccount(ctx, owner)).To(Succeed())
		Expect(store.UpsertAccount(ctx, other)).To(Succeed())

		Expect(store.ReconcileSuperRoot(ctx, owner.ID, now)).To(Succeed())
		panelUser, err := store.GetPanelUser(ctx, owner.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(panelUser.SystemRole).To(Equal(storage.SystemRoleSuperRoot))
		Expect(store.ReconcileSuperRoot(ctx, owner.ID, now.Add(time.Second))).To(Succeed())
		unchanged, err := store.GetPanelUser(ctx, owner.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(unchanged.Revision).To(Equal(panelUser.Revision))

		Expect(store.ReconcileSuperRoot(ctx, other.ID, now.Add(time.Minute))).To(Succeed())
		former, err := store.GetPanelUser(ctx, owner.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(former.SystemRole).To(Equal(storage.SystemRoleRoot))
		current, err := store.GetPanelUser(ctx, other.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(current.SystemRole).To(Equal(storage.SystemRoleSuperRoot))
	})

	It("makes newly discovered installations visible to the root owner", func() {
		owner := testAccount(now)
		Expect(store.UpsertAccount(ctx, owner)).To(Succeed())
		Expect(store.ReconcileSuperRoot(ctx, owner.ID, now)).To(Succeed())

		first := testInstallation(owner, now, nil)
		second := first
		second.TargetID = "github:installation:200"
		second.InstallationID = "200"
		Expect(store.ReconcileInstallation(ctx, first)).To(Succeed())
		firstTarget, err := store.GetTarget(ctx, first.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(firstTarget.RepositoryDefaultEnabled).To(BeFalse())
		Expect(store.ReconcileInstallation(ctx, second)).To(Succeed())

		targets, err := store.ListTargets(ctx, owner.ID, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(targets).To(HaveLen(2))
	})

	It("activates fresh derived Owners without bypassing soft removal", func() {
		owner := testAccount(now)
		Expect(store.ReconcileInstallation(ctx, testInstallation(owner, now, nil))).To(Succeed())
		activated, err := store.ActivateDerivedOwner(ctx, owner.ID, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(activated).To(BeTrue())
		user, err := store.GetPanelUser(ctx, owner.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(user.Status).To(Equal(storage.PanelUserActive))
		Expect(user.SystemRole).To(Equal(storage.SystemRoleNone))

		removed, err := store.UpdatePanelUser(ctx, storage.PanelUserChange{
			AccountID: owner.ID, ActorAccountID: owner.ID,
			Status:           storage.PanelUserRemoved,
			ExpectedRevision: user.Revision, ChangedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(removed.Status).To(Equal(storage.PanelUserRemoved))
		activated, err = store.ActivateDerivedOwner(ctx, owner.ID, now.Add(2*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(activated).To(BeFalse())
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
		installation.Ownership = storage.OwnershipSnapshot{}
		Expect(store.ReconcileInstallation(ctx, installation)).To(Succeed())

		target, err := store.GetTarget(ctx, installation.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(target.Account.Login).To(Equal("smykla-renamed"))
		Expect(target.Account.DisplayName).To(Equal("Smykla Skalski"))
		Expect(target.Account.AvatarURL).To(Equal(account.AvatarURL))
	})

	It("finds a repository by name within its own installation only", func() {
		/* Panel addresses name repositories the way people do - `api-gateway`
		   rather than an id - so the lookup accepts both. Two organizations very
		   often own a repository of the same name, and the name must never reach
		   across from one to the other: the lookup is scoped to the installation
		   asking, and there is no query that is not. */
		first := testAccount(now)
		second := derive(first, "second-org", "Second Organization")
		Expect(store.UpsertAccount(ctx, first)).To(Succeed())
		Expect(store.UpsertAccount(ctx, second)).To(Succeed())

		firstInstallation := testInstallation(first, now, []storage.RepositorySnapshot{
			testRepository("first-api", "smykla-skalski/api-gateway", false),
		})
		secondInstallation := testInstallation(second, now, []storage.RepositorySnapshot{
			testRepository("second-api", "second-org/api-gateway", false),
		})
		secondInstallation.TargetID = "github:installation:200"
		secondInstallation.InstallationID = "200"
		Expect(store.ReconcileInstallation(ctx, firstInstallation)).To(Succeed())
		Expect(store.ReconcileInstallation(ctx, secondInstallation)).To(Succeed())

		byName, err := store.GetRepository(ctx, firstInstallation.TargetID, "api-gateway")
		Expect(err).NotTo(HaveOccurred())
		Expect(byName.ID).To(Equal("first-api"))
		Expect(byName.FullName).To(Equal("smykla-skalski/api-gateway"))

		// The same name, asked of the other installation, is the other repository.
		otherByName, err := store.GetRepository(ctx, secondInstallation.TargetID, "api-gateway")
		Expect(err).NotTo(HaveOccurred())
		Expect(otherByName.ID).To(Equal("second-api"))
		Expect(otherByName.FullName).To(Equal("second-org/api-gateway"))

		// An id belonging to the other installation is not found here either.
		_, err = store.GetRepository(ctx, firstInstallation.TargetID, "second-api")
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())

		byID, err := store.GetRepository(ctx, firstInstallation.TargetID, "first-api")
		Expect(err).NotTo(HaveOccurred())
		Expect(byID.Name).To(Equal("api-gateway"))

		_, err = store.GetRepository(ctx, firstInstallation.TargetID, "no-such-repository")
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
	})

	It("prefers the id when a repository is named like another's id", func() {
		/* A repository may legitimately be called "1234", and another may carry
		   1234 as its id. Both readings are of the same installation, so scoping
		   does not settle it - the id wins, because that is the identifier the
		   panel itself passes and a name that looks like one is the coincidence. */
		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())
		installation := testInstallation(account, now, []storage.RepositorySnapshot{
			testRepository("1234", "smykla-skalski/actual-id", false),
			testRepository("other-id", "smykla-skalski/1234", false),
		})
		Expect(store.ReconcileInstallation(ctx, installation)).To(Succeed())

		found, err := store.GetRepository(ctx, installation.TargetID, "1234")
		Expect(err).NotTo(HaveOccurred())
		Expect(found.ID).To(Equal("1234"))
		Expect(found.Name).To(Equal("actual-id"))
	})

	It("reconciles GitHub catalog state without overwriting local controls", func() {
		account := testAccount(now)
		initial := testInstallation(account, now, []storage.RepositorySnapshot{
			testRepository("repo-1", "smykla-skalski/smyklot", false),
			testRepository("repo-2", "smykla-skalski/platform-infra", true),
		})
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())
		Expect(store.ReconcileSuperRoot(ctx, account.ID, now)).To(Succeed())
		Expect(store.ReconcileInstallation(ctx, initial)).To(Succeed())
		access, err := store.ResolveTargetAccess(ctx, account.ID, initial.TargetID, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Role).To(Equal(storage.InstallationRoleOwner))
		_, err = store.ResolveTargetAccess(ctx, account.ID, "missing-target", now)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())

		targets, err := store.ListTargets(ctx, account.ID, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(targets).To(HaveLen(1))
		Expect(targets[0].RepositoryCounts).To(Equal(storage.RepositoryCounts{
			Total: 2, Enabled: 0, Disabled: 2,
		}))

		quietSuccess := false
		saved, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: initial.TargetID, ActorAccountID: account.ID,
			ChangedAt: now.Add(time.Minute),
			Target: &storage.InstallationTargetSettingsChange{
				RepositoryDefaultEnabled: true,
				ConfigPatch:              config.Patch{QuietSuccess: &quietSuccess},
				ExpectedRevision:         1,
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.Target).NotTo(BeNil())
		target := *saved.Target
		Expect(target.Revision).To(Equal(int64(2)))

		emptyAliases := map[string]string{}
		disabled := false
		saved, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: initial.TargetID, ActorAccountID: account.ID,
			ChangedAt: now.Add(2 * time.Minute),
			Repositories: []storage.InstallationRepositorySettingsChange{{
				RepositoryID: "repo-1", EnabledOverride: &disabled,
				ConfigPatch:          config.Patch{CommandAliases: &emptyAliases},
				IgnoreRepositoryFile: false, ExpectedRevision: 1,
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.Repositories).To(HaveLen(1))
		repository := saved.Repositories[0]
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
		Expect(repositories[1].DefaultBranch).To(Equal("main"))
		Expect(repositories[1].EnabledOverride).To(HaveValue(BeFalse()))
		Expect(repositories[1].ConfigPatch.CommandAliases).To(HaveValue(BeEmpty()))

		_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: initial.TargetID, ActorAccountID: account.ID,
			ChangedAt: now.Add(4 * time.Minute),
			Target:    &storage.InstallationTargetSettingsChange{ExpectedRevision: 1},
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())

		audit, err := store.ListAudit(ctx, initial.TargetID, storage.AuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(audit.Items).To(HaveLen(2))
		Expect(audit.Total).To(Equal(2))
		for _, item := range audit.Items {
			Expect(item.Action).To(Equal("installation.settings.saved"))
			Expect(item.RepositoryFullName).To(BeNil())
		}

		accountAudit, err := store.ListAudit(ctx, initial.TargetID, storage.AuditPageRequest{
			HistoryPageRequest: storage.HistoryPageRequest{
				Limit: 10,
				Order: storage.HistoryOldest,
				Query: "Saved 1 installation settings",
			},
			Scope: storage.AuditAccount,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(accountAudit.Total).To(Equal(2))
		Expect(accountAudit.Items).To(HaveLen(2))
	})

	It("records only meaningful ownership synchronization changes", func() {
		account := testAccount(now)
		installation := testInstallation(account, now, nil)
		Expect(store.ReconcileInstallation(ctx, installation)).To(Succeed())

		listOwnershipAudit := func() storage.RootAuditPage {
			page, err := store.ListRootAudit(ctx, storage.RootAuditPageRequest{
				HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
				Categories:         []storage.AuditCategory{storage.AuditCategoryOwnership},
			})
			Expect(err).NotTo(HaveOccurred())

			return page
		}

		audit := listOwnershipAudit()
		Expect(audit.Total).To(Equal(1))
		Expect(audit.Items[0].Action).To(Equal("ownership.synced"))
		Expect(audit.Items[0].Actor.ID).To(Equal("smyklot:system"))

		installation.SyncedAt = now.Add(time.Minute)
		installation.Ownership.SyncedAt = installation.SyncedAt
		Expect(store.ReconcileInstallation(ctx, installation)).To(Succeed())
		Expect(listOwnershipAudit().Total).To(Equal(1))

		detail := "Members permission approval is required"
		installation.Ownership.Status = storage.OwnershipStatusPermissionPending
		installation.Ownership.Detail = &detail
		installation.Ownership.Owners = nil
		installation.SyncedAt = now.Add(2 * time.Minute)
		installation.Ownership.SyncedAt = installation.SyncedAt
		Expect(store.ReconcileInstallation(ctx, installation)).To(Succeed())

		audit = listOwnershipAudit()
		Expect(audit.Total).To(Equal(2))
		Expect(audit.Items[0].Action).To(Equal("ownership.permission_pending"))
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

		saved, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: account.ID,
			ChangedAt: now.Add(2 * time.Minute),
			Repositories: []storage.InstallationRepositorySettingsChange{{
				RepositoryID: "repo-1", ConfigPatch: config.Patch{},
				IgnoreRepositoryFile: true, ExpectedRevision: repository.Revision,
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.Repositories).To(HaveLen(1))
		repository = saved.Repositories[0]
		Expect(repository.ConfigFileStatus).To(Equal(storage.RepositoryFileBypassed))
		Expect(repository.ConfigFileError).To(HaveValue(Equal(problem)))
	})

	// Discovery looks in four places plus a panel-chosen one, so the status
	// alone stopped saying which file it was describing - and a repository that
	// migrated to TOML and left the YAML behind has a file it believes is in
	// charge and is not
	It("records which file was read and which were passed over", func() {
		_, target := seedInstallation(ctx, store, now)

		stateChanged, err := store.UpdateRepositoryFileState(ctx, storage.RepositoryFileState{
			TargetID:     target.TargetID,
			RepositoryID: "repo-1",
			Status:       storage.RepositoryFileValid,
			Path:         ".smyklot.toml",
			Superseded:   []string{".github/smyklot.yaml"},
			ObservedAt:   now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(stateChanged).To(BeTrue())

		repository, err := store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ConfigFilePath).To(Equal(".smyklot.toml"))
		Expect(repository.ConfigFileSuperseded).To(ConsistOf(".github/smyklot.yaml"))

		// The panel is told to refresh on the strength of this, and a
		// repository that moved its file changes neither status nor patch
		stateChanged, err = store.UpdateRepositoryFileState(ctx, storage.RepositoryFileState{
			TargetID:     target.TargetID,
			RepositoryID: "repo-1",
			Status:       storage.RepositoryFileValid,
			Path:         ".github/.smyklot.toml",
			ObservedAt:   now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(stateChanged).To(BeTrue())

		repository, err = store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ConfigFilePath).To(Equal(".github/.smyklot.toml"))
		Expect(repository.ConfigFileSuperseded).To(BeEmpty())
	})

	// A pull request somebody closed is a refusal, and asking again every sweep
	// tick would be the bot arguing with a decision a person already made
	It("remembers that a configuration migration was refused", func() {
		_, target := seedInstallation(ctx, store, now)

		repository, err := store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ConfigMigration).To(Equal(storage.ConfigMigrationNone))
		Expect(repository.ConfigMigrationPR).To(BeNil())

		number := 12
		Expect(store.SetRepositoryConfigMigration(ctx, storage.RepositoryConfigMigration{
			TargetID:     target.TargetID,
			RepositoryID: "repo-1",
			State:        storage.ConfigMigrationProposed,
			PullRequest:  &number,
		})).To(Succeed())

		repository, err = store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ConfigMigration).To(Equal(storage.ConfigMigrationProposed))
		Expect(repository.ConfigMigrationPR).To(HaveValue(Equal(number)))

		Expect(store.SetRepositoryConfigMigration(ctx, storage.RepositoryConfigMigration{
			TargetID:     target.TargetID,
			RepositoryID: "repo-1",
			State:        storage.ConfigMigrationDeclined,
			PullRequest:  &number,
		})).To(Succeed())

		repository, err = store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ConfigMigration).To(Equal(storage.ConfigMigrationDeclined))

		// Panel-owned settings are not touched by it, and it does not contend
		// for their revision: a sweep tick must not fail somebody's save
		saved, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: target.TargetID, ActorAccountID: testAccount(now).ID,
			ChangedAt: now.Add(time.Minute),
			Repositories: []storage.InstallationRepositorySettingsChange{{
				RepositoryID: "repo-1", ConfigPatch: config.Patch{},
				ExpectedRevision: repository.Revision,
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(saved.Repositories).To(HaveLen(1))
		settings := saved.Repositories[0]
		Expect(settings.ConfigMigration).To(Equal(storage.ConfigMigrationDeclined))

		// GitHub refusing the push is durable for a different reason than
		// somebody closing the pull request, and both engines have to accept it
		Expect(store.SetRepositoryConfigMigration(ctx, storage.RepositoryConfigMigration{
			TargetID:     target.TargetID,
			RepositoryID: "repo-1",
			State:        storage.ConfigMigrationBlocked,
		})).To(Succeed())

		repository, err = store.GetRepository(ctx, target.TargetID, "repo-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.ConfigMigration).To(Equal(storage.ConfigMigrationBlocked))
		Expect(repository.ConfigMigrationPR).To(BeNil())

		Expect(store.SetRepositoryConfigMigration(ctx, storage.RepositoryConfigMigration{
			TargetID:     target.TargetID,
			RepositoryID: "absent",
			State:        storage.ConfigMigrationNone,
		})).To(MatchError(storage.ErrNotFound))
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
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())
		Expect(store.ReconcileSuperRoot(ctx, account.ID, now)).To(Succeed())
		Expect(store.ReconcileCatalog(ctx, []storage.InstallationSnapshot{first, second})).To(Succeed())

		Expect(store.ReconcileCatalog(ctx, []storage.InstallationSnapshot{second})).To(Succeed())
		_, err := store.ResolveTargetAccess(ctx, account.ID, first.TargetID, now)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())
		target, err := store.GetTarget(ctx, first.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(target.Available).To(BeFalse())
	})

	It("resolves installation roles and suspension in order", func() {
		owner, target := seedInstallation(ctx, store, now)
		Expect(store.UpsertAccount(ctx, owner)).To(Succeed())
		Expect(store.ReconcileSuperRoot(ctx, owner.ID, now)).To(Succeed())

		viewer := owner
		viewer.ID = "github:viewer"
		viewer.SubjectID = "viewer"
		viewer.Login = "viewer"
		Expect(store.UpsertAccount(ctx, viewer)).To(Succeed())
		created, err := store.CreatePanelUser(ctx, storage.PanelUserCreate{
			AccountID:      viewer.ID,
			ActorAccountID: owner.ID,
			ChangedAt:      now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(created.Status).To(Equal(storage.PanelUserActive))

		access, err := store.ResolveTargetAccess(ctx, viewer.ID, target.TargetID, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Role).To(Equal(storage.InstallationRoleNone))
		Expect(access.Source).To(Equal(storage.AccessSourceDenied))
		Expect(access.Capabilities.Read).To(BeFalse())
		Expect(access.Capabilities.Write).To(BeFalse())

		override, err := store.SetTargetAccess(ctx, storage.TargetAccessChange{
			TargetID:         target.TargetID,
			SubjectAccountID: viewer.ID,
			ActorAccountID:   owner.ID,
			Role:             rolePointer(storage.InstallationRoleEditor),
			ExpectedRevision: 0,
			ChangedAt:        now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(override.Revision).To(Equal(int64(1)))
		access, err = store.ResolveTargetAccess(ctx, viewer.ID, target.TargetID, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Role).To(Equal(storage.InstallationRoleEditor))
		Expect(access.Source).To(Equal(storage.AccessSourceTarget))
		Expect(access.Capabilities.Write).To(BeTrue())

		reason := "security review"
		_, err = store.SetTargetAccess(ctx, storage.TargetAccessChange{
			TargetID:         target.TargetID,
			SubjectAccountID: viewer.ID,
			ActorAccountID:   owner.ID,
			Role:             rolePointer(storage.InstallationRoleEditor),
			Suspended:        true,
			SuspensionReason: &reason,
			ExpectedRevision: override.Revision,
			ChangedAt:        now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		access, err = store.ResolveTargetAccess(ctx, viewer.ID, target.TargetID, now.Add(2*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Role).To(Equal(storage.InstallationRoleNone))
		Expect(access.Source).To(Equal(storage.AccessSourceSuspended))
		Expect(access.SuspensionReason).To(HaveValue(Equal(reason)))
	})

	It("fails regular access closed when ownership is stale or unavailable", func() {
		owner, target := seedInstallation(ctx, store, now)
		Expect(store.UpsertAccount(ctx, owner)).To(Succeed())
		Expect(store.ReconcileSuperRoot(ctx, owner.ID, now)).To(Succeed())

		other := owner
		other.ID = "github:user:other-owner"
		other.SubjectID = "other-owner"
		other.Login = "other-owner"
		nonOwned := target
		nonOwned.TargetID = "github:installation:non-owned"
		nonOwned.InstallationID = "200"
		nonOwned.Ownership.Owners = []storage.Account{other}
		Expect(store.ReconcileInstallation(ctx, nonOwned)).To(Succeed())

		targets, err := store.ListTargets(ctx, owner.ID, now.Add(storage.OwnershipFreshFor))
		Expect(err).NotTo(HaveOccurred())
		Expect(targets).To(HaveLen(1))
		Expect(targets[0].ID).To(Equal(target.TargetID))
		access, err := store.ResolveTargetAccess(
			ctx, owner.ID, target.TargetID, now.Add(storage.OwnershipFreshFor),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Role).To(Equal(storage.InstallationRoleOwner))

		staleAt := now.Add(storage.OwnershipFreshFor + time.Second)
		targets, err = store.ListTargets(ctx, owner.ID, staleAt)
		Expect(err).NotTo(HaveOccurred())
		Expect(targets).To(BeEmpty())
		access, err = store.ResolveTargetAccess(ctx, owner.ID, target.TargetID, staleAt)
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Role).To(Equal(storage.InstallationRoleNone))
		Expect(access.Source).To(Equal(storage.AccessSourceDenied))

		detail := "organization Members read permission requires installation approval"
		target.Ownership = storage.OwnershipSnapshot{
			Source:   storage.OwnershipSourceOrganizationAdmin,
			Status:   storage.OwnershipStatusPermissionPending,
			Detail:   &detail,
			SyncedAt: staleAt,
		}
		target.SyncedAt = staleAt
		Expect(store.ReconcileInstallation(ctx, target)).To(Succeed())
		diagnostic, err := store.GetTarget(ctx, target.TargetID)
		Expect(err).NotTo(HaveOccurred())
		Expect(diagnostic.Available).To(BeTrue())
		Expect(diagnostic.Ownership.Status).To(Equal(storage.OwnershipStatusPermissionPending))
		access, err = store.ResolveTargetAccess(ctx, owner.ID, target.TargetID, staleAt)
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Role).To(Equal(storage.InstallationRoleNone))
	})

	It("lists, bans, removes, and re-adds panel users without losing identity", func() {
		owner, target := seedInstallation(ctx, store, now)
		Expect(store.UpsertAccount(ctx, owner)).To(Succeed())
		Expect(store.ReconcileSuperRoot(ctx, owner.ID, now)).To(Succeed())

		viewer := owner
		viewer.ID = "github:user:managed"
		viewer.SubjectID = "managed"
		viewer.Login = "managed-user"
		Expect(store.UpsertAccount(ctx, viewer)).To(Succeed())
		managed, err := store.CreatePanelUser(ctx, storage.PanelUserCreate{
			AccountID:      viewer.ID,
			ActorAccountID: owner.ID,
			ChangedAt:      now,
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.SetTargetAccess(ctx, storage.TargetAccessChange{
			TargetID:         target.TargetID,
			SubjectAccountID: viewer.ID,
			ActorAccountID:   owner.ID,
			Role:             rolePointer(storage.InstallationRoleEditor),
			ExpectedRevision: 0,
			ChangedAt:        now,
		})
		Expect(err).NotTo(HaveOccurred())

		users, err := store.ListPanelUsers(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(users).To(HaveLen(2))
		targetUsers, err := store.ListTargetPanelUsers(ctx, target.TargetID, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(targetUsers).To(HaveLen(2))
		managedTarget := targetUserByID(targetUsers, viewer.ID)
		Expect(managedTarget.Override).NotTo(BeNil())
		Expect(managedTarget.Access.Role).To(Equal(storage.InstallationRoleEditor))

		reason := "credential review"
		managed, err = store.UpdatePanelUser(ctx, storage.PanelUserChange{
			AccountID:        viewer.ID,
			ActorAccountID:   owner.ID,
			Status:           storage.PanelUserBanned,
			BanReason:        &reason,
			ExpectedRevision: managed.Revision,
			ChangedAt:        now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(managed.Status).To(Equal(storage.PanelUserBanned))
		Expect(managed.BanReason).To(HaveValue(Equal(reason)))

		managed, err = store.UpdatePanelUser(ctx, storage.PanelUserChange{
			AccountID:        viewer.ID,
			ActorAccountID:   owner.ID,
			Status:           storage.PanelUserRemoved,
			ExpectedRevision: managed.Revision,
			ChangedAt:        now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(managed.Status).To(Equal(storage.PanelUserRemoved))
		users, err = store.ListPanelUsers(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(users).To(HaveLen(1))

		managed, err = store.CreatePanelUser(ctx, storage.PanelUserCreate{
			AccountID:      viewer.ID,
			ActorAccountID: owner.ID,
			ChangedAt:      now.Add(3 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(managed.Status).To(Equal(storage.PanelUserActive))
		Expect(managed.Revision).To(Equal(int64(4)))
		_, err = store.SetTargetAccess(ctx, storage.TargetAccessChange{
			TargetID: target.TargetID, SubjectAccountID: viewer.ID, ActorAccountID: owner.ID,
			Role: rolePointer(storage.InstallationRoleAdmin), ExpectedRevision: 0,
			ChangedAt: now.Add(3 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		targetUsers, err = store.ListTargetPanelUsers(ctx, target.TargetID, now.Add(3*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(targetUsers).To(HaveLen(2))
		managedTarget = targetUserByID(targetUsers, viewer.ID)
		Expect(managedTarget.Override).NotTo(BeNil())
		Expect(managedTarget.Override.Role).To(HaveValue(Equal(storage.InstallationRoleAdmin)))
		Expect(managedTarget.Access.Role).To(Equal(storage.InstallationRoleAdmin))

		targetPage, err := store.ListTargetPanelUserPage(ctx, target.TargetID, now, storage.PanelUserPageRequest{
			Limit: 1, Roles: []storage.InstallationRole{storage.InstallationRoleAdmin},
			States: []storage.PanelUserListState{storage.PanelUserListActive},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(targetPage.Total).To(Equal(1))
		Expect(targetPage.Items[0].User.Account.ID).To(Equal(viewer.ID))

		roleDescending, err := store.ListTargetPanelUserPage(
			ctx,
			target.TargetID,
			now,
			storage.PanelUserPageRequest{Limit: 10, Order: storage.PanelUserRoleDescending},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(roleDescending.Items).To(HaveLen(2))
		Expect(roleDescending.Items[0].User.Account.ID).To(Equal(owner.ID))
		Expect(roleDescending.Items[1].User.Account.ID).To(Equal(viewer.ID))

		decisions, err := store.ListAccessDecisions(ctx, viewer.ID, nil, 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(decisions).NotTo(BeEmpty())
		Expect(decisions[0].Action).To(Equal("user.readded"))
		Expect(decisions).To(ContainElement(And(
			HaveField("Action", "user.banned"),
			HaveField("Summary", ContainSubstring(reason)),
		)))
	})

	It("lists Root accounts with system roles and installation counts", func() {
		root, target := seedInstallation(ctx, store, now)
		Expect(store.ReconcileSuperRoot(ctx, root.ID, now)).To(Succeed())
		viewer := root
		viewer.ID = "github:user:root-page"
		viewer.SubjectID = "root-page"
		viewer.Login = "root-page-user"
		viewer.DisplayName = "Root Page User"
		Expect(store.UpsertAccount(ctx, viewer)).To(Succeed())
		_, err := store.CreatePanelUser(ctx, storage.PanelUserCreate{
			AccountID:      viewer.ID,
			ActorAccountID: root.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.SetTargetAccess(ctx, storage.TargetAccessChange{
			TargetID: target.TargetID, SubjectAccountID: viewer.ID, ActorAccountID: root.ID,
			Role: rolePointer(storage.InstallationRoleEditor), ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())

		page, err := store.ListRootPanelUserPage(ctx, storage.RootPanelUserPageRequest{
			Limit: 10, Order: storage.RootPanelUserRoleDescending,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Total).To(Equal(2))
		Expect(page.Items).To(HaveLen(2))
		Expect(page.Items[0].User.SystemRole).To(Equal(storage.SystemRoleSuperRoot))
		Expect(page.Items[0].OwnedInstallationCount).To(Equal(1))
		Expect(page.Items[1].AssignedInstallationCount).To(Equal(1))

		filtered, err := store.ListRootPanelUserPage(ctx, storage.RootPanelUserPageRequest{
			Limit: 10, Query: "PAGE", SystemRoles: []storage.SystemRole{storage.SystemRoleNone},
			Statuses: []storage.PanelUserStatus{storage.PanelUserActive},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(filtered.Total).To(Equal(1))
		Expect(filtered.Items[0].User.Account.ID).To(Equal(viewer.ID))
	})

	It("promotes and demotes Root accounts with an audit trail", func() {
		root, _ := seedInstallation(ctx, store, now)
		Expect(store.ReconcileSuperRoot(ctx, root.ID, now)).To(Succeed())
		subject := root
		subject.ID = "github:user:root-role"
		subject.SubjectID = "root-role"
		subject.Login = "root-role-user"
		Expect(store.UpsertAccount(ctx, subject)).To(Succeed())
		created, err := store.CreatePanelUser(ctx, storage.PanelUserCreate{
			AccountID:      subject.ID,
			ActorAccountID: root.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())

		promoted, err := store.UpdateSystemRole(ctx, storage.SystemRoleChange{
			AccountID: subject.ID, ActorAccountID: root.ID,
			SystemRole: storage.SystemRoleRoot, ExpectedRevision: created.Revision,
			ChangedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(promoted.SystemRole).To(Equal(storage.SystemRoleRoot))
		Expect(promoted.Revision).To(Equal(created.Revision + 1))

		demoted, err := store.UpdateSystemRole(ctx, storage.SystemRoleChange{
			AccountID: subject.ID, ActorAccountID: root.ID,
			SystemRole: storage.SystemRoleNone, ExpectedRevision: promoted.Revision,
			ChangedAt: now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(demoted.SystemRole).To(Equal(storage.SystemRoleNone))
		decisions, err := store.ListAccessDecisions(ctx, subject.ID, nil, 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(decisions).To(ContainElements(
			HaveField("Summary", "changed system role to root"),
			HaveField("Summary", "changed system role to none"),
		))

		_, err = store.UpdateSystemRole(ctx, storage.SystemRoleChange{
			AccountID: root.ID, ActorAccountID: root.ID,
			SystemRole: storage.SystemRoleNone, ExpectedRevision: 1,
			ChangedAt: now.Add(3 * time.Minute),
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())
	})

	It("creates and accepts Root invitations separately from installation access", func() {
		root, _ := seedInstallation(ctx, store, now)
		Expect(store.ReconcileSuperRoot(ctx, root.ID, now)).To(Succeed())
		invitee := root
		invitee.ID = "github:user:invited-root"
		invitee.SubjectID = "invited-root"
		invitee.Login = "invited-root"
		invitee.DisplayName = "Invited Root"
		Expect(store.UpsertAccount(ctx, invitee)).To(Succeed())
		role := storage.SystemRoleRoot
		invitation, err := store.CreateInvitation(ctx, storage.InvitationCreate{
			ID: "root-invitation", TokenHash: "root-invitation-token", AccountID: invitee.ID,
			SystemRole: &role, ExpiresAt: now.Add(7 * 24 * time.Hour),
			CreatedByAccount: root.ID, CreatedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(invitation.SystemRole).NotTo(BeNil())
		Expect(*invitation.SystemRole).To(Equal(storage.SystemRoleRoot))

		rootPage, err := store.ListRootInvitationPage(ctx, now, storage.InvitationPageRequest{
			Limit: 10, Order: storage.InvitationCreatedNewest,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(rootPage.Total).To(Equal(1))
		regularPage, err := store.ListInvitationPage(ctx, nil, now, storage.InvitationPageRequest{
			Limit: 10, Order: storage.InvitationCreatedNewest,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(regularPage.Total).To(Equal(0))

		accepted, err := store.RespondToInvitation(ctx, storage.InvitationResponse{
			TokenHash: "root-invitation-token", AccountID: invitee.ID, Accept: true,
			At: now.Add(time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(accepted.Status).To(Equal(storage.InvitationAccepted))
		user, err := store.GetPanelUser(ctx, invitee.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(user.SystemRole).To(Equal(storage.SystemRoleRoot))
		Expect(user.Status).To(Equal(storage.PanelUserActive))
	})

	It("names the scope an installation invitation is for, and leaves it empty for Root", func() {
		owner, target := seedInstallation(ctx, store, now)
		Expect(store.UpsertAccount(ctx, owner)).To(Succeed())
		Expect(store.ReconcileSuperRoot(ctx, owner.ID, now)).To(Succeed())

		invitee := owner
		invitee.ID = "github:user:invitee"
		invitee.SubjectID = "invitee"
		invitee.Login = "invitee"
		Expect(store.UpsertAccount(ctx, invitee)).To(Succeed())

		scoped, err := store.CreateInvitation(ctx, storage.InvitationCreate{
			ID: "scoped-invitation", TokenHash: "scoped-invitation-token", AccountID: invitee.ID,
			TargetID: &target.TargetID, Role: rolePointer(storage.InstallationRoleViewer),
			ExpiresAt:        now.Add(7 * 24 * time.Hour),
			CreatedByAccount: owner.ID, CreatedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		// Whoever opens the link may never have heard of the installation, so the
		// offer carries what identifies it on GitHub rather than a display name
		// alone: the login is the handle they can check, and the kind says whether
		// accepting joins an organisation or one person's installation.
		Expect(scoped.TargetName).To(HaveValue(Equal(owner.DisplayName)))
		Expect(scoped.TargetLogin).To(HaveValue(Equal(owner.Login)))
		Expect(scoped.TargetKind).To(HaveValue(Equal(string(storage.TargetOrganization))))

		// Read back rather than trusting the value the write returned: the two
		// travel different query paths, and only this one runs after a restart.
		fetched, err := store.GetInvitationByToken(ctx, "scoped-invitation-token", now)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.TargetLogin).To(HaveValue(Equal(owner.Login)))
		Expect(fetched.TargetKind).To(HaveValue(Equal(string(storage.TargetOrganization))))

		systemRole := storage.SystemRoleRoot
		root, err := store.CreateInvitation(ctx, storage.InvitationCreate{
			ID: "unscoped-invitation", TokenHash: "unscoped-invitation-token",
			AccountID: invitee.ID, SystemRole: &systemRole,
			ExpiresAt:        now.Add(7 * 24 * time.Hour),
			CreatedByAccount: owner.ID, CreatedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		// A Root offer is scoped to nothing, so every scope column stays null. The
		// outer join is what makes that so, and it is worth pinning: a plain join
		// would drop the row entirely rather than return it without a scope.
		Expect(root.TargetID).To(BeNil())
		Expect(root.TargetName).To(BeNil())
		Expect(root.TargetLogin).To(BeNil())
		Expect(root.TargetKind).To(BeNil())
	})

	It("creates, reissues, expires, and atomically responds to installation invitations", func() {
		owner, target := seedInstallation(ctx, store, now)
		Expect(store.UpsertAccount(ctx, owner)).To(Succeed())
		Expect(store.ReconcileSuperRoot(ctx, owner.ID, now)).To(Succeed())

		invitee := owner
		invitee.ID = "github:user:invitee"
		invitee.SubjectID = "invitee"
		invitee.Login = "invitee"
		Expect(store.UpsertAccount(ctx, invitee)).To(Succeed())

		invitation, err := store.CreateInvitation(ctx, storage.InvitationCreate{
			ID: "invitation-1", TokenHash: "token-1", AccountID: invitee.ID,
			TargetID: &target.TargetID, Role: rolePointer(storage.InstallationRoleViewer),
			ExpiresAt:        now.Add(7 * 24 * time.Hour),
			CreatedByAccount: owner.ID, CreatedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(invitation.Status).To(Equal(storage.InvitationPending))
		page, err := store.ListInvitationPage(ctx, &target.TargetID, now, storage.InvitationPageRequest{
			Limit: 10, Query: "INVITEE", Roles: []storage.InstallationRole{storage.InstallationRoleViewer},
			Statuses: []storage.InvitationStatus{storage.InvitationPending},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Total).To(Equal(1))
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].ID).To(Equal(invitation.ID))

		invitation, err = store.ReissueInvitation(ctx, storage.InvitationReissue{
			ID: invitation.ID, TokenHash: "token-2", ExpiresAt: now.Add(24 * time.Hour),
			CreatedByAccount: owner.ID, CreatedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.GetInvitationByToken(ctx, "token-1", now)
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeTrue())

		_, err = store.RespondToInvitation(ctx, storage.InvitationResponse{
			TokenHash: "token-2", AccountID: owner.ID, Accept: true, At: now.Add(2 * time.Minute),
		})
		Expect(errors.Is(err, storage.ErrIdentityMismatch)).To(BeTrue())
		accepted, err := store.RespondToInvitation(ctx, storage.InvitationResponse{
			TokenHash: "token-2", AccountID: invitee.ID, Accept: true, At: now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(accepted.Status).To(Equal(storage.InvitationAccepted))
		user, err := store.GetPanelUser(ctx, invitee.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(user.Status).To(Equal(storage.PanelUserActive))
		_, err = store.RespondToInvitation(ctx, storage.InvitationResponse{
			TokenHash: "token-2", AccountID: invitee.ID, Accept: true, At: now.Add(3 * time.Minute),
		})
		Expect(errors.Is(err, storage.ErrConflict)).To(BeTrue())

		// Accepting granted the role, and an offer to somebody who holds it is refused. Taking
		// the access away is what makes the identity invitable again.
		_, err = store.SetTargetAccess(ctx, storage.TargetAccessChange{
			TargetID: target.TargetID, SubjectAccountID: invitee.ID, ActorAccountID: owner.ID,
			Role: rolePointer(storage.InstallationRoleNone), ExpectedRevision: 1,
			ChangedAt: now.Add(4 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		targetInvitation, err := store.CreateInvitation(ctx, storage.InvitationCreate{
			ID: "invitation-2", TokenHash: "token-3", AccountID: invitee.ID,
			TargetID: &target.TargetID, Role: rolePointer(storage.InstallationRoleEditor),
			ExpiresAt: now.Add(time.Hour), CreatedByAccount: owner.ID, CreatedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.GetInvitationByToken(ctx, "token-3", now.Add(2*time.Hour))
		Expect(errors.Is(err, storage.ErrExpired)).To(BeTrue())
		targetInvitation, err = store.ReissueInvitation(ctx, storage.InvitationReissue{
			ID: targetInvitation.ID, TokenHash: "token-4", ExpiresAt: now.Add(24 * time.Hour),
			CreatedByAccount: owner.ID, CreatedAt: now.Add(3 * time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		declined, err := store.RespondToInvitation(ctx, storage.InvitationResponse{
			TokenHash: "token-4", AccountID: invitee.ID, Accept: false, At: now.Add(4 * time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(declined.Status).To(Equal(storage.InvitationDeclined))

		listed, err := store.ListInvitations(ctx, &target.TargetID, now.Add(4*time.Hour))
		Expect(err).NotTo(HaveOccurred())
		Expect(listed).To(HaveLen(2))
		Expect(listed[0].Status).To(Equal(storage.InvitationDeclined))
		Expect(listed[1].Status).To(Equal(storage.InvitationAccepted))
	})

	// The offer is checked where it is written rather than in the handler above it: two managers
	// pressing at once would both pass a check made outside this transaction.
	It("refuses an invitation the invited identity cannot use", func() {
		owner, target := seedInstallation(ctx, store, now)
		Expect(store.UpsertAccount(ctx, owner)).To(Succeed())
		Expect(store.ReconcileSuperRoot(ctx, owner.ID, now)).To(Succeed())

		invitee := owner
		invitee.ID = "github:user:invitee"
		invitee.SubjectID = "invitee"
		invitee.Login = "invitee"
		Expect(store.UpsertAccount(ctx, invitee)).To(Succeed())

		offer := func(id, token string, at time.Time, acknowledged bool) error {
			_, err := store.CreateInvitation(ctx, storage.InvitationCreate{
				ID: id, TokenHash: token, AccountID: invitee.ID, TargetID: &target.TargetID,
				Role: rolePointer(storage.InstallationRoleViewer), ExpiresAt: at.Add(time.Hour),
				CreatedByAccount: owner.ID, CreatedAt: at, AcknowledgeDeclined: acknowledged,
			})

			return err
		}

		By("offering to an identity the app has never seen")
		Expect(offer("invitation-1", "token-1", now, false)).To(Succeed())

		By("replacing an offer nobody has answered")
		Expect(offer("invitation-2", "token-2", now.Add(time.Minute), false)).To(Succeed())
		superseded, err := store.GetInvitation(ctx, "invitation-1", now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(superseded.Status).To(Equal(storage.InvitationRevoked))

		By("offering again after the last one ran out")
		Expect(offer("invitation-3", "token-3", now.Add(2*time.Hour), false)).To(Succeed())

		By("refusing to offer what the identity already holds")
		_, err = store.RespondToInvitation(ctx, storage.InvitationResponse{
			TokenHash: "token-3", AccountID: invitee.ID, Accept: true, At: now.Add(2*time.Hour + time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(errors.Is(offer("invitation-4", "token-4", now.Add(3*time.Hour), false), storage.ErrAlreadyMember)).
			To(BeTrue())

		By("offering again once the access is gone")
		_, err = store.SetTargetAccess(ctx, storage.TargetAccessChange{
			TargetID: target.TargetID, SubjectAccountID: invitee.ID, ActorAccountID: owner.ID,
			Role: rolePointer(storage.InstallationRoleNone), ExpectedRevision: 1,
			ChangedAt: now.Add(3 * time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(offer("invitation-5", "token-5", now.Add(4*time.Hour), false)).To(Succeed())

		By("gating the offer after the identity said no")
		_, err = store.RespondToInvitation(ctx, storage.InvitationResponse{
			TokenHash: "token-5", AccountID: invitee.ID, Accept: false, At: now.Add(4*time.Hour + time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(errors.Is(offer("invitation-6", "token-6", now.Add(5*time.Hour), false), storage.ErrDeclinedEarlier)).
			To(BeTrue())

		By("letting the offer through once the manager says it meant to ask again")
		Expect(offer("invitation-7", "token-7", now.Add(5*time.Hour), true)).To(Succeed())

		By("dropping the gate once the decline is no longer the last word")
		Expect(offer("invitation-8", "token-8", now.Add(6*time.Hour), false)).To(Succeed())
	})

	// An offer can outlive the reason for it. Renewing one is refused on the same ground as making
	// it, because accepting a stale offer overwrites the role the identity holds now.
	It("refuses to renew an offer the invited identity has since outgrown", func() {
		owner, target := seedInstallation(ctx, store, now)
		Expect(store.UpsertAccount(ctx, owner)).To(Succeed())
		Expect(store.ReconcileSuperRoot(ctx, owner.ID, now)).To(Succeed())
		invitee := owner
		invitee.ID = "github:user:invitee"
		invitee.SubjectID = "invitee"
		invitee.Login = "invitee"
		Expect(store.UpsertAccount(ctx, invitee)).To(Succeed())

		_, err := store.CreateInvitation(ctx, storage.InvitationCreate{
			ID: "inv-1", TokenHash: "tok-1", AccountID: invitee.ID, TargetID: &target.TargetID,
			Role: rolePointer(storage.InstallationRoleViewer), ExpiresAt: now.Add(time.Hour),
			CreatedByAccount: owner.ID, CreatedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = store.CreatePanelUser(ctx, storage.PanelUserCreate{
			AccountID: invitee.ID, ActorAccountID: owner.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.SetTargetAccess(ctx, storage.TargetAccessChange{
			TargetID: target.TargetID, SubjectAccountID: invitee.ID, ActorAccountID: owner.ID,
			Role: rolePointer(storage.InstallationRoleEditor), ExpectedRevision: 0, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = store.CreateInvitation(ctx, storage.InvitationCreate{
			ID: "inv-2", TokenHash: "tok-2", AccountID: invitee.ID, TargetID: &target.TargetID,
			Role: rolePointer(storage.InstallationRoleViewer), ExpiresAt: now.Add(time.Hour),
			CreatedByAccount: owner.ID, CreatedAt: now,
		})
		Expect(errors.Is(err, storage.ErrAlreadyMember)).To(BeTrue(), "create should refuse")

		_, err = store.ReissueInvitation(ctx, storage.InvitationReissue{
			ID: "inv-1", TokenHash: "tok-3", ExpiresAt: now.Add(24 * time.Hour),
			CreatedByAccount: owner.ID, CreatedAt: now.Add(time.Minute),
		})
		Expect(errors.Is(err, storage.ErrAlreadyMember)).To(BeTrue(), "reissue should refuse too")
	})

	It("refuses a Root invitation for an identity the app already holds", func() {
		owner, _ := seedInstallation(ctx, store, now)
		Expect(store.UpsertAccount(ctx, owner)).To(Succeed())
		Expect(store.ReconcileSuperRoot(ctx, owner.ID, now)).To(Succeed())

		invitee := owner
		invitee.ID = "github:user:invitee"
		invitee.SubjectID = "invitee"
		invitee.Login = "invitee"
		Expect(store.UpsertAccount(ctx, invitee)).To(Succeed())

		rootOffer := func(id, token string) error {
			role := storage.SystemRoleRoot
			_, err := store.CreateInvitation(ctx, storage.InvitationCreate{
				ID: id, TokenHash: token, AccountID: invitee.ID, SystemRole: &role,
				ExpiresAt: now.Add(time.Hour), CreatedByAccount: owner.ID, CreatedAt: now,
			})

			return err
		}

		Expect(rootOffer("root-invitation-1", "root-token-1")).To(Succeed())
		_, err := store.CreatePanelUser(ctx, storage.PanelUserCreate{
			AccountID: invitee.ID, ActorAccountID: owner.ID, ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(errors.Is(rootOffer("root-invitation-2", "root-token-2"), storage.ErrAlreadyMember)).
			To(BeTrue())
	})

	It("orders invitation pages by invitee name descending", func() {
		owner, target := seedInstallation(ctx, store, now)
		Expect(store.UpsertAccount(ctx, owner)).To(Succeed())

		alpha := owner
		alpha.ID = "github:user:alpha"
		alpha.SubjectID = "alpha"
		alpha.Login = "alpha"
		alpha.DisplayName = "Alpha User"
		zulu := owner
		zulu.ID = "github:user:zulu"
		zulu.SubjectID = "zulu"
		zulu.Login = "zulu"
		zulu.DisplayName = "Zulu User"
		Expect(store.UpsertAccount(ctx, alpha)).To(Succeed())
		Expect(store.UpsertAccount(ctx, zulu)).To(Succeed())

		for id, account := range map[string]storage.Account{
			"invitation-alpha": alpha,
			"invitation-zulu":  zulu,
		} {
			_, err := store.CreateInvitation(ctx, storage.InvitationCreate{
				ID: id, TokenHash: id, AccountID: account.ID,
				TargetID: &target.TargetID, Role: rolePointer(map[string]storage.InstallationRole{
					"invitation-alpha": storage.InstallationRoleViewer,
					"invitation-zulu":  storage.InstallationRoleAdmin,
				}[id]), ExpiresAt: now.Add(24 * time.Hour),
				CreatedByAccount: owner.ID, CreatedAt: now,
			})
			Expect(err).NotTo(HaveOccurred())
		}

		page, err := store.ListInvitationPage(ctx, &target.TargetID, now, storage.InvitationPageRequest{
			Limit: 10,
			Order: storage.InvitationNameDescending,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(2))
		Expect(page.Items[0].Account.DisplayName).To(Equal("Zulu User"))
		Expect(page.Items[1].Account.DisplayName).To(Equal("Alpha User"))

		page, err = store.ListInvitationPage(ctx, &target.TargetID, now, storage.InvitationPageRequest{
			Limit: 10,
			Order: storage.InvitationRoleDescending,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(2))
		Expect(page.Items[0].Role).To(HaveValue(Equal(storage.InstallationRoleAdmin)))
		Expect(page.Items[1].Role).To(HaveValue(Equal(storage.InstallationRoleViewer)))
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
		_, err := store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: installation.TargetID, ActorAccountID: account.ID,
			ChangedAt: now.Add(2 * time.Minute),
			Repositories: []storage.InstallationRepositorySettingsChange{{
				RepositoryID: "repo-beta", EnabledOverride: &enabled,
				ConfigPatch: config.Patch{
					QuietSuccess: &enabled, AllowDraftMerges: &enabled,
				},
				IgnoreRepositoryFile: false, ExpectedRevision: 1,
			}},
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
		_, err = store.SaveInstallationSettings(ctx, storage.SaveInstallationSettingsRequest{
			TargetID: installation.TargetID, ActorAccountID: account.ID,
			ChangedAt: now.Add(3 * time.Minute),
			Repositories: []storage.InstallationRepositorySettingsChange{{
				RepositoryID: "repo-delta", ConfigPatch: config.Patch{CommandPrefix: &prefix},
				IgnoreRepositoryFile: false, ExpectedRevision: 1,
			}},
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

		draftMergeOverrides, err := store.ListRepositoryPage(
			ctx,
			installation.TargetID,
			storage.RepositoryPageRequest{
				Limit: 10, Order: storage.RepositoryNameAscending,
				ConfigOverrideKeys: []string{config.KeyAllowDraftMerges},
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(draftMergeOverrides.Total).To(Equal(1))
		Expect(draftMergeOverrides.Items[0].Name).To(Equal("beta"))
	})

	It("treats wildcard characters in a search as ordinary text", func() {
		account := testAccount(now)
		Expect(store.ReconcileInstallation(ctx, testInstallation(account, now, []storage.RepositorySnapshot{
			testRepository("repo-1", "smykla-skalski/alpha_one", false),
			testRepository("repo-2", "smykla-skalski/alphaXone", false),
		}))).To(Succeed())
		targetID := testInstallation(account, now, nil).TargetID

		search := func(query string) []string {
			page, err := store.ListRepositoryPage(ctx, targetID, storage.RepositoryPageRequest{
				Limit: 10,
				Order: storage.RepositoryNameAscending,
				Query: query,
			})
			Expect(err).NotTo(HaveOccurred())
			names := make([]string, 0, len(page.Items))
			for _, item := range page.Items {
				names = append(names, item.Name)
			}

			return names
		}

		// An underscore is a single-character wildcard to LIKE, and it is also
		// an ordinary character in a repository name.
		Expect(search("alpha_one")).To(Equal([]string{"alpha_one"}))
		// A bare percent would otherwise match every row.
		Expect(search("%")).To(BeEmpty())
		// Case-folded ordering puts _ before x; the search itself ignores case.
		Expect(search("ALPHA")).To(Equal([]string{"alpha_one", "alphaXone"}))
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

	It("leases, schedules, and recovers durable delivery payloads", func() {
		_, target := seedInstallation(ctx, store, now)
		payload := []byte(`{"action":"created"}`)
		claim := storage.DeliveryClaim{
			ClaimKey:   "issue_comment:created:repo:durable:revision",
			DeliveryID: "durable-delivery", TargetID: target.TargetID,
			RepositoryFullName: "smykla-skalski/smyklot",
			Event:              "created", Payload: payload, ClaimedAt: now,
		}

		accepted, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		Expect(accepted.Disposition).To(Equal(storage.DeliveryClaimAccepted))

		first, err := store.LeaseDelivery(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Work).NotTo(BeNil())
		Expect(first.Work.ID).To(Equal(accepted.ID))
		Expect(first.Work.Payload).To(Equal(payload))
		Expect(first.Work.Attempt).To(Equal(1))

		leased, err := store.LeaseDelivery(ctx, now.Add(30*time.Second), now.Add(2*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(leased.Work).To(BeNil())
		Expect(leased.AvailableAt).To(HaveValue(Equal(now.Add(time.Minute))))

		retryAt := now.Add(2 * time.Minute)
		Expect(store.RetryDelivery(ctx, storage.DeliveryRetryChange{
			ClaimID: accepted.ID, Stage: "execute",
			Reason: "temporary GitHub failure", RetryAt: retryAt,
		})).To(Succeed())
		waiting, err := store.LeaseDelivery(ctx, retryAt.Add(-time.Second), retryAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(waiting.Work).To(BeNil())
		Expect(waiting.AvailableAt).To(HaveValue(Equal(retryAt)))

		second, err := store.LeaseDelivery(ctx, retryAt, retryAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Work).NotTo(BeNil())
		Expect(second.Work.Attempt).To(Equal(2))

		recoveredAt := now.Add(3 * time.Minute)
		Expect(store.RecoverRunningDeliveries(ctx, recoveredAt)).To(Succeed())
		recovered, err := store.LeaseDelivery(ctx, recoveredAt, recoveredAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(recovered.Work).NotTo(BeNil())
		Expect(recovered.Work.ID).To(Equal(accepted.ID))
		Expect(recovered.Work.Attempt).To(Equal(3))
	})

	It("moves a webhook source deadline with its queue schedule", func() {
		account, target := seedInstallation(ctx, store, now)
		claim := storage.DeliveryClaim{
			ClaimKey:   "issue_comment:created:repo:scheduled:revision",
			DeliveryID: "scheduled-delivery", TargetID: target.TargetID,
			RepositoryFullName: "smykla-skalski/smyklot",
			Event:              "issue_comment", Payload: []byte(`{"action":"created"}`), ClaimedAt: now,
		}
		accepted, err := store.ClaimDelivery(ctx, claim)
		Expect(err).NotTo(HaveOccurred())
		first, err := store.LeaseDelivery(ctx, now, now.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Work).NotTo(BeNil())
		retryAt := now.Add(4 * time.Hour)
		Expect(store.RetryDelivery(ctx, storage.DeliveryRetryChange{
			ClaimID: accepted.ID, Stage: "execute", Reason: "temporary failure", RetryAt: retryAt,
		})).To(Succeed())

		itemID := fmt.Sprintf("delivery:%d", accepted.ID)
		item, err := store.GetQueueItem(ctx, itemID)
		Expect(err).NotTo(HaveOccurred())
		runAt := now.Add(2 * time.Minute)
		_, err = store.ApplyQueueAction(ctx, itemID, workqueue.ItemAction{
			Type: workqueue.ActionRunNow, ExpectedRevision: item.Revision,
			ActorID: account.ID, Reason: "recover webhook", ChangedAt: runAt,
		})
		Expect(err).NotTo(HaveOccurred())
		leased, err := store.LeaseDelivery(ctx, runAt, runAt.Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(leased.Work).NotTo(BeNil())
		Expect(leased.Work.ID).To(Equal(accepted.ID))
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

	It("treats missing preferences as an empty document at revision zero", func() {
		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())

		preferences, err := store.GetPreferences(ctx, account.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(preferences.AccountID).To(Equal(account.ID))
		Expect(preferences.Revision).To(BeZero())
		Expect(preferences.Values).To(BeEmpty())
	})

	It("merges preference changes per key with last-write-wins", func() {
		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())

		first, err := store.ApplyPreferences(ctx, storage.PreferenceChange{
			AccountID: account.ID,
			Changes: map[string]json.RawMessage{
				"theme":   json.RawMessage(`"dark"`),
				"sidebar": json.RawMessage(`"collapsed"`),
			},
			ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Revision).To(Equal(int64(1)))
		Expect(first.Values).To(HaveLen(2))
		Expect(first.UpdatedAt).To(Equal(now))

		second, err := store.ApplyPreferences(ctx, storage.PreferenceChange{
			AccountID: account.ID,
			Changes:   map[string]json.RawMessage{"theme": json.RawMessage(`"light"`)},
			ChangedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Revision).To(Equal(int64(2)))
		Expect(string(second.Values["theme"])).To(Equal(`"light"`))
		Expect(string(second.Values["sidebar"])).To(Equal(`"collapsed"`))

		stored, err := store.GetPreferences(ctx, account.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(Equal(second))
	})

	It("deletes preference keys on null and skips revision bumps for no-ops", func() {
		account := testAccount(now)
		Expect(store.UpsertAccount(ctx, account)).To(Succeed())

		seeded, err := store.ApplyPreferences(ctx, storage.PreferenceChange{
			AccountID: account.ID,
			Changes: map[string]json.RawMessage{
				"theme":   json.RawMessage(`"dark"`),
				"sidebar": json.RawMessage(`"collapsed"`),
			},
			ChangedAt: now,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(seeded.Revision).To(Equal(int64(1)))

		deleted, err := store.ApplyPreferences(ctx, storage.PreferenceChange{
			AccountID: account.ID,
			Changes:   map[string]json.RawMessage{"sidebar": json.RawMessage(`null`)},
			ChangedAt: now.Add(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(deleted.Revision).To(Equal(int64(2)))
		Expect(deleted.Values).To(HaveKey("theme"))
		Expect(deleted.Values).NotTo(HaveKey("sidebar"))

		unchanged, err := store.ApplyPreferences(ctx, storage.PreferenceChange{
			AccountID: account.ID,
			Changes: map[string]json.RawMessage{
				"theme":   json.RawMessage(`"dark"`),
				"sidebar": nil,
			},
			ChangedAt: now.Add(2 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(unchanged).To(Equal(deleted))

		empty, err := store.ApplyPreferences(ctx, storage.PreferenceChange{
			AccountID: account.ID,
			ChangedAt: now.Add(3 * time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(empty).To(Equal(deleted))
	})
}

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
		Ownership: storage.OwnershipSnapshot{
			Source:   storage.OwnershipSourceOrganizationAdmin,
			Status:   storage.OwnershipStatusFresh,
			Owners:   []storage.Account{account},
			SyncedAt: now,
		},
		SyncedAt: now,
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

	return storage.RepositorySnapshot{
		ID: id, Name: name, FullName: fullName, Private: private, DefaultBranch: "main",
	}
}

func rolePointer(role storage.InstallationRole) *storage.InstallationRole {
	return &role
}

func targetUserByID(users []storage.TargetPanelUser, accountID string) storage.TargetPanelUser {
	for _, user := range users {
		if user.User.Account.ID == accountID {
			return user
		}
	}

	return storage.TargetPanelUser{}
}

func seedInstallation(
	ctx context.Context,
	store storage.Store,
	now time.Time,
) (storage.Account, storage.InstallationSnapshot) {
	account := testAccount(now)
	target := testInstallation(account, now, []storage.RepositorySnapshot{
		testRepository("repo-1", "smykla-skalski/smyklot", false),
	})
	Expect(store.ReconcileInstallation(ctx, target)).To(Succeed())

	return account, target
}

func seedElevationScenario(
	ctx context.Context,
	store storage.Store,
	now time.Time,
) (storage.Account, storage.Account, storage.InstallationSnapshot, storage.Session) {
	owner := testAccount(now)
	target := testInstallation(owner, now, []storage.RepositorySnapshot{
		testRepository("repo-1", "smykla-skalski/smyklot", false),
	})
	Expect(store.ReconcileInstallation(ctx, target)).To(Succeed())

	root := owner
	root.ID = "github:root"
	root.SubjectID = "root"
	root.Login = "root"
	root.DisplayName = "Root Operator"
	Expect(store.UpsertAccount(ctx, root)).To(Succeed())
	Expect(store.ReconcileSuperRoot(ctx, root.ID, now)).To(Succeed())
	session := storage.Session{
		TokenHash: "root-session", AccountID: root.ID,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	Expect(store.CreateSession(ctx, session, 1)).To(Succeed())

	return root, owner, target, session
}
