package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/pflag"
)

const (
	// EnvConfigFile names a file of settings for the process to read.
	EnvConfigFile = "SMYKLOT_CONFIG_FILE"

	// FlagConfigFile is the command-line spelling of EnvConfigFile.
	//
	// It is not --config, which would sit one character away from
	// SMYKLOT_CONFIG and mean something else: that one carries the settings
	// themselves, this one names a file holding them.
	FlagConfigFile = "config-file"
)

// PrecedenceLayer is one place a setting can be given a value.
type PrecedenceLayer struct {
	// Name is what the layer is called.
	Name string

	// Where is where somebody writes it.
	Where string
}

// PrecedenceLayers returns every layer, lowest precedence first.
//
// A later layer replaces a setting an earlier one made; a setting no layer
// names keeps its default. The first five are the process, and collapse into
// one Config carrying SourceProcess - which is why the panel shows a setting
// the process supplied as coming from the process rather than from whichever
// of the five actually said it.
//
// The environment document sits below the individual variables on purpose.
// Both are the environment, and a deployment that wants to change one setting
// should be able to add one variable rather than rewrite the whole document.
// It used to be the other way round, and worse: the document was loaded
// through viper's override layer, so it beat the command line too.
func PrecedenceLayers() []PrecedenceLayer {
	return []PrecedenceLayer{
		{Name: "defaults", Where: "built into Smyklot"},
		{Name: "process file", Where: "--" + FlagConfigFile + ", or " + EnvConfigFile},
		{Name: "process document", Where: EnvConfig},
		{Name: "process environment", Where: EnvPrefix + "_* variables, one per setting"},
		{Name: "process flags", Where: "the command line"},
		{Name: "account settings", Where: "the panel, for every repository"},
		{Name: "repository file", Where: ".smyklot.toml in the repository"},
		{Name: "repository settings", Where: "the panel, for one repository"},
	}
}

// PrecedenceDoc renders the layers as the block written into README.md and
// CLAUDE.md, which a test then holds to this output.
//
// Five documents disagreed about this and the code contradicted all of them.
// Prose copied by hand is prose that goes stale, so there is one copy and the
// rest are generated from it.
func PrecedenceDoc() string {
	layers := PrecedenceLayers()

	widest := 0
	for _, layer := range layers {
		widest = max(widest, len(layer.Name))
	}

	var doc strings.Builder

	for index, layer := range layers {
		fmt.Fprintf(&doc, "%d. %-*s  %s\n", index+1, widest, layer.Name, layer.Where)
	}

	return doc.String()
}

// EnvVar names the environment variable a setting is read from.
//
// One rule rather than a constant per setting: a list of thirteen names is a
// list that comes to disagree with the keys beside it, which is the failure
// this whole package is being reshaped to make impossible.
func EnvVar(key string) string {
	return EnvPrefix + "_" + strings.ToUpper(key)
}

// LoadProcess resolves the configuration a process starts with, from every
// layer below the account and the repository.
//
// flags may be nil, and may be a set that registered none of the settings -
// an entry point that takes fewer flags reads fewer layers rather than a
// different ladder.
func LoadProcess(flags *pflag.FlagSet) (*Config, error) {
	path, err := configFilePath(flags)
	if err != nil {
		return nil, err
	}

	values := Default()

	// The order is PrecedenceLayers, minus the defaults this starts from.
	// Stating it as a list rather than as four calls is what lets a reader
	// check it against the documented ladder without following the control
	// flow.
	for _, read := range []func() (Patch, error){
		func() (Patch, error) { return filePatch(path) },
		documentPatch,
		func() (Patch, error) { return envPatch(os.LookupEnv) },
		func() (Patch, error) { return flagPatch(flags) },
	} {
		patch, err := read()
		if err != nil {
			return nil, err
		}

		// Normalising every layer here rather than inside each reader is what
		// makes "runner must name an entry point" one rule. ParsePatch
		// normalises too, for the repository file, and doing it twice is
		// harmless - a value the parser vouches for parses to itself.
		if err := patch.normalize(); err != nil {
			return nil, err
		}

		applyPatch(values, patch, nil, "")
	}

	return values, nil
}

// registerConfigFileFlag defines --config-file. The generated RegisterFlags
// calls it, so a command that takes the settings takes the file naming them.
func registerConfigFileFlag(flags *pflag.FlagSet) {
	flags.String(FlagConfigFile, "", "Path to a TOML file of settings")
}

// configFilePath reports the file to read, from the flag or the environment.
func configFilePath(flags *pflag.FlagSet) (string, error) {
	if flags == nil || !flags.Changed(FlagConfigFile) {
		return os.Getenv(EnvConfigFile), nil
	}

	path, err := flags.GetString(FlagConfigFile)
	if err != nil {
		return "", fmt.Errorf("read --%s: %w", FlagConfigFile, err)
	}

	return path, nil
}

// filePatch reads the layer held in a file.
//
// A file that was named and is not there is an error rather than an empty
// layer: somebody meant to configure the process from it, and starting with
// defaults instead would look like it had worked.
func filePatch(path string) (Patch, error) {
	if path == "" {
		return Patch{}, nil
	}

	format, err := FormatOf(path)
	if err != nil {
		return Patch{}, err
	}

	content, err := os.ReadFile(path) //nolint:gosec // the operator named this path
	if err != nil {
		return Patch{}, fmt.Errorf("read %s: %w", path, err)
	}

	return ParsePatch(format, content)
}

// documentPatch reads the layer held in one environment variable.
//
// Unknown keys are refused, where viper dropped them without a word. A process
// that will not start says which key it did not recognise; one that starts
// having ignored half its configuration says nothing at all, and the setting
// most likely to be misspelled is the one narrowing what may be run.
func documentPatch() (Patch, error) {
	raw := os.Getenv(EnvConfig)
	if strings.TrimSpace(raw) == "" {
		return Patch{}, nil
	}

	return ParsePatch(documentFormat(raw), []byte(raw))
}

// documentFormat reports how SMYKLOT_CONFIG is written.
//
// Everywhere else in this package the format is told rather than sniffed,
// because a caller always knows which file it read. Here there is nothing to
// tell it with: one variable, and the older spelling already deployed in
// repositories Smyklot cannot edit. A first non-space byte of `{` is
// unambiguous - no TOML document can begin that way, and no JSON object can
// begin any other way.
func documentFormat(raw string) Format {
	if strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return formatJSON
	}

	return FormatTOML
}

// DocumentIsLegacyJSON reports a SMYKLOT_CONFIG still written as JSON.
//
// A repository's configuration file can be migrated by pull request. This
// cannot: it may be an Actions variable, and the App has no permission to
// write those. So the entry points say so at startup and leave the rewrite to
// whoever owns the variable.
func DocumentIsLegacyJSON() bool {
	raw := os.Getenv(EnvConfig)

	return strings.TrimSpace(raw) != "" && documentFormat(raw) == formatJSON
}

// parseBool reads a variable that is either on or off.
//
// Strictly, where viper's cast read anything it did not recognise as false:
// SMYKLOT_QUIET_SUCCESS=yes turned the setting off, which is the opposite of
// what whoever wrote it meant.
func parseBool(key, raw string) (bool, error) {
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("%w for %s: %q is not true or false", ErrInvalidValue, key, raw)
	}

	return value, nil
}

// parseList reads a variable holding a list.
//
// Commas and whitespace both separate, because every list Smyklot takes holds
// command names, which contain neither - and because viper split on whitespace
// while the matching flag splits on commas, so both spellings are already in
// use and neither should start meaning one long entry.
func parseList(raw string) []string {
	return append([]string{}, strings.FieldsFunc(raw, isSeparator)...)
}

// parseMapping reads a variable holding name=value pairs, in the spelling the
// matching flag already takes.
func parseMapping(key, raw string) (map[string]string, error) {
	mapping := make(map[string]string)

	for _, pair := range strings.FieldsFunc(raw, isSeparator) {
		name, value, found := strings.Cut(pair, "=")
		if !found || name == "" || value == "" {
			return nil, fmt.Errorf("%w for %s: %q is not name=value", ErrInvalidValue, key, pair)
		}

		mapping[name] = value
	}

	return mapping, nil
}

func isSeparator(r rune) bool {
	return r == ',' || unicode.IsSpace(r)
}
