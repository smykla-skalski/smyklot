package configsync

import (
	"errors"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// FileSource separates the settings participating in synchronization from the
// repository-owned runner. Publication must retain Runner from the same observed
// file, never from a panel snapshot or a conflict resolution document.
type FileSource struct {
	Snapshot Snapshot
	Runner   *config.Runner
}

// ReadFileSource prepares an observed file for semantic comparison. Nil means
// missing; a present empty repository file is a valid, empty set of overrides.
// Legacy repository files gain an empty panel envelope only for comparison.
func ReadFileSource(format config.Format, content []byte, scope config.PanelFileScope) (FileSource, error) {
	if scope != config.PanelFileRepository && scope != config.PanelFileWorkspace {
		return FileSource{}, errors.New("unknown configuration file connection scope")
	}
	if content == nil {
		return FileSource{}, nil
	}
	document, err := config.ParseFileDocument(format, content)
	if err != nil {
		return FileSource{}, err
	}
	if document.Panel == nil && scope == config.PanelFileRepository {
		document.Panel = &config.PanelFileSection{Version: 1, Scope: scope}
	}
	runner := document.Runner
	if scope == config.PanelFileWorkspace && runner != nil {
		return FileSource{}, errors.New("runner belongs in a repository's own file, not workspace settings")
	}
	document.Runner = nil
	document, err = validateDocument(document, scope)
	if err != nil {
		return FileSource{}, err
	}
	semantic, err := config.EncodeJSONDocument(document)
	if err != nil {
		return FileSource{}, err
	}
	return FileSource{Snapshot: Snapshot{Exists: true, Document: semantic}, Runner: runner}, nil
}
