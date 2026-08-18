package storage_test

import (
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// TestGrantsAgreeWithTheInstallationTheyWereReadFrom holds the two answers to
// one question together.
//
// The planner asks the stored row and the executor asks it again minutes later;
// the catalog asks the installation GitHub just described. Two spellings of one
// rule is how those come to disagree about whether an installation may act, and
// the disagreement is silent - one of them plans work the other refuses.
//
// Held by a test rather than by shared code, because the alternative is
// pkg/github importing the sync domain to borrow four lines: the client is the
// layer that knows nothing about what Smyklot does with GitHub, and that is
// worth more than the duplication. What is not enforced decays, so this
// enforces it.
func TestGrantsAgreeWithTheInstallationTheyWereReadFrom(t *testing.T) {
	t.Parallel()

	// Every level GitHub spells for a repository permission, plus the two that
	// are not levels at all: the empty string a missing key gives, and a value
	// no version of this knows.
	for _, level := range []string{
		"", "none", "read", "triage", "write", "maintain", "admin", "sudo",
	} {
		installation := github.Installation{
			Permissions: map[string]string{"administration": level},
		}
		target := storage.Target{
			Permissions: map[string]string{"administration": level},
		}

		if installation.Grants("administration") != target.Grants("administration") {
			t.Errorf("level %q: the installation says %t and the stored row says %t",
				level, installation.Grants("administration"),
				target.Grants("administration"))
		}
	}

	// And the answer nobody reported at all, which is the one that decides
	// whether a malformed listing is read as permission.
	if (github.Installation{}).Grants("administration") !=
		(storage.Target{}).Grants("administration") {
		t.Error("an installation with no permissions at all is read two ways")
	}
	if (storage.Target{}).Grants("administration") {
		t.Error("permissions nobody could read were taken as granting something")
	}
}

func TestTargetReadCapabilityAcceptsEveryGrantedLevel(t *testing.T) {
	t.Parallel()
	for _, level := range []string{"read", "write", "admin"} {
		target := storage.Target{Permissions: map[string]string{"merge_queues": level}}
		if !target.CanRead("merge_queues") {
			t.Errorf("level %q did not grant read capability", level)
		}
	}
	for _, level := range []string{"", "none"} {
		target := storage.Target{Permissions: map[string]string{"merge_queues": level}}
		if target.CanRead("merge_queues") {
			t.Errorf("level %q granted read capability", level)
		}
	}
}
