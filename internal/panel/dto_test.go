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

// patchSize used to enumerate its fields and had fallen one behind, so a
// repository that overrode only its runner was reported as overriding nothing.
// The count now comes from the patch itself.
func TestPatchSizeCountsEverySetting(t *testing.T) {
	runner := config.RunnerAction
	prefix := "!"

	cases := map[string]struct {
		patch config.Patch
		want  int
	}{
		"nothing set":    {want: 0},
		"one setting":    {patch: config.Patch{CommandPrefix: &prefix}, want: 1},
		"runner alone":   {patch: config.Patch{Runner: &runner}, want: 1},
		"runner and one": {patch: config.Patch{Runner: &runner, CommandPrefix: &prefix}, want: 2},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			if got := patchSize(test.patch); got != test.want {
				t.Errorf("patchSize() = %d, want %d", got, test.want)
			}
		})
	}
}

// Every key the patch can carry has to be countable, or patchSize is back to
// enumerating a subset - just generated rather than hand-written.
func TestPatchSizeCountsAFullPatch(t *testing.T) {
	if got, want := patchSize(fullPatch(t)), len(config.Keys()); got != want {
		t.Errorf("patchSize(full patch) = %d, want %d", got, want)
	}
}

// The detail pane used to print ".github/smyklot.yaml" as a literal, which was
// true while that was the only place a configuration file could be. It is now
// one of five, so the pane has to be told which one won and which were passed
// over.
func TestRepositoryDetailNamesTheFileItRead(t *testing.T) {
	response := repositoryDetailDTO(config.Default(), storage.Target{}, storage.Repository{
		ConfigFileStatus:     storage.RepositoryFileValid,
		ConfigFilePath:       ".smyklot.toml",
		ConfigFileSuperseded: []string{".github/smyklot.yaml"},
	})

	if response.ConfigFilePath != ".smyklot.toml" {
		t.Errorf("detail names %q as the file it read", response.ConfigFilePath)
	}
	if !reflect.DeepEqual(response.ConfigFileSuperseded, []string{".github/smyklot.yaml"}) {
		t.Errorf("detail reports %#v as passed over", response.ConfigFileSuperseded)
	}
}
