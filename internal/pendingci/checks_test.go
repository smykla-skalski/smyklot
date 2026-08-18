package pendingci

import (
	"strings"
	"testing"
	"time"
)

func TestEnsureCheckSlotRejectsGitHubActionFieldsOverTheirLimits(t *testing.T) {
	t.Parallel()
	request := EnsureCheckSlotRequest{
		TargetID: "target", InstallationID: 1,
		RepositoryID: "repository", RepositoryFullName: "owner/repository",
		PullRequest: 1, HeadSHA: "head", AppID: 2, Name: "check",
		ExternalID: "external", DesiredStatus: "completed", DesiredConclusion: "success",
		DesiredTitle: "title", DesiredSummary: "summary", DesiredDigest: "digest",
		ChangedAt: time.Now().UTC(),
	}
	tests := []struct {
		name   string
		action CheckAction
	}{
		{name: "label", action: CheckAction{
			Label: strings.Repeat("l", 21), Description: "description", Identifier: "id",
		}},
		{name: "description", action: CheckAction{
			Label: "label", Description: strings.Repeat("d", 41), Identifier: "id",
		}},
		{name: "identifier", action: CheckAction{
			Label: "label", Description: "description", Identifier: strings.Repeat("i", 21),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := request
			candidate.DesiredActions = []CheckAction{test.action}
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected over-limit check action to be rejected")
			}
		})
	}
}
