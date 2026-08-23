package panel

import (
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/internal/storage/storagetest"
)

func saveRuntimeSettingsCheckpoint(
	t *testing.T,
	harness *panelHarness,
	level string,
	expectedRevision int64,
) int64 {
	t.Helper()
	result, err := harness.store.SaveRuntimeSettings(t.Context(), storage.RuntimeSettingsChange{
		LogLevel:                      &level,
		EffectivePendingCIQuietPeriod: 30 * time.Second,
		EffectiveSessionTTL:           12 * time.Hour,
		ExpectedRevision:              expectedRevision,
		ActorAccountID:                "github:test:user:1",
		ChangedAt:                     harness.now,
	})
	if err != nil || result.CheckpointID == nil {
		t.Fatalf("save runtime settings checkpoint = %#v, %v", result, err)
	}

	return *result.CheckpointID
}

func rewriteSettingsCheckpointDocumentVersion(
	t *testing.T,
	harness *panelHarness,
	checkpointID int64,
	identity storage.SettingsCheckpointItemIdentity,
) {
	t.Helper()
	if err := storagetest.RewriteSettingsCheckpointDocumentVersion(
		t.Context(),
		harness.store,
		checkpointID,
		identity,
		storage.SettingsCheckpointDocumentVersion+1,
	); err != nil {
		t.Fatal(err)
	}
}
