package storagetest

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"

	. "github.com/onsi/gomega"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func runtimeBehavior(value *config.Config) *storage.RuntimeBehavior {
	content, err := json.Marshal(value)
	Expect(err).NotTo(HaveOccurred())
	var behavior storage.RuntimeBehavior
	Expect(json.Unmarshal(content, &behavior)).To(Succeed())
	return &behavior
}

func declareRuntimeBehaviorSpecs(runtime func() (context.Context, storage.Store, time.Time)) {
	Describe("runtime behavior ownership", func() {
		It("round trips sparse intent through persistence, reset, and checkpoint restore", func() {
			ctx, store, now := runtime()
			actor := testAccount(now)
			Expect(store.UpsertAccount(ctx, actor)).To(Succeed())
			explicit := false
			behavior, err := storage.NewRuntimeBehavior(config.Patch{QuietSuccess: &explicit})
			Expect(err).NotTo(HaveOccurred())
			change := storage.RuntimeSettingsChange{
				BotConfig: &behavior, ActorAccountID: actor.ID, ChangedAt: now,
				EffectiveSessionTTL: time.Hour,
			}
			saved, err := store.SaveRuntimeSettings(ctx, change)
			Expect(err).NotTo(HaveOccurred())
			Expect(saved.CheckpointID).NotTo(BeNil())
			read, err := store.GetRuntimeSettings(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(read.BotConfig.Patch().QuietSuccess).To(HaveValue(BeFalse()))
			Expect(read.BotConfig.Patch().QuietPending).To(BeNil())
			change.ExpectedRevision = read.Revision
			noop, err := store.SaveRuntimeSettings(ctx, change)
			Expect(err).NotTo(HaveOccurred())
			Expect(noop.CheckpointID).To(BeNil())
			change.BotConfig = &storage.RuntimeBehavior{}
			reset, err := store.SaveRuntimeSettings(ctx, change)
			Expect(err).NotTo(HaveOccurred())
			Expect(reset.Settings.BotConfig).To(BeNil())
			change.ExpectedRevision = reset.Settings.Revision
			change.BotConfig = nil
			noop, err = store.SaveRuntimeSettings(ctx, change)
			Expect(err).NotTo(HaveOccurred())
			Expect(noop.CheckpointID).To(BeNil())
			restored, err := store.RestoreRuntimeSettings(ctx, storage.RestoreRuntimeSettingsRequest{
				CheckpointID: *saved.CheckpointID, ExpectedRevision: reset.Settings.Revision,
				Side:           storage.SettingsCheckpointRestoreAfter,
				ActorAccountID: actor.ID, ChangedAt: now.Add(time.Minute),
				Runner: config.RunnerService, EffectiveSessionTTL: time.Hour,
			})
			Expect(err).NotTo(HaveOccurred())
			deployment := config.Default()
			deployment.QuietSuccess, deployment.QuietPending = true, true
			resolved := restored.Settings.BotConfig.Resolve(deployment)
			Expect(resolved.QuietSuccess).To(BeFalse())
			Expect(resolved.QuietPending).To(BeTrue())
		})

		It("preserves historical complete records when converting to sparse storage", func() {
			ctx, store, now := runtime()
			actor := testAccount(now)
			Expect(store.UpsertAccount(ctx, actor)).To(Succeed())
			level := "debug"
			saved, err := store.SaveRuntimeSettings(ctx, storage.RuntimeSettingsChange{
				LogLevel: &level, ActorAccountID: actor.ID, ChangedAt: now,
				EffectiveSessionTTL: time.Hour,
			})
			Expect(err).NotTo(HaveOccurred())
			legacy := config.Default()
			legacy.QuietSuccess = true
			content, err := json.Marshal(legacy)
			Expect(err).NotTo(HaveOccurred())
			raw := store.(settingsCheckpointFixtureStore)
			_, err = raw.DB().ExecContext(ctx, raw.Dialect().Rebind(
				"UPDATE runtime_settings SET bot_config = ? WHERE singleton = 1"), string(content))
			Expect(err).NotTo(HaveOccurred())
			read, err := store.GetRuntimeSettings(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(read.BotConfig.Patch().QuietSuccess).To(HaveValue(BeTrue()))
			Expect(read.BotConfig.Patch().QuietPending).NotTo(BeNil())
			noop, err := store.SaveRuntimeSettings(ctx, storage.RuntimeSettingsChange{
				BotConfig: read.BotConfig, LogLevel: &level, ExpectedRevision: saved.Settings.Revision,
				ActorAccountID: actor.ID, ChangedAt: now, EffectiveSessionTTL: time.Hour,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(noop.CheckpointID).To(BeNil())
			deployment := config.Default()
			deployment.QuietSuccess = false
			deployment.QuietPending = !legacy.QuietPending
			resolved := read.BotConfig.Resolve(deployment)
			Expect(resolved.QuietSuccess).To(BeTrue())
			Expect(resolved.QuietPending).To(Equal(legacy.QuietPending))
		})
	})
}
