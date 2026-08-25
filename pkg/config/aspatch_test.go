package config_test

import (
	"reflect"
	"testing"
	"testing/quick"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// The panel resolves its process defaults by turning a Config into a patch and
// applying it to Default(). If AsPatch ever missed a field, that field would be
// silently reset to its default and the panel would report a setting nobody
// changed. This is that round trip, over values the test did not choose.
func TestAsPatchRoundTripsThroughApply(t *testing.T) {
	roundTrips := func(
		quietSuccess, quietReactions, quietPending bool,
		commands []string, aliases map[string]string, prefix string,
		mentions, bare, unapprove, reactions, deleted, selfApproval, draftMerges bool,
		action bool,
	) bool {
		runner := config.RunnerService
		if action {
			runner = config.RunnerAction
		}

		// A nil slice and a nil map are not values Config carries: Default
		// makes both empty, and resolution copies into them.
		if commands == nil {
			commands = []string{}
		}

		if aliases == nil {
			aliases = map[string]string{}
		}

		original := config.Config{
			QuietSuccess: quietSuccess, QuietReactions: quietReactions,
			QuietPending: quietPending, AllowedCommands: commands,
			CommandAliases: aliases, CommandPrefix: prefix,
			DisableMentions: mentions, DisableBareCommands: bare,
			DisableUnapprove: unapprove, DisableReactions: reactions,
			DisableDeletedComments: deleted, AllowSelfApproval: selfApproval,
			AllowDraftMerges: draftMerges, Runner: runner,
		}

		restored := config.ApplyPatch(config.Default(), original.AsPatch())

		return reflect.DeepEqual(*restored, original)
	}

	if err := quick.Check(roundTrips, &quick.Config{MaxCount: 500}); err != nil {
		t.Errorf("AsPatch does not round trip: %v", err)
	}
}

// AsPatch must hand out its own copies. Sharing the slice or the map would mean
// editing a patch edited the configuration it came from, and the two callers
// that hold both would see each other's writes.
func TestAsPatchDoesNotAliasItsSource(t *testing.T) {
	original := config.Config{
		AllowedCommands: []string{"approve"},
		CommandAliases:  map[string]string{"a": "approve"},
	}

	patch := original.AsPatch()

	(*patch.AllowedCommands)[0] = "merge"
	(*patch.CommandAliases)["a"] = "merge"
	*patch.CommandPrefix = "!"

	if original.AllowedCommands[0] != "approve" {
		t.Errorf("AllowedCommands aliased: %v", original.AllowedCommands)
	}

	if original.CommandAliases["a"] != "approve" {
		t.Errorf("CommandAliases aliased: %v", original.CommandAliases)
	}

	if original.CommandPrefix != "" {
		t.Errorf("CommandPrefix aliased: %q", original.CommandPrefix)
	}

	// The other direction needs no test of its own: sharing a slice or a map is
	// symmetric, so an implementation that let a patch edit its source would let
	// the source edit the patch too, and fail here just the same.
	original.AllowedCommands[0] = "close"

	if (*patch.AllowedCommands)[0] == "close" {
		t.Error("the patch saw a later edit to its source")
	}
}
