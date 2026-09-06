package configsync

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// WorkspaceFilePath keeps workspace defaults separate from the .github
// repository's own repository settings. The catalog chooses that repository;
// file contents cannot redirect a connection to another repository or path.
const WorkspaceFilePath = ".smyklot/workspace.toml"

type RemoteLocation struct {
	RepositoryID  int64
	Owner         string
	Repository    string
	DefaultBranch string
	Scope         config.PanelFileScope
}

func (location RemoteLocation) paths() ([]string, error) {
	if location.Owner == "" || location.Repository == "" || location.DefaultBranch == "" {
		return nil, errors.New("configuration file connection needs a repository and default branch")
	}
	if location.DefaultBranch == proposalBranch(location.Scope) {
		return nil, &BlockedError{Code: "reserved_branch", Message: "The configuration proposal branch cannot also be the default branch"}
	}
	switch location.Scope {
	case config.PanelFileRepository:
		return slices.Clone(config.RepoConfigPaths), nil
	case config.PanelFileWorkspace:
		return []string{WorkspaceFilePath}, nil
	default:
		return nil, errors.New("unknown configuration file connection scope")
	}
}

// RemoteFile is a complete observation at one immutable default-branch commit.
// Even a missing file has a Head and WritePath; an unreadable tree is an error.
type RemoteFile struct {
	Location  RemoteLocation
	Head      string
	Path      string
	WritePath string
	Ignored   []string
	Source    FileSource
	Content   []byte
	Migrate   bool
}

// BlockedError describes a condition that needs a repository or panel change.
// Network and API failures are returned separately so their retry policy remains
// owned by the GitHub client and durable queue.
type BlockedError struct {
	Code    string
	Message string
}

func (failure *BlockedError) Error() string { return failure.Message }

func ReadRemoteFile(ctx context.Context, client *github.Client, location RemoteLocation) (RemoteFile, error) {
	paths, err := location.paths()
	if err != nil {
		return RemoteFile{}, err
	}
	file := RemoteFile{Location: location, WritePath: paths[0]}
	file.Head, err = client.GetRef(ctx, location.Owner, location.Repository, "heads/"+location.DefaultBranch)
	if err != nil {
		return file, err
	}
	if file.Head == "" {
		return file, &BlockedError{Code: "missing_branch", Message: "The default branch is unavailable"}
	}
	if !validObjectID(file.Head) {
		return file, errors.New("GitHub returned an invalid configuration commit ID")
	}
	found, err := readRemotePaths(ctx, client, location, file.Head, paths)
	if err != nil {
		return file, err
	}
	for _, path := range paths {
		entry := found[path]
		if file.Path != "" {
			if entry.Found {
				file.Ignored = append(file.Ignored, path)
			}
			continue
		}
		if entry.Blocked != "" || (entry.Found && !entry.Entry.OrdinaryFile()) {
			return file, &BlockedError{Code: "file_path_blocked", Message: path + " is not a regular file path"}
		}
		if !entry.Found {
			continue
		}
		file.Path = path
	}
	if file.Path == "" {
		return file, nil
	}
	return readSelectedRemoteFile(ctx, client, file, found[file.Path].Entry)
}

func readSelectedRemoteFile(ctx context.Context, client *github.Client, file RemoteFile, entry github.TreeEntry) (RemoteFile, error) {
	location := file.Location
	if entry.Size > config.MaxFileDocumentBytes {
		return file, &BlockedError{Code: "invalid_file", Message: file.Path + " exceeds the configuration size limit"}
	}
	var err error
	file.Content, err = client.GetFileContent(ctx, location.Owner, location.Repository, file.Path, file.Head, config.MaxFileDocumentBytes)
	if err != nil {
		return file, err
	}
	if file.Content == nil {
		return file, errors.New("configuration tree named a file whose contents could not be read")
	}
	if len(file.Content) != entry.Size || orgsync.BlobID(file.Content) != entry.Blob {
		return file, errors.New("configuration contents do not match the observed commit")
	}
	format, err := config.FormatOf(file.Path)
	if err != nil {
		return file, err
	}
	file.Source, err = ReadFileSource(format, file.Content, location.Scope)
	if err != nil {
		return file, &BlockedError{Code: "invalid_file", Message: fmt.Sprintf("%s could not be read: %v", file.Path, err)}
	}
	file.Migrate = format != config.FormatTOML
	if !file.Migrate {
		file.WritePath = file.Path
	}
	return file, nil
}

func readRemotePaths(
	ctx context.Context, client *github.Client, location RemoteLocation, head string, paths []string,
) (map[string]github.TreePath, error) {
	tree, err := client.ListRepositoryTree(ctx, location.Owner, location.Repository, head)
	if err != nil {
		return nil, err
	}
	if tree.Missing {
		return nil, errors.New("the configuration commit's tree could not be read")
	}
	if tree.Truncated {
		return client.ResolveTreePaths(ctx, location.Owner, location.Repository, head, paths)
	}
	found := make(map[string]github.TreePath, len(paths))
	for _, path := range paths {
		found[path] = tree.At(path)
	}
	return found, nil
}

func invalidSettings(err error) *BlockedError {
	return &BlockedError{Code: "invalid_settings", Message: err.Error()}
}
