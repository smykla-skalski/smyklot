package storage

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	installationPrefix = "github:installation:"
	repositoryPrefix   = "github:repository:"
)

func InstallationID(id int64) string {
	return installationPrefix + strconv.FormatInt(id, 10)
}

func RepositoryID(id int64) string {
	return repositoryPrefix + strconv.FormatInt(id, 10)
}

func ParseRepositoryID(id string) (int64, error) {
	numeric, ok := strings.CutPrefix(id, repositoryPrefix)
	if !ok {
		return 0, fmt.Errorf("parse repository id %q: missing GitHub prefix", id)
	}
	parsed, err := strconv.ParseInt(numeric, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("parse repository id %q: invalid GitHub id", id)
	}

	return parsed, nil
}

func RepositoryEnabled(target Target, repository Repository) bool {
	if repository.EnabledOverride != nil {
		return *repository.EnabledOverride
	}

	return target.RepositoryDefaultEnabled
}
