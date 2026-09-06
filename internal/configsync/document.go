package configsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// PanelSnapshot carries values and optimistic revisions from the same scope.
// A nil Repository describes workspace settings, including every Sync kind.
type PanelSnapshot struct {
	Target        storage.Target
	Repository    *storage.Repository
	SyncConfigs   []orgsync.Config
	SyncOverrides []orgsync.RepositoryOverride
}

func (snapshot PanelSnapshot) Scope() config.PanelFileScope {
	if snapshot.Repository == nil {
		return config.PanelFileWorkspace
	}
	return config.PanelFileRepository
}

func (snapshot PanelSnapshot) OwnerRevision() int64 {
	if snapshot.Repository == nil {
		return snapshot.Target.Revision
	}
	return snapshot.Repository.Revision
}

func (snapshot PanelSnapshot) RepositoryID() string {
	if snapshot.Repository == nil {
		return ""
	}
	return snapshot.Repository.ID
}

func (snapshot PanelSnapshot) SyncRevisions() map[orgsync.Kind]int64 {
	revisions := make(map[orgsync.Kind]int64, len(orgsync.Kinds()))
	for _, kind := range orgsync.Kinds() {
		revisions[kind] = 0
	}
	if snapshot.Repository == nil {
		for _, item := range snapshot.SyncConfigs {
			revisions[item.Kind] = item.Revision
		}
	} else {
		for _, item := range snapshot.SyncOverrides {
			revisions[item.Kind] = item.Revision
		}
	}
	return revisions
}

// Document excludes control-plane authorization and the file-owned Runner.
// Branch exclusions and sparse nil/false values are carried without flattening.
func (snapshot PanelSnapshot) Document() (config.FileDocument, error) {
	section := &config.PanelFileSection{Version: 1, Scope: snapshot.Scope(), Sync: map[string]config.PanelFileSync{}}
	document := config.FileDocument{Panel: section}
	if snapshot.Repository == nil {
		target := snapshot.Target
		document.Patch = target.ConfigPatch
		section.Settings = config.PanelFileSettings{
			RepositoryDefaultEnabled: new(target.RepositoryDefaultEnabled),
			MergeMode:                new(string(target.PendingCIModeDefault)),
			ProtectedRefs:            fileRefs(&target.PendingCIBranchPatternsDefault),
			MergeExceptions:          fileExceptions(target.PendingCIBypassPolicyDefault),
			QuietPeriod:              fileDuration(target.PendingCIQuietPeriodOverride),
			FileIndexInterval:        fileDuration(target.PathIndexIntervalOverride),
		}
		for _, item := range snapshot.SyncConfigs {
			if item.TargetID != target.ID || !item.Kind.Valid() {
				return config.FileDocument{}, errors.New("sync configuration scope mismatch")
			}
			if _, exists := section.Sync[string(item.Kind)]; exists {
				return config.FileDocument{}, errors.New("duplicate sync configuration")
			}
			section.Sync[string(item.Kind)] = config.PanelFileSync{Enabled: new(item.Enabled), Document: slices.Clone(item.Document)}
		}
	} else {
		repository := *snapshot.Repository
		if repository.TargetID != snapshot.Target.ID {
			return config.FileDocument{}, errors.New("repository configuration scope mismatch")
		}
		document.Patch = repository.ConfigPatch
		section.Settings = config.PanelFileSettings{
			Enabled: repository.EnabledOverride, MergeMode: fileMode(repository.PendingCIModeOverride),
			ProtectedRefs:     fileRefs(repository.PendingCIBranchPatternsOverride),
			MergeExceptions:   fileExceptions(repository.PendingCIBypassPolicyOverride),
			QuietPeriod:       fileDuration(repository.PendingCIQuietPeriodOverride),
			FileIndexInterval: fileDuration(repository.PathIndexIntervalOverride),
		}
		for _, item := range snapshot.SyncOverrides {
			if item.RepositoryID != repository.ID || !item.Kind.Valid() {
				return config.FileDocument{}, errors.New("sync override scope mismatch")
			}
			if _, exists := section.Sync[string(item.Kind)]; exists {
				return config.FileDocument{}, errors.New("duplicate sync override")
			}
			content := slices.Clone(item.Document)
			if len(bytes.TrimSpace(content)) == 0 {
				content = []byte("{}")
			}
			section.Sync[string(item.Kind)] = config.PanelFileSync{Enabled: item.Enabled, Document: content}
		}
	}
	document.Runner = nil
	return document, document.Panel.Validate()
}

func (snapshot PanelSnapshot) JSON() ([]byte, error) {
	document, err := snapshot.Document()
	if err != nil {
		return nil, err
	}
	return config.EncodeJSONDocument(document)
}

// DecodeDocument validates the semantic envelope after a merge or resolution.
// It does not authorize writing those values; storage verifies scope and opt-in.
func DecodeDocument(content []byte, scope config.PanelFileScope) (config.FileDocument, error) {
	if _, err := config.DecodeJSONObject(content); err != nil {
		return config.FileDocument{}, err
	}
	var document config.FileDocument
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return config.FileDocument{}, err
	}
	if document.Panel == nil || document.Panel.Scope != scope {
		return config.FileDocument{}, errors.New("configuration document scope does not match its connection")
	}
	if document.Runner != nil {
		return config.FileDocument{}, errors.New("configuration sync cannot change the file-owned runner")
	}
	if err := document.Panel.Validate(); err != nil {
		return config.FileDocument{}, err
	}
	// Exercise Patch normalization and the final renderer before importing.
	if _, err := config.RenderFileDocument(document); err != nil {
		return config.FileDocument{}, err
	}
	return document, nil
}

func fileRefs(value *storage.PendingCIBranchPatterns) *config.PanelFileRefs {
	if value == nil {
		return nil
	}
	return &config.PanelFileRefs{Include: append([]string{}, value.Include...), Exclude: append([]string{}, value.Exclude...)}
}

func fileMode(value *storage.PendingCIMode) *string {
	if value == nil {
		return nil
	}
	return new(string(*value))
}

func fileDuration(value *time.Duration) *string {
	if value == nil {
		return nil
	}
	return new(value.String())
}

func fileExceptions(value *storage.PendingCIBypassPolicy) *config.PanelFileExceptions {
	if value == nil {
		return nil
	}
	actors := make([]config.PanelFileActor, 0, len(value.Actors))
	for _, actor := range value.Actors {
		var id *int64
		if actor.ActorID != 0 {
			id = new(actor.ActorID)
		}
		actors = append(actors, config.PanelFileActor{ID: id, Type: actor.ActorType, Mode: actor.Mode})
	}
	return &config.PanelFileExceptions{Allow: value.Allow, Actors: actors}
}

func durationValue(value *string) (*time.Duration, error) {
	if value == nil {
		return nil, nil
	}
	duration, err := time.ParseDuration(*value)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration duration: %w", err)
	}
	return &duration, nil
}
