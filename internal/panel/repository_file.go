package panel

import (
	"slices"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// Observation reports what was read independently of the panel's bypass switch.
// A newly discovered repository has not been checked, not a confirmed missing file.
type repositoryFileObservation struct {
	Status      string     `json:"status"`
	ObservedAt  *time.Time `json:"observed_at,omitempty"`
	SearchPaths []string   `json:"search_paths"`
}

func repositoryFileObservationDTO(repository storage.Repository) repositoryFileObservation {
	status := "unknown"
	if repository.ConfigFileObservedAt != nil {
		switch repository.ConfigFileObservedStatus {
		case storage.RepositoryFileMissing, storage.RepositoryFileValid, storage.RepositoryFileInvalid:
			status = string(repository.ConfigFileObservedStatus)
		}
	}

	return repositoryFileObservation{
		Status: status, ObservedAt: repository.ConfigFileObservedAt,
		SearchPaths: slices.Clone(config.RepoConfigPaths),
	}
}
