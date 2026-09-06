package storagetest

import (
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

// Seed both scopes before Sync settings exist. The preceding settings saves
// enabled each connection at revision 2, so all Sync revisions are still zero.
func (s *seeder) seedConfigFileConnections() error {
	for _, repositoryID := range []string{"", "repo-1"} {
		revisions := make(map[orgsync.Kind]int64)
		for _, kind := range orgsync.Kinds() {
			revisions[kind] = 0
		}
		_, err := s.store.SaveConfigFileState(s.ctx, storage.ConfigFileStateChange{
			TargetID: s.target.TargetID, RepositoryID: repositoryID, OwnerRevision: 2,
			SyncRevisions: revisions, ChangedAt: s.now.Add(3 * time.Minute),
			Document: []byte(`{"baseline":{"command_prefix":"/seed "},"observed_head":"seed-head"}`),
		})
		if err != nil {
			return err
		}
	}
	return nil
}
