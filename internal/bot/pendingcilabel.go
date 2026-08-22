package bot

import "github.com/smykla-skalski/smyklot/pkg/github"

// ParsePendingCILabel parses a pending-ci label and returns the merge method and required flag
//
// Returns:
// - MergeMethod, requiredOnly bool, and label name if valid pending-ci label
// - Empty string if not a pending-ci label
func ParsePendingCILabel(label string) (github.MergeMethod, bool, string) {
	switch label {
	case github.LabelPendingCIMerge, github.LegacyLabelPendingCIMerge:
		return github.MergeMethodMerge, false, label
	case github.LabelPendingCISquash, github.LegacyLabelPendingCISquash:
		return github.MergeMethodSquash, false, label
	case github.LabelPendingCIRebase, github.LegacyLabelPendingCIRebase:
		return github.MergeMethodRebase, false, label
	case github.LabelPendingCIMergeRequired, github.LegacyLabelPendingCIMergeRequired:
		return github.MergeMethodMerge, true, label
	case github.LabelPendingCISquashRequired, github.LegacyLabelPendingCISquashRequired:
		return github.MergeMethodSquash, true, label
	case github.LabelPendingCIRebaseRequired, github.LegacyLabelPendingCIRebaseRequired:
		return github.MergeMethodRebase, true, label
	default:
		return "", false, ""
	}
}
