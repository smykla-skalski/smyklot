package panel

import (
	"encoding/json"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
)

func syncConfigDisplayDocument(config orgsync.Config) json.RawMessage {
	document := documentOrEmpty(config.Document)
	if config.Kind == orgsync.KindFiles {
		return readableTemplateDocument(document)
	}
	return document
}

// Present legacy templates with the same terminator the planner writes. Keep
// unknown fields and invalid source intact so the panel can still repair it.
// RawMessage also preserves large JSON numbers in future configuration fields.
func readableTemplateDocument(document json.RawMessage) json.RawMessage {
	var object map[string]json.RawMessage
	if json.Unmarshal(document, &object) != nil {
		return document
	}
	var files []map[string]json.RawMessage
	if json.Unmarshal(object["files"], &files) != nil {
		return document
	}
	changed := false
	for _, file := range files {
		var path, source string
		if json.Unmarshal(file["path"], &path) != nil || json.Unmarshal(file["content"], &source) != nil {
			continue
		}
		content, err := filemerge.NormalizeTemplate(path, []byte(source))
		if err != nil || string(content) == source {
			continue
		}
		file["content"], _ = json.Marshal(string(content))
		changed = true
	}
	if !changed {
		return document
	}
	object["files"], _ = json.Marshal(files)
	result, _ := json.Marshal(object)
	return result
}
