package bot

import (
	"errors"
	"testing"
)

func TestOpErrorMatchesItsOperationAndItsCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("connection reset")

	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		// Given a failure wrapped with the operation it came from
		{"its own operation", NewGitHubError(errMergePR, cause), errMergePR, true},
		{"the cause underneath", NewGitHubError(errMergePR, cause), cause, true},
		{"a different operation", NewGitHubError(errMergePR, cause), errApprovePR, false},

		// Then two failures of the same kind are told apart by their operation,
		// which sharing one struct must not cost. This is the row the merge
		// changed: the deleted branch answered true for any same-typed target.
		{
			"another failure of the same kind",
			NewGitHubError(errMergePR, cause), NewGitHubError(errApprovePR, cause), false,
		},

		{"config keeps its own operation", NewConfigError(ErrConfigLoad, cause), ErrConfigLoad, true},
		{"config is not a GitHub failure", NewConfigError(ErrConfigLoad, cause), errMergePR, false},
		{"GitHub is not a config failure", NewGitHubError(errMergePR, cause), ErrConfigLoad, false},

		// And an operation with no cause still names itself
		{"no cause", NewGitHubError(errMergePR, nil), errMergePR, true},
	}

	for _, test := range tests {
		if got := errors.Is(test.err, test.target); got != test.want {
			t.Errorf("%s: errors.Is = %t, want %t (%v)", test.name, got, test.want, test.err)
		}
	}
}
