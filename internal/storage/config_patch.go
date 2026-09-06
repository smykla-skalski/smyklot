package storage

import (
	"errors"
	"slices"
	"strings"
	"unicode"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// ValidatePanelPatch is shared by file imports and checkpoint restoration.
// A syntactically valid file still cannot introduce unsupported commands or
// settings whose authority belongs to the repository file itself.
func ValidatePanelPatch(patch config.Patch) error {
	denied := config.PanelDeniedKeys()
	for _, set := range patch.SetKeys() {
		if slices.Contains(denied, set) {
			return errors.New("configuration contains a repository-managed setting")
		}
	}
	if patch.CommandPrefix != nil && (*patch.CommandPrefix == "" || len(*patch.CommandPrefix) > 64 ||
		strings.ContainsFunc(*patch.CommandPrefix, unicode.IsControl)) {
		return errors.New("configuration command prefix is invalid")
	}
	commands := map[string]bool{
		"approve": true, "merge": true, "squash": true, "rebase": true,
		"unapprove": true, "cleanup": true, "help": true,
	}
	if patch.AllowedCommands != nil {
		seen := map[string]bool{}
		for _, command := range *patch.AllowedCommands {
			if !commands[command] || seen[command] {
				return errors.New("configuration allowed commands are invalid")
			}
			seen[command] = true
		}
	}
	if patch.CommandAliases == nil {
		return nil
	}
	if len(*patch.CommandAliases) > 100 {
		return errors.New("configuration has too many command aliases")
	}
	for alias, command := range *patch.CommandAliases {
		if len(alias) == 0 || len(alias) > 64 || !commands[command] || !validPanelAlias(alias) {
			return errors.New("configuration command aliases are invalid")
		}
	}
	return nil
}

func validPanelAlias(alias string) bool {
	for _, character := range alias {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}
