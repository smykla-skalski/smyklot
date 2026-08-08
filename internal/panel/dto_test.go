package panel

import (
	"reflect"
	"testing"

	"github.com/smykla-skalski/smyklot/internal/storage"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestConfigurationDTOsExposeInheritedValues(t *testing.T) {
	targetPrefix := "!"
	repositoryPrefix := "#"
	fileCommands := []string{"merge"}
	repositoryCommands := []string{"approve"}

	target := storage.Target{
		ConfigPatch: config.Patch{CommandPrefix: &targetPrefix},
	}
	repository := storage.Repository{
		ConfigFilePatch: config.Patch{AllowedCommands: &fileCommands},
		ConfigPatch: config.Patch{
			CommandPrefix:   &repositoryPrefix,
			AllowedCommands: &repositoryCommands,
		},
	}

	targetResponse := targetDTO(config.Default(), target)
	if targetResponse.InheritedConfig.CommandPrefix != config.DefaultCommandPrefix {
		t.Fatalf("target inherited prefix = %q", targetResponse.InheritedConfig.CommandPrefix)
	}
	if targetResponse.EffectiveConfig.CommandPrefix != targetPrefix {
		t.Fatalf("target effective prefix = %q", targetResponse.EffectiveConfig.CommandPrefix)
	}

	repositoryResponse := repositoryDetailDTO(config.Default(), target, repository)
	if repositoryResponse.InheritedConfig.CommandPrefix != targetPrefix {
		t.Fatalf(
			"repository inherited prefix = %q",
			repositoryResponse.InheritedConfig.CommandPrefix,
		)
	}
	if !reflect.DeepEqual(repositoryResponse.InheritedConfig.AllowedCommands, fileCommands) {
		t.Fatalf(
			"repository inherited commands = %#v",
			repositoryResponse.InheritedConfig.AllowedCommands,
		)
	}
	if repositoryResponse.EffectiveConfig.CommandPrefix != repositoryPrefix {
		t.Fatalf(
			"repository effective prefix = %q",
			repositoryResponse.EffectiveConfig.CommandPrefix,
		)
	}
	if !reflect.DeepEqual(repositoryResponse.EffectiveConfig.AllowedCommands, repositoryCommands) {
		t.Fatalf(
			"repository effective commands = %#v",
			repositoryResponse.EffectiveConfig.AllowedCommands,
		)
	}
}
