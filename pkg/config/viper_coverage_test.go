package config_test

import (
	"reflect"
	"testing"

	"github.com/spf13/viper"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// SetupViper and LoadFromViper are the two enumerations the generator does not
// yet own. A setting added to Patch reaches Config, the keys and the schema on
// its own, but not these - and the failure is silent: the setting simply cannot
// be given through an environment variable, and reads as its zero value
// everywhere. These two tests are what makes that loud instead.

func TestSetupViperDefaultsEveryKey(t *testing.T) {
	v := viper.New()
	config.SetupViper(v)

	for _, key := range config.Keys() {
		if v.Get(key) == nil {
			t.Errorf("SetupViper does not default %q, so the setting has no environment variable", key)
		}
	}
}

func TestLoadFromViperReadsEveryKey(t *testing.T) {
	v := viper.New()
	config.SetupViper(v)

	// Every setting is given a value that differs from its default, so a field
	// LoadFromViper never reads shows up as one that kept the default.
	overrides := map[string]any{
		config.KeyQuietSuccess:           true,
		config.KeyQuietReactions:         true,
		config.KeyQuietPending:           true,
		config.KeyAllowedCommands:        []string{"approve"},
		config.KeyCommandAliases:         map[string]string{"a": "approve"},
		config.KeyCommandPrefix:          "!",
		config.KeyDisableMentions:        true,
		config.KeyDisableBareCommands:    true,
		config.KeyDisableUnapprove:       true,
		config.KeyDisableReactions:       true,
		config.KeyDisableDeletedComments: true,
		config.KeyAllowSelfApproval:      true,
		config.KeyRunner:                 string(config.RunnerAction),
	}

	// The map has to cover every key, or a field could go unread and unnoticed
	// because nothing ever set it.
	if len(overrides) != len(config.Keys()) {
		t.Fatalf("this test overrides %d settings but there are %d", len(overrides), len(config.Keys()))
	}

	for _, key := range config.Keys() {
		value, ok := overrides[key]
		if !ok {
			t.Fatalf("this test has no override for %q", key)
		}

		v.Set(key, value)
	}

	loaded, err := config.LoadFromViper(v)
	if err != nil {
		t.Fatalf("LoadFromViper() error = %v", err)
	}

	defaults := reflect.ValueOf(*config.Default())
	got := reflect.ValueOf(*loaded)

	for index := range got.NumField() {
		name := got.Type().Field(index).Name
		if reflect.DeepEqual(got.Field(index).Interface(), defaults.Field(index).Interface()) {
			t.Errorf("LoadFromViper did not read %s: it kept the default", name)
		}
	}
}
