package panel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
)

func (s *Server) syncOverrideBatchDocument(
	r *http.Request,
	targetID string,
	repository storage.Repository,
	kind orgsync.Kind,
	document json.RawMessage,
	proposedFiles *orgsync.FileConfig,
) ([]byte, error) {
	if len(document) == 0 || bytes.Equal(bytes.TrimSpace(document), []byte("null")) {
		document = emptyDocument
	}
	if kind != orgsync.KindFiles {
		var empty map[string]json.RawMessage
		if err := json.Unmarshal(document, &empty); err != nil || empty == nil || len(empty) != 0 {
			return nil, fmt.Errorf(
				"%w: a repository can only switch %s on or off", orgsync.ErrInvalidConfig, kind,
			)
		}
		return nil, nil
	}
	var adjustments orgsync.FileOverride
	if err := decodeStrictly(document, &adjustments); err != nil {
		return nil, err
	}
	keeping, err := s.alreadyAdjustedForBatch(r, targetID, repository.ID)
	if err != nil {
		return nil, err
	}
	var files orgsync.FileConfig
	if adjustsBeyond(adjustments, keeping) {
		if proposedFiles != nil {
			files = *proposedFiles
		} else {
			files, err = s.syncFileConfig(r, targetID)
			if err != nil {
				return nil, err
			}
		}
	}
	if err := adjustments.ValidateAgainst(files, keeping); err != nil {
		return nil, err
	}

	return json.Marshal(adjustments)
}

func (s *Server) alreadyAdjustedForBatch(
	r *http.Request,
	targetID, repositoryID string,
) ([]string, error) {
	stored, err := s.store.GetSyncRepositoryOverride(
		r.Context(), targetID, repositoryID, orgsync.KindFiles,
	)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var saved orgsync.FileOverride
	if err := json.Unmarshal(stored.Document, &saved); err != nil {
		return nil, nil
	}

	return saved.Adjusted(), nil
}
