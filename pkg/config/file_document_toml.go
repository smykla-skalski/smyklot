package config

import "encoding/json"

// Keep presentation outside the semantic document. In particular, JSON numbers
// and null must never pass through floating point or TOML's absent-value rules.
type tomlFileDocument struct {
	Patch
	Panel *tomlPanelSection `toml:"panel,omitempty"`
}

type tomlPanelSection struct {
	Version  int                      `toml:"version"`
	Scope    PanelFileScope           `toml:"scope"`
	Settings PanelFileSettings        `toml:"settings"`
	Sync     map[string]tomlPanelSync `toml:"sync,omitempty"`
}

type tomlPanelSync struct {
	Enabled  *bool  `toml:"enabled,omitempty"`
	Document string `toml:"document,multiline"`
}

func (wire tomlFileDocument) document() FileDocument {
	document := FileDocument{Patch: wire.Patch}
	if wire.Panel == nil {
		return document
	}
	document.Panel = &PanelFileSection{
		Version: wire.Panel.Version, Scope: wire.Panel.Scope, Settings: wire.Panel.Settings,
	}
	if wire.Panel.Sync != nil {
		document.Panel.Sync = make(map[string]PanelFileSync, len(wire.Panel.Sync))
		for kind, item := range wire.Panel.Sync {
			document.Panel.Sync[kind] = PanelFileSync{
				Enabled: item.Enabled, Document: json.RawMessage(item.Document),
			}
		}
	}
	return document
}

func fileDocumentTOML(document FileDocument) tomlFileDocument {
	wire := tomlFileDocument{Patch: document.Patch}
	if document.Panel == nil {
		return wire
	}
	wire.Panel = &tomlPanelSection{
		Version: document.Panel.Version, Scope: document.Panel.Scope, Settings: document.Panel.Settings,
	}
	if document.Panel.Sync != nil {
		wire.Panel.Sync = make(map[string]tomlPanelSync, len(document.Panel.Sync))
		for kind, item := range document.Panel.Sync {
			wire.Panel.Sync[kind] = tomlPanelSync{
				Enabled: item.Enabled, Document: string(item.Document),
			}
		}
	}
	return wire
}
