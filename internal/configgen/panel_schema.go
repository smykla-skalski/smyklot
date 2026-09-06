package configgen

import (
	"encoding/json"
	"fmt"
)

const (
	RepositoryPanelSchemaName = "repository-panel-v1.json"
	WorkspacePanelSchemaName  = "workspace-panel-v1.json"
	RepositoryPanelSchemaURL  = SchemaOrigin + "/schema/" + RepositoryPanelSchemaName
	WorkspacePanelSchemaURL   = SchemaOrigin + "/schema/" + WorkspacePanelSchemaName
	schemaWorkspace           = "workspace"
	schemaActorID             = "id"
	schemaActorType           = "type"
)

// RenderPanelSchemas extends the command schema without changing an existing
// editor's pinned contract. These describe TOML values, so a Sync document is a
// JSON string, not the object used internally for three-way reconciliation.
func RenderPanelSchemas(model Model) (map[string][]byte, error) {
	base, err := RenderSchema(model)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]byte, 2)
	for scope, name := range map[string]string{
		"repository": RepositoryPanelSchemaName, schemaWorkspace: WorkspacePanelSchemaName,
	} {
		var document schemaDocument
		if err := json.Unmarshal(base, &document); err != nil {
			return nil, fmt.Errorf("read command schema: %w", err)
		}
		document.ID = SchemaOrigin + "/schema/" + name
		document.Title = "Smyklot " + scope + " configuration with panel settings"
		document.Description = "Command and panel settings synchronized with this " + scope +
			" when configuration file sync is enabled in the panel. Access and connection permissions stay in the panel."
		document.Properties["panel"] = panelSectionSchema(scope)
		if scope == schemaWorkspace {
			delete(document.Properties, "runner")
			document.Required = []string{"panel"}
		}
		content, err := json.MarshalIndent(document, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode %s schema: %w", scope, err)
		}
		result[SchemaDir+"/"+name] = append(content, '\n')
	}
	return result, nil
}

func panelSectionSchema(scope string) schemaProperty {
	sync := make(map[string]schemaProperty, 4)
	for _, kind := range []string{"files", "labels", "settings", "rulesets"} {
		item := schemaObject("Shared "+kind+" configuration", map[string]schemaProperty{
			"enabled": {Type: jsonBoolean, Description: "Whether this configuration is synchronized"},
			"document": {
				Type: jsonString, ContentMediaType: "application/json",
				Description: "A JSON object written as a TOML string. Multiline literal strings preserve null values and file contents. The configuration is validated before import.",
			},
		}, "document")
		if scope == schemaWorkspace {
			item.Required = append(item.Required, "enabled")
		}
		sync[kind] = item
	}
	section := schemaObject("Versioned panel settings for this scope", map[string]schemaProperty{
		"version":  {Type: jsonInteger, Minimum: new(1), Maximum: new(1)},
		"scope":    {Type: jsonString, Enum: []string{scope}},
		"settings": panelSettingsSchema(scope),
		"sync":     schemaObject("Shared configurations. Omit a kind to remove its settings at this scope.", sync),
	}, "version", "scope")
	if scope == schemaWorkspace {
		section.Required = append(section.Required, "settings")
	}
	return section
}

func panelSettingsSchema(scope string) schemaProperty {
	patterns := schemaProperty{Type: jsonArray, Items: &schemaProperty{Type: jsonString, Pattern: `\S`}}
	includes := patterns
	includes.MinItems = new(1)
	duration := schemaProperty{
		Type:        jsonString,
		Description: "A nonnegative whole-second duration, for example 30s, 2m or 1h. Omit to inherit.",
	}
	properties := map[string]schemaProperty{
		"merge_mode": {Type: jsonString, Enum: []string{"checks", "labels"}},
		"protected_refs": schemaObject("Branches where merge protection applies", map[string]schemaProperty{
			"include": includes, "exclude": patterns,
		}, "include"),
		"merge_exceptions":    panelExceptionsSchema(),
		"quiet_period":        duration,
		"file_index_interval": duration,
	}
	settings := schemaObject("Omitted repository settings inherit workspace defaults", properties)
	if scope == schemaWorkspace {
		properties["repository_default_enabled"] = schemaProperty{Type: jsonBoolean}
		settings.Description = "Workspace defaults for repositories without their own overrides"
		settings.Required = []string{"repository_default_enabled", "merge_mode", "protected_refs"}
	} else {
		properties["enabled"] = schemaProperty{Type: jsonBoolean}
	}
	return settings
}

func panelExceptionsSchema() schemaProperty {
	actor := schemaObject("A GitHub ruleset bypass actor", map[string]schemaProperty{
		schemaActorID: {Type: jsonInteger, Minimum: new(0), Description: "GitHub actor ID. Omit for organization administrators and deploy keys."},
		schemaActorType: {Type: jsonString, Enum: []string{
			"Integration", "Team", "RepositoryRole", "OrganizationAdmin", "User", "DeployKey",
		}},
		"mode": {Type: jsonString, Enum: []string{"always", "pull_request", "exempt"}},
	}, schemaActorType, "mode")
	actor.OneOf = []schemaProperty{
		{Type: jsonObject, Required: []string{schemaActorID}, Properties: map[string]schemaProperty{
			schemaActorType: {Type: jsonString, Enum: []string{"Integration", "Team", "RepositoryRole", "User"}},
			schemaActorID:   {Type: jsonInteger, Minimum: new(1)},
		}},
		{Type: jsonObject, Properties: map[string]schemaProperty{
			schemaActorType: {Type: jsonString, Enum: []string{"OrganizationAdmin"}},
		}},
		{Type: jsonObject, Properties: map[string]schemaProperty{
			schemaActorType: {Type: jsonString, Enum: []string{"DeployKey"}},
			"mode":          {Type: jsonString, Enum: []string{"always", "exempt"}},
		}},
	}
	return schemaObject("Merge protection exceptions. Omit to preserve existing exceptions or inherit workspace defaults.", map[string]schemaProperty{
		"allow":  {Type: jsonBoolean, Description: "Allow these actors to bypass merge protection"},
		"actors": {Type: jsonArray, Items: &actor},
	})
}

func schemaObject(description string, properties map[string]schemaProperty, required ...string) schemaProperty {
	return schemaProperty{
		Type: jsonObject, Description: description, Properties: properties,
		AdditionalProperties: false, Required: required,
	}
}
