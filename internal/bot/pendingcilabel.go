package bot

import "github.com/smykla-skalski/smyklot/pkg/github"

const (
	// LegacyLabelPendingCIServiceOwner is removed from pull requests created by
	// older service versions. New requests use only their method label.
	LegacyLabelPendingCIServiceOwner = "smyklot:pending:ci:service"

	// LabelPendingCIMerge indicates PR is waiting for CI before merge
	LabelPendingCIMerge = "smyklot:pending:ci"

	// LabelPendingCISquash indicates PR is waiting for CI before squash merge
	LabelPendingCISquash = "smyklot:pending:ci:squash"

	// LabelPendingCIRebase indicates PR is waiting for CI before rebase merge
	LabelPendingCIRebase = "smyklot:pending:ci:rebase"

	// LabelPendingCIMergeRequired indicates PR is waiting for required CI only before merge
	LabelPendingCIMergeRequired = "smyklot:pending:ci:required"

	// LabelPendingCISquashRequired indicates PR is waiting for required CI only before squash merge
	LabelPendingCISquashRequired = "smyklot:pending:ci:squash:required"

	// LabelPendingCIRebaseRequired indicates PR is waiting for required CI only before rebase merge
	LabelPendingCIRebaseRequired = "smyklot:pending:ci:rebase:required"

	// Legacy pending-CI labels remain readable during the organization migration.
	LegacyLabelPendingCIMerge          = "smyklot:pending-ci"
	LegacyLabelPendingCISquash         = "smyklot:pending-ci:squash"
	LegacyLabelPendingCIRebase         = "smyklot:pending-ci:rebase"
	LegacyLabelPendingCIMergeRequired  = "smyklot:pending-ci:required"
	LegacyLabelPendingCISquashRequired = "smyklot:pending-ci:squash:required"
	LegacyLabelPendingCIRebaseRequired = "smyklot:pending-ci:rebase:required"
)

// ParsePendingCILabel parses a pending-ci label and returns the merge method and required flag
//
// Returns:
// - MergeMethod, requiredOnly bool, and label name if valid pending-ci label
// - Empty string if not a pending-ci label
func ParsePendingCILabel(label string) (github.MergeMethod, bool, string) {
	switch label {
	case LabelPendingCIMerge, LegacyLabelPendingCIMerge:
		return github.MergeMethodMerge, false, label
	case LabelPendingCISquash, LegacyLabelPendingCISquash:
		return github.MergeMethodSquash, false, label
	case LabelPendingCIRebase, LegacyLabelPendingCIRebase:
		return github.MergeMethodRebase, false, label
	case LabelPendingCIMergeRequired, LegacyLabelPendingCIMergeRequired:
		return github.MergeMethodMerge, true, label
	case LabelPendingCISquashRequired, LegacyLabelPendingCISquashRequired:
		return github.MergeMethodSquash, true, label
	case LabelPendingCIRebaseRequired, LegacyLabelPendingCIRebaseRequired:
		return github.MergeMethodRebase, true, label
	default:
		return "", false, ""
	}
}
