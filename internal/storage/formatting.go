package storage

import "github.com/smykla-skalski/smyklot/pkg/config"

// RepositoryFormattingPolicy resolves the persisted layers used by both sync
// planning and status reads. The repository file is omitted when bypassed.
func RepositoryFormattingPolicy(
	base config.FormattingPolicy,
	targetPatch config.Patch,
	repository Repository,
) config.FormattingPolicy {
	policy := applyFormattingLayer(base, targetPatch.Formatting)
	if !repository.IgnoreRepositoryFile {
		policy = applyFormattingLayer(policy, repository.ConfigFilePatch.Formatting)
	}
	return applyFormattingLayer(policy, repository.ConfigPatch.Formatting)
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
