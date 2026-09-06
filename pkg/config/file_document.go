package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// FileDocument keeps the existing sparse configuration at the file root. The
// versioned panel section carries settings outside the command configuration.
// Reading a file does not authorize importing its panel section: the service
// must first verify an enabled connection to this repository and scope.
type FileDocument struct {
	Patch
	Panel *PanelFileSection `toml:"panel,omitempty" json:"panel,omitempty"`
}

type PanelFileScope string

const (
	PanelFileRepository PanelFileScope = "repository"
	PanelFileWorkspace  PanelFileScope = "workspace"
)

// PanelFileSection version 1 is a sparse, human-editable file contract. Absence
// means inheritance, not an explicit false, zero, empty list, or empty object.
type PanelFileSection struct {
	Version  int                      `toml:"version" json:"version"`
	Scope    PanelFileScope           `toml:"scope" json:"scope"`
	Settings PanelFileSettings        `toml:"settings" json:"settings"`
	Sync     map[string]PanelFileSync `toml:"sync,omitempty" json:"sync,omitempty"`
}

// PanelFileSettings uses durations people can edit, instead of storage's
// nanoseconds. Connection controls and panel access are intentionally not file
// settings: a file cannot authorize its own import or grant access to the panel.
type PanelFileSettings struct {
	Enabled                  *bool                `toml:"enabled,omitempty" json:"enabled,omitempty"`
	RepositoryDefaultEnabled *bool                `toml:"repository_default_enabled,omitempty" json:"repository_default_enabled,omitempty"`
	MergeMode                *string              `toml:"merge_mode,omitempty" json:"merge_mode,omitempty"`
	ProtectedRefs            *PanelFileRefs       `toml:"protected_refs,omitempty" json:"protected_refs,omitempty"`
	MergeExceptions          *PanelFileExceptions `toml:"merge_exceptions,omitempty" json:"merge_exceptions,omitempty"`
	QuietPeriod              *string              `toml:"quiet_period,omitempty" json:"quiet_period,omitempty"`
	FileIndexInterval        *string              `toml:"file_index_interval,omitempty" json:"file_index_interval,omitempty"`
}

type PanelFileRefs struct {
	Include []string `toml:"include" json:"include"`
	Exclude []string `toml:"exclude" json:"exclude"`
}

type PanelFileExceptions struct {
	Allow  bool             `toml:"allow" json:"allow"`
	Actors []PanelFileActor `toml:"actors" json:"actors"`
}

type PanelFileActor struct {
	ID   *int64 `toml:"id,omitempty" json:"id,omitempty"`
	Type string `toml:"type" json:"type"`
	Mode string `toml:"mode" json:"mode"`
}

// Document is the existing Sync JSON document. TOML cannot express JSON null,
// which file adjustments use to delete keys. The TOML wire representation uses
// a multiline JSON string, while reconciliation sees a JSON object and can merge
// independent edits within it. Domain validation still happens before import.
type PanelFileSync struct {
	Enabled  *bool           `json:"enabled,omitempty"`
	Document json.RawMessage `json:"document"`
}

// ParseFileDocument accepts legacy files unchanged and strictly decodes the
// versioned TOML panel envelope. Unknown settings are never discarded.
func ParseFileDocument(format Format, content []byte) (FileDocument, error) {
	if len(content) > MaxFileDocumentBytes {
		return FileDocument{}, errors.New("configuration file exceeds its size limit")
	}
	if format != FormatTOML {
		var patch Patch
		if err := decode(format, content, &patch); err != nil {
			return FileDocument{}, err
		}
		if err := patch.normalize(); err != nil {
			return FileDocument{}, err
		}
		return FileDocument{Patch: patch}, nil
	}
	var wire tomlFileDocument
	decoder := toml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(content, byteOrderMark)))
	decoder.DisallowUnknownFields()
	if err := emptyIsNothing(unknownSettings(decoder.Decode(&wire))); err != nil {
		return FileDocument{}, fmt.Errorf("decode configuration file: %w", err)
	}
	document := wire.document()
	if err := document.Patch.normalize(); err != nil {
		return FileDocument{}, err
	}
	if err := document.Panel.Validate(); err != nil {
		return FileDocument{}, err
	}
	return document, nil
}

// RenderFileDocument writes a single complete file with a terminal newline.
func RenderFileDocument(document FileDocument) ([]byte, error) {
	if err := document.Patch.normalize(); err != nil {
		return nil, err
	}
	if err := document.Panel.Validate(); err != nil {
		return nil, err
	}
	content, err := toml.Marshal(fileDocumentTOML(document))
	if err != nil {
		return nil, fmt.Errorf("render configuration file: %w", err)
	}
	content = append(bytes.TrimRight(content, "\n"), '\n')
	if len(content) > MaxFileDocumentBytes {
		return nil, errors.New("rendered configuration file exceeds its size limit")
	}
	return content, nil
}

func (section *PanelFileSection) Validate() error {
	if section == nil {
		return nil
	}
	if section.Version != 1 {
		return fmt.Errorf("unsupported panel file version %d", section.Version)
	}
	switch section.Scope {
	case PanelFileRepository:
		if section.Settings.RepositoryDefaultEnabled != nil {
			return errors.New("repository files cannot change workspace defaults")
		}
	case PanelFileWorkspace:
		if section.Settings.Enabled != nil {
			return errors.New("workspace files use repository_default_enabled instead of enabled")
		}
		if section.Settings.RepositoryDefaultEnabled == nil || section.Settings.MergeMode == nil ||
			section.Settings.ProtectedRefs == nil {
			return errors.New("workspace files require repository_default_enabled, merge_mode and protected_refs")
		}
	default:
		return fmt.Errorf("unknown panel file scope %q", section.Scope)
	}
	if err := section.Settings.validate(); err != nil {
		return err
	}
	for kind, item := range section.Sync {
		switch kind {
		case "files", "labels", "settings", "rulesets":
		default:
			return fmt.Errorf("unknown sync kind %q", kind)
		}
		if section.Scope == PanelFileWorkspace && item.Enabled == nil {
			return fmt.Errorf("workspace sync %s requires enabled", kind)
		}
		if _, err := DecodeJSONObject(item.Document); err != nil {
			return fmt.Errorf("sync %s document: %w", kind, err)
		}
	}
	return nil
}

func (settings PanelFileSettings) validate() error {
	if settings.MergeMode != nil && *settings.MergeMode != "checks" && *settings.MergeMode != "labels" {
		return fmt.Errorf("unknown merge mode %q", *settings.MergeMode)
	}
	if settings.ProtectedRefs != nil {
		if len(settings.ProtectedRefs.Include) == 0 {
			return errors.New("protected refs require at least one included pattern")
		}
		for _, pattern := range append(append([]string{}, settings.ProtectedRefs.Include...), settings.ProtectedRefs.Exclude...) {
			if strings.TrimSpace(pattern) == "" {
				return errors.New("protected ref patterns cannot be blank")
			}
		}
	}
	for name, value := range map[string]*string{
		"quiet_period": settings.QuietPeriod, "file_index_interval": settings.FileIndexInterval,
	} {
		if value == nil {
			continue
		}
		duration, err := time.ParseDuration(*value)
		if err != nil || duration < 0 || duration%time.Second != 0 {
			return fmt.Errorf("%s must be a nonnegative whole-second duration", name)
		}
	}
	return nil
}
