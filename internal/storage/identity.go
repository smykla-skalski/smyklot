package storage

import "strconv"

// The prefixes rows are keyed under.
//
// A target is not always a GitHub installation and a repository is not always a
// GitHub repository - the panel invites people to accounts that have not
// installed the App yet - so the identifier says which kind it is rather than
// leaving a bare number that two sources could both mint.
const (
	installationPrefix = "github:installation:"
	repositoryPrefix   = "github:repository:"
)

// InstallationID is the target identifier a GitHub installation is stored under.
func InstallationID(id int64) string {
	return installationPrefix + strconv.FormatInt(id, 10)
}

// RepositoryID is the identifier a GitHub repository is stored under.
func RepositoryID(id int64) string {
	return repositoryPrefix + strconv.FormatInt(id, 10)
}

// RepositoryEnabled reports whether the bot acts on a repository.
//
// A repository carries an override or nothing at all, and nothing at all means
// the account's default - so a person switching an account on switches on every
// repository they have not decided about individually, and switching it off
// switches those back off. Neither answer is a fact about whether the
// repository is reachable; check Available for that first.
func RepositoryEnabled(target Target, repository Repository) bool {
	if repository.EnabledOverride != nil {
		return *repository.EnabledOverride
	}

	return target.RepositoryDefaultEnabled
}
