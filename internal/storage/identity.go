package storage

import "strconv"

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

func RepositoryEnabled(target Target, repository Repository) bool {
	if repository.EnabledOverride != nil {
		return *repository.EnabledOverride
	}

	return target.RepositoryDefaultEnabled
}
