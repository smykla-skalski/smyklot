package apply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

const digestInputFormatting = "formatting"

func repositoryFormattingPolicy(
	base config.FormattingPolicy,
	targetPatch config.Patch,
	repository storage.Repository,
) config.FormattingPolicy {
	policy := applyFormattingLayer(base, targetPatch.Formatting)
	if !repository.IgnoreRepositoryFile {
		policy = applyFormattingLayer(policy, repository.ConfigFilePatch.Formatting)
	}
	return applyFormattingLayer(policy, repository.ConfigPatch.Formatting)
}

// CurrentScopeDigest recomputes every setting that can affect planned bytes.
// Approval and execution use it to reject work created from an older scope.
func (s *Engine) CurrentScopeDigest(ctx context.Context, targetID string) (string, error) {
	configs, err := s.store.ListSyncConfigs(ctx, targetID)
	if err != nil {
		return "", fmt.Errorf("read sync configuration: %w", err)
	}
	target, err := s.store.GetTarget(ctx, targetID)
	if err != nil {
		return "", fmt.Errorf("read sync installation: %w", err)
	}
	held, err := s.syncInventoryFor(ctx, target, nil)
	if err != nil {
		return "", err
	}

	return scopeDigest(configs, held, s.formattingPolicy()), nil
}

func scopeDigest(
	configs []orgsync.Config,
	held syncInventory,
	formatting config.FormattingPolicy,
) string {
	return orgsync.DigestScopeWithInputs(configs, held.overrides, []orgsync.DigestInput{{
		Name: digestInputFormatting, Digest: formattingScopeDigest(formatting, held),
	}})
}

func applyFormattingLayer(
	base config.FormattingPolicy,
	patch *config.FormattingPatch,
) config.FormattingPolicy {
	if patch == nil {
		return base
	}

	return config.ApplyFormattingPatch(base, *patch)
}

type formattingScopeState struct {
	Runtime      config.FormattingPolicy          `json:"runtime"`
	Target       *config.FormattingPatch          `json:"target,omitempty"`
	Repositories []repositoryFormattingScopeState `json:"repositories"`
}

type repositoryFormattingScopeState struct {
	ID              string                       `json:"id"`
	IgnoreFile      bool                         `json:"ignore_file"`
	FileStatus      storage.RepositoryFileStatus `json:"file_status"`
	FilePath        string                       `json:"file_path"`
	FileError       *string                      `json:"file_error,omitempty"`
	FileFormatting  *config.FormattingPatch      `json:"file_formatting,omitempty"`
	PanelFormatting *config.FormattingPatch      `json:"panel_formatting,omitempty"`
}

func formattingScopeDigest(base config.FormattingPolicy, held syncInventory) string {
	repositories := slices.Clone(held.repositories)
	slices.SortFunc(repositories, func(one, two storage.Repository) int {
		if one.ID < two.ID {
			return -1
		}
		if one.ID > two.ID {
			return 1
		}
		return 0
	})
	state := formattingScopeState{
		Runtime: base,
		Target:  held.target.ConfigPatch.Formatting,
		Repositories: make(
			[]repositoryFormattingScopeState, 0, len(repositories),
		),
	}
	for _, repository := range repositories {
		state.Repositories = append(state.Repositories, repositoryFormattingScopeState{
			ID: repository.ID, IgnoreFile: repository.IgnoreRepositoryFile,
			FileStatus: repository.ConfigFileStatus, FilePath: repository.ConfigFilePath,
			FileError:       repository.ConfigFileError,
			FileFormatting:  repository.ConfigFilePatch.Formatting,
			PanelFormatting: repository.ConfigPatch.Formatting,
		})
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:])
}
