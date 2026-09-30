package orgsync

import (
	"regexp"
	"slices"
	"strings"
)

var (
	catalogCommit = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	catalogName   = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	profileName   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// CatalogSource identifies a versioned catalog and the paths it may write.
type CatalogSource struct {
	Owner           string   `json:"owner"`
	Repo            string   `json:"repo"`
	Commit          string   `json:"commit"`
	Path            string   `json:"path"`
	Profiles        []string `json:"profiles"`
	DefaultProfiles []string `json:"default_profiles,omitempty"`
	Paths           []string `json:"paths"`
}

func (source CatalogSource) Validate() error {
	if !catalogName.MatchString(source.Owner) || !catalogName.MatchString(source.Repo) ||
		strings.Trim(source.Owner, ".") == "" || strings.Trim(source.Repo, ".") == "" {
		return invalid("catalog owner and repository must be plain names")
	}
	if !catalogCommit.MatchString(source.Commit) {
		return invalid("catalog commit must be a full SHA")
	}
	if err := validateFilePath("catalog path", 0, source.Path); err != nil {
		return err
	}
	if len(source.Profiles) == 0 || len(source.Paths) == 0 {
		return invalid("catalog needs profiles and expected paths")
	}
	for index, name := range source.Profiles {
		if !profileName.MatchString(name) || slices.Contains(source.Profiles[:index], name) {
			return invalid("catalog profile %q is invalid or selected twice", name)
		}
	}
	for index, name := range source.DefaultProfiles {
		if !slices.Contains(source.Profiles, name) || slices.Contains(source.DefaultProfiles[:index], name) {
			return invalid("catalog default profile %q is unavailable or selected twice", name)
		}
	}
	return nil
}
