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

	targetResponse := targetDTO(config.Default(), target, testOwnerAccess())
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

func TestConfigurationDTOsExposeEmptyAllowedCommandsAsArray(t *testing.T) {
	emptyCommands := []string{}
	response := targetDTO(config.Default(), storage.Target{
		ConfigPatch: config.Patch{AllowedCommands: &emptyCommands},
	}, testOwnerAccess())

	if response.InheritedConfig.AllowedCommands == nil {
		t.Fatal("target inherited commands are nil; the panel requires an empty array")
	}
	if response.EffectiveConfig.AllowedCommands == nil {
		t.Fatal("target effective commands are nil; the panel requires an empty array")
	}
}

func testOwnerAccess() storage.TargetAccess {
	return storage.TargetAccess{
		Role:         storage.InstallationRoleOwner,
		Source:       storage.AccessSourceOwner,
		Root:         true,
		Capabilities: storage.EffectiveCapabilities(storage.InstallationRoleOwner),
	}
}
