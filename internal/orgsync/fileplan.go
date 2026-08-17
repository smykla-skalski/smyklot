package orgsync

import (
	"crypto/sha1" //nolint:gosec // git names its objects with SHA-1; this is an address, not a signature
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
)

// CurrentFile is one path a repository holds, as its tree describes it.
//
// The object rather than the bytes. A tree lists every path with the id of what
// is at it, and git's id is a hash of the content - so one request answers what
// a repository has and whether each of them already says what it should, where
// the tool this replaces downloaded every file from every repository to find
// out.
type CurrentFile struct {
	Blob string
	Size int

	// Conflict says why this repository cannot hold an ordinary file at this
	// path, and is empty for every ordinary case.
	//
	// A path that is a directory there, one whose parent is a file, a symbolic
	// link, an executable, a submodule: git will let a commit replace any of
	// them with a file and say nothing, and what it replaced is gone. The
	// answer is to refuse the repository until somebody resolves it, because
	// the alternative - leaving that one path alone - is the silence this
	// whole port exists to remove.
	Conflict string
}

// FilePlan is what one repository's files would take.
type FilePlan struct {
	Actions []Action

	// Proposal is the branch the changes go on, named after what the files
	// should end up saying.
	//
	// After the outcome rather than after the change, so a repository where
	// somebody has already fixed one file by hand is still the same proposal
	// and keeps the pull request that is open for it. Naming it after the
	// change would open a second one the moment the first was partly done.
	Proposal string
}

// ResolvedFile is one file with every question answered - rendered for this
// repository and composed with whatever it adjusts - and it is what an action
// carries.
type ResolvedFile struct {
	Path string `json:"path"`

	// Content is bytes rather than a string, so that whatever a template holds
	// survives being written down. A string would be re-encoded as UTF-8 and
	// anything that was not would come back as replacement characters.
	Content []byte `json:"content,omitempty"`

	// Proposal is the branch this repository's file work goes on. Every action
	// in it carries the same one: the plan is what says where the work lands,
	// and re-deriving it at apply time would derive it from a configuration
	// that may have moved.
	Proposal string `json:"proposal"`
}

// DecodeFile reads what an action says to write.
func DecodeFile(payload []byte) (ResolvedFile, error) {
	var file ResolvedFile
	if err := json.Unmarshal(payload, &file); err != nil {
		return ResolvedFile{}, fmt.Errorf("%w: %w", ErrInvalidPlan, err)
	}

	return file, nil
}

// PlanFiles answers what would have to change for a repository to carry the
// files its installation expects.
//
// It fails rather than falling back. A merge that cannot be applied returns an
// error and no actions, so nothing is written: the tool this replaces reported
// the same condition as a warning and wrote the raw template over the
// repository's copy, which is how a renamed heading destroyed a customized
// file.
func PlanFiles(
	repositoryID string,
	config FileConfig,
	override FileOverride,
	defaultBranch string,
	current map[string]CurrentFile,
) (FilePlan, error) {
	exclude := Excludes{Patterns: append(
		slices.Clone(config.Excludes), override.Excludes...)}

	desired, err := resolveFiles(config, override, defaultBranch, exclude)
	if err != nil {
		return FilePlan{}, err
	}

	retired := retiredPresent(config, exclude, current)

	if err := refuseConflicts(desired, retired, current); err != nil {
		return FilePlan{}, err
	}

	proposal := fileProposal(desired, retired)

	plan := FilePlan{Proposal: proposal}

	for _, file := range desired {
		file.Proposal = proposal

		held, present := current[file.Path]

		switch {
		case !present:
			plan.Actions = append(plan.Actions, Action{
				RepositoryID: repositoryID,
				Kind:         KindFiles,
				Operation:    OperationCreate,
				Subject:      file.Path,
				After:        describeFile(len(file.Content), override.MergeFor(file.Path)),
				Payload:      encodeFile(file),
				State:        ActionPending,
			})

		case held.Blob != BlobID(file.Content):
			plan.Actions = append(plan.Actions, Action{
				RepositoryID: repositoryID,
				Kind:         KindFiles,
				Operation:    OperationUpdate,
				Subject:      file.Path,
				Before:       describeSize(held.Size),
				After:        describeFile(len(file.Content), override.MergeFor(file.Path)),
				Payload:      encodeFile(file),
				State:        ActionPending,
			})
		}
	}

	for _, path := range retired {
		plan.Actions = append(plan.Actions, Action{
			RepositoryID: repositoryID,
			Kind:         KindFiles,
			Operation:    OperationDelete,
			Subject:      path,
			Before:       describeSize(current[path].Size),
			Payload:      encodeFile(ResolvedFile{Path: path, Proposal: proposal}),
			State:        ActionPending,
		})
	}

	return plan, nil
}

// refuseConflicts stops before writing a path this repository cannot hold an
// ordinary file at.
//
// The whole repository rather than the one path. Leaving that path alone would
// be a file the configuration names, the panel shows, and nothing ever writes,
// which is the silence this port exists to remove; refusing puts it in a log
// with the reason and leaves the repository unsettled, so it is answered again
// the moment somebody resolves it.
func refuseConflicts(
	desired []ResolvedFile,
	retired []string,
	current map[string]CurrentFile,
) error {
	for _, file := range desired {
		if conflict := current[file.Path].Conflict; conflict != "" {
			return fmt.Errorf("%w: %s", ErrRepositoryConflict, conflict)
		}
	}

	// Removing one matters just as much: a tree entry naming a directory with
	// no object removes the whole directory, not the file somebody retired.
	for _, path := range retired {
		if conflict := current[path].Conflict; conflict != "" {
			return fmt.Errorf("%w: %s", ErrRepositoryConflict, conflict)
		}
	}

	return nil
}

// resolveFiles answers what every managed path should say for one repository.
func resolveFiles(
	config FileConfig,
	override FileOverride,
	defaultBranch string,
	exclude Excludes,
) ([]ResolvedFile, error) {
	resolved := make([]ResolvedFile, 0, len(config.Files))

	for _, file := range config.Files {
		if exclude.Matches(file.Path) {
			continue
		}

		// Rendered before it is composed, so a template's placeholders are
		// filled in whether or not a repository adjusts the file. What a
		// repository writes in its own adjustments is taken literally.
		content, err := filemerge.Apply(
			file.Path,
			[]byte(Render(file.Content, defaultBranch)),
			override.MergeFor(file.Path),
		)
		if err != nil {
			return nil, fmt.Errorf("composing %s: %w", file.Path, err)
		}

		resolved = append(resolved, ResolvedFile{Path: file.Path, Content: content})
	}

	return resolved, nil
}

// retiredPresent is the retired paths a repository still has, sorted.
//
// Sorted, because the answer must not depend on the order a tree happened to
// arrive in: two plans of one state have to be one plan, or the digest that
// decides whether a repository has settled means nothing.
func retiredPresent(
	config FileConfig,
	exclude Excludes,
	current map[string]CurrentFile,
) []string {
	present := make([]string, 0, len(config.Retired))

	for _, path := range config.Retired {
		if exclude.Matches(path) {
			// A repository that asked for a path to be left alone asked about
			// that path, not about why it is being touched. The tool this
			// replaces checked its exclusion list when writing files and not
			// when removing them.
			continue
		}

		if _, held := current[path]; held {
			present = append(present, path)
		}
	}

	slices.Sort(present)

	return present
}

// fileProposal names the branch a repository's file work goes on.
//
// Over what the files should say and what should be gone, which is what makes
// two runs against one configuration the same proposal - and makes a
// configuration that has changed a different one, so a pull request somebody
// closed does not suppress the next thing they are asked about.
func fileProposal(desired []ResolvedFile, retired []string) string {
	sorted := slices.Clone(desired)
	slices.SortFunc(sorted, func(one, other ResolvedFile) int {
		return strings.Compare(one.Path, other.Path)
	})

	sum := sha256.New()

	for _, file := range sorted {
		writeField(sum, "file")
		writeField(sum, file.Path)
		writeField(sum, BlobID(file.Content))
	}

	for _, path := range retired {
		writeField(sum, "retired")
		writeField(sum, path)
	}

	return FileBranchPrefix + hex.EncodeToString(sum.Sum(nil))[:fileBranchDigits]
}

const (
	// FileBranchPrefix is where every file proposal goes. Under smyklot/ so a
	// repository can tell at a glance whose branch it is, and so a ruleset can
	// name the namespace rather than a branch at a time.
	FileBranchPrefix = "smyklot/files-"

	// fileBranchDigits is how much of the fingerprint the branch carries. Twelve
	// hexadecimal digits is 48 bits: enough that two configurations colliding is
	// not a thing that happens, and short enough to read in a branch list.
	fileBranchDigits = 12
)

// BlobID is the name git would give a file's contents.
//
// git hashes the type and length before the bytes, which is why this is not
// the SHA-1 of the file. Computing it here is what lets one listing of a
// repository's tree answer whether every managed file already says what it
// should, without downloading any of them.
func BlobID(content []byte) string {
	sum := sha1.New() //nolint:gosec // the algorithm is git's, and this is an address
	_, _ = fmt.Fprintf(sum, "blob %d\x00", len(content))
	_, _ = sum.Write(content)

	return hex.EncodeToString(sum.Sum(nil))
}

func encodeFile(file ResolvedFile) []byte {
	// A path, some bytes and a branch name cannot fail to encode, and a planner
	// that returned an error here would make every caller handle one that
	// cannot happen.
	payload, _ := json.Marshal(file)

	return payload
}

// describeFile renders what a file would become, for somebody reading the plan.
// It is display, never a value anything branches on.
func describeFile(size int, spec filemerge.Spec) string {
	if spec.Empty() {
		return describeSize(size) + " from the template"
	}

	return describeSize(size) + ", adjusted for this repository"
}

const bytesPerKilobyte = 1024

func describeSize(size int) string {
	if size < bytesPerKilobyte {
		return fmt.Sprintf("%d bytes", size)
	}

	return fmt.Sprintf("%.1f kB", float64(size)/bytesPerKilobyte)
}
