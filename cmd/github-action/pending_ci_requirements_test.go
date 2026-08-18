package main

import (
	"context"
	"errors"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

type pendingCIRequirementPolicyStub struct {
	requirements github.RequiredCIRequirements
}

func (stub pendingCIRequirementPolicyStub) GetRequiredStatusChecks(
	context.Context,
	string,
	string,
	string,
) ([]github.RequiredCheck, error) {
	return stub.requirements.StatusChecks, nil
}

func (stub pendingCIRequirementPolicyStub) GetRequiredCIRequirements(
	context.Context,
	string,
	string,
	string,
) (github.RequiredCIRequirements, error) {
	return stub.requirements, nil
}

func TestPendingCIRequiredOnlyRejectsRequiredWorkflows(t *testing.T) {
	t.Parallel()
	reader := pendingCIRequirementPolicyStub{requirements: github.RequiredCIRequirements{
		StatusChecks: []github.RequiredCheck{{Context: "build"}}, RequiredWorkflow: true,
	}}

	_, err := pendingCIRequiredChecks(
		t.Context(), reader, "owner", "repo", "main", true,
	)
	if !errors.Is(err, errRequiredWorkflowsUnsupported) {
		t.Fatalf("required-only policy error = %v", err)
	}
}

func TestPendingCIAllChecksAllowsRequiredWorkflows(t *testing.T) {
	t.Parallel()
	reader := pendingCIRequirementPolicyStub{requirements: github.RequiredCIRequirements{
		RequiredWorkflow: true,
	}}

	required, err := pendingCIRequiredChecks(
		t.Context(), reader, "owner", "repo", "main", false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if required != nil {
		t.Fatalf("ordinary all-check wait returned filter %#v", required)
	}
}
