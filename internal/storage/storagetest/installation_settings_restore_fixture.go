package storagetest

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func rewriteCheckpointItem(
	harness Harness,
	ctx context.Context,
	checkpointID int64,
	identity storage.SettingsCheckpointItemIdentity,
	documentVersion int,
	document any,
) {
	GinkgoHelper()
	encoded, err := json.Marshal(document)
	Expect(err).NotTo(HaveOccurred())
	harness.RewriteSettingsCheckpointItem(ctx, SettingsCheckpointItemRewrite{
		CheckpointID: checkpointID, Identity: identity,
		DocumentVersion: documentVersion, AfterDocument: encoded,
	})
}

func inspectedCheckpointItem(
	inspection storage.SettingsCheckpointInspection,
	identity storage.SettingsCheckpointItemIdentity,
) storage.SettingsCheckpointInspectionItem {
	GinkgoHelper()
	for _, item := range inspection.Items {
		if item.Identity == identity {
			return item
		}
	}
	Fail("settings checkpoint inspection item was not found")

	return storage.SettingsCheckpointInspectionItem{}
}
