package storagetest

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/pendingci"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func declareRuntimeSettingsHistorySpecs(
	harness Harness,
	runtime func() (context.Context, storage.Store, time.Time),
) {
	Describe("runtime settings checkpoints", func() {
		It("captures the default Root state before the first real save", func() {
			ctx, store, now := runtime()
			actor := testAccount(now)
			Expect(store.UpsertAccount(ctx, actor)).To(Succeed())

			debug := "debug"
			saved, err := store.SaveRuntimeSettings(ctx, storage.RuntimeSettingsChange{
				LogLevel:                      &debug,
				EffectivePendingCIQuietPeriod: 30 * time.Second,
				EffectiveSessionTTL:           time.Hour,
				ActorAccountID:                actor.ID,
				ChangedAt:                     now.Add(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(saved.CheckpointID).NotTo(BeNil())
			Expect(harness.CountSettingsCheckpoints(ctx)).To(Equal(int64(2)))

			baseline, err := store.InspectRootSettingsBaseline(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseline.Checkpoint.Action).To(Equal(storage.SettingsCheckpointActionBaseline))
			Expect(baseline.Checkpoint.ID).To(Equal(*saved.CheckpointID - 1))
			Expect(baseline.Items).To(ConsistOf(And(
				HaveField("Identity.Kind", storage.SettingsCheckpointItemRuntime),
				HaveField("Before.Available", false),
				HaveField("Before.Restorable", false),
				HaveField("After.State.Revision", int64(0)),
				HaveField("Current.Revision", int64(1)),
				HaveField("After.Differs", true),
				HaveField("After.Restorable", true),
			)))
			_, err = store.RestoreRuntimeSettings(
				ctx,
				storage.RestoreRuntimeSettingsRequest{
					CheckpointID: baseline.Checkpoint.ID, ExpectedRevision: 1,
					Side:           storage.SettingsCheckpointRestoreBefore,
					ActorAccountID: actor.ID, ChangedAt: now.Add(90 * time.Second),
					Runner:                        config.RunnerService,
					EffectivePendingCIQuietPeriod: 30 * time.Second,
					EffectiveSessionTTL:           time.Hour,
				},
			)
			Expect(errors.Is(err, storage.ErrSettingsRestoreBlocked)).To(BeTrue())

			restored, err := store.RestoreRuntimeSettings(
				ctx,
				storage.RestoreRuntimeSettingsRequest{
					CheckpointID: baseline.Checkpoint.ID, ExpectedRevision: 1,
					Side:           storage.SettingsCheckpointRestoreAfter,
					ActorAccountID: actor.ID, ChangedAt: now.Add(2 * time.Minute),
					Runner:                        config.RunnerService,
					EffectivePendingCIQuietPeriod: 30 * time.Second,
					EffectiveSessionTTL:           time.Hour,
				},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(restored.Settings.Revision).To(Equal(int64(2)))
			Expect(restored.Settings.LogLevel).To(BeNil())
			stable, err := store.InspectRootSettingsBaseline(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(stable.Checkpoint).To(Equal(baseline.Checkpoint))
		})

		It("keeps saves, no-ops, inspection, and restore in one immutable history", func() {
			ctx, store, now := runtime()
			actor := testAccount(now)
			Expect(store.UpsertAccount(ctx, actor)).To(Succeed())

			botConfig := config.Default()
			botConfig.QuietSuccess = true
			warn := "warn"
			poll := 90 * time.Second
			quiet := 45 * time.Second
			sessionTTL := 2 * time.Hour
			firstChange := storage.RuntimeSettingsChange{
				BotConfig: botConfig, LogLevel: &warn,
				PollInterval: &poll, PendingCIQuietPeriod: &quiet, SessionTTL: &sessionTTL,
				EffectivePendingCIQuietPeriod: quiet,
				EffectiveSessionTTL:           sessionTTL,
				ExpectedRevision:              0, ActorAccountID: actor.ID, ChangedAt: now.Add(time.Minute),
			}
			first, err := store.SaveRuntimeSettings(ctx, firstChange)
			Expect(err).NotTo(HaveOccurred())
			Expect(first.Settings.Revision).To(Equal(int64(1)))
			Expect(first.CheckpointID).NotTo(BeNil())

			firstChange.ExpectedRevision = 1
			firstChange.ChangedAt = now.Add(2 * time.Minute)
			noop, err := store.SaveRuntimeSettings(ctx, firstChange)
			Expect(err).NotTo(HaveOccurred())
			Expect(noop.Settings.Revision).To(Equal(int64(1)))
			Expect(noop.CheckpointID).To(BeNil())

			errorLevel := "error"
			second, err := store.SaveRuntimeSettings(ctx, storage.RuntimeSettingsChange{
				LogLevel:                      &errorLevel,
				EffectivePendingCIQuietPeriod: 30 * time.Second,
				EffectiveSessionTTL:           4 * time.Hour,
				ExpectedRevision:              1, ActorAccountID: actor.ID, ChangedAt: now.Add(3 * time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(second.Settings.Revision).To(Equal(int64(2)))
			Expect(second.CheckpointID).NotTo(BeNil())

			ref := storage.SettingsCheckpointRef{
				ID: *first.CheckpointID, Scope: storage.SettingsCheckpointScopeRoot,
			}
			inspection, err := store.InspectRootSettingsCheckpoint(ctx, ref)
			Expect(err).NotTo(HaveOccurred())
			Expect(inspection.Items).To(HaveLen(1))
			item := inspection.Items[0]
			Expect(item.Identity.Kind).To(Equal(storage.SettingsCheckpointItemRuntime))
			Expect(item.After.State.Revision).To(Equal(int64(1)))
			Expect(item.Current.Revision).To(Equal(int64(2)))
			Expect(item.After.Differs).To(BeTrue())
			Expect(item.After.Restorable).To(BeTrue())
			sourceDigest := item.After.State.Digest

			restored, err := store.RestoreRuntimeSettings(ctx, storage.RestoreRuntimeSettingsRequest{
				CheckpointID: *first.CheckpointID, ExpectedRevision: 2,
				Side:           storage.SettingsCheckpointRestoreBefore,
				ActorAccountID: actor.ID, ChangedAt: now.Add(4 * time.Minute),
				Runner:                        config.RunnerService,
				EffectivePendingCIQuietPeriod: 30 * time.Second,
				EffectiveSessionTTL:           4 * time.Hour,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(restored.CheckpointID).NotTo(BeNil())
			Expect(restored.Settings.Revision).To(Equal(int64(3)))
			Expect(restored.Settings.LogLevel).To(BeNil())
			assertRootRestoreCheckpoint(
				ctx, store, *restored.CheckpointID, *first.CheckpointID,
				storage.SettingsCheckpointRestoreBefore,
			)

			unchangedSource, err := store.InspectRootSettingsCheckpoint(ctx, ref)
			Expect(err).NotTo(HaveOccurred())
			Expect(unchangedSource.Items[0].After.State.Digest).To(Equal(sourceDigest))
			Expect(unchangedSource.Items[0].Current.Revision).To(Equal(int64(3)))
			Expect(unchangedSource.Items[0].After.Differs).To(BeTrue())

			redone, err := store.RestoreRuntimeSettings(ctx, storage.RestoreRuntimeSettingsRequest{
				CheckpointID: *first.CheckpointID, ExpectedRevision: 3,
				Side:           storage.SettingsCheckpointRestoreAfter,
				ActorAccountID: actor.ID, ChangedAt: now.Add(5 * time.Minute),
				Runner:                        config.RunnerService,
				EffectivePendingCIQuietPeriod: quiet,
				EffectiveSessionTTL:           sessionTTL,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(redone.Settings.Revision).To(Equal(int64(4)))
			Expect(redone.Settings.LogLevel).To(HaveValue(Equal(warn)))
			Expect(redone.Settings.PollInterval).To(HaveValue(Equal(poll)))
			assertRootRestoreCheckpoint(
				ctx, store, *redone.CheckpointID, *first.CheckpointID,
				storage.SettingsCheckpointRestoreAfter,
			)

			_, err = store.RestoreRuntimeSettings(ctx, storage.RestoreRuntimeSettingsRequest{
				CheckpointID: *first.CheckpointID, ExpectedRevision: 4,
				Side:           storage.SettingsCheckpointRestoreAfter,
				ActorAccountID: actor.ID, ChangedAt: now.Add(6 * time.Minute),
				Runner:                        config.RunnerService,
				EffectivePendingCIQuietPeriod: quiet,
				EffectiveSessionTTL:           sessionTTL,
			})
			Expect(errors.Is(err, storage.ErrSettingsRestoreNoop)).To(BeTrue())

			audit, err := store.ListRootAudit(ctx, storage.RootAuditPageRequest{
				HistoryPageRequest: storage.HistoryPageRequest{Limit: 10},
				Categories:         []storage.AuditCategory{storage.AuditCategoryRuntime},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(audit.Items).To(HaveLen(4))
			Expect(audit.Items[0].Action).To(Equal("runtime.settings.restored"))
			Expect(audit.Items[0].SettingsCheckpointID).To(HaveValue(Equal(*redone.CheckpointID)))
			Expect(audit.Items[1].SettingsCheckpointID).To(HaveValue(Equal(*restored.CheckpointID)))
			Expect(audit.Items[2].Action).To(Equal("runtime.settings.saved"))
			Expect(audit.Items[3].Action).To(Equal("runtime.settings.saved"))
		})

		It("rolls back every runtime effect when checkpoint creation fails", func() {
			ctx, store, now := runtime()
			actor := testAccount(now)
			Expect(store.ReconcileInstallation(ctx, storage.InstallationSnapshot{
				TargetID: "installation:77", InstallationID: "77",
				Kind: storage.TargetOrganization, Account: actor,
				Repositories: []storage.RepositorySnapshot{{
					ID: "repository-20", Name: "repo", FullName: "smykla-skalski/repo",
					DefaultBranch: "main",
				}},
				SyncedAt: now,
			})).To(Succeed())
			session := storage.Session{
				TokenHash: "runtime-rollback-session", AccountID: actor.ID,
				CreatedAt: now, ExpiresAt: now.Add(12 * time.Hour),
			}
			Expect(store.CreateSession(ctx, session, 1)).To(Succeed())

			info := "info"
			initialQuiet := 2 * time.Minute
			initialSessionTTL := 12 * time.Hour
			initial, err := store.SaveRuntimeSettings(ctx, storage.RuntimeSettingsChange{
				LogLevel: &info, PendingCIQuietPeriod: &initialQuiet,
				SessionTTL:                    &initialSessionTTL,
				EffectivePendingCIQuietPeriod: initialQuiet,
				EffectiveSessionTTL:           initialSessionTTL,
				ExpectedRevision:              0,
				ActorAccountID:                actor.ID,
				ChangedAt:                     now.Add(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(initial.CheckpointID).NotTo(BeNil())

			armed, err := store.Arm(ctx, pendingCIArm(now, 196, 99, "rollback-head"))
			Expect(err).NotTo(HaveOccurred())
			lease, err := store.LeaseDue(ctx, now, now.Add(time.Minute))
			Expect(err).NotTo(HaveOccurred())
			Expect(lease.Request).NotTo(BeNil())
			_, err = store.Reschedule(ctx, pendingci.RescheduleRequest{
				ID: lease.Request.ID, ExpectedRevision: lease.Request.Revision,
				Schedule: pendingci.ScheduleActive, HeadSHA: armed.Request.HeadSHA,
				NextCheckAt: now.Add(24 * time.Hour), NextCheckTrigger: pendingci.TriggerQuietPeriod,
				LastProgressAt: now, LastObservedState: string(pendingci.ObservedPassing),
				LastFingerprint: "passing:rollback", CheckedAt: now,
			})
			Expect(err).NotTo(HaveOccurred())
			retuned, err := store.RetuneQuietPeriod(ctx, pendingci.RetuneQuietPeriodRequest{
				PassingQuiet: initialQuiet, ChangedAt: now.Add(90 * time.Second), InheritedOnly: true,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(retuned).To(Equal(int64(1)))

			beforeRuntime, err := store.GetRuntimeSettings(ctx)
			Expect(err).NotTo(HaveOccurred())
			beforeSession, err := store.GetSession(ctx, session.TokenHash, now.Add(time.Minute))
			Expect(err).NotTo(HaveOccurred())
			beforePendingCI, err := store.GetArmed(ctx, "repository-20", 196)
			Expect(err).NotTo(HaveOccurred())
			Expect(beforePendingCI.NextCheckAt).To(Equal(now.Add(initialQuiet)))
			beforeCheckpointCount := harness.CountSettingsCheckpoints(ctx)
			beforeAudit, err := store.ListRootAudit(ctx, storage.RootAuditPageRequest{
				HistoryPageRequest: storage.HistoryPageRequest{Limit: 100},
				Categories:         []storage.AuditCategory{storage.AuditCategoryRuntime},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(beforeAudit.Total).To(Equal(1))

			harness.RejectSettingsCheckpoints(ctx)
			debug := "debug"
			shortQuiet := 15 * time.Second
			shortSessionTTL := 2 * time.Hour
			_, err = store.SaveRuntimeSettings(ctx, storage.RuntimeSettingsChange{
				LogLevel: &debug, PendingCIQuietPeriod: &shortQuiet,
				SessionTTL:                    &shortSessionTTL,
				EffectivePendingCIQuietPeriod: shortQuiet,
				EffectiveSessionTTL:           shortSessionTTL,
				ExpectedRevision:              beforeRuntime.Revision,
				ActorAccountID:                actor.ID,
				ChangedAt:                     now.Add(2 * time.Minute),
			})
			Expect(err).To(MatchError(ContainSubstring("insert settings checkpoint")))

			afterRuntime, err := store.GetRuntimeSettings(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(afterRuntime).To(Equal(beforeRuntime))
			afterSession, err := store.GetSession(ctx, session.TokenHash, now.Add(time.Minute))
			Expect(err).NotTo(HaveOccurred())
			Expect(afterSession).To(Equal(beforeSession))
			afterPendingCI, err := store.GetArmed(ctx, "repository-20", 196)
			Expect(err).NotTo(HaveOccurred())
			Expect(afterPendingCI).To(Equal(beforePendingCI))
			Expect(harness.CountSettingsCheckpoints(ctx)).To(Equal(beforeCheckpointCount))
			afterAudit, err := store.ListRootAudit(ctx, storage.RootAuditPageRequest{
				HistoryPageRequest: storage.HistoryPageRequest{Limit: 100},
				Categories:         []storage.AuditCategory{storage.AuditCategoryRuntime},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(afterAudit.Total).To(Equal(beforeAudit.Total))
			Expect(afterAudit.Items).To(Equal(beforeAudit.Items))
		})
	})
}

func assertRootRestoreCheckpoint(
	ctx context.Context,
	store storage.Store,
	checkpointID, sourceID int64,
	side storage.SettingsCheckpointRestoreSide,
) {
	GinkgoHelper()
	inspection, err := store.InspectRootSettingsCheckpoint(ctx, storage.SettingsCheckpointRef{
		ID: checkpointID, Scope: storage.SettingsCheckpointScopeRoot,
	})
	Expect(err).NotTo(HaveOccurred())
	checkpoint := inspection.Checkpoint
	Expect(checkpoint.Action).To(Equal(storage.SettingsCheckpointActionRestore))
	Expect(checkpoint.RestoredFromID).To(HaveValue(Equal(sourceID)))
	Expect(checkpoint.RestoredSide).To(Equal(side))
	Expect(checkpoint.Items).To(HaveLen(1))
	Expect(checkpoint.Items[0].Before).NotTo(BeNil())
	Expect(checkpoint.Items[0].After).NotTo(BeNil())
}
