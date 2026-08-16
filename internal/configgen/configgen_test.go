package configgen_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/smykla-skalski/smyklot/internal/configgen"
	"github.com/smykla-skalski/smyklot/pkg/config"
)

// repoRoot is where the tests read the real config package from. The tests run
// with the package directory as their working directory.
const repoRoot = "../.."

func parse(t *testing.T) configgen.Model {
	t.Helper()

	model, err := configgen.Parse(filepath.Join(repoRoot, configgen.PackageDir))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(model.Fields) == 0 {
		t.Fatal("Parse() found no fields, so every assertion below would pass vacuously")
	}

	return model
}

// A stale generated file is the one failure mode codegen adds that hand-written
// code does not have, so it is checked first and by comparing bytes rather than
// behaviour.
func TestGeneratedFileIsCurrent(t *testing.T) {
	rendered, err := configgen.RenderGo(parse(t))
	if err != nil {
		t.Fatalf("RenderGo() error = %v", err)
	}

	path := filepath.Join(repoRoot, configgen.GoFile)

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if string(rendered) != string(onDisk) {
		t.Errorf("%s is stale - run `mise run generate`", configgen.GoFile)
	}
}

// Rendering twice must produce the same bytes. Map iteration order is the usual
// way a generator stops being a pure function of its input, and it fails
// intermittently rather than immediately, so it is worth asserting directly.
func TestRenderIsDeterministic(t *testing.T) {
	model := parse(t)

	first, err := configgen.RenderGo(model)
	if err != nil {
		t.Fatalf("RenderGo() error = %v", err)
	}

	for range 8 {
		again, err := configgen.RenderGo(model)
		if err != nil {
			t.Fatalf("RenderGo() error = %v", err)
		}

		if string(again) != string(first) {
			t.Fatal("RenderGo() is not deterministic")
		}
	}
}

// Parsing twice must also agree: Parse ranges over a map of files, and a second
// Patch declaration appearing somewhere would make the winner arbitrary.
func TestParseIsDeterministic(t *testing.T) {
	first := parse(t)

	for range 8 {
		again := parse(t)
		if !reflect.DeepEqual(again, first) {
			t.Fatal("Parse() is not deterministic")
		}
	}
}

// The completeness test. It walks Patch by reflection rather than reading the
// model, so a field the parser silently dropped fails here: the two would have
// to be broken the same way to agree.
func TestEveryPatchFieldIsGenerated(t *testing.T) {
	model := parse(t)

	generated := make(map[string]configgen.Field, len(model.Fields))
	for _, field := range model.Fields {
		generated[field.GoName] = field
	}

	patch := reflect.TypeOf(config.Patch{})
	effective := reflect.TypeOf(config.Config{})

	if patch.NumField() != len(model.Fields) {
		t.Fatalf("Patch has %d fields, the model has %d", patch.NumField(), len(model.Fields))
	}

	for index := range patch.NumField() {
		field := patch.Field(index)

		described, ok := generated[field.Name]
		if !ok {
			t.Errorf("Patch.%s is missing from the generated model", field.Name)

			continue
		}

		if described.Description == "" {
			t.Errorf("Patch.%s has no description for the schema", field.Name)
		}

		// Config must carry the same setting, under the same key, with the
		// pointer removed. A type that drifted would decode a file into a value
		// nothing reads.
		value, ok := effective.FieldByName(field.Name)
		if !ok {
			t.Errorf("Config.%s is missing", field.Name)

			continue
		}

		if want := field.Type.Elem(); value.Type != want {
			t.Errorf("Config.%s is %s, want %s", field.Name, value.Type, want)
		}

		if got := jsonKey(value.Tag.Get("json")); got != described.Key {
			t.Errorf("Config.%s has key %q, Patch has %q", field.Name, got, described.Key)
		}
	}
}

// The reverse direction: a key in the generated code with no field behind it.
// Without this, deleting a field from Patch and leaving the generated file
// stale would go unnoticed by the test above.
func TestNoGeneratedKeyIsOrphaned(t *testing.T) {
	patch := reflect.TypeOf(config.Patch{})

	declared := make(map[string]struct{}, patch.NumField())
	for index := range patch.NumField() {
		declared[jsonKey(patch.Field(index).Tag.Get("json"))] = struct{}{}
	}

	for _, key := range config.Keys() {
		if _, ok := declared[key]; !ok {
			t.Errorf("config.Keys() reports %q, which no Patch field declares", key)
		}
	}

	if len(config.Keys()) != patch.NumField() {
		t.Errorf("config.Keys() has %d entries, Patch has %d fields",
			len(config.Keys()), patch.NumField())
	}
}

// Defaults are the half of the schema most likely to become decorative: the
// document says one thing and the running code does another, and nothing
// compares them. This is that comparison.
func TestDefaultsMatchTheModel(t *testing.T) {
	model := parse(t)
	defaults := reflect.ValueOf(*config.Default())

	for _, field := range model.Fields {
		value := defaults.FieldByName(field.GoName)

		switch field.Kind {
		case configgen.KindBool:
			want := field.Default == "true"
			if value.Bool() != want {
				t.Errorf("Default().%s = %v, tag says %v", field.GoName, value.Bool(), want)
			}

		case configgen.KindString, configgen.KindEnum:
			if value.String() != field.Default {
				t.Errorf("Default().%s = %q, tag says %q",
					field.GoName, value.String(), field.Default)
			}

		case configgen.KindStringSlice, configgen.KindStringMap:
			// A list or a mapping defaults to empty, and to a non-nil empty:
			// resolution copies into it, and a nil map would panic.
			if value.IsNil() {
				t.Errorf("Default().%s is nil, want an empty value", field.GoName)
			}

			if value.Len() != 0 {
				t.Errorf("Default().%s has %d entries, want none", field.GoName, value.Len())
			}
		}
	}
}

// The two exported constants that name a default in prose. They are part of the
// package's surface, so they cannot simply be generated away, but they can be
// held to what Default() actually returns.
func TestExportedDefaultConstantsAgree(t *testing.T) {
	defaults := config.Default()

	if defaults.CommandPrefix != config.DefaultCommandPrefix {
		t.Errorf("Default().CommandPrefix = %q, DefaultCommandPrefix = %q",
			defaults.CommandPrefix, config.DefaultCommandPrefix)
	}

	if defaults.Runner != config.DefaultRunner {
		t.Errorf("Default().Runner = %q, DefaultRunner = %q", defaults.Runner, config.DefaultRunner)
	}
}

// An enum's declared values have to be values the parser accepts, or the schema
// publishes a choice the code refuses.
func TestEnumValuesAreAccepted(t *testing.T) {
	for _, field := range parse(t).Fields {
		if field.Kind != configgen.KindEnum || field.GoName != "Runner" {
			continue
		}

		if len(field.Enum) == 0 {
			t.Fatal("Runner is an enum with no values")
		}

		for _, value := range field.Enum {
			if _, err := config.ParseRunner(value); err != nil {
				t.Errorf("ParseRunner(%q) error = %v, but the enum tag offers it", value, err)
			}
		}

		if !contains(field.Enum, field.Default) {
			t.Errorf("Runner defaults to %q, which its enum does not list", field.Default)
		}
	}
}

// The panel rule moved onto the field it governs. This is what stops it drifting
// back apart: the panel reads the tag rather than keeping its own list.
func TestPanelDeniedKeysComeFromTheTags(t *testing.T) {
	var want []string

	for _, field := range parse(t).Fields {
		if field.PanelDeny {
			want = append(want, field.Key)
		}
	}

	if got := config.PanelDeniedKeys(); !reflect.DeepEqual(got, want) {
		t.Errorf("PanelDeniedKeys() = %v, tags say %v", got, want)
	}

	if len(want) == 0 {
		t.Error("no field carries panel:\"deny\", so this test proves nothing")
	}
}

// Default() must hand out a fresh value each time. Sharing one would let a
// repository's resolution write into the defaults every other repository reads,
// and the bug would look like unrelated settings changing on their own.
func TestDefaultIsNotShared(t *testing.T) {
	first := config.Default()
	second := config.Default()

	first.AllowedCommands = append(first.AllowedCommands, "approve")
	first.CommandAliases["a"] = "approve"

	if len(second.AllowedCommands) != 0 {
		t.Errorf("Default().AllowedCommands leaked: %v", second.AllowedCommands)
	}

	if len(second.CommandAliases) != 0 {
		t.Errorf("Default().CommandAliases leaked: %v", second.CommandAliases)
	}
}

func TestGeneratedFileCarriesTheMarker(t *testing.T) {
	path := filepath.Join(repoRoot, configgen.GoFile)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	// The marker has to be on the first line for the tooling to see it.
	first, _, _ := strings.Cut(string(content), "\n")
	if !strings.Contains(first, configgen.GoDoc()) {
		t.Errorf("first line of %s is %q, want the generated marker", configgen.GoFile, first)
	}
}

func jsonKey(tag string) string {
	key, _, _ := strings.Cut(tag, ",")

	return key
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}

// The generator's refusals. Each fixture is a Patch that would otherwise render
// into Go that does not compile, or - worse - into Go that does and is wrong.
// Without these, the only test of an error path is the day somebody trips it.
func TestParseRefusesABadPatch(t *testing.T) {
	cases := map[string]error{
		"notpointer":  configgen.ErrFieldNotSparse,
		"badtype":     configgen.ErrUnsupportedType,
		"nodoc":       configgen.ErrMissingDoc,
		"nokey":       configgen.ErrMissingKey,
		"baddefault":  configgen.ErrInvalidDefault,
		"badenum":     configgen.ErrInvalidDefault,
		"listdefault": configgen.ErrInvalidDefault,
		"dupkey":      configgen.ErrDuplicateKey,
		"nopatch":     configgen.ErrPatchNotFound,
	}

	for fixture, want := range cases {
		t.Run(fixture, func(t *testing.T) {
			_, err := configgen.Parse(filepath.Join("testdata", fixture))
			if !errors.Is(err, want) {
				t.Errorf("Parse(%s) error = %v, want %v", fixture, err, want)
			}
		})
	}
}

// A refusal has to name the field, or the report sends the reader to a
// generated file rather than to the tag that caused it.
func TestRefusalNamesTheField(t *testing.T) {
	_, err := configgen.Parse(filepath.Join("testdata", "baddefault"))
	if err == nil {
		t.Fatal("Parse(baddefault) succeeded")
	}

	if !strings.Contains(err.Error(), "QuietSuccess") {
		t.Errorf("error %q does not name the field", err)
	}

	if !strings.Contains(err.Error(), "maybe") {
		t.Errorf("error %q does not name the offending value", err)
	}
}

// The command-line flags were the last hand-written enumeration, and they had
// fallen behind: quiet_pending had no flag at all and nothing said so. They are
// generated now, and this is what holds them to the tags.
func TestEverySettingWithAFlagHasOne(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.RegisterFlags(flags)

	registered := make(map[string]*pflag.Flag)
	flags.VisitAll(func(flag *pflag.Flag) { registered[flag.Name] = flag })

	// --config-file names a document of settings rather than being one, so it
	// is the one flag with no field behind it. Taking it out here is what lets
	// the count below still catch a flag nothing asked for
	if _, ok := registered[config.FlagConfigFile]; !ok {
		t.Errorf("--%s is not registered, so a process cannot be given a file of settings",
			config.FlagConfigFile)
	}

	delete(registered, config.FlagConfigFile)

	var wanted int

	for _, field := range parse(t).Fields {
		flag, ok := registered[field.Key]

		if !field.HasFlag {
			if ok {
				t.Errorf("%q carries flag:\"-\" but a flag was registered for it", field.Key)
			}

			continue
		}

		wanted++

		if !ok {
			t.Errorf("%q has no command-line flag", field.Key)

			continue
		}

		if flag.Usage != field.Description {
			t.Errorf("flag %q describes itself as %q, the setting says %q",
				field.Key, flag.Usage, field.Description)
		}
	}

	if len(registered) != wanted {
		t.Errorf("%d flags registered, %d settings take one", len(registered), wanted)
	}

	if wanted == 0 {
		t.Error("no setting takes a flag, so this test proves nothing")
	}
}

// The runner is the one setting deliberately without a flag: it says which
// entry point acts on a repository, and only the repository may answer that. A
// flag would let one deployment decide it for every repository it serves.
func TestTheRunnerTakesNoFlag(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.RegisterFlags(flags)

	if flag := flags.Lookup(config.KeyRunner); flag != nil {
		t.Error("the runner has a command-line flag, which would let a deployment set it")
	}
}

// A flag whose default differs from Default() reports one thing in --help and
// resolves to another.
func TestFlagDefaultsAreTheRealDefaults(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.RegisterFlags(flags)

	defaults := reflect.ValueOf(*config.Default())

	for _, field := range parse(t).Fields {
		flag := flags.Lookup(field.Key)
		if flag == nil {
			continue
		}

		value := defaults.FieldByName(field.GoName)

		switch field.Kind {
		case configgen.KindBool:
			if flag.DefValue != strconv.FormatBool(value.Bool()) {
				t.Errorf("flag %q defaults to %q, Default() says %v",
					field.Key, flag.DefValue, value.Bool())
			}

		case configgen.KindString, configgen.KindEnum:
			if flag.DefValue != value.String() {
				t.Errorf("flag %q defaults to %q, Default() says %q",
					field.Key, flag.DefValue, value.String())
			}
		}
	}
}
