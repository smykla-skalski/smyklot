package configsync

import (
	"bytes"
	"fmt"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

const problemUnwritableFile = "unwritable_file"

func renderPublication(file RemoteFile, semantic []byte) ([]byte, error) {
	document, err := DecodeDocument(semantic, file.Location.Scope)
	if err != nil {
		return nil, err
	}
	document.Runner = file.Source.Runner
	content, err := config.RenderFileDocument(document)
	if err != nil || file.Content == nil || file.Migrate {
		return content, err
	}
	original := bytes.TrimPrefix(file.Content, []byte{0xef, 0xbb, 0xbf})
	content, err = preserveFilePresentation(original, content, file.Source.Snapshot.Document, semantic)
	if err != nil {
		return nil, err
	}
	content, err = filemerge.RewriteTOML(original, content)
	if err != nil {
		return nil, &BlockedError{Code: problemUnwritableFile, Message: fmt.Sprintf("%s could not be updated without losing its comments: %v", file.Path, err)}
	}
	schema := config.RepositoryPanelSchemaURL
	if file.Location.Scope == config.PanelFileWorkspace {
		schema = config.WorkspacePanelSchemaURL
	}
	content = bytes.TrimPrefix(content, []byte{0xef, 0xbb, 0xbf})
	first, rest, hasLine := bytes.Cut(content, []byte("\n"))
	if bytes.HasPrefix(bytes.TrimSpace(first), []byte("#:schema ")) {
		if hasLine {
			content = rest
		} else {
			content = nil
		}
	}
	content = append([]byte("#:schema "+schema+"\n"), content...)
	content = append(bytes.TrimRight(content, "\r\n"), '\n')
	if len(content) > config.MaxFileDocumentBytes {
		return nil, &BlockedError{Code: problemUnwritableFile, Message: "The updated configuration exceeds the file size limit"}
	}
	// Keep the publication proof at the consumer boundary: preserved comments,
	// schemas and whitespace may not change the values that were reconciled.
	checked, err := ReadFileSource(config.FormatTOML, content, file.Location.Scope)
	if err != nil {
		return nil, err
	}
	same, err := Equivalent(checked.Snapshot.Document, semantic)
	if err != nil || !same {
		return nil, &BlockedError{Code: problemUnwritableFile, Message: "The updated configuration does not match the reconciled settings"}
	}
	return content, nil
}
