package main

import (
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

func TestRepositoryObservationRetainsPanelDocument(t *testing.T) {
	file := repositoryConfigFileFrom(foundRepoConfig{
		Path:       ".smyklot.toml",
		Content:    []byte("quiet_success=true\n[panel]\nversion=1\nscope='repository'\n[panel.settings]\nenabled=false\n"),
		Superseded: []string{".github/smyklot.yaml"},
	}, "roots")
	if file.status != storage.RepositoryFileValid || file.err != nil || file.panel == nil ||
		file.panel.Settings.Enabled == nil || *file.panel.Settings.Enabled ||
		file.patch.QuietSuccess == nil || !*file.patch.QuietSuccess {
		t.Fatalf("lost observed settings: %+v", file)
	}
	if file.path != ".smyklot.toml" || file.fingerprint != "roots" || len(file.superseded) != 1 {
		t.Fatalf("lost observation metadata: %+v", file)
	}
}

func TestRepositoryObservationRejectsWrongPanelScope(t *testing.T) {
	file := repositoryConfigFileFrom(foundRepoConfig{
		Path:    ".smyklot.toml",
		Content: []byte("[panel]\nversion=1\nscope='workspace'\n[panel.settings]\nrepository_default_enabled=true\nmerge_mode='checks'\nprotected_refs={include=['~DEFAULT_BRANCH'],exclude=[]}\n"),
	}, "roots")
	if file.status != storage.RepositoryFileInvalid || file.err == nil || file.panel != nil ||
		file.path != ".smyklot.toml" || file.fingerprint != "roots" {
		t.Fatalf("wrong-scope file was not surfaced as invalid: %+v", file)
	}
}
