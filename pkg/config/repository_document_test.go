package config_test

import (
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestRepositoryDocumentPreservesPanelWithoutChangingProcessContract(t *testing.T) {
	const source = "command_prefix='/repo '\nrunner='action'\n[panel]\nversion=1\nscope='repository'\n[panel.settings]\nenabled=false\n"
	document, err := config.ParseRepositoryFile(config.FormatTOML, []byte(source))
	if err != nil || document.Panel == nil || document.Panel.Settings.Enabled == nil || *document.Panel.Settings.Enabled {
		t.Fatalf("lost panel settings: %+v, %v", document, err)
	}
	if document.Runner == nil || *document.Runner != config.RunnerAction {
		t.Fatalf("lost file-owned runner: %+v", document.Patch)
	}
	values, err := config.LoadRepoConfig(config.Default(), config.FormatTOML, []byte(source))
	if err != nil || values.CommandPrefix != "/repo " || values.Runner != config.RunnerAction {
		t.Fatalf("command entry point lost root settings: %+v, %v", values, err)
	}
	if _, err := config.ParsePatch(config.FormatTOML, []byte(source)); err == nil {
		t.Fatal("process configuration silently accepted panel-only fields")
	}
}

func TestRepositoryDocumentRejectsWorkspaceScope(t *testing.T) {
	const source = "[panel]\nversion=1\nscope='workspace'\n[panel.settings]\nrepository_default_enabled=true\nmerge_mode='checks'\nprotected_refs={include=['~DEFAULT_BRANCH'],exclude=[]}\n"
	if _, err := config.ParseFileDocument(config.FormatTOML, []byte(source)); err != nil {
		t.Fatalf("invalid workspace fixture: %v", err)
	}
	if _, err := config.ParseRepositoryFile(config.FormatTOML, []byte(source)); err == nil {
		t.Fatal("repository reader accepted workspace scope")
	}
	if _, err := config.LoadRepoConfig(config.Default(), config.FormatTOML, []byte(source)); err == nil {
		t.Fatal("standalone entry point accepted workspace scope")
	}
}
