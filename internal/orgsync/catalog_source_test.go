package orgsync_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
)

type contentsOnly struct{}

func (contentsOnly) Grants(permission string) bool { return permission == "contents" }

func TestCatalogPathsRequireWorkflowPermission(t *testing.T) {
	config := orgsync.FileConfig{Catalog: &orgsync.CatalogSource{
		Owner: "smykla-skalski", Repo: ".github",
		Commit: "0123456789abcdef0123456789abcdef01234567",
		Path:   "sync/catalog.json", Profiles: []string{"opencode-plugin"},
		Paths: []string{".github/workflows/ci.yml", "mise.toml"},
	}}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(config.Permissions(), "workflows") {
		t.Fatal("catalog workflow path did not require Workflows permission")
	}
	document, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	unavailable, missing := orgsync.UnpermittedConfig(contentsOnly{}, orgsync.Config{
		Kind: orgsync.KindFiles, Document: document,
	})
	if !missing || unavailable.Permission != "workflows" {
		t.Fatalf("unavailable = %#v, missing = %t", unavailable, missing)
	}
}

func TestCatalogProfilesSelectRepositoryFiles(t *testing.T) {
	config := orgsync.FileConfig{
		Files: []orgsync.File{
			{Path: "inline.md"},
			{Path: "base.md", Profile: "base"},
			{Path: "ts.json", Profile: "typescript"},
		},
		CatalogProfiles: []string{"base", "typescript"},
		DefaultProfiles: []string{"base"},
	}
	if got := config.SelectProfiles(nil).Paths(); !slices.Equal(got, []string{"inline.md", "base.md"}) {
		t.Fatalf("default paths = %v", got)
	}
	selected := []string{"base", "typescript"}
	if got := config.SelectProfiles(&selected).Paths(); !slices.Equal(got, []string{"inline.md", "base.md", "ts.json"}) {
		t.Fatalf("selected paths = %v", got)
	}
	empty := []string{}
	if got := config.SelectProfiles(&empty).Paths(); !slices.Equal(got, []string{"inline.md"}) {
		t.Fatalf("empty selection paths = %v", got)
	}
	if err := (orgsync.FileOverride{Profiles: &[]string{"unknown"}}).Validate(config); err == nil {
		t.Fatal("unknown repository profile was accepted")
	}
}
