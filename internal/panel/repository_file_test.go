package panel

import (
	"reflect"
	"testing"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestRepositoryFileObservationIsIndependentOfBypass(t *testing.T) {
	now := time.Now().UTC()
	for _, status := range []storage.RepositoryFileStatus{
		storage.RepositoryFileValid, storage.RepositoryFileInvalid, storage.RepositoryFileMissing,
	} {
		t.Run(string(status), func(t *testing.T) {
			repository := storage.Repository{
				IgnoreRepositoryFile: true, ConfigFileStatus: storage.RepositoryFileBypassed,
				ConfigFileObservedStatus: status, ConfigFileObservedAt: &now,
			}
			answer := repositoryDetailDTO(testRuntimeValues(), storage.Target{}, repository)
			if answer.ConfigFileObservation.Status != string(status) ||
				answer.Repository.ConfigFileStatus != storage.RepositoryFileBypassed {
				t.Fatalf("observation was masked by bypass: %#v", answer.ConfigFileObservation)
			}
			if !reflect.DeepEqual(answer.ConfigFileObservation.SearchPaths, config.RepoConfigPaths) {
				t.Fatal("panel search order drifted from discovery")
			}
			if !answer.ConfigFileObservation.ObservedAt.Equal(now) {
				t.Fatal("observation clock lost")
			}
		})
	}
}

func TestRepositoryFileWithoutObservationIsUnknown(t *testing.T) {
	answer := repositoryFileObservationDTO(storage.Repository{
		ConfigFileStatus: storage.RepositoryFileMissing,
	})
	if answer.Status != "unknown" || answer.ObservedAt != nil {
		t.Fatalf("unchecked file presented as missing: %#v", answer)
	}
}
