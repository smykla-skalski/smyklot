package bot

import "os"

const EnvStepSummary = "GITHUB_STEP_SUMMARY"

// appendStepSummary adds one note to the GitHub Actions step summary.
//
// This is the only place the summary file is opened. Outside Actions there is
// no such file, and nothing is written.
func appendStepSummary(note string) error {
	summaryFile := os.Getenv(EnvStepSummary)
	if summaryFile == "" {
		// Not running in GitHub Actions, skip
		return nil
	}

	//nolint:gosec // summaryFile is from the trusted GitHub Actions environment
	file, err := os.OpenFile(summaryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return NewGitHubError(errStepSummary, err)
	}
	defer func() {
		_ = file.Close()
	}()

	if _, err := file.WriteString(note); err != nil {
		return NewGitHubError(errStepSummary, err)
	}

	return nil
}
