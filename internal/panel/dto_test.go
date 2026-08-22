package panel

import (
	"reflect"
	"testing"
	"time"

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

	targetResponse := targetDTO(testRuntimeValues(), target, testOwnerAccess())
	if targetResponse.InheritedConfig.CommandPrefix != config.DefaultCommandPrefix {
		t.Fatalf("target inherited prefix = %q", targetResponse.InheritedConfig.CommandPrefix)
	}
	if targetResponse.EffectiveConfig.CommandPrefix != targetPrefix {
		t.Fatalf("target effective prefix = %q", targetResponse.EffectiveConfig.CommandPrefix)
	}

	repositoryResponse := repositoryDetailDTO(testRuntimeValues(), target, repository)
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

func TestRuntimeServiceDTOExposesAVisiblePanelPath(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	for _, test := range []struct {
		name, basePath, want string
	}{
		{name: "root", want: "/"},
		{name: "nested", basePath: "/panel", want: "/panel"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			response := runtimeServiceDTO(
				storage.DatabaseStatus{}, Config{BasePath: test.basePath}, now, now,
			)
			if response.PublicPaths.Panel != test.want {
				t.Fatalf("panel path = %q, want %q", response.PublicPaths.Panel, test.want)
			}
		})
	}
}

// TestDurationDTOsResolveWhatALevelInherits covers the two settings that
// cascade as numbers rather than through `config.Resolve`.
//
// They used to be sent as "whatever the level above overrode, or null", so a
// panel under an installation that set nothing had nothing to prefill with and
// stood in an hour - on every deployment, whatever it was running. The DTO now
// answers what would actually happen.
func TestDurationDTOsResolveWhatALevelInherits(t *testing.T) {
	runtime := testRuntimeValues()
	target := storage.Target{}

	response := targetDTO(runtime, target, testOwnerAccess())
	if response.PathIndexIntervalSecondsInherited != 17*60 {
		t.Fatalf(
			"installation inherits %d seconds, want the process's 1020",
			response.PathIndexIntervalSecondsInherited,
		)
	}
	if response.PendingCIQuietPeriodSecondsInherited != 45 {
		t.Fatalf(
			"installation inherits a %d second quiet period, want the process's 45",
			response.PendingCIQuietPeriodSecondsInherited,
		)
	}

	// And a repository reads through the installation where that set one.
	installationInterval := 5 * time.Minute
	target.PathIndexIntervalOverride = &installationInterval
	detail := repositoryDetailDTO(runtime, target, storage.Repository{})
	if detail.PathIndexIntervalSecondsInherited != 300 {
		t.Fatalf(
			"repository inherits %d seconds, want the installation's 300",
			detail.PathIndexIntervalSecondsInherited,
		)
	}
	// The quiet period is untouched on this installation, so it still reads
	// through to the process rather than stopping at the nil above it.
	if detail.PendingCIQuietPeriodSecondsInherited != 45 {
		t.Fatalf(
			"repository inherits a %d second quiet period, want the process's 45",
			detail.PendingCIQuietPeriodSecondsInherited,
		)
	}
}

func TestConfigurationDTOsExposeEmptyAllowedCommandsAsArray(t *testing.T) {
	emptyCommands := []string{}
	response := targetDTO(testRuntimeValues(), storage.Target{
		ConfigPatch: config.Patch{AllowedCommands: &emptyCommands},
	}, testOwnerAccess())

	if response.InheritedConfig.AllowedCommands == nil {
		t.Fatal("target inherited commands are nil; the panel requires an empty array")
	}
	if response.EffectiveConfig.AllowedCommands == nil {
		t.Fatal("target effective commands are nil; the panel requires an empty array")
	}
}

// testRuntimeValues is what a running service would have resolved: the default
// bot config, and one distinguishable number per cascading duration so a DTO
// that carried the wrong one is visible rather than plausible.
func testRuntimeValues() RuntimeValues {
	return RuntimeValues{
		BotConfig:            config.Default(),
		PendingCIQuietPeriod: 45 * time.Second,
		PathIndexInterval:    17 * time.Minute,
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
	response := repositoryDetailDTO(testRuntimeValues(), storage.Target{}, storage.Repository{
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
