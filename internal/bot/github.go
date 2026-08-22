package bot

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

func CleanupGitHubError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var apiErr *github.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return nil
	}

	return fmt.Errorf("%s: %w", operation, err)
}

func HasLabel(labels []string, wanted string) bool {
	return slices.Contains(labels, wanted)
}
